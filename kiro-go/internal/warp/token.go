package warp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const firebaseAPIKey = "AIzaSyBdy3O3S9hrdayLJxJ7mriBR4qgUaUygAs"

// WarpCredential Warp 凭证
type WarpCredential struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	RefreshToken string `json:"refreshToken"`
	AccessToken  string `json:"accessToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
	UseCount     int64  `json:"useCount,omitempty"`
	ErrorCount   int    `json:"errorCount,omitempty"`
	LastError    string `json:"lastError,omitempty"`
	ApiKey       string `json:"apiKey,omitempty"`   // wk-xxx 格式的永久 API Key（上号器模式）
	AuthMode     string `json:"authMode,omitempty"` // "apikey" 或 "firebase"（默认）
}

// WarpCredentialStore 凭证存储
type WarpCredentialStore struct {
	mu          sync.RWMutex
	credentials []*WarpCredential
	filePath    string
	current     int
}

func NewCredentialStore(path string) *WarpCredentialStore {
	return &WarpCredentialStore{filePath: path}
}

func (s *WarpCredentialStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := readFileBytes(s.filePath)
	if err != nil {
		// 文件不存在，尝试自动获取匿名 token
		log.Printf("[Warp] 凭证文件不存在，尝试获取匿名访问 token...")
		s.mu.Unlock() // 临时解锁以调用 AcquireAnonymousAccessToken
		anonCred, anonErr := AcquireAnonymousAccessToken()
		s.mu.Lock()
		if anonErr != nil {
			log.Printf("[Warp] 获取匿名 token 失败: %v", anonErr)
			s.credentials = nil
			return nil
		}
		log.Printf("[Warp] ✅ 匿名 token 获取成功")
		s.credentials = []*WarpCredential{anonCred}
		// 保存到文件
		s.mu.Unlock()
		saveErr := s.Save()
		s.mu.Lock()
		if saveErr != nil {
			log.Printf("[Warp] 保存匿名凭证失败: %v", saveErr)
		}
		return nil
	}
	var creds []*WarpCredential
	if err := json.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("解析 warp 凭证失败: %w", err)
	}
	for i, c := range creds {
		if c.ID == 0 {
			c.ID = i + 1
		}
	}

	// 如果没有可用凭证，尝试获取匿名 token
	if len(creds) == 0 {
		log.Printf("[Warp] 凭证列表为空，尝试获取匿名访问 token...")
		s.mu.Unlock()
		anonCred, anonErr := AcquireAnonymousAccessToken()
		s.mu.Lock()
		if anonErr != nil {
			log.Printf("[Warp] 获取匿名 token 失败: %v", anonErr)
			s.credentials = nil
			return nil
		}
		log.Printf("[Warp] ✅ 匿名 token 获取成功")
		creds = []*WarpCredential{anonCred}
		s.credentials = creds
		s.mu.Unlock()
		saveErr := s.Save()
		s.mu.Lock()
		if saveErr != nil {
			log.Printf("[Warp] 保存匿名凭证失败: %v", saveErr)
		}
		return nil
	}

	s.credentials = creds
	return nil
}

func (s *WarpCredentialStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.MarshalIndent(s.credentials, "", "  ")
	if err != nil {
		return err
	}
	return writeFileBytes(s.filePath, data)
}

func (s *WarpCredentialStore) GetAll() []*WarpCredential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*WarpCredential, len(s.credentials))
	copy(result, s.credentials)
	return result
}

func (s *WarpCredentialStore) GetAllActive() []*WarpCredential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*WarpCredential
	for _, c := range s.credentials {
		if !c.Disabled {
			result = append(result, c)
		}
	}
	return result
}

// GetNext 轮询获取下一个可用凭证
func (s *WarpCredentialStore) GetNext() *WarpCredential {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := make([]*WarpCredential, 0)
	for _, c := range s.credentials {
		if !c.Disabled {
			active = append(active, c)
		}
	}
	if len(active) == 0 {
		return nil
	}
	c := active[s.current%len(active)]
	s.current++
	return c
}

func (s *WarpCredentialStore) IncrementUseCount(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.credentials {
		if c.ID == id {
			c.UseCount++
			return
		}
	}
}

func (s *WarpCredentialStore) MarkError(id int, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.credentials {
		if c.ID == id {
			c.ErrorCount++
			c.LastError = errMsg
			return
		}
	}
}

func (s *WarpCredentialStore) MarkDisabled(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.credentials {
		if c.ID == id {
			c.Disabled = true
			return
		}
	}
}

func (s *WarpCredentialStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.credentials)
}

func (s *WarpCredentialStore) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, c := range s.credentials {
		if !c.Disabled {
			n++
		}
	}
	return n
}

// ── Token 刷新 ──

type tokenRefreshResult struct {
	AccessToken  string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    string `json:"expires_in"`
}

// RefreshAccessToken 使用 Firebase refresh token 获取新的 access token
func RefreshAccessToken(refreshToken string) (accessToken string, expiresIn int, err error) {
	payload := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	apiURL := fmt.Sprintf("https://securetoken.googleapis.com/v1/token?key=%s", firebaseAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(apiURL, "application/x-www-form-urlencoded", strings.NewReader(payload.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("token 刷新请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("token 刷新失败 HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result tokenRefreshResult
	if err := json.Unmarshal(body, &result); err != nil {
		return "", 0, fmt.Errorf("解析 token 响应失败: %w", err)
	}

	expires := 3600
	if result.ExpiresIn != "" {
		fmt.Sscanf(result.ExpiresIn, "%d", &expires)
	}

	return result.AccessToken, expires, nil
}

// GetValidAccessToken 获取有效的 access token，过期则自动刷新
// 对于 apikey 模式（wk-xxx），直接返回 apikey 作为 bearer token
func GetValidAccessToken(cred *WarpCredential) (string, error) {
	// apikey 模式：直接使用 wk-xxx 作为 bearer token
	if cred.ApiKey != "" {
		return cred.ApiKey, nil
	}

	// firebase 模式：使用 refresh token 刷新 access token
	if cred.AccessToken != "" && !IsTokenExpired(cred.AccessToken) {
		return cred.AccessToken, nil
	}

	if cred.RefreshToken == "" {
		return "", fmt.Errorf("凭证 #%d (%s) 缺少 refreshToken 和 apiKey", cred.ID, cred.Name)
	}

	log.Printf("[Warp] 刷新 token: %s (#%d)", cred.Name, cred.ID)
	accessToken, expiresIn, err := RefreshAccessToken(cred.RefreshToken)
	if err != nil {
		return "", err
	}

	cred.AccessToken = accessToken
	cred.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second).Unix()
	return accessToken, nil
}

// IsTokenExpired 检查 JWT token 是否过期（5 分钟缓冲）
func IsTokenExpired(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return true
	}
	payload := parts[1]
	// base64 padding
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return true
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(decoded, &claims) != nil || claims.Exp == 0 {
		return true
	}
	return time.Now().Unix()+300 >= claims.Exp
}

// GetEmailFromToken 从 JWT token 中提取邮箱
func GetEmailFromToken(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}
	var claims struct {
		Email string `json:"email"`
	}
	json.Unmarshal(decoded, &claims)
	return claims.Email
}

// RefreshAllTokens 批量刷新所有凭证的 token
func RefreshAllTokens(store *WarpCredentialStore) (success, failed int) {
	creds := store.GetAll()
	for _, c := range creds {
		if c.Disabled {
			continue
		}
		_, err := GetValidAccessToken(c)
		if err != nil {
			log.Printf("[Warp] 刷新 token 失败 #%d (%s): %v", c.ID, c.Name, err)
			store.MarkError(c.ID, err.Error())
			failed++
		} else {
			success++
		}
	}
	if success > 0 || failed > 0 {
		store.Save()
	}
	return
}
