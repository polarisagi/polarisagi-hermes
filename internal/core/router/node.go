// 节点状态机 + 熔断器 + 计费控制
// 状态转换: Idle → Busy → Idle (成功) / Cooldown → Probation → Busy/Cooldown → Exhausted
// 节点同时管理熔断器（基于连续失败次数）和预算控制（基于费用累加）
package router

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"polaris-gateway/internal/config"

	"golang.org/x/oauth2"
)

// NodeStatus 节点运行时状态枚举
type NodeStatus int

const (
	StatusIdle      NodeStatus = iota // 空闲，可接收新请求
	StatusBusy                        // 繁忙，正在处理请求
	StatusCooldown                    // 冷却中，暂时不可用（熔断惩罚期）
	StatusProbation                   // 试用期，刚从冷却恢复，再次失败将快速回到冷却
	StatusExhausted                   // 已耗尽，预算超限或手动禁用
)

// NodeState 节点运行时状态，包装了静态配置（AccountDetail）和动态状态机
// 每个节点是 goroutine-safe 的，通过内部的 sync.Mutex 保护状态转换
type NodeState struct {
	config.AccountDetail                     // 内嵌静态配置
	TokenSource           oauth2.TokenSource // (可选) 如果配置了 ADC JSON，将在此处生成 OAuth2 TokenSource
	Status                NodeStatus         // 当前状态
	FailureTimestamps     []time.Time        // 失败时间戳列表（用于滑动窗口计数）
	CurrentCooldown       time.Duration      // 当前冷却时长（失败后翻倍增长）
	CooldownUntil         time.Time          // 冷却结束时间
	TotalConsumed         float64            // 当前账期累计消费金额
	ConcurrentConnections int                // 当前并发连接数
	LastAcquireTime       time.Time          // 上次被分配的时间，用于强制最小请求间隔防 RPM 429
	mu                    sync.Mutex         // 保护并发状态修改
}

// checkFailureWindow 检查在滑动窗口内是否有足够多的失败以触发熔断
// 使用 FailureWindowSeconds 作为窗口大小，超过窗口的旧失败记录会被自动清理
func (s *NodeState) checkFailureWindow() bool {
	threshold := config.AppConfig.Breaker.FailureThreshold
	window := time.Duration(config.AppConfig.Breaker.FailureWindowSeconds) * time.Second
	now := time.Now()
	var valid []time.Time
	for _, t := range s.FailureTimestamps {
		if now.Sub(t) <= window {
			valid = append(valid, t)
		}
	}
	s.FailureTimestamps = valid
	return len(s.FailureTimestamps) >= threshold
}

// recordFailureAndCheck 记录一次失败并检查是否需要熔断
func (s *NodeState) recordFailureAndCheck() bool {
	s.FailureTimestamps = append(s.FailureTimestamps, time.Now())
	return s.checkFailureWindow()
}

// UpdateOnSuccess 请求成功后更新节点状态：清除失败记录，重置冷却时间，恢复为 Idle
func (s *NodeState) UpdateOnSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status == StatusExhausted {
		return
	}
	s.FailureTimestamps = nil
	s.CurrentCooldown = time.Duration(config.AppConfig.Breaker.InitialCooldownSeconds) * time.Second
	if s.Concurrency == 0 || s.ConcurrentConnections < s.Concurrency {
		s.Status = StatusIdle
	} else {
		s.Status = StatusBusy
	}
}

// UpdateOnFailure 请求失败后更新节点状态
// 如果处于试用期（Probation）或累积失败数达到阈值，则将节点置为 Cooldown
// 冷却时长每次翻倍（指数退避），直到达到 MaxCooldownSeconds 上限
// 如果在到达 MaxCooldownSeconds 后探路依然失败，则彻底隔离为 Exhausted
func (s *NodeState) UpdateOnFailure(isProbationRun bool, traceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status == StatusExhausted {
		return
	}

	// 始终调用 recordFailureAndCheck 以确保时间戳数组正确记录和清理
	isThresholdReached := s.recordFailureAndCheck()

	if isProbationRun || isThresholdReached {
		maxDuration := time.Duration(config.AppConfig.Breaker.MaxCooldownSeconds) * time.Second

		// 终极死亡判定：如果在 Probation 阶段失败，并且冷却惩罚已经撑满上限，直接物理隔离
		if isProbationRun && s.CurrentCooldown >= maxDuration {
			s.Status = StatusExhausted
			slog.Warn("☠️ [终极隔离] 节点探路反复失败且触达最大冷却上限，永久隔离", "trace_id", traceID, "node", s.Name, "provider", s.Provider)
			return
		}

		s.Status = StatusCooldown
		s.CooldownUntil = time.Now().Add(s.CurrentCooldown)
		slog.Warn("🧊 [节点熔断] 账号进入冷却隔离", "trace_id", traceID, "node", s.Name, "provider", s.Provider, "duration", s.CurrentCooldown.String(), "until", s.CooldownUntil.Format("2006-01-02 15:04:05"), "failure_count", len(s.FailureTimestamps), "budget", fmt.Sprintf("$%.2f/%.2f", s.TotalConsumed, s.Balance))

		s.CurrentCooldown *= 2
		if s.CurrentCooldown > maxDuration {
			s.CurrentCooldown = maxDuration
		}
	} else {
		s.Status = StatusIdle
	}
}

// MarkAsExhausted 强制将节点标记为彻底耗尽（用于 Quota exceeded 等致命错误）
func (s *NodeState) MarkAsExhausted(reason string, traceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != StatusExhausted {
		s.Status = StatusExhausted
		slog.Warn("🚫 [致命隔离] 节点遇到不可恢复错误，物理隔离", "trace_id", traceID, "node", s.Name, "reason", reason)
	}
}

// RecordCost 累加请求费用，并在超过预算上限时自动将节点标记为 Exhausted
func (s *NodeState) RecordCost(cost float64, traceID string) {
	if cost <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalConsumed += cost
	if s.Balance > 0 && s.LimitPercent > 0 {
		usagePercent := (s.TotalConsumed / s.Balance) * 100
		if usagePercent >= s.LimitPercent {
			if s.Status != StatusExhausted {
				s.Status = StatusExhausted
				slog.Warn("🚫 [运行期熔断] 节点触达计费上限，物理隔离", "node", s.Name, "usage_percent", fmt.Sprintf("%.2f%%", usagePercent))
			}
		}
	}
}

// InjectGoogleAuth 统一为 Google/Vertex HTTP 请求注入认证信息 (ADC JSON 或 API Key)
func (s *NodeState) InjectGoogleAuth(req *http.Request) error {
	if s.TokenSource != nil {
		token, err := s.TokenSource.Token()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		return nil
	}

	// API Key 模式：通过 ?key= 查询参数注入，GEAP 和 generic Gemini API 均支持
	q := req.URL.Query()
	q.Set("key", s.Credentials)
	req.URL.RawQuery = q.Encode()
	return nil
}
