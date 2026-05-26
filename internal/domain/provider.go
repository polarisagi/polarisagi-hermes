package domain

import (
	"encoding/json"
	"time"
)

// SysProvider 代表大模型官方厂商（系统内置字典）
type SysProvider struct {
	ProviderID          string `json:"provider_id"`
	ProviderName        string `json:"provider_name"`
	APIProtocol         string `json:"api_protocol"`
	DefaultConcurrency  int    `json:"default_concurrency"`
	DefaultTimeoutSec   int    `json:"default_timeout_sec"`
}

// SysProviderAuthMode 代表某厂商官方提供的一种鉴权/接入模式
type SysProviderAuthMode struct {
	ModeID         string          `json:"mode_id"`
	ProviderID     string          `json:"provider_id"`
	ModeName       string          `json:"mode_name"`
	AuthType       string          `json:"auth_type"` // e.g., bearer, api_key_header, gcp_adc
	HeaderName     string          `json:"header_name"`
	URLTemplate    string          `json:"url_template"`
	RequiredFields json.RawMessage `json:"required_fields"` // JSON array of field names
}

// AuthType constants defining the system-level authentication methods.
const (
	AuthTypeNone     = "none"
	AuthTypeBearer   = "bearer"
	AuthTypeHeader   = "header"
	AuthTypeQuery    = "query"
	AuthTypeADC      = "adc"
	AuthTypeAWSSigV4 = "aws_sigv4"
)

// UserProvider 代表用户实例化的渠道（通道）
type UserProvider struct {
	ID                int             `json:"id"`
	Name              string          `json:"name"`
	SysProviderID     string          `json:"sys_provider_id"`
	SysAuthModeID     string          `json:"sys_auth_mode_id"`
	BaseURL           string          `json:"base_url"`
	AuthCredentials   json.RawMessage `json:"auth_credentials"` // JSON key-value pairs
	Priority          int             `json:"priority"`
	Weight            int             `json:"weight"`
	ConcurrencyLimit  int             `json:"concurrency_limit"`
	MinIntervalSec    int             `json:"min_interval_sec"`
	TimeoutSec        int             `json:"timeout_sec"`
	RetryTimes        int             `json:"retry_times"`
	Status            int             `json:"status"` // 1: 健康, 0: 禁用, -1: 熔断
	Balance           float64         `json:"balance"`
	UsedAmount        float64         `json:"used_amount"`
	CreatedAt         time.Time       `json:"created_at"`
	
	// 以下字段用于内存态状态控制，非数据库字段
	CurrentConcurrent int `json:"-"`
}
