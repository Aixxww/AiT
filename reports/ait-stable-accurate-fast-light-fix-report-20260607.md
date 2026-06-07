# AIT 稳准快轻专项修复报告

生成时间：2026-06-07 22:48 CST

## 1. 修复目标

本轮任务基于前序架构审检结论，围绕 AIT 实盘主链路的“稳、准、快、轻”进一步修复：

- 稳：减少 Binance 子接口偶发失败对整币细节数据的破坏。
- 准：修复压缩类指标语义，降低 breakout / accumulation / pre-move radar 误判。
- 快：保持 Hunter v7 本地毫秒级筛选，不引入额外 REST 请求。
- 轻：观察层继续复用已有快照，不增加 API 连接压力。

## 2. Agent 修复任务拆分

| 任务 | 责任模块 | 目标 | 状态 |
|---|---|---|---|
| A1 | `provider/local` | 修复 `BBWidthPercentile` 语义，使低百分位真正代表低波动压缩 | 已完成 |
| A2 | `datafetch` | 子接口部分失败时保留已获取的 OI/LSR/Kline 细节数据 | 已完成 |
| A3 | `datafetch` | 金融类/商品/RWA 合约从静态黑名单升级为 metadata + 黑名单双层过滤 | 已完成 |
| A4 | `trader` | 优化 anti-repeat，避免把升级中的 Hunter v7 机会错误过滤 | 已完成 |
| A5 | `web` / `store` | 暴露 `v7_watch_output` 观察输出配置 | 已完成 |
| A6 | `tests` / `reports` | 增加回归测试、实时验证并输出报告 | 已完成 |

## 3. 关键修复说明

### 3.1 BBWidthPercentile 语义修复

文件：`provider/local/hunter_v7_universe.go`

原逻辑统计 `w >= currentWidth`，导致当前 BB 宽度越低反而越容易得到高百分位，与注释和模块使用方式相反。已改为升序 percentile rank：统计 `w <= currentWidth`，使低值真正代表更强压缩。

影响模块：

- `trend_breakout_long`
- `accumulation_breakout_long`
- `pre_breakout_watch`
- `accumulation_watch`

新增测试：

- `provider/local/hunter_v7_universe_test.go`

### 3.2 细节数据容错修复

文件：`datafetch/fetcher_per_symbol.go`

原逻辑中任意子请求失败都会让调用方丢弃该 symbol 的所有细节 enrich 结果。已改为：

- OI/LSR/Kline 部分成功时仍返回已获取数据。
- 错误计数继续保留，便于观测 REST 健康度。
- 避免整币退化为 bulk-only 快照。

对实盘影响：

- Binance 单个 Kline/OI/LSR 请求偶发失败时，Hunter v7 仍能使用剩余字段判断。
- 提高筛选稳定性和机会覆盖率。

### 3.3 金融类合约过滤增强

文件：

- `datafetch/types.go`
- `datafetch/fetcher_bulk.go`
- `datafetch/collector_test.go`

增强点：

- 保留 `XAU/XAUT/XAG/CL/NVDA/TSLA/PAXG` 等静态黑名单。
- 新增 `underlyingType != COIN` 过滤。
- 新增 `underlyingSubType` 中 `COMMODITY/METAL/STOCK/EQUITY/ETF/INDEX/RWA` 过滤。
- 新增 `baseAsset` 兜底过滤。

### 3.4 Anti-repeat 策略修复

文件：

- `trader/auto_trader_loop.go`
- `trader/auto_trader_loop_test.go`

原逻辑按 symbol 连续 wait 过滤，可能在慢热机会即将确认时将其移除。已改为：

- legacy 普通候选仍保留旧行为。
- Hunter v7 如果升级为 `candidate`，不再过滤。
- Hunter v7 如果 `execution_quality=ready/near_confirm`，不再过滤。
- Hunter v7 如果 `ai_priority >= 50`，不再过滤。
- 低优先级 `watch_only` 重复 wait 仍可过滤，避免 prompt 噪声。

### 3.5 UI 配置补齐

文件：

- `web/src/types/strategy.ts`
- `web/src/components/strategy/CoinSourceEditor.tsx`
- `web/src/i18n/strategy-translations.ts`

新增：

- `v7_watch_output`
- 默认值 5
- 可选值：3 / 5 / 8 / 10

## 4. 验证结果

### 4.1 Go 全量测试

命令：

```bash
go test ./...
```

结果：通过。

### 4.2 前端构建

命令：

```bash
npm run build
```

目录：`web`

结果：通过。

备注：Vite 提示部分 chunk 超过 500KB，这是构建体积优化提示，不影响本轮功能正确性。

### 4.3 Hunter v7 实时验证

命令：

```bash
go run ./cmd/hunter_v7_validate -top-detail 220 -max-output 30 -watch-output 5 -min-priority 45 -aggressive=true -out-dir reports
```

结果：

- snapshot symbols：525
- universe：209
- regime：rotation
- Hunter v7 路由耗时：7.839906ms
- 输出 signals：4
- setup 分布：`trend_breakout_long=2`, `leader_momentum_long=1`, `pre_distribution_watch=1`
- JSON 校验：通过
- Prompt v7 JSON：存在
- issues：0

生成文件：

- `reports/hunter-v7-live-validation-report-20260607-224823.md`
- `reports/hunter-v7-live-validation-raw-20260607-224823.json`
- `reports/hunter-v7-live-prompt-20260607-224823.txt`

## 5. 当前稳准快轻评分

| 维度 | 修复前 | 修复后 | 说明 |
|---|---:|---:|---|
| 稳 | 7.0 | 8.0 | 细节抓取部分失败不再整币降级，anti-repeat 更稳 |
| 准 | 6.5 | 8.0 | BBWidth 压缩语义已修复，突破/蓄势判断更可信 |
| 快 | 8.0 热运行 / 5.5 冷启动 | 8.0 热运行 / 5.5 冷启动 | 本轮未改冷启动网络耗时，路由仍为毫秒级 |
| 轻 | 8.0 | 8.2 | 未增加 REST 请求，过滤更早、更结构化 |

## 6. 剩余建议

1. 冷启动仍需优化：当前初始 REST 全量细节快照仍可能约 40 秒。建议下一轮实现 two-stage cold start：先 1m/5m/15m/1h 快照进入交易循环，再异步补 4h/1d。
2. LLM 耗时仍是实盘入场延迟核心之一。建议对 top 1-3 标的做开仓前 micro-refresh，并在执行前复核最新价格、VWAP、5m/15m trigger。
3. 前端 chunk 体积可后续做 code split，不影响交易功能。

## 7. 结论

本轮修复后，AIT 的 Hunter v7 数据源链路在准确性和稳定性上有实质提升：压缩指标语义恢复一致，细节数据容错增强，金融类过滤更稳，anti-repeat 不再压制升级机会，观察层配置可运营化。当前热运行链路已基本符合“稳、准、快、轻”，剩余主要短板是冷启动 REST 耗时和 LLM 决策耗时。
