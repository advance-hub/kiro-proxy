package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kiro-go/internal/common"
	"kiro-go/internal/kiro"
	"kiro-go/internal/kiro/parser"
	"kiro-go/internal/logger"

	"github.com/google/uuid"
)

// HandleGetModels GET /v1/models
func HandleGetModels(w http.ResponseWriter, r *http.Request, provider *kiro.Provider) {
	models, err := provider.GetModels()
	if err != nil {
		logger.Warnf(logger.CatSystem, "获取模型列表失败: %v，返回默认列表", err)
		// 返回默认模型列表作为 fallback（匹配真实 API 格式）
		models = []map[string]interface{}{
			{
				"id": "auto", "object": "model", "created": time.Now().Unix(), "owned_by": "anthropic", "display_name": "Auto", "type": "chat", "max_tokens": 200000,
				"modelId": "auto", "modelName": "Auto", "description": "Models chosen by task for optimal usage and consistent quality",
				"rateMultiplier": 1.0, "rateUnit": "Credit",
				"supportedInputTypes": []string{"TEXT", "IMAGE"},
				"tokenLimits":         map[string]interface{}{"maxInputTokens": 200000, "maxOutputTokens": nil},
				"promptCaching":       map[string]interface{}{"supportsPromptCaching": true, "maximumCacheCheckpointsPerRequest": 4, "minimumTokensPerCacheCheckpoint": 1024},
			},
			{
				"id": "claude-opus-4.6", "object": "model", "created": time.Now().Unix(), "owned_by": "anthropic", "display_name": "Claude Opus 4.6", "type": "chat", "max_tokens": 200000,
				"modelId": "claude-opus-4.6", "modelName": "Claude Opus 4.6", "description": "Experimental preview of Claude Opus 4.6",
				"rateMultiplier": 2.2, "rateUnit": "Credit",
				"supportedInputTypes": []string{"TEXT", "IMAGE"},
				"tokenLimits":         map[string]interface{}{"maxInputTokens": 200000, "maxOutputTokens": nil},
				"promptCaching":       map[string]interface{}{"supportsPromptCaching": true, "maximumCacheCheckpointsPerRequest": 4, "minimumTokensPerCacheCheckpoint": 4096},
			},
			{
				"id": "claude-sonnet-4.6", "object": "model", "created": time.Now().Unix(), "owned_by": "anthropic", "display_name": "Claude Sonnet 4.6", "type": "chat", "max_tokens": 200000,
				"modelId": "claude-sonnet-4.6", "modelName": "Claude Sonnet 4.6", "description": "Experimental preview of the latest Claude Sonnet model",
				"rateMultiplier": 1.3, "rateUnit": "Credit",
				"supportedInputTypes": []string{"TEXT", "IMAGE"},
				"tokenLimits":         map[string]interface{}{"maxInputTokens": 200000, "maxOutputTokens": nil},
				"promptCaching":       map[string]interface{}{"supportsPromptCaching": true, "maximumCacheCheckpointsPerRequest": 4, "minimumTokensPerCacheCheckpoint": 1024},
			},
			{
				"id": "claude-opus-4.5", "object": "model", "created": time.Now().Unix(), "owned_by": "anthropic", "display_name": "Claude Opus 4.5", "type": "chat", "max_tokens": 200000,
				"modelId": "claude-opus-4.5", "modelName": "Claude Opus 4.5", "description": "The Claude Opus 4.5 model",
				"rateMultiplier": 2.2, "rateUnit": "Credit",
				"supportedInputTypes": []string{"TEXT", "IMAGE"},
				"tokenLimits":         map[string]interface{}{"maxInputTokens": 200000, "maxOutputTokens": nil},
				"promptCaching":       map[string]interface{}{"supportsPromptCaching": true, "maximumCacheCheckpointsPerRequest": 4, "minimumTokensPerCacheCheckpoint": 4096},
			},
			{
				"id": "claude-sonnet-4.5", "object": "model", "created": time.Now().Unix(), "owned_by": "anthropic", "display_name": "Claude Sonnet 4.5", "type": "chat", "max_tokens": 200000,
				"modelId": "claude-sonnet-4.5", "modelName": "Claude Sonnet 4.5", "description": "The Claude Sonnet 4.5 model",
				"rateMultiplier": 1.3, "rateUnit": "Credit",
				"supportedInputTypes": []string{"TEXT", "IMAGE"},
				"tokenLimits":         map[string]interface{}{"maxInputTokens": 200000, "maxOutputTokens": nil},
				"promptCaching":       map[string]interface{}{"supportsPromptCaching": true, "maximumCacheCheckpointsPerRequest": 4, "minimumTokensPerCacheCheckpoint": 1024},
			},
			{
				"id": "claude-sonnet-4", "object": "model", "created": time.Now().Unix(), "owned_by": "anthropic", "display_name": "Claude Sonnet 4", "type": "chat", "max_tokens": 200000,
				"modelId": "claude-sonnet-4", "modelName": "Claude Sonnet 4", "description": "Hybrid reasoning and coding for regular use",
				"rateMultiplier": 1.3, "rateUnit": "Credit",
				"supportedInputTypes": []string{"TEXT", "IMAGE"},
				"tokenLimits":         map[string]interface{}{"maxInputTokens": 200000, "maxOutputTokens": nil},
				"promptCaching":       map[string]interface{}{"supportsPromptCaching": true, "maximumCacheCheckpointsPerRequest": 4, "minimumTokensPerCacheCheckpoint": 1024},
			},
			{
				"id": "claude-haiku-4.5", "object": "model", "created": time.Now().Unix(), "owned_by": "anthropic", "display_name": "Claude Haiku 4.5", "type": "chat", "max_tokens": 200000,
				"modelId": "claude-haiku-4.5", "modelName": "Claude Haiku 4.5", "description": "The latest Claude Haiku model",
				"rateMultiplier": 0.4, "rateUnit": "Credit",
				"supportedInputTypes": []string{"TEXT", "IMAGE"},
				"tokenLimits":         map[string]interface{}{"maxInputTokens": 200000, "maxOutputTokens": nil},
				"promptCaching":       map[string]interface{}{"supportsPromptCaching": true, "maximumCacheCheckpointsPerRequest": 4, "minimumTokensPerCacheCheckpoint": 4096},
			},
		}
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"object": "list", "data": models})
}

// HandlePostMessages POST /v1/messages
func HandlePostMessages(w http.ResponseWriter, r *http.Request, provider *kiro.Provider) {
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

	// 检测模型名是否包含 "thinking" 后缀，覆写 thinking 配置
	overrideThinkingFromModelName(&req)

	logger.InfoFields(logger.CatRequest, "POST /v1/messages", logger.F{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"stream":     req.Stream,
		"messages":   len(req.Messages),
		"thinking":   req.Thinking != nil && req.Thinking.Type == "enabled",
	})

	// WebSearch 路由：tools 只有 web_search 时走 MCP
	if HasWebSearchTool(&req) {
		logger.Infof(logger.CatRequest, "检测到WebSearch请求，走MCP路由")
		HandleWebSearchRequest(w, &req, provider)
		return
	}

	kiroBody, err := ConvertToKiroRequest(&req)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	creds := common.GetCredsFromContext(r)
	actCode := common.GetActCodeFromContext(r)
	var resp *http.Response
	if creds != nil {
		resp, err = provider.CallWithCredentials(kiroBody, creds, actCode)
	} else {
		resp, _, err = provider.CallWithTokenManager(kiroBody)
	}
	if err != nil {
		logger.Errorf(logger.CatProxy, "Kiro API调用失败: %v", err)
		common.WriteError(w, http.StatusBadGateway, "api_error", "Kiro API error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		logger.ErrorFields(logger.CatResponse, "Kiro API返回错误", logger.F{
			"status": resp.StatusCode,
			"body":   logger.TruncateBody(string(respBody), 500),
		})
		common.WriteError(w, resp.StatusCode, "api_error", fmt.Sprintf("Upstream error: %d %s", resp.StatusCode, string(respBody)))
		return
	}

	thinkingEnabled := req.Thinking != nil && req.Thinking.Type == "enabled"

	if req.Stream {
		handleStreamResponse(w, resp, &req, thinkingEnabled)
	} else {
		handleNonStreamResponse(w, resp, &req, thinkingEnabled)
	}
}

// overrideThinkingFromModelName 检测模型名中的 "thinking" 后缀
func overrideThinkingFromModelName(req *MessagesRequest) {
	lower := strings.ToLower(req.Model)
	if strings.Contains(lower, "thinking") {
		if req.Thinking == nil {
			req.Thinking = &ThinkingConfig{Type: "enabled", BudgetTokens: 10000}
		}
	}
}

// readKiroEvents 从 HTTP 响应中读取并解析 AWS Event Stream 事件
func readKiroEvents(resp *http.Response) []*kiro.Event {
	decoder := parser.NewDecoder()
	var events []*kiro.Event

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			decoder.Feed(buf[:n])
			frames, _ := decoder.Decode()
			for _, frame := range frames {
				event, err := kiro.ParseEvent(frame)
				if err != nil {
					logger.Warnf(logger.CatStream, "解析事件失败: %v", err)
					continue
				}
				events = append(events, event)
			}
		}
		if err != nil {
			break
		}
	}
	return events
}

// handleStreamResponse 流式响应（使用 AWS Event Stream 解析 + SSE 状态机）
func handleStreamResponse(w http.ResponseWriter, resp *http.Response, req *MessagesRequest, thinkingEnabled bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		common.WriteError(w, http.StatusInternalServerError, "api_error", "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ctx := NewStreamContext(req.Model, 0, thinkingEnabled)

	// 发送初始事件
	for _, e := range ctx.GenerateInitialEvents() {
		e.Write(w, flusher)
	}

	// 解析 AWS Event Stream 并转换为 Anthropic SSE
	decoder := parser.NewDecoder()
	buf := make([]byte, 32*1024)
	pingTicker := time.NewTicker(25 * time.Second)
	defer pingTicker.Stop()

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-pingTicker.C:
				fmt.Fprintf(w, "event: ping\ndata: {\"type\": \"ping\"}\n\n")
				flusher.Flush()
			case <-done:
				return
			}
		}
	}()

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			decoder.Feed(buf[:n])
			frames, _ := decoder.Decode()
			for _, frame := range frames {
				event, parseErr := kiro.ParseEvent(frame)
				if parseErr != nil {
					logger.Warnf(logger.CatStream, "解析事件失败: %v", parseErr)
					continue
				}
				sseEvents := ctx.ProcessKiroEvent(event)
				for _, e := range sseEvents {
					e.Write(w, flusher)
				}
			}
		}
		if err != nil {
			break
		}
	}

	close(done)

	// 发送最终事件
	for _, e := range ctx.GenerateFinalEvents() {
		e.Write(w, flusher)
	}
}

// handleNonStreamResponse 非流式响应
// 通过 StreamContext 正确提取 thinking blocks，避免 <thinking> 标签泄漏到 text 中
func handleNonStreamResponse(w http.ResponseWriter, resp *http.Response, req *MessagesRequest, thinkingEnabled bool) {
	ctx := NewStreamContext(req.Model, 0, thinkingEnabled)
	ctx.GenerateInitialEvents() // 初始化状态

	events := readKiroEvents(resp)

	var fullText strings.Builder
	var fullThinking strings.Builder

	// tool_use 收集器（累积增量输入）
	type toolUseCollector struct {
		ID    string
		Name  string
		Input strings.Builder
	}
	toolCollectors := make(map[string]*toolUseCollector)
	var toolOrder []string

	// 通过 StreamContext 处理，正确分离 thinking 和 text
	for _, event := range events {
		sseEvents := ctx.ProcessKiroEvent(event)
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
					}
				case "thinking_delta":
					thinking, _ := delta["thinking"].(string)
					if thinking != "" {
						fullThinking.WriteString(thinking)
					}
				}
			}
		}

		// 收集 tool_use（累积所有增量输入，而非仅 ToolStop）
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
			}
		}
	}

	// Flush StreamContext 中残留的 thinking buffer
	for _, sseEvent := range ctx.GenerateFinalEvents() {
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
				}
			case "thinking_delta":
				thinking, _ := delta["thinking"].(string)
				if thinking != "" {
					fullThinking.WriteString(thinking)
				}
			}
		}
	}

	// 构建 content blocks
	var content []map[string]interface{}

	// thinking block 放在最前面
	if fullThinking.Len() > 0 {
		content = append(content, map[string]interface{}{
			"type":     "thinking",
			"thinking": fullThinking.String(),
		})
	}

	// text block
	content = append(content, map[string]interface{}{
		"type": "text",
		"text": fullText.String(),
	})

	// tool_use blocks
	for _, id := range toolOrder {
		tc := toolCollectors[id]
		var input interface{}
		if json.Unmarshal([]byte(tc.Input.String()), &input) != nil {
			input = map[string]interface{}{}
		}
		content = append(content, map[string]interface{}{
			"type": "tool_use", "id": tc.ID, "name": tc.Name, "input": input,
		})
	}

	stopReason := ctx.stateMgr.getStopReason()
	if len(toolOrder) > 0 && stopReason == "end_turn" {
		stopReason = "tool_use"
	}

	finalInputTokens := 0
	if ctx.ContextInputToks != nil {
		finalInputTokens = *ctx.ContextInputToks
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id": "msg_" + uuid.New().String()[:24], "type": "message", "role": "assistant",
		"content": content, "model": req.Model,
		"stop_reason": stopReason, "stop_sequence": nil,
		"usage": map[string]int{"input_tokens": finalInputTokens, "output_tokens": ctx.OutputTokens},
	})
}
