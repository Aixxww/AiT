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
    pos_support_bonus: float = 25.0     # 近支撑奖励 (v3: 对齐Go 4h支撑=25)
    pos_resistance_penalty: float = -25.0  # v6: 近阻力惩罚 -15→-25 (R5: 75%方向正确)
    chase_penalty_threshold: float = 50.0  # 追涨惩罚阈值 (v3: 恢复Go值50%)
    chase_penalty_bonus: float = -20.0  # 追涨惩罚分值 (同Go)
    pos_score_min: float = -35.0        # 位置分下限 (同Go)
    pos_score_max: float = 55.0         # 位置分上限 (v3: 对齐Go [-35, 55])

    # ── Short-side OI threshold (v7: lower to catch more short opportunities) ──
    oi_threshold_short: float = 1_500_000.0  # Short OI 门槛 (USD), 低于做多 $2M

    # ── Pillar S-A': OI Smart Score ──
    oi_threshold: float = 2_000_000.0   # OI 最低门槛 (USD) (同Go)
    oi_threshold_reduction: float = 0.80  # 被过滤 3 次后的门槛缩减系数
    oi_threshold_filter_count: int = 3  # 触发门槛缩减的过滤次数
    oi_strong_delta: float = 15.0       # OI 变动强信号阈值 (%)
    oi_moderate_delta: float = 8.0      # OI 变动中等信号阈值 (%)
    oi_aligned_bonus: float = 25.0      # OI-价格对齐奖励 (v3: 对齐Go中等增长=25)
    oi_accumulation_bonus: float = 40.0 # OI 积累奖励 (v3: 提升至40, Round2中56.3%胜率最强信号)
    oi_moderate_bonus: float = 15.0     # OI 中等信号奖励 (同Go低增长=15)
    oi_score_min: float = 0.0           # OI 分下限 (同Go)
    oi_score_max: float = 50.0          # OI 分上限 (同Go)

    # ── OI Short Squeeze 检测 (v6 新增) ──
    oi_squeeze_delta: float = -10.0     # OI 下降阈值 (%) → short squeeze
    oi_squeeze_bonus: float = 45.0      # OI↓>10% + price↑ = 最强信号 (FIDA +9.11% 验证)
    oi_squeeze_moderate_delta: float = -5.0  # OI 下降阈值 (%) → moderate squeeze
    oi_squeeze_moderate_bonus: float = 20.0  # OI↓>5% + price↑ = 弱 squeeze

    # ── 妖币检测器: OI 暴增 + 价格平坦 (v2新增, v3调优) ──
    oi_surge_threshold: float = 15.0    # OI 1h 增长阈值 (%)
    oi_surge_price_flat: float = 3.0    # 价格变动平坦阈值 (%)
    oi_surge_bonus: float = 15.0        # OI 暴增奖励分 (v3: 25→15, 降低噪声)
    vol_oi_ratio_min: float = 3.0       # Vol/OI 比值最低门槛
    vol_oi_high_bonus: float = 0.0      # v6: vol_oi_high 改为门控条件，不计分 (R5: 25%胜率，纯噪声)
    vol_oi_extreme_bonus: float = 0.0   # Vol/OI 极端高比值奖励 (v3: 25→0, 禁用! Round2中34.1%胜率/-5.12%收益)
    vol_oi_extreme_threshold: float = 999.0  # 极端比值阈值 (v3: 8→999, 等效禁用)

    # ── 妖币检测器: 资金费率 (v2 新增) ──
    funding_rate_negative_threshold: float = -0.0001  # 负费率阈值
    funding_rate_bonus: float = 15.0    # 负费率奖励 (空头拥挤→爆空动力)
    funding_rate_extreme_bonus: float = 25.0  # 极端负费率奖励
    funding_rate_extreme_threshold: float = -0.0005  # 极端负费率阈值

    # ── Taker 增强 (v3: 对齐Go) ──
    taker_buy_threshold: float = 0.60   # Taker 买入占比阈值 (v3: 恢复Go值0.60)
    taker_buy_bonus: float = 10.0       # Taker 买入奖励 (v3: 对齐Go moderate=10)
    taker_strong_threshold: float = 0.65  # Taker 强买入阈值 (同Go strong)
    taker_strong_bonus: float = 20.0    # Taker 强买入奖励 (v3: 对齐Go strong=20)

    # ── Pillar S-B': Smart Money (v3: 对齐Go) ──
    lsr_period: str = "1h"              # LSR 数据周期
    lsr_limit: int = 4                  # LSR 数据点数
    lsr_reversal_threshold: float = 0.8 # LSR 反转阈值 (oldestRatio < X)
    lsr_reversal_bonus: float = 20.0    # LSR 反转奖励
    lsr_delta_threshold: float = 10.0   # LSR 变动阈值 (%)
    lsr_bullish_bonus: float = 10.0     # LSR 多头奖励
    lsr_bearish_bonus: float = 10.0     # LSR 空头奖励
    taker_trend_bonus: float = 5.0      # v5: 10→5, 降级为辅助分 (40%胜率)
    sm_score_min: float = 0.0           # 聪明钱分下限 (同Go)
    sm_score_max: float = 65.0          # 聪明钱分上限 (v3: 对齐Go [0, 65])
    sb_scale_factor: float = 0.65       # S-B' 缩放系数 (v3: 对齐Go 0.65)

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

    # ── Pillar D': Extreme Loss Protection (v4 新增) ──
    # 小币 + 低OI + 极端亏损 = 庄家砸盘 / 流动性陷阱
    # 典型: BUSDT(-23.13%), OI=$5.8M, V/OI=15.8
    elp_oi_severe: float = 5_000_000.0      # OI < $5M: 严重小币
    elp_oi_moderate: float = 20_000_000.0   # OI < $20M: 中等小币 (v4.1: 10M→20M, BUSDT OI=$19.7M)
    elp_loss_severe: float = 15.0           # 24h亏损 > 15%: 极端
    elp_loss_moderate: float = 10.0         # 24h亏损 > 10%: 显著
    elp_volo_min: float = 3.0              # Vol/OI > 3: 高投机 (v4.1: 5→3, 匹配vol_oi_ratio_min)
    elp_severe_multiplier: float = 0.30    # 严重场景惩罚
    elp_moderate_multiplier: float = 0.50  # 中等场景惩罚

    # ── Short ELP: gain-side thresholds (做空侧用涨幅判定) ──
    # v7: gain>15% 无条件惩罚，无需OI条件（更敏感）
    short_elp_gain_hard: float = 20.0       # 涨幅>20% → 硬杀 0.10
    short_elp_gain_severe: float = 15.0     # 涨幅>15% → 严重 0.30
    short_elp_gain_moderate: float = 10.0   # 涨幅>10%+OI<20M → 中等 0.50

    # ── Pool ──
    candidate_pool: int = 50            # 候选池大小
    top_n: int = 30                     # 返回数量

    # ── 宁缺勿滥门控 (v6 新增) ──
    strong_signal_min: int = 3          # Top-N 中强信号标的最少数量，不足则观望
    strong_signals: set = field(default_factory=lambda: {
        "oi_accumulation", "oi_price_aligned", "oi_moderate",
        "oi_short_squeeze", "oi_squeeze_moderate",
        "lsr_reversal", "lsr_bullish",
    })

    # ── Short-side 宁缺勿滥门控 ──
    short_strong_signal_min: int = 2       # 短方向强信号最少数量
    short_strong_signals: set = field(default_factory=lambda: {
        "oi_distribution", "oi_price_aligned_short", "oi_moderate_short",
        "oi_long_squeeze", "oi_long_squeeze_moderate",
        "lsr_bearish_reversal", "lsr_bearish_strong",
    })

    # ── BTC 趋势过滤 (v3 新增) ──
    btc_rsi_filter_threshold: float = 35.0  # BTC RSI14 低于此值时跳过做多
    btc_symbol: str = "BTCUSDT"            # BTC 标的

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
