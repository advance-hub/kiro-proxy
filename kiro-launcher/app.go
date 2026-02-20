package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// ── Data Types ──

// getActivationServerURL 获取激活服务器地址（优先使用本地 kiro-go，回退到远程 7777）
func getActivationServerURL() string {
	// 优先使用本地 kiro-go (13000端口)，避免与远程 7777 数据串扰
	cfg, err := (&App{}).GetConfig()
	if err == nil && cfg.Host != "" && cfg.Port > 0 {
		return fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	}
	// 回退到默认本地地址
	return "http://127.0.0.1:13000"
}

type ActivationData struct {
	Code      string `json:"code"`
	Activated bool   `json:"activated"`
	MachineId string `json:"machineId"`
	Time      string `json:"time"`
}

type ActivationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ProxyConfig struct {
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	ApiKey      string  `json:"apiKey"`
	Region      string  `json:"region"`
	TlsBackend  string  `json:"tlsBackend"`
	AdminApiKey *string `json:"adminApiKey,omitempty"`
}

type CredentialsFile struct {
	AccessToken  *string `json:"accessToken,omitempty"`
	RefreshToken string  `json:"refreshToken"`
	ExpiresAt    string  `json:"expiresAt"`
	AuthMethod   string  `json:"authMethod"`
	ClientID     *string `json:"clientId,omitempty"`
	ClientSecret *string `json:"clientSecret,omitempty"`
	Region       *string `json:"region,omitempty"`
}

type StatusInfo struct {
	Running        bool        `json:"running"`
	HasCredentials bool        `json:"has_credentials"`
	Config         ProxyConfig `json:"config"`
}

type CredentialsInfo struct {
	Exists       bool   `json:"exists"`
	Source       string `json:"source"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at"`
	AuthMethod   string `json:"auth_method"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Expired      bool   `json:"expired"`
}

type KeychainSource struct {
	Source    string `json:"source"`
	ExpiresAt string `json:"expires_at"`
	HasDevice bool   `json:"has_device"`
	Provider  string `json:"provider"`
	Expired   bool   `json:"expired"`
}

// ── App ──

type App struct {
	ctx      context.Context
	proxyCmd *exec.Cmd
	proxyMu  sync.Mutex
	logs     []string
	logsMu   sync.Mutex
	maxLogs  int

	// Tunnel (frp) 相关
	tunnelMu        sync.Mutex
	tunnelClient    interface{ Close() }
	tunnelCancel    context.CancelFunc
	tunnelRunning   bool
	tunnelPublicURL string
	tunnelLastErr   string
}

func NewApp() *App {
	return &App{maxLogs: 500}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := initLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
	}
	logInfo("kiro-launcher 已启动")
}

func (a *App) shutdown(ctx context.Context) {
	logInfo("kiro-launcher 正在关闭")
	a.stopProxyInternal()
	closeLogger()
}

// ── Helpers ──

func getDataDir() (string, error) {
	var base string
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "Library", "Application Support")
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, "AppData", "Local")
		}
	default:
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, "kiro-launcher")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建数据目录失败: %v", err)
	}
	return dir, nil
}

func mask(s string, visible int) string {
	if len(s) <= visible {
		return s
	}
	end := len(s)
	if end < 4 {
		return s[:visible] + "..."
	}
	return s[:visible] + "..." + s[end-4:]
}

func strPtr(s string) *string { return &s }

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// SaveClaudeCodeConfig 保存 Claude Code 配置到 config.json
func (a *App) SaveClaudeCodeConfig(apiKey, baseUrl string) error {
	// 获取 kiro-go 配置文件路径
	var configPath string
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".kiro-proxy", "config.json")
	} else {
		configPath = "/opt/kiro-proxy/config.json"
	}

	// 确保目录存在
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %v", err)
	}

	// 读取现有配置
	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config map[string]interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("解析配置文件失败: %v", err)
		}
	} else {
		config = make(map[string]interface{})
	}

	// 更新 Claude Code 配置
	config["claudeCodeApiKey"] = apiKey
	config["claudeCodeBaseUrl"] = baseUrl

	// 写回文件
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	logInfo(fmt.Sprintf("Claude Code 配置已保存: baseUrl=%s", baseUrl))
	return nil
}
