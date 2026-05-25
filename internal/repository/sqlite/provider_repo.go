package sqlite

import (
	"context"
	"encoding/json"

	"polaris-hermes/internal/domain"
)

// ProviderRepo 处理供应商和用户渠道的数据存取
type ProviderRepo struct{}

func NewProviderRepo() *ProviderRepo {
	return &ProviderRepo{}
}

// GetUserProviders 获取所有未删除的用户配置渠道
func (r *ProviderRepo) GetUserProviders(ctx context.Context) ([]domain.UserProvider, error) {
	query := `
		SELECT id, name, sys_provider_id, sys_auth_mode_id, base_url, auth_credentials, 
		       priority, weight, concurrency_limit, timeout_sec, retry_times, status, 
		       balance, used_amount, created_at
		FROM user_providers
	`
	rows, err := DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []domain.UserProvider
	for rows.Next() {
		var p domain.UserProvider
		var creds []byte
		err := rows.Scan(
			&p.ID, &p.Name, &p.SysProviderID, &p.SysAuthModeID, &p.BaseURL, &creds,
			&p.Priority, &p.Weight, &p.ConcurrencyLimit, &p.TimeoutSec, &p.RetryTimes, &p.Status,
			&p.Balance, &p.UsedAmount, &p.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		p.AuthCredentials = json.RawMessage(creds)
		providers = append(providers, p)
	}
	return providers, nil
}

// GetSysProvider 获取系统预置大厂的底层协议信息
func (r *ProviderRepo) GetSysProvider(ctx context.Context, providerID string) (*domain.SysProvider, error) {
	query := `
		SELECT provider_id, provider_name, api_protocol, default_concurrency, default_timeout_sec
		FROM sys_providers
		WHERE provider_id = ?
	`
	var p domain.SysProvider
	err := DB().QueryRowContext(ctx, query, providerID).Scan(
		&p.ProviderID, &p.ProviderName, &p.APIProtocol, &p.DefaultConcurrency, &p.DefaultTimeoutSec,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetSysProviderAuthMode 获取指定的系统鉴权模式详情
func (r *ProviderRepo) GetSysProviderAuthMode(ctx context.Context, modeID string) (*domain.SysProviderAuthMode, error) {
	query := `
		SELECT mode_id, provider_id, mode_name, auth_type, header_name, url_template, required_fields
		FROM sys_provider_auth_modes
		WHERE mode_id = ?
	`
	var m domain.SysProviderAuthMode
	var reqFields []byte
	err := DB().QueryRowContext(ctx, query, modeID).Scan(
		&m.ModeID, &m.ProviderID, &m.ModeName, &m.AuthType, &m.HeaderName, &m.URLTemplate, &reqFields,
	)
	if err != nil {
		return nil, err
	}
	m.RequiredFields = json.RawMessage(reqFields)
	return &m, nil
}
