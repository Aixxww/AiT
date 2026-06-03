package engine

import (
	"nofx/datafetch"
	"time"
)

// Grade represents the trade signal quality classification.
type Grade int

const (
	GradeC Grade = iota // Score < 50 — do not trade
	GradeB              // Score >= 50 — AI confirmation needed
	GradeA              // Score >= 65 — execute
	GradeS              // Score >= 80 — immediate execute
)

func (g Grade) String() string {
	switch g {
	case GradeC:
		return "C"
	case GradeB:
		return "B"
	case GradeA:
		return "A"
	case GradeS:
		return "S"
	default:
		return "?"
	}
}

// IndicatorSet holds computed indicators for a single symbol.
type IndicatorSet struct {
	Symbol string

	// Technical indicators (from Klines)
	RSI14      float64
	MACDLine   float64
	MACDSignal float64
	MACDHist   float64
	BBUpper    float64
	BBMiddle   float64
	BBLower    float64
	BBWidth    float64
	EMA20      float64
	EMA50      float64
	EMA200     float64
	ATR14      float64

	// Quant indicators (from Snapshot)
	OIScore      float64 // -100 to 100 (positive = bullish OI pattern)
	OISpikeScore float64 // 0-100 (OI spike magnitude)
	FundingScore float64 // -100 to 100 (negative FR = bullish)
	LSRScore     float64 // -100 to 100 (low LSR rising = bullish)
	TakerScore   float64 // -100 to 100 (high buy ratio = bullish)
	VolumeScore  float64 // 0-100 (volume anomaly magnitude)

	// Social indicators (from LunarCrush)
	SocialHeatScore  float64 // 0-100
	SocialSentiment  float64 // -100 to 100
	SocialVolumePct  float64 // 0-100

	// Computed sub-scores
	TechBullScore   float64 // 0-40 (bullish technical signals)
	TechBearScore   float64 // 0-40 (bearish technical signals)
	QuantBullScore  float64 // 0-40 (bullish quant signals)
	QuantBearScore  float64 // 0-40 (bearish quant signals)
	SocialBullScore float64 // 0-20 (bullish social signals)
	SocialBearScore float64 // 0-20 (bearish social signals)

	// Final computed values
	Direction  int     // +1 LONG, -1 SHORT, 0 NEUTRAL
	FinalScore float64 // 0-100
}

// TradeSignal is the final output of the scoring engine.
type TradeSignal struct {
	Symbol    string
	Direction int     // +1 LONG, -1 SHORT, 0 NEUTRAL
	FinalScore float64 // 0-100
	Grade     Grade
	TechScore float64
	QuantScore float64
	SocialScore float64

	EntryPrice float64
	StopLoss   float64
	TP1        float64
	TP2        float64
	TP3        float64

	BullSignals []string
	BearSignals []string
	Reasons     []string

	Indicators *IndicatorSet
	Snapshot   *datafetch.SymbolSnapshot
	Timestamp  time.Time
}

// HubConfig holds configurable weights and thresholds.
type HubConfig struct {
	TechWeight   float64 // default 40
	QuantWeight  float64 // default 40
	SocialWeight float64 // default 20

	// Individual indicator enable/disable
	RSIEnabled  bool // default true
	MACDEnabled bool // default true
	BBEnabled   bool // default true
	EMAEnabled  bool // default true
	ATREnabled  bool // default true

	// Quant indicator enable/disable
	OIScoreEnabled  bool // default true
	OISpikeEnabled  bool // default true
	FundingEnabled  bool // default true
	LSREnabled      bool // default true
	TakerEnabled    bool // default true
	VolumeEnabled   bool // default true

	// Grade thresholds
	GradeSThreshold float64 // default 80
	GradeAThreshold float64 // default 65
	GradeBThreshold float64 // default 50

	// Direction margin (bull-bear difference needed for direction)
	DirectionMargin float64 // default 15

	// SL/TP multipliers (in ATR units)
	StopLossATR float64 // default 2.0
	TP1ATR      float64 // default 1.5
	TP2ATR      float64 // default 3.0
	TP3ATR      float64 // default 5.0

	// Engine settings
	MaxSignalsPerCycle int     // default 5
	MinScore           float64 // default 50
	CooldownMinutes    int     // default 60
	TopNForScoring     int     // default 100
}

// DefaultHubConfig returns a HubConfig with sensible defaults.
func DefaultHubConfig() HubConfig {
	return HubConfig{
		TechWeight:   40,
		QuantWeight:  40,
		SocialWeight: 20,

		RSIEnabled:  true,
		MACDEnabled: true,
		BBEnabled:   true,
		EMAEnabled:  true,
		ATREnabled:  true,

		OIScoreEnabled: true,
		OISpikeEnabled: true,
		FundingEnabled: true,
		LSREnabled:     true,
		TakerEnabled:   true,
		VolumeEnabled:  true,

		GradeSThreshold: 80,
		GradeAThreshold: 65,
		GradeBThreshold: 50,

		DirectionMargin: 15,

		StopLossATR: 2.0,
		TP1ATR:      1.5,
		TP2ATR:      3.0,
		TP3ATR:      5.0,

		MaxSignalsPerCycle: 5,
		MinScore:           50,
		CooldownMinutes:    60,
		TopNForScoring:     100,
	}
}
