package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"kiro-go/internal/common"

	"github.com/google/uuid"
)

// HandleCodexMessages POST /c/v1/messages - Codex 模式的 Claude API 兼容端点
func HandleCodexMessages(w http.ResponseWriter, r *http.Request, provider *Provider) {
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
	}
	if err := json.Unmarshal(body, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}

	systemPrompt := extractSystem(req.System)
	codexBody, err := ConvertClaudeToCodex(req.Model, req.Messages, systemPrompt, req.Tools)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "api_error", "Convert error: "+err.Error())
		return
	}

	log.Printf("[Codex] POST model=%s stream=%v msgs=%d", req.Model, req.Stream, len(req.Messages))

	if req.Stream {
		handleCodexStream(w, provider, codexBody, req.Model)
	} else {
		handleCodexNonStream(w, provider, codexBody, req.Model)
	}
}

func handleCodexStream(w http.ResponseWriter, provider *Provider, codexBody []byte, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		common.WriteError(w, http.StatusInternalServerError, "api_error", "Streaming not supported")
		return
	}

	resp, cred, err := provider.SendWithRetry(codexBody, true, 3)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "api_error", "Codex API error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if cred != nil {
		log.Printf("[Codex] 使用凭证 #%d (%s)", cred.ID, cred.Name)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	messageID := "msg_" + uuid.New().String()[:24]

	// message_start
	startData, _ := json.Marshal(map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id": messageID, "type": "message", "role": "assistant",
			"content": []interface{}{}, "model": model,
			"stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
		},
	})
	fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", startData)
	flusher.Flush()

	blockIndex := 0
	textStarted := false
	var fullText string
	var toolCalls []map[string]interface{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event map[string]interface{}
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}

		eventType, _ := event["type"].(string)

		switch eventType {
		case "response.output_text.delta":
			delta, _ := event["delta"].(string)
			if delta == "" {
				continue
			}
			if !textStarted {
				startBlock, _ := json.Marshal(map[string]interface{}{
					"type": "content_block_start", "index": blockIndex,
					"content_block": map[string]interface{}{"type": "text", "text": ""},
				})
				fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", startBlock)
				textStarted = true
			}
			deltaData, _ := json.Marshal(map[string]interface{}{
				"type": "content_block_delta", "index": blockIndex,
				"delta": map[string]interface{}{"type": "text_delta", "text": delta},
			})
			fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", deltaData)
			flusher.Flush()
			fullText += delta

		case "response.function_call_arguments.done":
			if textStarted {
				stopBlock, _ := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": blockIndex})
				fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", stopBlock)
				blockIndex++
				textStarted = false
			}
			callID, _ := event["call_id"].(string)
			name, _ := event["name"].(string)
			args, _ := event["arguments"].(string)
			if callID == "" {
				callID = "toolu_" + uuid.New().String()[:24]
			}
			var input map[string]interface{}
			json.Unmarshal([]byte(args), &input)
			if input == nil {
				input = map[string]interface{}{}
			}

			claudeName, claudeInput := codexToolToClaudeTool(name, input)
			toolCalls = append(toolCalls, map[string]interface{}{"id": callID, "name": claudeName, "input": claudeInput})

			startBlock, _ := json.Marshal(map[string]interface{}{
				"type": "content_block_start", "index": blockIndex,
				"content_block": map[string]interface{}{"type": "tool_use", "id": callID, "name": claudeName, "input": map[string]interface{}{}},
			})
			fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", startBlock)
			inputJSON, _ := json.Marshal(claudeInput)
			deltaData, _ := json.Marshal(map[string]interface{}{
				"type": "content_block_delta", "index": blockIndex,
				"delta": map[string]interface{}{"type": "input_json_delta", "partial_json": string(inputJSON)},
			})
			fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", deltaData)
			stopBlock, _ := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": blockIndex})
			fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", stopBlock)
			flusher.Flush()
			blockIndex++

		case "response.completed":
			// 结束
		}
	}

	// 关闭打开的块
	if textStarted {
		stopBlock, _ := json.Marshal(map[string]interface{}{"type": "content_block_stop", "index": blockIndex})
		fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", stopBlock)
	}

	stopReason := "end_turn"
	if len(toolCalls) > 0 {
		stopReason = "tool_use"
	}
	msgDelta, _ := json.Marshal(map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]interface{}{"output_tokens": len(fullText) * 2 / 5},
	})
	fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", msgDelta)
	msgStop, _ := json.Marshal(map[string]interface{}{"type": "message_stop"})
	fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", msgStop)
	flusher.Flush()

	log.Printf("[Codex] 流式完成: text=%dc tools=%d", len(fullText), len(toolCalls))
}

func handleCodexNonStream(w http.ResponseWriter, provider *Provider, codexBody []byte, model string) {
	resp, cred, err := provider.SendWithRetry(codexBody, false, 3)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "api_error", "Codex API error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if cred != nil {
		log.Printf("[Codex] 使用凭证 #%d (%s)", cred.ID, cred.Name)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var codexResp map[string]interface{}
	json.Unmarshal(respBody, &codexResp)

	// 简单转换为 Claude 格式
	messageID := "msg_" + uuid.New().String()[:24]
	content := []interface{}{map[string]interface{}{"type": "text", "text": string(respBody)}}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id": messageID, "type": "message", "role": "assistant",
		"content": content, "model": model,
		"stop_reason": "end_turn", "stop_sequence": nil,
		"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
	})
}

// HandleCodexChatCompletions POST /v1/chat/completions → Codex 后端
// 接收 OpenAI 格式请求，转换为 Codex 格式，返回 OpenAI SSE 格式
func HandleCodexChatCompletions(w http.ResponseWriter, r *http.Request, provider *Provider) {
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

	// 把 body 放回去，让 HandleCodexMessages 重新读取
	r.Body = io.NopCloser(strings.NewReader(string(body)))

	var openaiReq struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &openaiReq); err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": map[string]interface{}{"message": "Invalid JSON", "type": "invalid_request_error"},
		})
		return
	}

	// 转换 OpenAI messages → Codex messages + system
	var systemPrompt string
	var codexMessages []json.RawMessage
	for _, m := range openaiReq.Messages {
		if m.Role == "system" || m.Role == "developer" {
			systemPrompt += m.Content + "\n"
			continue
		}
		raw, _ := json.Marshal(map[string]interface{}{"role": m.Role, "content": m.Content})
		codexMessages = append(codexMessages, raw)
	}

	log.Printf("[Codex] POST /v1/chat/completions model=%s stream=%v msgs=%d",
		openaiReq.Model, openaiReq.Stream, len(codexMessages))

	codexBody, err := ConvertClaudeToCodex(openaiReq.Model, codexMessages, strings.TrimSpace(systemPrompt), nil)
	if err != nil {
		common.WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": map[string]interface{}{"message": "Convert error: " + err.Error(), "type": "api_error"},
		})
		return
	}

	if openaiReq.Stream {
		handleCodexChatCompletionsStream(w, provider, codexBody, openaiReq.Model)
	} else {
		handleCodexChatCompletionsNonStream(w, provider, codexBody, openaiReq.Model)
	}
}

func handleCodexChatCompletionsStream(w http.ResponseWriter, provider *Provider, codexBody []byte, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		common.WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": map[string]interface{}{"message": "Streaming not supported", "type": "server_error"},
		})
		return
	}

	resp, cred, err := provider.SendWithRetry(codexBody, true, 3)
	if err != nil {
		common.WriteJSON(w, http.StatusBadGateway, map[string]interface{}{
			"error": map[string]interface{}{"message": "Codex API error: " + err.Error(), "type": "server_error"},
		})
		return
	}
	defer resp.Body.Close()

	if cred != nil {
		log.Printf("[Codex] 使用凭证 #%d (%s)", cred.ID, cred.Name)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	chatID := "chatcmpl-" + uuid.New().String()[:24]

	// 发送第一个 OpenAI chunk（role）
	firstChunk, _ := json.Marshal(map[string]interface{}{
		"id": chatID, "object": "chat.completion.chunk", "created": 0, "model": model,
		"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{"role": "assistant", "content": ""}}},
	})
	fmt.Fprintf(w, "data: %s\n\n", firstChunk)
	flusher.Flush()

	var fullText string
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var event map[string]interface{}
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		eventType, _ := event["type"].(string)
		if eventType == "response.output_text.delta" {
			delta, _ := event["delta"].(string)
			if delta != "" {
				fullText += delta
				chunk, _ := json.Marshal(map[string]interface{}{
					"id": chatID, "object": "chat.completion.chunk", "created": 0, "model": model,
					"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{"content": delta}}},
				})
				fmt.Fprintf(w, "data: %s\n\n", chunk)
				flusher.Flush()
			}
		}
	}

	// 发送最终 chunk
	finalChunk, _ := json.Marshal(map[string]interface{}{
		"id": chatID, "object": "chat.completion.chunk", "created": 0, "model": model,
		"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{}, "finish_reason": "stop"}},
		"usage": map[string]interface{}{
			"prompt_tokens": 0, "completion_tokens": len(fullText) * 2 / 5,
			"total_tokens": len(fullText) * 2 / 5,
		},
	})
	fmt.Fprintf(w, "data: %s\n\n", finalChunk)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	log.Printf("[Codex] chat/completions 流式完成: text=%dc", len(fullText))
}

func handleCodexChatCompletionsNonStream(w http.ResponseWriter, provider *Provider, codexBody []byte, model string) {
	resp, cred, err := provider.SendWithRetry(codexBody, false, 3)
	if err != nil {
		common.WriteJSON(w, http.StatusBadGateway, map[string]interface{}{
			"error": map[string]interface{}{"message": "Codex API error: " + err.Error(), "type": "server_error"},
		})
		return
	}
	defer resp.Body.Close()

	if cred != nil {
		log.Printf("[Codex] 使用凭证 #%d (%s)", cred.ID, cred.Name)
	}

	respBody, _ := io.ReadAll(resp.Body)

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id": "chatcmpl-" + uuid.New().String()[:24], "object": "chat.completion", "model": model,
		"choices": []map[string]interface{}{{
			"index":         0,
			"message":       map[string]interface{}{"role": "assistant", "content": string(respBody)},
			"finish_reason": "stop",
		}},
		"usage": map[string]interface{}{
			"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0,
		},
	})

	log.Printf("[Codex] chat/completions 非流式完成")
}

// HandleCodexModels GET /c/v1/models
func HandleCodexModels(w http.ResponseWriter, r *http.Request) {
	models := []map[string]interface{}{
		{"id": "gpt-5-codex", "object": "model", "owned_by": "codex", "display_name": "GPT-5 Codex", "type": "chat", "max_tokens": 32000},
		{"id": "gpt-5-codex-mini", "object": "model", "owned_by": "codex", "display_name": "GPT-5 Codex Mini", "type": "chat", "max_tokens": 32000},
		{"id": "gpt-5-codex-max", "object": "model", "owned_by": "codex", "display_name": "GPT-5 Codex Max", "type": "chat", "max_tokens": 32000},
		{"id": "gpt-5.1-codex", "object": "model", "owned_by": "codex", "display_name": "GPT-5.1 Codex", "type": "chat", "max_tokens": 32000},
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"object": "list", "data": models})
}

// HandleCodexCredentials GET/POST /api/codex/credentials
func HandleCodexCredentials(w http.ResponseWriter, r *http.Request, store *CredentialStore) {
	switch r.Method {
	case http.MethodGet:
		creds := store.GetAll()
		safe := make([]map[string]interface{}, len(creds))
		for i, c := range creds {
			safe[i] = map[string]interface{}{
				"id": c.ID, "name": c.Name, "email": c.Email,
				"disabled": c.Disabled, "useCount": c.UseCount,
				"errorCount": c.ErrorCount, "lastError": c.LastError,
			}
		}
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": safe})

	case http.MethodPost:
		var req struct {
			Name         string `json:"name"`
			SessionToken string `json:"sessionToken"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
			return
		}
		if req.SessionToken == "" {
			common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionToken is required"})
			return
		}
		name := req.Name
		if name == "" {
			name = fmt.Sprintf("codex-%d", store.Count()+1)
		}
		store.mu.Lock()
		newID := len(store.credentials) + 1
		store.credentials = append(store.credentials, &Credential{
			ID: newID, Name: name, SessionToken: req.SessionToken,
		})
		store.mu.Unlock()
		store.Save()
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true, "data": map[string]interface{}{"id": newID, "name": name},
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleCodexCredentialsBatch POST /api/codex/credentials/batch-import
func HandleCodexCredentialsBatch(w http.ResponseWriter, r *http.Request, store *CredentialStore) {
	var req struct {
		Accounts []struct {
			Name         string `json:"name"`
			SessionToken string `json:"sessionToken"`
		} `json:"accounts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	success, failed := 0, 0
	for _, acc := range req.Accounts {
		if acc.SessionToken == "" {
			failed++
			continue
		}
		name := acc.Name
		if name == "" {
			name = fmt.Sprintf("codex-%d", store.Count()+1)
		}
		store.mu.Lock()
		newID := len(store.credentials) + 1
		store.credentials = append(store.credentials, &Credential{
			ID: newID, Name: name, SessionToken: acc.SessionToken,
		})
		store.mu.Unlock()
		success++
	}
	store.Save()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "data": map[string]interface{}{"success": success, "failed": failed},
	})
}

// HandleCodexRefreshAll POST /api/codex/refresh-all
func HandleCodexRefreshAll(w http.ResponseWriter, r *http.Request, store *CredentialStore) {
	// Codex session token 不需要刷新，只做健康检查
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "data": map[string]interface{}{"success": store.ActiveCount(), "failed": 0},
	})
}

// HandleCodexStats GET /api/codex/stats
func HandleCodexStats(w http.ResponseWriter, r *http.Request, store *CredentialStore) {
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    map[string]interface{}{"total": store.Count(), "active": store.ActiveCount()},
	})
}

// ── 工具转换 ──

func codexToolToClaudeTool(name string, input map[string]interface{}) (string, map[string]interface{}) {
	switch name {
	case "shell":
		cmdArr, _ := input["command"].([]interface{})
		var parts []string
		for _, c := range cmdArr {
			if s, ok := c.(string); ok {
				parts = append(parts, s)
			}
		}
		return "Bash", map[string]interface{}{"command": strings.Join(parts, " ")}
	case "apply_patch":
		// apply_patch 的 patch 内容在 arguments 里
		patch, _ := input["patch"].(string)
		return "Edit", map[string]interface{}{"patch": patch}
	default:
		return name, input
	}
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
