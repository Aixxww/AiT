#!/usr/bin/env python3
"""
Hunter Sniper Strategy — 回测验证
基于 R2-R6 信号数据，验证"精准猎杀"策略的入场/出场规则。

核心理念: 不追求覆盖面，只做高确定性交易。
- 只交易 Tier 1/2 强信号
- ATR 自适应止损止盈
- 严格 4h 时间窗口
- 3:1 风险回报比
"""

import json
import os
import sys
from datetime import datetime, timezone
from dataclasses import dataclass, field
from typing import Optional

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from api import BinanceAPI
from config import HunterConfig, DEFAULT_CONFIG
from scoring import score_coin, compute_atr, clamp

# ── 策略参数 ──

@dataclass
class SniperConfig:
    """Hunter Sniper 策略参数"""
    # 入场: 信号分级
    tier1_signals: set = field(default_factory=lambda: {
        "oi_short_squeeze",   # OI↓>10% + price↑ → +45
        "lsr_reversal",       # LSR 从空翻多 → +20
        "oi_accumulation",    # OI↑ + price↓ (主力吸筹) → +40
    })
    tier2_signals: set = field(default_factory=lambda: {
        "oi_price_aligned",   # OI↑ + price↑ → +40
        "lsr_bullish",        # LSR delta > +10% → +10
        "oi_squeeze_moderate", # OI↓>5% + price↑ → +20
    })

    # 入场: 最低确认分
    min_confirmation_score: int = 2  # near_support 需 ≥2 才可入场

    # 止损止盈
    atr_sl_multiplier: float = 1.0   # SL = 入场价 - 1×ATR(4h)
    atr_tp_multiplier: float = 3.0   # TP = 入场价 + 3×ATR(4h)
    min_risk_reward: float = 3.0     # 最低风险回报比

    # 时间窗口
    max_hold_bars: int = 4           # 最大持仓 4 根 4h K 线 (16小时)
    check_interval: str = "4h"       # 检查间隔

    # 仓位管理
    max_positions: int = 2           # 最多同时持仓 2 个
    position_pct: float = 0.5        # 每笔仓位 = 总资金 × 50%

    # 杠杆
    leverage: int = 3                # 3x 杠杆


# ── 回测引擎 ──

@dataclass
class TradeResult:
    """单笔交易结果"""
    symbol: str
    signal: str
    signal_score: float
    entry_price: float
    stop_loss: float
    take_profit: float
    exit_price: float
    exit_reason: str  # "tp", "sl", "timeout", "signal_reverse"
    pnl_pct: float
    hold_bars: int
    timestamp: str


def run_sniper_backtest(
    days: int = 7,
    interval_hours: int = 4,
    top_k: int = 5,
    cfg: HunterConfig = DEFAULT_CONFIG,
    sniper_cfg: SniperConfig = SniperConfig(),
) -> dict:
    """
    运行 Hunter Sniper 策略回测。

    流程:
    1. 在每个检查点，用 Hunter 评分选出 Top-K
    2. 只交易携带 Tier 1/2 信号的标的
    3. 用 ATR(4h) 计算 SL/TP
    4. 在后续 K 线中模拟持仓，检查 SL/TP 触发
    5. 统计胜率、均收、盈亏比
    """
    api = BinanceAPI(cache_ttl=3600)

    print(f"\n{'='*80}")
    print(f"  Hunter Sniper Strategy 回测")
    print(f"  {days}天 / {interval_hours}h间隔 / Top-{top_k}")
    print(f"  SL: 1×ATR / TP: 3×ATR / 杠杆: {sniper_cfg.leverage}x")
    print(f"{'='*80}\n")

    # 获取候选池
    tickers = api.get_usdt_perp_tickers()
    print(f"  候选池: {len(tickers)} 个合约")

    # 生成检查点时间戳
    now_ms = int(datetime.now(timezone.utc).timestamp() * 1000)
    interval_ms = interval_hours * 3600 * 1000
    checkpoints = [now_ms - (i + 1) * interval_ms for i in range(days * 24 // interval_hours)]
    checkpoints.reverse()  # 从早到晚

    all_trades: list[TradeResult] = []
    active_positions: dict = {}  # symbol → trade info

    for cp_idx, cp_ms in enumerate(checkpoints):
        cp_time = datetime.fromtimestamp(cp_ms / 1000, tz=timezone.utc)
        print(f"\r  检查点 {cp_idx+1}/{len(checkpoints)}: {cp_time.strftime('%m-%d %H:%M')}", end="", flush=True)

        # 检查活跃持仓: SL/TP/超时
        closed_symbols = []
        for sym, pos in list(active_positions.items()):
            bars_held = cp_idx - pos["entry_cp_idx"]
            try:
                # 获取当前价格
                klines = api.get_klines(sym, "4h", 5)
                if not klines:
                    continue
                current_price = klines[-1]["close"]

                # 检查 SL
                if current_price <= pos["stop_loss"]:
                    pnl = ((pos["stop_loss"] - pos["entry_price"]) / pos["entry_price"]) * sniper_cfg.leverage * 100
                    all_trades.append(TradeResult(
                        symbol=sym, signal=pos["signal"], signal_score=pos["signal_score"],
                        entry_price=pos["entry_price"], stop_loss=pos["stop_loss"],
                        take_profit=pos["take_profit"], exit_price=pos["stop_loss"],
                        exit_reason="sl", pnl_pct=pnl, hold_bars=bars_held,
                        timestamp=cp_time.isoformat(),
                    ))
                    closed_symbols.append(sym)
                    continue

                # 检查 TP
                if current_price >= pos["take_profit"]:
                    pnl = ((pos["take_profit"] - pos["entry_price"]) / pos["entry_price"]) * sniper_cfg.leverage * 100
                    all_trades.append(TradeResult(
                        symbol=sym, signal=pos["signal"], signal_score=pos["signal_score"],
                        entry_price=pos["entry_price"], stop_loss=pos["stop_loss"],
                        take_profit=pos["take_profit"], exit_price=pos["take_profit"],
                        exit_reason="tp", pnl_pct=pnl, hold_bars=bars_held,
                        timestamp=cp_time.isoformat(),
                    ))
                    closed_symbols.append(sym)
                    continue

                # 检查超时
                if bars_held >= sniper_cfg.max_hold_bars:
                    pnl = ((current_price - pos["entry_price"]) / pos["entry_price"]) * sniper_cfg.leverage * 100
                    all_trades.append(TradeResult(
                        symbol=sym, signal=pos["signal"], signal_score=pos["signal_score"],
                        entry_price=pos["entry_price"], stop_loss=pos["stop_loss"],
                        take_profit=pos["take_profit"], exit_price=current_price,
                        exit_reason="timeout", pnl_pct=pnl, hold_bars=bars_held,
                        timestamp=cp_time.isoformat(),
                    ))
                    closed_symbols.append(sym)
                    continue

            except Exception:
                continue

        for sym in closed_symbols:
            del active_positions[sym]

        # 如果仓位已满，跳过新开仓
        if len(active_positions) >= sniper_cfg.max_positions:
            continue

        # Hunter 评分
        scored = []
        for t in tickers[:50]:
            symbol = t["symbol"]
            if symbol in active_positions:
                continue
            if not api.is_usdt_perp(symbol):
                continue

            try:
                klines_4h = api.get_klines(symbol, "4h", cfg.kline_bars, end_time=cp_ms)
                if len(klines_4h) < 15:
                    continue

                # OI
                oi_value = 0.0
                oi_delta = 0.0
                try:
                    oi_data = api.get_oi_history(symbol, "4h", 2, end_time=cp_ms)
                    if len(oi_data) >= 2:
                        oi_value = oi_data[-1]["sumOpenInterestValue"]
                        prev_oi = oi_data[-2]["sumOpenInterestValue"]
                        if prev_oi > 0:
                            oi_delta = ((oi_data[-1]["sumOpenInterest"] - oi_data[-2]["sumOpenInterest"])
                                        / oi_data[-2]["sumOpenInterest"]) * 100
                except Exception:
                    pass

                # LSR
                lsr_data = []
                try:
                    lsr_data = api.get_lsr_history(symbol, cfg.lsr_period, cfg.lsr_limit, end_time=cp_ms)
                except Exception:
                    pass

                result = score_coin(
                    symbol=symbol, ticker=t, klines_4h=klines_4h,
                    lsr_data=lsr_data, oi_delta_4h=oi_delta,
                    oi_value_usd=oi_value, cfg=cfg,
                )

                if result.score.final_score > 0:
                    scored.append(result)
            except Exception:
                continue

        # 排序取 Top-K
        scored.sort(key=lambda x: x.score.final_score, reverse=True)

        # Sniper 过滤: 只交易 Tier 1/2 信号
        for sc in scored[:top_k]:
            if len(active_positions) >= sniper_cfg.max_positions:
                break

            # 检查信号等级
            tags = sc.score.tags
            signal = None
            signal_score = 0

            for t in tags:
                if t in sniper_cfg.tier1_signals:
                    signal = t
                    signal_score = 3
                    break
                elif t in sniper_cfg.tier2_signals:
                    signal = t
                    signal_score = 2

            if signal is None:
                continue  # 无 Tier 1/2 信号，跳过

            # 计算 SL/TP
            atr = sc.score.atr
            if atr <= 0:
                continue

            entry_price = sc.score.current_price
            sl = entry_price - sniper_cfg.atr_sl_multiplier * atr
            tp = entry_price + sniper_cfg.atr_tp_multiplier * atr

            # 验证 R:R
            risk = entry_price - sl
            reward = tp - entry_price
            if risk <= 0 or reward / risk < sniper_cfg.min_risk_reward:
                continue

            # 开仓
            active_positions[sc.symbol] = {
                "entry_price": entry_price,
                "stop_loss": sl,
                "take_profit": tp,
                "signal": signal,
                "signal_score": signal_score,
                "entry_cp_idx": cp_idx,
            }

    print(f"\r  回测完成: {len(all_trades)} 笔交易" + " " * 30)

    # 统计
    if not all_trades:
        print("  ⚠️  无交易产生")
        return {"trades": 0}

    wins = [t for t in all_trades if t.pnl_pct > 0]
    losses = [t for t in all_trades if t.pnl_pct <= 0]

    total_pnl = sum(t.pnl_pct for t in all_trades)
    avg_pnl = total_pnl / len(all_trades)
    win_rate = len(wins) / len(all_trades)
    avg_win = sum(t.pnl_pct for t in wins) / len(wins) if wins else 0
    avg_loss = sum(t.pnl_pct for t in losses) / len(losses) if losses else 0
    pf = abs(sum(t.pnl_pct for t in wins) / sum(t.pnl_pct for t in losses)) if losses and sum(t.pnl_pct for t in losses) != 0 else float('inf')

    # 按信号类型统计
    signal_stats = {}
    for t in all_trades:
        if t.signal not in signal_stats:
            signal_stats[t.signal] = {"count": 0, "wins": 0, "total_pnl": 0}
        signal_stats[t.signal]["count"] += 1
        signal_stats[t.signal]["total_pnl"] += t.pnl_pct
        if t.pnl_pct > 0:
            signal_stats[t.signal]["wins"] += 1

    # 按出场原因统计
    exit_stats = {}
    for t in all_trades:
        if t.exit_reason not in exit_stats:
            exit_stats[t.exit_reason] = {"count": 0, "total_pnl": 0}
        exit_stats[t.exit_reason]["count"] += 1
        exit_stats[t.exit_reason]["total_pnl"] += t.pnl_pct

    results = {
        "trades": len(all_trades),
        "wins": len(wins),
        "losses": len(losses),
        "win_rate": win_rate,
        "avg_pnl_pct": avg_pnl,
        "total_pnl_pct": total_pnl,
        "avg_win_pct": avg_win,
        "avg_loss_pct": avg_loss,
        "profit_factor": pf,
        "max_win": max(t.pnl_pct for t in all_trades),
        "max_loss": min(t.pnl_pct for t in all_trades),
        "avg_hold_bars": sum(t.hold_bars for t in all_trades) / len(all_trades),
        "signal_stats": {k: {**v, "win_rate": v["wins"]/v["count"] if v["count"] > 0 else 0}
                         for k, v in signal_stats.items()},
        "exit_stats": exit_stats,
        "config": {
            "sl_atr_mult": sniper_cfg.atr_sl_multiplier,
            "tp_atr_mult": sniper_cfg.atr_tp_multiplier,
            "leverage": sniper_cfg.leverage,
            "max_hold_bars": sniper_cfg.max_hold_bars,
            "max_positions": sniper_cfg.max_positions,
        },
        "raw_trades": [
            {
                "symbol": t.symbol, "signal": t.signal,
                "entry": t.entry_price, "sl": t.stop_loss, "tp": t.take_profit,
                "exit": t.exit_price, "reason": t.exit_reason,
                "pnl_pct": round(t.pnl_pct, 2), "hold_bars": t.hold_bars,
                "time": t.timestamp,
            }
            for t in all_trades
        ],
    }

    # 打印报告
    print(f"\n{'='*80}")
    print(f"  Hunter Sniper Strategy 回测结果")
    print(f"{'='*80}")
    print(f"\n  总交易: {len(all_trades)}  胜: {len(wins)}  负: {len(losses)}")
    print(f"  胜率: {win_rate:.1%}")
    print(f"  均收: {avg_pnl:+.2f}%  总收: {total_pnl:+.2f}%")
    print(f"  盈亏比: {pf:.2f}")
    print(f"  均赢: {avg_win:+.2f}%  均亏: {avg_loss:+.2f}%")
    print(f"  最大赢: {max(t.pnl_pct for t in all_trades):+.2f}%  最大亏: {min(t.pnl_pct for t in all_trades):+.2f}%")
    print(f"  均持仓: {sum(t.hold_bars for t in all_trades)/len(all_trades):.1f} 根 4h K线")

    print(f"\n  ── 按信号类型 ──")
    for sig, stats in sorted(signal_stats.items(), key=lambda x: x[1]["total_pnl"], reverse=True):
        wr = stats["wins"] / stats["count"] if stats["count"] > 0 else 0
        print(f"    {sig:25s}  {stats['count']:3d}笔  胜率{wr:.0%}  总收{stats['total_pnl']:+.2f}%")

    print(f"\n  ── 按出场原因 ──")
    for reason, stats in sorted(exit_stats.items(), key=lambda x: x[1]["total_pnl"], reverse=True):
        print(f"    {reason:12s}  {stats['count']:3d}笔  总收{stats['total_pnl']:+.2f}%")

    return results


if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("--days", type=int, default=7)
    parser.add_argument("--interval", type=int, default=4)
    parser.add_argument("--top-k", type=int, default=5)
    parser.add_argument("--leverage", type=int, default=3)
    args = parser.parse_args()

    sniper = SniperConfig(leverage=args.leverage)
    results = run_sniper_backtest(
        days=args.days, interval_hours=args.interval,
        top_k=args.top_k, sniper_cfg=sniper,
    )

    # 保存结果
    ts = datetime.now().strftime("%Y%m%d_%H%M")
    path = os.path.expanduser(f"~/.gstack/hunter_validator_cache/sniper_backtest_{ts}.json")
    with open(path, 'w') as f:
        json.dump(results, f, indent=2, default=str)
    print(f"\n  结果已保存: {path}")
