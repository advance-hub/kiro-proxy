package main

import (
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
	Provider     string `json:"provider"` // Google, Github, BuilderId
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
	// IdC 专用
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	ClientIDHash string `json:"clientIdHash,omitempty"`
	Region       string `json:"region,omitempty"`
	// Social 专用
	ProfileArn string `json:"profileArn,omitempty"`
	// 配额数据
	UsageData map[string]interface{} `json:"usageData,omitempty"`
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

	account := Account{
		ID:           uuid.New().String(),
		Email:        email,
		Label:        label,
		Status:       status,
		AddedAt:      time.Now().Format("2006/01/02 15:04:05"),
		Provider:     provider,
		AccessToken:  derefStr(refreshed.AccessToken),
		RefreshToken: refreshed.RefreshToken,
		ExpiresAt:    refreshed.ExpiresAt,
		ClientID:     clientID,
		ClientSecret: clientSecret,
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
	fmt.Printf("[DEBUG] SyncAccount: Provider=%s, ClientID=%s, isIdC=%v\n",
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
		// 只在成功获取配额时才更新 usageData
		usageBytes, _ := json.Marshal(usage)
		json.Unmarshal(usageBytes, &account.UsageData)
	}

	getAccountStore().Update(*account)
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

	if account.Provider == "BuilderId" {
		tokenData["authMethod"] = "IdC"
		tokenData["clientIdHash"] = account.ClientIDHash
		tokenData["region"] = account.Region

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
	isIdC := account.Provider == "BuilderId" || account.Provider == "Enterprise" || account.ClientID != ""
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
		fmt.Fprintf(os.Stderr, "写入 credentials.json 失败: %v\n", err)
	}

	return fmt.Sprintf("已切换到 %s", account.Email), nil
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
		return a.AddAccountByIdCWithProvider(refreshToken, clientID, clientSecret, region, provider)
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
