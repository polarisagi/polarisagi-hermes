// 网关中间件: 并发统计 + 实时统计 API
// ActiveCount/WaitingCount 用于监控大盘的实时队列状态
package webapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/store"
	"polaris-gateway/internal/core/router"
)

// InitMiddleware 初始化并发限制，concurrency 为活跃节点总数
func InitMiddleware(concurrency int) {
	if concurrency <= 0 {
		concurrency = 2
	}
	router.MaxConcurrency = concurrency
}

func ConcurrencyLimiter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	}
}

// StatsHandler 实时统计 API 端点
// 按 platform + node_name + client_id + method_name 多维度聚合查询指定日期范围内的用量数据
// 返回: 用量明细 + 活跃并发数 + 排队等待数 + 最大并发限制
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

	rows, err := store.DB().Query(query, startStr, endStr)
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
		absoluteTotal := store.GetTotalCost(r.name)
		cycleCost := store.GetConsumedSince(r.name, startDate)

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
		"active_count":  atomic.LoadInt32(&router.ActiveCount),
		"waiting_count": atomic.LoadInt32(&router.WaitingCount),
		"max_limit":     router.MaxConcurrency,
	}); err != nil {
		slog.Error("JSON 响应编码失败", "error", err)
	}
}
