# Hunter v7 币安实时 3 轮全链路跟踪报告 - 2026-08-02

> 测试时间：2026-08-02 06:35-06:53 CST
> 数据源：Binance USDT-M futures 实时 REST，代理 `127.0.0.1:7897`
> 验证目录：`reports/hunter-v7-binance-live-3round-20260802`
> 命令：`go run ./cmd/hunter_v7_validate -rounds=3 -round-interval=5m -top-detail=220 -max-workers=8 -max-output=30 -watch-output=8 -min-priority=45 -aggressive=true -post-track-duration=5m -post-track-interval=60s -out-dir=reports/hunter-v7-binance-live-3round-20260802`

## 1. 结论

本轮严格按最终 DB `EXECUTABLE/REVIEWABLE` 口径跟踪，真实 open-review 为 4 条，全部来自 `alt_ladder_breakdown_short`。三轮输出信号 21 条，整体开仓率 19.0%；剔除第 1 轮 REST 错误样本后，第 2/3 轮开仓率 20.0%。

5 分钟 post-track 后 4 条仍全部 ACTIVE，未触发 TP0/TP1/TP2/STOP/TIMEOUT，因此严格结案胜率暂不可判定。浮动口径为 3 盈 1 亏，active 正收益率 75.0%，平均 PnL +0.136%，平均 MFE +0.602%，平均 MAE -0.351%。

本轮验证了昨天 denominator 修正有效：raw readiness 仍可能给到 WATCH/EXECUTABLE，但长期 outcome tracker 只跟踪最终 DB REVIEWABLE/EXECUTABLE，`funding_reversal`、pre-watch、whale watch 没有进入主胜率分母。

## 2. 三轮数据质量

| Round | 时间 | Regime | Symbols | Universe | REST errors | 输出信号 | Final open-review | 开仓率 |
|---:|---|---|---:|---:|---:|---:|---:|---:|
| 1 | 06:36:34 | rotation | 524 | 85 | 220 | 1 | 0 | 0.0% |
| 2 | 06:42:28 | trend_down | 524 | 216 | 0 | 10 | 3 | 30.0% |
| 3 | 06:48:20 | trend_down | 524 | 213 | 0 | 10 | 1 | 10.0% |
| 合计 | - | - | - | - | - | 21 | 4 | 19.0% |

第 1 轮 REST errors=220，导致 universe 只有 85，且 prompt JSON 缺失问题数为 4；第 2/3 轮 REST errors=0，报告判断应以第 2/3 轮为主要有效样本。

## 3. 各形态开仓率与 outcome

| Setup | Final rows | Open-review | WATCH | REJECTED | 开仓率 | Outcome |
|---|---:|---:|---:|---:|---:|---|
| alt_ladder_breakdown_short | 6 | 4 | 2 | 0 | 66.7% | ACTIVE 4，3 盈 1 亏 |
| funding_reversal | 6 | 0 | 5 | 1 | 0.0% | 未进入主 outcome |
| pre_breakout_watch | 3 | 0 | 3 | 0 | 0.0% | watch-only |
| pre_distribution_watch | 2 | 0 | 2 | 0 | 0.0% | watch-only |
| whale_flow_reversal | 2 | 0 | 2 | 0 | 0.0% | 最终 tier guard 拦截 |
| panic_reversal_long | 1 | 0 | 0 | 1 | 0.0% | invalid RR |
| volatility_squeeze_breakout | 1 | 0 | 0 | 1 | 0.0% | invalid RR / 数据质量差轮次 |

严格胜率：0/0，暂无 terminal 样本。
active 正收益率：3/4 = 75.0%。
主 tracked setup：`alt_ladder_breakdown_short`，平均 PnL +0.136%，MFE +0.602%，MAE -0.351%。

## 4. 跟踪标的盈亏与止盈止损

| Symbol | Round | Dir | Tier | Entry zone | Stop | TP0 | TP1 | TP2 | Current | Status | PnL% | MFE% | MAE% |
|---|---:|---|---|---|---:|---:|---:|---:|---:|---|---:|---:|---:|
| PTBUSDT | 2 | SHORT | REVIEWABLE | 0.00073609-0.00074555 | 0.00075822 | 0.00072690 | 0.00066582 | 0.00060582 | 0.00073860 | ACTIVE | +0.300 | +0.408 | -0.524 |
| TAGUSDT | 2 | SHORT | REVIEWABLE | 0.00145715-0.00146925 | 0.00149314 | 0.00143925 | 0.00138180 | 0.00131668 | 0.00147400 | ACTIVE | -0.738 | +0.355 | -0.875 |
| TAKEUSDT | 2 | SHORT | REVIEWABLE | 0.02696946-0.02734829 | 0.02763180 | 0.02660173 | 0.02438733 | 0.02217009 | 0.02709000 | ACTIVE | +0.254 | +0.880 | -0.004 |
| AIOUSDT | 3 | SHORT | REVIEWABLE | 0.05071702-0.05110353 | 0.05195848 | 0.05012945 | 0.04840080 | 0.04444717 | 0.05054000 | ACTIVE | +0.727 | +0.767 | +0.000 |

观察：

- `AIOUSDT` 质量最好：入场后没有负 MAE，当前浮盈 +0.727%，接近 TP0 前仍未触发。
- `TAKEUSDT` 风险最好：MAE 仅 -0.004%，但 MFE +0.880 后未 TP0，说明 TP0 距离可能偏远或 entry/TP0 设置偏保守。
- `TAGUSDT` 是主要风险样本：当前价已高于 entry zone upper，浮亏 -0.738%，MAE -0.875%，但未到止损；需要反抽失败确认或更早保护退出。
- 4 条均未结案，当前窗口无法给出真实胜率，只能作为开仓率与短窗 MFE/MAE 评估。

## 5. 已验证有效的机制

1. outcome 分母修正有效。
   - DB 中最终 open-review=4，outcome tracked=4。
   - `funding_reversal` 虽然 2 轮共输出 6 条，但最终为 WATCH/REJECTED，未进入主 outcome。
   - 第 3 轮 `ONUSDT whale_flow_reversal` raw readiness 曾给到 EXECUTABLE，但最终 DB tier 为 WATCH，没有污染真实分母。

2. `funding_reversal` 拆桶有效。
   - ICP/FORM/TUT/SCRT/ROBO 等 funding short 继续因 retest、weak 4h OI flush、liquidity 等原因留在 WATCH/REJECTED。
   - 该形态当前适合作为 reversal watch pool，而不是主开仓池。

3. `alt_ladder_breakdown_short` 仍是当前 trend_down 下唯一能贡献开仓率的主形态。
   - 第 2/3 轮有效样本中，alt-ladder short 输出 6 条、放行 4 条。
   - 短窗 active 正收益率 75%，但未结案，仍需更长窗口验证 TP/SL 胜率。

## 6. 暴露问题

### P0：首轮 REST 错误应纳入报告剔除/重试机制

第 1 轮 REST errors=220，universe 从正常的 213-216 降到 85，导致输出只有 1 条且 invalid RR。当前报告仍把它计入总轮次，会拉低机会覆盖和开仓率解释质量。

可实施：

- validator 增加 `min_detail_success_rate`，例如 REST error rate > 20% 时标记 round invalid，不纳入主统计。
- 首轮 warm-up 后重试一次 ticker/OI/kline 明细；重试仍失败才落入 degraded round。
- 输出报告拆分 `all_rounds` 与 `valid_rounds` 两套开仓率。

### P1：alt_ladder_breakdown_short 直接 REVIEWABLE 仍缺反抽失败门

昨天已给跨轮 trigger memory 升级增加 `no_new_high_after_rejection`，但本轮直接 REVIEWABLE 的 DB tier reason 仍是：

- `live_reviewable_5m_or_15m_close_below_trigger`
- blocked_gate=`confirmation_missing`

这说明直接 tier guard 仍允许“只缺 close below trigger”的 alt-ladder short 进入 REVIEWABLE，没有强制检查反抽失败。`TAGUSDT` 浮亏样本正暴露这个问题。

可实施：

- 对 `alt_ladder_breakdown_short` 直接 tier 增加专属 gate：
  - 若 `no_new_high_after_rejection` 缺失或未通过：降为 WATCH。
  - blocked_gate 写 `alt_ladder_short_rebound_pending`。
  - 只有 `no_new_high_after_rejection` 已通过，且仅缺 `5m_or_15m_close_below_trigger` 时，才允许 REVIEWABLE。
- Prompt 中同步声明：alt-ladder short 的 REVIEWABLE 不是追空许可，必须等待反抽不创新高后再用 close-below trigger 复核。

### P1：TP0 与短窗 MFE 不匹配

`TAKEUSDT` MFE +0.880%、`AIOUSDT` MFE +0.767%，但均未 TP0。若 5-10 分钟内 MFE 较好却无法触发 TP0，说明 TP0 可能对 alt-ladder short 偏远，胜率统计会被大量 ACTIVE/回吐拖慢。

可实施：

- 为 `alt_ladder_breakdown_short` 增加 `micro_tp0`：
  - TP0 = min(当前 TP0, entry - max(0.55% price, 0.45 * stop distance))。
  - 仅用于 outcome/执行保护，不改变 TP1/TP2。
- 当 MFE >= 0.60% 且仍未 TP0，自动把 stop 拉到 breakeven 或 entry zone upper 下方，避免 `TAGUSDT` 式反抽回撤扩大。

### P2：开仓形态过度单一

第 2/3 轮有效样本中，所有真实 open-review 都来自 `alt_ladder_breakdown_short`。这对 trend_down 有利，但会导致策略收益高度依赖单一形态。

可实施：

- `range_expansion_event` 继续观察 raw pool：当前仍有潜力池命中但未输出为主开仓，说明反抽失败确认可能偏严或样本不足。
- `whale_flow_reversal` 逆势长单保持 WATCH 是正确的，但可增加 missed-opportunity 审计：若 30m MFE > 1.2% 且无 MAE > 0.5%，再考虑在 trend_down 下开放 REVIEWABLE 小仓。
- `relative_weakness_short` 本轮潜力池出现但未进入主输出，可加入 trend_down 下的补充 short family，避免 alt-ladder 单点拥挤。

## 7. 下一步实施建议

优先级按“提高胜率且不盲目提高开仓率”排序：

1. P0 数据质量：validator 增加 degraded round 剔除与重试，报告输出 valid-round 开仓率。
2. P1 alt-ladder direct tier guard：`no_new_high_after_rejection` 必须通过，否则 WATCH，并写 `alt_ladder_short_rebound_pending`。
3. P1 alt-ladder micro TP0 / breakeven：MFE >= 0.60% 后保护止损，减少短窗浮盈回吐。
4. P2 扩展 short family：把 `relative_weakness_short` 在 trend_down 下作为补充开仓池小样本验证。
5. P2 对 watch pool 做 missed-opportunity：funding、whale、range 不进入主分母，但保留 30m/60m MFE/MAE，用数据决定是否放宽。

## 8. 复测口径

下一轮建议使用同样 3 轮 5 分钟，但 post-track 拉长到 30 分钟：

```bash
HTTP_PROXY=http://127.0.0.1:7897 HTTPS_PROXY=http://127.0.0.1:7897 ALL_PROXY=http://127.0.0.1:7897 NO_PROXY=localhost,127.0.0.1,::1 \
go run ./cmd/hunter_v7_validate \
  -rounds=3 \
  -round-interval=5m \
  -post-track-duration=30m \
  -post-track-interval=60s \
  -top-detail=220 \
  -max-workers=8 \
  -max-output=30 \
  -watch-output=8 \
  -min-priority=45 \
  -aggressive=true \
  -out-dir=reports/hunter-v7-binance-live-3round-post30m-20260802
```

评价目标：

- valid-round REST error rate = 0。
- strict open-review rate 保持 15%-30%。
- terminal TP0/STOP 样本开始出现后，TP0 win rate >= 55%。
- active MAE 均值控制在 -0.45% 以内。
- 单一 setup 占 open-review 不超过 75%，避免形态集中风险。
