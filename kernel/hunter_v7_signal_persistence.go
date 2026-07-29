package kernel

import (
	"encoding/json"
	"time"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// BuildHunterV7SignalRecords merges raw V7 signals with kernel tier
// classification. It is shared by the live trader and validation tooling so
// persisted funnel stats use one verdict surface.
func BuildHunterV7SignalRecords(signals []local.V7SignalOutput, candidates []CandidateCoin) []local.V7SignalRecord {
	tierMap := make(map[string]CandidateCoin, len(candidates))
	for _, cc := range candidates {
		key := cc.Symbol + "|" + cc.V7SetupType
		tierMap[key] = cc
	}
	records := make([]local.V7SignalRecord, 0, len(signals))
	for _, sig := range signals {
		key := sig.Symbol + "|" + string(sig.SetupType)
		rec := local.V7SignalRecord{Signal: sig}
		if sig.SetupType == local.V7SetupModuleNoMatch {
			rec.Tier = string(local.V7ReadinessRejected)
			rec.TierReason = "module_no_match"
			rec.BlockedGate = "module_no_match"
		} else if cc, ok := tierMap[key]; ok {
			rec.Tier = cc.V7ExecutionTier
			rec.TierReason = cc.V7TierReason
			rec.BlockedGate = HunterV7BlockedGate(cc)
			if cc.V7Readiness != nil && cc.V7Readiness.BlockedGate != "" {
				rec.BlockedGate = cc.V7Readiness.BlockedGate
			}
		} else {
			rec.Tier = ""
			if sig.Status == local.V7StatusFiltered {
				rec.Tier = string(local.V7ReadinessRejected)
				rec.TierReason = "router_filtered"
				rec.BlockedGate = "router_filtered"
			} else {
				rec.BlockedGate = "router_priority_filtered"
			}
		}
		records = append(records, rec)
	}
	return records
}

// BuildHunterV7SignalDBRecords converts classified V7 records into the
// database model. The raw JSON snapshot is persisted with the flat columns so
// dashboards can reconstruct the full prompt contract.
func BuildHunterV7SignalDBRecords(cycleNum int, records []local.V7SignalRecord, ts time.Time, trackStatus string) []store.HunterV7SignalRecord {
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	dbRecords := make([]store.HunterV7SignalRecord, 0, len(records))
	for _, rec := range records {
		sig := rec.Signal
		var target1 float64
		if len(sig.Targets) > 0 {
			target1 = sig.Targets[0].Price
		}
		var oiValue, oiDelta1h, oiDelta4h, fundingRate, takerBuy15m float64
		if sig.DerivativesCtx != nil {
			oiValue = sig.DerivativesCtx.OIValue
			oiDelta1h = sig.DerivativesCtx.OIChange1h
			oiDelta4h = sig.DerivativesCtx.OIChange4h
			fundingRate = sig.DerivativesCtx.FundingRate
			takerBuy15m = sig.DerivativesCtx.TakerBuy15m
		}
		var change1h, change4h, change24h float64
		if sig.PriceCtx != nil {
			change1h = sig.PriceCtx.Change1h
			change4h = sig.PriceCtx.Change4h
			change24h = sig.PriceCtx.Change24h
		}
		var readyScore, windowHealth float64
		dataQuality := ""
		if sig.ExecutionReadiness != nil {
			readyScore = sig.ExecutionReadiness.ReadyScore
			windowHealth = sig.ExecutionReadiness.WindowHealth
			dataQuality = sig.ExecutionReadiness.DataQuality
		}
		rawJSON, _ := json.Marshal(sig)
		dbRecords = append(dbRecords, store.HunterV7SignalRecord{
			CycleNumber:       cycleNum,
			Timestamp:         ts,
			Symbol:            sig.Symbol,
			Direction:         string(sig.Direction),
			SetupType:         string(sig.SetupType),
			Status:            string(sig.Status),
			ExecutionQuality:  string(sig.ExecutionQuality),
			ExecutionTier:     rec.Tier,
			TierReason:        rec.TierReason,
			AIPriority:        sig.AIPriority,
			SetupScore:        sig.SetupScore,
			TimingScore:       sig.TimingScore,
			RiskScore:         sig.RiskScore,
			LiquidityScore:    sig.LiquidityScore,
			RegimeFitScore:    sig.RegimeFitScore,
			MarketRegime:      string(sig.MarketRegime),
			ReasonCodes:       store.ToJSON(sig.ReasonCodes),
			RiskTags:          store.ToJSON(sig.RiskTags),
			EntryZoneLower:    sig.EntryZone.Lower,
			EntryZoneUpper:    sig.EntryZone.Upper,
			InvalidationPrice: sig.Invalidation.Price,
			Target1:           target1,
			OIValue:           oiValue,
			OIDelta1h:         oiDelta1h,
			OIDelta4h:         oiDelta4h,
			FundingRate:       fundingRate,
			TakerBuy15m:       takerBuy15m,
			Change1h:          change1h,
			Change4h:          change4h,
			Change24h:         change24h,
			ReadyScore:        readyScore,
			WindowHealth:      windowHealth,
			DataQuality:       dataQuality,
			TP0Price:          sig.TP0Price,
			TP0RR:             sig.TP0RR,
			TP1Price:          sig.TP1Price,
			TP1RR:             sig.TP1RR,
			TP2Price:          sig.TP2Price,
			TP2RR:             sig.TP2RR,
			ResonanceBonus:    sig.ResonanceBonus,
			BlockedGate:       rec.BlockedGate,
			TrackStatus:       trackStatus,
			RawJSON:           string(rawJSON),
		})
	}
	return dbRecords
}

// HunterV7ShouldTrackSignal returns true for signals whose outcome should be
// followed. WATCH rows stay persisted for funnel stats but do not count as
// active trade outcomes unless a caller explicitly tracks missed opportunity.
func HunterV7ShouldTrackSignal(rec local.V7SignalRecord) bool {
	if rec.Tier == string(local.V7ReadinessExecutable) || rec.Tier == string(local.V7ReadinessReviewable) {
		return true
	}
	if rec.Signal.ExecutionReadiness != nil {
		tier := rec.Signal.ExecutionReadiness.Tier
		return tier == local.V7ReadinessExecutable || tier == local.V7ReadinessReviewable
	}
	return false
}

// HunterV7SignalEntryPrice returns the same synthetic entry used for dry-run
// outcome tracking when no exchange fill exists.
func HunterV7SignalEntryPrice(sig local.V7SignalOutput) float64 {
	if sig.EntryZone.Lower > 0 && sig.EntryZone.Upper >= sig.EntryZone.Lower {
		return (sig.EntryZone.Lower + sig.EntryZone.Upper) / 2
	}
	if sig.PriceCtx != nil && sig.PriceCtx.Last > 0 {
		return sig.PriceCtx.Last
	}
	return 0
}
