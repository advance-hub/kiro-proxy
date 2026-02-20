package common

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"kiro-go/internal/logger"
	"kiro-go/internal/model"
)

type contextKey string

const CredsContextKey contextKey = "kiro_credentials"
const ActCodeContextKey contextKey = "activation_code"
const RequestIDContextKey contextKey = "request_id"

// 激活码验证缓存
type actCodeCache struct {
	mu    sync.RWMutex
	cache map[string]actCodeCacheEntry
}

type actCodeCacheEntry struct {
	valid    bool
	message  string
	expireAt time.Time
}

var actCache = &actCodeCache{
	cache: make(map[string]actCodeCacheEntry),
}

func (c *actCodeCache) get(code string) (bool, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cache[code]
	if !ok || time.Now().After(entry.expireAt) {
		return false, "", false
	}
	return entry.valid, entry.message, true
}

func (c *actCodeCache) set(code string, valid bool, message string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[code] = actCodeCacheEntry{
		valid:    valid,
		message:  message,
		expireAt: time.Now().Add(ttl),
	}
}

// ExtractAPIKey 从请求中提取 API Key
func ExtractAPIKey(r *http.Request) string {
	if key := r.Header.Get("x-api-key"); key != "" {
		return key
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// GetCredsFromContext 从 context 中获取凭证
func GetCredsFromContext(r *http.Request) *model.KiroCredentials {
	if creds, ok := r.Context().Value(CredsContextKey).(*model.KiroCredentials); ok {
		return creds
	}
	return nil
}

// GetActCodeFromContext 从 context 中获取激活码
func GetActCodeFromContext(r *http.Request) string {
	if code, ok := r.Context().Value(ActCodeContextKey).(string); ok {
		return code
	}
	return ""
}

// GetRequestIDFromContext 从 context 中获取 request ID
func GetRequestIDFromContext(r *http.Request) string {
	if rid, ok := r.Context().Value(RequestIDContextKey).(string); ok {
		return rid
	}
	return ""
}

// WriteError 写入错误响应
func WriteError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"type":    errType,
			"message": message,
		},
	})
}

// WriteJSON 写入 JSON 响应
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// DecodeCredsKey 解码 creds- 前缀的 API Key
func DecodeCredsKey(key string) (*model.KiroCredentials, error) {
	encoded := strings.TrimPrefix(key, "creds-")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(encoded)
			if err != nil {
				return nil, fmt.Errorf("base64 decode failed")
			}
		}
	}
	var creds model.KiroCredentials
	if err := json.Unmarshal(decoded, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

func validateActivationCode(serverURL, code, machineId string) (bool, string) {
	if serverURL == "" {
		return true, ""
	}

	payload, _ := json.Marshal(map[string]string{
		"code":      code,
		"machineId": machineId,
	})

	// 使用 /api/code/validate 仅检查激活码有效性（不要求 machineId 和穿透权限）
	// 旧版使用 /api/tunnel/check 会因为 machineId 不匹配或无穿透权限而拒绝合法用户
	validateURL := serverURL + "/api/code/validate"

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(validateURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		// 验证服务不可用时，尝试回退到本地验证（如果 ActivationServerURL 指向自身）
		logger.WarnFields(logger.CatAuth, "激活码验证服务不可用，放行请求", logger.F{
			"error": err.Error(),
			"url":   validateURL,
		})
		return true, ""
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.WarnFields(logger.CatAuth, "激活码验证响应解析失败，放行请求", logger.F{
			"error":  err.Error(),
			"url":    validateURL,
			"status": resp.StatusCode,
		})
		return true, ""
	}

	if !result.Success {
		logger.WarnFields(logger.CatAuth, "激活码验证失败", logger.F{
			"code":    logger.MaskKey(code),
			"message": result.Message,
			"url":     validateURL,
		})
	}

	return result.Success, result.Message
}

// AuthMiddleware 认证中间件
type AuthMiddleware struct {
	Config                  *model.Config
	GetUserCreds            func(code string) *model.KiroCredentials
	GetUserCredsWithExpiry  func(code string) (*model.KiroCredentials, bool, string)
	GetUserCredsAutoRefresh func(code string) (*model.KiroCredentials, error)
	GetCodeExpiresDate      func(code string) (expiresDate string, expired bool) // 从 codes.json 获取过期日期
}

// Wrap 包装 handler
func (am *AuthMiddleware) Wrap(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := GenerateRequestID()
		ctx := context.WithValue(r.Context(), RequestIDContextKey, rid)
		r = r.WithContext(ctx)
		log := logger.NewContext(logger.CatAuth, rid, "")

		// 1. X-Kiro-Credentials header
		if h := r.Header.Get("x-kiro-credentials"); h != "" {
			var creds model.KiroCredentials
			if err := json.Unmarshal([]byte(h), &creds); err != nil {
				log.Warn("X-Kiro-Credentials 解析失败", logger.F{"error": err.Error()})
				WriteError(w, http.StatusUnauthorized, "authentication_error", "Invalid X-Kiro-Credentials")
				return
			}
			ctx := context.WithValue(r.Context(), CredsContextKey, &creds)
			handler(w, r.WithContext(ctx))
			return
		}

		// 2. 提取 API Key
		key := ExtractAPIKey(r)
		log.Info("收到请求", logger.F{
			"path":       r.URL.Path,
			"method":     r.Method,
			"api_key":    logger.MaskKey(key),
			"user_agent": r.Header.Get("User-Agent"),
		})
		if key == "" {
			log.Warn("缺少 API Key")
			WriteError(w, http.StatusUnauthorized, "authentication_error", "Missing API key")
			return
		}

		// 3. act- 激活码
		if strings.HasPrefix(key, "act-") {
			rawCode := strings.TrimPrefix(key, "act-")
			upperCode := strings.ToUpper(rawCode)
			log = logger.NewContext(logger.CatAuth, rid, logger.MaskKey(upperCode))
			log.Info("激活码认证", logger.F{"code": logger.MaskKey(upperCode)})

			// 3a. 调 app.js 验证激活码
			machineId := r.Header.Get("X-Machine-Id")
			if am.Config.ActivationServerURL != "" {
				cacheKey := upperCode + ":" + machineId
				if valid, msg, cached := actCache.get(cacheKey); cached {
					if !valid {
						log.Warn("激活码验证失败(缓存)", logger.F{"reason": msg})
						WriteError(w, http.StatusForbidden, "authentication_error", msg)
						return
					}
					log.Debug("激活码验证通过(缓存)")
				} else {
					log.Debug("调用 app.js 验证", logger.F{
						"server":     am.Config.ActivationServerURL,
						"machine_id": logger.MaskKey(machineId),
					})
					ok, msg := validateActivationCode(am.Config.ActivationServerURL, upperCode, machineId)
					ttl := 1 * time.Minute
					if ok {
						ttl = 5 * time.Minute
					}
					actCache.set(cacheKey, ok, msg, ttl)
					if !ok {
						log.Warn("激活码验证失败", logger.F{"reason": msg})
						WriteError(w, http.StatusForbidden, "authentication_error", msg)
						return
					}
					log.Info("激活码验证通过")
				}
			}

			// 3b. 先检查激活码本身的过期日期（从 codes.json）
			if am.GetCodeExpiresDate != nil {
				codeExpiresDate, codeExpired := am.GetCodeExpiresDate(upperCode)
				if codeExpired {
					logger.LogAuthResult(rid, logger.MaskKey(upperCode), "code_expired", logger.F{
						"code_expires_date": codeExpiresDate,
					})
					WriteError(w, http.StatusForbidden, "authentication_error",
						fmt.Sprintf("您的激活码已过期（过期日期：%s），请联系管理员续期", codeExpiresDate))
					return
				}
			}

			// 3c. 获取用户凭证（优先使用 auto-refresh）
			var creds *model.KiroCredentials
			var matchedKey string // 记录匹配到的 key 格式

			if am.GetUserCredsAutoRefresh != nil {
				// 使用 auto-refresh 方法，token 过期时自动后台刷新
				for _, code := range []string{upperCode, key, rawCode} {
					c, err := am.GetUserCredsAutoRefresh(code)
					if c != nil {
						creds = c
						matchedKey = code
						if err != nil {
							log.Warn("token 自动刷新失败，使用现有 token", logger.F{
								"error":       err.Error(),
								"matched_key": logger.MaskKey(matchedKey),
							})
						}
						break
					}
				}
			} else if am.GetUserCreds != nil {
				creds = am.GetUserCreds(upperCode)
				if creds != nil {
					matchedKey = upperCode
				}
				if creds == nil {
					creds = am.GetUserCreds(key)
					if creds != nil {
						matchedKey = key
					}
				}
				if creds == nil {
					creds = am.GetUserCreds(rawCode)
					if creds != nil {
						matchedKey = rawCode
					}
				}
			}

			if creds == nil {
				logger.LogAuthResult(rid, logger.MaskKey(upperCode), "no_creds_fallback_pool", logger.F{
					"tried_keys": fmt.Sprintf("[%s, %s, %s]",
						logger.MaskKey(upperCode), logger.MaskKey(key), logger.MaskKey(rawCode)),
				})
				ctx := context.WithValue(r.Context(), ActCodeContextKey, upperCode)
				handler(w, r.WithContext(ctx))
				return
			}

			logger.LogAuthResult(rid, logger.MaskKey(upperCode), "success", logger.F{
				"matched_key":   logger.MaskKey(matchedKey),
				"token_expires": creds.ExpiresAt,
				"has_token":     creds.AccessToken != "",
				"disabled":      creds.Disabled,
			})
			ctx := context.WithValue(r.Context(), CredsContextKey, creds)
			ctx = context.WithValue(ctx, ActCodeContextKey, upperCode)
			handler(w, r.WithContext(ctx))
			return
		}

		// 4. creds- base64 凭证
		if strings.HasPrefix(key, "creds-") {
			creds, err := DecodeCredsKey(key)
			if err != nil {
				log.Warn("creds key 解码失败", logger.F{"error": err.Error()})
				WriteError(w, http.StatusUnauthorized, "authentication_error", "Invalid creds key: "+err.Error())
				return
			}
			ctx := context.WithValue(r.Context(), CredsContextKey, creds)
			handler(w, r.WithContext(ctx))
			return
		}

		// 5. 普通 API Key
		if key != am.Config.APIKey {
			log.Warn("API Key 无效")
			WriteError(w, http.StatusUnauthorized, "authentication_error", "Invalid API key")
			return
		}
		handler(w, r)
	}
}
