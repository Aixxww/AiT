package store

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// #111 S2: an unknown prompt payload version is rejected at save.
func TestS2ValidateTradingConfigRejectsUnknownPayloadVersion(t *testing.T) {
	for _, v := range []string{"", V7PayloadVersionLegacy, V7PayloadVersionHybridV2} {
		cfg := GetDefaultStrategyConfig("en")
		cfg.CoinSource.Hunter = &HunterConfig{V7Prompt: &V7PromptConfig{PayloadVersion: v}}
		if err := ValidateTradingConfig(&cfg); err != nil && strings.Contains(err.Error(), "payload_version") {
			t.Fatalf("%q must be accepted: %v", v, err)
		}
	}
	for _, v := range []string{"hybrid_v3", "Hybrid_V2", "legacy "} {
		cfg := GetDefaultStrategyConfig("en")
		cfg.CoinSource.Hunter = &HunterConfig{V7Prompt: &V7PromptConfig{PayloadVersion: v}}
		err := ValidateTradingConfig(&cfg)
		if err == nil || !strings.Contains(err.Error(), "v7_prompt.payload_version") {
			t.Fatalf("%q must be rejected, got %v", v, err)
		}
	}
	// No hunter config or no v7_prompt block: nothing to check.
	cfg := GetDefaultStrategyConfig("en")
	if err := ValidateTradingConfig(&cfg); err != nil && strings.Contains(err.Error(), "payload_version") {
		t.Fatalf("no v7_prompt: %v", err)
	}
}

// The payload_version column is created by the migration and round-trips.
func TestS2PayloadVersionColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&HunterV7SignalRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !db.Migrator().HasColumn(&HunterV7SignalRecord{}, "payload_version") {
		t.Fatal("payload_version column missing")
	}
	rec := HunterV7SignalRecord{CycleNumber: 1, Timestamp: time.Now().UTC(), Symbol: "TESTUSDT", Direction: "LONG",
		SetupType: "leader_momentum_long", Status: "candidate", DoctrineVersion: V7DoctrineVersionHybridV2, PayloadVersion: V7PayloadVersionHybridV2}
	if err := NewHunterV7SignalStore(db).CreateBatch([]HunterV7SignalRecord{rec}); err != nil {
		t.Fatalf("create: %v", err)
	}
	var saved HunterV7SignalRecord
	if err := db.First(&saved).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if saved.PayloadVersion != V7PayloadVersionHybridV2 || saved.DoctrineVersion != V7DoctrineVersionHybridV2 {
		t.Fatalf("saved %q / %q", saved.PayloadVersion, saved.DoctrineVersion)
	}
}
