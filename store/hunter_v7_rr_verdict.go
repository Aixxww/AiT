package store

import (
	"time"
)

// HunterV7RRTargetTerm records one candidate target's reward before and after
// the backend TP cap (MaxTPMovePct), serialized into TargetRewardsJSON on
// HunterV7RRVerdictRecord.
type HunterV7RRTargetTerm struct {
	Index           int     `json:"index"`
	Label           string  `json:"label"`
	Price           float64 `json:"price"`
	Reason          string  `json:"reason,omitempty"`
	RawRewardPct    float64 `json:"raw_reward_pct"`
	CappedRewardPct float64 `json:"capped_reward_pct"`
	Capped          bool    `json:"capped"`
}

// HunterV7RRVerdictRecord persists the full computation terms whenever a
// Hunter v7 candidate's tier reason becomes backend_rr_infeasible,
// rr_below_floor, or rr_unknown. Observability only: nothing gates on it.
type HunterV7RRVerdictRecord struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CycleNumber int       `gorm:"column:cycle_number;not null;index:idx_v7rr_cycle" json:"cycle_number"`
	Timestamp   time.Time `gorm:"column:timestamp;not null;index:idx_v7rr_ts,sort:desc" json:"timestamp"`
	Stage       string    `gorm:"column:stage;not null;index:idx_v7rr_stage" json:"stage"` // "assembly" | "prompt_final"
	Symbol      string    `gorm:"column:symbol;not null;index:idx_v7rr_symbol" json:"symbol"`
	Direction   string    `gorm:"column:direction;not null" json:"direction"`
	SetupType   string    `gorm:"column:setup_type;not null;index:idx_v7rr_setup" json:"setup_type"`
	SignalID    string    `gorm:"column:signal_id" json:"signal_id"`
	Stack       string    `gorm:"column:stack" json:"stack"`
	SignalClass string    `gorm:"column:signal_class" json:"signal_class"`

	ReferencePrice       float64 `gorm:"column:reference_price" json:"reference_price"`
	ReferencePriceSource string  `gorm:"column:reference_price_source" json:"reference_price_source"`
	InvalidationPrice    float64 `gorm:"column:invalidation_price" json:"invalidation_price"`

	RawRiskPct          float64 `gorm:"column:raw_risk_pct" json:"raw_risk_pct"`
	EffectiveRiskPct    float64 `gorm:"column:effective_risk_pct" json:"effective_risk_pct"`
	MinSLMovePct        float64 `gorm:"column:min_sl_move_pct" json:"min_sl_move_pct"`
	MinStopFloorApplied bool    `gorm:"column:min_stop_floor_applied" json:"min_stop_floor_applied"`
	StopRepairBufferPct float64 `gorm:"column:stop_repair_buffer_pct" json:"stop_repair_buffer_pct"`

	TargetRewardsJSON      string  `gorm:"column:target_rewards_json;type:text" json:"target_rewards_json"`
	MaxTPMovePct           float64 `gorm:"column:max_tp_move_pct" json:"max_tp_move_pct"`
	BestRewardPct          float64 `gorm:"column:best_reward_pct" json:"best_reward_pct"`
	BestTargetIndex        int     `gorm:"column:best_target_index" json:"best_target_index"`
	BestTargetLabel        string  `gorm:"column:best_target_label" json:"best_target_label"`
	BestTargetPrice        float64 `gorm:"column:best_target_price" json:"best_target_price"`
	BestTargetRawRewardPct float64 `gorm:"column:best_target_raw_reward_pct" json:"best_target_raw_reward_pct"`
	BestTargetCapped       bool    `gorm:"column:best_target_capped" json:"best_target_capped"`

	CappedRR   float64 `gorm:"column:capped_rr" json:"capped_rr"`
	MinRRFloor float64 `gorm:"column:min_rr_floor" json:"min_rr_floor"`
	OK         bool    `gorm:"column:ok" json:"ok"`
	Reason     string  `gorm:"column:reason;not null;index:idx_v7rr_reason" json:"reason"`

	ResolvedMinRR        float64 `gorm:"column:resolved_min_rr" json:"resolved_min_rr"`
	ResolvedMinSLMovePct float64 `gorm:"column:resolved_min_sl_move_pct" json:"resolved_min_sl_move_pct"`
	ResolvedMaxTPMovePct float64 `gorm:"column:resolved_max_tp_move_pct" json:"resolved_max_tp_move_pct"`
	MinRRSource          string  `gorm:"column:min_rr_source" json:"min_rr_source"`
	MinSLSource          string  `gorm:"column:min_sl_source" json:"min_sl_source"`
	MaxTPSource          string  `gorm:"column:max_tp_source" json:"max_tp_source"`

	CreatedAt time.Time `json:"created_at"`
}

// TableName implements GORM table naming.
func (HunterV7RRVerdictRecord) TableName() string { return "hunter_v7_rr_verdicts" }

// CreateRRVerdict persists a single infeasible-RR verdict diagnostic row.
func (s *HunterV7SignalStore) CreateRRVerdict(rec *HunterV7RRVerdictRecord) error {
	if rec == nil {
		return nil
	}
	return s.db.Create(rec).Error
}

// CreateRRVerdictBatch inserts multiple RR verdict diagnostic rows in one batch.
func (s *HunterV7SignalStore) CreateRRVerdictBatch(records []HunterV7RRVerdictRecord) error {
	if len(records) == 0 {
		return nil
	}
	return s.db.CreateInBatches(records, 200).Error
}

// QueryRRVerdicts returns the most recent RR verdict diagnostic rows.
func (s *HunterV7SignalStore) QueryRRVerdicts(limit int) ([]HunterV7RRVerdictRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var records []HunterV7RRVerdictRecord
	err := s.db.Order("timestamp DESC, id DESC").Limit(limit).Find(&records).Error
	return records, err
}

// QueryRRVerdictsByCycle returns all RR verdict diagnostic rows for one cycle.
func (s *HunterV7SignalStore) QueryRRVerdictsByCycle(cycleNumber int) ([]HunterV7RRVerdictRecord, error) {
	var records []HunterV7RRVerdictRecord
	err := s.db.Where("cycle_number = ?", cycleNumber).Order("id ASC").Find(&records).Error
	return records, err
}
