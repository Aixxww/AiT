package store

import "strings"

// U11: prompt payload and doctrine versioning (Task C pass 1, hybrid
// payload). Owner decisions D1 (a), D2 (a), D3 (b), hybrid scope (a):
// every payload and doctrine change sits behind HunterConfig.V7Prompt and
// defaults off, so the legacy prompt stays byte-identical until a strategy
// opts into payload_version "hybrid_v2".

// Payload versions.
const (
	V7PayloadVersionLegacy   = "legacy"
	V7PayloadVersionHybridV2 = "hybrid_v2"
)

// V7DoctrineVersionHybridV2 is the doctrine version of the hybrid_v2
// payload. The legacy doctrine keeps V7DoctrineVersionNone ("none"), the
// value every pre-U11 record already carries, so records of the same
// doctrine share one tag value across the U11 boundary.
const V7DoctrineVersionHybridV2 = "hunter-v7-doctrine-hybrid-v2"

// LLM roles (D1 (a), descriptive only: the role is stated in the payload
// and the doctrine; no mechanism enforces it).
const (
	V7LLMRoleFull                = "full"                  // the LLM decides open / wait on the candidate
	V7LLMRoleDecideBeforeTrigger = "decide_before_trigger" // the LLM decides now; the backend fills on the trigger cross
	V7LLMRoleNone                = "none"                  // context only; the LLM takes no decision on it
)

// Hybrid payload defaults (fields audit, hybrid spec A).
const DefaultV7PromptBarsPerTimeframe = 30

// DefaultV7PromptTimeframes are the bar timeframes of the hybrid payload.
var DefaultV7PromptTimeframes = []string{"5m", "15m", "1h", "4h"}

// DefaultV7LLMRoles is the per-class role table (D1 (a)).
func DefaultV7LLMRoles() map[string]string {
	return map[string]string{
		V7ClassImpulse:  V7LLMRoleFull,
		V7ClassTrigger:  V7LLMRoleDecideBeforeTrigger,
		V7ClassReversal: V7LLMRoleFull,
		V7ClassContext:  V7LLMRoleNone,
	}
}

// V7PromptConfig is HunterConfig.V7Prompt (json v7_prompt).
type V7PromptConfig struct {
	// PayloadVersion selects the candidate payload and doctrine:
	// "" / "legacy" (default) or "hybrid_v2". Unknown values read as legacy.
	PayloadVersion string `json:"payload_version,omitempty"`
	// BarsPerTimeframe is the number of closed bars per timeframe in the
	// hybrid payload (the unfinished bar is printed separately).
	BarsPerTimeframe int `json:"bars_per_timeframe,omitempty"`
	// Timeframes are the bar timeframes of the hybrid payload; with
	// hybrid_v2 the market-data fetch adds any that the strategy's kline
	// selection lacks.
	Timeframes []string `json:"timeframes,omitempty"`
	// LLMRoles overrides entries of DefaultV7LLMRoles (class -> role).
	LLMRoles map[string]string `json:"llm_roles,omitempty"`
}

// EffectiveV7Prompt returns the config with defaults filled in.
func EffectiveV7Prompt(c *V7PromptConfig) V7PromptConfig {
	out := V7PromptConfig{}
	if c != nil {
		out = *c
	}
	if out.PayloadVersion != V7PayloadVersionHybridV2 {
		out.PayloadVersion = V7PayloadVersionLegacy
	}
	if out.BarsPerTimeframe <= 0 {
		out.BarsPerTimeframe = DefaultV7PromptBarsPerTimeframe
	}
	if len(out.Timeframes) == 0 {
		out.Timeframes = append([]string(nil), DefaultV7PromptTimeframes...)
	}
	roles := DefaultV7LLMRoles()
	for class, role := range out.LLMRoles {
		switch role {
		case V7LLMRoleFull, V7LLMRoleDecideBeforeTrigger, V7LLMRoleNone:
			roles[strings.ToUpper(class)] = role
		}
	}
	out.LLMRoles = roles
	return out
}

// HybridV2 reports whether the hybrid_v2 payload is selected.
func (c V7PromptConfig) HybridV2() bool { return c.PayloadVersion == V7PayloadVersionHybridV2 }

// DoctrineVersion is the version string of the selected doctrine.
func (c V7PromptConfig) DoctrineVersion() string {
	if c.HybridV2() {
		return V7DoctrineVersionHybridV2
	}
	return V7DoctrineVersionNone
}

// LLMRole is a candidate's effective role: the class role, except that
// decide_before_trigger applies only to a conditional entry (a candidate
// that has not reached its trigger); after the trigger the role is full.
// An unassigned class is full.
func (c V7PromptConfig) LLMRole(class string, conditional bool) string {
	role, ok := c.LLMRoles[class]
	if !ok {
		role, ok = DefaultV7LLMRoles()[class]
	}
	if !ok {
		return V7LLMRoleFull
	}
	if role == V7LLMRoleDecideBeforeTrigger && !conditional {
		return V7LLMRoleFull
	}
	return role
}

// ---------------------------------------------------------------------------
// D2 (a): Task-2 setup names
// ---------------------------------------------------------------------------

// V7T2SetupPrefix is the prefix of Task-2 setup types at emission (D2 (a)).
// Native setup types never start with it.
const V7T2SetupPrefix = "t2_"

// V7CanonicalSetupType strips the Task-2 prefix: "t2_ignition_early_long"
// and "ignition_early_long" both read as "ignition_early_long". Config and
// registry keys are compared in this form, so both names stay valid.
func V7CanonicalSetupType(setup string) string {
	return strings.TrimPrefix(setup, V7T2SetupPrefix)
}

// LookupV7Setup looks a setup type up in a setup-keyed map under either
// name: the exact key first, then the canonical name, then the prefixed
// name.
func LookupV7Setup[T any](m map[string]T, setup string) (T, bool) {
	if v, ok := m[setup]; ok {
		return v, true
	}
	canonical := V7CanonicalSetupType(setup)
	if v, ok := m[canonical]; ok {
		return v, true
	}
	if v, ok := m[V7T2SetupPrefix+canonical]; ok {
		return v, true
	}
	var zero T
	return zero, false
}

// canonicalV7SetupKeys returns m keyed by canonical setup type. When a
// config lists the same setup under both names, the prefixed (new) name
// wins, deterministically.
func canonicalV7SetupKeys[T any](m map[string]T) map[string]T {
	if m == nil {
		return nil
	}
	out := make(map[string]T, len(m))
	for k, v := range m {
		if !strings.HasPrefix(k, V7T2SetupPrefix) {
			out[k] = v
		}
	}
	for k, v := range m {
		if strings.HasPrefix(k, V7T2SetupPrefix) {
			out[V7CanonicalSetupType(k)] = v
		}
	}
	return out
}
