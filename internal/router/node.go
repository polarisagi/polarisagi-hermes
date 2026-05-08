package router

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"polaris-gateway/internal/config"
)

type NodeStatus int

const (
	StatusIdle NodeStatus = iota
	StatusBusy
	StatusCooldown
	StatusProbation
	StatusExhausted
)

type NodeState struct {
	config.AccountDetail
	Status            NodeStatus
	FailureTimestamps []time.Time
	CurrentCooldown   time.Duration
	CooldownUntil     time.Time
	TotalConsumed     float64
	mu                sync.Mutex
}

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

func (s *NodeState) recordFailureAndCheck() bool {
	s.FailureTimestamps = append(s.FailureTimestamps, time.Now())
	return s.checkFailureWindow()
}

func (s *NodeState) UpdateOnSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status == StatusExhausted {
		return
	}
	s.FailureTimestamps = nil
	s.CurrentCooldown = time.Duration(config.AppConfig.Breaker.InitialCooldownSeconds) * time.Second
	s.Status = StatusIdle
}

func (s *NodeState) UpdateOnFailure(isProbationRun bool, traceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status == StatusExhausted {
		return
	}
	if isProbationRun || s.recordFailureAndCheck() {
		s.Status = StatusCooldown
		s.CooldownUntil = time.Now().Add(s.CurrentCooldown)
		slog.Warn("🧊 [节点熔断] 账号进入冷却隔离", "trace_id", traceID, "node", s.Name, "provider", s.Provider, "duration", s.CurrentCooldown.String(), "until", s.CooldownUntil.Format("2006-01-02 15:04:05"), "failure_count", len(s.FailureTimestamps), "budget", fmt.Sprintf("$%.2f/%.2f", s.TotalConsumed, s.Balance))

		s.CurrentCooldown *= 2
		maxDuration := time.Duration(config.AppConfig.Breaker.MaxCooldownSeconds) * time.Second
		if s.CurrentCooldown > maxDuration {
			s.CurrentCooldown = maxDuration
		}
	} else {
		s.Status = StatusIdle
	}
}

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
