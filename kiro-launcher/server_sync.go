package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ServerSyncConfig 服务器同步配置
type ServerSyncConfig struct {
	ServerURL      string `json:"serverUrl"`
	ActivationCode string `json:"activationCode"`
}

// UploadCredentialsToServer 上传凭证到服务器
func (a *App) UploadCredentialsToServer(serverURL, activationCode, userName string) (string, error) {
	if serverURL == "" {
		return "", fmt.Errorf("服务器地址不能为空")
	}
	if activationCode == "" {
		return "", fmt.Errorf("激活码不能为空")
	}

	// 1. 读取本地凭证
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	credsPath := filepath.Join(dir, "credentials.json")

	data, err := os.ReadFile(credsPath)
	if err != nil {
		return "", fmt.Errorf("读取凭证失败: %v", err)
	}

	var rawCreds map[string]interface{}
	if err := json.Unmarshal(data, &rawCreds); err != nil {
		return "", fmt.Errorf("解析凭证失败: %v", err)
	}

	// 2. 构造凭证对象（包含所有字段，确保服务器能独立刷新 Token）
	credentials := map[string]interface{}{
		"accessToken":  rawCreds["accessToken"],
		"refreshToken": rawCreds["refreshToken"],
		"expiresAt":    rawCreds["expiresAt"],
	}
	if v, ok := rawCreds["authMethod"].(string); ok && v != "" {
		credentials["authMethod"] = v
	}
	if v, ok := rawCreds["clientId"].(string); ok && v != "" {
		credentials["clientId"] = v
	}
	if v, ok := rawCreds["clientSecret"].(string); ok && v != "" {
		credentials["clientSecret"] = v
	}
	if v, ok := rawCreds["region"].(string); ok && v != "" {
		credentials["region"] = v
	}

	// 3. 构造请求
	payload := map[string]interface{}{
		"activation_code": activationCode,
		"credentials":     credentials,
	}

	if userName != "" {
		payload["user_name"] = userName
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	// 4. 发送请求
	url := serverURL + "/api/admin/user-credentials"
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("上传失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("服务器返回错误 (%d): %s", resp.StatusCode, string(body))
	}

	return fmt.Sprintf("凭证上传成功！激活码: %s", activationCode), nil
}

// GetServerSyncConfig 获取服务器同步配置
func (a *App) GetServerSyncConfig() (*ServerSyncConfig, error) {
	dir, err := getDataDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(dir, "server_sync.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &ServerSyncConfig{}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config ServerSyncConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveServerSyncConfig 保存服务器同步配置
func (a *App) SaveServerSyncConfig(serverURL, activationCode string) error {
	dir, err := getDataDir()
	if err != nil {
		return err
	}

	config := ServerSyncConfig{
		ServerURL:      serverURL,
		ActivationCode: activationCode,
	}

	data, err := json.Marshal(config)
	if err != nil {
		return err
	}

	configPath := filepath.Join(dir, "server_sync.json")
	return os.WriteFile(configPath, data, 0644)
}

// TestServerConnection 测试服务器连接
func (a *App) TestServerConnection(serverURL string) (string, error) {
	if serverURL == "" {
		return "", fmt.Errorf("服务器地址不能为空")
	}

	url := serverURL + "/api/admin/user-credentials/stats"
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("连接失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("服务器返回错误: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var stats map[string]interface{}
	if err := json.Unmarshal(body, &stats); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	totalUsers := 0
	if total, ok := stats["total_users"].(float64); ok {
		totalUsers = int(total)
	}

	return fmt.Sprintf("连接成功！当前用户数: %d", totalUsers), nil
}
