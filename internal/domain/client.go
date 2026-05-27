package domain

// ClientStatus 表示一个客户端软件的当前配置状态（用于 API 响应）
type ClientStatus struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	IsInstalled  bool   `json:"is_installed"`  // 检测到该客户端已安装（配置文件目录存在）
	IsConfigured bool   `json:"is_configured"` // 已被 Polaris-Hermes 注入代理配置
	HasBackup    bool   `json:"has_backup"`    // 存在原始配置备份
	Error        string `json:"error,omitempty"`
}

// ClientBackup 表示单个客户端配置文件的备份记录
type ClientBackup struct {
	Path    string `json:"path"`    // 被备份的原始文件路径
	Content string `json:"content"` // 原始文件内容（base64 or raw string）
}
