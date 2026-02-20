package claudecode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kiro-go/internal/common"

	"github.com/google/uuid"
)

// HandleClaudeCodeMessages POST /claudecode/v1/messages - Anthropic 格式
func HandleClaudeCodeMessages(w http.ResponseWriter, r *http.Request, provider *Provider) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "Method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}

	stream, _ := req["stream"].(bool)

	// 代理请求到 Claude Code
	resp, err := provider.CallMessages(body)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "api_error", fmt.Sprintf("Claude Code API error: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		common.WriteError(w, resp.StatusCode, "api_error", string(respBody))
		return
	}

	// 流式响应
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			common.WriteError(w, http.StatusInternalServerError, "internal_error", "Streaming not supported")
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}
		return
	}

	// 非流式响应
	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// HandleClaudeCodeChatCompletions POST /claudecode/v1/chat/completions - OpenAI 格式
func HandleClaudeCodeChatCompletions(w http.ResponseWriter, r *http.Request, provider *Provider) {
	if r.Method != http.MethodPost {
		common.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "Method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	var openaiReq map[string]interface{}
	if err := json.Unmarshal(body, &openaiReq); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}

	// 转换 OpenAI 格式到 Anthropic 格式
	anthropicReq := convertOpenAIToAnthropic(openaiReq)
	anthropicBody, _ := json.Marshal(anthropicReq)

	stream, _ := openaiReq["stream"].(bool)

	// 调用 Claude Code API
	resp, err := provider.CallMessages(anthropicBody)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "api_error", fmt.Sprintf("Claude Code API error: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		common.WriteError(w, resp.StatusCode, "api_error", string(respBody))
		return
	}

	// 流式响应
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			common.WriteError(w, http.StatusInternalServerError, "internal_error", "Streaming not supported")
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				break
			}

			var anthropicEvent map[string]interface{}
			if err := json.Unmarshal([]byte(data), &anthropicEvent); err != nil {
				continue
			}

			// 转换为 OpenAI 格式
			openaiEvent := convertAnthropicStreamToOpenAI(anthropicEvent, openaiReq["model"].(string))
			if openaiEvent != nil {
				eventData, _ := json.Marshal(openaiEvent)
				fmt.Fprintf(w, "data: %s\n\n", eventData)
				flusher.Flush()
			}
		}
		return
	}

	// 非流式响应
	respBody, _ := io.ReadAll(resp.Body)
	var anthropicResp map[string]interface{}
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to parse response")
		return
	}

	// 转换为 OpenAI 格式
	openaiResp := convertAnthropicToOpenAI(anthropicResp, openaiReq["model"].(string))
	common.WriteJSON(w, http.StatusOK, openaiResp)
}

// HandleClaudeCodeModels GET /claudecode/v1/models
func HandleClaudeCodeModels(w http.ResponseWriter, r *http.Request, provider *Provider) {
	if r.Method != http.MethodGet {
		common.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "Method not allowed")
		return
	}

	models, err := provider.GetModels()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"object": "list",
		"data":   models,
	})
}

// convertAnthropicStreamToOpenAI 转换 Anthropic 流式事件到 OpenAI 格式
func convertAnthropicStreamToOpenAI(event map[string]interface{}, model string) map[string]interface{} {
	eventType, _ := event["type"].(string)

	switch eventType {
	case "message_start":
		return map[string]interface{}{
			"id":      "chatcmpl-" + uuid.New().String()[:24],
			"object":  "chat.completion.chunk",
			"created": event["message"].(map[string]interface{})["created_at"],
			"model":   model,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]interface{}{"role": "assistant", "content": ""},
				},
			},
		}

	case "content_block_delta":
		delta := event["delta"].(map[string]interface{})
		if text, ok := delta["text"].(string); ok {
			return map[string]interface{}{
				"id":      "chatcmpl-" + uuid.New().String()[:24],
				"object":  "chat.completion.chunk",
				"created": 0,
				"model":   model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{"content": text},
					},
				},
			}
		}

	case "message_delta":
		return map[string]interface{}{
			"id":      "chatcmpl-" + uuid.New().String()[:24],
			"object":  "chat.completion.chunk",
			"created": 0,
			"model":   model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": event["delta"].(map[string]interface{})["stop_reason"],
				},
			},
		}
	}

	return nil
}

// convertOpenAIToAnthropic 转换 OpenAI 请求到 Anthropic 格式
func convertOpenAIToAnthropic(openaiReq map[string]interface{}) map[string]interface{} {
	anthropicReq := map[string]interface{}{
		"model":      openaiReq["model"],
		"max_tokens": 4096,
		"messages":   []map[string]interface{}{},
	}

	if maxTokens, ok := openaiReq["max_tokens"].(float64); ok {
		anthropicReq["max_tokens"] = int(maxTokens)
	}

	if stream, ok := openaiReq["stream"].(bool); ok {
		anthropicReq["stream"] = stream
	}

	if temp, ok := openaiReq["temperature"].(float64); ok {
		anthropicReq["temperature"] = temp
	}

	// 转换 messages
	if messages, ok := openaiReq["messages"].([]interface{}); ok {
		anthropicMessages := []map[string]interface{}{}
		var systemPrompt string

		for _, msg := range messages {
			msgMap := msg.(map[string]interface{})
			role := msgMap["role"].(string)
			content := msgMap["content"].(string)

			if role == "system" {
				systemPrompt = content
				continue
			}

			anthropicMessages = append(anthropicMessages, map[string]interface{}{
				"role":    role,
				"content": content,
			})
		}

		anthropicReq["messages"] = anthropicMessages
		if systemPrompt != "" {
			anthropicReq["system"] = systemPrompt
		}
	}

	return anthropicReq
}

// convertAnthropicToOpenAI 转换 Anthropic 响应到 OpenAI 格式
func convertAnthropicToOpenAI(resp map[string]interface{}, model string) map[string]interface{} {
	content := ""
	if contentArr, ok := resp["content"].([]interface{}); ok && len(contentArr) > 0 {
		if textBlock, ok := contentArr[0].(map[string]interface{}); ok {
			content, _ = textBlock["text"].(string)
		}
	}

	usage := resp["usage"].(map[string]interface{})
	inputTokens, _ := usage["input_tokens"].(float64)
	outputTokens, _ := usage["output_tokens"].(float64)

	return map[string]interface{}{
		"id":      "chatcmpl-" + uuid.New().String()[:24],
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": resp["stop_reason"],
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     int(inputTokens),
			"completion_tokens": int(outputTokens),
			"total_tokens":      int(inputTokens + outputTokens),
		},
	}
}
