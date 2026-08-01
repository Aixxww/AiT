package store

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// HunterV7SignalStore persists every Hunter v7 signal output per cycle,
// enabling full funnel attribution and win-rate analysis.
type HunterV7SignalStore struct {
	db *gorm.DB
}

// HunterV7SignalRecord is the GORM model for hunter_v7_signal_records.
type HunterV7SignalRecord struct {
	ID                int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	CycleNumber       int        `gorm:"column:cycle_number;not null;index:idx_v7sig_cycle" json:"cycle_number"`
	Timestamp         time.Time  `gorm:"not null;index:idx_v7sig_ts,sort:desc" json:"timestamp"`
	Symbol            string     `gorm:"column:symbol;not null;index:idx_v7sig_symbol" json:"symbol"`
	Direction         string     `gorm:"column:direction;not null" json:"direction"`
	SetupType         string     `gorm:"column:setup_type;not null;index:idx_v7sig_setup" json:"setup_type"`
	Status            string     `gorm:"column:status;not null" json:"status"`
	ExecutionQuality  string     `gorm:"column:execution_quality" json:"execution_quality"`
	ExecutionTier     string     `gorm:"column:execution_tier" json:"execution_tier"`
	TierReason        string     `gorm:"column:tier_reason" json:"tier_reason"`
	AIPriority        float64    `gorm:"column:ai_priority" json:"ai_priority"`
	SetupScore        float64    `gorm:"column:setup_score" json:"setup_score"`
	TimingScore       float64    `gorm:"column:timing_score" json:"timing_score"`
	RiskScore         float64    `gorm:"column:risk_score" json:"risk_score"`
	LiquidityScore    float64    `gorm:"column:liquidity_score" json:"liquidity_score"`
	RegimeFitScore    float64    `gorm:"column:regime_fit_score" json:"regime_fit_score"`
	MarketRegime      string     `gorm:"column:market_regime" json:"market_regime"`
	ReasonCodes       string     `gorm:"column:reason_codes" json:"reason_codes"`
	RiskTags          string     `gorm:"column:risk_tags" json:"risk_tags"`
	EntryZoneLower    float64    `gorm:"column:entry_zone_lower" json:"entry_zone_lower"`
	EntryZoneUpper    float64    `gorm:"column:entry_zone_upper" json:"entry_zone_upper"`
	InvalidationPrice float64    `gorm:"column:invalidation_price" json:"invalidation_price"`
	Target1           float64    `gorm:"column:target_1" json:"target_1"`
	OIValue           float64    `gorm:"column:oi_value" json:"oi_value"`
	OIDelta1h         float64    `gorm:"column:oi_delta_1h" json:"oi_delta_1h"`
	OIDelta4h         float64    `gorm:"column:oi_delta_4h" json:"oi_delta_4h"`
	FundingRate       float64    `gorm:"column:funding_rate" json:"funding_rate"`
	TakerBuy15m       float64    `gorm:"column:taker_buy_15m" json:"taker_buy_15m"`
	Change1h          float64    `gorm:"column:change_1h" json:"change_1h"`
	Change4h          float64    `gorm:"column:change_4h" json:"change_4h"`
	Change24h         float64    `gorm:"column:change_24h" json:"change_24h"`
	ReadyScore        float64    `gorm:"column:ready_score" json:"ready_score"`
	WindowHealth      float64    `gorm:"column:window_health" json:"window_health"`
	DataQuality       string     `gorm:"column:data_quality" json:"data_quality"`
	TP0Price          float64    `gorm:"column:tp0_price" json:"tp0_price"`
	TP0RR             float64    `gorm:"column:tp0_rr" json:"tp0_rr"`
	TP1Price          float64    `gorm:"column:tp1_price" json:"tp1_price"`
	TP1RR             float64    `gorm:"column:tp1_rr" json:"tp1_rr"`
	TP2Price          float64    `gorm:"column:tp2_price" json:"tp2_price"`
	TP2RR             float64    `gorm:"column:tp2_rr" json:"tp2_rr"`
	ResonanceBonus    float64    `gorm:"column:resonance_bonus" json:"resonance_bonus"`
	BlockedGate       string     `gorm:"column:blocked_gate" json:"blocked_gate"`
	TrackStatus       string     `gorm:"column:track_status;index:idx_v7sig_track_status" json:"track_status"`
	TrackCurrentPrice float64    `gorm:"column:track_current_price" json:"track_current_price"`
	TrackExitPrice    float64    `gorm:"column:track_exit_price" json:"track_exit_price"`
	TrackStopPrice    float64    `gorm:"column:track_stop_price" json:"track_stop_price"`
	TrackPnLPct       float64    `gorm:"column:track_pnl_pct" json:"track_pnl_pct"`
	TrackMFE          float64    `gorm:"column:track_mfe" json:"track_mfe"`
	TrackMAE          float64    `gorm:"column:track_mae" json:"track_mae"`
	TrackExitTime     *time.Time `gorm:"column:track_exit_time" json:"track_exit_time,omitempty"`
	TrackSnapshots    string     `gorm:"column:track_snapshots;type:text" json:"track_snapshots"`
	RawJSON           string     `gorm:"column:raw_json;type:text" json:"raw_json"`
	CreatedAt         time.Time  `json:"created_at"`
}

// TableName implements GORM table naming.
func (HunterV7SignalRecord) TableName() string { return "hunter_v7_signal_records" }

// NewHunterV7SignalStore creates a new HunterV7SignalStore.
func NewHunterV7SignalStore(db *gorm.DB) *HunterV7SignalStore {
	return &HunterV7SignalStore{db: db}
}

// initTables creates the hunter_v7_signal_records table if it doesn't exist.
func (s *HunterV7SignalStore) initTables() error {
	return s.db.AutoMigrate(&HunterV7SignalRecord{})
}

// CreateBatch inserts multiple signal records in a single transaction.
func (s *HunterV7SignalStore) CreateBatch(records []HunterV7SignalRecord) error {
	if len(records) == 0 {
		return nil
	}
	return s.db.CreateInBatches(records, 200).Error
}

// HunterV7SignalTrackUpdate carries the latest tracking outcome for a signal.
type HunterV7SignalTrackUpdate struct {
	Status       string
	CurrentPrice float64
	ExitPrice    float64
	StopPrice    float64
	PnLPct       float64
	MFE          float64
	MAE          float64
	ExitTime     *time.Time
	Snapshots    string
}

// HunterV7OutcomeWindowStats summarizes terminal tracking outcomes in a time window.
type HunterV7OutcomeWindowStats struct {
	WindowName     string  `json:"window_name"`
	WindowDuration string  `json:"window_duration"`
	Total          int     `json:"total"`
	Wins           int     `json:"wins"`
	Stops          int     `json:"stops"`
	Timeouts       int     `json:"timeouts"`
	WinRate        float64 `json:"win_rate"`
	AvgPnL         float64 `json:"avg_pnl"`
	AvgMFE         float64 `json:"avg_mfe"`
	AvgMAE         float64 `json:"avg_mae"`
}

// HunterV7SetupRegimeOutcomeStats summarizes outcomes per setup×regime cell.
type HunterV7SetupRegimeOutcomeStats struct {
	SetupType    string  `json:"setup_type"`
	MarketRegime string  `json:"market_regime"`
	Total        int     `json:"total"`
	Wins         int     `json:"wins"`
	Stops        int     `json:"stops"`
	Timeouts     int     `json:"timeouts"`
	WinRate      float64 `json:"win_rate"`
	AvgPnL       float64 `json:"avg_pnl"`
	AvgMFE       float64 `json:"avg_mfe"`
	AvgMAE       float64 `json:"avg_mae"`
}

// UpdateTrackOutcome updates PnL tracking fields for one persisted signal record.
func (s *HunterV7SignalStore) UpdateTrackOutcome(recordID int64, update HunterV7SignalTrackUpdate) error {
	if recordID <= 0 {
		return nil
	}
	values := map[string]interface{}{
		"track_status":        update.Status,
		"track_current_price": update.CurrentPrice,
		"track_exit_price":    update.ExitPrice,
		"track_stop_price":    update.StopPrice,
		"track_pnl_pct":       update.PnLPct,
		"track_mfe":           update.MFE,
		"track_mae":           update.MAE,
		"track_exit_time":     update.ExitTime,
	}
	if update.Snapshots != "" {
		values["track_snapshots"] = update.Snapshots
	}
	return s.db.Model(&HunterV7SignalRecord{}).Where("id = ?", recordID).Updates(values).Error
}

// OutcomeWindowStats returns terminal tracking stats for records whose exit
// happened within maxDuration after their signal timestamp. Use this for P1
// metrics such as 30m TP0 and 2h TP1 win rates.
func (s *HunterV7SignalStore) OutcomeWindowStats(from, to time.Time, maxDuration time.Duration, winStatuses []string, windowName string) (HunterV7OutcomeWindowStats, error) {
	stats := HunterV7OutcomeWindowStats{
		WindowName:     windowName,
		WindowDuration: maxDuration.String(),
	}
	if len(winStatuses) == 0 {
		winStatuses = []string{"WIN_TP0", "WIN_TP1", "WIN_TP2"}
	}
	var records []HunterV7SignalRecord
	terminalStatuses := []string{"WIN_TP0", "WIN_TP1", "WIN_TP2", "STOP", "TIMEOUT"}
	err := s.db.Where("timestamp BETWEEN ? AND ? AND track_status IN ?", from, to, terminalStatuses).
		Find(&records).Error
	if err != nil {
		return stats, err
	}
	winSet := make(map[string]struct{}, len(winStatuses))
	for _, status := range winStatuses {
		winSet[status] = struct{}{}
	}
	for _, rec := range records {
		if rec.TrackExitTime == nil {
			continue
		}
		if maxDuration > 0 && rec.TrackExitTime.Sub(rec.Timestamp) > maxDuration {
			continue
		}
		stats.Total++
		if _, ok := winSet[rec.TrackStatus]; ok {
			stats.Wins++
		}
		switch rec.TrackStatus {
		case "STOP":
			stats.Stops++
		case "TIMEOUT":
			stats.Timeouts++
		}
		stats.AvgPnL += rec.TrackPnLPct
		stats.AvgMFE += rec.TrackMFE
		stats.AvgMAE += rec.TrackMAE
	}
	if stats.Total > 0 {
		stats.WinRate = float64(stats.Wins) / float64(stats.Total) * 100
		stats.AvgPnL /= float64(stats.Total)
		stats.AvgMFE /= float64(stats.Total)
		stats.AvgMAE /= float64(stats.Total)
	}
	return stats, nil
}

// SetupRegimeOutcomeStats returns terminal tracking stats grouped by setup type
// and market regime. These aggregates are the DB-backed input for adaptive
// regime weighting.
func (s *HunterV7SignalStore) SetupRegimeOutcomeStats(from, to time.Time, minSamples int) ([]HunterV7SetupRegimeOutcomeStats, error) {
	var records []HunterV7SignalRecord
	terminalStatuses := []string{"WIN_TP0", "WIN_TP1", "WIN_TP2", "STOP", "TIMEOUT"}
	err := s.db.Where("timestamp BETWEEN ? AND ? AND track_status IN ?", from, to, terminalStatuses).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	type bucket struct {
		HunterV7SetupRegimeOutcomeStats
	}
	buckets := make(map[string]*bucket)
	for _, rec := range records {
		key := rec.SetupType + "\x00" + rec.MarketRegime
		b := buckets[key]
		if b == nil {
			b = &bucket{HunterV7SetupRegimeOutcomeStats: HunterV7SetupRegimeOutcomeStats{
				SetupType:    rec.SetupType,
				MarketRegime: rec.MarketRegime,
			}}
			buckets[key] = b
		}
		b.Total++
		switch rec.TrackStatus {
		case "WIN_TP0", "WIN_TP1", "WIN_TP2":
			b.Wins++
		case "STOP":
			b.Stops++
		case "TIMEOUT":
			b.Timeouts++
		}
		b.AvgPnL += rec.TrackPnLPct
		b.AvgMFE += rec.TrackMFE
		b.AvgMAE += rec.TrackMAE
	}
	out := make([]HunterV7SetupRegimeOutcomeStats, 0, len(buckets))
	for _, b := range buckets {
		if minSamples > 0 && b.Total < minSamples {
			continue
		}
		if b.Total > 0 {
			b.WinRate = float64(b.Wins) / float64(b.Total) * 100
			b.AvgPnL /= float64(b.Total)
			b.AvgMFE /= float64(b.Total)
			b.AvgMAE /= float64(b.Total)
		}
		out = append(out, b.HunterV7SetupRegimeOutcomeStats)
	}
	return out, nil
}

// QueryBySymbol returns signal records for a given symbol within a time range.
func (s *HunterV7SignalStore) QueryBySymbol(symbol string, from, to time.Time) ([]HunterV7SignalRecord, error) {
	var records []HunterV7SignalRecord
	err := s.db.Where("symbol = ? AND timestamp BETWEEN ? AND ?", symbol, from, to).
		Order("timestamp ASC").
		Find(&records).Error
	return records, err
}

// QueryByCycle returns all signal records for a given cycle number.
func (s *HunterV7SignalStore) QueryByCycle(cycleNumber int) ([]HunterV7SignalRecord, error) {
	var records []HunterV7SignalRecord
	err := s.db.Where("cycle_number = ?", cycleNumber).
		Order("ai_priority DESC").
		Find(&records).Error
	return records, err
}

// RecentSignals returns the newest signal records with actionable rows first
// inside each timestamp. Validator cycles persist raw rows, including
// module_no_match diagnostics; priority ordering keeps small dashboard limits
// from being filled only by rejected tail rows.
func (s *HunterV7SignalStore) RecentSignals(limit int) ([]HunterV7SignalRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var records []HunterV7SignalRecord
	err := s.db.
		Order("timestamp DESC").
		Order("CASE execution_tier WHEN 'EXECUTABLE' THEN 1 WHEN 'REVIEWABLE' THEN 2 WHEN 'WATCH' THEN 3 WHEN 'REJECTED' THEN 4 ELSE 5 END").
		Order("ai_priority DESC").
		Order("id DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

// QueryByDateRange returns all signal records within a date range.
func (s *HunterV7SignalStore) QueryByDateRange(from, to time.Time) ([]HunterV7SignalRecord, error) {
	var records []HunterV7SignalRecord
	err := s.db.Where("timestamp BETWEEN ? AND ?", from, to).
		Order("timestamp ASC").
		Find(&records).Error
	return records, err
}

// QueryMatrixSource returns persisted signal records for regime×setup matrix
// diagnostics. It includes both confirmed and rejected records because the
// matrix is intended to expose funnel shape, not only final outcomes.
func (s *HunterV7SignalStore) QueryMatrixSource(from, to time.Time, regime string) ([]HunterV7SignalRecord, error) {
	var records []HunterV7SignalRecord
	q := s.db.Where("timestamp BETWEEN ? AND ?", from, to)
	if regime != "" {
		q = q.Where("market_regime = ?", regime)
	}
	err := q.Order("market_regime ASC, setup_type ASC, ai_priority DESC").
		Find(&records).Error
	return records, err
}

// FirstSeenAt returns the earliest timestamp a symbol appeared in signals.
func (s *HunterV7SignalStore) FirstSeenAt(symbol string, from, to time.Time) *time.Time {
	var ts time.Time
	err := s.db.Model(&HunterV7SignalRecord{}).
		Where("symbol = ? AND timestamp BETWEEN ? AND ?", symbol, from, to).
		Order("timestamp ASC").
		Select("timestamp").
		Limit(1).
		Scan(&ts).Error
	if err != nil || ts.IsZero() {
		return nil
	}
	return &ts
}

// BestTierForSymbol returns the best execution_tier a symbol reached in the given range.
// Tier priority: EXECUTABLE > REVIEWABLE > WATCH > REJECTED > ""
func (s *HunterV7SignalStore) BestTierForSymbol(symbol string, from, to time.Time) (string, string) {
	var records []HunterV7SignalRecord
	s.db.Where("symbol = ? AND timestamp BETWEEN ? AND ?", symbol, from, to).
		Order("timestamp ASC").
		Find(&records)

	bestTier := ""
	bestReason := ""
	for _, r := range records {
		if tierPriority(r.ExecutionTier) > tierPriority(bestTier) {
			bestTier = r.ExecutionTier
			bestReason = r.TierReason
		}
	}
	return bestTier, bestReason
}

func tierPriority(tier string) int {
	switch tier {
	case "EXECUTABLE":
		return 4
	case "REVIEWABLE":
		return 3
	case "WATCH":
		return 2
	case "REJECTED":
		return 1
	default:
		return 0
	}
}

// BlockedGateForSymbol returns the most common blocked_gate for a symbol.
func (s *HunterV7SignalStore) BlockedGateForSymbol(symbol string, from, to time.Time) string {
	type gateCount struct {
		BlockedGate string
		Cnt         int
	}
	var result gateCount
	s.db.Model(&HunterV7SignalRecord{}).
		Select("blocked_gate, COUNT(*) as cnt").
		Where("symbol = ? AND timestamp BETWEEN ? AND ? AND blocked_gate != ''", symbol, from, to).
		Group("blocked_gate").
		Order("cnt DESC").
		Limit(1).
		Scan(&result)
	return result.BlockedGate
}

// ToJSON is a helper to serialize reason codes / risk tags slices.
func ToJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
