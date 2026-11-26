package store

import (
	"reflect"
	"strings"
	"testing"
)

func hybridCfg(custom string) *StrategyConfig {
	cfg := GetDefaultStrategyConfig("en")
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.Hunter = &HunterConfig{V7Prompt: &V7PromptConfig{PayloadVersion: V7PayloadVersionHybridV2}}
	cfg.CustomPrompt = custom
	cfg.PromptSections = PromptSectionsConfig{}
	return &cfg
}

// #132 / Audit R2 H6: legacy-only references are rejected at save.
func TestLegacyRefs132_RejectedAtSave(t *testing.T) {
	for _, f := range V7LegacyOnlyPromptFields {
		cfg := hybridCfg("Read " + f + " before opening.")
		err := ValidateTradingConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "legacy-only content") || !strings.Contains(err.Error(), f) {
			t.Fatalf("%s: must be rejected at save, got %v", f, err)
		}
	}
	// Each prompt section is checked, not only custom_prompt.
	cfg := hybridCfg("")
	cfg.PromptSections.EntryStandards = "标签级阻断（llm_action）见文末框架 §2"
	if err := ValidateTradingConfig(cfg); err == nil {
		t.Fatal("entry_standards must be checked")
	}
	// The same text on a legacy strategy is fine.
	cfg.CoinSource.Hunter.V7Prompt = nil
	if err := ValidateTradingConfig(cfg); err != nil && strings.Contains(err.Error(), "legacy-only") {
		t.Fatalf("legacy strategy: %v", err)
	}
}

// The matching rule (documented in hunter_v7_prompt_legacy_refs.go).
func TestLegacyRefs132_MatchingRule(t *testing.T) {
	for text, want := range map[string][]string{
		"use tag_semantics.llm_action":        {"llm_action", "tag_semantics"},
		"see (confirmation_summary)":          {"confirmation_summary"},
		"AI_PRIORITY above 70":                {"ai_priority"},
		"my_llm_action and ai_priority_floor": nil, // not whole identifiers
		"follow your playbook, stay patient":  nil, // prose, no section mark
		"setup_type 对应 §3 剧本":                 {"剧本"},
		"the playbook in S3 applies":          {"playbook"},
		"S3 is the funnel. Use your playbook": {"playbook"}, // same line
		"priority and execution are separate": nil,
	} {
		if got := V7PromptLegacyOnlyRefs(hybridCfg(text)); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, want %v", text, got, want)
		}
	}
	if got := V7PromptLegacyOnlyRefs(hybridCfg("S3 section\nfollow your playbook")); got != nil {
		t.Errorf("section mark on another line: got %v", got)
	}
}
