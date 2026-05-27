package router

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"polaris-hermes/internal/repository/sqlite"
	"polaris-hermes/internal/service/channel"
)

var ErrNoAvailableModel = errors.New("no available model found for the requested capability tier")

// Pipeline 实现了 4 级降维匹配的路由管线，所有热路径均走内存缓存，零 DB 查询
type Pipeline struct {
	routeRepo     *sqlite.RouteRepo
	intentRepo    *sqlite.IntentRepo
	intentInferer *IntentInferer
	chanManager   *channel.Manager

	// 内存缓存，通过 Reload() 从数据库批量加载，热重载时原子替换
	mu              sync.RWMutex
	customRouteMap  map[string]int    // requestedModelID → targetUserModelID（精确匹配）
	wildcardRouteID int               // * 通配符对应的 targetUserModelID（0 表示未配置）
	sysIntentMap    map[string]string // requestedModelID → tier（系统内置，只读）
	userIntentMap   map[string]string // requestedModelID → tier（用户覆盖 + 自动学习，写透缓存）
}

func NewPipeline(
	routeRepo *sqlite.RouteRepo,
	intentRepo *sqlite.IntentRepo,
	intentInferer *IntentInferer,
	chanManager *channel.Manager,
) *Pipeline {
	return &Pipeline{
		routeRepo:      routeRepo,
		intentRepo:     intentRepo,
		intentInferer:  intentInferer,
		chanManager:    chanManager,
		customRouteMap: make(map[string]int),
		sysIntentMap:   make(map[string]string),
		userIntentMap:  make(map[string]string),
	}
}

// Reload 从数据库全量加载缓存，应在启动时和配置变更后调用
func (p *Pipeline) Reload(ctx context.Context) error {
	// 1. 加载自定义路由表
	newCustomRouteMap := make(map[string]int)
	var newWildcardID int
	routes, err := p.routeRepo.GetUserCustomRoutes(ctx)
	if err != nil {
		slog.Warn("⚠️ [Pipeline] 加载自定义路由缓存失败", "error", err)
	} else {
		for _, r := range routes {
			if r.RequestedModelID == "*" {
				newWildcardID = r.TargetUserModelID
			} else {
				newCustomRouteMap[r.RequestedModelID] = r.TargetUserModelID
			}
		}
	}

	// 2. 加载系统意图字典
	newSysIntentMap, err := p.intentRepo.GetAllSysIntents(ctx)
	if err != nil {
		slog.Warn("⚠️ [Pipeline] 加载系统意图字典缓存失败", "error", err)
		newSysIntentMap = make(map[string]string)
	}

	// 3. 加载用户意图字典
	newUserIntentMap, err := p.intentRepo.GetAllUserIntents(ctx)
	if err != nil {
		slog.Warn("⚠️ [Pipeline] 加载用户意图字典缓存失败", "error", err)
		newUserIntentMap = make(map[string]string)
	}

	// 4. 原子替换（写锁仅用于极短的指针替换，不持有锁执行 DB 查询）
	p.mu.Lock()
	p.customRouteMap = newCustomRouteMap
	p.wildcardRouteID = newWildcardID
	p.sysIntentMap = newSysIntentMap
	p.userIntentMap = newUserIntentMap
	p.mu.Unlock()

	slog.Info("✅ [Pipeline] 路由缓存热重载完成",
		"custom_routes", len(newCustomRouteMap),
		"sys_intents", len(newSysIntentMap),
		"user_intents", len(newUserIntentMap),
		"wildcard", newWildcardID > 0,
	)
	return nil
}

// RouteRequest 核心路由入口：给定一个客户端请求的模型 ID，返回最终应该调用的后台通道和真实模型名
func (p *Pipeline) RouteRequest(ctx context.Context, requestedModelID string) (*channel.ActiveChannel, string, error) {
	slog.Debug("🚀 启动 4 级智能路由管线", "requested_model", requestedModelID)

	// [优先级 1] 检查自定义硬路由 (1对1) — 读内存缓存
	if targetUserModelID := p.checkCustomRoute(requestedModelID); targetUserModelID > 0 {
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

	// Tier 降级熔断链：当目标 tier 无可用节点时，自动降级到 smart 兜底。
	// 场景：用户后端只有 DeepSeek 的 smart 模型，但客户端发来了 gpt-4o-mini (fast tier)，
	// 此时不应直接 503，而应降级到 smart 模型处理，保证服务连续性。
	tiersToTry := []string{tier}
	if tier != "smart" {
		tiersToTry = append(tiersToTry, "smart")
	}

	for _, t := range tiersToTry {
		ch, actualModel, err := p.chanManager.SelectBestChannelByTier(t)
		if err == nil {
			if t != tier {
				slog.Warn("⬇️ [降级路由] Tier 降级成功", "requested_tier", tier, "fallback_tier", t, "model", requestedModelID)
			}
			return ch, actualModel, nil
		}
		slog.Debug("🔍 [Tier 查找] 当前 tier 无可用节点，尝试下一级", "tier", t)
	}

	slog.Error("❌ 没有找到匹配该请求的任何可用节点", "requested_model", requestedModelID, "tried_tiers", tiersToTry)
	return nil, "", ErrNoAvailableModel
}

// resolveCapabilityTier 负责执行 2, 3, 4 级优先级解析，全程读内存缓存
func (p *Pipeline) resolveCapabilityTier(ctx context.Context, requestedModelID string) string {
	p.mu.RLock()
	userTier := p.userIntentMap[requestedModelID]
	sysTier := p.sysIntentMap[requestedModelID]
	p.mu.RUnlock()

	// [优先级 2] 用户级意图覆盖（内存缓存）
	if userTier != "" {
		slog.Debug("✅ 命中 [优先级 2] 用户级意图字典（缓存）", "tier", userTier)
		return userTier
	}

	// [优先级 3] 系统级意图兜底（内存缓存）
	if sysTier != "" {
		slog.Debug("✅ 命中 [优先级 3] 系统级意图字典（缓存）", "tier", sysTier)
		return sysTier
	}

	// [优先级 4] 未知模型自动学习与推断
	slog.Warn("⚠️ 遇到未知请求模型，触发自动推断引擎", "model", requestedModelID)
	inferredTier := p.intentInferer.InferUnknownModel(ctx, requestedModelID)
	slog.Info("🤖 自动推断引擎判定结果", "model", requestedModelID, "tier", inferredTier)

	// 写透缓存：将推断结果立即写入 userIntentMap，后续相同请求无需再推断
	p.mu.Lock()
	p.userIntentMap[requestedModelID] = inferredTier
	p.mu.Unlock()

	return inferredTier
}

// checkCustomRoute 检查是否存在针对该请求模型的自定义硬路由（读内存缓存，零 DB 查询）
// 匹配优先级：精确匹配 > 通配符 *
func (p *Pipeline) checkCustomRoute(requestedModelID string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if id, ok := p.customRouteMap[requestedModelID]; ok {
		return id
	}
	return p.wildcardRouteID
}
