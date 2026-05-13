-- 将协议标识符 "vertex" 统一重命名为 "google"（Google Agent Platform）
-- 官方文档：https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest
UPDATE sys_nodes SET provider = 'google' WHERE provider = 'vertex';
UPDATE sys_routes SET source_protocol = 'google' WHERE source_protocol = 'vertex';
UPDATE sys_routes SET target_protocol = 'google' WHERE target_protocol = 'vertex';
