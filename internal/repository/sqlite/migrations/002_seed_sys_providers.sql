-- Seed data for sys_providers and sys_provider_auth_modes

INSERT OR IGNORE INTO sys_providers (provider_id, provider_name, api_protocol, default_concurrency, default_timeout_sec) VALUES
-- Provider: 01ai
('01ai', '零一万物 (01.AI)', 'openai', 0, 120),

-- Provider: amazon-bedrock
('amazon-bedrock', 'Amazon-Bedrock (pi)', 'openai', 0, 120),

-- Provider: anthropic
('anthropic', 'Anthropic', 'anthropic', 0, 120),

-- Provider: arcee
('arcee', 'Arcee AI', 'openai', 0, 120),

-- Provider: azure
('azure', 'Microsoft Azure OpenAI', 'openai', 0, 120),

-- Provider: azure-openai-responses
('azure-openai-responses', 'Azure-Openai-Responses (pi)', 'openai', 0, 120),

-- Provider: azure_speech
('azure_speech', 'Azure Speech', 'openai', 0, 120),

-- Provider: baichuan
('baichuan', '百川智能 (Baichuan)', 'openai', 0, 120),

-- Provider: bedrock
('bedrock', 'Amazon AWS Bedrock', 'openai', 0, 120),

-- Provider: bedrock_mantle
('bedrock_mantle', 'Amazon Bedrock Mantle', 'openai', 0, 120),

-- Provider: byteplus
('byteplus', 'BytePlus', 'openai', 0, 120),

-- Provider: cerebras
('cerebras', 'Cerebras', 'openai', 0, 120),

-- Provider: chutes
('chutes', 'Chutes', 'openai', 0, 120),

-- Provider: cloudflare
('cloudflare', 'Cloudflare AI Gateway', 'openai', 0, 120),

-- Provider: cloudflare-ai-gateway
('cloudflare-ai-gateway', 'Cloudflare-Ai-Gateway (pi)', 'openai', 0, 120),

-- Provider: cloudflare-workers-ai
('cloudflare-workers-ai', 'Cloudflare-Workers-Ai (pi)', 'openai', 0, 120),

-- Provider: cloudflare_workers
('cloudflare_workers', 'Cloudflare Workers AI', 'openai', 0, 120),

-- Provider: cohere
('cohere', 'Cohere', 'openai', 0, 120),

-- Provider: comfyui
('comfyui', 'ComfyUI', 'local', 0, 120),

-- Provider: dashscope
('dashscope', '阿里云 (通义千问)', 'openai', 0, 120),

-- Provider: deepinfra
('deepinfra', 'DeepInfra', 'openai', 0, 120),

-- Provider: deepseek
('deepseek', 'DeepSeek (深度求索)', 'openai', 0, 120),

-- Provider: deepseek_anthropic
('deepseek_anthropic', 'DeepSeek (Anthropic 协议)', 'anthropic', 0, 120),

-- Provider: doubao
('doubao', '火山引擎 (字节豆包)', 'openai', 0, 120),

-- Provider: ds4
('ds4', 'ds4 (local DeepSeek V4)', 'local', 0, 120),

-- Provider: elevenlabs
('elevenlabs', 'ElevenLabs', 'openai', 0, 120),

-- Provider: fal
('fal', 'fal', 'openai', 0, 120),

-- Provider: fireworks
('fireworks', 'Fireworks AI', 'openai', 0, 120),

-- Provider: github-copilot
('github-copilot', 'Github-Copilot (pi)', 'openai', 0, 120),

-- Provider: github_copilot
('github_copilot', 'GitHub Copilot', 'openai', 0, 120),

-- Provider: google
('google', 'Google AI Studio (原生 Gemini)', 'google', 0, 120),

-- Provider: google-vertex
('google-vertex', 'Google-Vertex (pi)', 'openai', 0, 120),

-- Provider: google_cloud
('google_cloud', 'Google Agent Platform', 'google', 0, 120),

-- Provider: gradium
('gradium', 'Gradium', 'openai', 0, 120),

-- Provider: groq
('groq', 'Groq (LPU)', 'openai', 0, 120),

-- Provider: huggingface
('huggingface', 'HuggingFace', 'openai', 0, 120),

-- Provider: hunyuan
('hunyuan', '腾讯云 (混元)', 'openai', 0, 120),

-- Provider: inferrs
('inferrs', 'inferrs (local models)', 'local', 0, 120),

-- Provider: kilocode
('kilocode', 'Kilocode', 'openai', 0, 120),

-- Provider: kimi-coding
('kimi-coding', 'Kimi-Coding (pi)', 'openai', 0, 120),

-- Provider: kimi_coding
('kimi_coding', 'Kimi For Coding', 'openai', 0, 120),

-- Provider: litellm
('litellm', 'LiteLLM', 'openai', 0, 120),

-- Provider: lmstudio
('lmstudio', 'LM Studio (local models)', 'local', 0, 120),

-- Provider: minimax
('minimax', 'MiniMax (海螺)', 'openai', 0, 120),

-- Provider: mistral
('mistral', 'Mistral AI', 'openai', 0, 120),

-- Provider: moonshot
('moonshot', '月之暗面 (Kimi)', 'openai', 0, 120),

-- Provider: nvidia
('nvidia', 'NVIDIA NIM', 'openai', 0, 120),

-- Provider: ollama
('ollama', 'Ollama (本地部署)', 'local', 0, 120),

-- Provider: openai
('openai', 'OpenAI', 'openai', 0, 120),

-- Provider: openai-codex
('openai-codex', 'Openai-Codex (pi)', 'openai', 0, 120),

-- Provider: opencode
('opencode', 'OpenCode', 'openai', 0, 120),

-- Provider: opencode-go
('opencode-go', 'Opencode-Go (pi)', 'openai', 0, 120),

-- Provider: opencode_go
('opencode_go', 'OpenCode Go', 'openai', 0, 120),

-- Provider: opencode_zen
('opencode_zen', 'OpenCode Zen', 'openai', 0, 120),

-- Provider: openrouter
('openrouter', 'OpenRouter', 'openai', 0, 120),

-- Provider: perplexity
('perplexity', 'Perplexity', 'openai', 0, 120),

-- Provider: qianfan
('qianfan', '百度智能云 (千帆/文心)', 'openai', 0, 120),

-- Provider: replicate
('replicate', 'Replicate', 'openai', 0, 120),

-- Provider: runway
('runway', 'Runway', 'openai', 0, 120),

-- Provider: senseaudio
('senseaudio', 'SenseAudio', 'openai', 0, 120),

-- Provider: sensenova
('sensenova', '商汤科技 (日日新)', 'openai', 0, 120),

-- Provider: sglang
('sglang', 'SGLang (local models)', 'local', 0, 120),

-- Provider: siliconflow
('siliconflow', 'SiliconFlow (硅基流动)', 'openai', 0, 120),

-- Provider: stepfun
('stepfun', '阶跃星辰 (StepFun)', 'openai', 0, 120),

-- Provider: synthetic
('synthetic', 'Synthetic', 'openai', 0, 120),

-- Provider: together
('together', 'Together AI', 'openai', 0, 120),

-- Provider: venice
('venice', 'Venice AI', 'openai', 0, 120),

-- Provider: vercel
('vercel', 'Vercel AI Gateway', 'openai', 0, 120),

-- Provider: vercel-ai-gateway
('vercel-ai-gateway', 'Vercel-Ai-Gateway (pi)', 'openai', 0, 120),

-- Provider: vllm
('vllm', 'vLLM (本地部署)', 'local', 0, 120),

-- Provider: vydra
('vydra', 'Vydra', 'openai', 0, 120),

-- Provider: xai
('xai', 'xAI (Grok)', 'openai', 0, 120),

-- Provider: xiaomi
('xiaomi', 'Xiaomi', 'openai', 0, 120),

-- Provider: xiaomi_amsterdam
('xiaomi_amsterdam', 'Xiaomi MiMo Token Plan (Amsterdam)', 'openai', 0, 120),

-- Provider: xiaomi_china
('xiaomi_china', 'Xiaomi MiMo Token Plan (China)', 'openai', 0, 120),

-- Provider: xiaomi_singapore
('xiaomi_singapore', 'Xiaomi MiMo Token Plan (Singapore)', 'openai', 0, 120),

-- Provider: zai
('zai', 'ZAI', 'openai', 0, 120),

-- Provider: zhipu
('zhipu', '智谱 AI (ChatGLM)', 'openai', 0, 120);

INSERT OR IGNORE INTO sys_provider_auth_modes (mode_id, provider_id, mode_name, auth_type, header_name, url_template, required_fields) VALUES
-- Provider: 01ai
('01ai_bearer', '01ai', '零一万物 API Key', 'bearer', 'Authorization', 'https://api.lingyiwanwu.com/v1', '["api_key"]'),

-- Provider: anthropic
('anthropic_key', 'anthropic', 'Anthropic API Key', 'header', 'x-api-key', 'https://api.anthropic.com/v1', '["api_key"]'),

-- Provider: arcee
('arcee_bearer', 'arcee', 'Arcee AI Key', 'bearer', 'Authorization', 'https://api.arcee.ai/v1', '["api_key"]'),

-- Provider: azure
('azure_key', 'azure', 'Azure API Key', 'header', 'api-key', 'https://{resource}.openai.azure.com/openai', '["api_key", "resource"]'),

-- Provider: azure_speech
('azure_speech_key', 'azure_speech', 'Azure Speech Key', 'header', 'Ocp-Apim-Subscription-Key', 'https://{region}.tts.speech.microsoft.com', '["api_key", "region"]'),

-- Provider: baichuan
('baichuan_bearer', 'baichuan', '百川 API Key', 'bearer', 'Authorization', 'https://api.baichuan-ai.com/v1', '["api_key"]'),

-- Provider: bedrock
('bedrock_aksk', 'bedrock', 'AWS AK/SK', 'aws_sigv4', '', 'https://bedrock-runtime.{region}.amazonaws.com', '["aws_access_key", "aws_secret_key", "region"]'),

-- Provider: bedrock_mantle
('bedrock_mantle_aksk', 'bedrock_mantle', 'AWS AK/SK', 'aws_sigv4', '', 'https://bedrock-runtime.{region}.amazonaws.com', '["aws_access_key", "aws_secret_key", "region"]'),

-- Provider: byteplus
('byteplus_bearer', 'byteplus', 'BytePlus Key', 'bearer', 'Authorization', 'https://api.byteplus.com/v1', '["api_key"]'),

-- Provider: cerebras
('cerebras_bearer', 'cerebras', 'Cerebras API Key', 'bearer', 'Authorization', 'https://api.cerebras.ai/v1', '["api_key"]'),

-- Provider: chutes
('chutes_bearer', 'chutes', 'Chutes Key', 'bearer', 'Authorization', 'https://api.chutes.ai/v1', '["api_key"]'),

-- Provider: cloudflare
('cloudflare_bearer', 'cloudflare', 'Cloudflare Gateway Key', 'bearer', 'Authorization', 'https://gateway.ai.cloudflare.com/v1/{account_id}/{gateway_id}/openai', '["api_key", "account_id", "gateway_id"]'),

-- Provider: cloudflare_workers
('cloudflare_workers_bearer', 'cloudflare_workers', 'Cloudflare Workers Key', 'bearer', 'Authorization', 'https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1', '["api_key", "account_id"]'),

-- Provider: cohere
('cohere_bearer', 'cohere', 'Cohere API Key', 'bearer', 'Authorization', 'https://api.cohere.ai/v1', '["api_key"]'),

-- Provider: comfyui
('comfyui_none', 'comfyui', 'ComfyUI (无鉴权)', 'none', '', 'http://127.0.0.1:8188', '[]'),

-- Provider: dashscope
('dashscope_bearer', 'dashscope', '阿里云 API Key', 'bearer', 'Authorization', 'https://dashscope.aliyuncs.com/compatible-mode/v1', '["api_key"]'),

-- Provider: deepinfra
('deepinfra_bearer', 'deepinfra', 'DeepInfra Key', 'bearer', 'Authorization', 'https://api.deepinfra.com/v1/openai', '["api_key"]'),

-- Provider: deepseek
('deepseek_bearer', 'deepseek', 'DeepSeek API Key', 'bearer', 'Authorization', 'https://api.deepseek.com/v1', '["api_key"]'),

-- Provider: deepseek_anthropic
('deepseek_anthropic_key', 'deepseek_anthropic', 'DeepSeek Anthropic Key', 'header', 'x-api-key', 'https://api.deepseek.com/beta', '["api_key"]'),

-- Provider: doubao
('doubao_bearer', 'doubao', '火山引擎 API Key', 'bearer', 'Authorization', 'https://ark.cn-beijing.volces.com/api/v3', '["api_key"]'),

-- Provider: ds4
('ds4_none', 'ds4', 'ds4 (无鉴权)', 'none', '', 'http://127.0.0.1:8080/v1', '[]'),

-- Provider: elevenlabs
('elevenlabs_bearer', 'elevenlabs', 'ElevenLabs Key', 'bearer', 'Authorization', 'https://api.elevenlabs.io/v1', '["api_key"]'),

-- Provider: fal
('fal_bearer', 'fal', 'fal API Key', 'bearer', 'Authorization', 'https://api.fal.ai/v1', '["api_key"]'),

-- Provider: fireworks
('fireworks_bearer', 'fireworks', 'Fireworks API Key', 'bearer', 'Authorization', 'https://api.fireworks.ai/inference/v1', '["api_key"]'),

-- Provider: github_copilot
('github_copilot_bearer', 'github_copilot', 'GitHub Copilot Token', 'bearer', 'Authorization', 'https://api.githubcopilot.com', '["api_key"]'),

-- Provider: google
('google_key', 'google', 'Google AI Studio Key', 'query', 'key', 'https://generativelanguage.googleapis.com/v1beta', '["api_key"]'),

-- Provider: google_cloud
('google_cloud_adc', 'google_cloud', 'Google Agent Platform ADC', 'adc', 'Authorization', 'https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{region}', '["project_id", "region"]'),
('google_cloud_key', 'google_cloud', 'Google Agent Platform Key', 'header', 'Authorization', 'https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{region}', '["api_key", "project_id", "region"]'),

-- Provider: gradium
('gradium_bearer', 'gradium', 'Gradium Key', 'bearer', 'Authorization', 'https://api.gradium.ai/v1', '["api_key"]'),

-- Provider: groq
('groq_bearer', 'groq', 'Groq Key', 'bearer', 'Authorization', 'https://api.groq.com/openai/v1', '["api_key"]'),

-- Provider: huggingface
('huggingface_bearer', 'huggingface', 'HuggingFace Key', 'bearer', 'Authorization', 'https://api-inference.huggingface.co/v1', '["api_key"]'),

-- Provider: hunyuan
('hunyuan_bearer', 'hunyuan', '腾讯混元 API Key', 'bearer', 'Authorization', 'https://api.hunyuan.cloud.tencent.com/v1', '["api_key"]'),

-- Provider: inferrs
('inferrs_none', 'inferrs', 'inferrs (无鉴权)', 'none', '', 'http://127.0.0.1:8000/v1', '[]'),

-- Provider: kilocode
('kilocode_bearer', 'kilocode', 'Kilocode Key', 'bearer', 'Authorization', 'https://api.kilocode.com/v1', '["api_key"]'),

-- Provider: kimi_coding
('kimi_coding_bearer', 'kimi_coding', 'Kimi For Coding Key', 'bearer', 'Authorization', 'https://api.moonshot.cn/v1', '["api_key"]'),

-- Provider: litellm
('litellm_bearer', 'litellm', 'LiteLLM Key', 'bearer', 'Authorization', 'http://127.0.0.1:4000/v1', '["api_key"]'),

-- Provider: lmstudio
('lmstudio_none', 'lmstudio', 'LM Studio (无鉴权)', 'none', '', 'http://127.0.0.1:1234/v1', '[]'),

-- Provider: minimax
('minimax_bearer', 'minimax', 'MiniMax API Key', 'bearer', 'Authorization', 'https://api.minimax.chat/v1', '["api_key"]'),

-- Provider: mistral
('mistral_bearer', 'mistral', 'Mistral API Key', 'bearer', 'Authorization', 'https://api.mistral.ai/v1', '["api_key"]'),

-- Provider: moonshot
('moonshot_bearer', 'moonshot', '月之暗面 API Key', 'bearer', 'Authorization', 'https://api.moonshot.cn/v1', '["api_key"]'),

-- Provider: nvidia
('nvidia_bearer', 'nvidia', 'NVIDIA NIM Key', 'bearer', 'Authorization', 'https://integrate.api.nvidia.com/v1', '["api_key"]'),

-- Provider: ollama
('ollama_none', 'ollama', 'Ollama 无鉴权', 'none', '', 'http://127.0.0.1:11434/v1', '[]'),

-- Provider: openai
('openai_bearer', 'openai', 'OpenAI API Key', 'bearer', 'Authorization', 'https://api.openai.com/v1', '["api_key"]'),

-- Provider: opencode
('opencode_bearer', 'opencode', 'OpenCode Key', 'bearer', 'Authorization', 'https://api.opencode.com/v1', '["api_key"]'),

-- Provider: opencode_go
('opencode_go_bearer', 'opencode_go', 'OpenCode Go Key', 'bearer', 'Authorization', 'https://api.opencode.com/go/v1', '["api_key"]'),

-- Provider: opencode_zen
('opencode_zen_bearer', 'opencode_zen', 'OpenCode Zen Key', 'bearer', 'Authorization', 'https://api.opencode.com/zen/v1', '["api_key"]'),

-- Provider: openrouter
('openrouter_bearer', 'openrouter', 'OpenRouter Key', 'bearer', 'Authorization', 'https://openrouter.ai/api/v1', '["api_key"]'),

-- Provider: perplexity
('perplexity_bearer', 'perplexity', 'Perplexity Key', 'bearer', 'Authorization', 'https://api.perplexity.ai', '["api_key"]'),

-- Provider: qianfan
('qianfan_bearer', 'qianfan', '百度千帆 API Key', 'bearer', 'Authorization', 'https://qianfan.baidubce.com/v2', '["api_key"]'),

-- Provider: replicate
('replicate_bearer', 'replicate', 'Replicate Key', 'bearer', 'Authorization', 'https://api.replicate.com/v1', '["api_key"]'),

-- Provider: runway
('runway_bearer', 'runway', 'Runway Key', 'bearer', 'Authorization', 'https://api.runwayml.com/v1', '["api_key"]'),

-- Provider: senseaudio
('senseaudio_bearer', 'senseaudio', 'SenseAudio Key', 'bearer', 'Authorization', 'https://api.sensenova.cn/v1', '["api_key"]'),

-- Provider: sensenova
('sensenova_bearer', 'sensenova', '商汤 API Key', 'bearer', 'Authorization', 'https://api.sensenova.cn/compatible-mode/v1', '["api_key"]'),

-- Provider: sglang
('sglang_none', 'sglang', 'SGLang (无鉴权)', 'none', '', 'http://127.0.0.1:30000/v1', '[]'),

-- Provider: siliconflow
('siliconflow_bearer', 'siliconflow', 'SiliconFlow Key', 'bearer', 'Authorization', 'https://api.siliconflow.cn/v1', '["api_key"]'),

-- Provider: stepfun
('stepfun_bearer', 'stepfun', '阶跃星辰 API Key', 'bearer', 'Authorization', 'https://api.stepfun.com/v1', '["api_key"]'),

-- Provider: synthetic
('synthetic_bearer', 'synthetic', 'Synthetic Key', 'bearer', 'Authorization', 'https://api.synthetic.ai/v1', '["api_key"]'),

-- Provider: together
('together_bearer', 'together', 'Together AI Key', 'bearer', 'Authorization', 'https://api.together.xyz/v1', '["api_key"]'),

-- Provider: venice
('venice_bearer', 'venice', 'Venice AI Key', 'bearer', 'Authorization', 'https://api.venice.ai/api/v1', '["api_key"]'),

-- Provider: vercel
('vercel_bearer', 'vercel', 'Vercel AI Gateway Key', 'bearer', 'Authorization', 'https://api.vercel.com/v1', '["api_key"]'),

-- Provider: vllm
('vllm_none', 'vllm', 'vLLM 无鉴权', 'none', '', 'http://127.0.0.1:8000/v1', '[]'),

-- Provider: vydra
('vydra_bearer', 'vydra', 'Vydra Key', 'bearer', 'Authorization', 'https://api.vydra.ai/v1', '["api_key"]'),

-- Provider: xai
('xai_bearer', 'xai', 'xAI API Key', 'bearer', 'Authorization', 'https://api.x.ai/v1', '["api_key"]'),

-- Provider: xiaomi
('xiaomi_bearer', 'xiaomi', 'Xiaomi API Key', 'bearer', 'Authorization', 'https://api.xiaomi.com/v1', '["api_key"]'),

-- Provider: xiaomi_amsterdam
('xiaomi_amsterdam_bearer', 'xiaomi_amsterdam', 'Xiaomi MiMo (Amsterdam)', 'bearer', 'Authorization', 'https://eu.api.mimo.xiaomi.com/v1', '["api_key"]'),

-- Provider: xiaomi_china
('xiaomi_china_bearer', 'xiaomi_china', 'Xiaomi MiMo (China)', 'bearer', 'Authorization', 'https://api.mimo.xiaomi.com/v1', '["api_key"]'),

-- Provider: xiaomi_singapore
('xiaomi_singapore_bearer', 'xiaomi_singapore', 'Xiaomi MiMo (Singapore)', 'bearer', 'Authorization', 'https://sg.api.mimo.xiaomi.com/v1', '["api_key"]'),

-- Provider: zai
('zai_bearer', 'zai', 'ZAI Key', 'bearer', 'Authorization', 'https://api.zai.com/v1', '["api_key"]'),

-- Provider: zhipu
('zhipu_bearer', 'zhipu', '智谱 API Key', 'bearer', 'Authorization', 'https://open.bigmodel.cn/api/paas/v4', '["api_key"]');
