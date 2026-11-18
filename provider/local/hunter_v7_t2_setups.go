package local

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ============================================================================
// Hunter v7 — Task-2 scanner transplant (M3)
// ============================================================================
// A spec-literal port of the Task-2 whale-ambush scanner's selection rules
// (binance_whale_ambush_monitor.py, STRATEGY_VERSION v0.7.4-observe-chronic)
// into hunter_v7 as a native setup module. The single source of truth is
// T2_SCANNER_RULES_SPEC.md; section references below point at it.
//
// Architecture mirrors the Python original: the family functions consume a
// feature snapshot (the equivalent of the Python snapshot dict), never raw
// exchange responses. Feature construction from raw bars lives in
// BuildV7T2KlineFeatures / BuildV7T2OIFeatures / BuildV7T2OIAccFeatures /
// BuildV7T2DepthFeatures (spec section 2) and is exercised against the
// scanner's own stored payloads by cmd/t2_m3_recall's parity mode.
//
// Documented readings where the spec is ambiguous (see M3_REPORT.md):
//   - The main-chain squeeze (section 3 step 4), the main-chain bb_score and
//     the secondary-chain bb_score all read the 15m bb_width_ratio: the
//     trigger, the direction filter and every explicitly-named bb gate in
//     the spec are 15m, and no other timeframe is named for these.
//   - The secondary chain's long/short triggers are k15.resistance /
//     k15.support, the same trigger definition used by sections 4.1-4.3.
//   - The secondary chain's volume_ratio is max(k5, k15), the convention
//     sections 4.2 and 4.3 state explicitly for their own functions.
//   - The canonical snapshot "price" is k5.price (the most granular last
//     close); the 15m direction filter uses k15.price, matching the Python
//     function signature fifteen_minute_direction_ok(k15, direction).
//   - compression_score reads the 15m bb_width_ratio (payload-only field,
//     no gate consumes it).
//
// The module is disabled unless V7Config.T2 is set with Enabled=true; the
// production config paths never set it by default.

// ----------------------------------------------------------------------------
// Config
// ----------------------------------------------------------------------------

// V7T2Config controls the transplanted Task-2 setup families.
type V7T2Config struct {
	// Enabled turns the module on. Zero value = disabled.
	Enabled bool `json:"enabled"`
	// EnabledSetups is the setup-family checklist: setup_type strings
	// (spec section 4.6) that may be emitted. Empty = all 14 transplanted
	// setup types enabled. A signal whose setup_type is not listed is
	// dropped after its family function produced it (no re-routing),
	// mirroring a scanner build with that family compiled out.
	EnabledSetups []string `json:"enabled_setups,omitempty"`
	// SymbolValidityGate rejects symbols that do not match the Binance
	// USDT-perpetual symbol pattern. The Python scanner has no such gate
	// and has emitted dirty symbols in production (spec section 8 item
	// 12), so this gate defaults to OFF and never affects recall runs.
	SymbolValidityGate bool `json:"symbol_validity_gate,omitempty"`
	// ActivitySymbols mirrors the scanner's ops-maintained activity
	// exclusion list (spec section 1.5): symbols listed here are excluded
	// at the universe level and their live events are invalidated.
	ActivitySymbols []string `json:"activity_symbols,omitempty"`

	// Lifecycle parameters (U1/P1, plan overlap: F13 literals into config).
	// Zero values select the scanner constants, so an unset config keeps
	// the transplant's behaviour exactly.
	Lifecycle V7T2LifecycleParams `json:"lifecycle,omitempty"`
}

// V7T2LifecycleParams carries the spec section 5 / 6.2 lifecycle constants.
// Every zero value means "use the scanner constant".
type V7T2LifecycleParams struct {
	MinAlertScore    float64 `json:"min_alert_score,omitempty"`    // ARMED label threshold (scanner MIN_ALERT_SCORE 82)
	MinWatchScore    float64 `json:"min_watch_score,omitempty"`    // watch_promoted notify band floor (72)
	ArmedExpiryHours float64 `json:"armed_expiry_hours,omitempty"` // EVENT_EXPIRY_HOURS (4)
	WatchExpiryHours float64 `json:"watch_expiry_hours,omitempty"` // WATCH expiry (2)
	CooldownMinutes  float64 `json:"cooldown_minutes,omitempty"`   // COOLDOWN_MINUTES (60)
	// ObserveOnlySetups replaces the section 6.2 OBSERVE_ONLY_SETUPS list
	// when ObserveOnlyOverride is true (so an explicitly empty list is
	// possible); otherwise the scanner list applies (OD-5: values kept).
	ObserveOnlyOverride bool     `json:"observe_only_override,omitempty"`
	ObserveOnlySetups   []string `json:"observe_only_setups,omitempty"`
	// DisableTrigger turns off the section 5.3 trigger stage (U1/P1);
	// default false = WATCH and ARMED events become TRIGGERED when the
	// price reaches the signal entry.
	DisableTrigger bool `json:"disable_trigger,omitempty"`
	// TriggeredSurfaceMinutes is how long after triggering a TRIGGERED
	// event keeps being emitted to the pipeline (default 15 = one scanner
	// scan interval, spec section 5.5).
	TriggeredSurfaceMinutes float64 `json:"triggered_surface_minutes,omitempty"`
}

// Default lifecycle constants (scanner values, spec sections 0, 5, 5.5).
const (
	v7T2WatchExpiryHours            = 2.0
	v7T2DefaultTriggeredSurfaceMins = 15.0
)

func (p V7T2LifecycleParams) alertScore() float64 {
	if p.MinAlertScore > 0 {
		return p.MinAlertScore
	}
	return v7T2MinAlertScore
}
func (p V7T2LifecycleParams) watchScore() float64 {
	if p.MinWatchScore > 0 {
		return p.MinWatchScore
	}
	return v7T2MinWatchScore
}
func (p V7T2LifecycleParams) armedExpiry() time.Duration {
	h := p.ArmedExpiryHours
	if h <= 0 {
		h = v7T2EventExpiryHours
	}
	return time.Duration(h * float64(time.Hour))
}
func (p V7T2LifecycleParams) watchExpiry() time.Duration {
	h := p.WatchExpiryHours
	if h <= 0 {
		h = v7T2WatchExpiryHours
	}
	return time.Duration(h * float64(time.Hour))
}
func (p V7T2LifecycleParams) cooldown() time.Duration {
	m := p.CooldownMinutes
	if m <= 0 {
		m = v7T2CooldownMinutes
	}
	return time.Duration(m * float64(time.Minute))
}
func (p V7T2LifecycleParams) triggeredSurface() time.Duration {
	m := p.TriggeredSurfaceMinutes
	if m <= 0 {
		m = v7T2DefaultTriggeredSurfaceMins
	}
	return time.Duration(m * float64(time.Minute))
}
func (p V7T2LifecycleParams) observeOnly(setup string) bool {
	if !p.ObserveOnlyOverride {
		return v7T2ObserveOnlySetups[setup]
	}
	for _, s := range p.ObserveOnlySetups {
		if s == setup {
			return true
		}
	}
	return false
}

// lifecycle returns the lifecycle parameters (zero value when cfg is nil).
func (c *V7T2Config) lifecycle() V7T2LifecycleParams {
	if c == nil {
		return V7T2LifecycleParams{}
	}
	return c.Lifecycle
}

// V7T2SetupTypes lists every setup_type the transplanted families can emit
// (spec section 4.6), in dispatcher evaluation order.
var V7T2SetupTypes = []string{
	"momentum_continuation_long", "momentum_continuation_short",
	"momentum_continuation_watch_long", "momentum_continuation_watch_short",
	"bb_oi_accumulation_long", "bb_oi_accumulation_short",
	"confirmed_breakout_long", "confirmed_breakout_short",
	"reclaim_breakout_long", "reclaim_breakout_short",
	"trend_pullback_long", "trend_pullback_short",
	"ignition_early_long", "ignition_early_short",
	"compression_oi_watch_long", "compression_oi_watch_short",
}

// v7T2ObserveOnlySetups is the spec section 6.2 OBSERVE_ONLY_SETUPS list.
// SPEC-FAITHFUL ASYMMETRY: trend_pullback_short, reclaim_breakout_short and
// compression_oi_watch_short are deliberately NOT in this list — in the
// production scanner those three remain ARMED-capable while their long
// twins (and both confirmed_breakout directions) are forced to WATCH.
// Do not "fix" the asymmetry; it is the production behavior.
var v7T2ObserveOnlySetups = map[string]bool{
	"trend_pullback_long":       true,
	"reclaim_breakout_long":     true,
	"confirmed_breakout_long":   true,
	"confirmed_breakout_short":  true,
	"compression_oi_watch_long": true,
}

// Scanner constants (spec section 0). Names keep the Python identifiers.
const (
	v7T2LowCapUSD              = 500_000_000.0
	v7T2MinVolumeUSD           = 10_000_000.0
	v7T2EventExpiryHours       = 4.0
	v7T2CooldownMinutes        = 60.0
	v7T2MinAlertScore          = 82.0
	v7T2MinWatchScore          = 72.0
	v7T2RVDemoteScoreKeep      = 75.0
	v7T2MomentumWatchMinLatest = 0.515
	v7T2MinRR                  = 1.8 // payload rr constant; never recomputed (spec section 7)
	v7T2TP2Multiple            = 3.0
	v7T2MaxTrackingChangePct   = 120.0
	v7T2MinOINotionalUSD       = 1_500_000.0
	v7T2MaxOINotionalUSD       = 100_000_000.0
	v7T2MinOIVolumeRatio       = 0.01
	v7T2StrategyVersion        = "v0.7.4-observe-chronic"
)

// familyEnabled reports whether a setup_type passes the config checklist.
func (c *V7T2Config) familyEnabled(setupType string) bool {
	if c == nil || len(c.EnabledSetups) == 0 {
		return true
	}
	for _, s := range c.EnabledSetups {
		if s == setupType {
			return true
		}
	}
	return false
}

// activityExcluded reports whether the symbol is on the activity list.
func (c *V7T2Config) activityExcluded(symbol string) bool {
	if c == nil {
		return false
	}
	for _, s := range c.ActivitySymbols {
		if s == symbol {
			return true
		}
	}
	return false
}

// v7T2ValidSymbolPattern matches Binance USDT-margined perpetual symbols:
// uppercase alphanumerics ending in USDT (covers 1000PEPEUSDT-style names).
var v7T2ValidSymbolPattern = regexp.MustCompile(`^[0-9A-Z]+USDT$`)

// V7T2SymbolValid applies the optional symbol-validity gate.
func V7T2SymbolValid(symbol string) bool {
	return v7T2ValidSymbolPattern.MatchString(symbol)
}

// ----------------------------------------------------------------------------
// Snapshot / feature types (mirror the Python snapshot dict, spec section 9.1)
// ----------------------------------------------------------------------------

// V7T2Bar is one OHLCV bar. Field order mirrors the scanner's stored rows.
type V7T2Bar struct {
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime int64 // ms
	// TakerBuyShare is the bar's taker-buy share of volume in [0,1] when the
	// source provides it (used only to rebuild the taker series in the
	// datafetch adapter; 0 = unknown).
	TakerBuyShare float64
}

// V7T2KlineFeatures mirrors kline_features() output (spec section 2.3).
// HasEMA is false when the EMA fields are absent in the source dict; the
// 15m direction filter fails open in that case (spec section 2.8).
type V7T2KlineFeatures struct {
	Price         float64
	ATR           float64 // atr(rows, 20)
	ATRPct        float64
	ATR14         float64
	RSI           float64
	BBWidth       float64
	BBWidthMin50  float64
	BBWidthRatio  float64
	BBPercentile  float64 // ATR%-series p25; misnamed upstream, unused by gates
	Range6Pct     float64
	VolumeRatio   float64
	Resistance    float64
	Support       float64
	PriorHigh     float64
	PriorLow      float64
	EMA20         float64
	EMA50         float64
	EMATrendPct   float64
	Change3Pct    float64
	Change12Pct   float64
	Change24Pct   float64
	LastCloseTime int64
	HasEMA        bool
}

// V7T2OIFeatures mirrors oi_features() output (spec section 2.4), computed
// from the 5m OI series.
type V7T2OIFeatures struct {
	Pct5m       float64 // oi_5m_pct
	Pct15m      float64 // oi_15m_pct
	Pct1h       float64 // oi_1h_pct
	Z           float64 // oi_z (computed upstream but never gated on)
	NotionalUSD float64 // oi_notional_usd
}

// V7T2OIAccFeatures mirrors oi_accumulation_features() output (spec
// section 2.5). Note the upstream naming quirk: Pct4h is measured over the
// 8-point 1h OI series, i.e. ~7h, not 4h (spec section 8 item 3). The port
// reproduces the 8-point window literally.
type V7T2OIAccFeatures struct {
	Pct4h               float64 // oi_4h_pct (~7h window, see above)
	PriceChange4hPct    float64
	PriceAmplitude4hPct float64
}

// V7T2TakerFeatures mirrors taker_15m_features() output (spec section 2.6).
type V7T2TakerFeatures struct {
	BuyShare       float64   // latest bar share
	BuyShare3      []float64 // last 3 bar shares
	Buy3Confirmed  bool      // all 3 >= 0.55
	Sell3Confirmed bool      // all 3 <= 0.45
	RawRatio       float64   // latest raw buySellRatio
}

// V7T2DepthFeatures mirrors depth_features() output (spec section 2.7).
// Wall fields are carried for payload parity; no gate reads them upstream.
type V7T2DepthFeatures struct {
	BidTotal  float64
	AskTotal  float64
	Ratio     float64
	SpreadBps float64
	Available bool // false when the host snapshot has no order-book depth
}

// V7T2Snapshot is the detector's input: the Python snapshot dict equivalent.
type V7T2Snapshot struct {
	Symbol      string
	Base        string
	MarketCap   float64
	QuoteVolume float64
	Change24h   float64
	LastPrice   float64
	MarkPrice   float64
	FundingPct  float64 // lastFundingRate * 100
	GlobalLSR   float64 // global long/short account ratio (scanner default 1.0)
	TopLSR      float64 // carried for parity; unused by gates upstream

	K5  V7T2KlineFeatures
	K15 V7T2KlineFeatures
	K1h V7T2KlineFeatures
	K4h V7T2KlineFeatures

	Rows1h []V7T2Bar // raw 1h bars (ignition drift reads closes directly)

	OI    V7T2OIFeatures
	OIAcc V7T2OIAccFeatures
	Taker V7T2TakerFeatures

	TakerRatio    float64 // latest raw buySellRatio
	TakerTrendPct float64

	Depth V7T2DepthFeatures

	// Input-availability flags for host adapters whose snapshots lack a
	// series the scanner fetched directly. When a flag is false, gates
	// that need the series fail (no substitute value is fabricated).
	OI5mAvailable bool // 5m OI series present (oi_5m_pct / oi_15m_pct usable)
	// OIPct1hFrom5mSeries distinguishes spec oi_1h_pct (12x5m bars) from a
	// host-provided hourly figure; informational only.
	OIPct1hFrom5mSeries bool
}

// Price returns the canonical snapshot price (k5 last close; see header).
func (s *V7T2Snapshot) Price() float64 { return s.K5.Price }

// OIVolumeRatio recomputes oi_volume_ratio at signal time (spec section 9.2).
func (s *V7T2Snapshot) OIVolumeRatio() float64 {
	if s.QuoteVolume <= 0 {
		return 0
	}
	return s.OI.NotionalUSD / s.QuoteVolume
}

// OIMarketCapPct recomputes oi_market_cap_pct at signal time.
func (s *V7T2Snapshot) OIMarketCapPct() float64 {
	if s.MarketCap <= 0 {
		return 0
	}
	return s.OI.NotionalUSD / s.MarketCap * 100
}

// CompressionScore is the payload-only compression_score (spec section 9.2).
func (s *V7T2Snapshot) CompressionScore() float64 {
	return 1.0 / math.Max(s.K15.BBWidthRatio, 0.01) * 100
}

// ----------------------------------------------------------------------------
// Indicator ports (spec section 2.2)
// ----------------------------------------------------------------------------

// v7T2ATR ports atr(rows, period): simple mean of the last `period` true
// ranges (no Wilder smoothing). The first bar's TR is H-L (no prior close).
func v7T2ATR(rows []V7T2Bar, period int) float64 {
	if len(rows) == 0 || period <= 0 {
		return 0
	}
	trs := make([]float64, 0, len(rows))
	for i, r := range rows {
		tr := r.High - r.Low
		if i > 0 {
			pc := rows[i-1].Close
			tr = math.Max(tr, math.Max(math.Abs(r.High-pc), math.Abs(r.Low-pc)))
		}
		trs = append(trs, tr)
	}
	if len(trs) > period {
		trs = trs[len(trs)-period:]
	}
	sum := 0.0
	for _, v := range trs {
		sum += v
	}
	return sum / float64(len(trs))
}

// v7T2RSI ports rsi(rows, period=14): simple-mean gains/losses over the
// last `period` close changes (not Wilder). Insufficient samples -> 50.
func v7T2RSI(closes []float64, period int) float64 {
	if len(closes) <= period {
		return 50
	}
	changes := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		changes = append(changes, closes[i]-closes[i-1])
	}
	if len(changes) > period {
		changes = changes[len(changes)-period:]
	}
	var gain, loss float64
	for _, c := range changes {
		if c > 0 {
			gain += c
		} else {
			loss += -c
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	if avgLoss == 0 {
		if avgGain > 0 {
			return 100
		}
		return 50
	}
	return 100 - 100/(1+avgGain/avgLoss)
}

// v7T2EMA ports _ema(values, span): seeded at the first value.
func v7T2EMA(values []float64, span int) float64 {
	if len(values) == 0 {
		return 0
	}
	alpha := 2.0 / (float64(span) + 1)
	e := values[0]
	for _, v := range values[1:] {
		e += alpha * (v - e)
	}
	return e
}

// v7T2Percentile ports percentile(values, q): linear interpolation at
// position (n-1)*q of the sorted series.
func v7T2Percentile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := float64(len(sorted)-1) * q
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// v7T2Pstdev is the population standard deviation (Python statistics.pstdev).
func v7T2Pstdev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	ss := 0.0
	for _, v := range values {
		d := v - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(values)))
}

func v7T2Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// ----------------------------------------------------------------------------
// Feature builders (spec sections 2.3-2.7)
// ----------------------------------------------------------------------------

// BuildV7T2KlineFeatures ports kline_features(rows) (spec section 2.3).
// rows are oldest-first. Windows that lack data degrade the same way the
// slicing in the original does (shorter windows), except where the
// original would raise (e.g. empty rows) — those return a zero struct.
func BuildV7T2KlineFeatures(rows []V7T2Bar) V7T2KlineFeatures {
	f := V7T2KlineFeatures{}
	n := len(rows)
	if n == 0 {
		return f
	}
	closes := make([]float64, n)
	volumes := make([]float64, n)
	for i, r := range rows {
		closes[i] = r.Close
		volumes[i] = r.Volume
	}
	price := closes[n-1]
	f.Price = price
	f.ATR = v7T2ATR(rows, 20)
	if price > 0 {
		f.ATRPct = f.ATR / price * 100
	}
	f.ATR14 = v7T2ATR(rows, 14)
	f.RSI = v7T2RSI(closes, 14)

	// bb_width: final assignment in the original is the 20-close window at
	// the end of the full series (an earlier same-formula assignment is
	// overwritten; spec section 2.3 note).
	bbWidthAt := func(end int) float64 {
		if end < 20 || end > n {
			return 0
		}
		w := closes[end-20 : end]
		m := v7T2Mean(w)
		if m == 0 {
			return 0
		}
		return 4 * v7T2Pstdev(w) / m
	}
	f.BBWidth = bbWidthAt(n)
	// bb_width_min_50: min over the last 50 window widths (windows end at
	// every position from 20..n).
	var widths []float64
	for end := 20; end <= n; end++ {
		widths = append(widths, bbWidthAt(end))
	}
	if len(widths) > 50 {
		widths = widths[len(widths)-50:]
	}
	if len(widths) > 0 {
		mn := widths[0]
		for _, w := range widths[1:] {
			if w < mn {
				mn = w
			}
		}
		f.BBWidthMin50 = mn
	}
	if f.BBWidthMin50 == 0 {
		f.BBWidthRatio = 999.0
	} else {
		f.BBWidthRatio = f.BBWidth / f.BBWidthMin50
	}
	// bb_percentile: actually the p25 of the ATR% series (spec section 2.3).
	var atrPcts []float64
	for i := 25; i <= n; i++ {
		a := v7T2ATR(rows[:i], 20)
		prevClose := rows[i-1].Close
		if prevClose > 0 {
			atrPcts = append(atrPcts, a/prevClose*100)
		}
	}
	f.BBPercentile = v7T2Percentile(atrPcts, 0.25)

	if n >= 6 && price > 0 {
		hi, lo := rows[n-6].High, rows[n-6].Low
		for _, r := range rows[n-6:] {
			hi = math.Max(hi, r.High)
			lo = math.Min(lo, r.Low)
		}
		f.Range6Pct = (hi - lo) / price * 100
	}
	if n >= 21 {
		prevMean := v7T2Mean(volumes[n-21 : n-1])
		if prevMean > 0 {
			f.VolumeRatio = volumes[n-1] / prevMean
		}
	}
	// resistance/support: rows[-25:-1] (24 bars excluding the last); when
	// fewer than 25 rows exist the original falls back to rows[:-1].
	resWindow := rows[:n-1]
	if n >= 25 {
		resWindow = rows[n-25 : n-1]
	}
	if len(resWindow) > 0 {
		hi, lo := resWindow[0].High, resWindow[0].Low
		for _, r := range resWindow {
			hi = math.Max(hi, r.High)
			lo = math.Min(lo, r.Low)
		}
		f.Resistance = hi
		f.Support = lo
	}
	// prior_high/prior_low: rows[-21:-1] (20 bars excluding the last).
	priorWindow := rows[:n-1]
	if n >= 21 {
		priorWindow = rows[n-21 : n-1]
	}
	if len(priorWindow) > 0 {
		hi, lo := priorWindow[0].High, priorWindow[0].Low
		for _, r := range priorWindow {
			hi = math.Max(hi, r.High)
			lo = math.Min(lo, r.Low)
		}
		f.PriorHigh = hi
		f.PriorLow = lo
	}
	f.EMA20 = v7T2EMA(closes, 20)
	f.EMA50 = v7T2EMA(closes, 50)
	f.HasEMA = n >= 2
	if f.EMA50 != 0 {
		f.EMATrendPct = (f.EMA20/f.EMA50 - 1) * 100
	}
	if n >= 4 && closes[n-4] != 0 {
		f.Change3Pct = (closes[n-1]/closes[n-4] - 1) * 100
	}
	if n >= 13 && closes[n-13] != 0 {
		f.Change12Pct = (closes[n-1]/closes[n-13] - 1) * 100
	}
	if n >= 25 && closes[n-25] != 0 {
		f.Change24Pct = (closes[n-1]/closes[n-25] - 1) * 100
	}
	f.LastCloseTime = rows[n-1].CloseTime
	return f
}

// V7T2OIValueSeries selects the OI series the way oi_features() does
// (spec section 2.4): the notional-value series filtered to its positive
// entries; only when it has no positive entry at all does the
// contract-count series apply. (Verified against production payloads:
// symbols whose value field populates only in the last few rows get
// their pcts computed over the short positive tail — see M3_REPORT.md.)
func V7T2OIValueSeries(values, contracts []float64) []float64 {
	pos := make([]float64, 0, len(values))
	for _, v := range values {
		if v > 0 {
			pos = append(pos, v)
		}
	}
	if len(pos) > 0 {
		return pos
	}
	return contracts
}

// BuildV7T2OIFeatures ports oi_features() (spec section 2.4). values are
// the OI series (notional USD preferred upstream; contract count fallback)
// oldest-first.
func BuildV7T2OIFeatures(values []float64) V7T2OIFeatures {
	f := V7T2OIFeatures{}
	n := len(values)
	if n == 0 {
		return f
	}
	f.NotionalUSD = values[n-1]
	pctBack := func(k int) float64 {
		if n-1-k < 0 || values[n-1-k] == 0 {
			return 0
		}
		return (values[n-1]/values[n-1-k] - 1) * 100
	}
	f.Pct5m = pctBack(1)
	f.Pct15m = pctBack(3)
	f.Pct1h = pctBack(12)
	// oi_z: median/MAD of the adjacent change-rate series.
	if n >= 3 {
		var rets []float64
		for i := 1; i < n; i++ {
			if values[i-1] != 0 {
				rets = append(rets, values[i]/values[i-1]-1)
			}
		}
		if len(rets) > 0 {
			med := v7T2Percentile(rets, 0.5)
			var devs []float64
			for _, r := range rets {
				devs = append(devs, math.Abs(r-med))
			}
			mad := v7T2Percentile(devs, 0.5)
			if mad > 0 {
				f.Z = (rets[len(rets)-1] - med) / (1.4826 * mad)
			}
		}
	}
	return f
}

// BuildV7T2OIAccFeatures ports oi_accumulation_features() (spec section
// 2.5): oiValues are the 1h OI series (upstream limit=8, so Pct4h spans 7
// intervals); rows1h supplies the last-5-bar (4h) price window.
func BuildV7T2OIAccFeatures(oiValues []float64, rows1h []V7T2Bar) V7T2OIAccFeatures {
	f := V7T2OIAccFeatures{}
	if len(oiValues) >= 2 && oiValues[0] != 0 {
		f.Pct4h = (oiValues[len(oiValues)-1]/oiValues[0] - 1) * 100
	}
	if len(rows1h) >= 5 {
		w := rows1h[len(rows1h)-5:]
		if w[0].Close != 0 {
			f.PriceChange4hPct = (w[len(w)-1].Close/w[0].Close - 1) * 100
		}
		hi, lo := w[0].High, w[0].Low
		for _, r := range w {
			hi = math.Max(hi, r.High)
			lo = math.Min(lo, r.Low)
		}
		if lo != 0 {
			f.PriceAmplitude4hPct = (hi/lo - 1) * 100
		}
	}
	return f
}

// BuildV7T2TakerFeatures ports taker_15m_features() (spec section 2.6)
// from raw buySellRatio values (oldest-first).
func BuildV7T2TakerFeatures(ratios []float64) V7T2TakerFeatures {
	shares := make([]float64, len(ratios))
	for i, r := range ratios {
		shares[i] = r / (1 + r)
	}
	f := BuildV7T2TakerFeaturesFromShares(shares)
	if len(ratios) > 0 {
		f.RawRatio = ratios[len(ratios)-1]
	}
	return f
}

// BuildV7T2TakerFeaturesFromShares builds the same features from taker
// buy shares directly (share = ratio/(1+ratio)). Host adapters whose
// kline feed carries taker-buy volume per bar use this entry point.
func BuildV7T2TakerFeaturesFromShares(shares []float64) V7T2TakerFeatures {
	f := V7T2TakerFeatures{BuyShare: 0.5}
	if len(shares) == 0 {
		return f
	}
	f.BuyShare = shares[len(shares)-1]
	if f.BuyShare < 1 {
		f.RawRatio = f.BuyShare / (1 - f.BuyShare)
	}
	last3 := shares
	if len(last3) > 3 {
		last3 = last3[len(last3)-3:]
	}
	f.BuyShare3 = append([]float64(nil), last3...)
	f.Buy3Confirmed = len(last3) == 3
	f.Sell3Confirmed = len(last3) == 3
	for _, s := range last3 {
		if s < 0.55 {
			f.Buy3Confirmed = false
		}
		if s > 0.45 {
			f.Sell3Confirmed = false
		}
	}
	return f
}

// V7T2DepthLevel is one order-book level (price, quantity).
type V7T2DepthLevel struct {
	Price float64
	Qty   float64
}

// BuildV7T2DepthFeatures ports depth_features() (spec section 2.7): only
// levels within ±1% of price count toward the totals.
func BuildV7T2DepthFeatures(bids, asks []V7T2DepthLevel, price float64) V7T2DepthFeatures {
	f := V7T2DepthFeatures{}
	if len(bids) == 0 || len(asks) == 0 || price <= 0 {
		return f
	}
	inBand := func(l V7T2DepthLevel) bool {
		return math.Abs(l.Price/price-1) <= 0.01
	}
	for _, l := range bids {
		if inBand(l) {
			f.BidTotal += l.Price * l.Qty
		}
	}
	for _, l := range asks {
		if inBand(l) {
			f.AskTotal += l.Price * l.Qty
		}
	}
	if f.AskTotal > 0 {
		f.Ratio = f.BidTotal / f.AskTotal
	}
	bestBid, bestAsk := bids[0].Price, asks[0].Price
	f.SpreadBps = (bestAsk - bestBid) / price * 10000
	f.Available = true
	return f
}

// ----------------------------------------------------------------------------
// Shared gates (spec sections 2.8, 2.9)
// ----------------------------------------------------------------------------

// v7T2DirectionOK ports fifteen_minute_direction_ok (spec section 2.8).
// Missing EMA fields fail open, exactly as upstream.
func v7T2DirectionOK(k15 V7T2KlineFeatures, direction string) bool {
	if !k15.HasEMA {
		return true
	}
	switch direction {
	case "LONG":
		return k15.Price >= k15.EMA20*0.998 && k15.EMA20 >= k15.EMA50*0.998 && k15.RSI <= 78
	case "SHORT":
		return k15.Price <= k15.EMA20*1.002 && k15.EMA20 <= k15.EMA50*1.002 && k15.RSI >= 22
	}
	return false
}

// V7T2ActivityFlags ports activity_behavior_flags (spec section 2.9).
func V7T2ActivityFlags(s *V7T2Snapshot) []string {
	var flags []string
	if s.OIVolumeRatio() >= 0.20 {
		flags = append(flags, "oi_to_volume_extreme")
	}
	if s.K5.VolumeRatio >= 4.0 && math.Abs(s.K5.Change12Pct) <= 1.0 {
		flags = append(flags, "flat_price_extreme_volume")
	}
	if len(s.Taker.BuyShare3) > 0 {
		mn, mx := s.Taker.BuyShare3[0], s.Taker.BuyShare3[0]
		for _, v := range s.Taker.BuyShare3 {
			mn = math.Min(mn, v)
			mx = math.Max(mx, v)
		}
		if mx-mn >= 0.30 {
			flags = append(flags, "taker_ratio_unstable")
		}
	}
	if s.K15.VolumeRatio >= 5.0 && math.Abs(s.K15.Change12Pct) <= 1.5 {
		flags = append(flags, "15m_volume_price_mismatch")
	}
	return flags
}

// v7T2WallDistanceATR is the shared wall-distance measure (spec section
// 4.1): distance to the nearer 4h prior extreme, in 4h ATR14 units.
func v7T2WallDistanceATR(s *V7T2Snapshot) float64 {
	if s.K4h.ATR14 == 0 {
		return 0
	}
	price := s.Price()
	dHi := math.Abs(price - s.K4h.PriorHigh)
	dLo := math.Abs(price - s.K4h.PriorLow)
	return math.Min(dHi, dLo) / s.K4h.ATR14
}

func v7T2CountGE(shares []float64, th float64) int {
	n := 0
	for _, v := range shares {
		if v >= th {
			n++
		}
	}
	return n
}

func v7T2CountLE(shares []float64, th float64) int {
	n := 0
	for _, v := range shares {
		if v <= th {
			n++
		}
	}
	return n
}

func v7T2LastShare(s *V7T2Snapshot) float64 {
	if len(s.Taker.BuyShare3) == 0 {
		return 0.5
	}
	return s.Taker.BuyShare3[len(s.Taker.BuyShare3)-1]
}

// ----------------------------------------------------------------------------
// Signal (mirrors the Python signal dict, spec section 9.2)
// ----------------------------------------------------------------------------

// V7T2Signal is one emitted Task-2 signal before lifecycle upsert.
type V7T2Signal struct {
	Symbol           string
	Direction        string // LONG / SHORT
	SetupType        string
	Status           string // WATCH / ARMED (after section 6 modulation)
	Score            float64
	Trigger          float64
	Entry            float64
	Stop             float64
	Target1          float64
	Target2          float64
	RR               float64 // constant 1.8 (spec section 7)
	CompressionScore float64
	OINotionalUSD    float64
	OIMarketCapPct   float64
	OIVolumeRatio    float64
	QualityGates     map[string]interface{}
	Reasons          []string
	StrategyVersion  string

	// ExecutionEntry is present only for ignition / watch / momentum
	// (spec section 9.2); bb_oi and secondary leave HasExecutionEntry false.
	ExecutionEntry    float64
	HasExecutionEntry bool

	// Momentum-only pullback shadow plan (recorded, never executed;
	// spec section 4.5).
	EntryMode         string
	PBEntry           float64
	PBStop            float64
	PBTarget1         float64
	PBTarget2         float64
	PBRisk            float64
	PBATR15m          float64
	HasPullbackShadow bool

	// Regime modulation (spec section 6.1); HasRegime false when the BTC
	// RV24 input was unavailable (upstream fails open the same way).
	HasRegime    bool
	RegimeRV24   float64
	RegimeTier   string
	SizeFactor   float64
	RegimeAction string

	ObserveOnly bool // spec section 6.2

	ActivityStatus string
	ActivityFlags  []string
}

// V7T2RegimeInput carries the BTC trailing RV24 (percent) for section 6.1.
type V7T2RegimeInput struct {
	RV24  float64
	Valid bool
}

func v7t2BaseSignal(s *V7T2Snapshot, setupType, direction string) *V7T2Signal {
	return &V7T2Signal{
		Symbol:           s.Symbol,
		Direction:        direction,
		SetupType:        setupType,
		Status:           "WATCH",
		RR:               v7T2MinRR,
		CompressionScore: s.CompressionScore(),
		OINotionalUSD:    s.OI.NotionalUSD,
		OIMarketCapPct:   s.OIMarketCapPct(),
		OIVolumeRatio:    s.OIVolumeRatio(),
		QualityGates:     map[string]interface{}{},
		StrategyVersion:  v7T2StrategyVersion,
	}
}

func v7t2StatusForScore(score float64) string {
	if score >= v7T2MinAlertScore {
		return "ARMED"
	}
	return "WATCH"
}

// v7t2TriggerBasedPlan ports trigger_based_plan (spec section 7): entry is
// offset 0.10*risk beyond the trigger, stop is 0.90*risk behind it, so
// |entry-stop| == risk exactly.
func v7t2TriggerBasedPlan(trigger, atrValue, price float64, direction string) (entry, stop, tp1, tp2 float64) {
	risk := math.Max(atrValue, price*0.009)
	if direction == "LONG" {
		entry = trigger + 0.10*risk
		stop = trigger - 0.90*risk
		tp1 = entry + v7T2MinRR*risk
		tp2 = entry + v7T2TP2Multiple*risk
	} else {
		entry = trigger - 0.10*risk
		stop = trigger + 0.90*risk
		tp1 = entry - v7T2MinRR*risk
		tp2 = entry - v7T2TP2Multiple*risk
	}
	return
}

// ----------------------------------------------------------------------------
// Family: momentum continuation (spec section 4.5) — evaluated FIRST
// ----------------------------------------------------------------------------

func v7t2Momentum(s *V7T2Snapshot) *V7T2Signal {
	chg := s.Change24h
	absChg := math.Abs(chg)
	if absChg < 15 || absChg > v7T2MaxTrackingChangePct {
		return nil
	}
	direction := "LONG"
	if chg < 0 {
		direction = "SHORT"
	}
	if s.QuoteVolume < v7T2MinVolumeUSD {
		return nil
	}
	if !s.Depth.Available || s.Depth.SpreadBps > 35 { // momentum uses the wider 35bp gate
		return nil
	}
	k15 := s.K15
	if !k15.HasEMA {
		return nil
	}
	if s.OI.NotionalUSD < v7T2MinOINotionalUSD || s.OI.NotionalUSD > v7T2MaxOINotionalUSD {
		return nil
	}
	if s.OIAcc.Pct4h < 2.0 {
		return nil
	}
	if s.OIAcc.PriceChange4hPct*chg < 0 {
		return nil
	}
	// 15m structure (looser than the section 2.8 filter; spec section 4.5).
	if k15.EMA20 == 0 {
		return nil
	}
	extensionPct := math.Abs(k15.Price/k15.EMA20-1) * 100
	if direction == "LONG" {
		if !(k15.Price >= k15.EMA20*0.985 && k15.EMA20 >= k15.EMA50*0.995 && k15.RSI <= 82) {
			return nil
		}
	} else {
		if !(k15.Price <= k15.EMA20*1.015 && k15.EMA20 <= k15.EMA50*1.005 && k15.RSI >= 18) {
			return nil
		}
	}
	bucket := "normal"
	isExtreme := absChg >= 60 || extensionPct >= 12 ||
		(direction == "LONG" && k15.RSI >= 82) || (direction == "SHORT" && k15.RSI <= 18)
	isHigh := absChg >= 35 || extensionPct >= 8 ||
		(direction == "LONG" && k15.RSI >= 76) || (direction == "SHORT" && k15.RSI <= 24)
	if isExtreme {
		bucket = "extreme"
	} else if isHigh {
		bucket = "high"
	}
	// Taker flow: flow_ok (ARMED-capable) vs watch_flow_ok (WATCH only).
	shares := s.Taker.BuyShare3
	last := v7T2LastShare(s)
	var flowOK, watchFlowOK bool
	if direction == "LONG" {
		flowOK = v7T2CountGE(shares, 0.50) >= 2 && last >= 0.53 && s.Depth.Ratio >= 0.95
		watchFlowOK = v7T2CountGE(shares, 0.50) >= 2 && last >= v7T2MomentumWatchMinLatest && s.Depth.Ratio >= 0.85
	} else {
		flowOK = v7T2CountLE(shares, 0.50) >= 2 && last <= 0.47 && s.Depth.Ratio <= 1.05
		watchFlowOK = v7T2CountLE(shares, 0.50) >= 2 && last <= 1-v7T2MomentumWatchMinLatest && s.Depth.Ratio <= 1.15
	}
	if !flowOK && !watchFlowOK {
		return nil
	}
	armedFlow := flowOK && bucket != "extreme"

	price := s.Price()
	atrValue := math.Max(s.K5.ATR, price*0.01)
	risk := math.Max(atrValue, price*0.009)
	var entry, stop, tp1, tp2 float64
	if direction == "LONG" {
		entry = price + 0.10*risk
		stop = entry - risk
		tp1 = entry + v7T2MinRR*risk
		tp2 = entry + v7T2TP2Multiple*risk
	} else {
		entry = price - 0.10*risk
		stop = entry + risk
		tp1 = entry - v7T2MinRR*risk
		tp2 = entry - v7T2TP2Multiple*risk
	}

	oiScore := math.Min(18, 8+math.Max(0, s.OIAcc.Pct4h-2.0)*1.5)
	moveScore := math.Min(18, 6+absChg*0.20)
	flowScore := 3.0
	if armedFlow {
		flowScore = 8
		allStrong := len(shares) == 3
		for _, sh := range shares {
			if direction == "LONG" && sh < 0.53 {
				allStrong = false
			}
			if direction == "SHORT" && sh > 0.47 {
				allStrong = false
			}
		}
		if allStrong {
			flowScore = 12
		}
	}
	depthScore := math.Min(10, 6+math.Abs(math.Log(math.Max(s.Depth.Ratio, 0.01)))*5)
	cap := 79.0
	if armedFlow {
		cap = 96
	}
	score := math.Min(cap, 58+oiScore+moveScore+flowScore+depthScore)

	setupType := "momentum_continuation_watch_" + strings.ToLower(direction)
	if armedFlow {
		setupType = "momentum_continuation_" + strings.ToLower(direction)
	}
	sig := v7t2BaseSignal(s, setupType, direction)
	sig.Score = score
	sig.Status = v7t2StatusForScore(score)
	sig.Trigger = price
	sig.Entry, sig.Stop, sig.Target1, sig.Target2 = entry, stop, tp1, tp2
	sig.ExecutionEntry = entry
	sig.HasExecutionEntry = true
	sig.QualityGates["extension_bucket"] = bucket
	sig.QualityGates["armed_flow_ok"] = armedFlow
	sig.QualityGates["flow_ok"] = flowOK
	sig.QualityGates["watch_flow_ok"] = watchFlowOK
	// Pullback shadow plan (v0.7.2; recorded only, never executed).
	atr15 := math.Max(s.K15.ATR, s.K15.ATR14)
	pbRisk := 1.5 * math.Max(atr15, price*0.008)
	sig.EntryMode = "pullback_shadow"
	sig.PBEntry = s.K15.EMA20
	sig.PBRisk = pbRisk
	sig.PBATR15m = math.Max(atr15, price*0.008)
	if direction == "LONG" {
		sig.PBStop = sig.PBEntry - pbRisk
		sig.PBTarget1 = sig.PBEntry + v7T2MinRR*pbRisk
		sig.PBTarget2 = sig.PBEntry + v7T2TP2Multiple*pbRisk
	} else {
		sig.PBStop = sig.PBEntry + pbRisk
		sig.PBTarget1 = sig.PBEntry - v7T2MinRR*pbRisk
		sig.PBTarget2 = sig.PBEntry - v7T2TP2Multiple*pbRisk
	}
	sig.HasPullbackShadow = true
	sig.Reasons = append(sig.Reasons, "动量延续: 24h涨跌幅与OI同向累积, taker流确认")
	return sig
}

// ----------------------------------------------------------------------------
// Family: main ambush bb_oi_accumulation (spec sections 3, 4.1)
// ----------------------------------------------------------------------------

// v7t2MainChain runs Layer 2/3 + direction + trigger + geometry for the
// main ambush. hardStop=true means the geometry was invalid, which
// upstream answers with None and NO fallback chain (spec section 3 step 8).
func v7t2MainChain(s *V7T2Snapshot) (sig *V7T2Signal, hardStop bool) {
	price := s.Price()
	// Layer 2 strict resonance.
	bbSqueeze := s.K15.BBWidth > 0 && s.K15.BBWidthRatio <= 1.10
	oiBuild := s.OIAcc.Pct4h > 5.0 &&
		math.Abs(s.OIAcc.PriceChange4hPct) <= 2.0 &&
		s.OIAcc.PriceAmplitude4hPct <= 5.0 &&
		s.OI5mAvailable && s.OI.Pct15m >= -0.5
	if !bbSqueeze || !oiBuild {
		return nil, false
	}
	// Layer 3 late-stage / wall-distance gates.
	wall := v7T2WallDistanceATR(s)
	if math.Abs(s.Change24h) > 15 || math.Abs(s.K5.Change12Pct) > 3.0 || wall < 1.5 {
		return nil, false
	}
	// Direction resonance: exactly one side may hold.
	longOK := s.Taker.Buy3Confirmed && s.Depth.Ratio >= 1.10 &&
		s.K5.Change12Pct >= -1.5 && s.K1h.EMATrendPct >= -1.0 &&
		s.FundingPct <= 0.05 && s.GlobalLSR <= 1.7 &&
		v7T2DirectionOK(s.K15, "LONG")
	shortOK := s.Taker.Sell3Confirmed && s.Depth.Ratio <= 0.90 &&
		s.K5.Change12Pct <= 1.5 && s.K1h.EMATrendPct <= 1.0 &&
		s.FundingPct >= -0.05 && s.GlobalLSR >= 0.5 &&
		v7T2DirectionOK(s.K15, "SHORT")
	if longOK == shortOK {
		return nil, false
	}
	direction := "LONG"
	trigger := s.K15.Resistance
	if shortOK {
		direction = "SHORT"
		trigger = s.K15.Support
	}
	// Trigger-distance gate.
	if price <= 0 {
		return nil, false
	}
	dist := math.Abs(trigger-price) / price
	if dist < 0.0005 || dist > 0.02 {
		return nil, false
	}
	// Geometry (spec section 4.1).
	atrValue := math.Max(s.K5.ATR, price*0.008)
	risk := math.Max(atrValue*1.2, price*0.01)
	var entry, stop, tp1, tp2 float64
	if direction == "LONG" {
		entry = trigger + 0.1*atrValue
		stop = entry - risk
		tp1 = entry + v7T2MinRR*risk
		tp2 = entry + v7T2TP2Multiple*risk
		if !(stop < price && price < trigger) {
			return nil, true
		}
	} else {
		entry = trigger - 0.1*atrValue
		stop = entry + risk
		tp1 = entry - v7T2MinRR*risk
		tp2 = entry - v7T2TP2Multiple*risk
		if !(trigger < price && price < stop) {
			return nil, true
		}
	}
	// Scoring (spec section 4.1 table).
	bbScore := 21.0
	if s.K15.BBWidthRatio <= 1.03 {
		bbScore = 25
	}
	oiScore := math.Min(28, 20+math.Max(0, s.OIAcc.Pct4h-5.0)*1.5)
	shares := s.Taker.BuyShare3
	var edge float64
	if len(shares) > 0 {
		if direction == "LONG" {
			mn := shares[0]
			for _, v := range shares {
				mn = math.Min(mn, v)
			}
			edge = mn - 0.55
		} else {
			mx := shares[0]
			for _, v := range shares {
				mx = math.Max(mx, v)
			}
			edge = 0.45 - mx
		}
	}
	takerScore := math.Min(20, 16+math.Max(0, edge)*80)
	depthScore := math.Min(12, 8+math.Abs(math.Log(math.Max(s.Depth.Ratio, 0.01)))*6)
	wallScore := 5.0
	if wall >= 2.0 {
		wallScore = 8
	}
	trendScore := 4.0
	if math.Abs(s.K1h.EMATrendPct) <= 0.6 {
		trendScore = 7
	}
	liqScore := 3.0
	if s.OIVolumeRatio() >= 0.03 {
		liqScore = 5
	}
	score := math.Min(100, bbScore+oiScore+takerScore+depthScore+wallScore+trendScore+liqScore)

	sig = v7t2BaseSignal(s, "bb_oi_accumulation_"+strings.ToLower(direction), direction)
	sig.Score = score
	sig.Status = v7t2StatusForScore(score)
	sig.Trigger = trigger
	sig.Entry, sig.Stop, sig.Target1, sig.Target2 = entry, stop, tp1, tp2
	sig.QualityGates["wall_distance_atr"] = wall
	sig.Reasons = append(sig.Reasons, "主埋伏: BB压缩与OI累积共振, taker/深度同向确认")
	return sig, false
}

// ----------------------------------------------------------------------------
// Family: secondary confirmed chain (spec section 4.4)
// ----------------------------------------------------------------------------

func v7t2Secondary(s *V7T2Snapshot) *V7T2Signal {
	price := s.Price()
	if s.OI.NotionalUSD < v7T2MinOINotionalUSD || s.OI.NotionalUSD > v7T2MaxOINotionalUSD {
		return nil
	}
	if s.OIVolumeRatio() < v7T2MinOIVolumeRatio {
		return nil
	}
	if math.Abs(s.Change24h) > 15 || math.Abs(s.K5.Change12Pct) > 4.5 {
		return nil
	}
	if s.K15.BBWidthRatio > 1.80 {
		return nil
	}
	if !(s.OIAcc.Pct4h >= 2.5 && math.Abs(s.OIAcc.PriceChange4hPct) <= 5.0 && s.OIAcc.PriceAmplitude4hPct <= 10.0) {
		return nil
	}
	wall := v7T2WallDistanceATR(s)
	if wall < 0.9 {
		return nil
	}
	shares := s.Taker.BuyShare3
	buyCount := v7T2CountGE(shares, 0.55)
	sellCount := v7T2CountLE(shares, 0.45)
	mildBuy := v7T2CountGE(shares, 0.52)
	mildSell := v7T2CountLE(shares, 0.48)
	vr := math.Max(s.K5.VolumeRatio, s.K15.VolumeRatio)
	longTrig := s.K15.Resistance
	shortTrig := s.K15.Support

	var setupType, direction string
	// LONG side first; within a side: breakout > reclaim > pullback.
	breakoutLong := longTrig > 0 && price >= longTrig && price/longTrig-1 <= 0.015 &&
		buyCount >= 2 && s.Depth.Ratio >= 1.05 && vr >= 0.8 &&
		s.K1h.EMATrendPct >= -1.5 && v7T2DirectionOK(s.K15, "LONG")
	// Reclaim's "price beyond trigger" includes the exact touch
	// (price == trigger): production evidence PLUMEUSDT 2026-10-16,
	// where the stored price and resistance are identical and the
	// scanner emitted reclaim_breakout_long (see M3_REPORT.md).
	reclaimLong := !breakoutLong && s.OI5mAvailable && s.OI.Pct5m >= 0.25 && s.OI.Pct15m > -1.0 &&
		longTrig > 0 && price >= longTrig && buyCount >= 2 && s.Depth.Ratio >= 1.0 &&
		vr >= 0.8 && v7T2DirectionOK(s.K15, "LONG")
	pullbackLong := !breakoutLong && !reclaimLong && mildBuy >= 2 &&
		price >= s.K15.EMA20*0.998 && s.K1h.EMATrendPct >= -0.5 && s.Depth.Ratio >= 1.15 &&
		vr >= 0.6 && s.OI5mAvailable && s.OI.Pct5m > 0 && v7T2DirectionOK(s.K15, "LONG")
	breakoutShort := shortTrig > 0 && price <= shortTrig && shortTrig/price-1 <= 0.015 &&
		sellCount >= 2 && s.Depth.Ratio <= 0.95 && vr >= 0.8 &&
		s.K1h.EMATrendPct <= 1.5 && v7T2DirectionOK(s.K15, "SHORT")
	reclaimShort := !breakoutShort && s.OI5mAvailable && s.OI.Pct5m >= 0.25 && s.OI.Pct15m > -1.0 &&
		shortTrig > 0 && price <= shortTrig && sellCount >= 2 && s.Depth.Ratio <= 1.0 &&
		vr >= 0.8 && v7T2DirectionOK(s.K15, "SHORT")
	pullbackShort := !breakoutShort && !reclaimShort && mildSell >= 2 &&
		price <= s.K15.EMA20*1.002 && s.K1h.EMATrendPct <= 0.5 && s.Depth.Ratio <= 0.85 &&
		vr >= 0.6 && s.OI5mAvailable && s.OI.Pct5m > 0 && v7T2DirectionOK(s.K15, "SHORT")

	switch {
	case breakoutLong:
		setupType, direction = "confirmed_breakout_long", "LONG"
	case reclaimLong:
		setupType, direction = "reclaim_breakout_long", "LONG"
	case pullbackLong:
		setupType, direction = "trend_pullback_long", "LONG"
	case breakoutShort:
		setupType, direction = "confirmed_breakout_short", "SHORT"
	case reclaimShort:
		setupType, direction = "reclaim_breakout_short", "SHORT"
	case pullbackShort:
		setupType, direction = "trend_pullback_short", "SHORT"
	default:
		return nil
	}

	// Shared scoring (spec section 4.4 table).
	bbScore := math.Max(0, 16-(s.K15.BBWidthRatio-1.0)*10)
	oiScore := math.Min(18, 10+(s.OIAcc.Pct4h-3.0)*1.2)
	takerScore := 8.0
	if math.Max(float64(buyCount), float64(sellCount)) == 3 {
		takerScore = 12
	}
	volumeScore := math.Min(12, 6+math.Max(0, vr-1.0)*4)
	depthScore := math.Min(12, 7+math.Abs(math.Log(math.Max(s.Depth.Ratio, 0.01)))*5)
	wallBonus := 2.0
	if wall >= 1.5 {
		wallBonus = 4
	}
	score := math.Min(94, 58+bbScore+oiScore+takerScore+volumeScore+depthScore+wallBonus)

	atrValue := math.Max(s.K5.ATR, price*0.008)
	risk := math.Max(atrValue*1.0, price*0.009)
	var entry, stop, tp1, tp2 float64
	entry = price
	if direction == "LONG" {
		stop = entry - risk
		tp1 = entry + v7T2MinRR*risk
		tp2 = entry + v7T2TP2Multiple*risk
	} else {
		stop = entry + risk
		tp1 = entry - v7T2MinRR*risk
		tp2 = entry - v7T2TP2Multiple*risk
	}
	sig := v7t2BaseSignal(s, setupType, direction)
	sig.Score = score
	sig.Status = v7t2StatusForScore(score)
	if direction == "LONG" {
		sig.Trigger = longTrig
	} else {
		sig.Trigger = shortTrig
	}
	sig.Entry, sig.Stop, sig.Target1, sig.Target2 = entry, stop, tp1, tp2
	sig.QualityGates["wall_distance_atr"] = wall
	sig.Reasons = append(sig.Reasons, "二级确认链: OI累积形态下二级确认触发")
	return sig
}

// ----------------------------------------------------------------------------
// Family: ignition early (spec section 4.2) — WATCH-only by design
// ----------------------------------------------------------------------------

func v7t2Ignition(s *V7T2Snapshot) *V7T2Signal {
	if len(s.Rows1h) < 10 {
		return nil
	}
	closes := make([]float64, len(s.Rows1h))
	for i, r := range s.Rows1h {
		closes[i] = r.Close
	}
	n := len(closes)
	if closes[n-9] == 0 {
		return nil
	}
	drift := (closes[n-1]/closes[n-9] - 1) * 100
	longBand := drift >= 2.0 && drift <= 12.0
	shortBand := drift >= -12.0 && drift <= -2.0
	if longBand == shortBand {
		return nil
	}
	direction := "LONG"
	if shortBand {
		direction = "SHORT"
	}
	vr := math.Max(s.K5.VolumeRatio, s.K15.VolumeRatio)
	if vr < 1.3 || vr > 4.0 {
		return nil
	}
	if direction == "LONG" && s.K15.RSI >= 72 {
		return nil
	}
	if direction == "SHORT" && s.K15.RSI <= 28 {
		return nil
	}
	if math.Abs(s.Change24h) >= 25 {
		return nil
	}
	if math.Abs(s.K5.Change12Pct) > 6.0 {
		return nil
	}
	if !s.OI5mAvailable || s.OI.Pct15m < -2.0 {
		return nil
	}
	if len(s.Taker.BuyShare3) == 0 {
		return nil
	}
	meanShare := v7T2Mean(s.Taker.BuyShare3)
	if direction == "LONG" && meanShare < 0.52 {
		return nil
	}
	if direction == "SHORT" && meanShare > 0.48 {
		return nil
	}
	if !v7T2DirectionOK(s.K15, direction) {
		return nil
	}
	score := math.Min(78, 62+math.Min(8, math.Max(0, vr-1.3)*4)+math.Min(8, math.Max(0, math.Abs(drift)-2.0)))
	price := s.Price()
	trigger := s.K15.Resistance
	if direction == "SHORT" {
		trigger = s.K15.Support
	}
	atrValue := math.Max(s.K5.ATR, price*0.01)
	entry, stop, tp1, tp2 := v7t2TriggerBasedPlan(trigger, atrValue, price, direction)
	sig := v7t2BaseSignal(s, "ignition_early_"+strings.ToLower(direction), direction)
	sig.Score = score
	sig.Status = "WATCH" // hard-coded WATCH upstream (score cap 78 < 82)
	sig.Trigger = trigger
	sig.Entry, sig.Stop, sig.Target1, sig.Target2 = entry, stop, tp1, tp2
	sig.ExecutionEntry = entry
	sig.HasExecutionEntry = true
	sig.QualityGates["drift_8h_pct"] = drift
	sig.Reasons = append(sig.Reasons, "点火早期: 8h漂移进入点火带, 量能放大且OI未流失")
	return sig
}

// ----------------------------------------------------------------------------
// Family: compression OI watch (spec section 4.3) — WATCH-only by design
// ----------------------------------------------------------------------------

func v7t2WatchNearTrigger(s *V7T2Snapshot) *V7T2Signal {
	price := s.Price()
	if s.OI.NotionalUSD < v7T2MinOINotionalUSD || s.OI.NotionalUSD > v7T2MaxOINotionalUSD {
		return nil
	}
	if math.Abs(s.Change24h) > 15 || math.Abs(s.K5.Change12Pct) > 3.5 {
		return nil
	}
	if s.K15.BBWidthRatio > 1.80 {
		return nil
	}
	oiBuilding := s.OIAcc.Pct4h >= 2.5 ||
		(s.OI5mAvailable && s.OI.Pct15m >= 0.8 && s.OI.Pct1h >= 1.5)
	if !oiBuilding {
		return nil
	}
	vr := math.Max(s.K5.VolumeRatio, s.K15.VolumeRatio)
	if vr < 0.9 {
		return nil
	}
	if s.K4h.ATR14 == 0 {
		return nil
	}
	wall := v7T2WallDistanceATR(s)
	if wall < 0.8 {
		return nil
	}
	shares := s.Taker.BuyShare3
	last := v7T2LastShare(s)
	longTrig := s.K15.Resistance
	shortTrig := s.K15.Support
	longOK := false
	if longTrig > 0 && price > 0 {
		d := (longTrig - price) / price
		longOK = d >= 0.001 && d <= 0.015 &&
			v7T2CountGE(shares, 0.52) >= 2 && last >= 0.53 &&
			s.Depth.Ratio >= 1.05 && s.K1h.EMATrendPct >= -1.0 &&
			v7T2DirectionOK(s.K15, "LONG")
	}
	shortOK := false
	if shortTrig > 0 && price > 0 {
		d := (price - shortTrig) / price
		shortOK = d >= 0.001 && d <= 0.015 &&
			v7T2CountLE(shares, 0.48) >= 2 && last <= 0.47 &&
			s.Depth.Ratio <= 0.95 && s.K1h.EMATrendPct <= 1.0 &&
			v7T2DirectionOK(s.K15, "SHORT")
	}
	if longOK == shortOK {
		return nil
	}
	direction := "LONG"
	trigger := longTrig
	if shortOK {
		direction = "SHORT"
		trigger = shortTrig
	}
	wallBonus := 1.0
	if wall >= 1.2 {
		wallBonus = 3
	}
	score := math.Min(76, 64+math.Max(0, 1.8-s.K15.BBWidthRatio)*5+
		math.Min(6, math.Max(0, s.OIAcc.Pct4h))+wallBonus+3)
	atrValue := math.Max(s.K5.ATR, price*0.008)
	entry, stop, tp1, tp2 := v7t2TriggerBasedPlan(trigger, atrValue, price, direction)
	sig := v7t2BaseSignal(s, "compression_oi_watch_"+strings.ToLower(direction), direction)
	sig.Score = score
	sig.Status = "WATCH" // hard-coded WATCH upstream (score cap 76 < 82)
	sig.Trigger = trigger
	sig.Entry, sig.Stop, sig.Target1, sig.Target2 = entry, stop, tp1, tp2
	sig.ExecutionEntry = entry
	sig.HasExecutionEntry = true
	sig.QualityGates["wall_distance_atr"] = wall
	sig.Reasons = append(sig.Reasons, "近触发观察: 压缩区间内OI在建, 价格临近触发位")
	return sig
}

// ----------------------------------------------------------------------------
// Post-processing: regime modulation + observe-only (spec section 6)
// ----------------------------------------------------------------------------

func v7t2ApplyRegime(sig *V7T2Signal, regime *V7T2RegimeInput) {
	if regime == nil || !regime.Valid {
		return // upstream fails open when RV24 is unavailable
	}
	rv := regime.RV24
	tier := "HIGH"
	if rv < 0.25 {
		tier = "LOW"
	} else if rv < 0.35 {
		tier = "MID"
	}
	sig.HasRegime = true
	sig.RegimeRV24 = math.Round(rv*1000) / 1000
	sig.RegimeTier = tier
	sig.SizeFactor = 0.5
	if tier == "HIGH" {
		sig.SizeFactor = 1.0
	}
	// Only ignition_early_short gets extra modulation (spec section 6.1).
	if sig.SetupType == "ignition_early_short" {
		switch tier {
		case "MID":
			sig.Reasons = append(sig.Reasons, "区制MID: 建议半仓")
		case "LOW":
			if sig.Score >= v7T2RVDemoteScoreKeep {
				sig.Reasons = append(sig.Reasons, "区制LOW: 高分保留推送, 半仓")
			} else {
				sig.SizeFactor = 0.0
				sig.RegimeAction = "demoted"
			}
		}
	}
}

func v7t2ApplyObserveOnly(sig *V7T2Signal, p V7T2LifecycleParams) {
	if p.observeOnly(sig.SetupType) {
		// Spec section 6.2: the setup is flagged observe_only and the
		// EVENT is forced to WATCH (armed_at stays empty, 2h expiry,
		// notification suppressed). Production payloads show the
		// payload's own status field keeps the function-time value, so
		// the forcing is modeled at the event level via EffectiveStatus
		// rather than by rewriting Status here.
		sig.ObserveOnly = true
		sig.Reasons = append(sig.Reasons, "观察名单(v0.7.4): 强制WATCH, 只记录不推送")
	}
}

// EffectiveStatus is the lifecycle status after section 6.2 forcing:
// observe-only signals are WATCH at the event level regardless of the
// function-time status.
func (sig *V7T2Signal) EffectiveStatus() string {
	if sig.ObserveOnly {
		return "WATCH"
	}
	return sig.Status
}

// ----------------------------------------------------------------------------
// Dispatcher: ports score_snapshot (spec section 3)
// ----------------------------------------------------------------------------

// EvaluateV7T2Snapshot runs the full dispatcher over one snapshot and
// returns the emitted signal, or nil. Pure: no lifecycle state is touched
// (see V7T2EventStore for the upsert/cooldown layer).
func EvaluateV7T2Snapshot(s *V7T2Snapshot, cfg *V7T2Config, regime *V7T2RegimeInput) *V7T2Signal {
	if s == nil {
		return nil
	}
	if cfg != nil && cfg.SymbolValidityGate && !V7T2SymbolValid(s.Symbol) {
		return nil
	}
	if cfg != nil && cfg.activityExcluded(s.Symbol) {
		return nil
	}
	// Behavior isolation: >= 2 flags -> SUSPECT, no signal (spec 2.9/3.1).
	if flags := V7T2ActivityFlags(s); len(flags) >= 2 {
		return nil
	}
	// Depth is a hard input upstream: an unusable book skips the symbol
	// for every family, so a missing book fails all depth-reading gates
	// uniformly here instead of leaking through "<=" comparisons.
	depthOK := s.Depth.Available

	var sig *V7T2Signal
	// Momentum channel first (spec section 3 step 2).
	if depthOK {
		sig = v7t2Momentum(s)
	}
	if sig == nil {
		// Layer 1 hard gates (spec section 3 step 3): failure is final,
		// the fallback chain is NOT consulted.
		price := s.Price()
		if price <= 0 || !depthOK || s.Depth.SpreadBps > 25 ||
			s.QuoteVolume < v7T2MinVolumeUSD ||
			s.OI.NotionalUSD < v7T2MinOINotionalUSD || s.OI.NotionalUSD > v7T2MaxOINotionalUSD ||
			s.OIVolumeRatio() < v7T2MinOIVolumeRatio {
			return nil
		}
		// Main chain, then fallback chain secondary -> ignition -> watch.
		var hardStop bool
		sig, hardStop = v7t2MainChain(s)
		if sig == nil && !hardStop {
			if sig = v7t2Secondary(s); sig == nil {
				if sig = v7t2Ignition(s); sig == nil {
					sig = v7t2WatchNearTrigger(s)
				}
			}
		}
	}
	if sig == nil {
		return nil
	}
	if cfg != nil && !cfg.familyEnabled(sig.SetupType) {
		return nil
	}
	// U1: a configured ARMED threshold relabels score-derived statuses.
	// Ignition and compression are hard-coded WATCH upstream (their status
	// does not derive from the score), so they are left alone.
	if lp := cfg.lifecycle(); lp.MinAlertScore > 0 && v7t2StatusFromScore(sig.SetupType) {
		sig.Status = "WATCH"
		if sig.Score >= lp.alertScore() {
			sig.Status = "ARMED"
		}
	}
	v7t2ApplyRegime(sig, regime)
	v7t2ApplyObserveOnly(sig, cfg.lifecycle())
	return sig
}

// ----------------------------------------------------------------------------
// Conversion into the v7 pipeline output type
// ----------------------------------------------------------------------------

// ToV7SignalOutput converts a Task-2 signal into the hunter_v7 output
// contract. Mapping (documented in M3_REPORT.md): ARMED -> candidate,
// WATCH -> wait_confirm; the scanner score fills both SetupScore and
// AIPriority so downstream priority sorting keeps the scanner's ordering.
func (sig *V7T2Signal) ToV7SignalOutput(regime V7MarketRegime) V7SignalOutput {
	out := V7SignalOutput{
		Symbol:         sig.Symbol,
		Direction:      V7Direction(sig.Direction),
		SetupType:      V7SetupType(sig.SetupType),
		Stack:          V7StackT2,
		SetupScore:     sig.Score,
		AIPriority:     sig.Score,
		ReasonCodes:    append([]string(nil), sig.Reasons...),
		MarketRegime:   regime,
		QuoteVolume24h: 0,
	}
	if sig.Direction == "LONG" {
		out.EntryZone = V7PriceZone{Lower: math.Min(sig.Entry, sig.Trigger), Upper: math.Max(sig.Entry, sig.Trigger)}
	} else {
		out.EntryZone = V7PriceZone{Lower: math.Min(sig.Entry, sig.Trigger), Upper: math.Max(sig.Entry, sig.Trigger)}
	}
	out.Invalidation = V7InvalidationRule{Price: sig.Stop, Reason: "t2_scanner_stop"}
	out.Targets = []V7Target{
		{Price: sig.Target1, Reason: "t2_tp1_1.8R"},
		{Price: sig.Target2, Reason: "t2_tp2_3.0R"},
	}
	out.TP1Price = sig.Target1
	out.TP2Price = sig.Target2
	if sig.EffectiveStatus() == "ARMED" {
		out.Status = V7StatusCandidate
		out.Confidence = "B"
		out.EntryMode = V7EntryImmediate
	} else {
		out.Status = V7StatusWaitConfirm
		out.Confidence = "C"
		out.EntryMode = V7EntryWaitConfirm
	}
	if sig.ObserveOnly {
		out.RiskTags = append(out.RiskTags, "t2_observe_only")
	}
	if sig.HasRegime && sig.RegimeAction == "demoted" {
		out.RiskTags = append(out.RiskTags, "t2_regime_demoted")
	}
	out.RiskLevel = V7RiskMedium
	out.LifecycleState = sig.EffectiveStatus()
	return out
}

// ToV7SignalOutputWithLifecycle converts with the event's lifecycle state
// (U1/P1). A TRIGGERED event (price reached the entry, spec section 5.3) is
// emitted as a candidate with the t2_triggered tag, whatever its score;
// observe-only setups stay wait_confirm (OD-5: the observe-only list stands).
func (sig *V7T2Signal) ToV7SignalOutputWithLifecycle(regime V7MarketRegime, state string) V7SignalOutput {
	out := sig.ToV7SignalOutput(regime)
	if state == "" {
		return out
	}
	out.LifecycleState = state
	if state == "TRIGGERED" {
		out.RiskTags = append(out.RiskTags, "t2_triggered")
		if !sig.ObserveOnly {
			out.Status = V7StatusCandidate
			out.Confidence = "B"
			out.EntryMode = V7EntryImmediate
		}
	}
	return out
}

// DiagnoseV7T2Families runs each family function independently (no
// dispatcher ordering, no Layer-1 gate for the main chain, no section 6
// post-processing) and reports what each would emit. Diagnostics only —
// used by the M3 recall harness to attribute misses to a specific rule.
func DiagnoseV7T2Families(s *V7T2Snapshot) map[string]string {
	out := map[string]string{}
	if sig := v7t2Momentum(s); sig != nil {
		out["momentum"] = sig.SetupType + "/" + sig.Direction
	}
	if sig, hard := v7t2MainChain(s); sig != nil {
		out["main"] = sig.SetupType + "/" + sig.Direction
	} else if hard {
		out["main"] = "hard_stop_geometry"
	}
	if sig := v7t2Secondary(s); sig != nil {
		out["secondary"] = sig.SetupType + "/" + sig.Direction
	}
	if sig := v7t2Ignition(s); sig != nil {
		out["ignition"] = sig.SetupType + "/" + sig.Direction
	}
	if sig := v7t2WatchNearTrigger(s); sig != nil {
		out["watch"] = sig.SetupType + "/" + sig.Direction
	}
	return out
}

// v7t2StatusFromScore reports whether a setup's function-time status is
// v7t2StatusForScore(score) (momentum, bb_oi and the secondary chain) rather
// than hard-coded WATCH (ignition, compression watch).
func v7t2StatusFromScore(setup string) bool {
	return !strings.HasPrefix(setup, "ignition_") && !strings.HasPrefix(setup, "compression_oi_watch_")
}
