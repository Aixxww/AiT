package local

import (
	"sort"

	"github.com/Aixxww/AiT/store"
)

// Fix 15 (P2-15): the Task-2 module settings as the module applies them
// (zero lifecycle fields resolved to the scanner constants, the empty
// setup list expanded), through the same accessors the module calls.
// Read only: nothing decides from it.
type V7T2EffectiveSettings struct {
	Enabled                 bool     `json:"enabled"`
	EnabledSetups           []string `json:"enabled_setups"`            // in dispatcher order
	EnabledSetupsFrom       string   `json:"enabled_setups_from"`       // "all (empty list)" or "configured list"
	DisabledSetups          []string `json:"disabled_setups,omitempty"` // known setup types not enabled
	UnknownConfiguredSetups []string `json:"unknown_configured_setups,omitempty"`
	ObserveOnlySetups       []string `json:"observe_only_setups"` // forced to WATCH at the event level
	ObserveOnlyFrom         string   `json:"observe_only_from"`   // "scanner list" or "configured override"
	MinAlertScore           float64  `json:"armed_min_score"`
	MinWatchScore           float64  `json:"watch_promote_min_score"`
	ArmedExpiryHours        float64  `json:"armed_expiry_hours"`
	WatchExpiryHours        float64  `json:"watch_expiry_hours"`
	CooldownMinutes         float64  `json:"cooldown_minutes"`
	TriggerEnabled          bool     `json:"trigger_enabled"`
	TriggeredSurfaceMinutes float64  `json:"triggered_surface_minutes"`
	SymbolValidityGate      bool     `json:"symbol_validity_gate"`
	SetupPrefix             bool     `json:"setup_prefix"`
}

// EffectiveSettings resolves c. A nil or disabled config reports Enabled
// false with the resolved defaults.
func (c *V7T2Config) EffectiveSettings() V7T2EffectiveSettings {
	var p V7T2LifecycleParams
	enabled := c != nil && c.Enabled
	if c != nil {
		p = c.Lifecycle
	}
	out := V7T2EffectiveSettings{
		Enabled:                 enabled,
		EnabledSetups:           []string{},
		ObserveOnlySetups:       []string{},
		ObserveOnlyFrom:         "scanner list",
		MinAlertScore:           p.alertScore(),
		MinWatchScore:           p.watchScore(),
		ArmedExpiryHours:        p.armedExpiry().Hours(),
		WatchExpiryHours:        p.watchExpiry().Hours(),
		CooldownMinutes:         p.cooldown().Minutes(),
		TriggerEnabled:          !p.DisableTrigger,
		TriggeredSurfaceMinutes: p.triggeredSurface().Minutes(),
	}
	if c != nil {
		out.SymbolValidityGate, out.SetupPrefix = c.SymbolValidityGate, c.SetupPrefix
	}
	if p.ObserveOnlyOverride {
		out.ObserveOnlyFrom = "configured override"
	}
	out.EnabledSetupsFrom = "all (empty list)"
	if c != nil && len(c.EnabledSetups) > 0 {
		out.EnabledSetupsFrom = "configured list"
	}
	known := map[string]bool{}
	for _, s := range V7T2SetupTypes {
		known[s] = true
		if c.familyEnabled(s) {
			out.EnabledSetups = append(out.EnabledSetups, s)
		} else {
			out.DisabledSetups = append(out.DisabledSetups, s)
		}
		if p.observeOnly(s) {
			out.ObserveOnlySetups = append(out.ObserveOnlySetups, s)
		}
	}
	if c != nil {
		for _, s := range c.EnabledSetups {
			if !known[store.V7CanonicalSetupType(s)] {
				out.UnknownConfiguredSetups = append(out.UnknownConfiguredSetups, s)
			}
		}
		sort.Strings(out.UnknownConfiguredSetups)
	}
	return out
}
