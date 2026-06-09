package store

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HunterV7MoverStore persists daily large-amplitude mover labels,
// enabling recall-rate analysis against Hunter v7 signal records.
type HunterV7MoverStore struct {
	db *gorm.DB
}

// HunterV7MoverLabel is the GORM model for hunter_v7_mover_labels.
type HunterV7MoverLabel struct {
	ID                int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	TradeDate         string     `gorm:"column:trade_date;not null;index:idx_v7mover_date;uniqueIndex:idx_v7mover_date_symbol" json:"trade_date"`
	Symbol            string     `gorm:"column:symbol;not null;index:idx_v7mover_symbol;uniqueIndex:idx_v7mover_date_symbol" json:"symbol"`
	High24h           float64    `gorm:"column:high_24h" json:"high_24h"`
	Low24h            float64    `gorm:"column:low_24h" json:"low_24h"`
	Amplitude24h      float64    `gorm:"column:amplitude_24h;index:idx_v7mover_amp" json:"amplitude_24h"`
	MaxUp24h          float64    `gorm:"column:max_up_24h" json:"max_up_24h"`
	MaxDown24h        float64    `gorm:"column:max_down_24h" json:"max_down_24h"`
	FirstSeenAt       *time.Time `gorm:"column:first_seen_at" json:"first_seen_at"`
	FirstWatchAt      *time.Time `gorm:"column:first_watch_at" json:"first_watch_at"`
	FirstReviewableAt *time.Time `gorm:"column:first_reviewable_at" json:"first_reviewable_at"`
	FirstExecutableAt *time.Time `gorm:"column:first_executable_at" json:"first_executable_at"`
	FirstOpenAt       *time.Time `gorm:"column:first_open_at" json:"first_open_at"`
	MoveStartAt       *time.Time `gorm:"column:move_start_at" json:"move_start_at"`
	MissedStage       string     `gorm:"column:missed_stage" json:"missed_stage"`
	BestTier          string     `gorm:"column:best_tier" json:"best_tier"`
	BestTierReason    string     `gorm:"column:best_tier_reason" json:"best_tier_reason"`
	BlockedGate       string     `gorm:"column:blocked_gate" json:"blocked_gate"`
	LeadTimeMinutes   int        `gorm:"column:lead_time_minutes" json:"lead_time_minutes"`
	Notes             string     `gorm:"column:notes;type:text" json:"notes"`
	CreatedAt         time.Time  `json:"created_at"`
}

// TableName implements GORM table naming.
func (HunterV7MoverLabel) TableName() string { return "hunter_v7_mover_labels" }

// NewHunterV7MoverStore creates a new HunterV7MoverStore.
func NewHunterV7MoverStore(db *gorm.DB) *HunterV7MoverStore {
	return &HunterV7MoverStore{db: db}
}

// initTables creates the hunter_v7_mover_labels table if it doesn't exist.
func (s *HunterV7MoverStore) initTables() error {
	return s.db.AutoMigrate(&HunterV7MoverLabel{})
}

// CreateBatch upserts multiple mover labels in a single transaction.
func (s *HunterV7MoverStore) CreateBatch(labels []HunterV7MoverLabel) error {
	return s.UpsertBatch(labels)
}

// UpsertBatch inserts or updates daily mover labels by trade_date+symbol.
func (s *HunterV7MoverStore) UpsertBatch(labels []HunterV7MoverLabel) error {
	if len(labels) == 0 {
		return nil
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "trade_date"}, {Name: "symbol"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"high_24h",
			"low_24h",
			"amplitude_24h",
			"max_up_24h",
			"max_down_24h",
			"first_seen_at",
			"first_watch_at",
			"first_reviewable_at",
			"first_executable_at",
			"first_open_at",
			"move_start_at",
			"missed_stage",
			"best_tier",
			"best_tier_reason",
			"blocked_gate",
			"lead_time_minutes",
			"notes",
		}),
	}).CreateInBatches(labels, 200).Error
}

// QueryByDate returns all mover labels for a given trade date.
func (s *HunterV7MoverStore) QueryByDate(tradeDate string) ([]HunterV7MoverLabel, error) {
	var labels []HunterV7MoverLabel
	err := s.db.Where("trade_date = ?", tradeDate).
		Order("amplitude_24h DESC").
		Find(&labels).Error
	return labels, err
}

// QueryByAmplitude returns mover labels with amplitude >= threshold within a date range.
func (s *HunterV7MoverStore) QueryByAmplitude(from, to time.Time, minAmplitude float64) ([]HunterV7MoverLabel, error) {
	var labels []HunterV7MoverLabel
	err := s.db.Where("created_at BETWEEN ? AND ? AND amplitude_24h >= ?", from, to, minAmplitude).
		Order("amplitude_24h DESC").
		Find(&labels).Error
	return labels, err
}

// RecallStats returns summary recall statistics for a date range.
type RecallStats struct {
	TotalMovers     int     `json:"total_movers"`
	SeenCount       int     `json:"seen_count"`
	WatchCount      int     `json:"watch_count"`
	ReviewableCount int     `json:"reviewable_count"`
	ExecutableCount int     `json:"executable_count"`
	RecallRate      float64 `json:"recall_rate"`
}

// ComputeRecallStats computes recall stats for movers above the given amplitude.
func (s *HunterV7MoverStore) ComputeRecallStats(from, to time.Time, minAmplitude float64) (*RecallStats, error) {
	labels, err := s.QueryByAmplitude(from, to, minAmplitude)
	if err != nil {
		return nil, err
	}
	stats := &RecallStats{TotalMovers: len(labels)}
	for _, l := range labels {
		if l.FirstSeenAt != nil {
			stats.SeenCount++
		}
		if l.FirstWatchAt != nil {
			stats.WatchCount++
		}
		if l.FirstReviewableAt != nil {
			stats.ReviewableCount++
		}
		if l.FirstExecutableAt != nil {
			stats.ExecutableCount++
		}
	}
	if stats.TotalMovers > 0 {
		stats.RecallRate = float64(stats.SeenCount) / float64(stats.TotalMovers) * 100
	}
	return stats, nil
}
