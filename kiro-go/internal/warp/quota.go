package warp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// QuotaInfo Warp 账号配额信息
type QuotaInfo struct {
	RequestLimit    int    `json:"requestLimit"`
	RequestsUsed    int    `json:"requestsUsed"`
	Remaining       int    `json:"remaining"`
	IsUnlimited     bool   `json:"isUnlimited"`
	NextRefreshTime string `json:"nextRefreshTime,omitempty"`
	RefreshDuration string `json:"refreshDuration,omitempty"`
}

// GetRequestLimit 查询 Warp 账号配额
func GetRequestLimit(accessToken string) (*QuotaInfo, error) {
	query := `query GetRequestLimitInfo($requestContext: RequestContext!) {
  user(requestContext: $requestContext) {
    __typename
    ... on UserOutput {
      user {
        requestLimitInfo {
          isUnlimited
          nextRefreshTime
          requestLimit
          requestsUsedSinceLastRefresh
          requestLimitRefreshDuration
        }
      }
    }
    ... on UserFacingError {
      error {
        __typename
        message
      }
    }
  }
}`

	payload := map[string]interface{}{
		"operationName": "GetRequestLimitInfo",
		"variables": map[string]interface{}{
			"requestContext": map[string]interface{}{
				"clientContext": map[string]interface{}{
					"version": warpVersion,
				},
				"osContext": map[string]interface{}{
					"category": "macOS",
					"name":     "macOS",
					"version":  "15.7.2",
				},
			},
		},
		"query": query,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://app.warp.dev/graphql/v2?op=GetRequestLimitInfo", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("x-warp-client-id", "warp-app")
	req.Header.Set("x-warp-client-version", warpVersion)
	req.Header.Set("x-warp-os-category", "macOS")
	req.Header.Set("x-warp-os-name", "macOS")
	req.Header.Set("x-warp-os-version", "15.7.2")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("配额查询请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("配额查询失败 HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data struct {
			User struct {
				TypeName string `json:"__typename"`
				User     struct {
					RequestLimitInfo struct {
						IsUnlimited                  bool   `json:"isUnlimited"`
						NextRefreshTime              string `json:"nextRefreshTime"`
						RequestLimit                 int    `json:"requestLimit"`
						RequestsUsedSinceLastRefresh int    `json:"requestsUsedSinceLastRefresh"`
						RequestLimitRefreshDuration  string `json:"requestLimitRefreshDuration"`
					} `json:"requestLimitInfo"`
				} `json:"user"`
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"user"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析配额响应失败: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL 错误: %s", result.Errors[0].Message)
	}

	if result.Data.User.TypeName == "UserFacingError" {
		return nil, fmt.Errorf("用户错误: %s", result.Data.User.Error.Message)
	}

	info := result.Data.User.User.RequestLimitInfo
	return &QuotaInfo{
		RequestLimit:    info.RequestLimit,
		RequestsUsed:    info.RequestsUsedSinceLastRefresh,
		Remaining:       info.RequestLimit - info.RequestsUsedSinceLastRefresh,
		IsUnlimited:     info.IsUnlimited,
		NextRefreshTime: info.NextRefreshTime,
		RefreshDuration: info.RequestLimitRefreshDuration,
	}, nil
}

// GetRequestLimitByApiKey 使用 apikey (wk-xxx) 查询配额
// apikey 模式使用不同的 GraphQL 查询（currentUser 而非 user(requestContext)）
func GetRequestLimitByApiKey(apiKey string) (*QuotaInfo, error) {
	// 先尝试 currentUser 查询（更简单）
	query := `query GetRequestLimitInfo {
  currentUser {
    requestLimitInfo {
      isUnlimited
      nextRefreshTime
      requestLimit
      requestsUsedSinceLastRefresh
      requestLimitRefreshDuration
    }
  }
}`

	payload := map[string]interface{}{
		"operationName": "GetRequestLimitInfo",
		"variables":     map[string]interface{}{},
		"query":         query,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://app.warp.dev/graphql/v2", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("x-warp-client-version", warpVersion)
	req.Header.Set("x-warp-os-category", "desktop")
	req.Header.Set("x-warp-os-name", "macOS")
	req.Header.Set("x-warp-os-version", "14")
	req.Header.Set("User-Agent", "Warp/"+warpVersion)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("配额查询请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		// 回退到带 requestContext 的查询
		return GetRequestLimit(apiKey)
	}

	var result struct {
		Data struct {
			CurrentUser struct {
				RequestLimitInfo struct {
					IsUnlimited                  bool   `json:"isUnlimited"`
					NextRefreshTime              string `json:"nextRefreshTime"`
					RequestLimit                 int    `json:"requestLimit"`
					RequestsUsedSinceLastRefresh int    `json:"requestsUsedSinceLastRefresh"`
					RequestLimitRefreshDuration  string `json:"requestLimitRefreshDuration"`
				} `json:"requestLimitInfo"`
			} `json:"currentUser"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析配额响应失败: %w", err)
	}

	if len(result.Errors) > 0 {
		// 回退到带 requestContext 的查询
		return GetRequestLimit(apiKey)
	}

	info := result.Data.CurrentUser.RequestLimitInfo
	return &QuotaInfo{
		RequestLimit:    info.RequestLimit,
		RequestsUsed:    info.RequestsUsedSinceLastRefresh,
		Remaining:       info.RequestLimit - info.RequestsUsedSinceLastRefresh,
		IsUnlimited:     info.IsUnlimited,
		NextRefreshTime: info.NextRefreshTime,
		RefreshDuration: info.RequestLimitRefreshDuration,
	}, nil
}

// GetAllQuotas 查询所有活跃账号的配额
func GetAllQuotas(store *WarpCredentialStore) []map[string]interface{} {
	creds := store.GetAllActive()
	var results []map[string]interface{}

	for _, c := range creds {
		entry := map[string]interface{}{
			"id":       c.ID,
			"name":     c.Name,
			"email":    c.Email,
			"authMode": c.AuthMode,
		}

		// apikey 模式使用专用查询
		if c.ApiKey != "" {
			quota, err := GetRequestLimitByApiKey(c.ApiKey)
			if err != nil {
				entry["error"] = err.Error()
			} else {
				entry["quota"] = quota
			}
			results = append(results, entry)
			continue
		}

		// firebase 模式
		accessToken, err := GetValidAccessToken(c)
		if err != nil {
			entry["error"] = err.Error()
			results = append(results, entry)
			continue
		}

		quota, err := GetRequestLimit(accessToken)
		if err != nil {
			entry["error"] = err.Error()
			results = append(results, entry)
			continue
		}

		entry["quota"] = quota
		results = append(results, entry)
	}

	return results
}
