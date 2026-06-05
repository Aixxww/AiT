# Hunter v7 Binance 实时链路实测分析报告

> 生成时间：2026-06-05 08:13:36 CST  
> 实测命令：`go run ./cmd/hunter_v7_validate -top-detail 220 -max-output 30 -min-priority 45 -aggressive=true -out-dir reports`  
> 原始 JSON：`reports/hunter-v7-live-validation-raw-20260605-081336.json`  
> 自动报告：`reports/hunter-v7-live-validation-report-20260605-081336.md`  
> Prompt 预览：`reports/hunter-v7-live-prompt-20260605-081336.txt`

## 1. 链路结论

- Binance 实时数据链路已跑通：抓取 531 个合约标的，构建 v7 universe 185 个，最终输出 30 个 Hunter v7 信号。
- v7 到 AIT prompt 的结构化链路已跑通：`hunter_v7_signal_json` 已写入 prompt，JSON marshal/unmarshal 均通过，缺字段数 0，执行性缺口 0。
- 本轮市场 regime 判定为 `trend_down`，BTC 24h=-1.04%，ETH 24h=-2.88%。这不是强趋势多头环境，筛出的 LONG 应按“恐慌反弹/收复确认”处理，不能当成趋势追多。
- 本轮没有 momentum、squeeze、range、accumulation、distribution 类信号；机会集中在 `panic_reversal_long` 和 `funding_reversal`，说明当前 v7 识别的是“下跌趋势中的反转/拥挤交易”。

## 2. 运行中发现并修复的问题

首次联网实测触发 Go 运行时错误：

```text
fatal error: concurrent map read and map write
AiT/datafetch.(*DataFetcher).fetchPerSymbolData.func1()
datafetch/fetcher_per_symbol.go:80
```

根因：`fetchPerSymbolData` 的 worker goroutine 里读取 `all[sym]`，同时主 goroutine 会把 worker 结果写回 `all`，造成并发 map 读写。

已修复：启动 goroutine 前先取 `base := all[sym]`，worker 只持有 snapshot 指针，不再并发读取 `all` map。修复文件：

- `datafetch/fetcher_per_symbol.go`

验证：

- `go test ./datafetch` 通过。
- 修复后重新运行 Binance 实时验证成功。

## 3. 数据与筛选摘要

| 项目 | 结果 |
|---|---:|
| Binance symbols | 531 |
| v7 universe | 185 |
| REST errors | 0 |
| Market regime | `trend_down` |
| BTC 24h | -1.04% |
| ETH 24h | -2.88% |
| 输出信号 | 30 |
| LONG / SHORT | 16 / 14 |
| Prompt bytes | 87,706 |
| JSON 缺字段 | 0 |
| 执行性缺口 | 0 |

信号分布：

| 维度 | 分布 |
|---|---|
| setup_type | `panic_reversal_long=16`, `funding_reversal=14` |
| status | `candidate=26`, `wait_confirm=4` |
| risk_level | `LOW=30` |
| entry_mode | `wait_reclaim=16`, `wait_price_reversal=14` |
| market_regime | `trend_down=30` |

分数统计：

| 指标 | 平均 | 最低 | 最高 |
|---|---:|---:|---:|
| ai_priority | 55.28 | 50.40 | 63.39 |
| setup_score | 64.81 | 46.00 | 93.60 |
| risk_score | 15.60 | 8.00 | 30.00 |
| liquidity_score | 78.50 | 45.00 | 100.00 |
| timing_score | 47.80 | 30.00 | 72.00 |
| regime_fit_score | 74.15 | 67.00 | 80.40 |

## 4. 高优先级标的分析

以下只代表 v7 筛选出的结构化候选，不等于立即开仓。所有 `panic_reversal_long` 都需要 `15m_reclaim_vwap_or_entry_zone`、`taker_buy_15m_gt_0_52`、`no_new_low_after_reclaim`；所有 `funding_reversal` 都需要按方向等待 VWAP/entry zone 反转确认。

| Rank | Symbol | Dir | Setup | Priority | Status | 核心理由 | 执行判断 |
|---:|---|---|---|---:|---|---|---|
| 1 | BILLUSDT | LONG | `panic_reversal_long` | 63.39 | candidate | 24h -15.13%，OI 4h -5.60%，15m taker buy 0.546，强收复 | 结构较完整；但目标/原始失效价 R:R 约 1.27，需等更近止损或更低回踩 |
| 2 | GRASSUSDT | LONG | `panic_reversal_long` | 63.00 | candidate | 24h -24.72%，OI 4h -7.93%，taker buy 0.584 | 反弹强，但原始 invalidation 较远，按 v7 止损 R:R 不足 1.5 |
| 3 | TRADOORUSDT | LONG | `panic_reversal_long` | 60.08 | candidate | 24h -20.09%，taker buy 0.586，强收复 | 近端 VWAP 目标太近，需以 ATR 反转目标评估，R:R 约 1.40，仍偏低 |
| 4 | GUAUSDT | LONG | `panic_reversal_long` | 59.74 | candidate | 24h -23.26%，OI flush，1h +2.74% | 原始失效价极远，执行层风险收益不合格，观察优先 |
| 5 | FORMUSDT | SHORT | `funding_reversal` | 59.50 | wait_confirm | LSR 4.50 极端多头拥挤，1h -2.32%，taker buy 0.412 | 不能立即做空；若 15m 跌破 VWAP 且不创新高，target2 R:R 约 2.10，可列为重点触发 |
| 6 | FETUSDT | LONG | `panic_reversal_long` | 58.34 | candidate | 24h -16.13%，OI 4h -5.31%，taker buy 0.512 | 反转早期，确认较弱；R:R 约 1.19，不适合追 |
| 7 | ZBTUSDT | SHORT | `funding_reversal` | 58.05 | candidate | LSR 3.16，taker buy 0.400，价格转弱 | 方向一致，但给定目标过近，R:R 不足，需更紧执行止损或扩展目标 |
| 8 | NEARUSDT | LONG | `panic_reversal_long` | 57.82 | candidate | 24h -21.81%，OI 4h -5.15%，大币流动性较好 | 原始失效价远，R:R 不足；只适合等待更明确 15m 收复和窄止损 |
| 9 | USELESSUSDT | LONG | `panic_reversal_long` | 57.24 | candidate | 24h -23.46%，OI 下降，强收复 | 名义优先级高，但原始结构止损远，执行层不宜追 |
| 10 | ROBOUSDT | SHORT | `funding_reversal` | 57.05 | candidate | 多头拥挤，taker sell 强，价格转弱 | 第二目标 R:R 约 1.21，未达 1.5，需等反抽到更优入场 |

## 5. 策略解释

### 5.1 为什么本轮 LONG 和 SHORT 接近均衡

市场 regime 是 `trend_down`，v7 权重矩阵会抑制趋势追多，增强恐慌反转、Funding 反转和多头挤压做空。本轮输出 LONG=16、SHORT=14，并不是方向混乱，而是两类反转机会同时存在：

- `panic_reversal_long`：捕捉 24h 大跌后 OI 清洗、taker buy 恢复、价格从低位收复的反弹。
- `funding_reversal`：捕捉 LSR/Funding 拥挤后，价格和 taker flow 开始反向的拥挤交易。

### 5.2 为什么不能直接按 priority 开仓

v7 的 `ai_priority` 是候选排序分，不是最终开仓命令。它的作用是把值得 AI 关注的标的送进 prompt。实盘开仓还必须满足：

- 当前价格仍在 entry zone 或完成回踩确认。
- required_confirmations 全部满足。
- 以实际可执行止损计算 R:R >= 1.5。
- risk tags 没有 `do_not_market_chase`、`risk_filtered`、`liquidity_filtered` 等硬拒绝标签。

本轮前排多个 `panic_reversal_long` 的 `invalidation` 来自 4h capitulation low，距离当前价较远；这对结构识别合理，但对高频实盘止损过宽。因此执行层需要等待回踩，或使用更近的 15m/1h 失效位重新计算仓位。

## 6. 可执行观察清单

优先观察，不建议无确认追单：

| 优先级 | 标的 | 方向 | 条件 |
|---:|---|---|---|
| 1 | FORMUSDT | SHORT | 15m 收盘跌破 VWAP/entry zone，taker buy < 0.45，且不再创新高 |
| 2 | BILLUSDT | LONG | 15m 收复 VWAP/entry zone，taker buy > 0.52，回踩不破新低；需要更近执行止损 |
| 3 | GRASSUSDT | LONG | 同上，重点看 taker buy 是否维持 > 0.52，避免反弹衰竭 |
| 4 | ZBTUSDT | SHORT | 15m 跌破 VWAP 后 OI flush 或 OI 重建失败，否则不追空 |
| 5 | TRADOORUSDT | LONG | 收复有效但 R:R 接近不足，等待回踩 entry zone 下沿 |

## 7. 实盘参数建议

为了避免 v7 实盘出现“无候选币直接跳过”，数据源层建议放宽，执行层再收紧：

```json
{
  "source_type": "hunter_v7",
  "use_hunter": true,
  "hunter_limit": 10,
  "hunter_direction": "BOTH",
  "hunter_config": {
    "v7_max_output": 30,
    "v7_min_ai_priority": 45,
    "v7_aggressive": true
  }
}
```

Prompt/执行层仍要求：

- `ai_priority >= 55` 才考虑开仓。
- `risk_score < 55`，`liquidity_score >= 55`。
- `risk_level` 不能是 HIGH/EXTREME。
- `wait_confirm` 必须等确认，不得立即开仓。
- R:R 不足 1.5 时输出 wait。

## 8. 总结

本轮 Binance 实时验证证明 Hunter v7 数据链路已跑通：实时采集、universe 构建、regime 判断、模块筛选、JSON 序列化、AIT prompt 注入全部正常。筛选结果符合当前 `trend_down` 环境，主要输出恐慌反转做多与 Funding 拥挤反转做空。

需要注意的是，v7 是“候选发现器”，不是“无条件执行器”。本轮最有交易价值的是拥挤反转类 SHORT 的确认触发，以及恐慌反转 LONG 的回踩后窄止损机会；多数前排 LONG 若直接按结构失效价开仓，风险收益比不足，不应追价。
