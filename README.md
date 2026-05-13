<h1 align="center">AiT</h1>

<p align="center">
  <strong>AI 驱动的全自动加密货币交易系统</strong><br/>
  <strong>多交易所 · 多AI模型 · 社交热度信号 · 策略可视化</strong>
</p>

<p align="center">
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go" alt="Go"></a>
  <a href="https://reactjs.org/"><img src="https://img.shields.io/badge/React-18+-61DAFB?style=flat&logo=react" alt="React"></a>
  <a href="https://www.typescriptlang.org/"><img src="https://img.shields.io/badge/TypeScript-5.x-3178C6?style=flat&logo=typescript" alt="TypeScript"></a>
  <a href="https://www.python.org/"><img src="https://img.shields.io/badge/Python-3.10+-3776AB?style=flat&logo=python" alt="Python"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-AGPL--3.0-blue.svg?style=flat" alt="License"></a>
</p>

---

## 快速开始

```bash
# 一键安装（自动安装 Go/Node/TA-Lib、生成配置、编译项目）
curl -fsSL https://raw.githubusercontent.com/Aixxww/AiT/main/scripts/install.sh | bash

# 启动所有服务
cd ~/AiT && ./scripts/start.sh dev
```

浏览器打开 **http://localhost:3000** → 注册 → 添加 AI 模型和交易所 → 开始交易。

---

## 概述

AiT 是一个开源的全自动 AI 交易系统，专为加密货币合约交易设计。

系统通过 AI 模型实时分析市场数据（OI、资金费率、多空比、技术指标、社交热度），自动做出开仓/平仓决策，并在 10 家主流交易所执行交易。

**核心特性：**

- **多 AI 模型** — 支持 DeepSeek、OpenAI、Claude、Gemini、Grok、Kimi、MiniMax、小米 MiMo，运行时一键切换
- **多交易所** — Binance、Bybit、OKX、Bitget、KuCoin、Gate、Hyperliquid、Aster、Lighter、Indodax
- **社交热度信号** — 集成币安广场热帖监控，将社交情绪转化为交易信号
- **策略工作室** — 可视化配置：币种来源、技术指标、风控参数、AI 提示词
- **指标分析** — EMA、MACD、RSI、ATR、BOLL 等技术指标注入 AI 决策链
- **Telegram Bot** — 与 AI 交易助理对话，支持流式输出和工具调用
- **实盘看板** — 实时持仓、盈亏曲线、AI 决策日志（含思维链）

---

## 架构

```
┌─────────────────────────────────────────────────────┐
│                 Web Dashboard                        │
│          React + TypeScript + TradingView Charts     │
├─────────────────────────────────────────────────────┤
│              Go API Server (Gin)                     │
├──────────┬──────────┬──────────┬────────────────────┤
│ Strategy  │  Telegram │  Square   │  Anomaly         │
│  Engine   │   Agent   │  Monitor  │  Scanner         │
├──────────┴──────────┴──────────┴────────────────────┤
│             MCP AI Client Layer                      │
│   DeepSeek · OpenAI · Claude · Gemini · Grok         │
│   Kimi · MiniMax · Xiaomi MiMo                       │
├─────────────────────────────────────────────────────┤
│           Market Data Providers                      │
│   Binance · Coinank · Hyperliquid · TwelveData       │
│   NofxOS · Local Cache · Square Heat                 │
├─────────────────────────────────────────────────────┤
│           Exchange Connectors                        │
│   Binance · Bybit · OKX · Bitget · KuCoin · Gate     │
│   Hyperliquid · Aster · Lighter · Indodax            │
└─────────────────────────────────────────────────────┘
```

**数据流：**

```
币种来源选币 (ai500/OI/社交热度/混合)
  │
  ▼
市场数据增强 (OI/funding/LSR/价格排名/技术指标)
  │
  ▼
AI 模型分析 (思维链 + 多维数据 → JSON决策)
  │
  ▼
交易执行 (开仓/平仓/调整 → 交易所API)
  │
  ▼
盈亏追踪 + 决策日志 → 前端看板
```

---

## 功能模块

### 策略引擎 (`kernel/`)

- **币种来源** — `ai500`（动量排名）、`oi_top`/`oi_low`（持仓异动）、`square_heat`（社交热度）、`hyper_all`（Hyperliquid）、`mixed`（多源混合）
- **技术指标** — EMA、MACD、RSI、ATR、Bollinger Bands，策略中独立开关控制
- **AI 提示词** — `engine_prompt.go` 构建系统提示词 + 用户提示词，注入市场数据和指标分析
- **风控** — 手数上限、杠杆限制、排除币种列表、最小持仓金额

### 交易所适配 (`trader/`)

10 家交易所统一接口，支持：
- 合约交易（USDT-M / COIN-M）
- 持仓查询、止盈止损
- 余额同步、费率查询
- 强制平仓

### AI 模型层 (`mcp/`)

- **MCP 协议** — 统一的 AI 调用层，支持 OpenAI 兼容 API
- **8 个 Provider** — 每个有独立的超时、模型名、基础URL配置
- **Claw402 集成** — 通过 USDC 钱包支付，无需 API Key（Base 链微支付）
- **本地 Provider** — `"local"` 类型直连币安公共 API，零成本获取市场数据

### 社交热度 (`provider/square/` + `scripts/square-monitor/`)

- Python 独立服务：Playwright 抓取币安广场 → 热度评分 → 合约数据增强 → Web API
- Go 客户端：HTTP 读取 `/api/leaderboard`，按 composite_score 过滤
- 前端看板：`SquareHeatPanel` 实时展示热度信号、趋势方向、24h 涨跌

### 策略工作室 (`web/src/components/strategy/`)

可视化编辑器，配置项：
- 币种来源类型和参数
- 技术指标开关和参数
- AI 模型选择
- 风控参数（杠杆、手数、排除列表）
- 自定义 AI 提示词

### 前端看板 (`web/`)

- **交易仪表盘** — K线图、权益曲线、持仓列表、Square Heat 信号面板、AI 决策日志
- **策略管理** — 可视化编辑器 + 公开策略市场
- **AI 竞赛** — 多交易员实时排名
- **配置管理** — AI 模型、交易所、钱包

---

## 安装部署

### 环境要求

| 组件 | 最低版本 | 安装脚本自动处理 |
|------|----------|:---:|
| Go | 1.25+ | ✅ |
| Node.js | 18+ | ✅ |
| Python | 3.10+（仅 Square Monitor） | ✅ |
| TA-Lib | 0.4+（技术指标） | ✅ |

### 方式一：一键安装（推荐）

```bash
# 开发模式（自动安装所有依赖 + 编译）
curl -fsSL https://raw.githubusercontent.com/Aixxww/AiT/main/scripts/install.sh | bash

# Docker 模式（拉取预构建镜像）
curl -fsSL https://raw.githubusercontent.com/Aixxww/AiT/main/scripts/install.sh | bash -s -- --docker

# 最小安装（跳过 Python / Square Monitor）
curl -fsSL https://raw.githubusercontent.com/Aixxww/AiT/main/scripts/install.sh | bash -s -- --minimal
```

安装脚本自动检测操作系统（macOS / Ubuntu / CentOS / Arch），安装 Go、Node.js、TA-Lib 等依赖，生成加密密钥，编译项目。

### 方式二：手动安装

```bash
# 1. 克隆代码
git clone https://github.com/Aixxww/AiT.git
cd AiT

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env 填入 JWT_SECRET、DATA_ENCRYPTION_KEY 等
# 首次运行时密钥会自动生成

# 3. 安装依赖
go mod download
cd web && npm install && cd ..

# 4. 编译
CGO_ENABLED=1 go build -o ait .
cd web && npm run build && cd ..
```

### 启动服务

```bash
# 开发模式（后台启动 backend + frontend + square-monitor）
./scripts/start.sh dev

# 或使用 Makefile
make dev
```

服务启动后：
- **Web Dashboard:** http://localhost:3000
- **API Endpoint:** http://localhost:8080
- **Square Monitor:** http://localhost:8000（可选）

```bash
# 管理服务
./scripts/start.sh dev stop      # 停止
./scripts/start.sh dev status    # 状态
```

### Docker 部署

```bash
# 一键启动
./scripts/start.sh docker

# 或手动
docker compose up -d
```

### Railway 云部署

```bash
railway up
```

详见 `railway/` 目录配置文件。

---

## 使用流程

### 新手模式

1. 注册账号时选择 **新手模式**
2. 系统自动引导：AI 模型 → 交易所 → 策略 → 启动交易

### 高手模式

1. **AI 模型** — 添加 API Key 或配置 USDC 钱包（Claw402）
2. **交易所** — 连接交易所 API 密钥
3. **策略** — 在策略工作室创建：选择币种来源、配置指标、设置风控
4. **交易员** — 组合 AI + 交易所 + 策略 → 启动

所有操作通过 Web UI 完成：**http://localhost:3000**

---

## 项目结构

```
AiT/
├── api/                 # HTTP 接口层 (Gin)
├── auth/                # JWT 认证
├── config/              # 配置加载
├── crypto/              # 加密服务 (AES/RSA)
├── kernel/              # 策略引擎核心
│   ├── engine.go        # 选币 → 数据增强 → AI 决策
│   ├── engine_prompt.go # AI 提示词构建
│   └── grid_engine.go   # 网格交易引擎
├── mcp/                 # AI 模型调用层 (MCP 协议)
│   └── provider/        # 8 个 LLM Provider 适配器
├── market/              # 市场数据客户端
├── provider/            # 数据源 Provider
│   ├── binance/         # 币安公共 API 客户端
│   ├── local/           # 本地缓存 (Binance 数据)
│   ├── square/          # 币安广场热度客户端
│   └── ...              # Coinank, Hyperliquid, TwelveData
├── scripts/
│   ├── install.sh       # 一键安装脚本
│   ├── start.sh         # 统一启动脚本 (dev/docker/prod)
│   └── square-monitor/  # Python 社交热度监控 (独立服务)
├── store/               # 数据持久化 (SQLite/PostgreSQL)
├── trader/              # 交易所适配器 (10家)
├── telegram/            # Telegram Bot 集成
├── wallet/              # 钱包管理
├── web/                 # React 前端
│   ├── src/
│   │   ├── components/  # UI 组件
│   │   ├── pages/       # 页面
│   │   ├── lib/         # 工具库 (api, hooks, utils)
│   │   └── i18n/        # 国际化
│   └── package.json
├── main.go              # 程序入口
├── Makefile             # 构建/测试/启动命令
├── go.mod               # Go 模块定义
└── .env.example         # 环境变量模板
```

---

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.25, Gin, GORM, SQLite/PostgreSQL, ZeroLog |
| 前端 | React 18, TypeScript, Vite, Tailwind CSS, Zustand, SWR |
| 图表 | TradingView Lightweight Charts, Recharts |
| AI | MCP 协议, OpenAI 兼容 API, Claw402 USDC 微支付 |
| Python | FastAPI, Playwright, BeautifulSoup (Square Monitor) |
| 部署 | Docker Compose, Railway, Nginx |
| 通信 | REST API, WebSocket (实时行情), Telegram Bot API |

---

## 币种来源说明

| 来源 | 类型 | 数据 | 适用场景 |
|------|------|------|----------|
| `ai500` | 量化 | 币安涨幅 + 成交量 + 活跃度排名 | 市场动量跟踪 |
| `oi_top` | 量化 | OI 增加排名 | 持仓异动捕捉 |
| `oi_low` | 量化 | OI 低排名 | 反向信号 |
| `square_heat` | 社交 | 币安广场热帖 + 合约信号评分 | 情绪面驱动 |
| `hyper_all` | DEX | Hyperliquid 全量合约 | DEX 交易者 |
| `mixed` | 混合 | 多源合并 + 符号去重 | 信号交叉确认 |

---

## AI 模型支持

| 模型 | Provider | 默认模型 | 备注 |
|------|----------|----------|------|
| DeepSeek | deepseek | deepseek-chat | 性价比最高 |
| Qwen | qwen | qwen3-max | 阿里通义千问 |
| OpenAI | openai | gpt-5.1 | GPT 系列 |
| Claude | claude | claude-opus-4-6 | Anthropic |
| Gemini | gemini | gemini-3.1-pro | Google |
| Grok | grok | grok-3-latest | xAI |
| Kimi | kimi | moonshot-v1-auto | 月之暗面 |
| MiniMax | minimax | MiniMax-M2.7 | MiniMax |
| Xiaomi MiMo | mimo | mimo-v2.5-pro | 小米，5分钟超时 |

---

## 交易所支持

| 交易所 | 类型 | 状态 |
|--------|------|------|
| Binance | CEX | ✅ |
| Bybit | CEX | ✅ |
| OKX | CEX | ✅ |
| Bitget | CEX | ✅ |
| KuCoin | CEX | ✅ |
| Gate | CEX | ✅ |
| Hyperliquid | Perp-DEX | ✅ |
| Aster | Perp-DEX | ✅ |
| Lighter | Perp-DEX | ✅ |
| Indodax | CEX | ✅ |

---

## 安全特性

- **传输加密** — AES-256 + RSA 密钥交换，API Key 加密传输
- **SSRF 防护** — 所有自定义 URL 经 `security.ValidateURL()` 校验
- **JWT 认证** — 支持 Token 黑名单、过期自动失效
- **API Key 脱敏** — 存储加密，前端仅显示 `has_api_key` 布尔值
- **交易所白名单** — 支持 IP 白名单配置

---

## 贡献

欢迎提交 Issue 和 Pull Request。

详见 [CONTRIBUTING.md](CONTRIBUTING.md) · [安全策略](SECURITY.md)

---

## 风险提示

> **AI 自动交易存在重大风险。建议仅用于学习研究或小额测试。**
> **本项目不构成任何投资建议，使用者需自行承担交易风险。**

---

## 许可证

[AGPL-3.0](LICENSE)
