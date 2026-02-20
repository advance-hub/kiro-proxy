package common

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"
	"time"
)

// TruncationState 管理截断恢复状态
// 参考 kiro-gateway truncation_state.py
// 线程安全的内存缓存，用于跨请求跟踪截断信息

// ToolTruncationInfo 工具调用截断信息
type ToolTruncationInfo struct {
	ToolCallID     string                 // 稳定的 tool_call_id
	ToolName       string                 // 工具名称
	TruncationInfo map[string]interface{} // 截断诊断信息
	Timestamp      int64                  // Unix 时间戳
}

// ContentTruncationInfo 内容截断信息
type ContentTruncationInfo struct {
	MessageHash    string // 内容哈希（用于跟踪）
	ContentPreview string // 前 200 字符（用于调试）
	Timestamp      int64  // Unix 时间戳
}

var (
	// 内存缓存：条目持久化直到：
	// 1. 通过 Get* 函数检索（一次性检索后删除）
	// 2. 网关重启（内存缓存清空）
	// 无 TTL - 即使用户休息几小时，截断信息仍应可用
	toolTruncationCache    = make(map[string]*ToolTruncationInfo)
	contentTruncationCache = make(map[string]*ContentTruncationInfo)
	cacheLock              sync.RWMutex
)

// SaveToolTruncation 保存工具调用截断信息
// 线程安全操作
func SaveToolTruncation(toolCallID, toolName string, truncationInfo map[string]interface{}) {
	cacheLock.Lock()
	defer cacheLock.Unlock()

	info := &ToolTruncationInfo{
		ToolCallID:     toolCallID,
		ToolName:       toolName,
		TruncationInfo: truncationInfo,
		Timestamp:      time.Now().Unix(),
	}
	toolTruncationCache[toolCallID] = info
	log.Printf("[Truncation] Saved tool truncation for %s (%s)", toolCallID, toolName)
}

// GetToolTruncation 获取并删除工具调用截断信息
// 这是一次性操作 - 检索后信息被删除
// 线程安全操作
func GetToolTruncation(toolCallID string) *ToolTruncationInfo {
	cacheLock.Lock()
	defer cacheLock.Unlock()

	info, exists := toolTruncationCache[toolCallID]
	if exists {
		delete(toolTruncationCache, toolCallID)
		log.Printf("[Truncation] Retrieved tool truncation for %s", toolCallID)
	}
	return info
}

// SaveContentTruncation 保存内容截断信息
// 生成内容哈希作为稳定标识符
// 线程安全操作
// 返回内容哈希（用于跟踪）
func SaveContentTruncation(content string) string {
	// 使用前 500 字符计算哈希（足够唯一，不会太多）
	contentForHash := content
	if len(content) > 500 {
		contentForHash = content[:500]
	}

	hash := sha256.Sum256([]byte(contentForHash))
	messageHash := hex.EncodeToString(hash[:])[:16]

	cacheLock.Lock()
	defer cacheLock.Unlock()

	preview := content
	if len(content) > 200 {
		preview = content[:200]
	}

	info := &ContentTruncationInfo{
		MessageHash:    messageHash,
		ContentPreview: preview,
		Timestamp:      time.Now().Unix(),
	}
	contentTruncationCache[messageHash] = info
	log.Printf("[Truncation] Saved content truncation with hash %s", messageHash)

	return messageHash
}

// GetContentTruncation 获取并删除内容截断信息
// 从内容生成哈希并在缓存中查找
// 这是一次性操作 - 检索后信息被删除
// 线程安全操作
func GetContentTruncation(content string) *ContentTruncationInfo {
	contentForHash := content
	if len(content) > 500 {
		contentForHash = content[:500]
	}

	hash := sha256.Sum256([]byte(contentForHash))
	messageHash := hex.EncodeToString(hash[:])[:16]

	cacheLock.Lock()
	defer cacheLock.Unlock()

	info, exists := contentTruncationCache[messageHash]
	if exists {
		delete(contentTruncationCache, messageHash)
		log.Printf("[Truncation] Retrieved content truncation for hash %s", messageHash)
	}
	return info
}

// GetCacheStats 获取当前缓存统计信息
// 用于监控和调试
func GetCacheStats() map[string]int {
	cacheLock.RLock()
	defer cacheLock.RUnlock()

	return map[string]int{
		"tool_truncations":    len(toolTruncationCache),
		"content_truncations": len(contentTruncationCache),
		"total":               len(toolTruncationCache) + len(contentTruncationCache),
	}
}
