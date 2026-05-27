-- 客户端配置备份表
-- 每个客户端最多保存一条"当前有效备份"（upsert 语义）
CREATE TABLE IF NOT EXISTS client_config_backups (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    client_name     TEXT    NOT NULL UNIQUE,  -- 客户端唯一标识，如 claude_code
    config_path     TEXT    NOT NULL,          -- 被备份的原始文件完整路径
    original_content TEXT   NOT NULL DEFAULT '', -- 原始文件内容（空字符串表示文件不存在）
    backed_up_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
