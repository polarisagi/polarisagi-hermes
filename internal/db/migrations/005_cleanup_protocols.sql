-- 005: 统一协议标识符，清理历史遗留的 "gemini" provider/protocol
--
-- 节点/路由 provider 合法值: anthropic | openai | google
-- 路由 source/target_protocol 合法值:
--   source: anthropic | openai | google
--   target:
--     anthropic source → anthropic | google | openai
--     openai    source → openai | google
--     google    source → google

-- 1. 节点：将 provider='gemini' 统一迁移到 'google'
UPDATE sys_nodes SET provider = 'google' WHERE provider = 'gemini';

-- 2. 路由：source_protocol='gemini' → 'google'
UPDATE sys_routes SET source_protocol = 'google' WHERE source_protocol = 'gemini';

-- 3. 路由：target_protocol='gemini' → 'google'
--    (旧的 AI Studio 节点现已统一到 Google Agent Platform)
UPDATE sys_routes SET target_protocol = 'google' WHERE target_protocol = 'gemini';

-- 4. 删除非法路由组合（路由引擎中不存在对应的 translator，永远无法命中）
DELETE FROM sys_routes WHERE NOT (
    (source_protocol = 'anthropic' AND target_protocol IN ('anthropic', 'google', 'openai')) OR
    (source_protocol = 'openai'    AND target_protocol IN ('openai', 'google'))               OR
    (source_protocol = 'google'    AND target_protocol = 'google')
);
