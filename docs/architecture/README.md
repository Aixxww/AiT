# AiT Architecture Documentation

**Language:** [English](README.md) | [中文](README.zh-CN.md)

Technical documentation for developers who want to understand AiT internals.

---

## Overview

AiT is a full-stack AI trading platform for cryptocurrency and US stock markets:

- **Backend:** Go (Gin framework, SQLite)
- **Frontend:** React/TypeScript (Vite, TailwindCSS)
- **AI Models:** DeepSeek, Qwen, OpenAI (GPT-5.2), Claude, Gemini, Grok, Kimi
- **Exchanges:** Binance, Bybit, OKX, Hyperliquid, Aster, Lighter

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              AiT Platform                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐  ┌─────────────────────────────────────┐│
│  │  Strategy   │  │         Live Trading                ││
│  │   Studio    │  │        (Auto Trader)                ││
│  └──────┬──────┘  └──────────────────┬──────────────────┘│
│         │                            │                   │
│         └────────────────────────────┘                   │
│                                    │                                        │
│                          ┌─────────▼─────────┐                              │
│                          │   Core Services   │                              │
│                          │  - Market Data    │                              │
│                          │  - AI Providers   │                              │
│                          │  - Risk Control   │                              │
│                          └─────────┬─────────┘                              │
│                                    │                                        │
│         ┌──────────────────────────┼──────────────────────────┐            │
│         │                          │                          │            │
│  ┌──────▼──────┐         ┌─────────▼─────────┐      ┌────────▼────────┐   │
│  │  Exchanges  │         │     Database      │      │   Frontend UI   │   │
│  │  (CEX/DEX)  │         │    (SQLite)       │      │   (React SPA)   │   │
│  └─────────────┘         └───────────────────┘      └─────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Module Documentation

### Core Modules

| Module | Description | Documentation |
|--------|-------------|---------------|
| **Strategy Studio** | Strategy configuration, coin selection, data assembly, AI prompts | [STRATEGY_MODULE.md](STRATEGY_MODULE.md) |
| **Hunter v7 Signal Router** | SnapshotStore-based multi-setup coin selector, JSON signal protocol, and live validation workflow | [HUNTER_V7_SIGNAL_ROUTER.zh-CN.md](HUNTER_V7_SIGNAL_ROUTER.zh-CN.md) |

### Module Overview

#### Strategy Module
Complete strategy configuration system including:
- Coin source selection (static list, AI500 pool, OI ranking)
- Market data indicators (K-lines, EMA, MACD, RSI, ATR)
- Prompt construction (system prompt, user prompt, sections)
- AI response parsing and decision execution
- Risk control enforcement

**[Read Full Documentation →](STRATEGY_MODULE.md)**

#### Hunter v7 Signal Router
Multi-setup alpha signal router covering:
- SnapshotStore hot-snapshot data flow
- 10 setup modules
- `hunter_v7_signal_json` AIT prompt protocol
- Live validation command and latency budget

**[Read Documentation →](HUNTER_V7_SIGNAL_ROUTER.zh-CN.md)**

---

## Project Structure

```
nofx/
├── main.go                    # Entry point
├── api/                       # HTTP API (Gin framework)
├── trader/                    # Trading execution layer
├── strategy/                  # Strategy engine
├── market/                    # Market data service
├── mcp/                       # AI model clients
├── store/                     # Database operations
├── auth/                      # JWT authentication
├── manager/                   # Multi-trader management
└── web/                       # React frontend
    ├── src/pages/             # Page components
    ├── src/components/        # Shared components
    └── src/lib/api.ts         # API client
```

---

## Core Dependencies

### Backend (Go)

| Package | Purpose |
|---------|---------|
| `gin-gonic/gin` | HTTP API framework |
| `adshao/go-binance` | Binance API client |
| `markcheno/go-talib` | Technical indicators |
| `golang-jwt/jwt` | JWT authentication |

### Frontend (React)

| Package | Purpose |
|---------|---------|
| `react` | UI framework |
| `recharts` | Charts and visualizations |
| `swr` | Data fetching |
| `zustand` | State management |
| `tailwindcss` | CSS framework |

---

## Quick Links

- [Strategy Module](STRATEGY_MODULE.md) - How strategies work
- [Getting Started](../getting-started/README.md) - Setup guide
- [FAQ](../faq/README.md) - Frequently asked questions

---

## For Developers

**Want to contribute?**
- Read the module documentation above
- Check [Open Issues](https://github.com/Aixxww/AiT/issues)
- Join our community

**Repository:** https://github.com/Aixxww/AiT

---

[← Back to Documentation](../README.md)
