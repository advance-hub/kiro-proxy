package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ── Proxy Management ──

func (a *App) resolveBinary() (string, error) {
	// 1. Try extracting embedded binary (production mode)
	if p, err := extractSidecar(); err == nil {
		return p, nil
	}

	// 2. Dev mode: try workspace paths
	devPaths := []string{
		filepath.Join("..", "kiro-go", "kiro-go"),
		filepath.Join("..", "kiro-go", "kiro-go.exe"),
	}
	for _, p := range devPaths {
		abs, _ := filepath.Abs(p)
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}

	// 3. Try PATH
	if p, err := exec.LookPath("kiro-go"); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("代理服务初始化失败")
}

func (a *App) pushLog(line string) {
	a.logsMu.Lock()
	defer a.logsMu.Unlock()
	if len(a.logs) >= a.maxLogs {
		a.logs = a.logs[1:]
	}
	a.logs = append(a.logs, line)
	// 同时写入日志文件
	writeToLogFile(line)
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

	// 自动导出环境变量到 shell rc
	baseURL := fmt.Sprintf("http://%s:%d/v1", cfg.Host, cfg.Port)
	if err := a.ExportCredsToShellRC(baseURL); err != nil {
		// 导出失败不影响代理启动，仅记录日志
		a.pushLog(fmt.Sprintf("警告: 导出环境变量失败: %v", err))
	} else {
		a.pushLog("已自动更新 shell 环境变量 (KIRO_API_KEY, KIRO_BASE_URL)")
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

	return fmt.Sprintf("代理已启动: http://%s:%d", cfg.Host, cfg.Port), nil
}

func (a *App) startProxyInternal(configPath, credsPath string) error {
	a.stopProxyInternal()

	// 杀掉占用代理端口的旧进程（防止 address already in use）
	a.killPortProcess(configPath)

	a.logsMu.Lock()
	a.logs = nil
	a.logsMu.Unlock()

	binary, err := a.resolveBinary()
	if err != nil {
		return err
	}

	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()

	cmd := exec.Command(binary, "-config", configPath, "-credentials", credsPath)
	cmd.Env = os.Environ()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("代理服务启动失败，可能端口已被占用")
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
		return fmt.Errorf("kiro-go 启动后立即退出\n%s", detail)
	}

	a.proxyCmd = cmd
	return nil
}

// killPortProcess 读取配置中的端口，杀掉占用该端口的旧进程
func (a *App) killPortProcess(configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var cfg ProxyConfig
	if json.Unmarshal(data, &cfg) != nil {
		return
	}
	port := cfg.Port
	if port == 0 {
		port = 13000
	}

	// 用 lsof 找到占用端口的进程并杀掉
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port)).Output()
	if err != nil || len(out) == 0 {
		return
	}
	pids := strings.Fields(strings.TrimSpace(string(out)))
	for _, pid := range pids {
		logInfo("杀掉占用端口 %d 的旧进程: PID %s", port, pid)
		exec.Command("kill", "-9", pid).Run()
	}
	time.Sleep(300 * time.Millisecond)
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
			logWarn("写入 keychain 凭据失败: %v", err)
		}
	} else {
		if _, err := os.Stat(credsPath); os.IsNotExist(err) {
			return "", fmt.Errorf("未找到凭据，请先通过 Kiro IDE 登录: %v", kcErr)
		}
	}

	// 2. Refresh token（异步，不阻塞启动）
	go func() {
		if _, err := os.Stat(credsPath); err != nil {
			return
		}
		cf, readErr := readFirstCredential(credsPath)
		if readErr != nil {
			return
		}
		// 最多重试 2 次，间隔 3 秒
		for attempt := 1; attempt <= 2; attempt++ {
			refreshed, err := refreshCredentials(cf)
			if err == nil {
				saveCredentialsFileSmart(refreshed)
				logInfo("Token 刷新成功 (第 %d 次尝试)", attempt)
				// 刷新成功后同步凭证到服务器
				autoUploadToServer()
				return
			}
			logWarn("Token 刷新失败 (第 %d 次尝试，不影响启动): %v", attempt, err)
			if attempt < 2 {
				time.Sleep(3 * time.Second)
			}
		}
		// 即使刷新失败也尝试同步（用旧 token）
		autoUploadToServer()
	}()

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
