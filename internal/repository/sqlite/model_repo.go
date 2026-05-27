package sqlite

import (
	"context"

	"polaris-hermes/internal/domain"
)

type ModelRepo struct{}

func NewModelRepo() *ModelRepo {
	return &ModelRepo{}
}

// GetUserModels 获取所有开启的用户模型
func (r *ModelRepo) GetUserModels(ctx context.Context) ([]domain.UserModel, error) {
	query := `
		SELECT id, user_provider_id, IFNULL(display_name, ''), model_id, capability_tier, is_active
		FROM user_models
		WHERE is_active = 1
	`
	rows, err := DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []domain.UserModel
	for rows.Next() {
		var m domain.UserModel
		err := rows.Scan(
			&m.ID, &m.UserProviderID, &m.DisplayName, &m.ModelID, &m.CapabilityTier, &m.IsActive,
		)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}

// GetSysModels 获取系统内置的所有官方模型物理参数
func (r *ModelRepo) GetSysModels(ctx context.Context) ([]domain.SysModel, error) {
	query := `
		SELECT model_id, provider_id, display_name, context_length, max_output_tokens, supports_vision, supports_tools
		FROM sys_models
	`
	rows, err := DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []domain.SysModel
	for rows.Next() {
		var m domain.SysModel
		err := rows.Scan(
			&m.ModelID, &m.ProviderID, &m.DisplayName, &m.ContextLength, &m.MaxOutputTokens, &m.SupportsVision, &m.SupportsTools,
		)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}

// GetModelEndpointBinding 获取同一模型在某端点的具体请求字符串
func (r *ModelRepo) GetModelEndpointBinding(ctx context.Context, modelID, endpointID string) (*domain.SysModelEndpointBinding, error) {
	query := `
		SELECT model_id, endpoint_id, actual_model_id
		FROM sys_model_endpoint_bindings
		WHERE model_id = ? AND endpoint_id = ?
	`
	var b domain.SysModelEndpointBinding
	err := DB().QueryRowContext(ctx, query, modelID, endpointID).Scan(
		&b.ModelID, &b.EndpointID, &b.ActualModelID,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// UpdateUserModelTier 更新用户模型的意图分级
func (r *ModelRepo) UpdateUserModelTier(ctx context.Context, id int, tier string) error {
	query := `
		UPDATE user_models
		SET capability_tier = ?
		WHERE id = ?
	`
	_, err := DB().ExecContext(ctx, query, tier, id)
	return err
}

// CreateUserModel 为渠道手动添加一个自定义模型（主要用于本地模型如 Ollama/vLLM）
func (r *ModelRepo) CreateUserModel(ctx context.Context, m *domain.UserModel) error {
	query := `
		INSERT INTO user_models (user_provider_id, display_name, model_id, capability_tier, is_active)
		VALUES (?, ?, ?, ?, 1)
	`
	if m.DisplayName == "" {
		m.DisplayName = m.ModelID
	}
	res, err := DB().ExecContext(ctx, query,
		m.UserProviderID, m.DisplayName, m.ModelID, m.CapabilityTier,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		m.ID = int(id)
	}
	return nil
}

// DeleteUserModel 删除指定的用户模型实例
func (r *ModelRepo) DeleteUserModel(ctx context.Context, id int) error {
	query := `DELETE FROM user_models WHERE id = ?`
	_, err := DB().ExecContext(ctx, query, id)
	return err
}

// GetUserModelsByProvider 获取指定渠道下的所有模型（含禁用）
func (r *ModelRepo) GetUserModelsByProvider(ctx context.Context, userProviderID int) ([]domain.UserModel, error) {
	query := `
		SELECT id, user_provider_id, IFNULL(display_name, ''), model_id, capability_tier, is_active
		FROM user_models
		WHERE user_provider_id = ?
		ORDER BY capability_tier, model_id
	`
	rows, err := DB().QueryContext(ctx, query, userProviderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []domain.UserModel
	for rows.Next() {
		var m domain.UserModel
		err := rows.Scan(
			&m.ID, &m.UserProviderID, &m.DisplayName, &m.ModelID, &m.CapabilityTier, &m.IsActive,
		)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}
