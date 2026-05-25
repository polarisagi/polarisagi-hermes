package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"polaris-hermes/internal/api/webapi"
	"polaris-hermes/internal/proxy"
	"polaris-hermes/internal/repository/sqlite"
	"polaris-hermes/internal/service/channel"
	"polaris-hermes/internal/service/router"
	"polaris-hermes/internal/translator"
	"polaris-hermes/internal/translator/anthropic"
)

func main() {
	// 1. 初始化日志
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// 2. 初始化底层 SQLite (包含删库重建 001_init_v2.sql)
	sqlite.InitDB()

	// 3. 初始化 Repository 层
	providerRepo := sqlite.NewProviderRepo()
	modelRepo := sqlite.NewModelRepo()
	routeRepo := sqlite.NewRouteRepo()
	intentRepo := sqlite.NewIntentRepo()

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
	adminHandler := webapi.NewAdminHandler(providerRepo, modelRepo)

	// 8. 路由挂载
	mux := http.NewServeMux()
	
	// 控制面 (Admin UI 使用)
	adminHandler.RegisterRoutes(mux)
	
	// 数据面 (大模型客户端如 Claude Code 使用)
	// 拦截 /v1/chat/completions 等 OpenAI 标准协议前缀
	mux.Handle("/v1/", proxyServer)

	// 9. 启动服务
	slog.Info("🚀 Polaris Hermes v2 (2026 AI Standard) is starting on :8080...")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		slog.Error("Server crashed", "error", err)
	}
}
