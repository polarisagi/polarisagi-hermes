package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"polaris-gateway/internal/db"
)

// AccountDetail 上游节点配置，对应数据库 sys_nodes 表中的一条记录
// 每个节点代表一个可用的上游 API 账号
type AccountDetail struct {
	ID           int     `json:"id"`             // 节点唯一 ID
	Name         string  `json:"name"`           // 节点名称/账号标识（唯一）
	Provider     string  `json:"provider"`        // 上游协议类型: anthropic | openai | google
	// Provider 与路由 target_protocol 必须一致，路由引擎依此选节点
	// google 节点对应 Google Agent Platform (GEAP)，需要 ProjectID + Location + Credentials(API Key)
	BaseURL      string  `json:"base_url"`       // 自定义 API 端点（空则使用官方默认地址）
	Credentials  string  `json:"-"`              // API Key / JSON 凭证（JSON 序列化时隐藏）
	ProjectID    string  `json:"project_id"`     // GCP 项目 ID（仅 Google Agent Platform 节点使用）
	Location     string  `json:"location"`       // GCP 区域（仅 Google Agent Platform 节点使用，默认 global）
	Priority     int     `json:"priority"`       // 优先级，数字越大越优先被选中
	Balance      float64 `json:"balance"`        // 总额度上限（美元）
	UsedAmount   float64 `json:"used_amount"`    // 已使用金额
	LimitPercent float64 `json:"limit_percent"`  // 熔断水位线百分比，超过后自动隔离节点
	ValidFrom    string  `json:"valid_from"`     // 有效期起始时间
	ValidTo      string  `json:"valid_to"`       // 有效期截止时间
	Status       int     `json:"status"`         // 1=正常, 0=手动禁用, -1=熔断/过期
}

// ModelMapping 模型名映射规则，定义请求模型到目标模型的转换关系
// 支持精确匹配、"*" 通配符、前缀通配（如 "gpt-*"）
type ModelMapping struct {
	Match  string `json:"match"`  // 匹配模式: "gpt-4o" 精确匹配 / "*" 全匹配 / "gpt-*" 前缀通配
	Target string `json:"target"` // 目标模型名，匹配后替换为此值
}

// RouteDetail 路由规则配置，定义协议间的转换规则和模型映射
// 每条路由 = 源协议 + 目标协议 + 模型映射表
// 合法组合（与已注册的 translator 严格对应）:
//
//	anthropic → anthropic | google | openai
//	openai    → openai | google
//	google    → google
type RouteDetail struct {
	ID                 int            `json:"id"`                   // 路由唯一 ID
	SourceProtocol     string         `json:"source_protocol"`     // 源协议: anthropic | openai | google
	TargetProtocol     string         `json:"target_protocol"`     // 目标协议: anthropic | openai | google（必须与节点 provider 一致）
	ModelMappings      string         `json:"-"`                   // 模型映射 JSON 原始字符串（数据库存储）
	ModelMappingsParsed []ModelMapping `json:"model_mappings"`     // 解析后的模型映射列表（API 输出）
	Status             int            `json:"status"`              // 1=正常, 0=禁用
}

// Config 全局配置结构，对应数据库 sys_settings + sys_nodes + sys_routes 三张表
// 在网关启动时从 SQLite 加载，运行期间可通过管理后台热重载
type Config struct {
	ListenAddr             string `json:"listen_addr"`              // HTTP 监听地址，如 "127.0.0.1:28888"
	DebugMode              bool   `json:"debug_mode"`               // Debug 日志开关
	GoogleOAuthClientID    string `json:"google_oauth_client_id"`    // Google OAuth 2.0 客户端 ID（可选，留空用 gcloud 内置 ID）
	GoogleOAuthClientSecret string `json:"google_oauth_client_secret"` // Google OAuth 2.0 客户端密钥（可选）
	Breaker    struct {
		InitialCooldownSeconds int `json:"initial_cooldown_seconds"` // 熔断初始冷却时间（秒），每次失败翻倍
		MaxCooldownSeconds     int `json:"max_cooldown_seconds"`     // 熔断最大冷却时间上限（秒）
		FailureThreshold       int `json:"failure_threshold"`        // 连续失败阈值，超过后触发熔断
		FailureWindowSeconds   int `json:"failure_window_seconds"`   // 失败统计窗口（秒）
	} `json:"breaker"`
	Providers map[string][]AccountDetail `json:"providers"` // 按协议类型分组的节点池
	Routes    []RouteDetail              `json:"routes"`    // 路由表
}

var AppConfig Config

// OnConfigReloaded 配置热重载完成回调，由 router 包在初始化时注册
// 确保 admin CRUD 操作后路由引擎能同步更新 routesBySource 和 nodesMap
var OnConfigReloaded func()

// LoadConfig 加载配置（当前实现直接从 SQLite 数据库读取）
func LoadConfig(yamlFile string, envFile string) error {
	return ReloadFromDB()
}

// ReloadFromDB 从 SQLite 数据库重新加载全部配置
// 加载内容包括：系统设置、节点列表、路由表
// 节点过滤规则: 状态必须为启用(1)、在有效期内、且未超过预算限额
// 在以下时机被调用: 1) 网关启动  2) 管理后台修改后热重载
func ReloadFromDB() error {
	AppConfig = Config{
		Providers: make(map[string][]AccountDetail),
		Routes:    make([]RouteDetail, 0),
	}

	// 确保即使中途出错，路由引擎也能感知到最新的 AppConfig 状态
	defer func() {
		if OnConfigReloaded != nil {
			OnConfigReloaded()
		}
	}()

	// Load Settings
	err := db.DB().QueryRow("SELECT listen_addr, breaker_initial_cooldown_seconds, breaker_max_cooldown_seconds, breaker_failure_threshold, breaker_failure_window_seconds, COALESCE(google_oauth_client_id, '') AS google_oauth_client_id, COALESCE(google_oauth_client_secret, '') AS google_oauth_client_secret FROM sys_settings WHERE id = 1").Scan(
		&AppConfig.ListenAddr,
		&AppConfig.Breaker.InitialCooldownSeconds,
		&AppConfig.Breaker.MaxCooldownSeconds,
		&AppConfig.Breaker.FailureThreshold,
		&AppConfig.Breaker.FailureWindowSeconds,
		&AppConfig.GoogleOAuthClientID,
		&AppConfig.GoogleOAuthClientSecret,
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

	if len(AppConfig.Providers) == 0 {
		slog.Warn("无可用物理节点，网关将返回 503 直到添加新节点")
	}

	return nil
}
