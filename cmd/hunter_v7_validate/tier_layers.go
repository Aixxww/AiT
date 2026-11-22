package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// Fix 12 (P2-12): the three tier layers are distinct by design, and every
// report now names the one it shows.
//
//   - module status: the signal's own status (candidate / wait_confirm / ...),
//     set by the producing module.
//   - assembled tier: kernel AssembleHunterV7CandidateCoins at scoring time,
//     persisted as execution_tier / tier_reason, and the tier the outcome
//     tracker registers.
//   - prompt-final tier: the tier after the prompt-time reprice, live
//     confirmation and readiness re-check, i.e. what the LLM sees; persisted
//     as prompt_final_tier / prompt_final_tier_reason.
const (
	tierLayerModuleStatus = "module_status"
	tierLayerAssembled    = "assembled_tier"
	tierLayerPromptFinal  = "prompt_final_tier"
)

var tierLayerLegend = map[string]string{
	tierLayerModuleStatus: "the signal's own status from its module (signals[].status, by_status)",
	tierLayerAssembled:    "kernel assembly at scoring time; DB execution_tier / tier_reason; the tier the outcome tracker registers",
	tierLayerPromptFinal:  "after the prompt-time reprice, live confirmation and readiness re-check; what the LLM sees; DB prompt_final_tier / prompt_final_tier_reason",
}

// tierLayerReport is the round report's tier_layers section.
type tierLayerReport struct {
	Legend map[string]string `json:"legend"`
	// AssembledCounts counts the batch-assembled candidates (the tiers
	// written to execution_tier). opportunity_cover.by_execution_tier is the
	// same layer re-assembled one signal at a time.
	AssembledCounts map[string]int `json:"assembled_tier_counts"`
	// PromptFinalCounts counts the candidates shown in the prompt (held
	// positions and symbols without market data are not shown), from the
	// prompt build itself; ai_recognition.prompt_tier_counts is the same
	// layer parsed from the prompt text.
	PromptFinalCounts map[string]int                   `json:"prompt_final_tier_counts"`
	NotInPrompt       int                              `json:"assembled_not_in_prompt"`
	Promoted          []kernel.HunterV7PromptFinalTier `json:"promoted,omitempty"`
	Demoted           []kernel.HunterV7PromptFinalTier `json:"demoted,omitempty"`
	ReasonChanged     int                              `json:"same_tier_reason_changed"`
	PromptFinal       []kernel.HunterV7PromptFinalTier `json:"prompt_final"`
}

func tierRank(t string) int {
	switch t {
	case "EXECUTABLE":
		return 3
	case "REVIEWABLE":
		return 2
	case "WATCH":
		return 1
	default:
		return 0
	}
}

func buildTierLayerReport(assembled []kernel.CandidateCoin, final []kernel.HunterV7PromptFinalTier) tierLayerReport {
	r := tierLayerReport{
		Legend:            tierLayerLegend,
		AssembledCounts:   map[string]int{},
		PromptFinalCounts: map[string]int{},
		PromptFinal:       append([]kernel.HunterV7PromptFinalTier{}, final...),
	}
	shown := map[string]bool{}
	for _, f := range final {
		shown[kernel.HunterV7PromptFinalKey(f.Symbol, f.SetupType)] = true
		r.PromptFinalCounts[f.Tier]++
		switch {
		case tierRank(f.Tier) > tierRank(f.AssembledTier):
			r.Promoted = append(r.Promoted, f)
		case tierRank(f.Tier) < tierRank(f.AssembledTier):
			r.Demoted = append(r.Demoted, f)
		case f.TierReason != f.AssembledReason:
			r.ReasonChanged++
		}
	}
	for _, c := range assembled {
		r.AssembledCounts[c.V7ExecutionTier]++
		if !shown[kernel.HunterV7PromptFinalKey(c.Symbol, c.V7SetupType)] {
			r.NotInPrompt++
		}
	}
	return r
}

// tierLayerMarkdown is the report section listing the layers side by side.
func tierLayerMarkdown(r tierLayerReport) string {
	var sb strings.Builder
	sb.WriteString("## Tier layers (fix 12)\n\n")
	keys := []string{tierLayerModuleStatus, tierLayerAssembled, tierLayerPromptFinal}
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("- `%s`: %s\n", k, r.Legend[k]))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("| Layer | Counts |\n|---|---|\n| assembled_tier (DB execution_tier) | %s |\n| prompt_final_tier (LLM; DB prompt_final_tier) | %s |\n\n",
		sortedMap(r.AssembledCounts), sortedMap(r.PromptFinalCounts)))
	sb.WriteString(fmt.Sprintf("- Promoted at prompt time: %d; demoted: %d; same tier, reason changed: %d; assembled candidates not shown in the prompt: %d.\n\n",
		len(r.Promoted), len(r.Demoted), r.ReasonChanged, r.NotInPrompt))
	if len(r.Promoted)+len(r.Demoted) > 0 {
		sb.WriteString("| Change | Symbol | Setup | Assembled tier (reason) | Prompt-final tier (reason) |\n|---|---|---|---|---|\n")
		row := func(kind string, f kernel.HunterV7PromptFinalTier) {
			sb.WriteString(fmt.Sprintf("| %s | %s | `%s` | %s (`%s`) | %s (`%s`) |\n", kind, f.Symbol, f.SetupType, f.AssembledTier, f.AssembledReason, f.Tier, f.TierReason))
		}
		for _, f := range r.Promoted {
			row("promoted", f)
		}
		for _, f := range r.Demoted {
			row("demoted", f)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// signalTierColumns returns the assembled and prompt-final tier of a signal
// for the per-signal table ("-" = not assembled / not in the prompt).
func signalTierColumns(symbol, setup string, assembled map[string]string, final map[string]string) (string, string) {
	key := kernel.HunterV7PromptFinalKey(symbol, setup)
	a, ok := assembled[key]
	if !ok || a == "" {
		a = "-"
	}
	f, ok := final[key]
	if !ok || f == "" {
		f = "-"
	}
	return a, f
}

func (r tierLayerReport) tierMaps() (assembled, final map[string]string) {
	assembled, final = map[string]string{}, map[string]string{}
	for _, f := range r.PromptFinal {
		k := kernel.HunterV7PromptFinalKey(f.Symbol, f.SetupType)
		final[k] = f.Tier
		assembled[k] = f.AssembledTier
	}
	return assembled, final
}

// promptFinalTiersByID reads prompt_final_tier for persisted rows (the
// outcome report's prompt-final column). Nil without a store.
func promptFinalTiersByID(st *store.Store, ids []int64) map[int64]string {
	if st == nil || len(ids) == 0 {
		return nil
	}
	var rows []struct {
		ID              int64
		PromptFinalTier string
	}
	if err := st.GormDB().Model(&store.HunterV7SignalRecord{}).Select("id, prompt_final_tier").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil
	}
	out := make(map[int64]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.PromptFinalTier
	}
	return out
}

func sortedIDs(m map[int64]bool) []int64 {
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
