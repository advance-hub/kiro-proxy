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

	// 只有非 warp 模式才要求 Kiro 凭据文件
	backend, _ := a.GetBackend()
	if backend != "warp" {
		if _, err := os.Stat(credsPath); os.IsNotExist(err) {
			return "", fmt.Errorf("凭据文件不存在，请先登录或获取凭据")
		}
	}

	if err := a.startProxyInternal(configPath, credsPath); err != nil {
		return "", err
	}
	cfg, _ := a.GetConfig()

	// 自动导出环境变量到 shell rc
	baseURL := fmt.Sprintf("http://%s:%d/v1", cfg.Host, cfg.Port)
	if err := a.ExportCredsToShellRC(baseURL); err != nil {
		a.pushLog(fmt.Sprintf("警告: 导出环境变量失败: %v", err))
	} else {
		a.pushLog("已自动更新 shell 环境变量 (KIRO_API_KEY, KIRO_BASE_URL)")
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

func (a *App) ClearProxyLogs() {
	a.logsMu.Lock()
	defer a.logsMu.Unlock()
	a.logs = nil
}

// ── One Click Start ──

func (a *App) OneClickStart() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}

	// 读取当前 backend 模式
	backend, _ := a.GetBackend()

	// 确保 config 存在
	configPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := defaultConfig()
		data, _ := json.MarshalIndent(cfg, "", "  ")
		os.WriteFile(configPath, data, 0644)
	}

	credsPath := filepath.Join(dir, "credentials.json")

	if backend == "warp" {
		// ── Warp 模式：不需要 Kiro 凭据，直接启动 ──
		// 确保 credentials.json 存在（kiro-go 启动需要，即使是空数组）
		if _, err := os.Stat(credsPath); os.IsNotExist(err) {
			os.WriteFile(credsPath, []byte("[]"), 0644)
		}
	} else {
		// ── Kiro 模式：走原有的 keychain + token 刷新流程 ──
		a.oneClickKiroSetup(dir, credsPath)
	}

	cfg, _ := a.GetConfig()
	if err := a.startProxyInternal(configPath, credsPath); err != nil {
		return "", err
	}

	modeLabel := "Kiro"
	if backend == "warp" {
		modeLabel = "Warp"
	}
	return fmt.Sprintf("代理已启动 (%s 模式): http://%s:%d", modeLabel, cfg.Host, cfg.Port), nil
}

// oneClickKiroSetup Kiro 模式的凭证准备（keychain 读取 + token 刷新）
func (a *App) oneClickKiroSetup(dir, credsPath string) {
	// 1. Try keychain
	creds, kcErr := readKiroCredentials()
	if kcErr == nil {
		if err := writeKeychainCredentials(creds); err != nil {
			logWarn("写入 keychain 凭据失败: %v", err)
		}
	} else {
		if _, err := os.Stat(credsPath); os.IsNotExist(err) {
			logWarn("未找到 Kiro 凭据: %v", kcErr)
			return
		}
	}

	// 2. Refresh token（同步等待最多 8 秒）
	tokenRefreshed := false
	if _, statErr := os.Stat(credsPath); statErr == nil {
		if cf, readErr := readFirstCredential(credsPath); readErr == nil {
			type refreshResult struct {
				creds *CredentialsFile
				err   error
			}
			ch := make(chan refreshResult, 1)
			go func() {
				r, e := refreshCredentials(cf)
				ch <- refreshResult{r, e}
			}()
			select {
			case res := <-ch:
				if res.err == nil {
					saveCredentialsFileSmart(res.creds)
					logInfo("Token 刷新成功（启动前同步刷新）")
					tokenRefreshed = true
				} else {
					logWarn("Token 刷新失败（不影响启动）: %v", res.err)
				}
			case <-time.After(8 * time.Second):
				logWarn("Token 刷新超时(8s)，先启动代理，后台继续刷新")
				go func() {
					res := <-ch
					if res.err == nil {
						saveCredentialsFileSmart(res.creds)
						logInfo("Token 后台刷新成功")
						notifyProxyReload()
						autoUploadToServer()
					} else {
						logWarn("Token 后台刷新也失败: %v", res.err)
						autoUploadToServer()
					}
				}()
			}
		}
	}
	if tokenRefreshed {
		go func() {
			time.Sleep(1 * time.Second)
			notifyProxyReload()
			autoUploadToServer()
		}()
	}
}
