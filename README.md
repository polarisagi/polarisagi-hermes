# Polaris Gateway 🌌

Polaris Gateway 是一款轻量级、智能化的 **大语言模型 API 代理分发与并发控制网关**
主要用于高效、安全地访问 **Gemini Enterprise Agent Platform** (原 Google Cloud Vertex AI)。专为高并发业务和多账号池轮询设计，完美解决因单一 API Key 限流、封禁或余额不足造成的业务中断问题。

> **注意**：
> Google Cloud 的 Vertex AI 现已正式更名为 **Agent Platform**。
> google 的 Gemini Enterprise Agent Platform 
> 官方文档：https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/start?hl=zh_CN#googlegenaisdk_textgen_with_txt-drest。
> 只有在Google Cloud 上创建的项目，并且访问 Gemini Enterprise Agent Platform 才能使用每个新的Google Cloud 账号的赠金。其他方式现在都不可以（google 改了赠金的使用政策）。

最新版本已彻底重构为 **零配置启动 (Zero-Config)**、**SQLite 数据库驱动**，并内置了管理页面 **Web Admin Dashboard**，为您提供开箱即用的商用级体验。

---

## ✨ 核心特性

1. **可视化后台与热加载**：无需修改任何配置文件，通过 Web UI 即可新增、编辑、停用 OpenAI 或 Vertex AI 节点。配置秒级热重载生效，无需重启服务。
2. **多账号池与智能并发隔离**：所有请求基于物理账号池排队，**物理级单并发隔离** 保证单账号同一时间只处理一个请求，极大降低 Google Vertex 封禁风险。
3. **四态状态机与动态熔断**：
   - 🟢 **Idle** (空闲) | 🟡 **Busy** (处理中) | 🔴 **Cooldown** (小黑屋) | 🟠 **Probation** (探路试用)
   - 支持自定义 `failure_threshold`（失败阈值）和 `failure_window_seconds`（时间窗口）。偶发网络波动不会立刻封杀，连续出错触发指数级熔断退避。
4. **精细化计费与拦截**：
   - 使用内置 SQLite 高效追踪每一笔 Tokens 消费。
   - 可针对单个账号设置最高消费额（`limit_percent`），触达安全水位后自动从路由池下线，防止资金失控烧穿。
5. **极简部署**：单文件无依赖，自带 Web UI 及嵌入式数据迁移，直接运行！

---

## 🚀 快速安装 (一键脚本)

### macOS / Linux
```bash
curl -sSL https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/install.sh | bash
```

### Windows
以**管理员身份**打开 PowerShell 并执行：
```powershell
iwr -useb https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/install.ps1 | iex
```

安装完成后，Polaris Gateway 将作为后台服务自动运行，并在系统重启时自启动。

---

## 🗑️ 一键卸载

如果您想卸载 Polaris Gateway 及后台服务，可以执行以下命令：

### macOS / Linux
```bash
curl -sSL https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/uninstall.sh | bash
```

### Windows
以**管理员身份**打开 PowerShell 并执行：
```powershell
iwr -useb https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/uninstall.ps1 | iex
```

> **注意**: 卸载脚本只会移除系统服务和二进制主程序。为了防止误删数据，您的所有账号配置和账单记录（数据库）将安全保留在用户目录 `~/.polaris-gateway` 下。如果需要彻底清理，请手动删除该目录。

---

## 🛠️ 开始使用

网关启动后，默认监听 `127.0.0.1:28888` 端口。

### 1. 登录 Admin Panel
打开浏览器访问：[http://127.0.0.1:28888/dashboard](http://127.0.0.1:28888/dashboard)
在这里你可以：
- 查看实时的 API 请求统计与账单流水。
- 动态添加新的 OpenAI、Anthropic 或 Vertex (GCP) 物理节点。
- 修改系统的熔断参数和网关监听端口。

### 2. 在业务端调用
将你的客户端（如 Claude code, Opencode, Aider, Codex 等）的 API URL 指向 Polaris Gateway。

**OpenAI 协议接入 (含透传 + 可选转 Vertex/Gemini)：**
- Base URL: `http://127.0.0.1:28888/v1/openai/`
- API Key: 任意值（网关将透明替换为你配置在后端的物理 Key）

**Anthropic (Claude) 协议接入 (含透传 + 可选转 Vertex/OpenAI)：**
- Base URL: `http://127.0.0.1:28888/v1/anthropic/`
- API Key: 任意值

**Vertex / Gemini 协议接入 (含透传 + 可选转 OpenAI)：**
- Base URL: `http://127.0.0.1:28888/v1/vertex/`
- API Key: 任意值

> URL 的第一个 path segment 明确声明客户端协议（`openai` / `anthropic` / `vertex`），网关据此决定透传或协议转换。透传模式（如 `openai→openai`）只做负载均衡和计费，不改请求体。
> 如果 Claude code，Codex 使用，建议配合 cc-switch`https://github.com/farion1231/cc-switch` 使用。

### 3. 常见 AI 客户端接入配置

**Opencode 客户端接入：**
创建或编辑 `opencode.json` 文件：
```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "google-vertex": {
      "options": {
        "baseURL": "http://127.0.0.1:28888/v1/vertex/"
      }
    }
  }
}
```

**Aider 客户端接入：**
配置环境变量并启动程序：
```bash
export VERTEX_API_KEY="test-key" 
export VERTEX_API_BASE="http://127.0.0.1:28888/v1/vertex/"
export VERTEXAI_PROJECT="project-id"
export VERTEXAI_LOCATION="global"
aider --model vertex_ai/gemini-3.1-pro-preview-customtools
```

---

## 📂 项目结构

项目严格遵循 Go 标准规范 `golang-standards/project-layout`：

```text
polaris-gateway/
├── cmd/
│   └── polaris/           # 程序的唯一入口 main.go
├── internal/
│   ├── config/            # 动态配置与内存状态引擎
│   ├── db/                # SQLite 操作与内置 Schema 迁移
│   ├── logger/            # 基于 slog 的结构化日志实现
│   ├── router/            # 全局通用路由器与智能调度分发引擎
│   ├── translators/       # 协议转换与适配器 (Anthropic/OpenAI/Vertex 等)
│   └── webapi/            # 控制台 Dashboard 与 Admin API
├── scripts/               # 安装脚本 (install.sh, install.ps1)
└── Makefile               # 便捷编译指令
```

## 🏗️ 开发者编译

如果你想从源码编译：

```bash
# 编译当前系统架构
make build

# 交叉编译全平台 (macOS, Linux, Windows)
make build-all
```

构建产物将输出至 `bin/` 目录下。

---

## 📄 开源协议与声明

本项目基于 [MIT License](LICENSE) 开源。

**特别声明**：如果您使用了本项目的代码，请务必在您的项目中保留或添加原作者声明：`作者 ID: mrlaoliai`。
