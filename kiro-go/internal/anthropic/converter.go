package anthropic

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// MessagesRequest Anthropic /v1/messages 请求
type MessagesRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	Messages  []MessageItem     `json:"messages"`
	System    json.RawMessage   `json:"system,omitempty"`
	Stream    bool              `json:"stream"`
	Tools     []json.RawMessage `json:"tools,omitempty"`
	Metadata  *Metadata         `json:"metadata,omitempty"`
	Thinking  *ThinkingConfig   `json:"thinking,omitempty"`
}

type MessageItem struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type Metadata struct {
	UserID string `json:"user_id,omitempty"`
}

type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// injectThinkingTags 注入 fake reasoning 标签到用户消息内容
// 参考 kiro-gateway inject_thinking_tags：Kiro API 不支持 Anthropic 原生 thinking 参数，
// 而是通过在用户消息中注入 <thinking_mode> 标签来触发模型输出 <thinking> 块
func injectThinkingTags(content string, budgetTokens int) string {
	if budgetTokens <= 0 {
		budgetTokens = 4000
	}
	thinkingInstruction := "Think in English for better reasoning quality.\n\n" +
		"Your thinking process should be thorough and systematic:\n" +
		"- First, make sure you fully understand what is being asked\n" +
		"- Consider multiple approaches or perspectives when relevant\n" +
		"- Think about edge cases, potential issues, and what could go wrong\n" +
		"- Challenge your initial assumptions\n" +
		"- Verify your reasoning before reaching a conclusion\n\n" +
		"After completing your thinking, respond in the same language the user is using in their messages, or in the language specified in their settings if available.\n\n" +
		"Take the time you need. Quality of thought matters more than speed."

	prefix := fmt.Sprintf("<thinking_mode>enabled</thinking_mode>\n"+
		"<max_thinking_length>%d</max_thinking_length>\n"+
		"<thinking_instruction>%s</thinking_instruction>\n\n", budgetTokens, thinkingInstruction)

	return prefix + content
}

// ConvertToKiroRequest 将 Anthropic 请求转换为 Kiro 请求体
func ConvertToKiroRequest(req *MessagesRequest) ([]byte, error) {
	modelID, ok := ResolveModel(req.Model)
	if !ok {
		return nil, fmt.Errorf("模型不支持: %s", req.Model)
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("消息列表为空")
	}

	conversationID := uuid.New().String()
	if req.Metadata != nil && req.Metadata.UserID != "" {
		if sid := extractSessionID(req.Metadata.UserID); sid != "" {
			conversationID = sid
		}
	}

	// 消息规范化流水线（参考 kiro-gateway converters_core）
	hasTools := len(req.Tools) > 0
	normalized := normalizeMessagePipeline(req.Messages, hasTools)

	systemPrompt := extractSystemPrompt(req.System)
	tools := convertTools(req.Tools)

	// 构建 history（所有消息除了最后一条）
	historyMessages := normalized[:len(normalized)-1]

	// 如果有 system prompt 且 history 不为空，将其添加到 history 第一条 user 消息
	// 参考 kiro-gateway line 1423-1428
	if systemPrompt != "" && len(historyMessages) > 0 {
		firstMsg := historyMessages[0]
		if firstMsg.Role == "user" {
			originalContent := extractTextContent(firstMsg.Content)
			newContent := systemPrompt + "\n\n" + originalContent
			historyMessages[0].Content = json.RawMessage(fmt.Sprintf(`[{"type":"text","text":%s}]`, strconv.Quote(newContent)))
		}
	}

	history := buildHistory(historyMessages, modelID)

	// 当前消息（最后一条）
	lastMsg := normalized[len(normalized)-1]
	textContent := extractTextContent(lastMsg.Content)

	// 如果当前消息是 assistant，需要将其添加到 history，并创建 "Continue" user 消息
	// 参考 kiro-gateway line 1442-1448
	if lastMsg.Role == "assistant" {
		history = append(history, map[string]interface{}{
			"assistantResponseMessage": map[string]interface{}{
				"content": textContent,
			},
		})
		textContent = "Continue"
		// 重置 toolResults 和 images（assistant 消息不应该有这些）
		lastMsg = MessageItem{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"Continue"}]`)}
	}

	// 如果 system prompt 存在但 history 为空，添加到当前消息
	// 参考 kiro-gateway line 1436-1438
	if systemPrompt != "" && len(history) == 0 {
		if textContent != "" {
			textContent = systemPrompt + "\n\n" + textContent
		} else {
			textContent = systemPrompt
		}
	}

	toolResults := extractToolResults(lastMsg.Content)
	images := extractImages(lastMsg.Content)

	// Fake reasoning 注入（参考 kiro-gateway inject_thinking_tags）
	// Kiro API 不支持 Anthropic 原生 thinking 参数，通过 XML 标签触发
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		budgetTokens := req.Thinking.BudgetTokens
		if budgetTokens <= 0 {
			budgetTokens = 4000
		}
		textContent = injectThinkingTags(textContent, budgetTokens)
		log.Printf("Injected fake reasoning tags (budget=%d)", budgetTokens)
	}

	// 空内容兜底
	if textContent == "" {
		textContent = "Continue"
	}

	userInput := map[string]interface{}{
		"content": textContent,
		"modelId": modelID,
		"origin":  "AI_EDITOR",
	}

	// userInputMessageContext 只在有内容时才包含（参考 kiro-gateway）
	ctx := buildContext(tools, toolResults)
	if len(ctx) > 0 {
		userInput["userInputMessageContext"] = ctx
	}

	// 图片放入 userInputMessage.images（Kiro 原生格式）
	if len(images) > 0 {
		userInput["images"] = images
	}

	currentMessage := map[string]interface{}{"userInputMessage": userInput}

	// conversationState：只包含 Kiro API 需要的字段
	// 参考 kiro-gateway：不包含 agentContinuationId / agentTaskType
	convState := map[string]interface{}{
		"chatTriggerType": "MANUAL",
		"conversationId":  conversationID,
		"currentMessage":  currentMessage,
	}
	// 只在有历史消息时才包含 history（Kiro API 不接受 null）
	if len(history) > 0 {
		convState["history"] = history
	}

	kiroReq := map[string]interface{}{
		"conversationState": convState,
	}

	body, err := json.Marshal(kiroReq)
	if err != nil {
		return nil, err
	}
	log.Printf("Kiro request: model=%s convId=%s historyLen=%d toolsLen=%d bodyLen=%d",
		modelID, conversationID, len(history), len(tools), len(body))
	return body, nil
}

func extractSessionID(userID string) string {
	idx := strings.Index(userID, "session_")
	if idx == -1 {
		return ""
	}
	part := userID[idx+8:]
	if len(part) >= 36 && strings.Count(part[:36], "-") == 4 {
		return part[:36]
	}
	return ""
}

func extractSystemPrompt(raw json.RawMessage) string {
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

type parsedContent struct {
	blocks    []map[string]interface{}
	isString  bool
	stringVal string
}

func parseContent(content json.RawMessage) *parsedContent {
	if len(content) == 0 {
		return &parsedContent{}
	}

	// 尝试解析为字符串
	var s string
	if json.Unmarshal(content, &s) == nil {
		return &parsedContent{isString: true, stringVal: s}
	}

	// 尝试解析为数组
	var arr []map[string]interface{}
	if json.Unmarshal(content, &arr) == nil {
		return &parsedContent{blocks: arr}
	}

	return &parsedContent{}
}

func extractTextContent(content json.RawMessage) string {
	parsed := parseContent(content)
	if parsed.isString {
		return parsed.stringVal
	}

	var parts []string
	for _, item := range parsed.blocks {
		if item["type"] == "text" {
			if text, ok := item["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func extractTextContentFromParsed(parsed *parsedContent) string {
	if parsed.isString {
		return parsed.stringVal
	}

	var parts []string
	for _, item := range parsed.blocks {
		if item["type"] == "text" {
			if text, ok := item["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func extractToolResults(content json.RawMessage) []map[string]interface{} {
	var arr []map[string]interface{}
	if json.Unmarshal(content, &arr) != nil {
		return nil
	}
	var results []map[string]interface{}
	for _, item := range arr {
		if item["type"] != "tool_result" {
			continue
		}
		toolUseID, _ := item["tool_use_id"].(string)
		resultContent := ""
		switch c := item["content"].(type) {
		case string:
			resultContent = c
		case []interface{}:
			for _, ci := range c {
				if cm, ok := ci.(map[string]interface{}); ok {
					if text, ok := cm["text"].(string); ok {
						resultContent += text
					}
				}
			}
		}
		if resultContent == "" {
			resultContent = "(empty result)"
		}
		// Kiro API 要求 content 是 [{"text": "..."}] 数组格式，status 小写
		results = append(results, map[string]interface{}{
			"toolUseId": toolUseID,
			"content":   []map[string]interface{}{{"text": resultContent}},
			"status":    "success",
		})
	}
	return results
}

func extractToolUses(content json.RawMessage) []map[string]interface{} {
	var arr []map[string]interface{}
	if json.Unmarshal(content, &arr) != nil {
		return nil
	}
	var uses []map[string]interface{}
	for _, item := range arr {
		if item["type"] != "tool_use" {
			continue
		}
		uses = append(uses, map[string]interface{}{
			"toolUseId": item["id"], "name": item["name"], "input": item["input"],
		})
	}
	return uses
}

func convertTools(tools []json.RawMessage) []map[string]interface{} {
	var result []map[string]interface{}
	for _, raw := range tools {
		var tool map[string]interface{}
		if json.Unmarshal(raw, &tool) != nil {
			continue
		}
		name, _ := tool["name"].(string)
		desc, _ := tool["description"].(string)

		// 工具名校验：Kiro API 限制 64 字符
		if len(name) > 64 {
			name = name[:64]
		}

		// 空描述补位：Kiro API 要求非空 description
		if strings.TrimSpace(desc) == "" {
			desc = "Tool: " + name
		}

		// JSON Schema 清洗
		inputSchema := tool["input_schema"]
		if schemaMap, ok := inputSchema.(map[string]interface{}); ok {
			inputSchema = sanitizeJSONSchema(schemaMap)
		}

		result = append(result, map[string]interface{}{
			"toolSpecification": map[string]interface{}{
				"name":        name,
				"description": desc,
				"inputSchema": map[string]interface{}{"json": inputSchema},
			},
		})
	}
	return result
}

// sanitizeJSONSchema 清洗 JSON Schema，移除 Kiro API 不支持的字段
// 参考 kiro-gateway: additionalProperties 和空 required 会导致 400 错误
func sanitizeJSONSchema(schema map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range schema {
		// 跳过空 required 数组
		if key == "required" {
			if arr, ok := value.([]interface{}); ok && len(arr) == 0 {
				continue
			}
		}
		// 跳过 additionalProperties
		if key == "additionalProperties" {
			continue
		}
		// 递归处理嵌套 properties
		if key == "properties" {
			if props, ok := value.(map[string]interface{}); ok {
				cleaned := make(map[string]interface{})
				for propName, propValue := range props {
					if propMap, ok := propValue.(map[string]interface{}); ok {
						cleaned[propName] = sanitizeJSONSchema(propMap)
					} else {
						cleaned[propName] = propValue
					}
				}
				result[key] = cleaned
				continue
			}
		}
		// 递归处理其他 map 类型
		if m, ok := value.(map[string]interface{}); ok {
			result[key] = sanitizeJSONSchema(m)
			continue
		}
		// 递归处理数组（anyOf, oneOf 等）
		if arr, ok := value.([]interface{}); ok {
			cleaned := make([]interface{}, len(arr))
			for i, item := range arr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					cleaned[i] = sanitizeJSONSchema(itemMap)
				} else {
					cleaned[i] = item
				}
			}
			result[key] = cleaned
			continue
		}
		result[key] = value
	}
	return result
}

// extractImages 从消息内容中提取图片，转换为 Kiro 格式
// 支持 Anthropic 格式: {"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"..."}}
// 支持 OpenAI 格式: {"type":"image_url","image_url":{"url":"data:image/jpeg;base64,..."}}
func extractImages(content json.RawMessage) []map[string]interface{} {
	var arr []map[string]interface{}
	if json.Unmarshal(content, &arr) != nil {
		return nil
	}
	var images []map[string]interface{}
	for _, item := range arr {
		itemType, _ := item["type"].(string)

		// Anthropic 格式
		if itemType == "image" {
			source, ok := item["source"].(map[string]interface{})
			if !ok {
				continue
			}
			if source["type"] != "base64" {
				continue
			}
			mediaType, _ := source["media_type"].(string)
			data, _ := source["data"].(string)
			if data == "" {
				continue
			}
			format := "jpeg"
			if idx := strings.LastIndex(mediaType, "/"); idx >= 0 {
				format = mediaType[idx+1:]
			}
			images = append(images, map[string]interface{}{
				"format": format,
				"source": map[string]interface{}{"bytes": data},
			})
		}

		// OpenAI 格式
		if itemType == "image_url" {
			imgURL, ok := item["image_url"].(map[string]interface{})
			if !ok {
				continue
			}
			url, _ := imgURL["url"].(string)
			if !strings.HasPrefix(url, "data:") {
				continue
			}
			// 解析 data:image/jpeg;base64,/9j/...
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
			format := "jpeg"
			if idx := strings.LastIndex(mediaType, "/"); idx >= 0 {
				format = mediaType[idx+1:]
			}
			images = append(images, map[string]interface{}{
				"format": format,
				"source": map[string]interface{}{"bytes": data},
			})
		}
	}
	return images
}

func buildContext(tools []map[string]interface{}, toolResults []map[string]interface{}) map[string]interface{} {
	ctx := map[string]interface{}{}
	if len(tools) > 0 {
		ctx["tools"] = tools
	}
	if len(toolResults) > 0 {
		ctx["toolResults"] = toolResults
	}
	return ctx
}

func buildHistory(messages []MessageItem, modelID string) []interface{} {
	var history []interface{}
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			content := extractTextContent(msg.Content)
			if content == "" {
				content = "(empty)"
			}
			userInput := map[string]interface{}{
				"content": content,
				"modelId": modelID,
				"origin":  "AI_EDITOR",
			}
			// userInputMessageContext 只在有 toolResults 时才包含
			ctx := buildContext(nil, extractToolResults(msg.Content))
			if len(ctx) > 0 {
				userInput["userInputMessageContext"] = ctx
			}
			history = append(history, map[string]interface{}{
				"userInputMessage": userInput,
			})
		case "assistant":
			content := extractTextContent(msg.Content)
			if content == "" {
				content = "(empty)"
			}
			am := map[string]interface{}{
				"content": content,
			}
			if toolUses := extractToolUses(msg.Content); len(toolUses) > 0 {
				am["toolUses"] = toolUses
			}
			history = append(history, map[string]interface{}{"assistantResponseMessage": am})
		}
	}
	return history
}

// ── 消息规范化流水线（参考 kiro-gateway converters_core）──
// Kiro API 要求：
// 1. 第一条消息必须是 user
// 2. user/assistant 必须交替出现
// 3. 内容不能为空
// 4. 只支持 user 和 assistant 角色

// stripAllToolContent 移除所有 tool 相关内容（tool_calls 和 tool_results）
// 参考 kiro-gateway strip_all_tool_content：
// 当请求没有 tools 定义时，Kiro API 会拒绝包含 toolResults 的请求
// 将 tool 内容转换为文本以保留上下文
func stripAllToolContent(messages []MessageItem) ([]MessageItem, bool) {
	var result []MessageItem
	hadToolContent := false

	for _, msg := range messages {
		// 检查是否有 tool_calls 或 tool_results
		toolUses := extractToolUses(msg.Content)
		toolResults := extractToolResults(msg.Content)

		if len(toolUses) == 0 && len(toolResults) == 0 {
			result = append(result, msg)
			continue
		}

		hadToolContent = true
		var contentParts []string

		// 提取原始文本内容
		originalText := extractTextContent(msg.Content)
		if originalText != "" {
			contentParts = append(contentParts, originalText)
		}

		// 转换 tool_calls 为文本
		if len(toolUses) > 0 {
			for _, tu := range toolUses {
				name, _ := tu["name"].(string)
				input, _ := tu["input"].(map[string]interface{})
				inputJSON, _ := json.Marshal(input)
				contentParts = append(contentParts, fmt.Sprintf("Tool call: %s(%s)", name, string(inputJSON)))
			}
		}

		// 转换 tool_results 为文本
		if len(toolResults) > 0 {
			for _, tr := range toolResults {
				toolUseID, _ := tr["toolUseId"].(string)
				content, _ := tr["content"].([]map[string]interface{})
				var text string
				if len(content) > 0 {
					text, _ = content[0]["text"].(string)
				}
				contentParts = append(contentParts, fmt.Sprintf("Tool result (ID: %s):\n%s", toolUseID, text))
			}
		}

		// 合并所有文本
		newContent := strings.Join(contentParts, "\n\n")
		if newContent == "" {
			newContent = "(empty)"
		}

		// 创建新消息（只保留文本）
		newMsg := MessageItem{
			Role:    msg.Role,
			Content: json.RawMessage(fmt.Sprintf(`[{"type":"text","text":%s}]`, strconv.Quote(newContent))),
		}
		result = append(result, newMsg)
	}

	if hadToolContent {
		log.Printf("[INFO] Stripped tool content from messages (no tools defined)")
	}

	return result, hadToolContent
}

// ensureAssistantBeforeToolResults 确保有 tool_results 的消息前面有 assistant with tool_calls
// 参考 kiro-gateway ensure_assistant_before_tool_results：
// 当 tool_results 没有对应的 assistant tool_calls 时（Cursor 的截断历史），
// 将 tool_results 转换为文本追加到消息内容中
func ensureAssistantBeforeToolResults(messages []MessageItem) []MessageItem {
	if len(messages) == 0 {
		return messages
	}

	var result []MessageItem
	for _, msg := range messages {
		// 检查当前消息是否有 tool_results
		toolResults := extractToolResults(msg.Content)
		if len(toolResults) == 0 {
			result = append(result, msg)
			continue
		}

		// 检查前一条消息是否是 assistant with tool_calls
		hasPrecedingAssistant := false
		if len(result) > 0 {
			prev := result[len(result)-1]
			if prev.Role == "assistant" {
				prevToolUses := extractToolUses(prev.Content)
				hasPrecedingAssistant = len(prevToolUses) > 0
			}
		}

		if !hasPrecedingAssistant {
			// Orphaned tool_results：转换为文本
			log.Printf("[WARN] Converting %d orphaned tool_results to text (no preceding assistant with tool_calls)", len(toolResults))

			// 提取 tool_results 的文本表示
			var toolTexts []string
			for _, tr := range toolResults {
				toolUseID, _ := tr["toolUseId"].(string)
				content, _ := tr["content"].([]map[string]interface{})
				var text string
				if len(content) > 0 {
					text, _ = content[0]["text"].(string)
				}
				toolTexts = append(toolTexts, fmt.Sprintf("Tool result (ID: %s):\n%s", toolUseID, text))
			}
			toolResultsText := strings.Join(toolTexts, "\n\n")

			// 提取原始文本内容
			originalText := extractTextContent(msg.Content)

			// 合并文本
			var newContent string
			if originalText != "" && toolResultsText != "" {
				newContent = originalText + "\n\n" + toolResultsText
			} else if toolResultsText != "" {
				newContent = toolResultsText
			} else {
				newContent = originalText
			}

			// 创建新消息（只保留文本，移除 tool_results）
			newMsg := MessageItem{
				Role:    msg.Role,
				Content: json.RawMessage(fmt.Sprintf(`[{"type":"text","text":%s}]`, strconv.Quote(newContent))),
			}
			result = append(result, newMsg)
			continue
		}

		result = append(result, msg)
	}
	return result
}

// normalizeMessagePipeline 消息规范化流水线（参考 kiro-gateway build_kiro_payload）
// 正确顺序（line 1391-1415）：
// 1. strip_all_tool_content (if no tools) / ensure_assistant_before_tool_results
// 2. merge_adjacent_messages
// 3. ensure_first_message_is_user
// 4. normalize_message_roles
// 5. ensure_alternating_roles
func normalizeMessagePipeline(messages []MessageItem, hasTools bool) []MessageItem {
	if len(messages) == 0 {
		return messages
	}

	var processed []MessageItem

	// 1. 如果没有 tools，移除所有 tool 内容；否则确保 tool_results 前有 assistant
	if !hasTools {
		processed, _ = stripAllToolContent(messages)
	} else {
		processed = ensureAssistantBeforeToolResults(messages)
	}

	// 2. 合并相邻同角色消息（保留所有 content blocks）
	merged := mergeAdjacentMessages(processed)

	// 3. 确保第一条消息是 user
	merged = ensureFirstMessageIsUser(merged)

	// 4. 角色规范化：非 user/assistant → user
	// 必须在 ensure_alternating_roles 之前，以便正确检测连续的 user 消息
	merged = normalizeRoles(merged)

	// 5. 确保 user/assistant 交替
	merged = ensureAlternatingRoles(merged)

	return merged
}

// normalizeRoles 将非 user/assistant 角色规范化为 user
func normalizeRoles(messages []MessageItem) []MessageItem {
	result := make([]MessageItem, len(messages))
	for i, msg := range messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			result[i] = MessageItem{Role: "user", Content: msg.Content}
		} else {
			result[i] = msg
		}
	}
	return result
}

// mergeAdjacentMessages 合并相邻同角色消息
// 保留所有 content blocks（text、tool_use、tool_result 等），不仅仅是文本
func mergeAdjacentMessages(messages []MessageItem) []MessageItem {
	if len(messages) == 0 {
		return messages
	}
	result := []MessageItem{messages[0]}
	for _, msg := range messages[1:] {
		last := &result[len(result)-1]
		if msg.Role == last.Role {
			// 合并 content：将两个 JSON content 合并为一个数组
			prevBlocks := rawToBlocks(last.Content)
			nextBlocks := rawToBlocks(msg.Content)
			combined := append(prevBlocks, nextBlocks...)
			last.Content, _ = json.Marshal(combined)
		} else {
			result = append(result, msg)
		}
	}
	return result
}

// rawToBlocks 将 JSON RawMessage content 转换为 blocks 数组（保留所有类型）
func rawToBlocks(raw json.RawMessage) []interface{} {
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
	return nil
}

// ensureFirstMessageIsUser 确保第一条消息是 user
func ensureFirstMessageIsUser(messages []MessageItem) []MessageItem {
	if len(messages) == 0 || messages[0].Role == "user" {
		return messages
	}
	synthetic := MessageItem{Role: "user"}
	synthetic.Content, _ = json.Marshal("(empty)")
	return append([]MessageItem{synthetic}, messages...)
}

// ensureAlternatingRoles 确保 user/assistant 交替，插入合成消息
func ensureAlternatingRoles(messages []MessageItem) []MessageItem {
	if len(messages) < 2 {
		return messages
	}
	result := []MessageItem{messages[0]}
	for _, msg := range messages[1:] {
		prev := result[len(result)-1]
		if msg.Role == prev.Role {
			// 插入对面角色的合成消息
			syntheticRole := "assistant"
			if msg.Role == "assistant" {
				syntheticRole = "user"
			}
			synthetic := MessageItem{Role: syntheticRole}
			synthetic.Content, _ = json.Marshal("(empty)")
			result = append(result, synthetic)
		}
		result = append(result, msg)
	}
	return result
}
