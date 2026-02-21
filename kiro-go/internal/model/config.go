package model

import (
	"os"
	"path/filepath"
	"runtime"
)

// Config 服务器配置
type Config struct {
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	APIKey      string   `json:"apiKey"`
	AdminAPIKey string   `json:"adminApiKey"`
	Regions     []string `json:"regions"`

	// Kiro 伪装参数
	KiroVersion   string `json:"kiroVersion"`
	SystemVersion string `json:"systemVersion"`
	NodeVersion   string `json:"nodeVersion"`
	APIRegion     string `json:"apiRegion"`

	// 用户凭证文件路径（空则自动推断到 config.json 同目录）
	UserCredentialsPath string `json:"userCredentialsPath"`

	// 卡密文件路径（空则自动推断到 config.json 同目录）
	CodesPath string `json:"codesPath"`

	// 激活码验证服务地址（app.js，如 http://127.0.0.1:7777）
	ActivationServerURL string `json:"activationServerUrl"`

	// 上下文压缩配置
	ContextCompression    bool   `json:"contextCompression"`    // 是否启用上下文压缩（默认 false）
	CompressionModel      string `json:"compressionModel"`      // 压缩用的模型（默认 claude-haiku-4.5）
	CompressionThreshold  int    `json:"compressionThreshold"`  // 消息数阈值，超过则触发压缩（默认 8）
	CompressionKeepRecent int    `json:"compressionKeepRecent"` // 保留最近几条消息不压缩（默认 6）

	// Backend 选择: "kiro" (默认) | "anthropic"
	Backend          string   `json:"backend"`
	AnthropicAPIKey  string   `json:"anthropicApiKey"`
	AnthropicAPIKeys []string `json:"anthropicApiKeys"`
	AnthropicBaseURL string   `json:"anthropicBaseUrl"`
}

func (c *Config) EffectiveAPIRegion() string {
	if c.APIRegion != "" {
		return c.APIRegion
	}
	if len(c.Regions) > 0 {
		return c.Regions[0]
	}
	return "us-east-1"
}

// Defaults 填充默认值
func (c *Config) Defaults() {
	c.DefaultsWithDir("")
}

// DefaultsWithDir 填充默认值，configDir 为配置文件所在目录
// 如果 configDir 为空，使用平台默认目录（macOS: ~/.kiro-proxy, Linux: /opt/kiro-proxy）
func (c *Config) DefaultsWithDir(configDir string) {
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port == 0 {
		c.Port = 13000
	}
	if c.KiroVersion == "" {
		c.KiroVersion = "1.6.0"
	}
	if c.SystemVersion == "" {
		if runtime.GOOS == "darwin" {
			c.SystemVersion = "darwin"
		} else {
			c.SystemVersion = "linux"
		}
	}
	if c.NodeVersion == "" {
		c.NodeVersion = "v22.12.0"
	}

	// 数据文件路径推断策略：
	// 1. 如果 configDir 非空（kiro-launcher 场景），优先用 configDir
	// 2. 否则使用平台默认目录（macOS: ~/.kiro-proxy, Linux: /opt/kiro-proxy）
	baseDir := configDir
	if baseDir == "" {
		if runtime.GOOS == "darwin" {
			home, _ := os.UserHomeDir()
			baseDir = filepath.Join(home, ".kiro-proxy")
		} else {
			baseDir = "/opt/kiro-proxy"
		}
	}
	if c.UserCredentialsPath == "" {
		c.UserCredentialsPath = filepath.Join(baseDir, "user_credentials.json")
	}
	if c.CodesPath == "" {
		c.CodesPath = filepath.Join(baseDir, "codes.json")
	}
	if c.Backend == "" {
		c.Backend = "kiro"
	}
	if c.CompressionModel == "" {
		c.CompressionModel = "claude-haiku-4.5"
	}
	if c.CompressionThreshold == 0 {
		c.CompressionThreshold = 8
	}
	if c.CompressionKeepRecent == 0 {
		c.CompressionKeepRecent = 6
	}
	if c.AnthropicBaseURL == "" {
		c.AnthropicBaseURL = "https://api.anthropic.com"
	}
}
