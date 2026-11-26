package kernel

import (
	"fmt"
	"strings"

	"github.com/Aixxww/AiT/store"
)

// #111 S2 (review item A): the Hunter v7 prompt contract. The backend
// writes both sides of the prompt (the doctrine in the system prompt and
// the candidate payload in the user prompt), so the version check lives in
// the backend, not in the LLM:
//   - both payload modes print one header line before the Tier Summary,
//     "payload_version=<v> | doctrine_version=<d>" (hybrid_v2 appends
//     prompt_time_utc);
//   - before a round is sent, hunterV7PromptContractError fails it closed
//     when the stored payload_version is unknown, when the system prompt does
//     not carry exactly the doctrine of that version, when the user prompt's
//     header names another pair, or when a hybrid_v2 strategy's own prompt
//     text cites legacy-only doctrine content.
//
// Version bump policy: a breaking change to the field names, types or
// meaning of V7PromptPayload (legacy) or the hunterV7V2* payload requires a
// new store payload-version constant (legacy -> legacy_2, hybrid_v2 ->
// hybrid_v3) and its pair in hunterV7KnownPromptPairs. Adding an omitempty
// field does not bump the version.

// hunterV7PromptContractMismatch is the log and reasoning code of a round
// the contract check stopped.
const hunterV7PromptContractMismatch = "prompt_contract_mismatch"

// hunterV7KnownPromptPairs maps each payload version to its doctrine version.
var hunterV7KnownPromptPairs = map[string]string{
	store.V7PayloadVersionLegacy:   store.V7DoctrineVersionNone,
	store.V7PayloadVersionHybridV2: store.V7DoctrineVersionHybridV2,
}

// Doctrine headings, one per payload version and language. Each doctrine
// text starts with its heading (TestS2DoctrineHeadingsMatchTheDoctrines).
var hunterV7DoctrineHeadings = map[string][]string{
	store.V7PayloadVersionLegacy: {
		"# Hunter v7 Strategy Execution Framework (Five Segments)",
		"# Hunter v7 策略执行框架（五段）",
	},
	store.V7PayloadVersionHybridV2: {
		"# Hunter v7 Strategy Execution Framework (hybrid_v2)",
		"# Hunter v7 策略执行框架（混合制 hybrid_v2）",
	},
}

// hunterV7PromptHeader is the version header line (without the trailing
// prompt time hybrid_v2 adds).
func hunterV7PromptHeader(pc store.V7PromptConfig) string {
	return fmt.Sprintf("payload_version=%s | doctrine_version=%s", pc.PayloadVersion, pc.DoctrineVersion())
}

// HunterV7PromptSectionsLegacyOnlyRefs lists the legacy-only references a
// hybrid_v2 strategy's own prompt text cites (nil for any other strategy).
// The token set and matching rule live in store (V7PromptLegacyOnlyRefs,
// #132), so the save-time validation and this pre-send check agree.
func HunterV7PromptSectionsLegacyOnlyRefs(cfg *store.StrategyConfig) []string {
	return store.V7PromptLegacyOnlyRefs(cfg)
}

// hunterV7PromptContractError is the pre-send check of a hunter_v7 round.
// nil for any other source type. candidates is the round's hunter_v7
// candidate count: with any candidate the version header is required
// (#132), whatever the user prompt prints.
func (e *StrategyEngine) hunterV7PromptContractError(systemPrompt, userPrompt string, candidates int) error {
	if e == nil || e.config == nil || !strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") {
		return nil
	}
	var raw *store.V7PromptConfig
	if e.config.CoinSource.Hunter != nil {
		raw = e.config.CoinSource.Hunter.V7Prompt
	}
	if raw != nil && !store.ValidV7PayloadVersion(raw.PayloadVersion) {
		return fmt.Errorf("unknown v7_prompt.payload_version %q", raw.PayloadVersion)
	}
	pc := e.hunterV7PromptConfig()
	if want, ok := hunterV7KnownPromptPairs[pc.PayloadVersion]; !ok || want != pc.DoctrineVersion() {
		return fmt.Errorf("payload_version %q / doctrine_version %q is not a known pair", pc.PayloadVersion, pc.DoctrineVersion())
	}
	hasDoctrine := func(version string) bool {
		for _, h := range hunterV7DoctrineHeadings[version] {
			if strings.Contains(systemPrompt, h) {
				return true
			}
		}
		return false
	}
	if !hasDoctrine(pc.PayloadVersion) {
		return fmt.Errorf("system prompt lacks the %s doctrine", pc.PayloadVersion)
	}
	for version := range hunterV7DoctrineHeadings {
		if version != pc.PayloadVersion && hasDoctrine(version) {
			return fmt.Errorf("system prompt carries the %s doctrine, but payload_version is %s", version, pc.PayloadVersion)
		}
	}
	if candidates > 0 || strings.Contains(userPrompt, "Tier Summary:") {
		header := hunterV7PromptHeader(pc)
		if !strings.Contains(userPrompt, header) {
			return fmt.Errorf("user prompt lacks the header %q", header)
		}
		for version := range hunterV7KnownPromptPairs {
			if version == pc.PayloadVersion {
				continue
			}
			if other := "payload_version=" + version + " "; strings.Contains(userPrompt, other) {
				return fmt.Errorf("user prompt carries payload_version=%s, but the strategy selects %s", version, pc.PayloadVersion)
			}
		}
	}
	if refs := HunterV7PromptSectionsLegacyOnlyRefs(e.config); len(refs) > 0 {
		return fmt.Errorf("hybrid_v2 strategy prompt text cites legacy-only doctrine content %v", refs)
	}
	return nil
}

// hunterV7PromptContractMismatchDecision is the round result when the
// contract check fails: one wait with backend_guard_risk; the AI is not
// called, so the round is neither an AI call nor an AI failure.
func hunterV7PromptContractMismatchDecision(err error, systemPrompt, userPrompt string) *FullDecision {
	reason := hunterV7PromptContractMismatch + ": " + err.Error()
	return &FullDecision{
		AINotCalled:  true,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		CoTTrace:     reason,
		Decisions: []Decision{{
			Symbol:            "ALL",
			Action:            "wait",
			BlockedReasonCode: "backend_guard_risk",
			Reasoning:         reason,
		}},
	}
}
