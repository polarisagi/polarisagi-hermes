# Polaris Gateway 🌌

[![Go Report Card](https://goreportcard.com/badge/github.com/mrlaoliai/polaris-gateway)](https://goreportcard.com/report/github.com/mrlaoliai/polaris-gateway)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/v/release/mrlaoliai/polaris-gateway)](https://github.com/mrlaoliai/polaris-gateway/releases)

<p align="center">
  <a href="#english"><strong>🇬🇧 English</strong></a> · 
  <a href="#简体中文"><strong>🇨🇳 简体中文</strong></a>
</p>

---

<h2 id="english">🇬🇧 English</h2>

**Polaris Gateway** is a lightweight, intelligent **LLM API Proxy & Concurrency Control Gateway**. 
It is designed to efficiently and safely route requests to the **Gemini Enterprise Agent Platform** (formerly Google Cloud Vertex AI) and other providers like OpenAI and Anthropic. It completely solves business interruptions caused by API Key rate limits, bans, or depleted balances by utilizing multi-account rotation and intelligent concurrency queuing.

Latest version is purely **Zero-Config**, driven by an embedded **SQLite** database, and comes with a built-in **Web Admin Dashboard**.

### ✨ Core Features
1. **Visual Admin Dashboard & Hot Reload**: Add/Edit/Disable API nodes via Web UI. Changes take effect instantly without restarting the service.
2. **Multi-Account Pool & Single Concurrency Isolation**: Requests are queued based on physical accounts. Strict single-concurrency isolation prevents Vertex AI bans.
3. **Dynamic Circuit Breaker**: 4-State Machine (🟢 Idle | 🟡 Busy | 🔴 Cooldown | 🟠 Probation) with customizable failure thresholds and backoff times.
4. **Billing & Quota Management**: Tracks token usage via SQLite. Supports setting maximum spend limits (`limit_percent`) to auto-disable accounts near exhaustion.
5. **Zero Dependency**: Single binary, built-in Web UI, embedded DB migrations. Just run it!

### 🔀 Protocol Route Matrix

The gateway supports 6 routing paths. Each route is configured via the Admin Dashboard (source protocol → target protocol):

| Source | Target | Description |
|--------|--------|-------------|
| anthropic | anthropic | Pure passthrough — multi-account round-robin with billing |
| anthropic | google | Gemini format conversion; Claude models use GEAP `rawPredict` passthrough |
| anthropic | openai | Anthropic → OpenAI format conversion |
| openai | openai | Pure passthrough — multi-account round-robin with billing |
| openai | google | OpenAI → Vertex AI OpenAI-compatible endpoint conversion |
| google | google | Pure passthrough — multi-account round-robin with billing |

### 📂 Default Directory
All configurations, database files (`polaris_gateway.db`), and local state are safely stored in:
`~/.polaris-gateway/`

### 🚀 Quick Install

**macOS / Linux:**
```bash
curl -sSL https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/install.sh | bash
```

**Windows (PowerShell as Admin):**
```powershell
iwr -useb https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/install.ps1 | iex
```
*The gateway will run as a background service and auto-start on boot.*

### 🛠️ Getting Started
By default, the gateway listens on `127.0.0.1:28888`.

1. **Admin Panel**: Visit [http://127.0.0.1:28888/dashboard](http://127.0.0.1:28888/dashboard) to view stats and manage your API keys.
2. **API Endpoints**: Point your AI clients (Cursor, Aider, Opencode, etc.) to:
   - OpenAI: `http://127.0.0.1:28888/v1/openai/`
   - Anthropic: `http://127.0.0.1:28888/v1/anthropic/`
   - Google Agent Platform (GEAP): `http://127.0.0.1:28888/v1/google/`
   *(API keys can be anything, the gateway will swap them with your physical keys)*

> **Google Agent Platform REST API Reference**: https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest
> **Note**: The legacy `/v1/vertex/` path is still supported for backward compatibility.

> **Note**: If you are using Claude Code or Codex, it is recommended to use them together with [cc-switch](https://github.com/farion1231/cc-switch).

### 🗑️ Uninstall

**macOS / Linux:**
```bash
curl -sSL https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/uninstall.sh | bash
```
**Windows:**
```powershell
iwr -useb https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/uninstall.ps1 | iex
```
> **Note**: Uninstalling only removes the service and binary. Data remains safely in `~/.polaris-gateway/`.

### 📄 License
MIT License. *(If you use this code, please retain the original author credit: `mrlaoliai`)*

---

<h2 id="简体中文">🇨🇳 简体中文</h2>

**Polaris Gateway** 是一款轻量级、智能化的 **大语言模型 API 代理分发与并发控制网关**。
专为高并发业务和多账号池轮询设计，完美解决因单一 API Key 限流、封禁或余额不足造成的业务中断问题。主要用于高效、安全地访问 **Gemini Enterprise Agent Platform** (原 Google Cloud Vertex AI)，同时支持 OpenAI 和 Anthropic 等主流协议的跨协议转发。

最新版本已彻底重构为 **零配置启动 (Zero-Config)**、**SQLite 数据库驱动**，并内置了 **Web 管理面板**，为您提供开箱即用的商用级体验。

### ✨ 核心特性
1. **可视化后台与热加载**：通过 Web UI 即可新增、编辑、停用节点。配置秒级热重载生效，无需重启服务。
2. **多账号池与智能并发隔离**：所有请求基于物理账号池排队，**物理级单并发隔离** 保证单账号同一时间只处理一个请求，极大降低 Vertex 封禁风险。
3. **四态状态机与动态熔断**：🟢 Idle | 🟡 Busy | 🔴 Cooldown | 🟠 Probation。支持自定义失败阈值和退避时间。
4. **精细化计费与拦截**：内置 SQLite 高效追踪 Tokens 消费。可设置消费最高比例，防资金烧穿。
5. **极简部署**：单文件无依赖，自带 Web UI 及嵌入式数据迁移，直接运行。

### 🔀 协议路由矩阵

网关支持 6 条路由路径，在管理后台中按"源协议 → 目标协议"配置：

| 源协议 | 目标协议 | 说明 |
|--------|----------|------|
| anthropic | anthropic | 透传直通 — 多账号轮询 + 计费 |
| anthropic | google | Gemini 协议转换；`claude-*` 模型走 GEAP `rawPredict` 原生直通 |
| anthropic | openai | Anthropic → OpenAI 格式转换 |
| openai | openai | 透传直通 — 多账号轮询 + 计费 |
| openai | google | OpenAI → Vertex AI OpenAI 兼容端点转换 |
| google | google | 透传直通 — 多账号轮询 + 计费 |

### 📂 默认数据目录
程序的所有配置、账单记录和 SQLite 数据库 (`polaris_gateway.db`) 均安全保存在用户主目录的隐藏文件夹中：
`~/.polaris-gateway/`

### 🚀 快速安装

**macOS / Linux:**
```bash
curl -sSL https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/install.sh | bash
```

**Windows (以管理员身份打开 PowerShell):**
```powershell
iwr -useb https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/install.ps1 | iex
```
*安装完成后，Polaris Gateway 将作为后台服务自动运行，并开机自启。*

### 🛠️ 开始使用
网关启动后，默认监听 `127.0.0.1:28888` 端口。

1. **登录 Admin Panel**: 浏览器访问 [http://127.0.0.1:28888/dashboard](http://127.0.0.1:28888/dashboard) 以管理节点和查看账单。
2. **在业务端调用**: 将客户端的 API URL 指向以下地址（API Key 填任意值即可）：
   - OpenAI 协议: `http://127.0.0.1:28888/v1/openai/`
   - Anthropic 协议: `http://127.0.0.1:28888/v1/anthropic/`
   - Google Agent Platform (GEAP) 协议: `http://127.0.0.1:28888/v1/google/`

> **Google Agent Platform 官方 REST API 文档**: https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest
> **注意**: 旧路径 `/v1/vertex/` 仍然支持，向后兼容现有客户端配置。

> **提示**: 如果您在使用 Claude Code 或 Codex，建议配合 [cc-switch](https://github.com/farion1231/cc-switch) 使用。

### 🗑️ 一键卸载

**macOS / Linux:**
```bash
curl -sSL https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/uninstall.sh | bash
```
**Windows:**
```powershell
iwr -useb https://raw.githubusercontent.com/mrlaoliai/polaris-gateway/main/scripts/uninstall.ps1 | iex
```
> **注意**: 卸载脚本只会移除系统服务和二进制主程序。为了防止误删数据，您的所有账号配置和账单记录（数据库）将安全保留在 `~/.polaris-gateway/` 目录下。如需彻底清理，请手动删除该目录。

### 📄 开源协议
MIT License. *(如果您使用了此代码，请保留原作者信息：`mrlaoliai`)*