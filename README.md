# Polaris-Hermes 🌌

[![Go Report Card](https://goreportcard.com/badge/github.com/polarisagi/polarisagi-hermes)](https://goreportcard.com/report/github.com/polarisagi/polarisagi-hermes)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/v/release/polarisagi/polarisagi-hermes)](https://github.com/polarisagi/polarisagi-hermes/releases)

<p align="center">
  <strong>🇬🇧 English</strong> | <a href="README_zh.md">🇨🇳 简体中文</a>
</p>

---

**Polaris-Hermes** is a lightweight, highly intelligent **Universal LLM API Proxy & Concurrency Control Gateway**. 

Originally designed as a Google Vertex AI adapter, it has completely evolved into a **Universal AI Gateway**. It natively supports proxying, format conversion, and routing across almost all mainstream protocols (OpenAI, Google Gemini, Google Agent Platform, Anthropic, and Local models like Ollama/vLLM). It comes with a massive built-in dictionary covering **30+ global model providers** out-of-the-box. 

It completely solves business interruptions caused by API Key rate limits, bans, or depleted balances by utilizing multi-account rotation and intelligent concurrency queuing. The latest version is purely **Zero-Config**, driven by an embedded **SQLite** database, and comes with a built-in **Web Admin Dashboard**.

---

### ✨ Core Features

1. **🔑 One-Click Client Auto-Config (Highly Recommended!)**
   Break the restrictions of commercial AI clients (like Claude Code, Codex, Cursor) that lock you into official APIs! The Admin Panel lets you instantly inject Polaris-Hermes proxy settings, **allowing you to freely use your own API Keys and third-party models in closed software.**
   > 🔥 **Pro Tip: We highly recommend pairing this with [DeepSeek](https://platform.deepseek.com)**! DeepSeek is fully compatible with the OpenAI protocol and offers incredible performance at a disruptive price. When combined with Polaris-Hermes' multi-account rotation and circuit breakers, you achieve the ultimate seamless AI coding experience.

2. **🌐 Universal Protocol & Provider Support**
   Includes a built-in, ready-to-use dictionary of 30+ global AI providers (OpenAI, Anthropic, DeepSeek, Zhipu, Moonshot, Qwen, Doubao, Mistral, Groq, SiliconFlow, etc.). No need to manually dig up Base URLs anymore.

3. **🎛️ Simple / Pro Dual-Mode UI**
   **Simple Mode**: Streamlined forms for quick setup. **Pro Mode**: Unlocks full control over concurrency limits, billing alerts, intelligent retries, and advanced model mappings.

4. **🔀 Dual-Track Model Mapping Engine**
   - **Minimalist Mode (Intelligent Semantic Mapping)**: Automatically identifies model tiers and intelligently maps cross-protocol requests transparently.
   - **Pro Mode (1-to-1 Exact Mapping)**: Allows manual hardcoded routing rules with Regex support for absolute control.

5. **🛡️ Multi-Account Pool & Single Concurrency Isolation**
   Requests are queued based on physical accounts. Strict physical-level single-concurrency isolation prevents API bans (especially crucial for Google/Anthropic endpoints).

6. **⚡ 5-State Circuit Breaker & Auto-Recovery**
   🟢 Idle → 🟡 Busy → 🔴 Cooldown → 🟠 Probation → ⚫ Exhausted. Exponential backoff handles upstream 429/500 errors gracefully with seamless recovery.

7. **💰 Billing & Quota Management**
   Tracks token usage via SQLite. Supports setting maximum spend percentages to auto-disable accounts near exhaustion.

8. **🚀 Zero Dependency Deployment**
   Single binary, built-in Web UI, embedded DB migrations. Just run it!

---

### 🔀 Protocol Route Matrix

| Source Protocol (Client) | Target Protocol (Upstream) | Description |
|--------------------------|----------------------------|-------------|
| openai                   | openai                     | Passthrough + Rotation (Works for all OpenAI-compatible APIs) |
| openai                   | anthropic                  | OpenAI → Anthropic format conversion |
| openai                   | google                     | OpenAI → Google Gemini / Agent Platform conversion |
| anthropic                | anthropic                  | Passthrough + Rotation |
| anthropic                | openai                     | Anthropic → OpenAI format conversion |
| anthropic                | google                     | Anthropic → Google format conversion |
| google                   | google                     | Passthrough + Rotation |
| local                    | local                      | Local model passthrough (Ollama/vLLM, no auth) |

---

### 📂 Default Directory
All configurations, billing records, and SQLite databases (`polaris_hermes.db`) are safely stored in:
`~/.polarisagi-hermes/`

---

### 🚀 Quick Install

**macOS / Linux:**
```bash
curl -sSL https://raw.githubusercontent.com/polarisagi/polarisagi-hermes/main/scripts/install.sh | bash
```

**Windows (PowerShell as Admin):**
```powershell
iwr -useb https://raw.githubusercontent.com/polarisagi/polarisagi-hermes/main/scripts/install.ps1 | iex
```
*The gateway will run as a background service and auto-start on boot.*

---

### 🛠️ Getting Started

By default, the gateway listens on `127.0.0.1:27777`.

1. **Admin Panel**: Visit [http://127.0.0.1:27777/dashboard](http://127.0.0.1:27777/dashboard)
2. **Add Channel**: Select Protocol → Choose Provider → Enter API Key → Save.
3. **One-Click Client Config**: Navigate to "Client Config", pick your target software, select your injected API key, and you're done!
4. **API Endpoints** (Point your AI tools to these URLs, API Key can be any dummy text):
   - OpenAI Protocol: `http://127.0.0.1:27777/v1/openai/`
   - Anthropic Protocol: `http://127.0.0.1:27777/v1/anthropic/`
   - Google Protocol: `http://127.0.0.1:27777/v1/google/`

---

### 🧪 Local Testing Mode 

When testing modified gateway code locally, please use test mode (listens on port `28889`) to avoid conflicts with your running production instance:

```bash
make run-test
# Or manually:
TEST_MODE=true go run ./cmd/polaris
```

---

### 🗑️ Uninstall

**macOS / Linux:**
```bash
curl -sSL https://raw.githubusercontent.com/polarisagi/polarisagi-hermes/main/scripts/uninstall.sh | bash
```
**Windows:**
```powershell
iwr -useb https://raw.githubusercontent.com/polarisagi/polarisagi-hermes/main/scripts/uninstall.ps1 | iex
```
> **Note**: Uninstalling only removes the service and binary. Data remains safely in `~/.polarisagi-hermes/`. Delete it manually if you want a complete wipe.

---

### 📄 License
MIT License. *(If you use this code, please retain the original author credit: `mrlaoliai`)*

---

### 🌐 Links & Contact
* **Official Website**: [https://polarisagi.online/](https://polarisagi.online/)
* **GitHub Repository**: [https://github.com/polarisagi/polarisagi-hermes](https://github.com/polarisagi/polarisagi-hermes)
* **Author / Creator**: `mrlaoliai` (Find me on Xiaohongshu, Douyin, TikTok, and X)
* **Contact Email**: [polarisagi.online@gmail.com](mailto:polarisagi.online@gmail.com)
