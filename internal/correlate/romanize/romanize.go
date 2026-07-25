// Package romanize converts non-Latin names to a romanized cross-script fuzzy-matching signal.
package romanize

// PreferChinese routes pure-Han text with no kana to pinyin instead of the default Japanese tier.
var PreferChinese = false

const (
	hiraganaStart, hiraganaEnd = 0x3040, 0x309F
	katakanaStart, katakanaEnd = 0x30A0, 0x30FF
	hanStart, hanEnd           = 0x4E00, 0x9FFF
)

// isHangulRune reports whether r falls in the precomposed Hangul syllable block.
func isHangulRune(r rune) bool {
	return r >= hangulBase && r <= hangulLast
}

// isKanaRune reports whether r is hiragana or katakana.
func isKanaRune(r rune) bool {
	return (r >= hiraganaStart && r <= hiraganaEnd) || (r >= katakanaStart && r <= katakanaEnd)
}

// isHanRune reports whether r is a CJK ideograph.
func isHanRune(r rune) bool {
	return r >= hanStart && r <= hanEnd
}

// script is name's routing script: Hangul, kanji (mixed or alone), kana-only, or ambiguous Han.
type script int

const (
	scriptNone script = iota
	scriptHangul
	scriptKanaOnly
	scriptKanji
	scriptHanAmbiguous
)

// classify returns name's routing script.
func classify(name string) script {
	var sawKana, sawHan bool
	for _, r := range name {
		switch {
		case isHangulRune(r):
			return scriptHangul
		case isKanaRune(r):
			sawKana = true
		case isHanRune(r):
			sawHan = true
		}
	}
	switch {
	case sawHan && sawKana:
		return scriptKanji
	case sawHan:
		return scriptHanAmbiguous
	case sawKana:
		return scriptKanaOnly
	default:
		return scriptNone
	}
}

// Romanize converts name to a romanized form, or "" if name isn't fully covered by a single script tier.
func Romanize(name string) string {
	switch classify(name) {
	case scriptHangul:
		if out, ok := RomanizeHangul(name); ok {
			return out
		}
	case scriptKanaOnly:
		if out, ok := RomanizeKana(name); ok {
			return out
		}
	case scriptKanji:
		if out, ok := RomanizeJapanese(name); ok {
			return out
		}
	case scriptHanAmbiguous:
		if PreferChinese {
			if out, ok := RomanizePinyin(name); ok {
				return out
			}
		} else if out, ok := RomanizeJapanese(name); ok {
			return out
		}
	}
	return ""
}
