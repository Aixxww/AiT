# 币安广场热度监控 (Binance Square Heat Monitor)

> 原项目: `binance-square-monitor` — 整合到 AiT 策略引擎作为社交情绪数据源

## 信号轮询周期

| 环节 | 周期 | 说明 |
|------|------|------|
| 抓取循环 | **5分钟/轮** | Playwright headless Chrome 连续滚动，拦截币安内部 `pgc/feed` API |
| 帖子热度计算 | 每轮结束时 | 对最近 15 分钟内的帖子按互动加权评分（赞×1 + 评×3 + 转×5）× 时间衰减 |
| 合约快照刷新 | 每轮结束时 | 前30名代币各调 Binance 8+公开API（OI、资金费率、多空比、深度等） |
| 综合信号评分 | 实时（请求时计算） | 30分社交基线 + 资金费率 + 价格动量 + OI变化 + Taker比 + 深度 + 多空比 → 0-100分 |
| **端到端延迟** | **5-8分钟** | 热帖出现 → 排行榜反映信号 |

### 关键时间参数（`config.py`）

```python
SCRAPE_ROUND_SECONDS = 300      # 每轮滚动600秒
SHORT_WINDOW_MINUTES = 15       # 热度窗口：只看15分钟内帖子
SHORT_HALF_LIFE_HOURS = 0.25    # 时间衰减半衰期：15分钟
COMPOSITE_HISTORY_WINDOW = 20   # 综合评分回溯轮数
MARKET_ANALYSIS_MAX = 30        # 合约快照最大代币数
```

## HTTP API 接口（Go 客户端消费）

**端口**: `http://localhost:8000`

### `GET /api/leaderboard`

返回综合热度榜（按信号健康度→综合热度分→当前热度排序）。

```json
{
  "updated_at": "2026-05-11T12:00:00",
  "items": [
    {
      "token": "PEPE",
      "composite_score": 54.3,
      "score": 42.5,
      "mentions": 15,
      "trend": "↑↑",
      "market": {
        "snapshot": {
          "mark_price": 0.0000123,
          "oi_change_1h_pct": 3.5,
          "change_1h_pct": 2.1,
          "change_24h_pct": 15.8,
          "funding_rate_pct": 0.01,
          "long_short_ratio": 1.8
        },
        "analysis": {
          "verdict": "✅ 看起来健康",
          "direction": "↑ 偏多",
          "score": 72,
          "tags": ["OI上升+价格上涨"]
        }
      }
    }
  ],
  "skipped_no_contract": 3
}
```

## 启动方式

```bash
cd /Users/aixx/Code/AiT/scripts/square-monitor

# 安装依赖（首次）
pip install -r requirements.txt
playwright install chromium

# 启动
bash start.sh          # 后台运行
# 或
python web.py          # 前台运行（含 Web dashboard）

# 访问 dashboard
open http://localhost:8000
```

## 与 AiT 策略引擎的集成

Go 端通过 `provider/square/client.go` 消费此服务：

1. AiT 启动时，若策略配置了 `source_type: "square_heat"`，Go 客户端会连接 `localhost:8000`
2. 每次策略执行周期调用 `GET /api/leaderboard`
3. 按 `composite_score` 过滤，取前 N 个有合约的代币作为交易候选
4. 候选币进入 AiT 已有的 enrichment pipeline（OI/NetFlow/价格排名量化数据）
5. 若 Python 服务未运行，策略自动静默回退到 AI500 数据源

## 文件说明

| 文件 | 用途 |
|------|------|
| `config.py` | 所有时间参数和配置常量 |
| `scraper.py` | Playwright 币安广场抓取，拦截页内 `pgc/feed` API |
| `analyzer.py` | 代币提取 + 时间衰减热评分 + 综合热度 |
| `filters.py` | 帖子质量过滤（Bot检测 / 大V账号验证） |
| `signals.py` | 多因子信号分析（社交+OI+费率+动量+深度+多空比 → 0-100分） |
| `market.py` | Binance 公开 REST API 集合（标记价、OI、K线、多空比、深度） |
| `market_realtime.py` | WebSocket 实时缓存（仅观察列表代币，秒级更新） |
| `storage.py` | SQLite 数据层（帖子、作者、热度历史、快照、观察列表） |
| `worker.py` | 主工作进程：抓取→存储→排行榜→合约快照 |
| `web.py` | FastAPI Web Dashboard（含 `/api/leaderboard` 接口） |
| `main.py` | 终端 CLI 入口（Rich 表格输出） |
