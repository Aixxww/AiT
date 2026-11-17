package replay

import (
	"math"
	"strings"
)

// MakerRate is the per-side maker fee used by the reference
// (shadow_exec.py: MAKER = 0.0002; sim_live_replay.py settles Arm A
// with fee_units(entry, risk, MAKER)).
const MakerRate = 0.0002

// IsFast ports shadow_exec.is_fast: the ignition_/momentum_ families
// get the short (30m) fill TTL and the short (60m) time-stop.
func IsFast(setup string) bool {
	return strings.HasPrefix(setup, "ignition_") || strings.HasPrefix(setup, "momentum_")
}

// MWhitelisted ports shadow_exec.m_whitelisted (channel-M routing only;
// Arm A does not route, but Decide reports the route for completeness).
func MWhitelisted(setup string) bool {
	if strings.HasPrefix(setup, "ignition_") {
		return true
	}
	return setup == "momentum_continuation_long" || setup == "momentum_continuation_short"
}

// Decision is the result of Decide: routing + consumed + the index of
// the placement bar. Consumed is nil when Route == "NO_DATA"
// (Python: consumed None).
type Decision struct {
	Route    string // "L", "M", "SKIP", "NO_DATA"
	Reason   string // skip reason for Route == "SKIP", else ""
	Consumed *float64
	IAct     int // index of the placement bar; -1 when NO_DATA
}

// Decide ports shadow_exec.decide (shadow_exec.py:199-216).
//
// The placement bar is the first 1-minute bar opening at or after the
// first minute boundary at/after trigger+60s:
//
//	place_min = floor((trig_ms + 60000) / 60000) * 60000
//
// consumed is measured from that bar's CLOSE against the plan:
//
//	LONG:  (close - entry) / (tp1 - entry)
//	SHORT: (entry - close) / (entry - tp1)
//
// so consumed is the fraction of the entry->TP1 distance price has
// already travelled by the placement bar close.
func Decide(ev Event, bars []Bar) Decision {
	placeMin := (ev.TrigMs + 60000) / MinuteMs * MinuteMs
	iAct := -1
	for j, b := range bars {
		if b.OpenTime >= placeMin {
			iAct = j
			break
		}
	}
	if iAct < 0 {
		return Decision{Route: "NO_DATA", IAct: -1}
	}
	c := bars[iAct].Close
	var consumed float64
	if ev.Direction == "LONG" {
		consumed = (c - ev.Entry) / (ev.TP1 - ev.Entry)
	} else {
		consumed = (ev.Entry - c) / (ev.Entry - ev.TP1)
	}
	dec := Decision{Consumed: &consumed, IAct: iAct}
	switch {
	case consumed < 0:
		dec.Route, dec.Reason = "SKIP", "SKIP_CONSUMED_NEG"
	case consumed < 0.5:
		dec.Route = "L"
	case consumed < 1.0:
		if MWhitelisted(ev.Setup) {
			dec.Route = "M"
		} else {
			dec.Route, dec.Reason = "SKIP", "SKIP_M_NOT_WHITELIST"
		}
	default:
		dec.Route, dec.Reason = "SKIP", "SKIP_CONSUMED_GE1"
	}
	return dec
}

// LimitFillScan ports shadow_exec.limit_fill_scan
// (shadow_exec.py:175-197): the resting-guard plan-price limit fill
// scan for channel L / Arm A.
//
// Scans bars from iAct while bar open time <= deadline, where
// deadline = trig_ms + 60s + TTL (30m for fast families, else 120m):
//
//   - stop touched before entry  -> INVALIDATED
//   - TP1 touched before entry   -> RUN_AWAY
//   - entry and TP1 both touched in one bar and the bar open is closer
//     to TP1 than to entry       -> RUN_AWAY (same-bar proximity rule)
//   - entry touched              -> FILLED at that bar index
//
// If the scan ends because the bars run out before the deadline the
// status is MISSED_TENTATIVE, otherwise MISSED.
func LimitFillScan(ev Event, bars []Bar, iAct int) (string, int) {
	ttlMin := int64(120)
	if IsFast(ev.Setup) {
		ttlMin = 30
	}
	deadline := ev.TrigMs + 60000 + ttlMin*60000
	var lastT int64
	if len(bars) > 0 {
		lastT = bars[len(bars)-1].OpenTime
	}
	for j := iAct; j >= 0 && j < len(bars); j++ {
		b := bars[j]
		if b.OpenTime > deadline {
			break
		}
		var hitE, hitT1, hitS bool
		if ev.Direction == "SHORT" {
			hitE, hitT1, hitS = b.High >= ev.Entry, b.Low <= ev.TP1, b.High >= ev.Stop
		} else {
			hitE, hitT1, hitS = b.Low <= ev.Entry, b.High >= ev.TP1, b.Low <= ev.Stop
		}
		if hitS && !hitE {
			return "INVALIDATED", -1
		}
		if hitT1 && !hitE {
			return "RUN_AWAY", -1
		}
		if hitE && hitT1 && math.Abs(b.Open-ev.TP1) < math.Abs(b.Open-ev.Entry) {
			return "RUN_AWAY", -1
		}
		if hitE {
			return "FILLED", j
		}
	}
	if lastT < deadline {
		return "MISSED_TENTATIVE", -1
	}
	return "MISSED", -1
}

// FeeUnits ports shadow_exec.fee_units: round-trip fee expressed in R
// units, 2 * rate * fill / risk_dist.
func FeeUnits(fill, riskDist, rate float64) float64 {
	return 2 * rate * fill / riskDist
}

// Exit mode sets, ported from sim_live_replay.py:27-28.
var (
	fullSet    = map[string]bool{"ignition_early_long": true, "ignition_early_short": true, "momentum_continuation_long": true}
	splitBESet = map[string]bool{"momentum_continuation_watch_long": true}
)

// ExitMode ports sim_live_replay.exit_mode (sim_live_replay.py:31-35):
// FULL for the ignition-early / momentum-continuation-long families,
// SPLIT_BE for momentum_continuation_watch_long, SPLIT otherwise.
func ExitMode(setup string) string {
	if fullSet[setup] {
		return "FULL"
	}
	if splitBESet[setup] {
		return "SPLIT_BE"
	}
	return "SPLIT"
}

// WalkLive ports sim_live_replay.walk_live (sim_live_replay.py:39-112):
// the live exit model walk from the fill bar to the end of bars.
// Returns (gross_r, reason); R is measured against the plan risk
// distance |entry-stop| with u(px) = sgn*(px-entry)/risk.
//
// Per bar, in order (the order is part of the semantics):
//  1. peak = max(peak, favourable extreme of this bar, in R)
//  2. dynamic stop level from peak (FULL: peak>=2R locks stop to +1R;
//     SPLIT_BE Phase-8 ladder: >=1R -> BE, TP1 done & >=2R -> +1R,
//     >=3R -> +2R, >=4R -> peak-1R)
//  3. hard stop check on the adverse extreme (stop first: conservative)
//  4. TP checks on the favourable extreme (TP2 before setting TP1)
//  5. Phase-9 rule exits on the bar close: GIVEBACK (peak>=0.8R and
//     close<=0.1R), THESIS (age>=30m, peak<0.5R, close<=-0.5R),
//     TIME_STOP (age>=60m fast families, else 240m)
//
// If no exit triggers, the position is marked to the last close
// (TIMEOUT), with the TP1 half banked for SPLIT/SPLIT_BE after TP1.
func WalkLive(bars []Bar, ev Event, fillIdx int) (float64, string) {
	E, S, T1, T2 := ev.Entry, ev.Stop, ev.TP1, ev.TP2
	d := math.Abs(E - S)
	mode := ExitMode(ev.Setup)
	sgn := 1.0
	if ev.Direction == "SHORT" {
		sgn = -1.0
	}
	u := func(px float64) float64 { return sgn * (px - E) / d }
	fav := func(b Bar) float64 {
		if ev.Direction == "LONG" {
			return u(b.High)
		}
		return u(b.Low)
	}
	adv := func(b Bar) float64 {
		if ev.Direction == "LONG" {
			return u(b.Low)
		}
		return u(b.High)
	}
	fast := IsFast(ev.Setup)
	tFill := bars[fillIdx].OpenTime
	peak := 0.0
	tp1Done := false
	stopR := -1.0 // effective stop, in R
	for j := fillIdx; j < len(bars); j++ {
		bar := bars[j]
		if f := fav(bar); f > peak {
			peak = f
		}
		// dynamic stop levels
		switch mode {
		case "FULL":
			if peak >= 2.0 {
				stopR = math.Max(stopR, 1.0)
			}
		case "SPLIT_BE":
			if peak >= 1.0 {
				stopR = math.Max(stopR, 0.0)
			}
			if tp1Done && peak >= 2.0 {
				stopR = math.Max(stopR, 1.0)
			}
			if peak >= 3.0 {
				stopR = math.Max(stopR, 2.0)
			}
			if peak >= 4.0 {
				stopR = math.Max(stopR, peak-1.0)
			}
		}
		// hard exits on bar extremes (stop checked first: conservative)
		if adv(bar) <= stopR {
			if mode == "FULL" {
				if stopR > -1.0 {
					return stopR, "FULL_LOCK"
				}
				return stopR, "STOP"
			}
			if tp1Done {
				reason := "TP1_THEN_STOP"
				if stopR > 0 {
					reason = "TP1_THEN_TRAIL"
				} else if stopR == 0 {
					reason = "TP1_THEN_BE"
				}
				return 0.5*u(T1) + 0.5*stopR, reason
			}
			if stopR > -1.0 {
				return stopR, "TRAIL_EXIT"
			}
			return stopR, "STOP"
		}
		if mode == "FULL" {
			if fav(bar) >= u(T2) {
				return u(T2), "TP2"
			}
		} else {
			if fav(bar) >= u(T2) {
				return 0.5*u(T1) + 0.5*u(T2), "TP2"
			}
			if fav(bar) >= u(T1) {
				tp1Done = true
			}
		}
		// Phase-9 rule exits on the close
		cur := u(bar.Close)
		ageMin := float64(bar.OpenTime-tFill) / 60000.0
		base, w := 0.0, 1.0
		if mode != "FULL" && tp1Done {
			base, w = 0.5*u(T1), 0.5
		}
		if peak >= 0.8 && cur <= 0.1 {
			return base + w*cur, "GIVEBACK_EXIT"
		}
		if ageMin >= 30 && peak < 0.5 && cur <= -0.5 {
			return base + w*cur, "THESIS_EXIT"
		}
		timeStopMin := 240.0
		if fast {
			timeStopMin = 60.0
		}
		if ageMin >= timeStopMin {
			return base + w*cur, "TIME_STOP"
		}
	}
	last := bars[len(bars)-1].Close
	cur := u(last)
	if mode != "FULL" && tp1Done {
		return 0.5*u(T1) + 0.5*cur, "TIMEOUT"
	}
	return cur, "TIMEOUT"
}

// Result is the Arm A simulation outcome for one event, mirroring the
// dict returned by sim_live_replay.simulate.
type Result struct {
	Status   string   // "OK" or "NO_DATA"
	Consumed *float64 // nil when NO_DATA
	Intake   string   // "FILLED", "RUN_AWAY", "INVALIDATED", "MISSED"; "" when NO_DATA
	Outcome  string   // walk_live reason when FILLED
	Mode     string   // exit mode when FILLED
	GrossR   float64  // rounded to 4dp when FILLED
	NetR     float64  // gross - maker fees, rounded to 4dp when FILLED
	FillIdx  int      // fill bar index when FILLED, else -1
}

// Simulate ports sim_live_replay.simulate (sim_live_replay.py:115-130),
// Arm A: plan-price limit intake with the resting guard (no consumed
// gate), settled with the live exit model and maker fees.
func Simulate(ev Event, bars []Bar) Result {
	res := Result{Status: "NO_DATA", FillIdx: -1}
	if len(bars) < 30 {
		return res
	}
	dec := Decide(ev, bars)
	if dec.Route == "NO_DATA" {
		return res
	}
	res.Status = "OK"
	res.Consumed = dec.Consumed
	st, fillIdx := LimitFillScan(ev, bars, dec.IAct)
	if st != "FILLED" {
		if strings.HasPrefix(st, "MISSED") {
			res.Intake = "MISSED" // MISSED_TENTATIVE collapses to MISSED, as in Python
		} else {
			res.Intake = st
		}
		return res
	}
	gross, reason := WalkLive(bars, ev, fillIdx)
	fees := FeeUnits(ev.Entry, math.Abs(ev.Entry-ev.Stop), MakerRate)
	res.Intake = "FILLED"
	res.Outcome = reason
	res.Mode = ExitMode(ev.Setup)
	res.GrossR = round4(gross)
	res.NetR = round4(gross - fees)
	res.FillIdx = fillIdx
	return res
}
