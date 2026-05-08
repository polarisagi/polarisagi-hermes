package router

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
)

var (
	nodesMap     map[int]*NodeState
	activeRoutes []config.RouteDetail
	poolMutex    sync.RWMutex
	initOnce     sync.Once
)

func InitRouter() {
	initOnce.Do(func() {
		go cooldownManager()
	})
	ReloadFromConfig()
}

func ReloadFromConfig() {
	poolMutex.Lock()
	defer poolMutex.Unlock()

	nodesMap = make(map[int]*NodeState)
	activeRoutes = make([]config.RouteDetail, 0)

	// Build nodes map
	for _, providers := range config.AppConfig.Providers {
		for _, acc := range providers {
			cycleConsumed := db.GetConsumedSince(acc.Name, acc.ValidFrom)
			state := &NodeState{
				AccountDetail:   acc,
				CurrentCooldown: time.Duration(config.AppConfig.Breaker.InitialCooldownSeconds) * time.Second,
				TotalConsumed:   cycleConsumed,
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

	// Filter active routes
	for _, route := range config.AppConfig.Routes {
		if route.Status == 1 {
			activeRoutes = append(activeRoutes, route)
		}
	}
	slog.Info("🛤️ Core Router Reloaded", "nodes", len(nodesMap), "routes", len(activeRoutes))
}

func cooldownManager() {
	for {
		time.Sleep(1 * time.Second)
		now := time.Now()
		
		poolMutex.RLock()
		for _, state := range nodesMap {
			state.mu.Lock()
			if state.Status == StatusCooldown && now.After(state.CooldownUntil) {
				state.Status = StatusProbation
				slog.Info("⏳ [冷却守护] 节点冷却结束", "node", state.Name, "status", "Probation")
			}
			state.mu.Unlock()
		}
		poolMutex.RUnlock()
	}
}

type MatchedDestination struct {
	Node           *NodeState
	TargetModel    string
	IsProbationRun bool
}

func MatchAndAcquireRoute(ctx context.Context, reqModel string) (*MatchedDestination, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(120 * time.Second)
	}
	timeoutChan := time.After(time.Until(deadline))
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutChan:
			return nil, fmt.Errorf("queue timeout")
		case <-ticker.C:
			if dest, found := tryAcquire(reqModel); found {
				return dest, nil
			}
		}
	}
}

func tryAcquire(reqModel string) (*MatchedDestination, bool) {
	poolMutex.RLock()
	defer poolMutex.RUnlock()

	var candidateRoutes []config.RouteDetail

	for _, r := range activeRoutes {
		if r.MatchModel == reqModel || r.MatchModel == "*" || (len(r.MatchModel) > 1 && r.MatchModel[len(r.MatchModel)-1] == '*' && reqModel[:len(r.MatchModel)-1] == r.MatchModel[:len(r.MatchModel)-1]) {
			candidateRoutes = append(candidateRoutes, r)
		}
	}

	if len(candidateRoutes) == 0 {
		return nil, false
	}

	type Candidate struct {
		State *NodeState
		Route config.RouteDetail
	}
	var validCandidates []Candidate

	for _, cr := range candidateRoutes {
		state, exists := nodesMap[cr.NodeID]
		if !exists {
			continue
		}
		
		state.mu.Lock()
		isIdleOrProb := (state.Status == StatusIdle || state.Status == StatusProbation)
		state.mu.Unlock()

		if isIdleOrProb {
			validCandidates = append(validCandidates, Candidate{State: state, Route: cr})
		}
	}

	if len(validCandidates) == 0 {
		return nil, false
	}

	// Sort by Priority Descending
	sort.Slice(validCandidates, func(i, j int) bool {
		return validCandidates[i].State.Priority > validCandidates[j].State.Priority
	})

	chosen := validCandidates[0]
	
	chosen.State.mu.Lock()
	isProbationRun := (chosen.State.Status == StatusProbation)
	chosen.State.Status = StatusBusy
	chosen.State.mu.Unlock()

	return &MatchedDestination{
		Node:           chosen.State,
		TargetModel:    chosen.Route.TargetModel,
		IsProbationRun: isProbationRun,
	}, true
}

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
