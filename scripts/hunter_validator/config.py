"""
Hunter Validator - 可调参数配置
忠实复刻 hunter.go 中的所有硬编码常量，同时支持参数化调整。
"""

from dataclasses import dataclass, field
from typing import Optional
import json


@dataclass
class HunterConfig:
    """所有 Hunter 评分参数，带默认值 (与 Go 代码一致)。"""

    # ── Pillar S-A': Position Score ──
    atr_period: int = 14                # ATR 计算周期
    atr_multiplier: float = 2.0         # 支撑/阻力判定距离 = atr_multiplier × ATR
    kline_bars: int = 20                # 4h K线获取数量
    pos_support_bonus: float = 30.0     # 近支撑奖励
    pos_resistance_penalty: float = -15.0  # 近阻力惩罚
    chase_penalty_threshold: float = 50.0  # 追涨惩罚阈值 (%)
    chase_penalty_bonus: float = -20.0  # 追涨惩罚分值
    pos_score_min: float = -35.0        # 位置分下限
    pos_score_max: float = 30.0         # 位置分上限

    # ── Pillar S-A': OI Smart Score ──
    oi_threshold: float = 2_000_000.0   # OI 最低门槛 (USD)
    oi_threshold_reduction: float = 0.80  # 被过滤 3 次后的门槛缩减系数
    oi_threshold_filter_count: int = 3  # 触发门槛缩减的过滤次数
    oi_strong_delta: float = 15.0       # OI 变动强信号阈值 (%)
    oi_moderate_delta: float = 8.0      # OI 变动中等信号阈值 (%)
    oi_aligned_bonus: float = 40.0      # OI-价格对齐奖励
    oi_accumulation_bonus: float = 25.0 # OI 积累奖励
    oi_moderate_bonus: float = 15.0     # OI 中等信号奖励
    oi_score_min: float = 0.0           # OI 分下限
    oi_score_max: float = 50.0          # OI 分上限

    # ── Pillar S-B': Smart Money ──
    lsr_period: str = "1h"              # LSR 数据周期
    lsr_limit: int = 4                  # LSR 数据点数
    lsr_reversal_threshold: float = 0.8 # LSR 反转阈值 (oldestRatio < X)
    lsr_reversal_bonus: float = 20.0    # LSR 反转奖励
    lsr_delta_threshold: float = 10.0   # LSR 变动阈值 (%)
    lsr_bullish_bonus: float = 10.0     # LSR 多头奖励
    lsr_bearish_bonus: float = 10.0     # LSR 空头奖励
    taker_buy_threshold: float = 0.60   # Taker 买入占比阈值
    taker_buy_bonus: float = 10.0       # Taker 买入强奖励
    taker_trend_bonus: float = 10.0     # Taker 趋势上行奖励
    sm_score_min: float = 0.0           # 聪明钱分下限
    sm_score_max: float = 50.0          # 聪明钱分上限
    sb_scale_factor: float = 0.50       # S-B' 缩放系数

    # ── Composite ──
    composite_min: float = 0.0
    composite_max: float = 75.0
    sa_range_max: float = 50.0          # S-A' composite 上限

    # ── Pillar D': Wash Trade Detection ──
    wash_micro_trade_count: int = 1_000_000
    wash_avg_trade_size: float = 5.0    # USD
    wash_micro_multiplier: float = 0.20
    wash_oi_vol_ratio: float = 0.01     # OI/Volume 低于此视为虚假量
    wash_fake_vol_multiplier: float = 0.30
    wash_spike_bars: int = 5            # 最后 N 根 K 线检测异常放量
    wash_avg_bars: int = 15             # 前 N 根 K 线计算均量
    wash_spike_threshold: float = 10.0  # 异常放量倍数
    wash_spike_count: int = 3           # 至少 N 根异常
    wash_spike_multiplier: float = 0.30

    # ── Pool ──
    candidate_pool: int = 50            # 候选池大小
    top_n: int = 30                     # 返回数量

    def to_dict(self) -> dict:
        return {k: getattr(self, k) for k in self.__dataclass_fields__}

    def save(self, path: str):
        with open(path, 'w') as f:
            json.dump(self.to_dict(), f, indent=2)

    @classmethod
    def load(cls, path: str) -> 'HunterConfig':
        with open(path) as f:
            data = json.load(f)
        return cls(**{k: v for k, v in data.items() if k in cls.__dataclass_fields__})


# 默认配置实例
DEFAULT_CONFIG = HunterConfig()
