package store

import (
	"strconv"
	"strings"
)

// U5: outcome tag schema (redesign plan §5.4) and the parameters it uses.
//
// Every persisted Hunter v7 signal record carries the tags below. The
// outcome tracker adds the fee-net R under the owner R convention (OD-2):
// full exit at the first of TP1 or the stop, stop first inside one 1m bar,
// entry E at the close of the first full 1m bar after the signal, and a
// round-trip cost of FeeNetCostPct * E inside the risk unit:
//
//	R(TP1)  = (|TP1 - E| - E*cost) / (|E - stop| + E*cost)
//	R(stop) = -1
//	R(none) = 0 (neither level reached inside the tracking horizon)

// Outcome sources (plan §5.4). Only prompt_open_review outcomes enter the
// real win-rate denominators; watch_audit and shadow are audit buckets.
const (
	V7OutcomeSourcePromptOpenReview = "prompt_open_review"
	V7OutcomeSourceWatchAudit       = "watch_audit"
	V7OutcomeSourceShadow           = "shadow"
)

// Decision paths (plan §5.4). Until U11 every open-reviewable candidate
// gets the full LLM decision; nothing else reaches the LLM as a candidate.
const (
	V7DecisionPathLLMFull = "llm_full"
	V7DecisionPathNone    = "none"
)

// Stacks.
const (
	V7StackNative = "native"
	V7StackT2     = "t2"
)

// Tag values for versions and cell status that later units replace.
const (
	// V7CellStatusProbation: no cell evaluation exists yet (plan §5.3 runs
	// in U6/U7). Under §5.3 a cell that has not been evaluated is PROBATION.
	V7CellStatusProbation = "probation"
	// V7CalibrationVersionNone: no E[R] calibration exists yet (U7).
	V7CalibrationVersionNone = "none"
	// V7DoctrineVersionNone: the prompt doctrine has no version string yet
	// (U11).
	V7DoctrineVersionNone = "none"
	// V7ClassUnassigned marks a setup the class registry does not list.
	V7ClassUnassigned = "UNASSIGNED"
	// V7BandUnknown marks a band whose inputs are missing.
	V7BandUnknown = "unknown"
)

// Fee-net walk outcomes written to fee_net_outcome.
const (
	V7FeeNetTP1            = "tp1"
	V7FeeNetStop           = "stop"
	V7FeeNetNone           = "none"            // horizon ended with neither level reached
	V7FeeNetDegenerateStop = "degenerate_stop" // E already at or beyond the stop: no R
	V7FeeNetDegenerateTP1  = "degenerate_tp1"  // E already at or beyond TP1: no R
	V7FeeNetNoAnchor       = "no_anchor"       // no 1m bar after the signal inside the horizon
)

// Defaults (plan §5.3 RR bands, U0-D stop/ATR bands, OD-2 cost).
var (
	DefaultV7RRBandEdges      = []float64{1.0, 1.2, 1.5, 2.0}
	DefaultV7StopATRBandEdges = []float64{0.5, 1.0, 1.5, 2.0, 3.0}
)

// DefaultV7FeeNetCostPct is the OD-2 round-trip cost as a fraction of E. It
// equals V7FASTStructureConfig.CostPct, the value behind every U0 fee-net R.
const DefaultV7FeeNetCostPct = 0.0015

// DefaultV7WatchAuditMaxTracked caps the watch_audit / shadow rows the
// outcome tracker follows, separately from the open-review cap, so audit
// rows can never displace a real candidate.
const DefaultV7WatchAuditMaxTracked = 200

// V7OutcomeTagConfig is HunterConfig.V7OutcomeTags (json v7_outcome_tags).
// Zero values mean the defaults above.
type V7OutcomeTagConfig struct {
	RRBandEdges      []float64 `json:"rr_band_edges,omitempty"`
	StopATRBandEdges []float64 `json:"stop_atr_band_edges,omitempty"`
	FeeNetCostPct    float64   `json:"fee_net_cost_pct,omitempty"`
	// WatchAudit makes the outcome tracker follow WATCH and shadow-only
	// rows into the watch_audit / shadow buckets. Off by default: it adds
	// tracked symbols and 1m backfill calls, and no forward phase is
	// approved.
	WatchAudit           bool `json:"watch_audit,omitempty"`
	WatchAuditMaxTracked int  `json:"watch_audit_max_tracked,omitempty"`
}

// EffectiveV7OutcomeTags returns the config with defaults filled in.
func EffectiveV7OutcomeTags(c *V7OutcomeTagConfig) V7OutcomeTagConfig {
	out := V7OutcomeTagConfig{}
	if c != nil {
		out = *c
	}
	if len(out.RRBandEdges) == 0 {
		out.RRBandEdges = append([]float64(nil), DefaultV7RRBandEdges...)
	}
	if len(out.StopATRBandEdges) == 0 {
		out.StopATRBandEdges = append([]float64(nil), DefaultV7StopATRBandEdges...)
	}
	if out.FeeNetCostPct <= 0 {
		out.FeeNetCostPct = DefaultV7FeeNetCostPct
	}
	if out.WatchAuditMaxTracked <= 0 {
		out.WatchAuditMaxTracked = DefaultV7WatchAuditMaxTracked
	}
	return out
}

// V7BandLabel places v on ascending edges: "lt_1.0", "1.0_1.2", ...,
// "ge_2.0". ok=false (or no edges) gives V7BandUnknown.
func V7BandLabel(v float64, ok bool, edges []float64) string {
	if !ok || len(edges) == 0 {
		return V7BandUnknown
	}
	fx := func(x float64) string {
		s := strconv.FormatFloat(x, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	}
	if v < edges[0] {
		return "lt_" + fx(edges[0])
	}
	for i := 1; i < len(edges); i++ {
		if v < edges[i] {
			return fx(edges[i-1]) + "_" + fx(edges[i])
		}
	}
	return "ge_" + fx(edges[len(edges)-1])
}

// V7FeeNetRRAtTP1 is the OD-2 fee-net RR of a full exit at TP1 from entry e.
func V7FeeNetRRAtTP1(e, stop, tp1, costPct float64) (float64, bool) {
	if e <= 0 || stop <= 0 || tp1 <= 0 {
		return 0, false
	}
	risk := v7Abs(e-stop) + e*costPct
	if risk <= 0 {
		return 0, false
	}
	return (v7Abs(tp1-e) - e*costPct) / risk, true
}

// V7Family is the setup type without its _long / _short suffix (plan §3.2
// families group both directions; direction stays its own column).
// The Task-2 prefix is stripped first (U11, D2 (a)), so a family keeps
// one value whichever name the setup was emitted under.
func V7Family(setupType string) string {
	setupType = V7CanonicalSetupType(setupType)
	for _, suf := range []string{"_long", "_short"} {
		if strings.HasSuffix(setupType, suf) {
			return strings.TrimSuffix(setupType, suf)
		}
	}
	return setupType
}

func v7Abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
