# Hunter v7 Signal Router 架构说明

> 更新时间：2026-06-04

Hunter v7 是 AiT 选币引擎的新一代多形态信号路由器。它不再只用单一综合分筛选标的，而是基于统一快照数据识别不同市场结构下的可交易机会，再把结构化信号交给 AIT 交易引擎进行二次 AI 决策。

## 1. 数据流

运行中交易链路如下：

```text
Binance Futures REST/WS
  -> datafetch.DataCollector
  -> datafetch.Store SnapshotStore
  -> kernel.SnapshotEngine
  -> StrategyEngine.scoreFromSnapshot()
  -> local.ScoreHunterV7()
  -> CandidateCoin
  -> AIT prompt hunter_v7_signal_json
  -> AI decision JSON
```

核心原则：

- 数据采集由 `SnapshotEngine` 后台维护，交易循环读取热快照。
- `datafetch.Store.Current()` 是原子指针读取，不应在交易循环内重新拉全量 Binance 明细。
- `hunter_v7` 已纳入 SnapshotEngine 初始化条件，和 `hunter`、`hunter_sniff`、`ai500` 共用同一份快照。

冷启动首次构建 Snapshot 可能超过 10 秒；运行中从热快照筛选 v7 信号通常是几十到几百毫秒。

## 2. v7 信号家族

Hunter v7 当前注册 10 类 setup：

| Setup | Direction | 目标机会 |
|---|---|---|
| `pullback_reversal_long` | LONG | 趋势回调后的支撑反转 |
| `short_squeeze_long` | LONG | 空头拥挤后的向上挤仓 |
| `trend_breakout_long` | LONG | 压缩区间向上突破 |
| `leader_momentum_long` | LONG | 强势龙头动量延续 |
| `panic_reversal_long` | LONG | 急跌后的 V 型反弹 |
| `accumulation_breakout_long` | LONG | 吸筹压缩后的突破 |
| `distribution_short` | SHORT | 拉高派发后的做空 |
| `long_squeeze_short` | SHORT | 多头拥挤后的踩踏 |
| `range_reversion` | LONG/SHORT | 区间边缘均值回归 |
| `funding_reversal` | LONG/SHORT | 资金费率/拥挤度反转 |

单轮实时行情不会必然触发全部 setup。验证时应看当前 market regime 是否与触发的 setup 匹配，而不是要求每一轮覆盖所有形态。

## 3. 输出 JSON 协议

每个 v7 信号会在 AIT prompt 中输出一行紧凑 JSON：

```text
hunter_v7_signal_json: {...}
```

核心字段：

| 字段 | 含义 |
|---|---|
| `symbol` | 合约交易对 |
| `direction` | `LONG` / `SHORT` |
| `setup_type` | v7 setup 类型 |
| `status` | `candidate` / `wait_confirm` / `conflict_watch` |
| `market_regime` | 当前大盘 regime |
| `entry_mode` | 入场方式，如 `wait_reclaim`、`wait_price_reversal` |
| `ai_priority` | 给 AI 的优先级排序分 |
| `setup_score` | 形态分 |
| `timing_score` | 时机分 |
| `risk_score` | 风险分，越高越危险 |
| `risk_level` | `LOW` / `MEDIUM` / `HIGH` / `EXTREME` |
| `reason_codes` | 触发原因标签 |
| `risk_tags` | 风险标签 |
| `required_confirmations` | AI 入场前必须确认的条件 |
| `entry_zone` | 可接受入场区间 |
| `invalidation` | 失效价和失效原因 |
| `targets` | 目标价列表 |
| `price_context` | 价格、涨跌幅、ATR |
| `derivatives_context` | OI、资金费率、LSR、Taker 数据 |

所有非 `immediate` 入场模式都会通过 router 兜底生成 `required_confirmations`，避免 AI 只看到“等待确认”但不知道等什么。

## 4. Snapshot 热路径耗时

最近一次实时验证：

```text
market_regime = trend_down
BTC 24h = -4.91%
ETH 24h = -5.30%
universe = 182
signals = 24
ScoreHunterV7 elapsed = 182.65ms
```

耗时口径：

| 路径 | 典型耗时 | 说明 |
|---|---:|---|
| 冷启动全量快照构建 | 30s ~ 120s | Binance 明细接口网络耗时，非交易循环 |
| SnapshotStore 热读取 | 微秒级 | `Store.Current()` 原子读取 |
| Hunter v7 纯筛选 | 45ms ~ 600ms | 取决于 universe 和机器状态 |
| CandidateCoin + prompt 构造 | < 1s | 不含 AI 模型请求 |
| AI 二次决策 | 不固定 | 取决于模型供应商 |

因此运行中热路径目标是：

```text
SnapshotStore -> Hunter v7 -> CandidateCoins -> AIT prompt ~= 0.2s ~ 1s
```

如果交易循环中出现超过 10 秒，应优先检查是否没有挂上 SnapshotEngine、是否 Snapshot 为空导致 fallback 到 legacy 路径、或是否在循环内冷拉取 Binance 数据。

## 5. 实时验证命令

使用以下命令拉取实时 Binance Futures 数据并验证 v7 JSON / prompt / 覆盖度：

```bash
go run ./cmd/hunter_v7_validate \
  -top-detail 220 \
  -max-output 30 \
  -min-priority 45 \
  -aggressive=true \
  -out-dir reports
```

输出：

- `hunter-v7-live-validation-raw-*.json`
- `hunter-v7-live-validation-report-*.md`
- `hunter-v7-live-prompt-*.txt`

最近一次通过验证：

```text
JSON marshal/unmarshal: true
missing fields: 0
executable gaps: 0
prompt contains hunter_v7_signal_json: true
issues: 0
```

## 6. 关键实现文件

| 文件 | 作用 |
|---|---|
| `provider/local/hunter_v7.go` | v7 主入口 |
| `provider/local/hunter_v7_types.go` | v7 类型与 JSON 协议 |
| `provider/local/hunter_v7_universe.go` | 多维候选池构建 |
| `provider/local/hunter_v7_regime.go` | 大盘 regime 识别 |
| `provider/local/hunter_v7_router.go` | 模块调度、冲突处理、确认条件兜底 |
| `provider/local/hunter_v7_mod_*.go` | 各 setup 评分模块 |
| `provider/local/hunter_v7_risk.go` | 风险评分 |
| `kernel/engine.go` | v7 信号转 CandidateCoin |
| `kernel/engine_prompt.go` | AIT prompt 中输出 `hunter_v7_signal_json` |
| `trader/auto_trader.go` | `hunter_v7` 接入 SnapshotEngine |
| `cmd/hunter_v7_validate/main.go` | 实时验证工具 |

## 7. 风控原则

Hunter v7 只负责把高质量机会送入 AI 交易引擎，不直接代表必须入场。

AIT AI 必须继续检查：

- 当前持仓和账户风险；
- 是否已满足 `required_confirmations`；
- 入场价是否仍在 `entry_zone`；
- 止损是否严格参考 `invalidation`；
- 当前行情是否与 `market_regime` 冲突；
- `risk_level=HIGH/EXTREME` 时默认不直接开仓。
