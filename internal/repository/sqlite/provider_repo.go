package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/polarisagi/polarisagi-hermes/internal/domain"
)

// ProviderRepo 处理供应商和用户渠道的数据存取
type ProviderRepo struct{}

func NewProviderRepo() *ProviderRepo {
	return &ProviderRepo{}
}

// GetUserProviders 获取所有未删除的用户配置渠道
func (r *ProviderRepo) GetUserProviders(ctx context.Context) ([]domain.UserProvider, error) {
	query := `
		SELECT id, name, provider_id, base_url, auth_credentials, 
		       priority, weight, concurrency_limit, min_interval_sec, timeout_sec, retry_times, status, 
		       balance, limit_percent, used_amount, IFNULL(valid_from, ''), IFNULL(valid_to, ''), created_at
		FROM user_providers
		ORDER BY id ASC
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
			&p.ID, &p.Name, &p.ProviderID, &p.BaseURL, &creds,
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

// GetSysProvider 获取系统预置大厂的基本信息
func (r *ProviderRepo) GetSysProvider(ctx context.Context, providerID string) (*domain.SysProvider, error) {
	query := `
		SELECT provider_id, provider_name
		FROM sys_providers
		WHERE provider_id = ?
	`
	var p domain.SysProvider
	err := DB().QueryRowContext(ctx, query, providerID).Scan(
		&p.ProviderID, &p.ProviderName,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetSysAccessEndpoint 获取指定的系统接入端点详情
func (r *ProviderRepo) GetSysAccessEndpoint(ctx context.Context, endpointID string) (*domain.SysAccessEndpoint, error) {
	query := `
		SELECT endpoint_id, provider_id, display_name, api_protocol, default_base_url, auth_type, auth_header, required_credential_fields, display_order
		FROM sys_access_endpoints
		WHERE endpoint_id = ?
	`
	var e domain.SysAccessEndpoint
	var reqFields []byte
	err := DB().QueryRowContext(ctx, query, endpointID).Scan(
		&e.EndpointID, &e.ProviderID, &e.DisplayName, &e.APIProtocol, &e.DefaultBaseURL, &e.AuthType, &e.AuthHeader, &reqFields, &e.DisplayOrder,
	)
	if err != nil {
		return nil, err
	}
	e.RequiredCredentialFields = json.RawMessage(reqFields)
	return &e, nil
}

// GetAllSysProviders 获取所有系统预置大厂
func (r *ProviderRepo) GetAllSysProviders(ctx context.Context) ([]domain.SysProvider, error) {
	query := "SELECT provider_id, provider_name FROM sys_providers ORDER BY provider_name ASC"
	rows, err := DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []domain.SysProvider
	for rows.Next() {
		var p domain.SysProvider
		if err := rows.Scan(&p.ProviderID, &p.ProviderName); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, nil
}

// GetSysAccessEndpointsByProvider 获取指定厂商的所有接入端点
func (r *ProviderRepo) GetSysAccessEndpointsByProvider(ctx context.Context, providerID string) ([]domain.SysAccessEndpoint, error) {
	query := `SELECT endpoint_id, provider_id, display_name, api_protocol, default_base_url, auth_type, auth_header, required_credential_fields, display_order 
	          FROM sys_access_endpoints WHERE provider_id = ? ORDER BY display_order ASC`
	rows, err := DB().QueryContext(ctx, query, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var endpoints []domain.SysAccessEndpoint
	for rows.Next() {
		var e domain.SysAccessEndpoint
		var reqFields []byte
		if err := rows.Scan(&e.EndpointID, &e.ProviderID, &e.DisplayName, &e.APIProtocol, &e.DefaultBaseURL, &e.AuthType, &e.AuthHeader, &reqFields, &e.DisplayOrder); err != nil {
			return nil, err
		}
		e.RequiredCredentialFields = json.RawMessage(reqFields)
		endpoints = append(endpoints, e)
	}
	return endpoints, nil
}

// GetAllSysAccessEndpoints 获取所有系统接入端点配置
func (r *ProviderRepo) GetAllSysAccessEndpoints(ctx context.Context) ([]domain.SysAccessEndpoint, error) {
	query := "SELECT endpoint_id, provider_id, display_name, api_protocol, default_base_url, auth_type, auth_header, required_credential_fields, display_order FROM sys_access_endpoints ORDER BY display_order ASC"
	rows, err := DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var endpoints []domain.SysAccessEndpoint
	for rows.Next() {
		var e domain.SysAccessEndpoint
		var reqFields []byte
		if err := rows.Scan(&e.EndpointID, &e.ProviderID, &e.DisplayName, &e.APIProtocol, &e.DefaultBaseURL, &e.AuthType, &e.AuthHeader, &reqFields, &e.DisplayOrder); err != nil {
			return nil, err
		}
		e.RequiredCredentialFields = json.RawMessage(reqFields)
		endpoints = append(endpoints, e)
	}
	return endpoints, nil
}

// CreateUserProvider 创建新用户渠道
func (r *ProviderRepo) CreateUserProvider(ctx context.Context, p *domain.UserProvider) error {
	query := `
		INSERT INTO user_providers (
			name, provider_id, base_url, auth_credentials,
			priority, weight, concurrency_limit, min_interval_sec, timeout_sec, retry_times, status, balance, limit_percent, used_amount, valid_from, valid_to
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	creds, _ := json.Marshal(p.AuthCredentials)
	if len(creds) == 0 || string(creds) == "null" {
		creds = []byte("{}")
	}

	res, err := DB().ExecContext(ctx, query,
		p.Name, p.ProviderID, p.BaseURL, creds,
		p.Priority, p.Weight, p.ConcurrencyLimit, p.MinIntervalSec, p.TimeoutSec, p.RetryTimes, p.Status, p.Balance, p.LimitPercent, p.UsedAmount, p.ValidFrom, p.ValidTo,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		p.ID = int(id)
	}

	// 创建渠道成功后，自动从 sys_models 为该厂商批量生成 user_models。
	// 使用 COALESCE 优先采用 sys_model_intent_dict 中正确的 tier 数据。
	// 本地模型（auth_type='none'，如 Ollama/vLLM）跳过自动导入，由用户手动配置模型名。
	if p.ID > 0 {
		_ = r.seedUserModels(ctx, p.ID, p.ProviderID, p.EnableClaude)
	}
	return nil
}

// seedUserModels 在创建渠道后，批量将该厂商的所有系统模型导入为用户模型实例。
// tier 从 sys_model_intent_dict 获取（单一数据源），若无记录则 fallback 到 'smart'。
func (r *ProviderRepo) seedUserModels(ctx context.Context, userProviderID int, providerID string, enableClaude bool) error {
	extraCondition := ""
	if providerID == "gemini_enterprise_agent_platform" && !enableClaude {
		extraCondition = " AND sm.model_id NOT LIKE '%claude%' "
	}
	seedSQL := fmt.Sprintf(`
		INSERT OR IGNORE INTO user_models (user_provider_id, display_name, model_id, capability_tier, is_active)
		SELECT DISTINCT
			? AS user_provider_id,
			sm.display_name,
			sm.model_id,
			COALESCE(
				(SELECT capability_tier FROM sys_model_intent_dict WHERE model_id = sm.model_id),
				'smart'
			) AS capability_tier,
			1 AS is_active
		FROM sys_models sm
		WHERE sm.provider_id = ? %s
	`, extraCondition)
	_, err := DB().ExecContext(ctx, seedSQL, userProviderID, providerID)
	return err
}


// UpdateUserProvider 更新用户渠道
func (r *ProviderRepo) UpdateUserProvider(ctx context.Context, p *domain.UserProvider) error {
	query := `
		UPDATE user_providers SET
			name = ?, provider_id = ?, base_url = ?, auth_credentials = ?,
			priority = ?, weight = ?, concurrency_limit = ?, min_interval_sec = ?, timeout_sec = ?, retry_times = ?, status = ?, balance = ?, limit_percent = ?, valid_from = ?, valid_to = ?
		WHERE id = ?
	`
	creds, _ := json.Marshal(p.AuthCredentials)
	if len(creds) == 0 || string(creds) == "null" {
		creds = []byte("{}")
	}

	_, err := DB().ExecContext(ctx, query,
		p.Name, p.ProviderID, p.BaseURL, creds,
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
