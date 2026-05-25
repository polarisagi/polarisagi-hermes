package router

import (
	"context"
	"errors"
	"log/slog"

	"polaris-gateway/internal/repository/sqlite"
	"polaris-gateway/internal/service/channel"
)

var ErrNoAvailableModel = errors.New("no available model found for the requested capability tier")

// Pipeline 实现了 4 级降维匹配的路由管线
type Pipeline struct {
	routeRepo     *sqlite.RouteRepo
	intentRepo    *sqlite.IntentRepo
	intentInferer *IntentInferer
	chanManager   *channel.Manager
}

func NewPipeline(
	routeRepo *sqlite.RouteRepo,
	intentRepo *sqlite.IntentRepo,
	intentInferer *IntentInferer,
	chanManager *channel.Manager,
) *Pipeline {
	return &Pipeline{
		routeRepo:     routeRepo,
		intentRepo:    intentRepo,
		intentInferer: intentInferer,
		chanManager:   chanManager,
	}
}

// RouteRequest 核心路由入口：给定一个客户端请求的模型 ID，返回最终应该调用的后台通道和真实模型名
func (p *Pipeline) RouteRequest(ctx context.Context, requestedModelID string) (*channel.ActiveChannel, string, error) {
	slog.Debug("🚀 启动 4 级智能路由管线", "requested_model", requestedModelID)

	// [优先级 1] 检查自定义硬路由 (1对1)
	if targetUserModelID := p.checkCustomRoute(ctx, requestedModelID); targetUserModelID > 0 {
		slog.Debug("✅ 命中 [优先级 1] 自定义硬路由", "target_user_model_id", targetUserModelID)
		ch, actualModel, err := p.chanManager.GetChannelByUserModelID(targetUserModelID)
		if err == nil {
			return ch, actualModel, nil
		}
		slog.Warn("⚠️ 自定义路由对应的节点不可用，降级至意图推断模式")
	}

	// 开始意图推断 (Tier 解析)
	tier := p.resolveCapabilityTier(ctx, requestedModelID)
	slog.Info("🧠 意图解析完成", "requested_model", requestedModelID, "resolved_tier", tier)

	// 根据意图标签，交给 Channel Manager 去从内存健康池里挑一个最优的节点
	ch, actualModel, err := p.chanManager.SelectBestChannelByTier(tier)
	if err != nil {
		slog.Error("❌ 没有找到匹配该意图标签的健康节点", "tier", tier, "error", err)
		return nil, "", ErrNoAvailableModel
	}

	return ch, actualModel, nil
}

// resolveCapabilityTier 负责执行 2, 3, 4 级优先级解析
func (p *Pipeline) resolveCapabilityTier(ctx context.Context, requestedModelID string) string {
	// [优先级 2] 用户级意图覆盖
	userTier, _ := p.intentRepo.GetUserIntent(ctx, requestedModelID)
	if userTier != "" {
		slog.Debug("✅ 命中 [优先级 2] 用户级意图字典", "tier", userTier)
		return userTier
	}

	// [优先级 3] 系统级意图兜底
	sysTier, _ := p.intentRepo.GetSysIntent(ctx, requestedModelID)
	if sysTier != "" {
		slog.Debug("✅ 命中 [优先级 3] 系统级意图字典", "tier", sysTier)
		return sysTier
	}

	// [优先级 4] 未知模型自动学习与推断
	slog.Warn("⚠️ 遇到未知请求模型，触发自动推断引擎", "model", requestedModelID)
	inferredTier := p.intentInferer.InferUnknownModel(ctx, requestedModelID)
	slog.Info("🤖 自动推断引擎判定结果", "model", requestedModelID, "tier", inferredTier)
	
	return inferredTier
}

// checkCustomRoute 检查是否存在针对该请求模型的自定义硬路由
func (p *Pipeline) checkCustomRoute(ctx context.Context, requestedModelID string) int {
	routes, err := p.routeRepo.GetUserCustomRoutes(ctx)
	if err != nil {
		return 0
	}
	for _, r := range routes {
		if r.RequestedModelID == requestedModelID && r.IsActive {
			return r.TargetUserModelID
		}
	}
	return 0
}
