package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"kiro-go/internal/kiro"

	"github.com/google/uuid"
)

// ── SSE Event ──

type SSEEvent struct {
	Event string
	Data  interface{}
}

func (e *SSEEvent) Write(w http.ResponseWriter, flusher http.Flusher) {
	data, _ := json.Marshal(e.Data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Event, string(data))
	flusher.Flush()
}

// ── Block State ──

type blockState struct {
	blockType string
	started   bool
	stopped   bool
}

// ── SSE State Manager ──

type sseStateManager struct {
	messageStarted   bool
	messageDeltaSent bool
	activeBlocks     map[int]*blockState
	messageEnded     bool
	nextBlockIdx     int
	stopReason       string
	hasToolUse       bool
}

func newSSEStateManager() *sseStateManager {
	return &sseStateManager{activeBlocks: make(map[int]*blockState)}
}

func (m *sseStateManager) nextBlockIndex() int {
	idx := m.nextBlockIdx
	m.nextBlockIdx++
	return idx
}

func (m *sseStateManager) isBlockOpenOfType(index int, expectedType string) bool {
	b, ok := m.activeBlocks[index]
	return ok && b.started && !b.stopped && b.blockType == expectedType
}

func (m *sseStateManager) hasNonThinkingBlocks() bool {
	for _, b := range m.activeBlocks {
		if b.blockType != "thinking" {
			return true
		}
	}
	return false
}

func (m *sseStateManager) getStopReason() string {
	if m.stopReason != "" {
		return m.stopReason
	}
	if m.hasToolUse {
		return "tool_use"
	}
	return "end_turn"
}

func (m *sseStateManager) handleMessageStart(data interface{}) *SSEEvent {
	if m.messageStarted {
		return nil
	}
	m.messageStarted = true
	return &SSEEvent{Event: "message_start", Data: data}
}

func (m *sseStateManager) handleContentBlockStart(index int, blockType string, event *SSEEvent) []*SSEEvent {
	var events []*SSEEvent

	// tool_use 块开始时，先关闭之前的文本块
	if blockType == "tool_use" {
		m.hasToolUse = true
		for idx, b := range m.activeBlocks {
			if b.blockType == "text" && b.started && !b.stopped {
				events = append(events, &SSEEvent{Event: "content_block_stop", Data: map[string]interface{}{"type": "content_block_stop", "index": idx}})
				b.stopped = true
			}
		}
	}

	if b, ok := m.activeBlocks[index]; ok {
		if b.started {
			return events
		}
		b.started = true
	} else {
		m.activeBlocks[index] = &blockState{blockType: blockType, started: true}
	}

	events = append(events, event)
	return events
}

func (m *sseStateManager) handleContentBlockDelta(index int, event *SSEEvent) *SSEEvent {
	b, ok := m.activeBlocks[index]
	if !ok || !b.started || b.stopped {
		return nil
	}
	return event
}

func (m *sseStateManager) handleContentBlockStop(index int) *SSEEvent {
	b, ok := m.activeBlocks[index]
	if !ok || b.stopped {
		return nil
	}
	b.stopped = true
	return &SSEEvent{Event: "content_block_stop", Data: map[string]interface{}{"type": "content_block_stop", "index": index}}
}

func (m *sseStateManager) generateFinalEvents(inputTokens, outputTokens int) []*SSEEvent {
	var events []*SSEEvent

	// 关闭所有未关闭的块
	for idx, b := range m.activeBlocks {
		if b.started && !b.stopped {
			events = append(events, &SSEEvent{Event: "content_block_stop", Data: map[string]interface{}{"type": "content_block_stop", "index": idx}})
			b.stopped = true
		}
	}

	if !m.messageDeltaSent {
		m.messageDeltaSent = true
		events = append(events, &SSEEvent{Event: "message_delta", Data: map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": m.getStopReason(), "stop_sequence": nil},
			"usage": map[string]int{"input_tokens": inputTokens, "output_tokens": outputTokens},
		}})
	}

	if !m.messageEnded {
		m.messageEnded = true
		events = append(events, &SSEEvent{Event: "message_stop", Data: map[string]interface{}{"type": "message_stop"}})
	}

	return events
}

// ── Stream Context ──

const contextWindowSize = 200000

// StreamContext 流处理上下文（Kiro → Anthropic SSE 状态机）
type StreamContext struct {
	stateMgr         *sseStateManager
	Model            string
	MessageID        string
	InputTokens      int
	ContextInputToks *int // 从 contextUsageEvent 计算
	OutputTokens     int
	ThinkingEnabled  bool

	// thinking 状态
	thinkingBuffer              string
	inThinkingBlock             bool
	thinkingExtracted           bool
	thinkingBlockIndex          *int
	textBlockIndex              *int
	stripThinkingLeadingNewline bool

	// tool 块索引映射
	toolBlockIndices map[string]int
}

func NewStreamContext(model string, inputTokens int, thinkingEnabled bool) *StreamContext {
	return &StreamContext{
		stateMgr:         newSSEStateManager(),
		Model:            model,
		MessageID:        "msg_" + uuid.New().String()[:24],
		InputTokens:      inputTokens,
		ThinkingEnabled:  thinkingEnabled,
		toolBlockIndices: make(map[string]int),
	}
}

// GenerateInitialEvents 生成初始事件
func (ctx *StreamContext) GenerateInitialEvents() []*SSEEvent {
	var events []*SSEEvent

	msgStart := ctx.stateMgr.handleMessageStart(map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id": ctx.MessageID, "type": "message", "role": "assistant",
			"content": []interface{}{}, "model": ctx.Model,
			"stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": ctx.InputTokens, "output_tokens": 1},
		},
	})
	if msgStart != nil {
		events = append(events, msgStart)
	}

	if ctx.ThinkingEnabled {
		return events // thinking 模式下不在初始化时创建文本块
	}

	// 创建初始文本块
	idx := ctx.stateMgr.nextBlockIndex()
	ctx.textBlockIndex = &idx
	startEvents := ctx.stateMgr.handleContentBlockStart(idx, "text", &SSEEvent{
		Event: "content_block_start",
		Data:  map[string]interface{}{"type": "content_block_start", "index": idx, "content_block": map[string]interface{}{"type": "text", "text": ""}},
	})
	events = append(events, startEvents...)

	return events
}

// ProcessKiroEvent 处理 Kiro 事件
func (ctx *StreamContext) ProcessKiroEvent(event *kiro.Event) []*SSEEvent {
	switch event.Type {
	case "assistant_response":
		return ctx.processAssistantResponse(event.Content)
	case "tool_use":
		return ctx.processToolUse(event)
	case "context_usage":
		actualInputTokens := int(event.ContextUsagePercentage * float64(contextWindowSize) / 100.0)
		ctx.ContextInputToks = &actualInputTokens
		if event.ContextUsagePercentage >= 100.0 {
			ctx.stateMgr.stopReason = "model_context_window_exceeded"
		}
		return nil
	case "exception":
		if event.ExceptionType == "ContentLengthExceededException" {
			ctx.stateMgr.stopReason = "max_tokens"
		}
		return nil
	default:
		return nil
	}
}

func (ctx *StreamContext) processAssistantResponse(content string) []*SSEEvent {
	if content == "" {
		return nil
	}
	ctx.OutputTokens += CountTokens(content)

	if ctx.ThinkingEnabled {
		return ctx.processContentWithThinking(content)
	}
	return ctx.createTextDeltaEvents(content)
}

// ── Thinking 处理 ──

// 引用字符集
const quoteChars = "`\"'\\#!@$%^&*()-_=+[]{};:<>,.?/"

func isQuoteChar(s string, pos int) bool {
	if pos < 0 || pos >= len(s) {
		return false
	}
	return strings.ContainsRune(quoteChars, rune(s[pos]))
}

func findRealThinkingStartTag(buffer string) int {
	tag := "<thinking>"
	searchStart := 0
	for {
		pos := strings.Index(buffer[searchStart:], tag)
		if pos == -1 {
			return -1
		}
		absPos := searchStart + pos
		hasBefore := absPos > 0 && isQuoteChar(buffer, absPos-1)
		hasAfter := isQuoteChar(buffer, absPos+len(tag))
		if !hasBefore && !hasAfter {
			return absPos
		}
		searchStart = absPos + 1
	}
}

func findRealThinkingEndTag(buffer string) int {
	tag := "</thinking>"
	searchStart := 0
	for {
		pos := strings.Index(buffer[searchStart:], tag)
		if pos == -1 {
			return -1
		}
		absPos := searchStart + pos
		hasBefore := absPos > 0 && isQuoteChar(buffer, absPos-1)
		afterPos := absPos + len(tag)
		hasAfter := isQuoteChar(buffer, afterPos)
		if hasBefore || hasAfter {
			searchStart = absPos + 1
			continue
		}
		// 真正的结束标签后面有 \n\n
		if afterPos+2 <= len(buffer) && buffer[afterPos:afterPos+2] == "\n\n" {
			return absPos
		}
		searchStart = absPos + 1
	}
}

func findRealThinkingEndTagAtBufferEnd(buffer string) int {
	tag := "</thinking>"
	searchStart := 0
	for {
		pos := strings.Index(buffer[searchStart:], tag)
		if pos == -1 {
			return -1
		}
		absPos := searchStart + pos
		hasBefore := absPos > 0 && isQuoteChar(buffer, absPos-1)
		afterPos := absPos + len(tag)
		hasAfter := isQuoteChar(buffer, afterPos)
		if hasBefore || hasAfter {
			searchStart = absPos + 1
			continue
		}
		if strings.TrimSpace(buffer[afterPos:]) == "" {
			return absPos
		}
		searchStart = absPos + 1
	}
}

func findCharBoundary(s string, target int) int {
	if target >= len(s) {
		return len(s)
	}
	for target > 0 && !utf8.RuneStart(s[target]) {
		target--
	}
	return target
}

func (ctx *StreamContext) processContentWithThinking(content string) []*SSEEvent {
	var events []*SSEEvent
	ctx.thinkingBuffer += content

	for {
		if !ctx.inThinkingBlock && !ctx.thinkingExtracted {
			startPos := findRealThinkingStartTag(ctx.thinkingBuffer)
			if startPos >= 0 {
				if startPos > 0 && strings.TrimSpace(ctx.thinkingBuffer[:startPos]) != "" {
					before := ctx.thinkingBuffer[:startPos]
					ctx.thinkingBuffer = ctx.thinkingBuffer[startPos:]
					events = append(events, ctx.createTextDeltaEvents(before)...)
				} else if startPos > 0 {
					ctx.thinkingBuffer = ctx.thinkingBuffer[startPos:]
				}

				ctx.inThinkingBlock = true
				ctx.stripThinkingLeadingNewline = true
				ctx.thinkingBuffer = ctx.thinkingBuffer[len("<thinking>"):]

				idx := ctx.stateMgr.nextBlockIndex()
				ctx.thinkingBlockIndex = &idx
				startEvents := ctx.stateMgr.handleContentBlockStart(idx, "thinking", &SSEEvent{
					Event: "content_block_start",
					Data:  map[string]interface{}{"type": "content_block_start", "index": idx, "content_block": map[string]interface{}{"type": "thinking", "thinking": ""}},
				})
				events = append(events, startEvents...)
			} else {
				targetLen := len(ctx.thinkingBuffer) - len("<thinking>")
				if targetLen < 0 {
					targetLen = 0
				}
				safeLen := findCharBoundary(ctx.thinkingBuffer, targetLen)
				if safeLen > 0 && strings.TrimSpace(ctx.thinkingBuffer[:safeLen]) != "" {
					safe := ctx.thinkingBuffer[:safeLen]
					ctx.thinkingBuffer = ctx.thinkingBuffer[safeLen:]
					events = append(events, ctx.createTextDeltaEvents(safe)...)
				}
				break
			}
		} else if ctx.inThinkingBlock {
			if ctx.stripThinkingLeadingNewline {
				if strings.HasPrefix(ctx.thinkingBuffer, "\n") {
					ctx.thinkingBuffer = ctx.thinkingBuffer[1:]
					ctx.stripThinkingLeadingNewline = false
				} else if len(ctx.thinkingBuffer) > 0 {
					ctx.stripThinkingLeadingNewline = false
				}
			}

			endPos := findRealThinkingEndTag(ctx.thinkingBuffer)
			if endPos >= 0 {
				if endPos > 0 && ctx.thinkingBlockIndex != nil {
					thinkingContent := ctx.thinkingBuffer[:endPos]
					events = append(events, ctx.createThinkingDelta(*ctx.thinkingBlockIndex, thinkingContent))
				}

				ctx.inThinkingBlock = false
				ctx.thinkingExtracted = true

				if ctx.thinkingBlockIndex != nil {
					events = append(events, ctx.createThinkingDelta(*ctx.thinkingBlockIndex, ""))
					if stop := ctx.stateMgr.handleContentBlockStop(*ctx.thinkingBlockIndex); stop != nil {
						events = append(events, stop)
					}
				}

				ctx.thinkingBuffer = ctx.thinkingBuffer[endPos+len("</thinking>\n\n"):]
			} else {
				targetLen := len(ctx.thinkingBuffer) - len("</thinking>\n\n")
				if targetLen < 0 {
					targetLen = 0
				}
				safeLen := findCharBoundary(ctx.thinkingBuffer, targetLen)
				if safeLen > 0 && ctx.thinkingBlockIndex != nil {
					safe := ctx.thinkingBuffer[:safeLen]
					ctx.thinkingBuffer = ctx.thinkingBuffer[safeLen:]
					if safe != "" {
						events = append(events, ctx.createThinkingDelta(*ctx.thinkingBlockIndex, safe))
					}
				}
				break
			}
		} else {
			// thinking 已提取完成
			if ctx.thinkingBuffer != "" {
				remaining := ctx.thinkingBuffer
				ctx.thinkingBuffer = ""
				events = append(events, ctx.createTextDeltaEvents(remaining)...)
			}
			break
		}
	}
	return events
}

func (ctx *StreamContext) createTextDeltaEvents(text string) []*SSEEvent {
	var events []*SSEEvent

	// 检查当前文本块是否已被关闭
	if ctx.textBlockIndex != nil && !ctx.stateMgr.isBlockOpenOfType(*ctx.textBlockIndex, "text") {
		ctx.textBlockIndex = nil
	}

	// 获取或创建文本块
	if ctx.textBlockIndex == nil {
		idx := ctx.stateMgr.nextBlockIndex()
		ctx.textBlockIndex = &idx
		startEvents := ctx.stateMgr.handleContentBlockStart(idx, "text", &SSEEvent{
			Event: "content_block_start",
			Data:  map[string]interface{}{"type": "content_block_start", "index": idx, "content_block": map[string]interface{}{"type": "text", "text": ""}},
		})
		events = append(events, startEvents...)
	}

	idx := *ctx.textBlockIndex
	delta := ctx.stateMgr.handleContentBlockDelta(idx, &SSEEvent{
		Event: "content_block_delta",
		Data:  map[string]interface{}{"type": "content_block_delta", "index": idx, "delta": map[string]interface{}{"type": "text_delta", "text": text}},
	})
	if delta != nil {
		events = append(events, delta)
	}
	return events
}

func (ctx *StreamContext) createThinkingDelta(index int, thinking string) *SSEEvent {
	return &SSEEvent{
		Event: "content_block_delta",
		Data:  map[string]interface{}{"type": "content_block_delta", "index": index, "delta": map[string]interface{}{"type": "thinking_delta", "thinking": thinking}},
	}
}

func (ctx *StreamContext) processToolUse(event *kiro.Event) []*SSEEvent {
	var events []*SSEEvent

	ctx.stateMgr.hasToolUse = true

	// thinking 模式下，tool_use 前需要关闭 thinking 块
	if ctx.ThinkingEnabled && ctx.inThinkingBlock {
		endPos := findRealThinkingEndTagAtBufferEnd(ctx.thinkingBuffer)
		if endPos >= 0 {
			if endPos > 0 && ctx.thinkingBlockIndex != nil {
				events = append(events, ctx.createThinkingDelta(*ctx.thinkingBlockIndex, ctx.thinkingBuffer[:endPos]))
			}
			ctx.inThinkingBlock = false
			ctx.thinkingExtracted = true
			if ctx.thinkingBlockIndex != nil {
				events = append(events, ctx.createThinkingDelta(*ctx.thinkingBlockIndex, ""))
				if stop := ctx.stateMgr.handleContentBlockStop(*ctx.thinkingBlockIndex); stop != nil {
					events = append(events, stop)
				}
			}
			ctx.thinkingBuffer = ctx.thinkingBuffer[endPos+len("</thinking>"):]
			remaining := strings.TrimLeft(ctx.thinkingBuffer, " \t\n\r")
			ctx.thinkingBuffer = ""
			if remaining != "" {
				events = append(events, ctx.createTextDeltaEvents(remaining)...)
			}
		}
	}

	// flush thinking buffer 中的待输出文本
	if ctx.ThinkingEnabled && !ctx.inThinkingBlock && !ctx.thinkingExtracted && ctx.thinkingBuffer != "" {
		buffered := ctx.thinkingBuffer
		ctx.thinkingBuffer = ""
		events = append(events, ctx.createTextDeltaEvents(buffered)...)
	}

	// 获取或分配块索引
	blockIdx, ok := ctx.toolBlockIndices[event.ToolUseID]
	if !ok {
		blockIdx = ctx.stateMgr.nextBlockIndex()
		ctx.toolBlockIndices[event.ToolUseID] = blockIdx
	}

	// content_block_start
	startEvents := ctx.stateMgr.handleContentBlockStart(blockIdx, "tool_use", &SSEEvent{
		Event: "content_block_start",
		Data: map[string]interface{}{
			"type": "content_block_start", "index": blockIdx,
			"content_block": map[string]interface{}{"type": "tool_use", "id": event.ToolUseID, "name": event.ToolName, "input": map[string]interface{}{}},
		},
	})
	events = append(events, startEvents...)

	// input_json_delta
	if event.ToolInput != "" {
		ctx.OutputTokens += CountTokens(event.ToolInput)
		delta := ctx.stateMgr.handleContentBlockDelta(blockIdx, &SSEEvent{
			Event: "content_block_delta",
			Data: map[string]interface{}{
				"type": "content_block_delta", "index": blockIdx,
				"delta": map[string]interface{}{"type": "input_json_delta", "partial_json": event.ToolInput},
			},
		})
		if delta != nil {
			events = append(events, delta)
		}
	}

	// stop
	if event.ToolStop {
		if stop := ctx.stateMgr.handleContentBlockStop(blockIdx); stop != nil {
			events = append(events, stop)
		}
	}

	return events
}

// HasToolUse 返回是否检测到 tool_use
func (ctx *StreamContext) HasToolUse() bool {
	return ctx.stateMgr.hasToolUse
}

// GenerateFinalEvents 生成最终事件
func (ctx *StreamContext) GenerateFinalEvents() []*SSEEvent {
	var events []*SSEEvent

	// Flush thinking buffer
	if ctx.ThinkingEnabled && ctx.thinkingBuffer != "" {
		if ctx.inThinkingBlock {
			endPos := findRealThinkingEndTagAtBufferEnd(ctx.thinkingBuffer)
			if endPos >= 0 {
				if endPos > 0 && ctx.thinkingBlockIndex != nil {
					events = append(events, ctx.createThinkingDelta(*ctx.thinkingBlockIndex, ctx.thinkingBuffer[:endPos]))
				}
				if ctx.thinkingBlockIndex != nil {
					events = append(events, ctx.createThinkingDelta(*ctx.thinkingBlockIndex, ""))
					if stop := ctx.stateMgr.handleContentBlockStop(*ctx.thinkingBlockIndex); stop != nil {
						events = append(events, stop)
					}
				}
				ctx.thinkingBuffer = ctx.thinkingBuffer[endPos+len("</thinking>"):]
				remaining := strings.TrimLeft(ctx.thinkingBuffer, " \t\n\r")
				ctx.inThinkingBlock = false
				ctx.thinkingExtracted = true
				ctx.thinkingBuffer = ""
				if remaining != "" {
					events = append(events, ctx.createTextDeltaEvents(remaining)...)
				}
			} else {
				bufContent := ctx.thinkingBuffer
				ctx.thinkingBuffer = ""
				if ctx.thinkingBlockIndex != nil {
					events = append(events, ctx.createThinkingDelta(*ctx.thinkingBlockIndex, bufContent))
					events = append(events, ctx.createThinkingDelta(*ctx.thinkingBlockIndex, ""))
					if stop := ctx.stateMgr.handleContentBlockStop(*ctx.thinkingBlockIndex); stop != nil {
						events = append(events, stop)
					}
				}
			}
		} else {
			bufContent := ctx.thinkingBuffer
			ctx.thinkingBuffer = ""
			events = append(events, ctx.createTextDeltaEvents(bufContent)...)
		}
	}

	// 只有 thinking 块没有其他内容时
	if ctx.ThinkingEnabled && ctx.thinkingBlockIndex != nil && !ctx.stateMgr.hasNonThinkingBlocks() {
		ctx.stateMgr.stopReason = "max_tokens"
		events = append(events, ctx.createTextDeltaEvents(" ")...)
	}

	finalInputTokens := ctx.InputTokens
	if ctx.ContextInputToks != nil {
		finalInputTokens = *ctx.ContextInputToks
	}

	events = append(events, ctx.stateMgr.generateFinalEvents(finalInputTokens, ctx.OutputTokens)...)
	return events
}
