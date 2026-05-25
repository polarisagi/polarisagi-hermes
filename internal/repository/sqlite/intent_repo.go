package sqlite

import (
	"context"
	"database/sql"
	"polaris-gateway/internal/domain"
)

type IntentRepo struct{}

func NewIntentRepo() *IntentRepo {
	return &IntentRepo{}
}

// GetSysIntent 查询系统内置的全局模型意图
func (r *IntentRepo) GetSysIntent(ctx context.Context, requestedModelID string) (string, error) {
	query := `
		SELECT capability_tier
		FROM sys_model_intent_dict
		WHERE requested_model_id = ?
	`
	var tier string
	err := DB().QueryRowContext(ctx, query, requestedModelID).Scan(&tier)
	if err == sql.ErrNoRows {
		return "", nil // 未找到不算报错
	}
	return tier, err
}

// GetUserIntent 查询用户级别的覆盖/自动学习意图
func (r *IntentRepo) GetUserIntent(ctx context.Context, requestedModelID string) (string, error) {
	query := `
		SELECT capability_tier
		FROM user_model_intent_dict
		WHERE requested_model_id = ?
	`
	var tier string
	err := DB().QueryRowContext(ctx, query, requestedModelID).Scan(&tier)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return tier, err
}

// SaveUserIntent 保存系统自动推断（或者用户手动覆盖）的模型意图分类，具有自学习闭环特性
func (r *IntentRepo) SaveUserIntent(ctx context.Context, intent *domain.UserModelIntentDict) error {
	query := `
		INSERT INTO user_model_intent_dict (requested_model_id, capability_tier, source)
		VALUES (?, ?, ?)
		ON CONFLICT(requested_model_id) DO UPDATE SET
			capability_tier = excluded.capability_tier,
			source = excluded.source
	`
	_, err := DB().ExecContext(ctx, query, intent.RequestedModelID, intent.CapabilityTier, intent.Source)
	return err
}
