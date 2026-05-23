"""
Hunter Validator - 报告生成
控制台彩色输出 + JSON 导出。
"""

import json
import os
from datetime import datetime, timezone
from typing import Optional


# ── 颜色工具 ──

class C:
    """ANSI 颜色。"""
    RESET = "\033[0m"
    BOLD = "\033[1m"
    DIM = "\033[2m"
    RED = "\033[31m"
    GREEN = "\033[32m"
    YELLOW = "\033[33m"
    BLUE = "\033[34m"
    CYAN = "\033[36m"
    MAGENTA = "\033[35m"

    @staticmethod
    def score_color(val: float, max_val: float = 75) -> str:
        ratio = val / max_val if max_val > 0 else 0
        if ratio >= 0.6:
            return C.GREEN
        elif ratio >= 0.3:
            return C.YELLOW
        return C.RED


def fmt_score(val: float, width: int = 6) -> str:
    color = C.score_color(val)
    return f"{color}{val:>{width}.1f}{C.RESET}"


def fmt_pct(val: float, width: int = 7) -> str:
    color = C.GREEN if val > 0 else (C.RED if val < 0 else C.DIM)
    return f"{color}{val:>+{width}.2f}%{C.RESET}"


# ── 实时报告 ──

def print_live_report(scored_coins: list, config=None):
    """打印实时 Hunter 选币报告。"""
    now = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")

    print(f"\n{'='*80}")
    print(f"{C.BOLD}{C.CYAN}  Hunter 实时验证  {C.RESET}")
    print(f"  时间: {now}")
    if config:
        print(f"  配置: default (与 Go 代码一致)")
    print(f"{'='*80}\n")

    # 表头
    print(f"  {'排名':>4}  {'标的':<14} {'综合分':>6}  "
          f"{'位置':>5} {'OI':>5} {'聪明钱':>6}  "
          f"{'冷却':>4} {'刷量':>5}  {'最终分':>6}  "
          f"{'24h%':>7}  {'标签'}")
    print(f"  {'─'*4}  {'─'*14} {'─'*6}  "
          f"{'─'*5} {'─'*5} {'─'*6}  "
          f"{'─'*4} {'─'*5}  {'─'*6}  "
          f"{'─'*7}  {'─'*30}")

    for i, sc in enumerate(scored_coins[:10], 1):
        s = sc.score
        pct24h = float(sc.ticker.get("priceChangePercent", 0))
        tags_str = ", ".join(s.tags[:5]) if s.tags else "-"

        composite = (s.position_score + s.oi_smart_score) / 2
        composite = max(0, min(50, composite))

        print(f"  {i:>4}  {sc.symbol:<14} {fmt_score(composite)}  "
              f"{fmt_score(s.position_score, 5)} {fmt_score(s.oi_smart_score, 5)} "
              f"{fmt_score(s.smart_money_score, 6)}  "
              f"{s.cooldown_mod:>4.1f} {s.wash_mod:>5.2f}  "
              f"{fmt_score(s.final_score, 6)}  "
              f"{fmt_pct(pct24h)}  {C.DIM}{tags_str}{C.RESET}")

    # 详细信号分解
    print(f"\n{'─'*80}")
    print(f"{C.BOLD}  信号分解 (Top 5){C.RESET}")
    print(f"{'─'*80}")

    for i, sc in enumerate(scored_coins[:5], 1):
        s = sc.score
        print(f"\n  {C.BOLD}#{i} {sc.symbol}{C.RESET} (最终分: {s.final_score:.1f})")

        # 位置分
        dist_to_support = s.current_price - s.low20 if s.low20 > 0 else 0
        dist_to_resist = s.high20 - s.current_price if s.high20 > 0 else 0
        atr_dist = dist_to_support / s.atr if s.atr > 0 else 0
        print(f"    位置分: {C.score_color(s.position_score, 30)}{s.position_score:+.0f}{C.RESET}"
              f" (ATR={s.atr:.6f}, 距支撑={atr_dist:.1f}×ATR, "
              f"距阻力={dist_to_resist/s.atr:.1f}×ATR)" if s.atr > 0 else
              f"    位置分: {s.position_score:+.0f} (无 ATR 数据)")

        # OI 分
        print(f"    OI 分: {C.score_color(s.oi_smart_score, 50)}{s.oi_smart_score:+.0f}{C.RESET}"
              f" (OI市值=${s.oi_value:,.0f}, 4h变动={s.oi_delta_4h:+.1f}%)")

        # 聪明钱分
        lsr_str = f"LSR: {s.lsr_oldest:.3f}→{s.lsr_newest:.3f}" if s.lsr_oldest > 0 else "LSR: 无数据"
        print(f"    聪明钱: {C.score_color(s.smart_money_score, 50)}{s.smart_money_score:+.0f}{C.RESET}"
              f" ({lsr_str}, Taker买入={s.taker_buy_ratio:.1%})")

        # 刷量
        wash_str = "正常" if s.wash_mod >= 0.99 else f"×{s.wash_mod:.2f}"
        print(f"    刷量: {wash_str} {s.wash_details}")

        # 标签
        if s.tags:
            print(f"    标签: {', '.join(s.tags)}")

    print()


# ── 回测报告 ──

def print_backtest_report(results: dict):
    """打印回测结果。"""
    print(f"\n{'='*80}")
    print(f"{C.BOLD}{C.CYAN}  Hunter 回测报告  {C.RESET}")
    print(f"{'='*80}\n")

    hunter = results.get("hunter", {})
    random_bl = results.get("random_baseline", {})
    volume_bl = results.get("volume_baseline", {})

    # 总览
    print(f"  检查点数: {results.get('checkpoints', 0)}")
    print(f"  总选币数: {results.get('total_picks', 0)}")
    print(f"  回测天数: {results.get('days', 0)}")
    print()

    # 对比表
    headers = ["指标", "Hunter", "随机基线", "成交量基线", "Alpha"]
    print(f"  {headers[0]:<18} {headers[1]:>10} {headers[2]:>10} {headers[3]:>10} {headers[4]:>10}")
    print(f"  {'─'*18} {'─'*10} {'─'*10} {'─'*10} {'─'*10}")

    metrics = [
        ("1h Hit Rate", "hit_rate_1h"),
        ("4h Hit Rate", "hit_rate_4h"),
        ("24h Hit Rate", "hit_rate_24h"),
        ("平均 1h 收益", "avg_return_1h"),
        ("平均 4h 收益", "avg_return_4h"),
        ("平均 24h 收益", "avg_return_24h"),
        ("Sharpe (1h)", "sharpe_1h"),
        ("方向准确率", "directional_accuracy"),
    ]

    for label, key in metrics:
        h_val = hunter.get(key, 0)
        r_val = random_bl.get(key, 0)
        v_val = volume_bl.get(key, 0)
        alpha = h_val - r_val

        is_pct = "Rate" in label or "准确率" in label
        is_return = "收益" in label

        if is_pct:
            print(f"  {label:<18} {h_val:>9.1%} {r_val:>9.1%} {v_val:>9.1%} {alpha:>+9.1%}")
        elif is_return:
            print(f"  {label:<18} {h_val:>+9.3f}% {r_val:>+9.3f}% {v_val:>+9.3f}% {alpha:>+9.3f}%")
        else:
            print(f"  {label:<18} {h_val:>10.3f} {r_val:>10.3f} {v_val:>10.3f} {alpha:>+10.3f}")

    # 统计显著性
    sig = results.get("significance", {})
    if sig:
        print(f"\n  {C.BOLD}统计检验{C.RESET}")
        print(f"  t-stat (1h收益): {sig.get('t_stat', 0):.3f}, p-value: {sig.get('p_value', 0):.4f}")
        print(f"  {'显著 ✓' if sig.get('p_value', 1) < 0.05 else '不显著 ✗'} (α=0.05)")

    # Top 10 最频繁选中的币
    freq = results.get("selection_frequency", {})
    if freq:
        print(f"\n  {C.BOLD}最频繁选中 (Top 10){C.RESET}")
        sorted_freq = sorted(freq.items(), key=lambda x: x[1], reverse=True)[:10]
        for sym, count in sorted_freq:
            bar = "█" * min(count, 40)
            print(f"    {sym:<14} {count:>3}次 {C.CYAN}{bar}{C.RESET}")

    print()


# ── 优化报告 ──

def print_optimize_report(results: dict):
    """打印权重优化结果。"""
    print(f"\n{'='*80}")
    print(f"{C.BOLD}{C.CYAN}  权重优化报告  {C.RESET}")
    print(f"{'='*80}\n")

    baseline = results.get("baseline", {})
    best = results.get("best", {})
    improvements = results.get("improvements", [])

    print(f"  {C.BOLD}基线 → 最优 对比{C.RESET}\n")
    print(f"  {'参数':<28} {'基线':>10} {'最优':>10} {'变化':>10}")
    print(f"  {'─'*28} {'─'*10} {'─'*10} {'─'*10}")

    for imp in improvements:
        param = imp["param"]
        old = imp["old"]
        new = imp["new"]
        delta = new - old
        color = C.GREEN if delta > 0 else (C.RED if delta < 0 else C.DIM)
        print(f"  {param:<28} {old:>10.1f} {new:>10.1f} {color}{delta:>+10.1f}{C.RESET}")

    # 表现提升
    perf = results.get("performance_delta", {})
    if perf:
        print(f"\n  {C.BOLD}表现提升{C.RESET}")
        for metric, delta in perf.items():
            color = C.GREEN if delta > 0 else C.RED
            print(f"    {metric}: {color}{delta:+.3f}{C.RESET}")

    # 建议
    recs = results.get("recommendations", [])
    if recs:
        print(f"\n  {C.BOLD}优化建议{C.RESET}")
        for i, rec in enumerate(recs, 1):
            print(f"    {i}. {rec}")

    print()


def save_report(results: dict, path: str):
    """保存报告为 JSON。"""
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    with open(path, 'w') as f:
        json.dump(results, f, indent=2, ensure_ascii=False, default=str)
    print(f"  报告已保存: {path}")
