package store

import "time"

// U14: the live position protector (plan §7.1.2 "model A") in config, with
// every PnL threshold carrying its unit (OD-25). The defaults are today's
// values in today's units, so behaviour is unchanged (OD-22 (a), §10.3);
// switching a level to "r" is a config change. Read through
// EffectiveProtector.

// Level units.
const (
	// ProtectorUnitROEPct: leveraged PnL in percent of margin (ROE %).
	ProtectorUnitROEPct = "roe_pct"
	// ProtectorUnitPricePct: unleveraged price move from entry, percent.
	ProtectorUnitPricePct = "price_pct"
	// ProtectorUnitR: multiples of the planned stop distance (R). A level in
	// R needs the position's planned stop; without one the rule does not
	// fire (and is logged).
	ProtectorUnitR = "r"
)

// ProtectorLevel is one PnL threshold with its unit. An empty unit means
// the default level.
type ProtectorLevel struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

func roe(v float64) ProtectorLevel   { return ProtectorLevel{Value: v, Unit: ProtectorUnitROEPct} }
func price(v float64) ProtectorLevel { return ProtectorLevel{Value: v, Unit: ProtectorUnitPricePct} }

// ValidProtectorUnit reports whether u is a level unit.
func ValidProtectorUnit(u string) bool {
	return u == ProtectorUnitROEPct || u == ProtectorUnitPricePct || u == ProtectorUnitR
}

// ProtectorConfig is RiskControlConfig.Protector (json protector). Zero
// values (and levels without a unit) mean the defaults below.
type ProtectorConfig struct {
	// Poll cadence (seconds) and the default leverage of a position that
	// reports none.
	FastIntervalSeconds float64 `json:"fast_interval_seconds,omitempty"`
	BaseIntervalSeconds float64 `json:"base_interval_seconds,omitempty"`
	IdleIntervalSeconds float64 `json:"idle_interval_seconds,omitempty"`
	DefaultLeverage     int     `json:"default_leverage,omitempty"`
	// PlanLookbackRecords is how many recent decision records are searched
	// for the planned stop and targets of an open position.
	PlanLookbackRecords int `json:"plan_lookback_records,omitempty"`

	// Hard loss: close 100% at HardLoss, or at EarlyLoss within
	// EarlyLossWindowMinutes of the open. A planned stop wider than
	// HardLoss pushes the backstop to planned x HardLossSlackMultiplier.
	HardLoss                ProtectorLevel `json:"hard_loss"`
	HardLossSlackMultiplier float64        `json:"hard_loss_slack_multiplier,omitempty"`
	EarlyLoss               ProtectorLevel `json:"early_loss"`
	EarlyLossWindowMinutes  float64        `json:"early_loss_window_minutes,omitempty"`

	// Profit floor and profit locks.
	ProfitFloorPeak           ProtectorLevel `json:"profit_floor_peak"`
	ProfitFloorBuffer         ProtectorLevel `json:"profit_floor_buffer"`
	ProfitFloorNetBuffer      ProtectorLevel `json:"profit_floor_net_buffer"`
	ProfitLockMidPeak         ProtectorLevel `json:"profit_lock_mid_peak"`
	ProfitLockHighPeak        ProtectorLevel `json:"profit_lock_high_peak"`
	ProfitLockMidRatio        float64        `json:"profit_lock_mid_ratio,omitempty"`
	ProfitLockHighRatio       float64        `json:"profit_lock_high_ratio,omitempty"`
	ProfitLockMidDrawdownPct  float64        `json:"profit_lock_mid_drawdown_pct,omitempty"`
	ProfitLockHighDrawdownPct float64        `json:"profit_lock_high_drawdown_pct,omitempty"`

	// Take-profit stages.
	TP0        ProtectorLevel `json:"tp0"`
	TP1        ProtectorLevel `json:"tp1"`
	TP1MinMove ProtectorLevel `json:"tp1_min_move"`
	TP2        ProtectorLevel `json:"tp2"`
	TP2MinMove ProtectorLevel `json:"tp2_min_move"`
	// TrailDrawdownPct: after TP1, close on this drawdown (% of peak).
	TrailDrawdownPct float64 `json:"trail_drawdown_pct,omitempty"`

	// Pre-TP1 givebacks.
	// NearTP1Peak (default 95% of TP1) and NearTP1SecondChance (90%) are
	// separate levels so their defaults are exactly the pre-U14 values.
	NearTP1Peak         ProtectorLevel `json:"near_tp1_peak"`
	NearTP1SecondChance ProtectorLevel `json:"near_tp1_second_chance"`
	NearTP1GivebackPct  float64        `json:"near_tp1_giveback_pct,omitempty"`
	NearTP1LossExit     ProtectorLevel `json:"near_tp1_loss_exit"`
	PreTPMinCurrent     ProtectorLevel `json:"pre_tp_min_current"`
	PreTPPeak           ProtectorLevel `json:"pre_tp_peak"`
	PreTPGivebackPct    float64        `json:"pre_tp_giveback_pct,omitempty"`
	PreTPMinHoldMinutes float64        `json:"pre_tp_min_hold_minutes,omitempty"`

	// Partial sizes (fraction of the remaining quantity) and the
	// breakeven buffers (fraction of entry) of the stop after a partial.
	TP0CloseRatio               float64 `json:"tp0_close_ratio,omitempty"`
	TP1CloseRatio               float64 `json:"tp1_close_ratio,omitempty"`
	TP2CloseRatio               float64 `json:"tp2_close_ratio,omitempty"`
	GivebackCloseRatio          float64 `json:"giveback_close_ratio,omitempty"`
	BreakevenBufferFrac         float64 `json:"breakeven_buffer_frac,omitempty"`
	GivebackBreakevenBufferFrac float64 `json:"giveback_breakeven_buffer_frac,omitempty"`
	// MinCloseNotional (USDT) applies when RiskControl.MinPositionSize is 0.
	MinCloseNotional float64 `json:"min_close_notional,omitempty"`

	// Dynamic stop update.
	DynamicStopMinUpdateSeconds float64        `json:"dynamic_stop_min_update_seconds,omitempty"`
	DynamicStopEarlyHoldMinutes float64        `json:"dynamic_stop_early_hold_minutes,omitempty"`
	DynamicStopEarlyPeak        ProtectorLevel `json:"dynamic_stop_early_peak"`
	DynamicStopEarlyMove        ProtectorLevel `json:"dynamic_stop_early_move"`
	// DynamicStopBaseRisk is the distance of the synthetic base stop the
	// DynamicStopManager works on (today 35% ROE, not the planned stop).
	DynamicStopBaseRisk      ProtectorLevel `json:"dynamic_stop_base_risk"`
	DynamicStopATRPercentile float64        `json:"dynamic_stop_atr_percentile,omitempty"`
	// MinStopDeltaFrac: a new stop must beat the active one by this
	// fraction of its price.
	MinStopDeltaFrac float64 `json:"min_stop_delta_frac,omitempty"`
	// DynamicStopManager parameters (hunter_v7_dynamic_stop.go).
	HighVolThreshold      float64 `json:"high_vol_threshold,omitempty"`
	LowVolThreshold       float64 `json:"low_vol_threshold,omitempty"`
	HighVolStopWiden      float64 `json:"high_vol_stop_widen,omitempty"`
	LowVolStopTighten     float64 `json:"low_vol_stop_tighten,omitempty"`
	TimeDecayDisabled     bool    `json:"time_decay_disabled,omitempty"`
	TimeDecayAfterMinutes float64 `json:"time_decay_after_minutes,omitempty"`
	TimeDecayRate         float64 `json:"time_decay_rate,omitempty"`
	TimeDecayMaxFraction  float64 `json:"time_decay_max_fraction,omitempty"`
	TrailingDisabled      bool    `json:"trailing_disabled,omitempty"`
	TrailingActivationR   float64 `json:"trailing_activation_r,omitempty"`
	TrailingDistanceR     float64 `json:"trailing_distance_r,omitempty"`

	// Micro TP0 cap: a planned TP0 is pulled in to
	// max(MicroTP0MinMove, MicroTP0StopDistRatio x stop distance).
	MicroTP0MinMove       ProtectorLevel `json:"micro_tp0_min_move"`
	MicroTP0StopDistRatio float64        `json:"micro_tp0_stop_dist_ratio,omitempty"`

	// LogMinPnL: the per-poll status line is logged above this level.
	LogMinPnL ProtectorLevel `json:"log_min_pnl"`
}

// Defaults: the protector literals as of c35f115 (plan §7.1.2).
const (
	DefaultProtectorFastIntervalSeconds   = 15
	DefaultProtectorBaseIntervalSeconds   = 30
	DefaultProtectorIdleIntervalSeconds   = 60
	DefaultProtectorLeverage              = 10
	DefaultProtectorPlanLookbackRecords   = 50
	DefaultProtectorHardLossROEPct        = -12.0
	DefaultProtectorHardLossSlack         = 1.1
	DefaultProtectorEarlyLossROEPct       = -8.0
	DefaultProtectorEarlyLossWindowMin    = 15
	DefaultProtectorProfitFloorPeakROEPct = 5.0
	DefaultProtectorProfitFloorBufferROE  = 0.8
	DefaultProtectorProfitFloorNetROE     = 3.0
	DefaultProtectorProfitLockMidPeakROE  = 10.0
	DefaultProtectorProfitLockHighPeakROE = 15.0
	DefaultProtectorProfitLockMidRatio    = 0.30
	DefaultProtectorProfitLockHighRatio   = 0.45
	DefaultProtectorProfitLockMidDDPct    = 50.0
	DefaultProtectorProfitLockHighDDPct   = 45.0
	DefaultProtectorTP0ROEPct             = 8.0
	DefaultProtectorTP1ROEPct             = 6.0
	DefaultProtectorTP1MinMovePct         = 1.0
	DefaultProtectorTP2ROEPct             = 12.0
	DefaultProtectorTP2MinMovePct         = 1.5
	DefaultProtectorTrailDrawdownPct      = 35.0
	DefaultProtectorNearTP1PeakROE        = DefaultProtectorTP1ROEPct * 0.95
	DefaultProtectorNearTP1SecondChance   = DefaultProtectorTP1ROEPct * 0.90
	DefaultProtectorNearTP1GivebackPct    = 45.0
	DefaultProtectorNearTP1LossExitROE    = -5.0
	DefaultProtectorPreTPMinCurrentROE    = 3.0
	DefaultProtectorPreTPPeakROE          = 8.0
	DefaultProtectorPreTPGivebackPct      = 80.0
	DefaultProtectorPreTPMinHoldMin       = 20
	DefaultProtectorTP0CloseRatio         = 0.35
	DefaultProtectorTP1CloseRatio         = 0.40
	DefaultProtectorTP2CloseRatio         = 0.50
	DefaultProtectorGivebackCloseRatio    = 0.5
	DefaultProtectorBreakevenBufferFrac   = 0.001
	DefaultProtectorGivebackBreakevenFrac = 0.0015
	DefaultProtectorMinCloseNotional      = 12.0
	DefaultProtectorDynStopMinUpdateSec   = 30
	DefaultProtectorDynStopEarlyHoldMin   = 10
	DefaultProtectorDynStopEarlyPeakROE   = 8.0
	DefaultProtectorDynStopEarlyMovePct   = 0.60
	DefaultProtectorDynStopBaseRiskROE    = 35.0
	DefaultProtectorDynStopATRPercentile  = 50
	DefaultProtectorMinStopDeltaFrac      = 0.0002
	DefaultProtectorHighVolThreshold      = 80.0
	DefaultProtectorLowVolThreshold       = 20.0
	DefaultProtectorHighVolStopWiden      = 1.3
	DefaultProtectorLowVolStopTighten     = 0.7
	DefaultProtectorTimeDecayAfterMin     = 30
	DefaultProtectorTimeDecayRate         = 0.02
	DefaultProtectorTimeDecayMaxFraction  = 0.5
	DefaultProtectorTrailingActivationR   = 0.5
	DefaultProtectorTrailingDistanceR     = 0.4
	DefaultProtectorMicroTP0MinMovePct    = 0.55
	DefaultProtectorMicroTP0StopDistRatio = 0.45
	DefaultProtectorLogMinPnLROEPct       = 3.0
)

// EffectiveProtector returns the config with defaults filled in. A level
// with an unknown unit reads as its default.
func EffectiveProtector(c *ProtectorConfig) ProtectorConfig {
	out := ProtectorConfig{}
	if c != nil {
		out = *c
	}
	f := func(v *float64, d float64) {
		if *v <= 0 {
			*v = d
		}
	}
	lvl := func(l *ProtectorLevel, d ProtectorLevel) {
		if !ValidProtectorUnit(l.Unit) {
			*l = d
		}
	}
	f(&out.FastIntervalSeconds, DefaultProtectorFastIntervalSeconds)
	f(&out.BaseIntervalSeconds, DefaultProtectorBaseIntervalSeconds)
	f(&out.IdleIntervalSeconds, DefaultProtectorIdleIntervalSeconds)
	if out.DefaultLeverage <= 0 {
		out.DefaultLeverage = DefaultProtectorLeverage
	}
	if out.PlanLookbackRecords <= 0 {
		out.PlanLookbackRecords = DefaultProtectorPlanLookbackRecords
	}
	lvl(&out.HardLoss, roe(DefaultProtectorHardLossROEPct))
	f(&out.HardLossSlackMultiplier, DefaultProtectorHardLossSlack)
	lvl(&out.EarlyLoss, roe(DefaultProtectorEarlyLossROEPct))
	f(&out.EarlyLossWindowMinutes, DefaultProtectorEarlyLossWindowMin)
	lvl(&out.ProfitFloorPeak, roe(DefaultProtectorProfitFloorPeakROEPct))
	lvl(&out.ProfitFloorBuffer, roe(DefaultProtectorProfitFloorBufferROE))
	lvl(&out.ProfitFloorNetBuffer, roe(DefaultProtectorProfitFloorNetROE))
	lvl(&out.ProfitLockMidPeak, roe(DefaultProtectorProfitLockMidPeakROE))
	lvl(&out.ProfitLockHighPeak, roe(DefaultProtectorProfitLockHighPeakROE))
	f(&out.ProfitLockMidRatio, DefaultProtectorProfitLockMidRatio)
	f(&out.ProfitLockHighRatio, DefaultProtectorProfitLockHighRatio)
	f(&out.ProfitLockMidDrawdownPct, DefaultProtectorProfitLockMidDDPct)
	f(&out.ProfitLockHighDrawdownPct, DefaultProtectorProfitLockHighDDPct)
	lvl(&out.TP0, roe(DefaultProtectorTP0ROEPct))
	lvl(&out.TP1, roe(DefaultProtectorTP1ROEPct))
	lvl(&out.TP1MinMove, price(DefaultProtectorTP1MinMovePct))
	lvl(&out.TP2, roe(DefaultProtectorTP2ROEPct))
	lvl(&out.TP2MinMove, price(DefaultProtectorTP2MinMovePct))
	f(&out.TrailDrawdownPct, DefaultProtectorTrailDrawdownPct)
	lvl(&out.NearTP1Peak, roe(DefaultProtectorNearTP1PeakROE))
	lvl(&out.NearTP1SecondChance, roe(DefaultProtectorNearTP1SecondChance))
	f(&out.NearTP1GivebackPct, DefaultProtectorNearTP1GivebackPct)
	lvl(&out.NearTP1LossExit, roe(DefaultProtectorNearTP1LossExitROE))
	lvl(&out.PreTPMinCurrent, roe(DefaultProtectorPreTPMinCurrentROE))
	lvl(&out.PreTPPeak, roe(DefaultProtectorPreTPPeakROE))
	f(&out.PreTPGivebackPct, DefaultProtectorPreTPGivebackPct)
	f(&out.PreTPMinHoldMinutes, DefaultProtectorPreTPMinHoldMin)
	f(&out.TP0CloseRatio, DefaultProtectorTP0CloseRatio)
	f(&out.TP1CloseRatio, DefaultProtectorTP1CloseRatio)
	f(&out.TP2CloseRatio, DefaultProtectorTP2CloseRatio)
	f(&out.GivebackCloseRatio, DefaultProtectorGivebackCloseRatio)
	f(&out.BreakevenBufferFrac, DefaultProtectorBreakevenBufferFrac)
	f(&out.GivebackBreakevenBufferFrac, DefaultProtectorGivebackBreakevenFrac)
	f(&out.MinCloseNotional, DefaultProtectorMinCloseNotional)
	f(&out.DynamicStopMinUpdateSeconds, DefaultProtectorDynStopMinUpdateSec)
	f(&out.DynamicStopEarlyHoldMinutes, DefaultProtectorDynStopEarlyHoldMin)
	lvl(&out.DynamicStopEarlyPeak, roe(DefaultProtectorDynStopEarlyPeakROE))
	lvl(&out.DynamicStopEarlyMove, price(DefaultProtectorDynStopEarlyMovePct))
	lvl(&out.DynamicStopBaseRisk, roe(DefaultProtectorDynStopBaseRiskROE))
	f(&out.DynamicStopATRPercentile, DefaultProtectorDynStopATRPercentile)
	f(&out.MinStopDeltaFrac, DefaultProtectorMinStopDeltaFrac)
	f(&out.HighVolThreshold, DefaultProtectorHighVolThreshold)
	f(&out.LowVolThreshold, DefaultProtectorLowVolThreshold)
	f(&out.HighVolStopWiden, DefaultProtectorHighVolStopWiden)
	f(&out.LowVolStopTighten, DefaultProtectorLowVolStopTighten)
	f(&out.TimeDecayAfterMinutes, DefaultProtectorTimeDecayAfterMin)
	f(&out.TimeDecayRate, DefaultProtectorTimeDecayRate)
	f(&out.TimeDecayMaxFraction, DefaultProtectorTimeDecayMaxFraction)
	f(&out.TrailingActivationR, DefaultProtectorTrailingActivationR)
	f(&out.TrailingDistanceR, DefaultProtectorTrailingDistanceR)
	lvl(&out.MicroTP0MinMove, price(DefaultProtectorMicroTP0MinMovePct))
	f(&out.MicroTP0StopDistRatio, DefaultProtectorMicroTP0StopDistRatio)
	lvl(&out.LogMinPnL, roe(DefaultProtectorLogMinPnLROEPct))
	return out
}

func seconds(v float64) time.Duration { return time.Duration(v * float64(time.Second)) }
func minutes(v float64) time.Duration { return time.Duration(v * float64(time.Minute)) }

// Durations.
func (c ProtectorConfig) FastInterval() time.Duration { return seconds(c.FastIntervalSeconds) }
func (c ProtectorConfig) BaseInterval() time.Duration { return seconds(c.BaseIntervalSeconds) }
func (c ProtectorConfig) IdleInterval() time.Duration { return seconds(c.IdleIntervalSeconds) }
func (c ProtectorConfig) EarlyLossWindow() time.Duration {
	return minutes(c.EarlyLossWindowMinutes)
}
func (c ProtectorConfig) PreTPMinHold() time.Duration { return minutes(c.PreTPMinHoldMinutes) }
func (c ProtectorConfig) DynamicStopMinUpdate() time.Duration {
	return seconds(c.DynamicStopMinUpdateSeconds)
}
func (c ProtectorConfig) DynamicStopEarlyHold() time.Duration {
	return minutes(c.DynamicStopEarlyHoldMinutes)
}
func (c ProtectorConfig) TimeDecayAfter() time.Duration { return minutes(c.TimeDecayAfterMinutes) }
