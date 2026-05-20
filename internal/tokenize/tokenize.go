package tokenize

import (
	"strings"
	"unicode"

	stemmer "github.com/blevesearch/go-porterstemmer"
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

// Tokenize returns normalized tokens from text, in order of appearance.
// The pipeline is: lowercase → split → normalize → filter → stem.
func Tokenize(text string) []string {
	// Step 1: lowercase the entire text.
	// TODO: use strings.ToLower
	lowerText := strings.ToLower(text)

	// Step 2: split into raw tokens on whitespace and non-word characters.
	// Hint: strings.FieldsFunc lets you split on a custom function.
	// A character is a "split point" if it is not a letter, digit, or apostrophe.
	// TODO: use strings.FieldsFunc with a rune check (unicode.IsLetter, unicode.IsDigit)

	rawTokens := strings.FieldsFunc(lowerText, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
	})
	// Step 3: for each raw token, normalize and filter.
	var tokens []string
	for _, tok := range rawTokens {
		tok = normalizeWord(tok)

		// TODO: drop the token if length < 2
		if len(tok) < 2{
			continue
		}

		// TODO: drop the token if it is in Stopwords
		if Stopwords[tok]{
			continue
		}

		// Step 4: Porter stem the token.
		// The stemmer works on a []rune, not a string.
		// stemmer.Stem() takes a []rune and returns a []rune.
		stemmed := string(stemmer.Stem([]rune(tok)))

		tokens = append(tokens, stemmed)
	}

	return tokens
}

// normalizeWord cleans a single token:
//   - removes leading/trailing punctuation
//   - strips apostrophes (don't → dont)
//   - returns the cleaned string (may be empty)
func normalizeWord(word string) string {
	// TODO: remove apostrophes using strings.ReplaceAll
	word = strings.ReplaceAll(word, "'", "")
	
	// TODO: strip leading and trailing non-letter/non-digit characters.
	// Hint: strings.TrimFunc with a rune check works cleanly here.
	
	word = strings.TrimFunc(word, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	return word
}