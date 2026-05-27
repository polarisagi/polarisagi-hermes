-- Seed data for sys_models (Built-in standard models mapping)

INSERT OR IGNORE INTO sys_models (provider_id, actual_model_id, display_name) VALUES
-- Anthropic
('anthropic', 'claude-opus-4-7', 'Claude Opus 4.7 (Claude CLI)'),
('anthropic', 'claude-sonnet-4-6', 'Claude Sonnet 4.6 (Claude CLI)'),
('anthropic', 'claude-opus-4-6', 'Claude Opus 4.6 (Claude CLI)'),
('anthropic', 'claude-3-7-sonnet-latest', 'Claude 3.7 Sonnet'),
('anthropic', 'claude-3-7-opus-latest', 'Claude 3.7 Opus'),

-- Google
('google', 'gemini-3.1-pro-preview', 'Gemini 3.1 Pro Preview'),
('google', 'gemini-2.5-pro', 'Gemini 2.5 Pro'),
('google', 'gemini-3.1-flash', 'Gemini 3.1 Flash'),
('google', 'gemini-3.1-flash-lite', 'Gemini 3.1 Flash Lite'),
('google', 'gemma-4', 'Gemma 4'),
('google', 'gemini-3.0-pro', 'Gemini 3.0 Pro'),
('google', 'gemini-3.0-flash', 'Gemini 3.0 Flash'),

-- Google Cloud (Agent Platform)
('google_cloud', 'gemini-3.1-pro-preview', 'Gemini 3.1 Pro (GCP)'),
('google_cloud', 'gemini-2.5-pro', 'Gemini 2.5 Pro (GCP)'),
('google_cloud', 'gemini-3.1-flash', 'Gemini 3.1 Flash (GCP)'),

-- Azure OpenAI
('azure', 'gpt-5.5', 'Azure GPT-5.5'),
('azure', 'gpt-5.4-mini', 'Azure GPT-5.4 Mini'),
('azure', 'o4', 'Azure o4'),
('azure', 'o3-mini', 'Azure o3-mini'),

-- AWS Bedrock
('bedrock', 'anthropic.claude-3-7-sonnet', 'Claude 3.7 Sonnet (Bedrock)'),
('bedrock', 'meta.llama-4-70b-instruct', 'Llama 4 70B (Bedrock)'),
('bedrock', 'amazon.nova-pro-latest', 'Nova Pro (Bedrock)'),

-- Cohere
('cohere', 'command-r-plus', 'Command R+'),
('cohere', 'command-r7', 'Command R7'),

-- OpenAI
('openai', 'gpt-5.3-chat-latest', 'GPT-5.3 Chat (latest)'),
('openai', 'gpt-5.3-codex', 'GPT-5.3 Codex'),
('openai', 'gpt-5.4', 'GPT-5.4'),
('openai', 'gpt-5.4-mini', 'GPT-5.4 mini'),
('openai', 'gpt-5.4-nano', 'GPT-5.4 nano'),
('openai', 'gpt-5.4-pro', 'GPT-5.4 Pro'),
('openai', 'gpt-5.5', 'GPT-5.5'),
('openai', 'gpt-5.5-pro', 'GPT-5.5 Pro'),
('openai', 'o1', 'o1'),
('openai', 'o1-pro', 'o1-pro'),
('openai', 'o3', 'o3'),
('openai', 'o3-deep-research', 'o3-deep-research'),
('openai', 'o3-mini', 'o3-mini'),
('openai', 'o3-pro', 'o3-pro'),
('openai', 'o4-mini', 'o4-mini'),
('openai', 'o4-mini-deep-research', 'o4-mini-deep-research'),

-- xAI (Grok)
('xai', 'grok-beta', 'Grok Beta'),
('xai', 'grok-2', 'Grok 2'),
('xai', 'grok-3', 'Grok 3'),

-- BytePlus
('byteplus', 'seed-1-8-251228', 'Seed 1.8'),
('byteplus', 'kimi-k2-5-260127', 'Kimi K2.5'),
('byteplus', 'glm-4-7-251222', 'GLM 4.7'),
('byteplus', 'ark-code-latest', 'Ark Coding Plan'),
('byteplus', 'doubao-seed-code', 'Doubao Seed Code'),
('byteplus', 'glm-4.7', 'GLM 4.7 Coding'),
('byteplus', 'kimi-k2-thinking', 'Kimi K2 Thinking'),
('byteplus', 'kimi-k2.5', 'Kimi K2.5 Coding'),

-- Cerebras
('cerebras', 'zai-glm-4.7', 'Z.ai GLM 4.7'),
('cerebras', 'gpt-oss-120b', 'GPT OSS 120B'),
('cerebras', 'qwen-3-235b-a22b-instruct-2507', 'Qwen 3 235B Instruct'),
('cerebras', 'llama3.1-8b', 'Llama 3.1 8B'),

-- Chutes
('chutes', 'Qwen/Qwen3-32B', 'Qwen/Qwen3-32B'),
('chutes', 'unsloth/Mistral-Nemo-Instruct-2407', 'unsloth/Mistral-Nemo-Instruct-2407'),
('chutes', 'deepseek-ai/DeepSeek-V3-0324-TEE', 'deepseek-ai/DeepSeek-V3-0324-TEE'),
('chutes', 'Qwen/Qwen3-235B-A22B-Instruct-2507-TEE', 'Qwen/Qwen3-235B-A22B-Instruct-2507-TEE'),
('chutes', 'openai/gpt-oss-120b-TEE', 'openai/gpt-oss-120b-TEE'),
('chutes', 'chutesai/Mistral-Small-3.1-24B-Instruct-2503', 'chutesai/Mistral-Small-3.1-24B-Instruct-2503'),
('chutes', 'deepseek-ai/DeepSeek-V3.2-TEE', 'deepseek-ai/DeepSeek-V3.2-TEE'),
('chutes', 'zai-org/GLM-4.7-TEE', 'zai-org/GLM-4.7-TEE'),
('chutes', 'moonshotai/Kimi-K2.5-TEE', 'moonshotai/Kimi-K2.5-TEE'),
('chutes', 'unsloth/gemma-3-27b-it', 'unsloth/gemma-3-27b-it'),
('chutes', 'XiaomiMiMo/MiMo-V2-Flash-TEE', 'XiaomiMiMo/MiMo-V2-Flash-TEE'),
('chutes', 'chutesai/Mistral-Small-3.2-24B-Instruct-2506', 'chutesai/Mistral-Small-3.2-24B-Instruct-2506'),
('chutes', 'deepseek-ai/DeepSeek-R1-0528-TEE', 'deepseek-ai/DeepSeek-R1-0528-TEE'),
('chutes', 'zai-org/GLM-5-TEE', 'zai-org/GLM-5-TEE'),
('chutes', 'deepseek-ai/DeepSeek-V3.1-TEE', 'deepseek-ai/DeepSeek-V3.1-TEE'),
('chutes', 'deepseek-ai/DeepSeek-V3.1-Terminus-TEE', 'deepseek-ai/DeepSeek-V3.1-Terminus-TEE'),
('chutes', 'unsloth/gemma-3-4b-it', 'unsloth/gemma-3-4b-it'),
('chutes', 'MiniMaxAI/MiniMax-M2.5-TEE', 'MiniMaxAI/MiniMax-M2.5-TEE'),
('chutes', 'tngtech/DeepSeek-TNG-R1T2-Chimera', 'tngtech/DeepSeek-TNG-R1T2-Chimera'),
('chutes', 'Qwen/Qwen3-Coder-Next-TEE', 'Qwen/Qwen3-Coder-Next-TEE'),
('chutes', 'NousResearch/Hermes-4-405B-FP8-TEE', 'NousResearch/Hermes-4-405B-FP8-TEE'),
('chutes', 'deepseek-ai/DeepSeek-V3', 'deepseek-ai/DeepSeek-V3'),
('chutes', 'openai/gpt-oss-20b', 'openai/gpt-oss-20b'),
('chutes', 'unsloth/Llama-3.2-3B-Instruct', 'unsloth/Llama-3.2-3B-Instruct'),
('chutes', 'unsloth/Mistral-Small-24B-Instruct-2501', 'unsloth/Mistral-Small-24B-Instruct-2501'),
('chutes', 'zai-org/GLM-4.7-FP8', 'zai-org/GLM-4.7-FP8'),
('chutes', 'zai-org/GLM-4.6-TEE', 'zai-org/GLM-4.6-TEE'),
('chutes', 'Qwen/Qwen3.5-397B-A17B-TEE', 'Qwen/Qwen3.5-397B-A17B-TEE'),
('chutes', 'Qwen/Qwen2.5-72B-Instruct', 'Qwen/Qwen2.5-72B-Instruct'),
('chutes', 'NousResearch/DeepHermes-3-Mistral-24B-Preview', 'NousResearch/DeepHermes-3-Mistral-24B-Preview'),
('chutes', 'Qwen/Qwen3-Next-80B-A3B-Instruct', 'Qwen/Qwen3-Next-80B-A3B-Instruct'),
('chutes', 'zai-org/GLM-4.6-FP8', 'zai-org/GLM-4.6-FP8'),
('chutes', 'Qwen/Qwen3-235B-A22B-Thinking-2507', 'Qwen/Qwen3-235B-A22B-Thinking-2507'),
('chutes', 'deepseek-ai/DeepSeek-R1-Distill-Llama-70B', 'deepseek-ai/DeepSeek-R1-Distill-Llama-70B'),
('chutes', 'tngtech/R1T2-Chimera-Speed', 'tngtech/R1T2-Chimera-Speed'),
('chutes', 'zai-org/GLM-4.6V', 'zai-org/GLM-4.6V'),
('chutes', 'Qwen/Qwen2.5-VL-32B-Instruct', 'Qwen/Qwen2.5-VL-32B-Instruct'),
('chutes', 'Qwen/Qwen3-VL-235B-A22B-Instruct', 'Qwen/Qwen3-VL-235B-A22B-Instruct'),
('chutes', 'Qwen/Qwen3-14B', 'Qwen/Qwen3-14B'),
('chutes', 'Qwen/Qwen2.5-Coder-32B-Instruct', 'Qwen/Qwen2.5-Coder-32B-Instruct'),
('chutes', 'Qwen/Qwen3-30B-A3B', 'Qwen/Qwen3-30B-A3B'),
('chutes', 'unsloth/gemma-3-12b-it', 'unsloth/gemma-3-12b-it'),
('chutes', 'unsloth/Llama-3.2-1B-Instruct', 'unsloth/Llama-3.2-1B-Instruct'),
('chutes', 'nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-BF16-TEE', 'nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-BF16-TEE'),
('chutes', 'NousResearch/Hermes-4-14B', 'NousResearch/Hermes-4-14B'),
('chutes', 'Qwen/Qwen3Guard-Gen-0.6B', 'Qwen/Qwen3Guard-Gen-0.6B'),
('chutes', 'rednote-hilab/dots.ocr', 'rednote-hilab/dots.ocr'),

-- DeepInfra
('deepinfra', 'deepseek-ai/DeepSeek-V3.2', 'DeepSeek V3.2'),
('deepinfra', 'zai-org/GLM-5.1', 'GLM-5.1'),
('deepinfra', 'stepfun-ai/Step-3.5-Flash', 'Step 3.5 Flash'),
('deepinfra', 'MiniMaxAI/MiniMax-M2.5', 'MiniMax M2.5'),
('deepinfra', 'moonshotai/Kimi-K2.5', 'Kimi K2.5'),
('deepinfra', 'nvidia/NVIDIA-Nemotron-3-Super-120B-A12B', 'NVIDIA Nemotron 3 Super 120B A12B'),
('deepinfra', 'meta-llama/Llama-3.3-70B-Instruct-Turbo', 'Llama 3.3 70B Instruct Turbo'),

-- DeepSeek
('deepseek', 'deepseek-v4-flash', 'DeepSeek V4 Flash'),
('deepseek', 'deepseek-v4-pro', 'DeepSeek V4 Pro'),
('deepseek', 'deepseek-chat', 'DeepSeek Chat'),
('deepseek', 'deepseek-reasoner', 'DeepSeek Reasoner'),
('deepseek_anthropic', 'deepseek-reasoner', 'DeepSeek Reasoner (Anthropic)'),

-- Fireworks
('fireworks', 'accounts/fireworks/models/kimi-k2p6', 'Kimi K2.6'),
('fireworks', 'accounts/fireworks/routers/kimi-k2p5-turbo', 'Kimi K2.5 Turbo (Fire Pass)'),

-- Copilot
('github_copilot', 'claude-opus-4.6', 'Claude Opus 4.6'),
('github_copilot', 'claude-opus-4.7', 'Claude Opus 4.7'),
('github_copilot', 'claude-sonnet-4.6', 'Claude Sonnet 4.6'),
('github_copilot', 'gemini-2.5-pro', 'Gemini 2.5 Pro'),
('github_copilot', 'gemini-3-flash', 'Gemini 3 Flash'),
('github_copilot', 'gemini-3.1-pro', 'Gemini 3.1 Pro'),
('github_copilot', 'gpt-5.3-codex', 'GPT-5.3-Codex'),
('github_copilot', 'gpt-5.4', 'GPT-5.4'),
('github_copilot', 'gpt-5.5', 'GPT-5.5'),
('github_copilot', 'gpt-5.4-mini', 'GPT-5.4 mini'),
('github_copilot', 'gpt-5.4-nano', 'GPT-5.4 nano'),
('github_copilot', 'raptor-mini', 'Raptor mini'),
('github_copilot', 'goldeneye', 'Goldeneye'),

-- groq
('groq', 'groq/compound', 'Compound'),
('groq', 'groq/compound-mini', 'Compound Mini'),
('groq', 'llama-3.1-8b-instant', 'Llama 3.1 8B Instant'),
('groq', 'llama-3.3-70b-versatile', 'Llama 3.3 70B Versatile'),
('groq', 'meta-llama/llama-4-scout-17b-16e-instruct', 'Llama 4 Scout 17B'),
('groq', 'openai/gpt-oss-120b', 'GPT OSS 120B'),
('groq', 'openai/gpt-oss-20b', 'GPT OSS 20B'),
('groq', 'openai/gpt-oss-safeguard-20b', 'Safety GPT OSS 20B'),
('groq', 'qwen/qwen3-32b', 'Qwen3 32B'),

-- Kilo Gateway
('kilocode', 'kilo/auto', 'Kilo Auto'),

-- Mistral AI
('mistral', 'codestral-latest', 'Codestral (latest)'),
('mistral', 'devstral-medium-latest', 'Devstral 2 (latest)'),
('mistral', 'magistral-small', 'Magistral Small'),
('mistral', 'mistral-large-latest', 'Mistral Large (latest)'),
('mistral', 'mistral-medium-2508', 'Mistral Medium 3.1'),
('mistral', 'mistral-medium-3-5', 'Mistral Medium 3.5'),
('mistral', 'mistral-small-latest', 'Mistral Small (latest)'),
('mistral', 'pixtral-large-latest', 'Pixtral Large (latest)'),

-- Moonshot AI
('moonshot', 'kimi-k2.6', 'Kimi K2.6'),
('moonshot', 'kimi-k2.5', 'Kimi K2.5'),
('moonshot', 'kimi-k2-thinking', 'Kimi K2 Thinking'),
('moonshot', 'kimi-k2-thinking-turbo', 'Kimi K2 Thinking Turbo'),
('moonshot', 'kimi-k2-turbo', 'Kimi K2 Turbo'),

-- NVIDIA
('nvidia', 'nvidia/nemotron-3-super-120b-a12b', 'NVIDIA Nemotron 3 Super 120B'),
('nvidia', 'moonshotai/kimi-k2.5', 'Kimi K2.5'),
('nvidia', 'minimaxai/minimax-m2.5', 'MiniMax M2.5'),
('nvidia', 'z-ai/glm5', 'GLM-5'),

-- OpenCode
('opencode', 'deepseek-v4-pro', 'DeepSeek V4 Pro'),
('opencode', 'deepseek-v4-flash', 'DeepSeek V4 Flash'),

-- Qianfan
('qianfan', 'deepseek-v3.2', 'DEEPSEEK V3.2'),
('qianfan', 'ernie-5.0-thinking-preview', 'ERNIE-5.0-Thinking-Preview'),

-- StepFun
('stepfun', 'step-3.5-flash', 'Step 3.5 Flash'),
('stepfun', 'step-3.5-flash-2603', 'Step 3.5 Flash 2603'),

-- Tencent Cloud
('hunyuan', 'hy3-preview', 'Hy3 preview (TokenHub)'),

-- Together AI
('together', 'zai-org/GLM-4.7', 'GLM 4.7 Fp8'),
('together', 'moonshotai/Kimi-K2.5', 'Kimi K2.5'),
('together', 'meta-llama/Llama-3.3-70B-Instruct-Turbo', 'Llama 3.3 70B Instruct Turbo'),
('together', 'meta-llama/Llama-4-Scout-17B-16E-Instruct', 'Llama 4 Scout 17B 16E Instruct'),
('together', 'meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8', 'Llama 4 Maverick 17B 128E Instruct FP8'),
('together', 'deepseek-ai/DeepSeek-V3.1', 'DeepSeek V3.1'),
('together', 'deepseek-ai/DeepSeek-R1', 'DeepSeek R1'),
('together', 'moonshotai/Kimi-K2-Instruct-0905', 'Kimi K2-Instruct 0905'),

-- Venice AI
('venice', 'llama-3.3-70b', 'Llama 3.3 70B'),
('venice', 'llama-3.2-3b', 'Llama 3.2 3B'),
('venice', 'hermes-3-llama-3.1-405b', 'Hermes 3 Llama 3.1 405B'),
('venice', 'qwen3-235b-a22b-thinking-2507', 'Qwen3 235B Thinking'),
('venice', 'qwen3-235b-a22b-instruct-2507', 'Qwen3 235B Instruct'),
('venice', 'qwen3-coder-480b-a35b-instruct', 'Qwen3 Coder 480B'),
('venice', 'qwen3-coder-480b-a35b-instruct-turbo', 'Qwen3 Coder 480B Turbo'),
('venice', 'qwen3-5-35b-a3b', 'Qwen3.5 35B A3B'),
('venice', 'qwen3-next-80b', 'Qwen3 Next 80B'),
('venice', 'qwen3-vl-235b-a22b', 'Qwen3 VL 235B (Vision)'),
('venice', 'qwen3-4b', 'Venice Small (Qwen3 4B)'),
('venice', 'deepseek-v3.2', 'DeepSeek V3.2'),
('venice', 'venice-uncensored', 'Venice Uncensored (Dolphin-Mistral)'),
('venice', 'mistral-31-24b', 'Venice Medium (Mistral)'),
('venice', 'google-gemma-3-27b-it', 'Google Gemma 3 27B Instruct'),
('venice', 'openai-gpt-oss-120b', 'OpenAI GPT OSS 120B'),
('venice', 'nvidia-nemotron-3-nano-30b-a3b', 'NVIDIA Nemotron 3 Nano 30B'),
('venice', 'olafangensan-glm-4.7-flash-heretic', 'GLM 4.7 Flash Heretic'),
('venice', 'zai-org-glm-4.6', 'GLM 4.6'),
('venice', 'zai-org-glm-4.7', 'GLM 4.7'),
('venice', 'zai-org-glm-4.7-flash', 'GLM 4.7 Flash'),
('venice', 'zai-org-glm-5', 'GLM 5'),
('venice', 'kimi-k2-5', 'Kimi K2.5'),
('venice', 'kimi-k2-thinking', 'Kimi K2 Thinking'),
('venice', 'minimax-m21', 'MiniMax M2.1'),
('venice', 'minimax-m25', 'MiniMax M2.5'),
('venice', 'claude-opus-4-6', 'Claude Opus 4.6 (via Venice)'),
('venice', 'claude-sonnet-4-6', 'Claude Sonnet 4.6 (via Venice)'),
('venice', 'openai-gpt-52', 'GPT-5.2 (via Venice)'),
('venice', 'openai-gpt-52-codex', 'GPT-5.2 Codex (via Venice)'),
('venice', 'openai-gpt-53-codex', 'GPT-5.3 Codex (via Venice)'),
('venice', 'openai-gpt-54', 'GPT-5.4 (via Venice)'),
('venice', 'openai-gpt-4o-2024-11-20', 'GPT-4o (via Venice)'),
('venice', 'openai-gpt-4o-mini-2024-07-18', 'GPT-4o Mini (via Venice)'),
('venice', 'gemini-3-pro-preview', 'Gemini 3 Pro (via Venice)'),
('venice', 'gemini-3-1-pro-preview', 'Gemini 3.1 Pro (via Venice)'),
('venice', 'gemini-3-flash-preview', 'Gemini 3 Flash (via Venice)'),
('venice', 'grok-41-fast', 'Grok 4.1 Fast (via Venice)'),

-- Volcano Engine (doubao)
('doubao', 'doubao-seed-code-preview-251028', 'Doubao Seed Code Preview'),
('doubao', 'doubao-seed-1-8-251228', 'Doubao Seed 1.8'),
('doubao', 'kimi-k2-5-260127', 'Kimi K2.5'),
('doubao', 'glm-4-7-251222', 'GLM 4.7'),
('doubao', 'deepseek-v3-2-251201', 'DeepSeek V3.2'),
('doubao', 'ark-code-latest', 'Ark Coding Plan'),
('doubao', 'doubao-seed-code', 'Doubao Seed Code'),
('doubao', 'glm-4.7', 'GLM 4.7 Coding'),
('doubao', 'kimi-k2-thinking', 'Kimi K2 Thinking'),
('doubao', 'kimi-k2.5', 'Kimi K2.5 Coding'),

-- Xiaomi
('xiaomi', 'mimo-v2-flash', 'Xiaomi MiMo V2 Flash'),
('xiaomi', 'mimo-v2-pro', 'Xiaomi MiMo V2 Pro'),
('xiaomi', 'mimo-v2-omni', 'Xiaomi MiMo V2 Omni'),

-- Z.AI
('zai', 'glm-5.1', 'GLM-5.1'),
('zai', 'glm-5', 'GLM-5'),
('zai', 'glm-5-turbo', 'GLM-5 Turbo'),
('zai', 'glm-5v-turbo', 'GLM-5V Turbo'),
('zai', 'glm-4.7', 'GLM-4.7'),
('zai', 'glm-4.7-flash', 'GLM-4.7 Flash'),
('zai', 'glm-4.7-flashx', 'GLM-4.7 FlashX'),
('zai', 'glm-4.6', 'GLM-4.6'),
('zai', 'glm-4.6v', 'GLM-4.6V'),
('zai', 'glm-4.5', 'GLM-4.5'),
('zai', 'glm-4.5-air', 'GLM-4.5 Air'),
('zai', 'glm-4.5-flash', 'GLM-4.5 Flash'),
('zai', 'glm-4.5v', 'GLM-4.5V'),

-- Local / Open Source Additions
('ollama', 'llama4:70b', 'Llama 4 70B (Ollama)'),
('ollama', 'qwen3:32b', 'Qwen 3 32B (Ollama)'),
('ollama', 'deepseek-r2:32b', 'DeepSeek R2 32B (Ollama)'),
('vllm', 'Qwen/Qwen-3-72B-Instruct', 'Qwen 3 72B (vLLM)'),
('vllm', 'meta-llama/Llama-4-8B-Instruct', 'Llama 4 8B (vLLM)'),

-- Perplexity
('perplexity', 'sonar-pro', 'Sonar Pro'),
('perplexity', 'sonar-reasoning', 'Sonar Reasoning'),

-- Replicate
('replicate', 'meta/llama-4-70b-instruct', 'Llama 4 70B'),
('replicate', 'meta/llama-4-8b-instruct', 'Llama 4 8B'),

-- HuggingFace
('huggingface', 'meta-llama/Meta-Llama-4-70B-Instruct', 'Llama 4 70B Instruct'),

-- SiliconFlow
('siliconflow', 'deepseek-ai/DeepSeek-V4', 'DeepSeek V4 (SF)'),
('siliconflow', 'THUDM/glm-5', 'GLM-5 (SF)'),

-- OpenRouter
('openrouter', 'anthropic/claude-3.7-sonnet', 'Claude 3.7 Sonnet (OR)'),
('openrouter', 'openai/gpt-5.5', 'GPT-5.5 (OR)'),
('openrouter', 'openai/o4', 'o4 (OR)'),
('openrouter', 'google/gemini-3.1-pro', 'Gemini 3.1 Pro (OR)'),

-- Zhipu
('zhipu', 'glm-5', 'GLM-5'),
('zhipu', 'glm-5.1', 'GLM-5.1'),
('zhipu', 'glm-4-plus', 'GLM-4 Plus'),

-- Dashscope
('dashscope', 'qwen-3-max', 'Qwen 3 Max'),
('dashscope', 'qwen-3-plus', 'Qwen 3 Plus'),
('dashscope', 'qwen-coder-plus', 'Qwen Coder Plus'),

-- Baichuan
('baichuan', 'baichuan-5', 'Baichuan 5'),

-- 01.AI
('01ai', 'yi-lightning', 'Yi Lightning'),

-- SenseNova
('sensenova', 'sensenova-6.0', 'SenseNova 6.0');
