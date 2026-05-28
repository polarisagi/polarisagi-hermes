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
		ORDER BY capability_tier ASC, model_id ASC
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
		SELECT model_id, provider_id, actual_model_id, display_name, context_length, max_output_tokens, supports_vision, supports_tools, version_weight, is_legacy
		FROM sys_models
		ORDER BY provider_id ASC, model_id ASC
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
			&m.ModelID, &m.ProviderID, &m.ActualModelID, &m.DisplayName, &m.ContextLength, &m.MaxOutputTokens, &m.SupportsVision, &m.SupportsTools, &m.VersionWeight, &m.IsLegacy,
		)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}


// UpsertSysModel 插入或更新系统模型
func (r *ModelRepo) UpsertSysModel(ctx context.Context, m *domain.SysModel) error {
	query := `
		INSERT INTO sys_models (model_id, provider_id, actual_model_id, display_name, context_length, max_output_tokens, supports_vision, supports_tools, version_weight, is_legacy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_id, model_id) DO UPDATE SET
			actual_model_id = excluded.actual_model_id,
			display_name = excluded.display_name,
			context_length = excluded.context_length,
			max_output_tokens = excluded.max_output_tokens,
			supports_vision = excluded.supports_vision,
			supports_tools = excluded.supports_tools,
			version_weight = excluded.version_weight,
			is_legacy = excluded.is_legacy
	`
	_, err := DB().ExecContext(ctx, query,
		m.ModelID, m.ProviderID, m.ActualModelID, m.DisplayName, m.ContextLength, m.MaxOutputTokens, m.SupportsVision, m.SupportsTools, m.VersionWeight, m.IsLegacy,
	)
	return err
}

// UpdateSysModelLegacyStatus 批量更新特定系列模型的 legacy 状态
func (r *ModelRepo) UpdateSysModelLegacyStatus(ctx context.Context, providerID, modelPrefix string, isLegacy bool) error {
	query := `
		UPDATE sys_models
		SET is_legacy = ?
		WHERE provider_id = ? AND model_id LIKE ?
	`
	_, err := DB().ExecContext(ctx, query, isLegacy, providerID, modelPrefix+"%")
	return err
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
