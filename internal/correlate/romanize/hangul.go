package romanize

import "strings"

// hangulBase/hangulLast bound the precomposed Hangul syllable block.
const (
	hangulBase = 0xAC00
	hangulLast = 0xD7A3
)

// rrInitials/rrMedials/rrFinals are Revised Romanization tables indexed by decomposed jamo.
var rrInitials = [...]string{"g", "kk", "n", "d", "tt", "r", "m", "b", "pp", "s", "ss", "", "j", "jj", "ch", "k", "t", "p", "h"}
var rrMedials = [...]string{"a", "ae", "ya", "yae", "eo", "e", "yeo", "ye", "o", "wa", "wae", "oe", "yo", "u", "wo", "we", "wi", "yu", "eu", "ui", "i"}
var rrFinals = [...]string{"", "g", "kk", "gs", "n", "nj", "nh", "d", "l", "lg", "lm", "lb", "ls", "lt", "lp", "lh", "m", "b", "bs", "s", "ss", "ng", "j", "ch", "k", "t", "p", "h"}

// RomanizeHangul converts s to Revised Romanization, reporting false if s has a non-Hangul, non-ASCII rune.
func RomanizeHangul(s string) (string, bool) {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r < 128:
			sb.WriteRune(r)
		case r >= hangulBase && r <= hangulLast:
			sIndex := int(r) - hangulBase
			initial := sIndex / (21 * 28)
			medial := (sIndex % (21 * 28)) / 28
			final := sIndex % 28
			sb.WriteString(rrInitials[initial])
			sb.WriteString(rrMedials[medial])
			sb.WriteString(rrFinals[final])
		default:
			return "", false
		}
	}
	return sb.String(), true
}
