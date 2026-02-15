package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/client"
)

// TunnelConfig 穿透配置
type TunnelConfig struct {
	Enabled      bool   `json:"enabled"`
	ServerAddr   string `json:"serverAddr"`
	ServerPort   int    `json:"serverPort"`
	Token        string `json:"token"`
	ProxyName    string `json:"proxyName"`
	CustomDomain string `json:"customDomain"`
	RemotePort   int    `json:"remotePort,omitempty"` // TCP 模式用
	ProxyType    string `json:"proxyType"`            // http / tcp
}

// TunnelStatus 穿透状态
type TunnelStatus struct {
	Running    bool   `json:"running"`
	PublicURL  string `json:"publicUrl"`
	Error      string `json:"error,omitempty"`
}

// TunnelManager 穿透管理器
type TunnelManager struct {
	mu       sync.Mutex
	client   *client.Service
	config   *TunnelConfig
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
	publicURL string
	lastErr  string
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
		Enabled:      false,
		ServerAddr:   "117.72.183.248",
		ServerPort:   7000,
		Token:        "kiro-proxy",
		ProxyName:    "kiro-proxy",
		CustomDomain: "kiro-proxy.advance123.cn",
		ProxyType:    "http",
	}
}

// StartTunnel 启动穿透
func (a *App) StartTunnel() (string, error) {
	cfg, err := a.LoadTunnelConfig()
	if err != nil {
		return "", err
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
		a.tunnelPublicURL = fmt.Sprintf("http://%s:8080", cfg.CustomDomain)
	}
	
	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	a.tunnelCancel = cancel
	
	// 创建并启动 frp 客户端
	svc, err := client.NewService(client.ServiceOptions{
		Common:      clientCfg,
		ProxyCfgs:   proxyCfgs,
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
