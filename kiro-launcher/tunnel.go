package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fatedier/frp/client"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/util/log"
)

// TunnelConfig 穿透配置
type TunnelConfig struct {
	Enabled       bool   `json:"enabled"`
	ServerAddr    string `json:"serverAddr"`
	ServerPort    int    `json:"serverPort"`
	Token         string `json:"token"`
	ProxyName     string `json:"proxyName"`
	CustomDomain  string `json:"customDomain"`
	RemotePort    int    `json:"remotePort,omitempty"`    // TCP 模式用
	ProxyType     string `json:"proxyType"`               // http / tcp
	VhostHTTPPort int    `json:"vhostHTTPPort,omitempty"` // HTTP 模式服务端 vhost 端口，默认 8080
}

// TunnelStatus 穿透状态
type TunnelStatus struct {
	Running   bool   `json:"running"`
	PublicURL string `json:"publicUrl"`
	Error     string `json:"error,omitempty"`
}

// TunnelManager 穿透管理器
type TunnelManager struct {
	mu        sync.Mutex
	client    *client.Service
	config    *TunnelConfig
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	publicURL string
	lastErr   string
}

func NewTunnelManager() *TunnelManager {
	return &TunnelManager{}
}

// GetTunnelConfigPath 获取穿透配置文件路径
func getTunnelConfigPath() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tunnel.json"), nil
}

// LoadTunnelConfig 加载穿透配置
func (a *App) LoadTunnelConfig() (TunnelConfig, error) {
	path, err := getTunnelConfigPath()
	if err != nil {
		return defaultTunnelConfig(), err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return defaultTunnelConfig(), nil
	}

	var cfg TunnelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultTunnelConfig(), nil
	}
	return cfg, nil
}

// SaveTunnelConfig 保存穿透配置
func (a *App) SaveTunnelConfig(cfg TunnelConfig) (string, error) {
	path, err := getTunnelConfigPath()
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("写入配置失败: %v", err)
	}

	return "穿透配置已保存", nil
}

func defaultTunnelConfig() TunnelConfig {
	return TunnelConfig{
		Enabled:       false,
		ServerAddr:    "",
		ServerPort:    7000,
		Token:         "",
		ProxyName:     "kiro-proxy",
		CustomDomain:  "",
		ProxyType:     "http",
		VhostHTTPPort: 8080,
	}
}

// isOfficialTunnelServer 判断是否使用官方穿透服务器
func isOfficialTunnelServer(cfg TunnelConfig) bool {
	return cfg.ServerAddr == "117.72.183.248" || cfg.CustomDomain == "kiro-proxy.advance123.cn"
}

// StartTunnel 启动穿透
func (a *App) StartTunnel() (string, error) {
	cfg, err := a.LoadTunnelConfig()
	if err != nil {
		return "", err
	}

	// 校验必填字段
	if cfg.ServerAddr == "" {
		return "", fmt.Errorf("请先配置 FRP 服务器地址")
	}
	if cfg.Token == "" {
		return "", fmt.Errorf("请先配置 FRP 认证 Token")
	}
	if cfg.ProxyType == "http" && cfg.CustomDomain == "" {
		return "", fmt.Errorf("HTTP 模式需要配置自定义域名")
	}
	if cfg.ProxyType == "tcp" && cfg.RemotePort == 0 {
		return "", fmt.Errorf("TCP 模式需要配置远程端口")
	}

	// 使用官方服务器时需要验证激活码和穿透权限
	if isOfficialTunnelServer(cfg) {
		if err := a.checkTunnelPermission(); err != nil {
			return "", err
		}
	}

	// 启动时自动启用
	if !cfg.Enabled {
		cfg.Enabled = true
		a.SaveTunnelConfig(cfg)
	}

	proxyCfg, _ := a.GetConfig()

	a.tunnelMu.Lock()
	defer a.tunnelMu.Unlock()

	// 停止已有的穿透
	if a.tunnelCancel != nil {
		a.tunnelCancel()
	}

	// 创建 frp 客户端配置
	clientCfg := &v1.ClientCommonConfig{
		ServerAddr: cfg.ServerAddr,
		ServerPort: cfg.ServerPort,
	}
	clientCfg.Auth.Token = cfg.Token

	// 禁用 frp 日志输出到控制台
	log.InitLogger("", "", 0, false)

	// 创建代理配置
	var proxyCfgs []v1.ProxyConfigurer

	if cfg.ProxyType == "tcp" {
		tcpProxy := &v1.TCPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name: cfg.ProxyName,
				Type: "tcp",
				ProxyBackend: v1.ProxyBackend{
					LocalIP:   proxyCfg.Host,
					LocalPort: proxyCfg.Port,
				},
			},
			RemotePort: cfg.RemotePort,
		}
		proxyCfgs = append(proxyCfgs, tcpProxy)
		a.tunnelPublicURL = fmt.Sprintf("http://%s:%d", cfg.ServerAddr, cfg.RemotePort)
	} else {
		httpProxy := &v1.HTTPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name: cfg.ProxyName,
				Type: "http",
				ProxyBackend: v1.ProxyBackend{
					LocalIP:   "127.0.0.1",
					LocalPort: proxyCfg.Port,
				},
			},
			DomainConfig: v1.DomainConfig{
				CustomDomains: []string{cfg.CustomDomain},
			},
		}
		proxyCfgs = append(proxyCfgs, httpProxy)
		vhostPort := cfg.VhostHTTPPort
		if vhostPort == 0 {
			vhostPort = 8080
		}
		if vhostPort == 80 {
			a.tunnelPublicURL = fmt.Sprintf("http://%s", cfg.CustomDomain)
		} else {
			a.tunnelPublicURL = fmt.Sprintf("http://%s:%d", cfg.CustomDomain, vhostPort)
		}
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	a.tunnelCancel = cancel

	// 创建并启动 frp 客户端
	svc, err := client.NewService(client.ServiceOptions{
		Common:    clientCfg,
		ProxyCfgs: proxyCfgs,
	})
	if err != nil {
		return "", fmt.Errorf("创建穿透客户端失败: %v", err)
	}

	a.tunnelClient = svc
	a.tunnelRunning = true

	// 后台运行
	go func() {
		err := svc.Run(ctx)
		a.tunnelMu.Lock()
		a.tunnelRunning = false
		if err != nil && err != context.Canceled {
			a.tunnelLastErr = err.Error()
		}
		a.tunnelMu.Unlock()
	}()

	return fmt.Sprintf("穿透已启动: %s", a.tunnelPublicURL), nil
}

// StopTunnel 停止穿透
func (a *App) StopTunnel() (string, error) {
	a.tunnelMu.Lock()
	defer a.tunnelMu.Unlock()

	if a.tunnelCancel != nil {
		a.tunnelCancel()
		a.tunnelCancel = nil
	}

	if a.tunnelClient != nil {
		a.tunnelClient.Close()
		a.tunnelClient = nil
	}

	a.tunnelRunning = false
	a.tunnelPublicURL = ""

	return "穿透已停止", nil
}

// GetTunnelStatus 获取穿透状态
func (a *App) GetTunnelStatus() TunnelStatus {
	a.tunnelMu.Lock()
	defer a.tunnelMu.Unlock()

	return TunnelStatus{
		Running:   a.tunnelRunning,
		PublicURL: a.tunnelPublicURL,
		Error:     a.tunnelLastErr,
	}
}

// checkTunnelPermission 检查穿透权限
func (a *App) checkTunnelPermission() error {
	// 读取激活数据
	actData, err := a.CheckActivation()
	if err != nil || !actData.Activated {
		return fmt.Errorf("请先激活软件")
	}

	// 获取当前机器码
	machineId, err := getMachineId()
	if err != nil {
		return fmt.Errorf("获取机器码失败: %v", err)
	}

	// 验证机器码
	if actData.MachineId != machineId {
		return fmt.Errorf("激活码与当前设备不匹配")
	}

	// 从服务器验证穿透权限
	type TunnelCheckResponse struct {
		Success    bool   `json:"success"`
		Message    string `json:"message"`
		TunnelDays int    `json:"tunnelDays"`
		ExpiresAt  string `json:"expiresAt,omitempty"`
	}

	reqBody := map[string]string{
		"code":      actData.Code,
		"machineId": machineId,
	}
	reqData, _ := json.Marshal(reqBody)

	resp, err := http.Post(
		ActivationServer+"/api/tunnel/check",
		"application/json",
		strings.NewReader(string(reqData)),
	)
	if err != nil {
		return fmt.Errorf("无法连接到激活服务器")
	}
	defer resp.Body.Close()

	var result TunnelCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析服务器响应失败")
	}

	if !result.Success {
		return fmt.Errorf(result.Message)
	}

	return nil
}
