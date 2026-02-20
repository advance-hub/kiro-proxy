package model

// KiroCredentials Kiro 凭证
type KiroCredentials struct {
	ID           *int   `json:"id,omitempty"`
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ProfileArn   string `json:"profileArn,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
	AuthMethod   string `json:"authMethod,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Region       string `json:"region,omitempty"`
	AuthRegion   string `json:"authRegion,omitempty"`
	APIRegion    string `json:"apiRegion,omitempty"`
	MachineID    string `json:"machineId,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
	Priority     *int   `json:"priority,omitempty"`
}

func (c *KiroCredentials) EffectiveRegion(cfg *Config) string {
	if c.APIRegion != "" {
		return c.APIRegion
	}
	if c.Region != "" {
		return c.Region
	}
	return cfg.EffectiveAPIRegion()
}

func (c *KiroCredentials) EffectiveAuthRegion(cfg *Config) string {
	if c.AuthRegion != "" {
		return c.AuthRegion
	}
	if c.Region != "" {
		return c.Region
	}
	return cfg.EffectiveAPIRegion()
}

// UserCredentialEntry 用户凭证条目（多用户模式）
type UserCredentialEntry struct {
	ActivationCode string          `json:"activation_code"`
	UserName       string          `json:"user_name,omitempty"`
	Credentials    KiroCredentials `json:"credentials"`
	ExpiresDate    string          `json:"expires_date,omitempty"` // 有效期截止日期
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}
