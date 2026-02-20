package kiro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kiro-go/internal/logger"
	"kiro-go/internal/model"
)

// UserCredentialsManager 用户凭证管理器
type UserCredentialsManager struct {
	filePath   string
	config     *model.Config
	data       map[string]*model.UserCredentialEntry
	refreshing map[string]bool // 正在刷新中的激活码
	mu         sync.RWMutex
}

func NewUserCredentialsManager(filePath string) *UserCredentialsManager {
	mgr := &UserCredentialsManager{
		filePath:   filePath,
		data:       make(map[string]*model.UserCredentialEntry),
		refreshing: make(map[string]bool),
	}
	mgr.loadFromFile()
	return mgr
}

// SetConfig sets the config for token refresh
func (m *UserCredentialsManager) SetConfig(cfg *model.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = cfg
}

func (m *UserCredentialsManager) loadFromFile() {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return
	}
	var entries map[string]*model.UserCredentialEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		logger.Errorf(logger.CatCreds, "解析用户凭证文件失败: %v", err)
		return
	}
	m.data = entries
	// 启动时打印所有已注册的激活码（方便运维排查）
	var keys []string
	for k := range entries {
		keys = append(keys, logger.MaskKey(k))
	}
	logger.InfoFields(logger.CatCreds, "已加载用户凭证", logger.F{
		"count": len(entries),
		"keys":  keys,
	})
}

func (m *UserCredentialsManager) saveToFile() error {
	data, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}

// normalizeKey 归一化激活码 key：去除 act- 前缀，转大写
func normalizeKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "act-")
	key = strings.TrimPrefix(key, "ACT-")
	return strings.ToUpper(key)
}

func (m *UserCredentialsManager) GetCredentials(activationCode string) *model.KiroCredentials {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 先尝试原始 key
	if entry, ok := m.data[activationCode]; ok {
		cred := entry.Credentials
		return &cred
	}
	// 再尝试归一化后的 key
	normalized := normalizeKey(activationCode)
	if entry, ok := m.data[normalized]; ok {
		cred := entry.Credentials
		return &cred
	}
	// 最后尝试带 act- 前缀的 key
	if !strings.HasPrefix(activationCode, "act-") {
		if entry, ok := m.data["act-"+normalized]; ok {
			cred := entry.Credentials
			return &cred
		}
	}
	return nil
}

// GetCredentialsAutoRefresh 获取凭证，如果 token 过期则自动刷新
// 这是解决多用户 token 过期卡顿的核心方法
func (m *UserCredentialsManager) GetCredentialsAutoRefresh(activationCode string) (*model.KiroCredentials, error) {
	m.mu.RLock()
	// 先尝试原始 key
	entry, ok := m.data[activationCode]
	if !ok {
		// 再尝试归一化后的 key
		normalized := normalizeKey(activationCode)
		entry, ok = m.data[normalized]
		if !ok && !strings.HasPrefix(activationCode, "act-") {
			// 最后尝试带 act- 前缀的 key
			entry, ok = m.data["act-"+normalized]
		}
		if !ok {
			m.mu.RUnlock()
			return nil, nil
		}
	}
	cred := entry.Credentials
	cfg := m.config
	isRefreshing := m.refreshing[activationCode]
	m.mu.RUnlock()

	if cred.Disabled {
		return &cred, nil
	}

	// token 未过期，直接返回
	if !IsTokenExpired(&cred) {
		return &cred, nil
	}

	// 没有 config 或 refreshToken，无法刷新，直接返回现有 token
	if cfg == nil || cred.RefreshToken == "" {
		logger.Warnf(logger.CatToken, "用户 %s token 已过期但无法刷新 (config=%v, hasRefreshToken=%v)",
			logger.MaskKey(activationCode), cfg != nil, cred.RefreshToken != "")
		return &cred, nil
	}

	// 已有其他 goroutine 在刷新，直接返回现有 token（避免并发刷新）
	if isRefreshing {
		logger.Debugf(logger.CatToken, "用户 %s token 刷新中，使用现有 token", logger.MaskKey(activationCode))
		return &cred, nil
	}

	// 标记刷新中
	m.mu.Lock()
	if m.refreshing[activationCode] {
		m.mu.Unlock()
		return &cred, nil
	}
	m.refreshing[activationCode] = true
	m.mu.Unlock()

	// 异步刷新 token
	go m.refreshUserToken(activationCode, &cred, cfg)

	// 返回现有 token（即使过期，Kiro API 可能仍接受）
	return &cred, nil
}

// refreshUserToken 后台刷新用户 token 并持久化
func (m *UserCredentialsManager) refreshUserToken(activationCode string, cred *model.KiroCredentials, cfg *model.Config) {
	defer func() {
		m.mu.Lock()
		delete(m.refreshing, activationCode)
		m.mu.Unlock()
	}()

	logger.LogCredentialStatus("", logger.MaskKey(activationCode), "refresh_start", logger.F{
		"expires_at":  cred.ExpiresAt,
		"auth_method": cred.AuthMethod,
	})

	newCred, err := RefreshToken(cred, cfg)
	if err != nil {
		logger.LogCredentialStatus("", logger.MaskKey(activationCode), "refresh_failed", logger.F{
			"error":       err.Error(),
			"expires_at":  cred.ExpiresAt,
			"auth_method": cred.AuthMethod,
		})
		return
	}

	// 更新内存和文件
	m.mu.Lock()
	if entry, ok := m.data[activationCode]; ok {
		entry.Credentials.AccessToken = newCred.AccessToken
		if newCred.RefreshToken != "" {
			entry.Credentials.RefreshToken = newCred.RefreshToken
		}
		if newCred.ProfileArn != "" {
			entry.Credentials.ProfileArn = newCred.ProfileArn
		}
		entry.Credentials.ExpiresAt = newCred.ExpiresAt
		entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := m.saveToFile(); err != nil {
			logger.Errorf(logger.CatToken, "保存刷新后的用户凭证失败: %v", err)
		} else {
			logger.LogCredentialStatus("", logger.MaskKey(activationCode), "refresh_success", logger.F{
				"new_expires_at": newCred.ExpiresAt,
				"auth_method":    cred.AuthMethod,
			})
		}
	}
	m.mu.Unlock()
}

// StartAutoRefresh 启动后台定时刷新，检查所有用户凭证的 token 有效期
func (m *UserCredentialsManager) StartAutoRefresh(cfg *model.Config, interval time.Duration) {
	m.SetConfig(cfg)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			m.refreshExpiring(cfg)
		}
	}()
}

// refreshExpiring 刷新即将过期的用户 token
func (m *UserCredentialsManager) refreshExpiring(cfg *model.Config) {
	m.mu.RLock()
	var toRefresh []struct {
		code string
		cred model.KiroCredentials
	}
	for code, entry := range m.data {
		if entry.Credentials.Disabled || entry.Credentials.RefreshToken == "" {
			continue
		}
		if IsTokenExpiringSoon(&entry.Credentials) {
			toRefresh = append(toRefresh, struct {
				code string
				cred model.KiroCredentials
			}{code, entry.Credentials})
		}
	}
	m.mu.RUnlock()

	if len(toRefresh) > 0 {
		logger.Infof(logger.CatToken, "定时检查: %d 个用户 token 即将过期，开始刷新", len(toRefresh))
	}

	for _, item := range toRefresh {
		cred := item.cred
		m.mu.Lock()
		if m.refreshing[item.code] {
			m.mu.Unlock()
			continue
		}
		m.refreshing[item.code] = true
		m.mu.Unlock()

		go m.refreshUserToken(item.code, &cred, cfg)
	}
}

// GetCredentialsWithExpiry 获取凭证并检查有效期
func (m *UserCredentialsManager) GetCredentialsWithExpiry(activationCode string) (*model.KiroCredentials, bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 先尝试原始 key
	entry, ok := m.data[activationCode]
	if !ok {
		// 再尝试归一化后的 key
		normalized := normalizeKey(activationCode)
		entry, ok = m.data[normalized]
		if !ok && !strings.HasPrefix(activationCode, "act-") {
			// 最后尝试带 act- 前缀的 key
			entry, ok = m.data["act-"+normalized]
		}
		if !ok {
			return nil, false, ""
		}
	}
	cred := entry.Credentials
	if entry.ExpiresDate != "" {
		expiresAt, err := time.Parse("2006-01-02", entry.ExpiresDate)
		if err == nil {
			expiresAt = expiresAt.Add(24*time.Hour - time.Second)
			if time.Now().After(expiresAt) {
				return &cred, true, entry.ExpiresDate
			}
		}
	}
	return &cred, false, entry.ExpiresDate
}

// SetExpiresDate 设置凭证有效期
func (m *UserCredentialsManager) SetExpiresDate(activationCode string, days int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.data[activationCode]
	if !ok {
		return nil
	}
	if days == 0 {
		// 设置为 0 天表示立即过期（禁止使用）
		expiresDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		entry.ExpiresDate = expiresDate
	} else if days > 0 {
		expiresDate := time.Now().AddDate(0, 0, days).Format("2006-01-02")
		entry.ExpiresDate = expiresDate
	} else {
		// days < 0 则清空过期日期
		entry.ExpiresDate = ""
	}
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return m.saveToFile()
}

// BatchSetExpiresDate 批量设置凭证有效期
func (m *UserCredentialsManager) BatchSetExpiresDate(activationCodes []string, days int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	updated := 0
	expiresDate := ""
	if days == 0 {
		// 设置为 0 天表示立即过期（禁止使用）
		expiresDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	} else if days > 0 {
		expiresDate = time.Now().AddDate(0, 0, days).Format("2006-01-02")
	}
	// days < 0 则清空过期日期（expiresDate 保持为空字符串）

	now := time.Now().UTC().Format(time.RFC3339)
	for _, code := range activationCodes {
		if entry, ok := m.data[code]; ok {
			entry.ExpiresDate = expiresDate
			entry.UpdatedAt = now
			updated++
		}
	}

	if updated > 0 {
		return updated, m.saveToFile()
	}
	return 0, nil
}

// GetEntry 获取完整条目
func (m *UserCredentialsManager) GetEntry(activationCode string) *model.UserCredentialEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 先尝试原始 key
	if entry, ok := m.data[activationCode]; ok {
		return entry
	}
	// 再尝试归一化后的 key
	normalized := normalizeKey(activationCode)
	if entry, ok := m.data[normalized]; ok {
		return entry
	}
	// 最后尝试带 act- 前缀的 key
	if !strings.HasPrefix(activationCode, "act-") {
		if entry, ok := m.data["act-"+normalized]; ok {
			return entry
		}
	}
	return nil
}

func (m *UserCredentialsManager) AddOrUpdate(activationCode, userName string, creds model.KiroCredentials) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 归一化 key：统一用大写无前缀格式存储
	normalizedKey := normalizeKey(activationCode)
	now := time.Now().UTC().Format(time.RFC3339)
	if entry, ok := m.data[normalizedKey]; ok {
		entry.Credentials = creds
		if userName != "" {
			entry.UserName = userName
		}
		entry.UpdatedAt = now
		// 如果现有条目没有过期日期，设置默认 0 天（立即禁止使用）
		if entry.ExpiresDate == "" {
			entry.ExpiresDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		}
	} else {
		// 新增条目时自动设置默认有效期 0 天（立即禁止使用）
		defaultExpiresDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		m.data[normalizedKey] = &model.UserCredentialEntry{
			ActivationCode: normalizedKey,
			UserName:       userName,
			Credentials:    creds,
			ExpiresDate:    defaultExpiresDate,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}
	return m.saveToFile()
}

func (m *UserCredentialsManager) Remove(activationCode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, activationCode)
	return m.saveToFile()
}

func (m *UserCredentialsManager) ListAll() []*model.UserCredentialEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.UserCredentialEntry
	for _, entry := range m.data {
		list = append(list, entry)
	}
	return list
}

func (m *UserCredentialsManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// MarkDisabled 标记凭证为不可用（额度用尽等）
func (m *UserCredentialsManager) MarkDisabled(activationCode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.data[activationCode]; ok {
		entry.Credentials.Disabled = true
		logger.Warnf(logger.CatCreds, "用户凭证已标记为不可用: %s", logger.MaskKey(activationCode))
	}
}

// GetNextAvailable 获取下一个可用的用户凭证（跳过已禁用的）
func (m *UserCredentialsManager) GetNextAvailable(excludeCode string) (string, *model.KiroCredentials) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for code, entry := range m.data {
		if code == excludeCode {
			continue
		}
		if entry.Credentials.Disabled {
			continue
		}
		cred := entry.Credentials
		return code, &cred
	}
	return "", nil
}
