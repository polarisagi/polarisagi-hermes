# Polaris Gateway 🌌

[![Go Report Card](https://goreportcard.com/badge/github.com/mrlaoliai/polaris-gateway)](https://goreportcard.com/report/github.com/mrlaoliai/polaris-gateway)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/v/release/mrlaoliai/polaris-gateway)](https://github.com/mrlaoliai/polaris-gateway/releases)

<p align="center">
  <strong>🇬🇧 English</strong> | <a href="README_zh.md">🇨🇳 简体中文</a>
</p>

---

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

### 🧪 Local Testing Mode (For AI Coding Assistants)
When testing modified gateway code locally, **DO NOT** run it on the default `28888` port to avoid port conflicts with the running production instance. 

Use the test mode which listens on port `28889`:
```bash
make run-test
# Or manually:
TEST_MODE=true go run ./cmd/polaris
```
AI coding clients (like Claude Code) should change the target API URL to `http://127.0.0.1:28889/...` when sending test requests to the newly built gateway.

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
