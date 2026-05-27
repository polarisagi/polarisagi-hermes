-- Migration 009: 架构精简 — 消除模型梯队标签的双重存储
--
-- 问题根因：
--   1. sys_models.capability_tier 与 sys_model_intent_dict 重复存储相同信息，
--      需要手写迁移脚本 008 来保持两张表同步，违反单一数据源原则。
--   2. sys_model_intent_dict / user_model_intent_dict 中的列名 `requested_model_id`
--      语义有误：该字典覆盖客户端模型名和服务端模型名，不只是"请求端"模型 ID。
--
-- 修复内容：
--   1. 重命名 sys_model_intent_dict.requested_model_id → model_id
--   2. 重命名 user_model_intent_dict.requested_model_id → model_id
--   3. 删除 sys_models.capability_tier（单一数据源：只从 sys_model_intent_dict 读取梯队）
--   4. 删除 user_models.capability_tier 与 sys_models 的同步依赖（改为直接查 sys_model_intent_dict）
--
-- SQLite 列重命名需要重建表（RENAME COLUMN 在 3.25+ 支持但需验证环境版本）

-- ─────────────────────────────────────────────────
-- STEP 1: 重建 sys_model_intent_dict（renamed column）
-- ─────────────────────────────────────────────────
CREATE TABLE sys_model_intent_dict_new (
    model_id       VARCHAR PRIMARY KEY,   -- 原 requested_model_id，含客户端和服务端所有模型 ID
    capability_tier VARCHAR NOT NULL
);

INSERT INTO sys_model_intent_dict_new (model_id, capability_tier)
SELECT requested_model_id, capability_tier FROM sys_model_intent_dict;

DROP TABLE sys_model_intent_dict;
ALTER TABLE sys_model_intent_dict_new RENAME TO sys_model_intent_dict;

-- ─────────────────────────────────────────────────
-- STEP 2: 重建 user_model_intent_dict（renamed column）
-- ─────────────────────────────────────────────────
CREATE TABLE user_model_intent_dict_new (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id       VARCHAR NOT NULL UNIQUE,   -- 原 requested_model_id
    capability_tier VARCHAR NOT NULL,
    source         VARCHAR DEFAULT 'manual'
);

INSERT INTO user_model_intent_dict_new (id, model_id, capability_tier, source)
SELECT id, requested_model_id, capability_tier, source FROM user_model_intent_dict;

DROP TABLE user_model_intent_dict;
ALTER TABLE user_model_intent_dict_new RENAME TO user_model_intent_dict;

-- ─────────────────────────────────────────────────
-- STEP 3: 删除 sys_models.capability_tier（冗余字段）
-- 单一数据源统一由 sys_model_intent_dict 提供，不再在 sys_models 存梯队标签。
-- ─────────────────────────────────────────────────
CREATE TABLE sys_models_new (
    model_id          VARCHAR PRIMARY KEY,
    provider_id       VARCHAR NOT NULL,
    display_name      VARCHAR NOT NULL,
    -- capability_tier 已移除，统一从 sys_model_intent_dict 查询
    context_length    INTEGER,
    max_output_tokens INTEGER,
    supports_vision   BOOLEAN DEFAULT 0,
    supports_tools    BOOLEAN DEFAULT 0,
    FOREIGN KEY(provider_id) REFERENCES sys_providers(provider_id)
);

INSERT INTO sys_models_new (model_id, provider_id, display_name, context_length, max_output_tokens, supports_vision, supports_tools)
SELECT model_id, provider_id, display_name, context_length, max_output_tokens, supports_vision, supports_tools
FROM sys_models;

DROP TABLE sys_models;
ALTER TABLE sys_models_new RENAME TO sys_models;
