package claudecode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Provider Claude Code 反向代理
type Provider struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

// NewProvider 创建 Claude Code Provider
func NewProvider(baseURL, apiKey string) *Provider {
	return &Provider{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// ProxyRequest 代理请求到 Claude Code
func (p *Provider) ProxyRequest(method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	url := p.BaseURL + path
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// 复制其他请求头
	for k, v := range headers {
		if k != "Authorization" && k != "Content-Type" {
			req.Header.Set(k, v)
		}
	}

	return p.Client.Do(req)
}

// CallMessages 调用 /v1/messages 端点
func (p *Provider) CallMessages(reqBody []byte) (*http.Response, error) {
	return p.ProxyRequest("POST", "/v1/messages", reqBody, nil)
}

// GetModels 获取模型列表（从 Claude Code API）
func (p *Provider) GetModels() ([]map[string]interface{}, error) {
	resp, err := p.ProxyRequest("GET", "/v1/models", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("获取模型列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return result.Data, nil
}
