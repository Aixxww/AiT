package local

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Aixxww/AiT/store"
)

// Hunter scoring parameters (#155). The CoinSourceEditor "advanced filters"
// persist these store.HunterConfig fields; ResolveHunterParams is the single
// place they are read, for both Hunter scorers (ScoreHunterFromSnapshot and
// the local-provider GetHunterList). Every unset/zero field resolves to the
// behavior the scorers had before #155 (owner decision on #155): new
// features are off until set, existing thresholds keep their old values.

// Hunter strategy modes.
const (
	HunterStrategyModeDefault  = "default"
	HunterStrategyModeBreakout = "breakout"
)

// Defaults that preserve the pre-#155 hardcoded behavior.
const (
	hunterDefaultChasePct        = 50.0 // chase penalty threshold (24h |change| %)
	hunterDefaultBBCoarsePct     = 25.0 // breakout funnel: keep the bottom 25% BB width
	hunterDefaultBBCacheTTL      = 180 * time.Second
	hunterDefaultLoneBreakerPct  = 15.0 // breakout bypass: 1h OI delta %
	hunterRSIAdjustPoints        = 5.0
	hunterFundingThreshold       = 0.0005 // 0.05% per settlement
	hunterFundingAdjustPoints    = 5.0
	hunterBreakoutPillarCapBonus = 20.0
	hunterBaseSqueezePillarCap   = 25.0
	hunterLoneBreakerMinWash     = 0.20
)

// HunterWashThresholds are the wash-trade detection thresholds of one
// sensitivity level. "medium" is the pre-#155 hardcoded set.
type HunterWashThresholds struct {
	TradeCount  int64   // micro-trades check: trades above this ...
	AvgOrderUSD float64 // ... with an average order below this
	OIVolRatio  float64 // fake-volume check: OI/volume below this
	SpikeMult   float64 // volume-spike check: volume above this multiple of the average
}

var hunterWashLevels = map[string]HunterWashThresholds{
	"low":    {TradeCount: 2_000_000, AvgOrderUSD: 2.5, OIVolRatio: 0.005, SpikeMult: 20},
	"medium": {TradeCount: 1_000_000, AvgOrderUSD: 5, OIVolRatio: 0.01, SpikeMult: 10},
	"high":   {TradeCount: 500_000, AvgOrderUSD: 10, OIVolRatio: 0.02, SpikeMult: 5},
}

// HunterParams is the resolved, behavior-preserving view of the Hunter
// advanced-filter settings.
type HunterParams struct {
	RSIOversold   float64 // 0 = RSI oversold adjustment off
	RSIOverbought float64 // 0 = RSI overbought adjustment off
	FundingSignal bool
	ChasePct      float64
	WashLevel     string
	Wash          HunterWashThresholds
	Cooldown      bool
	MinTradeCount int64 // 0 = no filter
	// PositionTimeframes limits the support/resistance timeframes of the
	// position pillar; nil = every timeframe the scorer has.
	PositionTimeframes map[string]bool
	Breakout           bool
	BBCoarsePct        float64
	BBCacheTTL         time.Duration
	OILoneBreakerPct   float64
}

// ResolveHunterParams resolves cfg (nil-safe).
func ResolveHunterParams(cfg *store.HunterConfig) HunterParams {
	p := HunterParams{
		ChasePct:         hunterDefaultChasePct,
		WashLevel:        "medium",
		Wash:             hunterWashLevels["medium"],
		Cooldown:         true,
		BBCoarsePct:      hunterDefaultBBCoarsePct,
		BBCacheTTL:       hunterDefaultBBCacheTTL,
		OILoneBreakerPct: hunterDefaultLoneBreakerPct,
	}
	if cfg == nil {
		return p
	}
	if cfg.RSIOversoldThreshold > 0 {
		p.RSIOversold = cfg.RSIOversoldThreshold
	}
	if cfg.RSIOverboughtThreshold > 0 {
		p.RSIOverbought = cfg.RSIOverboughtThreshold
	}
	p.FundingSignal = cfg.EnableFundingRateSignal
	if cfg.Max24hChange > 0 {
		p.ChasePct = cfg.Max24hChange
	}
	if lvl := strings.ToLower(strings.TrimSpace(cfg.WashTradeSensitivity)); lvl != "" {
		if w, ok := hunterWashLevels[lvl]; ok {
			p.WashLevel, p.Wash = lvl, w
		}
	}
	// Cooldown (#155 owner decision): the legacy enable_cooldown key was
	// serialized as false for every config, so it cannot express "off";
	// only the explicit disable_cooldown key turns the cooldown off.
	p.Cooldown = !cfg.DisableCooldown
	if cfg.MinTradeCount > 0 {
		p.MinTradeCount = cfg.MinTradeCount
	}
	for _, tf := range cfg.PositionTimeframes {
		tf = strings.ToLower(strings.TrimSpace(tf))
		if tf == "" {
			continue
		}
		if p.PositionTimeframes == nil {
			p.PositionTimeframes = map[string]bool{}
		}
		p.PositionTimeframes[tf] = true
	}
	p.Breakout = strings.EqualFold(strings.TrimSpace(cfg.StrategyMode), HunterStrategyModeBreakout)
	if cfg.BBWidthCoarseFilter > 0 {
		p.BBCoarsePct = cfg.BBWidthCoarseFilter
	}
	if cfg.BBWidthCacheTTL > 0 {
		p.BBCacheTTL = time.Duration(cfg.BBWidthCacheTTL) * time.Second
	}
	if cfg.OILoneBreakerThreshold > 0 {
		p.OILoneBreakerPct = cfg.OILoneBreakerThreshold
	}
	return p
}

// Key identifies the resolved parameters (the local-provider result cache
// is keyed by it, so a changed setting takes effect at once).
func (p HunterParams) Key() string {
	tfs := make([]string, 0, len(p.PositionTimeframes))
	for tf := range p.PositionTimeframes {
		tfs = append(tfs, tf)
	}
	sort.Strings(tfs)
	return fmt.Sprintf("rsi=%g/%g|fund=%v|chase=%g|wash=%s|cd=%v|mtc=%d|tf=%s|bo=%v/%g/%s/%g",
		p.RSIOversold, p.RSIOverbought, p.FundingSignal, p.ChasePct, p.WashLevel, p.Cooldown,
		p.MinTradeCount, strings.Join(tfs, ","), p.Breakout, p.BBCoarsePct, p.BBCacheTTL, p.OILoneBreakerPct)
}

// positionTF reports whether the position pillar scores timeframe tf.
func (p HunterParams) positionTF(tf string) bool {
	return p.PositionTimeframes == nil || p.PositionTimeframes[tf]
}

// tradeCountOK reports whether a symbol passes min_trade_count. An unknown
// count (0, e.g. the CoinGecko fallback ticker) is not filtered.
func (p HunterParams) tradeCountOK(count int64) bool {
	return p.MinTradeCount <= 0 || count <= 0 || count >= p.MinTradeCount
}

func (p HunterParams) cooldownMultiplier(symbol string) float64 {
	if !p.Cooldown {
		return 1.0
	}
	return globalCooldown.getCooldownMultiplier(symbol)
}

func (p HunterParams) oiThresholdReduction(symbol string) float64 {
	if !p.Cooldown {
		return 1.0
	}
	return globalCooldown.getOIThresholdReduction(symbol)
}

// rsiAdjust is the RSI signal: RSI below the oversold threshold favours
// LONG (+5) and penalises SHORT (-5); above the overbought threshold the
// mirror. Each side is off while its threshold is unset.
func (p HunterParams) rsiAdjust(rsi float64, ok bool) (long, short float64, longTags, shortTags []string) {
	if !ok {
		return
	}
	if p.RSIOversold > 0 && rsi < p.RSIOversold {
		long, short = hunterRSIAdjustPoints, -hunterRSIAdjustPoints
		longTags, shortTags = []string{"rsi_oversold"}, []string{"rsi_oversold_short_penalty"}
	} else if p.RSIOverbought > 0 && rsi > p.RSIOverbought {
		long, short = -hunterRSIAdjustPoints, hunterRSIAdjustPoints
		longTags, shortTags = []string{"rsi_overbought_long_penalty"}, []string{"rsi_overbought"}
	}
	return
}

// fundingAdjust is the funding-rate signal: funding at or below -0.05%
// (crowded shorts) favours LONG +5, at or above +0.05% favours SHORT +5.
func (p HunterParams) fundingAdjust(funding float64) (long, short float64, longTags, shortTags []string) {
	if !p.FundingSignal {
		return
	}
	switch {
	case funding <= -hunterFundingThreshold:
		return hunterFundingAdjustPoints, 0, []string{"funding_negative_long_bias"}, nil
	case funding >= hunterFundingThreshold:
		return 0, hunterFundingAdjustPoints, nil, []string{"funding_positive_short_bias"}
	}
	return
}

func (p HunterParams) squeezePillarCap() float64 {
	if p.Breakout {
		return hunterBaseSqueezePillarCap + hunterBreakoutPillarCapBonus
	}
	return hunterBaseSqueezePillarCap
}

// hunterRSI is RSI14 over closes (Wilder); ok is false with too few bars.
func hunterRSI(closes []float64) (float64, bool) {
	const period = 14
	if len(closes) < period+1 {
		return 0, false
	}
	var gains, losses float64
	for i := 1; i <= period; i++ {
		ch := closes[i] - closes[i-1]
		if ch > 0 {
			gains += ch
		} else {
			losses -= ch
		}
	}
	avgGain, avgLoss := gains/period, losses/period
	for i := period + 1; i < len(closes); i++ {
		ch := closes[i] - closes[i-1]
		g, l := 0.0, 0.0
		if ch > 0 {
			g = ch
		} else {
			l = -ch
		}
		avgGain = (avgGain*(period-1) + g) / period
		avgLoss = (avgLoss*(period-1) + l) / period
	}
	if avgLoss == 0 {
		if avgGain == 0 {
			return 50, true
		}
		return 100, true
	}
	return 100 - 100/(1+avgGain/avgLoss), true
}

// ---- Breakout mode (#155, revived spec 60443e1) ----

// hunterBBWidthMinBars is the fewest bars a BB-width percentile is computed
// from. The spec used 288 5m bars (24h); the scorers hold 50 5m bars, so
// the percentile ranks the current width among the widths available (about
// 4h of 5m bars) — owner-approved limitation.
const hunterBBWidthMinBars = 30

// bbWidthPercentile ranks the latest BB(20, 2) width among the widths of
// every 20-bar window of closes: 0 = narrowest, 100 = widest.
func bbWidthPercentile(closes []float64) (float64, bool) {
	if len(closes) < hunterBBWidthMinBars {
		return 0, false
	}
	var widths []float64
	for end := 20; end <= len(closes); end++ {
		if w := bbWidthOfWindow(closes[end-20 : end]); w > 0 {
			widths = append(widths, w)
		}
	}
	if len(widths) < 2 {
		return 0, false
	}
	cur := widths[len(widths)-1]
	eps := cur * 1e-9 // equal widths (float noise) rank as equal
	below := 0
	for _, w := range widths[:len(widths)-1] {
		if w < cur-eps {
			below++
		}
	}
	return float64(below) / float64(len(widths)-1) * 100, true
}

func bbWidthOfWindow(closes []float64) float64 {
	n := float64(len(closes))
	sum := 0.0
	for _, c := range closes {
		sum += c
	}
	mid := sum / n
	if mid == 0 {
		return 0
	}
	v := 0.0
	for _, c := range closes {
		v += (c - mid) * (c - mid)
	}
	sd := math.Sqrt(v / n)
	return 4 * sd / mid * 100 // (upper-lower)/middle with mult 2.0
}

// bbPercentileCache holds BB-width percentiles for BBCacheTTL.
type bbPercentileCache struct {
	mu      sync.Mutex
	entries map[string]bbPercentileEntry
	now     func() time.Time
}

type bbPercentileEntry struct {
	pct float64
	ok  bool
	at  time.Time
}

var hunterBBPercentileCache = &bbPercentileCache{entries: map[string]bbPercentileEntry{}, now: time.Now}

func (c *bbPercentileCache) get(symbol string, ttl time.Duration, compute func() (float64, bool)) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if e, hit := c.entries[symbol]; hit && now.Sub(e.at) < ttl {
		return e.pct, e.ok
	}
	pct, ok := compute()
	c.entries[symbol] = bbPercentileEntry{pct: pct, ok: ok, at: now}
	return pct, ok
}

// breakoutFunnel is Phase 1 of breakout mode: a symbol enters scoring when
// its BB-width percentile is at or below the coarse filter, or through the
// lone-breaker bypass (1h OI delta above the threshold). A symbol without a
// computable percentile enters only through the bypass.
func (p HunterParams) breakoutFunnel(symbol string, closes5m []float64, oiDelta1h float64) (pct float64, pctOK, pass, loneBreaker bool) {
	pct, pctOK = hunterBBPercentileCache.get(symbol, p.BBCacheTTL, func() (float64, bool) {
		return bbWidthPercentile(closes5m)
	})
	if pctOK && pct <= p.BBCoarsePct {
		return pct, pctOK, true, false
	}
	if oiDelta1h > p.OILoneBreakerPct {
		return pct, pctOK, true, true
	}
	return pct, pctOK, false, false
}

// breakoutSqueezeBonus is Phase 2: the squeeze-pillar bonus by percentile.
func breakoutSqueezeBonus(pct float64, ok bool) (float64, []string) {
	if !ok {
		return 0, nil
	}
	switch {
	case pct <= 5:
		return 20, []string{"bb_width_extreme_squeeze"}
	case pct <= 10:
		return 12, []string{"bb_width_tight"}
	case pct <= 20:
		return 5, []string{"bb_width_narrow"}
	}
	return 0, nil
}

// hasBreakoutSqueezeTag reports a squeeze signal strong enough for the
// resistance exemption and the lone-breaker check.
func hasBreakoutSqueezeTag(tags []string) bool {
	for _, t := range tags {
		switch t {
		case "bb_squeeze_15m", "bb_squeeze_5m", "bb_width_extreme_squeeze", "bb_width_tight":
			return true
		}
	}
	return false
}

// loneBreakerKeeps is the post-scoring check for a symbol that entered via
// the bypass: it must show a squeeze, a final score >= 15 and a wash
// multiplier above 0.20.
func loneBreakerKeeps(tags []string, finalScore, washMod float64) bool {
	return hasBreakoutSqueezeTag(tags) && finalScore >= 15 && washMod > hunterLoneBreakerMinWash
}

func closesOfKlineBars(bars []klineBar) []float64 {
	out := make([]float64, len(bars))
	for i, b := range bars {
		out[i] = b.Close
	}
	return out
}
