package diagnostic

func Suggestion(value string, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	min := -1
	index := -1
	for i, candidate := range candidates {
		dist := Levenshtein([]rune(value), []rune(candidate))
		if min == -1 || dist < min {
			min = dist
			index = i
		}
	}
	failLimit := 1
	if len(value) > 3 {
		failLimit = 2
	}
	if min > failLimit {
		return ""
	}
	return candidates[index]
}

// Levenshtein returns the levenshtein distance between two rune arrays.
//
// This implementation translated from the optimized C code at
// https://en.wikibooks.org/wiki/Algorithm_Implementation/Strings/Levenshtein_distance#C
func Levenshtein(s1, s2 []rune) int {
	s1len := len(s1)
	s2len := len(s2)
	column := make([]int, len(s1)+1)

	for y := 1; y <= s1len; y++ {
		column[y] = y
	}
	for x := 1; x <= s2len; x++ {
		column[0] = x
		lastdiag := x - 1
		for y := 1; y <= s1len; y++ {
			olddiag := column[y]
			var incr int
			if s1[y-1] != s2[x-1] {
				incr = 1
			}

			column[y] = min3(column[y]+1, column[y-1]+1, lastdiag+incr)
			lastdiag = olddiag
		}
	}
	return column[s1len]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
	} else {
		if b < c {
			return b
		}
	}
	return c
}
