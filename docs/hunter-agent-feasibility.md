# Hunter 自动交易 Agent 可行性分析

## 结论：✅ 技术可行，但需要 3 处工程改造

---

## 一、整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                    Claude Code Session (24/7)                    │
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐   │
│  │  CronCreate   │───▶│ Hunter 选币  │───▶│  AI 策略决策      │   │
│  │  每3~5分钟     │    │ (Cron 触发)  │    │  (策略提示词)     │   │
│  └──────────────┘    └──────┬───────┘    └────────┬─────────┘   │
│                             │                     │              │
│                     ┌───────▼───────┐    ┌────────▼─────────┐   │
│                     │ AiT Backend    │    │  Binance CLI      │   │
│                     │ :8080/api/     │    │  (USDS-M Futures) │   │
│                     │ hunter-coins   │    │  建仓/平仓/止盈止损 │   │
│                     └───────────────┘    └──────────────────┘   │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Monitor / Guard                        │   │
│  │  - 持仓监控 (position check)                              │   │
│  │  - 保证金告警 (margin > 60% → 暂停)                       │   │
│  │  - 连败熔断 (consecutive loss → reduce)                   │   │
│  │  - 日志记录                                               │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 二、可行性逐项评估

### 2.1 Hunter 选币 — ⚠️ 需要加 API 端点

| 方案 | 可行性 | 延迟 | 数据完整度 |
|------|--------|------|-----------|
| **A. 加 `/api/hunter/coins` 端点** (推荐) | ✅ | <1s | 完整 (score + tags + 4柱明细) |
| B. 用 `POST /api/strategies/test-run` | ⚠️ | 2-5s | 只有 symbol，没有 score/tags |
| C. CLI `go run cmd/coinselect/main.go` | ⚠️ | 3-8s | 完整但要解析 JSON 文件 |
| D. Agent 直接调 Binance API 自算 | ❌ | 复制整个 Hunter 逻辑，维护噩梦 |

**推荐方案 A**: 在 AiT backend 加一个轻量端点：

```go
// api/handler_hunter.go
func handleHunterCoins(w http.ResponseWriter, r *http.Request) {
    client := local.NewClient("")
    coins, err := client.GetHunterList()  // 已有函数，含完整4柱评分
    // 返回 JSON: {coins: [{symbol, score, tags[], pillar_scores}]}
}
```

**工作量**: ~30 行 Go 代码，无需重启服务 (加路由即可热编译)

---

### 2.2 Binance CLI 交易执行 — ✅ 完全支持

`binance-cli` (未安装) 支持 USDS-M 全套操作：

| 操作 | 命令 | 状态 |
|------|------|------|
| 设杠杆 | `futures-usds change-initial-leverage` | ✅ |
| 设保证金模式 | `futures-usds change-margin-type` | ✅ |
| 市价开仓 | `futures-usds new-order --type MARKET` | ✅ |
| 限价开仓 | `futures-usds new-order --type LIMIT` | ✅ |
| 止损单 | `futures-usds new-order --type STOP_MARKET` | ✅ |
| 止盈单 | `futures-usds new-order --type TAKE_PROFIT_MARKET` | ✅ |
| 追踪止损 | `futures-usds new-order --type TRAILING_STOP_MARKET` | ✅ |
| 平仓 | `futures-usds new-order --reduce-only` | ✅ |
| 查持仓 | `futures-usds position-information-v3` | ✅ |
| 查账户 | `futures-usds account-information-v3` | ✅ |
| 测试下单 | `futures-usds test-order` | ✅ |
| 查限频 | `futures-usds query-user-rate-limit` | ✅ |

**⚠️ 安全限制**: SKILL.md 第 56 行要求 prod 交易必须用户输入 `CONFIRM`。
→ 自动化需要绕过此限制 (见下方 §3 方案)

---

### 2.3 Claude Code 作为 AI 决策引擎 — ✅ 可行

| 能力 | 工具 | 状态 |
|------|------|------|
| 定时触发 | `CronCreate` | ✅ (会话内最长 7 天) |
| 并行分析 | `Agent` | ✅ |
| HTTP 请求 | `WebFetch` / `Bash curl` | ✅ |
| 执行交易 | `Bash binance-cli` | ✅ |
| 状态记忆 | Memory 文件 | ✅ |
| 持续监控 | `Monitor` | ✅ |

---

### 2.4 24/7 自动运行 — ⚠️ 有约束

| 约束 | 影响 | 解决方案 |
|------|------|---------|
| Claude Code 会话最长 ~7 天 | Cron job 7 天后过期 | 设置 cron 定期提醒重启，或用 nohup wrapper |
| CronCreate 仅会话内有效 | 重启后需重新设置 | 脚本化启动流程 |
| Claude 不会主动醒来 | 无事件时无输出 | CronCreate 设定轮询间隔 |
| 网络/API 故障 | 交易中断 | 内置重试 + 告警 |

---

## 三、需要的 3 处工程改造

### 改造 1: AiT — 新增 Hunter API 端点 (30行)

```go
// api/server.go 中添加路由
r.HandleFunc("/api/hunter/coins", handleHunterCoins).Methods("GET")

// api/hunter_handler.go 新文件
func handleHunterCoins(w http.ResponseWriter, r *http.Request) {
    client := local.NewClient("")
    coins, err := client.GetHunterList()
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    // 只返回需要的字段
    type HunterOutput struct {
        Symbol    string   `json:"symbol"`
        Score     float64  `json:"score"`
        Tags      []string `json:"tags"`
        Price     float64  `json:"price"`
        Change24h float64  `json:"change_24h"`
    }
    var out []HunterOutput
    for _, c := range coins {
        out = append(out, HunterOutput{
            Symbol: c.Pair, Score: c.Score,
            Tags: c.SignalTags, Price: c.StartPrice,
            Change24h: c.IncreasePercent,
        })
    }
    json.NewEncoder(w).Encode(map[string]interface{}{"coins": out})
}
```

### 改造 2: Binance CLI 安装 + 配置

```bash
# 1. 安装
npm install -g @binance/binance-cli

# 2. 配置 (用 testnet 先测试)
binance-cli profile create \
  --name ait-agent \
  --api-key <YOUR_KEY> \
  --api-secret <YOUR_SECRET> \
  --env testnet    # ← 先用测试网!

# 3. 选中 profile
binance-cli profile select --name ait-agent

# 4. 验证
binance-cli futures-usds account-information-v2
```

### 改造 3: Agent 自动化脚本 (Claude Code 内启动)

见下方 §四 的完整 Agent 循环逻辑。

---

## 四、Agent 自动交易循环设计

### 每轮循环 (每 3-5 分钟)

```
┌─ Step 1: 环境检查 ─────────────────────────────────┐
│ - BTC 大盘: RSI7, MACD, 1h趋势                      │
│ - 账户状态: 保证金%, 持仓数, 日盈亏                   │
│ - 如果 保证金 > 60% → 跳到 Step 4 (只管持仓)          │
│ - 如果 日亏损 > 5% → 跳到 Step 5 (暂停)              │
└─────────────────────────────────────────────────────┘
          │
          ▼
┌─ Step 2: Hunter 选币 + 数据获取 ────────────────────┐
│ - GET :8080/api/hunter/coins → Top-10 带 tags        │
│ - 对 Top-5 币获取详细市场数据 (K线+OI+Quant)          │
│ - 过滤: 排除 wash_*, 冷却中, 24h>40% 的币            │
└─────────────────────────────────────────────────────┘
          │
          ▼
┌─ Step 3: AI 策略决策 ──────────────────────────────┐
│ - 将 Hunter tags + 市场数据 + 持仓信息 → 策略提示词   │
│ - Claude 自我推理: 逐项检查共振条件                    │
│ - 输出: JSON 决策 [{symbol, action, leverage, ...}]  │
│ - 信心度 ≥ 75 且盈亏比 ≥ 1:3.5 才执行               │
└─────────────────────────────────────────────────────┘
          │
          ▼
┌─ Step 4: 持仓管理 ─────────────────────────────────┐
│ - 对每个持仓: 检查 PnL%, PeakPnL, 回撤              │
│ - 回撤 > 30% from peak → 触发移动止盈               │
│ - 亏损 > -3.5% → 强制止损                            │
│ - 反向信号出现 → 考虑平仓                             │
└─────────────────────────────────────────────────────┘
          │
          ▼
┌─ Step 5: 执行交易 ─────────────────────────────────┐
│ - 执行 Step 3/4 产生的决策                            │
│ - 开仓: 设杠杆 → 市价开 → 挂止损单 + 止盈单          │
│ - 平仓: reduce-only 市价平仓                         │
│ - 记录日志到 ~/.openclaw/workspace/hunter-agent/     │
└─────────────────────────────────────────────────────┘
```

### CronCreate 配置

```python
# 主循环: 每 5 分钟
CronCreate:
  cron: "*/5 * * * *"   # 每5分钟
  recurring: true
  prompt: |
    执行 Hunter 自动交易循环:
    1. GET http://localhost:8080/api/hunter/coins 获取选币
    2. 用策略提示词分析共振信号
    3. 检查现有持仓 → 止盈止损
    4. 如有高信心信号 → 执行交易
    5. 记录决策日志

# 持仓监控: 每 1 分钟 (更频繁)
CronCreate:
  cron: "*/1 * * * *"
  recurring: true
  prompt: |
    持仓紧急检查:
    1. binance-cli futures-usds position-information-v3
    2. 任何持仓 PnL < -3.5% → 立即市价平仓
    3. 保证金使用率 > 60% → 发送告警

# 日报: 每天 UTC 0:00
CronCreate:
  cron: "3 0 * * *"
  recurring: true
  prompt: |
    生成日交易报告:
    1. 汇总今日所有交易 (从日志)
    2. 计算: 胜率, 总盈亏, 最大回撤
    3. 推送到 Telegram
```

---

## 五、风险管理方案

### 自动风控 (代码层)

| 规则 | 阈值 | 动作 |
|------|------|------|
| 单笔止损 | -3.5% PnL | 强制市价平仓 |
| 追踪止盈 | 峰值回撤 30% | 市价平仓锁定利润 |
| 保证金警戒 | > 60% | 禁止新开仓 |
| 保证金危险 | > 80% | 减仓至 50% |
| 连败熔断 | 连续 2 笔亏损 | 仓位减半 |
| 连败暂停 | 连续 3 笔亏损 | 暂停 2 小时 |
| 日亏损限制 | > 5% | 当日停止交易 |
| 最大持仓数 | 3 个 | 不开新仓 |
| 异常检测 | 单笔 > 5% 权益 | 拒绝执行 |

### 安全措施

| 措施 | 说明 |
|------|------|
| **Testnet 先行** | 先用 testnet 跑 3-7 天验证 |
| **小仓位起步** | 初始仓位 = equity × 0.2 (正式的 1/5) |
| **Telegram 告警** | 每笔交易实时推送确认 |
| **日志审计** | 所有决策和执行记录到文件 |
| **Kill Switch** | 用户随时可通过 Telegram 发 "STOP" 暂停 |

---

## 六、完整实施路线图

### Phase 1: 基础设施 (Day 1)

```
□ 安装 binance-cli
□ 配置 Binance Testnet API keys
□ 在 AiT 加 /api/hunter/coins 端点
□ 验证 Hunter API 返回正确数据
□ 验证 binance-cli testnet 连通性
```

### Phase 2: Agent 核心 (Day 2-3)

```
□ 实现策略决策循环 (Step 1-5)
□ 集成策略提示词
□ 实现持仓管理逻辑
□ 实现风控规则
□ 测试: testnet 模拟开仓/平仓
```

### Phase 3: 自动化 + 监控 (Day 4-5)

```
□ CronCreate 设定主循环
□ CronCreate 设定持仓监控
□ Telegram 交易通知
□ 日志系统
□ Kill Switch
```

### Phase 4: 实盘试运行 (Day 6-12)

```
□ 切换到 Mainnet (小仓位)
□ 观察 7 天
□ 收集实际胜率/回撤数据
□ 与策略预期对比
□ 调参优化
```

---

## 七、风险声明

1. **Claude Code 会话限制**: CronCreate 最长 7 天。需要定期重启会话或用外部 cron wrapper
2. **API 故障**: Hunter API 或 Binance API 停机时交易中断，需要 fallback 逻辑
3. **滑点**: 市价单在低流动性币上可能有较大滑点
4. **网络延迟**: Claude 推理 + API 调用可能需要 10-30 秒，错过快速行情
5. **策略失效**: 市场环境变化可能导致策略从正期望变为负期望
6. **资金安全**: API key 权限必须限制为 Futures only，禁止提现权限

### 建议 API Key 权限设置

```
✅ Enable Futures
✅ Enable Spot & Margin Trading (可选)
❌ Disable Withdrawals
❌ Disable Internal Transfer
✅ Restrict to trusted IPs (本机 IP)
```
