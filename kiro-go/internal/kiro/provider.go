package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"kiro-go/internal/logger"
	"kiro-go/internal/model"

	"github.com/google/uuid"
)

const (
	maxRetriesPerCredential = 3
	maxTotalRetries         = 9
)

// Provider 负责与 Kiro API 通信
type Provider struct {
	Config       *model.Config
	TokenMgr     *TokenManager
	UserCredsMgr *UserCredentialsManager
	Client       *http.Client
}

func NewProvider(cfg *model.Config, tm *TokenManager) *Provider {
	return &Provider{
		Config:   cfg,
		TokenMgr: tm,
		Client:   &http.Client{Timeout: 720 * time.Second},
	}
}

// GetModels 从 Kiro API 获取模型列表
func (p *Provider) GetModels() ([]map[string]interface{}, error) {
	// 获取一个可用的凭证和 token
	cred, token, err := p.TokenMgr.AcquireContext()
	if err != nil {
		return nil, fmt.Errorf("获取凭证和 token 失败: %w", err)
	}

	// 构建请求
	region := cred.EffectiveRegion(p.Config)
	url := fmt.Sprintf("https://q.%s.amazonaws.com/ListAvailableModels?origin=AI_EDITOR&profileArn=arn%%3Aaws%%3Acodewhisperer%%3A%s%%3A143353052107%%3Aprofile%%2FMQECWP49CVEW", region, region)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	mid := GenerateMachineID(cred, p.Config)
	kv := p.Config.KiroVersion
	req.Header.Set("User-Agent", fmt.Sprintf("aws-sdk-js/1.0.0 ua/2.1 os/%s lang/js md/nodejs#%s api/codewhispererruntime#1.0.0 m/N,E KiroIDE-%s-%s", p.Config.SystemVersion, p.Config.NodeVersion, kv, mid))
	req.Header.Set("x-amz-user-agent", fmt.Sprintf("aws-sdk-js/1.0.0 KiroIDE-%s-%s", kv, mid))
	req.Header.Set("amz-sdk-invocation-id", uuid.New().String())
	req.Header.Set("amz-sdk-request", "attempt=1; max=1")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Connection", "close")

	// 发送请求
	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误 %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var result struct {
		Models []struct {
			ModelID             string   `json:"modelId"`
			ModelName           string   `json:"modelName"`
			Description         string   `json:"description"`
			RateMultiplier      float64  `json:"rateMultiplier"`
			RateUnit            string   `json:"rateUnit"`
			SupportedInputTypes []string `json:"supportedInputTypes"`
			TokenLimits         struct {
				MaxInputTokens  int  `json:"maxInputTokens"`
				MaxOutputTokens *int `json:"maxOutputTokens"`
			} `json:"tokenLimits"`
			PromptCaching struct {
				SupportsPromptCaching             bool `json:"supportsPromptCaching"`
				MaximumCacheCheckpointsPerRequest int  `json:"maximumCacheCheckpointsPerRequest"`
				MinimumTokensPerCacheCheckpoint   int  `json:"minimumTokensPerCacheCheckpoint"`
			} `json:"promptCaching"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 转换为标准格式（保留所有原始字段）
	models := []map[string]interface{}{}
	for _, m := range result.Models {
		maxTokens := 200000
		if m.TokenLimits.MaxInputTokens > 0 {
			maxTokens = m.TokenLimits.MaxInputTokens
		}

		model := map[string]interface{}{
			"id":           m.ModelID,
			"object":       "model",
			"created":      time.Now().Unix(),
			"owned_by":     "anthropic",
			"display_name": m.ModelName,
			"type":         "chat",
			"max_tokens":   maxTokens,
			// 保留原始 API 字段
			"modelId":             m.ModelID,
			"modelName":           m.ModelName,
			"description":         m.Description,
			"rateMultiplier":      m.RateMultiplier,
			"rateUnit":            m.RateUnit,
			"supportedInputTypes": m.SupportedInputTypes,
		}

		// 添加 tokenLimits
		tokenLimits := map[string]interface{}{
			"maxInputTokens": m.TokenLimits.MaxInputTokens,
		}
		if m.TokenLimits.MaxOutputTokens != nil {
			tokenLimits["maxOutputTokens"] = *m.TokenLimits.MaxOutputTokens
		} else {
			tokenLimits["maxOutputTokens"] = nil
		}
		model["tokenLimits"] = tokenLimits

		// 添加 promptCaching
		model["promptCaching"] = map[string]interface{}{
			"supportsPromptCaching":             m.PromptCaching.SupportsPromptCaching,
			"maximumCacheCheckpointsPerRequest": m.PromptCaching.MaximumCacheCheckpointsPerRequest,
			"minimumTokensPerCacheCheckpoint":   m.PromptCaching.MinimumTokensPerCacheCheckpoint,
		}

		models = append(models, model)
	}

	return models, nil
}

func (p *Provider) BaseURL(cred *model.KiroCredentials) string {
	return fmt.Sprintf("https://q.%s.amazonaws.com/generateAssistantResponse", cred.EffectiveRegion(p.Config))
}

func (p *Provider) BaseDomain(cred *model.KiroCredentials) string {
	return fmt.Sprintf("q.%s.amazonaws.com", cred.EffectiveRegion(p.Config))
}

func (p *Provider) BuildHeaders(cred *model.KiroCredentials, token string) http.Header {
	mid := GenerateMachineID(cred, p.Config)
	kv := p.Config.KiroVersion
	osName := p.Config.SystemVersion
	nv := p.Config.NodeVersion

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("x-amzn-codewhisperer-optout", "true")
	h.Set("x-amzn-kiro-agent-mode", "vibe")
	h.Set("x-amz-user-agent", fmt.Sprintf("aws-sdk-js/1.0.27 KiroIDE-%s-%s", kv, mid))
	h.Set("User-Agent", fmt.Sprintf("aws-sdk-js/1.0.27 ua/2.1 os/%s lang/js md/nodejs#%s api/codewhispererstreaming#1.0.27 m/E KiroIDE-%s-%s", osName, nv, kv, mid))
	h.Set("Host", p.BaseDomain(cred))
	h.Set("amz-sdk-invocation-id", uuid.New().String())
	h.Set("amz-sdk-request", "attempt=1; max=3")
	h.Set("Authorization", "Bearer "+token)
	h.Set("Connection", "close")
	return h
}

// CallAPI 发送 API 请求
func (p *Provider) CallAPI(body []byte, cred *model.KiroCredentials, token string) (*http.Response, error) {
	req, err := http.NewRequest("POST", p.BaseURL(cred), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = p.BuildHeaders(cred, token)

	// 记录上游请求
	rid := req.Header.Get("amz-sdk-invocation-id")
	logger.LogHTTPRequest(rid, "", req.Method, req.URL.String(), req.Header, body, 2000)

	startTime := time.Now()
	resp, err := p.Client.Do(req)
	latency := time.Since(startTime)

	if err != nil {
		logger.ErrorFields(logger.CatHTTP, "上游请求失败", logger.F{
			"rid":     rid,
			"url":     req.URL.String(),
			"error":   err.Error(),
			"latency": latency.String(),
		})
		return nil, err
	}

	// 记录上游响应（只读取前 2000 字节用于日志，不影响后续读取）
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		logger.LogHTTPResponse(rid, "", resp.StatusCode, respBody, latency, 2000)
		// 重新包装 body 供后续使用
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
	} else {
		logger.LogHTTPResponse(rid, "", resp.StatusCode, nil, latency, 0)
	}

	return resp, nil
}

// CallWithTokenManager 使用 TokenManager 获取凭证并调用（带重试和故障转移）
func (p *Provider) CallWithTokenManager(body []byte) (*http.Response, *model.KiroCredentials, error) {
	totalCreds := len(p.TokenMgr.Credentials)
	maxRetries := totalCreds * maxRetriesPerCredential
	if maxRetries > maxTotalRetries {
		maxRetries = maxTotalRetries
	}
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		cred, token, err := p.TokenMgr.AcquireContext()
		if err != nil {
			lastErr = err
			continue
		}

		resp, err := p.CallAPI(body, cred, token)
		if err != nil {
			logger.Warnf(logger.CatProxy, "API 请求发送失败（尝试 %d/%d）: %v", attempt+1, maxRetries, err)
			lastErr = err
			if attempt+1 < maxRetries {
				time.Sleep(retryDelay(attempt))
			}
			continue
		}

		status := resp.StatusCode

		// 成功
		if status >= 200 && status < 300 {
			return resp, cred, nil
		}

		// 读取错误 body
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodyStr := string(respBody)

		// 402 额度用尽
		if status == 402 && isMonthlyRequestLimit(bodyStr) {
			logger.Warnf(logger.CatProxy, "API 请求失败（额度已用尽，尝试 %d/%d）: %d %s", attempt+1, maxRetries, status, bodyStr)
			cred.Disabled = true
			lastErr = fmt.Errorf("API 请求失败: %d %s", status, bodyStr)
			continue
		}

		// 400 Bad Request - 不重试
		if status == 400 {
			return nil, nil, fmt.Errorf("API 请求失败: %d %s", status, bodyStr)
		}

		// 401/403 凭据问题 - 切换凭据重试
		if status == 401 || status == 403 {
			logger.Warnf(logger.CatProxy, "API 请求失败（凭据错误，尝试 %d/%d）: %d %s", attempt+1, maxRetries, status, bodyStr)
			lastErr = fmt.Errorf("API 请求失败: %d %s", status, bodyStr)
			continue
		}

		// 408/429/5xx 瞬态错误 - 重试
		if status == 408 || status == 429 || status >= 500 {
			logger.Warnf(logger.CatProxy, "API 请求失败（瞬态错误，尝试 %d/%d）: %d %s", attempt+1, maxRetries, status, bodyStr)
			lastErr = fmt.Errorf("API 请求失败: %d %s", status, bodyStr)
			if attempt+1 < maxRetries {
				time.Sleep(retryDelay(attempt))
			}
			continue
		}

		// 其他 4xx - 不重试
		if status >= 400 && status < 500 {
			return nil, nil, fmt.Errorf("API 请求失败: %d %s", status, bodyStr)
		}

		// 兜底
		lastErr = fmt.Errorf("API 请求失败: %d %s", status, bodyStr)
		if attempt+1 < maxRetries {
			time.Sleep(retryDelay(attempt))
		}
	}

	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("API 请求失败：已达到最大重试次数（%d次）", maxRetries)
}

// CallWithCredentials 使用指定凭证调用（act- 模式）
// 不在请求路径上刷新 token，直接使用现有 token（即使过期，Kiro API 仍可接受）
// Token 刷新由 kiro-launcher 负责
// 402 额度用尽时自动切换到下一个可用用户凭证
func (p *Provider) CallWithCredentials(body []byte, cred *model.KiroCredentials, activationCode string) (*http.Response, error) {
	if cred.AccessToken == "" {
		return nil, fmt.Errorf("没有可用的 accessToken")
	}

	resp, err := p.CallAPI(body, cred, cred.AccessToken)
	if err != nil {
		return nil, err
	}

	// 500/429 瞬态错误：自动重试最多 3 次（如 MODEL_TEMPORARILY_UNAVAILABLE）
	if resp.StatusCode == 500 || resp.StatusCode == 429 {
		maxRetries := 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			resp.Body.Close()
			delay := time.Duration(attempt*2) * time.Second
			logger.Warnf(logger.CatProxy, "收到 %d，等待 %v 后重试 (%d/%d)", resp.StatusCode, delay, attempt, maxRetries)
			time.Sleep(delay)
			retryResp, retryErr := p.CallAPI(body, cred, cred.AccessToken)
			if retryErr != nil {
				return nil, retryErr
			}
			if retryResp.StatusCode != 500 && retryResp.StatusCode != 429 {
				return retryResp, nil
			}
			resp = retryResp
		}
		logger.Warnf(logger.CatProxy, "重试 %d 次后仍然返回 %d，放弃", maxRetries, resp.StatusCode)
		return resp, nil
	}

	// 402 额度用尽：标记当前凭证不可用，尝试自动换号
	if resp.StatusCode == 402 && p.UserCredsMgr != nil && activationCode != "" {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodyStr := string(respBody)

		if isMonthlyRequestLimit(bodyStr) {
			logger.Warnf(logger.CatCreds, "用户 %s 额度已用尽，尝试自动换号", logger.MaskKey(activationCode))
			p.UserCredsMgr.MarkDisabled(activationCode)

			// 尝试下一个可用凭证
			nextCode, nextCred := p.UserCredsMgr.GetNextAvailable(activationCode)
			if nextCode != "" && nextCred != nil {
				logger.Infof(logger.CatCreds, "自动切换到用户凭证: %s", logger.MaskKey(nextCode))
				return p.CallWithCredentials(body, nextCred, nextCode)
			}

			// 没有其他用户凭证，回退到主凭证池
			logger.Warnf(logger.CatCreds, "没有其他可用用户凭证，回退到主凭证池")
			mainResp, _, mainErr := p.CallWithTokenManager(body)
			if mainErr != nil {
				return nil, fmt.Errorf("所有凭证额度已用尽: %s", bodyStr)
			}
			return mainResp, nil
		}
	}

	return resp, nil
}

// MCPURL 获取 MCP API URL
func (p *Provider) MCPURL(cred *model.KiroCredentials) string {
	return fmt.Sprintf("https://q.%s.amazonaws.com/mcp", cred.EffectiveRegion(p.Config))
}

// CallMCP 调用 MCP API（用于 WebSearch 等工具）
func (p *Provider) CallMCP(body []byte) (*http.Response, error) {
	cred, token, err := p.TokenMgr.AcquireContext()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", p.MCPURL(cred), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = p.BuildHeaders(cred, token)
	return p.Client.Do(req)
}

// ReloadCredentials 重新加载凭据（用于账号切换时清除缓存）
func (p *Provider) ReloadCredentials(creds []*model.KiroCredentials) {
	p.TokenMgr.mu.Lock()
	defer p.TokenMgr.mu.Unlock()
	p.TokenMgr.Credentials = creds
	p.TokenMgr.current = 0
	logger.Infof(logger.CatCreds, "凭据已重新加载，共 %d 个", len(creds))
}

// retryDelay 指数退避 + 抖动
func retryDelay(attempt int) time.Duration {
	baseMs := 200
	maxMs := 2000
	exp := baseMs
	for i := 0; i < attempt && i < 6; i++ {
		exp *= 2
	}
	if exp > maxMs {
		exp = maxMs
	}
	jitter := rand.Intn(exp/4 + 1)
	return time.Duration(exp+jitter) * time.Millisecond
}

// isMonthlyRequestLimit 检测是否为月度额度用尽
func isMonthlyRequestLimit(body string) bool {
	if strings.Contains(body, "MONTHLY_REQUEST_COUNT") {
		return true
	}
	var v map[string]interface{}
	if json.Unmarshal([]byte(body), &v) != nil {
		return false
	}
	if reason, ok := v["reason"].(string); ok && reason == "MONTHLY_REQUEST_COUNT" {
		return true
	}
	if errObj, ok := v["error"].(map[string]interface{}); ok {
		if reason, ok := errObj["reason"].(string); ok && reason == "MONTHLY_REQUEST_COUNT" {
			return true
		}
	}
	return false
}
