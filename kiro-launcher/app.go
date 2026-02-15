package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const ActivationServer = "http://117.72.183.248:7777"

// ── Data Types ──

type ActivationData struct {
	Code      string `json:"code"`
	Activated bool   `json:"activated"`
	MachineId string `json:"machineId"`
	Time      string `json:"time"`
}

type ActivationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ProxyConfig struct {
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	ApiKey      string  `json:"apiKey"`
	Region      string  `json:"region"`
	TlsBackend  string  `json:"tlsBackend"`
	AdminApiKey *string `json:"adminApiKey,omitempty"`
}

type CredentialsFile struct {
	AccessToken  *string `json:"accessToken,omitempty"`
	RefreshToken string  `json:"refreshToken"`
	ExpiresAt    string  `json:"expiresAt"`
	AuthMethod   string  `json:"authMethod"`
	ClientID     *string `json:"clientId,omitempty"`
	ClientSecret *string `json:"clientSecret,omitempty"`
	Region       *string `json:"region,omitempty"`
}

type StatusInfo struct {
	Running        bool        `json:"running"`
	HasCredentials bool        `json:"has_credentials"`
	Config         ProxyConfig `json:"config"`
}

type CredentialsInfo struct {
	Exists       bool   `json:"exists"`
	Source       string `json:"source"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at"`
	AuthMethod   string `json:"auth_method"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Expired      bool   `json:"expired"`
}

type KeychainSource struct {
	Source    string `json:"source"`
	ExpiresAt string `json:"expires_at"`
	HasDevice bool   `json:"has_device"`
	Provider  string `json:"provider"`
	Expired   bool   `json:"expired"`
}

// ── App ──

type App struct {
	ctx      context.Context
	proxyCmd *exec.Cmd
	proxyMu  sync.Mutex
	logs     []string
	logsMu   sync.Mutex
	maxLogs  int

	// Tunnel (frp) 相关
	tunnelMu        sync.Mutex
	tunnelClient    interface{ Close() }
	tunnelCancel    context.CancelFunc
	tunnelRunning   bool
	tunnelPublicURL string
	tunnelLastErr   string
}

func NewApp() *App {
	return &App{maxLogs: 500}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	a.stopProxyInternal()
}

// ── Helpers ──

func getDataDir() (string, error) {
	var base string
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "Library", "Application Support")
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, "AppData", "Local")
		}
	default:
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, "kiro-launcher")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建数据目录失败: %v", err)
	}
	return dir, nil
}

func mask(s string, visible int) string {
	if len(s) <= visible {
		return s
	}
	end := len(s)
	if end < 4 {
		return s[:visible] + "..."
	}
	return s[:visible] + "..." + s[end-4:]
}

func strPtr(s string) *string { return &s }

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ── Config Commands ──

func (a *App) SaveConfig(host string, port int, apiKey string, region string) (string, error) {
	cfg := ProxyConfig{
		Host:       host,
		Port:       port,
		ApiKey:     apiKey,
		Region:     region,
		TlsBackend: "rustls",
	}
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0644); err != nil {
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

// ── Proxy Management ──

func (a *App) resolveBinary() (string, error) {
	// 1. Try extracting embedded binary (production mode)
	if p, err := extractSidecar(); err == nil {
		return p, nil
	}

	// 2. Dev mode: try workspace paths
	devPaths := []string{
		filepath.Join("..", "target", "release", "kiro-rs"),
		filepath.Join("..", "target", "debug", "kiro-rs"),
		filepath.Join("..", "kiro.rs", "target", "release", "kiro-rs"),
		filepath.Join("..", "kiro.rs", "target", "debug", "kiro-rs"),
	}
	for _, p := range devPaths {
		abs, _ := filepath.Abs(p)
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}

	// 3. Try PATH
	if p, err := exec.LookPath("kiro-rs"); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("找不到 kiro-rs 二进制，请先 build kiro.rs 项目")
}

func (a *App) pushLog(line string) {
	a.logsMu.Lock()
	defer a.logsMu.Unlock()
	if len(a.logs) >= a.maxLogs {
		a.logs = a.logs[1:]
	}
	a.logs = append(a.logs, line)
}

func (a *App) spawnReader(r io.Reader) {
	go func() {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)
		for scanner.Scan() {
			a.pushLog(scanner.Text())
		}
	}()
}

func (a *App) StartProxy() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(dir, "config.json")
	credsPath := filepath.Join(dir, "credentials.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "", fmt.Errorf("配置文件不存在，请先保存配置")
	}
	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		return "", fmt.Errorf("凭据文件不存在，请先登录或获取凭据")
	}

	if err := a.startProxyInternal(configPath, credsPath); err != nil {
		return "", err
	}
	cfg, _ := a.GetConfig()
	return fmt.Sprintf("代理已启动: http://%s:%d", cfg.Host, cfg.Port), nil
}

func (a *App) startProxyInternal(configPath, credsPath string) error {
	a.stopProxyInternal()

	a.logsMu.Lock()
	a.logs = nil
	a.logsMu.Unlock()

	binary, err := a.resolveBinary()
	if err != nil {
		return err
	}

	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()

	cmd := exec.Command(binary, "-c", configPath, "--credentials", credsPath)
	cmd.Env = append(os.Environ(), "RUST_LOG=info")

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 kiro-rs 失败: %v", err)
	}

	a.spawnReader(stdout)
	a.spawnReader(stderr)

	// Wait briefly to detect immediate crash
	time.Sleep(500 * time.Millisecond)
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		a.logsMu.Lock()
		tail := a.logs
		a.logsMu.Unlock()
		detail := strings.Join(tail, "\n")
		return fmt.Errorf("kiro-rs 启动后立即退出\n%s", detail)
	}

	a.proxyCmd = cmd
	return nil
}

func (a *App) StopProxy() (string, error) {
	a.stopProxyInternal()
	return "代理已停止", nil
}

func (a *App) stopProxyInternal() {
	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()
	if a.proxyCmd != nil && a.proxyCmd.Process != nil {
		_ = a.proxyCmd.Process.Kill()
		_ = a.proxyCmd.Wait()
		a.proxyCmd = nil
	}
}

func (a *App) isRunning() bool {
	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()
	if a.proxyCmd == nil || a.proxyCmd.Process == nil {
		return false
	}
	// Check if process is still running
	if a.proxyCmd.ProcessState != nil {
		return false
	}
	return true
}

func (a *App) GetStatus() (StatusInfo, error) {
	cfg, _ := a.GetConfig()
	dir, _ := getDataDir()
	credsPath := filepath.Join(dir, "credentials.json")
	_, err := os.Stat(credsPath)
	return StatusInfo{
		Running:        a.isRunning(),
		HasCredentials: err == nil,
		Config:         cfg,
	}, nil
}

func (a *App) GetProxyLogs() []string {
	a.logsMu.Lock()
	defer a.logsMu.Unlock()
	result := make([]string, len(a.logs))
	copy(result, a.logs)
	return result
}

// ── One Click Start ──

func (a *App) OneClickStart() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	credsPath := filepath.Join(dir, "credentials.json")

	// 1. Try keychain
	creds, kcErr := readKiroCredentials()
	if kcErr == nil {
		if err := writeKeychainCredentials(creds); err != nil {
			// non-fatal
			fmt.Fprintf(os.Stderr, "写入 keychain 凭据失败: %v\n", err)
		}
	} else {
		if _, err := os.Stat(credsPath); os.IsNotExist(err) {
			return "", fmt.Errorf("未找到凭据，请先通过 Kiro IDE 登录: %v", kcErr)
		}
	}

	// 2. Refresh token
	if _, err := os.Stat(credsPath); err == nil {
		if cf, readErr := readFirstCredential(credsPath); readErr == nil {
			refreshed, err := refreshCredentials(cf)
			if err == nil {
				saveCredentialsFileSmart(refreshed)
			} else {
				fmt.Fprintf(os.Stderr, "Token 刷新失败 (不影响启动): %v\n", err)
			}
		}
	}

	// 3. Ensure config exists
	configPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := defaultConfig()
		data, _ := json.MarshalIndent(cfg, "", "  ")
		os.WriteFile(configPath, data, 0644)
	}

	cfg, _ := a.GetConfig()
	if err := a.startProxyInternal(configPath, credsPath); err != nil {
		return "", err
	}
	return fmt.Sprintf("代理已启动: http://%s:%d", cfg.Host, cfg.Port), nil
}

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
	return fmt.Sprintf("Token 已刷新，有效期至 %s", refreshed.ExpiresAt), nil
}

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

// ── Activation ──

func getActivationPath() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "activation.json"), nil
}

func (a *App) CheckActivation() (ActivationData, error) {
	path, err := getActivationPath()
	if err != nil {
		return ActivationData{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ActivationData{Activated: false}, nil
	}
	var ad ActivationData
	if err := json.Unmarshal(data, &ad); err != nil {
		return ActivationData{Activated: false}, nil
	}
	return ad, nil
}

func (a *App) Activate(code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", fmt.Errorf("请输入激活码")
	}

	mid, err := getMachineId()
	if err != nil {
		return "", fmt.Errorf("获取机器码失败: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	reqBody, _ := json.Marshal(map[string]string{"code": code, "machineId": mid})
	url := ActivationServer + "/api/activate"

	resp, err := client.Post(url, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("无法连接激活服务器: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result ActivationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("服务器响应异常")
	}

	if !result.Success {
		return "", fmt.Errorf("%s", result.Message)
	}

	ad := ActivationData{
		Code:      code,
		Activated: true,
		MachineId: mid,
		Time:      time.Now().Format(time.RFC3339),
	}
	path, err := getActivationPath()
	if err != nil {
		return "", err
	}
	adData, _ := json.MarshalIndent(ad, "", "  ")
	if err := os.WriteFile(path, adData, 0644); err != nil {
		return "", fmt.Errorf("保存激活信息失败: %v", err)
	}

	return "激活成功", nil
}

func getMachineId() (string, error) {
	var raw string
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "IOPlatformUUID") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					raw = strings.Trim(strings.TrimSpace(parts[1]), "\"")
					break
				}
			}
		}
	case "windows":
		// wmic is deprecated/removed in Windows 11, use PowerShell Get-CimInstance instead
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"(Get-CimInstance -ClassName Win32_ComputerSystemProduct).UUID").Output()
		if err != nil {
			// Fallback to wmic for older Windows versions
			out, err = exec.Command("wmic", "csproduct", "get", "UUID").Output()
			if err != nil {
				return "", err
			}
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) >= 2 {
				raw = strings.TrimSpace(lines[1])
			}
		} else {
			raw = strings.TrimSpace(string(out))
		}
	default:
		out, err := os.ReadFile("/etc/machine-id")
		if err != nil {
			return "", err
		}
		raw = strings.TrimSpace(string(out))
	}
	if raw == "" {
		return "", fmt.Errorf("无法获取机器标识")
	}
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:]), nil
}

func (a *App) Deactivate() (string, error) {
	path, err := getActivationPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		os.Remove(path)
	}
	return "已取消激活", nil
}

// ── File Save Dialog ──

func (a *App) SaveAccountsToFile(filePath, content string) error {
	return os.WriteFile(filePath, []byte(content), 0644)
}
