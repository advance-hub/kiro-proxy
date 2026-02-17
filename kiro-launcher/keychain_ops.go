package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ── Keychain ──

func (a *App) ListKeychainSources() []KeychainSource {
	all := readAllCredentials()
	var results []KeychainSource
	for _, c := range all {
		expired := false
		if t, err := time.Parse(time.RFC3339, c.Token.ExpiresAt); err == nil {
			expired = t.Before(time.Now())
		}
		provider := ""
		if c.Token.Provider != nil {
			provider = *c.Token.Provider
		}
		results = append(results, KeychainSource{
			Source:    c.Source,
			ExpiresAt: c.Token.ExpiresAt,
			HasDevice: c.Device != nil,
			Provider:  provider,
			Expired:   expired,
		})
	}
	if results == nil {
		return []KeychainSource{}
	}
	return results
}

func (a *App) UseKeychainSource(source string) (string, error) {
	all := readAllCredentials()
	for _, c := range all {
		if c.Source == source {
			if err := writeKeychainCredentials(&c); err != nil {
				return "", err
			}
			return fmt.Sprintf("已切换到 %s 凭据", source), nil
		}
	}
	return "", fmt.Errorf("Keychain 中未找到 %s 凭据", source)
}

// ── Factory API Key ──

func (a *App) EnsureFactoryApiKey() (string, error) {
	home, _ := os.UserHomeDir()
	rcPath := filepath.Join(home, ".zshrc")
	exportLine := `export FACTORY_API_KEY="fk-kiro-proxy"`

	if data, err := os.ReadFile(rcPath); err == nil {
		if strings.Contains(string(data), "FACTORY_API_KEY") {
			return "FACTORY_API_KEY 已存在于 ~/.zshrc", nil
		}
	}

	f, err := os.OpenFile(rcPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("打开 ~/.zshrc 失败: %v", err)
	}
	defer f.Close()
	fmt.Fprintf(f, "\n# Kiro Proxy - Factory API Key\n%s\n", exportLine)
	return "已写入 FACTORY_API_KEY 到 ~/.zshrc", nil
}
