CREATE TABLE IF NOT EXISTS account_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	platform TEXT,
	node_name TEXT,
	client_id TEXT,
	method_name TEXT,
	prompt_tokens INTEGER,
	completion_tokens INTEGER,
	cost_usd REAL,
	status_code INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_platform ON account_logs(platform);
CREATE INDEX IF NOT EXISTS idx_node_name ON account_logs(node_name);
CREATE INDEX IF NOT EXISTS idx_client_method ON account_logs(client_id, method_name);
CREATE INDEX IF NOT EXISTS idx_created_at ON account_logs(created_at);
