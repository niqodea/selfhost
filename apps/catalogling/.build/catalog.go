package main

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Catalog is the library's shows, and the other names each goes by
type Catalog struct {
	Shows []string
	// The shows each other name means, by the key of the name
	Aliases map[string][]string
}

// Find returns the shows a name means: the show of that name, and the shows
// it is an alias of
func (c Catalog) Find(name string) []string {
	var found []string
	for _, show := range c.Shows {
		if key(show) == key(name) {
			found = append(found, show)
		}
	}
	for _, show := range c.Aliases[key(name)] {
		if slices.Contains(c.Shows, show) && !slices.Contains(found, show) {
			found = append(found, show)
		}
	}
	return found
}

// Similar returns up to 3 shows a name may mean but does not name, most
// similar first
func (c Catalog) Similar(name string) []string {
	type match struct {
		show  string
		score float64
	}
	var matches []match
	for _, show := range c.Shows {
		if key(show) == key(name) {
			continue
		}
		names := []string{show}
		for alias, shows := range c.Aliases {
			if slices.Contains(shows, show) {
				names = append(names, alias)
			}
		}
		best := 0.0
		for _, other := range names {
			if score, ok := similar(name, other); ok {
				best = max(best, score)
			}
		}
		if best > 0 {
			matches = append(matches, match{show, best})
		}
	}
	slices.SortStableFunc(matches, func(a, b match) int { return cmp.Compare(b.score, a.score) })
	var shows []string
	for _, m := range matches[:min(len(matches), 3)] {
		shows = append(shows, m.show)
	}
	return shows
}

// Spell returns the name as the library spells it, when it names a show
func (c Catalog) Spell(name string) string {
	for _, show := range c.Shows {
		if key(show) == key(name) {
			return show
		}
	}
	return name
}

// similar reports whether two names may mean the same show, though they do
// not name it the same, and how close they are, from 0 to 1. Names are
// similar when they differ by a typo, or when the words of one are among the
// words of the other, in order, as in Agents of SHIELD and Marvel's Agents
// of S.H.I.E.L.D. A word may stand for the initials of several, as in Law
// and Order SVU. The words must start where the other name starts, or make at
// least half of it, since a word found anywhere in a long name, like
// Deutschland in Wie clever ist Deutschland, is likely chance. A year at the
// end of a name, as in The Floor (2024), is left out, as key does
func similar(a, b string) (float64, bool) {
	wordsA, wordsB := words(trailingYearRe.ReplaceAllString(a, "")), words(trailingYearRe.ReplaceAllString(b, ""))
	keyA, keyB := strings.Join(wordsA, ""), strings.Join(wordsB, "")
	if keyA == "" || keyB == "" || keyA == keyB {
		return 0, false
	}
	short, long := keyA, keyB
	if len(long) < len(short) {
		short, long = long, short
	}
	// A typo per 6 letters, in a name long enough to tell one from a
	// different word
	if d := distance(short, long); len(short) >= 6 && d <= len(short)/6 {
		return 1 - float64(d)/float64(len(long)), true
	}
	if len(wordsB) < len(wordsA) {
		wordsA, wordsB = wordsB, wordsA
	}
	if !slices.ContainsFunc(wordsA, func(w string) bool { return !slices.Contains(stopWords, w) }) {
		return 0, false
	}
	first, matched, ok := inOrder(wordsA, wordsB)
	if !ok || first > firstWord(wordsB) && 2*matched < letters(wordsB) {
		return 0, false
	}
	return float64(len(short)) / float64(len(long)), true
}

var trailingYearRe = regexp.MustCompile(` \(?(?:19|20)\d{2}\)?$`)

// Words that alone never tell one show from another
var stopWords = []string{"the", "a", "an", "of", "and", "in", "on", "to", "de", "la", "le", "el", "les", "los", "der", "die", "das"}

// firstWord is the index of the first word that is not a stop word
func firstWord(words []string) int {
	for i, w := range words {
		if !slices.Contains(stopWords, w) {
			return i
		}
	}
	return 0
}

// letters counts the letters of the words that are not stop words
func letters(words []string) int {
	n := 0
	for _, w := range words {
		if !slices.Contains(stopWords, w) {
			n += len(w)
		}
	}
	return n
}

// inOrder reports whether each of the short words matches one or more of the
// long words, in order, and returns the first long word matched, and how
// many letters of the long words that are not stop words were matched
func inOrder(short, long []string) (first, matched int, ok bool) {
	j := 0
	first = -1
	for _, w := range short {
		for {
			if j >= len(long) {
				return 0, 0, false
			}
			if n := wordMatch(w, long[j:]); n > 0 {
				if first < 0 && !slices.Contains(stopWords, w) {
					first = j
				}
				matched += letters(long[j : j+n])
				j += n
				break
			}
			j++
		}
	}
	return first, matched, true
}

// wordMatch returns how many of the words at the start of words a word
// matches: one that is the same, or several it holds the initials of, as SVU
// for Special Victims Unit. Zero when it matches none. A word of 2 letters is
// often the initials of 2 words by chance
func wordMatch(w string, words []string) int {
	if w == words[0] {
		return 1
	}
	if len(w) < 3 || len(w) > len(words) {
		return 0
	}
	for i, r := range w {
		if !strings.HasPrefix(words[i], string(r)) {
			return 0
		}
	}
	return len(w)
}

// key compares names by their words only, so names that differ only in case,
// punctuation or spaces are the same. A year at the end of a name is left
// out, so Chicago Fire names Chicago Fire (2012), as no two shows of the
// library share a name
func key(name string) string {
	return strings.Join(words(trailingYearRe.ReplaceAllString(name, "")), "")
}

// words splits a name into lowercase words of letters and digits. An
// apostrophe joins a word, as in It's, & is and, and single letters in a row
// are a word, as in S.H.I.E.L.D.
func words(name string) []string {
	name = strings.NewReplacer("'", "", "’", "", "`", "", "&", " and ").Replace(strings.ToLower(name))
	var result []string
	letters := ""
	for _, w := range strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) == 1 && unicode.IsLetter([]rune(w)[0]) {
			letters += w
			continue
		}
		if letters != "" {
			result, letters = append(result, letters), ""
		}
		result = append(result, w)
	}
	if letters != "" {
		result = append(result, letters)
	}
	return result
}

// distance is the number of letters to add, remove, change or swap with the
// next to turn a into b
// Ref: https://en.wikipedia.org/wiki/Damerau%E2%80%93Levenshtein_distance
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			change := 1
			if ra[i-1] == rb[j-1] {
				change = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+change)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}
