# 策略回测 (Backtest)

AiT 内置回测引擎，支持用历史 K 线数据模拟策略运行，验证 AI 交易模型的实际表现后再投入实盘。

## 功能概述

回测模块从 nofx-dev 项目移植并适配到 AiT 架构，复用 `kernel/` 策略引擎的完整决策链（选币→数据增强→AI 决策→模拟下单），确保回测环境与实盘逻辑完全一致。

### 核心能力

| 能力 | 说明 |
|------|------|
| **历史回放** | 按指定时间范围和周期回放 K 线，逐 bar 驱动策略引擎 |
| **多币种** | 支持同时回测多个交易对，自动从 Binance 拉取历史数据 |
| **AI 决策** | 复用实盘 AI 提示词和模型配置，每次决策均调用 LLM |
| **资金曲线** | 实时绘制权益曲线、持仓盈亏，直观展示策略表现 |
| **交易记录** | 完整的开平仓记录，含入场价、出场价、盈亏、持仓时间 |
| **绩效指标** | 总收益率、夏普比率、最大回撤、胜率、盈亏比 |
| **运行管理** | 支持暂停/恢复/停止/删除回测，运行状态持久化存储 |
| **导出** | 支持导出回测结果为 JSON |

### 前端界面

导航栏点击 **「回测」**（或英文 **Backtest**），进入回测页面，包含 4 个标签页：

- **概览** — 绩效指标卡片、活跃持仓列表
- **图表** — 权益曲线 + K 线图 + 交易标记
- **交易** — 完整交易历史表格
- **决策** — AI 决策日志（含思维链）

## API 接口

所有接口挂载在 `/api/backtest/` 路径下，需要认证。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/start` | 启动回测 |
| POST | `/pause` | 暂停回测 |
| POST | `/resume` | 恢复回测 |
| POST | `/stop` | 停止回测 |
| POST | `/delete` | 删除回测记录 |
| POST | `/label` | 修改回测标签 |
| GET | `/status` | 获取运行状态 (SSE) |
| GET | `/runs` | 列出所有回测记录 |
| GET | `/equity` | 获取权益曲线数据 |
| GET | `/trades` | 获取交易记录 |
| GET | `/metrics` | 获取绩效指标 |
| GET | `/decisions` | 获取决策日志 |
| GET | `/export` | 导出回测结果 |
| GET | `/klines` | 获取回测用 K 线数据 |

### 启动回测示例

```bash
curl -X POST http://localhost:8080/api/backtest/start \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "config": {
      "symbols": ["BTCUSDT", "ETHUSDT"],
      "timeframe": "1h",
      "start_time": "2026-01-01T00:00:00Z",
      "end_time": "2026-05-01T00:00:00Z",
      "initial_balance": 10000,
      "strategy_id": "default"
    }
  }'
```

## 文件结构

```
backtest/                    # 回测引擎核心（Go）
├── manager.go               # 回测生命周期管理
├── runner.go                # 逐 bar 回放引擎
├── config.go                # 配置结构定义
└── metrics.go               # 绩效指标计算

api/backtest.go              # HTTP 接口层
store/backtest.go            # 数据持久化层（SQLite/PostgreSQL）

web/src/components/backtest/ # 前端组件
├── BacktestPage.tsx         # 主页面
├── BacktestConfigForm.tsx   # 配置表单（分步向导）
├── BacktestRunList.tsx      # 运行列表
├── BacktestOverviewTab.tsx  # 概览标签
├── BacktestChartTab.tsx     # 图表标签
├── BacktestTradesTab.tsx    # 交易标签
└── BacktestDecisionsTab.tsx # 决策标签

web/src/lib/api/backtest.ts  # 前端 API 客户端
web/src/types/backtest.ts    # TypeScript 类型定义
```

## 配置参数

| 参数 | 类型 | 说明 |
|------|------|------|
| `symbols` | string[] | 回测交易对列表 |
| `timeframe` | string | K 线周期：1m, 5m, 15m, 1h, 4h, 1d |
| `start_time` | string | 回测起始时间 (ISO 8601) |
| `end_time` | string | 回测结束时间 (ISO 8601) |
| `initial_balance` | number | 初始资金 (USDT) |
| `strategy_id` | string | 复用的策略配置 ID |
| `model_id` | string | AI 模型 ID（可选，默认用策略配置的模型） |
| `leverage` | number | 杠杆倍数（可选） |
| `custom_prompt` | string | 自定义 AI 提示词（可选） |

## 技术说明

- **数据源**：优先从 Binance 公共 API 拉取历史 K 线，CoinAnk 作为 fallback
- **引擎复用**：回测共享 `kernel/` 策略引擎的完整决策链，包括选币逻辑、技术指标计算、AI 提示词构建
- **状态管理**：回测状态通过 SQLite/PostgreSQL 持久化，支持跨重启恢复
- **SSE 推送**：回测运行状态通过 Server-Sent Events 实时推送到前端
- **性能**：单次回测速度取决于 LLM 响应延迟，K 线数据加载和指标计算本身很快
