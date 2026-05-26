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
		       priority, weight, concurrency_limit, min_interval_sec, timeout_sec, retry_times, status, 
		       balance, limit_percent, used_amount, IFNULL(valid_from, ''), IFNULL(valid_to, ''), created_at
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
			&p.Priority, &p.Weight, &p.ConcurrencyLimit, &p.MinIntervalSec, &p.TimeoutSec, &p.RetryTimes, &p.Status,
			&p.Balance, &p.LimitPercent, &p.UsedAmount, &p.ValidFrom, &p.ValidTo, &p.CreatedAt,
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

// GetAllSysProviders 获取所有系统预置大厂
func (r *ProviderRepo) GetAllSysProviders(ctx context.Context) ([]domain.SysProvider, error) {
	query := "SELECT provider_id, provider_name, api_protocol, default_concurrency, default_timeout_sec FROM sys_providers"
	rows, err := DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []domain.SysProvider
	for rows.Next() {
		var p domain.SysProvider
		if err := rows.Scan(&p.ProviderID, &p.ProviderName, &p.APIProtocol, &p.DefaultConcurrency, &p.DefaultTimeoutSec); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, nil
}

// GetAllSysProviderAuthModes 获取所有系统鉴权模式
func (r *ProviderRepo) GetAllSysProviderAuthModes(ctx context.Context) ([]domain.SysProviderAuthMode, error) {
	query := "SELECT mode_id, provider_id, mode_name, auth_type, header_name, url_template, required_fields FROM sys_provider_auth_modes"
	rows, err := DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var modes []domain.SysProviderAuthMode
	for rows.Next() {
		var m domain.SysProviderAuthMode
		var reqFields []byte
		if err := rows.Scan(&m.ModeID, &m.ProviderID, &m.ModeName, &m.AuthType, &m.HeaderName, &m.URLTemplate, &reqFields); err != nil {
			return nil, err
		}
		m.RequiredFields = json.RawMessage(reqFields)
		modes = append(modes, m)
	}
	return modes, nil
}

// CreateUserProvider 创建新用户渠道
func (r *ProviderRepo) CreateUserProvider(ctx context.Context, p *domain.UserProvider) error {
	query := `
		INSERT INTO user_providers (
			name, sys_provider_id, sys_auth_mode_id, base_url, auth_credentials,
			priority, weight, concurrency_limit, min_interval_sec, timeout_sec, retry_times, status, balance, limit_percent, used_amount, valid_from, valid_to
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	creds, _ := json.Marshal(p.AuthCredentials)
	if len(creds) == 0 || string(creds) == "null" {
		creds = []byte("{}")
	}

	res, err := DB().ExecContext(ctx, query,
		p.Name, p.SysProviderID, p.SysAuthModeID, p.BaseURL, creds,
		p.Priority, p.Weight, p.ConcurrencyLimit, p.MinIntervalSec, p.TimeoutSec, p.RetryTimes, p.Status, p.Balance, p.LimitPercent, p.UsedAmount, p.ValidFrom, p.ValidTo,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		p.ID = int(id)
	}
	return nil
}

// UpdateUserProvider 更新用户渠道
func (r *ProviderRepo) UpdateUserProvider(ctx context.Context, p *domain.UserProvider) error {
	query := `
		UPDATE user_providers SET
			name = ?, sys_provider_id = ?, sys_auth_mode_id = ?, base_url = ?, auth_credentials = ?,
			priority = ?, weight = ?, concurrency_limit = ?, min_interval_sec = ?, timeout_sec = ?, retry_times = ?, status = ?, balance = ?, limit_percent = ?, valid_from = ?, valid_to = ?
		WHERE id = ?
	`
	creds, _ := json.Marshal(p.AuthCredentials)
	if len(creds) == 0 || string(creds) == "null" {
		creds = []byte("{}")
	}

	_, err := DB().ExecContext(ctx, query,
		p.Name, p.SysProviderID, p.SysAuthModeID, p.BaseURL, creds,
		p.Priority, p.Weight, p.ConcurrencyLimit, p.MinIntervalSec, p.TimeoutSec, p.RetryTimes, p.Status, p.Balance, p.LimitPercent, p.ValidFrom, p.ValidTo,
		p.ID,
	)
	return err
}

// DeleteUserProvider 删除用户渠道
func (r *ProviderRepo) DeleteUserProvider(ctx context.Context, id int) error {
	query := "DELETE FROM user_providers WHERE id = ?"
	_, err := DB().ExecContext(ctx, query, id)
	return err
}
