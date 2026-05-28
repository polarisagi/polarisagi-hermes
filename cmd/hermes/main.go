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
	"polaris-hermes/internal/service/sync"
	"time"
	"polaris-hermes/internal/translator"
	anthropic2anthropic "polaris-hermes/internal/translator/anthropic/toanthropic"
	anthropic2google "polaris-hermes/internal/translator/anthropic/togoogle"
	anthropic2openai "polaris-hermes/internal/translator/anthropic/toopenai"
	google2anthropic "polaris-hermes/internal/translator/google/toanthropic"
	google2google "polaris-hermes/internal/translator/google/togoogle"
	google2openai "polaris-hermes/internal/translator/google/toopenai"
	openai2anthropic "polaris-hermes/internal/translator/openai/toanthropic"
	openai2google "polaris-hermes/internal/translator/openai/togoogle"
	openai2openai "polaris-hermes/internal/translator/openai/toopenai"
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
	clientBackupRepo := sqlite.NewClientBackupRepo(sqlite.DB())

	// 4. 初始化 Service/Brain 层
	intentInferer := router.NewIntentInferer(intentRepo)
	chanManager := channel.NewManager(providerRepo, modelRepo)
	
	// 从数据库预加载渠道
	if err := chanManager.Reload(context.Background()); err != nil {
		slog.Error("Failed to reload channels", "error", err)
	}

	pipeline := router.NewPipeline(routeRepo, intentRepo, intentInferer, chanManager)

	// 预热路由缓存（自定义路由表 + 系统意图字典 + 用户意图字典）
	if err := pipeline.Reload(context.Background()); err != nil {
		slog.Warn("路由管线缓存预热失败，将降级为实时查询", "error", err)
	}


	// 初始化外网模型同步引擎
	syncService := sync.NewSyncService(modelRepo, intentInferer)
	go func() {
		// 启动后延迟 1 分钟执行一次全量同步，然后每天执行一次
		time.Sleep(1 * time.Minute)
		syncService.SyncGlobalModels(context.Background())
		
		ticker := time.NewTicker(24 * time.Hour)
		for range ticker.C {
			syncService.SyncGlobalModels(context.Background())
		}
	}()

	// 5. 初始化协议转换工厂
	transFactory := translator.NewTranslatorFactory()
	
	// OpenAI 源协议
	transFactory.Register("openai_openai", openai2openai.NewOpenAITranslator()) // 直连透传
	transFactory.Register("openai_local", openai2openai.NewOpenAITranslator()) // 本地模型透传 (等同 openai_openai)
	transFactory.Register("openai_anthropic", openai2anthropic.NewOpenAIToAnthropicTranslator()) // OpenAI 转 Anthropic
	transFactory.Register("openai_google", openai2google.NewOpenAIToGoogleTranslator()) // OpenAI 转 Google
	
	// Anthropic 源协议
	transFactory.Register("anthropic_google", anthropic2google.NewAnthropicGoogleTranslator()) // Anthropic 转 Google GEAP
	transFactory.Register("anthropic_openai", anthropic2openai.NewAnthropicToOpenAITranslator()) // Anthropic 转 OpenAI
	transFactory.Register("anthropic_anthropic", anthropic2anthropic.NewAnthropicToAnthropicTranslator()) // Anthropic 转 Anthropic

	// Google 源协议
	transFactory.Register("google_google", google2google.NewGoogleToGoogleTranslator()) // Google 透传
	transFactory.Register("google_openai", google2openai.NewGoogleToOpenAITranslator()) // Google 转 OpenAI
	transFactory.Register("google_anthropic", google2anthropic.NewGoogleToAnthropicTranslator()) // Google 转 Anthropic

	// 6. 初始化高并发 Proxy 层
	proxyServer := proxy.NewServer(pipeline, chanManager, transFactory)

	// 7. 初始化控制面 WebAPI
	clientSvc := client.NewManager(settingsRepo, clientBackupRepo)
	adminHandler := webapi.NewAdminHandler(providerRepo, modelRepo, routeRepo, intentRepo, settingsRepo, clientSvc, chanManager, pipeline)

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
