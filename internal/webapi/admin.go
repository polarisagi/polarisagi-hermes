package webapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"polaris-gateway/internal/clients_config"
	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
	"polaris-gateway/internal/logger"
	"polaris-gateway/internal/models"
)

// AdminDebugHandler toggles debug mode
var DebugEnabled bool

// normalizeDatetime 将仅含日期的字符串（YYYY-MM-DD）补全为完整的日期时间格式（YYYY-MM-DD HH:MM:SS）
// defaultTime 为补全的时间部分，如 "00:00:00" 或 "23:59:59"
// 已含时间部分的字符串直接原样返回
func normalizeDatetime(dt, defaultTime string) string {
	dt = strings.TrimSpace(dt)
	if len(dt) == 10 { // 仅 YYYY-MM-DD，补全时间
		return dt + " " + defaultTime
	}
	return dt
}

// Version can be injected via build flags (-ldflags="-X 'polaris-gateway/internal/webapi.Version=vX.Y.Z'")
var Version = "dev"

func AdminInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"version": "%s", "debug": %v}`, Version, DebugEnabled)))
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func AdminDebugHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"debug": %v}`, DebugEnabled)))
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		DebugEnabled = req.Enabled
		logger.SetDebug(DebugEnabled)
		slog.Info("🔧 Debug模式已切换", "enabled", DebugEnabled)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"debug": %v}`, DebugEnabled)))
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func init() {
	if Version == "dev" && config.Version != "dev" {
		Version = config.Version
	}
}

// AdminSettingsHandler handles GET and POST for /api/settings
func AdminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config.AppConfig)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			ListenAddr              string `json:"listen_addr"`
			InitialCooldownSeconds  int    `json:"initial_cooldown_seconds"`
			MaxCooldownSeconds      int    `json:"max_cooldown_seconds"`
			FailureThreshold        int    `json:"failure_threshold"`
			FailureWindowSeconds    int    `json:"failure_window_seconds"`
			GoogleOAuthClientID     string `json:"google_oauth_client_id"`
			GoogleOAuthClientSecret string `json:"google_oauth_client_secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_, err := db.DB().Exec("UPDATE sys_settings SET listen_addr=?, breaker_initial_cooldown_seconds=?, breaker_max_cooldown_seconds=?, breaker_failure_threshold=?, breaker_failure_window_seconds=?, google_oauth_client_id=?, google_oauth_client_secret=? WHERE id=1",
			req.ListenAddr, req.InitialCooldownSeconds, req.MaxCooldownSeconds, req.FailureThreshold, req.FailureWindowSeconds, req.GoogleOAuthClientID, req.GoogleOAuthClientSecret)

		if err != nil {
			slog.Error("更新系统配置失败", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func AdminUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TargetVersion string `json:"target_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetVersion == "" {
		http.Error(w, "invalid request or missing target_version", http.StatusBadRequest)
		return
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goarch == "x86_64" {
		goarch = "amd64" // Go reports amd64, but just in case
	}

	downloadURL := fmt.Sprintf("https://github.com/mrlaoliai/polaris-gateway/releases/download/%s/polaris-gateway-%s-%s", req.TargetVersion, goos, goarch)

	slog.Info("🔄 开始热更新程序...", "url", downloadURL)

	go func() {
		// 延迟执行以确保 HTTP 响应先返回给客户端
		time.Sleep(1 * time.Second)

		resp, err := http.Get(downloadURL)
		if err != nil {
			slog.Error("更新失败: 下载错误", "error", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			slog.Error("更新失败: 非 200 响应", "status", resp.StatusCode)
			return
		}

		exePath, err := os.Executable()
		if err != nil {
			slog.Error("更新失败: 无法获取可执行文件路径", "error", err)
			return
		}

		tmpPath := exePath + ".new"
		out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			slog.Error("更新失败: 无法创建临时文件", "error", err)
			return
		}

		if _, err := io.Copy(out, resp.Body); err != nil {
			out.Close()
			os.Remove(tmpPath)
			slog.Error("更新失败: 写入文件错误", "error", err)
			return
		}
		out.Close()

		if err := os.Rename(tmpPath, exePath); err != nil {
			os.Remove(tmpPath)
			slog.Error("更新失败: 替换可执行文件错误", "error", err)
			return
		}

		slog.Info("✅ 更新成功，准备退出并由守护进程自动重启...")
		os.Exit(0)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "success", "message": "正在后台更新，系统将在几秒内自动重启"}`))
}

// AdminModelsHandler 返回指定协议的可用模型列表，用于后台路由配置页面的模型选择下拉框
// GET /api/admin/models?protocol=google → 返回 Google Agent Platform 所有可用模型（含 Gemini + Claude GEAP）
// GET /api/admin/models → 返回所有协议的所有模型
func AdminModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	protocol := r.URL.Query().Get("protocol")

	var result []models.ModelInfo
	if protocol != "" {
		result = models.GetModelsByProtocol(protocol)
		if result == nil {
			result = []models.ModelInfo{}
		}
	} else {
		// 返回所有协议的模型，按协议分组
		for _, p := range models.GetAllProtocols() {
			result = append(result, models.GetModelsByProtocol(p)...)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"models": result,
	})
}

// AdminNodesHandler handles CRUD for /api/nodes
func AdminNodesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		rows, err := db.DB().Query("SELECT id, name, provider, base_url, credentials, project_id, location, priority, balance, used_amount, limit_percent, valid_from, valid_to, status, COALESCE(min_request_interval_sec, 0), COALESCE(concurrency, 0) FROM sys_nodes ORDER BY provider, priority DESC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var nodes []map[string]interface{}
		for rows.Next() {
			var id, priority, status, minRequestIntervalSec, concurrency int
			var name, provider, baseURL, credentials, projectID, location, validFrom, validTo string
			var balance, usedAmount, limitPercent float64

			if err := rows.Scan(&id, &name, &provider, &baseURL, &credentials, &projectID, &location, &priority, &balance, &usedAmount, &limitPercent, &validFrom, &validTo, &status, &minRequestIntervalSec, &concurrency); err != nil {
				continue
			}

			maskedCred := credentials
			if len(credentials) > 15 {
				maskedCred = credentials[:5] + "......" + credentials[len(credentials)-5:]
			} else if len(credentials) > 0 {
				maskedCred = "***"
			}

			nodes = append(nodes, map[string]interface{}{
				"id":                       id,
				"name":                     name,
				"provider":                 provider,
				"base_url":                 baseURL,
				"credentials":              maskedCred,
				"project_id":               projectID,
				"location":                 location,
				"priority":                 priority,
				"balance":                  balance,
				"used_amount":              usedAmount,
				"limit_percent":            limitPercent,
				"min_request_interval_sec": minRequestIntervalSec,
				"concurrency":              concurrency,
				"valid_from":               validFrom,
				"valid_to":                 validTo,
				"status":                   status,
			})
		}
		_ = json.NewEncoder(w).Encode(nodes)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Name                  string  `json:"name"`
			Provider              string  `json:"provider"`
			BaseURL               string  `json:"base_url"`
			Credentials           string  `json:"credentials"`
			ProjectID             string  `json:"project_id"`
			Location              string  `json:"location"`
			Priority              int     `json:"priority"`
			Balance               float64 `json:"balance"`
			LimitPercent          float64 `json:"limit_percent"`
			MinRequestIntervalSec int     `json:"min_request_interval_sec"`
			Concurrency           int     `json:"concurrency"`
			ValidFrom             string  `json:"valid_from"`
			ValidTo               string  `json:"valid_to"`
			Status                int     `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Provider == "google" && strings.TrimSpace(req.ProjectID) == "" {
			http.Error(w, `{"error":"project_id is required for Google Agent Platform nodes"}`, http.StatusBadRequest)
			return
		}
		if req.Concurrency < 0 || req.Concurrency > 1000 {
			http.Error(w, `{"error":"concurrency must be between 0 and 1000"}`, http.StatusBadRequest)
			return
		}
		if req.LimitPercent == 0 {
			req.LimitPercent = 90.0
		}
		req.ValidFrom = normalizeDatetime(req.ValidFrom, "00:00:00")
		req.ValidTo = normalizeDatetime(req.ValidTo, "23:59:59")
		if req.ValidFrom == "" {
			req.ValidFrom = "2000-01-01 00:00:00"
		}
		if req.ValidTo == "" {
			req.ValidTo = "2099-12-31 23:59:59"
		}

		_, err := db.DB().Exec(`
			INSERT INTO sys_nodes (name, provider, base_url, credentials, project_id, location, priority, balance, used_amount, limit_percent, min_request_interval_sec, concurrency, valid_from, valid_to, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0.0, ?, ?, ?, ?, ?, ?)`,
			req.Name, req.Provider, req.BaseURL, req.Credentials, req.ProjectID, req.Location, req.Priority, req.Balance, req.LimitPercent, req.MinRequestIntervalSec, req.Concurrency, req.ValidFrom, req.ValidTo, req.Status)

		if err != nil {
			slog.Error("节点写入数据库失败", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_ = config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
		return
	}

	if r.Method == http.MethodPut {
		var req struct {
			ID                    int     `json:"id"`
			Name                  string  `json:"name"`
			Provider              string  `json:"provider"`
			BaseURL               string  `json:"base_url"`
			Credentials           string  `json:"credentials"`
			ProjectID             string  `json:"project_id"`
			Location              string  `json:"location"`
			Priority              int     `json:"priority"`
			Balance               float64 `json:"balance"`
			LimitPercent          float64 `json:"limit_percent"`
			MinRequestIntervalSec int     `json:"min_request_interval_sec"`
			Concurrency           int     `json:"concurrency"`
			ValidFrom             string  `json:"valid_from"`
			ValidTo               string  `json:"valid_to"`
			Status                int     `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Provider == "google" && strings.TrimSpace(req.ProjectID) == "" {
			http.Error(w, `{"error":"project_id is required for Google Agent Platform nodes"}`, http.StatusBadRequest)
			return
		}
		if req.Concurrency < 0 || req.Concurrency > 1000 {
			http.Error(w, `{"error":"concurrency must be between 0 and 1000"}`, http.StatusBadRequest)
			return
		}
		req.ValidFrom = normalizeDatetime(req.ValidFrom, "00:00:00")
		req.ValidTo = normalizeDatetime(req.ValidTo, "23:59:59")
		if !strings.Contains(req.Credentials, "......") && req.Credentials != "***" && req.Credentials != "" {
			_, err := db.DB().Exec(`
				UPDATE sys_nodes SET name=?, provider=?, base_url=?, credentials=?, project_id=?, location=?, priority=?, balance=?, limit_percent=?, min_request_interval_sec=?, concurrency=?, valid_from=?, valid_to=?, status=?
				WHERE id=?`,
				req.Name, req.Provider, req.BaseURL, req.Credentials, req.ProjectID, req.Location, req.Priority, req.Balance, req.LimitPercent, req.MinRequestIntervalSec, req.Concurrency, req.ValidFrom, req.ValidTo, req.Status, req.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			_, err := db.DB().Exec(`
				UPDATE sys_nodes SET name=?, provider=?, base_url=?, project_id=?, location=?, priority=?, balance=?, limit_percent=?, min_request_interval_sec=?, concurrency=?, valid_from=?, valid_to=?, status=?
				WHERE id=?`,
				req.Name, req.Provider, req.BaseURL, req.ProjectID, req.Location, req.Priority, req.Balance, req.LimitPercent, req.MinRequestIntervalSec, req.Concurrency, req.ValidFrom, req.ValidTo, req.Status, req.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		_ = config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
		return
	}

	if r.Method == http.MethodDelete {
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, "missing id parameter", http.StatusBadRequest)
			return
		}
		id, _ := strconv.Atoi(idStr)
		_, err := db.DB().Exec("DELETE FROM sys_nodes WHERE id=?", id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// AdminLogsHandler returns the tail of the polaris-gateway.log file
func AdminLogsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	logPath := logger.GetLogPath()
	f, err := os.Open(logPath)
	if err != nil {
		_, _ = w.Write([]byte("No log file configured or polaris-gateway.log not found.\n"))
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var size int64 = 50 * 1024
	if info.Size() < size {
		size = info.Size()
	}

	buf := make([]byte, size)
	_, err = f.ReadAt(buf, info.Size()-size)
	if err != nil && err != io.EOF {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, _ = w.Write(buf)
}

// AdminRoutesHandler handles CRUD for /api/admin/routes
func AdminRoutesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		rows, err := db.DB().Query("SELECT id, source_protocol, target_protocol, model_mappings, status FROM sys_routes ORDER BY id DESC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var routes []map[string]interface{}
		for rows.Next() {
			var id, status int
			var sourceProtocol, targetProtocol, modelMappings string

			if err := rows.Scan(&id, &sourceProtocol, &targetProtocol, &modelMappings, &status); err != nil {
				continue
			}

			// Parse model_mappings JSON for the frontend
			var mappings []map[string]string
			if modelMappings != "" {
				_ = json.Unmarshal([]byte(modelMappings), &mappings)
			}
			if mappings == nil {
				mappings = []map[string]string{}
			}

			routes = append(routes, map[string]interface{}{
				"id":              id,
				"source_protocol": sourceProtocol,
				"target_protocol": targetProtocol,
				"model_mappings":  mappings,
				"status":          status,
			})
		}
		_ = json.NewEncoder(w).Encode(routes)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			SourceProtocol string              `json:"source_protocol"`
			TargetProtocol string              `json:"target_protocol"`
			ModelMappings  []map[string]string `json:"model_mappings"`
			Status         int                 `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		modelMappingsJSON, _ := json.Marshal(req.ModelMappings)

		_, err := db.DB().Exec(`
			INSERT INTO sys_routes (source_protocol, target_protocol, model_mappings, status)
			VALUES (?, ?, ?, ?)`,
			req.SourceProtocol, req.TargetProtocol, string(modelMappingsJSON), req.Status)

		if err != nil {
			slog.Error("路由写入数据库失败", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_ = config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
		return
	}

	if r.Method == http.MethodPut {
		var req struct {
			ID             int                 `json:"id"`
			SourceProtocol string              `json:"source_protocol"`
			TargetProtocol string              `json:"target_protocol"`
			ModelMappings  []map[string]string `json:"model_mappings"`
			Status         int                 `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		modelMappingsJSON, _ := json.Marshal(req.ModelMappings)

		_, err := db.DB().Exec(`
			UPDATE sys_routes SET source_protocol=?, target_protocol=?, model_mappings=?, status=?
			WHERE id=?`,
			req.SourceProtocol, req.TargetProtocol, string(modelMappingsJSON), req.Status, req.ID)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_ = config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
		return
	}

	if r.Method == http.MethodDelete {
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, "missing id parameter", http.StatusBadRequest)
			return
		}
		id, _ := strconv.Atoi(idStr)
		_, err := db.DB().Exec("DELETE FROM sys_routes WHERE id=?", id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "success"}`))
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// AdminClientsConfigApplyHandler handles POST /api/admin/clients/apply
func AdminClientsConfigApplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Client string `json:"client"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Client == "" {
		http.Error(w, "invalid request or missing client", http.StatusBadRequest)
		return
	}

	configurator, err := clients_config.GetConfigurator(req.Client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	gatewayAddr := config.AppConfig.ListenAddr
	// In some deployments, it might be behind a reverse proxy, but locally it is ListenAddr.
	if err := configurator.Apply(gatewayAddr); err != nil {
		slog.Error("Failed to apply client config", "client", req.Client, "error", err)
		http.Error(w, fmt.Sprintf("failed to apply config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "success"}`))
}

// AdminClientsConfigRestoreHandler handles POST /api/admin/clients/restore
func AdminClientsConfigRestoreHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Client string `json:"client"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Client == "" {
		http.Error(w, "invalid request or missing client", http.StatusBadRequest)
		return
	}

	configurator, err := clients_config.GetConfigurator(req.Client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := configurator.Restore(); err != nil {
		slog.Error("Failed to restore client config", "client", req.Client, "error", err)
		http.Error(w, fmt.Sprintf("failed to restore config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "success"}`))
}

// AdminClientsConfigStatusHandler handles GET /api/admin/clients/status
func AdminClientsConfigStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var statuses []clients_config.ClientStatus
	for _, client := range clients_config.GetAllSupportedClients() {
		configurator, err := clients_config.GetConfigurator(client)
		if err != nil {
			continue
		}

		isConfigured, hasBackup, err := configurator.Status()
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}

		statuses = append(statuses, clients_config.ClientStatus{
			Name:         client,
			IsConfigured: isConfigured,
			HasBackup:    hasBackup,
			Error:        errMsg,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"clients": statuses,
	})
}
