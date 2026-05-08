-- 新版 sys_routes: 协议到协议路由 + JSON 模型映射
-- source_protocol: 客户端使用的协议 (openai/anthropic/vertex)
-- target_protocol: 转发到上游的协议 (openai/vertex/gemini)
-- model_mappings: JSON 数组，每个元素 {match, target}，支持精确/通配符/前缀匹配

CREATE TABLE IF NOT EXISTS sys_routes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_protocol TEXT NOT NULL DEFAULT 'openai',
    target_protocol TEXT NOT NULL DEFAULT 'openai',
    model_mappings TEXT DEFAULT '[]',
    status INTEGER NOT NULL DEFAULT 1
);
