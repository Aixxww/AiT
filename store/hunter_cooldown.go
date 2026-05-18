package store

import "time"

// HunterCooldown persists per-symbol cooldown state for the Hunter
// coin selection module. Survives process restarts.
type HunterCooldown struct {
	ID                uint      `gorm:"primaryKey"`
	Symbol            string    `gorm:"uniqueIndex;size:20"`
	ConsecutiveWaits  int
	Selections24h     int
	LastSelected      time.Time
	OIFilteredCount   int
	VolatileTrapUntil time.Time
	UpdatedAt         time.Time
}
