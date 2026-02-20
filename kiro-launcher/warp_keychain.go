package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type WarpKeychainData struct {
	IDToken struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
	} `json:"id_token"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	LocalID     string `json:"local_id"`
}

type WarpPlistConfig struct {
	DidNonAnonymousUserLogIn bool   `plist:"DidNonAnonymousUserLogIn"`
	ExperimentId             string `plist:"ExperimentId"`
	IsSettingsSyncEnabled    bool   `plist:"IsSettingsSyncEnabled"`
}

// GetWarpCredentialFromKeychain 从 macOS Keychain 读取 Warp 登录凭证
func (a *App) GetWarpCredentialFromKeychain() (map[string]string, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("此功能仅支持 macOS")
	}

	// 从 Keychain 读取 Warp 凭证（hex 编码）
	cmd := exec.Command("security", "find-generic-password", "-s", "dev.warp.Warp-Stable", "-a", "User", "-w")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("未找到 Warp 登录凭证，请先在 Warp 终端中登录")
	}

	// hex 解码
	hexStr := strings.TrimSpace(string(output))
	jsonBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("解码凭证失败: %v", err)
	}

	// 解析 JSON
	var data WarpKeychainData
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("解析凭证失败: %v", err)
	}

	if data.IDToken.RefreshToken == "" {
		return nil, fmt.Errorf("未找到有效的 refresh token")
	}

	return map[string]string{
		"email":        data.Email,
		"displayName":  data.DisplayName,
		"refreshToken": data.IDToken.RefreshToken,
	}, nil
}

// ImportWarpCredentialFromKeychain 从 Keychain 读取并导入 Warp 凭证到 kiro-proxy
func (a *App) ImportWarpCredentialFromKeychain() (string, error) {
	// 1. 从 Keychain 读取凭证
	cred, err := a.GetWarpCredentialFromKeychain()
	if err != nil {
		return "", err
	}

	// 2. 获取代理配置
	cfg, err := a.GetConfig()
	if err != nil {
		return "", fmt.Errorf("获取代理配置失败: %v", err)
	}

	// 3. 调用 kiro-proxy API 添加凭证
	baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	apiKey := cfg.ApiKey

	payload := map[string]string{
		"name":         cred["displayName"],
		"refreshToken": cred["refreshToken"],
	}

	payloadBytes, _ := json.Marshal(payload)
	cmd := exec.Command("curl", "-X", "POST",
		fmt.Sprintf("%s/api/warp/credentials", baseURL),
		"-H", "Content-Type: application/json",
		"-H", fmt.Sprintf("x-api-key: %s", apiKey),
		"-d", string(payloadBytes),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("导入失败: %v, 输出: %s", err, string(output))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if success, ok := result["success"].(bool); !ok || !success {
		msg := "未知错误"
		if m, ok := result["message"].(string); ok {
			msg = m
		} else if e, ok := result["error"].(string); ok {
			msg = e
		}
		return "", fmt.Errorf("导入失败: %s", msg)
	}

	return fmt.Sprintf("成功导入 Warp 凭证: %s (%s)", cred["displayName"], cred["email"]), nil
}
