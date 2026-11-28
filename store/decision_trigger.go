package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync/atomic"
	"unicode"

	"github.com/Aixxww/AiT/logger"
)

var decisionTriggerCoercionCount atomic.Int64

// DecisionTriggerCoercionCount returns the cumulative number of wait-trigger
// string fields coerced from JSON numbers or booleans.
func DecisionTriggerCoercionCount() int64 {
	return decisionTriggerCoercionCount.Load()
}

// RawJSONList captures every occurrence of a JSON object key (including
// duplicates) in appearance order so callers validate every occurrence rather
// than silently discarding earlier invalid or coerced values.
type RawJSONList []json.RawMessage

// UnmarshalJSON appends a copy of data as one occurrence.
func (r *RawJSONList) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("store.RawJSONList: UnmarshalJSON on nil pointer")
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	*r = append(*r, json.RawMessage(cp))
	return nil
}

type triggerCoercion struct {
	fieldPath string
	rawType   string
	literal   string
}

// UnmarshalDecisionTrigger decodes a DecisionTrigger JSON object.
//
// Tolerance is strictly scoped to action == "wait": string-typed fields inside
// the trigger sub-object (required_close, action_if_triggered) that arrive as
// JSON numbers or booleans are lexically coerced to their raw JSON literal
// string. Every applied coercion increments DecisionTriggerCoercionCount and
// logs a distinct warning.
//
// All other actions (open_long, open_short, close_*, hold, etc.) and all
// non-string trigger fields (trigger_price, expires_in_bars) remain strictly
// fail-closed.
func UnmarshalDecisionTrigger(data []byte, action string, symbol string) (*DecisionTrigger, error) {
	return UnmarshalDecisionTriggerList([]json.RawMessage{data}, action, symbol)
}

// UnmarshalDecisionTriggerList validates every occurrence of a "trigger" key
// in order and returns the last occurrence's decoded value (matching
// encoding/json last-key-wins for well-formed input). Coercions from every
// occurrence are buffered and emitted only if all occurrences succeed.
func UnmarshalDecisionTriggerList(occurrences []json.RawMessage, action string, symbol string) (*DecisionTrigger, error) {
	var effective *DecisionTrigger
	var allCoercions []triggerCoercion
	for _, raw := range occurrences {
		tr, coercions, err := decodeDecisionTrigger(raw, action)
		if err != nil {
			return nil, err
		}
		allCoercions = append(allCoercions, coercions...)
		effective = tr
	}

	if len(allCoercions) > 0 {
		sym := sanitizeLogSymbol(symbol)
		for _, c := range allCoercions {
			decisionTriggerCoercionCount.Add(1)
			logger.Warnf("⚠️ [decision_parse_coercion] coerced wait %s from %s to string (symbol=%s raw=%s)",
				c.fieldPath, c.rawType, sym, c.literal)
		}
	}

	return effective, nil
}

func decodeDecisionTrigger(data []byte, action string) (*DecisionTrigger, []triggerCoercion, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil, nil
	}
	if action != "wait" {
		var tr DecisionTrigger
		if err := json.Unmarshal(trimmed, &tr); err != nil {
			return nil, nil, err
		}
		return &tr, nil, nil
	}

	type rawWaitTrigger struct {
		TriggerPrice      float64     `json:"trigger_price,omitempty"`
		RequiredClose     RawJSONList `json:"required_close,omitempty"`
		ExpiresInBars     int         `json:"expires_in_bars,omitempty"`
		ActionIfTriggered RawJSONList `json:"action_if_triggered,omitempty"`
	}
	var raw rawWaitTrigger
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, nil, err
	}

	var coercions []triggerCoercion
	reqClose, c1, err := decodeWaitTriggerStringFieldList(raw.RequiredClose, "trigger.required_close")
	if err != nil {
		return nil, nil, err
	}
	coercions = append(coercions, c1...)

	actIfTrig, c2, err := decodeWaitTriggerStringFieldList(raw.ActionIfTriggered, "trigger.action_if_triggered")
	if err != nil {
		return nil, nil, err
	}
	coercions = append(coercions, c2...)

	return &DecisionTrigger{
		TriggerPrice:      raw.TriggerPrice,
		RequiredClose:     reqClose,
		ExpiresInBars:     raw.ExpiresInBars,
		ActionIfTriggered: actIfTrig,
	}, coercions, nil
}

func decodeWaitTriggerStringFieldList(raws RawJSONList, fieldPath string) (string, []triggerCoercion, error) {
	var effective string
	var coercions []triggerCoercion
	for _, raw := range raws {
		v, c, err := decodeWaitTriggerStringField(raw, fieldPath)
		if err != nil {
			return "", nil, err
		}
		if c != nil {
			coercions = append(coercions, *c)
		}
		effective = v
	}
	return effective, coercions, nil
}

func sanitizeLogSymbol(symbol string) string {
	if symbol == "" {
		return "<none>"
	}
	for _, r := range symbol {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return strconv.QuoteToASCII(symbol)
		}
	}
	return symbol
}

func decodeWaitTriggerStringField(raw json.RawMessage, fieldPath string) (string, *triggerCoercion, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil, nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", nil, fmt.Errorf("%s: %w", fieldPath, err)
		}
		return s, nil, nil
	}
	if bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false")) {
		literal := string(trimmed)
		return literal, &triggerCoercion{
			fieldPath: fieldPath,
			rawType:   "bool",
			literal:   literal,
		}, nil
	}
	if trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9') {
		var num json.Number
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.UseNumber()
		if err := dec.Decode(&num); err == nil {
			var extra any
			if err2 := dec.Decode(&extra); err2 == io.EOF {
				if _, err3 := num.Float64(); err3 == nil {
					literal := string(trimmed)
					return literal, &triggerCoercion{
						fieldPath: fieldPath,
						rawType:   "number",
						literal:   literal,
					}, nil
				}
			}
		}
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return "", nil, fmt.Errorf("%s: %w", fieldPath, err)
	}
	return s, nil, nil
}
