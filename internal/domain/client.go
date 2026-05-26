package domain

import "time"

// ClientConfig 表示本地客户端代理配置的备份状态
type ClientConfig struct {
	ClientName    string    `json:"client_name"`
	IsConfigured  bool      `json:"is_configured"`
	BackupContent string    `json:"backup_content"`
	UpdatedAt     time.Time `json:"updated_at"`
}
