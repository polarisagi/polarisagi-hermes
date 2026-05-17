// 路由引擎核心：节点池管理 + 路由匹配 + 负载均衡
//
// 架构说明：
//   routesBySource: 按源协议索引的路由表，key 为 "openai"/"google"/"anthropic"
//   nodesMap:       全局节点池，按节点 ID 索引，每个节点有状态机 (Idle/Busy/Cooldown/Probation/Exhausted)
//
// 请求分发流程：
//   1. MatchAndAcquireRoute() 轮询等待可用路由和节点
//   2. tryAcquire() 遍历路由 → 匹配模型映射 → 选择 Idle/Probation 的节点
//   3. 按优先级排序选择最优节点 → 标记为 Busy → 交给协议转换器处理
//   4. 请求完成后调用 ReleaseNode() 归还节点到 Idle 状态
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"

	"golang.org/x/oauth2/google"
)

var (
	nodesMap        map[int]*NodeState          // 全局节点池，key=节点ID
	routesBySource  map[string][]config.RouteDetail // 按源协议索引的路由表
	poolMutex       sync.RWMutex                // 节点池读写锁
	initOnce        sync.Once                   // 确保初始化只执行一次
)

// InitRouter 初始化路由引擎：启动冷却管理器后台协程 + 注册热重载回调 + 加载配置
func InitRouter() {
	initOnce.Do(func() {
		go cooldownManager()
	})
	// 注册回调：当管理员通过后台修改配置后，config.ReloadFromDB() 会自动触发路由引擎重建
	config.OnConfigReloaded = ReloadFromConfig
	ReloadFromConfig()
}

// ReloadFromConfig 从数据库重新加载全部配置并重建路由表和节点池
// 在以下时机调用: 1) 网关启动  2) 管理后台 CRUD 操作触发热重载
// 节点过滤规则: 状态=启用、有效期范围内、预算未超限
func ReloadFromConfig() {
	poolMutex.Lock()
	defer poolMutex.Unlock()

	nodesMap = make(map[int]*NodeState)
	routesBySource = make(map[string][]config.RouteDetail)

	// Build nodes map from all providers
	for _, providers := range config.AppConfig.Providers {
		for _, acc := range providers {
			cycleConsumed := db.GetConsumedSince(acc.Name, acc.ValidFrom)
			state := &NodeState{
				AccountDetail:   acc,
				CurrentCooldown: time.Duration(config.AppConfig.Breaker.InitialCooldownSeconds) * time.Second,
				TotalConsumed:   cycleConsumed,
			}

			// 如果 Credentials 看起来是一个 JSON（通常以 { 开头），尝试解析为 OAuth2 TokenSource
			if strings.HasPrefix(strings.TrimSpace(acc.Credentials), "{") && json.Valid([]byte(acc.Credentials)) {
				creds, err := google.CredentialsFromJSON(context.Background(), []byte(acc.Credentials), "https://www.googleapis.com/auth/cloud-platform")
				if err == nil && creds != nil {
					state.TokenSource = creds.TokenSource
					slog.Info("🔑 [ADC JSON] 成功为节点加载 OAuth2 TokenSource", "node", acc.Name)
				} else {
					slog.Warn("⚠️ [ADC JSON] 节点 Credentials 疑似 JSON，但解析 OAuth2 失败", "node", acc.Name, "err", err)
				}
			}

			if state.Balance > 0 && state.LimitPercent > 0 {
				usagePercent := (state.TotalConsumed / state.Balance) * 100
				if usagePercent >= state.LimitPercent {
					state.Status = StatusExhausted
					slog.Warn("🚫 [启动期熔断] 节点物理隔离", "node", state.Name, "percent", state.LimitPercent, "consumed", state.TotalConsumed, "budget", state.Balance)
				} else {
					state.Status = StatusIdle
				}
			} else {
				state.Status = StatusIdle
			}
			nodesMap[acc.ID] = state
		}
	}

	// Index routes by source_protocol
	for _, route := range config.AppConfig.Routes {
		if route.Status == 1 {
			routesBySource[route.SourceProtocol] = append(routesBySource[route.SourceProtocol], route)
		}
	}

	totalRoutes := 0
	for _, v := range routesBySource {
		totalRoutes += len(v)
	}
	slog.Info("🛤️ Core Router Reloaded", "nodes", len(nodesMap), "routes", totalRoutes)
}

// cooldownManager 冷却守护协程：每秒检查一次，将冷却时间已到的节点
// 从 Cooldown 状态恢复到 Probation (试用) 状态
func cooldownManager() {
	for {
		time.Sleep(1 * time.Second)
		now := time.Now()

		poolMutex.RLock()
		for _, state := range nodesMap {
			state.mu.Lock()
			if state.Status == StatusCooldown && now.After(state.CooldownUntil) {
				state.Status = StatusProbation
				slog.Info("⏳ [冷却守护] 节点冷却结束", "node", state.Name, "provider", state.Provider, "status", "Probation")
			}
			state.mu.Unlock()
		}
		poolMutex.RUnlock()
	}
}

// MatchedDestination 路由匹配结果，包含选中的节点、目标模型和目标协议
type MatchedDestination struct {
	Node           *NodeState // 被选中的目标节点
	TargetModel    string     // 映射后的目标模型名
	TargetProtocol string     // 目标协议类型 (openai/vertex/gemini)
	IsProbationRun bool       // 是否为试用运行 (节点刚从冷却恢复)
}

// modelMatches 检查请求的模型名是否匹配路由映射规则
// 支持三种匹配模式:
//  1. 精确匹配: "gpt-4o" == "gpt-4o"
//  2. 通配符匹配: "*" 匹配所有模型
//  3. 前缀通配: "gpt-*" 匹配 "gpt-4o", "gpt-3.5-turbo" 等
func modelMatches(reqModel string, mapping config.ModelMapping) bool {
	if mapping.Match == "*" {
		return true
	}
	if reqModel == mapping.Match {
		return true
	}
	// Wildcard prefix: "gpt-*" matches "gpt-4o", "gpt-3.5"
	if strings.HasSuffix(mapping.Match, "*") {
		prefix := strings.TrimSuffix(mapping.Match, "*")
		if strings.HasPrefix(reqModel, prefix) {
			return true
		}
	}
	return false
}

// MatchAndAcquireRoute 排队等待并获取可用路由和节点
// 轮询机制: 每 100ms 调用 tryAcquire() 尝试匹配，直到成功或超时
// 返回: 匹配的目标路由信息，或超时/取消错误
// 首次失败时会输出一次 WARN 日志说明原因，方便诊断路由配置问题
func MatchAndAcquireRoute(ctx context.Context, sourceProtocol, reqModel string) (*MatchedDestination, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(120 * time.Second)
	}
	timeoutChan := time.After(time.Until(deadline))
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	diagnosed := false
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutChan:
			return nil, fmt.Errorf("queue timeout")
		case <-ticker.C:
			dest, reason, found := tryAcquire(sourceProtocol, reqModel)
			if found {
				return dest, nil
			}
			if !diagnosed {
				diagnosed = true
				pairs := []any{"source_protocol", sourceProtocol, "req_model", reqModel, "reason", reason}
				poolMutex.RLock()
				if srcRoutes, exist := routesBySource[sourceProtocol]; exist {
					pairs = append(pairs, "route_count", len(srcRoutes))
				} else {
					pairs = append(pairs, "available_protocols", func() []string {
						keys := make([]string, 0, len(routesBySource))
						for k := range routesBySource {
							keys = append(keys, k)
						}
						return keys
					}())
				}
				pairs = append(pairs, "total_nodes", len(nodesMap))
				poolMutex.RUnlock()
				slog.Warn("⚠️ [路由引擎] 当前无法匹配到可用路由，将在后队列等待", pairs...)
			}
		}
	}
}

// tryAcquire 尝试从路由表和节点池中匹配一个可用的目标和节点
// 匹配流程:
//  1. 查找源协议对应的所有路由
//  2. 遍历每条路由的模型映射，找到匹配的规则
//  3. 在目标协议的所有节点中寻找 Idle/Probation 状态的
//  4. 按优先级排序，选择最高优先级的节点
//  5. 将选中节点标记为 Busy
//
// 返回值: dest=匹配结果, reason=失败原因(调试用), found=是否匹配成功
func tryAcquire(sourceProtocol, reqModel string) (dest *MatchedDestination, reason string, found bool) {
	poolMutex.RLock()
	defer poolMutex.RUnlock()

	// Find routes for this source protocol
	candidateRoutes, exists := routesBySource[sourceProtocol]
	if !exists || len(candidateRoutes) == 0 {
		return nil, "no routes for source_protocol", false
	}

	type Candidate struct {
		State         *NodeState
		TargetModel   string
		TargetProtocol string
	}
	var validCandidates []Candidate
	var modelMatchedAnyRoute bool

	// Iterate through matching routes to find model mappings
	for _, route := range candidateRoutes {
		// Try exact match first, then wildcard
		var matchedTargetModel string
		matched := false

		for _, mapping := range route.ModelMappingsParsed {
			if modelMatches(reqModel, mapping) {
				matchedTargetModel = mapping.Target
				matched = true
				break
			}
		}

		// If no model mapping matched, use the first mapping's target as fallback
		// or skip this route
		if !matched {
			if len(route.ModelMappingsParsed) == 0 {
				continue
			}
			// Route with empty match ("*") acts as catch-all
			for _, mapping := range route.ModelMappingsParsed {
				if mapping.Match == "*" || mapping.Match == "" {
					matchedTargetModel = mapping.Target
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		modelMatchedAnyRoute = true

		// 节点选择：node.Provider == route.TargetProtocol 是绑定关系
		// 节点配置时 provider 字段必须与路由目标协议保持一致才能被分配
		for _, state := range nodesMap {
			if state.Provider != route.TargetProtocol {
				continue
			}

			state.mu.Lock()
			isIdleOrProb := (state.Status == StatusIdle || state.Status == StatusProbation)
			state.mu.Unlock()

			if isIdleOrProb {
				validCandidates = append(validCandidates, Candidate{
					State:          state,
					TargetModel:    matchedTargetModel,
					TargetProtocol:  route.TargetProtocol,
				})
			}
		}

		// If we found candidates for this route, break (first matching route wins)
		if len(validCandidates) > 0 {
			break
		}
	}

	if len(validCandidates) == 0 {
		if !modelMatchedAnyRoute && len(candidateRoutes) > 0 {
			return nil, "no model mapping matched", false
		}
		if len(candidateRoutes) > 0 {
			// Routes exist but no model matched or all nodes busy
			totalNodes := 0
			busyNodes := 0
			for _, route := range candidateRoutes {
				for _, state := range nodesMap {
					if state.Provider == route.TargetProtocol {
						totalNodes++
						if state.Status != StatusIdle && state.Status != StatusProbation {
							busyNodes++
						}
					}
				}
			}
			if totalNodes == 0 {
				return nil, "no nodes for target protocol of matching routes", false
			}
			return nil, fmt.Sprintf("all %d nodes busy/exhausted for target protocol", busyNodes), false
		}
		return nil, "no model mapping matched", false
	}

	// 相同优先级的节点随机打乱，实现随机负载均衡
	rand.Shuffle(len(validCandidates), func(i, j int) {
		validCandidates[i], validCandidates[j] = validCandidates[j], validCandidates[i]
	})

	// Sort by Priority Descending -> auto load balancing
	sort.SliceStable(validCandidates, func(i, j int) bool {
		return validCandidates[i].State.Priority > validCandidates[j].State.Priority
	})

	// CAS 抢占：候选筛选时的状态检查与最终 Busy 设置存在时间窗口，必须在持锁内重新验证
	// 严格保证「一节点一并发」：两个 goroutine 同时看到 node1=Idle 时，只有一个能成功
	// 失败方会跳到下一个候选；如所有候选都被抢占，返回 false 让上层 ticker 继续轮询
	// 这是避免单账号并发触发上游 429 的核心机制
	racedCount := 0
	for _, candidate := range validCandidates {
		candidate.State.mu.Lock()
		if candidate.State.Status != StatusIdle && candidate.State.Status != StatusProbation {
			candidate.State.mu.Unlock()
			racedCount++
			continue
		}
		isProbationRun := (candidate.State.Status == StatusProbation)
		candidate.State.Status = StatusBusy
		candidate.State.mu.Unlock()

		slog.Debug("🎯 [负载均衡] 自动选择目标节点",
			"source_protocol", sourceProtocol, "req_model", reqModel,
			"chosen_node", candidate.State.Name, "priority", candidate.State.Priority,
			"target_model", candidate.TargetModel, "is_probation", isProbationRun,
			"raced_skip", racedCount)

		return &MatchedDestination{
			Node:           candidate.State,
			TargetModel:    candidate.TargetModel,
			TargetProtocol: candidate.TargetProtocol,
			IsProbationRun: isProbationRun,
		}, "", true
	}

	// 所有候选都在筛选与 acquire 之间被并发请求抢占
	return nil, fmt.Sprintf("all %d candidates raced by concurrent requests", racedCount), false
}

// ReleaseNode 释放节点：将 Busy 状态的节点恢复为 Idle，供后续请求使用
func ReleaseNode(nodeID int) {
	poolMutex.RLock()
	defer poolMutex.RUnlock()

	if state, exists := nodesMap[nodeID]; exists {
		state.mu.Lock()
		if state.Status == StatusBusy {
			state.Status = StatusIdle
		}
		state.mu.Unlock()
	}
}
