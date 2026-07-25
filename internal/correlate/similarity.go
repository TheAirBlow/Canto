package correlate

import (
	"sort"
	"strings"
)

// jaroWinkler returns the Jaro-Winkler similarity of a and b in [0, 1].
func jaroWinkler(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	jaro := jaroSimilarity(ra, rb)
	if jaro <= 0 {
		return jaro
	}

	prefix := 0
	for prefix < len(ra) && prefix < len(rb) && prefix < 4 && ra[prefix] == rb[prefix] {
		prefix++
	}
	return jaro + float64(prefix)*0.1*(1-jaro)
}

// jaroSimilarity returns the plain Jaro similarity of ra and rb in [0, 1].
func jaroSimilarity(ra, rb []rune) float64 {
	if len(ra) == 0 && len(rb) == 0 {
		return 1
	}
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}

	matchDist := max(len(ra), len(rb))/2 - 1
	if matchDist < 0 {
		matchDist = 0
	}

	aMatched := make([]bool, len(ra))
	bMatched := make([]bool, len(rb))
	matches := 0
	for i := range ra {
		lo, hi := max(0, i-matchDist), min(len(rb)-1, i+matchDist)
		for j := lo; j <= hi; j++ {
			if bMatched[j] || ra[i] != rb[j] {
				continue
			}
			aMatched[i], bMatched[j] = true, true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}

	transpositions := 0
	k := 0
	for i := range ra {
		if !aMatched[i] {
			continue
		}
		for !bMatched[k] {
			k++
		}
		if ra[i] != rb[k] {
			transpositions++
		}
		k++
	}

	m := float64(matches)
	return (m/float64(len(ra)) + m/float64(len(rb)) + (m-float64(transpositions)/2)/m) / 3
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

// editRatio returns a Levenshtein-based similarity of a and b in [0, 1].
func editRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1
	}
	dist := levenshtein(ra, rb)
	return 1 - float64(dist)/float64(max(len(ra), len(rb)))
}

// tokenize lowercases and splits s into its whitespace-separated tokens.
func tokenize(s string) []string {
	return strings.Fields(strings.ToLower(s))
}

// tokenSetRatio compares a and b by token overlap, tolerant of word reordering and subsets.
func tokenSetRatio(a, b string) float64 {
	ta, tb := tokenize(a), tokenize(b)
	if len(ta) == 0 && len(tb) == 0 {
		return 1
	}
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}

	setA := make(map[string]bool, len(ta))
	for _, t := range ta {
		setA[t] = true
	}
	setB := make(map[string]bool, len(tb))
	for _, t := range tb {
		setB[t] = true
	}

	var intersection, onlyA, onlyB []string
	for t := range setA {
		if setB[t] {
			intersection = append(intersection, t)
		} else {
			onlyA = append(onlyA, t)
		}
	}
	for t := range setB {
		if !setA[t] {
			onlyB = append(onlyB, t)
		}
	}
	sort.Strings(intersection)
	sort.Strings(onlyA)
	sort.Strings(onlyB)

	inter := strings.Join(intersection, " ")
	combinedA := strings.TrimSpace(inter + " " + strings.Join(onlyA, " "))
	combinedB := strings.TrimSpace(inter + " " + strings.Join(onlyB, " "))

	best := editRatio(inter, combinedA)
	if r := editRatio(inter, combinedB); r > best {
		best = r
	}
	if r := editRatio(combinedA, combinedB); r > best {
		best = r
	}
	return best
}

// bestSimilarity returns the higher of Jaro-Winkler and token-set-ratio similarity for a and b.
func bestSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	jw := jaroWinkler(strings.ToLower(a), strings.ToLower(b))
	ts := tokenSetRatio(a, b)
	return max(jw, ts)
}
