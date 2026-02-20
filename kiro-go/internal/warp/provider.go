package warp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	warpHost    = "app.warp.dev"
	warpPath    = "/ai/multi-agent"
	warpVersion = "v0.2026.02.11.08.23.stable_02"
)

// Provider Warp API provider
type Provider struct {
	Store  *WarpCredentialStore
	Client *http.Client
}

// NewProvider 创建 Warp provider
func NewProvider(credStore *WarpCredentialStore) *Provider {
	// 暂时使用标准 HTTP 客户端，Warp JA3 检测问题待后续解决
	return &Provider{
		Store: credStore,
		Client: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (p *Provider) buildHeaders(accessToken string, bodyLen int) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/x-protobuf")
	h.Set("Accept", "text/event-stream")
	h.Set("Accept-Encoding", "identity")
	h.Set("Authorization", "Bearer "+accessToken)
	h.Set("User-Agent", "Warp/"+warpVersion+" (darwin)")
	h.Set("x-warp-client-id", "warp-app")
	h.Set("x-warp-client-version", warpVersion)
	h.Set("x-warp-os-category", "macOS")
	h.Set("x-warp-os-name", "macOS")
	h.Set("x-warp-os-version", "15.7.2")
	h.Set("Content-Length", fmt.Sprintf("%d", bodyLen))
	return h
}

// SendRequest 发送请求到 Warp API
func (p *Provider) SendRequest(body []byte, accessToken string) (*http.Response, error) {
	url := fmt.Sprintf("https://%s%s", warpHost, warpPath)
	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header = p.buildHeaders(accessToken, len(body))
	return p.Client.Do(req)
}

// SendWithRetry 带重试和故障转移的请求
func (p *Provider) SendWithRetry(body []byte, maxRetries int) (*http.Response, *WarpCredential, error) {
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
			return nil, nil, fmt.Errorf("没有可用的 Warp 账号")
		}
		tried[cred.ID] = true

		accessToken, err := GetValidAccessToken(cred)
		if err != nil {
			log.Printf("[Warp] token 刷新失败 #%d: %v", cred.ID, err)
			p.Store.MarkError(cred.ID, err.Error())
			lastErr = err
			continue
		}

		log.Printf("[Warp] 尝试 %d/%d 使用凭证 #%d (%s)", attempt+1, maxRetries, cred.ID, cred.Name)

		resp, err := p.SendRequest(body, accessToken)
		if err != nil {
			log.Printf("[Warp] 请求失败 #%d: %v", cred.ID, err)
			p.Store.MarkError(cred.ID, err.Error())
			lastErr = err
			continue
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			log.Printf("[Warp] 429 配额耗尽 #%d, 切换下一个", cred.ID)
			p.Store.MarkDisabled(cred.ID)
			lastErr = fmt.Errorf("HTTP 429: 配额耗尽")
			time.Sleep(time.Second)
			continue
		}

		if resp.StatusCode != 200 {
			errBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(errBody))
			log.Printf("[Warp] 请求错误 #%d: %s", cred.ID, errMsg)
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

func (p *Provider) getNextAvailable(tried map[int]bool) *WarpCredential {
	active := p.Store.GetAllActive()
	for _, c := range active {
		if !tried[c.ID] {
			return c
		}
	}
	return nil
}

// GetModels 从 Warp GraphQL API 获取模型列表
func (p *Provider) GetModels() ([]map[string]interface{}, error) {
	// 获取一个可用的凭证来调用 GraphQL API
	creds := p.Store.GetAllActive()
	if len(creds) == 0 {
		return nil, fmt.Errorf("没有可用的 Warp 凭证")
	}

	cred := creds[0]
	accessToken, err := GetValidAccessToken(cred)
	if err != nil {
		return nil, fmt.Errorf("获取 access token 失败: %w", err)
	}

	// GraphQL 查询
	query := `query GetWorkspacesMetadataForUser($requestContext: RequestContext!) {
		user(requestContext: $requestContext) {
			__typename
			... on UserOutput {
				user {
					workspaces {
						featureModelChoice {
							agentMode {
								choices {
									displayName
									baseModelName
									id
									provider
								}
							}
						}
					}
				}
			}
		}
	}`

	payload := map[string]interface{}{
		"query": query,
		"variables": map[string]interface{}{
			"requestContext": map[string]interface{}{
				"clientContext": map[string]string{"version": warpVersion},
				"osContext": map[string]interface{}{
					"category":           "macOS",
					"linuxKernelVersion": nil,
					"name":               "macOS",
					"version":            "15.6.1",
				},
			},
		},
		"operationName": "GetWorkspacesMetadataForUser",
	}

	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://app.warp.dev/graphql/v2?op=GetWorkspacesMetadataForUser", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("x-warp-client-id", "warp-app")
	req.Header.Set("x-warp-client-version", warpVersion)
	req.Header.Set("x-warp-os-category", "macOS")
	req.Header.Set("x-warp-os-name", "macOS")
	req.Header.Set("x-warp-os-version", "15.6.1")

	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GraphQL 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GraphQL 返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			User struct {
				User struct {
					Workspaces []struct {
						FeatureModelChoice struct {
							AgentMode struct {
								Choices []struct {
									ID          string `json:"id"`
									DisplayName string `json:"displayName"`
									Provider    string `json:"provider"`
								} `json:"choices"`
							} `json:"agentMode"`
						} `json:"featureModelChoice"`
					} `json:"workspaces"`
				} `json:"user"`
			} `json:"user"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 GraphQL 响应失败: %w", err)
	}

	// 转换为标准格式
	models := []map[string]interface{}{}
	if len(result.Data.User.User.Workspaces) > 0 {
		for _, choice := range result.Data.User.User.Workspaces[0].FeatureModelChoice.AgentMode.Choices {
			models = append(models, map[string]interface{}{
				"id":           choice.ID,
				"object":       "model",
				"owned_by":     strings.ToLower(choice.Provider),
				"display_name": choice.DisplayName,
				"type":         "chat",
				"max_tokens":   32000,
			})
		}
	}

	return models, nil
}

// ReadSSEResponse 读取 SSE 响应并解析为 WarpEvent
func ReadSSEResponse(resp *http.Response, onEvent func([]WarpEvent)) {
	buf := make([]byte, 32*1024)
	var lineBuf strings.Builder

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			lineBuf.Write(buf[:n])
			text := lineBuf.String()
			lines := strings.Split(text, "\n")
			// 保留最后一个不完整的行
			lineBuf.Reset()
			lineBuf.WriteString(lines[len(lines)-1])

			for _, line := range lines[:len(lines)-1] {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "event:") {
					continue
				}
				events := ParseWarpSSELine(line)
				if len(events) > 0 {
					onEvent(events)
				}
			}
		}
		if err != nil {
			break
		}
	}

	// 处理剩余数据
	remaining := strings.TrimSpace(lineBuf.String())
	if remaining != "" {
		events := ParseWarpSSELine(remaining)
		if len(events) > 0 {
			onEvent(events)
		}
	}
}
