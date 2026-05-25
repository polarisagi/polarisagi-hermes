// Polaris Gateway — 多协议 LLM API 网关入口
// 支持 OpenAI、Anthropic、Google Agent Platform 三大协议之间的任意转换路由
// 核心功能: 协议转换 + 路由分发 + 负载均衡 + 熔断保护 + 用量计费
package main

import (
	"log/slog"
	"net/http"
	"os"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/store"
	"polaris-gateway/pkg/logger"
	"polaris-gateway/internal/core/router"
	_ "polaris-gateway/pkg/translators/anthropic" // 通过 init() 自动注册协议转换器
	_ "polaris-gateway/pkg/translators/google"
	_ "polaris-gateway/pkg/translators/openai"
	"polaris-gateway/internal/api/webapi"
)

func main() {
	logger.InitLogger()

	store.InitDB()
	defer store.CloseDB()

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
	mux.HandleFunc("/api/admin/info", webapi.AdminInfoHandler)
	mux.HandleFunc("/api/admin/settings", webapi.AdminSettingsHandler)
	mux.HandleFunc("/api/admin/nodes", webapi.AdminNodesHandler)
	mux.HandleFunc("/api/admin/routes", webapi.AdminRoutesHandler)
	mux.HandleFunc("/api/admin/logs", webapi.AdminLogsHandler)
	mux.HandleFunc("/api/admin/debug", webapi.AdminDebugHandler)
	mux.HandleFunc("/api/admin/models", webapi.AdminModelsHandler)
	mux.HandleFunc("/api/admin/update", webapi.AdminUpdateHandler)
	mux.HandleFunc("/api/admin/oauth/google/start", webapi.AdminOAuthGoogleStartHandler)
	mux.HandleFunc("/api/admin/oauth/google/callback", webapi.AdminOAuthGoogleCallbackHandler)

	// Client auto-configuration routes
	mux.HandleFunc("/api/admin/clients/apply", webapi.AdminClientsConfigApplyHandler)
	mux.HandleFunc("/api/admin/clients/restore", webapi.AdminClientsConfigRestoreHandler)
	mux.HandleFunc("/api/admin/clients/status", webapi.AdminClientsConfigStatusHandler)

	// Unified Router Catch-All
	mux.HandleFunc("/v1/", router.ServeHTTP)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "Polaris Gateway Active"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "Endpoint not found"}`))
	})

	// 6. 启动控制平面
	slog.Info("==================================================")
	slog.Info("🚀 Project Atlas / Polaris Gateway Active", "address", config.AppConfig.ListenAddr)
	slog.Info("🚦 IO 并发排队槽位 (仅统计 Enabled: true 的物理节点)", "totalActive", totalActive)
	slog.Info("🌐 OpenAI 接入", "url", "http://"+config.AppConfig.ListenAddr+"/v1/openai/")
	slog.Info("🌐 Anthropic 接入", "url", "http://"+config.AppConfig.ListenAddr+"/v1/anthropic/")
	slog.Info("🌐 Google Agent Platform 接入", "url", "http://"+config.AppConfig.ListenAddr+"/v1/google/")
	slog.Info("   (旧路径向后兼容)", "url", "http://"+config.AppConfig.ListenAddr+"/v1/vertex/")
	slog.Info("==================================================")

	if err := http.ListenAndServe(config.AppConfig.ListenAddr, mux); err != nil {
		slog.Error("Gateway Server Crash", "error", err)
		os.Exit(1)
	}
}
