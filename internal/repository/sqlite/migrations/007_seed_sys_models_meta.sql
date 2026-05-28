-- Migration 007: sys_models 物理属性补充
-- 为主流模型补充 context_length / max_output_tokens / supports_vision / supports_tools
-- 仅更新已存在的行，不插入新行（INSERT OR IGNORE 已由 003 负责）

-- ============================================================
-- OpenAI
-- ============================================================
UPDATE sys_models SET context_length = 1047576, max_output_tokens = 32768,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-5.5';
UPDATE sys_models SET context_length = 1047576, max_output_tokens = 32768,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-5.5-pro';
UPDATE sys_models SET context_length = 1047576, max_output_tokens = 32768,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-5.4';
UPDATE sys_models SET context_length = 1047576, max_output_tokens = 32768,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-5.4-pro';
UPDATE sys_models SET context_length = 1047576, max_output_tokens = 16384,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-5.4-mini';
UPDATE sys_models SET context_length = 1047576, max_output_tokens = 8192,   supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-5.4-nano';
UPDATE sys_models SET context_length = 128000,  max_output_tokens = 16384,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-4.1';
UPDATE sys_models SET context_length = 128000,  max_output_tokens = 16384,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-4.1-mini';
UPDATE sys_models SET context_length = 128000,  max_output_tokens = 8192,   supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-4.1-nano';
UPDATE sys_models SET context_length = 128000,  max_output_tokens = 16384,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-4o';
UPDATE sys_models SET context_length = 128000,  max_output_tokens = 16384,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'gpt-4o-mini';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 100000, supports_vision = 0, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'o1';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 100000, supports_vision = 0, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'o1-pro';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 100000, supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'o3';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 100000, supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'o3-pro';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 65536,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'o3-mini';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 100000, supports_vision = 1, supports_tools = 1 WHERE provider_id = 'openai' AND model_id = 'o4-mini';

-- ============================================================
-- Anthropic
-- ============================================================
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 32000,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'anthropic' AND model_id = 'claude-opus-4-7';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 32000,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'anthropic' AND model_id = 'claude-opus-4-6';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 32000,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'anthropic' AND model_id = 'claude-opus-4-5';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 64000,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'anthropic' AND model_id = 'claude-sonnet-4-6';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 64000,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'anthropic' AND model_id = 'claude-sonnet-4-5';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 32000,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'anthropic' AND model_id = 'claude-haiku-4-5';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 8192,   supports_vision = 1, supports_tools = 1 WHERE provider_id = 'anthropic' AND model_id = 'claude-3-5-haiku-latest';
UPDATE sys_models SET context_length = 200000,  max_output_tokens = 8192,   supports_vision = 1, supports_tools = 1 WHERE provider_id = 'anthropic' AND model_id = 'claude-3-7-sonnet-latest';

-- ============================================================
-- Google / Gemini
-- ============================================================
UPDATE sys_models SET context_length = 1000000, max_output_tokens = 65536,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'google' AND model_id = 'gemini-3.1-pro-preview';
UPDATE sys_models SET context_length = 1000000, max_output_tokens = 65536,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'google' AND model_id = 'gemini-3.1-pro-preview-customtools';
UPDATE sys_models SET context_length = 1000000, max_output_tokens = 65536,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'google' AND model_id = 'gemini-3.1-flash';
UPDATE sys_models SET context_length = 1000000, max_output_tokens = 32768,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'google' AND model_id = 'gemini-3.1-flash-lite';
UPDATE sys_models SET context_length = 2097152, max_output_tokens = 65536,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'google' AND model_id = 'gemini-2.5-pro';
UPDATE sys_models SET context_length = 1048576, max_output_tokens = 65536,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'google' AND model_id = 'gemini-2.5-flash';
UPDATE sys_models SET context_length = 1048576, max_output_tokens = 32768,  supports_vision = 1, supports_tools = 1 WHERE provider_id = 'google' AND model_id = 'gemini-2.5-flash-lite';
UPDATE sys_models SET context_length = 1048576, max_output_tokens = 8192,   supports_vision = 1, supports_tools = 1 WHERE provider_id = 'google' AND model_id = 'gemini-3.0-flash';
UPDATE sys_models SET context_length = 1048576, max_output_tokens = 8192,   supports_vision = 1, supports_tools = 1 WHERE provider_id = 'google' AND model_id = 'gemini-3.0-pro';

-- ============================================================
-- DeepSeek
-- ============================================================
UPDATE sys_models SET context_length = 128000,  max_output_tokens = 16384,  supports_vision = 0, supports_tools = 1 WHERE provider_id = 'deepseek' AND model_id = 'deepseek-v4-flash';
UPDATE sys_models SET context_length = 128000,  max_output_tokens = 16384,  supports_vision = 0, supports_tools = 1 WHERE provider_id = 'deepseek' AND model_id = 'deepseek-v4-pro';

-- ============================================================
-- Groq (LPU 极速推理，低延迟)
-- ============================================================
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 8192,   supports_vision = 0, supports_tools = 1 WHERE provider_id = 'groq' AND model_id = 'qwen/qwen3-32b';

-- ============================================================
-- Mistral
-- ============================================================
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 4096,   supports_vision = 0, supports_tools = 1 WHERE provider_id = 'mistral' AND model_id = 'mistral-large-latest';
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 4096,   supports_vision = 0, supports_tools = 1 WHERE provider_id = 'mistral' AND model_id = 'mistral-small-latest';
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 4096,   supports_vision = 0, supports_tools = 1 WHERE provider_id = 'mistral' AND model_id = 'codestral-latest';
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 4096,   supports_vision = 0, supports_tools = 1 WHERE provider_id = 'mistral' AND model_id = 'devstral-medium-latest';

-- ============================================================
-- Moonshot / Kimi
-- ============================================================
UPDATE sys_models SET context_length = 1000000, max_output_tokens = 16384,  supports_vision = 0, supports_tools = 1 WHERE provider_id = 'moonshot' AND model_id = 'kimi-k2.5';
UPDATE sys_models SET context_length = 1000000, max_output_tokens = 16384,  supports_vision = 0, supports_tools = 1 WHERE provider_id = 'moonshot' AND model_id = 'kimi-k2.6';
UPDATE sys_models SET context_length = 1000000, max_output_tokens = 32768,  supports_vision = 0, supports_tools = 0 WHERE provider_id = 'moonshot' AND model_id = 'kimi-k2-thinking';

-- ============================================================
-- Dashscope (阿里通义千问)
-- ============================================================
UPDATE sys_models SET context_length = 1000000, max_output_tokens = 16384,  supports_vision = 0, supports_tools = 1 WHERE provider_id = 'dashscope' AND model_id = 'qwen-3-max';
UPDATE sys_models SET context_length = 1000000, max_output_tokens = 16384,  supports_vision = 0, supports_tools = 1 WHERE provider_id = 'dashscope' AND model_id = 'qwen-3-plus';
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 8192,   supports_vision = 0, supports_tools = 0 WHERE provider_id = 'dashscope' AND model_id = 'qwq-32b';

-- ============================================================
-- SiliconFlow
-- ============================================================
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 8192,   supports_vision = 0, supports_tools = 1 WHERE provider_id = 'siliconflow' AND model_id = 'deepseek-ai/DeepSeek-V4';

-- ============================================================
-- Ollama (本地部署参考值)
-- ============================================================
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 8192,   supports_vision = 0, supports_tools = 1 WHERE provider_id = 'ollama' AND model_id = 'qwen3:32b';
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 8192,   supports_vision = 0, supports_tools = 1 WHERE provider_id = 'ollama' AND model_id = 'llama4:70b';
UPDATE sys_models SET context_length = 131072,  max_output_tokens = 32768,  supports_vision = 0, supports_tools = 0 WHERE provider_id = 'ollama' AND model_id = 'deepseek-r2:32b';
