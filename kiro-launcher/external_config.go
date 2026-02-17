package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ── Factory Config ──

func getFactoryConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".factory", "config.json")
}

func getSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".factory", "settings.json")
}

func (a *App) ReadFactoryConfig() (map[string]interface{}, error) {
	path := getFactoryConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"custom_models": []interface{}{}}, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析 config.json 失败: %v", err)
	}
	return result, nil
}

func (a *App) WriteFactoryConfig(config map[string]interface{}) (string, error) {
	path := getFactoryConfigPath()
	if dir := filepath.Dir(path); dir != "" {
		os.MkdirAll(dir, 0755)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化 config 失败: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("写入 config.json 失败: %v", err)
	}
	return "config.json 已保存", nil
}

func (a *App) ReadDroidSettings() (map[string]interface{}, error) {
	path := getSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{}, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析 settings.json 失败: %v", err)
	}
	return result, nil
}

func (a *App) WriteDroidSettings(settings map[string]interface{}) (string, error) {
	path := getSettingsPath()
	if dir := filepath.Dir(path); dir != "" {
		os.MkdirAll(dir, 0755)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化 settings 失败: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("写入 settings.json 失败: %v", err)
	}
	return "Settings 已保存", nil
}

// ── OpenCode Config ──

func getOpenCodeConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

func (a *App) ReadOpenCodeConfig() (map[string]interface{}, error) {
	path := getOpenCodeConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{}, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析 opencode.json 失败: %v", err)
	}
	return result, nil
}

func (a *App) WriteOpenCodeConfig(config map[string]interface{}) (string, error) {
	path := getOpenCodeConfigPath()
	if dir := filepath.Dir(path); dir != "" {
		os.MkdirAll(dir, 0755)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化 opencode config 失败: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("写入 opencode.json 失败: %v", err)
	}
	return "opencode.json 已保存", nil
}

// ── Claude Code Settings ──

func getClaudeCodeSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func (a *App) ReadClaudeCodeSettings() (map[string]interface{}, error) {
	path := getClaudeCodeSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{}, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析 Claude Code settings.json 失败: %v", err)
	}
	return result, nil
}

func (a *App) WriteClaudeCodeSettings(config map[string]interface{}) (string, error) {
	path := getClaudeCodeSettingsPath()
	if dir := filepath.Dir(path); dir != "" {
		os.MkdirAll(dir, 0755)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化 Claude Code settings 失败: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("写入 Claude Code settings.json 失败: %v", err)
	}
	return "Claude Code settings.json 已保存", nil
}
