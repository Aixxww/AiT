"""
主程序：定时抓取 -> 过滤 -> 分析 -> 终端展示 + Web API 服务
运行：python main.py
按 Ctrl+C 停止
"""
import asyncio
import signal
import sys
import time
import threading
from datetime import datetime, timezone

from rich.console import Console
from rich.table import Table
from rich.columns import Columns

import config
import storage
from scraper import SquareScraper
from filters import is_likely_human
from analyzer import extract_tokens_from_text, compute_short_scores
from market import has_perpetual, get_market_snapshot, get_futures_symbols
from signals import analyze as analyze_signals


console = Console()
_running = True
_scraper = None  # Global reference for signal handler


def start_web_server():
    """在后台守护线程中启动 FastAPI web 服务 (port 8000)"""
    import uvicorn
    from web import app
    uvicorn.run(app, host=config.WEB_HOST, port=config.WEB_PORT, log_level="warning")


def stop(*_):
    global _running, _scraper
    _running = False
    # Also tell the scraper to stop its current loop immediately
    if _scraper:
        _scraper.request_stop()
    console.print("\n[yellow]收到退出信号，抓完当前轮后停止...[/yellow]")


signal.signal(signal.SIGINT, stop)
signal.signal(signal.SIGTERM, stop)


def _utcnow():
    return datetime.now(timezone.utc)


def render_social_table(title: str, scores: list[dict], top_n: int,
                        new_hot_tokens: set = None) -> Table:
    new_hot_tokens = new_hot_tokens or set()
    table = Table(title=title)
    table.add_column("#", style="dim", width=3)
    table.add_column("代币", style="bold cyan")
    table.add_column("热度分", justify="right", style="bold yellow")
    table.add_column("帖子", justify="right")
    table.add_column("点赞", justify="right")
    table.add_column("评论", justify="right")
    table.add_column("转发", justify="right")

    for i, s in enumerate(scores[:top_n], 1):
        token_display = s["token"]
        if s["token"] in new_hot_tokens:
            token_display = f"[bold red]{s['token']} 🔥[/bold red]"
        table.add_row(
            str(i),
            token_display,
            f"{s['score']:.1f}",
            str(s["unique_posts"]),
            str(s["total_likes"]),
            str(s["total_comments"]),
            str(s["total_shares"]),
        )
    return table


def render_market_table(rows: list[dict]) -> Table:
    """rows: [{token, social_score, snap, analysis}]"""
    table = Table(title="📈 合约综合分析（仅信息展示，非投资建议）")
    table.add_column("代币", style="bold cyan")
    table.add_column("社交", justify="right", style="yellow")
    table.add_column("价格", justify="right")
    table.add_column("15m", justify="right")
    table.add_column("1h", justify="right")
    table.add_column("4h", justify="right")
    table.add_column("费率/8h", justify="right")
    table.add_column("OI 1h", justify="right")
    table.add_column("多空", justify="right")
    table.add_column("综合", justify="right", style="bold")
    table.add_column("判断")

    def color_pct(v, good_dir="up"):
        """根据数值正负上色"""
        if v is None:
            return "[dim]-[/dim]"
        sign = "+" if v > 0 else ""
        color = "green" if (v > 0) == (good_dir == "up") else "red"
        return f"[{color}]{sign}{v:.2f}%[/{color}]"

    def color_fr(fr_pct):
        if fr_pct is None:
            return "[dim]-[/dim]"
        if fr_pct >= 0.05:
            return f"[red]{fr_pct:+.3f}%[/red]"
        if fr_pct <= -0.01:
            return f"[yellow]{fr_pct:+.3f}%[/yellow]"
        return f"{fr_pct:+.3f}%"

    for r in rows:
        snap = r["snap"]
        ana = r["analysis"]
        price = snap.get("mark_price")
        price_str = f"{price:.4f}" if price else "-"

        table.add_row(
            r["token"],
            f"{r['social_score']:.1f}",
            price_str,
            color_pct(snap.get("change_15m_pct")),
            color_pct(snap.get("change_1h_pct")),
            color_pct(snap.get("change_4h_pct")),
            color_fr(snap.get("funding_rate_pct")),
            color_pct(snap.get("oi_change_1h_pct")),
            f"{snap['long_short_ratio']:.2f}" if snap.get("long_short_ratio") else "-",
            f"{ana['score']:.0f}",
            ana["verdict"],
        )
    return table


def find_new_hot_tokens(short_scores, long_scores,
                        short_top_n=10, long_threshold_rank=20) -> set:
    short_top = {s["token"] for s in short_scores[:short_top_n]}
    long_top_map = {s["token"]: i for i, s in enumerate(long_scores)}
    new_hot = set()
    for token in short_top:
        rank = long_top_map.get(token, 999)
        if rank >= long_threshold_rank:
            new_hot.add(token)
    return new_hot


def db_stats(conn):
    humans = conn.execute("SELECT COUNT(*) FROM authors WHERE is_human=1").fetchone()[0]
    posts = conn.execute("SELECT COUNT(*) FROM posts").fetchone()[0]
    recent = conn.execute(
        "SELECT COUNT(*) FROM posts WHERE posted_at > datetime('now', '-24 hours')"
    ).fetchone()[0]
    return humans, posts, recent


def analyze_market_for_tokens(short_scores: list[dict]) -> list[dict]:
    """对 15m 榜单里所有有永续合约的代币，拉取行情 + 综合打分
    返回: [{token, social_score, snap, analysis}, ...]
    """
    console.print("[blue]=> 查询币安合约数据...[/blue]")
    try:
        futures_set = get_futures_symbols()
    except Exception as e:
        console.print(f"[red]   获取合约列表失败: {e}[/red]")
        return []

    if not futures_set:
        console.print("[yellow]   合约列表为空（可能网络问题），跳过市场分析[/yellow]")
        return []

    # 过滤：只保留有永续合约的
    to_analyze = [s for s in short_scores if s["token"].upper() in futures_set]
    skipped = len(short_scores) - len(to_analyze)
    console.print(f"   15m 榜 {len(short_scores)} 个代币，其中 {len(to_analyze)} 个有 USDT 永续合约"
                  + (f"，跳过 {skipped} 个无合约的" if skipped else ""))

    results = []
    for i, s in enumerate(to_analyze, 1):
        token = s["token"]
        social_score = s["score"]
        try:
            snap = get_market_snapshot(token)
        except Exception as e:
            console.print(f"   [red][{i}/{len(to_analyze)}] {token} 抓取失败: {e}[/red]")
            continue
        if not snap:
            continue
        analysis = analyze_signals(snap, social_score)
        results.append({
            "token": token,
            "social_score": social_score,
            "snap": snap,
            "analysis": analysis,
        })
        # 节流：每秒 2 个，友好一点
        time.sleep(0.5)

    # 按综合分降序排
    results.sort(key=lambda r: r["analysis"]["score"], reverse=True)
    return results


async def process_round(scraper: SquareScraper):
    console.print(f"[blue]=> 开始抓取... {datetime.now():%H:%M:%S}[/blue]")
    try:
        posts, authors = await scraper.scrape_once()
    except Exception as e:
        console.print(f"[red]   Playwright 抓取异常: {e}[/red]")
        console.print("[yellow]   等待 30 秒后重试...[/yellow]")
        await asyncio.sleep(30)
        return  # Skip this round, retry next time
    console.print(f"   本轮捕获 {len(posts)} 条帖子，{len(authors)} 个作者")

    with storage.get_conn() as conn:
        human_count = 0
        for user_id, a in authors.items():
            a["is_human"] = 1 if is_likely_human(a) else 0
            a["post_count_24h"] = 0
            a["last_seen"] = _utcnow()
            storage.upsert_author(conn, a)
            if a["is_human"]:
                human_count += 1

        saved = 0
        token_hits = 0
        filtered_excluded = 0
        excluded = config.EXCLUDED_TOKENS or set()
        for post in posts:
            author = authors.get(post["user_id"])
            if not author or not author.get("is_human"):
                continue

            tokens = post.get("tokens") or set()
            if not tokens:
                tokens = extract_tokens_from_text(post.get("content", ""))

            if config.TRACKED_TOKENS:
                tokens = {t for t in tokens if t in config.TRACKED_TOKENS}

            before = len(tokens)
            tokens = {t for t in tokens if t not in excluded}
            filtered_excluded += before - len(tokens)

            post_for_db = {k: v for k, v in post.items() if k != "tokens"}
            storage.upsert_post(conn, post_for_db)
            if tokens:
                storage.insert_mentions(conn, post["post_id"], tokens)
                token_hits += len(tokens)
            saved += 1

        total_humans, total_posts, recent_posts = db_stats(conn)
        console.print(
            f"   本轮真人作者 {human_count}/{len(authors)}，入库帖子 {saved} 条，"
            f"代币提及 {token_hits} 个（黑名单过滤 {filtered_excluded} 个）"
        )
        console.print(
            f"   [dim]数据库累计：真人作者 {total_humans}, "
            f"帖子总数 {total_posts}, 近 24h 帖子 {recent_posts}[/dim]"
        )

        storage.purge_old(conn, days=7)

        short_scores = compute_short_scores(conn, max_post_age_hours=24)
        # Long window: reuse compute_short_scores but with wider effective window
        # (compute_short_scores uses config.SHORT_WINDOW_MINUTES internally)
        long_scores = compute_short_scores(conn, max_post_age_hours=24)
        new_hot = find_new_hot_tokens(short_scores, long_scores)

    # === 展示社交榜 ===
    if short_scores or long_scores:
        ts = datetime.now().strftime('%H:%M:%S')
        short_table = render_social_table(
            f"🔥 近 15 分钟 · {ts}", short_scores, config.TOP_N_SHORT, new_hot,
        )
        long_table = render_social_table(
            f"📊 近 24 小时 · {ts}", long_scores, config.TOP_N, new_hot,
        )
        console.print(Columns([short_table, long_table], equal=False, expand=False))

        if new_hot:
            console.print(
                f"[bold red]🔥 新冒头热点（15m 有但 24h 排名靠后）：{', '.join(sorted(new_hot))}[/bold red]"
            )

    # === 合约综合分析（隔离执行，失败不影响下一轮）===
    if config.ENABLE_MARKET_ANALYSIS and short_scores:
        try:
            market_rows = analyze_market_for_tokens(short_scores[:config.MARKET_ANALYSIS_MAX])
            if market_rows:
                console.print(render_market_table(market_rows))
                # 高综合分的提示一下解读
                notable = [r for r in market_rows if r["analysis"]["score"] >= 60 and r["analysis"]["notes"]]
                if notable:
                    console.print("\n[bold]🔍 关注代币解读：[/bold]")
                    for r in notable[:5]:
                        token = r["token"]
                        verdict = r["analysis"]["verdict"]
                        console.print(f"  [cyan]{token}[/cyan] {verdict}")
                        for note in r["analysis"]["notes"]:
                            console.print(f"    · {note}")
                console.print(
                    "\n[dim italic]⚠️ 以上仅是数据模式识别，不是投资建议。"
                    "综合分高 ≠ 一定上涨，市场永远可能反向。[/dim italic]"
                )
        except Exception as e:
            console.print(f"[yellow]  合约分析出错（已跳过）: {e}[/yellow]")


async def main():
    global _scraper
    storage.init_db()

    # 启动 Web API 服务（后台守护线程）
    web_thread = threading.Thread(target=start_web_server, daemon=True)
    web_thread.start()
    console.print(f"[green]Web API 已启动 http://{config.WEB_HOST}:{config.WEB_PORT}[/green]")

    scraper = SquareScraper()
    _scraper = scraper  # Wire up signal handler
    round_count = 0

    console.print("[green]币安广场监控启动[/green]")
    console.print(f"   每轮抓取：{config.SCRAPE_ROUND_SECONDS}s")
    console.print(f"   轮次间隔：{config.SCRAPE_INTERVAL_SECONDS}s")
    console.print(f"   粉丝阈值：{config.MIN_FOLLOWERS}")
    console.print(f"   排除代币：{len(config.EXCLUDED_TOKENS)} 个")
    if config.ENABLE_MARKET_ANALYSIS:
        console.print(f"   合约分析：开启（最多分析 15m 榜前 {config.MARKET_ANALYSIS_MAX} 个）")
    else:
        console.print(f"   合约分析：关闭")
    console.print()

    while _running:
        round_count += 1
        scraper.reset()  # Reset stop flag for new round
        try:
            await process_round(scraper)
        except Exception as e:
            console.print(f"[red]本轮出错：{e}[/red]")
            import traceback
            traceback.print_exc()
            # If scraper crashed, wait before retrying to avoid rapid restart loops
            if _running:
                console.print("[yellow]   等待 30 秒后重试...[/yellow]")
                for _ in range(30):
                    if not _running:
                        break
                    await asyncio.sleep(1)

        # === 每 N 轮清理历史缓存 ===
        if round_count % config.HEAT_HISTORY_PURGE_INTERVAL_ROUNDS == 0:
            try:
                with storage.get_conn() as conn:
                    storage.heat_history_purge_old(conn, keep_last_rounds=config.HEAT_HISTORY_KEEP_ROUNDS)
                    # 清理过期的 market_snapshots
                    stale_min = getattr(config, 'SNAPSHOT_STALE_MINUTES', 10)
                    conn.execute(
                        "DELETE FROM market_snapshots WHERE updated_at < datetime('now', ?)",
                        (f'-{stale_min} minutes',)
                    )
                    conn.execute(
                        "DELETE FROM market_realtime_cache WHERE updated_at < datetime('now', ?)",
                        (f'-{stale_min} minutes',)
                    )
                console.print(f"[dim]  🧹 已清理历史缓存（保留最近 {config.HEAT_HISTORY_KEEP_ROUNDS} 轮）[/dim]")
            except Exception as e:
                console.print(f"[yellow]  清理缓存出错: {e}[/yellow]")

        # === 通知 web.py 刷新 leaderboard 缓存 ===
        try:
            import urllib.request
            urllib.request.urlopen("http://127.0.0.1:8000/api/leaderboard", timeout=2)
        except Exception:
            pass  # web.py 可能没启动，忽略

        if not _running:
            break

        for _ in range(config.SCRAPE_INTERVAL_SECONDS):
            if not _running:
                break
            await asyncio.sleep(1)

    console.print("[green]已退出[/green]")


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        sys.exit(0)
