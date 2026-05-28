package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/polarisagi/polarisagi-hermes/internal/domain"
	"github.com/polarisagi/polarisagi-hermes/internal/repository/sqlite"
)

var (
	ErrChannelNotFound = errors.New("channel not found")
	ErrAllChannelsBusy = errors.New("all channels are busy, cooling down, or exhausted")
)

// 节点状态机定义 (从原系统移植)
const (
	StatusIdle      = 0 // 空闲可用
	StatusBusy      = 1 // 并发已满
	StatusCooldown  = 2 // 触发上游 429 正在冷却
	StatusProbation = 3 // 冷却刚结束，处于试用期（只能放入 1 个并发）
	StatusExhausted = 4 // 预算耗尽物理隔离
)

// ActiveChannel 代表内存中一个活跃的用户渠道实例
type ActiveChannel struct {
	Provider  *domain.UserProvider
	Models    []domain.UserModel
	Endpoints map[string]*domain.SysAccessEndpoint // Key: APIProtocol

	mu                    sync.Mutex
	Status                int
	ConcurrentConnections int
	LastAcquireTime       time.Time
	CooldownUntil         time.Time
}

// SysModelCacheInfo 缓存的系统模型元数据
type SysModelCacheInfo struct {
	ActualModelID string
	IsLegacy      bool
	VersionWeight int
}

// Manager 负责在内存中维护所有健康的渠道，并执行强一致性的并发控制与负载均衡
type Manager struct {
	providerRepo *sqlite.ProviderRepo
	modelRepo    *sqlite.ModelRepo

	mu       sync.RWMutex
	channels map[int]*ActiveChannel // Key: UserProviderID
	sysModels map[string]map[string]SysModelCacheInfo // Key: ModelID -> ProviderID -> SysModelCacheInfo
}

func NewManager(providerRepo *sqlite.ProviderRepo, modelRepo *sqlite.ModelRepo) *Manager {
	m := &Manager{
		providerRepo: providerRepo,
		modelRepo:    modelRepo,
		channels:     make(map[int]*ActiveChannel),
		sysModels:    make(map[string]map[string]SysModelCacheInfo),
	}
	go m.cooldownManager()
	return m
}

// filterEndpoints 根据用户提供的凭证精确筛选出对应的系统端点
func filterEndpoints(endpoints []domain.SysAccessEndpoint, credentials []byte) map[string]*domain.SysAccessEndpoint {
	var credsMap map[string]interface{}
	if err := json.Unmarshal(credentials, &credsMap); err != nil {
		credsMap = make(map[string]interface{})
	}

	bestEndpoints := make(map[string]*domain.SysAccessEndpoint)
	maxFieldsCount := make(map[string]int)

	for i := range endpoints {
		ep := &endpoints[i]

		var reqFields []string
		if err := json.Unmarshal(ep.RequiredCredentialFields, &reqFields); err != nil {
			reqFields = []string{}
		}

		satisfied := true
		for _, field := range reqFields {
			if _, exists := credsMap[field]; !exists {
				satisfied = false
				break
			}
		}

		if satisfied {
			currentFieldsCount := len(reqFields)
			if existingCount, exists := maxFieldsCount[ep.APIProtocol]; !exists || currentFieldsCount > existingCount {
				bestEndpoints[ep.APIProtocol] = ep
				maxFieldsCount[ep.APIProtocol] = currentFieldsCount
			}
		}
	}

	return bestEndpoints
}

// Reload 从数据库热加载所有开启的渠道和模型
func (m *Manager) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	providers, err := m.providerRepo.GetUserProviders(ctx)
	if err != nil {
		return err
	}

	models, err := m.modelRepo.GetUserModels(ctx)
	if err != nil {
		return err
	}

	// Load all sys_models for actual_model_id resolution
	globalSysModels := make(map[string]domain.SysModel)
	sysModelsList, err := m.modelRepo.GetSysModels(ctx)
	if err == nil {
		for _, sm := range sysModelsList {
			globalSysModels[sm.ModelID] = sm
		}
	}

	sysModelsMap := make(map[string]map[string]SysModelCacheInfo)
	sysProviderModels, err := m.modelRepo.GetSysProviderModels(ctx)
	if err == nil {
		for _, pm := range sysProviderModels {
			if sysModelsMap[pm.ModelID] == nil {
				sysModelsMap[pm.ModelID] = make(map[string]SysModelCacheInfo)
			}
			isLegacy := false
			versionWeight := 0
			if sm, ok := globalSysModels[pm.ModelID]; ok {
				isLegacy = sm.IsLegacy
				versionWeight = sm.VersionWeight
			}
			sysModelsMap[pm.ModelID][pm.ProviderID] = SysModelCacheInfo{
				ActualModelID: pm.ActualModelID,
				IsLegacy:      isLegacy,
				VersionWeight: versionWeight,
			}
		}
	}

	newChannels := make(map[int]*ActiveChannel)
	for _, p := range providers {
		if p.Status <= 0 {
			continue
		}

		endpointsList, err := m.providerRepo.GetSysAccessEndpointsByProvider(ctx, p.ProviderID)
		if err != nil || len(endpointsList) == 0 {
			slog.Warn("加载系统端点信息失败或无端点，跳过该渠道", "provider", p.Name, "provider_id", p.ProviderID, "error", err)
			continue
		}

		endpointsMap := filterEndpoints(endpointsList, p.AuthCredentials)

		provCopy := p
		ch := &ActiveChannel{
			Provider:  &provCopy,
			Endpoints: endpointsMap,
			Status:    StatusIdle,
		}

		// 检查预算熔断
		if ch.Provider.Balance > 0 && ch.Provider.UsedAmount >= ch.Provider.Balance {
			ch.Status = StatusExhausted
		}

		newChannels[p.ID] = ch
	}

	for _, mod := range models {
		if ch, exists := newChannels[mod.UserProviderID]; exists {
			ch.Models = append(ch.Models, mod)
		}
	}

	// 继承内存状态
	for id, newCh := range newChannels {
		if oldCh, exists := m.channels[id]; exists {
			newCh.mu.Lock()
			oldCh.mu.Lock()
			newCh.Status = oldCh.Status
			newCh.ConcurrentConnections = oldCh.ConcurrentConnections
			newCh.LastAcquireTime = oldCh.LastAcquireTime
			newCh.CooldownUntil = oldCh.CooldownUntil
			oldCh.mu.Unlock()
			newCh.mu.Unlock()
		}
	}

	m.channels = newChannels
	m.sysModels = sysModelsMap
	return nil
}

func (m *Manager) resolveActualModelID(modelID, providerID string) SysModelCacheInfo {
	if providers, ok := m.sysModels[modelID]; ok {
		if info, ok := providers[providerID]; ok {
			return info
		}
	}
	// Fallback to model_id if no specific binding
	return SysModelCacheInfo{
		ActualModelID: modelID,
		IsLegacy:      false,
		VersionWeight: 0,
	}
}

// GetChannelByUserModelID 用于处理用户自定义 1对1 强制路由
func (m *Manager) GetChannelByUserModelID(userModelID int) (*ActiveChannel, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ch := range m.channels {
		for _, mod := range ch.Models {
			if mod.ID == userModelID {
				if ch.Status == StatusExhausted {
					return nil, "", fmt.Errorf("channel exhausted")
				}
				info := m.resolveActualModelID(mod.ModelID, ch.Provider.ProviderID)
				// 强制路由不走负载均衡锁，直接返回
				return ch, info.ActualModelID, nil
			}
		}
	}
	return nil, "", ErrChannelNotFound
}

// SelectBestChannelByTier 核心负载均衡：筛选、排序、CAS 抢占（严格移植自原版防并发 429 逻辑）
func (m *Manager) SelectBestChannelByTier(tier string) (*ActiveChannel, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type Candidate struct {
		Channel       *ActiveChannel
		TargetModelID string
		ModelInfo     SysModelCacheInfo
	}
	var candidates []Candidate

	// 1. 筛选状态满足要求的节点
	for _, ch := range m.channels {
		var matchedModel string
		var matchedInfo SysModelCacheInfo
		matched := false
		for _, mod := range ch.Models {
			if mod.CapabilityTier == tier && mod.IsActive {
				matchedInfo = m.resolveActualModelID(mod.ModelID, ch.Provider.ProviderID)
				matchedModel = matchedInfo.ActualModelID
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		ch.mu.Lock()
		isIdleOrProb := (ch.Status == StatusIdle || ch.Status == StatusProbation)
		if isIdleOrProb {
			if ch.Status == StatusIdle && ch.Provider.ConcurrencyLimit > 0 && ch.ConcurrentConnections >= ch.Provider.ConcurrencyLimit {
				isIdleOrProb = false
			} else if ch.Status == StatusProbation && ch.ConcurrentConnections >= 1 {
				isIdleOrProb = false
			}
		}
		if isIdleOrProb && !ch.LastAcquireTime.IsZero() {
			if ch.Provider.MinIntervalSec > 0 && time.Since(ch.LastAcquireTime) < time.Duration(ch.Provider.MinIntervalSec)*time.Second {
				isIdleOrProb = false
			}
		}
		ch.mu.Unlock()

		if isIdleOrProb {
			candidates = append(candidates, Candidate{
				Channel:       ch,
				TargetModelID: matchedModel,
				ModelInfo:     matchedInfo,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, "", ErrAllChannelsBusy
	}

	// 2. 按是否为Legacy、版本权重、优先级 + LRU 排序
	sort.SliceStable(candidates, func(i, j int) bool {
		// 维度 1：非 Legacy 优先
		legacyI, legacyJ := candidates[i].ModelInfo.IsLegacy, candidates[j].ModelInfo.IsLegacy
		if legacyI != legacyJ {
			return !legacyI // 若 i 非 legacy 且 j 是 legacy，i 排前面
		}

		// 维度 2：VersionWeight 大的优先
		weightI, weightJ := candidates[i].ModelInfo.VersionWeight, candidates[j].ModelInfo.VersionWeight
		if weightI != weightJ {
			return weightI > weightJ
		}

		// 维度 3：Provider 优先级小的优先 (值越小优先级越高)
		pi, pj := candidates[i].Channel.Provider.Priority, candidates[j].Channel.Provider.Priority
		if pi != pj {
			return pi < pj
		}

		// 维度 4：LRU 策略，按上次获取时间排序
		ti, tj := candidates[i].Channel.LastAcquireTime, candidates[j].Channel.LastAcquireTime
		if ti.IsZero() != tj.IsZero() {
			return ti.IsZero()
		}
		return ti.Before(tj)
	})

	// 3. CAS 持锁抢占
	for _, candidate := range candidates {
		ch := candidate.Channel
		ch.mu.Lock()

		if ch.Status != StatusIdle && ch.Status != StatusProbation {
			ch.mu.Unlock()
			continue
		}
		if ch.Status == StatusIdle && ch.Provider.ConcurrencyLimit > 0 && ch.ConcurrentConnections >= ch.Provider.ConcurrencyLimit {
			ch.mu.Unlock()
			continue
		}
		if ch.Status == StatusProbation && ch.ConcurrentConnections >= 1 {
			ch.mu.Unlock()
			continue
		}
		if !ch.LastAcquireTime.IsZero() && ch.Provider.MinIntervalSec > 0 && time.Since(ch.LastAcquireTime) < time.Duration(ch.Provider.MinIntervalSec)*time.Second {
			ch.mu.Unlock()
			continue
		}

		isProbationRun := (ch.Status == StatusProbation)
		ch.ConcurrentConnections++
		if ch.Provider.ConcurrencyLimit > 0 && ch.ConcurrentConnections >= ch.Provider.ConcurrencyLimit {
			if !isProbationRun {
				ch.Status = StatusBusy
			}
		}
		ch.LastAcquireTime = time.Now()
		ch.mu.Unlock()

		slog.Debug("🎯 [负载均衡] 自动抢占目标渠道成功", "channel", ch.Provider.Name, "model", candidate.TargetModelID)
		return ch, candidate.TargetModelID, nil
	}

	return nil, "", ErrAllChannelsBusy
}

// ReleaseChannel 在请求结束或异常时归还并发连接并可能触发结算
func (m *Manager) ReleaseChannel(ch *ActiveChannel) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.ConcurrentConnections > 0 {
		ch.ConcurrentConnections--
	}
	if ch.Status == StatusBusy {
		ch.Status = StatusIdle
	}
}

// cooldownManager 守护协程，定期将 Cooldown 的渠道恢复到 Probation
func (m *Manager) cooldownManager() {
	for {
		time.Sleep(1 * time.Second)
		now := time.Now()

		m.mu.RLock()
		for _, ch := range m.channels {
			ch.mu.Lock()
			if ch.Status == StatusCooldown && now.After(ch.CooldownUntil) {
				ch.Status = StatusProbation
				ch.LastAcquireTime = now // 重置间隔，防止并发抢占
				slog.Info("⏳ [冷却守护] 渠道冷却结束", "channel", ch.Provider.Name, "status", "Probation")
			}
			ch.mu.Unlock()
		}
		m.mu.RUnlock()
	}
}
