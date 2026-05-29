# Polarisagi-Hermes 架构设计文档

**Polarisagi-Hermes** 是一款全能型大语言模型 API 代理分发与并发控制网关。它从最初专注于 Google Vertex AI 账号适配的工具，全面进化为支持 OpenAI、Anthropic、Google Gemini 原生、Google Agent Platform、本地模型（Ollama/vLLM）等**所有主流 LLM 协议**的通用 AI 代理网关。

---

## 1. 系统分层

系统整体解耦为四个核心层次：

1. **接入层 (HTTP/Router)**：负责接收请求、协议检测、模型名提取与 URL 清洗。
2. **路由与调度层 (Manager)**：负责根据请求特征匹配路由规则，并在对应的节点池中执行负载均衡和抢占式调度。
3. **协议转换层 (Translators)**：负责在不同大模型协议（OpenAI, Anthropic, Google Gemini, Google Agent Platform）之间进行请求体和流式响应的格式适配。
4. **存储与控制层 (DB/State)**：依托嵌入式 SQLite 持久化配置和账单，通过内存状态机独立管理各节点的健康度。

---

## 2. 核心请求链路

```
Client Request (Claude Code: /v1/messages | Codex: /v1/chat/completions)
  → http.go (多协议入口兼容)
  → Pipeline (意图映射推断 + Pro路由拦截 + Tier降级兜底)
  → Manager (优先级排序 + CAS 锁抢占)
  → TranslatorFactory (动态实例化协议翻译器，如 Anthropic→Google, OpenAI→OpenAI 透传)
  → Stream Response (SSE 流式解析 Usage)
  → FinalizeNodeState (节点状态结算)
  → 异步写入 SQLite 计费
```

---

## 3. 关键架构设计

### 3.1 物理级单并发隔离 (防 429 核心机制)
针对 Google/Anthropic 等严格限流的平台，网关采用三层防 429 策略：

**层1 — 节点状态机并发隔离**：通过 `sync.Mutex` + CAS (Compare-And-Swap) 抢占式调度，严格验证节点处于 `Idle` 或 `Probation` 时才设为 `Busy`，保障**同一时刻一个账号仅处理一个请求**。未抢到的请求跳过该候选，进入下一轮 100ms 轮询，直至超时或成功。

**层2 — 强制请求间隔 (MinRequestIntervalSec)**：每个节点可独立配置最小请求间隔（默认 30s），通过 `LastAcquireTime` 时间戳校验。CAS 抢占阶段持锁重验间隔（TOCTOU 保护），防止筛选与抢占之间的窗口内导致间隔失效。

**层3 — LRU 确定性调度**：同优先级节点按 `LastAcquireTime` 升序排序（最久未用的优先）。确定性 LRU 在高并发下自然将请求分散到不同节点，避免随机打乱导致的短时间内同一账号被多次命中。

**冷恢复安全**：冷却结束转为 `Probation` 时同步重置 `LastAcquireTime`，防止刚苏醒的节点被多个并发请求同时抢占。

### 3.2 动态熔断状态机 (Circuit Breaker)
每个节点拥有独立的五态健康转换机：
- 🟢 **Idle (空闲)**: 正常接收请求。
- 🟡 **Busy (忙碌)**: 正在处理请求。
- 🔴 **Cooldown (冷却)**: 遇到网络异常或服务端报错（如 429/5xx），进入指数退避冷却期。
- 🟠 **Probation (试探)**: 冷却结束后的试用期，仅允许接收 1 个探路请求。成功则恢复 `Idle`，失败则冷却时长翻倍。
- ⚫️ **Exhausted (耗尽)**: 试探达上限或收到余额不足（Quota Exceeded）错误，进入永久物理隔离。

**动态可配参数**：熔断机的各项核心阈值（初始冷却时间、最大冷却时间、失败阈值次数、失败统计窗口）均已实现全局动态可配，通过 SQLite `settings` 表进行持久化，前端随时可调。

### 3.3 零配置与无缝热重载
- **SQLite 驱动**: 抛弃繁琐的 YAML 配置文件，所有节点、路由、配置项均存入嵌入式 SQLite，支持数据库迁移脚本 (Migrations) 自动升级表结构。
- **双层热重载**: 管理后台 (Web UI) 的写操作（增删改渠道/路由/模型）异步触发 `chanManager.Reload()` 与 `pipeline.Reload()`。前者在读写锁保护下重建节点健康池；后者批量拉取意图字典与路由规则至内存 Map，原子替换指针后立即生效，**不中断存量流量**。

### 3.4 异步流式计费
- **用量解析与兜底估算**：拦截后端 SSE (Server-Sent Events) 流的尾部数据块，解析原生 Token 消耗。若流意外中断，启动字节数算法进行兜底估算。
- **异步落盘**：通过 Go Channel 将消费记录移交至独立的 `dbWriter` 协程，异步写入 SQLite `account_logs` 表，保证 API 响应零延迟。
- **动态水位线拦截**：同步扣减节点的内存 `Balance`，触发熔断百分比（如 90%）即自动隔离下线。

### 3.5 日志规范
统一使用标准库 `log/slog` 的结构化日志：`slog.Error/Warn/Info/Debug("固定中文消息", "key", val)`。消息字符串必须为固定字面量，禁止用 `fmt.Sprintf` 拼接动态值；所有上下文变量（如 `trace_id`、`node`、`error`）一律以 key-value 参数追加，便于日志系统过滤与告警。

---

## 4. 重大架构更新 (v2 重构)

### 4.1 全协议支持 & 厂商数据字典
**背景**：v1 主要聚焦于 Google Vertex AI 账号，v2 全面扩展为通用 AI 网关。

- **支持协议**：OpenAI (Bearer Token)、Anthropic (x-api-key)、Google Gemini 原生 (Query Key)、Google Agent Platform (ADC/API Key)、本地模型协议（Ollama/vLLM，无鉴权）。
- **内置厂商字典**：通过 SQL 种子脚本 (`002_seed_sys_providers.sql`) 预置全球 30+ 家主流大模型厂商数据，包括 OpenAI、Anthropic、DeepSeek、智谱、月之暗面(Kimi)、阿里通义千问、腾讯混元、百度千帆、火山引擎(豆包)、Groq、Mistral、Cohere、SiliconFlow、OpenRouter 等，开箱即用，无需手动填写 Base URL。
- **双协议厂商**：对于同时提供多套协议接口的厂商（如 DeepSeek 同时支持 OpenAI 和 Anthropic 协议），在 `sys_providers` 表中独立建档，在 `sys_provider_auth_modes` 表中对应绑定不同鉴权模式，实现精准的协议级分流。

### 4.2 数据驱动的动态鉴权体系 (Auth Modes)
打破硬编码的供应商逻辑，全面采用"协议即数据"的动态设计：

- **核心解耦**：大模型厂商（`sys_providers`）与鉴权方式（`sys_provider_auth_modes`）实现 1 对 N 解耦。同一厂商可同时挂载多种独立鉴权模式（如 Google Agent Platform 同时支持 ADC JSON 和 API Key）。
- **系统级常量**：在 `internal/domain/provider.go` 中定义强类型字符串常量，确保 Go 层类型安全，数据库层保持高可读性：
  ```go
  const (
      AuthTypeNone     = "none"
      AuthTypeBearer   = "bearer"   // Authorization: Bearer <key>
      AuthTypeHeader   = "header"   // 自定义 Header，如 x-api-key
      AuthTypeQuery    = "query"    // URL 参数，如 ?key=xxx
      AuthTypeADC      = "adc"      // Google ADC 令牌置换
      AuthTypeAWSSigV4 = "aws_sigv4" // AWS 签名认证
  )
  ```
- **动态 UI 渲染**：前端管理面板不包含任何厂商字段的写死逻辑。UI 完全依据当前选定 Auth Mode 中的 `required_fields` 和 `auth_type`，自动渲染表单控件（如仅在选中 Agent Platform 时才显示 Project ID 和 Region 字段）。

### 4.3 双轨制模型映射引擎与多协议路由 (Pipeline)
为解决客户端（如 Claude Code、Codex）发出的异构模型请求能够被正确分发到后端的 DeepSeek 或 Google Agent Platform，网关重构了 Pipeline 路由与翻译器机制：

- **极简模式 (Intelligent Intent Mapping)**：
  主打"开箱即用，动态映射"。底层彻底移除了 `sys_models` 中冗余的主观梯队标签，引入单一数据源 **意图字典 (Intent Dict)**，将任意模型名（`model_id`）映射为三大核心能力梯队：
  1. **`smart` (旗舰型)**：主攻高难度代码与复杂推理，如 `gpt-4o`, `claude-3-5-sonnet`, `gemini-2.5-pro`。
  2. **`fast` (极速型)**：主攻低延迟、高并发的轻量任务，如 `gpt-4o-mini`, `claude-3-haiku`。
  3. **`reasoning` (沉思型)**：主攻 CoT 深度思维链逻辑题，如 `o1`, `DeepSeek-R1`。
  **降级熔断链**：当 `fast` 或 `reasoning` 梯队无可用渠道节点时，系统会自动向下兼容 fallback 到 `smart` 梯队兜底，确保服务不中断。

- **专业模式 (Deterministic Custom Routes)**：
  主打"绝对控制"。通过 `user_custom_routes` 表实现 1:1 强制路由，优先级高于极简模式。支持精确拦截或使用通配符 `*` 兜底，适用于私有化部署模型的特定引流。

- **全链路翻译体系 (Translators)**：
  实现了 `/v1/messages` (Anthropic) 和 `/v1/chat/completions` (OpenAI) 双入口协议。请求经过路由确定后端渠道后，通过 `TranslatorFactory` 动态调用对应翻译器（如 `OpenAITranslator` 透传 Codex 至 DeepSeek，或 `AnthropicGoogleTranslator` 转换至 GEAP），实现协议透明解耦。

### 4.4 一键客户端配置 (Client Auto-Config)
**核心亮点功能**：打破商业 AI 客户端（如 Claude Code、Codex 等）锁定官方 API 的限制。

- **痛点**：Claude Code、OpenAI Codex 等软件将 Base URL 固定写死，普通用户无法切换大模型厂商或使用自购的第三方 API Key（如 DeepSeek）。
- **方案**：在管理后台一键注入 Polarisagi-Hermes 代理配置，自动修改目标软件的环境变量或配置文件，将流量劫持到本地网关，再由网关转发给真正的目标大模型。
- **支持客户端**：Claude Code、OpenAI Codex、OpenCode、Gemini CLI、Hermes、OpenClaw 等。
- **推荐组合**：**特别推荐使用 DeepSeek 作为后端**。DeepSeek 兼容 OpenAI 协议，价格极具竞争力，配合 Polarisagi-Hermes 的多账号轮询和熔断机制，可实现近乎无感的高可用 AI 编程体验。

### 4.5 简单模式 / 专业模式 双 UI 切换
- **简单模式**：精简表单，隐藏高级配置项（如并发上限、请求间隔、余额预警等），降低用户心智负担，适合个人开发者快速上手。
- **专业模式**：展开全部控制项，适合有精细化账号管理需求的企业级用户或高级玩家，与"专业模式模型映射"联动。

### 4.6 外部生态聚合与 Git 智能同步引擎 (Zero-Cost Sync)
**背景**：全球大模型市场瞬息万变，新的模型与厂商层出不穷，单纯依靠静态代码维护数据字典难以跟上发展速度。为此，网关引入了全自动的外部生态聚合模块。

- **生态接入**：网关深度集成了 OpenRouter 和 LiteLLM 两大顶级聚合生态的官方对外接口：
  - **OpenRouter API** (`https://openrouter.ai/api/v1/providers`, `https://openrouter.ai/api/v1/models`)：获取全球最全的前沿大模型厂商列表，以及权威的模型元数据（上下文长度、多模态能力、定价、上架时间戳）。
  - **LiteLLM API** (`https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json`)：获取业界标准化的模型账单与参数格式，作为补充的兜底字典。
- **本地落盘与配置化**：在 `config.toml` 中通过 `[sync].data_dir` 配置外部数据的缓存目录。
- **Git 智能拦截 (Zero-Cost Sync)**：这是同步模块的极客特性。引擎下载完外部最新的 JSON 后，系统会在底层自动执行 `git status --porcelain` 比对文件差异。若全网数据没有任何更新，系统瞬间中断流程，直接跳过海量 JSON 的内存反序列化和 SQLite 写入；若检测到变更，更新完数据库后系统会自动提交 `git commit`。它不仅做到了真正的“零消耗”同步，还在本地为您留存了一份极其珍贵的“大模型市场进化史” Git 时间快照！

---

## 5. 数据库表结构概要

| 表名 | 说明 |
|------|------|
| `sys_providers` | 系统内置厂商字典（70+ 家），包含协议类型、默认配置 |
| `sys_provider_auth_modes` | 厂商鉴权模式字典，1 对 N，定义 auth_type、header_name、url_template、required_fields |
| `sys_models` | 系统内置模型物理属性（900+ 条），去除了主观的 capability_tier，仅保留纯客观元数据 |
| `sys_model_intent_dict` | 全局模型意图单一数据源（model_id → capability_tier），涵盖客户端请求名与服务端真实模型名 |
| `user_providers` | 用户自定义的渠道账号，关联 sys_providers 和 sys_provider_auth_modes |
| `user_models` | 创建渠道时由系统自动导入生成的模型可用实例池 |
| `user_model_intent_dict` | 用户手动覆盖及系统自动推断学习的意图字典（实时生效），优先级高于 sys 字典 |
| `user_custom_routes` | 专业模式 1:1 强制路由表，支持精确匹配与通配符 `*` 兜底 |
| `system_settings` | 全局系统配置，包含熔断参数、UI 模式等 |
| `account_logs` | 异步写入的 Token 消费账单记录 |
| `client_config_backups` | 一键客户端配置的原始备份，用于还原 |
| `_migrations` | 数据库迁移版本记录，防止重复执行 |
