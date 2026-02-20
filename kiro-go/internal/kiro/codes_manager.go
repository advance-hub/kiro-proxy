package kiro

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kiro-go/internal/logger"
	"kiro-go/internal/model"
)

// CodeEntry 卡密条目（兼容 app.js 的 codes.json 格式）
type CodeEntry struct {
	Code        string                 `json:"code"`
	Active      bool                   `json:"active"`
	MachineID   *string                `json:"machineId"`
	ActivatedAt *string                `json:"activatedAt"`
	TunnelDays  int                    `json:"tunnelDays"`
	ExpiresDate string                 `json:"expiresDate,omitempty"` // API 使用有效期（YYYY-MM-DD 格式）
	UserName    string                 `json:"userName,omitempty"`    // 用户名
	Credentials *model.KiroCredentials `json:"credentials,omitempty"` // Kiro 用户凭证（AccessToken、RefreshToken 等）
}

// CodesManager 卡密管理器
type CodesManager struct {
	filePath string
	codes    []CodeEntry
	mu       sync.RWMutex
}

func NewCodesManager(filePath string) *CodesManager {
	mgr := &CodesManager{filePath: filePath}
	mgr.loadFromFile()
	return mgr
}

func (m *CodesManager) loadFromFile() {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		logger.Infof(logger.CatAdmin, "卡密文件不存在，将创建空列表: %s", m.filePath)
		m.codes = []CodeEntry{}
		return
	}
	if err := json.Unmarshal(data, &m.codes); err != nil {
		logger.Errorf(logger.CatAdmin, "解析卡密文件失败: %v", err)
		m.codes = []CodeEntry{}
	}
}

func (m *CodesManager) saveToFile() error {
	data, err := json.MarshalIndent(m.codes, "", "  ")
	if err != nil {
		return err
	}
	// 确保父目录存在
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}

// FindByCode 按激活码查找（大写匹配）
func (m *CodesManager) FindByCode(code string) *CodeEntry {
	upper := strings.ToUpper(strings.TrimSpace(code))
	for i := range m.codes {
		if m.codes[i].Code == upper {
			return &m.codes[i]
		}
	}
	return nil
}

// Activate 激活码激活（绑定机器）
func (m *CodesManager) Activate(code, machineId string) (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	trimmed := strings.ToUpper(strings.TrimSpace(code))
	entry := m.FindByCode(trimmed)
	if entry == nil {
		return false, "激活码无效"
	}
	if entry.Active && entry.MachineID != nil && *entry.MachineID != machineId {
		return false, "该激活码已被其他设备使用"
	}
	if entry.Active && entry.MachineID != nil && *entry.MachineID == machineId {
		return true, "已激活"
	}
	entry.Active = true
	entry.MachineID = &machineId
	now := time.Now().UTC().Format(time.RFC3339)
	entry.ActivatedAt = &now
	m.saveToFile()
	return true, "激活成功"
}

// CheckTunnel 检查穿透权限
func (m *CodesManager) CheckTunnel(code, machineId string) (bool, string, int, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trimmed := strings.ToUpper(strings.TrimSpace(code))
	entry := m.FindByCode(trimmed)
	if entry == nil {
		return false, "激活码无效", 0, ""
	}
	if !entry.Active || entry.MachineID == nil || *entry.MachineID != machineId {
		return false, "激活码未激活或设备不匹配", 0, ""
	}
	if entry.TunnelDays <= 0 {
		return false, "您的账户暂无内网穿透权限，请联系管理员开通", 0, ""
	}
	if entry.ActivatedAt == nil {
		return false, "激活时间异常", 0, ""
	}
	activatedAt, err := time.Parse(time.RFC3339, *entry.ActivatedAt)
	if err != nil {
		return false, "激活时间解析失败", 0, ""
	}
	expiresAt := activatedAt.Add(time.Duration(entry.TunnelDays) * 24 * time.Hour)
	if time.Now().After(expiresAt) {
		return false, fmt.Sprintf("内网穿透权限已过期（过期时间：%s）", expiresAt.Format("2006-01-02 15:04:05")), 0, ""
	}
	return true, "验证通过", entry.TunnelDays, expiresAt.Format(time.RFC3339)
}

// IsValidCode 检查激活码是否有效（不检查穿透权限，仅检查激活状态）
func (m *CodesManager) IsValidCode(code, machineId string) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trimmed := strings.ToUpper(strings.TrimSpace(code))
	entry := m.FindByCode(trimmed)
	if entry == nil {
		return false, "激活码无效"
	}
	if !entry.Active {
		return false, "激活码未激活"
	}
	// machineId 为空时跳过设备检查（兼容无 machineId 的场景）
	if machineId != "" && entry.MachineID != nil && *entry.MachineID != machineId {
		return false, "激活码已被其他设备使用"
	}
	return true, ""
}

// GetAll 获取所有卡密
func (m *CodesManager) GetAll() []CodeEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]CodeEntry, len(m.codes))
	copy(result, m.codes)
	return result
}

// Stats 统计信息
func (m *CodesManager) Stats() (total, activated, unused int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total = len(m.codes)
	for _, c := range m.codes {
		if c.Active {
			activated++
		} else {
			unused++
		}
	}
	return
}

// AddCodes 批量添加卡密
func (m *CodesManager) AddCodes(customCodes []string, count, tunnelDays int) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 默认有效期为 0 天（昨天过期，禁止使用）
	defaultExpiresDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	var added []string
	if len(customCodes) > 0 {
		for _, c := range customCodes {
			trimmed := strings.ToUpper(strings.TrimSpace(c))
			if trimmed == "" || m.FindByCode(trimmed) != nil {
				continue
			}
			m.codes = append(m.codes, CodeEntry{
				Code: trimmed, Active: false, TunnelDays: tunnelDays, ExpiresDate: defaultExpiresDate,
			})
			added = append(added, trimmed)
		}
	} else if count > 0 {
		if count > 1000 {
			count = 1000
		}
		for i := 0; i < count; i++ {
			var code string
			for {
				code = generateCode()
				if m.FindByCode(code) == nil {
					break
				}
			}
			m.codes = append(m.codes, CodeEntry{
				Code: code, Active: false, TunnelDays: tunnelDays, ExpiresDate: defaultExpiresDate,
			})
			added = append(added, code)
		}
	}
	m.saveToFile()
	return added
}

// DeleteCodes 批量删除
func (m *CodesManager) DeleteCodes(codesToDelete []string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	deleteSet := make(map[string]bool)
	for _, c := range codesToDelete {
		deleteSet[strings.ToUpper(strings.TrimSpace(c))] = true
	}
	var newCodes []CodeEntry
	deleted := 0
	for _, c := range m.codes {
		if deleteSet[c.Code] {
			deleted++
		} else {
			newCodes = append(newCodes, c)
		}
	}
	m.codes = newCodes
	m.saveToFile()
	return deleted
}

// SetExpiresDate 设置激活码过期日期
func (m *CodesManager) SetExpiresDate(code string, days int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.FindByCode(code)
	if entry == nil {
		return nil
	}

	if days == 0 {
		// 设置为 0 天表示立即过期（禁止使用）
		entry.ExpiresDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	} else if days > 0 {
		entry.ExpiresDate = time.Now().AddDate(0, 0, days).Format("2006-01-02")
	} else {
		// days < 0 则清空过期日期
		entry.ExpiresDate = ""
	}

	return m.saveToFile()
}

// BatchSetExpiresDate 批量设置激活码过期日期
func (m *CodesManager) BatchSetExpiresDate(codes []string, days int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	updated := 0
	expiresDate := ""
	if days == 0 {
		expiresDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	} else if days > 0 {
		expiresDate = time.Now().AddDate(0, 0, days).Format("2006-01-02")
	}

	for _, code := range codes {
		entry := m.FindByCode(code)
		if entry != nil {
			entry.ExpiresDate = expiresDate
			updated++
		}
	}

	if updated > 0 {
		return updated, m.saveToFile()
	}
	return 0, nil
}

// SetCredentials 设置激活码的用户凭证
func (m *CodesManager) SetCredentials(code, userName string, credentials *model.KiroCredentials) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.FindByCode(code)
	if entry == nil {
		return fmt.Errorf("激活码不存在: %s", code)
	}

	entry.Credentials = credentials
	if userName != "" {
		entry.UserName = userName
	}

	return m.saveToFile()
}

// GetCredentials 获取激活码的用户凭证
func (m *CodesManager) GetCredentials(code string) *model.KiroCredentials {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 去掉 act- 前缀（如果有）
	normalizedCode := strings.TrimPrefix(strings.ToUpper(code), "ACT-")
	entry := m.FindByCode(normalizedCode)
	if entry == nil || entry.Credentials == nil {
		return nil
	}

	return entry.Credentials
}

// UpdateTunnelDays 批量更新穿透天数
func (m *CodesManager) UpdateTunnelDays(codesToUpdate []string, tunnelDays int) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	updated := 0
	if len(codesToUpdate) > 0 {
		updateSet := make(map[string]bool)
		for _, c := range codesToUpdate {
			updateSet[strings.ToUpper(strings.TrimSpace(c))] = true
		}
		for i := range m.codes {
			if updateSet[m.codes[i].Code] {
				m.codes[i].TunnelDays = tunnelDays
				updated++
			}
		}
	} else {
		for i := range m.codes {
			m.codes[i].TunnelDays = tunnelDays
			updated++
		}
	}
	m.saveToFile()
	return updated
}

// ResetCodes 重置卡密绑定
func (m *CodesManager) ResetCodes(codesToReset []string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	resetSet := make(map[string]bool)
	for _, c := range codesToReset {
		resetSet[strings.ToUpper(strings.TrimSpace(c))] = true
	}
	resetCount := 0
	for i := range m.codes {
		if resetSet[m.codes[i].Code] {
			m.codes[i].Active = false
			m.codes[i].MachineID = nil
			m.codes[i].ActivatedAt = nil
			resetCount++
		}
	}
	m.saveToFile()
	return resetCount
}

// ExportCodes 导出卡密（纯文本）
func (m *CodesManager) ExportCodes(filterType string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lines []string
	for _, c := range m.codes {
		switch filterType {
		case "activated":
			if !c.Active {
				continue
			}
		case "unused":
			if c.Active {
				continue
			}
		}
		lines = append(lines, c.Code)
	}
	return strings.Join(lines, "\n")
}

// generateCode 生成随机激活码 XXXX-XXXX-XXXX-XXXX
func generateCode() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	seg := func() string {
		b := make([]byte, 4)
		for i := range b {
			b[i] = chars[rand.Intn(len(chars))]
		}
		return string(b)
	}
	return fmt.Sprintf("%s-%s-%s-%s", seg(), seg(), seg(), seg())
}
