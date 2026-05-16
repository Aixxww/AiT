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

    if abs(oi_delta_4h) > cfg.oi_strong_delta:
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

    # ── Taker Signal ──
    if len(klines_4h) >= 5:
        latest = klines_4h[-1]
        if latest["volume"] > 0:
            taker_ratio = latest["taker_buy_volume"] / latest["volume"]
            if taker_ratio > cfg.taker_buy_threshold:
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
) -> ScoredCoin:
    """
    对单个标的执行完整的 4 支柱评分。
    对应 hunter.go:294-329。
    """
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

    # S-B': Smart Money
    sm_score, sm_tags = compute_smart_money_score(lsr_data, klines_4h, cfg)
    sc.smart_money_score = sm_score
    if lsr_data:
        sc.lsr_oldest = lsr_data[0]["longShortRatio"]
        sc.lsr_newest = lsr_data[-1]["longShortRatio"]
    if klines_4h and klines_4h[-1]["volume"] > 0:
        sc.taker_buy_ratio = klines_4h[-1]["taker_buy_volume"] / klines_4h[-1]["volume"]

    base_score_25 = sm_score * cfg.sb_scale_factor

    composite = clamp(base_score_50 + base_score_25, cfg.composite_min, cfg.composite_max)

    # C': Cooldown (回测中始终为 1.0)
    sc.cooldown_mod = 1.0

    # D': Wash Trade
    wash_mod, wash_tags, wash_details = compute_wash_multiplier(
        ticker, klines_4h, oi_value_usd, cfg
    )
    sc.wash_mod = wash_mod
    sc.wash_details = wash_details

    # 最终得分
    composite_final = 0.0 if sc.cooldown_mod == 0 else composite
    sc.final_score = composite_final * sc.cooldown_mod * sc.wash_mod
    sc.tags = pos_tags + oi_tags + sm_tags + wash_tags

    return ScoredCoin(symbol=symbol, ticker=ticker, score=sc)
