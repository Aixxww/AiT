package kernel

import (
	"strings"
	"testing"
)

// #111 S3: why_this_could_fail is optional and fail-soft.

func s3Parse(t *testing.T, resp string) []Decision {
	t.Helper()
	fd, err := parseFullDecisionResponse(resp, 1000, 20, 20, 10, 10, nil, true, map[string]float64{"XUSDT": 100}, FixedDecisionRRFloor(1.2), 0)
	if err != nil {
		t.Fatalf("parse must not fail: %v\n%s", err, resp)
	}
	return fd.Decisions
}

const s3Open = `{"symbol":"XUSDT","action":"open_long","leverage":5,"position_size_usd":100,"price":100,"stop_loss":98,"take_profit":104,"confidence":80,"why_this_could_fail":%s}`

func s3Resp(value string) string {
	return "<reasoning>ok</reasoning>\n<decision>\n[" + strings.Replace(s3Open, "%s", value, 1) + "]\n</decision>"
}

func TestS3WhyThisCouldFailRoundTrip(t *testing.T) {
	d := s3Parse(t, s3Resp(`"A 1h close back under the range high turns this into a failed breakout."`))
	if len(d) != 1 || d[0].Action != "open_long" || string(d[0].WhyThisCouldFail) != "A 1h close back under the range high turns this into a failed breakout." {
		t.Fatalf("got %+v", d)
	}
	// Absent: parses as before, empty value.
	d = s3Parse(t, "<decision>\n[{\"symbol\":\"XUSDT\",\"action\":\"open_long\",\"leverage\":5,\"position_size_usd\":100,\"price\":100,\"stop_loss\":98,\"take_profit\":104,\"confidence\":80}]\n</decision>")
	if d[0].Action != "open_long" || d[0].WhyThisCouldFail != "" {
		t.Fatalf("absent field: %+v", d[0])
	}
}

// Values that would have failed the whole reply now only drop or clean the
// field; the open itself is parsed and validated as usual.
func TestS3WhyThisCouldFailIsFailSoft(t *testing.T) {
	for name, c := range map[string]struct {
		value string
		want  string
	}{
		"number":             {`42`, ""},
		"null":               {`null`, ""},
		"object":             {`{"a":"b"}`, ""},
		"array of strings":   {`["x","y"]`, ""},
		"array of objects":   {`[{"a":1},{"b":"]}"}]`, ""},
		"thousands sep":      {`"a drop below 1,200 support breaks it"`, "a drop below 1200 support breaks it"},
		"full-width sep":     {`"跌破 1，200 支撑即失败"`, "跌破 1200 支撑即失败"},
		"curly quotes":       {`"a “fake” breakout"`, "a 'fake' breakout"},
		"brackets and tilde": {`"loses [support] ~2% {fast}"`, "loses (support) -2% (fast)"},
		"cjk punctuation":    {`"假突破：回落【区间】"`, "假突破:回落(区间)"},
	} {
		d := s3Parse(t, s3Resp(c.value))
		if len(d) != 1 || d[0].Action != "open_long" || d[0].StopLoss != 98 {
			t.Fatalf("%s: the open must survive, got %+v", name, d)
		}
		if got := string(d[0].WhyThisCouldFail); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
	// No <decision> tag (the regex fallback path).
	resp := "[" + strings.Replace(s3Open, "%s", `"a “trap” at 1,050"`, 1) + "]"
	if d := s3Parse(t, resp); string(d[0].WhyThisCouldFail) != "a 'trap' at 1050" {
		t.Fatalf("fallback path: %+v", d[0])
	}
	resp = "[" + strings.Replace(s3Open, "%s", `[{"why":"x"}]`, 1) + "]"
	if d := s3Parse(t, resp); d[0].Action != "open_long" || d[0].WhyThisCouldFail != "" {
		t.Fatalf("fallback path, array of objects: %+v", d[0])
	}
}

func TestS3WhyThisCouldFailOpensOnlyAndBounded(t *testing.T) {
	resp := `<decision>
[{"symbol":"ALL","action":"wait","blocked_reason_code":"confirmation_missing","why_this_could_fail":"n/a"}]
</decision>`
	if d := s3Parse(t, resp); d[0].WhyThisCouldFail != "" {
		t.Fatalf("a wait keeps no why_this_could_fail: %+v", d[0])
	}
	long := strings.Repeat("x", whyThisCouldFailMaxRunes+50)
	d := s3Parse(t, s3Resp(`"  `+long+`  "`))
	if n := len([]rune(string(d[0].WhyThisCouldFail))); n != whyThisCouldFailMaxRunes {
		t.Fatalf("length %d, want %d", n, whyThisCouldFailMaxRunes)
	}
	// A demoted open keeps the value (it was an open as sent).
	resp = s3Resp(`"thin target"`)
	resp = strings.Replace(resp, `"take_profit":104`, `"take_profit":100.5`, 1) // RR 0.25: demoted
	d = s3Parse(t, resp)
	if d[0].Action != "wait" || d[0].RejectedAction != "open_long" || d[0].WhyThisCouldFail != "thin target" {
		t.Fatalf("demoted open: %+v", d[0])
	}
}

// Text outside why_this_could_fail is not touched by the sanitizer.
func TestS3SanitizerScope(t *testing.T) {
	in := `x "reasoning":"1,200 [a]" "why_this_could_fail" : "b [1,000]" y`
	got := sanitizeWhyThisCouldFail(in)
	want := `x "reasoning":"1,200 [a]" "why_this_could_fail" : "b (1000)" y`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if s := `"why_this_could_fail":"unterminated`; sanitizeWhyThisCouldFail(s) != s {
		t.Fatal("an unterminated value is left as it is")
	}
}

func TestS3OutputFormatLine(t *testing.T) {
	const line = "`why_this_could_fail` (opens only, optional): one plain sentence, at most 160 characters"
	for _, cfg := range []func(string) string{
		func(lang string) string { _, s, _, _ := u11RenderHybrid(u11FixtureConfig(lang)); return s },
		func(lang string) string { _, s, _, _ := u11RenderHybrid(u11HybridConfig(lang)); return s },
	} {
		for _, lang := range []string{"en", "zh"} {
			if !strings.Contains(cfg(lang), line) {
				t.Fatalf("system prompt (%s) lacks the why_this_could_fail line", lang)
			}
		}
	}
}
