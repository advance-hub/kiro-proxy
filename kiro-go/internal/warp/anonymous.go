package warp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	anonymousGraphQLURL = "https://app.warp.dev/graphql/v2?op=CreateAnonymousUser"
	identityToolkitURL  = "https://identitytoolkit.googleapis.com/v1/accounts:signInWithCustomToken"
)

// CreateAnonymousUserResponse GraphQL 响应结构
type CreateAnonymousUserResponse struct {
	Data struct {
		CreateAnonymousUser struct {
			Typename          string `json:"__typename"`
			ExpiresAt         string `json:"expiresAt"`
			AnonymousUserType string `json:"anonymousUserType"`
			FirebaseUID       string `json:"firebaseUid"`
			IDToken           string `json:"idToken"`
			IsInviteValid     bool   `json:"isInviteValid"`
		} `json:"createAnonymousUser"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// SignInWithCustomTokenResponse Identity Toolkit 响应结构
type SignInWithCustomTokenResponse struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    string `json:"expiresIn"`
}

// createAnonymousUser 调用 Warp GraphQL API 创建匿名用户
func createAnonymousUser() (string, error) {
	query := `mutation CreateAnonymousUser($input: CreateAnonymousUserInput!, $requestContext: RequestContext!) {
  createAnonymousUser(input: $input, requestContext: $requestContext) {
    __typename
    ... on CreateAnonymousUserOutput {
      expiresAt
      anonymousUserType
      firebaseUid
      idToken
      isInviteValid
      responseContext { serverVersion }
    }
    ... on UserFacingError {
      error { __typename message }
      responseContext { serverVersion }
    }
  }
}`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"anonymousUserType": "NATIVE_CLIENT_ANONYMOUS_USER_FEATURE_GATED",
			"expirationType":    "NO_EXPIRATION",
			"referralCode":      nil,
		},
		"requestContext": map[string]interface{}{
			"clientContext": map[string]string{
				"version": warpVersion,
			},
			"osContext": map[string]interface{}{
				"category":           "macOS",
				"linuxKernelVersion": nil,
				"name":               "macOS",
				"version":            "15.7.2",
			},
		},
	}

	payload := map[string]interface{}{
		"query":         query,
		"variables":     variables,
		"operationName": "CreateAnonymousUser",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload failed: %w", err)
	}

	req, err := http.NewRequest("POST", anonymousGraphQLURL, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, br")
	req.Header.Set("x-warp-client-version", warpVersion)
	req.Header.Set("x-warp-os-category", "macOS")
	req.Header.Set("x-warp-os-name", "macOS")
	req.Header.Set("x-warp-os-version", "15.7.2")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("CreateAnonymousUser failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result CreateAnonymousUserResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response failed: %w", err)
	}

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	if result.Data.CreateAnonymousUser.IDToken == "" {
		return "", fmt.Errorf("CreateAnonymousUser did not return idToken")
	}

	return result.Data.CreateAnonymousUser.IDToken, nil
}

// exchangeIDTokenForRefreshToken 用 idToken 换取 refreshToken
func exchangeIDTokenForRefreshToken(idToken string) (string, error) {
	apiURL := fmt.Sprintf("%s?key=%s", identityToolkitURL, firebaseAPIKey)

	formData := url.Values{
		"returnSecureToken": {"true"},
		"token":             {idToken},
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Encoding", "gzip, br")
	req.Header.Set("x-warp-client-version", warpVersion)
	req.Header.Set("x-warp-os-category", "macOS")
	req.Header.Set("x-warp-os-name", "macOS")
	req.Header.Set("x-warp-os-version", "15.7.2")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("signInWithCustomToken failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result SignInWithCustomTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response failed: %w", err)
	}

	if result.RefreshToken == "" {
		return "", fmt.Errorf("signInWithCustomToken did not return refreshToken")
	}

	return result.RefreshToken, nil
}

// AcquireAnonymousAccessToken 获取匿名访问 token（完整流程）
func AcquireAnonymousAccessToken() (*WarpCredential, error) {
	// 1. 创建匿名用户，获取 idToken
	idToken, err := createAnonymousUser()
	if err != nil {
		return nil, fmt.Errorf("create anonymous user failed: %w", err)
	}

	// 2. 用 idToken 换取 refreshToken
	refreshToken, err := exchangeIDTokenForRefreshToken(idToken)
	if err != nil {
		return nil, fmt.Errorf("exchange id token failed: %w", err)
	}

	// 3. 用 refreshToken 刷新获取 accessToken
	accessToken, expiresIn, err := RefreshAccessToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh access token failed: %w", err)
	}

	// 4. 构建凭证对象
	cred := &WarpCredential{
		ID:           1,
		Name:         "anonymous",
		Email:        "anonymous@warp.dev",
		RefreshToken: refreshToken,
		AccessToken:  accessToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second).Unix(),
		AuthMode:     "firebase",
	}

	return cred, nil
}
