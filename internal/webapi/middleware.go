package webapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
)

var (
	MaxConcurrency int
	ActiveCount    int32
	WaitingCount   int32
)

func InitMiddleware(concurrency int) {
	if concurrency <= 0 {
		concurrency = 2
	}
	MaxConcurrency = concurrency
}

func ConcurrencyLimiter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	}
}

func StatsHandler(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" {
		startStr = time.Now().Format("2006-01-02")
	}
	if endStr == "" {
		endStr = startStr
	}

	// 🆕 核心 SQL 升级：提取 platform 字段并加入聚合维度
	query := `
		SELECT COALESCE(platform, 'unknown'),
		       node_name as account_name, 
			   COALESCE(client_id, 'Unknown') as client_name, 
			   COALESCE(method_name, 'Unknown'),
			   SUM(prompt_tokens), SUM(completion_tokens), SUM(cost_usd),
			   SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as err,
			   SUM(CASE WHEN status_code < 400 THEN 1 ELSE 0 END) as succ
		FROM account_logs 
		WHERE date(created_at, 'localtime') >= date(?) AND date(created_at, 'localtime') <= date(?)
		GROUP BY platform, node_name, client_id, method_name`

	rows, err := db.DB().Query(query, startStr, endStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type rowData struct {
		platform   string
		name       string
		client     string
		method     string
		pt, ct     int64
		periodCost float64
		errs, succ int64
	}
	var memRows []rowData

	for rows.Next() {
		var d rowData
		if err := rows.Scan(&d.platform, &d.name, &d.client, &d.method, &d.pt, &d.ct, &d.periodCost, &d.errs, &d.succ); err == nil {
			memRows = append(memRows, d)
		}
	}
	rows.Close()

	// 🆕 动态寻址：通过 platform 和 name 组合去 config 中精准查找账号信息
	getAccountMeta := func(platform string, name string) (float64, float64, string) {
		accounts, ok := config.AppConfig.Providers[platform]
		if ok {
			for _, acc := range accounts {
				if acc.Name == name {
					return acc.Balance, acc.LimitPercent, acc.ValidFrom
				}
			}
		}
		return 0, 90.0, "2000-01-01"
	}

	var details []map[string]interface{}
	for _, r := range memRows {
		budget, limitPercent, startDate := getAccountMeta(r.platform, r.name)
		absoluteTotal := db.GetTotalCost(r.name)
		cycleCost := db.GetConsumedSince(r.name, startDate)

		details = append(details, map[string]interface{}{
			"platform":          r.platform, // 🆕 将 platform 抛给前端
			"account":           r.name,
			"client":            r.client,
			"method":            r.method,
			"prompt_tokens":     r.pt,
			"completion_tokens": r.ct,
			"period_cost_usd":   r.periodCost,
			"total_cost_usd":    absoluteTotal,
			"cycle_cost_usd":    cycleCost,
			"start_date":        startDate,
			"error_count":       r.errs,
			"success_count":     r.succ,
			"balance":           budget,
			"limit_percent":     limitPercent,
			"valid_from":        startDate,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"details":       details,
		"active_count":  atomic.LoadInt32(&ActiveCount),
		"waiting_count": atomic.LoadInt32(&WaitingCount),
		"max_limit":     MaxConcurrency,
	}); err != nil {
		slog.Error("JSON 响应编码失败", "error", err)
	}
}
