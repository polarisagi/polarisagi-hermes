package webapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
	"polaris-gateway/internal/logger"
)

// AdminDebugHandler toggles debug mode
var DebugEnabled bool

func AdminDebugHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"debug": %v}`, DebugEnabled)))
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
		w.Write([]byte(fmt.Sprintf(`{"debug": %v}`, DebugEnabled)))
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func init() {}

// AdminSettingsHandler handles GET and POST for /api/settings
func AdminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config.AppConfig)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			ListenAddr             string `json:"listen_addr"`
			InitialCooldownSeconds int    `json:"initial_cooldown_seconds"`
			MaxCooldownSeconds     int    `json:"max_cooldown_seconds"`
			FailureThreshold       int    `json:"failure_threshold"`
			FailureWindowSeconds   int    `json:"failure_window_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_, err := db.DB().Exec("UPDATE sys_settings SET listen_addr=?, breaker_initial_cooldown_seconds=?, breaker_max_cooldown_seconds=?, breaker_failure_threshold=?, breaker_failure_window_seconds=? WHERE id=1",
			req.ListenAddr, req.InitialCooldownSeconds, req.MaxCooldownSeconds, req.FailureThreshold, req.FailureWindowSeconds)
		
		if err != nil {
			slog.Error("Failed to update settings", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// AdminNodesHandler handles CRUD for /api/nodes
func AdminNodesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		rows, err := db.DB().Query("SELECT id, name, provider, base_url, credentials, project_id, location, priority, balance, used_amount, limit_percent, valid_from, valid_to, status FROM sys_nodes ORDER BY provider, priority DESC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var nodes []map[string]interface{}
		for rows.Next() {
			var id, priority, status int
			var name, provider, baseURL, credentials, projectID, location, validFrom, validTo string
			var balance, usedAmount, limitPercent float64
			
			if err := rows.Scan(&id, &name, &provider, &baseURL, &credentials, &projectID, &location, &priority, &balance, &usedAmount, &limitPercent, &validFrom, &validTo, &status); err != nil {
				continue
			}
			
			maskedCred := credentials
			if len(credentials) > 15 {
				maskedCred = credentials[:5] + "......" + credentials[len(credentials)-5:]
			} else if len(credentials) > 0 {
				maskedCred = "***"
			}

			nodes = append(nodes, map[string]interface{}{
				"id":            id,
				"name":          name,
				"provider":      provider,
				"base_url":      baseURL,
				"credentials":   maskedCred,
				"project_id":    projectID,
				"location":      location,
				"priority":      priority,
				"balance":       balance,
				"used_amount":   usedAmount,
				"limit_percent": limitPercent,
				"valid_from":    validFrom,
				"valid_to":      validTo,
				"status":        status,
			})
		}
		json.NewEncoder(w).Encode(nodes)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Name         string  `json:"name"`
			Provider     string  `json:"provider"`
			BaseURL      string  `json:"base_url"`
			Credentials  string  `json:"credentials"`
			ProjectID    string  `json:"project_id"`
			Location     string  `json:"location"`
			Priority     int     `json:"priority"`
			Balance      float64 `json:"balance"`
			LimitPercent float64 `json:"limit_percent"`
			ValidFrom    string  `json:"valid_from"`
			ValidTo      string  `json:"valid_to"`
			Status       int     `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.LimitPercent == 0 {
			req.LimitPercent = 90.0
		}
		if req.ValidFrom == "" {
			req.ValidFrom = "2000-01-01 00:00:00"
		}
		if req.ValidTo == "" {
			req.ValidTo = "2099-12-31 23:59:59"
		}

		_, err := db.DB().Exec(`
			INSERT INTO sys_nodes (name, provider, base_url, credentials, project_id, location, priority, balance, used_amount, limit_percent, valid_from, valid_to, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0.0, ?, ?, ?, ?)`,
			req.Name, req.Provider, req.BaseURL, req.Credentials, req.ProjectID, req.Location, req.Priority, req.Balance, req.LimitPercent, req.ValidFrom, req.ValidTo, req.Status)
		
		if err != nil {
			slog.Error("Failed to insert node", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
		return
	}

	if r.Method == http.MethodPut {
		var req struct {
			ID           int     `json:"id"`
			Name         string  `json:"name"`
			Provider     string  `json:"provider"`
			BaseURL      string  `json:"base_url"`
			Credentials  string  `json:"credentials"`
			ProjectID    string  `json:"project_id"`
			Location     string  `json:"location"`
			Priority     int     `json:"priority"`
			Balance      float64 `json:"balance"`
			LimitPercent float64 `json:"limit_percent"`
			ValidFrom    string  `json:"valid_from"`
			ValidTo      string  `json:"valid_to"`
			Status       int     `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if !strings.Contains(req.Credentials, "......") && req.Credentials != "***" && req.Credentials != "" {
			_, err := db.DB().Exec(`
				UPDATE sys_nodes SET name=?, provider=?, base_url=?, credentials=?, project_id=?, location=?, priority=?, balance=?, limit_percent=?, valid_from=?, valid_to=?, status=?
				WHERE id=?`,
				req.Name, req.Provider, req.BaseURL, req.Credentials, req.ProjectID, req.Location, req.Priority, req.Balance, req.LimitPercent, req.ValidFrom, req.ValidTo, req.Status, req.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			_, err := db.DB().Exec(`
				UPDATE sys_nodes SET name=?, provider=?, base_url=?, project_id=?, location=?, priority=?, balance=?, limit_percent=?, valid_from=?, valid_to=?, status=?
				WHERE id=?`,
				req.Name, req.Provider, req.BaseURL, req.ProjectID, req.Location, req.Priority, req.Balance, req.LimitPercent, req.ValidFrom, req.ValidTo, req.Status, req.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		
		config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
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
		config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
		return
	}
	
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// AdminLogsHandler returns the tail of the polaris-gateway.log file
func AdminLogsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if logger.LogFile == nil {
		w.Write([]byte("No log file configured or polaris-gateway.log not found.\n"))
		return
	}

	info, err := logger.LogFile.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var size int64 = 50 * 1024
	if info.Size() < size {
		size = info.Size()
	}

	buf := make([]byte, size)
	_, err = logger.LogFile.ReadAt(buf, info.Size()-size)
	if err != nil && err != io.EOF {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(buf)
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
				json.Unmarshal([]byte(modelMappings), &mappings)
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
		json.NewEncoder(w).Encode(routes)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			SourceProtocol  string                   `json:"source_protocol"`
			TargetProtocol  string                   `json:"target_protocol"`
			ModelMappings   []map[string]string      `json:"model_mappings"`
			Status          int                      `json:"status"`
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
			slog.Error("Failed to insert route", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
		return
	}

	if r.Method == http.MethodPut {
		var req struct {
			ID              int                      `json:"id"`
			SourceProtocol  string                   `json:"source_protocol"`
			TargetProtocol  string                   `json:"target_protocol"`
			ModelMappings   []map[string]string      `json:"model_mappings"`
			Status          int                      `json:"status"`
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
		
		config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
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
		config.ReloadFromDB()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
		return
	}
	
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
