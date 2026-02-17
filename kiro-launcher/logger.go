package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	logFile   *os.File
	logFileMu sync.Mutex
)

// initLogger 初始化日志文件，写入到数据目录下的 kiro-launcher.log
func initLogger() error {
	dir, err := getDataDir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(dir, "kiro-launcher.log")

	// 日志文件超过 5MB 时轮转
	if info, err := os.Stat(logPath); err == nil && info.Size() > 5*1024*1024 {
		oldPath := logPath + ".old"
		os.Remove(oldPath)
		os.Rename(logPath, oldPath)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %v", err)
	}
	logFileMu.Lock()
	logFile = f
	logFileMu.Unlock()

	writeToLogFile("========== kiro-launcher 启动 ==========")
	return nil
}

// closeLogger 关闭日志文件
func closeLogger() {
	logFileMu.Lock()
	defer logFileMu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

// writeToLogFile 写入一行日志到文件
func writeToLogFile(line string) {
	logFileMu.Lock()
	defer logFileMu.Unlock()
	if logFile == nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(logFile, "[%s] %s\n", ts, line)
}

// logInfo 记录信息日志（写文件 + stderr）
func logInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	writeToLogFile("[INFO] " + msg)
	fmt.Fprintf(os.Stderr, "[INFO] %s\n", msg)
}

// logWarn 记录警告日志
func logWarn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	writeToLogFile("[WARN] " + msg)
	fmt.Fprintf(os.Stderr, "[WARN] %s\n", msg)
}

// logError 记录错误日志
func logError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	writeToLogFile("[ERROR] " + msg)
	fmt.Fprintf(os.Stderr, "[ERROR] %s\n", msg)
}

// GetLogFilePath 返回日志文件路径（供前端调用）
func (a *App) GetLogFilePath() (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kiro-launcher.log"), nil
}

// GetRecentLogs 读取最近 N 行日志文件内容（供前端调用）
func (a *App) GetRecentLogs(lines int) (string, error) {
	dir, err := getDataDir()
	if err != nil {
		return "", err
	}
	logPath := filepath.Join(dir, "kiro-launcher.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", fmt.Errorf("日志文件不存在")
	}

	if lines <= 0 {
		lines = 100
	}

	// 取最后 N 行
	content := string(data)
	allLines := splitLines(content)
	if len(allLines) > lines {
		allLines = allLines[len(allLines)-lines:]
	}
	result := ""
	for _, l := range allLines {
		result += l + "\n"
	}
	return result, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
