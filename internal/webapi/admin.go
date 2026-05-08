package webapi

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
	"polaris-gateway/internal/logger"
)

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
			
			// Mask credentials for security if it's an API Key. For JSON, just masking part of it is tricky, but let's do a simple mask
			maskedCred := credentials
			if len(credentials) > 15 {
				maskedCred = credentials[:5] + "......" + credentials[len(credentials)-5:]
			} else {
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

		// Only update credentials if it wasn't masked
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

	// Read last 50KB maximum
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

// AdminRoutesHandler handles CRUD for /api/routes
func AdminRoutesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		// Join with sys_nodes to get the node name
		query := `
			SELECT r.id, r.match_model, r.node_id, r.target_model, r.status, n.name as node_name
			FROM sys_routes r
			LEFT JOIN sys_nodes n ON r.node_id = n.id
			ORDER BY r.id DESC
		`
		rows, err := db.DB().Query(query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var routes []map[string]interface{}
		for rows.Next() {
			var id, nodeID, status int
			var matchModel, targetModel string
			var nodeName sql.NullString
			
			if err := rows.Scan(&id, &matchModel, &nodeID, &targetModel, &status, &nodeName); err != nil {
				continue
			}

			routes = append(routes, map[string]interface{}{
				"id":           id,
				"match_model":  matchModel,
				"node_id":      nodeID,
				"node_name":    nodeName.String,
				"target_model": targetModel,
				"status":       status,
			})
		}
		json.NewEncoder(w).Encode(routes)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			MatchModel  string `json:"match_model"`
			NodeID      int    `json:"node_id"`
			TargetModel string `json:"target_model"`
			Status      int    `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.TargetModel == "" {
			req.TargetModel = req.MatchModel
		}

		_, err := db.DB().Exec(`
			INSERT INTO sys_routes (match_model, node_id, target_model, status)
			VALUES (?, ?, ?, ?)`,
			req.MatchModel, req.NodeID, req.TargetModel, req.Status)
		
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
			ID          int    `json:"id"`
			MatchModel  string `json:"match_model"`
			NodeID      int    `json:"node_id"`
			TargetModel string `json:"target_model"`
			Status      int    `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.TargetModel == "" {
			req.TargetModel = req.MatchModel
		}

		_, err := db.DB().Exec(`
			UPDATE sys_routes SET match_model=?, node_id=?, target_model=?, status=?
			WHERE id=?`,
			req.MatchModel, req.NodeID, req.TargetModel, req.Status, req.ID)
		
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
