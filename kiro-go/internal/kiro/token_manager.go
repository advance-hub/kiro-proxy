package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"kiro-go/internal/logger"
	"kiro-go/internal/model"
)

// TokenManager 多凭据 Token 管理器
type TokenManager struct {
	Config      *model.Config
	Credentials []*model.KiroCredentials
	mu          sync.Mutex
	current     int
}

func NewTokenManager(cfg *model.Config, creds []*model.KiroCredentials) *TokenManager {
	return &TokenManager{Config: cfg, Credentials: creds}
}

// AcquireContext 获取一个可用的凭据和 token
// 不在请求路径上刷新 token，直接使用现有 token（即使过期，Kiro API 仍可接受）
// Token 刷新由 kiro-launcher 负责，通过 /api/admin/reload-credentials 热加载
func (tm *TokenManager) AcquireContext() (*model.KiroCredentials, string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(tm.Credentials) == 0 {
		return nil, "", fmt.Errorf("没有可用的凭据")
	}

	// 优先选未过期且未禁用的凭据
	for i := 0; i < len(tm.Credentials); i++ {
		idx := (tm.current + i) % len(tm.Credentials)
		cred := tm.Credentials[idx]
		if cred.Disabled || cred.AccessToken == "" {
			continue
		}
		if !IsTokenExpired(cred) {
			tm.current = (idx + 1) % len(tm.Credentials)
			return cred, cred.AccessToken, nil
		}
	}

	// 没有未过期的，直接用第一个有 accessToken 的凭据（不刷新，不阻塞）
	for i := 0; i < len(tm.Credentials); i++ {
		idx := (tm.current + i) % len(tm.Credentials)
		cred := tm.Credentials[idx]
		if cred.Disabled {
			continue
		}
		if cred.AccessToken != "" {
			tm.current = (idx + 1) % len(tm.Credentials)
			return cred, cred.AccessToken, nil
		}
	}

	return nil, "", fmt.Errorf("所有凭据均无可用 AccessToken（共 %d 个）", len(tm.Credentials))
}

func IsTokenExpired(cred *model.KiroCredentials) bool {
	if cred.ExpiresAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, cred.ExpiresAt)
	if err != nil {
		return true
	}
	return time.Now().After(t.Add(-5 * time.Minute))
}

func IsTokenExpiringSoon(cred *model.KiroCredentials) bool {
	if cred.ExpiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, cred.ExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().After(t.Add(-10 * time.Minute))
}

// RefreshToken 刷新凭证
func RefreshToken(cred *model.KiroCredentials, cfg *model.Config) (*model.KiroCredentials, error) {
	if cred.RefreshToken == "" {
		return nil, fmt.Errorf("缺少 refreshToken")
	}

	authMethod := cred.AuthMethod
	if authMethod == "" {
		if cred.ClientID != "" && cred.ClientSecret != "" {
			authMethod = "idc"
		} else {
			authMethod = "social"
		}
	}

	switch authMethod {
	case "idc", "builder-id", "iam", "BuilderId", "Enterprise":
		return refreshIdcToken(cred, cfg)
	default:
		return refreshSocialToken(cred, cfg)
	}
}

func refreshSocialToken(cred *model.KiroCredentials, cfg *model.Config) (*model.KiroCredentials, error) {
	logger.Infof(logger.CatToken, "正在刷新 Social Token...")

	region := cred.EffectiveAuthRegion(cfg)
	refreshURL := fmt.Sprintf("https://prod.%s.auth.desktop.kiro.dev/refreshToken", region)
	refreshDomain := fmt.Sprintf("prod.%s.auth.desktop.kiro.dev", region)
	machineID := GenerateMachineID(cred, cfg)

	body, _ := json.Marshal(map[string]string{"refreshToken": cred.RefreshToken})

	req, _ := http.NewRequest("POST", refreshURL, bytes.NewReader(body))
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("KiroIDE-%s-%s", cfg.KiroVersion, machineID))
	req.Header.Set("Accept-Encoding", "gzip, compress, deflate, br")
	req.Header.Set("Host", refreshDomain)
	req.Header.Set("Connection", "close")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("刷新请求失败: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		errMsg := "Token 刷新失败"
		switch resp.StatusCode {
		case 401:
			errMsg = "OAuth 凭证已过期或无效，需要重新认证"
		case 403:
			errMsg = "权限不足"
		case 429:
			errMsg = "请求过于频繁"
		}
		return nil, fmt.Errorf("%s: %d %s", errMsg, resp.StatusCode, string(respBody))
	}

	var data struct {
		AccessToken  string  `json:"accessToken"`
		RefreshToken *string `json:"refreshToken"`
		ProfileArn   *string `json:"profileArn"`
		ExpiresIn    *int64  `json:"expiresIn"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, fmt.Errorf("解析刷新响应失败: %v", err)
	}

	newCred := *cred
	newCred.AccessToken = data.AccessToken
	if data.RefreshToken != nil {
		newCred.RefreshToken = *data.RefreshToken
	}
	if data.ProfileArn != nil {
		newCred.ProfileArn = *data.ProfileArn
	}
	if data.ExpiresIn != nil {
		newCred.ExpiresAt = time.Now().Add(time.Duration(*data.ExpiresIn) * time.Second).Format(time.RFC3339)
	}

	logger.Infof(logger.CatToken, "Social Token 刷新成功，有效期至 %s", newCred.ExpiresAt)
	return &newCred, nil
}

func refreshIdcToken(cred *model.KiroCredentials, cfg *model.Config) (*model.KiroCredentials, error) {
	logger.Infof(logger.CatToken, "正在刷新 IdC Token...")

	region := cred.EffectiveAuthRegion(cfg)
	refreshURL := fmt.Sprintf("https://oidc.%s.amazonaws.com/token", region)

	body, _ := json.Marshal(map[string]string{
		"clientId":     cred.ClientID,
		"clientSecret": cred.ClientSecret,
		"refreshToken": cred.RefreshToken,
		"grantType":    "refresh_token",
	})

	req, _ := http.NewRequest("POST", refreshURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", fmt.Sprintf("oidc.%s.amazonaws.com", region))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/3.738.0 ua/2.1 os/other lang/js md/browser#unknown_unknown api/sso-oidc#3.738.0 m/E KiroIDE")
	req.Header.Set("User-Agent", "node")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IdC 刷新请求失败: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("IdC Token 刷新失败: %d %s", resp.StatusCode, string(respBody))
	}

	var data struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int64  `json:"expiresIn"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, fmt.Errorf("解析 IdC 刷新响应失败: %v", err)
	}

	newCred := *cred
	newCred.AccessToken = data.AccessToken
	if data.RefreshToken != "" {
		newCred.RefreshToken = data.RefreshToken
	}
	if data.ExpiresIn > 0 {
		newCred.ExpiresAt = time.Now().Add(time.Duration(data.ExpiresIn) * time.Second).Format(time.RFC3339)
	}

	logger.Infof(logger.CatToken, "IdC Token 刷新成功，有效期至 %s", newCred.ExpiresAt)
	return &newCred, nil
}
