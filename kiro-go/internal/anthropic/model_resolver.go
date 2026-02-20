package anthropic

import (
	"regexp"
	"strings"
)

// 预编译正则（避免每次调用重新编译）
var (
	reStandard = regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)-(\d{1,2})(?:-(?:\d{8}|latest|\d+))?$`)
	reNoMinor  = regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)(?:-\d{8})?$`)
	reLegacy   = regexp.MustCompile(`^(claude)-(\d+)-(\d+)-(haiku|sonnet|opus)(?:-(?:\d{8}|latest|\d+))?$`)
	reDotDate  = regexp.MustCompile(`^(claude-(?:\d+\.\d+-)?(?:haiku|sonnet|opus)(?:-\d+\.\d+)?)-\d{8}$`)
	reInverted = regexp.MustCompile(`^claude-(\d+)\.(\d+)-(haiku|sonnet|opus)-.+$`)
)

// NormalizeModelName 将各种客户端模型名格式统一为 Kiro 格式
//
// 转换示例：
//
//	claude-haiku-4-5                   → claude-haiku-4.5
//	claude-haiku-4-5-20251001          → claude-haiku-4.5
//	claude-haiku-4-5-20251001-thinking → claude-haiku-4.5
//	claude-sonnet-4-20250514           → claude-sonnet-4
//	claude-3-7-sonnet                  → claude-3.7-sonnet
//	claude-4.5-opus-high               → claude-opus-4.5
func NormalizeModelName(name string) string {
	if name == "" {
		return name
	}
	lower := strings.ToLower(name)

	// 剥离 -thinking 后缀（thinking 由 handler 层单独处理）
	lower = strings.TrimSuffix(lower, "-thinking")

	// claude-haiku-4-5 / claude-haiku-4-5-20251001 / claude-haiku-4-5-latest
	// 注意：必须在 reNoMinor 之前检查，因为 4-5 格式优先级更高
	if m := reStandard.FindStringSubmatch(lower); m != nil {
		return m[1] + "." + m[2]
	}
	// claude-sonnet-4 / claude-sonnet-4-20250514（只匹配没有 minor 版本的）
	// 但 claude-opus-4-6 应该被 reStandard 匹配，不应该到这里
	if m := reNoMinor.FindStringSubmatch(lower); m != nil {
		return m[1]
	}
	// claude-3-7-sonnet / claude-3-7-sonnet-20250219
	if m := reLegacy.FindStringSubmatch(lower); m != nil {
		return m[1] + "-" + m[2] + "." + m[3] + "-" + m[4]
	}
	// claude-haiku-4.5-20251001
	if m := reDotDate.FindStringSubmatch(lower); m != nil {
		return m[1]
	}
	// claude-4.5-opus-high → claude-opus-4.5
	if m := reInverted.FindStringSubmatch(lower); m != nil {
		return "claude-" + m[3] + "-" + m[1] + "." + m[2]
	}

	return lower
}

// ResolveModel 解析模型名并映射到 Kiro 内部 ID
// 先标准化名称，再查找映射
func ResolveModel(model string) (string, bool) {
	normalized := NormalizeModelName(model)

	// 已知模型映射
	knownModels := map[string]string{
		"claude-sonnet-4.5": "claude-sonnet-4.5",
		"claude-sonnet-4":   "claude-sonnet-4.5",
		"claude-opus-4.5":   "claude-opus-4.5",
		"claude-opus-4.6":   "claude-opus-4.6",
		"claude-haiku-4.5":  "claude-haiku-4.5",
		"claude-3.7-sonnet": "claude-3.7-sonnet",
		"claude-3.5-sonnet": "claude-sonnet-4.5",
		"claude-3.5-haiku":  "claude-haiku-4.5",
	}

	if id, ok := knownModels[normalized]; ok {
		return id, true
	}

	// 模糊匹配：包含关键字
	lower := strings.ToLower(normalized)
	if strings.Contains(lower, "sonnet") {
		return "claude-sonnet-4.5", true
	}
	if strings.Contains(lower, "opus") {
		if strings.Contains(lower, "4-5") || strings.Contains(lower, "4.5") {
			return "claude-opus-4.5", true
		}
		return "claude-opus-4.6", true
	}
	if strings.Contains(lower, "haiku") {
		return "claude-haiku-4.5", true
	}

	// 非 Claude 模型直通（让 Kiro 决定）
	if strings.HasPrefix(lower, "deepseek") || strings.HasPrefix(lower, "minimax") || strings.HasPrefix(lower, "qwen") {
		return normalized, true
	}

	return "", false
}
