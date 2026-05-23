package tokenize

import (
	"strings"
	"unicode"
)

// Stopwords is the set of common English words we ignore during indexing.
// These words appear in almost every document so they carry no signal.
var Stopwords = map[string]bool{
	"a": true, "an": true, "the": true,
	"and": true, "or": true, "but": true, "nor": true,
	"in": true, "on": true, "at": true, "to": true, "for": true,
	"of": true, "with": true, "by": true, "from": true, "as": true,
	"is": true, "was": true, "are": true, "were": true, "be": true,
	"been": true, "being": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true,
	"it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "i": true, "you": true, "he": true, "she": true,
	"we": true, "they": true, "what": true, "which": true, "who": true,
	"not": true, "no": true, "so": true, "if": true, "then": true,
}

func Tokenize(text string) []string {
	toks := TokenizeWithOffsets(text)
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = t.Stemmed
	}
	return out
}

// normalizeWord cleans a single token:
//   - removes leading/trailing punctuation
//   - strips apostrophes (don't → dont)
//   - returns the cleaned string (may be empty)
func normalizeWord(word string) string {
	word = strings.ReplaceAll(word, "'", "")
	
	word = strings.TrimFunc(word, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	return word
}