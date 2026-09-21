# Square Monitor 运维手册（币安广场热度监控）

> 相关代码：`scripts/square-monitor/`
> 上游模块文档：[`scripts/square-monitor/README.md`](../../scripts/square-monitor/README.md)（功能与配置项）
> 集成方式：[`scripts/square-monitor/INTEGRATION.md`](../../scripts/square-monitor/INTEGRATION.md)

本文只记录**部署与排障**相关的部分，尤其是那些排查过、又容易再犯的坑。

---

## 1. 看板链路

AiT 前端的「Square 热度」面板读的是 Go 后端的 `/api/square-heat`，
数据由 `scripts/square-monitor/` 这套 Python 服务采集后写入。

```
worker.py（抓取 worker，setsid 脱离）
    │  写入
    ▼
binance_square.db  ──►  web.py（FastAPI :8000）
                            │
                            │  socat 中继 [::1]:8000
                            ▼
                     Go 后端 /api/square-heat  ──►  前端看板
```

看板报 `No heat signals` / `Square Monitor may be offline` 时，
**先分层定位，不要一上来就怀疑真人验证**。

```bash
curl -s http://127.0.0.1:8080/api/square-heat | head -c 300   # Go 后端（看板真正读的）
tail -5 .logs/square-monitor-worker.log                       # 抓取 worker
tail -5 .logs/square-monitor.log                              # web.py
grep "health:" .logs/ait-stack-autostart.log | tail -1        # 栈健康行
```

健康行示例：

```
health: sing-box=up ait=up vite=up square-web=up square-worker=up socat=up funnel=up v7-persist=DOWN
```

`v7-persist=DOWN` 是**预期状态**，由 `.proxy/ait-stack.env` 里的
`AIT_V7_PERSIST_TRACK=0` 控制，与 Square Monitor 无关。

---

## 2. 三层故障（都真实发生过）

### 第 1 层：Playwright 浏览器不存在

**症状**：worker 日志里 `Executable doesn't exist at /root/.cache/ms-playwright/...`

**原因**：容器 rootfs / 镜像刷新会清空 `/root`。

**处理**：把浏览器装到 `/workspace` 下，并在 `.proxy/ait-stack.env` 里持久化：

```bash
PLAYWRIGHT_BROWSERS_PATH=/workspace/.ms-playwright
```

### 第 2 层：`web.py` 死了，但健康检查没发现

**症状**：`square-monitor.log` 里 `ModuleNotFoundError: No module named 'fastapi'`
（venv 重建窗口期崩溃），但健康行仍显示 `square-web=up`。

**原因**：健康检查原先用 `port_listening 8000` 判断，
而 **socat 的 `TCP6-LISTEN:8000,bind=[::1]` 中继也占着 8000**，
于是「端口有人听」恒为真，真正的后端死了也发现不了——故障可以无声持续一整天。

**处理**：`is_square_web_up()` 改为要求真实 IPv4 监听
（`0.0.0.0:8000` / `127.0.0.1:8000`），因为 `web.py` 以 `WEB_HOST=0.0.0.0` 启动：

```bash
pgrep -f '[.]venv/bin/python web\.py' >/dev/null 2>&1 && return 0
ss -tlnH 'sport = :8000' 2>/dev/null \
  | grep -qE '(0\.0\.0\.0|127\.0\.0\.1):8000' && return 0
return 1
```

> **通用教训**：判断某个服务是否活着，要匹配**它自己的监听地址或进程**，
> 不要只匹配端口号——同端口可能被中继、代理之类的东西占着。

### 第 3 层：代理触发币安 AWS WAF 真人验证（最常见）

**症状**：抓取长期 0 条；页面标题为 `Human Verification`，
响应头带 `x-amzn-waf-action: challenge`，随后 `405 waf=captcha`，
`pgc/feed` 接口调用数为 0。

**判据对照（实测可复现）**：

| 浏览器出口 | 结果 |
|---|---|
| 直连（机房本机 IP） | 真实广场页，`pgc/feed` 正常 ✅ |
| 走代理（Oracle 机房 IP） | `Human Verification`，0 条 ❌ |

**根因**：币安对机房 / 数据中心 IP 段返回 AWS WAF 挑战。
这不是需要人工点验证码的问题，**换出口即可**。

**关键坑：Chromium 会读环境变量 `HTTP_PROXY` / `HTTPS_PROXY`。**
即使 Playwright 完全不传 `proxy` 选项，只要环境里有这两个变量，浏览器照样走代理。
所以「只把 `launch_kwargs["proxy"]` 去掉」是无效的——
必须让启动器**真正移除**这些变量（设成空串不够稳）：

```bash
env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy ...
```

---

## 3. 出口分离设计

浏览器和行情 API 对出口的要求**正好相反**：

| 用途 | 要求 | 原因 |
|---|---|---|
| 浏览器抓广场 | **必须直连** | 走代理会触发 WAF 真人验证 |
| `market.py` 取行情 | **必须走代理** | `api.binance.com` 直连返回 HTTP 451（地区限制） |

因此拆成两个独立开关：

```bash
# scraper.py：浏览器出口，默认不设 → 永远直连
proxy_server = (os.environ.get("SQUARE_BROWSER_PROXY") or "").strip()
if proxy_server.lower() in ("", "none", "off", "direct", "false", "0"):
    proxy_server = None            # 只有显式配置了才走代理

# market.py：行情出口，优先专用开关
proxy = (os.environ.get("SQUARE_API_PROXY")
         or os.environ.get("SQUARE_PROXY")
         or os.environ.get("HTTPS_PROXY")
         or os.environ.get("HTTP_PROXY"))

# ait-stack-autostart 拉起 worker 时：
#   exec env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy \
#            SQUARE_CDP_URL= SQUARE_BROWSER_PROXY= \
#            SQUARE_API_PROXY=http://127.0.0.1:10809 \
#            NO_PROXY=127.0.0.1,localhost,::1 no_proxy=127.0.0.1,localhost,::1 \
#            "$SQUARE_VENV_PY" worker.py
```

**验证成功的样子**：单轮 `累计 N 条` 不再是 0（实测 857 条），
`/api/square-heat` 返回 `count>0`、`items>0`、`updated` 是当前时间、`error=None`。

### 人工复核工具

`scripts/square-monitor/manual_playwright_verify.py` 会**故意走代理**并开一个可见浏览器，
用来复现 WAF 挑战、确认问题确实出在出口上：

```bash
python3 scripts/square-monitor/manual_playwright_verify.py
# 输出 MANUAL_VERIFY_READY status=... title=...  后保持打开，Ctrl-C 退出
```

`title=Human Verification` 即确认是 WAF；`title` 为正常广场页则说明出口没问题。

---

## 4. 配置开关速查

| 环境变量 | 作用 | 默认 |
|---|---|---|
| `SQUARE_BROWSER_PROXY` | 浏览器出口代理；留空 = 直连 | 空（直连） |
| `SQUARE_API_PROXY` | 行情出口代理 | 无（回退到 `HTTPS_PROXY`） |
| `SQUARE_CDP_URL` | 接管已存在的 Chrome 调试端口 | 空 |
| `PLAYWRIGHT_BROWSERS_PATH` | Playwright 浏览器安装位置 | `/workspace/.ms-playwright` |
| `AIT_V7_PERSIST_TRACK` | Hunter v7 持久化跟踪开关 | `0`（关） |

---

## 5. 重启注意

- **`worker.py` 是 `setsid` 脱离进程组的**：改了它的环境变量后，
  必须**单独 kill worker**，`ait-stack-autostart` 才会用新环境重新拉起；
  只重启 watcher 不会换掉已存在的 worker。
- **改完 `ait-stack-autostart` 必须重启 watcher**：运行中的 bash 已把函数载入内存，
  不会热加载新代码。
- **重启 watcher 前要清 flock 残留**：`kill` 掉 watcher 后它的 `sleep` 子进程会孤儿化
  并继续持有 `/tmp/ait-stack-autostart.watch.lock`，新实例会直接退出。
  顺序：kill watcher → `fuser <lockfile>` 找出并 kill 残留 → 确认 LOCK FREE → 再启动。
