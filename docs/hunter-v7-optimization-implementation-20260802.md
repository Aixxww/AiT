# Hunter v7 实测问题优化实施记录 - 2026-08-02

## 背景

2026-08-02 三轮币安实时测试暴露出三类问题：

1. 首轮 REST 明细异常会拉低 universe 覆盖，不能直接纳入主开仓率结论。
2. `alt_ladder_breakdown_short` 直接 REVIEWABLE 仍可能只缺 `5m_or_15m_close_below_trigger`，缺少反抽失败确认。
3. alt-ladder short 短窗 MFE 达到 0.6%-0.9% 后仍未 TP0，若不保护容易回吐为亏损止损。

## 已实施

### 1. Validator degraded round 与 valid-round 开仓率

- 每轮报告新增：
  - `rest_error_rate`
  - `universe_coverage`
  - `degraded`
  - `degradation_reasons`
- 规则：
  - REST error rate > 20% 标记 degraded。
  - universe coverage < 30% 标记 degraded。
- 多轮验证结束后新增 run summary：
  - all open-review rate
  - valid-round open-review rate
  - degraded round 列表与原因

### 2. alt_ladder_breakdown_short 反抽失败门

- 最终 tier guard 增加 `no_new_high_after_rejection` 要求。
- 若只缺 `5m_or_15m_close_below_trigger`，但未通过 `no_new_high_after_rejection`，不再升为 REVIEWABLE。
- 新 tier reason：`alt_ladder_short_rebound_pending`。
- 保留已有高质量路径：带 `no_new_high_after_rejection` 的 early/mid/close-through alt-ladder short 仍可 REVIEWABLE/EXECUTABLE。

### 3. alt_ladder_breakdown_short MFE 后保本保护

- outcome tracker 新增 `BreakevenMFEThreshold`，默认 0.60%。
- `alt_ladder_breakdown_short` SHORT 达到 MFE 阈值后，将动态止损推进到 entry breakeven。
- 目标是把短窗浮盈回抽从亏损 STOP 转为保本/保护 STOP。

## 验证

已通过：

```bash
go test -count=1 ./provider/local ./trader ./kernel ./store ./cmd/hunter_v7_validate
```

短实时验证：

```bash
HTTP_PROXY=http://127.0.0.1:7897 HTTPS_PROXY=http://127.0.0.1:7897 ALL_PROXY=http://127.0.0.1:7897 NO_PROXY=localhost,127.0.0.1,::1 TZ=Asia/Shanghai \
go run ./cmd/hunter_v7_validate \
  -rounds=1 \
  -top-detail=80 \
  -max-workers=4 \
  -max-output=20 \
  -watch-output=5 \
  -min-priority=45 \
  -aggressive=true \
  -post-track-duration=0 \
  -out-dir=reports/hunter-v7-optimization-verify-20260802
```

验证结果：

- run summary 正常生成。
- top-detail=80 导致 universe coverage=20.4%，被正确标记为 degraded。
- `alt_ladder_breakdown_short` 输出 3 条，最终 open-review=0。
- `CAPUSDT` 被新 gate 拦截，tier reason=`alt_ladder_short_rebound_pending`。

## 后续复测建议

下一轮建议用 `top-detail=220`、3 轮、post-track 30 分钟，观察：

- valid-round open-review rate 是否保持 15%-30%。
- `alt_ladder_short_rebound_pending` 是否减少 TAGUSDT 类反抽浮亏。
- breakeven stop 是否降低 `alt_ladder_breakdown_short` 的亏损 STOP 数量。
