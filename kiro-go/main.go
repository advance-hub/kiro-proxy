package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"kiro-go/internal/anthropic"
	"kiro-go/internal/common"
	"kiro-go/internal/kiro"
	"kiro-go/internal/model"
	"kiro-go/internal/openai"
)

func main() {
	configPath := flag.String("config", "/opt/kiro-proxy/config.json", "配置文件路径")
	credsPath := flag.String("credentials", "/opt/kiro-proxy/credentials.json", "凭证文件路径")
	flag.Parse()

	// 加载配置
	cfg := loadConfig(*configPath)
	cfg.Defaults()

	// 加载凭证
	credsList := loadCredentials(*credsPath)
	log.Printf("已加载 %d 个凭据配置", len(credsList))

	// 创建核心组件
	tokenMgr := kiro.NewTokenManager(cfg, credsList)
	userCredsMgr := kiro.NewUserCredentialsManager(cfg.UserCredentialsPath)
	provider := kiro.NewProvider(cfg, tokenMgr)
	provider.UserCredsMgr = userCredsMgr

	log.Printf("多用户模式已启用，当前用户数: %d", userCredsMgr.Count())

	// 认证中间件
	authMw := &common.AuthMiddleware{
		Config:       cfg,
		GetUserCreds: userCredsMgr.GetCredentials,
	}

	// 路由
	mux := http.NewServeMux()

	// Anthropic API
	mux.HandleFunc("/v1/models", authMw.Wrap(anthropic.HandleGetModels))
	mux.HandleFunc("/v1/messages", authMw.Wrap(func(w http.ResponseWriter, r *http.Request) {
		anthropic.HandlePostMessages(w, r, provider)
	}))

	// OpenAI 兼容
	mux.HandleFunc("/v1/chat/completions", authMw.Wrap(func(w http.ResponseWriter, r *http.Request) {
		openai.HandleChatCompletions(w, r, provider)
	}))

	// 用户凭证管理 API
	mux.HandleFunc("/api/admin/user-credentials", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListUserCredentials(w, userCredsMgr)
		case http.MethodPost:
			handleAddUserCredential(w, r, userCredsMgr)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/admin/user-credentials/stats", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{"total_users": userCredsMgr.Count()})
	})
	mux.HandleFunc("/api/admin/user-credentials/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/api/admin/user-credentials/")
		if code == "" || code == "stats" {
			return
		}
		switch r.Method {
		case http.MethodGet:
			creds := userCredsMgr.GetCredentials(code)
			if creds == nil {
				common.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "激活码未找到"})
				return
			}
			common.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"activation_code": code, "has_credentials": true, "expires_at": creds.ExpiresAt,
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
		log.Printf("凭据已热加载，共 %d 个", len(newCreds))
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": true, "message": fmt.Sprintf("凭据已重新加载，共 %d 个", len(newCreds)),
		})
	})

	// CORS
	handler := corsMiddleware(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("启动服务器: %s", addr)
	log.Printf("API Key: %s***", cfg.APIKey[:len(cfg.APIKey)/2])
	log.Printf("Anthropic API:  GET /v1/models | POST /v1/messages")
	log.Printf("OpenAI API:     POST /v1/chat/completions")
	log.Printf("凭证管理 API:   /api/admin/user-credentials")

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

func loadConfig(path string) *model.Config {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	var cfg model.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("解析配置失败: %v", err)
	}
	return &cfg
}

func loadCredentials(path string) []*model.KiroCredentials {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("加载凭证失败: %v，使用空凭证列表", err)
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
	log.Printf("凭证文件格式无效")
	return nil
}

func handleListUserCredentials(w http.ResponseWriter, ucm *kiro.UserCredentialsManager) {
	entries := ucm.ListAll()
	var result []map[string]interface{}
	for _, e := range entries {
		result = append(result, map[string]interface{}{
			"activation_code": e.ActivationCode, "user_name": e.UserName,
			"has_credentials": true, "expires_at": e.Credentials.ExpiresAt,
			"created_at": e.CreatedAt, "updated_at": e.UpdatedAt,
		})
	}
	if result == nil {
		result = []map[string]interface{}{}
	}
	common.WriteJSON(w, http.StatusOK, result)
}

func handleAddUserCredential(w http.ResponseWriter, r *http.Request, ucm *kiro.UserCredentialsManager) {
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
	if err := ucm.AddOrUpdate(payload.ActivationCode, payload.UserName, payload.Credentials); err != nil {
		common.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("用户凭证已保存: %s (%s)", payload.ActivationCode, payload.UserName)
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
