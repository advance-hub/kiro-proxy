package warp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	pb "kiro-go/internal/warp/pb"

	"google.golang.org/protobuf/proto"
)

// WarpEvent 解析后的 Warp 响应事件
type WarpEvent struct {
	Type string // text_delta, tool_use, reasoning, stream_finished, etc.

	// text_delta / reasoning
	Text string

	// tool_use
	ToolUse *ClaudeToolUse

	// stream_finished
	StopReason string
	Usage      *TokenUsage
	Cost       float32
	ErrorMsg   string
}

type ClaudeToolUse struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

type TokenUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// SSEState 流式 SSE 状态机
type SSEState struct {
	MessageID         string
	Model             string
	BlockIndex        int
	TextBlockStarted  bool
	ThinkBlockStarted bool
	FullText          string
	ThinkingText      string
	ToolCalls         []*ClaudeToolUse
	Finished          bool
	Usage             *TokenUsage
	StopReason        string
	InputTokens       int
}

func NewSSEState(messageID, model string, inputTokens int) *SSEState {
	return &SSEState{
		MessageID:   messageID,
		Model:       model,
		InputTokens: inputTokens,
	}
}

// ParseWarpSSELine 解析 Warp SSE 行（data: base64encoded_protobuf）
func ParseWarpSSELine(line string) []WarpEvent {
	if !strings.HasPrefix(line, "data:") {
		return nil
	}
	data := strings.TrimSpace(line[5:])
	if data == "" {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(data)
		if err != nil {
			return nil
		}
	}

	return ParseWarpResponseEvent(decoded)
}

// ParseWarpResponseEvent 解析 Warp ResponseEvent protobuf
func ParseWarpResponseEvent(data []byte) []WarpEvent {
	respEvent := &pb.ResponseEvent{}
	if err := proto.Unmarshal(data, respEvent); err != nil {
		log.Printf("[Warp] proto.Unmarshal ResponseEvent failed: %v", err)
		return nil
	}

	var events []WarpEvent

	switch t := respEvent.Type.(type) {
	case *pb.ResponseEvent_Init:
		events = append(events, WarpEvent{Type: "stream_init"})

	case *pb.ResponseEvent_ClientActions_:
		for _, action := range t.ClientActions.GetActions() {
			actionEvents := parseClientAction(action)
			events = append(events, actionEvents...)
		}

	case *pb.ResponseEvent_Finished:
		events = append(events, parseStreamFinished(t.Finished))
	}

	return events
}

func parseClientAction(action *pb.ClientAction) []WarpEvent {
	if action == nil {
		return nil
	}

	var events []WarpEvent

	switch a := action.Action.(type) {
	case *pb.ClientAction_AppendToMessageContent_:
		if msg := a.AppendToMessageContent.GetMessage(); msg != nil {
			events = append(events, parseMessage(msg)...)
		}

	case *pb.ClientAction_AddMessagesToTask_:
		for _, msg := range a.AddMessagesToTask.GetMessages() {
			events = append(events, parseMessage(msg)...)
		}

	case *pb.ClientAction_UpdateTaskMessage_:
		if msg := a.UpdateTaskMessage.GetMessage(); msg != nil {
			events = append(events, parseMessage(msg)...)
		}
	}

	return events
}

func parseMessage(msg *pb.Message) []WarpEvent {
	var events []WarpEvent

	switch m := msg.Message.(type) {
	case *pb.Message_AgentOutput_:
		text := m.AgentOutput.GetText()
		if text != "" {
			events = append(events, WarpEvent{Type: "text_delta", Text: text})
		}

	case *pb.Message_AgentReasoning_:
		reasoning := m.AgentReasoning.GetReasoning()
		if reasoning != "" {
			events = append(events, WarpEvent{Type: "reasoning", Text: reasoning})
		}

	case *pb.Message_ToolCall_:
		tc := m.ToolCall
		id, name, input := WarpToolCallToClaudeToolUse(tc)
		if name != "" && name != "unknown" {
			events = append(events, WarpEvent{
				Type: "tool_use",
				ToolUse: &ClaudeToolUse{
					Type:  "tool_use",
					ID:    id,
					Name:  name,
					Input: input,
				},
			})
		}
	}

	return events
}

func parseStreamFinished(fin *pb.ResponseEvent_StreamFinished) WarpEvent {
	ev := WarpEvent{
		Type:       "stream_finished",
		StopReason: "end_turn",
		Usage:      &TokenUsage{},
	}

	switch fin.Reason.(type) {
	case *pb.ResponseEvent_StreamFinished_Done_:
		ev.StopReason = "end_turn"
	case *pb.ResponseEvent_StreamFinished_MaxTokenLimit:
		ev.StopReason = "max_tokens"
	case *pb.ResponseEvent_StreamFinished_QuotaLimit_:
		ev.StopReason = "quota_limit"
	case *pb.ResponseEvent_StreamFinished_ContextWindowExceeded_:
		ev.StopReason = "context_window_exceeded"
	case *pb.ResponseEvent_StreamFinished_LlmUnavailable:
		ev.StopReason = "llm_unavailable"
	case *pb.ResponseEvent_StreamFinished_InternalError_:
		ev.StopReason = "internal_error"
		if ie, ok := fin.Reason.(*pb.ResponseEvent_StreamFinished_InternalError_); ok {
			ev.ErrorMsg = ie.InternalError.GetMessage()
		}
	case *pb.ResponseEvent_StreamFinished_InvalidApiKey_:
		ev.StopReason = "invalid_api_key"
	case *pb.ResponseEvent_StreamFinished_Other_:
		ev.StopReason = "other"
	}

	// token_usage
	for _, tu := range fin.GetTokenUsage() {
		ev.Usage.InputTokens += int(tu.GetTotalInput())
		ev.Usage.OutputTokens += int(tu.GetOutput())
		ev.Usage.CacheReadInputTokens += int(tu.GetInputCacheRead())
		ev.Usage.CacheCreationInputTokens += int(tu.GetInputCacheWrite())
	}

	// request_cost
	if rc := fin.GetRequestCost(); rc != nil {
		ev.Cost = rc.GetExact()
	}

	return ev
}

// ── SSE 输出生成 ──

// GenerateMessageStartSSE 生成 message_start SSE 事件
func GenerateMessageStartSSE(state *SSEState) string {
	msg := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            state.MessageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         state.Model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  state.InputTokens,
				"output_tokens": 0,
			},
		},
	}
	data, _ := json.Marshal(msg)
	return fmt.Sprintf("event: message_start\ndata: %s\n\n", data)
}

// ProcessWarpEvent 处理 Warp 事件并生成 Claude SSE 输出
func ProcessWarpEvent(event WarpEvent, state *SSEState) string {
	var sb strings.Builder

	switch event.Type {
	case "text_delta":
		if !state.TextBlockStarted {
			// 先关闭 thinking block
			if state.ThinkBlockStarted {
				sb.WriteString(sseEvent("content_block_stop", map[string]interface{}{
					"type": "content_block_stop", "index": state.BlockIndex,
				}))
				state.BlockIndex++
				state.ThinkBlockStarted = false
			}
			sb.WriteString(sseEvent("content_block_start", map[string]interface{}{
				"type":          "content_block_start",
				"index":         state.BlockIndex,
				"content_block": map[string]interface{}{"type": "text", "text": ""},
			}))
			state.TextBlockStarted = true
		}
		sb.WriteString(sseEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": state.BlockIndex,
			"delta": map[string]interface{}{"type": "text_delta", "text": event.Text},
		}))
		state.FullText += event.Text

	case "reasoning":
		if !state.ThinkBlockStarted {
			if state.TextBlockStarted {
				sb.WriteString(sseEvent("content_block_stop", map[string]interface{}{
					"type": "content_block_stop", "index": state.BlockIndex,
				}))
				state.BlockIndex++
				state.TextBlockStarted = false
			}
			sb.WriteString(sseEvent("content_block_start", map[string]interface{}{
				"type":          "content_block_start",
				"index":         state.BlockIndex,
				"content_block": map[string]interface{}{"type": "thinking", "thinking": ""},
			}))
			state.ThinkBlockStarted = true
		}
		sb.WriteString(sseEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": state.BlockIndex,
			"delta": map[string]interface{}{"type": "thinking_delta", "thinking": event.Text},
		}))
		state.ThinkingText += event.Text

	case "tool_use":
		// 关闭之前的块
		if state.TextBlockStarted {
			sb.WriteString(sseEvent("content_block_stop", map[string]interface{}{
				"type": "content_block_stop", "index": state.BlockIndex,
			}))
			state.BlockIndex++
			state.TextBlockStarted = false
		}
		if state.ThinkBlockStarted {
			sb.WriteString(sseEvent("content_block_stop", map[string]interface{}{
				"type": "content_block_stop", "index": state.BlockIndex,
			}))
			state.BlockIndex++
			state.ThinkBlockStarted = false
		}

		tu := event.ToolUse
		sb.WriteString(sseEvent("content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": state.BlockIndex,
			"content_block": map[string]interface{}{
				"type": "tool_use", "id": tu.ID, "name": tu.Name, "input": map[string]interface{}{},
			},
		}))
		inputJSON, _ := json.Marshal(tu.Input)
		sb.WriteString(sseEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": state.BlockIndex,
			"delta": map[string]interface{}{"type": "input_json_delta", "partial_json": string(inputJSON)},
		}))
		sb.WriteString(sseEvent("content_block_stop", map[string]interface{}{
			"type": "content_block_stop", "index": state.BlockIndex,
		}))
		state.ToolCalls = append(state.ToolCalls, tu)
		state.BlockIndex++

	case "stream_finished":
		// 关闭打开的块
		if state.ThinkBlockStarted {
			sb.WriteString(sseEvent("content_block_stop", map[string]interface{}{
				"type": "content_block_stop", "index": state.BlockIndex,
			}))
			state.BlockIndex++
			state.ThinkBlockStarted = false
		}
		if state.TextBlockStarted {
			sb.WriteString(sseEvent("content_block_stop", map[string]interface{}{
				"type": "content_block_stop", "index": state.BlockIndex,
			}))
			state.TextBlockStarted = false
		}

		stopReason := event.StopReason
		if len(state.ToolCalls) > 0 && stopReason == "end_turn" {
			stopReason = "tool_use"
		}

		outputTokens := 0
		if event.Usage != nil {
			outputTokens = event.Usage.OutputTokens
		}

		sb.WriteString(sseEvent("message_delta", map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": stopReason, "stop_sequence": nil},
			"usage": map[string]interface{}{"output_tokens": outputTokens},
		}))
		sb.WriteString(sseEvent("message_stop", map[string]interface{}{"type": "message_stop"}))

		state.Finished = true
		state.Usage = event.Usage
		state.StopReason = stopReason
	}

	return sb.String()
}

// GenerateFinalSSE 生成最终 SSE 事件（如果流未正常结束）
func GenerateFinalSSE(state *SSEState) string {
	if state.Finished {
		return ""
	}
	var sb strings.Builder
	if state.ThinkBlockStarted {
		sb.WriteString(sseEvent("content_block_stop", map[string]interface{}{
			"type": "content_block_stop", "index": state.BlockIndex,
		}))
		state.BlockIndex++
	}
	if state.TextBlockStarted {
		sb.WriteString(sseEvent("content_block_stop", map[string]interface{}{
			"type": "content_block_stop", "index": state.BlockIndex,
		}))
	}
	stopReason := "end_turn"
	if len(state.ToolCalls) > 0 {
		stopReason = "tool_use"
	}
	sb.WriteString(sseEvent("message_delta", map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]interface{}{"output_tokens": estimateTokens(state.FullText)},
	}))
	sb.WriteString(sseEvent("message_stop", map[string]interface{}{"type": "message_stop"}))
	return sb.String()
}

// BuildClaudeNonStreamResponse 构建非流式 Claude API 响应
func BuildClaudeNonStreamResponse(state *SSEState) map[string]interface{} {
	var content []interface{}

	if state.ThinkingText != "" {
		content = append(content, map[string]interface{}{
			"type": "thinking", "thinking": state.ThinkingText,
		})
	}
	content = append(content, map[string]interface{}{
		"type": "text", "text": state.FullText,
	})
	for _, tu := range state.ToolCalls {
		content = append(content, map[string]interface{}{
			"type": "tool_use", "id": tu.ID, "name": tu.Name, "input": tu.Input,
		})
	}

	stopReason := state.StopReason
	if stopReason == "" {
		stopReason = "end_turn"
	}
	if len(state.ToolCalls) > 0 && stopReason == "end_turn" {
		stopReason = "tool_use"
	}

	inputTokens := state.InputTokens
	outputTokens := 0
	if state.Usage != nil {
		if state.Usage.InputTokens > 0 {
			inputTokens = state.Usage.InputTokens
		}
		outputTokens = state.Usage.OutputTokens
	}

	return map[string]interface{}{
		"id":            state.MessageID,
		"type":          "message",
		"role":          "assistant",
		"content":       content,
		"model":         state.Model,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]int{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}
}

func sseEvent(eventType string, data interface{}) string {
	jsonData, _ := json.Marshal(data)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, jsonData)
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// 粗略估算
	return len(text) * 2 / 5
}
