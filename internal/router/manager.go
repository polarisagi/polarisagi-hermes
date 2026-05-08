package router

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"polaris-gateway/internal/config"
	"polaris-gateway/internal/db"
)

var (
	nodesMap     map[int]*NodeState
	routesBySource map[string][]config.RouteDetail
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
	TargetProtocol string
	IsProbationRun bool
}

// modelMatches checks if the requested model matches a mapping rule
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

func MatchAndAcquireRoute(ctx context.Context, sourceProtocol, reqModel string) (*MatchedDestination, error) {
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
			if dest, found := tryAcquire(sourceProtocol, reqModel); found {
				return dest, nil
			}
		}
	}
}

func tryAcquire(sourceProtocol, reqModel string) (*MatchedDestination, bool) {
	poolMutex.RLock()
	defer poolMutex.RUnlock()

	// Find routes for this source protocol
	candidateRoutes, exists := routesBySource[sourceProtocol]
	if !exists || len(candidateRoutes) == 0 {
		return nil, false
	}

	type Candidate struct {
		State         *NodeState
		TargetModel   string
		TargetProtocol string
	}
	var validCandidates []Candidate

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

		// Find all available nodes for the target protocol
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
		return nil, false
	}

	// Sort by Priority Descending -> auto load balancing
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
		TargetModel:    chosen.TargetModel,
		TargetProtocol:  chosen.TargetProtocol,
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
