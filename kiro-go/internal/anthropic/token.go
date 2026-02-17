package anthropic

import (
	"encoding/json"
	"unicode"
)

// isNonWesternChar 判断字符是否为非西文字符
func isNonWesternChar(c rune) bool {
	// 西文字符范围
	if c <= 0x007F { // ASCII
		return false
	}
	if c >= 0x0080 && c <= 0x00FF { // Latin Extended-A
		return false
	}
	if c >= 0x0100 && c <= 0x024F { // Latin Extended-B
		return false
	}
	if c >= 0x1E00 && c <= 0x1EFF { // Latin Extended Additional
		return false
	}
	if c >= 0x2C60 && c <= 0x2C7F { // Latin Extended-C
		return false
	}
	if c >= 0xA720 && c <= 0xA7FF { // Latin Extended-D
		return false
	}
	if c >= 0xAB30 && c <= 0xAB6F { // Latin Extended-E
		return false
	}
	return true
}

// CountTokens 计算文本的 token 数量
// 非西文字符每个计 4.0 字符单位，西文每个计 1.0，4 字符单位 = 1 token
// 短文本有修正系数
func CountTokens(text string) int {
	var charUnits float64
	for _, c := range text {
		if isNonWesternChar(c) {
			charUnits += 4.0
		} else {
			charUnits += 1.0
		}
	}

	tokens := charUnits / 4.0

	// 短文本修正系数
	var accTokens float64
	switch {
	case tokens < 100:
		accTokens = tokens * 1.5
	case tokens < 200:
		accTokens = tokens * 1.3
	case tokens < 300:
		accTokens = tokens * 1.25
	case tokens < 800:
		accTokens = tokens * 1.2
	default:
		accTokens = tokens
	}

	result := int(accTokens)
	if result < 1 {
		return 1
	}
	return result
}

// CountInputTokens 估算请求的输入 tokens
func CountInputTokens(req *MessagesRequest) int {
	total := 0

	// 系统消息
	if req.System != nil {
		var sysStr string
		if json.Unmarshal(req.System, &sysStr) == nil {
			total += CountTokens(sysStr)
		} else {
			var sysArr []map[string]interface{}
			if json.Unmarshal(req.System, &sysArr) == nil {
				for _, item := range sysArr {
					if text, ok := item["text"].(string); ok {
						total += CountTokens(text)
					}
				}
			}
		}
	}

	// 消息
	for _, msg := range req.Messages {
		var text string
		if json.Unmarshal(msg.Content, &text) == nil {
			total += CountTokens(text)
			continue
		}
		var arr []map[string]interface{}
		if json.Unmarshal(msg.Content, &arr) == nil {
			for _, item := range arr {
				if t, ok := item["text"].(string); ok {
					total += CountTokens(t)
				}
			}
		}
	}

	// 工具定义
	for _, raw := range req.Tools {
		var tool map[string]interface{}
		if json.Unmarshal(raw, &tool) == nil {
			if name, ok := tool["name"].(string); ok {
				total += CountTokens(name)
			}
			if desc, ok := tool["description"].(string); ok {
				total += CountTokens(desc)
			}
			if schema, ok := tool["input_schema"]; ok {
				schemaJSON, _ := json.Marshal(schema)
				total += CountTokens(string(schemaJSON))
			}
		}
	}

	if total < 1 {
		return 1
	}
	return total
}

// CountOutputTokens 估算输出 tokens（从内容块列表）
func CountOutputTokens(content []map[string]interface{}) int {
	total := 0
	for _, block := range content {
		if text, ok := block["text"].(string); ok {
			total += CountTokens(text)
		}
		if blockType, _ := block["type"].(string); blockType == "tool_use" {
			if input, ok := block["input"]; ok {
				inputJSON, _ := json.Marshal(input)
				total += CountTokens(string(inputJSON))
			}
		}
	}
	if total < 1 {
		return 1
	}
	return total
}

// EstimateTokens 简单估算文本 token 数（用于流式输出计数）
func EstimateTokens(text string) int {
	count := 0
	for _, c := range text {
		if unicode.Is(unicode.Han, c) || unicode.Is(unicode.Hiragana, c) || unicode.Is(unicode.Katakana, c) || unicode.Is(unicode.Hangul, c) {
			count += 2
		} else {
			count++
		}
	}
	result := (count + 3) / 4
	if result < 1 {
		return 1
	}
	return result
}
