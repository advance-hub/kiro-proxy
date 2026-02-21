package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kiro-go/internal/anthropic"
	"kiro-go/internal/common"
	"kiro-go/internal/kiro"
	"kiro-go/internal/logger"
	"kiro-go/internal/model"
	"kiro-go/internal/openai"
)

func main() {
	// 智能默认路径：macOS 用 ~/.kiro-proxy/，Linux 用 /opt/kiro-proxy/
	defaultDir := "/opt/kiro-proxy"
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		defaultDir = filepath.Join(home, ".kiro-proxy")
	}

	configPath := flag.String("config", filepath.Join(defaultDir, "config.json"), "配置文件路径")
	credsPath := flag.String("credentials", filepath.Join(defaultDir, "credentials.json"), "凭证文件路径")
	flag.Parse()

	// 加载配置，传入配置文件目录作为数据文件基准路径
	cfg := loadConfig(*configPath)
	configDir := filepath.Dir(*configPath)
	cfg.DefaultsWithDir(configDir)

	// 启用文件日志：每个 category 写入独立文件 logs/auth-YYYY-MM-DD.log
	logDir := filepath.Join(configDir, "logs")
	logger.SetLogDir(logDir)
	logger.Infof(logger.CatSystem, "文件日志已启用: %s", logDir)

	logger.InfoFields(logger.CatSystem, "配置已加载", logger.F{
		"config_dir":      configDir,
		"user_creds_path": cfg.UserCredentialsPath,
		"codes_path":      cfg.CodesPath,
	})

	// 加载凭证
	credsList := loadCredentials(*credsPath)
	logger.Infof(logger.CatSystem, "已加载 %d 个凭据配置", len(credsList))

	// 创建核心组件
	tokenMgr := kiro.NewTokenManager(cfg, credsList)
	userCredsMgr := kiro.NewUserCredentialsManager(cfg.UserCredentialsPath)
	userCredsMgr.SetConfig(cfg)
	userCredsMgr.StartAutoRefresh(cfg, 5*60*1000000000) // 5分钟检查一次
	codesMgr := kiro.NewCodesManager(cfg.CodesPath)
	provider := kiro.NewProvider(cfg, tokenMgr)
	provider.UserCredsMgr = userCredsMgr

	logger.Infof(logger.CatSystem, "多用户模式已启用，当前用户数: %d", userCredsMgr.Count())
	logger.Infof(logger.CatSystem, "卡密管理已启用，当前卡密数: %d", len(codesMgr.GetAll()))

	// 认证中间件（使用 CodesManager + 自动刷新）
	authMw := &common.AuthMiddleware{
		Config: cfg,
		GetUserCredsAutoRefresh: func(code string) (*model.KiroCredentials, error) {
			// 去掉 act- 前缀（如果有）
			normalizedCode := strings.TrimPrefix(strings.ToUpper(code), "ACT-")

			// 先从 user_credentials.json 获取并自动刷新
			creds, err := userCredsMgr.GetCredentialsAutoRefresh(normalizedCode)
			if creds != nil && err == nil {
				// 刷新成功，同步到 codes.json
				entry := codesMgr.FindByCode(normalizedCode)
				if entry != nil && entry.Credentials != nil {
					// 只有当 codes.json 中已有凭证时才更新（避免覆盖空数据）
					if entry.Credentials.AccessToken != creds.AccessToken ||
						entry.Credentials.RefreshToken != creds.RefreshToken {
						// token 已更新，同步到 codes.json
						codesMgr.SetCredentials(normalizedCode, entry.UserName, creds)
					}
				}
			}

			// 如果 user_credentials.json 中没有，尝试从 codes.json 获取
			if creds == nil {
				creds = codesMgr.GetCredentials(normalizedCode)
			}

			return creds, err
		},
		GetCodeExpiresDate: func(code string) (string, bool) {
			// 去掉 act- 前缀（如果有）
			normalizedCode := strings.TrimPrefix(strings.ToUpper(code), "ACT-")
			entry := codesMgr.FindByCode(normalizedCode)
			if entry == nil || entry.ExpiresDate == "" {
				return "", false
			}
			expiresDate := entry.ExpiresDate
			expired := false
			if t, err := time.Parse("2006-01-02", expiresDate); err == nil {
				expired = time.Now().After(t)
			}
			return expiresDate, expired
		},
	}

	// 直连 Anthropic provider（有 apiKey 就初始化，不再要求 backend==anthropic）
	var directProvider *anthropic.DirectProvider
	if cfg.AnthropicAPIKey != "" || len(cfg.AnthropicAPIKeys) > 0 {
		directProvider = anthropic.NewDirectProvider(cfg)
		logger.Infof(logger.CatSystem, "Anthropic 直连已启用 (%s)", cfg.AnthropicBaseURL)
	}

	// 路由
	mux := http.NewServeMux()

	// ==================== 统一 API 端点（只走 Kiro）====================

	// GET /v1/models - Kiro 模型列表
	mux.HandleFunc("/v1/models", authMw.Wrap(func(w http.ResponseWriter, r *http.Request) {
		anthropic.HandleGetModels(w, r, provider)
	}))

	// POST /v1/messages - Kiro Anthropic 格式
	mux.HandleFunc("/v1/messages", authMw.Wrap(func(w http.ResponseWriter, r *http.Request) {
		anthropic.HandlePostMessages(w, r, provider)
	}))

	// POST /v1/chat/completions - Kiro OpenAI 格式
	mux.HandleFunc("/v1/chat/completions", authMw.Wrap(func(w http.ResponseWriter, r *http.Request) {
		openai.HandleChatCompletions(w, r, provider)
	}))

	// ==================== /anthropic/v1/ - Anthropic 直连后端 ====================
	mux.HandleFunc("/anthropic/v1/models", authMw.Wrap(func(w http.ResponseWriter, r *http.Request) {
		anthropic.HandleGetModels(w, r, provider)
	}))
	mux.HandleFunc("/anthropic/v1/messages", authMw.Wrap(func(w http.ResponseWriter, r *http.Request) {
		if directProvider != nil {
			anthropic.HandlePostMessagesDirect(w, r, directProvider)
		} else {
			common.WriteError(w, http.StatusServiceUnavailable, "api_error", "Anthropic 直连未配置 (需要 anthropicApiKey)")
		}
	}))
	mux.HandleFunc("/anthropic/v1/chat/completions", authMw.Wrap(func(w http.ResponseWriter, r *http.Request) {
		if directProvider != nil {
			openai.HandleChatCompletionsDirect(w, r, directProvider)
		} else {
			common.WriteError(w, http.StatusServiceUnavailable, "api_error", "Anthropic 直连未配置 (需要 anthropicApiKey)")
		}
	}))

	// 用户凭证管理 API
	mux.HandleFunc("/api/admin/user-credentials", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListUserCredentials(w, userCredsMgr)
		case http.MethodPost:
			handleAddUserCredential(w, r, codesMgr) // 使用 CodesManager
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/admin/user-credentials/stats", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{"total_users": userCredsMgr.Count()})
	})
	// 批量设置有效期（改为操作 codes.json）
	mux.HandleFunc("/api/admin/user-credentials/batch-set-expiry", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ActivationCodes []string `json:"activation_codes"`
			Days            int      `json:"days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
			return
		}
		if len(req.ActivationCodes) == 0 {
			common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "activation_codes is required"})
			return
		}
		count, err := codesMgr.BatchSetExpiresDate(req.ActivationCodes, req.Days)
		if err != nil {
			common.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		expiresDate := ""
		if req.Days == 0 {
			expiresDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		} else if req.Days > 0 {
			expiresDate = time.Now().AddDate(0, 0, req.Days).Format("2006-01-02")
		}
		logger.InfoFields(logger.CatAdmin, "批量设置激活码有效期", logger.F{
			"count":        count,
			"days":         req.Days,
			"expires_date": expiresDate,
		})
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":      true,
			"message":      fmt.Sprintf("已更新 %d 个激活码的有效期", count),
			"updated":      count,
			"expires_date": expiresDate,
		})
	})
	mux.HandleFunc("/api/admin/user-credentials/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/api/admin/user-credentials/")
		if code == "" || code == "stats" {
			return
		}
		// 处理 /set-expiry 子路径（改为操作 codes.json）
		if strings.HasSuffix(code, "/set-expiry") {
			code = strings.TrimSuffix(code, "/set-expiry")
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Days int `json:"days"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
				return
			}
			if err := codesMgr.SetExpiresDate(code, req.Days); err != nil {
				common.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			entry := codesMgr.FindByCode(code)
			msg := fmt.Sprintf("激活码有效期已设置: %s", code)
			expiresDate := ""
			if entry != nil && entry.ExpiresDate != "" {
				expiresDate = entry.ExpiresDate
				msg = fmt.Sprintf("激活码有效期已设置: %s (过期日期: %s)", code, entry.ExpiresDate)
			}
			logger.InfoFields(logger.CatAdmin, "激活码有效期已设置", logger.F{
				"code":         code,
				"expires_date": expiresDate,
			})
			common.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"success": true, "message": msg, "expires_date": expiresDate,
			})
			return
		}
		switch r.Method {
		case http.MethodGet:
			creds, expired, expiresDate := userCredsMgr.GetCredentialsWithExpiry(code)
			if creds == nil {
				common.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "激活码未找到"})
				return
			}
			common.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"activation_code": code, "has_credentials": true, "expires_at": creds.ExpiresAt,
				"expires_date": expiresDate, "expired": expired,
			})
		case http.MethodDelete:
			if err := userCredsMgr.Remove(code); err != nil {
				common.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			common.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "凭证已删除: " + code})
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// 凭据热加载 API（kiro-launcher 切换账号时调用）
	mux.HandleFunc("/api/admin/reload-credentials", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// 重新加载凭据文件
		newCreds := loadCredentials(*credsPath)
		provider.ReloadCredentials(newCreds)
		logger.Infof(logger.CatCreds, "凭据已热加载，共 %d 个", len(newCreds))
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true, "message": fmt.Sprintf("凭据已重新加载，共 %d 个", len(newCreds)),
		})
	})

	// ==================== 卡密管理 API ====================
	// 激活码激活
	mux.HandleFunc("/api/activate", func(w http.ResponseWriter, r *http.Request) {
		kiro.HandleActivate(w, r, codesMgr)
	})
	// 激活码验证（API 访问鉴权，不检查 machineId 和穿透权限）
	mux.HandleFunc("/api/code/validate", func(w http.ResponseWriter, r *http.Request) {
		kiro.HandleCodeValidate(w, r, codesMgr)
	})
	// 穿透权限检查（需要 machineId + tunnelDays）
	mux.HandleFunc("/api/tunnel/check", func(w http.ResponseWriter, r *http.Request) {
		kiro.HandleTunnelCheck(w, r, codesMgr)
	})
	// 卡密列表（简单 HTML）
	mux.HandleFunc("/api/codeslist", func(w http.ResponseWriter, r *http.Request) {
		kiro.HandleCodesList(w, r, codesMgr)
	})
	// 管理后台页面
	mux.HandleFunc("/admin", kiro.HandleAdminPage)
	// 管理 API
	mux.HandleFunc("/api/admin/codes", func(w http.ResponseWriter, r *http.Request) {
		kiro.HandleAdminCodesRouter(w, r, codesMgr)
	})
	mux.HandleFunc("/api/admin/codes/delete", func(w http.ResponseWriter, r *http.Request) {
		kiro.HandleAdminDeleteCodes(w, r, codesMgr)
	})
	mux.HandleFunc("/api/admin/codes/update", func(w http.ResponseWriter, r *http.Request) {
		kiro.HandleAdminUpdateCodes(w, r, codesMgr)
	})
	mux.HandleFunc("/api/admin/codes/reset", func(w http.ResponseWriter, r *http.Request) {
		kiro.HandleAdminResetCodes(w, r, codesMgr)
	})
	mux.HandleFunc("/api/admin/codes/export", func(w http.ResponseWriter, r *http.Request) {
		kiro.HandleAdminExportCodes(w, r, codesMgr)
	})

	// CORS
	handler := corsMiddleware(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	logger.InfoFields(logger.CatSystem, "启动服务器", logger.F{
		"addr":    addr,
		"api_key": logger.MaskKey(cfg.APIKey),
		"backend": cfg.Backend,
	})
	logger.Infof(logger.CatSystem, "路由: /v1/ (Kiro) | /anthropic/v1/ (直连) | /admin")

	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Fatalf(logger.CatSystem, "服务器启动失败: %v", err)
	}
}

func loadConfig(path string) *model.Config {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Fatalf(logger.CatSystem, "加载配置失败: %v", err)
	}
	var cfg model.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		logger.Fatalf(logger.CatSystem, "解析配置失败: %v", err)
	}
	return &cfg
}

func loadCredentials(path string) []*model.KiroCredentials {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Warnf(logger.CatCreds, "加载凭证失败: %v，使用空凭证列表", err)
		return nil
	}
	var list []*model.KiroCredentials
	if json.Unmarshal(data, &list) == nil {
		return list
	}
	var single model.KiroCredentials
	if json.Unmarshal(data, &single) == nil {
		return []*model.KiroCredentials{&single}
	}
	logger.Warnf(logger.CatCreds, "凭证文件格式无效: %s", path)
	return nil
}

func handleListUserCredentials(w http.ResponseWriter, ucm *kiro.UserCredentialsManager) {
	entries := ucm.ListAll()
	var result []map[string]interface{}
	for _, e := range entries {
		result = append(result, map[string]interface{}{
			"activation_code": e.ActivationCode, "user_name": e.UserName,
			"has_credentials": true, "expires_at": e.Credentials.ExpiresAt,
			"expires_date": e.ExpiresDate,
			"created_at":   e.CreatedAt, "updated_at": e.UpdatedAt,
		})
	}
	if result == nil {
		result = []map[string]interface{}{}
	}
	common.WriteJSON(w, http.StatusOK, result)
}

func handleAddUserCredential(w http.ResponseWriter, r *http.Request, cm *kiro.CodesManager) {
	var payload struct {
		ActivationCode string                `json:"activation_code"`
		UserName       string                `json:"user_name"`
		Credentials    model.KiroCredentials `json:"credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON: " + err.Error()})
		return
	}
	if payload.ActivationCode == "" {
		common.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "activation_code is required"})
		return
	}
	// 保存到 codes.json
	if err := cm.SetCredentials(payload.ActivationCode, payload.UserName, &payload.Credentials); err != nil {
		common.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	logger.InfoFields(logger.CatAdmin, "用户凭证已保存到 codes.json", logger.F{
		"code":      logger.MaskKey(payload.ActivationCode),
		"user_name": payload.UserName,
	})
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true, "message": fmt.Sprintf("凭证已保存: %s", payload.ActivationCode),
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key, anthropic-version, x-kiro-credentials")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
