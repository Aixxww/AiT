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
	return BuildHunterV7SignalRecordsTagged(signals, candidates, DefaultHunterV7RecordTagOptions())
}

// BuildHunterV7SignalRecordsTagged is BuildHunterV7SignalRecords with the U5
// outcome tags (plan §5.4) computed from opts.
func BuildHunterV7SignalRecordsTagged(signals []local.V7SignalOutput, candidates []CandidateCoin, opts HunterV7RecordTagOptions) []local.V7SignalRecord {
	tierMap := make(map[string]CandidateCoin, len(candidates))
	for _, cc := range candidates {
		key := cc.Symbol + "|" + cc.V7SetupType
		tierMap[key] = cc
	}
	records := make([]local.V7SignalRecord, 0, len(signals))
	for _, sig := range signals {
		key := sig.Symbol + "|" + string(sig.SetupType)
		rec := local.V7SignalRecord{Signal: sig}
		var matched *CandidateCoin
		if sig.SetupType == local.V7SetupModuleNoMatch {
			rec.Tier = string(local.V7ReadinessRejected)
			rec.TierReason = "module_no_match"
			rec.BlockedGate = "module_no_match"
		} else if cc, ok := tierMap[key]; ok {
			matched = &cc
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
		rec.Tags = hunterV7SignalTags(sig, matched, rec.Tier, opts)
		records = append(records, rec)
	}
	return records
}

// BuildHunterV7SignalDBRecords converts classified V7 records into the
// database model. The raw JSON snapshot is persisted with the flat columns so
// dashboards can reconstruct the full prompt contract.
func BuildHunterV7SignalDBRecords(cycleNum int, records []local.V7SignalRecord, ts time.Time, trackStatus string) []store.HunterV7SignalRecord {
	return BuildHunterV7SignalDBRecordsWithAudit(cycleNum, records, ts, trackStatus, false)
}

// BuildHunterV7SignalDBRecordsWithAudit also marks watch_audit / shadow rows
// as tracked when watchAudit is on (U5, v7_outcome_tags.watch_audit).
func BuildHunterV7SignalDBRecordsWithAudit(cycleNum int, records []local.V7SignalRecord, ts time.Time, trackStatus string, watchAudit bool) []store.HunterV7SignalRecord {
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
		recordTrackStatus := ""
		if trackStatus != "" && HunterV7ShouldTrackRecord(rec, watchAudit) {
			recordTrackStatus = trackStatus
		}
		var triggerPrice *float64
		if rec.Tags.TriggerPrice > 0 {
			p := rec.Tags.TriggerPrice
			triggerPrice = &p
		}
		dataGaps := rec.Tags.DataGaps
		if dataGaps == nil {
			dataGaps = []string{}
		}
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
			TrackStatus:       recordTrackStatus,
			RawJSON:           string(rawJSON),

			Stack:                    rec.Tags.Stack,
			SignalClass:              rec.Tags.SignalClass,
			Family:                   rec.Tags.Family,
			RRBand:                   rec.Tags.RRBand,
			StopATRBand:              rec.Tags.StopATRBand,
			CellStatus:               rec.Tags.CellStatus,
			ProfileVersion:           rec.Tags.ProfileVersion,
			CalibrationVersion:       rec.Tags.CalibrationVersion,
			DoctrineVersion:          rec.Tags.DoctrineVersion,
			PayloadVersion:           rec.Tags.PayloadVersion,
			LifecycleStateAtDecision: rec.Tags.LifecycleState,
			TriggerPrice:             triggerPrice,
			T2EventID:                rec.Tags.T2EventID,
			DecisionPath:             rec.Tags.DecisionPath,
			OutcomeSource:            rec.Tags.OutcomeSource,
			DataGaps:                 store.ToJSON(dataGaps),
			ExpectedR:                rec.Tags.ExpectedR,
			ExpectedRLower:           rec.Tags.ExpectedRLower,
		})
	}
	return dbRecords
}

// HunterV7ShouldTrackSignal returns true for signals whose outcome should be
// followed. WATCH rows stay persisted for funnel stats but do not count as
// active trade outcomes unless a caller explicitly tracks missed opportunity.
func HunterV7ShouldTrackSignal(rec local.V7SignalRecord) bool {
	return hunterV7IsOpenReviewTier(rec.Tier)
}

// HunterV7ShouldTrackRecord is HunterV7ShouldTrackSignal plus, when
// watchAudit is on, WATCH-tier rows in the watch_audit or shadow bucket
// (U5). REJECTED and router-dropped rows are never tracked.
func HunterV7ShouldTrackRecord(rec local.V7SignalRecord, watchAudit bool) bool {
	if HunterV7ShouldTrackSignal(rec) {
		return true
	}
	return watchAudit && rec.Tier == string(local.V7ReadinessWatch) &&
		(rec.Tags.OutcomeSource == store.V7OutcomeSourceWatchAudit || rec.Tags.OutcomeSource == store.V7OutcomeSourceShadow)
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

// ApplyHunterV7PromptFinalTiers stamps the prompt-final tier (fix 12) on
// the DB records of one cycle, matching by Symbol|SetupType as the record
// builder does. Records not shown in the prompt keep "". It returns how
// many records were stamped. execution_tier is not touched.
func ApplyHunterV7PromptFinalTiers(records []store.HunterV7SignalRecord, final []HunterV7PromptFinalTier) int {
	if len(final) == 0 {
		return 0
	}
	byKey := make(map[string]HunterV7PromptFinalTier, len(final))
	for _, f := range final {
		byKey[HunterV7PromptFinalKey(f.Symbol, f.SetupType)] = f
	}
	n := 0
	for i := range records {
		if f, ok := byKey[HunterV7PromptFinalKey(records[i].Symbol, records[i].SetupType)]; ok {
			records[i].PromptFinalTier, records[i].PromptFinalTierReason = f.Tier, f.TierReason
			n++
		}
	}
	return n
}
