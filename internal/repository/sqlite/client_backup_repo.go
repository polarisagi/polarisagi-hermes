package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ClientBackupRecord 对应 client_config_backups 表的一行数据
type ClientBackupRecord struct {
	ID              int64
	ClientName      string
	ConfigPath      string
	OriginalContent string // 空字符串表示备份时文件不存在
	BackedUpAt      time.Time
	UpdatedAt       time.Time
}

// ClientBackupRepo 操作 client_config_backups 表
type ClientBackupRepo struct {
	db *sql.DB
}

// NewClientBackupRepo 创建 ClientBackupRepo 实例
func NewClientBackupRepo(db *sql.DB) *ClientBackupRepo {
	return &ClientBackupRepo{db: db}
}

// Upsert 保存或更新指定客户端的备份记录（每个客户端只保留最新一条）
func (r *ClientBackupRepo) Upsert(ctx context.Context, clientName, configPath, content string) error {
	query := `
		INSERT INTO client_config_backups (client_name, config_path, original_content, backed_up_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(client_name) DO UPDATE SET
			config_path      = excluded.config_path,
			original_content = excluded.original_content,
			updated_at       = CURRENT_TIMESTAMP
	`
	_, err := r.db.ExecContext(ctx, query, clientName, configPath, content)
	return err
}

// Get 获取指定客户端的备份记录，找不到时返回 nil, nil
func (r *ClientBackupRepo) Get(ctx context.Context, clientName string) (*ClientBackupRecord, error) {
	query := `
		SELECT id, client_name, config_path, original_content, backed_up_at, updated_at
		FROM client_config_backups
		WHERE client_name = ?
	`
	row := r.db.QueryRowContext(ctx, query, clientName)

	var rec ClientBackupRecord
	err := row.Scan(
		&rec.ID,
		&rec.ClientName,
		&rec.ConfigPath,
		&rec.OriginalContent,
		&rec.BackedUpAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
}

// Exists 判断指定客户端是否存在有效备份
func (r *ClientBackupRepo) Exists(ctx context.Context, clientName string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM client_config_backups WHERE client_name = ?",
		clientName,
	).Scan(&count)
	return count > 0, err
}

// Delete 删除指定客户端的备份记录（恢复后清除）
func (r *ClientBackupRepo) Delete(ctx context.Context, clientName string) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM client_config_backups WHERE client_name = ?",
		clientName,
	)
	return err
}

// GetAll 获取全部备份记录（用于状态面板批量展示）
func (r *ClientBackupRepo) GetAll(ctx context.Context) (map[string]*ClientBackupRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, client_name, config_path, original_content, backed_up_at, updated_at
		FROM client_config_backups
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*ClientBackupRecord)
	for rows.Next() {
		var rec ClientBackupRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.ClientName,
			&rec.ConfigPath,
			&rec.OriginalContent,
			&rec.BackedUpAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result[rec.ClientName] = &rec
	}
	return result, rows.Err()
}
