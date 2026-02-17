package kiro

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"kiro-go/internal/model"
)

// UserCredentialsManager 用户凭证管理器
type UserCredentialsManager struct {
	filePath string
	data     map[string]*model.UserCredentialEntry
	mu       sync.RWMutex
}

func NewUserCredentialsManager(filePath string) *UserCredentialsManager {
	mgr := &UserCredentialsManager{
		filePath: filePath,
		data:     make(map[string]*model.UserCredentialEntry),
	}
	mgr.loadFromFile()
	return mgr
}

func (m *UserCredentialsManager) loadFromFile() {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return
	}
	var entries map[string]*model.UserCredentialEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("解析用户凭证文件失败: %v", err)
		return
	}
	m.data = entries
}

func (m *UserCredentialsManager) saveToFile() error {
	data, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}

func (m *UserCredentialsManager) GetCredentials(activationCode string) *model.KiroCredentials {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if entry, ok := m.data[activationCode]; ok {
		cred := entry.Credentials
		return &cred
	}
	return nil
}

func (m *UserCredentialsManager) AddOrUpdate(activationCode, userName string, creds model.KiroCredentials) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	if entry, ok := m.data[activationCode]; ok {
		entry.Credentials = creds
		if userName != "" {
			entry.UserName = userName
		}
		entry.UpdatedAt = now
	} else {
		m.data[activationCode] = &model.UserCredentialEntry{
			ActivationCode: activationCode,
			UserName:       userName,
			Credentials:    creds,
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
		log.Printf("用户凭证已标记为不可用: %s", activationCode)
	}
}

// GetNextAvailable 获取下一个可用的用户凭证（跳过已禁用的）
// 用于余额不足时自动换号
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
