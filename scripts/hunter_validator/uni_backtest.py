#!/usr/bin/env python3
"""
UNIUSDT Hunter 回测分析 — 量化 LSR Bug 影响
对 UNIUSDT 最近 7 天的每 4h 数据进行 Hunter 评分，
比较当前逻辑 vs 修复后逻辑的 SHORT 方向评分差异。
"""

import json
import sys
import time
from datetime import datetime, timedelta, timezone

sys.path.insert(0, ".")
from api import BinanceAPI
from config import HunterConfig, DEFAULT_CONFIG
from scoring import (
    compute_atr, compute_position_score, compute_oi_smart_score,
    compute_smart_money_score, compute_short_position_score,
    compute_short_oi_smart_score, compute_short_smart_money_score,
    compute_wash_multiplier, compute_extreme_loss_multiplier,
    compute_oi_surge_score, compute_vol_oi_ratio_score,
    compute_funding_rate_score, clamp,
)

SYMBOL = "UNIUSDT"
DAYS = 7
INTERVAL_HOURS = 4


# ── 修复版: LSR > 2.0 应该给 SHORT 加分 ──
def fixed_short_smart_money_score(
    lsr_data: list[dict],
    klines_4h: list[dict],
    cfg: HunterConfig = DEFAULT_CONFIG,
) -> tuple[float, list[str]]:
    """修复版 compute_short_smart_money_score:
    原始 (scoring.py:585-587): newest_ratio > 2.0 → score -= 10
    修复: newest_ratio > 2.0 → score += 15
    理由: LSR > 2.0 = 头部交易员做多比例 > 67% = 拥挤多头 → 利好做空
    """
    score = 0.0
    tags = []

    if len(lsr_data) >= 2:
        oldest_ratio = lsr_data[0]["longShortRatio"]
        newest_ratio = lsr_data[-1]["longShortRatio"]

        if oldest_ratio > 0:
            lsr_delta_pct = ((newest_ratio - oldest_ratio) / oldest_ratio) * 100

            if oldest_ratio > 1.1 and newest_ratio < oldest_ratio:
                score += 20; tags.append("lsr_bearish_reversal")
            if lsr_delta_pct < -cfg.lsr_delta_threshold:
                score += 10; tags.append("lsr_bearish_strong")
            if lsr_delta_pct > cfg.lsr_delta_threshold:
                score += 5; tags.append("lsr_bullish_weak")
            if newest_ratio < 0.5:
                score += 15; tags.append("lsr_extreme_bullish_short")

            # FIX: LSR > 2.0 = crowded longs = FAVOR SHORT
            if newest_ratio > 2.0:
                score += 15  # WAS: score -= 10
                tags.append("lsr_crowded_long_favor_short")  # WAS: lsr_extreme_bearish_penalize

    # Taker (same as original)
    if len(klines_4h) >= 5:
        latest = klines_4h[-1]
        if latest["volume"] > 0:
            taker_ratio = latest["taker_buy_volume"] / latest["volume"]
            if taker_ratio < 0.40:
                score += 10; tags.append("taker_sell_strong")
        ratios = []
        for k in klines_4h[-4:]:
            if k["volume"] > 0:
                ratios.append(k["taker_buy_volume"] / k["volume"])
        if len(ratios) >= 3 and ratios[-1] < ratios[0]:
            score += 10; tags.append("taker_trending_down")
        if len(ratios) >= 3:
            strong_bars = sum(1 for r in ratios if r < 0.45)
            if strong_bars >= 3:
                score += 20; tags.append("taker_sustained_selling")
        if len(ratios) >= 4 and ratios[0] > 0.55 and ratios[-1] < 0.45:
            score += 10; tags.append("taker_reversal_short")

    return clamp(score, 0.0, 65.0), tags


def run_analysis():
    api = BinanceAPI(cache_ttl=300)
    cfg = DEFAULT_CONFIG

    print("=" * 72)
    print(f"UNIUSDT Hunter Scoring — {DAYS}d × {24//INTERVAL_HOURS}pts/day")
    print("=" * 72)

    # Get current ticker
    tickers = api.get_usdt_perp_tickers()
    uni_ticker = next((t for t in tickers if t["symbol"] == SYMBOL), None)
    if not uni_ticker:
        print(f"ERROR: {SYMBOL} not found"); return
    pct24h = float(uni_ticker.get("priceChangePercent", 0))
    print(f"Current: {SYMBOL} @ {uni_ticker['lastPrice']} ({pct24h:+.2f}%)")

    now_ms = int(time.time() * 1000)
    results = []
    step_ms = INTERVAL_HOURS * 3600 * 1000
    total = DAYS * 24 // INTERVAL_HOURS

    print(f"\nRunning {total} checkpoints...\n")

    for i in range(total):
        t_ms = now_ms - (i + 1) * step_ms
        dt = datetime.fromtimestamp(t_ms / 1000, tz=timezone(timedelta(hours=8)))
        pct = (i + 1) / total * 100
        print(f"\r  [{pct:5.1f}%] {dt.strftime('%m-%d %H:%M')} CST", end="", flush=True)

        try:
            # Fetch multi-TF klines
            k4h = api.get_klines(SYMBOL, "4h", 20, end_time=t_ms)
            if len(k4h) < 15:
                continue
            k1d = api.get_klines(SYMBOL, "1d", 20, end_time=t_ms)
            k1h = api.get_klines(SYMBOL, "1h", 20, end_time=t_ms)
            k15m = api.get_klines(SYMBOL, "15m", 20, end_time=t_ms)
            k5m = api.get_klines(SYMBOL, "5m", 20, end_time=t_ms)

            price = k4h[-1]["close"]
            price_chg_pct = float(uni_ticker.get("priceChangePercent", 0))

            # OI proxy
            latest = k4h[-1]
            prev = k4h[-2]
            oi_val = latest["volume"] * latest["close"]
            oi_delta = ((latest["volume"] - prev["volume"]) / prev["volume"] * 100) if prev["volume"] > 0 else 0

            # Forward returns (SHORT = negative of price movement)
            fwd_1h_k = api.get_klines(SYMBOL, "1h", 1, end_time=t_ms + 3600000)
            fwd_4h_k = api.get_klines(SYMBOL, "1h", 1, end_time=t_ms + 4*3600000)
            fwd_1h = ((fwd_1h_k[-1]["close"] - price) / price * 100) if fwd_1h_k else 0
            fwd_4h = ((fwd_4h_k[-1]["close"] - price) / price * 100) if fwd_4h_k else 0

            # ===== SHORT SCORING =====
            # Position scores
            l_pos, l_pos_tags = compute_position_score(k4h, price_chg_pct, cfg)
            s_pos, s_pos_tags = compute_short_position_score(k4h, price_chg_pct, cfg, k1d, k1h, k15m, k5m)

            # OI
            s_oi, s_oi_tags = compute_short_oi_smart_score(oi_delta, oi_val, price_chg_pct, cfg)

            # Smart Money (CURRENT - buggy)
            s_sm, s_sm_tags = compute_short_smart_money_score([], k4h, cfg)

            # Smart Money (FIXED)
            s_sm_fix, s_sm_tags_fix = fixed_short_smart_money_score([], k4h, cfg)

            # Composite (current)
            base50 = clamp((s_pos + s_oi) / 2, -35, 50)
            base25 = s_sm * 0.65
            s_comp = clamp(base50 + base25, 0, 75)

            # Composite (fixed)
            base50_f = clamp((s_pos + s_oi) / 2, -35, 50)
            base25_f = s_sm_fix * 0.65
            s_comp_fix = clamp(base50_f + base25_f, 0, 75)

            # ===== LONG SCORING =====
            # l_pos already computed above
            l_oi, l_oi_tags = compute_oi_smart_score(oi_delta, oi_val, price_chg_pct, cfg)
            l_sm, l_sm_tags = compute_smart_money_score([], k4h, cfg)
            l_base50 = clamp((l_pos + l_oi) / 2, -35, 50)
            l_base25 = l_sm * 0.65
            l_comp = clamp(l_base50 + l_base25, 0, 75)

            # Direction picks
            dir_cur = "SHORT" if s_comp > l_comp else "LONG"
            dir_fix = "SHORT" if s_comp_fix > l_comp else "LONG"

            results.append({
                "ts": t_ms, "time": dt.strftime("%m-%d %H:%M"),
                "price": price,
                "s_pos": s_pos, "s_oi": s_oi,
                "s_sm": s_sm, "s_sm_fix": s_sm_fix,
                "s_comp": s_comp, "s_comp_fix": s_comp_fix,
                "l_comp": l_comp,
                "dir_cur": dir_cur, "dir_fix": dir_fix,
                "fwd_1h": fwd_1h, "fwd_4h": fwd_4h,
                "short_ret_1h": -fwd_1h, "short_ret_4h": -fwd_4h,
                "s_pos_tags": s_pos_tags, "s_oi_tags": s_oi_tags,
                "s_sm_tags": s_sm_tags, "s_sm_tags_fix": s_sm_tags_fix,
            })

            time.sleep(0.3)
        except Exception as e:
            print(f" [err:{e}]", end="")
            continue

    print(f"\n\n{'='*72}\nRESULTS: {len(results)} checkpoints\n{'='*72}")
    if not results:
        print("No results!"); return

    # ── 1. Score Comparison ──
    print(f"\n{'─'*72}")
    print("1. SHORT SCORE COMPARISON (Current vs Fixed)")
    print(f"{'─'*72}")
    cur_avg = sum(r["s_comp"] for r in results) / len(results)
    fix_avg = sum(r["s_comp_fix"] for r in results) / len(results)
    l_avg = sum(r["l_comp"] for r in results) / len(results)
    print(f"  Current SHORT avg: {cur_avg:.1f}")
    print(f"  Fixed SHORT avg:   {fix_avg:.1f}")
    print(f"  LONG avg:          {l_avg:.1f}")

    # ── 2. Direction changes ──
    changes = sum(1 for r in results if r["dir_cur"] != r["dir_fix"])
    print(f"\n  Direction changes: {changes}/{len(results)}")
    print(f"  Current→SHORT: {sum(1 for r in results if r['dir_cur']=='SHORT')}/{len(results)}")
    print(f"  Fixed→SHORT:   {sum(1 for r in results if r['dir_fix']=='SHORT')}/{len(results)}")

    # ── 3. Detail ──
    print(f"\n{'─'*72}")
    print("2. PER-CHECKPOINT")
    print(f"{'─'*72}")
    print(f"  {'Time':<12}{'Price':>7}{'S_pos':>6}{'S_oi':>5}{'S_sm':>5}{'S_fix':>6}{'L':>6}{'Dir':>5}{'Fix':>5}{'1h%':>7}{'4h%':>7}")
    for r in results:
        print(f"  {r['time']:<12}{r['price']:>7.4f}{r['s_pos']:>6.0f}{r['s_oi']:>5.0f}"
              f"{r['s_sm']:>5.0f}{r['s_sm_fix']:>6.0f}{r['l_comp']:>6.1f}"
              f"{r['dir_cur']:>5}{r['dir_fix']:>5}{r['fwd_1h']:>+6.2f}%{r['fwd_4h']:>+6.2f}%")

    # ── 4. Win Rate ──
    print(f"\n{'─'*72}")
    print("3. SHORT PICK WIN RATE")
    print(f"{'─'*72}")
    for label, key in [("Current", "dir_cur"), ("Fixed", "dir_fix")]:
        picks = [r for r in results if r[key] == "SHORT"]
        if not picks:
            print(f"  {label}: 0 SHORT picks"); continue
        w1 = sum(1 for r in picks if r["short_ret_1h"] > 0)
        w4 = sum(1 for r in picks if r["short_ret_4h"] > 0)
        a1 = sum(r["short_ret_1h"] for r in picks) / len(picks)
        a4 = sum(r["short_ret_4h"] for r in picks) / len(picks)
        print(f"  {label} SHORT ({len(picks)} picks):")
        print(f"    1h: {w1}/{len(picks)} = {w1/len(picks)*100:.1f}% | avg {a1:+.2f}%")
        print(f"    4h: {w4}/{len(picks)} = {w4/len(picks)*100:.1f}% | avg {a4:+.2f}%")

    # ── 5. LSR Impact ──
    print(f"\n{'─'*72}")
    print("4. LSR BUG IMPACT")
    print(f"{'─'*72}")
    diffs = [r["s_sm_fix"] - r["s_sm"] for r in results]
    print(f"  Avg SM score diff: {sum(diffs)/len(diffs):+.1f}")
    print(f"  Max diff: {max(diffs):+.0f}")
    print(f"  Improved: {sum(1 for d in diffs if d > 0)}/{len(diffs)}")
    print(f"  Unchanged: {sum(1 for d in diffs if d == 0)}/{len(diffs)}")

    # ── 6. Direction-changing checkpoints ──
    print(f"\n{'─'*72}")
    print("5. DIRECTION CHANGES (where fix changes the pick)")
    print(f"{'─'*72}")
    for r in results:
        if r["dir_cur"] != r["dir_fix"]:
            # Was the fixed direction better?
            actual_1h = r["fwd_1h"]
            if r["dir_fix"] == "LONG":
                fix_ret = actual_1h  # LONG = bet on price up
                cur_ret = -actual_1h  # SHORT = bet on price down
            else:
                fix_ret = -actual_1h
                cur_ret = actual_1h
            better = "✅ BETTER" if fix_ret > cur_ret else "❌ WORSE"
            print(f"  {r['time']} | P:{r['price']:.4f} | "
                  f"{r['dir_cur']}({r['s_comp']:.1f})→{r['dir_fix']}({r['s_comp_fix']:.1f}) | "
                  f"Actual 1h:{actual_1h:+.2f}% | {better}")

    # ── 7. Event timeline around entry ──
    print(f"\n{'─'*72}")
    print("6. ENTRY WINDOW (around 2026-05-23 09:00-10:00 CST)")
    print(f"{'─'*72}")
    for r in results:
        if "05-23 0" in r["time"] or "05-23 1" in r["time"]:
            tags_str = ", ".join(r["s_sm_tags"][:3])
            fix_tags_str = ", ".join(r["s_sm_tags_fix"][:3])
            print(f"  {r['time']} | P:{r['price']:.4f} | SHORT:{r['s_comp']:.1f}(fix:{r['s_comp_fix']:.1f}) | "
                  f"LONG:{r['l_comp']:.1f} | Dir:{r['dir_cur']}(fix:{r['dir_fix']}) | "
                  f"1h:{r['fwd_1h']:+.2f}% | SM:{tags_str}")

    # Save
    out = "uni_hunter_analysis.json"
    with open(out, "w") as f:
        json.dump({"symbol": SYMBOL, "days": DAYS, "interval_h": INTERVAL_HOURS,
                    "n": len(results), "results": results,
                    "summary": {"cur_avg": cur_avg, "fix_avg": fix_avg, "l_avg": l_avg,
                                "changes": changes, "diff_avg": sum(diffs)/len(diffs)}},
                   f, indent=2, default=str)
    print(f"\n  Saved: {out}")


if __name__ == "__main__":
    run_analysis()
