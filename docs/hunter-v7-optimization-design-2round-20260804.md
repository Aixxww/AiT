# Hunter v7 优化成果两轮复核与前端产品规范落地 - 2026-08-04

> 验证时间：2026-08-04 CST  
> 实时验证目录：`reports/hunter-v7-optimization-design-2round-20260804`  
> 数据源：Binance USDS-M Futures public REST，只读未下单。  
> 目标：复核 `alt_ladder_breakdown_short` 平衡软升档成果，并把 Hunter v7 策略前端 UI 按交易工作台信息层级优化。

## 1. 两轮实时复核结论

| Round | Regime | Universe | Signals | Prompt open-review | Open-rate | REST errors | Degraded |
|---:|---|---:|---:|---:|---:|---:|---|
| 1 | trend_down | 210 | 8 | 0 | 0.0% | 0 | false |
| 2 | trend_down | 214 | 10 | 0 | 0.0% | 0 | false |
| 合计 | trend_down | - | 18 | 0 | 0.0% | 0 | 2/2 valid |

Outcome：

- tracked = 0
- active = 0
- 无 `EXECUTABLE/REVIEWABLE` 结案样本

报告文件：

- `reports/hunter-v7-optimization-design-2round-20260804/hunter-v7-validation-run-summary-20260804-061858.md`
- `reports/hunter-v7-optimization-design-2round-20260804/hunter-v7-validation-outcomes-20260804-061858.md`

## 2. alt_ladder_breakdown_short 复核

两轮主输出均以 `alt_ladder_breakdown_short` 为第一梯队，但全部保持 WATCH。

| Round | Symbol | OI 1h | Taker buy 15m | Zone pos | Stop dist | Risk tags | 拦截理由 |
|---:|---|---:|---:|---:|---:|---|---|
| 1 | 1000RATSUSDT | -4.561% | 0.437 | 31.8% | 2.50% | execution_stop_tightened | OI 下降、止损过宽、未下破 trigger |
| 1 | AKEUSDT | +0.835% | 0.417 | 31.8% | 2.00% | execution_stop_tightened | 有新空 OI，但 taker 不够低且未下破 trigger |
| 1 | AIOUSDT | +0.995% | 0.560 | 31.8% | 2.20% | - | 买盘占优，taker 明显反向 |
| 2 | 1000RATSUSDT | -4.561% | 0.350 | 31.8% | 2.50% | execution_stop_tightened | 低 taker 但 OI 下降、止损过宽 |
| 2 | TAKEUSDT | -5.628% | 0.532 | 31.8% | 2.20% | - | OI 下降且买盘偏高 |
| 2 | BICOUSDT | -2.632% | 0.557 | 31.8% | 2.01% | funding_elevated, execution_stop_tightened | 买盘反向、funding+止损收紧组合 |

判断：

- 这组行情比上一组更偏下跌，但依然没有出现“低区位 + 新空 OI + taker<=0.38 + 可控 stop + close-through”的合格短空。
- 1000RATSUSDT 第二轮虽然 taker=0.350，但 OI=-4.561%、stop=2.50%，属于出清式下跌，不应释放主开仓。
- AKEUSDT/AIOUSDT 虽有 OI 正增，但 taker 仍不满足软升档门槛，保持 WATCH 正确。

## 3. 优化成果评估

- 正向成果：没有把 `long_flush + OI下降` 的短空误放为 open-review，说明平衡软升档没有被单轮行情带偏。
- 副作用：本轮 open-rate 为 0.0%，在 `trend_down` 环境下仍偏保守，说明开仓率尚未达到高优水平。
- 胜率：本轮无 tracked outcome，不能计算真实胜率；只能证明“没有新增误开仓样本”，不能证明盈利能力提升。
- 长期 outcome：最新 DB 写入窗口中 `execution_tier WATCH/REJECTED/空 tier` 的 `track_status` 均为空，非开仓行不再残留 `ACTIVE`，防污染修正生效。

## 4. 前端产品规范已落地

变更文件：

- `web/src/components/hunter/SignalPanel.tsx`
- `web/src/components/hunter/SignalPanel.test.tsx`
- `web/src/i18n/translations.ts`

已实施：

- 新增 `buildSignalSummary`，只基于最新 cycle 计算 open-rate、open-review、WATCH、REJECTED、outcome、平均 PnL。
- SignalPanel 顶部新增 KPI strip：`OPEN RATE / OPEN REVIEW / WATCH / REJECTED / PROT/WIN / STOP / AVG PNL`。
- Score 区从“卡中卡”改为密集指标带，降低视觉噪声。
- WATCH 行改为稳定列宽的工作台列表，避免 setup、tier reason、zone 信息挤压跳动。
- outcome badge 与 summary 同源显示，保护止损、胜、止损的颜色与 tier 颜色解耦。
- 三语文案补齐：英文、中文、印尼语。

产品口径：

- Tier 只表达信号质量，不使用盈亏红绿。
- Direction 单独用 LONG/SHORT 语义色。
- Outcome 才使用盈利/亏损语义色。
- 开仓率与胜率分开显示，避免用户把“有信号”误读成“可开仓”。

## 5. 后续可实施优化

1. 不再继续放宽 `alt_ladder_breakdown_short` 主开仓门槛；当前数据不支持。
2. 建立 `alt_ladder_flush_watch_pool`：单独累计 OI 下降的 flush 型短空，不进入真实 open-review。
3. 增加 validation outcome 明细输出：symbol、setup、tier、entry、TP0、SL、status、PnL、MFE、MAE，减少人工查 DB。
4. 若后续连续 6-9 轮 `trend_down` 仍 open-rate=0，可考虑新增一个极小仓位 `REVIEWABLE-micro` 分支，但必须同时满足：
   - 5m/15m close below trigger
   - taker_buy_15m <= 0.36
   - stop distance <= 2.20%
   - 无 `execution_stop_tightened`
   - OI 下降分支只允许进入 audit，不进入主开仓统计

## 6. 验证

已通过：

```bash
go test ./kernel ./trader
npm test -- SignalPanel.test.tsx
npm run build
npx prettier --check src/components/hunter/SignalPanel.tsx src/components/hunter/SignalPanel.test.tsx src/i18n/translations.ts
```

当前判断：优化成果主要体现在“避免误开仓”和“长期 outcome 分母更干净”；开仓率尚未恢复到高优，需要继续用多轮数据分桶，而不是在本轮 `trend_down` 里强行放宽 short 分支。
