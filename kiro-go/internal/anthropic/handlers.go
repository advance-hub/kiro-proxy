package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"kiro-go/internal/common"
	"kiro-go/internal/kiro"
	"kiro-go/internal/kiro/parser"

	"github.com/google/uuid"
)

// HandleGetModels GET /v1/models
func HandleGetModels(w http.ResponseWriter, r *http.Request) {
	models := []map[string]interface{}{
		{"id": "claude-sonnet-4-5-20250929", "object": "model", "created": 1727568000, "owned_by": "anthropic", "display_name": "Claude Sonnet 4.5", "type": "chat", "max_tokens": 32000},
		{"id": "claude-sonnet-4-5-20250929-thinking", "object": "model", "created": 1727568000, "owned_by": "anthropic", "display_name": "Claude Sonnet 4.5 (Thinking)", "type": "chat", "max_tokens": 32000},
		{"id": "claude-opus-4-5-20251101", "object": "model", "created": 1730419200, "owned_by": "anthropic", "display_name": "Claude Opus 4.5", "type": "chat", "max_tokens": 32000},
		{"id": "claude-opus-4-5-20251101-thinking", "object": "model", "created": 1730419200, "owned_by": "anthropic", "display_name": "Claude Opus 4.5 (Thinking)", "type": "chat", "max_tokens": 32000},
		{"id": "claude-opus-4-6", "object": "model", "created": 1770314400, "owned_by": "anthropic", "display_name": "Claude Opus 4.6", "type": "chat", "max_tokens": 32000},
		{"id": "claude-opus-4-6-thinking", "object": "model", "created": 1770314400, "owned_by": "anthropic", "display_name": "Claude Opus 4.6 (Thinking)", "type": "chat", "max_tokens": 32000},
		{"id": "claude-haiku-4-5-20251001", "object": "model", "created": 1727740800, "owned_by": "anthropic", "display_name": "Claude Haiku 4.5", "type": "chat", "max_tokens": 32000},
		{"id": "claude-haiku-4-5-20251001-thinking", "object": "model", "created": 1727740800, "owned_by": "anthropic", "display_name": "Claude Haiku 4.5 (Thinking)", "type": "chat", "max_tokens": 32000},
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

	log.Printf("POST /v1/messages model=%s max_tokens=%d stream=%v messages=%d thinking=%v",
		req.Model, req.MaxTokens, req.Stream, len(req.Messages), req.Thinking != nil && req.Thinking.Type == "enabled")

	// WebSearch 路由：tools 只有 web_search 时走 MCP
	if HasWebSearchTool(&req) {
		log.Printf("检测到 WebSearch 请求，走 MCP 路由")
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
		log.Printf("Kiro API 调用失败: %v", err)
		common.WriteError(w, http.StatusBadGateway, "api_error", "Kiro API error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("Kiro API 返回错误: %d %s", resp.StatusCode, string(respBody))
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
					log.Printf("解析事件失败: %v", err)
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
					log.Printf("解析事件失败: %v", parseErr)
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
func handleNonStreamResponse(w http.ResponseWriter, resp *http.Response, req *MessagesRequest, thinkingEnabled bool) {
	ctx := NewStreamContext(req.Model, 0, thinkingEnabled)

	// 读取所有事件
	events := readKiroEvents(resp)
	for _, event := range events {
		ctx.ProcessKiroEvent(event)
	}

	// 收集内容
	var contentBlocks []map[string]interface{}
	var fullText strings.Builder

	// 重新处理事件收集内容
	for _, event := range events {
		switch event.Type {
		case "assistant_response":
			fullText.WriteString(event.Content)
		case "tool_use":
			if event.ToolStop {
				// 解析工具输入
				var input interface{}
				if json.Unmarshal([]byte(event.ToolInput), &input) != nil {
					input = map[string]interface{}{}
				}
				contentBlocks = append(contentBlocks, map[string]interface{}{
					"type": "tool_use", "id": event.ToolUseID, "name": event.ToolName, "input": input,
				})
			}
		}
	}

	// 构建响应
	content := []map[string]interface{}{{"type": "text", "text": fullText.String()}}
	content = append(content, contentBlocks...)

	stopReason := "end_turn"
	if len(contentBlocks) > 0 {
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
