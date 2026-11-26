package kernel

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/mcp"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// #111 S2 (review item A): the prompt version header and the fail-closed
// prompt contract.

func s2Render(cfg *store.StrategyConfig) (system, user string) {
	_, system, _, user = u11RenderHybrid(cfg)
	return system, user
}

// Both modes print the version header before the Tier Summary, and a
// correctly paired round passes the contract.
func TestS2HeaderAndContractPass(t *testing.T) {
	for _, c := range []struct {
		cfg    *store.StrategyConfig
		header string
	}{
		{u11FixtureConfig("en"), "payload_version=legacy | doctrine_version=none\nTier Summary:"},
		{u11FixtureConfig("zh"), "payload_version=legacy | doctrine_version=none\nTier Summary:"},
		{u11HybridConfig("en"), "payload_version=hybrid_v2 | doctrine_version=hunter-v7-doctrine-hybrid-v2 | prompt_time_utc="},
		{u11HybridConfig("zh"), "payload_version=hybrid_v2 | doctrine_version=hunter-v7-doctrine-hybrid-v2 | prompt_time_utc="},
	} {
		system, user := s2Render(c.cfg)
		if !strings.Contains(user, c.header) {
			t.Fatalf("user prompt lacks %q", c.header)
		}
		if err := NewStrategyEngine(c.cfg).hunterV7PromptContractError(system, user); err != nil {
			t.Fatalf("a correctly paired round must pass: %v", err)
		}
	}
	// Other source types are not checked.
	other := store.GetDefaultStrategyConfig("en")
	if err := NewStrategyEngine(&other).hunterV7PromptContractError("", ""); err != nil {
		t.Fatalf("non-hunter_v7: %v", err)
	}
}

// Each failure mode of the contract.
func TestS2ContractFailsClosed(t *testing.T) {
	legacyCfg, hybridCfg := u11FixtureConfig("en"), u11HybridConfig("en")
	legacySys, legacyUser := s2Render(legacyCfg)
	hybridSys, hybridUser := s2Render(hybridCfg)
	legacy, hybrid := NewStrategyEngine(legacyCfg), NewStrategyEngine(hybridCfg)

	unknown := u11FixtureConfig("en")
	unknown.CoinSource.Hunter.V7Prompt = &store.V7PromptConfig{PayloadVersion: "hybrid_v3"}
	cases := []struct {
		name         string
		err          error
		wantContains string
	}{
		{"unknown stored version", NewStrategyEngine(unknown).hunterV7PromptContractError(legacySys, legacyUser), `unknown v7_prompt.payload_version "hybrid_v3"`},
		{"legacy payload, hybrid doctrine", legacy.hunterV7PromptContractError(hybridSys, legacyUser), "lacks the legacy doctrine"},
		{"hybrid payload, legacy doctrine", hybrid.hunterV7PromptContractError(legacySys, hybridUser), "lacks the hybrid_v2 doctrine"},
		{"both doctrines", legacy.hunterV7PromptContractError(legacySys+hybridSys, legacyUser), "carries the hybrid_v2 doctrine"},
		{"legacy strategy, hybrid user prompt", legacy.hunterV7PromptContractError(legacySys, hybridUser), "lacks the header"},
		{"legacy header plus a hybrid header", legacy.hunterV7PromptContractError(legacySys, legacyUser+"\npayload_version=hybrid_v2 | x"), "carries payload_version=hybrid_v2"},
	}
	for _, c := range cases {
		if c.err == nil || !strings.Contains(c.err.Error(), c.wantContains) {
			t.Errorf("%s: got %v, want an error containing %q", c.name, c.err, c.wantContains)
		}
	}
	// A user prompt without the tier section (no candidates) needs no header.
	if err := legacy.hunterV7PromptContractError(legacySys, "no candidates this round"); err != nil {
		t.Fatalf("no tier section: %v", err)
	}
}

// Strategy-07 cites legacy-only doctrine content (llm_action, §3 剧本):
// it passes as legacy and fails closed with hybrid_v2. Every shipped
// template passes as it is stored.
func TestS2TemplatePairing(t *testing.T) {
	cfg := loadStrategyTemplate(t, strategyTemplateRedesigned)
	if refs := HunterV7PromptSectionsLegacyOnlyRefs(cfg); refs != nil {
		t.Fatalf("legacy strategy-07: %v", refs)
	}
	sys, user := s2Render(cfg)
	if err := NewStrategyEngine(cfg).hunterV7PromptContractError(sys, user); err != nil {
		t.Fatalf("strategy-07 as stored: %v", err)
	}
	cfg.CoinSource.Hunter.V7Prompt = &store.V7PromptConfig{PayloadVersion: store.V7PayloadVersionHybridV2}
	refs := HunterV7PromptSectionsLegacyOnlyRefs(cfg)
	if len(refs) == 0 || !strings.Contains(strings.Join(refs, " "), "llm_action") {
		t.Fatalf("strategy-07 + hybrid_v2 must report its legacy-only references, got %v", refs)
	}
	sys, user = s2Render(cfg)
	if err := NewStrategyEngine(cfg).hunterV7PromptContractError(sys, user); err == nil || !strings.Contains(err.Error(), "legacy-only doctrine content") {
		t.Fatalf("strategy-07 + hybrid_v2 must fail the contract, got %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(strategyTemplateDir, "*.json"))
	if len(files) == 0 {
		t.Fatal("no templates found")
	}
	for _, f := range files {
		tpl := loadStrategyTemplate(t, filepath.Base(f))
		if refs := HunterV7PromptSectionsLegacyOnlyRefs(tpl); refs != nil {
			t.Errorf("%s: hybrid_v2 with legacy-only references %v", filepath.Base(f), refs)
		}
		if err := store.ValidateTradingConfig(tpl); err != nil && strings.Contains(err.Error(), "payload_version") {
			t.Errorf("%s: %v", filepath.Base(f), err)
		}
	}
}

// The headings the contract looks for are the doctrines' own headings.
func TestS2DoctrineHeadingsMatchTheDoctrines(t *testing.T) {
	in := NewStrategyEngine(u11FixtureConfig("en")).hunterV7DoctrineInputs()
	opts := NewStrategyEngine(u11HybridConfig("en")).hunterV7DoctrineV2Options()
	got := map[string][]string{
		store.V7PayloadVersionLegacy:   {hunterV7DoctrinePrompt(LangEnglish, in), hunterV7DoctrinePrompt(LangChinese, in)},
		store.V7PayloadVersionHybridV2: {hunterV7DoctrineV2Prompt(LangEnglish, opts), hunterV7DoctrineV2Prompt(LangChinese, opts)},
	}
	for version, docs := range got {
		for i, d := range docs {
			if !strings.HasPrefix(strings.TrimSpace(d), hunterV7DoctrineHeadings[version][i]) {
				t.Errorf("%s doctrine %d does not start with %q", version, i, hunterV7DoctrineHeadings[version][i])
			}
		}
	}
	for v, d := range hunterV7KnownPromptPairs {
		if pc := store.EffectiveV7Prompt(&store.V7PromptConfig{PayloadVersion: v}); pc.DoctrineVersion() != d {
			t.Errorf("pair %s/%s does not match the store (%s)", v, d, pc.DoctrineVersion())
		}
	}
}

// s2NoCallClient fails the test if the AI is called.
type s2NoCallClient struct{ t *testing.T }

func (c s2NoCallClient) fail() (string, error) {
	c.t.Fatal("the AI must not be called when the prompt contract fails")
	return "", nil
}
func (c s2NoCallClient) SetAPIKey(string, string, string)                {}
func (c s2NoCallClient) SetTimeout(time.Duration)                        {}
func (c s2NoCallClient) CallWithMessages(string, string) (string, error) { return c.fail() }
func (c s2NoCallClient) CallWithRequest(*mcp.Request) (string, error)    { return c.fail() }
func (c s2NoCallClient) CallWithRequestFull(*mcp.Request) (*mcp.LLMResponse, error) {
	_, err := c.fail()
	return nil, err
}
func (c s2NoCallClient) CallWithRequestStream(*mcp.Request, func(string)) (string, error) {
	return c.fail()
}

func s2Client(t *testing.T) mcp.AIClient { return s2NoCallClient{t: t} }

// End to end: a stored unknown version stops the round before the AI call
// with one wait / backend_guard_risk, and no error (not an AI failure).
func TestS2GetFullDecisionFailsClosedWithoutCallingAI(t *testing.T) {
	cfg := u11FixtureConfig("en")
	cfg.CoinSource.Hunter.V7Prompt = &store.V7PromptConfig{PayloadVersion: "hybrid_v3"}
	e := NewStrategyEngine(cfg)
	ctx := u11FixtureContext(e)
	ctx.OITopDataMap = map[string]*OITopData{}
	d, err := GetFullDecisionWithStrategy(ctx, s2Client(t), e, "")
	if err != nil {
		t.Fatalf("contract failure must not be an error: %v", err)
	}
	if d == nil || len(d.Decisions) != 1 {
		t.Fatalf("decision %+v", d)
	}
	got := d.Decisions[0]
	if got.Symbol != "ALL" || got.Action != "wait" || got.BlockedReasonCode != "backend_guard_risk" || !strings.HasPrefix(got.Reasoning, hunterV7PromptContractMismatch+": ") {
		t.Fatalf("wait %+v", got)
	}
	if d.RawResponse != "" || d.SystemPrompt == "" || d.UserPrompt == "" {
		t.Fatalf("prompts must be kept for audit and no AI response recorded: raw=%q", d.RawResponse)
	}
}

// Persisted records carry the payload version next to the doctrine version.
func TestS2PayloadVersionTag(t *testing.T) {
	sigs := []local.V7SignalOutput{t2TagSignal("ignition_early_long", local.V7DirLong, "TRIGGERED")}
	cands := AssembleHunterV7CandidateCoins(sigs, "BOTH", defaultGeometry())
	for _, c := range []struct {
		opts HunterV7RecordTagOptions
		want string
	}{
		{DefaultHunterV7RecordTagOptions(), store.V7PayloadVersionLegacy},
		{HunterV7RecordTagOptionsFromConfig(u11FixtureConfig("en")), store.V7PayloadVersionLegacy},
		{HunterV7RecordTagOptionsFromConfig(u11HybridConfig("en")), store.V7PayloadVersionHybridV2},
	} {
		recs := BuildHunterV7SignalRecordsTagged(sigs, cands, c.opts)
		if recs[0].Tags.PayloadVersion != c.want {
			t.Errorf("payload_version = %q, want %q", recs[0].Tags.PayloadVersion, c.want)
		}
	}
}
