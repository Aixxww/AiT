package kernel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/store"
)

// strategyTemplateDir holds the exported strategy JSONs used as prompt design
// inputs (issue #67).
var strategyTemplateDir = filepath.Join("..", "docs", "strategy-prompt-templates")

const (
	strategyTemplateCurrent    = "strategy-06-hunter-v7-current.json"
	strategyTemplateRedesigned = "strategy-07-hunter-v7-redesigned.json"
)

func loadStrategyTemplate(t *testing.T, name string) *store.StrategyConfig {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(strategyTemplateDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var export struct {
		Config store.StrategyConfig `json:"config"`
	}
	if err := json.Unmarshal(raw, &export); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return &export.Config
}

func renderTemplateSystemPrompt(cfg *store.StrategyConfig, lang string) string {
	c := *cfg
	c.Language = lang
	return NewStrategyEngine(&c).BuildSystemPrompt(1000, "")
}

// TestStrategyTemplateSystemPromptDump writes the rendered system prompts to
// PROMPT_DUMP_DIR when set (design aid for REDESIGN-NOTES, no assertions).
func TestStrategyTemplateSystemPromptDump(t *testing.T) {
	dir := os.Getenv("PROMPT_DUMP_DIR")
	if dir == "" {
		t.Skip("PROMPT_DUMP_DIR not set")
	}
	for _, name := range []string{strategyTemplateCurrent, strategyTemplateRedesigned} {
		cfg := loadStrategyTemplate(t, name)
		for _, lang := range []string{"zh", "en"} {
			p := renderTemplateSystemPrompt(cfg, lang)
			out := filepath.Join(dir, name+"."+lang+".txt")
			if err := os.WriteFile(out, []byte(p), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("%s %s: %d bytes (~%d tokens at chars/4)", name, lang, len(p), len(p)/4)
		}
	}
}

// The redesign only changes prompt text: every config value of strategy-07
// equals strategy-06.
func TestRedesignedTemplateChangesPromptTextOnly(t *testing.T) {
	cur := *loadStrategyTemplate(t, strategyTemplateCurrent)
	red := *loadStrategyTemplate(t, strategyTemplateRedesigned)
	cur.PromptSections, red.PromptSections = store.PromptSectionsConfig{}, store.PromptSectionsConfig{}
	cur.CustomPrompt, red.CustomPrompt = "", ""
	a, _ := json.Marshal(cur)
	b, _ := json.Marshal(red)
	if string(a) != string(b) {
		t.Fatalf("strategy-07 changes config beyond prompt_sections/custom_prompt:\n06: %s\n07: %s", a, b)
	}
}

// The redesigned system prompt stays within the current budget.
func TestRedesignedTemplateSystemPromptBudget(t *testing.T) {
	cur := loadStrategyTemplate(t, strategyTemplateCurrent)
	red := loadStrategyTemplate(t, strategyTemplateRedesigned)
	for _, lang := range []string{"zh", "en"} {
		before, after := len(renderTemplateSystemPrompt(cur, lang)), len(renderTemplateSystemPrompt(red, lang))
		t.Logf("%s system prompt: %d -> %d bytes", lang, before, after)
		if after > before {
			t.Errorf("%s: redesigned system prompt grew %d -> %d bytes", lang, before, after)
		}
	}
}

// hunterV7PromptSetupFamilies is the brief's canonical setup_type list
// (doctrine S3 families); the prompt may name no other setup_type.
var hunterV7PromptSetupFamilies = []string{
	"leader_momentum_long", "displacement_momentum_long", "mms_trend_ride_long", "mms_squeeze_engine_long", "intraday_scalp_long",
	"trend_breakout_long", "accumulation_breakout_long", "volatility_squeeze_breakout", "pre_breakout_watch", "pre_squeeze_watch",
	"alt_ladder_momentum_long", "alt_ladder_breakdown_short",
	"panic_reversal_long", "pullback_reversal_long", "whale_flow_reversal", "funding_reversal", "mms_bottom_wake_long", "accumulation_watch",
	"relative_weakness_short", "distribution_short", "breakdown_momentum_short", "long_squeeze_short", "range_reversion", "range_expansion_event", "pre_distribution_watch",
}

// Template vocabulary that contradicts the v7 ground truth (issue #67 brief,
// section 4): foreign regime labels, foreign setup names, foreign action
// enums and fixed minute cadences.
var hunterV7ForbiddenPromptTerms = []string{
	"short_squeeze_long", "module_no_match",
	"CHAOTIC", "TRANSITION", "trend_continuation",
	"Long Squeeze", "Short Squeeze", "Liquidation Wick",
	"OPEN_LONG", "OPEN_SHORT", "CLOSE_POSITION",
	"5-minute", "15-minute", "5-Min", "every 5 minutes", "every 15 minutes", "每 5 分钟", "每5分钟", "每15分钟", "分钟循环", "trades/day", "每天2-4笔",
}

func TestRedesignedSystemPromptVocabulary(t *testing.T) {
	red := loadStrategyTemplate(t, strategyTemplateRedesigned)
	for _, lang := range []string{"zh", "en"} {
		p := renderTemplateSystemPrompt(red, lang)
		for _, term := range hunterV7ForbiddenPromptTerms {
			if strings.Contains(p, term) {
				t.Errorf("%s: system prompt contains %q", lang, term)
			}
		}
		for _, setup := range hunterV7PromptSetupFamilies {
			if !strings.Contains(p, setup) {
				t.Errorf("%s: system prompt does not name setup family member %q", lang, setup)
			}
		}
		for _, code := range []string{"entry_not_in_zone", "rr_insufficient", "confirmation_missing", "oi_too_low", "funding_crowded", "account_risk", "backend_guard_risk", "no_reviewable_candidate"} {
			if !strings.Contains(p, code) {
				t.Errorf("%s: system prompt does not name blocked_reason_code %q", lang, code)
			}
		}
	}
}

var doctrineTokenRe = regexp.MustCompile(`[a-z][a-z0-9]*(?:_[a-z0-9*]+)+|\d+(?:\.\d+)?`)

func doctrineTokens(s string) []string {
	set := map[string]bool{}
	for _, tok := range doctrineTokenRe.FindAllString(s, -1) {
		set[tok] = true
	}
	out := make([]string, 0, len(set))
	for tok := range set {
		out = append(out, tok)
	}
	sort.Strings(out)
	return out
}

// ZH and EN doctrine are semantic equals: they name the same identifiers and
// the same numbers.
func TestHunterV7DoctrineLanguageParity(t *testing.T) {
	e := NewStrategyEngine(loadStrategyTemplate(t, strategyTemplateRedesigned))
	in := e.hunterV7DoctrineInputs()
	zh, en := doctrineTokens(hunterV7DoctrinePrompt(LangChinese, in)), doctrineTokens(hunterV7DoctrinePrompt(LangEnglish, in))
	if strings.Join(zh, " ") != strings.Join(en, " ") {
		zs, es := map[string]bool{}, map[string]bool{}
		for _, t := range zh {
			zs[t] = true
		}
		for _, t := range en {
			es[t] = true
		}
		var onlyZH, onlyEN []string
		for _, t := range zh {
			if !es[t] {
				onlyZH = append(onlyZH, t)
			}
		}
		for _, t := range en {
			if !zs[t] {
				onlyEN = append(onlyEN, t)
			}
		}
		t.Fatalf("doctrine ZH/EN token sets differ:\n only ZH: %v\n only EN: %v", onlyZH, onlyEN)
	}
}

// The doctrine's position-protection ladder is rendered from the protector
// config the backend uses, not from literals.
func TestHunterV7DoctrineProtectionLadderFollowsConfig(t *testing.T) {
	cfg := loadStrategyTemplate(t, strategyTemplateRedesigned)
	def := NewStrategyEngine(cfg).hunterV7DoctrineInputs()
	en := hunterV7DoctrinePrompt(LangEnglish, def)
	for _, want := range []string{"Peak >= 5% ROE", "Peak >= 10% ROE with ~50% giveback", "Peak >= 15% ROE with ~45% giveback", "PnL >= 8% ROE", "PnL <= -8% ROE inside 15 minutes", "<= -12% ROE anytime", "Peak < 5.7% ROE or raw_move < 1% price move"} {
		if !strings.Contains(en, want) {
			t.Errorf("default ladder missing %q", want)
		}
	}

	c := *cfg
	c.RiskControl.Protector = &store.ProtectorConfig{
		HardLoss:                  store.ProtectorLevel{Value: -1.5, Unit: store.ProtectorUnitR},
		ProfitLockHighDrawdownPct: 40,
	}
	custom := hunterV7DoctrinePrompt(LangEnglish, NewStrategyEngine(&c).hunterV7DoctrineInputs())
	if !strings.Contains(custom, "<= -1.5R anytime") || !strings.Contains(custom, "~40% giveback") {
		t.Fatalf("doctrine does not follow the configured protector levels:\n%s", custom)
	}
}
