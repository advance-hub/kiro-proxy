package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"kiro-go/internal/kiro"
	"kiro-go/internal/kiro/parser"
	"kiro-go/internal/logger"
	"kiro-go/internal/model"

	"github.com/google/uuid"
)

// CompressContext 检查是否需要压缩上下文，如需要则执行压缩
// 返回压缩后的消息列表和是否进行了压缩
func CompressContext(
	messages []MessageItem,
	cfg *model.Config,
	provider *kiro.Provider,
	creds *model.KiroCredentials,
	actCode string,
) ([]MessageItem, bool) {
	if !cfg.ContextCompression {
		return messages, false
	}

	threshold := cfg.CompressionThreshold
	keepRecent := cfg.CompressionKeepRecent

	if len(messages) <= threshold {
		return messages, false
	}

	// 分割：旧消息（需要压缩）和新消息（保留原样）
	splitIdx := len(messages) - keepRecent
	if splitIdx <= 0 {
		return messages, false
	}

	oldMessages := messages[:splitIdx]
	recentMessages := messages[splitIdx:]

	logger.InfoFields(logger.CatProxy, "上下文压缩触发", logger.F{
		"total_messages": len(messages),
		"compress_count": len(oldMessages),
		"keep_recent":    len(recentMessages),
		"compress_model": cfg.CompressionModel,
	})

	// 将旧消息序列化为文本
	conversationText := serializeMessagesForCompression(oldMessages)
	if conversationText == "" {
		return messages, false
	}

	// 截断过长文本（避免压缩请求本身超限）
	if len(conversationText) > 100000 {
		conversationText = "...(前文已截断)...\n\n" + conversationText[len(conversationText)-100000:]
	}

	// 用小模型做摘要
	start := time.Now()
	summary, err := callCompressionModel(conversationText, cfg, provider, creds, actCode)
	elapsed := time.Since(start)

	if err != nil {
		logger.ErrorFields(logger.CatProxy, "上下文压缩失败，使用原始消息", logger.F{
			"error":   err.Error(),
			"latency": elapsed.String(),
		})
		return messages, false
	}

	logger.InfoFields(logger.CatProxy, "上下文压缩完成", logger.F{
		"original_chars": len(conversationText),
		"summary_chars":  len(summary),
		"ratio":          fmt.Sprintf("%.1f%%", float64(len(summary))/float64(len(conversationText))*100),
		"latency":        elapsed.String(),
	})

	// 构造摘要消息，替换旧消息
	summaryContent := fmt.Sprintf("[以下是之前对话的摘要]\n\n%s\n\n[摘要结束，以下是最近的对话]", summary)
	summaryRaw, _ := json.Marshal(summaryContent)

	result := make([]MessageItem, 0, 2+len(recentMessages))
	result = append(result, MessageItem{
		Role:    "user",
		Content: summaryRaw,
	})

	// 确保角色交替：如果 recentMessages 第一条也是 user，需要插入一个 assistant 过渡
	if len(recentMessages) > 0 && recentMessages[0].Role == "user" {
		ackRaw, _ := json.Marshal("好的，我已了解之前的对话上下文。请继续。")
		result = append(result, MessageItem{
			Role:    "assistant",
			Content: ackRaw,
		})
	}

	result = append(result, recentMessages...)
	return result, true
}

// serializeMessagesForCompression 将消息列表序列化为可读文本
func serializeMessagesForCompression(messages []MessageItem) string {
	var sb strings.Builder
	for i, msg := range messages {
		text := extractTextForCompression(msg.Content)
		if text == "" {
			continue
		}
		role := msg.Role
		if role == "user" {
			role = "User"
		} else if role == "assistant" {
			role = "Assistant"
		}
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s", role, text))
	}
	return sb.String()
}

// extractTextForCompression 从 JSON content 中提取纯文本（用于序列化）
func extractTextForCompression(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	// 尝试解析为字符串
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}

	// 尝试解析为 content blocks 数组
	var blocks []map[string]interface{}
	if json.Unmarshal(content, &blocks) == nil {
		var parts []string
		for _, block := range blocks {
			blockType, _ := block["type"].(string)
			switch blockType {
			case "text":
				if text, ok := block["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			case "tool_use":
				name, _ := block["name"].(string)
				input, _ := block["input"].(map[string]interface{})
				inputJSON, _ := json.Marshal(input)
				parts = append(parts, fmt.Sprintf("[Tool call: %s(%s)]", name, string(inputJSON)))
			case "tool_result":
				toolID, _ := block["tool_use_id"].(string)
				var resultText string
				switch c := block["content"].(type) {
				case string:
					resultText = c
				case []interface{}:
					for _, ci := range c {
						if cm, ok := ci.(map[string]interface{}); ok {
							if t, ok := cm["text"].(string); ok {
								resultText += t
							}
						}
					}
				}
				parts = append(parts, fmt.Sprintf("[Tool result %s: %s]", toolID, resultText))
			}
		}
		return strings.Join(parts, "\n")
	}

	return ""
}

// callCompressionModel 调用压缩模型做摘要
func callCompressionModel(
	conversationText string,
	cfg *model.Config,
	provider *kiro.Provider,
	creds *model.KiroCredentials,
	actCode string,
) (string, error) {
	prompt := fmt.Sprintf(`请简洁地总结以下对话内容，保留关键信息（包括：代码片段、文件路径、技术决策、具体的数值/配置、错误信息等）。
总结应该让后续的对话模型能够理解之前讨论的完整上下文。
不需要逐条总结，用自然语言段落形式描述即可。

<conversation>
%s
</conversation>

请直接输出总结内容，不需要任何前缀或解释。`, conversationText)

	// 构建 Kiro 请求（直接构造，不走 ConvertToKiroRequest 避免模型映射问题）
	modelID := cfg.CompressionModel
	conversationID := uuid.New().String()

	userInput := map[string]interface{}{
		"content": prompt,
		"modelId": modelID,
		"origin":  "AI_EDITOR",
	}
	currentMessage := map[string]interface{}{"userInputMessage": userInput}

	convState := map[string]interface{}{
		"agentContinuationId": uuid.New().String(),
		"agentTaskType":       "vibe",
		"chatTriggerType":     "MANUAL",
		"conversationId":      conversationID,
		"currentMessage":      currentMessage,
	}

	kiroReq := map[string]interface{}{
		"conversationState": convState,
	}

	body, err := json.Marshal(kiroReq)
	if err != nil {
		return "", fmt.Errorf("构建压缩请求失败: %v", err)
	}

	logger.Debugf(logger.CatProxy, "压缩请求体大小: %d bytes, model: %s", len(body), modelID)

	// 发送请求
	var resp *http.Response
	if creds != nil {
		resp, err = provider.CallWithCredentials(body, creds, actCode)
	} else {
		resp, _, err = provider.CallWithTokenManager(body)
	}
	if err != nil {
		return "", fmt.Errorf("压缩请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("压缩请求返回错误: %d", resp.StatusCode)
	}

	// 读取 AWS Event Stream 响应，提取文本
	return readKiroTextResponse(resp), nil
}

// readKiroTextResponse 从 Kiro 的 AWS Event Stream 响应中提取完整文本
func readKiroTextResponse(resp *http.Response) string {
	decoder := parser.NewDecoder()
	var fullText strings.Builder

	buf := make([]byte, 32*1024)
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
				if event.Type == "assistant_response" && event.Content != "" {
					fullText.WriteString(event.Content)
				}
			}
		}
		if err != nil {
			break
		}
	}
	return fullText.String()
}
