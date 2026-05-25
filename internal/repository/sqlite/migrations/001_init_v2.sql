-- Polaris Gateway Unified Initial Migration (2026 Architecture)

-- ==========================================
-- LAYER 1: SYSTEM DICTIONARY (READ-ONLY)
-- ==========================================

-- 1. sys_providers
CREATE TABLE IF NOT EXISTS sys_providers (
    provider_id VARCHAR PRIMARY KEY,
    provider_name VARCHAR NOT NULL,
    api_protocol VARCHAR NOT NULL,
    default_concurrency INTEGER DEFAULT 0,
    default_timeout_sec INTEGER DEFAULT 120
);

-- 2. sys_provider_auth_modes
CREATE TABLE IF NOT EXISTS sys_provider_auth_modes (
    mode_id VARCHAR PRIMARY KEY,
    provider_id VARCHAR NOT NULL,
    mode_name VARCHAR NOT NULL,
    auth_type VARCHAR NOT NULL,
    header_name VARCHAR,
    url_template VARCHAR NOT NULL,
    required_fields JSON NOT NULL,
    FOREIGN KEY(provider_id) REFERENCES sys_providers(provider_id)
);

-- 3. sys_models (Objective metadata only)
CREATE TABLE IF NOT EXISTS sys_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id VARCHAR NOT NULL,
    actual_model_id VARCHAR NOT NULL,
    display_name VARCHAR NOT NULL,
    context_length INTEGER,
    max_output_tokens INTEGER,
    supports_vision BOOLEAN DEFAULT 0,
    supports_tools BOOLEAN DEFAULT 0,
    FOREIGN KEY(provider_id) REFERENCES sys_providers(provider_id)
);

-- 4. sys_model_intent_dict (Global mapping of requested model strings to capability intents)
CREATE TABLE IF NOT EXISTS sys_model_intent_dict (
    requested_model_id VARCHAR PRIMARY KEY,
    capability_tier VARCHAR NOT NULL
);

-- ==========================================
-- LAYER 2: USER CONFIGURATION (DYNAMIC)
-- ==========================================

-- 5. user_providers
CREATE TABLE IF NOT EXISTS user_providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR NOT NULL,
    sys_provider_id VARCHAR NOT NULL,
    sys_auth_mode_id VARCHAR NOT NULL,
    base_url VARCHAR NOT NULL,
    auth_credentials JSON NOT NULL,
    priority INTEGER DEFAULT 10,
    weight INTEGER DEFAULT 100,
    concurrency_limit INTEGER DEFAULT 0,
    timeout_sec INTEGER DEFAULT 120,
    retry_times INTEGER DEFAULT 3,
    status INTEGER DEFAULT 1,
    balance REAL DEFAULT 0,
    used_amount REAL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(sys_provider_id) REFERENCES sys_providers(provider_id),
    FOREIGN KEY(sys_auth_mode_id) REFERENCES sys_provider_auth_modes(mode_id)
);

-- 6. user_models
CREATE TABLE IF NOT EXISTS user_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_provider_id INTEGER NOT NULL,
    display_name VARCHAR NOT NULL,
    actual_model_id VARCHAR NOT NULL,
    capability_tier VARCHAR NOT NULL,
    is_active BOOLEAN DEFAULT 1,
    FOREIGN KEY(user_provider_id) REFERENCES user_providers(id)
);

-- 7. user_model_intent_dict (User overrides and auto-learned intents)
CREATE TABLE IF NOT EXISTS user_model_intent_dict (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    requested_model_id VARCHAR NOT NULL UNIQUE,
    capability_tier VARCHAR NOT NULL,
    source VARCHAR DEFAULT 'manual'
);

-- 8. user_custom_routes
CREATE TABLE IF NOT EXISTS user_custom_routes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    requested_model_id VARCHAR NOT NULL,
    target_user_model_id INTEGER NOT NULL,
    is_active BOOLEAN DEFAULT 1,
    FOREIGN KEY(target_user_model_id) REFERENCES user_models(id)
);

-- 9. account_logs & sys_settings
CREATE TABLE IF NOT EXISTS account_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_provider_id INTEGER,
    requested_model_id VARCHAR,
    actual_model_id VARCHAR,
    prompt_tokens INTEGER,
    completion_tokens INTEGER,
    total_tokens INTEGER,
    latency_ms INTEGER,
    status_code INTEGER,
    error_msg TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sys_settings (
    setting_key VARCHAR PRIMARY KEY,
    setting_value TEXT NOT NULL
);


-- ==========================================
-- DML: SYSTEM DATA INJECTION (2026 LATEST)
-- ==========================================

-- (System Providers and Auth Modes omitted for brevity, but they will be fully populated in the final script via code)
-- To keep the DB init script clean, we will implement an InitDB function in the Go code that parses and inserts these default records on first boot, or we can include them here.
-- For this review, the table structures are the focus.
