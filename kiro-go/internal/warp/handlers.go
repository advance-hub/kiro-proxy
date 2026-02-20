package warp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"kiro-go/internal/common"

	"github.com/google/uuid"
)

// HandleWarpMessages POST /w/v1/messages - Warp 模式的 Claude API 兼容端点
func HandleWarpMessages(w http.ResponseWriter, r *http.Request, provider *Provider) {
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

	var req struct {
		Model     string            `json:"model"`
		MaxTokens int               `json:"max_tokens"`
		Messages  []json.RawMessage `json:"messages"`
		System    json.RawMessage   `json:"system,omitempty"`
		Stream    bool              `json:"stream"`
		Tools     []json.RawMessage `json:"tools,omitempty"`
		Metadata  *struct {
			WorkingDir string `json:"working_dir"`
		} `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}

	warpModel := MapModelToWarp(req.Model)

	log.Printf("[Warp] POST /w/v1/messages model=%s→%s stream=%v msgs=%d tools=%d",
		req.Model, warpModel, req.Stream, len(req.Messages), len(req.Tools))

	// 构建 Warp protobuf 请求（使用极简格式）
	warpBody := BuildWarpRequestFromMessages(req.Messages)

	if req.Stream {
		handleWarpStream(w, provider, warpBody, warpModel, req.Messages)
	} else {
		handleWarpNonStream(w, provider, warpBody, warpModel, req.Messages)
	}
}

func handleWarpStream(w http.ResponseWriter, provider *Provider, warpBody []byte, model string, messages []json.RawMessage) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		common.WriteError(w, http.StatusInternalServerError, "api_error", "Streaming not supported")
		return
	}

	resp, cred, err := provider.SendWithRetry(warpBody, 3)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "api_error", "Warp API error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if cred != nil {
		log.Printf("[Warp] 使用凭证 #%d (%s)", cred.ID, cred.Name)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	messageID := "msg_" + uuid.New().String()[:24]
	inputTokens := estimateTokens(marshalMessages(messages))
	state := NewSSEState(messageID, model, inputTokens)

	// 发送 message_start
	fmt.Fprint(w, GenerateMessageStartSSE(state))
	flusher.Flush()

	ReadSSEResponse(resp, func(events []WarpEvent) {
		for _, ev := range events {
			output := ProcessWarpEvent(ev, state)
			if output != "" {
				fmt.Fprint(w, output)
				flusher.Flush()
			}
		}
	})

	// 确保发送结束事件
	final := GenerateFinalSSE(state)
	if final != "" {
		fmt.Fprint(w, final)
		flusher.Flush()
	}

	log.Printf("[Warp] 流式完成: text=%dc tools=%d", len(state.FullText), len(state.ToolCalls))
}

func handleWarpNonStream(w http.ResponseWriter, provider *Provider, warpBody []byte, model string, messages []json.RawMessage) {
	resp, cred, err := provider.SendWithRetry(warpBody, 3)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "api_error", "Warp API error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if cred != nil {
		log.Printf("[Warp] 使用凭证 #%d (%s)", cred.ID, cred.Name)
	}

	messageID := "msg_" + uuid.New().String()[:24]
	inputTokens := estimateTokens(marshalMessages(messages))
	state := NewSSEState(messageID, model, inputTokens)

	ReadSSEResponse(resp, func(events []WarpEvent) {
		for _, ev := range events {
			ProcessWarpEvent(ev, state)
		}
	})

	result := BuildClaudeNonStreamResponse(state)
	common.WriteJSON(w, http.StatusOK, result)

	log.Printf("[Warp] 非流式完成: text=%dc tools=%d", len(state.FullText), len(state.ToolCalls))
}

// HandleWarpChatCompletions POST /v1/chat/completions → Warp 后端
// 接收 OpenAI 格式请求，转换为 Warp protobuf，返回 OpenAI SSE 格式
func HandleWarpChatCompletions(w http.ResponseWriter, r *http.Request, provider *Provider) {
	if r.Method != http.MethodPost {
		common.WriteJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": map[string]interface{}{"message": "Method not allowed", "type": "invalid_request_error"},
		})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]interface{}{"message": "Failed to read request body", "type": "invalid_request_error"},
		})
		return
	}
	defer r.Body.Close()

	var openaiReq struct {
		Model    string        `json:"model"`
		Messages []interface{} `json:"messages"`
		Stream   bool          `json:"stream"`
	}
	if err := json.Unmarshal(body, &openaiReq); err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]interface{}{"message": "Invalid JSON", "type": "invalid_request_error"},
		})
		return
	}

	// 转换 OpenAI messages → 简化的 Anthropic messages + system
	var systemParts []string
	var claudeMessages []json.RawMessage
	for _, m := range openaiReq.Messages {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "system" || role == "developer" {
			if text, ok := msg["content"].(string); ok {
				systemParts = append(systemParts, text)
			}
			continue
		}
		raw, _ := json.Marshal(msg)
		claudeMessages = append(claudeMessages, raw)
	}

	warpModel := MapModelToWarp(openaiReq.Model)

	log.Printf("[Warp] POST /v1/chat/completions model=%s→%s stream=%v msgs=%d",
		openaiReq.Model, warpModel, openaiReq.Stream, len(claudeMessages))

	// 使用极简 protobuf 格式
	warpBody := BuildWarpRequestFromMessages(claudeMessages)

	if openaiReq.Stream {
		handleWarpChatCompletionsStream(w, provider, warpBody, openaiReq.Model, claudeMessages)
	} else {
		handleWarpChatCompletionsNonStream(w, provider, warpBody, openaiReq.Model, claudeMessages)
	}
}

func handleWarpChatCompletionsStream(w http.ResponseWriter, provider *Provider, warpBody []byte, model string, messages []json.RawMessage) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		common.WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": map[string]interface{}{"message": "Streaming not supported", "type": "server_error"},
		})
		return
	}

	resp, cred, err := provider.SendWithRetry(warpBody, 3)
	if err != nil {
		common.WriteJSON(w, http.StatusBadGateway, map[string]interface{}{
			"error": map[string]interface{}{"message": "Warp API error: " + err.Error(), "type": "server_error"},
		})
		return
	}
	defer resp.Body.Close()

	if cred != nil {
		log.Printf("[Warp] 使用凭证 #%d (%s)", cred.ID, cred.Name)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	chatID := "chatcmpl-" + uuid.New().String()[:24]
	messageID := "msg_" + uuid.New().String()[:24]
	inputTokens := estimateTokens(marshalMessages(messages))
	state := NewSSEState(messageID, model, inputTokens)

	// 发送第一个 OpenAI chunk（role）
	firstChunk, _ := json.Marshal(map[string]interface{}{
		"id": chatID, "object": "chat.completion.chunk", "created": 0, "model": model,
		"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{"role": "assistant", "content": ""}}},
	})
	fmt.Fprintf(w, "data: %s\n\n", firstChunk)
	flusher.Flush()

	ReadSSEResponse(resp, func(events []WarpEvent) {
		for _, ev := range events {
			ProcessWarpEvent(ev, state)
			// 输出文本 delta
			if ev.Type == "text_delta" && ev.Text != "" {
				chunk, _ := json.Marshal(map[string]interface{}{
					"id": chatID, "object": "chat.completion.chunk", "created": 0, "model": model,
					"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{"content": ev.Text}}},
				})
				fmt.Fprintf(w, "data: %s\n\n", chunk)
				flusher.Flush()
			}
		}
	})

	// 发送最终 chunk
	finishReason := "stop"
	if len(state.ToolCalls) > 0 {
		finishReason = "tool_calls"
	}
	outputTokens := 0
	if state.Usage != nil {
		outputTokens = state.Usage.OutputTokens
	}
	finalChunk, _ := json.Marshal(map[string]interface{}{
		"id": chatID, "object": "chat.completion.chunk", "created": 0, "model": model,
		"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{}, "finish_reason": finishReason}},
		"usage": map[string]interface{}{
			"prompt_tokens": inputTokens, "completion_tokens": outputTokens,
			"total_tokens": inputTokens + outputTokens,
		},
	})
	fmt.Fprintf(w, "data: %s\n\n", finalChunk)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	log.Printf("[Warp] chat/completions 流式完成: text=%dc", len(state.FullText))
}

func handleWarpChatCompletionsNonStream(w http.ResponseWriter, provider *Provider, warpBody []byte, model string, messages []json.RawMessage) {
	resp, cred, err := provider.SendWithRetry(warpBody, 3)
	if err != nil {
		common.WriteJSON(w, http.StatusBadGateway, map[string]interface{}{
			"error": map[string]interface{}{"message": "Warp API error: " + err.Error(), "type": "server_error"},
		})
		return
	}
	defer resp.Body.Close()

	if cred != nil {
		log.Printf("[Warp] 使用凭证 #%d (%s)", cred.ID, cred.Name)
	}

	messageID := "msg_" + uuid.New().String()[:24]
	inputTokens := estimateTokens(marshalMessages(messages))
	state := NewSSEState(messageID, model, inputTokens)

	ReadSSEResponse(resp, func(events []WarpEvent) {
		for _, ev := range events {
			ProcessWarpEvent(ev, state)
		}
	})

	finishReason := "stop"
	if len(state.ToolCalls) > 0 {
		finishReason = "tool_calls"
	}
	outputTokens := 0
	if state.Usage != nil {
		outputTokens = state.Usage.OutputTokens
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id": "chatcmpl-" + uuid.New().String()[:24], "object": "chat.completion", "model": model,
		"choices": []map[string]interface{}{{
			"index":         0,
			"message":       map[string]interface{}{"role": "assistant", "content": state.FullText},
			"finish_reason": finishReason,
		}},
		"usage": map[string]interface{}{
			"prompt_tokens": inputTokens, "completion_tokens": outputTokens,
			"total_tokens": inputTokens + outputTokens,
		},
	})

	log.Printf("[Warp] chat/completions 非流式完成: text=%dc", len(state.FullText))
}

// HandleWarpModels GET /warp/v1/models
func HandleWarpModels(w http.ResponseWriter, r *http.Request, provider *Provider) {
	models, err := provider.GetModels()
	if err != nil {
		log.Printf("[Warp] 获取模型列表失败: %v，返回默认列表", err)
		// 返回默认模型列表作为 fallback（与 GraphQL API 返回格式一致）
		models = []map[string]interface{}{
			{"id": "claude-4-sonnet", "object": "model", "owned_by": "anthropic", "display_name": "claude 4 sonnet", "type": "chat", "max_tokens": 32000},
			{"id": "claude-4.1-opus", "object": "model", "owned_by": "anthropic", "display_name": "claude 4.1 opus", "type": "chat", "max_tokens": 32000},
			{"id": "claude-4-5-haiku", "object": "model", "owned_by": "anthropic", "display_name": "claude 4.5 haiku", "type": "chat", "max_tokens": 32000},
			{"id": "claude-4-5-opus", "object": "model", "owned_by": "anthropic", "display_name": "claude 4.5 opus", "type": "chat", "max_tokens": 32000},
			{"id": "claude-4-5-sonnet", "object": "model", "owned_by": "anthropic", "display_name": "claude 4.5 sonnet", "type": "chat", "max_tokens": 32000},
			{"id": "claude-4-6-opus-high", "object": "model", "owned_by": "anthropic", "display_name": "claude 4.6 opus", "type": "chat", "max_tokens": 32000},
			{"id": "claude-4-6-sonnet-high", "object": "model", "owned_by": "anthropic", "display_name": "claude 4.6 sonnet", "type": "chat", "max_tokens": 32000},
			{"id": "gpt-5", "object": "model", "owned_by": "openai", "display_name": "gpt-5 (medium reasoning)", "type": "chat", "max_tokens": 32000},
			{"id": "gpt-5-1-codex-low", "object": "model", "owned_by": "openai", "display_name": "gpt-5.1 codex (low reasoning)", "type": "chat", "max_tokens": 32000},
			{"id": "gemini-2.5-pro", "object": "model", "owned_by": "google", "display_name": "gemini 2.5 pro", "type": "chat", "max_tokens": 32000},
		}
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"object": "list", "data": models})
}

// HandleWarpCredentials GET/POST /api/warp/credentials
func HandleWarpCredentials(w http.ResponseWriter, r *http.Request, store *WarpCredentialStore) {
	switch r.Method {
	case http.MethodGet:
		creds := store.GetAll()
		safe := make([]map[string]interface{}, len(creds))
		for i, c := range creds {
			safe[i] = map[string]interface{}{
				"id": c.ID, "name": c.Name, "email": c.Email,
				"disabled": c.Disabled, "useCount": c.UseCount,
				"errorCount": c.ErrorCount, "lastError": c.LastError,
				"authMode": c.AuthMode,
			}
		}
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": safe})

	case http.MethodPost:
		var req struct {
			Name         string `json:"name"`
			RefreshToken string `json:"refreshToken"`
			ApiKey       string `json:"apiKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
			return
		}

		// apikey 模式：wk-xxx 格式
		if req.ApiKey != "" {
			name := req.Name
			if name == "" {
				name = fmt.Sprintf("warp-apikey-%d", store.Count()+1)
			}
			store.mu.Lock()
			newID := len(store.credentials) + 1
			store.credentials = append(store.credentials, &WarpCredential{
				ID: newID, Name: name, Email: name,
				ApiKey: req.ApiKey, AuthMode: "apikey",
				RefreshToken: req.RefreshToken, // 保存 refresh token 备用
			})
			store.mu.Unlock()
			store.Save()
			common.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"success": true, "data": map[string]interface{}{"id": newID, "name": name, "authMode": "apikey"},
			})
			return
		}

		// firebase 模式：refresh token
		if req.RefreshToken == "" {
			common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "refreshToken or apiKey is required"})
			return
		}

		accessToken, _, err := RefreshAccessToken(req.RefreshToken)
		if err != nil {
			common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Token 验证失败: " + err.Error()})
			return
		}
		email := GetEmailFromToken(accessToken)
		name := req.Name
		if name == "" {
			name = email
		}
		if name == "" {
			name = fmt.Sprintf("warp-%d", store.Count()+1)
		}

		store.mu.Lock()
		newID := len(store.credentials) + 1
		store.credentials = append(store.credentials, &WarpCredential{
			ID: newID, Name: name, Email: email,
			RefreshToken: req.RefreshToken, AccessToken: accessToken,
			AuthMode: "firebase",
		})
		store.mu.Unlock()
		store.Save()

		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true, "data": map[string]interface{}{"id": newID, "name": name, "email": email},
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleWarpCredentialsBatch POST /api/warp/credentials/batch-import
func HandleWarpCredentialsBatch(w http.ResponseWriter, r *http.Request, store *WarpCredentialStore) {
	var req struct {
		Accounts []struct {
			Name         string `json:"name"`
			RefreshToken string `json:"refreshToken"`
			ApiKey       string `json:"apiKey"`
		} `json:"accounts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}

	success, failed := 0, 0
	for _, acc := range req.Accounts {
		// apikey 模式
		if acc.ApiKey != "" {
			name := acc.Name
			if name == "" {
				name = fmt.Sprintf("warp-apikey-%d", store.Count()+1)
			}
			store.mu.Lock()
			newID := len(store.credentials) + 1
			store.credentials = append(store.credentials, &WarpCredential{
				ID: newID, Name: name, Email: name,
				ApiKey: acc.ApiKey, AuthMode: "apikey",
				RefreshToken: acc.RefreshToken,
			})
			store.mu.Unlock()
			success++
			continue
		}

		// firebase 模式
		if acc.RefreshToken == "" {
			failed++
			continue
		}
		accessToken, _, err := RefreshAccessToken(acc.RefreshToken)
		if err != nil {
			failed++
			continue
		}
		email := GetEmailFromToken(accessToken)
		name := acc.Name
		if name == "" {
			name = email
		}

		store.mu.Lock()
		newID := len(store.credentials) + 1
		store.credentials = append(store.credentials, &WarpCredential{
			ID: newID, Name: name, Email: email,
			RefreshToken: acc.RefreshToken, AccessToken: accessToken,
			AuthMode: "firebase",
		})
		store.mu.Unlock()
		success++
	}
	store.Save()

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "data": map[string]interface{}{"success": success, "failed": failed},
	})
}

// HandleWarpBatchImportApiKey POST /api/warp/credentials/batch-import-apikey
// 支持上号器格式: email----wk-xxx----refreshToken (每行一个)
func HandleWarpBatchImportApiKey(w http.ResponseWriter, r *http.Request, store *WarpCredentialStore) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}

	lines := strings.Split(strings.TrimSpace(req.Text), "\n")
	success, failed := 0, 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "----", 3)
		if len(parts) < 2 {
			failed++
			continue
		}

		email := strings.TrimSpace(parts[0])
		apiKey := strings.TrimSpace(parts[1])
		refreshToken := ""
		if len(parts) >= 3 {
			refreshToken = strings.TrimSpace(parts[2])
		}

		if !strings.HasPrefix(apiKey, "wk-") {
			failed++
			continue
		}

		name := email
		if name == "" {
			name = fmt.Sprintf("warp-apikey-%d", store.Count()+1)
		}

		store.mu.Lock()
		newID := len(store.credentials) + 1
		store.credentials = append(store.credentials, &WarpCredential{
			ID: newID, Name: name, Email: email,
			ApiKey: apiKey, AuthMode: "apikey",
			RefreshToken: refreshToken,
		})
		store.mu.Unlock()
		success++
	}
	store.Save()

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "data": map[string]interface{}{"success": success, "failed": failed},
	})
}

// HandleWarpRefreshAll POST /api/warp/refresh-all
func HandleWarpRefreshAll(w http.ResponseWriter, r *http.Request, store *WarpCredentialStore) {
	success, failed := RefreshAllTokens(store)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "data": map[string]interface{}{"success": success, "failed": failed},
	})
}

// HandleWarpStats GET /api/warp/stats
func HandleWarpStats(w http.ResponseWriter, r *http.Request, store *WarpCredentialStore) {
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"total":  store.Count(),
			"active": store.ActiveCount(),
		},
	})
}

// HandleWarpQuotas GET /api/warp/quotas - 查询所有账号配额
func HandleWarpQuotas(w http.ResponseWriter, r *http.Request, store *WarpCredentialStore) {
	results := GetAllQuotas(store)
	if results == nil {
		results = []map[string]interface{}{}
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    results,
	})
}

// ── 模型映射 ──

var modelMapping = map[string]string{
	// Anthropic
	"claude-opus-4-5-20251101":   "claude-4-5-opus",
	"claude-haiku-4-5-20251001":  "claude-4-5-sonnet",
	"claude-sonnet-4-20250514":   "claude-4-sonnet",
	"claude-sonnet-4-5-20250929": "claude-4-5-sonnet",
	"claude-3-5-sonnet-20241022": "claude-4-sonnet",
	"claude-3-opus-20240229":     "claude-4-opus",
	"claude-3-sonnet-20240229":   "claude-4-sonnet",
	"claude-3-haiku-20240307":    "claude-4-sonnet",
	"claude-opus-4-6":            "claude-4.1-opus",
	// Gemini
	"gemini-2.5-pro":            "gemini-2.5-pro",
	"gemini-2.5-flash":          "gemini-2.5-pro",
	"gemini-2.5-flash-lite":     "gemini-2.5-pro",
	"gemini-2.5-flash-thinking": "gemini-2.5-pro",
	"gemini-3-flash":            "gemini-2.5-pro",
	"gemini-3-pro":              "gemini-3-pro",
	"gemini-3-pro-high":         "gemini-3-pro",
	"gemini-3-pro-low":          "gemini-2.5-pro",
	// OpenAI
	"gpt-4-turbo":         "gpt-4.1",
	"gpt-4-turbo-preview": "gpt-4.1",
	"gpt-4":               "gpt-4.1",
	"gpt-4o":              "gpt-4o",
	"gpt-4o-mini":         "gpt-4.1",
	"o1":                  "o3",
	"o1-mini":             "o4-mini",
	"o1-preview":          "o3",
}

func MapModelToWarp(model string) string {
	if model == "" {
		return "claude-4.1-opus"
	}
	lower := strings.ToLower(strings.TrimSpace(model))
	if mapped, ok := modelMapping[lower]; ok {
		return mapped
	}
	// 检查是否已经是 Warp 模型
	warpModels := []string{"claude-4.1-opus", "claude-4-opus", "claude-4-5-opus", "claude-4-sonnet", "claude-4-5-sonnet", "gpt-5", "gpt-4.1", "gpt-4o", "o3", "o4-mini", "gemini-2.5-pro", "gemini-3-pro"}
	for _, wm := range warpModels {
		if lower == wm {
			return wm
		}
	}
	// 模糊匹配
	if strings.Contains(lower, "opus") {
		if strings.Contains(lower, "4.5") || strings.Contains(lower, "4-5") {
			return "claude-4-5-opus"
		}
		if strings.Contains(lower, "4.1") {
			return "claude-4.1-opus"
		}
		return "claude-4-opus"
	}
	if strings.Contains(lower, "sonnet") {
		if strings.Contains(lower, "4.5") || strings.Contains(lower, "4-5") {
			return "claude-4-5-sonnet"
		}
		return "claude-4-sonnet"
	}
	if strings.Contains(lower, "haiku") {
		return "claude-4-sonnet"
	}
	if strings.Contains(lower, "claude") {
		return "claude-4.1-opus"
	}
	if strings.Contains(lower, "gemini") {
		return "gemini-2.5-pro"
	}
	if strings.Contains(lower, "gpt") {
		return "gpt-4.1"
	}
	return "claude-4.1-opus"
}

func extractSystem(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var arr []map[string]interface{}
	if json.Unmarshal(raw, &arr) == nil {
		var parts []string
		for _, item := range arr {
			if text, ok := item["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func marshalMessages(messages []json.RawMessage) string {
	data, _ := json.Marshal(messages)
	return string(data)
}
