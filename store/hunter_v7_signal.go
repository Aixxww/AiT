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
	ID                int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CycleNumber       int       `gorm:"column:cycle_number;not null;index:idx_v7sig_cycle" json:"cycle_number"`
	Timestamp         time.Time `gorm:"not null;index:idx_v7sig_ts,sort:desc" json:"timestamp"`
	Symbol            string    `gorm:"column:symbol;not null;index:idx_v7sig_symbol" json:"symbol"`
	Direction         string    `gorm:"column:direction;not null" json:"direction"`
	SetupType         string    `gorm:"column:setup_type;not null;index:idx_v7sig_setup" json:"setup_type"`
	Status            string    `gorm:"column:status;not null" json:"status"`
	ExecutionQuality  string    `gorm:"column:execution_quality" json:"execution_quality"`
	ExecutionTier     string    `gorm:"column:execution_tier" json:"execution_tier"`
	TierReason        string    `gorm:"column:tier_reason" json:"tier_reason"`
	AIPriority        float64   `gorm:"column:ai_priority" json:"ai_priority"`
	SetupScore        float64   `gorm:"column:setup_score" json:"setup_score"`
	TimingScore       float64   `gorm:"column:timing_score" json:"timing_score"`
	RiskScore         float64   `gorm:"column:risk_score" json:"risk_score"`
	LiquidityScore    float64   `gorm:"column:liquidity_score" json:"liquidity_score"`
	RegimeFitScore    float64   `gorm:"column:regime_fit_score" json:"regime_fit_score"`
	MarketRegime      string    `gorm:"column:market_regime" json:"market_regime"`
	ReasonCodes       string    `gorm:"column:reason_codes" json:"reason_codes"`
	RiskTags          string    `gorm:"column:risk_tags" json:"risk_tags"`
	EntryZoneLower    float64   `gorm:"column:entry_zone_lower" json:"entry_zone_lower"`
	EntryZoneUpper    float64   `gorm:"column:entry_zone_upper" json:"entry_zone_upper"`
	InvalidationPrice float64   `gorm:"column:invalidation_price" json:"invalidation_price"`
	Target1           float64   `gorm:"column:target_1" json:"target_1"`
	OIValue           float64   `gorm:"column:oi_value" json:"oi_value"`
	OIDelta1h         float64   `gorm:"column:oi_delta_1h" json:"oi_delta_1h"`
	OIDelta4h         float64   `gorm:"column:oi_delta_4h" json:"oi_delta_4h"`
	FundingRate       float64   `gorm:"column:funding_rate" json:"funding_rate"`
	TakerBuy15m       float64   `gorm:"column:taker_buy_15m" json:"taker_buy_15m"`
	Change1h          float64   `gorm:"column:change_1h" json:"change_1h"`
	Change4h          float64   `gorm:"column:change_4h" json:"change_4h"`
	Change24h         float64   `gorm:"column:change_24h" json:"change_24h"`
	BlockedGate       string    `gorm:"column:blocked_gate" json:"blocked_gate"`
	RawJSON           string    `gorm:"column:raw_json;type:text" json:"raw_json"`
	CreatedAt         time.Time `json:"created_at"`
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

// QueryByDateRange returns all signal records within a date range.
func (s *HunterV7SignalStore) QueryByDateRange(from, to time.Time) ([]HunterV7SignalRecord, error) {
	var records []HunterV7SignalRecord
	err := s.db.Where("timestamp BETWEEN ? AND ?", from, to).
		Order("timestamp ASC").
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
