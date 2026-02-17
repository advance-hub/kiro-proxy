package kiro

import (
	"encoding/json"
	"fmt"
	"log"

	"kiro-go/internal/kiro/parser"
)

// Event Kiro API 返回的事件
type Event struct {
	Type string // "assistant_response", "tool_use", "context_usage", "error", "exception", "metering", "unknown"

	// AssistantResponse
	Content string

	// ToolUse
	ToolName  string
	ToolUseID string
	ToolInput string
	ToolStop  bool

	// ContextUsage
	ContextUsagePercentage float64

	// Error/Exception
	ErrorCode     string
	ErrorMessage  string
	ExceptionType string
}

// ParseEvent 从 AWS Event Stream Frame 解析事件
func ParseEvent(frame *parser.Frame) (*Event, error) {
	msgType := frame.MessageType()

	switch msgType {
	case "event":
		return parseEventFrame(frame)
	case "error":
		return &Event{
			Type:         "error",
			ErrorCode:    frame.Headers["error-code"],
			ErrorMessage: frame.PayloadString(),
		}, nil
	case "exception":
		return &Event{
			Type:          "exception",
			ExceptionType: frame.Headers[":exception-type"],
			ErrorMessage:  frame.PayloadString(),
		}, nil
	default:
		return parseEventFrame(frame) // 默认当 event 处理
	}
}

func parseEventFrame(frame *parser.Frame) (*Event, error) {
	eventType := frame.EventType()

	switch eventType {
	case "assistantResponseEvent":
		var payload struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(frame.Payload, &payload); err != nil {
			return nil, fmt.Errorf("parse assistantResponseEvent: %w", err)
		}
		return &Event{Type: "assistant_response", Content: payload.Content}, nil

	case "toolUseEvent":
		var payload struct {
			Name      string `json:"name"`
			ToolUseID string `json:"toolUseId"`
			Input     string `json:"input"`
			Stop      bool   `json:"stop"`
		}
		if err := json.Unmarshal(frame.Payload, &payload); err != nil {
			return nil, fmt.Errorf("parse toolUseEvent: %w", err)
		}
		return &Event{
			Type: "tool_use", ToolName: payload.Name,
			ToolUseID: payload.ToolUseID, ToolInput: payload.Input, ToolStop: payload.Stop,
		}, nil

	case "contextUsageEvent":
		var payload struct {
			ContextUsagePercentage float64 `json:"contextUsagePercentage"`
		}
		if err := json.Unmarshal(frame.Payload, &payload); err != nil {
			return nil, fmt.Errorf("parse contextUsageEvent: %w", err)
		}
		return &Event{Type: "context_usage", ContextUsagePercentage: payload.ContextUsagePercentage}, nil

	case "meteringEvent":
		return &Event{Type: "metering"}, nil

	default:
		log.Printf("未知事件类型: %s", eventType)
		return &Event{Type: "unknown"}, nil
	}
}
