# Polaris-Hermes 架构设计文档

**Polaris-Hermes** 是一款全能型大语言模型 API 代理分发与并发控制网关。它从最初专注于 Google Vertex AI 账号适配的工具，全面进化为支持 OpenAI、Anthropic、Google Gemini 原生、Google Agent Platform、本地模型（Ollama/vLLM）等**所有主流 LLM 协议**的通用 AI 代理网关。

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
Client Request
  → http.go (协议检测 / 提取模型名)
  → MatchAndAcquireRoute (路由匹配 + 等待可用节点)
  → Manager (优先级排序 + CAS 锁抢占)
  → Translator (协议转换 & 凭证注入 & 请求上游)
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
- **双层锁热重载**: 管理后台 (Web UI) 的 CRUD 操作触发 `config.ReloadFromDB()`。网关持有全局读写锁 (`poolMutex`) 和单节点互斥锁 (`mu`)，在内存中静默重建节点池与路由快照，**实现无感热加载，不中断存量流量**。

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

### 4.3 双轨制模型映射引擎 (Model Mapping)
为适应跨越协议（如 OpenAI 请求直接打给 Anthropic 或 Google）的场景，网关提供双轨制的模型映射策略：

- **极简模式 (Intelligent / Semantic Mapping)**：
  主打"开箱即用"与"不认死 ID，只认模型档次"。底层引入了 `CapabilityTier`（模型能力梯队）的概念，将杂乱的模型归类为三大核心类别：
  1. **`smart` (旗舰型)**：主攻高难度代码与复杂推理，如 `gpt-4o`, `claude-3-5-sonnet`, `gemini-2.5-pro`。
  2. **`fast` (极速型)**：主攻低延迟、高并发的轻量任务，如 `gpt-4o-mini`, `claude-3-haiku`, `gemini-2.5-flash`。
  3. **`reasoning` (沉思型)**：主攻 CoT 深度思维链逻辑题，如 `o1`, `o3-mini`, `DeepSeek-R1`。
  **运行机制**：当客户端请求 `gpt-4o-mini` 时，网关识别其梯队为 `fast`。若路由命中 Anthropic 节点，Translator 会自动将其映射为 Anthropic 的 `fast` 级代表模型（如 Haiku），对客户端完全透明且无需修改任何路由规则。
  
- **专业模式 (Pro / Deterministic 1-to-1 Mapping)**：
  主打"绝对控制"。底层通过 `UserCustomRoute` 实体实现完全可配置的用户自定义路由表。用户可在前端高级面板中手动配置强制映射（Hard-coded Mapping），支持精确匹配或正则模式拦截。
  **通配符兜底 (`*`)**：支持特殊的全局通配符 `*` 映射规则。当配置为 `*` 时，客户端发来的所有未被精确命中的异构模型请求，都将被无脑引流并强制转换为该指定的唯一模型（例如将所有请求强制引向特定的私有化部署大模型）。
  适用于将内部代号、旧模型 ID 或不规范的调用统一收拢强制引流至特定微调模型或本地部署模型的场景。**专业模式（Custom Routes）优先级绝对高于极简模式的系统智能推断。**

### 4.4 一键客户端配置 (Client Auto-Config)
**核心亮点功能**：打破商业 AI 客户端（如 Claude Code、Codex 等）锁定官方 API 的限制。

- **痛点**：Claude Code、OpenAI Codex 等软件将 Base URL 固定写死，普通用户无法切换大模型厂商或使用自购的第三方 API Key（如 DeepSeek）。
- **方案**：在管理后台一键注入 Polaris-Hermes 代理配置，自动修改目标软件的环境变量或配置文件，将流量劫持到本地网关，再由网关转发给真正的目标大模型。
- **支持客户端**：Claude Code、OpenAI Codex、OpenCode、Gemini CLI、Hermes、OpenClaw 等。
- **推荐组合**：**特别推荐使用 DeepSeek 作为后端**。DeepSeek 兼容 OpenAI 协议，价格极具竞争力，配合 Polaris-Hermes 的多账号轮询和熔断机制，可实现近乎无感的高可用 AI 编程体验。

### 4.5 简单模式 / 专业模式 双 UI 切换
- **简单模式**：精简表单，隐藏高级配置项（如并发上限、请求间隔、余额预警等），降低用户心智负担，适合个人开发者快速上手。
- **专业模式**：展开全部控制项，适合有精细化账号管理需求的企业级用户或高级玩家，与"专业模式模型映射"联动。

---

## 5. 数据库表结构概要

| 表名 | 说明 |
|------|------|
| `sys_providers` | 系统内置厂商字典（30+ 家），包含协议类型、默认配置 |
| `sys_provider_auth_modes` | 厂商鉴权模式字典，1 对 N，定义 auth_type、header_name、url_template、required_fields |
| `user_providers` | 用户自定义的渠道账号，关联 sys_providers 和 sys_provider_auth_modes |
| `routes` | 路由规则表，定义来源模型匹配规则 → 目标节点池的映射 |
| `settings` | 全局系统配置，包含熔断参数、UI 模式等 |
| `account_logs` | 异步写入的 Token 消费账单记录 |
| `_migrations` | 数据库迁移版本记录，防止重复执行 |
