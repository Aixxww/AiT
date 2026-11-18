package kernel

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

func u11HybridConfig(lang string) *store.StrategyConfig {
	cfg := u11FixtureConfig(lang)
	cfg.CoinSource.Hunter.V7Prompt = &store.V7PromptConfig{PayloadVersion: store.V7PayloadVersionHybridV2}
	return cfg
}

// u11RenderHybrid renders the fixture with the hybrid_v2 payload at the
// fixture's prompt time; it returns the golden document and its parts.
func u11RenderHybrid(cfg *store.StrategyConfig, mutate ...func(*Context)) (doc, system, doctrine, user string) {
	e := NewStrategyEngine(cfg)
	ctx := u11FixtureContext(e)
	ctx.PromptTime = u11PromptTime
	u11WithRequiredConfirmations(ctx)
	for _, m := range mutate {
		m(ctx)
	}
	system = e.BuildSystemPrompt(1000, "")
	doctrine = hunterV7DoctrineV2Prompt(e.GetLanguage(), e.hunterV7DoctrineV2Options())
	user = e.BuildUserPrompt(ctx)
	sys := system
	if i := strings.Index(sys, "\n\n---\n\n"); i >= 0 {
		sys = "<data dictionary omitted>" + sys[i:]
	}
	return "===== SYSTEM =====\n" + sys + "\n===== USER =====\n" + user, system, doctrine, user
}

// u11WithRequiredConfirmations gives two candidates a must-check list, so
// the hybrid_v2 tests cover required_confirmations (PR #28 owner decision 3).
// Applied only on the hybrid path; the legacy goldens keep the base fixture.
// The confirmation summary passes review, as a module emits it when the
// list is reviewable, so the tiers stay those of the base fixture.
func u11WithRequiredConfirmations(ctx *Context) {
	for i := range ctx.CandidateCoins {
		switch ctx.CandidateCoins[i].Symbol {
		case "LEADUSDT", "TRIGUSDT":
			ctx.CandidateCoins[i].V7ConfirmSummary = &local.V7ConfirmationSummary{PassedHard: true, PassedReview: true}
		}
		switch ctx.CandidateCoins[i].Symbol {
		case "LEADUSDT":
			ctx.CandidateCoins[i].V7RequiredConfirms = []string{"5m_price_holds_ema20_or_trailing_support", "live_price_in_entry_zone"}
		case "TRIGUSDT":
			ctx.CandidateCoins[i].V7RequiredConfirms = []string{"5m_or_15m_close_above_trigger", "taker_flow_confirms_long"}
		}
	}
}

// The hybrid_v2 payload golden (D3 (b): verified with the config enabled).
func TestU11HybridV2PromptGolden(t *testing.T) {
	en, _, _, _ := u11RenderHybrid(u11HybridConfig("en"))
	u11CheckGolden(t, "hunter_v7_prompt_hybrid_v2_en.golden", en)
	zh, _, _, _ := u11RenderHybrid(u11HybridConfig("zh"))
	u11CheckGolden(t, "hunter_v7_prompt_hybrid_v2_zh.golden", zh)
}

// u11DeletedFieldTokens is the fields-audit deletion list (spec C). The
// audit counts 11; the list has 12 entries because "flow labels" and the
// "compact execution summary line" are both listed. All 12 are enforced.
var u11DeletedFieldTokens = []string{
	"setup_score", "timing_score", "ai_priority", "confidence", "risk_score",
	"liquidity_score", "regime_fit_score", "market_regime", "market_shape", "entry_signal",
}

// Entry 12, the compact execution summary line: its markers.
var u11DeletedLineMarkers = []string{"Hunter v7 execution compact", "compact_execution_json", "execution_compact"}

// Entry 11, the flow labels: every catalog flow_ tag, plus any token that
// starts with flow_.
var u11FlowTokenRe = regexp.MustCompile(`(^|[^a-z0-9_])flow_[a-z0-9_]+`)

// u11DeletedHits returns every deleted-list entry found in text. allowed
// lists tokens exempt for this text.
func u11DeletedHits(text string, allowed ...string) []string {
	lower := strings.ToLower(text)
	skip := map[string]bool{}
	for _, a := range allowed {
		skip[a] = true
	}
	var hits []string
	for _, tok := range u11DeletedFieldTokens {
		if !skip[tok] && strings.Contains(lower, tok) {
			hits = append(hits, tok)
		}
	}
	for _, m := range u11DeletedLineMarkers {
		if strings.Contains(lower, strings.ToLower(m)) {
			hits = append(hits, m)
		}
	}
	for _, def := range local.HunterV7TagCatalogEntries() {
		if strings.HasPrefix(def.Tag, "flow_") && strings.Contains(lower, def.Tag) {
			hits = append(hits, def.Tag)
		}
	}
	if m := u11FlowTokenRe.FindString(lower); m != "" {
		hits = append(hits, "flow label "+strings.TrimSpace(m))
	}
	return hits
}

// TestU11DeletedFieldsNeverReachHybridPrompt is the deletion test: none of
// the 12 deleted entries reappears in the prompt as presented to the LLM
// with hybrid_v2 on. The user prompt and the doctrine are checked without
// exemption. The rest of the system prompt is checked for every entry
// except "confidence", which there is the LLM's own output field
// (Decision.Confidence, "confidence": 0-100 in the output schema and the
// sizing guidance), not the module's deleted confidence label.
func TestU11DeletedFieldsNeverReachHybridPrompt(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		for _, prefix := range []bool{false, true} {
			cfg := u11HybridConfig(lang)
			cfg.CoinSource.Hunter.V7T2SetupPrefix = prefix
			_, system, doctrine, user := u11RenderHybrid(cfg)
			if !strings.Contains(system, doctrine) {
				t.Fatalf("%s: system prompt does not carry the hybrid_v2 doctrine", lang)
			}
			if hits := u11DeletedHits(user); len(hits) > 0 {
				t.Errorf("%s prefix=%v: user prompt carries deleted entries %v", lang, prefix, hits)
			}
			if hits := u11DeletedHits(doctrine); len(hits) > 0 {
				t.Errorf("%s: doctrine carries deleted entries %v", lang, hits)
			}
			if hits := u11DeletedHits(strings.Replace(system, doctrine, "", 1), "confidence"); len(hits) > 0 {
				t.Errorf("%s: system prompt carries deleted entries %v", lang, hits)
			}
		}
	}
	// Every summary path: positions held up to the limit (open-disabled
	// summary, position block) and a REJECTED candidate whose tier reason
	// names a deleted field.
	stress := func(ctx *Context) {
		rejected := hunterV7PromptReadyLeaderCandidate("RISKUSDT", 5)
		rejected.V7RiskScore = 70
		ctx.CandidateCoins = append(ctx.CandidateCoins, rejected)
		ctx.MarketDataMap["RISKUSDT"] = u11MarketData("RISKUSDT", 5, 0.4)
		for i, sym := range []string{"POSAUSDT", "POSBUSDT", "POSCUSDT"} {
			ctx.Positions = append(ctx.Positions, PositionInfo{Symbol: sym, Side: "long", EntryPrice: 10, MarkPrice: 10.2, Quantity: 1, Leverage: 3, UnrealizedPnLPct: 6, PeakPnLPct: 7, UpdateTime: u11PromptTime.UnixMilli() - int64(i)*60000})
			ctx.MarketDataMap[sym] = u11MarketData(sym, 10.2, float64(i))
		}
	}
	for _, lang := range []string{"en", "zh"} {
		_, _, _, user := u11RenderHybrid(u11HybridConfig(lang), stress)
		if !strings.Contains(user, "risk_penalty_0_100_gte_65: 1") && !strings.Contains(user, "tier_reason=risk_penalty_0_100_gte_65") {
			t.Fatalf("%s: stress fixture did not reach the REJECTED path", lang)
		}
		if hits := u11DeletedHits(user); len(hits) > 0 {
			t.Errorf("%s stress: user prompt carries deleted entries %v", lang, hits)
		}
	}
	// The detector is live: the legacy prompt of the same fixture trips it.
	legacy := u11RenderPrompts(u11FixtureConfig("en"))
	if hits := u11DeletedHits(legacy); len(hits) < 12 {
		t.Fatalf("detector found only %v in the legacy prompt", hits)
	}
}

// The 12 entries reconcile with the audit's list of 11: ten field names,
// the flow labels and the compact line.
func TestU11DeletionListHasTwelveEntries(t *testing.T) {
	if n := len(u11DeletedFieldTokens) + 1 + 1; n != 12 {
		t.Fatalf("deletion list has %d entries, want 12", n)
	}
}

func u11HybridPayloads(t *testing.T, cfg *store.StrategyConfig) map[string]hunterV7V2Payload {
	t.Helper()
	_, _, _, user := u11RenderHybrid(cfg)
	out := map[string]hunterV7V2Payload{}
	for _, line := range strings.Split(user, "\n") {
		raw, ok := strings.CutPrefix(line, "hunter_v7_signal_v2_json: ")
		if !ok {
			continue
		}
		var p hunterV7V2Payload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("payload json: %v", err)
		}
		out[p.Signal.Symbol] = p
	}
	return out
}

// D1 (a): llm_role per candidate, descriptive only.
func TestU11LLMRolePerClass(t *testing.T) {
	got := u11HybridPayloads(t, u11HybridConfig("en"))
	want := map[string]string{
		"LEADUSDT": "role_full",                  // unassigned class (fixture)
		"TRIGUSDT": "role_decide_before_trigger", // TRIGGER, conditional (WATCH)
		"IMPUSDT":  "role_full",                  // IMPULSE
		"REVUSDT":  "role_full",                  // REVERSAL
	}
	for sym, role := range want {
		if got[sym].Execution.LLMRole != role {
			t.Errorf("%s llm_role = %q, want %q", sym, got[sym].Execution.LLMRole, role)
		}
	}
	pc := store.EffectiveV7Prompt(nil)
	for _, c := range []struct {
		class string
		cond  bool
		want  string
	}{
		{store.V7ClassTrigger, false, store.V7LLMRoleFull}, // after the trigger
		{store.V7ClassTrigger, true, store.V7LLMRoleDecideBeforeTrigger},
		{store.V7ClassContext, false, store.V7LLMRoleNone},
		{"", false, store.V7LLMRoleFull},
	} {
		if r := pc.LLMRole(c.class, c.cond); r != c.want {
			t.Errorf("role(%q, %v) = %q, want %q", c.class, c.cond, r, c.want)
		}
	}
	over := store.EffectiveV7Prompt(&store.V7PromptConfig{LLMRoles: map[string]string{"reversal": "none", "IMPULSE": "bogus"}})
	if over.LLMRole(store.V7ClassReversal, false) != store.V7LLMRoleNone || over.LLMRole(store.V7ClassImpulse, false) != store.V7LLMRoleFull {
		t.Fatalf("role overrides: %v", over.LLMRoles)
	}
}

// The appendix fields of the hybrid spec B, with their units and the
// single reference price.
func TestU11HybridAppendixContract(t *testing.T) {
	got := u11HybridPayloads(t, u11HybridConfig("en"))
	lead, trig := got["LEADUSDT"], got["TRIGUSDT"]
	if lead.Market.PriceAtPromptBuild != 2.0 || lead.Signal.ZonePositionPct == nil || *lead.Signal.ZonePositionPct != 50 {
		t.Fatalf("lead zone position: %+v", lead.Signal.ZonePositionPct)
	}
	if lead.Signal.InvalidationDistancePct == nil || *lead.Signal.InvalidationDistancePct != -2.5 {
		t.Fatalf("lead invalidation distance: %v", lead.Signal.InvalidationDistancePct)
	}
	// RR from 2.0 with stop 1.95: TP1 2.12 -> 0.12/0.05 = 2.4.
	if lead.Signal.TP1RRRatio == nil || *lead.Signal.TP1RRRatio != 2.4 || lead.Signal.NearestTargetRRRatio == nil || *lead.Signal.NearestTargetRRRatio != 2.4 {
		t.Fatalf("lead RRs: %+v", lead.Signal)
	}
	if trig.Signal.SetupType != "t2_ignition_early_long" || trig.Signal.LifecycleState != "lifecycle_WATCH" || trig.Signal.Class != "class_TRIGGER" {
		t.Fatalf("t2 identity: %+v", trig.Signal)
	}
	if trig.Signal.CellStatus != "cell_probation" || trig.Execution.DoctrineVersion != store.V7DoctrineVersionHybridV2 {
		t.Fatalf("cell_status / doctrine_version: %q %q", trig.Signal.CellStatus, trig.Execution.DoctrineVersion)
	}
	if strings.Join(trig.Signal.RequiredConfirmations, ",") != "5m_or_15m_close_above_trigger,taker_flow_confirms_long" ||
		strings.Join(lead.Signal.RequiredConfirmations, ",") != "5m_price_holds_ema20_or_trailing_support,live_price_in_entry_zone" {
		t.Fatalf("required_confirmations: %v / %v", trig.Signal.RequiredConfirmations, lead.Signal.RequiredConfirmations)
	}
	if got["IMPUSDT"].Signal.RequiredConfirmations == nil {
		t.Fatal("required_confirmations must be present (empty list) when the module lists none")
	}
	if trig.Signal.ZonePositionPct != nil {
		t.Fatalf("single-price zone must omit zone_position_pct")
	}
	if trig.Execution.Tier != "tier_REVIEWABLE" || trig.Execution.EntryPermission != "conditional_on_trigger" || trig.Execution.Limits.MinRRRatio != 1.2 || trig.Execution.Limits.MinStopDistancePct != 0.5 {
		t.Fatalf("t2 execution: %+v", trig.Execution)
	}
	if trig.Market.OINotionalUSDT == nil || *trig.Market.OINotionalUSDT != 12_500_000 || trig.Market.TakerBuyRatioLatest15mBarFrac == nil || trig.Market.SnapshotAgeMs != 4200 {
		t.Fatalf("t2 market: %+v", trig.Market)
	}
	if trig.Signal.MenuBasis.Mode != "reference_levels" || trig.Signal.MenuBasis.HorizonClass != "FAST" || trig.Signal.MenuBasis.StructureBars != 39 {
		t.Fatalf("menu basis: %+v", trig.Signal.MenuBasis)
	}
}

// The bars block: 30 closed bars per timeframe, the unfinished bar apart.
func TestU11HybridBarsBlock(t *testing.T) {
	_, _, _, user := u11RenderHybrid(u11HybridConfig("en"))
	for _, tf := range []string{"5m", "15m", "1h", "4h"} {
		if !strings.Contains(user, "bars_"+tf+" (30 closed, old->new, UTC;") {
			t.Errorf("bars_%s header missing", tf)
		}
		if !strings.Contains(user, "unfinished_"+tf+" (unfinished — do not use for levels): ") {
			t.Errorf("unfinished_%s line missing", tf)
		}
	}
	// The 15m unfinished bar opened at 12:00 (prompt time 12:07:30); the
	// last closed 15m bar opened at 11:45.
	if !strings.Contains(user, "unfinished_15m (unfinished — do not use for levels): 2026-11-15T12:00Z ") {
		t.Fatalf("15m unfinished bar is not the 12:00 bar")
	}
	if !strings.Contains(user, "\n2026-11-15T11:45Z ") {
		t.Fatalf("last closed 15m bar missing")
	}
}

// D3 (b): default off. No v7_prompt block, an empty block and an unknown
// version all read as legacy.
func TestU11DefaultIsLegacy(t *testing.T) {
	for _, c := range []*store.V7PromptConfig{nil, {}, {PayloadVersion: "hybrid_v3"}} {
		pc := store.EffectiveV7Prompt(c)
		if pc.HybridV2() || pc.PayloadVersion != store.V7PayloadVersionLegacy || pc.DoctrineVersion() != store.V7DoctrineVersionNone {
			t.Fatalf("%+v -> %+v", c, pc)
		}
	}
	cfg := u11FixtureConfig("en")
	if NewStrategyEngine(cfg).hunterV7PromptHybridV2() {
		t.Fatal("default must be legacy")
	}
	cfg = u11HybridConfig("en")
	cfg.CoinSource.SourceType = "ai500"
	if NewStrategyEngine(cfg).hunterV7PromptHybridV2() {
		t.Fatal("hybrid_v2 applies to hunter_v7 only")
	}
}

// U5 doctrine_version tag: the legacy doctrine keeps "none", hybrid_v2
// records its version.
func TestU11DoctrineVersionTag(t *testing.T) {
	sigs := []local.V7SignalOutput{t2TagSignal("ignition_early_long", local.V7DirLong, "TRIGGERED")}
	cands := AssembleHunterV7CandidateCoins(sigs, "BOTH", defaultGeometry())
	for _, c := range []struct {
		cfg  *store.StrategyConfig
		want string
	}{
		{u11FixtureConfig("en"), store.V7DoctrineVersionNone},
		{u11HybridConfig("en"), store.V7DoctrineVersionHybridV2},
	} {
		recs := BuildHunterV7SignalRecordsTagged(sigs, cands, HunterV7RecordTagOptionsFromConfig(c.cfg))
		if recs[0].Tags.DoctrineVersion != c.want {
			t.Errorf("doctrine_version = %q, want %q", recs[0].Tags.DoctrineVersion, c.want)
		}
	}
}

// Pass-1 renames inside reason codes.
func TestU11ReasonCodeRenames(t *testing.T) {
	for in, want := range map[string]string{
		"risk_score_gte_65":                  "risk_penalty_0_100_gte_65",
		"prompt_readiness_risk_score_gte_65": "prompt_readiness_risk_penalty_0_100_gte_65",
		"flow_taker_buy_weak":                "taker_flow_gate",
		"tag_flow_taker_sell_strong":         "tag_taker_flow_gate",
		"whale_flow_reversal_wait":           "whale_flow_reversal_wait",
		"t2_trigger_conditional_watch":       "t2_trigger_conditional_watch",
		"context_only_low_priority":          "context_only_low_priority",
	} {
		if got := hunterV7V2ReasonCode(in); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}

// hybrid_v2 fetches its timeframes and one bar over the closed count,
// without touching the strategy's slice.
func TestU11MergeTimeframes(t *testing.T) {
	have := make([]string, 3, 8)
	copy(have, []string{"5m", "15m", "1h"})
	got := hunterV7MergeTimeframes(have, store.DefaultV7PromptTimeframes)
	if strings.Join(got, ",") != "5m,15m,1h,4h" || have[:4][3] != "" {
		t.Fatalf("merge = %v (have aliased: %q)", got, have[:4][3])
	}
}

// D2 (a): a prefixed Task-2 setup assembles exactly like its scanner name
// (class, admission, tier, risk stamps); only the setup string differs.
func TestU11PrefixedT2SetupAssemblesLikeCanonical(t *testing.T) {
	g := condGeometry(true)
	for _, setup := range []string{"ignition_early_long", "ignition_early_short", "momentum_continuation_short", "trend_pullback_short", "reclaim_breakout_long"} {
		plain := t2TagSignal(setup, local.V7DirLong, "WATCH")
		pref := plain
		pref.SetupType = local.V7SetupType("t2_" + setup)
		c := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{plain, pref}, "BOTH", g)
		a, b := c[0], c[1]
		if a.V7SignalClass != b.V7SignalClass || a.V7ClassAdmission != b.V7ClassAdmission || a.V7ExecutionTier != b.V7ExecutionTier ||
			a.V7TierReason != b.V7TierReason || a.V7RequiredRR != b.V7RequiredRR || a.V7RequiredMinStop != b.V7RequiredMinStop ||
			a.V7RequiredMaxTP != b.V7RequiredMaxTP || a.V7ConditionalEntry != b.V7ConditionalEntry {
			t.Errorf("%s: plain %+v vs prefixed %+v", setup, []any{a.V7SignalClass, a.V7ExecutionTier, a.V7TierReason}, []any{b.V7SignalClass, b.V7ExecutionTier, b.V7TierReason})
		}
	}
	// The U5 family tag is the canonical family under either name.
	sigs := []local.V7SignalOutput{t2TagSignal("t2_ignition_early_long", local.V7DirLong, "TRIGGERED")}
	recs := BuildHunterV7SignalRecordsTagged(sigs, AssembleHunterV7CandidateCoins(sigs, "BOTH", defaultGeometry()), DefaultHunterV7RecordTagOptions())
	if recs[0].Tags.Family != "ignition_early" || recs[0].Tags.SignalClass != store.V7ClassTrigger {
		t.Fatalf("tags = %+v", recs[0].Tags)
	}
	// Emission wiring: the strategy flag reaches the T2 module config.
	if hunterV7T2ModuleConfig(&store.HunterConfig{}).SetupPrefix || !hunterV7T2ModuleConfig(&store.HunterConfig{V7T2SetupPrefix: true}).SetupPrefix {
		t.Fatal("v7_t2_setup_prefix not wired to the T2 module (default must be off)")
	}
}
