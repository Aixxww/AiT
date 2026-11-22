package kernel

import (
	"math"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// HunterV7RecordTagOptions carries the U5 tag parameters into record
// building: the effective outcome-tag config and the class registry.
type HunterV7RecordTagOptions struct {
	Tags           store.V7OutcomeTagConfig
	Classes        map[string]store.V7SignalClassEntry
	ProfileVersion string // "" = default registry + profile version
	// DoctrineVersion is the prompt doctrine version (U11): "" reads as
	// store.V7DoctrineVersionNone, the legacy doctrine's value.
	DoctrineVersion string
}

// DefaultHunterV7RecordTagOptions is the option set with every default.
func DefaultHunterV7RecordTagOptions() HunterV7RecordTagOptions {
	return HunterV7RecordTagOptions{Tags: store.EffectiveV7OutcomeTags(nil), Classes: store.DefaultV7SignalClassRegistry()}
}

// HunterV7RecordTagOptionsFromConfig builds the options from strategy config.
func HunterV7RecordTagOptionsFromConfig(cfg *store.StrategyConfig) HunterV7RecordTagOptions {
	if cfg == nil {
		return DefaultHunterV7RecordTagOptions()
	}
	if cfg.CoinSource.Hunter == nil {
		o := DefaultHunterV7RecordTagOptions()
		o.ProfileVersion = store.V7EffectiveProfileVersion(cfg.RiskControl, nil)
		return o
	}
	return HunterV7RecordTagOptions{
		Tags:            store.EffectiveV7OutcomeTags(cfg.CoinSource.Hunter.V7OutcomeTags),
		Classes:         cfg.CoinSource.Hunter.EffectiveV7SignalClasses(),
		ProfileVersion:  store.V7EffectiveProfileVersion(cfg.RiskControl, cfg.CoinSource.Hunter),
		DoctrineVersion: store.EffectiveV7Prompt(cfg.CoinSource.Hunter.V7Prompt).DoctrineVersion(),
	}
}

func hunterV7IsOpenReviewTier(tier string) bool {
	return tier == string(local.V7ReadinessExecutable) || tier == string(local.V7ReadinessReviewable)
}

// hunterV7SignalTags computes the plan §5.4 tags of one record. cc is the
// assembled candidate when the signal reached assembly, else nil.
func hunterV7SignalTags(sig local.V7SignalOutput, cc *CandidateCoin, tier string, opts HunterV7RecordTagOptions) local.V7SignalTags {
	tags := local.V7SignalTags{
		Stack:              store.V7StackNative,
		Family:             store.V7Family(string(sig.SetupType)),
		CellStatus:         store.V7CellStatusProbation,
		ProfileVersion:     opts.ProfileVersion,
		CalibrationVersion: store.V7CalibrationVersionNone,
		DoctrineVersion:    store.V7DoctrineVersionNone,
		LifecycleState:     sig.LifecycleState,
		TriggerPrice:       sig.TriggerPrice,
		DecisionPath:       store.V7DecisionPathNone,
		DataGaps:           append([]string{}, sig.DataGaps...),
	}
	if sig.Stack == local.V7StackT2 {
		tags.Stack = store.V7StackT2
	}
	if opts.DoctrineVersion != "" {
		tags.DoctrineVersion = opts.DoctrineVersion
	}
	if tags.ProfileVersion == "" {
		tags.ProfileVersion = store.V7EffectiveProfileVersion(store.RiskControlConfig{}, nil)
	}
	if tags.LifecycleState == "" {
		tags.LifecycleState = string(sig.Status)
	}

	class, admission := "", ""
	if cc != nil {
		class, admission = cc.V7SignalClass, cc.V7ClassAdmission
	} else if entry, ok := store.LookupV7Setup(opts.Classes, string(sig.SetupType)); ok {
		class, admission = entry.Class, entry.Admission
	}
	if cc != nil && cc.V7CalibrationVersion != "" {
		tags.CalibrationVersion = cc.V7CalibrationVersion
		tags.ExpectedR, tags.ExpectedRLower = cc.V7ExpectedR, cc.V7ExpectedRLower
	}
	tags.SignalClass = class
	if tags.SignalClass == "" {
		tags.SignalClass = store.V7ClassUnassigned
	}

	switch {
	case admission == store.V7AdmissionShadowOnly:
		tags.OutcomeSource = store.V7OutcomeSourceShadow
	case hunterV7IsOpenReviewTier(tier):
		tags.OutcomeSource = store.V7OutcomeSourcePromptOpenReview
	default:
		tags.OutcomeSource = store.V7OutcomeSourceWatchAudit
	}
	if hunterV7IsOpenReviewTier(tier) {
		tags.DecisionPath = store.V7DecisionPathLLMFull
	}

	// Bands are measured from the assembly's reference price, the price the
	// backend RR check uses (top of the entry zone for LONG, bottom for
	// SHORT, or the live price when the producer carries one). Fix 06:
	// Task-2 signals keep the entry-zone edge (V7RRReferencePrice).
	ref := hunterV7ReferencePrice(CandidateCoin{Direction: string(sig.Direction), V7PriceContext: sig.PriceCtx, V7EntryZone: sig.EntryZone,
		V7RRReferencePrice: hunterV7T2RRReferencePrice(sig.Stack, string(sig.Direction), sig.EntryZone)})
	stop := sig.Invalidation.Price
	tp1 := sig.TP1Price
	if tp1 <= 0 && len(sig.Targets) > 0 {
		tp1 = sig.Targets[0].Price
	}
	rr, ok := store.V7FeeNetRRAtTP1(ref, stop, tp1, opts.Tags.FeeNetCostPct)
	tags.RRBand = store.V7BandLabel(rr, ok, opts.Tags.RRBandEdges)

	atrPct := sig.ATR15mPct
	if atrPct <= 0 && sig.ExecutionContext != nil {
		if tf, ok := sig.ExecutionContext.Timeframes["15m"]; ok && tf.HasATR {
			atrPct = tf.ATRPct
		}
	}
	k, kok := 0.0, false
	if ref > 0 && stop > 0 && atrPct > 0 {
		k, kok = math.Abs(ref-stop)/ref*100/atrPct, true
	}
	tags.StopATRBand = store.V7BandLabel(k, kok, opts.Tags.StopATRBandEdges)
	return tags
}
