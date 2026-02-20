package openai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kiro-go/internal/anthropic"
	"kiro-go/internal/common"
	"kiro-go/internal/kiro"
	"kiro-go/internal/kiro/parser"
	"kiro-go/internal/logger"
	"kiro-go/internal/model"

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

	rid := common.GetRequestIDFromContext(r)
	actCode := common.GetActCodeFromContext(r)
	rlog := logger.NewContext(logger.CatRequest, rid, logger.MaskKey(actCode))

	rlog.Debug("收到 OpenAI 请求", logger.F{"body_size": len(body)})

	req := convertOpenAIToAnthropic(openaiReq)

	kiroBody, err := anthropic.ConvertToKiroRequest(req)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	start := time.Now()
	creds := common.GetCredsFromContext(r)
	var resp *http.Response
	if creds != nil {
		resp, err = provider.CallWithCredentials(kiroBody, creds, actCode)
	} else {
		resp, _, err = provider.CallWithTokenManager(kiroBody)
	}
	elapsed := time.Since(start)
	if err != nil {
		rlog.Error("上游请求失败", logger.F{
			"model":   req.Model,
			"latency": elapsed.String(),
			"error":   err.Error(),
		})
		writeOpenAIError(w, http.StatusBadGateway, "server_error", err.Error())
		return
	}
	defer resp.Body.Close()

	rlog.Info("上游响应", logger.F{
		"model":   req.Model,
		"status":  resp.StatusCode,
		"latency": elapsed.String(),
		"stream":  req.Stream,
	})

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		rlog.Error("上游错误", logger.F{
			"status": resp.StatusCode,
			"body":   logger.TruncateBody(string(respBody), 500),
		})
		writeOpenAIError(w, resp.StatusCode, "api_error", fmt.Sprintf("Upstream error: %d %s", resp.StatusCode, string(respBody)))
		return
	}

	if req.Stream {
		handleStreamResponse(w, resp, req, provider, openaiReq, creds, actCode)
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

	// Truncation Recovery: 检查并注入截断恢复消息
	// 参考 kiro-gateway routes_openai.py
	if messages, ok := req["messages"].([]interface{}); ok {
		var msgMaps []map[string]interface{}
		for _, m := range messages {
			if msgMap, ok := m.(map[string]interface{}); ok {
				msgMaps = append(msgMaps, msgMap)
			}
		}
		recoveredMsgs, injected := common.InjectTruncationRecoveryOpenAI(msgMaps)
		if injected {
			logger.Infof(logger.CatRequest, "截断恢复: 已注入恢复消息")
			// 转换回 []interface{}
			var newMessages []interface{}
			for _, m := range recoveredMsgs {
				newMessages = append(newMessages, m)
			}
			req["messages"] = newMessages
		}
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
// 支持两种格式：
// 1. OpenAI 格式：content(string) + tool_calls 数组
// 2. Anthropic 格式：content 是 [{type:"text",...}, {type:"tool_use",...}] 数组
func buildAssistantContent(msg map[string]interface{}) []map[string]interface{} {
	var blocks []map[string]interface{}

	// 检查 content 是否是数组（Anthropic 格式）
	if contentArr, ok := msg["content"].([]interface{}); ok {
		for _, item := range contentArr {
			block, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case "text":
				text, _ := block["text"].(string)
				if text != "" {
					blocks = append(blocks, map[string]interface{}{"type": "text", "text": text})
				}
			case "tool_use":
				// 直接保留 tool_use block
				id, _ := block["id"].(string)
				name, _ := block["name"].(string)
				input := block["input"]
				if input == nil {
					input = map[string]interface{}{}
				}
				blocks = append(blocks, map[string]interface{}{
					"type":  "tool_use",
					"id":    id,
					"name":  name,
					"input": input,
				})
			case "thinking":
				// 保留 thinking block
				thinking, _ := block["thinking"].(string)
				if thinking != "" {
					blocks = append(blocks, map[string]interface{}{"type": "thinking", "thinking": thinking})
				}
			}
		}
		// 如果数组中提取到了 blocks，直接返回（不再处理 tool_calls）
		if len(blocks) > 0 {
			return blocks
		}
	}

	// OpenAI 格式：content 是字符串
	text := extractTextFromContent(msg["content"])
	if text != "" {
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": text})
	}

	// tool_calls → tool_use blocks（OpenAI 格式）
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
		logger.Warnf(logger.CatRequest, "工具名超过64字符限制(%d)，已截断: %s", len(name), name[:32])
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

func handleStreamResponse(w http.ResponseWriter, resp *http.Response, req *anthropic.MessagesRequest, provider *kiro.Provider, openaiReq map[string]interface{}, creds *model.KiroCredentials, actCode string) {
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
	// 使用 bufio.Reader 进行流式读取，避免固定 buffer 限制
	reader := bufio.NewReaderSize(resp.Body, 64*1024) // 64KB 读取缓冲区

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
	var fullContent strings.Builder                // 收集完整文本内容（用于截断检测）
	toolCollector := common.NewToolCallCollector() // 收集 tool_calls（用于截断检测）
	streamCompletedNormally := false               // 流是否正常完成
	totalBytesRead := 0                            // 累计读取字节数

	// 流式读取，每次读取可用数据
	buf := make([]byte, 256*1024) // 256KB 临时缓冲区
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			totalBytesRead += n
			decoder.Feed(buf[:n])
			frames, _ := decoder.Decode()
			for _, frame := range frames {
				event, parseErr := kiro.ParseEvent(frame)
				if parseErr != nil {
					logger.Warnf(logger.CatStream, "事件解析失败: %v", parseErr)
					continue
				}
				logger.Debugf(logger.CatStream, "Kiro事件: type=%s contentLen=%d", event.Type, len(event.Content))

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
								fullContent.WriteString(text)
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
						toolCollector.AddToolName(event.ToolUseID, event.ToolName)
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
						toolCollector.AppendArguments(event.ToolUseID, event.ToolInput)
						writeSSEChunk(w, flusher, chatID, created, model, nil,
							[]map[string]interface{}{{
								"index": idx,
								"function": map[string]interface{}{
									"arguments": event.ToolInput,
								},
							}})
					}
				} else if event.Type == "error" || event.Type == "exception" {
					logger.WarnFields(logger.CatStream, "Kiro流式错误", logger.F{
						"error_code": event.ErrorCode,
						"message":    event.ErrorMessage,
					})
					streamCompletedNormally = true
				} else if event.Type == "metering" || event.Type == "context_usage" {
					// meteringEvent / contextUsageEvent 出现在流末尾，表示正常完成
					streamCompletedNormally = true
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

	contentLength := fullContent.Len()
	contentStr := fullContent.String()

	// 检测模型提前停止：finishReason=stop 但输出以不完整句子结尾
	prematureStop := false
	shouldAutoContinue := false
	if finishReason == "stop" && !hasToolUse && contentLength > 0 {
		trimmed := strings.TrimSpace(contentStr)
		if len(trimmed) > 0 {
			lastChar := trimmed[len(trimmed)-1]
			// 检测以冒号、逗号、省略号、破折号等结尾（可能未完成）
			if lastChar == ':' || lastChar == ',' || lastChar == '-' || strings.HasSuffix(trimmed, "...") {
				prematureStop = true
				shouldAutoContinue = true
				logger.Infof(logger.CatStream, "检测到提前停止(末尾字符='%c')，将自动续写", lastChar)
			}
		}
	}

	logger.InfoFields(logger.CatStream, "流式输出完成", logger.F{
		"output_tokens":    outputTokens,
		"content_chars":    contentLength,
		"stream_completed": streamCompletedNormally,
		"has_tool_use":     hasToolUse,
		"finish_reason":    finishReason,
		"premature_stop":   prematureStop,
		"auto_continue":    shouldAutoContinue,
	})
	logger.Debugf(logger.CatStream, "流式输出内容: %s", logger.TruncateBody(contentStr, 200))

	// 自动继续：如果检测到提前停止，发起新请求并继续流式输出
	if shouldAutoContinue && openaiReq != nil {
		logger.Infof(logger.CatStream, "自动续写: 注入Continue消息并发起新请求")

		// 复制原始请求
		continueOpenAIReq := make(map[string]interface{})
		for k, v := range openaiReq {
			continueOpenAIReq[k] = v
		}

		// 注入 "Continue" 消息
		if messages, ok := continueOpenAIReq["messages"].([]interface{}); ok {
			// 添加 assistant 消息（当前输出）
			messages = append(messages, map[string]interface{}{
				"role":    "assistant",
				"content": contentStr,
			})
			// 添加 user 消息（Continue）
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": "Continue",
			})
			continueOpenAIReq["messages"] = messages

			// 转换并发起新请求
			continueReq := convertOpenAIToAnthropic(continueOpenAIReq)
			continueBody, err := anthropic.ConvertToKiroRequest(continueReq)
			if err == nil {
				// 使用与初始请求相同的凭据
				var continueResp *http.Response
				continueCreds := creds
				if continueCreds == nil && actCode != "" && provider.UserCredsMgr != nil {
					continueCreds = provider.UserCredsMgr.GetCredentials(actCode)
					logger.Debugf(logger.CatStream, "自动续写: 从UserCredsMgr获取凭证 user=%s", logger.MaskKey(actCode))
				}
				if continueCreds != nil {
					continueResp, err = provider.CallWithCredentials(continueBody, continueCreds, actCode)
				} else {
					logger.Warnf(logger.CatStream, "自动续写: 无可用凭证，跳过")
					err = fmt.Errorf("no credentials for auto-continue")
				}
				if err == nil && continueResp != nil && continueResp.StatusCode == 200 {
					logger.Infof(logger.CatStream, "自动续写: 续写请求已发起")
					// 继续读取并流式输出（不发送初始 role chunk，直接输出内容）
					// 简化实现：直接在当前流中继续输出
					continueReader := bufio.NewReaderSize(continueResp.Body, 64*1024)
					continueBuf := make([]byte, 256*1024)
					continueDecoder := parser.NewDecoder()

					for {
						n, err := continueReader.Read(continueBuf)
						if n > 0 {
							continueDecoder.Feed(continueBuf[:n])
							frames, _ := continueDecoder.Decode()
							for _, frame := range frames {
								event, parseErr := kiro.ParseEvent(frame)
								if parseErr != nil {
									continue
								}
								// 只处理文本内容，忽略其他事件
								if event.Type == "assistant_response" && event.Content != "" {
									writeSSEChunk(w, flusher, chatID, created, model,
										map[string]interface{}{"content": event.Content}, nil)
								}
							}
						}
						if err != nil {
							break
						}
					}
					continueResp.Body.Close()
					logger.Infof(logger.CatStream, "自动续写: 续写完成")
					// 继续后，使用 stop 作为最终 finish_reason
					finishReason = "stop"
				} else {
					logger.Warnf(logger.CatStream, "自动续写失败: %v", err)
				}
			}
		}
	}

	// Truncation Detection: 完整的截断检测
	// 参考 kiro-gateway streaming_openai.py + parsers.py
	if common.ShouldInjectRecovery() {
		// 1. 检测 tool_calls 截断（JSON 完整性分析）
		if hasToolUse {
			truncatedCount := toolCollector.DetectTruncations()
			if truncatedCount > 0 {
				logger.Warnf(logger.CatStream, "截断检测: %d 个tool_call被截断，下次请求将恢复", truncatedCount)
			}
		}

		// 2. 检测 content 截断（流未正常完成 + 有内容 + 无 tool_calls）
		contentStr := fullContent.String()
		contentWasTruncated := !streamCompletedNormally && len(contentStr) > 0 && !hasToolUse
		if contentWasTruncated {
			common.SaveContentTruncation(contentStr)
			logger.Warnf(logger.CatStream, "截断检测: 内容被截断，流未正常完成，共%d字符", len(contentStr))
		}
	}

	// 从 StreamContext 获取 input tokens（由 context_usage 事件计算）
	promptTokens := 0
	if streamCtx.ContextInputToks != nil {
		promptTokens = *streamCtx.ContextInputToks
	}

	finalChunk := map[string]interface{}{
		"id": chatID, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []map[string]interface{}{{
			"index":         0,
			"delta":         map[string]interface{}{},
			"finish_reason": finishReason,
		}},
		"usage": map[string]interface{}{
			"prompt_tokens":     promptTokens,
			"completion_tokens": outputTokens,
			"total_tokens":      promptTokens + outputTokens,
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

// ── Anthropic 直连模式（backend=anthropic）──

// HandleChatCompletionsDirect POST /v1/chat/completions（直连 Anthropic API）
// OpenAI 请求 → Anthropic 格式 → DirectProvider → Anthropic SSE → OpenAI SSE
func HandleChatCompletionsDirect(w http.ResponseWriter, r *http.Request, dp *anthropic.DirectProvider) {
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

	logger.InfoFields(logger.CatRequest, "Anthropic直连请求", logger.F{
		"model":  openaiReq["model"],
		"stream": openaiReq["stream"],
	})

	req := convertOpenAIToAnthropic(openaiReq)

	betaHeader := r.Header.Get("anthropic-beta")
	resp, err := dp.CallAnthropic(req, betaHeader)
	if err != nil {
		logger.ErrorFields(logger.CatRequest, "Anthropic直连请求失败", logger.F{
			"error": err.Error(),
		})
		writeOpenAIError(w, http.StatusBadGateway, "server_error", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		logger.ErrorFields(logger.CatResponse, "Anthropic直连错误", logger.F{
			"status": resp.StatusCode,
			"body":   logger.TruncateBody(string(respBody), 500),
		})
		writeOpenAIError(w, resp.StatusCode, "api_error", fmt.Sprintf("Upstream error: %d %s", resp.StatusCode, string(respBody)))
		return
	}

	if req.Stream {
		handleDirectStreamResponse(w, resp, req)
	} else {
		handleDirectNonStreamResponse(w, resp, req)
	}
}

// handleDirectStreamResponse 解析 Anthropic SSE 流 → OpenAI SSE chunks
func handleDirectStreamResponse(w http.ResponseWriter, resp *http.Response, req *anthropic.MessagesRequest) {
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
	modelName := req.Model

	// 发送第一个 chunk（role）
	writeSSEChunk(w, flusher, chatID, created, modelName, map[string]interface{}{"role": "assistant", "content": ""}, nil)

	hasToolUse := false
	toolCallIndex := 0
	toolIndexMap := make(map[string]int)
	toolNameSent := make(map[string]bool)
	outputTokens := 0
	promptTokens := 0

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	var currentEventType string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "" {
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
			continue
		}

		switch currentEventType {
		case "content_block_delta":
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
					writeSSEChunk(w, flusher, chatID, created, modelName,
						map[string]interface{}{"content": text}, nil)
				}
			case "thinking_delta":
				thinking, _ := delta["thinking"].(string)
				if thinking != "" {
					outputTokens += anthropic.CountTokens(thinking)
					writeSSEChunk(w, flusher, chatID, created, modelName,
						map[string]interface{}{"reasoning_content": thinking}, nil)
				}
			case "input_json_delta":
				// tool input delta
				partialJSON, _ := delta["partial_json"].(string)
				if partialJSON == "" {
					continue
				}
				// 找到当前正在构建的 tool_use block
				blockIdx, _ := data["index"].(float64)
				toolUseID := fmt.Sprintf("toolu_%d", int(blockIdx))
				// 用 content_block_start 里记录的真实 ID
				if realID, exists := findToolIDByBlockIndex(toolIndexMap, int(blockIdx)); exists {
					toolUseID = realID
				}
				idx, exists := toolIndexMap[toolUseID]
				if exists {
					outputTokens += anthropic.CountTokens(partialJSON)
					writeSSEChunk(w, flusher, chatID, created, modelName, nil,
						[]map[string]interface{}{{
							"index": idx,
							"function": map[string]interface{}{
								"arguments": partialJSON,
							},
						}})
				}
			}

		case "content_block_start":
			contentBlock, _ := data["content_block"].(map[string]interface{})
			if contentBlock == nil {
				continue
			}
			blockType, _ := contentBlock["type"].(string)
			if blockType == "tool_use" {
				hasToolUse = true
				toolID, _ := contentBlock["id"].(string)
				toolName, _ := contentBlock["name"].(string)
				blockIdx, _ := data["index"].(float64)

				idx := toolCallIndex
				toolIndexMap[toolID] = idx
				// 也按 block index 记录映射
				toolIndexMap[fmt.Sprintf("__block_%d", int(blockIdx))] = idx
				toolCallIndex++

				if !toolNameSent[toolID] && toolName != "" {
					toolNameSent[toolID] = true
					writeSSEChunk(w, flusher, chatID, created, modelName, nil,
						[]map[string]interface{}{{
							"index": idx,
							"id":    toolID,
							"type":  "function",
							"function": map[string]interface{}{
								"name":      toolName,
								"arguments": "",
							},
						}})
				}
			}

		case "message_delta":
			// 提取 usage
			if usage, ok := data["usage"].(map[string]interface{}); ok {
				if ot, ok := usage["output_tokens"].(float64); ok && int(ot) > outputTokens {
					outputTokens = int(ot)
				}
			}

		case "message_start":
			// 从 message_start 提取 input_tokens
			if msg, ok := data["message"].(map[string]interface{}); ok {
				if usage, ok := msg["usage"].(map[string]interface{}); ok {
					if it, ok := usage["input_tokens"].(float64); ok {
						promptTokens = int(it)
					}
				}
			}
		}
	}

	// 发送带 finish_reason 的最终 chunk
	finishReason := "stop"
	if hasToolUse {
		finishReason = "tool_calls"
	}

	finalChunk := map[string]interface{}{
		"id": chatID, "object": "chat.completion.chunk", "created": created, "model": modelName,
		"choices": []map[string]interface{}{{
			"index":         0,
			"delta":         map[string]interface{}{},
			"finish_reason": finishReason,
		}},
		"usage": map[string]interface{}{
			"prompt_tokens":     promptTokens,
			"completion_tokens": outputTokens,
			"total_tokens":      promptTokens + outputTokens,
		},
	}
	finalData, _ := json.Marshal(finalChunk)
	fmt.Fprintf(w, "data: %s\n\n", string(finalData))
	flusher.Flush()

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// findToolIDByBlockIndex 通过 Anthropic block index 查找 tool ID
func findToolIDByBlockIndex(toolIndexMap map[string]int, blockIdx int) (string, bool) {
	key := fmt.Sprintf("__block_%d", blockIdx)
	if _, exists := toolIndexMap[key]; exists {
		// 反查：找到 block index 对应的真实 tool ID
		targetIdx := toolIndexMap[key]
		for id, idx := range toolIndexMap {
			if idx == targetIdx && !strings.HasPrefix(id, "__block_") {
				return id, true
			}
		}
	}
	return "", false
}

// handleDirectNonStreamResponse 解析 Anthropic JSON 响应 → OpenAI JSON
func handleDirectNonStreamResponse(w http.ResponseWriter, resp *http.Response, req *anthropic.MessagesRequest) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "server_error", "Failed to read response: "+err.Error())
		return
	}

	var anthropicResp map[string]interface{}
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "server_error", "Invalid response JSON")
		return
	}

	// 提取内容
	var fullText strings.Builder
	var fullReasoning strings.Builder
	var toolCalls []map[string]interface{}
	finishReason := "stop"

	if content, ok := anthropicResp["content"].([]interface{}); ok {
		for _, block := range content {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			blockType, _ := blockMap["type"].(string)
			switch blockType {
			case "text":
				text, _ := blockMap["text"].(string)
				fullText.WriteString(text)
			case "thinking":
				thinking, _ := blockMap["thinking"].(string)
				fullReasoning.WriteString(thinking)
			case "tool_use":
				id, _ := blockMap["id"].(string)
				name, _ := blockMap["name"].(string)
				input := blockMap["input"]
				argsBytes, _ := json.Marshal(input)
				toolCalls = append(toolCalls, map[string]interface{}{
					"id":   id,
					"type": "function",
					"function": map[string]interface{}{
						"name":      name,
						"arguments": string(argsBytes),
					},
				})
			}
		}
	}

	if stopReason, ok := anthropicResp["stop_reason"].(string); ok {
		switch stopReason {
		case "tool_use":
			finishReason = "tool_calls"
		case "end_turn":
			finishReason = "stop"
		case "max_tokens":
			finishReason = "length"
		default:
			finishReason = "stop"
		}
	}

	message := map[string]interface{}{
		"role":    "assistant",
		"content": fullText.String(),
	}
	if fullReasoning.Len() > 0 {
		message["reasoning_content"] = fullReasoning.String()
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	// 提取 usage
	promptTokens := 0
	completionTokens := 0
	if usage, ok := anthropicResp["usage"].(map[string]interface{}); ok {
		if it, ok := usage["input_tokens"].(float64); ok {
			promptTokens = int(it)
		}
		if ot, ok := usage["output_tokens"].(float64); ok {
			completionTokens = int(ot)
		}
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id": "chatcmpl-" + uuid.New().String()[:24], "object": "chat.completion",
		"created": time.Now().Unix(), "model": req.Model,
		"choices": []map[string]interface{}{
			{"index": 0, "message": message, "finish_reason": finishReason},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	})
}

// ── 非流式响应（Kiro → OpenAI JSON）──

func handleNonStreamResponse(w http.ResponseWriter, resp *http.Response, req *anthropic.MessagesRequest) {
	decoder := parser.NewDecoder()
	// 使用 bufio.Reader 进行流式读取
	reader := bufio.NewReaderSize(resp.Body, 64*1024)

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

	buf := make([]byte, 256*1024) // 256KB 临时缓冲区
	for {
		n, err := reader.Read(buf)
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

	// Truncation Detection（非流式）
	// 参考 kiro-gateway streaming_openai.py
	if common.ShouldInjectRecovery() {
		// 1. 检测 tool_calls 截断
		if len(toolOrder) > 0 {
			truncCollector := common.NewToolCallCollector()
			for _, id := range toolOrder {
				tc := toolCollectors[id]
				truncCollector.AddToolName(id, tc.Name)
				if args := tc.Input.String(); args != "" {
					truncCollector.AppendArguments(id, args)
				}
			}
			if truncated := truncCollector.DetectTruncations(); truncated > 0 {
				logger.Warnf(logger.CatStream, "截断检测(非流式): %d 个tool_call被截断", truncated)
			}
		}
		// 2. 检测 content 截断（非流式一般不会截断 content，但以防万一）
		contentStr := fullText.String()
		if len(contentStr) > 0 && len(toolOrder) == 0 {
			// 非流式没有 streamCompletedNormally 信号，跳过 content 截断检测
			// content 截断主要发生在流式模式
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

	// 从 StreamContext 获取 input tokens
	promptTokens := 0
	if streamCtx.ContextInputToks != nil {
		promptTokens = *streamCtx.ContextInputToks
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id": "chatcmpl-" + uuid.New().String()[:24], "object": "chat.completion",
		"created": time.Now().Unix(), "model": req.Model,
		"choices": []map[string]interface{}{
			{"index": 0, "message": message, "finish_reason": finishReason},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     promptTokens,
			"completion_tokens": outputTokens,
			"total_tokens":      promptTokens + outputTokens,
		},
	})
}
