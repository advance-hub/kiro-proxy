package codex

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	codexBaseURL = "https://chatgpt.com/backend-api/codex"
	codexHost    = "chatgpt.com"
)

// Provider Codex API provider
type Provider struct {
	Store  *CredentialStore
	Client *http.Client
}

func NewProvider(store *CredentialStore) *Provider {
	return &Provider{
		Store: store,
		Client: &http.Client{
			Timeout: 720 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// codexRequest Codex API 请求体
type codexRequest struct {
	Model           string                   `json:"model"`
	Instructions    string                   `json:"instructions"`
	Messages        []map[string]interface{} `json:"messages"`
	Tools           []map[string]interface{} `json:"tools,omitempty"`
	Stream          bool                     `json:"stream"`
	MaxOutputTokens int                      `json:"max_output_tokens,omitempty"`
}

// SendRequest 发送请求到 Codex API
func (p *Provider) SendRequest(body []byte, sessionToken string, stream bool) (*http.Response, error) {
	url := codexBaseURL
	if stream {
		url += "?stream=true"
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Accept", "text/event-stream")
	return p.Client.Do(req)
}

// SendWithRetry 带重试和故障转移的请求
func (p *Provider) SendWithRetry(body []byte, stream bool, maxRetries int) (*http.Response, *Credential, error) {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	tried := make(map[int]bool)
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		cred := p.getNextAvailable(tried)
		if cred == nil {
			if lastErr != nil {
				return nil, nil, lastErr
			}
			return nil, nil, fmt.Errorf("没有可用的 Codex 账号")
		}
		tried[cred.ID] = true

		log.Printf("[Codex] 尝试 %d/%d 使用凭证 #%d (%s)", attempt+1, maxRetries, cred.ID, cred.Name)

		resp, err := p.SendRequest(body, cred.SessionToken, stream)
		if err != nil {
			log.Printf("[Codex] 请求失败 #%d: %v", cred.ID, err)
			p.Store.MarkError(cred.ID, err.Error())
			lastErr = err
			continue
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			log.Printf("[Codex] 429 配额耗尽 #%d, 切换下一个", cred.ID)
			p.Store.MarkDisabled(cred.ID)
			lastErr = fmt.Errorf("HTTP 429: 配额耗尽")
			time.Sleep(time.Second)
			continue
		}

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			errBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(errBody))
			log.Printf("[Codex] 认证失败 #%d: %s", cred.ID, errMsg)
			p.Store.MarkError(cred.ID, errMsg)
			p.Store.MarkDisabled(cred.ID)
			lastErr = fmt.Errorf("%s", errMsg)
			continue
		}

		if resp.StatusCode != 200 {
			errBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(errBody))
			log.Printf("[Codex] 请求错误 #%d: %s", cred.ID, errMsg)
			p.Store.MarkError(cred.ID, errMsg)
			lastErr = fmt.Errorf("%s", errMsg)
			if resp.StatusCode >= 500 {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			return nil, nil, lastErr
		}

		p.Store.IncrementUseCount(cred.ID)
		return resp, cred, nil
	}

	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("所有重试都失败")
}

func (p *Provider) getNextAvailable(tried map[int]bool) *Credential {
	active := p.Store.GetAllActive()
	for _, c := range active {
		if !tried[c.ID] {
			return c
		}
	}
	return nil
}

// ── Claude → Codex 请求转换 ──

// ConvertClaudeToCodex 将 Claude API 请求转换为 Codex 请求
func ConvertClaudeToCodex(model string, messages []json.RawMessage, system string, tools []json.RawMessage) ([]byte, error) {
	codexModel := MapModelToCodex(model)

	var codexMessages []map[string]interface{}
	for _, raw := range messages {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		codexMsg := convertMessage(msg.Role, msg.Content)
		codexMessages = append(codexMessages, codexMsg...)
	}

	req := codexRequest{
		Model:        codexModel,
		Instructions: system,
		Messages:     codexMessages,
		Stream:       true,
	}

	return json.Marshal(req)
}

func convertMessage(role string, content json.RawMessage) []map[string]interface{} {
	// 尝试字符串
	var textStr string
	if json.Unmarshal(content, &textStr) == nil {
		r := role
		if r == "assistant" {
			r = "assistant"
		}
		return []map[string]interface{}{{"role": r, "content": textStr}}
	}

	// 数组
	var blocks []map[string]interface{}
	if json.Unmarshal(content, &blocks) != nil {
		return nil
	}

	var result []map[string]interface{}
	for _, block := range blocks {
		blockType, _ := block["type"].(string)
		switch blockType {
		case "text":
			text, _ := block["text"].(string)
			result = append(result, map[string]interface{}{"role": role, "content": text})
		case "tool_use":
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			input, _ := block["input"].(map[string]interface{})
			inputJSON, _ := json.Marshal(input)
			result = append(result, map[string]interface{}{
				"role": "assistant",
				"content": []map[string]interface{}{
					{"type": "function_call", "id": id, "name": name, "arguments": string(inputJSON)},
				},
			})
		case "tool_result":
			toolUseID, _ := block["tool_use_id"].(string)
			resultContent := ""
			if c, ok := block["content"].(string); ok {
				resultContent = c
			} else if arr, ok := block["content"].([]interface{}); ok {
				var parts []string
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						if text, ok := m["text"].(string); ok {
							parts = append(parts, text)
						}
					}
				}
				resultContent = strings.Join(parts, "\n")
			}
			result = append(result, map[string]interface{}{
				"role":         "tool",
				"content":      resultContent,
				"tool_call_id": toolUseID,
			})
		}
	}
	return result
}

// ── 模型映射 ──

var codexModelMapping = map[string]string{
	"claude-opus-4-6":            "gpt-5-codex",
	"claude-opus-4-5-20251101":   "gpt-5-codex",
	"claude-sonnet-4-20250514":   "gpt-5-codex-mini",
	"claude-sonnet-4-5-20250929": "gpt-5-codex",
	"claude-3-5-sonnet-20241022": "gpt-5-codex-mini",
	"claude-3-opus-20240229":     "gpt-5-codex",
	"gpt-4o":                     "gpt-5-codex-mini",
	"gpt-4-turbo":                "gpt-5-codex-mini",
}

func MapModelToCodex(model string) string {
	if model == "" {
		return "gpt-5-codex"
	}
	lower := strings.ToLower(strings.TrimSpace(model))
	if mapped, ok := codexModelMapping[lower]; ok {
		return mapped
	}
	// 已经是 codex 模型
	if strings.Contains(lower, "codex") || strings.HasPrefix(lower, "gpt-5") {
		return lower
	}
	if strings.Contains(lower, "opus") || strings.Contains(lower, "max") {
		return "gpt-5-codex"
	}
	if strings.Contains(lower, "sonnet") || strings.Contains(lower, "mini") {
		return "gpt-5-codex-mini"
	}
	return "gpt-5-codex"
}
