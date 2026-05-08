CREATE TABLE IF NOT EXISTS sys_settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	listen_addr TEXT DEFAULT '127.0.0.1:28888',
	breaker_initial_cooldown_seconds INTEGER DEFAULT 60,
	breaker_max_cooldown_seconds INTEGER DEFAULT 3600,
	breaker_failure_threshold INTEGER DEFAULT 3,
	breaker_failure_window_seconds INTEGER DEFAULT 120
);

INSERT OR IGNORE INTO sys_settings (id, listen_addr, breaker_initial_cooldown_seconds, breaker_max_cooldown_seconds, breaker_failure_threshold, breaker_failure_window_seconds) 
VALUES (1, '127.0.0.1:28888', 60, 3600, 3, 120);

CREATE TABLE IF NOT EXISTS sys_nodes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	provider TEXT NOT NULL,
	base_url TEXT DEFAULT '',
	credentials TEXT NOT NULL,
	project_id TEXT DEFAULT '',
	location TEXT DEFAULT 'global',
	priority INTEGER DEFAULT 0 CHECK (priority >= 0),
	balance REAL DEFAULT 0.0,
	used_amount REAL DEFAULT 0.0,
	limit_percent REAL DEFAULT 90.0,
	valid_from DATETIME DEFAULT CURRENT_TIMESTAMP,
	valid_to DATETIME DEFAULT '2099-12-31 23:59:59',
	status INTEGER DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sys_routes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	match_model TEXT NOT NULL,
	node_id INTEGER NOT NULL,
	target_model TEXT NOT NULL,
	status INTEGER DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (node_id) REFERENCES sys_nodes(id) ON DELETE CASCADE
);
