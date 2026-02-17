package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ── Credentials ──

func (a *App) GetCredentialsInfo() (CredentialsInfo, error) {
	dir, err := getDataDir()
	if err != nil {
		return CredentialsInfo{}, err
	}
	credsPath := filepath.Join(dir, "credentials.json")

	source := "none"
	var cf *CredentialsFile

	if c, readErr := readFirstCredential(credsPath); readErr == nil {
		source = "file"
		cf = c
	}

	if cf == nil {
		if kc, err := readKiroCredentials(); err == nil {
			source = "keychain"
			hasDevice := kc.Device != nil && kc.Device.ClientID != "" && kc.Device.ClientSecret != ""
			authMethod := "social"
			if hasDevice {
				authMethod = "idc"
			}
			var clientID, clientSecret *string
			if kc.Device != nil {
				clientID = &kc.Device.ClientID
				clientSecret = &kc.Device.ClientSecret
			}
			cf = &CredentialsFile{
				AccessToken:  &kc.Token.AccessToken,
				RefreshToken: kc.Token.RefreshToken,
				ExpiresAt:    kc.Token.ExpiresAt,
				AuthMethod:   authMethod,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				Region:       &kc.Token.Region,
			}
		}
	}

	if cf == nil {
		return CredentialsInfo{Exists: false, Source: source}, nil
	}

	expired := false
	if t, err := time.Parse(time.RFC3339, cf.ExpiresAt); err == nil {
		expired = t.Before(time.Now())
	}

	return CredentialsInfo{
		Exists:       true,
		Source:       source,
		AccessToken:  mask(derefStr(cf.AccessToken), 8),
		RefreshToken: mask(cf.RefreshToken, 8),
		ExpiresAt:    cf.ExpiresAt,
		AuthMethod:   cf.AuthMethod,
		ClientID:     mask(derefStr(cf.ClientID), 8),
		ClientSecret: mask(derefStr(cf.ClientSecret), 8),
		Expired:      expired,
	}, nil
}

func (a *App) ImportCredentials(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("文件不存在: %s", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}
	var v json.RawMessage
	if json.Unmarshal(content, &v) != nil {
		return "", fmt.Errorf("文件不是有效 JSON")
	}
	dir, _ := getDataDir()
	dest := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(dest, content, 0644); err != nil {
		return "", fmt.Errorf("写入凭据失败: %v", err)
	}
	go autoUploadToServer()
	return "凭据已导入", nil
}

func (a *App) SaveCredentialsRaw(jsonStr string) (string, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return "", fmt.Errorf("JSON 格式错误: %v", err)
	}
	if _, ok := parsed["refreshToken"]; !ok {
		if _, ok := parsed["refresh_token"]; !ok {
			return "", fmt.Errorf("缺少 refreshToken 字段")
		}
	}
	dir, _ := getDataDir()
	dest := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(dest, []byte(jsonStr), 0644); err != nil {
		return "", fmt.Errorf("写入失败: %v", err)
	}
	go autoUploadToServer()
	return "凭据已保存", nil
}

func (a *App) ReadCredentialsRaw() (string, error) {
	dir, _ := getDataDir()
	path := filepath.Join(dir, "credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("凭据文件不存在")
	}
	return string(data), nil
}

func (a *App) ClearCredentials() (string, error) {
	dir, _ := getDataDir()
	path := filepath.Join(dir, "credentials.json")
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("删除凭据文件失败: %v", err)
		}
	}
	return "凭据已清空", nil
}

func (a *App) RefreshNow() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	credsPath := filepath.Join(dir, "credentials.json")

	// Try keychain first
	if kc, err := readKiroCredentials(); err == nil {
		writeKeychainCredentials(kc)
	}

	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		return "", fmt.Errorf("凭据文件不存在")
	}

	cf, err := readFirstCredential(credsPath)
	if err != nil {
		return "", fmt.Errorf("读取凭据失败: %v", err)
	}

	refreshed, err := refreshCredentials(cf)
	if err != nil {
		return "", err
	}
	saveCredentialsFileSmart(refreshed)

	// 自动导出环境变量到 shell rc
	cfg, cfgErr := a.GetConfig()
	if cfgErr == nil {
		baseURL := fmt.Sprintf("http://%s:%d/v1", cfg.Host, cfg.Port)
		if err := a.ExportCredsToShellRC(baseURL); err != nil {
			a.pushLog(fmt.Sprintf("警告: 导出环境变量失败: %v", err))
		} else {
			a.pushLog("已自动更新 shell 环境变量")
		}
	}

	// 自动上传凭证到服务器（如果配置了服务器同步）
	if syncCfg, err := a.GetServerSyncConfig(); err == nil && syncCfg.ServerURL != "" && syncCfg.ActivationCode != "" {
		serverURL := syncCfg.ServerURL
		activationCode := syncCfg.ActivationCode

		a.pushLog(fmt.Sprintf("正在上传凭证到服务器: %s", serverURL))
		if msg, err := a.UploadCredentialsToServer(serverURL, activationCode, ""); err != nil {
			a.pushLog(fmt.Sprintf("警告: 上传凭证失败: %v", err))
		} else {
			a.pushLog(msg)
		}
	}

	return fmt.Sprintf("Token 已刷新，有效期至 %s", refreshed.ExpiresAt), nil
}

// ── Token Refresh ──

type socialRefreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

type socialRefreshResp struct {
	AccessToken  string  `json:"accessToken"`
	RefreshToken *string `json:"refreshToken,omitempty"`
	ExpiresIn    *int64  `json:"expiresIn,omitempty"`
}

type idcRefreshReq struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	RefreshToken string `json:"refreshToken"`
	GrantType    string `json:"grantType"`
}

type idcRefreshResp struct {
	AccessToken  string  `json:"accessToken"`
	RefreshToken *string `json:"refreshToken,omitempty"`
	ExpiresIn    *int64  `json:"expiresIn,omitempty"`
}

func refreshCredentials(creds *CredentialsFile) (*CredentialsFile, error) {
	// 配置 HTTP 传输层，增加超时设置
	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	region := "us-east-1"
	if creds.Region != nil {
		region = *creds.Region
	}

	updated := *creds

	if creds.AuthMethod == "idc" {
		if creds.ClientID == nil {
			return nil, fmt.Errorf("IdC 刷新需要 clientId")
		}
		if creds.ClientSecret == nil {
			return nil, fmt.Errorf("IdC 刷新需要 clientSecret")
		}
		apiUrl := fmt.Sprintf("https://oidc.%s.amazonaws.com/token", region)

		payload := idcRefreshReq{
			ClientID:     *creds.ClientID,
			ClientSecret: *creds.ClientSecret,
			RefreshToken: creds.RefreshToken,
			GrantType:    "refresh_token",
		}
		jsonData, _ := json.Marshal(payload)

		req, _ := http.NewRequest("POST", apiUrl, strings.NewReader(string(jsonData)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-amz-user-agent", "aws-sdk-js/3.738.0 ua/2.1 os/other lang/js md/browser#unknown_unknown api/sso-oidc#3.738.0 m/E KiroIDE")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("IdC 刷新请求失败: %v", err)
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("IdC Token 刷新失败 (%d): %s", resp.StatusCode, string(respBody))
		}
		var data idcRefreshResp
		if err := json.Unmarshal(respBody, &data); err != nil {
			return nil, fmt.Errorf("解析刷新响应失败: %v, body: %s", err, string(respBody))
		}
		updated.AccessToken = &data.AccessToken
		if data.RefreshToken != nil {
			updated.RefreshToken = *data.RefreshToken
		}
		if data.ExpiresIn != nil {
			expiresAt := time.Now().Add(time.Duration(*data.ExpiresIn) * time.Second).Format(time.RFC3339)
			updated.ExpiresAt = expiresAt
		}
	} else {
		// Social refresh
		url := fmt.Sprintf("https://prod.%s.auth.desktop.kiro.dev/refreshToken", region)
		body := socialRefreshReq{RefreshToken: creds.RefreshToken}
		bodyData, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", url, strings.NewReader(string(bodyData)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Social 刷新请求失败: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("Social Token 刷新失败 (%d): %s", resp.StatusCode, string(respBody))
		}
		var data socialRefreshResp
		json.NewDecoder(resp.Body).Decode(&data)
		updated.AccessToken = &data.AccessToken
		if data.RefreshToken != nil {
			updated.RefreshToken = *data.RefreshToken
		}
		if data.ExpiresIn != nil {
			expiresAt := time.Now().Add(time.Duration(*data.ExpiresIn) * time.Second).Format(time.RFC3339)
			updated.ExpiresAt = expiresAt
		}
	}

	return &updated, nil
}

func saveCredentialsFile(creds *CredentialsFile) error {
	dir, err := getDataDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "credentials.json")
	data, _ := json.MarshalIndent(creds, "", "  ")
	return os.WriteFile(path, data, 0644)
}

// saveCredentialsFileSmart 智能写入 credentials.json
//
// 如果现有文件是数组格式（多凭据），则替换第一个凭据（保留其他凭据不变）。
// 如果现有文件是单对象格式或不存在，则直接写入单对象。
func saveCredentialsFileSmart(creds *CredentialsFile) error {
	dir, err := getDataDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "credentials.json")

	// 尝试读取现有文件，判断格式
	existing, readErr := os.ReadFile(path)
	if readErr == nil && len(existing) > 0 {
		trimmed := strings.TrimSpace(string(existing))
		if len(trimmed) > 0 && trimmed[0] == '[' {
			// 数组格式：解析为 []map，替换第一个条目
			var arr []map[string]interface{}
			if json.Unmarshal(existing, &arr) == nil {
				// 将新凭据序列化为 map
				newData, _ := json.Marshal(creds)
				var newMap map[string]interface{}
				json.Unmarshal(newData, &newMap)

				if len(arr) > 0 {
					// 替换第一个条目（保留其 id 和 priority）
					if id, ok := arr[0]["id"]; ok {
						newMap["id"] = id
					}
					if priority, ok := arr[0]["priority"]; ok {
						newMap["priority"] = priority
					}
					arr[0] = newMap
				} else {
					arr = append(arr, newMap)
				}

				data, _ := json.MarshalIndent(arr, "", "  ")
				return os.WriteFile(path, data, 0644)
			}
		}
	}

	// 单对象格式或文件不存在：直接写入
	data, _ := json.MarshalIndent(creds, "", "  ")
	return os.WriteFile(path, data, 0644)
}

// readFirstCredential 从 credentials.json 读取第一个凭据
//
// 支持单对象和数组格式。数组格式时返回第一个元素。
func readFirstCredential(path string) (*CredentialsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("凭据文件为空")
	}

	if trimmed[0] == '[' {
		// 数组格式
		var arr []CredentialsFile
		if err := json.Unmarshal(data, &arr); err != nil {
			return nil, fmt.Errorf("解析凭据数组失败: %v", err)
		}
		if len(arr) == 0 {
			return nil, fmt.Errorf("凭据数组为空")
		}
		return &arr[0], nil
	}

	// 单对象格式
	var cf CredentialsFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("解析凭据失败: %v", err)
	}
	return &cf, nil
}
