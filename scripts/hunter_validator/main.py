#!/usr/bin/env python3
"""
Hunter Validator - CLI 入口
用法:
    python main.py live              # 实时验证
    python main.py backtest --days 7 # 历史回测
    python main.py optimize          # 权重优化
    python main.py sensitivity       # 敏感性分析
"""

import argparse
import json
import os
import sys
import time
from datetime import datetime, timezone

# 确保可以导入同目录模块
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from api import BinanceAPI
from config import HunterConfig, DEFAULT_CONFIG
from scoring import score_coin, ScoredCoin
from report import print_live_report, print_backtest_report, print_optimize_report, save_report


def cmd_live(args):
    """实时验证: 获取当前数据，复刻 Hunter 评分。"""
    api = BinanceAPI(cache_ttl=120)
    cfg = DEFAULT_CONFIG
    if args.config:
        cfg = HunterConfig.load(args.config)

    print(f"\n  获取 Binance USDT 永续合约数据...")
    tickers = api.get_usdt_perp_tickers()
    print(f"  候选池: {len(tickers)} 个合约 (按 24h 成交额排序)")

    scored_coins = []
    total = min(len(tickers), cfg.candidate_pool)

    for i, t in enumerate(tickers[:total]):
        symbol = t["symbol"]
        pct = (i + 1) / total * 100
        print(f"\r  评分中: {i+1}/{total} ({pct:.0f}%) {symbol:<14}", end="", flush=True)

        try:
            # 获取数据
            klines_4h = api.get_klines(symbol, "4h", cfg.kline_bars)
            if len(klines_4h) < 15:
                continue

            # OI
            oi_value = 0.0
            oi_delta = 0.0
            try:
                oi_data = api.get_oi_history(symbol, "4h", 2)
                if len(oi_data) >= 2:
                    oi_value = oi_data[-1]["sumOpenInterestValue"]
                    prev_oi = oi_data[-2]["sumOpenInterestValue"]
                    if prev_oi > 0:
                        oi_delta = ((oi_data[-1]["sumOpenInterest"] - oi_data[-2]["sumOpenInterest"])
                                    / oi_data[-2]["sumOpenInterest"]) * 100
                elif len(oi_data) == 1:
                    oi_value = oi_data[0]["sumOpenInterestValue"]
            except Exception:
                # fallback: 用量×价近似
                if klines_4h:
                    oi_value = klines_4h[-1]["volume"] * klines_4h[-1]["close"]
                    if len(klines_4h) >= 2 and klines_4h[-2]["volume"] > 0:
                        oi_delta = ((klines_4h[-1]["volume"] - klines_4h[-2]["volume"])
                                    / klines_4h[-2]["volume"]) * 100

            # LSR
            lsr_data = []
            try:
                lsr_data = api.get_lsr_history(symbol, cfg.lsr_period, cfg.lsr_limit)
            except Exception:
                pass

            # 评分
            result = score_coin(
                symbol=symbol,
                ticker=t,
                klines_4h=klines_4h,
                lsr_data=lsr_data,
                oi_delta_4h=oi_delta,
                oi_value_usd=oi_value,
                cfg=cfg,
            )

            if result.score.final_score > 0:
                scored_coins.append(result)

        except Exception as e:
            continue

    print(f"\r  评分完成: {len(scored_coins)} 个有效标的" + " " * 30)

    # 排序
    scored_coins.sort(key=lambda x: x.score.final_score, reverse=True)

    # v6: 宁缺勿滥门控 — Top-10 中强信号标的不足则观望
    top_10 = scored_coins[:10]
    strong_count = sum(1 for sc in top_10 if sc.score.has_strong_signal)
    if strong_count < cfg.strong_signal_min:
        print(f"\n  ⚠️  宁缺勿滥: Top-10 中仅 {strong_count} 个强信号标的 (需≥{cfg.strong_signal_min})")
        print(f"  市场缺乏 OI/LSR 强信号, 观望不选币")
        # 保存空报告
        report_path = os.path.expanduser("~/.gstack/hunter_validator_cache/live_report.json")
        report_data = {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "status": "observe_mode",
            "reason": f"strong_signal_count={strong_count} < min={cfg.strong_signal_min}",
            "top_10": [],
        }
        save_report(report_data, report_path)
        return []

    # 输出报告
    print_live_report(scored_coins, cfg)

    # 保存 JSON
    report_path = os.path.expanduser("~/.gstack/hunter_validator_cache/live_report.json")
    report_data = {
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "config_version": "v6",
        "top_10": [
            {
                "rank": i + 1,
                "symbol": sc.symbol,
                "final_score": sc.score.final_score,
                "position_score": sc.score.position_score,
                "oi_smart_score": sc.score.oi_smart_score,
                "smart_money_score": sc.score.smart_money_score,
                "cooldown_mod": sc.score.cooldown_mod,
                "wash_mod": sc.score.wash_mod,
                "tags": sc.score.tags,
                "has_strong_signal": sc.score.has_strong_signal,
                "details": {
                    "atr": sc.score.atr,
                    "high20": sc.score.high20,
                    "low20": sc.score.low20,
                    "current_price": sc.score.current_price,
                    "oi_delta_4h": sc.score.oi_delta_4h,
                    "oi_value": sc.score.oi_value,
                    "lsr_oldest": sc.score.lsr_oldest,
                    "lsr_newest": sc.score.lsr_newest,
                    "taker_buy_ratio": sc.score.taker_buy_ratio,
                },
                "pct_24h": float(sc.ticker.get("priceChangePercent", 0)),
            }
            for i, sc in enumerate(scored_coins[:10])
        ],
    }
    save_report(report_data, report_path)

    return scored_coins


def cmd_backtest(args):
    """历史回测。"""
    api = BinanceAPI(cache_ttl=3600)
    cfg = DEFAULT_CONFIG
    if args.config:
        cfg = HunterConfig.load(args.config)

    from backtest import BacktestEngine
    engine = BacktestEngine(api, cfg)

    results = engine.run_backtest(
        days=args.days,
        interval_hours=args.interval,
        top_k=args.top_k,
    )

    print_backtest_report(results)

    # 保存
    ts = datetime.now().strftime("%Y%m%d_%H%M")
    report_path = os.path.expanduser(
        f"~/.gstack/hunter_validator_cache/backtest_{ts}.json")
    # 移除 raw_returns (太大)
    save_data = {k: v for k, v in results.items() if k != "raw_returns"}
    save_report(save_data, report_path)


def cmd_optimize(args):
    """权重优化。"""
    api = BinanceAPI(cache_ttl=3600)

    from optimizer import WeightOptimizer
    opt = WeightOptimizer(api)

    results = opt.optimize(days=args.days, interval_hours=args.interval)

    print_optimize_report(results)

    # 保存最优配置
    best_cfg = results.get("best", {}).get("config", {})
    if best_cfg:
        cfg_path = os.path.expanduser(
            "~/.gstack/hunter_validator_cache/optimized_config.json")
        with open(cfg_path, 'w') as f:
            json.dump(best_cfg, f, indent=2)
        print(f"  最优配置已保存: {cfg_path}")

    # 保存报告
    ts = datetime.now().strftime("%Y%m%d_%H%M")
    report_path = os.path.expanduser(
        f"~/.gstack/hunter_validator_cache/optimize_{ts}.json")
    save_report(results, report_path)


def cmd_sensitivity(args):
    """敏感性分析: 固定其他参数，扫描目标参数。"""
    api = BinanceAPI(cache_ttl=3600)
    cfg = DEFAULT_CONFIG

    from backtest import BacktestEngine

    param = args.param
    if not hasattr(cfg, param):
        print(f"  错误: 未知参数 '{param}'")
        print(f"  可用参数: {[f for f in cfg.__dataclass_fields__]}")
        return

    # 参数范围
    default_val = getattr(cfg, param)
    if args.range_str:
        parts = [float(x) for x in args.range_str.split(",")]
        if len(parts) == 3:
            values = list(range(int(parts[0]), int(parts[1]), int(parts[2])))
        else:
            values = parts
    else:
        # 自动范围: 默认值 ±50%, 10 步
        values = [default_val * (0.5 + i * 0.1) for i in range(11)]

    print(f"\n  敏感性分析: {param}")
    print(f"  默认值: {default_val}")
    print(f"  扫描范围: {values}")
    print()

    # 获取数据
    tickers = api.get_usdt_perp_tickers()
    now_ms = int(time.time() * 1000)
    checkpoints_ms = [now_ms - (i + 1) * 6 * 3600 * 1000 for i in range(28)]  # 7天, 每6h

    results = []
    for val in values:
        cfg_copy = HunterConfig(**cfg.to_dict())
        setattr(cfg_copy, param, val)

        engine = BacktestEngine(api, cfg_copy)
        all_returns = []
        for t_ms in checkpoints_ms[:10]:  # 取前10个检查点加速
            try:
                cp = engine.run_checkpoint(tickers, t_ms, top_k=5)
                for pick in cp["hunter_picks"]:
                    all_returns.append(pick["return_1h"])
            except Exception:
                continue

        if all_returns:
            n = len(all_returns)
            mean = sum(all_returns) / n
            hit = sum(1 for r in all_returns if r > 1.0) / n
            std = (sum((r - mean) ** 2 for r in all_returns) / max(n - 1, 1)) ** 0.5
            sharpe = (mean / std * (252 ** 0.5)) if std > 0 else 0
        else:
            mean = hit = sharpe = 0

        results.append({"value": val, "hit_rate": hit, "avg_return": mean, "sharpe": sharpe})
        print(f"    {param}={val:>10.1f}  hit={hit:.1%}  ret={mean:+.3f}%  sharpe={sharpe:.3f}")

    # 找最优
    best = max(results, key=lambda x: x["hit_rate"])
    print(f"\n  最优 {param}: {best['value']} (hit rate: {best['hit_rate']:.1%})")

    # 保存
    ts = datetime.now().strftime("%Y%m%d_%H%M")
    report_path = os.path.expanduser(
        f"~/.gstack/hunter_validator_cache/sensitivity_{param}_{ts}.json")
    save_report({"param": param, "default": default_val, "results": results}, report_path)


def main():
    parser = argparse.ArgumentParser(
        description="Hunter 选币模块验证工具",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  python main.py live                         # 实时验证当前选币
  python main.py live --config optimized.json # 用优化后的配置
  python main.py backtest --days 7            # 7天回测
  python main.py optimize --days 3            # 3天快速优化
  python main.py sensitivity --param oi_aligned_bonus --range 20,60,5
        """,
    )
    parser.add_argument("--config", help="自定义配置文件路径")

    subparsers = parser.add_subparsers(dest="command")

    # live
    live_parser = subparsers.add_parser("live", help="实时验证")

    # backtest
    bt_parser = subparsers.add_parser("backtest", help="历史回测")
    bt_parser.add_argument("--days", type=int, default=7, help="回测天数")
    bt_parser.add_argument("--interval", type=int, default=1, help="检查点间隔(小时)")
    bt_parser.add_argument("--top-k", type=int, default=10, help="每次选币数")

    # optimize
    opt_parser = subparsers.add_parser("optimize", help="权重优化")
    opt_parser.add_argument("--days", type=int, default=7, help="优化数据范围")
    opt_parser.add_argument("--interval", type=int, default=6, help="检查点间隔(小时)")

    # sensitivity
    sens_parser = subparsers.add_parser("sensitivity", help="敏感性分析")
    sens_parser.add_argument("--param", required=True, help="目标参数名")
    sens_parser.add_argument("--range", dest="range_str", help="扫描范围 (start,end,step)")

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        return

    print(f"\n{'='*80}")
    print(f"  Hunter 选币验证工具 v1.0")
    print(f"  AiT Project | {datetime.now().strftime('%Y-%m-%d %H:%M')}")
    print(f"{'='*80}")

    if args.command == "live":
        cmd_live(args)
    elif args.command == "backtest":
        cmd_backtest(args)
    elif args.command == "optimize":
        cmd_optimize(args)
    elif args.command == "sensitivity":
        cmd_sensitivity(args)


if __name__ == "__main__":
    main()
