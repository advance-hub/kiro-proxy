package common

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// ── Truncation Recovery 系统 ──
// 参考 kiro-gateway truncation_recovery.py + parsers.py
//
// Kiro API 会在流式传输中截断大型 tool call payload 和内容。
// 由于这是上游限制无法预防，我们：
// 1. 检测截断（JSON 完整性分析）
// 2. 保存截断状态（内存缓存）
// 3. 下次请求时注入合成消息告知模型

// ── 配置 ──

// ShouldInjectRecovery 检查是否启用截断恢复
// 可通过环境变量 TRUNCATION_RECOVERY=false 关闭
func ShouldInjectRecovery() bool {
	envValue := os.Getenv("TRUNCATION_RECOVERY")
	if envValue == "" {
		return true
	}
	enabled, err := strconv.ParseBool(envValue)
	if err != nil {
		return true
	}
	return enabled
}

// ── JSON 截断诊断 ──
// 参考 kiro-gateway parsers.py _diagnose_json_truncation

// TruncationDiagnosis JSON 截断诊断结果
type TruncationDiagnosis struct {
	IsTruncated bool
	Reason      string
	SizeBytes   int
}

// DiagnoseJSONTruncation 分析 JSON 字符串是否被截断
// 区分上游截断（Kiro API 切断大型 tool call arguments）和实际格式错误
func DiagnoseJSONTruncation(jsonStr string) *TruncationDiagnosis {
	sizeBytes := len(jsonStr)
	stripped := strings.TrimSpace(jsonStr)

	if stripped == "" {
		return &TruncationDiagnosis{IsTruncated: false, Reason: "empty string", SizeBytes: sizeBytes}
	}

	openBraces := strings.Count(stripped, "{")
	closeBraces := strings.Count(stripped, "}")
	openBrackets := strings.Count(stripped, "[")
	closeBrackets := strings.Count(stripped, "]")

	// 以 { 开头但不以 } 结尾
	if strings.HasPrefix(stripped, "{") && !strings.HasSuffix(stripped, "}") {
		missing := openBraces - closeBraces
		return &TruncationDiagnosis{
			IsTruncated: true,
			Reason:      fmt.Sprintf("missing %d closing brace(s)", missing),
			SizeBytes:   sizeBytes,
		}
	}

	// 以 [ 开头但不以 ] 结尾
	if strings.HasPrefix(stripped, "[") && !strings.HasSuffix(stripped, "]") {
		missing := openBrackets - closeBrackets
		return &TruncationDiagnosis{
			IsTruncated: true,
			Reason:      fmt.Sprintf("missing %d closing bracket(s)", missing),
			SizeBytes:   sizeBytes,
		}
	}

	// 大括号不平衡
	if openBraces != closeBraces {
		return &TruncationDiagnosis{
			IsTruncated: true,
			Reason:      fmt.Sprintf("unbalanced braces (%d open, %d close)", openBraces, closeBraces),
			SizeBytes:   sizeBytes,
		}
	}

	// 方括号不平衡
	if openBrackets != closeBrackets {
		return &TruncationDiagnosis{
			IsTruncated: true,
			Reason:      fmt.Sprintf("unbalanced brackets (%d open, %d close)", openBrackets, closeBrackets),
			SizeBytes:   sizeBytes,
		}
	}

	// 未闭合的字符串（计算未转义的引号）
	quoteCount := 0
	for i := 0; i < len(stripped); i++ {
		if stripped[i] == '\\' && i+1 < len(stripped) {
			i++ // 跳过转义字符
			continue
		}
		if stripped[i] == '"' {
			quoteCount++
		}
	}
	if quoteCount%2 != 0 {
		return &TruncationDiagnosis{
			IsTruncated: true,
			Reason:      "unclosed string literal",
			SizeBytes:   sizeBytes,
		}
	}

	// 不像被截断，可能只是格式错误
	return &TruncationDiagnosis{IsTruncated: false, Reason: "malformed JSON", SizeBytes: sizeBytes}
}

// ── 合成消息生成 ──
// 参考 kiro-gateway truncation_recovery.py
// 措辞精心设计：
// - 承认 API 限制（不是模型的错）
// - 警告不要重复相同操作
// - 不给出具体指令（避免 micro-steps）

const truncationToolResultContent = "[API Limitation] Your tool call was truncated by the upstream API due to output size limits.\n\n" +
	"If the tool result below shows an error or unexpected behavior, this is likely a CONSEQUENCE of the truncation, " +
	"not the root cause. The tool call itself was cut off before it could be fully transmitted.\n\n" +
	"Repeating the exact same operation will be truncated again. Consider adapting your approach."

const truncationUserMessageContent = "[System Notice] Your previous response was truncated by the API due to " +
	"output size limitations. This is not an error on your part. " +
	"If you need to continue, please adapt your approach rather than repeating the same output."

// GenerateTruncationToolResult 为截断的 tool call 生成 OpenAI 格式合成 tool message
func GenerateTruncationToolResult(toolName, toolCallID string, truncationInfo map[string]interface{}) map[string]interface{} {
	sizeBytes, reason := extractDiagInfo(truncationInfo)
	log.Printf("[Truncation] Generated synthetic tool_result for '%s' (id=%s, size=%s, reason=%s)",
		toolName, toolCallID, sizeBytes, reason)

	return map[string]interface{}{
		"role":         "tool",
		"tool_call_id": toolCallID,
		"content":      truncationToolResultContent,
	}
}

// GenerateTruncationUserMessage 为内容截断生成合成用户消息文本
func GenerateTruncationUserMessage() string {
	return truncationUserMessageContent
}

func extractDiagInfo(info map[string]interface{}) (string, string) {
	sizeBytes := "unknown"
	reason := "unknown"
	if sb, ok := info["size_bytes"]; ok {
		sizeBytes = fmt.Sprintf("%v", sb)
	}
	if r, ok := info["reason"]; ok {
		reason = fmt.Sprintf("%v", r)
	}
	return sizeBytes, reason
}

// ── ToolCallCollector 流式 tool_calls 收集器 ──
// 在流式响应中收集完整的 tool_calls 数据，用于截断检测

// ToolCallCollector 收集流式 tool_calls 的完整数据
type ToolCallCollector struct {
	tools map[string]*CollectedToolCall // toolUseID → data
}

// CollectedToolCall 收集到的单个 tool call
type CollectedToolCall struct {
	ID        string
	Name      string
	Arguments strings.Builder // 累积的 arguments JSON 片段
}

// NewToolCallCollector 创建新的收集器
func NewToolCallCollector() *ToolCallCollector {
	return &ToolCallCollector{tools: make(map[string]*CollectedToolCall)}
}

// AddToolName 记录 tool call 的名称
func (c *ToolCallCollector) AddToolName(toolUseID, name string) {
	tc, exists := c.tools[toolUseID]
	if !exists {
		tc = &CollectedToolCall{ID: toolUseID}
		c.tools[toolUseID] = tc
	}
	tc.Name = name
}

// AppendArguments 追加 tool call 的 arguments 片段
func (c *ToolCallCollector) AppendArguments(toolUseID, fragment string) {
	tc, exists := c.tools[toolUseID]
	if !exists {
		tc = &CollectedToolCall{ID: toolUseID}
		c.tools[toolUseID] = tc
	}
	tc.Arguments.WriteString(fragment)
}

// DetectTruncations 检测所有收集到的 tool calls 是否被截断
// 对每个 tool call 的 arguments 进行 JSON 完整性检查
// 将截断信息保存到缓存中
// 返回截断的 tool call 数量
func (c *ToolCallCollector) DetectTruncations() int {
	truncatedCount := 0
	for _, tc := range c.tools {
		args := tc.Arguments.String()
		if args == "" {
			continue
		}

		// 尝试解析 JSON，如果失败则诊断截断
		var parsed json.RawMessage
		if json.Unmarshal([]byte(args), &parsed) != nil {
			diag := DiagnoseJSONTruncation(args)
			if diag.IsTruncated {
				truncatedCount++
				SaveToolTruncation(tc.ID, tc.Name, map[string]interface{}{
					"size_bytes": diag.SizeBytes,
					"reason":     diag.Reason,
				})
				log.Printf("[Truncation] Tool call truncated by Kiro API: tool='%s', id=%s, size=%d bytes, reason=%s",
					tc.Name, tc.ID, diag.SizeBytes, diag.Reason)
			}
		}
	}
	return truncatedCount
}

// ── 请求注入 ──
// 参考 kiro-gateway routes_openai.py / routes_anthropic.py

// InjectTruncationRecoveryOpenAI 在 OpenAI 消息中注入截断恢复
// 检查历史消息中的 tool_call_id 和 assistant content，
// 如果发现截断记录，注入合成消息
func InjectTruncationRecoveryOpenAI(messages []map[string]interface{}) ([]map[string]interface{}, bool) {
	if !ShouldInjectRecovery() {
		return messages, false
	}

	var result []map[string]interface{}
	injected := false
	toolNotices := 0
	contentNotices := 0

	for i, msg := range messages {
		role, _ := msg["role"].(string)

		// 对于 tool 消息：检查其 tool_call_id 是否有截断记录
		// 参考 kiro-gateway routes_openai.py: 在 tool result 前面加截断提示
		if role == "tool" {
			toolCallID, _ := msg["tool_call_id"].(string)
			if toolCallID != "" {
				if info := GetToolTruncation(toolCallID); info != nil {
					// 修改 tool result content，在前面加截断提示
					originalContent, _ := msg["content"].(string)
					modifiedContent := truncationToolResultContent + "\n\n---\n\nOriginal tool result:\n" + originalContent
					modifiedMsg := make(map[string]interface{})
					for k, v := range msg {
						modifiedMsg[k] = v
					}
					modifiedMsg["content"] = modifiedContent
					result = append(result, modifiedMsg)
					toolNotices++
					injected = true
					continue
				}
			}
		}

		result = append(result, msg)

		// 对于 assistant 消息：检查内容是否被截断
		if role == "assistant" {
			content, _ := msg["content"].(string)
			if content != "" {
				if info := GetContentTruncation(content); info != nil {
					// 在 assistant 消息后面注入合成 user message
					if i+1 < len(messages) {
						syntheticMsg := map[string]interface{}{
							"role":    "user",
							"content": truncationUserMessageContent,
						}
						result = append(result, syntheticMsg)
						contentNotices++
						injected = true
					}
				}
			}
		}
	}

	if injected {
		log.Printf("[Truncation] Injected recovery: %d tool notice(s), %d content notice(s)", toolNotices, contentNotices)
	}

	return result, injected
}
