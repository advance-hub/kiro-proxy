package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"kiro-go/internal/anthropic"
	"kiro-go/internal/common"
	"kiro-go/internal/kiro"
	"kiro-go/internal/kiro/parser"

	"github.com/google/uuid"
)

// HandleChatCompletions POST /v1/chat/completions
func HandleChatCompletions(w http.ResponseWriter, r *http.Request, provider *kiro.Provider) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "Method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	defer r.Body.Close()

	var openaiReq map[string]interface{}
	if err := json.Unmarshal(body, &openaiReq); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}

	req := convertOpenAIToAnthropic(openaiReq)
	log.Printf("POST /v1/chat/completions model=%s stream=%v messages=%d tools=%d thinking=%v",
		req.Model, req.Stream, len(req.Messages), len(req.Tools), req.Thinking != nil && req.Thinking.Type == "enabled")

	kiroBody, err := anthropic.ConvertToKiroRequest(req)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	start := time.Now()
	creds := common.GetCredsFromContext(r)
	actCode := common.GetActCodeFromContext(r)
	var resp *http.Response
	if creds != nil {
		resp, err = provider.CallWithCredentials(kiroBody, creds, actCode)
	} else {
		resp, _, err = provider.CallWithTokenManager(kiroBody)
	}
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("[RESP] /v1/chat/completions model=%s ERROR after %v: %v", req.Model, elapsed, err)
		writeOpenAIError(w, http.StatusBadGateway, "server_error", err.Error())
		return
	}
	defer resp.Body.Close()

	log.Printf("[RESP] /v1/chat/completions model=%s status=%d latency=%v", req.Model, resp.StatusCode, elapsed)

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[RESP] Upstream error body: %s", string(respBody))
		writeOpenAIError(w, resp.StatusCode, "api_error", fmt.Sprintf("Upstream error: %d %s", resp.StatusCode, string(respBody)))
		return
	}

	if req.Stream {
		handleStreamResponse(w, resp, req)
	} else {
		handleNonStreamResponse(w, resp, req)
	}
}

// ── OpenAI 错误格式 ──

func writeOpenAIError(w http.ResponseWriter, status int, errType, message string) {
	common.WriteJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errType,
			"param":   nil,
			"code":    nil,
		},
	})
}

// ── OpenAI → Anthropic 请求转换 ──
//
// 完整处理：
// - system / developer 角色 → system prompt
// - user 消息：字符串或多部分内容（text + image_url）
// - assistant 消息：text + tool_calls → text + tool_use content blocks
// - tool 消息 → user 消息 with tool_result content blocks
// - tools (function calling) → Anthropic tool 格式 + JSON Schema 清理 + 名称校验
// - max_tokens / max_completion_tokens
// - 模型名中包含 "thinking" 时自动启用 thinking

func convertOpenAIToAnthropic(req map[string]interface{}) *anthropic.MessagesRequest {
	model, _ := req["model"].(string)
	stream, _ := req["stream"].(bool)

	// max_tokens: 优先 max_completion_tokens，其次 max_tokens
	maxTokens := 16384
	if mt, ok := req["max_completion_tokens"].(float64); ok && mt > 0 {
		maxTokens = int(mt)
	} else if mt, ok := req["max_tokens"].(float64); ok && mt > 0 {
		maxTokens = int(mt)
	}

	var systemParts []string
	var anthropicMessages []anthropic.MessageItem
	var pendingToolResults []map[string]interface{} // 累积 tool 消息

	if msgs, ok := req["messages"].([]interface{}); ok {
		for _, m := range msgs {
			msg, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)

			switch role {
			case "system", "developer":
				// system / developer → 提取为 system prompt
				systemParts = append(systemParts, extractTextFromContent(msg["content"]))

			case "tool":
				// tool 消息 → 累积为 tool_result，后续合并到 user 消息
				toolCallID, _ := msg["tool_call_id"].(string)
				content := extractTextFromContent(msg["content"])
				if content == "" {
					content = "(empty result)"
				}
				pendingToolResults = append(pendingToolResults, map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": toolCallID,
					"content":     content,
				})

			case "assistant":
				// 先 flush 累积的 tool_results
				flushToolResults(&anthropicMessages, &pendingToolResults)

				// assistant 消息：处理 text + tool_calls
				contentBlocks := buildAssistantContent(msg)
				if len(contentBlocks) > 0 {
					raw, _ := json.Marshal(contentBlocks)
					anthropicMessages = append(anthropicMessages, anthropic.MessageItem{Role: "assistant", Content: raw})
				}

			case "user":
				// 先 flush 累积的 tool_results
				flushToolResults(&anthropicMessages, &pendingToolResults)

				// user 消息：处理字符串或多部分内容（text + image_url）
				contentBlocks := buildUserContent(msg["content"])
				if !isEmptyContent(contentBlocks) {
					raw, _ := json.Marshal(contentBlocks)
					anthropicMessages = append(anthropicMessages, anthropic.MessageItem{Role: "user", Content: raw})
				}

			default:
				// 未知角色（如 function）→ 当作 user
				flushToolResults(&anthropicMessages, &pendingToolResults)
				text := extractTextFromContent(msg["content"])
				raw, _ := json.Marshal(text)
				anthropicMessages = append(anthropicMessages, anthropic.MessageItem{Role: "user", Content: raw})
			}
		}
	}

	// flush 尾部残留的 tool_results
	flushToolResults(&anthropicMessages, &pendingToolResults)

	// 合并相邻同角色消息（Anthropic API 要求角色必须交替）
	anthropicMessages = mergeConsecutiveRoles(anthropicMessages)

	// system prompt（结构化格式，与 Rust 版一致）
	var system json.RawMessage
	if len(systemParts) > 0 {
		var systemBlocks []map[string]interface{}
		for _, part := range systemParts {
			if part != "" {
				systemBlocks = append(systemBlocks, map[string]interface{}{"type": "text", "text": part})
			}
		}
		if len(systemBlocks) > 0 {
			system, _ = json.Marshal(systemBlocks)
		}
	}

	// tools 转换
	var tools []json.RawMessage
	if rawTools, ok := req["tools"].([]interface{}); ok {
		for _, rt := range rawTools {
			toolMap, ok := rt.(map[string]interface{})
			if !ok {
				continue
			}
			converted := convertOpenAITool(toolMap)
			if converted != nil {
				raw, _ := json.Marshal(converted)
				tools = append(tools, raw)
			}
		}
	}

	result := &anthropic.MessagesRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  anthropicMessages,
		System:    system,
		Stream:    stream,
		Tools:     tools,
	}

	// 模型名包含 "thinking" → 自动启用 thinking
	if strings.Contains(strings.ToLower(model), "thinking") {
		result.Thinking = &anthropic.ThinkingConfig{Type: "enabled", BudgetTokens: 20000}
	}

	return result
}

// flushToolResults 将累积的 tool_result 合并为一条 user 消息
func flushToolResults(messages *[]anthropic.MessageItem, pending *[]map[string]interface{}) {
	if len(*pending) == 0 {
		return
	}
	raw, _ := json.Marshal(*pending)
	*messages = append(*messages, anthropic.MessageItem{Role: "user", Content: raw})
	*pending = nil
}

// mergeConsecutiveRoles 合并相邻同角色消息（Anthropic API 要求角色必须交替）
// 参考 Rust 版 merge_consecutive_roles
func mergeConsecutiveRoles(messages []anthropic.MessageItem) []anthropic.MessageItem {
	if len(messages) <= 1 {
		return messages
	}

	var merged []anthropic.MessageItem
	merged = append(merged, messages[0])

	for i := 1; i < len(messages); i++ {
		last := &merged[len(merged)-1]
		if messages[i].Role == last.Role {
			// 合并 content：将两个 JSON content 合并为一个数组
			prevBlocks := contentToBlocks(last.Content)
			nextBlocks := contentToBlocks(messages[i].Content)
			combined := append(prevBlocks, nextBlocks...)
			last.Content, _ = json.Marshal(combined)
		} else {
			merged = append(merged, messages[i])
		}
	}
	return merged
}

// contentToBlocks 将 JSON content 转换为 blocks 数组
func contentToBlocks(raw json.RawMessage) []interface{} {
	// 尝试解析为数组
	var arr []interface{}
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	// 尝试解析为字符串
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return nil
		}
		return []interface{}{map[string]interface{}{"type": "text", "text": s}}
	}
	// 其他情况当作文本
	return []interface{}{map[string]interface{}{"type": "text", "text": string(raw)}}
}

// isEmptyContent 检查 content 是否为空（参考 Rust 版 is_empty_content）
func isEmptyContent(content interface{}) bool {
	if content == nil {
		return true
	}
	if s, ok := content.(string); ok {
		return s == ""
	}
	if arr, ok := content.([]map[string]interface{}); ok {
		return len(arr) == 0
	}
	if arr, ok := content.([]interface{}); ok {
		return len(arr) == 0
	}
	return false
}

// extractTextFromContent 从 OpenAI content 提取纯文本
// 支持字符串和数组格式
func extractTextFromContent(content interface{}) string {
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]interface{}); ok {
		var parts []string
		for _, item := range arr {
			block, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	if content == nil {
		return ""
	}
	return fmt.Sprintf("%v", content)
}

// buildAssistantContent 构建 assistant 消息的 Anthropic content blocks
// 处理 text + tool_calls → text block + tool_use blocks
func buildAssistantContent(msg map[string]interface{}) []map[string]interface{} {
	var blocks []map[string]interface{}

	// 文本内容
	text := extractTextFromContent(msg["content"])
	if text != "" {
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": text})
	}

	// tool_calls → tool_use blocks
	if toolCalls, ok := msg["tool_calls"].([]interface{}); ok {
		for _, tc := range toolCalls {
			tcMap, ok := tc.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := tcMap["id"].(string)
			fn, _ := tcMap["function"].(map[string]interface{})
			if fn == nil {
				continue
			}
			name, _ := fn["name"].(string)
			argsStr, _ := fn["arguments"].(string)

			// 解析 arguments JSON
			var input interface{}
			if json.Unmarshal([]byte(argsStr), &input) != nil {
				input = map[string]interface{}{}
			}

			blocks = append(blocks, map[string]interface{}{
				"type":  "tool_use",
				"id":    id,
				"name":  name,
				"input": input,
			})
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": ""})
	}
	return blocks
}

// buildUserContent 构建 user 消息的 Anthropic content blocks
// 支持字符串和数组格式（text + image_url）
func buildUserContent(content interface{}) interface{} {
	// 纯字符串 → 直接返回
	if s, ok := content.(string); ok {
		return s
	}

	// 数组格式 → 转换每个 block
	arr, ok := content.([]interface{})
	if !ok {
		return fmt.Sprintf("%v", content)
	}

	var blocks []map[string]interface{}
	for _, item := range arr {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)

		switch blockType {
		case "text":
			text, _ := block["text"].(string)
			blocks = append(blocks, map[string]interface{}{"type": "text", "text": text})

		case "image_url":
			// OpenAI image_url → Anthropic image source
			imgURL, _ := block["image_url"].(map[string]interface{})
			if imgURL == nil {
				continue
			}
			url, _ := imgURL["url"].(string)
			if strings.HasPrefix(url, "data:") {
				// data:image/jpeg;base64,/9j/...
				parts := strings.SplitN(url, ",", 2)
				if len(parts) != 2 {
					continue
				}
				header := parts[0]
				data := parts[1]
				mediaType := "image/jpeg"
				if idx := strings.Index(header, ":"); idx >= 0 {
					mediaPart := header[idx+1:]
					if semiIdx := strings.Index(mediaPart, ";"); semiIdx >= 0 {
						mediaType = mediaPart[:semiIdx]
					}
				}
				blocks = append(blocks, map[string]interface{}{
					"type": "image",
					"source": map[string]interface{}{
						"type":       "base64",
						"media_type": mediaType,
						"data":       data,
					},
				})
			} else if url != "" {
				// 非 data: URL → Anthropic URL 格式（与 Rust 版一致）
				blocks = append(blocks, map[string]interface{}{
					"type": "image",
					"source": map[string]interface{}{
						"type": "url",
						"url":  url,
					},
				})
			}

		case "tool_result":
			// 透传 tool_result（某些客户端可能在 user content 中放 tool_result）
			blocks = append(blocks, block)
		}
	}

	if len(blocks) == 0 {
		return ""
	}
	if len(blocks) == 1 && blocks[0]["type"] == "text" {
		return blocks[0]["text"]
	}
	return blocks
}

// convertOpenAITool 将 OpenAI tool 转换为 Anthropic tool 格式
// 支持标准格式 {"type":"function","function":{...}} 和扁平格式 {"name":...,"input_schema":...}
func convertOpenAITool(tool map[string]interface{}) map[string]interface{} {
	var name, desc string
	var inputSchema interface{}

	if fn, ok := tool["function"].(map[string]interface{}); ok {
		// 标准 OpenAI 格式
		name, _ = fn["name"].(string)
		desc, _ = fn["description"].(string)
		inputSchema = fn["parameters"]
	} else {
		// 扁平格式（Cursor 风格）
		name, _ = tool["name"].(string)
		desc, _ = tool["description"].(string)
		inputSchema = tool["input_schema"]
		if inputSchema == nil {
			inputSchema = tool["parameters"]
		}
	}

	if name == "" {
		return nil
	}

	// 工具名长度校验（Kiro API 限制 64 字符）
	if len(name) > 64 {
		log.Printf("警告: 工具名 '%s' 超过 64 字符限制 (%d)，将截断", name, len(name))
		name = name[:64]
	}

	if inputSchema == nil {
		inputSchema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}

	// JSON Schema 清理（Kiro API 不支持 additionalProperties 和空 required）
	if schemaMap, ok := inputSchema.(map[string]interface{}); ok {
		inputSchema = sanitizeJSONSchema(schemaMap)
	}

	if desc == "" {
		desc = fmt.Sprintf("Tool: %s", name)
	}

	return map[string]interface{}{
		"name":         name,
		"description":  desc,
		"input_schema": inputSchema,
	}
}

// sanitizeJSONSchema 递归清理 JSON Schema（参考 kiro-gateway converters_core）
// Kiro API 不支持 additionalProperties 和空 required 数组
func sanitizeJSONSchema(schema map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(schema))
	for key, value := range schema {
		// 跳过 additionalProperties
		if key == "additionalProperties" {
			continue
		}
		// 跳过空 required 数组
		if key == "required" {
			if arr, ok := value.([]interface{}); ok && len(arr) == 0 {
				continue
			}
		}
		// 递归处理 properties
		if key == "properties" {
			if props, ok := value.(map[string]interface{}); ok {
				newProps := make(map[string]interface{}, len(props))
				for pName, pValue := range props {
					if pMap, ok := pValue.(map[string]interface{}); ok {
						newProps[pName] = sanitizeJSONSchema(pMap)
					} else {
						newProps[pName] = pValue
					}
				}
				result[key] = newProps
				continue
			}
		}
		// 递归处理嵌套对象
		if nested, ok := value.(map[string]interface{}); ok {
			result[key] = sanitizeJSONSchema(nested)
		} else if arr, ok := value.([]interface{}); ok {
			// 处理 anyOf, oneOf 等数组
			newArr := make([]interface{}, len(arr))
			for i, item := range arr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					newArr[i] = sanitizeJSONSchema(itemMap)
				} else {
					newArr[i] = item
				}
			}
			result[key] = newArr
		} else {
			result[key] = value
		}
	}
	return result
}

// ── 流式响应（Kiro → OpenAI SSE chunks）──
//
// 处理：
// - assistant_response → content delta
// - tool_use → tool_calls delta（function name + arguments）
// - thinking（通过 Anthropic stream context）→ reasoning_content delta
// - 正确的 finish_reason（stop / tool_calls）
// - usage 统计

func handleStreamResponse(w http.ResponseWriter, resp *http.Response, req *anthropic.MessagesRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAIError(w, http.StatusInternalServerError, "server_error", "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	chatID := "chatcmpl-" + uuid.New().String()[:24]
	created := time.Now().Unix()
	model := req.Model

	// 发送第一个 chunk（包含 role）
	writeSSEChunk(w, flusher, chatID, created, model, map[string]interface{}{"role": "assistant", "content": ""}, nil)

	decoder := parser.NewDecoder()
	buf := make([]byte, 32*1024)

	// 使用 Anthropic StreamContext 处理 thinking 提取
	thinkingEnabled := req.Thinking != nil && req.Thinking.Type == "enabled"
	streamCtx := anthropic.NewStreamContext(model, 0, thinkingEnabled)
	// 生成初始事件（不使用，仅初始化状态）
	streamCtx.GenerateInitialEvents()

	hasToolUse := false
	toolCallIndex := 0
	toolIndexMap := make(map[string]int)
	toolNameSent := make(map[string]bool)
	outputTokens := 0

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			decoder.Feed(buf[:n])
			frames, _ := decoder.Decode()
			for _, frame := range frames {
				event, parseErr := kiro.ParseEvent(frame)
				if parseErr != nil {
					continue
				}

				// 通过 Anthropic StreamContext 处理（提取 thinking）
				sseEvents := streamCtx.ProcessKiroEvent(event)
				for _, sseEvent := range sseEvents {
					switch sseEvent.Event {
					case "content_block_delta":
						data, ok := sseEvent.Data.(map[string]interface{})
						if !ok {
							continue
						}
						delta, _ := data["delta"].(map[string]interface{})
						if delta == nil {
							continue
						}
						deltaType, _ := delta["type"].(string)

						switch deltaType {
						case "text_delta":
							text, _ := delta["text"].(string)
							if text != "" {
								outputTokens += anthropic.CountTokens(text)
								writeSSEChunk(w, flusher, chatID, created, model,
									map[string]interface{}{"content": text}, nil)
							}
						case "thinking_delta":
							thinking, _ := delta["thinking"].(string)
							if thinking != "" {
								outputTokens += anthropic.CountTokens(thinking)
								writeSSEChunk(w, flusher, chatID, created, model,
									map[string]interface{}{"reasoning_content": thinking}, nil)
							}
						case "input_json_delta":
							// tool input 通过下面的 tool_use 事件处理
						}
					}
				}
				// 直接处理 tool_use 事件（不通过 StreamContext 的 Anthropic 格式）
				if event.Type == "tool_use" {
					hasToolUse = true
					idx, exists := toolIndexMap[event.ToolUseID]
					if !exists {
						idx = toolCallIndex
						toolIndexMap[event.ToolUseID] = idx
						toolCallIndex++
					}
					if !toolNameSent[event.ToolUseID] && event.ToolName != "" {
						toolNameSent[event.ToolUseID] = true
						writeSSEChunk(w, flusher, chatID, created, model, nil,
							[]map[string]interface{}{{
								"index": idx,
								"id":    event.ToolUseID,
								"type":  "function",
								"function": map[string]interface{}{
									"name":      event.ToolName,
									"arguments": "",
								},
							}})
					}
					if event.ToolInput != "" {
						outputTokens += anthropic.CountTokens(event.ToolInput)
						writeSSEChunk(w, flusher, chatID, created, model, nil,
							[]map[string]interface{}{{
								"index": idx,
								"function": map[string]interface{}{
									"arguments": event.ToolInput,
								},
							}})
					}
				} else if event.Type == "error" || event.Type == "exception" {
					log.Printf("Kiro 流式错误: %s %s", event.ErrorCode, event.ErrorMessage)
				}
			}
		}
		if err != nil {
			break
		}
	}

	// Flush StreamContext 中的 thinking buffer 和剩余内容
	finalSSEEvents := streamCtx.GenerateFinalEvents()
	for _, sseEvent := range finalSSEEvents {
		if sseEvent.Event == "content_block_delta" {
			data, ok := sseEvent.Data.(map[string]interface{})
			if !ok {
				continue
			}
			delta, _ := data["delta"].(map[string]interface{})
			if delta == nil {
				continue
			}
			deltaType, _ := delta["type"].(string)
			switch deltaType {
			case "text_delta":
				text, _ := delta["text"].(string)
				if text != "" {
					outputTokens += anthropic.CountTokens(text)
					writeSSEChunk(w, flusher, chatID, created, model,
						map[string]interface{}{"content": text}, nil)
				}
			case "thinking_delta":
				thinking, _ := delta["thinking"].(string)
				if thinking != "" {
					outputTokens += anthropic.CountTokens(thinking)
					writeSSEChunk(w, flusher, chatID, created, model,
						map[string]interface{}{"reasoning_content": thinking}, nil)
				}
			}
		}
	}

	// 发送带 finish_reason 的最终 chunk
	finishReason := "stop"
	if hasToolUse || streamCtx.HasToolUse() {
		finishReason = "tool_calls"
	}
	finalChunk := map[string]interface{}{
		"id": chatID, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []map[string]interface{}{{
			"index":         0,
			"delta":         map[string]interface{}{},
			"finish_reason": finishReason,
		}},
		"usage": map[string]interface{}{
			"prompt_tokens":     0,
			"completion_tokens": outputTokens,
			"total_tokens":      outputTokens,
		},
	}
	data, _ := json.Marshal(finalChunk)
	fmt.Fprintf(w, "data: %s\n\n", string(data))
	flusher.Flush()

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// writeSSEChunk 写入一个 OpenAI SSE chunk
func writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, chatID string, created int64, model string, delta map[string]interface{}, toolCalls []map[string]interface{}) {
	if delta == nil {
		delta = map[string]interface{}{}
	}
	if toolCalls != nil {
		delta["tool_calls"] = toolCalls
	}
	chunk := map[string]interface{}{
		"id": chatID, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []map[string]interface{}{{"index": 0, "delta": delta}},
	}
	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", string(data))
	flusher.Flush()
}

// ── 非流式响应（Kiro → OpenAI JSON）──

func handleNonStreamResponse(w http.ResponseWriter, resp *http.Response, req *anthropic.MessagesRequest) {
	decoder := parser.NewDecoder()
	buf := make([]byte, 32*1024)

	// 使用 Anthropic StreamContext 处理 thinking 提取
	thinkingEnabled := req.Thinking != nil && req.Thinking.Type == "enabled"
	streamCtx := anthropic.NewStreamContext(req.Model, 0, thinkingEnabled)
	streamCtx.GenerateInitialEvents()

	var fullText strings.Builder
	var fullReasoning strings.Builder
	outputTokens := 0

	// 收集 tool_use 事件
	type toolUseCollector struct {
		ID    string
		Name  string
		Input strings.Builder
	}
	toolCollectors := make(map[string]*toolUseCollector)
	var toolOrder []string

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			decoder.Feed(buf[:n])
			frames, _ := decoder.Decode()
			for _, frame := range frames {
				event, parseErr := kiro.ParseEvent(frame)
				if parseErr != nil {
					continue
				}

				// 通过 StreamContext 处理（提取 thinking）
				sseEvents := streamCtx.ProcessKiroEvent(event)
				for _, sseEvent := range sseEvents {
					if sseEvent.Event == "content_block_delta" {
						data, ok := sseEvent.Data.(map[string]interface{})
						if !ok {
							continue
						}
						delta, _ := data["delta"].(map[string]interface{})
						if delta == nil {
							continue
						}
						deltaType, _ := delta["type"].(string)
						switch deltaType {
						case "text_delta":
							text, _ := delta["text"].(string)
							if text != "" {
								fullText.WriteString(text)
								outputTokens += anthropic.CountTokens(text)
							}
						case "thinking_delta":
							thinking, _ := delta["thinking"].(string)
							if thinking != "" {
								fullReasoning.WriteString(thinking)
								outputTokens += anthropic.CountTokens(thinking)
							}
						}
					}
				}

				// 收集 tool_use
				if event.Type == "tool_use" {
					tc, exists := toolCollectors[event.ToolUseID]
					if !exists {
						tc = &toolUseCollector{ID: event.ToolUseID, Name: event.ToolName}
						toolCollectors[event.ToolUseID] = tc
						toolOrder = append(toolOrder, event.ToolUseID)
					}
					if event.ToolName != "" && tc.Name == "" {
						tc.Name = event.ToolName
					}
					if event.ToolInput != "" {
						tc.Input.WriteString(event.ToolInput)
						outputTokens += anthropic.CountTokens(event.ToolInput)
					}
				}
			}
		}
		if err != nil {
			break
		}
	}

	// 构建 message
	message := map[string]interface{}{
		"role":    "assistant",
		"content": fullText.String(),
	}

	// 添加 reasoning_content（thinking 模式）
	if fullReasoning.Len() > 0 {
		message["reasoning_content"] = fullReasoning.String()
	}

	finishReason := "stop"

	// 添加 tool_calls
	if len(toolOrder) > 0 {
		finishReason = "tool_calls"
		var toolCalls []map[string]interface{}
		for _, id := range toolOrder {
			tc := toolCollectors[id]
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      tc.Name,
					"arguments": tc.Input.String(),
				},
			})
		}
		message["tool_calls"] = toolCalls
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id": "chatcmpl-" + uuid.New().String()[:24], "object": "chat.completion",
		"created": time.Now().Unix(), "model": req.Model,
		"choices": []map[string]interface{}{
			{"index": 0, "message": message, "finish_reason": finishReason},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     0,
			"completion_tokens": outputTokens,
			"total_tokens":      outputTokens,
		},
	})
}
