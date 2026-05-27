package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"polaris-hermes/internal/api/webapi"
	"polaris-hermes/internal/config"
	"polaris-hermes/internal/proxy"
	"polaris-hermes/internal/repository/sqlite"
	"polaris-hermes/internal/service/channel"
	"polaris-hermes/internal/service/client"
	"polaris-hermes/internal/service/router"
	"polaris-hermes/internal/translator"
	"polaris-hermes/internal/translator/anthropic"
	"polaris-hermes/pkg/logger"
)

func main() {
	// 1. 加载 TOML 配置文件
	if err := config.LoadConfig("config.toml"); err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// 2. 初始化持久化日志 (根据 config.WorkDir 存放日志到对应目录)
	logger.InitLogger()

	// 3. 初始化底层 SQLite (包含删库重建 001_init_v2.sql)
	sqlite.InitDB()

	// 3. 初始化 Repository 层
	providerRepo := sqlite.NewProviderRepo()
	modelRepo := sqlite.NewModelRepo()
	routeRepo := sqlite.NewRouteRepo()
	intentRepo := sqlite.NewIntentRepo()
	settingsRepo := sqlite.NewSettingsRepo(sqlite.DB())

	// 4. 初始化 Service/Brain 层
	intentInferer := router.NewIntentInferer(intentRepo)
	chanManager := channel.NewManager(providerRepo, modelRepo)
	
	// 从数据库预加载渠道
	if err := chanManager.Reload(context.Background()); err != nil {
		slog.Error("Failed to reload channels", "error", err)
	}

	pipeline := router.NewPipeline(routeRepo, intentRepo, intentInferer, chanManager)

	// 5. 初始化协议转换工厂
	transFactory := translator.NewTranslatorFactory()
	transFactory.Register("google", anthropic.NewAnthropicGoogleTranslator())

	// 6. 初始化高并发 Proxy 层
	proxyServer := proxy.NewServer(pipeline, chanManager, transFactory)

	// 7. 初始化控制面 WebAPI
	clientSvc := client.NewManager(settingsRepo)
	adminHandler := webapi.NewAdminHandler(providerRepo, modelRepo, routeRepo, settingsRepo, clientSvc)

	// 8. 路由挂载
	mux := http.NewServeMux()
	
	// 控制面 (Admin UI 使用)
	adminHandler.RegisterRoutes(mux)
	
	// 静态文件服务 (前端 Dashboard)
	fs := http.FileServer(http.Dir("web/ui"))
	mux.Handle("/", fs)
	
	// 兼容老的 /dashboard 路径
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})

	// 数据面 (大模型客户端如 Claude Code 使用)
	// 拦截 /v1/chat/completions 等 OpenAI 标准协议前缀
	mux.Handle("/v1/", proxyServer)

	// 9. 启动服务
	listenAddr := config.GlobalConfig.Server.ListenAddr
	slog.Info("🚀 Polaris Hermes v2 (2026 AI Standard) is starting on " + listenAddr + "...")
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		slog.Error("Server crashed", "error", err)
	}
}
