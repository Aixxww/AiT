package store

import (
	"regexp"
	"strings"
)

// #132 / Audit R2 H6: legacy-only prompt references. A hybrid_v2 strategy
// whose own prompt text (prompt_sections, custom_prompt) cites content only
// the legacy payload or doctrine carries points the LLM at something that
// is not in its prompt. ValidateTradingConfig rejects it at save; the
// kernel's pre-send contract check (#111 S2) also fails such a round closed.
//
// Matching rule:
//   - legacy payload field names and doctrine keys (snake_case identifiers)
//     match as whole identifiers: the characters around them must not be
//     ASCII letters, digits or "_", so "ai_priority_x" or "my_llm_action"
//     do not match, while "llm_action", "tag_semantics.llm_action" or
//     "（llm_action）" do;
//   - the legacy doctrine's playbook section is cited in prose, and the
//     bare word "playbook" / "剧本" is common English / Chinese, so it
//     matches only as a doctrine section reference: on one line with a
//     section mark "§<digit>" or "S<digit>" (e.g. "§3 剧本", "S3 playbook",
//     "the playbook in S3"). "follow your playbook" alone does not match.

// V7LegacyOnlyPromptFields are the legacy-only identifiers checked by
// V7PromptLegacyOnlyRefs: legacy doctrine keys and legacy payload fields
// the hybrid_v2 payload never carries.
var V7LegacyOnlyPromptFields = []string{
	"llm_action",
	"tag_semantics",
	"hunter_v7_signal_json",
	"confirmation_summary",
	"execution_contract",
	"ai_priority",
	"suggested_trigger",
}

// v7LegacyPlaybookWords are the prose words of the legacy playbook section.
var v7LegacyPlaybookWords = []string{"playbook", "剧本"}

var reV7DoctrineSectionMark = regexp.MustCompile(`(§\s*\d|\bS\d\b)`)

func isV7IdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// containsV7Identifier reports a whole-identifier occurrence of ident.
func containsV7Identifier(text, ident string) bool {
	for from := 0; ; {
		i := strings.Index(text[from:], ident)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(ident)
		if (i == 0 || !isV7IdentByte(text[i-1])) && (end >= len(text) || !isV7IdentByte(text[end])) {
			return true
		}
		from = i + 1
	}
}

// V7PromptLegacyOnlyRefs lists the legacy-only references a hybrid_v2
// strategy's own prompt text cites (nil for any other strategy).
func V7PromptLegacyOnlyRefs(cfg *StrategyConfig) []string {
	if cfg == nil || cfg.CoinSource.Hunter == nil || !EffectiveV7Prompt(cfg.CoinSource.Hunter.V7Prompt).HybridV2() {
		return nil
	}
	ps := cfg.PromptSections
	text := strings.Join([]string{ps.RoleDefinition, ps.TradingFrequency, ps.EntryStandards, ps.DecisionProcess, cfg.CustomPrompt}, "\n")
	lower := strings.ToLower(text)
	var hits []string
	for _, f := range V7LegacyOnlyPromptFields {
		if containsV7Identifier(lower, f) {
			hits = append(hits, f)
		}
	}
	for _, w := range v7LegacyPlaybookWords {
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(strings.ToLower(line), w) && reV7DoctrineSectionMark.MatchString(line) {
				hits = append(hits, w)
				break
			}
		}
	}
	return hits
}
