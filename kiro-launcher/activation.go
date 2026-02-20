package main

import (
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
	"time"
)

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
	url := getActivationServerURL() + "/api/activate"

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
