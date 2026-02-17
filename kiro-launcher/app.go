package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

const ActivationServer = "http://117.72.183.248:7777"

// ── Data Types ──

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
