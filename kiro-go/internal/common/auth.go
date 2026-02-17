package common

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"kiro-go/internal/model"
)

type contextKey string

const CredsContextKey contextKey = "kiro_credentials"
const ActCodeContextKey contextKey = "activation_code"

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

// maskKey 遮蔽 API Key 中间部分用于日志
func maskKey(key string) string {
	if key == "" {
		return "<empty>"
	}
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
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

// validateActivationCode 调用 app.js 激活码验证服务
// code: 原始激活码（XXXX-XXXX-XXXX-XXXX 格式，不含 act- 前缀）
// machineId: 客户端机器码（从 X-Machine-Id header 获取）
func validateActivationCode(serverURL, code, machineId string) (bool, string) {
	if serverURL == "" {
		return true, "" // 未配置激活码服务器，跳过验证
	}

	payload, _ := json.Marshal(map[string]string{
		"code":      code,
		"machineId": machineId,
	})

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(serverURL+"/api/tunnel/check", "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("激活码验证服务不可用: %v，放行请求", err)
		return true, "" // 验证服务不可用时放行（降级策略）
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("激活码验证响应解析失败: %v，放行请求", err)
		return true, ""
	}

	return result.Success, result.Message
}

// AuthMiddleware 认证中间件
type AuthMiddleware struct {
	Config       *model.Config
	GetUserCreds func(code string) *model.KiroCredentials
}

// Wrap 包装 handler
func (am *AuthMiddleware) Wrap(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. X-Kiro-Credentials header（本地模式直传凭证）
		if h := r.Header.Get("x-kiro-credentials"); h != "" {
			var creds model.KiroCredentials
			if err := json.Unmarshal([]byte(h), &creds); err != nil {
				WriteError(w, http.StatusUnauthorized, "authentication_error", "Invalid X-Kiro-Credentials")
				return
			}
			ctx := context.WithValue(r.Context(), CredsContextKey, &creds)
			handler(w, r.WithContext(ctx))
			return
		}

		// 2. 提取 API Key
		key := ExtractAPIKey(r)
		log.Printf("[AUTH] 请求路径: %s | API Key: %s | User-Agent: %s", r.URL.Path, maskKey(key), r.Header.Get("User-Agent"))
		if key == "" {
			log.Printf("[AUTH] ❌ 缺少 API Key")
			WriteError(w, http.StatusUnauthorized, "authentication_error", "Missing API key")
			return
		}

		// 3. act- 激活码（与 app.js 卡密系统集成）
		// 格式: act-XXXX-XXXX-XXXX-XXXX
		// 流程: 提取卡密 → 调 app.js 验证（有效性+穿透权限+过期检查）→ 查 user_credentials.json 获取 Kiro 凭证
		if strings.HasPrefix(key, "act-") {
			rawCode := strings.TrimPrefix(key, "act-")
			upperCode := strings.ToUpper(rawCode)
			log.Printf("[AUTH] 🔑 激活码模式: %s", maskKey(key))

			// 3a. 调 app.js 验证激活码（如果配置了 activationServerUrl）
			machineId := r.Header.Get("X-Machine-Id")
			if am.Config.ActivationServerURL != "" {
				// 先查缓存
				cacheKey := upperCode + ":" + machineId
				if valid, msg, cached := actCache.get(cacheKey); cached {
					if !valid {
						log.Printf("[AUTH] ❌ 激活码验证失败 (缓存): %s - %s", maskKey(upperCode), msg)
						WriteError(w, http.StatusForbidden, "authentication_error", msg)
						return
					}
					log.Printf("[AUTH] ✅ app.js 验证通过 (缓存)")
				} else {
					// 缓存未命中，调用验证
					log.Printf("[AUTH] 调用 app.js 验证: %s (machineId: %s)", am.Config.ActivationServerURL, maskKey(machineId))
					ok, msg := validateActivationCode(am.Config.ActivationServerURL, upperCode, machineId)
					// 缓存结果（成功缓存 5 分钟，失败缓存 1 分钟）
					ttl := 1 * time.Minute
					if ok {
						ttl = 5 * time.Minute
					}
					actCache.set(cacheKey, ok, msg, ttl)
					if !ok {
						log.Printf("[AUTH] ❌ 激活码验证失败: %s - %s", maskKey(upperCode), msg)
						WriteError(w, http.StatusForbidden, "authentication_error", msg)
						return
					}
					log.Printf("[AUTH] ✅ app.js 验证通过")
				}
			}

			// 3b. 查 user_credentials.json 获取 Kiro 凭证
			creds := am.GetUserCreds(key)
			if creds == nil {
				// 激活码在 app.js 验证通过，但 kiro-go 没有对应凭证
				// 回退到主凭证池（让所有已验证的激活码用户都能用）
				log.Printf("[AUTH] ⚠️  激活码 %s 无独立凭证，使用主凭证池", maskKey(key))
				ctx := context.WithValue(r.Context(), ActCodeContextKey, key)
				handler(w, r.WithContext(ctx))
				return
			}
			log.Printf("[AUTH] ✅ 使用激活码独立凭证")
			ctx := context.WithValue(r.Context(), CredsContextKey, creds)
			ctx = context.WithValue(ctx, ActCodeContextKey, key)
			handler(w, r.WithContext(ctx))
			return
		}

		// 4. creds- base64 凭证
		if strings.HasPrefix(key, "creds-") {
			creds, err := DecodeCredsKey(key)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "authentication_error", "Invalid creds key: "+err.Error())
				return
			}
			ctx := context.WithValue(r.Context(), CredsContextKey, creds)
			handler(w, r.WithContext(ctx))
			return
		}

		// 5. 普通 API Key（使用主凭证池）
		if key != am.Config.APIKey {
			WriteError(w, http.StatusUnauthorized, "authentication_error", "Invalid API key")
			return
		}
		handler(w, r)
	}
}
