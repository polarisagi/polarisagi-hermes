package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
	"polaris-gateway/internal/logger"
	"polaris-gateway/internal/proxy/protocol_anthropic"
	"polaris-gateway/internal/proxy/protocol_openai"
	"polaris-gateway/internal/proxy/protocol_vertex"
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

	vertexAccs := config.AppConfig.Providers["vertex"]
	openaiAccs := config.AppConfig.Providers["openai"]
	totalActive := len(vertexAccs) + len(openaiAccs)

	if totalActive == 0 {
		slog.Warn("⚠️ 控制面无存活物理节点，API代理将挂起，请登录管理后台添加节点后重启网关")
	}

	webapi.InitMiddleware(totalActive)

	// 🛠️ 关键修复：传入账号列表参数以符合 Handler 定义
	vertexHandler := protocol_vertex.NewHandler(vertexAccs)
	openaiHandler := protocol_openai.NewHandler(openaiAccs)
	anthropicHandler := protocol_anthropic.NewHandler(vertexAccs)

	mux := http.NewServeMux()
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
	})
	mux.Handle("/dashboard/", webapi.DashboardHandler())
	mux.HandleFunc("/api/stats", webapi.StatsHandler)
	mux.HandleFunc("/api/admin/settings", webapi.AdminSettingsHandler)
	mux.HandleFunc("/api/admin/nodes", webapi.AdminNodesHandler)
	mux.HandleFunc("/api/admin/logs", webapi.AdminLogsHandler)

	mux.HandleFunc("/v1/openai/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/v1/openai")
		webapi.ConcurrencyLimiter(openaiHandler.ProxyHandler)(w, r)
	})

	mux.HandleFunc("/v1/vertex/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/v1/vertex")
		webapi.ConcurrencyLimiter(vertexHandler.ProxyHandler)(w, r)
	})

	mux.HandleFunc("/v1/anthropic/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/v1/anthropic")
		webapi.ConcurrencyLimiter(anthropicHandler.ProxyHandler)(w, r)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "Polaris Gateway Active"}`))
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "Only /v1/openai/*, /v1/vertex/*, and /v1/anthropic/* endpoints are supported"}`))
	})

	// 6. 启动控制平面
	slog.Info("==================================================")
	slog.Info("🚀 Project Atlas / Polaris Gateway Active", "address", config.AppConfig.ListenAddr)
	slog.Info("🚦 IO 并发排队槽位 (仅统计 Enabled: true 的物理节点)", "totalActive", totalActive)
	slog.Info("🌐 OpenAI    协议入口", "url", "http://"+config.AppConfig.ListenAddr+"/v1/openai")
	slog.Info("🌐 Vertex    协议入口", "url", "http://"+config.AppConfig.ListenAddr+"/v1/vertex")
	slog.Info("🌐 Anthropic 协议入口", "url", "http://"+config.AppConfig.ListenAddr+"/v1/anthropic")
	slog.Info("==================================================")

	if err := http.ListenAndServe(config.AppConfig.ListenAddr, mux); err != nil {
		slog.Error("Gateway Server Crash", "error", err)
		os.Exit(1)
	}
}
