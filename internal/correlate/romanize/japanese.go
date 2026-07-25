package romanize

import (
	"strings"
	"sync"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// japaneseTokenizer is built once, lazily, since it loads the full IPA morphological dictionary.
var (
	japaneseTokenizerOnce sync.Once
	japaneseTokenizer     *tokenizer.Tokenizer
)

func getJapaneseTokenizer() *tokenizer.Tokenizer {
	japaneseTokenizerOnce.Do(func() {
		t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
		if err != nil {
			panic("romanize: build japanese tokenizer: " + err.Error())
		}
		japaneseTokenizer = t
	})
	return japaneseTokenizer
}

// RomanizeJapanese romanizes s via morphological analysis, reporting false if any token's reading can't be resolved.
func RomanizeJapanese(s string) (string, bool) {
	tokens := getJapaneseTokenizer().Tokenize(s)
	readings := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if reading, ok := tok.Reading(); ok && reading != "" {
			readings = append(readings, reading)
			continue
		}
		if isASCII(tok.Surface) {
			readings = append(readings, tok.Surface)
			continue
		}
		return "", false
	}
	return RomanizeKana(strings.Join(readings, " "))
}

// isASCII reports whether every rune in s is in the ASCII range.
func isASCII(s string) bool {
	for _, r := range s {
		if r >= 128 {
			return false
		}
	}
	return true
}
