package anthropic

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kiro-go/internal/common"
	"kiro-go/internal/model"
)

// DirectProvider 直连 Anthropic API 的 provider
// 支持多 API Key 轮询 + 故障转移
type DirectProvider struct {
	Config  *model.Config
	Client  *http.Client
	keys    []string       // API key 池
	index   uint64         // 原子轮询计数器
	disabled map[int]time.Time // 被禁用的 key index → 恢复时间
	mu       sync.RWMutex
}

// NewDirectProvider 创建直连 provider，合并单个 key 和 key 数组
func NewDirectProvider(cfg *model.Config) *DirectProvider {
	var keys []string
	// 先加数组里的
	for _, k := range cfg.AnthropicAPIKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	// 单个 key 如果不在数组里，也加进去
	if cfg.AnthropicAPIKey != "" {
		found := false
		for _, k := range keys {
			if k == cfg.AnthropicAPIKey {
				found = true
				break
			}
		}
		if !found {
			keys = append([]string{cfg.AnthropicAPIKey}, keys...)
		}
	}
	if len(keys) == 0 {
		log.Fatalf("[direct] 没有可用的 Anthropic API Key")
	}
	log.Printf("[direct] 初始化 %d 个 API Key", len(keys))
	return &DirectProvider{
		Config:   cfg,
		Client:   &http.Client{Timeout: 720 * time.Second},
		keys:     keys,
		disabled: make(map[int]time.Time),
	}
}

// nextKey 轮询获取下一个可用的 key，跳过被禁用的
func (dp *DirectProvider) nextKey() (string, int) {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	n := len(dp.keys)
	now := time.Now()
	for i := 0; i < n; i++ {
		idx := int(atomic.AddUint64(&dp.index, 1)-1) % n
		if until, disabled := dp.disabled[idx]; disabled {
			if now.Before(until) {
				continue // 还在冷却期
			}
			// 冷却期过了，重新启用（写锁在外面处理）
		}
		return dp.keys[idx], idx
	}
	// 全部禁用了，强制用第一个
	idx := int(atomic.AddUint64(&dp.index, 1)-1) % n
	return dp.keys[idx], idx
}

// disableKey 临时禁用一个 key（429/529 时冷却 60s，402 冷却 5min）
func (dp *DirectProvider) disableKey(idx int, duration time.Duration) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	dp.disabled[idx] = time.Now().Add(duration)
	log.Printf("[direct] API Key #%d 已禁用 %v", idx, duration)
}

// HandlePostMessagesDirect 处理 /v1/messages 请求，直连 Anthropic API
// 支持自动重试：429/529 换 key 重试，最多尝试 len(keys) 次
func HandlePostMessagesDirect(w http.ResponseWriter, r *http.Request, dp *DirectProvider) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "Method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	defer r.Body.Close()

	var req MessagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON: "+err.Error())
		return
	}

	// thinking 后缀检测
	overrideThinkingFromModelName(&req)

	log.Printf("[direct] POST /v1/messages model=%s max_tokens=%d stream=%v messages=%d thinking=%v",
		req.Model, req.MaxTokens, req.Stream, len(req.Messages), req.Thinking != nil && req.Thinking.Type == "enabled")

	forwardBody, err := buildAnthropicRequestBody(&req)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	// 带重试的请求发送
	maxAttempts := len(dp.keys)
	if maxAttempts < 2 {
		maxAttempts = 2
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		apiKey, keyIdx := dp.nextKey()

		apiURL := strings.TrimRight(dp.Config.AnthropicBaseURL, "/") + "/v1/messages"
		httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, apiURL, bytes.NewReader(forwardBody))
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "api_error", "Failed to create request: "+err.Error())
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")

		// 透传客户端的 anthropic-beta header
		if beta := r.Header.Get("anthropic-beta"); beta != "" {
			httpReq.Header.Set("anthropic-beta", beta)
		}

		resp, err := dp.Client.Do(httpReq)
		if err != nil {
			log.Printf("[direct] key#%d 请求失败: %v (attempt %d/%d)", keyIdx, err, attempt+1, maxAttempts)
			continue
		}

		// 可重试的状态码
		if resp.StatusCode == 429 || resp.StatusCode == 529 {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("[direct] key#%d 限流 %d: %s (attempt %d/%d)", keyIdx, resp.StatusCode, string(respBody), attempt+1, maxAttempts)
			dp.disableKey(keyIdx, 60*time.Second)
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			continue
		}
		if resp.StatusCode == 402 {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("[direct] key#%d 额度不足 402: %s", keyIdx, string(respBody))
			dp.disableKey(keyIdx, 5*time.Minute)
			continue
		}

		// 非 200 但不可重试 → 透传错误
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("[direct] Anthropic API 返回错误: %d %s", resp.StatusCode, string(respBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			w.Write(respBody)
			return
		}

		// 成功
		defer resp.Body.Close()
		if attempt > 0 {
			log.Printf("[direct] key#%d 成功 (attempt %d)", keyIdx, attempt+1)
		}

		if req.Stream {
			proxyStreamResponse(w, resp)
		} else {
			proxyNonStreamResponse(w, resp)
		}
		return
	}

	// 所有 key 都失败了
	common.WriteError(w, http.StatusBadGateway, "api_error", "All Anthropic API keys exhausted")
}

// buildAnthropicRequestBody 构建发往 Anthropic API 的请求体
func buildAnthropicRequestBody(req *MessagesRequest) ([]byte, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("消息列表为空")
	}

	body := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   req.Messages,
		"stream":     req.Stream,
	}

	if req.System != nil && len(req.System) > 0 && string(req.System) != "null" {
		body["system"] = req.System
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}
	if req.ToolChoice != nil {
		body["tool_choice"] = req.ToolChoice
	}
	if req.Metadata != nil {
		body["metadata"] = req.Metadata
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		body["top_k"] = *req.TopK
	}
	if len(req.StopSequences) > 0 {
		body["stop_sequences"] = req.StopSequences
	}
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		body["thinking"] = req.Thinking
		if req.MaxTokens < 16000 {
			body["max_tokens"] = 16000
		}
	}

	return json.Marshal(body)
}

// proxyStreamResponse 透传 Anthropic SSE 流式响应
func proxyStreamResponse(w http.ResponseWriter, resp *http.Response) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		common.WriteError(w, http.StatusInternalServerError, "api_error", "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintf(w, "%s\n", line)
		if line == "" {
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[direct] SSE 读取错误: %v", err)
	}
	flusher.Flush()
}

// proxyNonStreamResponse 透传 Anthropic 非流式响应
func proxyNonStreamResponse(w http.ResponseWriter, resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "api_error", "Failed to read response: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// CallAnthropic 给 OpenAI 兼容层用的直连入口
// 带 key 轮询 + 重试
func (dp *DirectProvider) CallAnthropic(req *MessagesRequest, betaHeader string) (*http.Response, error) {
	forwardBody, err := buildAnthropicRequestBody(req)
	if err != nil {
		return nil, err
	}

	maxAttempts := len(dp.keys)
	if maxAttempts < 2 {
		maxAttempts = 2
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		apiKey, keyIdx := dp.nextKey()

		apiURL := strings.TrimRight(dp.Config.AnthropicBaseURL, "/") + "/v1/messages"
		httpReq, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(forwardBody))
		if err != nil {
			return nil, fmt.Errorf("create request failed: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
		if betaHeader != "" {
			httpReq.Header.Set("anthropic-beta", betaHeader)
		}

		resp, err := dp.Client.Do(httpReq)
		if err != nil {
			lastErr = err
			log.Printf("[direct] CallAnthropic key#%d 失败: %v (attempt %d)", keyIdx, err, attempt+1)
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode == 529 {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("rate limited %d: %s", resp.StatusCode, string(respBody))
			dp.disableKey(keyIdx, 60*time.Second)
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			continue
		}
		if resp.StatusCode == 402 {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("quota exhausted 402: %s", string(respBody))
			dp.disableKey(keyIdx, 5*time.Minute)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("all API keys exhausted: %w", lastErr)
}
