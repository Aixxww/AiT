package kernel

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/store"
)

// strategy-08: the family-checklist successor of strategy-07. It binds the
// hybrid_v2 payload/doctrine through coin_source.hunter_config.v7_prompt,
// carries the seven setup-family evidence checklists in entry_standards
// (hybrid-visible fields only), reads conditional-entry candidates in
// decision_process steps 3-4, and treats funding_reversal as CONTEXT
// background. Unlike strategy-07 it must pass the hybrid_v2 side of every
// pairing check: no legacy-only reference in its own prompt text, and its
// rendered system prompts carry the hybrid_v2 doctrine, not the legacy one.

const strategyTemplateFamilyChecklists = "strategy-08-hunter-v7-family-checklists.json"

func TestStrategy08TemplateHybridBinding(t *testing.T) {
	cfg := loadStrategyTemplate(t, strategyTemplateFamilyChecklists)
	h := cfg.CoinSource.Hunter
	if h == nil || h.V7Prompt == nil {
		t.Fatal("strategy-08 must carry coin_source.hunter_config.v7_prompt")
	}
	if got := store.EffectiveV7Prompt(h.V7Prompt); !got.HybridV2() {
		t.Fatalf("strategy-08 effective payload version = %q, want hybrid_v2", got.PayloadVersion)
	}
	if refs := HunterV7PromptSectionsLegacyOnlyRefs(cfg); refs != nil {
		t.Fatalf("strategy-08 cites legacy-only content %v", refs)
	}
	if err := store.ValidateTradingConfig(cfg); err != nil {
		t.Fatalf("strategy-08 must validate as stored: %v", err)
	}
}

func TestStrategy08TemplateSections(t *testing.T) {
	cfg := loadStrategyTemplate(t, strategyTemplateFamilyChecklists)
	ps := cfg.PromptSections

	es := ps.EntryStandards
	assertContains173(t, "strategy-08 entry_standards", es,
		"# 开仓标准：形态族证据清单（hybrid_v2）",
		"## 族 1 趋势动量延续",
		"## 族 2 压缩与触发突破",
		"## 族 3 挤压与拥挤反转",
		"## 族 4 恐慌、派发与弱势反转",
		"## 族 5 破位与位移",
		"## 族 6 区间、底部唤醒与扩张事件",
		"## 族 7 资金流背离",
		"whale_flow_reversal",
		// funding_reversal is background-only (CONTEXT), never an entry list.
		"funding_reversal 为 CONTEXT 类",
		"禁止据此开仓",
		// The conditional-entry supplement (draft section B).
		"## 条件入场候选的审验口径（补充）",
		"execution.entry_permission=conditional_on_trigger",
		"从 trigger 起算的 RR",
		// Falsifiers keep their blocked_reason_codes.
		"oi_too_low",
		"funding_crowded",
		"rr_insufficient",
		"entry_not_in_zone",
	)

	dp := ps.DecisionProcess
	assertContains173(t, "strategy-08 decision_process", dp,
		// Step 3: the two-read entry geometry of draft section B.
		"入场几何分两读",
		"signal.trigger_price 存在且位于 entry zone 内",
		"(i)–(iii) 不成立 → entry_not_in_zone",
		// Step 4: pre-trigger confirmations now, trigger-action
		// confirmations at the backend; the passthrough rule still applies.
		"透传裁决照教义 §3 适用",
		"触发前结构值不值得预先批准",
		"price 设为 signal.trigger_price 并带 trigger 对象",
	)

	// No legacy-only field or mechanism survives in the strategy's own
	// text (the save-time validator's token set, plus the legacy hard
	// boundary list and the candidate priority table strategy-07 carried).
	own := strings.Join([]string{ps.RoleDefinition, ps.TradingFrequency, es, dp, cfg.CustomPrompt}, "\n")
	assertNotContains173(t, "strategy-08 own text", own,
		"liquidity_score", "risk_score", "execution_quality",
		"llm_action", "execution_contract", "confirmation_summary",
		"tag_semantics", "suggested_trigger", "hunter_v7_signal_json",
		"trigger_memory", "候选优先级",
	)
	for _, f := range store.V7LegacyOnlyPromptFields {
		assertNotContains173(t, "strategy-08 own text", own, f)
	}
}

func TestStrategy08TemplateRenderedPrompts(t *testing.T) {
	cfg := loadStrategyTemplate(t, strategyTemplateFamilyChecklists)

	sysZH := renderTemplateSystemPrompt(cfg, "zh")
	assertContains173(t, "strategy-08 zh system prompt", sysZH,
		"# Hunter v7 策略执行框架（混合制 hybrid_v2）",
		"透传候选（tier_reason=confirmation_passthrough_<code>）的裁决点就是 <code> 那一项",
		"## 族 7 资金流背离",
	)
	assertNotContains173(t, "strategy-08 zh system prompt", sysZH,
		"# Hunter v7 策略执行框架（五段）",
	)

	sysEN := renderTemplateSystemPrompt(cfg, "en")
	assertContains173(t, "strategy-08 en system prompt", sysEN,
		"# Hunter v7 Strategy Execution Framework (hybrid_v2)",
		"the decision point is that one item, <code>",
	)
	assertNotContains173(t, "strategy-08 en system prompt", sysEN,
		"# Hunter v7 Strategy Execution Framework (Five Segments)",
	)
}

// Evidence-weight rework (owner decision 2026-11-28, after the replay
// evaluation showed family-checklist magnitudes acting as a second hard
// gate behind the #173 passthrough adjudication): checklist magnitudes
// are reference magnitudes that weigh the evidence, never admission
// gates. The numeric reference values stay; only their normative force
// changes. The conditional-entry reading (steps 3-4) and the risk
// controls are untouched by the rework.
func TestStrategy08TemplateEvidenceWeights(t *testing.T) {
	cfg := loadStrategyTemplate(t, strategyTemplateFamilyChecklists)
	ps := cfg.PromptSections
	es, dp := ps.EntryStandards, ps.DecisionProcess

	// (a) The evidence-weight rule is stated where the checklists are
	// introduced and mirrored in decision_process step 2, the step that
	// consumes the checklists.
	assertContains173(t, "strategy-08 entry_standards weight rule", es,
		"均为参考量级，是证据权重（evidence weights），不是准入门槛",
		"Checklist magnitudes are evidence weights, not admission gates: no candidate may be rejected solely because one magnitude falls below its reference value; rejection must cite the totality of the evidence.",
		"凡以量级不足为由 wait，理由中必须点名该量级短板，并说明其余证据为何不足以补偿",
		"按量级权重口径综合判断，不作单独证伪",
	)
	assertContains173(t, "strategy-08 decision_process weight rule", dp,
		"清单中的量级是证据权重、不是准入门槛（checklist magnitudes are evidence weights, not admission gates）",
		"不得仅因单个量级未达参考值就否决候选",
		"以量级不足为由 wait 时，必须点名该量级并说明其余证据为何不足以补偿",
	)
	sysZH := renderTemplateSystemPrompt(cfg, "zh")
	assertContains173(t, "strategy-08 zh system prompt weight rule", sysZH,
		"Checklist magnitudes are evidence weights, not admission gates",
	)

	// (b) No hard-gate phrasing for magnitudes survives in the checklist
	// sections: neither the old machine-gate framing, nor requirement
	// verbs on a magnitude, nor a magnitude mapped straight to a
	// blocked_reason_code.
	assertNotContains173(t, "strategy-08 entry_standards hard gates", es,
		"数值均为与机器门同数量级的量级读数",
		"volatility_squeeze 要求 oi_change_last_1h_pct",
		"起为有效冲洗",
		"OI 增长达标",
		"short_squeeze 的 24h 涨幅未达 +25% 量级透支区",
		"accumulation 型 24h 涨幅已 > +8% 量级 → confirmation_missing",
		"或 1h 涨跌幅进入极端区 → confirmation_missing",
		"mms_squeeze_engine 型 rsi14(1h) ≥ 82 量级或 24h 涨幅 > +35% 量级 → confirmation_missing",
		"panic 24h 跌幅深于 −45% 量级（崩盘而非投降）→ confirmation_missing",
		"breakdown 1h 跌幅深于 −12% 量级（已耗尽）→ confirmation_missing",
		"displacement 1h 涨幅 > +12% 量级，或现价远离 15m ema20 进入追价区且无回踩 → confirmation_missing",
		"displacement funding > +0.1%/结算 → funding_crowded",
		"range_reversion OI 单向爆量（|oi_change_last_1h_pct| > 10% 量级）→ confirmation_missing",
		"mms_bottom_wake 4h 收盘涨幅 ≥ +6% 或跌幅 ≤ −2.5% 量级 → confirmation_missing",
		"mms_bottom_wake 4h OI 量级超出 +45% → confirmation_missing",
		"oi_change_last_1h_pct < +5% 量级 → oi_too_low",
		"|funding| 超出中性区 → funding_crowded",
	)

	// The numeric reference values themselves are preserved — only
	// their normative force changed.
	assertContains173(t, "strategy-08 entry_standards reference values kept", es,
		"−15%~−45%", "−12%", "+8%", "+25%", "+35%", "−12%",
		"+0.1%/结算", "10% 量级", "+45%", "+5% 量级",
	)

	// (c) risk_control still deep-equals strategy-07's, value for value.
	cfg07 := loadStrategyTemplate(t, strategyTemplateRedesigned)
	if !reflect.DeepEqual(cfg.RiskControl, cfg07.RiskControl) {
		t.Fatalf("strategy-08 risk_control drifted from strategy-07:\n08: %+v\n07: %+v", cfg.RiskControl, cfg07.RiskControl)
	}
}
