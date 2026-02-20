package common

import "strings"

// Backend 后端类型
type Backend string

const (
	BackendKiro      Backend = "kiro"
	BackendWarp      Backend = "warp"
	BackendCodex     Backend = "codex"
	BackendAnthropic Backend = "anthropic"
)

// warp 原生模型名（不含 Anthropic 标准模型名，那些通过 MapModelToWarp 映射）
var warpNativeModels = map[string]bool{
	"claude-4.1-opus":   true,
	"claude-4-opus":     true,
	"claude-4-5-opus":   true,
	"claude-4-sonnet":   true,
	"claude-4-5-sonnet": true,
	"gpt-5":             true,
	"gpt-4.1":           true,
	"o3":                true,
	"o4-mini":           true,
	"gemini-2.5-pro":    true,
}

// ResolveBackend 根据模型名自动判断应该走哪个后端
// 优先级: codex 模型名 > warp 原生模型名 > 默认后端(kiro/anthropic)
func ResolveBackend(model string, defaultBackend Backend) Backend {
	if model == "" {
		return defaultBackend
	}
	lower := strings.ToLower(strings.TrimSpace(model))

	// 1. codex 模型：包含 "codex" 关键字
	if strings.Contains(lower, "codex") {
		return BackendCodex
	}

	// 2. warp 原生模型名（精确匹配）
	if warpNativeModels[lower] {
		return BackendWarp
	}

	// 3. warp 模糊匹配：非标准 Anthropic 格式的模型名
	//    标准 Anthropic 格式: claude-{tier}-{version}-{date} 如 claude-sonnet-4-5-20250929
	//    warp 格式: claude-{version}-{tier} 如 claude-4-opus
	if isWarpStyleModel(lower) {
		return BackendWarp
	}

	// 4. gemini / gpt / o 系列 → warp（这些不是 kiro 支持的模型）
	if strings.HasPrefix(lower, "gemini") ||
		strings.HasPrefix(lower, "gpt-") ||
		lower == "o3" || lower == "o4-mini" ||
		strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3-") || strings.HasPrefix(lower, "o4-") {
		return BackendWarp
	}

	// 5. 默认后端
	return defaultBackend
}

// isWarpStyleModel 检测是否为 warp 风格的模型名
// warp 模型名格式: claude-{数字}-{tier} 或 claude-{数字}.{数字}-{tier}
// 例如: claude-4-opus, claude-4-5-sonnet, claude-4.1-opus
// 而 Anthropic 标准格式: claude-{tier}-{数字}-{数字}-{date}
// 例如: claude-sonnet-4-5-20250929, claude-opus-4-5-20251101
func isWarpStyleModel(model string) bool {
	if !strings.HasPrefix(model, "claude-") {
		return false
	}
	rest := strings.TrimPrefix(model, "claude-")
	// warp 格式第一段是数字: claude-4-xxx, claude-4.1-xxx
	if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
		return true
	}
	return false
}
