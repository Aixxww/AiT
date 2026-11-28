package kernel

import (
	"testing"
	"time"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// The OD-2 cost used for fee-net R is the structure planner's CostPct, the
// value behind every U0 fee-net R.
func TestFeeNetCostMatchesStructureCost(t *testing.T) {
	if got := local.V7StructureConfigForSetup("ignition_early_long").CostPct; got != store.DefaultV7FeeNetCostPct {
		t.Fatalf("structure CostPct %v != DefaultV7FeeNetCostPct %v", got, store.DefaultV7FeeNetCostPct)
	}
}

func t2TagSignal(setup string, dir local.V7Direction, state string) local.V7SignalOutput {
	// LONG: entry 100, stop 99 (1%), TP1 101.8; ATR15m 1.25% -> stop 0.8 ATR.
	stop, tp1, tp2 := 99.0, 101.8, 103.0
	if dir == local.V7DirShort {
		stop, tp1, tp2 = 101, 98.2, 97
	}
	return local.V7SignalOutput{
		SignalID: setup, Stack: local.V7StackT2, LifecycleState: state, TriggerPrice: 100, ATR15mPct: 1.25, LiquidityScore: 80,
		DataGaps: []string{local.V7T2DataGapGlobalLSR, local.V7T2DataGapMarketCap},
		Symbol:   "AUSDT", Direction: dir, SetupType: local.V7SetupType(setup), Status: local.V7StatusCandidate,
		EntryZone: local.V7PriceZone{Lower: 100, Upper: 100}, Invalidation: local.V7InvalidationRule{Price: stop},
		Targets: []local.V7Target{{Price: tp1}, {Price: tp2}}, TP1Price: tp1, TP2Price: tp2, RiskLevel: local.V7RiskMedium,
	}
}

func TestT2RecordCarriesAllTags(t *testing.T) {
	sigs := []local.V7SignalOutput{
		t2TagSignal("ignition_early_long", local.V7DirLong, "TRIGGERED"),
		t2TagSignal("ignition_early_short", local.V7DirShort, "TRIGGERED"),
		t2TagSignal("ignition_early_long", local.V7DirLong, "WATCH"),
	}
	sigs[2].Symbol = "CUSDT"
	sigs[1].Symbol = "BUSDT"
	cands := AssembleHunterV7CandidateCoins(sigs, "BOTH", defaultGeometry())
	recs := BuildHunterV7SignalRecordsTagged(sigs, cands, DefaultHunterV7RecordTagOptions())
	db := BuildHunterV7SignalDBRecordsWithAudit(1, recs, time.Now(), "ACTIVE", false)

	r := db[0]
	if recs[0].Tier != "REVIEWABLE" {
		t.Fatalf("fixture tier = %s/%s", recs[0].Tier, recs[0].TierReason)
	}
	want := map[string]string{
		"stack": "t2", "signal_class": "TRIGGER", "family": "ignition_early", "rr_band": "1.2_1.5",
		"stop_atr_band": "0.5_1.0", "cell_status": "probation", "profile_version": store.V7SignalClassRegistryVersion + "/" + store.V7ClassProfileVersion,
		"calibration_version": "none", "doctrine_version": "none", "lifecycle": "TRIGGERED", "decision_path": "llm_full",
		"outcome_source": "prompt_open_review", "data_gaps": `["global_lsr_default_1.0","market_cap_missing"]`,
	}
	got := map[string]string{
		"stack": r.Stack, "signal_class": r.SignalClass, "family": r.Family, "rr_band": r.RRBand,
		"stop_atr_band": r.StopATRBand, "cell_status": r.CellStatus, "profile_version": r.ProfileVersion,
		"calibration_version": r.CalibrationVersion, "doctrine_version": r.DoctrineVersion, "lifecycle": r.LifecycleStateAtDecision,
		"decision_path": r.DecisionPath, "outcome_source": r.OutcomeSource, "data_gaps": r.DataGaps,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if r.TriggerPrice == nil || *r.TriggerPrice != 100 || r.TrackStatus != "ACTIVE" {
		t.Errorf("trigger price / track status = %v / %q", r.TriggerPrice, r.TrackStatus)
	}
	// Shadow-only class: shadow source, no LLM path, not tracked without audit.
	if db[1].OutcomeSource != "shadow" || db[1].DecisionPath != "none" || db[1].TrackStatus != "" {
		t.Errorf("shadow row = %q/%q/%q", db[1].OutcomeSource, db[1].DecisionPath, db[1].TrackStatus)
	}
	// TRIGGER class before the trigger: since #162 the sole-reason
	// awaiting-trigger row advances flagged to open review (its lifecycle
	// state at decision stays WATCH; the row is tracked as an LLM row).
	if db[2].OutcomeSource != "prompt_open_review" || db[2].LifecycleStateAtDecision != "WATCH" || db[2].TrackStatus != "ACTIVE" {
		t.Errorf("watch row = %q/%q/%q", db[2].OutcomeSource, db[2].LifecycleStateAtDecision, db[2].TrackStatus)
	}
	// With watch_audit on, the shadow row is tracked too; the #162 row is
	// an open-review row and is tracked regardless of the audit switch.
	audit := BuildHunterV7SignalDBRecordsWithAudit(1, recs, time.Now(), "ACTIVE", true)
	if audit[1].TrackStatus != "ACTIVE" || audit[2].TrackStatus != "ACTIVE" {
		t.Errorf("audit tracking = %q/%q", audit[1].TrackStatus, audit[2].TrackStatus)
	}
	if !HunterV7ShouldTrackRecord(recs[2], true) || !HunterV7ShouldTrackRecord(recs[2], false) {
		t.Errorf("HunterV7ShouldTrackRecord audit switch")
	}
}

// Native records: native stack, the native status as lifecycle, no
// trigger price, no data gaps; unknown bands when inputs are missing.
func TestNativeRecordTags(t *testing.T) {
	sig := local.V7SignalOutput{Symbol: "NUSDT", Direction: local.V7DirLong, SetupType: "leader_momentum_long", Status: local.V7StatusWaitConfirm}
	recs := BuildHunterV7SignalRecords([]local.V7SignalOutput{sig}, nil)
	db := BuildHunterV7SignalDBRecords(1, recs, time.Now(), "ACTIVE")[0]
	if db.Stack != "native" || db.SignalClass != "IMPULSE" || db.LifecycleStateAtDecision != "wait_confirm" ||
		db.TriggerPrice != nil || db.DataGaps != "[]" || db.RRBand != store.V7BandUnknown || db.StopATRBand != store.V7BandUnknown ||
		db.OutcomeSource != "watch_audit" || db.DecisionPath != "none" {
		t.Fatalf("native tags = %+v", db)
	}
	un := BuildHunterV7SignalRecords([]local.V7SignalOutput{{Symbol: "M", Direction: local.V7DirLong, SetupType: "mms_trend_ride_long"}}, nil)
	if un[0].Tags.SignalClass != store.V7ClassUnassigned {
		t.Fatalf("unassigned class = %q", un[0].Tags.SignalClass)
	}
}

func TestRecordTagOptionsFromConfig(t *testing.T) {
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.Hunter = &store.HunterConfig{V7OutcomeTags: &store.V7OutcomeTagConfig{RRBandEdges: []float64{1.5}}}
	o := HunterV7RecordTagOptionsFromConfig(cfg)
	if len(o.Tags.RRBandEdges) != 1 || o.Tags.FeeNetCostPct != store.DefaultV7FeeNetCostPct || o.Classes["ignition_early_long"].Class != store.V7ClassTrigger {
		t.Fatalf("options = %+v", o.Tags)
	}
	if d := HunterV7RecordTagOptionsFromConfig(nil); len(d.Tags.RRBandEdges) != 4 {
		t.Fatalf("nil config must give defaults")
	}
}
