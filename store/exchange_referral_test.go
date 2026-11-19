package store

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// OD-19: referral_id exists and defaults to empty for new accounts.
func TestExchangeReferralIDDefaultsEmpty(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	s := NewExchangeStore(db)
	if err := s.initTables(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !db.Migrator().HasColumn(&Exchange{}, "referral_id") {
		t.Fatal("referral_id column missing")
	}
	id, err := s.Create("u1", "binance", "Main", true, "", "", "", false, "", false, "", "", "", "", "", "", 0, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ex, err := s.GetByID("u1", id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ex.ReferralID != "" {
		t.Fatalf("referral_id = %q, want empty", ex.ReferralID)
	}
}
