-- Migration 008: 用 sys_model_intent_dict 的正确数据回填 sys_models.capability_tier
-- 原因：002_seed_sys_providers.sql 中所有 sys_models 的 capability_tier 均被初始化为 'smart'，
--       而 006_seed_sys_model_intent_dict.sql 中已有 570+ 条正确的 model_id → tier 映射。
--       此迁移将两张表对齐，确保 user_models 自动生成时使用正确的 tier 数据。

UPDATE sys_models
SET capability_tier = (
    SELECT capability_tier
    FROM sys_model_intent_dict
    WHERE sys_model_intent_dict.requested_model_id = sys_models.model_id
)
WHERE EXISTS (
    SELECT 1
    FROM sys_model_intent_dict
    WHERE requested_model_id = sys_models.model_id
);
