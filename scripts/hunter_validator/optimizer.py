"""
Hunter Validator - 权重优化器
分组网格搜索，避免维度诅咒。
"""

import copy
import itertools
import time
from typing import Optional

from api import BinanceAPI
from config import HunterConfig, DEFAULT_CONFIG
from backtest import BacktestEngine


class WeightOptimizer:
    """分组网格搜索优化器。"""

    def __init__(self, api: BinanceAPI):
        self.api = api

    def _param_groups(self, cfg: HunterConfig) -> list[dict]:
        """定义参数搜索空间 (分组避免组合爆炸)。"""
        return [
            {
                "name": "G1: 权重分配",
                "params": {
                    "sa_range_max": [35, 40, 45, 50, 55],
                    "sb_scale_factor": [0.30, 0.40, 0.50, 0.60, 0.70],
                },
            },
            {
                "name": "G2: OI 参数",
                "params": {
                    "oi_aligned_bonus": [25, 30, 35, 40, 45],
                    "oi_accumulation_bonus": [15, 20, 25, 30],
                    "oi_moderate_bonus": [10, 15, 20],
                },
            },
            {
                "name": "G3: 位置分参数",
                "params": {
                    "pos_support_bonus": [20, 25, 30, 35],
                    "pos_resistance_penalty": [-25, -20, -15, -10],
                    "chase_penalty_threshold": [30, 40, 50, 60],
                },
            },
            {
                "name": "G4: 聪明钱参数",
                "params": {
                    "lsr_reversal_bonus": [15, 20, 25, 30],
                    "lsr_bullish_bonus": [5, 10, 15],
                    "taker_buy_bonus": [5, 10, 15],
                    "taker_trend_bonus": [5, 10, 15],
                },
            },
            {
                "name": "G5: 门槛参数",
                "params": {
                    "oi_threshold": [1_000_000, 1_500_000, 2_000_000, 3_000_000],
                    "taker_buy_threshold": [0.50, 0.55, 0.60, 0.65],
                },
            },
        ]

    def _evaluate_config(self, cfg: HunterConfig, tickers: list[dict],
                         backtest_engine: BacktestEngine,
                         checkpoints_ms: list[int]) -> dict:
        """用给定配置评估回测表现。"""
        returns = []
        for t_ms in checkpoints_ms:
            try:
                result = backtest_engine.run_checkpoint(tickers, t_ms, top_k=5)
                for pick in result["hunter_picks"]:
                    returns.append(pick["return_1h"])
            except Exception:
                continue

        if not returns:
            return {"composite_score": -999, "hit_rate": 0, "avg_return": 0, "sharpe": 0}

        n = len(returns)
        mean = sum(returns) / n
        var = sum((r - mean) ** 2 for r in returns) / max(n - 1, 1)
        std = var ** 0.5
        hit_rate = sum(1 for r in returns if r > 1.0) / n
        sharpe = (mean / std * (252 ** 0.5)) if std > 0 else 0
        dir_acc = sum(1 for r in returns if r > 0) / n

        # 加权综合得分
        composite = hit_rate * 0.40 + (mean / 100) * 0.30 + (sharpe / 10) * 0.20 + dir_acc * 0.10

        return {
            "composite_score": composite,
            "hit_rate": hit_rate,
            "avg_return": mean,
            "sharpe": sharpe,
            "directional_accuracy": dir_acc,
            "n_samples": n,
        }

    def optimize(self, days: int = 7, interval_hours: int = 6) -> dict:
        """运行分组网格搜索。"""
        print(f"  获取 ticker 和检查点时间...")
        tickers = self.api.get_usdt_perp_tickers()

        now_ms = int(time.time() * 1000)
        step_ms = interval_hours * 3600 * 1000
        checkpoints_ms = [now_ms - (i + 1) * step_ms
                          for i in range(days * 24 // interval_hours)]

        print(f"  检查点数: {len(checkpoints_ms)}, 间隔: {interval_hours}h")

        # 基线评估
        print(f"\n  评估基线配置...")
        base_engine = BacktestEngine(self.api, DEFAULT_CONFIG)
        baseline = self._evaluate_config(DEFAULT_CONFIG, tickers, base_engine, checkpoints_ms)
        print(f"  基线: composite={baseline['composite_score']:.4f}, "
              f"hit={baseline['hit_rate']:.1%}, ret={baseline['avg_return']:+.3f}%")

        best_cfg = copy.deepcopy(DEFAULT_CONFIG)
        best_score = baseline["composite_score"]
        improvements = []

        groups = self._param_groups(DEFAULT_CONFIG)

        for group in groups:
            print(f"\n  优化 {group['name']}...")
            param_names = list(group["params"].keys())
            param_values = list(group["params"].values())
            combos = list(itertools.product(*param_values))
            print(f"    组合数: {len(combos)}")

            group_best_score = best_score
            group_best_combo = None

            for j, combo in enumerate(combos):
                cfg = copy.deepcopy(best_cfg)
                for name, val in zip(param_names, combo):
                    setattr(cfg, name, val)

                engine = BacktestEngine(self.api, cfg)
                result = self._evaluate_config(cfg, tickers, engine, checkpoints_ms)

                if result["composite_score"] > group_best_score:
                    group_best_score = result["composite_score"]
                    group_best_combo = dict(zip(param_names, combo))

                if (j + 1) % 10 == 0:
                    print(f"    进度: {j+1}/{len(combos)}, 当前最优: {group_best_score:.4f}", end="\r")

            if group_best_combo:
                for name, val in group_best_combo.items():
                    old_val = getattr(best_cfg, name)
                    setattr(best_cfg, name, val)
                    improvements.append({
                        "param": name,
                        "old": old_val,
                        "new": val,
                        "group": group["name"],
                    })
                best_score = group_best_score
                print(f"    ✓ 改进: {group_best_combo} → score={best_score:.4f}")
            else:
                print(f"    - 无改进")

        # 最终评估
        final_engine = BacktestEngine(self.api, best_cfg)
        final = self._evaluate_config(best_cfg, tickers, final_engine, checkpoints_ms)

        # 生成建议
        recommendations = []
        for imp in improvements:
            if abs(imp["new"] - imp["old"]) > 0:
                direction = "增加" if imp["new"] > imp["old"] else "减少"
                recommendations.append(
                    f"{imp['param']}: {imp['old']} → {imp['new']} ({direction}, 来自 {imp['group']})"
                )

        if not recommendations:
            recommendations.append("当前参数已接近最优，无需调整")

        return {
            "baseline": {
                "composite_score": baseline["composite_score"],
                "hit_rate": baseline["hit_rate"],
                "avg_return": baseline["avg_return"],
                "sharpe": baseline["sharpe"],
            },
            "best": {
                "composite_score": final["composite_score"],
                "hit_rate": final["hit_rate"],
                "avg_return": final["avg_return"],
                "sharpe": final["sharpe"],
                "config": best_cfg.to_dict(),
            },
            "improvements": improvements,
            "performance_delta": {
                "composite_score": final["composite_score"] - baseline["composite_score"],
                "hit_rate": final["hit_rate"] - baseline["hit_rate"],
                "avg_return": final["avg_return"] - baseline["avg_return"],
                "sharpe": final["sharpe"] - baseline["sharpe"],
            },
            "recommendations": recommendations,
        }
