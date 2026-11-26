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

// reThousandsSep matches a digit-group separator inside free text.
var reThousandsSep = regexp.MustCompile(`(\d)[,，](\d{3})`)

// whyThisCouldFailTextReplacer neutralises the characters the reply
// preprocessors (fixMissingQuotes, validateJSONFormat, the JSON-array
// regex) would turn into JSON syntax or reject inside the value.
var whyThisCouldFailTextReplacer = strings.NewReplacer(
	"\u201c", "'", "\u201d", "'", // curly double quotes become straight quotes in fixMissingQuotes
	"{", "(", "}", ")", "[", "(", "]", ")",
	"｛", "(", "｝", ")", "［", "(", "］", ")", "【", "(", "】", ")", "〔", "(", "〕", ")",
	"~", "-",
)

// sanitizeWhyThisCouldFail rewrites the why_this_could_fail values of a
// raw reply so that nothing inside them can break or reject the decision
// parse. It runs before every other preprocessing step:
//   - a string value has the characters the preprocessors would turn into
//     JSON syntax or reject replaced (whyThisCouldFailTextReplacer,
//     thousands separators removed);
//   - an array or object value is replaced by null (an array of objects
//     would otherwise cut the no-fence JSON-array match short).
//
// Text outside these values is unchanged; an unterminated value is left as
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
		switch s[start] {
		case '"':
			end := jsonStringEnd(s, start+1)
			if end < 0 {
				continue
			}
			value := whyThisCouldFailTextReplacer.Replace(s[start+1 : end])
			value = reThousandsSep.ReplaceAllString(value, "$1$2")
			sb.WriteString(s[prev : start+1])
			sb.WriteString(value)
			prev = end
		case '[', '{':
			end := jsonCompositeEnd(s, start)
			if end < 0 {
				continue
			}
			sb.WriteString(s[prev:start])
			sb.WriteString("null")
			prev = end + 1
		}
	}
	sb.WriteString(s[prev:])
	return sb.String()
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
// object that opens at i (string-aware), or -1.
func jsonCompositeEnd(s string, i int) int {
	depth := 0
	for ; i < len(s); i++ {
		switch s[i] {
		case '"':
			end := jsonStringEnd(s, i+1)
			if end < 0 {
				return -1
			}
			i = end
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
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
