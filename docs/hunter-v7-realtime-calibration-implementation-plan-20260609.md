# Hunter v7 实时链路校准实施方案

> 日期：2026-06-09
> 范围：Binance Futures 实时数据采样、Hunter v7 universe/detail 入口、OI 单位、displacement/leader 分层、实时验证工具
> 原则：先修链路事实与单位一致性，再调召回和分层；避免继续用高频 REST 轮测触发 Binance 418。

## 1. 实测结论摘要

本轮实时验证显示：

- `go build ./...` 与 `go test ./...` 均通过。
- Binance REST 多轮连续采样触发 HTTP 418，说明当前轮测方式请求密度过高。
- 实时 Hunter v7 三轮有效样本均为 `rotation` regime，输出 11-13 个信号。
- setup 过度集中：`funding_reversal` 占主导，仅少量 `leader_momentum_long` / `panic_reversal_long`。
- REVIEWABLE 数量偏少：三轮分别为 0、1、1。
- mover audit 发现 24h 振幅 20%+ 标的 46 个，但当前 DB 没有历史 signal records，召回率无法用历史库证明。
- 代码层面存在两个关键前置瓶颈：
  - `BuildV7Universe` 要求 `OI > 0`，而 OI 只对 detail symbols 拉取，导致不少高振幅标的在 universe 前被过滤。
  - v7 的 `OIValue` 和 liquidity score 直接使用 Binance `openInterest` 数量，未统一为 USDT notional。

## 2. 改造目标

### P0：实时链路安全与事实正确

1. 轮测工具支持多轮采样时自动拉开间隔，避免短时间重复 REST 全量拉取。
2. 验证工具降低默认并发，并显式暴露 `--max-workers`、`--rounds`、`--round-interval`。
3. v7 内部 OI 全部统一为 notional 口径：`openInterest quantity × current price`。
4. universe 构建不再因为 `OI == 0` 直接排除 high amplitude 标的。

### P1：大波动召回

1. detail symbol 选择新增 amplitude quota，保证 24h high-low 大振幅标的优先拿到 OI/K 线明细。
2. displacement 模块允许缺少 OI delta 时先进入 watch/reviewable 预备状态，不把 OI delta 当唯一硬门。
3. leader momentum 的 late pullback 不再一刀切 WATCH，但只允许 `strong_4h_momentum` / `taker_sustained_buy` / `taker_buy_aggressive` 这类强确认例外进入 REVIEWABLE，普通上沿弱回踩继续 WATCH。

### P2：验证可解释性

1. validation report 增加 tier 分布，直接展示 EXECUTABLE / REVIEWABLE / WATCH / REJECTED。
2. mover audit 与 validation 的轮测方式明确推荐低频运行，不再用 shell tight loop 压测 Binance REST。

## 3. 具体实施项

| 优先级 | 模块 | 文件 | 改动 |
|---|---|---|---|
| P0 | 实时验证工具 | `cmd/hunter_v7_validate/main.go` | 新增 `--rounds`、`--round-interval`、`--max-workers`；多轮自动 sleep；默认 workers 从 50 降至 8 |
| P0 | 数据采样 | `datafetch/detail_selector.go` | detail symbols 增加 24h amplitude quota |
| P0 | Universe | `provider/local/hunter_v7_universe.go` | 允许 high amplitude 标的在 OI 缺失时进入基础 universe |
| P0 | OI 单位 | `provider/local/hunter_v7_universe.go` / `hunter_v7_risk.go` / `hunter_v7_mod_pullback.go` | 统一 v7 OI notional |
| P1 | Displacement | `provider/local/hunter_v7_mod_displacement.go` | OI delta 缺失时不硬拒；增加缺 OI 的 wait/review 语义 |
| P1 | Tier | `kernel/engine.go` | leader late pullback 增加强确认 REVIEWABLE 例外；displacement 缺 OI 维持低风险复核 |
| P2 | 验证报告 | `cmd/hunter_v7_validate/main.go` | 增加 tier distribution 与 markdown 输出 |

## 4. 风险控制

- 不改真实下单路径的 order placement 逻辑。
- 不降低后端硬风控：价格、SL/TP、RR、漂移校验保持有效。
- 不把 WATCH 直接开放成 open；只允许满足明确复核条件的信号进入 REVIEWABLE。
- REST 轮测默认低并发，建议多轮间隔不少于 90 秒；连续 3 轮以上建议使用已有 Collector/WS 复用快照。

## 5. 验收标准

### 静态验收

```bash
go build ./...
go test ./...
```

### 实时低频验收

建议 Binance 418 冷却结束后执行：

```bash
go run ./cmd/hunter_v7_validate \
  --rounds 3 \
  --round-interval 120s \
  --top-detail 220 \
  --max-workers 8 \
  --max-output 40 \
  --watch-output 8 \
  --min-priority 45 \
  --aggressive \
  --out-dir reports
```

### 指标验收

- 不触发 Binance HTTP 418。
- `rest_errors` 明显下降。
- universe 中 high amplitude 标的覆盖提升。
- `displacement_momentum_long` 或 high amplitude 相关 watch/reviewable 有样本出现。
- REVIEWABLE 每轮目标：至少 1-3 个，且不是全部来自 `funding_reversal`。
- v7 prompt 中 `oi_value` 使用 USDT notional 口径。

## 6. 本轮执行结果

### 已落地改动

- `cmd/hunter_v7_validate/main.go`
  - 默认 `MaxWorkers` 从 50 下调到 8。
  - 新增 `--max-workers`、`--rounds`、`--round-interval`，多轮验证会自动 sleep。
  - validation raw/markdown/console 增加 execution tier 分布。
- `datafetch/detail_selector.go`
  - detail symbol 选择从纯成交额/涨跌幅/funding 扩展为成交额 40%、24h amplitude 15%、gainer 18%、loser 15%、funding 10%。
- `provider/local/hunter_v7_universe.go`
  - `amplitude >= 12%` 的标的即使缺 OI detail 也进入 universe，用于大波动召回和归因。
  - v7 `Snapshot.OI` 统一转换为 USDT notional：`openInterest quantity * price/markPrice`。
  - 新增 24h amplitude 与 1h range expansion 上下文。
- `provider/local/hunter_v7_mod_displacement.go`
  - 新增 `displacement_momentum_long` 模块。
  - OI delta 缺失但 amplitude/taker 满足时不硬拒，标记为 `near_confirm` + `needs_oi_confirmation`。
- `kernel/engine.go`
  - validation/runtime candidate 填充 `V7QuoteVolume24h` 和 runtime tier。
  - displacement 增加 REVIEWABLE/EXECUTABLE 复核路径。
  - leader momentum 上沿回踩例外已收窄，避免复活 SAHARA 类高位弱回踩风险。

### 验证结果

```bash
go build ./...
go test ./...
go test ./datafetch ./provider/local ./kernel ./cmd/hunter_v7_validate
```

结果：全部通过。

### 实时样本复核

本轮使用 Binance 实时数据得到 3 个有效样本：

| 时间 | symbols | universe | regime | signals | REVIEWABLE | rest_errors |
|---|---:|---:|---|---:|---:|---:|
| 07:18:56 | 525 | 207 | rotation | 11 | 0 | 0 |
| 07:21:06 | 525 | 209 | rotation | 13 | 1 | 0 |
| 07:21:47 | 525 | 193 | rotation | 11 | 1 | 63 |

随后 Binance 返回 HTTP 418 临时封禁提示，说明不能用 tight loop 继续轮测。后续实时复测必须使用本方案的低并发、长间隔命令，或改为复用 Collector/WebSocket 快照。

## 7. 多视角复核结论

### API / 运维视角

- 已把验证工具默认并发降到 8，并提供多轮间隔参数。
- 当前不建议立刻再次请求 Binance REST；最近一次样本已出现 HTTP 418 风险信号。
- 后续 3 轮以上验证建议 `--round-interval >= 120s`，并观察 `rest_errors` 是否归零。

### 数据口径视角

- datafetch 层 `OI` 明确为 Binance `openInterest` 数量。
- Hunter v7 层 `SymbolSnapshotData.OI` 明确为 USDT notional。
- 已补测试覆盖：高 amplitude 无 OI detail 不被 universe 丢弃，且 OI quantity 会转换成 notional。

### 策略召回视角

- detail selector 增加 amplitude quota，解决大波动标的明细拉取不足。
- universe 增加 amplitude/range expansion 通道，解决“大波动标的在 OI detail 前被过滤”的入口问题。
- displacement 模块补上“1h true range 扩张 + taker 对齐 + OI 待确认”的形态缺口。

### 风控 / 执行视角

- WATCH 没有被直接放开为 open。
- 缺 OI 的 displacement 只进入 `near_confirm`/REVIEWABLE 复核，不绕过 RR、chase risk、funding/RSI 过热过滤。
- leader momentum 上沿回踩保留保护：普通 `solid_4h_momentum + taker_strong_buy` 仍为 `WATCH momentum_late_pullback_zone_upper_wait`。

### 测试覆盖视角

- 新增/更新了 detail selector、universe OI notional、high amplitude universe、leader momentum 高位回踩保护相关测试。
- 全量测试通过，但实时回归需等待 Binance 418 冷却后用低频命令复测。
