package kernel

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// #111 S3 (review item F, owner decision Q3): the optional decision field
// why_this_could_fail. The model is asked for one plain sentence on opens.
// The field is fail-soft end to end: a missing, non-string or otherwise
// unusable value is dropped (stored as NULL) and never fails the reply's
// parse or the decision.

// whyThisCouldFailMaxRunes bounds the stored value. The prompt asks for at
// most 160 characters; a longer sentence is kept up to twice that and then
// cut, so a slightly long answer is not lost.
const whyThisCouldFailMaxRunes = 320

// FailSoftText is a free-text decision field that never fails JSON
// decoding: a JSON string is kept, any other value (number, array, object,
// null) decodes to "".
type FailSoftText string

// UnmarshalJSON implements json.Unmarshaler.
func (t *FailSoftText) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		*t = ""
		return nil
	}
	*t = FailSoftText(s)
	return nil
}

// reWhyThisCouldFailKey matches the key and the colon in a raw reply.
var reWhyThisCouldFailKey = regexp.MustCompile(`"why_this_could_fail"\s*:\s*`)

// reThousandsSep matches a digit-group separator inside free text: the
// ASCII comma, the full-width comma and the ideographic enumeration comma
// (U+3001), which fixMissingQuotes turns into an ASCII comma before
// validateJSONFormat rejects "d,ddd".
var reThousandsSep = regexp.MustCompile(`(\d)[,，、](\d{3})`)

// whyThisCouldFailTextReplacer neutralises the characters the reply
// preprocessors (fixMissingQuotes, validateJSONFormat, the JSON-array
// regex) would turn into JSON syntax or reject inside the value.
var whyThisCouldFailTextReplacer = strings.NewReplacer(
	"\u201c", "'", "\u201d", "'", // curly double quotes become straight quotes in fixMissingQuotes
	"{", "(", "}", ")", "[", "(", "]", ")",
	"｛", "(", "｝", ")", "［", "(", "］", ")", "【", "(", "】", ")", "〔", "(", "〕", ")",
	"~", "-",
	"、", "/", // #131: becomes "," in fixMissingQuotes
)

// sanitizeWhyThisCouldFail rewrites the why_this_could_fail values of a
// decision payload so that nothing inside them can break or reject the
// decision parse. #131 / Audit R2 H5: extractDecisions calls it on the
// extracted decision payload only, never on <reasoning> or the raw reply.
//   - a string value: its real closing quote is the one followed by JSON
//     structure (",\"", ",{", "}", "]" or the end), so unescaped inner quotes
//     become "'"; control characters become spaces; an invalid escape's
//     backslash becomes "/"; thousands separators are removed (to a fixed
//     point); the characters in whyThisCouldFailTextReplacer are replaced;
//   - an array or object value is replaced by null; its end is the
//     bracket-stack match when the payload stays valid JSON after it, else
//     the position before the closer that makes it valid (a malformed
//     value never eats the parent's closer), see whyThisCouldFailCompositeEnd;
//   - a number, true, false or null is kept;
//   - any other token (N/A, none, ...) is replaced by null up to the next
//     ",", "}", "]" or line end.
//
// Text outside these values is unchanged; an unterminated string is left as
// it is.
func sanitizeWhyThisCouldFail(s string) string {
	locs := reWhyThisCouldFailKey.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}
	var sb strings.Builder
	prev := 0
	for _, loc := range locs {
		start := loc[1]
		if start < prev || start >= len(s) {
			continue
		}
		switch c := s[start]; {
		case c == '"':
			end := whyThisCouldFailStringEnd(s, start+1)
			if end < 0 {
				continue
			}
			sb.WriteString(s[prev : start+1])
			sb.WriteString(cleanWhyThisCouldFailText(s[start+1 : end]))
			sb.WriteString(`"`)
			prev = end + 1
		case c == '[' || c == '{':
			end := whyThisCouldFailCompositeEnd(s, start)
			sb.WriteString(s[prev:start])
			sb.WriteString("null")
			prev = end + 1
		case c == '-' || (c >= '0' && c <= '9'):
			// a number: kept (it decodes to "")
		case strings.HasPrefix(s[start:], "true") || strings.HasPrefix(s[start:], "false") || strings.HasPrefix(s[start:], "null"):
			// a JSON literal: kept
		default:
			end := start
			for end < len(s) && !strings.ContainsRune(",}]\n\r", rune(s[end])) {
				end++
			}
			sb.WriteString(s[prev:start])
			sb.WriteString("null")
			prev = end
		}
	}
	sb.WriteString(s[prev:])
	return sb.String()
}

// whyThisCouldFailStringEnd returns the index of the closing quote of a
// string value whose content starts at i: the first unescaped quote that is
// followed by JSON structure, else the first unescaped quote, else -1.
func whyThisCouldFailStringEnd(s string, i int) int {
	first := -1
	for escaped := false; i < len(s); i++ {
		switch {
		case escaped:
			escaped = false
		case s[i] == '\\':
			escaped = true
		case s[i] == '"':
			if first < 0 {
				first = i
			}
			if structuralAfterString(s, i+1) {
				return i
			}
		}
	}
	return first
}

// structuralAfterString reports whether s[i:] continues like JSON after a
// closed string value: whitespace, then the end, "}", "]", or "," followed
// by a key quote or an object.
func structuralAfterString(s string, i int) bool {
	skip := func(j int) int {
		for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
			j++
		}
		return j
	}
	i = skip(i)
	if i >= len(s) || s[i] == '}' || s[i] == ']' {
		return true
	}
	if s[i] != ',' {
		return false
	}
	j := skip(i + 1)
	return j < len(s) && (s[j] == '"' || s[j] == '{')
}

// cleanWhyThisCouldFailText makes a raw string value safe to embed between
// two straight quotes and to pass through the reply preprocessors.
func cleanWhyThisCouldFailText(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '\\':
			if i+1 < len(v) && strings.IndexByte(`"\\/bfnrt`, v[i+1]) >= 0 {
				b.WriteByte(c)
				b.WriteByte(v[i+1])
				i++
			} else if i+5 < len(v) && v[i+1] == 'u' && isHex4(v[i+2:i+6]) {
				b.WriteString(v[i : i+6])
				i += 5
			} else {
				b.WriteByte('/')
			}
		case c == '"':
			b.WriteByte('\'') // an unescaped inner quote
		case c < 0x20:
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
		}
	}
	out := b.String()
	for {
		next := reThousandsSep.ReplaceAllString(out, "$1$2")
		if next == out {
			break
		}
		out = next
	}
	return whyThisCouldFailTextReplacer.Replace(out)
}

func isHex4(s string) bool {
	if len(s) != 4 {
		return false
	}
	for i := 0; i < 4; i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// jsonStringEnd returns the index of the closing straight quote of a JSON
// string whose content starts at i, or -1.
func jsonStringEnd(s string, i int) int {
	for escaped := false; i < len(s); i++ {
		switch {
		case escaped:
			escaped = false
		case s[i] == '\\':
			escaped = true
		case s[i] == '"':
			return i
		}
	}
	return -1
}

// jsonCompositeEnd returns the index of the bracket closing the array or
// object that opens at i, scanned with a bracket stack (string-aware), or
// -1 when a closer of the wrong type comes first or the value is
// unterminated.
func jsonCompositeEnd(s string, i int) int {
	var stack []byte
	for ; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			end := jsonStringEnd(s, i+1)
			if end < 0 {
				return -1
			}
			i = end
		case '[', '{':
			stack = append(stack, c)
		case ']', '}':
			want := byte('[')
			if c == '}' {
				want = '{'
			}
			if len(stack) == 0 || stack[len(stack)-1] != want {
				return -1 // type mismatch
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i
			}
		}
	}
	return -1
}

// whyThisCouldFailCompositeEnd picks where an array or object value that
// opens at i ends, before it is replaced by null. The value always sits in
// a decision object inside the decision array, so a candidate end is
// accepted only when the rest of the payload continues that object and
// array as valid JSON (validContinuation). Candidates, in order: the
// bracket-stack match, then the position before each closer after i (a
// malformed value then never eats the parent's "}"). With no accepted
// candidate it falls back to the stack match, else the end of s.
func whyThisCouldFailCompositeEnd(s string, i int) int {
	matched := jsonCompositeEnd(s, i)
	if matched >= 0 && validContinuation(s[matched+1:]) {
		return matched
	}
	for k := i + 1; k < len(s); k++ {
		switch s[k] {
		case '"':
			end := jsonStringEnd(s, k+1)
			if end < 0 {
				k = len(s)
				continue
			}
			k = end
		case ']', '}':
			if validContinuation(s[k:]) {
				return k - 1
			}
		}
	}
	if matched >= 0 {
		return matched
	}
	return len(s) - 1
}

// validContinuation reports whether rest, following a replaced field value,
// completes a decision object and its array: [{"_":null<rest> must decode
// as one JSON value (trailing text after it, e.g. a closing fence, is
// allowed).
func validContinuation(rest string) bool {
	var v interface{}
	return json.NewDecoder(strings.NewReader(`[{"_":null`+rest)).Decode(&v) == nil
}

// normalizeWhyThisCouldFail keeps the field only on opens (as the model
// sent them, before any demotion), collapses whitespace and bounds its
// length.
func normalizeWhyThisCouldFail(decisions []Decision) {
	for i := range decisions {
		d := &decisions[i]
		if d.Action != "open_long" && d.Action != "open_short" {
			d.WhyThisCouldFail = ""
			continue
		}
		text := strings.Join(strings.Fields(string(d.WhyThisCouldFail)), " ")
		if !utf8.ValidString(text) {
			text = ""
		}
		if r := []rune(text); len(r) > whyThisCouldFailMaxRunes {
			text = string(r[:whyThisCouldFailMaxRunes])
		}
		d.WhyThisCouldFail = FailSoftText(text)
	}
}
