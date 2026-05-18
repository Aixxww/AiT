package local

import (
	"log"
	"sync"
	"time"

	"nofx/store"

	"gorm.io/gorm"
)

// symbolCooldown tracks per-symbol selection history for smart cooldown.
type symbolCooldown struct {
	ConsecutiveWaits  int       // how many cycles symbol was a candidate but AI said "wait"
	Selections24h     int       // times selected in last 24h
	LastSelected      time.Time // last time this symbol was selected
	OIFilteredCount   int       // times filtered by OI threshold
	VolatileTrapUntil time.Time // if set, exclude until this time
}

// cooldownTracker is a global in-memory tracker backed by SQLite.
type cooldownTracker struct {
	mu      sync.RWMutex
	symbols map[string]*symbolCooldown
	db      *gorm.DB
}

var globalCooldown = &cooldownTracker{
	symbols: make(map[string]*symbolCooldown),
}

var initCooldownOnce sync.Once

// SetCooldownDB initializes the global cooldown tracker with a GORM database.
// It auto-migrates the HunterCooldown table and loads existing records into memory.
// Safe to call multiple times — only the first call takes effect.
func SetCooldownDB(db *gorm.DB) {
	initCooldownOnce.Do(func() {
		if db == nil {
			return
		}
		if err := db.AutoMigrate(&store.HunterCooldown{}); err != nil {
			log.Printf("⚠️  Hunter cooldown: AutoMigrate failed: %v", err)
			return
		}
		globalCooldown.db = db
		globalCooldown.loadFromDB()
	})
}

// loadFromDB reads all persisted cooldown records into the in-memory map.
func (ct *cooldownTracker) loadFromDB() {
	if ct.db == nil {
		return
	}
	var records []store.HunterCooldown
	if err := ct.db.Find(&records).Error; err != nil {
		log.Printf("⚠️  Hunter cooldown: load failed: %v", err)
		return
	}
	ct.mu.Lock()
	defer ct.mu.Unlock()
	for _, r := range records {
		ct.symbols[r.Symbol] = &symbolCooldown{
			ConsecutiveWaits:  r.ConsecutiveWaits,
			Selections24h:     r.Selections24h,
			LastSelected:      r.LastSelected,
			OIFilteredCount:   r.OIFilteredCount,
			VolatileTrapUntil: r.VolatileTrapUntil,
		}
	}
	if len(records) > 0 {
		log.Printf("🔗 Hunter cooldown: loaded %d records from DB", len(records))
	}
}

// persistSymbol upserts a single symbol's cooldown state to the database.
// Must be called while ct.mu is held (at least RLock).
func (ct *cooldownTracker) persistSymbol(symbol string, sc *symbolCooldown) {
	if ct.db == nil {
		return
	}
	// Fire-and-forget upsert outside the lock to avoid deadlocks.
	// Copy the values we need so the caller can release the lock.
	record := store.HunterCooldown{
		Symbol:            symbol,
		ConsecutiveWaits:  sc.ConsecutiveWaits,
		Selections24h:     sc.Selections24h,
		LastSelected:      sc.LastSelected,
		OIFilteredCount:   sc.OIFilteredCount,
		VolatileTrapUntil: sc.VolatileTrapUntil,
	}
	go func() {
		ct.db.Where("symbol = ?", symbol).Assign(record).FirstOrCreate(&store.HunterCooldown{})
	}()
}

// getCooldownMultiplier returns score modifier for a symbol.
// Returns 1.0 (no penalty), 0.50 (consecutive waits), or 0.0 (volatile trap excluded).
func (ct *cooldownTracker) getCooldownMultiplier(symbol string) float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	sc, ok := ct.symbols[symbol]
	if !ok {
		return 1.0
	}
	// Volatile trap: exclude for 24h
	if !sc.VolatileTrapUntil.IsZero() && time.Now().Before(sc.VolatileTrapUntil) {
		return 0.0
	}
	// Consecutive waits penalty
	if sc.ConsecutiveWaits >= 2 {
		return 0.50
	}
	return 1.0
}

// recordWait increments consecutive_waits for a symbol.
func (ct *cooldownTracker) recordWait(symbol string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	sc, ok := ct.symbols[symbol]
	if !ok {
		sc = &symbolCooldown{}
		ct.symbols[symbol] = sc
	}
	sc.ConsecutiveWaits++
	// If 24h selections > 5, tag as volatile trap
	if sc.Selections24h > 5 {
		sc.VolatileTrapUntil = time.Now().Add(24 * time.Hour)
	}
	ct.persistSymbol(symbol, sc)
}

// recordSelection resets consecutive_waits and increments 24h count.
func (ct *cooldownTracker) recordSelection(symbol string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	sc, ok := ct.symbols[symbol]
	if !ok {
		sc = &symbolCooldown{}
		ct.symbols[symbol] = sc
	}
	sc.ConsecutiveWaits = 0
	sc.Selections24h++
	sc.LastSelected = time.Now()
	ct.persistSymbol(symbol, sc)
}

// getOIThresholdReduction returns a reduced OI threshold multiplier for
// coins that have been OI-filtered multiple times (anti-repeat).
func (ct *cooldownTracker) getOIThresholdReduction(symbol string) float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	sc, ok := ct.symbols[symbol]
	if !ok || sc.OIFilteredCount < 3 {
		return 1.0
	}
	return 0.80 // 20% lower OI threshold
}

func (ct *cooldownTracker) recordOIFilter(symbol string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	sc, ok := ct.symbols[symbol]
	if !ok {
		sc = &symbolCooldown{}
		ct.symbols[symbol] = sc
	}
	sc.OIFilteredCount++
	ct.persistSymbol(symbol, sc)
}
