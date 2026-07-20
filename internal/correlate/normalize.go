package correlate

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// diacriticsFold strips combining marks after NFD decomposition.
var diacriticsFold = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// NormalizeName lowercases, folds diacritics, strips punctuation, and collapses whitespace.
func NormalizeName(name string) string {
	folded, _, err := transform.String(diacriticsFold, name)
	if err != nil {
		folded = name
	}
	folded = strings.ToLower(folded)

	var sb strings.Builder
	lastWasSpace := false
	for _, r := range folded {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			sb.WriteRune(r)
			lastWasSpace = false
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
			if !lastWasSpace {
				sb.WriteRune(' ')
				lastWasSpace = true
			}
		}
	}
	return strings.TrimSpace(sb.String())
}
