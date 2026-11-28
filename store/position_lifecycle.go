package store

import (
	"fmt"
	"strings"
	"time"
)

// Position lifecycle record (#92 / audit cluster 2: E3, C4, C6).
//
// One row per position open attempt, keyed by trader, symbol and side. It is
// written BEFORE the order ("intent", carrying the plan) and updated after it
// from the exchange's fill read-back ("open"), so a crash between the fill and
// the decision-record save no longer loses the plan. Conditional entries are
// persisted in the same table ("pending_entry") so a restart re-arms or
// visibly cancels them. Cluster 4 (protection-state persistence) extends this
// record; it must not add a second one.
const (
	LifecycleStatusIntent         = "intent"          // plan written, order not confirmed
	LifecycleStatusOpen           = "open"            // fill confirmed (or adopted at reconcile)
	LifecycleStatusFailed         = "failed"          // order rejected / never filled
	LifecycleStatusClosed         = "closed"          // closed by the LLM or the protector
	LifecycleStatusPendingEntry   = "pending_entry"   // conditional entry armed, not fired
	LifecycleStatusEntryFired     = "entry_fired"     // conditional entry fired (its open has its own row)
	LifecycleStatusEntryCancelled = "entry_cancelled" // conditional entry cancelled (reason in Note)
)

// PositionLifecycle is the persisted per-position lifecycle record.
type PositionLifecycle struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	TraderID string `gorm:"column:trader_id;not null;index:idx_lifecycle_trader_status" json:"trader_id"`
	Symbol   string `gorm:"column:symbol;not null;index:idx_lifecycle_symbol_side" json:"symbol"`
	Side     string `gorm:"column:side;not null;index:idx_lifecycle_symbol_side" json:"side"` // LONG / SHORT
	Status   string `gorm:"column:status;not null;index:idx_lifecycle_trader_status" json:"status"`

	SignalID  string `gorm:"column:signal_id" json:"signal_id,omitempty"`
	IntentKey string `gorm:"column:intent_key" json:"intent_key,omitempty"`

	// Plan, written before the order.
	PlanEntryPrice float64 `gorm:"column:plan_entry_price" json:"plan_entry_price"`
	PlanStopLoss   float64 `gorm:"column:plan_stop_loss" json:"plan_stop_loss"`
	PlanTakeProfit float64 `gorm:"column:plan_take_profit" json:"plan_take_profit"`
	PlanQuantity   float64 `gorm:"column:plan_quantity" json:"plan_quantity"`
	PlanSizeUSD    float64 `gorm:"column:plan_size_usd" json:"plan_size_usd"`
	Leverage       int     `gorm:"column:leverage" json:"leverage"`

	// Fill, written after the order from the exchange read-back.
	OrderID       int64   `gorm:"column:order_id" json:"order_id,omitempty"`
	ExecutedQty   float64 `gorm:"column:executed_qty" json:"executed_qty"`
	AvgFillPrice  float64 `gorm:"column:avg_fill_price" json:"avg_fill_price"`
	FillConfirmed bool    `gorm:"column:fill_confirmed" json:"fill_confirmed"`
	StopLoss      float64 `gorm:"column:stop_loss" json:"stop_loss"`     // placed, fill-anchored
	TakeProfit    float64 `gorm:"column:take_profit" json:"take_profit"` // placed, fill-anchored

	// Protection state (#93 / AUD-D4): the protector's per-position flags,
	// peak PnL and first-seen time, so a restart neither re-fires partials
	// on the remainder nor loses the peak and the age clocks.
	TP0Done         bool    `gorm:"column:tp0_done" json:"tp0_done"`
	TP1Done         bool    `gorm:"column:tp1_done" json:"tp1_done"`
	TP2Done         bool    `gorm:"column:tp2_done" json:"tp2_done"`
	GivebackDone    bool    `gorm:"column:giveback_done" json:"giveback_done"`
	TrailDone       bool    `gorm:"column:trail_done" json:"trail_done"`
	PeakPnLPct      float64 `gorm:"column:peak_pnl_pct" json:"peak_pnl_pct"`
	FirstSeenAt     int64   `gorm:"column:first_seen_at" json:"first_seen_at,omitempty"` // Unix ms
	InitialQuantity float64 `gorm:"column:initial_quantity" json:"initial_quantity,omitempty"`

	// Conditional entry (C6): the serialized pending entry and its expiry.
	PendingPayload string `gorm:"column:pending_payload;type:text" json:"-"`
	ExpiresAt      int64  `gorm:"column:expires_at" json:"expires_at,omitempty"`
	// EntrySource (U9 P0-2): provenance marker for pending entries
	// ("" for scan-time #162 entries, "rerouted_conditional_entry" for
	// confirmation-only guard-veto reroutes).
	EntrySource string `gorm:"column:entry_source" json:"entry_source,omitempty"`

	Note      string `gorm:"column:note;type:text" json:"note,omitempty"`
	CreatedAt int64  `gorm:"column:created_at" json:"created_at"` // Unix ms
	UpdatedAt int64  `gorm:"column:updated_at" json:"updated_at"` // Unix ms
	ClosedAt  int64  `gorm:"column:closed_at" json:"closed_at,omitempty"`
}

// TableName is the lifecycle table name.
func (PositionLifecycle) TableName() string { return "position_lifecycles" }

func (s *DecisionStore) initLifecycleTable() error {
	return s.db.AutoMigrate(&PositionLifecycle{})
}

func normalizeLifecycleSide(side string) string {
	return strings.ToUpper(strings.TrimSpace(side))
}

// CreateLifecycle inserts a lifecycle row and fills its ID.
func (s *DecisionStore) CreateLifecycle(rec *PositionLifecycle) error {
	if rec == nil {
		return fmt.Errorf("nil lifecycle record")
	}
	now := time.Now().UnixMilli()
	rec.Symbol = strings.ToUpper(strings.TrimSpace(rec.Symbol))
	rec.Side = normalizeLifecycleSide(rec.Side)
	if rec.CreatedAt == 0 {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now
	return s.db.Create(rec).Error
}

// UpdateLifecycle updates the given columns of one row. updated_at is set,
// and closed_at when the status becomes closed.
func (s *DecisionStore) UpdateLifecycle(id uint, fields map[string]interface{}) error {
	if id == 0 {
		return fmt.Errorf("lifecycle id is required")
	}
	if fields == nil {
		fields = map[string]interface{}{}
	}
	now := time.Now().UnixMilli()
	fields["updated_at"] = now
	if fields["status"] == LifecycleStatusClosed {
		if _, ok := fields["closed_at"]; !ok {
			fields["closed_at"] = now
		}
	}
	return s.db.Model(&PositionLifecycle{}).Where("id = ?", id).Updates(fields).Error
}

// GetLifecycle loads one row by ID.
func (s *DecisionStore) GetLifecycle(id uint) (*PositionLifecycle, error) {
	var rec PositionLifecycle
	if err := s.db.Where("id = ?", id).First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListLifecycles returns a trader's rows in the given statuses, oldest first.
func (s *DecisionStore) ListLifecycles(traderID string, statuses ...string) ([]PositionLifecycle, error) {
	var rows []PositionLifecycle
	q := s.db.Where("trader_id = ?", traderID)
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	err := q.Order("id ASC").Find(&rows).Error
	return rows, err
}

// LatestLifecycle returns the newest row for a trader/symbol/side in the
// given statuses, or nil when none exists.
func (s *DecisionStore) LatestLifecycle(traderID, symbol, side string, statuses ...string) (*PositionLifecycle, error) {
	var rows []PositionLifecycle
	q := s.db.Where("trader_id = ? AND symbol = ? AND side = ?", traderID,
		strings.ToUpper(strings.TrimSpace(symbol)), normalizeLifecycleSide(side))
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	if err := q.Order("id DESC").Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}
