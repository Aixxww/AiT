package trader

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	local "github.com/Aixxww/AiT/provider/local"
)

// ============================================================================
// Hunter v8 — Signal PnL Tracking Engine (P0-A)
// ============================================================================
// Tracks active signals using live price data to measure win/loss outcomes,
// MFE (Max Favorable Excursion), and MAE (Max Adverse Excursion).  Results
// feed back into setup-type expectancy scoring.

// TrackedStatus represents the lifecycle of a tracked signal.
type TrackedStatus string

const (
	TrackedActive  TrackedStatus = "ACTIVE"
	TrackedWinTP0  TrackedStatus = "WIN_TP0"
	TrackedWinTP1  TrackedStatus = "WIN_TP1"
	TrackedWinTP2  TrackedStatus = "WIN_TP2"
	TrackedStop    TrackedStatus = "STOP"
	TrackedTimeout TrackedStatus = "TIMEOUT"
)

// PriceSnapshot captures a single point-in-time price observation.
type PriceSnapshot struct {
	T        time.Time `json:"t"`
	Price    float64   `json:"price"`
	High     float64   `json:"high,omitempty"`
	Low      float64   `json:"low,omitempty"`
	Volume   float64   `json:"volume,omitempty"`
	PnLPct   float64   `json:"pnl_pct"`
	StopUsed float64   `json:"stop_used,omitempty"`
}

// TrackedCandle captures one candle used for high/low TP/SL adjudication.
type TrackedCandle struct {
	T      time.Time `json:"t"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
}

// TrackedOutcome is emitted whenever a signal receives a fresh tracking update.
type TrackedOutcome struct {
	RecordID                int64         `json:"record_id"`
	Status                  TrackedStatus `json:"status"`
	CurrentPrice            float64       `json:"current_price"`
	ExitPrice               float64       `json:"exit_price,omitempty"`
	StopUsed                float64       `json:"stop_used,omitempty"`
	MaxFavorable            float64       `json:"max_favorable"`
	MaxAdverse              float64       `json:"max_adverse"`
	PnLPct                  float64       `json:"pnl_pct"`
	MissedOpportunityAudit  bool          `json:"missed_opportunity_audit,omitempty"`
	MissedOpportunityReason string        `json:"missed_opportunity_reason,omitempty"`
	MissedOpportunityAt     *time.Time    `json:"missed_opportunity_at,omitempty"`
	SnapshotJSON            string        `json:"snapshot_json,omitempty"`
	ExitTime                *time.Time    `json:"exit_time,omitempty"`
}

// TrackedSignal holds all state for a single signal being monitored.
type TrackedSignal struct {
	RecordID      int64           `json:"record_id"`
	Symbol        string          `json:"symbol"`
	Direction     string          `json:"direction"`
	SetupType     string          `json:"setup_type"`
	Tier          string          `json:"tier"`
	SignalTime    time.Time       `json:"signal_time"`
	SignalPrice   float64         `json:"signal_price"`
	StopPrice     float64         `json:"stop_price"`
	TP0Price      float64         `json:"tp0_price"`
	TP1Price      float64         `json:"tp1_price"`
	TP2Price      float64         `json:"tp2_price"`
	DynamicStop   float64         `json:"dynamic_stop,omitempty"`
	LastCandleAt  time.Time       `json:"last_candle_at,omitempty"`
	LastOutcomeAt time.Time       `json:"last_outcome_at,omitempty"`
	Status        TrackedStatus   `json:"status"`
	CurrentPrice  float64         `json:"current_price"`
	MaxFavorable  float64         `json:"max_favorable"` // MFE %
	MaxAdverse    float64         `json:"max_adverse"`   // MAE %
	ExitTime      *time.Time      `json:"exit_time,omitempty"`
	ExitPrice     float64         `json:"exit_price"`
	Snapshots     []PriceSnapshot `json:"snapshots,omitempty"`

	TP0TouchStreak          int        `json:"tp0_touch_streak,omitempty"`
	MissedOpportunityAudit  bool       `json:"missed_opportunity_audit,omitempty"`
	MissedOpportunityReason string     `json:"missed_opportunity_reason,omitempty"`
	MissedOpportunityAt     *time.Time `json:"missed_opportunity_at,omitempty"`
}

// SetupStats aggregates win/loss statistics per setup type.
type SetupStats struct {
	SetupType           string        `json:"setup_type"`
	Total               int           `json:"total"`
	Wins                int           `json:"wins"`
	Losses              int           `json:"losses"`
	Stops               int           `json:"stops"`
	Timeouts            int           `json:"timeouts"`
	TP0Wins             int           `json:"tp0_wins"`
	TP1Wins             int           `json:"tp1_wins"`
	TP2Wins             int           `json:"tp2_wins"`
	WinRate             float64       `json:"win_rate"`
	TP0WinRate          float64       `json:"tp0_win_rate"`
	TP1WinRate          float64       `json:"tp1_win_rate"`
	NoSLSurvivalRate    float64       `json:"no_sl_survival_rate"`
	AvgPnL              float64       `json:"avg_pnl"`
	AvgMFE              float64       `json:"avg_mfe"`
	AvgMAE              float64       `json:"avg_mae"`
	AvgDuration         time.Duration `json:"avg_duration"`
	MissedOpportunities int           `json:"missed_opportunities,omitempty"`
}

// TrackerConfig holds configuration for the signal outcome tracker.
type TrackerConfig struct {
	PollInterval                time.Duration // default 1 minute
	TimeoutDuration             time.Duration // default 8 hours
	MaxTracked                  int           // default 200
	SnapshotLimit               int           // default 480 = 8h @ 1m
	ActiveOutcomeInterval       time.Duration // default 5 minutes; terminal outcomes always emit
	DuplicateWindow             time.Duration // default 30 minutes; same symbol/setup/direction keeps latest active thesis only
	BreakevenMFEThreshold       float64       // default 0.60%; setup-specific breakeven protection after enough MFE
	EnableDynamicStop           bool          // default true
	ATRPercentile               float64       // default 50 when no volatility percentile is available
	MissedOpportunityMinTouches int           // default 2 consecutive TP0 touches for WATCH audit
}

// DefaultTrackerConfig returns sensible defaults.
func DefaultTrackerConfig() *TrackerConfig {
	return &TrackerConfig{
		PollInterval:                1 * time.Minute,
		TimeoutDuration:             8 * time.Hour,
		MaxTracked:                  200,
		SnapshotLimit:               480,
		ActiveOutcomeInterval:       5 * time.Minute,
		DuplicateWindow:             30 * time.Minute,
		BreakevenMFEThreshold:       0.60,
		EnableDynamicStop:           true,
		ATRPercentile:               50,
		MissedOpportunityMinTouches: 2,
	}
}

// SignalOutcomeTracker monitors active signals for TP/SL/timeout outcomes.
type SignalOutcomeTracker struct {
	db            *gorm.DB
	config        *TrackerConfig
	mu            sync.RWMutex
	activeSignals map[int64]*TrackedSignal
	activeThesis  map[string]int64
	priceFunc     func(symbol string) float64
	candleFunc    func(symbol string) *TrackedCandle
	candlesFunc   func(symbol string, since time.Time) []TrackedCandle
	backfillFunc  func(symbol string, from, to time.Time) []TrackedCandle
	outcomeFunc   func(TrackedOutcome)
	dynamicStop   *DynamicStopManager
	completed     []*TrackedSignal // archived after terminal state
}

// NewSignalOutcomeTracker creates a new tracker.  Pass nil db for in-memory only.
func NewSignalOutcomeTracker(cfg *TrackerConfig, priceFunc func(string) float64) *SignalOutcomeTracker {
	if cfg == nil {
		cfg = DefaultTrackerConfig()
	} else {
		cfg = normalizeTrackerConfig(cfg)
	}
	return &SignalOutcomeTracker{
		config:        cfg,
		activeSignals: make(map[int64]*TrackedSignal),
		activeThesis:  make(map[string]int64),
		priceFunc:     priceFunc,
		dynamicStop:   DefaultDynamicStopManager(),
	}
}

func normalizeTrackerConfig(cfg *TrackerConfig) *TrackerConfig {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Minute
	}
	if cfg.TimeoutDuration <= 0 {
		cfg.TimeoutDuration = 8 * time.Hour
	}
	if cfg.MaxTracked <= 0 {
		cfg.MaxTracked = 200
	}
	if cfg.SnapshotLimit <= 0 {
		cfg.SnapshotLimit = 480
	}
	if cfg.ActiveOutcomeInterval <= 0 {
		cfg.ActiveOutcomeInterval = 5 * time.Minute
	}
	if cfg.DuplicateWindow <= 0 {
		cfg.DuplicateWindow = 30 * time.Minute
	}
	if cfg.BreakevenMFEThreshold <= 0 {
		cfg.BreakevenMFEThreshold = 0.60
	}
	if cfg.ATRPercentile <= 0 {
		cfg.ATRPercentile = 50
	}
	return cfg
}

// SetCandleSource registers a high/low candle source. When present, tracker
// adjudicates TP/SL with candle high/low instead of point price.
func (t *SignalOutcomeTracker) SetCandleSource(fn func(string) *TrackedCandle) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.candleFunc = fn
}

// SetCandleHistorySource registers a candle history source for replaying all
// unprocessed 1m candles since the signal was created. This closes the gap
// between signal creation and the first tracker tick.
func (t *SignalOutcomeTracker) SetCandleHistorySource(fn func(symbol string, since time.Time) []TrackedCandle) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.candlesFunc = fn
}

// SetCandleBackfillSource registers a fallback source for 1m REST backfill.
// It is used only when the history source does not cover the signal window.
func (t *SignalOutcomeTracker) SetCandleBackfillSource(fn func(symbol string, from, to time.Time) []TrackedCandle) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.backfillFunc = fn
}

// SetOutcomeCallback registers a callback for active and terminal tracking updates.
func (t *SignalOutcomeTracker) SetOutcomeCallback(fn func(TrackedOutcome)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.outcomeFunc = fn
}

// Register adds a new signal for tracking.
func (t *SignalOutcomeTracker) Register(
	recordID int64,
	symbol, direction, setupType, tier string,
	signalPrice, stopPrice, tp0, tp1, tp2 float64,
	signalTime ...time.Time,
) (bool, int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.activeThesis == nil {
		t.activeThesis = make(map[string]int64)
	}

	if _, exists := t.activeSignals[recordID]; exists {
		return false, 0 // already tracked
	}

	registeredAt := time.Now()
	if len(signalTime) > 0 && !signalTime[0].IsZero() {
		registeredAt = signalTime[0]
	}

	replacedID := int64(0)
	key := trackedThesisKey(symbol, setupType, direction)
	if t.config.DuplicateWindow > 0 && key != "" {
		if existingID, ok := t.activeThesis[key]; ok {
			if existing := t.activeSignals[existingID]; existing != nil {
				if !registeredAt.After(existing.SignalTime) {
					return false, 0
				}
				if registeredAt.Sub(existing.SignalTime) <= t.config.DuplicateWindow {
					delete(t.activeSignals, existingID)
					replacedID = existingID
				}
			}
		}
	}
	if len(t.activeSignals) >= t.config.MaxTracked {
		return false, 0 // at capacity; caller should evict oldest
	}

	t.activeSignals[recordID] = &TrackedSignal{
		RecordID:    recordID,
		Symbol:      symbol,
		Direction:   direction,
		SetupType:   setupType,
		Tier:        tier,
		SignalTime:  registeredAt,
		SignalPrice: signalPrice,
		StopPrice:   stopPrice,
		TP0Price:    tp0,
		TP1Price:    tp1,
		TP2Price:    tp2,
		Status:      TrackedActive,
		Snapshots:   make([]PriceSnapshot, 0, 8),
	}
	if key != "" {
		t.activeThesis[key] = recordID
	}
	return true, replacedID
}

func trackedThesisKey(symbol, setupType, direction string) string {
	if symbol == "" || setupType == "" || direction == "" {
		return ""
	}
	return symbol + "\x00" + setupType + "\x00" + direction
}

// TickNow runs one tracking cycle immediately. It is primarily used by tests
// and by callers that want an initial update without waiting for the ticker.
func (t *SignalOutcomeTracker) TickNow() {
	t.tick()
}

// Start launches a background goroutine that polls prices at the configured interval.
func (t *SignalOutcomeTracker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(t.config.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.tick()
			}
		}
	}()
}

// tick updates all active signals and checks TP/SL/timeout.
func (t *SignalOutcomeTracker) tick() {
	if t.priceFunc == nil && t.candleFunc == nil && t.candlesFunc == nil && t.backfillFunc == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	var toArchive []*TrackedSignal
	var outcomes []TrackedOutcome

	for id, sig := range t.activeSignals {
		candles := t.candlesForSignal(sig, now)
		if len(candles) == 0 {
			continue
		}

		pnlPct := 0.0
		terminal := false
		processed := false
		auditChanged := false
		for _, candle := range candles {
			if !sig.LastCandleAt.IsZero() && !candle.T.After(sig.LastCandleAt) {
				continue
			}
			price := candle.Close
			if price <= 0 {
				continue
			}

			sig.LastCandleAt = candle.T
			processed = true
			sig.CurrentPrice = price
			pnlPct = t.calcPnLPct(sig, price)
			mfePct, maePct := t.calcExcursionsPct(sig, candle)

			// Update MFE/MAE
			if mfePct > sig.MaxFavorable {
				sig.MaxFavorable = mfePct
			}
			if maePct < sig.MaxAdverse {
				sig.MaxAdverse = maePct
			}

			stopUsed := t.updateDynamicStop(sig, price, candle.T)
			t.appendSnapshot(sig, candle, pnlPct, stopUsed)
			if t.updateMissedOpportunityAudit(sig, candle) {
				auditChanged = true
			}

			var exitPrice float64
			terminal, exitPrice = t.checkTerminalWithCandle(sig, candle, stopUsed)
			if terminal {
				exitTime := candle.T
				sig.ExitTime = &exitTime
				sig.ExitPrice = exitPrice
				pnlPct = t.calcPnLPct(sig, sig.ExitPrice)
				break
			}
		}

		// Timeout check
		if !terminal && now.Sub(sig.SignalTime) > t.config.TimeoutDuration {
			sig.Status = TrackedTimeout
			sig.ExitTime = &now
			sig.ExitPrice = sig.CurrentPrice
			pnlPct = t.calcPnLPct(sig, sig.ExitPrice)
			terminal = true
		}
		if !processed && !terminal {
			continue
		}

		if terminal {
			toArchive = append(toArchive, sig)
			delete(t.activeSignals, id)
			key := trackedThesisKey(sig.Symbol, sig.SetupType, sig.Direction)
			if t.activeThesis[key] == sig.RecordID {
				delete(t.activeThesis, key)
			}
		}

		if terminal || auditChanged || t.shouldEmitActiveOutcome(sig, now) {
			outcomes = append(outcomes, t.buildOutcome(sig, pnlPct))
			sig.LastOutcomeAt = now
		}
	}

	// Archive completed signals
	t.completed = append(t.completed, toArchive...)
	// Keep completed list bounded
	if len(t.completed) > 1000 {
		t.completed = t.completed[len(t.completed)-1000:]
	}

	outcomeFunc := t.outcomeFunc
	if outcomeFunc != nil {
		for _, outcome := range outcomes {
			outcomeFunc(outcome)
		}
	}
}

func (t *SignalOutcomeTracker) shouldEmitActiveOutcome(sig *TrackedSignal, now time.Time) bool {
	if sig == nil {
		return false
	}
	if sig.LastOutcomeAt.IsZero() {
		return true
	}
	interval := t.config.ActiveOutcomeInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return now.Sub(sig.LastOutcomeAt) >= interval
}

func (t *SignalOutcomeTracker) latestCandle(symbol string) TrackedCandle {
	if t.candleFunc != nil {
		if candle := t.candleFunc(symbol); candle != nil && candle.Close > 0 {
			if candle.T.IsZero() {
				candle.T = time.Now()
			}
			if candle.High <= 0 {
				candle.High = candle.Close
			}
			if candle.Low <= 0 {
				candle.Low = candle.Close
			}
			return *candle
		}
	}
	return TrackedCandle{}
}

func (t *SignalOutcomeTracker) candlesForSignal(sig *TrackedSignal, now time.Time) []TrackedCandle {
	var candles []TrackedCandle
	since := sig.SignalTime
	if !sig.LastCandleAt.IsZero() {
		since = sig.LastCandleAt
	}
	if t.candlesFunc != nil {
		for _, candle := range t.candlesFunc(sig.Symbol, since) {
			if normalized, ok := normalizeTrackedCandle(candle, now); ok {
				candles = append(candles, normalized)
			}
		}
	}
	if t.backfillFunc != nil && shouldBackfillCandles(sig, candles, since) {
		for _, candle := range t.backfillFunc(sig.Symbol, since, now) {
			if normalized, ok := normalizeTrackedCandle(candle, now); ok {
				candles = append(candles, normalized)
			}
		}
	}
	if len(candles) == 0 {
		candle := t.latestCandle(sig.Symbol)
		if normalized, ok := normalizeTrackedCandle(candle, now); ok {
			candles = append(candles, normalized)
		}
	}
	if len(candles) == 0 && t.priceFunc != nil {
		price := t.priceFunc(sig.Symbol)
		if price > 0 {
			candles = append(candles, TrackedCandle{T: now, High: price, Low: price, Close: price})
		}
	}
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].T.Before(candles[j].T)
	})
	return candles
}

func shouldBackfillCandles(sig *TrackedSignal, candles []TrackedCandle, since time.Time) bool {
	if sig == nil {
		return false
	}
	if !sig.LastCandleAt.IsZero() {
		return false
	}
	if len(candles) == 0 {
		return true
	}
	first := candles[0].T
	for _, candle := range candles[1:] {
		if candle.T.Before(first) {
			first = candle.T
		}
	}
	if since.IsZero() {
		since = sig.SignalTime
	}
	return first.After(since.Add(90 * time.Second))
}

func normalizeTrackedCandle(candle TrackedCandle, fallbackTime time.Time) (TrackedCandle, bool) {
	if candle.Close <= 0 {
		return TrackedCandle{}, false
	}
	if candle.T.IsZero() {
		candle.T = fallbackTime
	}
	if candle.High <= 0 {
		candle.High = candle.Close
	}
	if candle.Low <= 0 {
		candle.Low = candle.Close
	}
	if candle.Open <= 0 {
		candle.Open = candle.Close
	}
	return candle, true
}

func (t *SignalOutcomeTracker) appendSnapshot(sig *TrackedSignal, candle TrackedCandle, pnlPct, stopUsed float64) {
	if t.config.SnapshotLimit <= 0 {
		t.config.SnapshotLimit = 480
	}
	if len(sig.Snapshots) >= t.config.SnapshotLimit {
		return
	}
	sig.Snapshots = append(sig.Snapshots, PriceSnapshot{
		T:        candle.T,
		Price:    candle.Close,
		High:     candle.High,
		Low:      candle.Low,
		Volume:   candle.Volume,
		PnLPct:   pnlPct,
		StopUsed: stopUsed,
	})
}

func (t *SignalOutcomeTracker) updateDynamicStop(sig *TrackedSignal, currentPrice float64, now time.Time) float64 {
	stopUsed := sig.StopPrice
	if t.config == nil || !t.config.EnableDynamicStop || t.dynamicStop == nil {
		return stopUsed
	}
	isLong := sig.Direction == string(local.V7DirLong)
	maxFavorableDelta := sig.MaxFavorable / 100 * sig.SignalPrice
	if maxFavorableDelta < 0 {
		maxFavorableDelta = 0
	}
	atrPercentile := t.config.ATRPercentile
	if atrPercentile <= 0 {
		atrPercentile = 50
	}
	dyn := t.dynamicStop.CalcDynamicStop(sig.SignalPrice, sig.StopPrice, currentPrice, maxFavorableDelta, now.Sub(sig.SignalTime), atrPercentile, isLong)
	if dyn <= 0 {
		return stopUsed
	}
	if isLong {
		if sig.DynamicStop <= 0 || dyn > sig.DynamicStop {
			sig.DynamicStop = dyn
		}
	} else if sig.DynamicStop <= 0 || dyn < sig.DynamicStop {
		sig.DynamicStop = dyn
	}
	if protected := t.breakevenStop(sig); protected > 0 {
		if isLong {
			if sig.DynamicStop <= 0 || protected > sig.DynamicStop {
				sig.DynamicStop = protected
			}
		} else if sig.DynamicStop <= 0 || protected < sig.DynamicStop {
			sig.DynamicStop = protected
		}
	}
	if sig.DynamicStop > 0 {
		stopUsed = sig.DynamicStop
	}
	return stopUsed
}

func (t *SignalOutcomeTracker) breakevenStop(sig *TrackedSignal) float64 {
	if sig == nil || sig.SignalPrice <= 0 {
		return 0
	}
	threshold := 0.60
	if t.config != nil && t.config.BreakevenMFEThreshold > 0 {
		threshold = t.config.BreakevenMFEThreshold
	}
	if sig.MaxFavorable < threshold {
		return 0
	}
	if sig.SetupType == string(local.V7SetupAltLadderShort) && sig.Direction == string(local.V7DirShort) {
		return sig.SignalPrice
	}
	return 0
}

func (t *SignalOutcomeTracker) updateMissedOpportunityAudit(sig *TrackedSignal, candle TrackedCandle) bool {
	if sig == nil || sig.Tier != "WATCH" || sig.TP0Price <= 0 || sig.MissedOpportunityAudit {
		return false
	}
	touched := false
	switch sig.Direction {
	case string(local.V7DirLong):
		touched = candle.High >= sig.TP0Price
	case string(local.V7DirShort):
		touched = candle.Low <= sig.TP0Price
	}
	if !touched {
		sig.TP0TouchStreak = 0
		return false
	}
	sig.TP0TouchStreak++
	minTouches := 2
	if t.config != nil && t.config.MissedOpportunityMinTouches > 0 {
		minTouches = t.config.MissedOpportunityMinTouches
	}
	if sig.TP0TouchStreak < minTouches {
		return false
	}
	auditAt := candle.T
	sig.MissedOpportunityAudit = true
	sig.MissedOpportunityAt = &auditAt
	sig.MissedOpportunityReason = "watch_mfe_reached_tp0_threshold"
	return true
}

func (t *SignalOutcomeTracker) checkTerminalWithCandle(sig *TrackedSignal, candle TrackedCandle, stopUsed float64) (bool, float64) {
	if sig.Tier == "WATCH" {
		return false, 0
	}
	switch sig.Direction {
	case string(local.V7DirLong):
		if stopUsed > 0 && candle.Low <= stopUsed {
			sig.Status = TrackedStop
			return true, stopUsed
		}
		if sig.TP2Price > 0 && candle.High >= sig.TP2Price {
			sig.Status = TrackedWinTP2
			return true, sig.TP2Price
		}
		if sig.TP1Price > 0 && candle.High >= sig.TP1Price {
			sig.Status = TrackedWinTP1
			return true, sig.TP1Price
		}
		if sig.TP0Price > 0 && candle.High >= sig.TP0Price {
			sig.Status = TrackedWinTP0
			return true, sig.TP0Price
		}
	case string(local.V7DirShort):
		if stopUsed > 0 && candle.High >= stopUsed {
			sig.Status = TrackedStop
			return true, stopUsed
		}
		if sig.TP2Price > 0 && candle.Low <= sig.TP2Price {
			sig.Status = TrackedWinTP2
			return true, sig.TP2Price
		}
		if sig.TP1Price > 0 && candle.Low <= sig.TP1Price {
			sig.Status = TrackedWinTP1
			return true, sig.TP1Price
		}
		if sig.TP0Price > 0 && candle.Low <= sig.TP0Price {
			sig.Status = TrackedWinTP0
			return true, sig.TP0Price
		}
	}
	return false, 0
}

func (t *SignalOutcomeTracker) buildOutcome(sig *TrackedSignal, pnlPct float64) TrackedOutcome {
	exitPrice := sig.ExitPrice
	if exitPrice <= 0 {
		exitPrice = sig.CurrentPrice
	}
	snapshotJSON := ""
	if len(sig.Snapshots) > 0 {
		b, _ := json.Marshal(sig.Snapshots)
		snapshotJSON = string(b)
	}
	return TrackedOutcome{
		RecordID:                sig.RecordID,
		Status:                  sig.Status,
		CurrentPrice:            sig.CurrentPrice,
		ExitPrice:               exitPrice,
		StopUsed:                sig.DynamicStop,
		MaxFavorable:            sig.MaxFavorable,
		MaxAdverse:              sig.MaxAdverse,
		PnLPct:                  pnlPct,
		MissedOpportunityAudit:  sig.MissedOpportunityAudit,
		MissedOpportunityReason: sig.MissedOpportunityReason,
		MissedOpportunityAt:     sig.MissedOpportunityAt,
		SnapshotJSON:            snapshotJSON,
		ExitTime:                sig.ExitTime,
	}
}

// GetActive returns all currently active tracked signals.
func (t *SignalOutcomeTracker) GetActive() []*TrackedSignal {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]*TrackedSignal, 0, len(t.activeSignals))
	for _, sig := range t.activeSignals {
		out = append(out, sig)
	}
	return out
}

// GetByStatus returns all tracked signals (active + completed) matching a status.
func (t *SignalOutcomeTracker) GetByStatus(status TrackedStatus) []*TrackedSignal {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var out []*TrackedSignal
	for _, sig := range t.activeSignals {
		if sig.Status == status {
			out = append(out, sig)
		}
	}
	for _, sig := range t.completed {
		if sig.Status == status {
			out = append(out, sig)
		}
	}
	return out
}

// GetStatsBySetupType computes aggregate win/loss/MFE/MAE per setup type.
func (t *SignalOutcomeTracker) GetStatsBySetupType() map[string]SetupStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	buckets := make(map[string]*SetupStats)
	all := make([]*TrackedSignal, 0, len(t.activeSignals)+len(t.completed))
	for _, sig := range t.activeSignals {
		all = append(all, sig)
	}
	all = append(all, t.completed...)

	for _, sig := range all {
		if sig.Tier == "WATCH" {
			st, ok := buckets[sig.SetupType]
			if !ok {
				st = &SetupStats{SetupType: sig.SetupType}
				buckets[sig.SetupType] = st
			}
			if sig.MissedOpportunityAudit {
				st.MissedOpportunities++
			}
			continue
		}
		if sig.Status == TrackedActive {
			continue // only count completed
		}
		st, ok := buckets[sig.SetupType]
		if !ok {
			st = &SetupStats{SetupType: sig.SetupType}
			buckets[sig.SetupType] = st
		}
		st.Total++
		pnl := t.calcPnLPct(sig, sig.ExitPrice)
		switch sig.Status {
		case TrackedWinTP0:
			st.Wins++
			st.TP0Wins++
		case TrackedWinTP1:
			st.Wins++
			st.TP0Wins++
			st.TP1Wins++
		case TrackedWinTP2:
			st.Wins++
			st.TP0Wins++
			st.TP1Wins++
			st.TP2Wins++
		case TrackedStop:
			st.Losses++
			st.Stops++
		case TrackedTimeout:
			st.Timeouts++
		}
		st.AvgPnL += pnl
		st.AvgMFE += sig.MaxFavorable
		st.AvgMAE += sig.MaxAdverse
		if sig.ExitTime != nil {
			st.AvgDuration += sig.ExitTime.Sub(sig.SignalTime)
		}
	}

	result := make(map[string]SetupStats, len(buckets))
	for k, st := range buckets {
		if st.Total > 0 {
			st.WinRate = float64(st.Wins) / float64(st.Total) * 100
			st.TP0WinRate = float64(st.TP0Wins) / float64(st.Total) * 100
			st.TP1WinRate = float64(st.TP1Wins) / float64(st.Total) * 100
			st.NoSLSurvivalRate = float64(st.Total-st.Stops) / float64(st.Total) * 100
			st.AvgPnL /= float64(st.Total)
			st.AvgMFE /= float64(st.Total)
			st.AvgMAE /= float64(st.Total)
			st.AvgDuration /= time.Duration(st.Total)
		}
		result[k] = *st
	}
	return result
}

// GetTrackedSignalsJSON returns a JSON string of all active + completed signals
// suitable for API consumption.
func (t *SignalOutcomeTracker) GetTrackedSignalsJSON() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	all := make([]*TrackedSignal, 0, len(t.activeSignals)+len(t.completed))
	for _, sig := range t.activeSignals {
		all = append(all, sig)
	}
	all = append(all, t.completed...)

	b, _ := json.Marshal(all)
	return string(b)
}

// calcPnLPct computes the PnL percentage for a tracked signal at a given price.
func (t *SignalOutcomeTracker) calcPnLPct(sig *TrackedSignal, price float64) float64 {
	if sig.SignalPrice <= 0 {
		return 0
	}
	switch sig.Direction {
	case string(local.V7DirLong):
		return (price - sig.SignalPrice) / sig.SignalPrice * 100
	case string(local.V7DirShort):
		return (sig.SignalPrice - price) / sig.SignalPrice * 100
	}
	return 0
}

func (t *SignalOutcomeTracker) calcExcursionsPct(sig *TrackedSignal, candle TrackedCandle) (float64, float64) {
	if sig.SignalPrice <= 0 {
		return 0, 0
	}
	high := candle.High
	low := candle.Low
	if high <= 0 {
		high = candle.Close
	}
	if low <= 0 {
		low = candle.Close
	}
	switch sig.Direction {
	case string(local.V7DirLong):
		mfe := (high - sig.SignalPrice) / sig.SignalPrice * 100
		mae := (low - sig.SignalPrice) / sig.SignalPrice * 100
		return mfe, mae
	case string(local.V7DirShort):
		mfe := (sig.SignalPrice - low) / sig.SignalPrice * 100
		mae := (sig.SignalPrice - high) / sig.SignalPrice * 100
		return mfe, mae
	}
	return 0, 0
}
