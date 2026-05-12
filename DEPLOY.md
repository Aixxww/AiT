# AiT 本地部署与测试指南

## 本次代码审计修复

| Bug | 文件 | 修复 |
|-----|------|------|
| `getEffectiveCoinCount` 缺少 `"square_heat"` case | `store/strategy.go:866` | ✅ 已添加，mixed模式也加入 UseSquareHeat 计数 |
| SSRF: `square_heat_url` 未验证 | `kernel/engine.go:205` | ✅ 已添加 `security.ValidateURL()`，非法URL回退默认值 |

## 部署前预检

```bash
cd /Users/aixx/Code/AiT
go build ./... && go vet ./...   # Go 编译 + 静态检查
cd web && npx tsc --noEmit        # TypeScript 类型检查
```

---

## Step 1: 创建 .env 配置

```bash
cp .env.example .env
```

编辑 `.env`，关键字段：

```bash
# 端口
AiT_BACKEND_PORT=8080
AiT_FRONTEND_PORT=3000

# JWT（测试用随机值即可）
JWT_SECRET=$(openssl rand -base64 32)

# 加密（测试可禁用）
TRANSPORT_ENCRYPTION=false

# 数据库（SQLite 测试最简单）
DB_TYPE=sqlite
DB_PATH=data/data.db

# 时间
AiT_TIMEZONE=Asia/Shanghai
```

```bash
# 运行时自动创建 data/ 目录
mkdir -p data
```

---

## Step 2: 启动币安广场热度监控（Python Sidecar）

**终端 1** — 启动 Square Monitor：

```bash
cd /Users/aixx/Code/AiT/scripts/square-monitor

# 首次运行 - 安装 Python 依赖
pip install -r requirements.txt
playwright install chromium

# 启动服务（前台，方便查看日志）
python web.py
```

**验证** — 看到类似输出：
```
INFO:     Started server process [12345]
INFO:     Application startup complete.
INFO:     Uvicorn running on http://0.0.0.0:8000
```

打开浏览器：`http://localhost:8000` → 应看到 Dashboard

**预热数据**（第一次访问时排行榜为空，需要等一轮采集）：
```bash
# 等5-6分钟后，数据才会出现
curl -s http://localhost:8000/api/leaderboard | python3 -m json.tool | head -30
```

如果返回 `{"items": [], "skipped_no_contract": 0}` — 数据还没采集完，正常。

---

## Step 3: 启动 AiT 后端

**终端 2**：

```bash
cd /Users/aixx/Code/AiT
go run main.go
```

应看到：
```
🚀 AiT - AI-Powered Trading System
✅ Configuration loaded
✅ Database initialized
```

---

## Step 4: 启动前端开发服务器

**终端 3**：

```bash
cd /Users/aixx/Code/AiT/web
npm install
npm run dev
```

浏览器打开 `http://localhost:3000`

---

## Step 5: 创建测试策略（广场热度数据源）

1. 登录 AiT（`http://localhost:3000`）
2. 进入策略管理 → 新建策略
3. 在 **Coin Source** 区域：
   - 选择 `🔥 广场热度` (Square Heat)
   - Limit 设为 `10`
4. 在 **AI Model** 区域选择已配置的模型（如 DeepSeek / OpenAI）
5. 选择交易所连接
6. 保存策略

---

## Step 6: 监控日志与调试

### Go 后端日志（终端 2）

关注以下日志序列：

```
# 正常启动
📊 Using local Binance-backed data provider (AI500/OI/NetFlow/Price)
🔥 Using Binance Square heat source (url=http://localhost:8000, minScore=25.0)

# 每个交易周期：
🔥 Square Heat: loaded 8 candidates (limit=10)
# 或回退：
⚠ Square Heat unavailable, falling back to ai500: square request failed: ...
```

### Python 服务日志（终端 1）

关注每轮采集的状态：
```
# 抓取开始
🔄 Starting scrape round (timeout=300s)...
# 抓取完成
✅ Round complete: 153 posts saved, 12 unique tokens, 8 with contracts
# 快照刷新
📊 Market snapshot refreshed for 8 tokens
```

### 关键错误标志

| 日志信息 | 含义 | 处理 |
|---------|------|------|
| `Square Heat unavailable: connection refused` | Python服务未启动 | 确认终端1运行 |
| `SSRF check rejected` | URL配置非法，已自动用默认值 | 正常，无需处理 |
| `square_heat fallback` | 热度服务不可用，已静默回退AI500 | 当Python服务不可用时自动回退 |
| `loaded 0 candidates` | 排行榜无有效数据 | 等待采集积累（首次需5-10分钟） |

---

## 信号延迟时间线

从帖子出现在币安广场到成为AiT交易候选，端到端约 **6-11分钟**：

```
1. 帖子发布           T+0min
2. Playwright 抓取     T+0~5min（取决于采集窗口位置）
3. 热度评分计算        T+5min（抓取结束后）
4. 合约快照刷新        T+6min（top 30代币，~0.4s/币）
5. AiT 策略周期        T+7-11min（取决于策略执行间隔）
```

### 快速验证命令

```bash
# 1. Python服务是否健康
curl -s http://localhost:8000/api/leaderboard | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Items: {len(d[\"items\"])}, Skipped: {d[\"skipped_no_contract\"]}')"

# 2. Go端能否解析
curl -s http://localhost:8080/api/health
```

---

## 数据源切换对比测试清单

用同一账户测试以下场景，记录日志比对：

| 数据源 | 币源数 | 信号类型 | 预期 |
|--------|--------|----------|------|
| `ai500` | 10 | 涨幅+成交量+活跃度 | 偏市场动量 |
| `oi_top` | 10 | OI 增加排名 | 偏持仓矛盾 |
| `square_heat` | 10 | 社交热度+合约信号 | 偏情绪面 |
| `mixed` (heat+ai500) | 10 | 双源合并 | 信号交叉确认 |

### 关注指标日志

```bash
# 开启详细日志（在 .env 中加入）：
LOG_LEVEL=debug

# Go端关注
# kernel/engine.go 中所有 logger.Infof 输出
# provider/square/client.go 的 HTTP 请求结果
# auto_trader.go 的 market.Get(symbol) 数据

# Python端关注
# worker.py 的每轮 Round complete 输出
# analyzer.py 的 compute_short_scores 结果
```

---

## 问题排查

**问题1: `source not configured (squareClient is nil)`**
→ 确认策略 config 中 `source_type` 是 `"square_heat"`，不是字符串拼写错误

**问题2: `Square Heat unavailable: Get "http://localhost:8000/...": context deadline exceeded`**
→ Python 服务响应超时，检查终端1是否正常运行

**问题3: `loaded 0 candidates`**
→ 热帖窗口还在采集，等待第一轮完成（~5-6分钟）

**问题4: 前端 CoinSourceEditor 不显示 `🔥 广场热度`**
→ 检查前端是否重新 build：`cd web && npm run build`
