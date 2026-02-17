package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ── Account Data Model ──

type Account struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Label        string `json:"label"`
	Status       string `json:"status"` // 正常, 已封禁, Token已失效
	AddedAt      string `json:"addedAt"`
	Provider     string `json:"provider"` // Google, Github, BuilderId, Enterprise
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
	// IdC 专用
	AuthMethod   string `json:"authMethod,omitempty"` // IdC, social
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	ClientIDHash string `json:"clientIdHash,omitempty"`
	Region       string `json:"region,omitempty"`
	// Social 专用
	ProfileArn string `json:"profileArn,omitempty"`
	// 用户信息
	UserID string `json:"userId,omitempty"`
	// 配额数据
	UsageData map[string]interface{} `json:"usageData,omitempty"`
	// 机器标识（用于 relay）
	MachineID string `json:"machineId,omitempty"`
}

// computeClientIDHash 计算 clientId 的 SHA-256 哈希
// 与参考项目 kiro-account-manager 的 compute_client_id_hash 保持一致
func computeClientIDHash(clientID string) string {
	h := sha256.Sum256([]byte(clientID))
	return hex.EncodeToString(h[:])
}

type AccountStore struct {
	accounts []Account
	mu       sync.RWMutex
	filePath string
}

var globalAccountStore *AccountStore
var accountStoreOnce sync.Once

func getAccountStore() *AccountStore {
	accountStoreOnce.Do(func() {
		dir, _ := getDataDir()
		path := filepath.Join(dir, "accounts.json")
		globalAccountStore = &AccountStore{filePath: path}
		globalAccountStore.load()
	})
	return globalAccountStore
}

func (s *AccountStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		s.accounts = []Account{}
		return
	}
	var accounts []Account
	if json.Unmarshal(data, &accounts) != nil {
		s.accounts = []Account{}
		return
	}
	s.accounts = accounts
}

func (s *AccountStore) save() error {
	data, err := json.MarshalIndent(s.accounts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func (s *AccountStore) GetAll() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Account, len(s.accounts))
	copy(result, s.accounts)
	return result
}

func (s *AccountStore) Add(account Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 按 email + provider 去重
	for i, a := range s.accounts {
		if a.Email == account.Email && a.Provider == account.Provider {
			s.accounts[i] = account
			s.save()
			return
		}
	}
	s.accounts = append([]Account{account}, s.accounts...)
	s.save()
}

func (s *AccountStore) Update(account Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.accounts {
		if a.ID == account.ID {
			s.accounts[i] = account
			s.save()
			return
		}
	}
}

func (s *AccountStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.accounts {
		if a.ID == id {
			s.accounts = append(s.accounts[:i], s.accounts[i+1:]...)
			s.save()
			return true
		}
	}
	return false
}

func (s *AccountStore) FindByID(id string) *Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.accounts {
		if a.ID == id {
			return &a
		}
	}
	return nil
}

// ── Usage API ──

const (
	DesktopAuthAPI = "https://prod.us-east-1.auth.desktop.kiro.dev"
	UsageLimitsAPI = "https://codewhisperer.us-east-1.amazonaws.com"
	DefaultProfile = "arn:aws:codewhisperer:us-east-1:699475941385:profile/EHGA3GRVQMUK"
)

type UsageResponse struct {
	UserInfo         *UserInfo         `json:"userInfo,omitempty"`
	SubscriptionInfo *SubscriptionInfo `json:"subscriptionInfo,omitempty"`
	UsageBreakdown   []UsageBreakdown  `json:"usageBreakdownList,omitempty"`
}

type UserInfo struct {
	Email  string `json:"email,omitempty"`
	UserID string `json:"userId,omitempty"`
}

type SubscriptionInfo struct {
	Type  string `json:"type,omitempty"`
	Title string `json:"subscriptionTitle,omitempty"`
}

type UsageBreakdown struct {
	UsageLimit    float64        `json:"usageLimit,omitempty"`
	CurrentUsage  float64        `json:"currentUsage,omitempty"`
	NextDateReset float64        `json:"nextDateReset,omitempty"`
	FreeTrialInfo *FreeTrialInfo `json:"freeTrialInfo,omitempty"`
	Bonuses       []BonusInfo    `json:"bonuses,omitempty"`
}

type FreeTrialInfo struct {
	UsageLimit   float64 `json:"usageLimit,omitempty"`
	CurrentUsage float64 `json:"currentUsage,omitempty"`
}

type BonusInfo struct {
	DisplayName  string  `json:"displayName,omitempty"`
	UsageLimit   float64 `json:"usageLimit,omitempty"`
	CurrentUsage float64 `json:"currentUsage,omitempty"`
}

func getUsageLimits(accessToken string, isIdC bool) (*UsageResponse, error) {
	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	var apiURL string
	if isIdC {
		// IdC/BuilderId 账号使用 resourceType 参数
		apiURL = fmt.Sprintf("%s/getUsageLimits?isEmailRequired=true&origin=AI_EDITOR&resourceType=AGENTIC_REQUEST", UsageLimitsAPI)
	} else {
		// Social 账号使用 profileArn 参数
		apiURL = fmt.Sprintf("%s/getUsageLimits?isEmailRequired=true&origin=AI_EDITOR&profileArn=%s",
			UsageLimitsAPI, url.QueryEscape(DefaultProfile))
	}

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		// 检查是否被封禁
		var errResp map[string]interface{}
		if json.Unmarshal(body, &errResp) == nil {
			if reason, ok := errResp["reason"].(string); ok {
				return nil, fmt.Errorf("BANNED:%s", reason)
			}
		}
		return nil, fmt.Errorf("获取配额失败 (%d): %s", resp.StatusCode, string(body))
	}

	var usage UsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	return &usage, nil
}

// ── Account API Methods ──

func (a *App) GetAccounts() []Account {
	return getAccountStore().GetAll()
}

func (a *App) DeleteAccount(id string) (string, error) {
	if getAccountStore().Delete(id) {
		return "账号已删除", nil
	}
	return "", fmt.Errorf("账号不存在")
}

// AddAccountBySocial 通过 Social RefreshToken 添加账号
func (a *App) AddAccountBySocial(refreshToken string, provider string) (Account, error) {
	if refreshToken == "" {
		return Account{}, fmt.Errorf("RefreshToken 不能为空")
	}
	if provider == "" {
		provider = "Google"
	}

	// 1. 刷新 Token
	creds := &CredentialsFile{
		RefreshToken: refreshToken,
		AuthMethod:   "social",
	}
	refreshed, err := refreshCredentials(creds)
	if err != nil {
		return Account{}, fmt.Errorf("刷新 Token 失败: %v", err)
	}

	// 2. 获取配额和用户信息
	usage, usageErr := getUsageLimits(*refreshed.AccessToken, false)

	email := "unknown@kiro.dev"
	status := "正常"
	var usageData map[string]interface{}

	if usageErr != nil {
		if strings.HasPrefix(usageErr.Error(), "BANNED:") {
			status = "已封禁"
		} else {
			status = "Token已失效"
		}
	} else if usage != nil {
		if usage.UserInfo != nil && usage.UserInfo.Email != "" {
			email = usage.UserInfo.Email
		}
		// 转换为 map
		usageBytes, _ := json.Marshal(usage)
		json.Unmarshal(usageBytes, &usageData)
	}

	account := Account{
		ID:           uuid.New().String(),
		Email:        email,
		Label:        fmt.Sprintf("Kiro %s 账号", provider),
		Status:       status,
		AddedAt:      time.Now().Format("2006/01/02 15:04:05"),
		Provider:     provider,
		AccessToken:  derefStr(refreshed.AccessToken),
		RefreshToken: refreshed.RefreshToken,
		ExpiresAt:    refreshed.ExpiresAt,
		ProfileArn:   DefaultProfile,
		UsageData:    usageData,
	}

	getAccountStore().Add(account)
	return account, nil
}

// AddAccountByIdC 通过 IdC 凭证添加账号
func (a *App) AddAccountByIdC(refreshToken, clientID, clientSecret, region string) (Account, error) {
	return a.AddAccountByIdCWithProvider(refreshToken, clientID, clientSecret, region, "BuilderId")
}

// AddAccountByIdCWithProvider 通过 IdC 凭证添加账号（支持指定 provider）
func (a *App) AddAccountByIdCWithProvider(refreshToken, clientID, clientSecret, region, provider string) (Account, error) {
	if refreshToken == "" || clientID == "" || clientSecret == "" {
		return Account{}, fmt.Errorf("RefreshToken, ClientID, ClientSecret 不能为空")
	}
	if region == "" {
		region = "us-east-1"
	}
	if provider == "" {
		provider = "BuilderId"
	}

	// 1. 刷新 Token
	creds := &CredentialsFile{
		RefreshToken: refreshToken,
		AuthMethod:   "idc",
		ClientID:     &clientID,
		ClientSecret: &clientSecret,
		Region:       &region,
	}
	refreshed, err := refreshCredentials(creds)
	if err != nil {
		return Account{}, fmt.Errorf("刷新 Token 失败: %v", err)
	}

	// 2. 获取配额和用户信息
	usage, usageErr := getUsageLimits(*refreshed.AccessToken, true)

	email := "builderid@kiro.dev"
	status := "正常"
	var usageData map[string]interface{}

	if usageErr != nil {
		if strings.HasPrefix(usageErr.Error(), "BANNED:") {
			status = "已封禁"
		} else {
			status = "Token已失效"
		}
	} else if usage != nil {
		if usage.UserInfo != nil && usage.UserInfo.Email != "" {
			email = usage.UserInfo.Email
		}
		usageBytes, _ := json.Marshal(usage)
		json.Unmarshal(usageBytes, &usageData)
	}

	// 根据 provider 设置标签
	label := fmt.Sprintf("Kiro %s 账号", provider)

	// 计算 clientIdHash（如果缺失）
	clientIdHash := ""
	if clientID != "" {
		clientIdHash = computeClientIDHash(clientID)
	}

	account := Account{
		ID:           uuid.New().String(),
		Email:        email,
		Label:        label,
		Status:       status,
		AddedAt:      time.Now().Format("2006/01/02 15:04:05"),
		Provider:     provider,
		AuthMethod:   "IdC",
		AccessToken:  derefStr(refreshed.AccessToken),
		RefreshToken: refreshed.RefreshToken,
		ExpiresAt:    refreshed.ExpiresAt,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		ClientIDHash: clientIdHash,
		Region:       region,
		UsageData:    usageData,
	}

	getAccountStore().Add(account)
	return account, nil
}

// SyncAccount 刷新账号 Token 并更新配额
func (a *App) SyncAccount(id string) (Account, error) {
	account := getAccountStore().FindByID(id)
	if account == nil {
		return Account{}, fmt.Errorf("账号不存在")
	}

	// 构建凭证
	var creds *CredentialsFile
	isIdC := account.Provider == "BuilderId" || account.Provider == "Enterprise" || account.ClientID != ""

	// 调试日志
	logInfo("SyncAccount: Provider=%s, ClientID=%s, isIdC=%v",
		account.Provider, account.ClientID, isIdC)

	if isIdC {
		creds = &CredentialsFile{
			RefreshToken: account.RefreshToken,
			AuthMethod:   "idc",
			ClientID:     &account.ClientID,
			ClientSecret: &account.ClientSecret,
			Region:       &account.Region,
		}
	} else {
		creds = &CredentialsFile{
			RefreshToken: account.RefreshToken,
			AuthMethod:   "social",
		}
	}

	// 刷新 Token
	refreshed, err := refreshCredentials(creds)
	if err != nil {
		account.Status = "Token已失效"
		getAccountStore().Update(*account)
		return *account, fmt.Errorf("刷新失败: %v", err)
	}

	account.AccessToken = derefStr(refreshed.AccessToken)
	if refreshed.RefreshToken != "" {
		account.RefreshToken = refreshed.RefreshToken
	}
	account.ExpiresAt = refreshed.ExpiresAt

	// 写入 credentials.json（确保 autoUploadToServer 能读到最新凭证）
	saveCredentialsFileSmart(refreshed)

	// 确保 AuthMethod 和 ClientIDHash 正确
	if isIdC {
		account.AuthMethod = "IdC"
		if account.ClientIDHash == "" && account.ClientID != "" {
			account.ClientIDHash = computeClientIDHash(account.ClientID)
		}
	} else {
		account.AuthMethod = "social"
	}

	// 获取配额
	usage, usageErr := getUsageLimits(account.AccessToken, isIdC)
	if usageErr != nil {
		if strings.HasPrefix(usageErr.Error(), "BANNED:") {
			account.Status = "已封禁"
		} else {
			account.Status = "Token已失效"
		}
		// 保留旧的 usageData，不清空
	} else if usage != nil {
		account.Status = "正常"
		if usage.UserInfo != nil && usage.UserInfo.Email != "" {
			account.Email = usage.UserInfo.Email
		}
		if usage.UserInfo != nil && usage.UserInfo.UserID != "" {
			account.UserID = usage.UserInfo.UserID
		}
		// 只在成功获取配额时才更新 usageData
		usageBytes, _ := json.Marshal(usage)
		json.Unmarshal(usageBytes, &account.UsageData)
	}

	getAccountStore().Update(*account)

	// 同步刷新后的凭证到服务器映射
	go autoUploadToServer()

	return *account, nil
}

// SwitchAccount 切换到指定账号
func (a *App) SwitchAccount(id string) (string, error) {
	account := getAccountStore().FindByID(id)
	if account == nil {
		return "", fmt.Errorf("账号不存在")
	}

	if account.RefreshToken == "" {
		return "", fmt.Errorf("账号缺少 RefreshToken")
	}

	// 判断是否为 IdC 账号（BuilderId、Enterprise 或有 clientId 的账号）
	isIdC := account.Provider == "BuilderId" || account.Provider == "Enterprise" || account.ClientID != ""

	// 确保 clientIdHash 存在（如果缺失则计算）
	if isIdC && account.ClientIDHash == "" && account.ClientID != "" {
		account.ClientIDHash = computeClientIDHash(account.ClientID)
		getAccountStore().Update(*account)
	}

	// 写入 ~/.aws/sso/cache/kiro-auth-token.json
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".aws", "sso", "cache")
	os.MkdirAll(cacheDir, 0755)

	tokenPath := filepath.Join(cacheDir, "kiro-auth-token.json")

	// 构建 token 数据
	tokenData := map[string]interface{}{
		"accessToken":  account.AccessToken,
		"refreshToken": account.RefreshToken,
		"expiresAt":    account.ExpiresAt,
		"provider":     account.Provider,
	}

	if isIdC {
		tokenData["authMethod"] = "IdC"
		tokenData["clientIdHash"] = account.ClientIDHash
		region := account.Region
		if region == "" {
			region = "us-east-1"
		}
		tokenData["region"] = region

		// 写入 client registration
		if account.ClientID != "" && account.ClientSecret != "" && account.ClientIDHash != "" {
			regPath := filepath.Join(cacheDir, account.ClientIDHash+".json")
			regData := map[string]interface{}{
				"clientId":     account.ClientID,
				"clientSecret": account.ClientSecret,
				"expiresAt":    time.Now().Add(90 * 24 * time.Hour).Format(time.RFC3339),
			}
			regBytes, _ := json.MarshalIndent(regData, "", "  ")
			os.WriteFile(regPath, regBytes, 0644)
		}
	} else {
		tokenData["authMethod"] = "social"
		tokenData["profileArn"] = account.ProfileArn
		if tokenData["profileArn"] == "" {
			tokenData["profileArn"] = DefaultProfile
		}
	}

	tokenBytes, _ := json.MarshalIndent(tokenData, "", "  ")

	// 原子写入 kiro-auth-token.json
	tmpPath := tokenPath + ".tmp"
	if err := os.WriteFile(tmpPath, tokenBytes, 0644); err != nil {
		return "", fmt.Errorf("写入临时文件失败: %v", err)
	}
	if err := os.Rename(tmpPath, tokenPath); err != nil {
		return "", fmt.Errorf("重命名文件失败: %v", err)
	}

	// 同时写入 credentials.json，让 kiro.rs 代理能通过热加载感知账号切换
	authMethod := "social"
	if isIdC {
		authMethod = "idc"
	}
	creds := &CredentialsFile{
		RefreshToken: account.RefreshToken,
		ExpiresAt:    account.ExpiresAt,
		AuthMethod:   authMethod,
	}
	if account.AccessToken != "" {
		creds.AccessToken = &account.AccessToken
	}
	if isIdC {
		creds.ClientID = &account.ClientID
		creds.ClientSecret = &account.ClientSecret
		region := account.Region
		if region == "" {
			region = "us-east-1"
		}
		creds.Region = &region
	}
	if err := saveCredentialsFileSmart(creds); err != nil {
		// 非致命错误，不阻止切换
		logWarn("写入 credentials.json 失败: %v", err)
	}

	// 通知 kiro-go 代理重新加载凭据（清除缓存的 token）
	go notifyProxyReload()

	// 自动上传凭证到服务器（如果已配置服务器同步）
	go autoUploadToServer()

	return fmt.Sprintf("已切换到 %s", account.Email), nil
}

// autoUploadToServer 切换账号后自动上传凭证到服务器
func autoUploadToServer() {
	dir, err := getDataDir()
	if err != nil {
		return
	}
	// 读取服务器同步配置
	syncData, err := os.ReadFile(filepath.Join(dir, "server_sync.json"))
	if err != nil {
		return // 未配置服务器同步，跳过
	}
	var syncCfg ServerSyncConfig
	if json.Unmarshal(syncData, &syncCfg) != nil || syncCfg.ServerURL == "" || syncCfg.ActivationCode == "" {
		return
	}

	logInfo("自动同步凭证到服务器: %s (激活码: %s)", syncCfg.ServerURL, syncCfg.ActivationCode)

	// 读取本地凭证
	credsData, err := os.ReadFile(filepath.Join(dir, "credentials.json"))
	if err != nil {
		logError("自动上传：读取凭证失败: %v", err)
		return
	}
	var rawCreds map[string]interface{}
	if json.Unmarshal(credsData, &rawCreds) != nil {
		return
	}

	// 构造上传请求（包含所有凭证字段，确保服务器能独立刷新 Token）
	credentials := map[string]interface{}{
		"accessToken":  rawCreds["accessToken"],
		"refreshToken": rawCreds["refreshToken"],
		"expiresAt":    rawCreds["expiresAt"],
	}
	if v, ok := rawCreds["authMethod"].(string); ok && v != "" {
		credentials["authMethod"] = v
	}
	if v, ok := rawCreds["clientId"].(string); ok && v != "" {
		credentials["clientId"] = v
	}
	if v, ok := rawCreds["clientSecret"].(string); ok && v != "" {
		credentials["clientSecret"] = v
	}
	if v, ok := rawCreds["region"].(string); ok && v != "" {
		credentials["region"] = v
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"activation_code": syncCfg.ActivationCode,
		"credentials":     credentials,
	})

	client := &http.Client{Timeout: 5 * time.Second}
	url := syncCfg.ServerURL + "/api/admin/user-credentials"
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		logError("自动上传凭证到服务器失败: %v", err)
		return
	}
	resp.Body.Close()
	logInfo("✅ 已自动上传凭证到服务器 %s (状态: %d)", syncCfg.ServerURL, resp.StatusCode)
}

// notifyProxyReload 通知本地代理重新加载凭据
func notifyProxyReload() {
	// 读取代理配置获取端口
	dir, err := getDataDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return
	}
	var cfg ProxyConfig
	if json.Unmarshal(data, &cfg) != nil {
		return
	}
	port := cfg.Port
	if port == 0 {
		port = 13000
	}
	host := cfg.Host
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}

	url := fmt.Sprintf("http://%s:%d/api/admin/reload-credentials", host, port)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		logWarn("通知代理重新加载凭据失败: %v", err)
		return
	}
	resp.Body.Close()
	logInfo("已通知代理重新加载凭据")
}

// ImportLocalAccount 导入本地 Kiro IDE 账号
func (a *App) ImportLocalAccount() (Account, error) {
	// 读取 ~/.aws/sso/cache/kiro-auth-token.json
	home, _ := os.UserHomeDir()
	tokenPath := filepath.Join(home, ".aws", "sso", "cache", "kiro-auth-token.json")

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return Account{}, fmt.Errorf("未找到本地账号，请先在 Kiro IDE 中登录")
	}

	var tokenJSON map[string]interface{}
	if json.Unmarshal(data, &tokenJSON) != nil {
		return Account{}, fmt.Errorf("解析本地 Token 失败")
	}

	refreshToken, _ := tokenJSON["refreshToken"].(string)
	if refreshToken == "" {
		return Account{}, fmt.Errorf("本地账号缺少 RefreshToken")
	}

	authMethod, _ := tokenJSON["authMethod"].(string)
	provider, _ := tokenJSON["provider"].(string)

	if strings.EqualFold(authMethod, "IdC") {
		// IdC 账号 (BuilderId 或 Enterprise)
		clientIdHash, _ := tokenJSON["clientIdHash"].(string)
		region, _ := tokenJSON["region"].(string)
		if region == "" {
			region = "us-east-1"
		}

		// 读取 client registration
		regPath := filepath.Join(home, ".aws", "sso", "cache", clientIdHash+".json")
		regData, err := os.ReadFile(regPath)
		if err != nil {
			return Account{}, fmt.Errorf("未找到 IdC 客户端注册信息")
		}

		var reg map[string]interface{}
		json.Unmarshal(regData, &reg)
		clientID, _ := reg["clientId"].(string)
		clientSecret, _ := reg["clientSecret"].(string)

		// 传递 provider 信息
		if provider == "" {
			provider = "BuilderId"
		}
		account, err := a.AddAccountByIdCWithProvider(refreshToken, clientID, clientSecret, region, provider)
		if err != nil {
			return account, err
		}
		// 确保 clientIdHash 被保存
		if account.ClientIDHash == "" && clientIdHash != "" {
			account.ClientIDHash = clientIdHash
			getAccountStore().Update(account)
		}
		return account, nil
	}

	// Social 账号
	if provider == "" {
		provider = "Google"
	}
	return a.AddAccountBySocial(refreshToken, provider)
}

// UpdateAccountLabel 更新账号备注
func (a *App) UpdateAccountLabel(id, label string) (Account, error) {
	account := getAccountStore().FindByID(id)
	if account == nil {
		return Account{}, fmt.Errorf("账号不存在")
	}
	account.Label = label
	getAccountStore().Update(*account)
	return *account, nil
}

// UpdateAccount 更新账号信息
func (a *App) UpdateAccount(id string, label, accessToken, refreshToken, clientId, clientSecret *string) (Account, error) {
	account := getAccountStore().FindByID(id)
	if account == nil {
		return Account{}, fmt.Errorf("账号不存在")
	}
	if label != nil {
		account.Label = *label
	}
	if accessToken != nil {
		account.AccessToken = *accessToken
	}
	if refreshToken != nil {
		account.RefreshToken = *refreshToken
	}
	if clientId != nil {
		account.ClientID = *clientId
	}
	if clientSecret != nil {
		account.ClientSecret = *clientSecret
	}
	getAccountStore().Update(*account)
	return *account, nil
}

// ExportAccounts 导出账号为 JSON
func (a *App) ExportAccounts(ids []string) (string, error) {
	store := getAccountStore()
	var accounts []Account

	if len(ids) == 0 {
		accounts = store.GetAll()
	} else {
		for _, id := range ids {
			if acc := store.FindByID(id); acc != nil {
				accounts = append(accounts, *acc)
			}
		}
	}

	// 导出格式：只包含必要字段
	type ExportAccount struct {
		Email        string `json:"email"`
		Label        string `json:"label,omitempty"`
		Provider     string `json:"provider"`
		RefreshToken string `json:"refreshToken"`
		AuthMethod   string `json:"authMethod,omitempty"`
		ClientID     string `json:"clientId,omitempty"`
		ClientSecret string `json:"clientSecret,omitempty"`
		ClientIDHash string `json:"clientIdHash,omitempty"`
		Region       string `json:"region,omitempty"`
	}

	exports := make([]ExportAccount, len(accounts))
	for i, acc := range accounts {
		// 根据账号类型设置 authMethod
		authMethod := "social"
		if acc.Provider == "BuilderId" || acc.Provider == "Enterprise" || acc.ClientID != "" {
			authMethod = "idc"
		}

		exports[i] = ExportAccount{
			Email:        acc.Email,
			Label:        acc.Label,
			Provider:     acc.Provider,
			RefreshToken: acc.RefreshToken,
			AuthMethod:   authMethod,
			ClientID:     acc.ClientID,
			ClientSecret: acc.ClientSecret,
			ClientIDHash: acc.ClientIDHash,
			Region:       acc.Region,
		}
	}

	data, err := json.MarshalIndent(exports, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ExportAccountsToFile 导出账号到文件（带文件保存对话框）
func (a *App) ExportAccountsToFile(ids []string) (string, error) {
	// 获取导出数据
	jsonData, err := a.ExportAccounts(ids)
	if err != nil {
		return "", fmt.Errorf("生成导出数据失败: %v", err)
	}

	// 显示文件保存对话框
	defaultFilename := fmt.Sprintf("kiro-accounts-%s.json", time.Now().Format("2006-01-02"))
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "导出账号",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON Files (*.json)", Pattern: "*.json"},
		},
	})

	if err != nil {
		return "", fmt.Errorf("文件对话框失败: %v", err)
	}

	if filePath == "" {
		return "", fmt.Errorf("用户取消了保存")
	}

	// 保存文件
	if err := os.WriteFile(filePath, []byte(jsonData), 0644); err != nil {
		return "", fmt.Errorf("保存文件失败: %v", err)
	}

	count := len(ids)
	if count == 0 {
		count = len(getAccountStore().GetAll())
	}
	return fmt.Sprintf("已导出 %d 个账号到 %s", count, filePath), nil
}

// BatchDeleteAccounts 批量删除账号
func (a *App) BatchDeleteAccounts(ids []string) (int, error) {
	count := 0
	for _, id := range ids {
		if getAccountStore().Delete(id) {
			count++
		}
	}
	return count, nil
}
