-- ============================================================================
-- Migration 006: Seed sys_model_intent_dict
-- Maps requested_model_id → capability_tier for the intelligent routing pipeline.
--
-- Tiers:
--   "reasoning" — deep reasoning / chain-of-thought models (o1, o3, o4, R1, QwQ, etc.)
--   "fast"      — lightweight / cost-efficient models (mini, haiku, flash, nano, lite, small, etc.)
--   "smart"     — flagship / powerful models (sonnet, opus, pro, gpt-4o, gpt-5.5, etc.)
-- ============================================================================

-- ============================================================
-- TIER: reasoning
-- Deep reasoning, chain-of-thought, thinking models
-- ============================================================
INSERT OR IGNORE INTO sys_model_intent_dict (model_id, capability_tier) VALUES

-- OpenAI o-series reasoning models
('o1', 'reasoning'),
('o1-pro', 'reasoning'),
('o3', 'reasoning'),
('o3-mini', 'reasoning'),
('o3-mini-high', 'reasoning'),
('o3-pro', 'reasoning'),
('o3-deep-research', 'reasoning'),
('o4', 'reasoning'),
('o4-mini', 'reasoning'),
('o4-mini-high', 'reasoning'),
('o4-mini-deep-research', 'reasoning'),

-- OpenAI o-series via gateway prefixes
('openai/o1', 'reasoning'),
('openai/o3', 'reasoning'),
('openai/o3-mini', 'reasoning'),
('openai/o3-mini-high', 'reasoning'),
('openai/o3-pro', 'reasoning'),
('openai/o3-deep-research', 'reasoning'),
('openai/o4', 'reasoning'),
('openai/o4-mini', 'reasoning'),
('openai/o4-mini-high', 'reasoning'),
('openai/o4-mini-deep-research', 'reasoning'),

-- DeepSeek reasoning models
('deepseek-v4-pro', 'reasoning'),
('deepseek-ai/DeepSeek-R1', 'reasoning'),
('deepseek-ai/DeepSeek-R1-0528-TEE', 'reasoning'),
('deepseek-ai/DeepSeek-R1-Distill-Llama-70B', 'reasoning'),
('deepseek/deepseek-r1', 'reasoning'),
('deepseek/deepseek-r1-0528', 'reasoning'),
('deepseek.r1-v1:0', 'reasoning'),
('us.deepseek.r1-v1:0', 'reasoning'),
('deepseek-r2:32b', 'reasoning'),

-- Kimi thinking/reasoning models
('kimi-k2-thinking', 'reasoning'),
('kimi-k2-thinking-turbo', 'reasoning'),

-- QwQ reasoning models
('qwq-32b', 'reasoning'),

-- Qwen thinking models
('Qwen/Qwen3-235B-A22B-Thinking-2507', 'reasoning'),
('qwen3-235b-a22b-thinking-2507', 'reasoning'),
('qwen/qwen3-vl-30b-a3b-thinking', 'reasoning'),
('qwen/qwen3-vl-8b-thinking', 'reasoning'),
('alibaba/qwen3-vl-thinking', 'reasoning'),

-- ERNIE thinking model
('ernie-5.0-thinking-preview', 'reasoning'),

-- Perplexity reasoning
('sonar-reasoning', 'reasoning'),

-- OpenAI GPT thinking variants
('gpt-5.1-thinking', 'reasoning'),
('openai/gpt-5.1-thinking', 'reasoning'),

-- Arcee thinking
('arcee-ai/trinity-large-thinking', 'reasoning'),
('arcee-ai/trinity-large-thinking:free', 'reasoning'),

-- Nemotron reasoning
('nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free', 'reasoning'),

-- TNG Chimera reasoning
('tngtech/DeepSeek-TNG-R1T2-Chimera', 'reasoning'),
('tngtech/R1T2-Chimera-Speed', 'reasoning'),

-- Tongyi deep research
('alibaba/tongyi-deepresearch-30b-a3b', 'reasoning');


-- ============================================================
-- TIER: fast
-- Lightweight, cost-efficient, small parameter models
-- ============================================================
INSERT OR IGNORE INTO sys_model_intent_dict (model_id, capability_tier) VALUES

-- OpenAI mini/nano models
('gpt-4o-mini', 'fast'),
('gpt-4.1-mini', 'fast'),
('gpt-4.1-nano', 'fast'),
('gpt-5-mini', 'fast'),
('gpt-5-nano', 'fast'),
('gpt-5.4-mini', 'fast'),
('gpt-5.4-nano', 'fast'),
('openai/gpt-4o-mini', 'fast'),
('openai/gpt-4.1-mini', 'fast'),
('openai/gpt-4.1-nano', 'fast'),
('openai/gpt-5-mini', 'fast'),
('openai/gpt-5-nano', 'fast'),
('openai/gpt-5.4-mini', 'fast'),
('openai/gpt-5.4-nano', 'fast'),
('openai/gpt-audio-mini', 'fast'),
('~openai/gpt-mini-latest', 'fast'),
('raptor-mini', 'fast'),

-- OpenAI GPT 3.5 turbo (legacy, but fast tier)

-- OpenAI OSS smaller models
('openai/gpt-oss-20b', 'fast'),

-- Claude haiku models (fast tier)
('claude-3-5-haiku', 'fast'),
('claude-3-5-haiku-latest', 'fast'),
('claude-3.5-haiku', 'fast'),
('claude-haiku-4-5', 'fast'),
('claude-haiku-4-5-20251001', 'fast'),
('claude-haiku-4.5', 'fast'),
('anthropic/claude-3-haiku', 'fast'),
('anthropic/claude-3.5-haiku', 'fast'),
('anthropic/claude-haiku-4-5', 'fast'),
('anthropic/claude-haiku-4.5', 'fast'),
('~anthropic/claude-haiku-latest', 'fast'),

-- Gemini flash/lite models
('gemini-2.5-flash-lite', 'fast'),
('gemini-3-flash-preview', 'fast'),
('gemini-3-flash', 'fast'),
('gemini-3.0-flash', 'fast'),
('gemini-3.1-flash', 'fast'),
('gemini-3.1-flash-lite', 'fast'),
('gemini-3.1-flash-lite-preview', 'fast'),
('gemini-3.5-flash', 'fast'),
('gemini-flash-latest', 'fast'),
('gemini-flash-lite-latest', 'fast'),
('google/gemini-3-flash-preview', 'fast'),
('google/gemini-3-flash', 'fast'),
('google/gemini-3.1-flash-lite', 'fast'),
('google/gemini-3.1-flash-lite-preview', 'fast'),
('google/gemini-3.5-flash', 'fast'),
('~google/gemini-flash-latest', 'fast'),
('qwen3.5-flash', 'fast'),
('qwen3.6-flash', 'fast'),
('qwen/qwen3.6-flash', 'fast'),
('alibaba/qwen3.5-flash', 'fast'),

-- Step flash models
('step-3.5-flash', 'fast'),
('step-3.5-flash-2603', 'fast'),
('stepfun/step-3.5-flash', 'fast'),
('stepfun-ai/Step-3.5-Flash', 'fast'),

-- Amazon Nova lite/micro
('amazon.nova-lite-v1:0', 'fast'),
('amazon.nova-micro-v1:0', 'fast'),
('amazon/nova-lite-v1', 'fast'),
('amazon/nova-micro-v1', 'fast'),

-- GLM flash/turbo/air (lightweight)
('glm-4.5-flash', 'fast'),
('glm-4.5-air', 'fast'),
('glm-4.7-flash', 'fast'),
('glm-4.7-flashx', 'fast'),
('glm-5-turbo', 'fast'),
('glm-5v-turbo', 'fast'),
('z-ai/glm-4.5-air', 'fast'),
('z-ai/glm-4.5-air:free', 'fast'),
('z-ai/glm-5-turbo', 'fast'),
('z-ai/glm-5v-turbo', 'fast'),
('zai/glm-4.5-air', 'fast'),
('zai/glm-4.5-air:free', 'fast'),
('zai/glm-4.6v-flash', 'fast'),
('zai/glm-5-turbo', 'fast'),
('zai/glm-5v-turbo', 'fast'),
('zai-org-glm-4.7-flash', 'fast'),
('olafangensan-glm-4.7-flash-heretic', 'fast'),

-- Mistral small/ministral (lightweight)
('mistral-small-latest', 'fast'),
('ministral-3b-latest', 'fast'),
('ministral-8b-latest', 'fast'),
('magistral-small', 'fast'),
('mistralai/devstral-small', 'fast'),
('mistralai/mistral-small', 'fast'),
('mistralai/mistral-saba', 'fast'),
('mistral/codestral', 'fast'),
('mistral/devstral-small', 'fast'),
('mistral/ministral-3b', 'fast'),
('mistral/ministral-8b', 'fast'),
('mistral.ministral-3-14b-instruct', 'fast'),
('mistral.ministral-3-3b-instruct', 'fast'),
('mistral.ministral-3-8b-instruct', 'fast'),

-- Small open-source models (3b/4b/7b/8b/14b)
('llama3.1-8b', 'fast'),
('meta/llama-4-8b-instruct', 'fast'),
('google.gemma-3-4b-it', 'fast'),
('unsloth/gemma-3-4b-it', 'fast'),
('@cf/google/gemma-7b-it', 'fast'),
('@cf/mistral/mistral-7b-instruct-v0.1', 'fast'),
('@cf/ibm-granite/granite-4.0-h-micro', 'fast'),
('ibm-granite/granite-4.1-8b', 'fast'),
('Qwen/Qwen3-14B', 'fast'),
('qwen/qwen3-14b', 'fast'),
('qwen/qwen3-8b', 'fast'),
('Baichuan-M1-14B', 'fast'),
('NousResearch/Hermes-4-14B', 'fast'),
('qwen3-4b', 'fast'),
('Qwen/Qwen3Guard-Gen-0.6B', 'fast'),

-- MiMo flash models
('mimo-v2-flash', 'fast'),
('mimo-v2-flash-eu', 'fast'),
('mimo-v2-flash-cn', 'fast'),
('mimo-v2-flash-sg', 'fast'),
('XiaomiMiMo/MiMo-V2-Flash-TEE', 'fast'),

-- Nemotron nano
('nvidia.nemotron-nano-3-30b', 'fast'),
('nvidia/nemotron-3-nano-30b-a3b', 'fast'),
('nvidia/nemotron-3-nano-30b-a3b:free', 'fast'),
('nvidia-nemotron-3-nano-30b-a3b', 'fast'),

-- Seed flash
('bytedance-seed/seed-1.6-flash', 'fast'),

-- SenseNova flash
('sensenova-6.0-flash', 'fast'),

-- Groq compound mini
('groq/compound-mini', 'fast'),

-- Arcee lite/spark
('arcee-lite', 'fast'),
('arcee-spark', 'fast'),
('arcee-ai/trinity-mini', 'fast'),

-- Longcat flash
('meituan/longcat-flash-chat', 'fast'),

-- DeepSeek V4 flash
('deepseek-v4-flash', 'fast'),
('deepseek-v4-flash-free', 'fast'),
('deepseek/deepseek-v4-flash', 'fast'),
('deepseek/deepseek-v4-flash:free', 'fast'),
('accounts/fireworks/models/deepseek-v4-flash', 'fast'),

-- Grok fast
('grok-3-fast', 'fast'),
('grok-code-fast-1', 'fast'),

-- Inception mercury small
('inception/mercury-coder-small', 'fast'),

-- Reka edge
('rekaai/reka-edge', 'fast'),

-- SenseNova flash

-- Kimi K2 turbo (fast variant)
('kimi-k2-turbo', 'fast'),
('accounts/fireworks/routers/kimi-k2p5-turbo', 'fast'),

-- GLM 4 9b (small model)

-- Qwen3 coder flash
('qwen/qwen3-coder-flash', 'fast'),

-- DeepSeek V4 Flash (various provider paths)

-- Gemma small models
('unsloth/gemma-3-12b-it', 'fast'),

-- GPT-5.1 instant (fast variant)
('gpt-5.1-instant', 'fast'),
('openai/gpt-5.1-instant', 'fast');


-- ============================================================
-- TIER: smart
-- Flagship, powerful, large parameter models (default tier)
-- ============================================================
INSERT OR IGNORE INTO sys_model_intent_dict (model_id, capability_tier) VALUES

-- OpenAI GPT flagship models
('gpt-4o', 'smart'),
('gpt-4.1', 'smart'),
('gpt-5', 'smart'),
('gpt-5-pro', 'smart'),
('gpt-5-chat-latest', 'smart'),
('gpt-5.1', 'smart'),
('gpt-5.1-chat-latest', 'smart'),
('gpt-5.3-chat-latest', 'smart'),
('gpt-5.3-codex', 'smart'),
('gpt-5.4', 'smart'),
('gpt-5.4-pro', 'smart'),
('gpt-5.5', 'smart'),
('gpt-5.5-pro', 'smart'),
('openai/gpt-4o', 'smart'),
('openai/gpt-4o-audio-preview', 'smart'),
('openai/gpt-4.1', 'smart'),
('openai/gpt-5', 'smart'),
('openai/gpt-5-pro', 'smart'),
('openai/gpt-5-chat', 'smart'),
('openai/gpt-5.1', 'smart'),
('openai/gpt-5.1-chat', 'smart'),
('openai/gpt-5.3-chat', 'smart'),
('openai/gpt-5.4', 'smart'),
('openai/gpt-5.4-pro', 'smart'),
('openai/gpt-5.5', 'smart'),
('openai/gpt-5.5-pro', 'smart'),
('openai/gpt-audio', 'smart'),
('openai/gpt-chat-latest', 'smart'),
('~openai/gpt-latest', 'smart'),
('openai-gpt-4o-2024-11-20', 'smart'),
('openai-gpt-52', 'smart'),
('openai-gpt-52-codex', 'smart'),
('openai-gpt-53-codex', 'smart'),
('openai-gpt-54', 'smart'),
('gpt-oss-120b', 'smart'),
('openai/gpt-oss-120b', 'smart'),
('openai/gpt-oss-120b-TEE', 'smart'),
('openai-gpt-oss-120b', 'smart'),
('openai/gpt-oss-safeguard-20b', 'smart'),

-- Claude Sonnet models (smart)
('claude-3-5-sonnet', 'smart'),
('claude-3.5-sonnet', 'smart'),
('claude-3-7-sonnet-latest', 'smart'),
('claude-sonnet-4', 'smart'),
('claude-sonnet-4-0', 'smart'),
('claude-sonnet-4-5', 'smart'),
('claude-sonnet-4-5-20250929', 'smart'),
('claude-sonnet-4-6', 'smart'),
('claude-sonnet-4.5', 'smart'),
('claude-sonnet-4.6', 'smart'),
('anthropic/claude-3-sonnet', 'smart'),
('anthropic/claude-3.5-sonnet', 'smart'),
('anthropic/claude-3.7-sonnet', 'smart'),
('anthropic/claude-sonnet-4', 'smart'),
('anthropic/claude-sonnet-4-6', 'smart'),
('anthropic/claude-sonnet-4.5', 'smart'),
('anthropic/claude-sonnet-4.6', 'smart'),
('~anthropic/claude-sonnet-latest', 'smart'),
('anthropic.claude-3-5-sonnet', 'smart'),
('anthropic.claude-3-7-sonnet', 'smart'),
('gemini-claude-sonnet-4-5-thinking', 'smart'),

-- Claude Opus models (smart)
('claude-3-7-opus-latest', 'smart'),
('claude-opus-4', 'smart'),
('claude-opus-4-0', 'smart'),
('claude-opus-4-1', 'smart'),
('claude-opus-4-5', 'smart'),
('claude-opus-4-5-20251101', 'smart'),
('claude-opus-4-6', 'smart'),
('claude-opus-4-7', 'smart'),
('claude-opus-4.5', 'smart'),
('claude-opus-4.6', 'smart'),
('claude-opus-4.7', 'smart'),
('anthropic/claude-opus-4', 'smart'),
('anthropic/claude-opus-4-7', 'smart'),
('anthropic/claude-opus-4.1', 'smart'),
('anthropic/claude-opus-4.5', 'smart'),
('anthropic/claude-opus-4.6', 'smart'),
('anthropic/claude-opus-4.6-fast', 'smart'),
('~anthropic/claude-opus-latest', 'smart'),
('anthropic.claude-opus-4-6-v1', 'smart'),
('au.anthropic.claude-opus-4-6-v1', 'smart'),
('eu.anthropic.claude-opus-4-6-v1', 'smart'),
('global.anthropic.claude-opus-4-6-v1', 'smart'),
('us.anthropic.claude-opus-4-6-v1', 'smart'),
('anthropic.claude-sonnet-4-6', 'smart'),
('au.anthropic.claude-sonnet-4-6', 'smart'),
('eu.anthropic.claude-sonnet-4-6', 'smart'),
('global.anthropic.claude-sonnet-4-6', 'smart'),
('jp.anthropic.claude-sonnet-4-6', 'smart'),
('us.anthropic.claude-sonnet-4-6', 'smart'),
('gemini-claude-opus-4-5-thinking', 'smart'),

-- Gemini Pro models (smart)
('gemini-2.5-pro', 'smart'),
('gemini-3-pro-preview', 'smart'),
('gemini-3.0-pro', 'smart'),
('gemini-3.1-pro-preview', 'smart'),
('gemini-3.1-pro-preview-customtools', 'smart'),
('google/gemini-3-pro', 'smart'),
('google/gemini-3-pro-preview', 'smart'),
('google/gemini-3.1-pro', 'smart'),
('google/gemini-3.1-pro-preview', 'smart'),
('google/gemini-3.1-pro-preview-customtools', 'smart'),
('~google/gemini-pro-latest', 'smart'),
('gemini-3-1-pro-preview', 'smart'),

-- DeepSeek flagship models
('deepseek-ai/DeepSeek-V3', 'smart'),
('deepseek-ai/DeepSeek-V3-0324-TEE', 'smart'),
('deepseek-ai/DeepSeek-V3.1', 'smart'),
('deepseek-ai/DeepSeek-V3.1-TEE', 'smart'),
('deepseek-ai/DeepSeek-V3.1-Terminus-TEE', 'smart'),
('deepseek-ai/DeepSeek-V3.2', 'smart'),
('deepseek-ai/DeepSeek-V3.2-TEE', 'smart'),
('deepseek-ai/DeepSeek-V4', 'smart'),
('deepseek-ai/DeepSeek-V4-Pro', 'smart'),
('deepseek/deepseek-v3', 'smart'),
('deepseek/deepseek-v3-0324', 'smart'),
('deepseek/deepseek-v3.1', 'smart'),
('deepseek/deepseek-v3.1-terminus', 'smart'),
('deepseek/deepseek-v4-pro', 'smart'),
('deepseek-v3.2', 'smart'),
('deepseek-v3-2-251201', 'smart'),
('deepseek-v4', 'smart'),
('accounts/fireworks/models/deepseek-v4-pro', 'smart'),

-- GLM flagship models (not flash/turbo/air)
('glm-4.5', 'smart'),
('glm-4.5v', 'smart'),
('glm-4.6', 'smart'),
('glm-4.6v', 'smart'),
('glm-4.7', 'smart'),
('glm-4-7-251222', 'smart'),
('glm-4-plus', 'smart'),
('glm-5', 'smart'),
('glm-5.1', 'smart'),
('glm-5.1-highspeed', 'smart'),
('zai-glm-4.7', 'smart'),
('z-ai/glm-4.5', 'smart'),
('z-ai/glm-4.5v', 'smart'),
('z-ai/glm-4.6', 'smart'),
('z-ai/glm-4.6v', 'smart'),
('z-ai/glm-5', 'smart'),
('z-ai/glm-5.1', 'smart'),
('zai/glm-4.5', 'smart'),
('zai/glm-4.5v', 'smart'),
('zai/glm-4.6', 'smart'),
('zai/glm-4.6v', 'smart'),
('zai/glm-5', 'smart'),
('zai/glm-5.1', 'smart'),
('zai-org/GLM-4.6-FP8', 'smart'),
('zai-org/GLM-4.6-TEE', 'smart'),
('zai-org/GLM-4.6V', 'smart'),
('zai-org/GLM-4.7', 'smart'),
('zai-org/GLM-4.7-FP8', 'smart'),
('zai-org/GLM-4.7-TEE', 'smart'),
('zai-org/GLM-5-TEE', 'smart'),
('zai-org/glm-5', 'smart'),
('zai-org/GLM-5.1', 'smart'),
('zai-org/GLM-5.1-FP8', 'smart'),
('zai-org-glm-4.6', 'smart'),
('zai-org-glm-4.7', 'smart'),
('zai-org-glm-5', 'smart'),
('z-ai/glm-5p1', 'smart'),
('accounts/fireworks/models/glm-5p1', 'smart'),
('accounts/fireworks/routers/glm-5p1-fast', 'smart'),
('z-ai/glm5', 'smart'),
('THUDM/glm-5', 'smart'),
('zai.glm-5', 'smart'),

-- Kimi flagship models (non-thinking, non-turbo)
('kimi-k2.5', 'smart'),
('kimi-k2.5-coding', 'smart'),
('kimi-k2-5-260127', 'smart'),
('kimi-k2.6', 'smart'),
('kimi-k2.6-coding', 'smart'),
('kimi-for-coding', 'smart'),
('moonshotai/Kimi-K2.5', 'smart'),
('moonshotai/kimi-k2.5', 'smart'),
('moonshotai/Kimi-K2.5-TEE', 'smart'),
('moonshotai/Kimi-K2-Instruct-0905', 'smart'),
('~moonshotai/kimi-latest', 'smart'),
('kimi-k2-5', 'smart'),
('accounts/fireworks/models/kimi-k2p6', 'smart'),

-- Qwen flagship models (plus/max/large/coder)
('qwen-3-max', 'smart'),
('qwen-3-plus', 'smart'),
('qwen-coder-plus', 'smart'),
('qwen3.5-plus', 'smart'),
('qwen3.6-27b', 'smart'),
('qwen3.7-max', 'smart'),
('qwen/qwen3-plus', 'smart'),
('qwen/qwen3-coder', 'smart'),
('qwen/qwen3-coder-plus', 'smart'),
('qwen/qwen3-coder:free', 'smart'),
('qwen/qwen3-coder-30b-a3b-instruct', 'smart'),
('qwen/qwen3-235b-a22b-fp8', 'smart'),
('qwen/qwen3-30b-a3b', 'smart'),
('qwen/qwen3.5-35b-a3b', 'smart'),
('qwen/qwen3.5-9b', 'smart'),
('qwen/qwen3.6-35b-a3b', 'smart'),
('qwen/qwen3.6-plus', 'smart'),
('alibaba/qwen-3-14b', 'smart'),
('alibaba/qwen-3-30b', 'smart'),
('alibaba/qwen3-coder', 'smart'),
('alibaba/qwen3-coder-30b-a3b', 'smart'),
('alibaba/qwen3-coder-plus', 'smart'),
('alibaba/qwen3.5-plus', 'smart'),
('alibaba/qwen3.6-plus', 'smart'),
('accounts/fireworks/models/qwen3p6-plus', 'smart'),
('qwen.qwen3-coder-30b-a3b-v1:0', 'smart'),
('qwen.qwen3-coder-480b-a35b-v1:0', 'smart'),
('qwen3-235b-a22b-instruct-2507', 'smart'),
('qwen-3-235b-a22b-instruct-2507', 'smart'),
('Qwen/Qwen3-235B-A22B-Instruct-2507-TEE', 'smart'),
('Qwen/Qwen3-30B-A3B', 'smart'),
('Qwen/Qwen3-32B', 'smart'),
('Qwen/Qwen3-Coder-Next-TEE', 'smart'),
('Qwen/Qwen3-Coder-480B-A35B-Instruct', 'smart'),
('Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8', 'smart'),
('Qwen/Qwen3-Next-80B-A3B-Instruct', 'smart'),
('Qwen/Qwen3-VL-235B-A22B-Instruct', 'smart'),
('Qwen/Qwen3.5-397B-A17B-TEE', 'smart'),
('Qwen/Qwen3.6-Plus', 'smart'),
('Qwen/Qwen-3-72B-Instruct', 'smart'),
('Qwen/Qwen3.5-72B-Instruct', 'smart'),
('qwen/qwen3-32b', 'smart'),
('qwen/qwen3-vl-30b-a3b-instruct', 'smart'),
('qwen/qwen3-vl-8b-instruct', 'smart'),
('qwen3-coder-480b-a35b-instruct', 'smart'),
('qwen3-coder-480b-a35b-instruct-turbo', 'smart'),
('qwen3-next-80b', 'smart'),
('qwen3-vl-235b-a22b', 'smart'),
('qwen3-5-35b-a3b', 'smart'),
('@cf/qwen/qwen3-30b-a3b-fp8', 'smart'),

-- Llama large models (70b/405b)
('meta-llama/Meta-Llama-4-70B-Instruct', 'smart'),
('meta-llama/llama-4-scout-17b-16e-instruct', 'smart'),
('meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8', 'smart'),
('meta-llama/Llama-4-Scout-17B-16E-Instruct', 'smart'),
('meta/llama-4-maverick', 'smart'),
('meta/llama-4-scout', 'smart'),
('meta.llama-4-70b-instruct', 'smart'),
('us.meta.llama4-maverick-17b-instruct-v1:0', 'smart'),
('meta/llama-4-70b-instruct', 'smart'),
('llama4:70b', 'smart'),
('hermes-3-405b', 'smart'),
('hermes-3-70b', 'smart'),
('NousResearch/DeepHermes-3-Mistral-24B-Preview', 'smart'),
('NousResearch/Hermes-4-405B-FP8-TEE', 'smart'),

-- Mistral large/medium models
('mistral-large-latest', 'smart'),
('mistral-medium-3-5', 'smart'),
('mistral-medium-3.5', 'smart'),
('magistral-medium-latest', 'smart'),
('devstral-medium-latest', 'smart'),
('codestral-latest', 'smart'),
('pixtral-large-latest', 'smart'),
('mistralai/devstral-medium', 'smart'),
('mistralai/mistral-large', 'smart'),
('mistralai/mistral-medium-3', 'smart'),
('mistralai/mistral-medium-3-5', 'smart'),
('mistralai/mistral-medium-3.1', 'smart'),
('mistral/mistral-medium-3.5', 'smart'),
('chutesai/Mistral-Small-3.1-24B-Instruct-2503', 'smart'),
('chutesai/Mistral-Small-3.2-24B-Instruct-2506', 'smart'),

-- MiniMax flagship
('MiniMax-M2.1', 'smart'),
('MiniMax-M2.5-highspeed', 'smart'),
('MiniMax-M2.7', 'smart'),
('MiniMax-M2.7-highspeed', 'smart'),
('MiniMaxAI/MiniMax-M2.5-TEE', 'smart'),
('MiniMaxAI/MiniMax-M2.5', 'smart'),
('minimaxai/minimax-m2.5', 'smart'),
('minimax/minimax-m2.7', 'smart'),
('minimax-m21', 'smart'),
('minimax-m25', 'smart'),

-- NVIDIA Nemotron large
('nvidia/NVIDIA-Nemotron-3-Super-120B-A12B', 'smart'),
('nvidia/nemotron-3-super-120b-a12b', 'smart'),
('nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-BF16-TEE', 'smart'),
('nemotron-3-super-free', 'smart'),

-- Gemma large models
('gemma-4', 'smart'),
('gemma-4-31b-it', 'smart'),
('google/gemma-4-31b-it', 'smart'),
('google/gemma-4-31b-it:free', 'smart'),
('google/gemma-4-31B-it', 'smart'),
('unsloth/gemma-3-27b-it', 'smart'),
('google-gemma-3-27b-it', 'smart'),

-- Amazon Nova pro/premier
('amazon.nova-pro-v1:0', 'smart'),
('amazon.nova-pro-latest', 'smart'),
('amazon/nova-pro-v1', 'smart'),
('amazon/nova-premier-v1', 'smart'),

-- Grok models
('grok-3', 'smart'),
('grok-4.3', 'smart'),
('grok-build-0.1', 'smart'),
('grok-41-fast', 'smart'),

-- Baichuan flagship
('baichuan-5', 'smart'),
('Baichuan-M2-32B', 'smart'),
('Baichuan-M3-235B', 'smart'),
('Baichuan-Omni-1.5', 'smart'),

-- Yi models
('yi-lightning', 'smart'),

-- Cohere
('command-r-plus', 'smart'),
('command-r7', 'smart'),
('cohere/command-a', 'smart'),

-- Doubao/ByteDance Seed
('ark-code-latest', 'smart'),
('doubao-seed-code', 'smart'),
('doubao-seed-code-preview-251028', 'smart'),
('doubao-seed-1-8-251228', 'smart'),
('doubao-seed-2-0-code-preview-latest', 'smart'),
('seed-1-8-251228', 'smart'),
('bytedance-seed/seed-1.6', 'smart'),
('bytedance/seed-1.6', 'smart'),

-- SenseNova pro
('sensenova-6.0', 'smart'),
('sensenova-6.0-pro', 'smart'),

-- Hunyuan
('hy3-preview', 'smart'),
('tencent/hy3-preview', 'smart'),

-- Perplexity pro
('sonar-pro', 'smart'),

-- Qianfan
('qianfan-code-latest', 'smart'),

-- Misc flagship/specialized
('goldeneye', 'smart'),
('big-pickle', 'smart'),
('arcee-agent', 'smart'),
('arcee-ai/virtuoso-large', 'smart'),
('arcee-ai/trinity-large-preview', 'smart'),
('groq/compound', 'smart'),
('openrouter/auto', 'smart'),
('openrouter/free', 'smart'),
('openrouter/owl-alpha', 'smart'),
('openrouter/pareto-code', 'smart'),
('auto', 'smart'),
('kilo/auto', 'smart'),
('poolside/laguna-m.1:free', 'smart'),
('prime-intellect/intellect-3', 'smart'),
('essentialai/rnj-1-instruct', 'smart'),
('essentialai/Rnj-1-Instruct', 'smart'),
('upstage/solar-pro-3', 'smart'),
('relace/relace-search', 'smart'),
('baidu/cobuddy:free', 'smart'),
('venice-uncensored', 'smart'),
('mimo-v2-pro', 'smart'),
('mimo-v2-omni', 'smart'),
('qwen3:32b', 'smart'),
('meta-llama/Llama-4-8B-Instruct', 'smart'),
('mistral-31-24b', 'smart'),
('openai-gpt-4o-mini-2024-07-18', 'smart'),
('rednote-hilab/dots.ocr', 'smart'),
('vllm/meta-llama/Llama-4-8B-Instruct', 'smart');
