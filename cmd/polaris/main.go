package main

import (
	"log/slog"
	"net/http"
	"os"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
	"polaris-gateway/internal/logger"
	"polaris-gateway/internal/router"
	_ "polaris-gateway/internal/translators" // Register all translators
	"polaris-gateway/internal/webapi"
)

func main() {
	logger.InitLogger()

	db.InitDB()
	defer db.CloseDB()

	if err := config.LoadConfig("config.yaml", ".env"); err != nil {
		slog.Error("配置栈装载故障", "error", err)
		os.Exit(1)
	}

	router.InitRouter()

	totalActive := 0
	for _, providers := range config.AppConfig.Providers {
		totalActive += len(providers)
	}

	if totalActive == 0 {
		slog.Warn("⚠️ 控制面无存活物理节点，API代理将挂起，请登录管理后台添加节点后重启网关")
	}

	webapi.InitMiddleware(totalActive)

	mux := http.NewServeMux()
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
	})
	mux.Handle("/dashboard/", webapi.DashboardHandler())
	mux.HandleFunc("/api/stats", webapi.StatsHandler)
	mux.HandleFunc("/api/admin/settings", webapi.AdminSettingsHandler)
	mux.HandleFunc("/api/admin/nodes", webapi.AdminNodesHandler)
	mux.HandleFunc("/api/admin/routes", webapi.AdminRoutesHandler)
	mux.HandleFunc("/api/admin/logs", webapi.AdminLogsHandler)

	// Unified Router Catch-All
	mux.HandleFunc("/v1/", router.ServeHTTP)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "Polaris Gateway Active"}`))
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "Endpoint not found"}`))
	})

	// 6. 启动控制平面
	slog.Info("==================================================")
	slog.Info("🚀 Project Atlas / Polaris Gateway Active", "address", config.AppConfig.ListenAddr)
	slog.Info("🚦 IO 并发排队槽位 (仅统计 Enabled: true 的物理节点)", "totalActive", totalActive)
	slog.Info("🌐 Universal API 入口", "url", "http://"+config.AppConfig.ListenAddr+"/v1/")
	slog.Info("==================================================")

	if err := http.ListenAndServe(config.AppConfig.ListenAddr, mux); err != nil {
		slog.Error("Gateway Server Crash", "error", err)
		os.Exit(1)
	}
}

