package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"polaris-gateway/internal/db"
)

type AccountDetail struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Provider     string  `json:"provider"`
	BaseURL      string  `json:"base_url"`
	Credentials  string  `json:"-"`
	ProjectID    string  `json:"project_id"`
	Location     string  `json:"location"`
	Priority     int     `json:"priority"`
	Balance      float64 `json:"balance"`
	UsedAmount   float64 `json:"used_amount"`
	LimitPercent float64 `json:"limit_percent"`
	ValidFrom    string  `json:"valid_from"`
	ValidTo      string  `json:"valid_to"`
	Status       int     `json:"status"` // 1=正常, 0=手动禁用, -1=熔断/过期
}

type ModelMapping struct {
	Match  string `json:"match"`
	Target string `json:"target"`
}

type RouteDetail struct {
	ID              int            `json:"id"`
	SourceProtocol  string         `json:"source_protocol"`
	TargetProtocol  string         `json:"target_protocol"`
	ModelMappings   string         `json:"-"`
	ModelMappingsParsed []ModelMapping `json:"model_mappings"`
	Status          int            `json:"status"` // 1=正常, 0=禁用
}

type Config struct {
	ListenAddr string `json:"listen_addr"`
	DebugMode  bool   `json:"debug_mode"`
	Breaker    struct {
		InitialCooldownSeconds int `json:"initial_cooldown_seconds"`
		MaxCooldownSeconds     int `json:"max_cooldown_seconds"`
		FailureThreshold       int `json:"failure_threshold"`
		FailureWindowSeconds   int `json:"failure_window_seconds"`
	} `json:"breaker"`
	Providers map[string][]AccountDetail `json:"providers"`
	Routes    []RouteDetail              `json:"routes"`
}

var AppConfig Config

func LoadConfig(yamlFile string, envFile string) error {
	return ReloadFromDB()
}

func ReloadFromDB() error {
	AppConfig = Config{
		Providers: make(map[string][]AccountDetail),
		Routes:    make([]RouteDetail, 0),
	}

	// Load Settings
	err := db.DB().QueryRow("SELECT listen_addr, breaker_initial_cooldown_seconds, breaker_max_cooldown_seconds, breaker_failure_threshold, breaker_failure_window_seconds FROM sys_settings WHERE id = 1").Scan(
		&AppConfig.ListenAddr,
		&AppConfig.Breaker.InitialCooldownSeconds,
		&AppConfig.Breaker.MaxCooldownSeconds,
		&AppConfig.Breaker.FailureThreshold,
		&AppConfig.Breaker.FailureWindowSeconds,
	)
	if err != nil {
		slog.Error("读取系统配置失败，将使用默认值", "error", err)
		AppConfig.ListenAddr = "127.0.0.1:28888"
		AppConfig.Breaker.InitialCooldownSeconds = 60
		AppConfig.Breaker.MaxCooldownSeconds = 3600
		AppConfig.Breaker.FailureThreshold = 3
		AppConfig.Breaker.FailureWindowSeconds = 120
	}

	if AppConfig.ListenAddr == "" {
		AppConfig.ListenAddr = "127.0.0.1:28888"
	}

	// Load Nodes
	rows, err := db.DB().Query("SELECT id, name, provider, base_url, credentials, project_id, location, priority, balance, used_amount, limit_percent, valid_from, valid_to, status FROM sys_nodes")
	if err != nil {
		return fmt.Errorf("读取节点列表失败: %v", err)
	}
	defer rows.Close()

	now := time.Now()

	for rows.Next() {
		var acc AccountDetail
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.Provider, &acc.BaseURL, &acc.Credentials, &acc.ProjectID, &acc.Location, &acc.Priority, &acc.Balance, &acc.UsedAmount, &acc.LimitPercent, &acc.ValidFrom, &acc.ValidTo, &acc.Status); err != nil {
			slog.Error("扫描节点数据失败", "error", err)
			continue
		}

		if acc.Status != 1 {
			continue
		}

		if acc.ValidFrom != "" && acc.ValidFrom != "2000-01-01" {
			tFrom, _ := time.Parse("2006-01-02 15:04:05", acc.ValidFrom)
			if !tFrom.IsZero() && now.Before(tFrom) {
				continue
			}
		}
		if acc.ValidTo != "" && acc.ValidTo != "2099-12-31 23:59:59" {
			tTo, _ := time.Parse("2006-01-02 15:04:05", acc.ValidTo)
			if !tTo.IsZero() && now.After(tTo) {
				continue
			}
		}

		if acc.Balance > 0 {
			limitAmount := acc.Balance * (acc.LimitPercent / 100.0)
			if acc.UsedAmount >= limitAmount {
				continue
			}
		}

		AppConfig.Providers[acc.Provider] = append(AppConfig.Providers[acc.Provider], acc)
	}

	for provider, accounts := range AppConfig.Providers {
		sort.Slice(accounts, func(i, j int) bool {
			return accounts[i].Priority > accounts[j].Priority
		})
		AppConfig.Providers[provider] = accounts
		if len(accounts) > 0 {
			slog.Info("🚦 平台装载完成", "provider", provider, "active_nodes", len(accounts))
		}
	}

	// Load Routes (new schema)
	routeRows, err := db.DB().Query("SELECT id, source_protocol, target_protocol, model_mappings, status FROM sys_routes WHERE status = 1")
	if err != nil {
		slog.Warn("读取路由列表失败(非致命)", "error", err)
	} else {
		defer routeRows.Close()
		for routeRows.Next() {
			var r RouteDetail
			var mappingsJSON string
			if err := routeRows.Scan(&r.ID, &r.SourceProtocol, &r.TargetProtocol, &mappingsJSON, &r.Status); err == nil {
				r.ModelMappings = mappingsJSON
				if mappingsJSON != "" {
					json.Unmarshal([]byte(mappingsJSON), &r.ModelMappingsParsed)
				}
				if r.ModelMappingsParsed == nil {
					r.ModelMappingsParsed = []ModelMapping{}
				}
				AppConfig.Routes = append(AppConfig.Routes, r)
			}
		}
	}
	if len(AppConfig.Routes) > 0 {
		slog.Info("🛤️ 路由表装载完成", "active_routes", len(AppConfig.Routes))
	}

	if len(AppConfig.Providers["vertex"]) == 0 && len(AppConfig.Providers["openai"]) == 0 {
		slog.Warn("无可用物理节点，网关将返回 503 直到添加新节点")
	}

	return nil
}
