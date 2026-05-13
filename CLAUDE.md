# CLAUDE.md

本文件为 Claude Code 提供项目协作规范与架构导读。

## 交互纪律

- **[强制] 全部输出使用中文**，包括分析推理、技术讨论、文档与架构决策说明
- 输出精简，直接落盘。禁止问候、解释、确认语
- **[Token 效率]** 结论前置，依据紧随。禁止描述性铺垫、拟人化措辞、冗余修饰词
- 只交付当前目标的最少代码集。禁止超前抽象、臆测性开发
- 变更保持 100% 指令溯源性。禁止顺手重构未受影响的代码、擅改历史排版
- 遇指令歧义或架构冲突时主动提问，禁止静默决策
- 所有结论必须有依据，引用时指明文件名和位置

## 语言规范

- **代码注释**：中文，说明"为什么"——设计意图、约束条件、非显而易见的逻辑
- **代码标识符**：英文（遵循 Go 社区惯例），命名清晰自解释
- **提交信息**：中文简述，`type(scope): 描述` 格式
- **内部思考**：语言不限，以推理质量优先

## 构建与运行

```bash
make build              # 当前平台编译
make build-all          # 交叉编译全平台
go run ./cmd/polaris    # 开发模式启动
go test ./...           # 运行全部测试
go build ./...          # 仅做编译检查
```

数据目录：`~/.polaris-gateway/`（SQLite 数据库）  
管理后台：`http://127.0.0.1:28888/dashboard`

## 项目概览

**Polaris Gateway** 是多协议 LLM API 代理网关，在 OpenAI、Anthropic、Google Agent Platform 三种协议之间互相转换，提供节点负载均衡、熔断保护与用量计费。核心价值：客户端无感知地在多个上游账号之间轮询，规避单账号的速率限制。

## 架构

### 请求处理流程

1. HTTP 入口根据 URL 路径识别客户端使用的源协议
2. 从请求中提取模型名称
3. 路由引擎按模型匹配规则、节点状态、优先级选出可用节点并标记占用
4. 查找对应的协议转换器，将请求格式转换后转发到上游
5. 将上游响应转换回客户端协议格式返回，释放节点

### 目录结构

```
internal/
  router/          # 路由引擎：节点池、状态机、负载均衡
  config/          # 配置加载与热重载
  translators/
    anthropic/     # Anthropic 协议的输入解析与输出转换
    openai/        # OpenAI 协议适配
    google/        # Google Agent Platform 原生协议适配
    utils/         # 共享工具：计费、HTTP 客户端、错误处理
  models/          # 模型价格目录
  db/              # SQLite 持久化与异步写入
  admin/           # 管理后台 API
```

### 协议转换器

每种协议对有独立的转换器，通过包初始化自动注册。当前支持的转换方向：

| 源协议 | 目标协议 | 说明 |
|--------|----------|------|
| Anthropic | Google Agent Platform | Gemini 协议转换；Claude 模型直通 |
| Anthropic | OpenAI | 协议转换 |
| Anthropic | Anthropic | 透传（多账号轮询） |
| OpenAI | Google Agent Platform | 协议转换 |
| OpenAI | OpenAI | 透传（多账号轮询） |
| Google | Google | 透传（多账号轮询） |
| Google | OpenAI | 协议转换 |

### 节点状态机

```
Idle ──→ Busy ──→ Idle                    正常完成，节点归还
              └──→ Cooldown               失败，进入指数退避冷却
Cooldown ──→ Probation                   冷却到期，进入试探状态
Probation ──→ Busy ──→ Idle              探路成功，恢复正常
              └──→ Cooldown              探路失败，重新冷却
Cooldown（已封顶）再次失败 ──→ Exhausted  永久隔离
```

冷却时长指数退避，上限由配置决定。节点并发安全由互斥锁逐节点保护，避免同一账号被并发请求同时占用（防 429）。

### 配置与热重载

全部配置持久化到 SQLite，分三类：全局参数（监听地址、熔断参数）、上游节点账号、路由规则（含 JSON 格式的模型映射）。管理后台执行 CRUD 后自动触发内存配置重建，无需重启服务。

## 关键设计约束

### Anthropic → Google Agent Platform（Gemini）转换

**请求映射：**
- Gemini 的 JSON Schema `type` 字段为严格大写枚举，不支持数组形式；`null` 类型需转换为 `nullable` 标记
- Gemini 不支持部分标准 JSON Schema 关键字（`additionalProperties`、`allOf`、`oneOf`、`if/then/else`、`not`、`$ref` 等），必须在转发前清洗
- 对话轮次要求严格 user/model 交替；连续同角色消息需合并为一条
- 工具调用响应（functionResponse）的名称必须与对应工具调用（functionCall）名称严格一致
- 对话历史中的思考块（thinking/redacted_thinking）在转发给 Gemini 前丢弃
- 默认禁用所有内容安全过滤类别，避免代码和安全研究内容被误拦截

**响应映射：**
- Gemini 的思考内容（thought=true 的 part）转换为 Anthropic thinking 内容块；多个连续 thought 分片合并为一个块
- 思考 token 和工具描述 token 从 Gemini 的用量元数据中分别提取并计入对应的 Anthropic 用量字段
- Gemini 的停止原因（finishReason）需逐一映射到 Anthropic 的 stop_reason 语义

**Claude 直通（GEAP 合作伙伴模型）：**
- 目标模型以 `claude-` 开头时，走 GEAP 的 rawPredict / streamRawPredict 端点
- 请求体需注入 `anthropic_version` 字段并移除 `model` 字段（模型名在 URL 中指定）
- count_tokens 端点有独立处理路径

### 计费

- Token 数量优先使用上游返回的精确值，无法获取时降级为字节估算
- 用量通过带缓冲的 channel 异步写入数据库，不阻塞请求路径
- 节点预算超限（配置的消费比例）在启动时检测，超限节点直接进入 Exhausted 状态

### 数据库迁移

使用嵌入式 SQL 文件，启动时按文件名顺序自动执行，已执行的迁移通过追踪表跳过。新增迁移文件命名格式：`NNN_描述.sql`。
