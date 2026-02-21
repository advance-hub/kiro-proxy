package logger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	}
	return "UNKNOWN"
}

type Category string

const (
	CatSystem   Category = "SYSTEM"
	CatAuth     Category = "AUTH"
	CatProxy    Category = "PROXY"
	CatToken    Category = "TOKEN"
	CatRequest  Category = "REQUEST"
	CatResponse Category = "RESPONSE"
	CatCreds    Category = "CREDS"
	CatAdmin    Category = "ADMIN"
	CatStream   Category = "STREAM"
	CatHTTP     Category = "HTTP"
)

type LogEntry struct {
	Time      string                 `json:"time"`
	Level     string                 `json:"level"`
	Category  string                 `json:"category"`
	RequestID string                 `json:"request_id,omitempty"`
	UserCode  string                 `json:"user_code,omitempty"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
}

// ── 文件写入器：按 category + 日期 写入独立日志文件 ──

type fileWriter struct {
	mu      sync.Mutex
	dir     string
	streams map[string]*os.File // key: "category-YYYY-MM-DD"
}

func newFileWriter(dir string) *fileWriter {
	if dir == "" {
		return nil
	}
	os.MkdirAll(dir, 0755)
	return &fileWriter{dir: dir, streams: make(map[string]*os.File)}
}

func (fw *fileWriter) write(category, dateStr string, line []byte) {
	if fw == nil {
		return
	}
	key := strings.ToLower(category) + "-" + dateStr
	fw.mu.Lock()
	f, ok := fw.streams[key]
	if !ok {
		// 关闭同 category 旧日期的文件
		prefix := strings.ToLower(category) + "-"
		for k, oldF := range fw.streams {
			if strings.HasPrefix(k, prefix) && k != key {
				oldF.Close()
				delete(fw.streams, k)
			}
		}
		path := filepath.Join(fw.dir, key+".log")
		var err error
		f, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fw.mu.Unlock()
			return
		}
		fw.streams[key] = f
	}
	fw.mu.Unlock()
	// 写入不加锁（os.File.Write 对 append 模式是原子的）
	f.Write(line)
}

func (fw *fileWriter) close() {
	if fw == nil {
		return
	}
	fw.mu.Lock()
	defer fw.mu.Unlock()
	for _, f := range fw.streams {
		f.Close()
	}
	fw.streams = make(map[string]*os.File)
}

// ── Logger 核心 ──

type Logger struct {
	level      Level
	mu         sync.Mutex
	asyncCh    chan logMsg
	jsonMode   bool
	fileWriter *fileWriter
}

type logMsg struct {
	line     []byte
	category string
	dateStr  string
}

var (
	defaultLogger atomic.Pointer[Logger]
)

func init() {
	l := &Logger{level: INFO}
	l.startAsync(4096)
	defaultLogger.Store(l)
}

func Default() *Logger {
	return defaultLogger.Load()
}

func SetLevel(l Level) {
	Default().level = l
}

func SetJSONMode(on bool) {
	Default().jsonMode = on
}

// SetLogDir 设置日志文件目录，启用文件日志
// 每个 category 写入独立文件: logs/auth-2026-02-18.log
func SetLogDir(dir string) {
	d := Default()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fileWriter != nil {
		d.fileWriter.close()
	}
	d.fileWriter = newFileWriter(dir)
}

func (l *Logger) startAsync(bufSize int) {
	l.asyncCh = make(chan logMsg, bufSize)
	go func() {
		for msg := range l.asyncCh {
			os.Stderr.Write(msg.line)
			if l.fileWriter != nil {
				l.fileWriter.write(msg.category, msg.dateStr, msg.line)
			}
		}
	}()
}

func (l *Logger) write(entry LogEntry) {
	if l.asyncCh == nil {
		return
	}

	now := entry.Time
	dateStr := now[:10] // "2006-01-02"

	var line []byte
	if l.jsonMode {
		line, _ = json.Marshal(entry)
		line = append(line, '\n')
	} else {
		var sb strings.Builder
		sb.WriteString(entry.Time)
		sb.WriteString(" [")
		sb.WriteString(entry.Level)
		sb.WriteString("] [")
		sb.WriteString(entry.Category)
		sb.WriteString("]")
		if entry.RequestID != "" {
			sb.WriteString(" rid=")
			sb.WriteString(entry.RequestID)
		}
		if entry.UserCode != "" {
			sb.WriteString(" user=")
			sb.WriteString(entry.UserCode)
		}
		sb.WriteString(" ")
		sb.WriteString(entry.Message)
		if len(entry.Fields) > 0 {
			for k, v := range entry.Fields {
				sb.WriteString(fmt.Sprintf(" %s=%v", k, v))
			}
		}
		sb.WriteString("\n")
		line = []byte(sb.String())
	}

	select {
	case l.asyncCh <- logMsg{line: line, category: entry.Category, dateStr: dateStr}:
	default:
		// channel full, drop — 绝不阻塞
	}
}

func (l *Logger) log(level Level, cat Category, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}
	_, file, line, _ := runtime.Caller(2)
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}

	entry := LogEntry{
		Time:     time.Now().Format("2006-01-02 15:04:05.000"),
		Level:    level.String(),
		Category: string(cat),
		Message:  msg,
		Fields:   fields,
		Caller:   fmt.Sprintf("%s:%d", file, line),
	}
	l.write(entry)
}

func (l *Logger) logCtx(level Level, cat Category, requestID, userCode, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}
	_, file, line, _ := runtime.Caller(2)
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}

	entry := LogEntry{
		Time:      time.Now().Format("2006-01-02 15:04:05.000"),
		Level:     level.String(),
		Category:  string(cat),
		RequestID: requestID,
		UserCode:  userCode,
		Message:   msg,
		Fields:    fields,
		Caller:    fmt.Sprintf("%s:%d", file, line),
	}
	l.write(entry)
}

// ── ContextLogger: 请求级别上下文 ──

type ContextLogger struct {
	logger    *Logger
	requestID string
	userCode  string
	category  Category
}

func NewContext(cat Category, requestID, userCode string) *ContextLogger {
	return &ContextLogger{
		logger:    Default(),
		requestID: requestID,
		userCode:  userCode,
		category:  cat,
	}
}

func (c *ContextLogger) WithCategory(cat Category) *ContextLogger {
	return &ContextLogger{
		logger:    c.logger,
		requestID: c.requestID,
		userCode:  c.userCode,
		category:  cat,
	}
}

func (c *ContextLogger) Debug(msg string, fields ...map[string]interface{}) {
	c.logger.logCtx(DEBUG, c.category, c.requestID, c.userCode, msg, mergeFields(fields))
}

func (c *ContextLogger) Info(msg string, fields ...map[string]interface{}) {
	c.logger.logCtx(INFO, c.category, c.requestID, c.userCode, msg, mergeFields(fields))
}

func (c *ContextLogger) Warn(msg string, fields ...map[string]interface{}) {
	c.logger.logCtx(WARN, c.category, c.requestID, c.userCode, msg, mergeFields(fields))
}

func (c *ContextLogger) Error(msg string, fields ...map[string]interface{}) {
	c.logger.logCtx(ERROR, c.category, c.requestID, c.userCode, msg, mergeFields(fields))
}

// ── Package-level convenience functions ──

func Debugf(cat Category, format string, args ...interface{}) {
	Default().log(DEBUG, cat, fmt.Sprintf(format, args...), nil)
}

func Infof(cat Category, format string, args ...interface{}) {
	Default().log(INFO, cat, fmt.Sprintf(format, args...), nil)
}

func Warnf(cat Category, format string, args ...interface{}) {
	Default().log(WARN, cat, fmt.Sprintf(format, args...), nil)
}

func Errorf(cat Category, format string, args ...interface{}) {
	Default().log(ERROR, cat, fmt.Sprintf(format, args...), nil)
}

func Fatalf(cat Category, format string, args ...interface{}) {
	Default().log(FATAL, cat, fmt.Sprintf(format, args...), nil)
	os.Exit(1)
}

func InfoFields(cat Category, msg string, fields map[string]interface{}) {
	Default().log(INFO, cat, msg, fields)
}

func WarnFields(cat Category, msg string, fields map[string]interface{}) {
	Default().log(WARN, cat, msg, fields)
}

func ErrorFields(cat Category, msg string, fields map[string]interface{}) {
	Default().log(ERROR, cat, msg, fields)
}

// F is a shorthand for map[string]interface{}
type F = map[string]interface{}

func mergeFields(fields []map[string]interface{}) map[string]interface{} {
	if len(fields) == 0 {
		return nil
	}
	return fields[0]
}

// ── 请求/响应日志 ──

// RequestLog 记录完整的请求/响应日志
func RequestLog(cat Category, rid, userCode, msg string, fields map[string]interface{}) {
	Default().logCtx(INFO, cat, rid, userCode, msg, fields)
}

// RequestError 记录请求级别的错误
func RequestError(cat Category, rid, userCode, msg string, fields map[string]interface{}) {
	Default().logCtx(ERROR, cat, rid, userCode, msg, fields)
}

// LogHTTPRequest 记录完整的 HTTP 请求（请求体、请求头、URL）
// 写入 HTTP category 的独立日志文件
func LogHTTPRequest(rid, userCode, method, url string, headers http.Header, body []byte, bodyLimit int) {
	if bodyLimit <= 0 {
		bodyLimit = 2000
	}
	bodyStr := string(body)
	if len(bodyStr) > bodyLimit {
		bodyStr = bodyStr[:bodyLimit] + fmt.Sprintf("...(truncated, total %d)", len(body))
	}

	// 构建 curl 命令
	var curl strings.Builder
	curl.WriteString(fmt.Sprintf("curl -X %s '%s'", method, url))
	for k, vals := range headers {
		kl := strings.ToLower(k)
		if kl == "authorization" || kl == "x-api-key" {
			curl.WriteString(fmt.Sprintf(" \\\n  -H '%s: %s'", k, MaskKey(vals[0])))
		} else {
			curl.WriteString(fmt.Sprintf(" \\\n  -H '%s: %s'", k, strings.Join(vals, ", ")))
		}
	}
	if len(body) > 0 {
		curl.WriteString(fmt.Sprintf(" \\\n  -d '%s'", TruncateBody(string(body), 500)))
	}

	Default().logCtx(INFO, CatHTTP, rid, userCode, "REQUEST", F{
		"method": method,
		"url":    url,
		"body":   bodyStr,
		"curl":   curl.String(),
	})
}

// LogHTTPResponse 记录完整的 HTTP 响应（状态码、响应体）
func LogHTTPResponse(rid, userCode string, statusCode int, body []byte, latency time.Duration, bodyLimit int) {
	if bodyLimit <= 0 {
		bodyLimit = 2000
	}
	bodyStr := string(body)
	if len(bodyStr) > bodyLimit {
		bodyStr = bodyStr[:bodyLimit] + fmt.Sprintf("...(truncated, total %d)", len(body))
	}

	level := INFO
	if statusCode >= 400 {
		level = ERROR
	}

	Default().logCtx(level, CatHTTP, rid, userCode, "RESPONSE", F{
		"status":  statusCode,
		"latency": latency.String(),
		"body":    bodyStr,
		"size":    len(body),
	})
}

// LogAuthResult 记录认证结果（成功/失败原因、凭证状态）
func LogAuthResult(rid, userCode, result string, fields F) {
	level := INFO
	if result != "success" {
		level = WARN
	}
	if fields == nil {
		fields = F{}
	}
	fields["result"] = result
	Default().logCtx(level, CatAuth, rid, userCode, "AUTH_RESULT", fields)
}

// LogCredentialStatus 记录凭证状态（过期、刷新、切换等）
func LogCredentialStatus(rid, userCode, action string, fields F) {
	if fields == nil {
		fields = F{}
	}
	fields["action"] = action
	Default().logCtx(INFO, CatCreds, rid, userCode, "CRED_STATUS", fields)
}

// ── 工具函数 ──

// MaskKey masks the middle of a key for safe logging
func MaskKey(key string) string {
	if key == "" {
		return "<empty>"
	}
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}

// TruncateBody truncates a body string for logging
func TruncateBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return body
	}
	return body[:maxLen] + fmt.Sprintf("...(truncated, total %d bytes)", len(body))
}
