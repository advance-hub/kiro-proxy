package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ── Config Commands ──

func (a *App) SaveConfig(host string, port int, apiKey string, region string) (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(dir, "config.json")

	// 读取现有配置，保留 kiro-go 的额外字段
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &existing)
	}

	existing["host"] = host
	existing["port"] = port
	existing["apiKey"] = apiKey
	existing["region"] = region
	if _, ok := existing["tlsBackend"]; !ok {
		existing["tlsBackend"] = "rustls"
	}

	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "", fmt.Errorf("写入配置文件失败: %v", err)
	}

	// 自动更新 Droid 和 OpenCode 配置中的 baseUrl/apiKey
	a.syncExternalConfigs(host, port, apiKey)

	return "配置已保存", nil
}

// syncExternalConfigs 自动更新 Droid (Factory)、OpenCode 和 Claude Code 配置中的代理地址
func (a *App) syncExternalConfigs(host string, port int, apiKey string) {
	baseUrl := fmt.Sprintf("http://%s:%d", host, port)

	// 1. 更新 ~/.factory/config.json (Droid CLI custom_models)
	a.syncFactoryConfig(baseUrl, apiKey)

	// 2. 更新 ~/.factory/settings.json (Droid settings customModels)
	a.syncDroidSettings(baseUrl, apiKey)

	// 3. 更新 ~/.config/opencode/opencode.json
	a.syncOpenCodeConfig(baseUrl, apiKey)

	// 4. 更新 ~/.claude/settings.json (Claude Code)
	a.syncClaudeCodeSettings(baseUrl, apiKey)
}

func (a *App) syncFactoryConfig(baseUrl string, apiKey string) {
	path := getFactoryConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return // 文件不存在则跳过
	}
	var config map[string]interface{}
	if json.Unmarshal(data, &config) != nil {
		return
	}

	models, ok := config["custom_models"].([]interface{})
	if !ok || len(models) == 0 {
		return
	}

	changed := false
	for _, item := range models {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if oldUrl, _ := m["base_url"].(string); oldUrl != "" && oldUrl != baseUrl {
			m["base_url"] = baseUrl
			changed = true
		}
		if oldKey, _ := m["api_key"].(string); oldKey != "" && oldKey != apiKey {
			m["api_key"] = apiKey
			changed = true
		}
	}

	if changed {
		out, _ := json.MarshalIndent(config, "", "  ")
		os.WriteFile(path, out, 0644)
	}
}

func (a *App) syncDroidSettings(baseUrl string, apiKey string) {
	path := getSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var settings map[string]interface{}
	if json.Unmarshal(data, &settings) != nil {
		return
	}

	models, ok := settings["customModels"].([]interface{})
	if !ok || len(models) == 0 {
		return
	}

	changed := false
	for _, item := range models {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if oldUrl, _ := m["baseUrl"].(string); oldUrl != "" && oldUrl != baseUrl {
			m["baseUrl"] = baseUrl
			changed = true
		}
		if oldKey, _ := m["apiKey"].(string); oldKey != "" && oldKey != apiKey {
			m["apiKey"] = apiKey
			changed = true
		}
	}

	if changed {
		out, _ := json.MarshalIndent(settings, "", "  ")
		os.WriteFile(path, out, 0644)
	}
}

func (a *App) syncOpenCodeConfig(baseUrl string, apiKey string) {
	path := getOpenCodeConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var config map[string]interface{}
	if json.Unmarshal(data, &config) != nil {
		return
	}

	providers, ok := config["provider"].(map[string]interface{})
	if !ok || len(providers) == 0 {
		return
	}

	changed := false
	for _, prov := range providers {
		p, ok := prov.(map[string]interface{})
		if !ok {
			continue
		}
		opts, ok := p["options"].(map[string]interface{})
		if !ok {
			continue
		}
		newBaseURL := baseUrl + "/v1"
		if oldURL, _ := opts["baseURL"].(string); oldURL != "" && oldURL != newBaseURL {
			opts["baseURL"] = newBaseURL
			changed = true
		}
		if oldKey, _ := opts["apiKey"].(string); oldKey != "" && oldKey != apiKey {
			opts["apiKey"] = apiKey
			changed = true
		}
	}

	if changed {
		out, _ := json.MarshalIndent(config, "", "  ")
		os.WriteFile(path, out, 0644)
	}
}

func (a *App) syncClaudeCodeSettings(baseUrl string, apiKey string) {
	path := getClaudeCodeSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return // 文件不存在则跳过
	}
	var config map[string]interface{}
	if json.Unmarshal(data, &config) != nil {
		return
	}

	env, ok := config["env"].(map[string]interface{})
	if !ok || len(env) == 0 {
		return
	}

	changed := false
	if oldUrl, _ := env["ANTHROPIC_BASE_URL"].(string); oldUrl != "" && oldUrl != baseUrl {
		env["ANTHROPIC_BASE_URL"] = baseUrl
		changed = true
	}
	if oldKey, _ := env["ANTHROPIC_AUTH_TOKEN"].(string); oldKey != "" && oldKey != apiKey {
		env["ANTHROPIC_AUTH_TOKEN"] = apiKey
		changed = true
	}

	if changed {
		out, _ := json.MarshalIndent(config, "", "  ")
		os.WriteFile(path, out, 0644)
	}
}

func (a *App) GetConfig() (ProxyConfig, error) {
	dir, err := getDataDir()
	if err != nil {
		return ProxyConfig{}, err
	}
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig(), nil
	}
	var cfg ProxyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig(), nil
	}
	return cfg, nil
}

func defaultConfig() ProxyConfig {
	return ProxyConfig{
		Host:       "127.0.0.1",
		Port:       13000,
		ApiKey:     "kiro-proxy-123",
		Region:     "us-east-1",
		TlsBackend: "rustls",
	}
}

func (a *App) GetDataDirPath() (string, error) {
	return getDataDir()
}

func (a *App) OpenDataDir() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("打开目录失败: %v", err)
	}
	return dir, nil
}

// GetBackend 获取当前后端模式
func (a *App) GetBackend() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "kiro", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return "kiro", nil
	}
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		return "kiro", nil
	}
	if b, ok := cfg["backend"].(string); ok && b != "" {
		return b, nil
	}
	return "kiro", nil
}
