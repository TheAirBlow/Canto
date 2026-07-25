package romanize

import (
	"strings"

	"github.com/mozillazg/go-pinyin"
)

// RomanizePinyin converts s to pinyin, reporting false if any rune is neither hanzi nor ASCII.
func RomanizePinyin(s string) (string, bool) {
	ok := true
	args := pinyin.NewArgs()
	args.Fallback = func(r rune, _ pinyin.Args) []string {
		if r < 128 {
			return []string{string(r)}
		}
		ok = false
		return []string{""}
	}

	parts := pinyin.LazyPinyin(s, args)
	if !ok {
		return "", false
	}
	return strings.Join(parts, " "), true
}
