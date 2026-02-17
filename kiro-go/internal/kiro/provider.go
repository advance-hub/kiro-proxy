package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

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
	return p.Client.Do(req)
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
			log.Printf("API 请求发送失败（尝试 %d/%d）: %v", attempt+1, maxRetries, err)
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
			log.Printf("API 请求失败（额度已用尽，尝试 %d/%d）: %d %s", attempt+1, maxRetries, status, bodyStr)
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
			log.Printf("API 请求失败（凭据错误，尝试 %d/%d）: %d %s", attempt+1, maxRetries, status, bodyStr)
			lastErr = fmt.Errorf("API 请求失败: %d %s", status, bodyStr)
			continue
		}

		// 408/429/5xx 瞬态错误 - 重试
		if status == 408 || status == 429 || status >= 500 {
			log.Printf("API 请求失败（瞬态错误，尝试 %d/%d）: %d %s", attempt+1, maxRetries, status, bodyStr)
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
// activationCode 非空时，刷新后的凭证会写回 UserCredentialsManager
// 402 额度用尽时自动切换到下一个可用用户凭证
func (p *Provider) CallWithCredentials(body []byte, cred *model.KiroCredentials, activationCode string) (*http.Response, error) {
	if IsTokenExpired(cred) || IsTokenExpiringSoon(cred) {
		refreshed, err := RefreshToken(cred, p.Config)
		if err != nil {
			return nil, fmt.Errorf("凭证刷新失败: %v", err)
		}
		*cred = *refreshed
		// 写回刷新后的凭证到持久化存储
		if activationCode != "" && p.UserCredsMgr != nil {
			if err := p.UserCredsMgr.AddOrUpdate(activationCode, "", *cred); err != nil {
				log.Printf("写回刷新凭证失败 (%s): %v", activationCode, err)
			} else {
				log.Printf("已写回刷新凭证: %s", activationCode)
			}
		}
	}
	if cred.AccessToken == "" {
		return nil, fmt.Errorf("没有可用的 accessToken")
	}

	resp, err := p.CallAPI(body, cred, cred.AccessToken)
	if err != nil {
		return nil, err
	}

	// 500/429 瞬态错误：自动重试一次（如 MODEL_TEMPORARILY_UNAVAILABLE）
	if resp.StatusCode == 500 || resp.StatusCode == 429 {
		resp.Body.Close()
		log.Printf("收到 %d，等待 2 秒后重试", resp.StatusCode)
		time.Sleep(2 * time.Second)
		retryResp, retryErr := p.CallAPI(body, cred, cred.AccessToken)
		if retryErr != nil {
			return nil, retryErr
		}
		if retryResp.StatusCode == 500 || retryResp.StatusCode == 429 {
			log.Printf("重试仍然返回 %d，放弃", retryResp.StatusCode)
		}
		return retryResp, nil
	}

	// 403 Token 过期：强制刷新后重试一次（参考 kiro-gateway force_refresh 逻辑）
	if resp.StatusCode == 403 {
		resp.Body.Close()
		log.Printf("收到 403，强制刷新 Token 后重试")
		refreshed, refreshErr := RefreshToken(cred, p.Config)
		if refreshErr != nil {
			return nil, fmt.Errorf("403 后强制刷新失败: %v", refreshErr)
		}
		*cred = *refreshed
		if activationCode != "" && p.UserCredsMgr != nil {
			p.UserCredsMgr.AddOrUpdate(activationCode, "", *cred)
		}
		retryResp, retryErr := p.CallAPI(body, cred, cred.AccessToken)
		if retryErr != nil {
			return nil, retryErr
		}
		return retryResp, nil
	}

	// 402 额度用尽：标记当前凭证不可用，尝试自动换号
	if resp.StatusCode == 402 && p.UserCredsMgr != nil && activationCode != "" {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodyStr := string(respBody)

		if isMonthlyRequestLimit(bodyStr) {
			log.Printf("用户 %s 额度已用尽，尝试自动换号", activationCode)
			p.UserCredsMgr.MarkDisabled(activationCode)

			// 尝试下一个可用凭证
			nextCode, nextCred := p.UserCredsMgr.GetNextAvailable(activationCode)
			if nextCode != "" && nextCred != nil {
				log.Printf("自动切换到用户凭证: %s", nextCode)
				return p.CallWithCredentials(body, nextCred, nextCode)
			}

			// 没有其他用户凭证，回退到主凭证池
			log.Printf("没有其他可用用户凭证，回退到主凭证池")
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
	log.Printf("凭据已重新加载，共 %d 个", len(creds))
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
