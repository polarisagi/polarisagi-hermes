-- Seed data for sys_providers and sys_provider_auth_modes

INSERT INTO sys_providers (provider_id, provider_name, api_protocol, default_concurrency, default_timeout_sec) VALUES
-- 国际主流大厂
('openai', 'OpenAI', 'openai', 0, 120),
('anthropic', 'Anthropic', 'anthropic', 0, 120),
('google', 'Google AI Studio (原生 Gemini)', 'google', 0, 120),
('google_cloud', 'Google Agent Platform', 'google', 0, 120),
('azure', 'Microsoft Azure OpenAI', 'openai', 0, 120),
('bedrock', 'Amazon AWS Bedrock', 'openai', 0, 120),
('cohere', 'Cohere', 'openai', 0, 120),
('mistral', 'Mistral AI', 'openai', 0, 120),

-- 聚合器与云平台
('openrouter', 'OpenRouter', 'openai', 0, 120),
('siliconflow', 'SiliconFlow (硅基流动)', 'openai', 0, 120),
('groq', 'Groq (LPU)', 'openai', 0, 120),
('perplexity', 'Perplexity', 'openai', 0, 120),
('replicate', 'Replicate', 'openai', 0, 120),
('huggingface', 'HuggingFace', 'openai', 0, 120),
('fireworks', 'Fireworks AI', 'openai', 0, 120),
('cerebras', 'Cerebras', 'openai', 0, 120),

-- 中国大陆第一梯队
('deepseek', 'DeepSeek (深度求索)', 'openai', 0, 120),
('deepseek_anthropic', 'DeepSeek (Anthropic 协议)', 'anthropic', 0, 120),
('zhipu', '智谱 AI (ChatGLM)', 'openai', 0, 120),
('moonshot', '月之暗面 (Kimi)', 'openai', 0, 120),
('baichuan', '百川智能 (Baichuan)', 'openai', 0, 120),
('minimax', 'MiniMax (海螺)', 'openai', 0, 120),
('doubao', '火山引擎 (字节豆包)', 'openai', 0, 120),
('dashscope', '阿里云 (通义千问)', 'openai', 0, 120),
('qianfan', '百度智能云 (千帆/文心)', 'openai', 0, 120),
('hunyuan', '腾讯云 (混元)', 'openai', 0, 120),
('stepfun', '阶跃星辰 (StepFun)', 'openai', 0, 120),
('01ai', '零一万物 (01.AI)', 'openai', 0, 120),
('sensenova', '商汤科技 (日日新)', 'openai', 0, 120),

-- 本地部署模型
('ollama', 'Ollama (本地部署)', 'local', 0, 120),
('vllm', 'vLLM (本地部署)', 'local', 0, 120);

INSERT INTO sys_provider_auth_modes (mode_id, provider_id, mode_name, auth_type, header_name, url_template, required_fields) VALUES
-- 国际大厂
('openai_bearer', 'openai', 'OpenAI API Key', 'bearer', 'Authorization', 'https://api.openai.com/v1', '["api_key"]'),
('anthropic_key', 'anthropic', 'Anthropic API Key', 'header', 'x-api-key', 'https://api.anthropic.com/v1', '["api_key"]'),
('google_key', 'google', 'Google AI Studio Key', 'query', 'key', 'https://generativelanguage.googleapis.com/v1beta', '["api_key"]'),
('google_cloud_adc', 'google_cloud', 'Google Agent Platform ADC', 'adc', 'Authorization', 'https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{region}', '["project_id", "region"]'),
('google_cloud_key', 'google_cloud', 'Google Agent Platform Key', 'header', 'Authorization', 'https://aiplatform.googleapis.com/v1/projects/{project_id}/locations/{region}', '["api_key", "project_id", "region"]'),
('azure_key', 'azure', 'Azure API Key', 'header', 'api-key', 'https://{resource}.openai.azure.com/openai', '["api_key", "resource"]'),
('bedrock_aksk', 'bedrock', 'AWS AK/SK', 'aws_sigv4', '', 'https://bedrock-runtime.{region}.amazonaws.com', '["aws_access_key", "aws_secret_key", "region"]'),
('cohere_bearer', 'cohere', 'Cohere API Key', 'bearer', 'Authorization', 'https://api.cohere.ai/v1', '["api_key"]'),
('mistral_bearer', 'mistral', 'Mistral API Key', 'bearer', 'Authorization', 'https://api.mistral.ai/v1', '["api_key"]'),

-- 聚合器与云平台
('openrouter_bearer', 'openrouter', 'OpenRouter Key', 'bearer', 'Authorization', 'https://openrouter.ai/api/v1', '["api_key"]'),
('siliconflow_bearer', 'siliconflow', 'SiliconFlow Key', 'bearer', 'Authorization', 'https://api.siliconflow.cn/v1', '["api_key"]'),
('groq_bearer', 'groq', 'Groq Key', 'bearer', 'Authorization', 'https://api.groq.com/openai/v1', '["api_key"]'),
('perplexity_bearer', 'perplexity', 'Perplexity Key', 'bearer', 'Authorization', 'https://api.perplexity.ai', '["api_key"]'),
('replicate_bearer', 'replicate', 'Replicate Key', 'bearer', 'Authorization', 'https://api.replicate.com/v1', '["api_key"]'),
('huggingface_bearer', 'huggingface', 'HuggingFace Key', 'bearer', 'Authorization', 'https://api-inference.huggingface.co/v1', '["api_key"]'),
('fireworks_bearer', 'fireworks', 'Fireworks API Key', 'bearer', 'Authorization', 'https://api.fireworks.ai/inference/v1', '["api_key"]'),
('cerebras_bearer', 'cerebras', 'Cerebras API Key', 'bearer', 'Authorization', 'https://api.cerebras.ai/v1', '["api_key"]'),

-- 中国大陆第一梯队 (全部使用 Bearer Token)
('deepseek_bearer', 'deepseek', 'DeepSeek API Key', 'bearer', 'Authorization', 'https://api.deepseek.com/v1', '["api_key"]'),
('deepseek_anthropic_key', 'deepseek_anthropic', 'DeepSeek Anthropic Key', 'header', 'x-api-key', 'https://api.deepseek.com/beta', '["api_key"]'),
('zhipu_bearer', 'zhipu', '智谱 API Key', 'bearer', 'Authorization', 'https://open.bigmodel.cn/api/paas/v4', '["api_key"]'),
('moonshot_bearer', 'moonshot', '月之暗面 API Key', 'bearer', 'Authorization', 'https://api.moonshot.cn/v1', '["api_key"]'),
('baichuan_bearer', 'baichuan', '百川 API Key', 'bearer', 'Authorization', 'https://api.baichuan-ai.com/v1', '["api_key"]'),
('minimax_bearer', 'minimax', 'MiniMax API Key', 'bearer', 'Authorization', 'https://api.minimax.chat/v1', '["api_key"]'),
('doubao_bearer', 'doubao', '火山引擎 API Key', 'bearer', 'Authorization', 'https://ark.cn-beijing.volces.com/api/v3', '["api_key"]'),
('dashscope_bearer', 'dashscope', '阿里云 API Key', 'bearer', 'Authorization', 'https://dashscope.aliyuncs.com/compatible-mode/v1', '["api_key"]'),
('qianfan_bearer', 'qianfan', '百度千帆 API Key', 'bearer', 'Authorization', 'https://qianfan.baidubce.com/v2', '["api_key"]'),
('hunyuan_bearer', 'hunyuan', '腾讯混元 API Key', 'bearer', 'Authorization', 'https://api.hunyuan.cloud.tencent.com/v1', '["api_key"]'),
('stepfun_bearer', 'stepfun', '阶跃星辰 API Key', 'bearer', 'Authorization', 'https://api.stepfun.com/v1', '["api_key"]'),
('01ai_bearer', '01ai', '零一万物 API Key', 'bearer', 'Authorization', 'https://api.lingyiwanwu.com/v1', '["api_key"]'),
('sensenova_bearer', 'sensenova', '商汤 API Key', 'bearer', 'Authorization', 'https://api.sensenova.cn/compatible-mode/v1', '["api_key"]'),

-- 本地部署
('ollama_none', 'ollama', 'Ollama 无鉴权', 'none', '', 'http://127.0.0.1:11434/v1', '[]'),
('vllm_none', 'vllm', 'vLLM 无鉴权', 'none', '', 'http://127.0.0.1:8000/v1', '[]');
