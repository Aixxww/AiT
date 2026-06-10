package local

import "testing"

func TestDescribeHunterV7TagsAddsWaitOnlyAndUnknownContextSemantics(t *testing.T) {
	defs := DescribeHunterV7Tags(
		[]string{"strong_reclaim", "new_unclassified_reason"},
		[]string{"do_not_open_until_confirmed", "funding_extreme"},
		[]string{"taker_buy_15m_gt_0_52"},
	)

	if !hasTagAction(defs, "strong_reclaim", V7TagActionOpenSupport) {
		t.Fatalf("missing strong_reclaim open-support semantics: %+v", defs)
	}
	if !hasTagAction(defs, "do_not_open_until_confirmed", V7TagActionWaitOnly) {
		t.Fatalf("missing do_not_open_until_confirmed wait-only semantics: %+v", defs)
	}
	if !hasTagAction(defs, "funding_extreme", V7TagActionReduceOrWait) {
		t.Fatalf("missing funding_extreme reduce-or-wait semantics: %+v", defs)
	}
	if !hasTagAction(defs, "taker_buy_15m_gt_0_52", V7TagActionRequiredConfirm) {
		t.Fatalf("missing required confirmation semantics: %+v", defs)
	}
	if !hasTagAction(defs, "new_unclassified_reason", V7TagActionUnknown) {
		t.Fatalf("unknown tags must default to context-only semantics: %+v", defs)
	}
}

func hasTagAction(defs []HunterV7TagDefinition, tag, action string) bool {
	for _, def := range defs {
		if def.Tag == tag && def.LLMAction == action {
			return true
		}
	}
	return false
}
