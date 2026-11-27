package kernel

import (
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
