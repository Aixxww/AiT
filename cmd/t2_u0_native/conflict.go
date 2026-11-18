package main

// U8 conflict re-measure (plan §4.4, §6.3 U8 row). With -conflict-out, every
// snapshot's T2 signal (at its created state and at TRIGGERED) is merged with
// the native final signals of the same symbol and passed through the
// production final one-direction pass (U3, config from
// kernel.HunterV7FinalConflictConfigFor with the default strategy config).
// Each row records the opposite-direction co-fire before the pass, the native
// families involved, whether any is the T2 family's twin (§4.1), the pass
// resolution, and whether both directions survive the pass (unresolved).

import (
	"encoding/json"
	"flag"
	"os"
	"strings"

	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

var conflictOut = flag.String("conflict-out", "", "U8 conflict re-measure output")

// v7TwinPairs maps a T2 family prefix to its native twin (plan §4.1 A-D).
var v7TwinPairs = map[string]string{
	"bb_oi_accumulation":     "accumulation_breakout_long",
	"momentum_continuation_": "leader_momentum_long",
	"compression_oi_watch":   "volatility_squeeze_breakout",
	"confirmed_breakout":     "trend_breakout_long",
}

type conflictRow struct {
	EventID     string                       `json:"event_id"`
	T2Setup     string                       `json:"t2_setup"`
	T2Dir       string                       `json:"t2_direction"`
	State       string                       `json:"t2_state"`
	Opposite    []string                     `json:"opposite_native"` // native final setups opposite the T2 signal
	TwinPair    string                       `json:"twin_pair,omitempty"`
	TwinOpposed bool                         `json:"twin_opposite"`
	Resolution  []local.V7ConflictResolution `json:"resolution,omitempty"`
	Unresolved  bool                         `json:"unresolved"`
	T2Survives  bool                         `json:"t2_survives"`
}

var conflictRows []conflictRow

func conflictAdd(eventID string, sig *local.V7T2Signal, natives []local.V7SignalOutput) {
	if *conflictOut == "" || sig == nil {
		return
	}
	cfg := kernel.HunterV7FinalConflictConfigFor(&store.HunterConfig{})
	pair, twin := "", ""
	for prefix, nat := range v7TwinPairs {
		if strings.HasPrefix(sig.SetupType, prefix) {
			pair, twin = prefix, nat
		}
	}
	for _, state := range []string{sig.EffectiveStatus(), "TRIGGERED"} {
		t2 := sig.ToV7SignalOutputWithLifecycle("", state)
		row := conflictRow{EventID: eventID, T2Setup: sig.SetupType, T2Dir: sig.Direction, State: state, TwinPair: pair}
		for _, n := range natives {
			if string(n.Direction) != sig.Direction {
				row.Opposite = append(row.Opposite, string(n.SetupType))
				if string(n.SetupType) == twin {
					row.TwinOpposed = true
				}
			}
		}
		out, res := local.ResolveV7FinalDirectionConflicts(append(append([]local.V7SignalOutput(nil), natives...), t2), cfg)
		row.Resolution = res
		long, short := false, false
		for _, s := range out {
			if s.Symbol != t2.Symbol {
				continue
			}
			if s.Stack == local.V7StackT2 {
				row.T2Survives = true
			}
			if s.Status == local.V7StatusConflictWatch {
				continue
			}
			long = long || s.Direction == local.V7DirLong
			short = short || s.Direction == local.V7DirShort
		}
		row.Unresolved = long && short
		conflictRows = append(conflictRows, row)
	}
}

func conflictWrite() {
	if *conflictOut == "" {
		return
	}
	b, _ := json.Marshal(conflictRows)
	_ = os.WriteFile(*conflictOut, b, 0o644)
}
