package kernel

import (
	"reflect"
	"strings"
	"testing"
)

// #131 / Audit R2 H5: the why_this_could_fail sanitizer is scoped to the
// decision payload and closes the remaining parse breakers. Every case must
// leave the open fully parsed (the field is fail-soft).

// Scenario 1: an unclosed why_this_could_fail example inside <reasoning>
// no longer rewrites the decision JSON.
func TestSanitizer131_ReasoningCannotCorruptDecision(t *testing.T) {
	decision := `[{"symbol":"XUSDT","action":"open_long","leverage":5,"position_size_usd":100,"price":100,"stop_loss":98,"take_profit":104,"confidence":80,"reasoning":"breakout, keep [size] small","why_this_could_fail":"a 1h close back under 99"}]`
	clean := s3Parse(t, "<reasoning>ok</reasoning>\n<decision>\n"+decision+"\n</decision>")
	for name, resp := range map[string]string{
		"tagged":   "<reasoning>format: \"why_this_could_fail\":\"unclosed example</reasoning>\n<decision>\n" + decision + "\n</decision>",
		"no tag":   "<reasoning>format: \"why_this_could_fail\":\"unclosed example</reasoning>\n" + decision,
		"array ex": "<reasoning>e.g. \"why_this_could_fail\":[{\"x\":1</reasoning>\n<decision>\n" + decision + "\n</decision>",
	} {
		got := s3Parse(t, resp)
		if !reflect.DeepEqual(got, clean) {
			t.Fatalf("%s: decision changed by reasoning text:\n got  %+v\n want %+v", name, got, clean)
		}
	}
	// The other fields of the payload are untouched (verbatim).
	if clean[0].Reasoning != "breakout, keep [size] small" {
		t.Fatalf("reasoning field rewritten: %q", clean[0].Reasoning)
	}
}

func TestSanitizer131_ValueBreakers(t *testing.T) {
	for name, c := range map[string]struct{ value, want string }{
		// 2: repeated groups, previously "1000,000".
		"millions":         {`"below 1,000,000 volume"`, "below 1000000 volume"},
		"mixed separators": {`"1,234,567 and 2，345，678"`, "1234567 and 2345678"},
		// 3: the ideographic enumeration comma.
		"enumeration comma":      {`"跌破 1、200 支撑"`, "跌破 1200 支撑"},
		"enumeration comma text": {`"量能、OI、taker 同时转弱"`, "量能/OI/taker 同时转弱"},
		// 5: unescaped quotes, control characters, bad escapes, barewords.
		"inner quotes":  {`"a "fake" breakout"`, "a 'fake' breakout"},
		"newline tab":   {"\"line one\nline\ttwo\"", "line one line two"},
		"bad escape":    {`"path c:\x drive"`, "path c:/x drive"},
		"valid escapes": {`"say \"no\" \u00e9"`, `say "no" é`},
		"bareword N/A":  {`N/A`, ""},
		"bareword none": {`none`, ""},
		"literal null":  {`null`, ""},
		"literal true":  {`true`, ""},
		"number":        {`42`, ""},
	} {
		d := s3Parse(t, s3Resp(c.value))
		if len(d) != 1 || d[0].Action != "open_long" || d[0].StopLoss != 98 || d[0].Confidence != 80 {
			t.Fatalf("%s: the open must survive, got %+v", name, d)
		}
		if got := string(d[0].WhyThisCouldFail); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

// Scenario 4: a composite value with mismatched or unclosed brackets ends
// at the parent's closer instead of eating it.
func TestSanitizer131_CompositeBracketStack(t *testing.T) {
	for name, value := range map[string]string{
		"unclosed array before }":  `["a", "b"`,
		"array closed by }":        `[1, 2`,
		"nested mismatch":          `[{"a": [1}`,
		"object with ] in string":  `{"x": "]"}`,
		"array of objects":         `[{"a":1},{"b":"]}"}]`,
		"unclosed object in array": `[{"a":1`,
	} {
		d := s3Parse(t, s3Resp(value))
		if len(d) != 1 || d[0].Action != "open_long" || d[0].TakeProfit != 104 || d[0].WhyThisCouldFail != "" {
			t.Fatalf("%s: got %+v", name, d)
		}
	}
	// Next to another decision object, the second object survives.
	resp := "<decision>\n[" + strings.Replace(s3Open, "%s", `["x"`, 1) + `,{"symbol":"ALL","action":"wait","blocked_reason_code":"confirmation_missing"}]` + "\n</decision>"
	d := s3Parse(t, resp)
	if len(d) != 2 || d[0].Action != "open_long" || d[1].Action != "wait" {
		t.Fatalf("two decisions: %+v", d)
	}
}

// Unit level: the bracket stack returns the match, or -1 on a type
// mismatch or an unterminated value.
func TestSanitizer131_JSONCompositeEnd(t *testing.T) {
	for in, want := range map[string]int{
		`[1,2]}`:    4,
		`[1,2}`:     -1,
		`{"a":[1]}`: 8,
		`{"a":"]"}`: 8,
		`[{"a":1]`:  -1,
		`[1,2`:      -1,
		`["x`:       -1,
	} {
		if got := jsonCompositeEnd(in, 0); got != want {
			t.Errorf("%s: got %d, want %d", in, got, want)
		}
	}
	// The chooser keeps the parent's closer.
	in := `[1, 2},{"symbol":"ALL"}]`
	if end := whyThisCouldFailCompositeEnd(in, 0); in[end+1:] != `},{"symbol":"ALL"}]` {
		t.Fatalf("chooser ended at %d: rest %q", end, in[end+1:])
	}
}
