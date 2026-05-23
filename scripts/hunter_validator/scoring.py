"""
Hunter Validator - 评分引擎
忠实复刻 hunter.go 的 4 支柱评分逻辑。
每个函数与 Go 代码一一对应，确保结果一致。
"""

import math
from dataclasses import dataclass, field
from typing import Optional

from config import HunterConfig, DEFAULT_CONFIG


@dataclass
class CoinScore:
    """单个标的的评分结果，对应 Go 的 HunterCoinScore。"""
    symbol: str = ""
    position_score: float = 0.0    # S-A' 位置分 [-35, 30]
    oi_smart_score: float = 0.0    # S-A' OI 分 [0, 50]
    smart_money_score: float = 0.0 # S-B' 聪明钱分 [0, 50]
    cooldown_mod: float = 1.0      # C' 冷却乘数
    wash_mod: float = 1.0          # D' 刷量乘数
    final_score: float = 0.0
    tags: list = field(default_factory=list)
    # 中间数据 (调试用)
    atr: float = 0.0
    high20: float = 0.0
    low20: float = 0.0
    current_price: float = 0.0
    oi_delta_4h: float = 0.0
    oi_value: float = 0.0
    lsr_oldest: float = 0.0
    lsr_newest: float = 0.0
    taker_buy_ratio: float = 0.0
    wash_details: dict = field(default_factory=dict)
    has_strong_signal: bool = False     # v6: 宁缺勿滥标记

    # ── Short-direction scoring (mirror of long scoring) ──
    short_position_score: float = 0.0    # Short position score [-35, 55]
    short_oi_smart_score: float = 0.0    # Short OI score [0, 50]
    short_smart_money_score: float = 0.0 # Short smart money score [0, 65]
    short_final_score: float = 0.0
    short_tags: list = field(default_factory=list)
    direction: str = "LONG"              # "LONG" or "SHORT"
    has_short_strong_signal: bool = False  # Short 宁缺勿滥标记


def clamp(value: float, min_val: float, max_val: float) -> float:
    return max(min_val, min(max_val, value))


# ── ATR 计算 (Wilder's Smoothing) ──

def compute_atr(klines: list[dict], period: int = 14) -> float:
    """
    hunter.go:27-48
    Wilder's ATR: 前 period 根用 SMA，之后用 EMA 递推。
    """
    if len(klines) < period + 1:
        return 0.0

    tr_sum = 0.0
    for i in range(1, period + 1):
        high = klines[i]["high"]
        low = klines[i]["low"]
        prev_close = klines[i - 1]["close"]
        tr = max(high - low, abs(high - prev_close), abs(low - prev_close))
        tr_sum += tr

    atr = tr_sum / period

    for i in range(period + 1, len(klines)):
        high = klines[i]["high"]
        low = klines[i]["low"]
        prev_close = klines[i - 1]["close"]
        tr = max(high - low, abs(high - prev_close), abs(low - prev_close))
        atr = (atr * (period - 1) + tr) / period

    return atr


# ── Pillar S-A': Position Score ──

def compute_position_score(
    klines_4h: list[dict],
    price_change_pct: float,
    cfg: HunterConfig = DEFAULT_CONFIG
) -> tuple[float, list[str]]:
    """
    hunter.go:53-101
    用 4h K线评估支撑/阻力位置。
    """
    if len(klines_4h) < 15:
        return 0.0, []

    score = 0.0
    tags = []

    atr = compute_atr(klines_4h, cfg.atr_period)
    if atr <= 0:
        return 0.0, []

    current_price = klines_4h[-1]["close"]

    # 20-bar high/low
    high20 = max(k["high"] for k in klines_4h)
    low20 = min(k["low"] for k in klines_4h)

    # Near support: currentPrice - low20 <= 2 * ATR
    if current_price - low20 <= cfg.atr_multiplier * atr:
        score += cfg.pos_support_bonus
        tags.append("near_support")

    # Near resistance: high20 - currentPrice <= 2 * ATR
    if high20 - current_price <= cfg.atr_multiplier * atr:
        score += cfg.pos_resistance_penalty
        tags.append("near_resistance")

    # Chase penalty: >50% move in 24h
    pct24h = abs(price_change_pct)
    if pct24h > cfg.chase_penalty_threshold:
        score += cfg.chase_penalty_bonus
        tags.append("chase_penalty")

    return clamp(score, cfg.pos_score_min, cfg.pos_score_max), tags


# ── Pillar S-A': OI Smart Score ──

def compute_oi_smart_score(
    oi_delta_4h: float,       # OI 4h 变动率 (%)
    oi_value_usd: float,      # 当前 OI 市值 (USD)
    price_change_pct: float,
    cfg: HunterConfig = DEFAULT_CONFIG
) -> tuple[float, list[str]]:
    """
    hunter.go:104-152
    OI 变动率分析，替代硬阈值。
    """
    score = 0.0
    tags = []

    # OI 绝对值低于门槛 → 过滤
    if oi_value_usd < cfg.oi_threshold:
        tags.append("oi_too_low")
        return 0.0, tags

    price_dir = price_change_pct  # >0 涨, <0 跌

    # v6: OI Short Squeeze 检测 (OI↓ + 价格↑ = 空头清算, 最强信号)
    if oi_delta_4h < cfg.oi_squeeze_delta and price_dir > 0:
        score += cfg.oi_squeeze_bonus
        tags.append("oi_short_squeeze")
    elif oi_delta_4h < cfg.oi_squeeze_moderate_delta and price_dir > 0:
        score += cfg.oi_squeeze_moderate_bonus
        tags.append("oi_squeeze_moderate")
    elif abs(oi_delta_4h) > cfg.oi_strong_delta:
        # OI-价格对齐
        if (price_dir > 0 and oi_delta_4h > 0) or (price_dir < 0 and oi_delta_4h < 0):
            score += cfg.oi_aligned_bonus
            tags.append("oi_price_aligned")
        # OI 积累 (OI 增但价格跌 → 主力吸筹)
        if oi_delta_4h > 0 and price_dir < 0:
            score += cfg.oi_accumulation_bonus
            tags.append("oi_accumulation")
    elif abs(oi_delta_4h) > cfg.oi_moderate_delta:
        score += cfg.oi_moderate_bonus
        tags.append("oi_moderate")

    return clamp(score, cfg.oi_score_min, cfg.oi_score_max), tags


# ── Pillar S-B': Smart Money Score ──

def compute_smart_money_score(
    lsr_data: list[dict],     # LSR 历史数据
    klines_4h: list[dict],
    cfg: HunterConfig = DEFAULT_CONFIG
) -> tuple[float, list[str]]:
    """
    hunter.go:155-203
    聪明钱信号: LSR 反转 + Taker 买入占比。
    """
    score = 0.0
    tags = []

    # ── LSR Signal ──
    if len(lsr_data) >= 2:
        oldest_ratio = lsr_data[0]["longShortRatio"]
        newest_ratio = lsr_data[-1]["longShortRatio"]

        if oldest_ratio > 0:
            lsr_delta_pct = ((newest_ratio - oldest_ratio) / oldest_ratio) * 100

            # LSR 反转: 曾经偏空 (<0.8) 现在回升
            if oldest_ratio < cfg.lsr_reversal_threshold and newest_ratio > oldest_ratio:
                score += cfg.lsr_reversal_bonus
                tags.append("lsr_reversal")
            # LSR 多头
            if lsr_delta_pct > cfg.lsr_delta_threshold:
                score += cfg.lsr_bullish_bonus
                tags.append("lsr_bullish")
            # LSR 空头
            if lsr_delta_pct < -cfg.lsr_delta_threshold:
                score += cfg.lsr_bearish_bonus
                tags.append("lsr_bearish")

    # ── Taker Signal (v2: 增强阈值 + 强买入档) ──
    if len(klines_4h) >= 5:
        latest = klines_4h[-1]
        if latest["volume"] > 0:
            taker_ratio = latest["taker_buy_volume"] / latest["volume"]
            if taker_ratio > cfg.taker_strong_threshold:
                score += cfg.taker_strong_bonus
                tags.append("taker_buy_extreme")
            elif taker_ratio > cfg.taker_buy_threshold:
                score += cfg.taker_buy_bonus
                tags.append("taker_buy_strong")

        # TakerBuyRatio 趋势上行 (最后 4 根)
        ratios = []
        for k in klines_4h[-4:]:
            if k["volume"] > 0:
                ratios.append(k["taker_buy_volume"] / k["volume"])
        if len(ratios) >= 3 and ratios[-1] > ratios[0]:
            score += cfg.taker_trend_bonus
            tags.append("taker_trending_up")

    return clamp(score, cfg.sm_score_min, cfg.sm_score_max), tags


# ── 妖币检测器: OI 暴增 + 价格平坦 (v2 新增) ──

def compute_oi_surge_score(
    oi_delta_1h: float,        # OI 1h 变动率 (%)
    price_change_1h: float,    # 价格 1h 变动率 (%)
    cfg: HunterConfig = DEFAULT_CONFIG
) -> tuple[float, list[str]]:
    """
    妖币核心信号: OI 暴增但价格不动 → 庄家锁仓吸筹。
    对应妖币指标: OI_Growth_1h > 15% + Price_Change < 3%
    """
    score = 0.0
    tags = []

    if oi_delta_1h > cfg.oi_surge_threshold and abs(price_change_1h) < cfg.oi_surge_price_flat:
        score += cfg.oi_surge_bonus
        tags.append("oi_surge_price_flat")  # 妖币预警: 庄家吸筹

    return score, tags


def compute_vol_oi_ratio_score(
    ticker: dict,
    oi_value_usd: float,
    cfg: HunterConfig = DEFAULT_CONFIG
) -> tuple[float, list[str]]:
    """
    妖币指标: Vol/OI Ratio > 5 → 投机资金异常活跃。
    V/OI = 24h成交额 / OI市值
    """
    score = 0.0
    tags = []

    qv = float(ticker.get("quoteVolume", 0))
    if qv > 0 and oi_value_usd > 0:
        vol_oi_ratio = qv / oi_value_usd

        if vol_oi_ratio > cfg.vol_oi_extreme_threshold:
            score += cfg.vol_oi_extreme_bonus
            tags.append("vol_oi_extreme")
        elif vol_oi_ratio > cfg.vol_oi_ratio_min:
            score += cfg.vol_oi_high_bonus
            tags.append("vol_oi_high")

    return score, tags


def compute_funding_rate_score(
    funding_data: list[dict],
    cfg: HunterConfig = DEFAULT_CONFIG
) -> tuple[float, list[str]]:
    """
    妖币指标: 负资金费率 → 空头拥挤 → 爆空动力。
    Funding_Rate < -0.01% → 做多燃料
    """
    score = 0.0
    tags = []

    if not funding_data:
        return score, tags

    latest_rate = funding_data[-1]["fundingRate"]

    if latest_rate < cfg.funding_rate_extreme_threshold:
        score += cfg.funding_rate_extreme_bonus
        tags.append("funding_extreme_negative")
    elif latest_rate < cfg.funding_rate_negative_threshold:
        score += cfg.funding_rate_bonus
        tags.append("funding_negative")

    return score, tags


# ── Pillar D': Wash Trade Detection ──

def compute_wash_multiplier(
    ticker: dict,
    klines_4h: list[dict],
    oi_value_usd: float,
    cfg: HunterConfig = DEFAULT_CONFIG
) -> tuple[float, list[str]]:
    """
    hunter.go:206-260
    三重刷量检测，乘数叠加。
    """
    multiplier = 1.0
    tags = []
    details = {}

    trades = ticker.get("count", 0)
    qv = float(ticker.get("quoteVolume", 0))

    # Check 1: 微单 (交易笔数多但单笔金额小)
    if trades > cfg.wash_micro_trade_count and qv > 0:
        avg_trade_size = qv / trades
        details["avg_trade_size"] = avg_trade_size
        if avg_trade_size < cfg.wash_avg_trade_size:
            multiplier *= cfg.wash_micro_multiplier
            tags.append("wash_micro_trades")

    # Check 2: 虚假量 (OI/Volume 比值过低)
    if qv > 0 and oi_value_usd > 0:
        oi_vol_ratio = oi_value_usd / qv
        details["oi_vol_ratio"] = oi_vol_ratio
        if oi_vol_ratio < cfg.wash_oi_vol_ratio:
            multiplier *= cfg.wash_fake_vol_multiplier
            tags.append("wash_fake_volume")

    # Check 3: 异常放量 (最后 5 根 K 线中有 3+ 根 >10×均量)
    if len(klines_4h) >= 20:
        avg_vol = sum(k["volume"] for k in klines_4h[:cfg.wash_avg_bars]) / cfg.wash_avg_bars
        spikes = 0
        if avg_vol > 0:
            for k in klines_4h[cfg.wash_avg_bars:]:
                if k["volume"] > avg_vol * cfg.wash_spike_threshold:
                    spikes += 1
        details["volume_spikes"] = spikes
        if spikes >= cfg.wash_spike_count:
            multiplier *= cfg.wash_spike_multiplier
            tags.append("wash_volume_spikes")

    return multiplier, tags, details


# ── Extreme Loss Protection (v4 新增) ──

def compute_extreme_loss_multiplier(
    ticker: dict,
    oi_value_usd: float,
    cfg: HunterConfig = DEFAULT_CONFIG
) -> tuple[float, list[str]]:
    """
    v4: 小币 + 低OI + 极端亏损 → 庄家砸盘/流动性陷阱。
    典型案例: BUSDT OI=$5.8M, 24h=-23.13%, V/OI=15.8
    逻辑: 低OI标的的V/OI高是虚假活跃度，极端亏损说明已无承接盘。
    """
    multiplier = 1.0
    tags = []

    qv = float(ticker.get("quoteVolume", 0))
    pct24h = float(ticker.get("priceChangePercent", 0))
    loss = -pct24h  # 正数=亏损

    if loss <= 0 or oi_value_usd <= 0 or qv <= 0:
        return multiplier, tags

    vol_oi = qv / oi_value_usd

    # P0-红线: 绝对亏损 >20% → 硬杀 (不看OI/Vol)
    # 案例: BILLUSDT(-25.28%), BUSDT(-23.13%)
    if loss > 20.0:
        multiplier *= 0.10
        tags.append("elp_hard_kill")
        return multiplier, tags

    # 严重: OI < $5M + 亏损 >15% + V/OI > 5
    if (oi_value_usd < cfg.elp_oi_severe
            and loss > cfg.elp_loss_severe
            and vol_oi > cfg.elp_volo_min):
        multiplier *= cfg.elp_severe_multiplier
        tags.append("elp_severe")
        return multiplier, tags

    # 中等: OI < $10M + 亏损 >10% + V/OI > 5
    if (oi_value_usd < cfg.elp_oi_moderate
            and loss > cfg.elp_loss_moderate
            and vol_oi > cfg.elp_volo_min):
        multiplier *= cfg.elp_moderate_multiplier
        tags.append("elp_moderate")

    return multiplier, tags


# ══════════════════════════════════════════════════════════════════════
# ── Short-Direction Scoring (mirror of Go computeShort* functions) ──
# ══════════════════════════════════════════════════════════════════════

def compute_short_position_score(
    klines_4h: list[dict],
    price_change_pct: float,
    cfg: HunterConfig = DEFAULT_CONFIG,
    klines_1d: list[dict] | None = None,
    klines_1h: list[dict] | None = None,
    klines_15m: list[dict] | None = None,
    klines_5m: list[dict] | None = None,
) -> tuple[float, list[str]]:
    """
    hunter.go computeShortPositionScore (line 362-455)
    镜像做多位置评分: 评估阻力位接近程度。
    Near 4h resistance (2×ATR) → +25; near support → -25;
    1d resistance → +15; 1h → +10; 15m → +8; 5m → +5;
    chase penalty >50% → -20.
    Clamp [-35, 55].
    """
    if len(klines_4h) < 15:
        return 0.0, []

    score = 0.0
    tags = []

    atr = compute_atr(klines_4h, cfg.atr_period)
    if atr <= 0:
        return 0.0, []

    current_price = klines_4h[-1]["close"]

    # --- 4h resistance/proximity (mirrored from long) ---
    high20 = max(k["high"] for k in klines_4h)
    low20 = min(k["low"] for k in klines_4h)

    # Near resistance → +25 (short opportunity)
    if high20 - current_price <= 2 * atr:
        score += 25.0
        tags.append("near_resistance_4h")

    # Near support → -25 (risky for short)
    if current_price - low20 <= 1.5 * atr:
        score -= 25.0
        tags.append("near_support_4h_penalize")

    # --- 1d resistance ---
    if klines_1d is not None and len(klines_1d) >= 15:
        atr_1d = compute_atr(klines_1d, cfg.atr_period)
        if atr_1d > 0:
            high_1d = max(k["high"] for k in klines_1d)
            if high_1d - current_price <= 2 * atr_1d:
                score += 15.0
                tags.append("near_resistance_1d")

    # --- 1h resistance ---
    if klines_1h is not None and len(klines_1h) >= 15:
        atr_1h = compute_atr(klines_1h, cfg.atr_period)
        if atr_1h > 0:
            high_1h = max(k["high"] for k in klines_1h)
            if high_1h - current_price <= 1 * atr_1h:
                score += 10.0
                tags.append("near_resistance_1h")

    # --- 15m resistance ---
    if klines_15m is not None and len(klines_15m) >= 15:
        atr_15m = compute_atr(klines_15m, cfg.atr_period)
        if atr_15m > 0:
            high_15m = max(k["high"] for k in klines_15m)
            if high_15m - current_price <= 1 * atr_15m:
                score += 8.0
                tags.append("near_resistance_15m")

    # --- 5m resistance ---
    if klines_5m is not None and len(klines_5m) >= 15:
        atr_5m = compute_atr(klines_5m, cfg.atr_period)
        if atr_5m > 0:
            high_5m = max(k["high"] for k in klines_5m)
            if high_5m - current_price <= 1 * atr_5m:
                score += 5.0
                tags.append("near_resistance_5m")

    # Chase penalty: >50% move in 24h (same as long)
    pct24h = abs(price_change_pct)
    if pct24h > cfg.chase_penalty_threshold:
        score += cfg.chase_penalty_bonus  # -20
        tags.append("chase_penalty")

    return clamp(score, -35.0, 55.0), tags


def compute_short_oi_smart_score(
    oi_delta_4h: float,
    oi_value_usd: float,
    price_change_pct: float,
    cfg: HunterConfig = DEFAULT_CONFIG,
) -> tuple[float, list[str]]:
    """
    hunter.go computeShortOISmartScore (line 457-505)
    做空侧 OI 变动分析，使用独立 $1.5M 门槛。
    OI↓ + Price↓ (long liquidation) → +45
    OI↓ + Price↓ moderate → +20
    OI↑ + Price↑ (distribution) → +40
    OI↑ + Price↑ moderate → +20
    """
    score = 0.0
    tags = []

    # v7: 做空侧独立OI门槛 $1.5M
    if oi_value_usd < cfg.oi_threshold_short:
        tags.append("oi_too_low")
        return 0.0, tags

    price_dir = price_change_pct  # >0 涨, <0 跌

    # OI Long Squeeze: OI↓ + Price↓ = long liquidation cascade
    if oi_delta_4h < -10 and price_dir < 0:
        score += 45.0
        tags.append("oi_long_squeeze")
    elif oi_delta_4h < -5 and price_dir < 0:
        score += 20.0
        tags.append("oi_long_squeeze_moderate")
    elif abs(oi_delta_4h) > 10:
        # OI-价格对齐 (short 方向: OI↓+Price↓ or OI↑+Price↑)
        if (price_dir < 0 and oi_delta_4h < 0) or (price_dir > 0 and oi_delta_4h > 0):
            score += 40.0
            tags.append("oi_price_aligned_short")
        # Distribution: OI↑ + Price↑ = smart money building short positions
        if oi_delta_4h > 0 and price_dir > 0:
            score += 40.0
            tags.append("oi_distribution")
    elif abs(oi_delta_4h) > 5:
        score += 15.0
        tags.append("oi_moderate_short")

    return clamp(score, 0.0, 50.0), tags


def compute_short_smart_money_score(
    lsr_data: list[dict],
    klines_4h: list[dict],
    cfg: HunterConfig = DEFAULT_CONFIG,
) -> tuple[float, list[str]]:
    """
    hunter.go computeShortSmartMoneyScore (line 507-592)
    做空侧聪明钱: LSR bearish reversal + Taker 卖出信号。
    """
    score = 0.0
    tags = []

    # --- LSR Signal (mirrored) ---
    if len(lsr_data) >= 2:
        oldest_ratio = lsr_data[0]["longShortRatio"]
        newest_ratio = lsr_data[-1]["longShortRatio"]

        if oldest_ratio > 0:
            lsr_delta_pct = ((newest_ratio - oldest_ratio) / oldest_ratio) * 100

            # Bearish reversal: was bullish (>1.1), now falling
            if oldest_ratio > 1.1 and newest_ratio < oldest_ratio:
                score += 20
                tags.append("lsr_bearish_reversal")

            # Strong bearish momentum
            if lsr_delta_pct < -cfg.lsr_delta_threshold:
                score += 10
                tags.append("lsr_bearish_strong")

            # Bullish momentum (opposing short — weak signal)
            if lsr_delta_pct > cfg.lsr_delta_threshold:
                score += 5
                tags.append("lsr_bullish_weak")

            # Extreme bullish (crowded longs) → dump risk, favor SHORT
            if newest_ratio < 0.5:
                score += 15
                tags.append("lsr_extreme_bullish_short")

            # Extreme bullish (crowded longs) → squeeze/dump potential, favor SHORT
            # LSR > 2.0 = top traders 67%+ long = crowded longs
            if newest_ratio > 2.0:
                score += 15
                tags.append("lsr_crowded_long_favor_short")

    # --- Taker Signal (mirrored: sell-side) ---
    if len(klines_4h) >= 5:
        latest = klines_4h[-1]
        if latest["volume"] > 0:
            taker_ratio = latest["taker_buy_volume"] / latest["volume"]
            # Strong selling (taker buy < 40%)
            if taker_ratio < 0.40:
                score += 10
                tags.append("taker_sell_strong")

        # TakerBuyRatio trending DOWN over 4 bars
        ratios = []
        for k in klines_4h[-4:]:
            if k["volume"] > 0:
                ratios.append(k["taker_buy_volume"] / k["volume"])
        if len(ratios) >= 3 and ratios[-1] < ratios[0]:
            score += 10
            tags.append("taker_trending_down")

        # Consecutive strong selling (3+ bars < 45% taker buy)
        if len(ratios) >= 3:
            strong_bars = sum(1 for r in ratios if r < 0.45)
            if strong_bars >= 3:
                score += 20
                tags.append("taker_sustained_selling")

        # Taker reversal for short (was buying >0.55, now selling <0.45)
        if len(ratios) >= 4 and ratios[0] > 0.55 and ratios[-1] < 0.45:
            score += 10
            tags.append("taker_reversal_short")

    return clamp(score, 0.0, 65.0), tags


def compute_short_elp_multiplier(
    pct_24h: float,
    current_oi: float,
    cfg: HunterConfig = DEFAULT_CONFIG,
) -> tuple[float, list[str]]:
    """
    做空侧 ELP (v7): 用涨幅（而非跌幅）判定极端风险。
    gain>20% → 0.10 (硬杀)
    gain>15% → 0.30 (严重，无需OI条件)
    gain>10%+OI<20M → 0.50 (中等)
    """
    multiplier = 1.0
    tags = []

    if pct_24h > cfg.short_elp_gain_hard:
        multiplier *= 0.10
        tags.append("elp_short_hard_kill")
    elif pct_24h > cfg.short_elp_gain_severe:
        multiplier *= 0.30
        tags.append("elp_short_severe")
    elif pct_24h > cfg.short_elp_gain_moderate and current_oi < cfg.elp_oi_moderate:
        multiplier *= 0.50
        tags.append("elp_short_moderate")

    return multiplier, tags


# ── 综合评分 ──

@dataclass
class ScoredCoin:
    """完整的评分结果。"""
    symbol: str
    ticker: dict
    score: CoinScore


def score_coin(
    symbol: str,
    ticker: dict,
    klines_4h: list[dict],
    lsr_data: list[dict],
    oi_delta_4h: float,
    oi_value_usd: float,
    cfg: HunterConfig = DEFAULT_CONFIG,
    oi_delta_1h: float = 0.0,
    price_change_1h: float = 0.0,
    funding_data: list = None,
) -> ScoredCoin:
    """
    对单个标的执行完整的评分 (v3: Go对齐 + 信号确认过滤)。
    v1: 4支柱 → v2: +妖币检测器 → v3: +信号确认过滤 + 复合上限修正
    """
    if funding_data is None:
        funding_data = []

    sc = CoinScore(symbol=symbol, current_price=float(ticker["lastPrice"]))

    # S-A': Position + OI Smart
    pos_score, pos_tags = compute_position_score(
        klines_4h, float(ticker.get("priceChangePercent", 0)), cfg
    )
    sc.position_score = pos_score
    sc.atr = compute_atr(klines_4h, cfg.atr_period) if len(klines_4h) > cfg.atr_period else 0
    if klines_4h:
        sc.high20 = max(k["high"] for k in klines_4h)
        sc.low20 = min(k["low"] for k in klines_4h)

    oi_score, oi_tags = compute_oi_smart_score(
        oi_delta_4h, oi_value_usd, float(ticker.get("priceChangePercent", 0)), cfg
    )
    sc.oi_smart_score = oi_score
    sc.oi_delta_4h = oi_delta_4h
    sc.oi_value = oi_value_usd

    base_score_50 = clamp((pos_score + oi_score) / 2, -35, cfg.sa_range_max)

    # S-B': Smart Money (v2: taker增强)
    sm_score, sm_tags = compute_smart_money_score(lsr_data, klines_4h, cfg)
    sc.smart_money_score = sm_score
    if lsr_data:
        sc.lsr_oldest = lsr_data[0]["longShortRatio"]
        sc.lsr_newest = lsr_data[-1]["longShortRatio"]
    if klines_4h and klines_4h[-1]["volume"] > 0:
        sc.taker_buy_ratio = klines_4h[-1]["taker_buy_volume"] / klines_4h[-1]["volume"]

    base_score_25 = sm_score * cfg.sb_scale_factor

    # ── 妖币检测器 (v2 新增) ──
    yaobi_score = 0.0
    yaobi_tags = []

    # 妖币信号1: OI暴增+价格平坦 (庄家吸筹)
    surge_score, surge_tags = compute_oi_surge_score(oi_delta_1h, price_change_1h, cfg)
    yaobi_score += surge_score
    yaobi_tags.extend(surge_tags)

    # 妖币信号2: Vol/OI比值异常 (投机资金活跃)
    vol_oi_score, vol_oi_tags = compute_vol_oi_ratio_score(ticker, oi_value_usd, cfg)
    yaobi_score += vol_oi_score
    yaobi_tags.extend(vol_oi_tags)

    # 妖币信号3: 资金费率异常 (空头拥挤)
    fr_score, fr_tags = compute_funding_rate_score(funding_data, cfg)
    yaobi_score += fr_score
    yaobi_tags.extend(fr_tags)

    # 最终合成: 原始4支柱 + 妖币检测器
    composite = clamp(base_score_50 + base_score_25 + yaobi_score,
                      cfg.composite_min, cfg.composite_max)  # v3: 严格上限75

    # ── v5: 三级信号确认过滤器 ──
    # 替代v3二元确认: near_support需累积确认分≥2才不降权
    # oi_moderate单独(分=1)不再足够 → 过滤CRCLUSDT(-1.43%), BILLUSDT(-25.28%)
    confirmation_scores = {
        "oi_accumulation": 3,      # 56.3% 胜率, 最强信号
        "lsr_reversal": 3,         # 72%+ 组合胜率
        "oi_short_squeeze": 3,     # v6: OI↓+price↑, FIDA验证
        "oi_price_aligned": 2,     # 高胜率但样本少
        "lsr_bullish": 2,          # 组合胜率高
        "oi_squeeze_moderate": 2,  # v6: 弱版 squeeze
        "oi_moderate": 1,          # 46.2% 胜率, 单独不够
    }
    CONFIRMATION_THRESHOLD = 2  # near_support 需确认分 ≥ 2

    all_tags = pos_tags + oi_tags + sm_tags + yaobi_tags
    conf_score = sum(confirmation_scores.get(t, 0) for t in all_tags)

    if "near_support" in all_tags and conf_score < CONFIRMATION_THRESHOLD:
        composite *= 0.3
        sc.tags_extra = ["unconfirmed_support"]
    else:
        sc.tags_extra = []

    # C': Cooldown (回测中始终为 1.0)
    sc.cooldown_mod = 1.0

    # D': Wash Trade
    wash_mod, wash_tags, wash_details = compute_wash_multiplier(
        ticker, klines_4h, oi_value_usd, cfg
    )

    # D': Extreme Loss Protection (v4 新增)
    elp_mod, elp_tags = compute_extreme_loss_multiplier(ticker, oi_value_usd, cfg)
    wash_mod *= elp_mod
    wash_tags.extend(elp_tags)

    sc.wash_mod = wash_mod
    sc.wash_details = wash_details

    # 最终得分
    composite_final = 0.0 if sc.cooldown_mod == 0 else composite

    # v6: 宁缺勿滥 — 标记是否携带强信号
    all_tags_for_check = pos_tags + oi_tags + sm_tags
    for t in all_tags_for_check:
        if t in cfg.strong_signals:
            sc.has_strong_signal = True
            break
    sc.final_score = composite_final * sc.cooldown_mod * sc.wash_mod
    sc.tags = pos_tags + oi_tags + sm_tags + wash_tags + yaobi_tags + getattr(sc, 'tags_extra', [])

    return ScoredCoin(symbol=symbol, ticker=ticker, score=sc)


# ══════════════════════════════════════════════════════════════════════
# ── Bidirectional Scoring (Go GetHunterList SHORT block mirror) ──
# ══════════════════════════════════════════════════════════════════════

def score_coin_both_directions(
    symbol: str,
    ticker: dict,
    klines_4h: list[dict],
    lsr_data: list[dict],
    oi_delta_4h: float,
    oi_value_usd: float,
    current_oi: float,
    cfg: HunterConfig = DEFAULT_CONFIG,
    oi_delta_1h: float = 0.0,
    price_change_1h: float = 0.0,
    funding_data: list = None,
    klines_1d: list[dict] | None = None,
    klines_1h: list[dict] | None = None,
    klines_15m: list[dict] | None = None,
    klines_5m: list[dict] | None = None,
) -> ScoredCoin:
    """
    对单个标的执行 LONG + SHORT 双向评分，选较高分方向。
    镜像 Go GetHunterList 的 "FULL BINANCE PATH" 逻辑 (line 696-832)。

    参数:
        symbol:          交易对
        ticker:          24hr ticker 数据
        klines_4h:       4h K线
        lsr_data:        LSR 历史
        oi_delta_4h:     OI 4h 变动率 (%)
        oi_value_usd:    当前 OI (USD) — 用于做多侧阈值
        current_oi:      当前 OI (USD) — 用于 ELP / 做空侧阈值
        cfg:             配置
        oi_delta_1h:     OI 1h 变动率 (妖币检测)
        price_change_1h: 价格 1h 变动率 (妖币检测)
        funding_data:    资金费率数据
        klines_1d/1h/15m/5m: 多时间框架 K线 (做空位置评分可选)
    """
    if funding_data is None:
        funding_data = []

    pct24h = float(ticker.get("priceChangePercent", 0))

    # ── 评分中间数据 ──
    sc = CoinScore(symbol=symbol, current_price=float(ticker["lastPrice"]))
    sc.atr = compute_atr(klines_4h, cfg.atr_period) if len(klines_4h) > cfg.atr_period else 0
    if klines_4h:
        sc.high20 = max(k["high"] for k in klines_4h)
        sc.low20 = min(k["low"] for k in klines_4h)
    sc.oi_delta_4h = oi_delta_4h
    sc.oi_value = oi_value_usd

    # ══════════ LONG DIRECTION ══════════
    pos_score, pos_tags = compute_position_score(klines_4h, pct24h, cfg)
    sc.position_score = pos_score

    oi_score, oi_tags = compute_oi_smart_score(oi_delta_4h, oi_value_usd, pct24h, cfg)
    sc.oi_smart_score = oi_score

    base_50 = clamp((pos_score + oi_score) / 2, -35, cfg.sa_range_max)

    sm_score, sm_tags = compute_smart_money_score(lsr_data, klines_4h, cfg)
    sc.smart_money_score = sm_score
    if lsr_data:
        sc.lsr_oldest = lsr_data[0]["longShortRatio"]
        sc.lsr_newest = lsr_data[-1]["longShortRatio"]
    if klines_4h and klines_4h[-1]["volume"] > 0:
        sc.taker_buy_ratio = klines_4h[-1]["taker_buy_volume"] / klines_4h[-1]["volume"]

    base_25 = sm_score * cfg.sb_scale_factor

    # 妖币检测器
    yaobi_score = 0.0
    yaobi_tags = []
    surge_score, surge_tags = compute_oi_surge_score(oi_delta_1h, price_change_1h, cfg)
    yaobi_score += surge_score
    yaobi_tags.extend(surge_tags)
    vol_oi_score, vol_oi_tags = compute_vol_oi_ratio_score(ticker, oi_value_usd, cfg)
    yaobi_score += vol_oi_score
    yaobi_tags.extend(vol_oi_tags)
    fr_score, fr_tags = compute_funding_rate_score(funding_data, cfg)
    yaobi_score += fr_score
    yaobi_tags.extend(fr_tags)

    long_composite = clamp(base_50 + base_25 + yaobi_score,
                           cfg.composite_min, cfg.composite_max)

    # Long signal confirmation filter (near_support without confirmation → 0.5x)
    all_long_tags = pos_tags + oi_tags + sm_tags
    long_confirming = {
        "oi_accumulation", "oi_price_aligned", "oi_moderate",
        "lsr_reversal", "lsr_bullish",
        "taker_buy_strong", "taker_sustained_buying",
    }
    has_near_support = any(
        t in ("near_support_4h", "near_support_1d", "near_support_1h")
        for t in all_long_tags
    )
    if has_near_support and not (set(all_long_tags) & long_confirming):
        long_composite *= 0.5

    # Long ELP (Go line 743-755)
    loss24h = -pct24h
    long_elp_tags = []
    if loss24h > 20.0:
        long_composite *= 0.10
        long_elp_tags.append("elp_hard_kill")
    elif loss24h > 10.0 and current_oi < 5_000_000:
        long_composite *= 0.30
        long_elp_tags.append("elp_severe")
    elif loss24h > 10.0 and current_oi < 20_000_000:
        long_composite *= 0.50
        long_elp_tags.append("elp_moderate")

    # ══════════ SHORT DIRECTION ══════════
    short_pos_score, short_pos_tags = compute_short_position_score(
        klines_4h, pct24h, cfg,
        klines_1d=klines_1d, klines_1h=klines_1h,
        klines_15m=klines_15m, klines_5m=klines_5m,
    )
    sc.short_position_score = short_pos_score

    short_oi_score, short_oi_tags = compute_short_oi_smart_score(
        oi_delta_4h, current_oi, pct24h, cfg,
    )
    sc.short_oi_smart_score = short_oi_score

    short_base_50 = clamp((short_pos_score + short_oi_score) / 2, -35, 50.0)

    short_sm_score, short_sm_tags = compute_short_smart_money_score(lsr_data, klines_4h, cfg)
    sc.short_smart_money_score = short_sm_score

    short_base_25 = short_sm_score * cfg.sb_scale_factor

    short_composite = clamp(short_base_50 + short_base_25,
                            cfg.composite_min, cfg.composite_max)

    # Short signal confirmation filter (near_resistance without confirmation → 0.5x)
    all_short_tags = short_pos_tags + short_oi_tags + short_sm_tags
    short_confirming = {
        "oi_distribution", "oi_price_aligned_short", "oi_moderate_short",
        "lsr_bearish_reversal", "lsr_bearish_strong",
        "taker_sell_strong", "taker_sustained_selling",
    }
    has_near_resistance = any(
        t in ("near_resistance_4h", "near_resistance_1d", "near_resistance_1h")
        for t in all_short_tags
    )
    if has_near_resistance and not (set(all_short_tags) & short_confirming):
        short_composite *= 0.5

    # Short ELP (v7: gain-side thresholds, Go line 809-819)
    short_elp_mult, short_elp_tags = compute_short_elp_multiplier(pct24h, current_oi, cfg)
    short_composite *= short_elp_mult

    # ══════════ SHARED Pillar C/D ══════════
    sc.cooldown_mod = 1.0  # 回测中始终 1.0
    wash_mod, wash_tags, wash_details = compute_wash_multiplier(
        ticker, klines_4h, oi_value_usd, cfg
    )
    elp_mod, elp_tags = compute_extreme_loss_multiplier(ticker, oi_value_usd, cfg)
    wash_mod *= elp_mod
    wash_tags.extend(elp_tags)

    sc.wash_mod = wash_mod
    sc.wash_details = wash_details

    # Apply shared multipliers
    if sc.cooldown_mod == 0:
        long_composite = 0.0
        short_composite = 0.0

    long_final = long_composite * sc.cooldown_mod * wash_mod
    short_final = short_composite * sc.cooldown_mod * wash_mod

    # ══════════ PICK DOMINANT DIRECTION ══════════
    sc.short_final_score = short_final
    sc.short_tags = short_pos_tags + short_oi_tags + short_sm_tags + short_elp_tags + wash_tags

    long_all_tags = pos_tags + oi_tags + sm_tags + long_elp_tags + wash_tags + yaobi_tags

    if short_final > long_final:
        sc.final_score = short_final
        sc.tags = sc.short_tags
        sc.direction = "SHORT"
    else:
        sc.final_score = long_final
        sc.tags = long_all_tags
        sc.direction = "LONG"

    # v6: 宁缺勿滥 — 标记是否携带强信号 (long/short 各自独立)
    for t in pos_tags + oi_tags + sm_tags:
        if t in cfg.strong_signals:
            sc.has_strong_signal = True
            break
    for t in short_oi_tags + short_sm_tags:
        if t in cfg.short_strong_signals:
            sc.has_short_strong_signal = True
            break

    return ScoredCoin(symbol=symbol, ticker=ticker, score=sc)
