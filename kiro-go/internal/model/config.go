package model

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

	// 用户凭证文件路径
	UserCredentialsPath string `json:"userCredentialsPath"`

	// 激活码验证服务地址（app.js，如 http://127.0.0.1:7777）
	ActivationServerURL string `json:"activationServerUrl"`
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

func (c *Config) Defaults() {
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 13000
	}
	if c.KiroVersion == "" {
		c.KiroVersion = "1.6.0"
	}
	if c.SystemVersion == "" {
		c.SystemVersion = "linux"
	}
	if c.NodeVersion == "" {
		c.NodeVersion = "v22.12.0"
	}
	if c.UserCredentialsPath == "" {
		c.UserCredentialsPath = "/opt/kiro-proxy/user_credentials.json"
	}
}
