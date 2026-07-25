package romanize

import "strings"

// hiraganaRomaji maps hiragana morae (including youon/loanword combos) to Hepburn romaji.
var hiraganaRomaji = map[string]string{
	"あ": "a", "い": "i", "う": "u", "え": "e", "お": "o",
	"か": "ka", "き": "ki", "く": "ku", "け": "ke", "こ": "ko",
	"さ": "sa", "し": "shi", "す": "su", "せ": "se", "そ": "so",
	"た": "ta", "ち": "chi", "つ": "tsu", "て": "te", "と": "to",
	"な": "na", "に": "ni", "ぬ": "nu", "ね": "ne", "の": "no",
	"は": "ha", "ひ": "hi", "ふ": "fu", "へ": "he", "ほ": "ho",
	"ま": "ma", "み": "mi", "む": "mu", "め": "me", "も": "mo",
	"や": "ya", "ゆ": "yu", "よ": "yo",
	"ら": "ra", "り": "ri", "る": "ru", "れ": "re", "ろ": "ro",
	"わ": "wa", "ゐ": "i", "ゑ": "e", "を": "o", "ん": "n",
	"が": "ga", "ぎ": "gi", "ぐ": "gu", "げ": "ge", "ご": "go",
	"ざ": "za", "じ": "ji", "ず": "zu", "ぜ": "ze", "ぞ": "zo",
	"だ": "da", "ぢ": "ji", "づ": "zu", "で": "de", "ど": "do",
	"ば": "ba", "び": "bi", "ぶ": "bu", "べ": "be", "ぼ": "bo",
	"ぱ": "pa", "ぴ": "pi", "ぷ": "pu", "ぺ": "pe", "ぽ": "po",
	"ゔ": "vu",

	"きゃ": "kya", "きゅ": "kyu", "きょ": "kyo",
	"しゃ": "sha", "しゅ": "shu", "しょ": "sho",
	"ちゃ": "cha", "ちゅ": "chu", "ちょ": "cho",
	"にゃ": "nya", "にゅ": "nyu", "にょ": "nyo",
	"ひゃ": "hya", "ひゅ": "hyu", "ひょ": "hyo",
	"みゃ": "mya", "みゅ": "myu", "みょ": "myo",
	"りゃ": "rya", "りゅ": "ryu", "りょ": "ryo",
	"ぎゃ": "gya", "ぎゅ": "gyu", "ぎょ": "gyo",
	"じゃ": "ja", "じゅ": "ju", "じょ": "jo",
	"ぢゃ": "ja", "ぢゅ": "ju", "ぢょ": "jo",
	"びゃ": "bya", "びゅ": "byu", "びょ": "byo",
	"ぴゃ": "pya", "ぴゅ": "pyu", "ぴょ": "pyo",

	"ふぁ": "fa", "ふぃ": "fi", "ふぇ": "fe", "ふぉ": "fo",
	"ゔぁ": "va", "ゔぃ": "vi", "ゔぇ": "ve", "ゔぉ": "vo",
	"てぃ": "ti", "とぅ": "tu", "でぃ": "di", "どぅ": "du",
	"ちぇ": "che", "じぇ": "je", "しぇ": "she",
	"うぃ": "wi", "うぇ": "we", "うぉ": "wo",
	"つぁ": "tsa", "つぃ": "tsi", "つぇ": "tse", "つぉ": "tso",
}

// hiraganaToKatakana shift is the fixed codepoint offset between corresponding hiragana and katakana runes.
const hiraganaToKatakanaShift = 0x60

// kanaRomaji is hiraganaRomaji plus its derived katakana entries, keyed by kana substring.
var kanaRomaji = buildKanaRomaji()

// kanaKeyLen is the length in runes of the longest key in kanaRomaji, used to bound greedy lookahead.
var kanaKeyLen = 1

func buildKanaRomaji() map[string]string {
	table := make(map[string]string, len(hiraganaRomaji)*2)
	for hira, romaji := range hiraganaRomaji {
		table[hira] = romaji
		kata := shiftKana(hira, hiraganaToKatakanaShift)
		table[kata] = romaji
		if n := len([]rune(hira)); n > kanaKeyLen {
			kanaKeyLen = n
		}
	}
	table["ー"] = "" // long vowel mark; handled specially by RomanizeKana
	return table
}

// shiftKana shifts every rune of s by delta codepoints.
func shiftKana(s string, delta rune) string {
	var sb strings.Builder
	for _, r := range s {
		sb.WriteRune(r + delta)
	}
	return sb.String()
}

// isVowelLetter reports whether b is an ASCII vowel letter.
func isVowelLetter(b byte) bool {
	switch b {
	case 'a', 'i', 'u', 'e', 'o':
		return true
	}
	return false
}

// RomanizeKana converts s to Hepburn-ish romaji, reporting false if any rune isn't kana or ASCII.
func RomanizeKana(s string) (string, bool) {
	runes := []rune(s)
	var sb strings.Builder

	for i := 0; i < len(runes); {
		r := runes[i]

		if r < 128 {
			sb.WriteRune(r)
			i++
			continue
		}

		if r == 'っ' || r == 'ッ' {
			next, ok := lookupKana(runes, i+1)
			if !ok || next == "" {
				return "", false
			}
			sb.WriteByte(next[0])
			i++
			continue
		}

		if r == 'ー' {
			out := sb.String()
			if out == "" || !isVowelLetter(out[len(out)-1]) {
				return "", false
			}
			sb.WriteByte(out[len(out)-1])
			i++
			continue
		}

		romaji, matched, consumed := matchKana(runes, i)
		if !matched {
			return "", false
		}
		sb.WriteString(romaji)
		i += consumed
	}

	return sb.String(), true
}

// lookupKana romanizes the single mora at runes[i], used to find the consonant a following sokuon doubles.
func lookupKana(runes []rune, i int) (string, bool) {
	romaji, matched, _ := matchKana(runes, i)
	return romaji, matched
}

// matchKana finds the longest kana key in kanaRomaji starting at runes[i], returning its romaji and rune length.
func matchKana(runes []rune, i int) (romaji string, matched bool, consumed int) {
	maxLen := kanaKeyLen
	if i+maxLen > len(runes) {
		maxLen = len(runes) - i
	}
	for l := maxLen; l >= 1; l-- {
		key := string(runes[i : i+l])
		if r, ok := kanaRomaji[key]; ok {
			return r, true, l
		}
	}
	return "", false, 0
}
