package sqlite

import (
	"context"
	"database/sql"
	"github.com/polarisagi/polarisagi-hermes/internal/domain"
)

type IntentRepo struct{}

func NewIntentRepo() *IntentRepo {
	return &IntentRepo{}
}

// GetSysIntent 查询系统内置的全局模型意图字典
// 同时覆盖客户端模型名（gpt-4o、claude-sonnet）和服务端模型名（deepseek-v4-flash、gemini-pro）
func (r *IntentRepo) GetSysIntent(ctx context.Context, modelID string) (string, error) {
	query := `
		SELECT capability_tier
		FROM sys_model_intent_dict
		WHERE model_id = ?
	`
	var tier string
	err := DB().QueryRowContext(ctx, query, modelID).Scan(&tier)
	if err == sql.ErrNoRows {
		return "", nil // 未找到不算报错
	}
	return tier, err
}

// GetUserIntent 查询用户级别的覆盖意图（优先级高于系统字典）
func (r *IntentRepo) GetUserIntent(ctx context.Context, modelID string) (string, error) {
	query := `
		SELECT capability_tier
		FROM user_model_intent_dict
		WHERE model_id = ?
	`
	var tier string
	err := DB().QueryRowContext(ctx, query, modelID).Scan(&tier)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return tier, err
}

// SaveUserIntent 保存用户手动配置或自动推断的意图分类（具有自学习闭环特性）
func (r *IntentRepo) SaveUserIntent(ctx context.Context, intent *domain.UserModelIntentDict) error {
	query := `
		INSERT INTO user_model_intent_dict (model_id, capability_tier, source)
		VALUES (?, ?, ?)
		ON CONFLICT(model_id) DO UPDATE SET
			capability_tier = excluded.capability_tier,
			source = excluded.source
	`
	_, err := DB().ExecContext(ctx, query, intent.ModelID, intent.CapabilityTier, intent.Source)
	return err
}

// SaveSysIntent 保存系统推断的意图分类
func (r *IntentRepo) SaveSysIntent(ctx context.Context, intent *domain.UserModelIntentDict) error {
	query := `
		INSERT INTO sys_model_intent_dict (model_id, capability_tier, source)
		VALUES (?, ?, ?)
		ON CONFLICT(model_id) DO UPDATE SET
			capability_tier = excluded.capability_tier,
			source = excluded.source
	`
	_, err := DB().ExecContext(ctx, query, intent.ModelID, intent.CapabilityTier, intent.Source)
	return err
}

// GetAllSysIntents 全量加载系统意图字典到内存 map（用于 Pipeline 热重载缓存）
func (r *IntentRepo) GetAllSysIntents(ctx context.Context) (map[string]string, error) {
	rows, err := DB().QueryContext(ctx, `SELECT model_id, capability_tier FROM sys_model_intent_dict ORDER BY model_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, nil
}

// GetAllUserIntents 全量加载用户意图字典到内存 map（用于 Pipeline 热重载缓存）
func (r *IntentRepo) GetAllUserIntents(ctx context.Context) (map[string]string, error) {
	rows, err := DB().QueryContext(ctx, `SELECT model_id, capability_tier FROM user_model_intent_dict ORDER BY model_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, nil
}

// DeleteUserIntent 删除用户手动创建的意图覆盖条目
func (r *IntentRepo) DeleteUserIntent(ctx context.Context, modelID string) error {
	query := `DELETE FROM user_model_intent_dict WHERE model_id = ?`
	_, err := DB().ExecContext(ctx, query, modelID)
	return err
}
