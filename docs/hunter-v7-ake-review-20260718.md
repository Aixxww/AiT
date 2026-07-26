# Hunter v7 AKEUSDT 极端高振幅复盘与优化记录

日期：2026-07-18  
标的：`AKEUSDT` Binance USD-M Futures

## 1. 实时数据结论

Binance public futures 数据显示：

- 当前价格约 `0.00192 - 0.00194`。
- 24h 高低约 `0.0019778 / 0.0009780`，24h 涨幅约 `+85% - +98%`。
- 24h quote volume 约 `10.2 亿 USDT`，成交笔数约 `1200 万`。
- 7月15日观测低点约 `0.0001853`，到当前累计约 `+937%`。
- 1h OI 约 `+6.8%`，4h OI 约 `+25%`，说明仍有资金参与。
- 当前资金费率约 `0.11% / 8h`，属于拥挤。
- 当前 15m taker buy 在 `0.45 - 0.51` 附近波动，未稳定站上 `0.52`。
- 当前价格明显高于 15m EMA25/VWAP 回踩区，属于极端延伸状态。

## 2. 原 Hunter v7 为什么没有捕捉

在改造前，AKE 当前阶段不会进入 LLM prompt：

- `leader_momentum_long`：24h 涨幅上限为 60%，AKE 当前超过上限。
- `alt_ladder_momentum_long`：24h 涨幅上限为 45%，AKE 当前超过上限。
- `displacement_momentum_long`：资金费率超过 `0.001`，且当前不是低风险位移初段。
- `mms_trend_ride_long`：需要 EMA25 缩量回踩并收回；AKE 当前远离 EMA25。
- `mms_squeeze_engine_long`：Top trader LSR 未达到 `1.55`。
- 空头路由：当前 1h/4h 仍偏强，未形成破 VWAP/EMA 的下跌结构。

结论：不是数据源拿不到 AKE，而是策略把它视为“极端末段”，没有为极端强庄趋势提供 watch/回踩触发层，导致 LLM 无法看到后续入场计划。

## 3. 优化实施

已在 `alt_ladder_momentum_long` 中新增极端阶段：

- 新增 reason：`alt_ladder_stage_extreme`
- 新增 risk tag：`alt_ladder_extreme_continuation_watch`
- 24h 涨幅上限从 45% 扩展到 120%，但极端阶段必须满足：
  - 15m EMA7 > EMA25 > EMA99
  - 4h 涨幅 >= 10%
  - 4h OI 增幅 >= 15%
  - 至少一项 flow 继续确认
- 极端阶段 entry zone 不再围绕现价，而是锚定 EMA25/VWAP 回踩区，避免现价追多。
- Kernel tier 对 `alt_ladder_extreme_continuation_watch` 特例映射为 WATCH，而不是 REJECTED。
- Prompt 明确：`alt_ladder_stage_extreme` 只跟踪 EMA25/VWAP 回踩或重新放量，不得现价追多。

## 4. 验证结果

测试：

```bash
go test ./provider/local ./kernel ./trader
# ok
```

实时验证：

```bash
go run ./cmd/hunter_v7_validate -rounds=1 -top-detail=340 -max-workers=6 -max-output=160 -watch-output=60 -min-priority=20 -aggressive=true -out-dir=reports/ake-v7-validate-20260718-watch
```

验证结果：

- `symbols=524`
- `universe=255`
- `signals=91`
- `EXECUTABLE=12`
- `REVIEWABLE=4`
- `WATCH=41`
- `REJECTED=34`
- `issues=0`

AKEUSDT 已进入 prompt WATCH 摘要：

```text
AKEUSDT LONG setup=alt_ladder_momentum_long status=candidate quality=near_confirm ai_priority=63.1 risk=80 reason=alt_ladder_extreme_continuation_watch
```

AKE 当前结构：

- setup：`alt_ladder_momentum_long`
- reason：`alt_ladder_stage_extreme`, `alt_ladder_oi_inflow`
- risk：`alt_ladder_extreme_continuation_watch`, `extreme_volatility`, `extended_24h_gain`, `do_not_market_chase`, `funding_extreme`
- entry zone：约 `0.0017376 - 0.0017737`
- 当前价：约 `0.001943`
- 缺失确认：
  - 当前价未回到 entry zone
  - 15m taker buy 未稳定大于 `0.52`

## 5. 交易解释

AKE 从 7月15日低点至今确实存在多个历史入场阶段：

- 初段：`+5% - +30%` 应由 `alt_ladder_momentum_long` 捕捉。
- 中段：`+30% - +120%` 应由 `alt_ladder_stage_mid/late` 捕捉，但需要 flow 同向。
- 极端延伸：`+400%` 以后不应现价追多，应进入 `alt_ladder_stage_extreme`，等待 EMA25/VWAP 回踩或重新放量。
- 转空阶段：只有跌破 VWAP/EMA/15m 中轨并伴随 taker sell/OI/volume，才由 `alt_ladder_breakdown_short` 或 `breakdown_momentum_short` 捕捉。

当前 AKE 的正确策略不是 open long，而是 watch：

- 回踩到 `0.00174 - 0.00177` 附近；
- 15m taker buy 重新站上 `0.52`；
- OI 不反向流出；
- RR 仍 >= 1.5；
- 才允许进入 REVIEWABLE/EXECUTABLE 复核。
