package kernel

// #173 acceptance tests: the prompt layer explains the #161
// confirmation_passthrough_<code> flag. Before #173 the flag reached the
// assembled prompt as tier_reason but no doctrine text said what it
// means, and strategy-07's step 4 read literally forced its rejection.
//
// The rule lives in BOTH doctrines, each in its own vocabulary: doctrine
// v2 (§2/§3, hybrid_v2 strategies) and the legacy five-segment doctrine
// (S2/S3). strategy-07 binds the LEGACY doctrine (it carries no
// v7_prompt block), so a pointer from its prompt_sections only resolves
// if the legacy text carries the rule — the template test below renders
// the assembled system prompt and checks exactly that.

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// section173 returns doc's text from the start marker up to (not
// including) the end marker, so assertions prove placement inside one
// named section instead of counting occurrences across the whole text.
func section173(t *testing.T, doc, start, end string) string {
	t.Helper()
	i := strings.Index(doc, start)
	if i < 0 {
		t.Fatalf("section start %q not found", start)
	}
	rest := doc[i:]
	j := strings.Index(rest[len(start):], end)
	if j < 0 {
		t.Fatalf("section end %q not found after %q", end, start)
	}
	return rest[:len(start)+j]
}

func assertContains173(t *testing.T, where, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("%s missing %q", where, want)
		}
	}
}

func assertNotContains173(t *testing.T, where, text string, bads ...string) {
	t.Helper()
	for _, bad := range bads {
		if strings.Contains(text, bad) {
			t.Errorf("%s must not contain %q", where, bad)
		}
	}
}

// (a) Doctrine v2, both languages, asserted per section: §2 defines the
// passthrough form of tier_reason (exactly one machine-verifiable item
// failed; the code appears in an expanded candidate's
// required_confirmations; checklist-only items are never machine
// checked) and §3 carries the adjudication rule (the named code is the
// decision point; re-verify from closed bars + market fields; the
// adjudication decides only that item — every remaining confirmation
// still verified; an adjudicated hold counts as visible for the §4 high
// band; the backend refresh re-verifies before execution; failed or
// unverifiable re-verification waits with confirmation_missing naming
// the code; no other standard lowered).
func TestPassthrough173DoctrineV2ExplainsFlag(t *testing.T) {
	o := hunterV7DoctrineV2Options{BarsPerTimeframe: 40, Timeframes: []string{"5m", "15m", "1h", "4h"}}

	zh := hunterV7DoctrineV2Prompt(LangChinese, o)
	zhS2 := section173(t, zh, "## §2 数据契约", "## §3 决策漏斗")
	assertContains173(t, "v2 zh §2", zhS2,
		"confirmation_passthrough_<code>",
		"恰有一项未过",
		"在展开候选的附录里，它必出现在 signal.required_confirmations 清单中",
		"其他机器可核验项都已通过机器核验",
		"纯清单项从不经机器核验",
	)
	zhS3 := section173(t, zh, "## §3 决策漏斗", "## §4 风险与几何")
	assertContains173(t, "v2 zh §3", zhS3,
		"裁决点就是 <code> 那一项",
		"用已收盘 K 线与 market 字段专门复核它",
		"仅在其余规则全部通过时才开仓",
		"以 confirmation_missing 为 blocked_reason_code、在 reasoning 中点名该 code",
		"signal.required_confirmations 的其余各项（包括从不经机器核验的纯清单项）仍须按本节通则逐条在 K 线上核验",
		"在 §4 高仓位档的「逐条可见」条件上视同可见",
		"在执行前由后端刷新再次复核，与裁决结果无关",
		"其余标准（execution.limits 校验、RR、高周期规则、仓位档）一律不降",
	)
	assertNotContains173(t, "v2 zh §3", zhS3, "硬边界")

	en := hunterV7DoctrineV2Prompt(LangEnglish, o)
	enS2 := section173(t, en, "## S2 Data Contract", "## S3 Funnel")
	assertContains173(t, "v2 en S2", enS2,
		"confirmation_passthrough_<code>",
		"exactly one of the candidate's machine-verifiable must-check confirmations failed",
		"in an expanded candidate's appendix it always appears in signal.required_confirmations",
		"The other machine-verifiable items on that list passed the machine check",
		"checklist-only items are never machine-checked",
	)
	assertNotContains173(t, "v2 en S2", enS2, "the machine-verifiable item of that list")
	enS3 := section173(t, en, "## S3 Funnel", "## S4 Risk And Geometry")
	assertContains173(t, "v2 en S3", enS3,
		"the decision point is that one item, <code>",
		"re-verify it specifically against the closed bars and the market fields",
		"you may continue the audit under the remaining rules, and open only if the remaining rules pass",
		"wait with blocked_reason_code confirmation_missing and name the code in reasoning",
		"every remaining item of signal.required_confirmations (including checklist-only items the machine never checks) must still be verified one by one on the bars",
		"counts as visible for the S4 high-band condition",
		"re-verified by the backend refresh before execution, whatever the adjudication",
		"No other standard (the execution.limits checks, RR, the HTF rule, sizing bands) is lowered",
	)
	assertNotContains173(t, "v2 en S3", enS3,
		"hard limits",
		"continue the audit under the remaining rules and open;", // pre-fix imperative
	)
}

// (a') Legacy doctrine, both languages, asserted per section: S2
// defines the passthrough form against the legacy payload (the code is
// the one listed in confirmation_summary.missing_review) and S3 carries
// the adjudication rule in legacy evidence vocabulary (closed bars +
// price_context / derivatives_context fields). The legacy sizing gate
// is stated explicitly: an adjudicated hold satisfies the must_confirm
// visibility check, while S4's passed_review and release-grade
// requirements stand.
func TestPassthrough173LegacyDoctrineExplainsFlag(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	in := NewStrategyEngine(&cfg).hunterV7DoctrineInputs()

	zh := hunterV7DoctrinePrompt(LangChinese, in)
	zhS2 := section173(t, zh, "## §2 数据契约", "## §3 决策漏斗与形态族剧本")
	assertContains173(t, "legacy zh §2", zhS2,
		"tier_reason=confirmation_passthrough_<code>",
		"只差一项机器可核验的必核确认未过",
		"与 confirmation_summary.missing_review 里列出的代码相同",
		"机器清单判它缺失，不等于它在 K 线上不成立",
		"按 §3 透传裁决规则复核裁决",
	)
	zhS3 := section173(t, zh, "## §3 决策漏斗与形态族剧本", "## §4 风险与仓位")
	assertContains173(t, "legacy zh §3", zhS3,
		"透传裁决（tier_reason=confirmation_passthrough_<code>）",
		"裁决点就是 <code> 对应的那一项",
		"用已收盘 K 线与 price_context / derivatives_context 字段专门复核它",
		"仅在其余规则全部通过时才开仓",
		"以 confirmation_missing 为 blocked_reason_code、在 reasoning 中点名该 code",
		"must_confirm / required_confirmations 的其余各项（包括从未经机器核验的清单项）仍须按本节通则逐条核验",
		"在 must_confirm 逐条可见这一条上视同已满足",
		"§4 对 passed_review 与放行档位的要求不因此改变",
		"在执行前由后端刷新再次复核，与裁决结果无关",
	)

	en := hunterV7DoctrinePrompt(LangEnglish, in)
	enS2 := section173(t, en, "## S2 Data Contract", "## S3 Funnel And Family Playbooks")
	assertContains173(t, "legacy en S2", enS2,
		"tier_reason=confirmation_passthrough_<code>",
		"missed exactly one machine-verifiable must-check confirmation",
		"the same code listed in confirmation_summary.missing_review",
		"The machine checklist marking it missing does not mean it fails on the bars",
		"adjudicated under the S3 pass-through rule",
	)
	enS3 := section173(t, en, "## S3 Funnel And Family Playbooks", "## S4 Risk And Sizing")
	assertContains173(t, "legacy en S3", enS3,
		"Pass-through adjudication (tier_reason=confirmation_passthrough_<code>)",
		"the decision point is the one item <code> names",
		"against the closed bars and the price_context / derivatives_context fields",
		"open only if the remaining rules pass",
		"wait with blocked_reason_code confirmation_missing and name the code in reasoning",
		"every other item of must_confirm / required_confirmations (including checklist-only items the machine never checks) must still be verified one by one",
		"counts as visibly satisfied for the must_confirm check",
		"the S4 requirements on passed_review and release grade do not change",
		"re-verified by the backend refresh before execution, whatever the adjudication",
	)
}

// (b) Legacy assembled prompt: a candidate the #161 pass-through
// promotes shows the flag in the rendered prompt, next to the
// required_confirmations list and the confirmation summary that name
// the same code — so the doctrine text above has something visible to
// attach to. The coin is the #161 fixture plus the invalidation/target
// the prompt-time RR geometry needs to be computable (mirrors
// withNeutralRRGeometry).
func TestPassthrough173AssembledPromptShowsFlag(t *testing.T) {
	engine := NewStrategyEngine(&store.StrategyConfig{Language: "en", CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"}})
	coin := passthrough161Coin(
		withInvalidation(0.00305*0.978),
		withTargets(local.V7Target{Price: 0.00305 * 1.10}),
	)
	ctx := &Context{
		CandidateCoins: []CandidateCoin{coin},
		MarketDataMap: map[string]*market.Data{
			"PASS161USDT": hunterV7PromptTestMarketData("PASS161USDT", 0.00305),
		},
	}
	prompt := engine.BuildUserPrompt(ctx)
	for _, want := range []string{
		"execution_tier=REVIEWABLE tier_reason=confirmation_passthrough_taker_flow_confirms_long",
		`"required_confirmations":["taker_flow_confirms_long"]`,
		`"missing_review":[{"code":"taker_flow_confirms_long"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("assembled prompt missing %q\n%s", want, prompt)
		}
	}
}

// (b') hybrid_v2 assembled prompt: the same passthrough candidate,
// rendered through the hybrid_v2 path. The v2 payload carries the flag
// in execution.tier_reason and the contested code in
// signal.required_confirmations (confirmation_summary is stripped in
// hybrid_v2), and the system prompt carries the doctrine v2 rule the
// flag attaches to.
func TestPassthrough173HybridV2PromptShowsFlagAndRule(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.Hunter = &store.HunterConfig{
		V7Prompt: &store.V7PromptConfig{PayloadVersion: store.V7PayloadVersionHybridV2},
	}
	cfg.CustomPrompt = ""
	engine := NewStrategyEngine(&cfg)
	coin := passthrough161Coin(
		withInvalidation(0.00305*0.978),
		withTargets(local.V7Target{Price: 0.00305 * 1.10}),
	)
	ctx := &Context{
		PromptTime:     u11PromptTime,
		CandidateCoins: []CandidateCoin{coin},
		MarketDataMap: map[string]*market.Data{
			"PASS161USDT": u11MarketData("PASS161USDT", 0.00305, 0.3),
		},
	}
	user := engine.BuildUserPrompt(ctx)
	for _, want := range []string{
		`"tier_reason":"confirmation_passthrough_taker_flow_confirms_long"`,
		`"required_confirmations":["taker_flow_confirms_long"]`,
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("hybrid_v2 user prompt missing %q\n%s", want, user)
		}
	}
	if strings.Contains(user, "missing_review") {
		t.Fatalf("hybrid_v2 user prompt must strip confirmation_summary:\n%s", user)
	}
	system := engine.BuildSystemPrompt(1000, "")
	if !strings.Contains(system, "the decision point is that one item, <code>") {
		t.Fatalf("hybrid_v2 system prompt does not carry the doctrine v2 adjudication rule")
	}
}

// (c) strategy-07 template: role_definition assigns the adjudication
// duty and defers evidence specifics to the framework S3 rule;
// decision_process step 4 routes only the <code> item to that rule and
// keeps every other confirmation blocking at the step. The template
// binds the legacy doctrine, so the rendered system prompts (both
// languages) must actually contain the legacy rule the template points
// at — a dangling cross-reference fails here.
func TestPassthrough173Strategy07Template(t *testing.T) {
	cfg := loadStrategyTemplate(t, strategyTemplateRedesigned)

	role := cfg.PromptSections.RoleDefinition
	assertContains173(t, "strategy-07 role_definition", role,
		"confirmation_passthrough_<code>",
		"裁决 <code> 对应的那一项确认是否实际成立",
		"证据来源与裁决口径以文末框架 §3 透传裁决规则为准",
	)
	dp := cfg.PromptSections.DecisionProcess
	assertContains173(t, "strategy-07 decision_process", dp,
		"<code> 对应的那一项不在本步直接阻断",
		"其余各项在本步仍逐条必核、缺失即按本步阻断",
		"仍以 confirmation_missing 在本步阻断并在 reasoning 中点名该 code",
	)
	assertNotContains173(t, "strategy-07 sections", role+dp,
		"唯一所缺",   // false premise: other items can also be missing
		"market 字段", // no market namespace exists in the legacy payload
	)

	sysZH := renderTemplateSystemPrompt(cfg, "zh")
	assertContains173(t, "strategy-07 zh system prompt", sysZH,
		"透传裁决（tier_reason=confirmation_passthrough_<code>）",
	)
	sysEN := renderTemplateSystemPrompt(cfg, "en")
	assertContains173(t, "strategy-07 en system prompt", sysEN,
		"Pass-through adjudication (tier_reason=confirmation_passthrough_<code>)",
	)
}
