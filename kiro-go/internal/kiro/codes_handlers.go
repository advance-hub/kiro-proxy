package kiro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"kiro-go/internal/common"
)

// HandleActivate POST /api/activate
func HandleActivate(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code      string `json:"code"`
		MachineID string `json:"machineId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效请求"})
		return
	}
	if req.Code == "" {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "缺少激活码"})
		return
	}
	if req.MachineID == "" {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "缺少机器码"})
		return
	}
	ok, msg := cm.Activate(req.Code, req.MachineID)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": ok, "message": msg})
}

// HandleCodeValidate POST /api/code/validate
// 仅检查激活码是否存在且已激活，不检查 machineId 和穿透权限
// 用于 API 访问鉴权（Cursor 等客户端不发送 machineId）
func HandleCodeValidate(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code      string `json:"code"`
		MachineID string `json:"machineId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": false, "message": "缺少参数"})
		return
	}
	if req.Code == "" {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": false, "message": "缺少激活码"})
		return
	}
	// 仅检查激活码是否有效（存在且已激活），machineId 为空时跳过设备检查
	ok, msg := cm.IsValidCode(req.Code, req.MachineID)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": ok, "message": msg})
}

// HandleTunnelCheck POST /api/tunnel/check
func HandleTunnelCheck(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code      string `json:"code"`
		MachineID string `json:"machineId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": false, "message": "缺少参数"})
		return
	}
	if req.Code == "" || req.MachineID == "" {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": false, "message": "缺少参数"})
		return
	}
	ok, msg, tunnelDays, expiresAt := cm.CheckTunnel(req.Code, req.MachineID)
	resp := map[string]interface{}{"success": ok, "message": msg}
	if ok {
		resp["tunnelDays"] = tunnelDays
		resp["expiresAt"] = expiresAt
	}
	common.WriteJSON(w, http.StatusOK, resp)
}

// HandleCodesList GET /api/codeslist
func HandleCodesList(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	codes := cm.GetAll()
	var rows strings.Builder
	for _, c := range codes {
		machineId := "-"
		if c.MachineID != nil {
			machineId = *c.MachineID
		}
		activatedAt := "-"
		if c.ActivatedAt != nil {
			activatedAt = *c.ActivatedAt
		}
		status := "❌"
		if c.Active {
			status = "✅"
		}
		rows.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%d</td></tr>",
			c.Code, status, machineId, activatedAt, c.TunnelDays))
	}
	html := fmt.Sprintf(`<html><head><meta charset="UTF-8"><style>
body{font-family:monospace;margin:40px}
table{border-collapse:collapse}
th,td{border:1px solid #ccc;padding:6px 12px}
th{background:#eee}
</style></head><body>
<h3>激活码列表</h3>
<table><tr><th>激活码</th><th>状态</th><th>机器码</th><th>激活时间</th><th>穿透天数</th></tr>%s</table>
</body></html>`, rows.String())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// HandleAdminGetCodes GET /api/admin/codes
func HandleAdminGetCodes(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	codes := cm.GetAll()
	total, activated, unused := cm.Stats()
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"codes":   codes,
		"stats": map[string]int{
			"total":     total,
			"activated": activated,
			"unused":    unused,
		},
	})
}

// HandleAdminAddCodes POST /api/admin/codes
func HandleAdminAddCodes(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	var req struct {
		Count       int      `json:"count"`
		CustomCodes []string `json:"customCodes"`
		TunnelDays  int      `json:"tunnelDays"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "无效请求"})
		return
	}
	if len(req.CustomCodes) == 0 && req.Count <= 0 {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "请提供 count 或 customCodes"})
		return
	}
	added := cm.AddCodes(req.CustomCodes, req.Count, req.TunnelDays)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("成功添加 %d 个卡密", len(added)),
		"added":   added,
	})
}

// HandleAdminDeleteCodes POST /api/admin/codes/delete
func HandleAdminDeleteCodes(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	var req struct {
		CodesToDelete []string `json:"codesToDelete"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.CodesToDelete) == 0 {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "请提供 codesToDelete 数组"})
		return
	}
	count := cm.DeleteCodes(req.CodesToDelete)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("成功删除 %d 个卡密", count),
	})
}

// HandleAdminUpdateCodes POST /api/admin/codes/update
func HandleAdminUpdateCodes(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	var req struct {
		CodesToUpdate []string `json:"codesToUpdate"`
		TunnelDays    *int     `json:"tunnelDays"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TunnelDays == nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "请提供 tunnelDays"})
		return
	}
	count := cm.UpdateTunnelDays(req.CodesToUpdate, *req.TunnelDays)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("成功更新 %d 个卡密", count),
	})
}

// HandleAdminResetCodes POST /api/admin/codes/reset
func HandleAdminResetCodes(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	var req struct {
		CodesToReset []string `json:"codesToReset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.CodesToReset) == 0 {
		common.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "message": "请提供 codesToReset 数组"})
		return
	}
	count := cm.ResetCodes(req.CodesToReset)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("成功重置 %d 个卡密", count),
	})
}

// HandleAdminExportCodes GET /api/admin/codes/export
func HandleAdminExportCodes(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	filterType := r.URL.Query().Get("type")
	if filterType == "" {
		filterType = "all"
	}
	text := cm.ExportCodes(filterType)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=codes-%s.txt", filterType))
	w.Write([]byte(text))
}

// HandleAdminCodesRouter /api/admin/codes 路由分发（GET/POST）
func HandleAdminCodesRouter(w http.ResponseWriter, r *http.Request, cm *CodesManager) {
	switch r.Method {
	case http.MethodGet:
		HandleAdminGetCodes(w, r, cm)
	case http.MethodPost:
		HandleAdminAddCodes(w, r, cm)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
