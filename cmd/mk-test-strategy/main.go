// mk-test-strategy creates the Tier A validation strategy row in a fresh
// test SQLite DB. Throwaway tooling for the 2026-11-19 sandbox live test
// only; it never touches production data and creates no traders.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/Aixxww/AiT/store"
)

func main() {
	dbPath := flag.String("db", "", "SQLite database path")
	flag.Parse()
	if *dbPath == "" {
		log.Fatal("--db is required")
	}
	st, err := store.New(*dbPath)
	if err != nil {
		log.Fatalf("open store failed: %v", err)
	}
	defer st.Close()

	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.Hunter = &store.HunterConfig{
		V7T2Enabled:     true,
		V7T2Setups:      []string{},
		V7MinAIPriority: 55,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		log.Fatalf("marshal config failed: %v", err)
	}
	now := time.Now().UTC()
	s := &store.Strategy{
		ID:          "t2-live-test-tier-a",
		UserID:      "live-test",
		Name:        "Tier A live validation (hunter_v7 + T2, min-priority 55)",
		Description: "Throwaway strategy for the 2026-11-19 sandbox Tier A run. No traders reference it; the harness only reads its config.",
		Config:      string(raw),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.Strategy().Create(s); err != nil {
		log.Fatalf("create strategy failed: %v", err)
	}
	fmt.Printf("strategy id: %s\n", s.ID)
	fmt.Printf("coin_source=%s v7_t2_enabled=%v v7_t2_setups=%v v7_min_ai_priority=%v\n",
		cfg.CoinSource.SourceType, cfg.CoinSource.Hunter.V7T2Enabled, cfg.CoinSource.Hunter.V7T2Setups, cfg.CoinSource.Hunter.V7MinAIPriority)
}
