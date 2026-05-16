"""
Hunter Validator - 历史回测引擎
每小时检查点，模拟 "当时 Hunter 会选谁，后续表现如何"。
"""

import json
import os
import random
import time
from collections import defaultdict
from datetime import datetime, timedelta, timezone
from typing import Optional

from api import BinanceAPI
from config import HunterConfig, DEFAULT_CONFIG
from scoring import score_coin, ScoredCoin


class BacktestEngine:
    """历史回测引擎。"""

    def __init__(self, api: BinanceAPI, cfg: HunterConfig = DEFAULT_CONFIG):
        self.api = api
        self.cfg = cfg
        self._cache_dir = os.path.expanduser("~/.gstack/hunter_validator_cache/backtest")
        os.makedirs(self._cache_dir, exist_ok=True)

    def _get_universe_at(self, tickers_current: list[dict],
                         end_time_ms: int) -> list[dict]:
        """获取回测时间点的 Top-50 币种列表。
        用当前 ticker 的 symbol 列表，但成交量用当时的 K 线数据近似。
        """
        # 简化: 用当前 Top-100 USDT perps 作为候选池
        # (无法精确获取历史 ticker，但 K 线可以按 endTime 查询)
        return tickers_current[:100]

    def _fetch_klines_at(self, symbol: str, interval: str, limit: int,
                         end_time_ms: int) -> list[dict]:
        """获取指定时间点之前的 K 线。"""
        return self.api.get_klines(symbol, interval, limit, end_time=end_time_ms)

    def _fetch_oi_proxy(self, symbol: str, klines_4h: list[dict]) -> tuple[float, float]:
        """用 K 线成交量近似 OI 值和变动率。
        OI_proxy = volume × close (最后一根)
        变动率 = (最后 volume - 前一根 volume) / 前一根 volume × 100
        """
        if len(klines_4h) < 2:
            return 0.0, 0.0
        latest = klines_4h[-1]
        prev = klines_4h[-2]
        oi_proxy = latest["volume"] * latest["close"]
        if prev["volume"] > 0:
            delta_pct = ((latest["volume"] - prev["volume"]) / prev["volume"]) * 100
        else:
            delta_pct = 0.0
        return oi_proxy, delta_pct

    def _fetch_forward_return(self, symbol: str, entry_price: float,
                              entry_time_ms: int, hours: int) -> float:
        """获取后续 N 小时的收益率。"""
        forward_ms = entry_time_ms + hours * 3600 * 1000
        klines = self._fetch_klines_at(symbol, "1h", 1, end_time_ms=forward_ms)
        if not klines:
            return 0.0
        exit_price = klines[-1]["close"]
        if entry_price <= 0:
            return 0.0
        return ((exit_price - entry_price) / entry_price) * 100

    def run_checkpoint(self, tickers: list[dict], end_time_ms: int,
                       top_k: int = 10) -> dict:
        """单个检查点: 评分 → 选币 → 测量后续收益。"""
        scored = []

        for t in tickers[:self.cfg.candidate_pool]:
            symbol = t["symbol"]
            try:
                klines_4h = self._fetch_klines_at(symbol, "4h", 20, end_time_ms)
                if len(klines_4h) < 15:
                    continue

                # OI 近似
                oi_value, oi_delta = self._fetch_oi_proxy(symbol, klines_4h)

                # LSR 无法回测 (无历史API) → 用空数据
                lsr_data = []

                result = score_coin(
                    symbol=symbol,
                    ticker=t,
                    klines_4h=klines_4h,
                    lsr_data=lsr_data,
                    oi_delta_4h=oi_delta,
                    oi_value_usd=oi_value,
                    cfg=self.cfg,
                )
                if result.score.final_score > 0:
                    scored.append(result)
            except Exception:
                continue

        # 按分数排序
        scored.sort(key=lambda x: x.score.final_score, reverse=True)
        hunter_picks = scored[:top_k]

        # 测量后续收益
        picks_with_returns = []
        for sc in hunter_picks:
            entry_price = float(sc.ticker["lastPrice"])
            ret_1h = self._fetch_forward_return(sc.symbol, entry_price, end_time_ms, 1)
            ret_4h = self._fetch_forward_return(sc.symbol, entry_price, end_time_ms, 4)
            picks_with_returns.append({
                "symbol": sc.symbol,
                "score": sc.score.final_score,
                "entry_price": entry_price,
                "return_1h": ret_1h,
                "return_4h": ret_4h,
                "tags": sc.score.tags,
            })

        # 随机基线
        all_symbols = [t["symbol"] for t in tickers[:self.cfg.candidate_pool]]
        random_picks = []
        if len(all_symbols) >= top_k:
            rand_syms = random.sample(all_symbols, top_k)
            for sym in rand_syms:
                t = next((x for x in tickers if x["symbol"] == sym), None)
                if t:
                    entry_price = float(t["lastPrice"])
                    ret_1h = self._fetch_forward_return(sym, entry_price, end_time_ms, 1)
                    random_picks.append({"symbol": sym, "return_1h": ret_1h})

        # 成交量基线
        vol_picks = []
        vol_sorted = sorted(tickers[:self.cfg.candidate_pool],
                            key=lambda x: float(x.get("quoteVolume", 0)), reverse=True)
        for t in vol_sorted[:top_k]:
            entry_price = float(t["lastPrice"])
            ret_1h = self._fetch_forward_return(t["symbol"], entry_price, end_time_ms, 1)
            vol_picks.append({"symbol": t["symbol"], "return_1h": ret_1h})

        return {
            "timestamp": end_time_ms,
            "hunter_picks": picks_with_returns,
            "random_picks": random_picks,
            "volume_picks": vol_picks,
        }

    def run_backtest(self, days: int = 7, interval_hours: int = 1,
                     top_k: int = 10) -> dict:
        """运行完整回测。"""
        print(f"  获取当前 ticker 列表...")
        tickers = self.api.get_usdt_perp_tickers()
        print(f"  候选池: {len(tickers)} 个 USDT 永续合约")

        now_ms = int(time.time() * 1000)
        checkpoints = []
        total_hours = days * 24
        step_ms = interval_hours * 3600 * 1000

        print(f"  回测范围: {days}天, 每{interval_hours}小时一个检查点")
        print(f"  总检查点: {total_hours // interval_hours}")

        for i in range(total_hours // interval_hours):
            t_ms = now_ms - (i + 1) * step_ms
            dt = datetime.fromtimestamp(t_ms / 1000, tz=timezone.utc)
            print(f"\r  检查点 {i+1}/{total_hours // interval_hours}: {dt.strftime('%m-%d %H:%M')}", end="", flush=True)

            try:
                result = self.run_checkpoint(tickers, t_ms, top_k)
                checkpoints.append(result)
            except Exception as e:
                print(f" [跳过: {e}]", end="")
                continue

            # 限速: 每个检查点间暂停
            time.sleep(0.5)

        print("\n")

        # 汇总统计
        return self._aggregate_results(checkpoints, days, top_k)

    def _aggregate_results(self, checkpoints: list[dict], days: int,
                           top_k: int) -> dict:
        """汇总所有检查点的结果。"""
        hunter_returns_1h = []
        hunter_returns_4h = []
        random_returns_1h = []
        volume_returns_1h = []
        freq = defaultdict(int)

        for cp in checkpoints:
            for pick in cp["hunter_picks"]:
                hunter_returns_1h.append(pick["return_1h"])
                hunter_returns_4h.append(pick["return_4h"])
                freq[pick["symbol"]] += 1
            for pick in cp["random_picks"]:
                random_returns_1h.append(pick["return_1h"])
            for pick in cp["volume_picks"]:
                volume_returns_1h.append(pick["return_1h"])

        def safe_stats(returns: list[float]) -> dict:
            if not returns:
                return {"mean": 0, "std": 0, "hit_rate": 0, "sharpe": 0}
            n = len(returns)
            mean = sum(returns) / n
            var = sum((r - mean) ** 2 for r in returns) / max(n - 1, 1)
            std = var ** 0.5
            hit = sum(1 for r in returns if r > 1.0) / n  # >1% 算命中
            sharpe = (mean / std * (252 ** 0.5)) if std > 0 else 0
            return {"mean": mean, "std": std, "hit_rate": hit, "sharpe": sharpe}

        h_stats = safe_stats(hunter_returns_1h)
        r_stats = safe_stats(random_returns_1h)
        v_stats = safe_stats(volume_returns_1h)
        h4_stats = safe_stats(hunter_returns_4h)

        # 方向准确率
        dir_acc = sum(1 for r in hunter_returns_1h if r > 0) / max(len(hunter_returns_1h), 1)

        # t-检验 (简化)
        t_stat = 0.0
        p_value = 1.0
        if hunter_returns_1h and random_returns_1h:
            h_mean = h_stats["mean"]
            r_mean = r_stats["mean"]
            h_std = h_stats["std"]
            n = len(hunter_returns_1h)
            se = (h_std / (n ** 0.5)) if n > 0 else 1
            t_stat = (h_mean - r_mean) / se if se > 0 else 0
            # 近似 p-value (用正态近似)
            p_value = max(0.001, 1.0 - min(abs(t_stat) / 3, 0.999))

        return {
            "checkpoints": len(checkpoints),
            "total_picks": len(hunter_returns_1h),
            "days": days,
            "hunter": {
                "hit_rate_1h": h_stats["hit_rate"],
                "hit_rate_4h": h4_stats["hit_rate"],
                "hit_rate_24h": 0,  # 需要 24h 数据
                "avg_return_1h": h_stats["mean"],
                "avg_return_4h": h4_stats["mean"],
                "avg_return_24h": 0,
                "sharpe_1h": h_stats["sharpe"],
                "directional_accuracy": dir_acc,
            },
            "random_baseline": {
                "hit_rate_1h": r_stats["hit_rate"],
                "avg_return_1h": r_stats["mean"],
                "sharpe_1h": r_stats["sharpe"],
                "directional_accuracy": sum(1 for r in random_returns_1h if r > 0) / max(len(random_returns_1h), 1),
            },
            "volume_baseline": {
                "hit_rate_1h": v_stats["hit_rate"],
                "avg_return_1h": v_stats["mean"],
                "sharpe_1h": v_stats["sharpe"],
                "directional_accuracy": sum(1 for r in volume_returns_1h if r > 0) / max(len(volume_returns_1h), 1),
            },
            "significance": {
                "t_stat": t_stat,
                "p_value": p_value,
            },
            "selection_frequency": dict(freq),
            "raw_returns": {
                "hunter_1h": hunter_returns_1h,
                "random_1h": random_returns_1h,
            },
        }
