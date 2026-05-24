package tokenize

import (
	"strings"
	"unicode"
)

// Stopwords is the set of common English words we ignore during indexing.
// These words appear in almost every document so they carry no signal.
var Stopwords = map[string]bool{
    // articles, conjunctions, prepositions
    "a": true, "an": true, "the": true,
    "and": true, "or": true, "but": true, "nor": true, "yet": true,
    "in": true, "on": true, "at": true, "to": true, "for": true,
    "of": true, "with": true, "by": true, "from": true, "as": true,
    "into": true, "onto": true, "upon": true, "over": true, "under": true,
    "about": true, "against": true, "between": true, "through": true,
    "during": true, "before": true, "after": true, "above": true, "below": true,

    // be / have / do / modals
    "is": true, "was": true, "are": true, "were": true, "be": true,
    "been": true, "being": true, "am": true,
    "have": true, "has": true, "had": true, "having": true,
    "do": true, "does": true, "did": true, "doing": true, "done": true,
    "will": true, "would": true, "could": true, "should": true,
    "may": true, "might": true, "must": true, "can": true, "shall": true,

    // pronouns / determiners
    "it": true, "its": true, "this": true, "that": true, "these": true,
    "those": true, "i": true, "you": true, "he": true, "she": true,
    "we": true, "they": true, "them": true, "us": true, "him": true, "her": true,
    "my": true, "your": true, "our": true, "their": true, "his": true,
    "what": true, "which": true, "who": true, "whom": true, "whose": true,

    // common adverbs / connectives
    "not": true, "no": true, "so": true, "if": true, "then": true,
    "than": true, "such": true, "also": true, "just": true, "very": true,
    "more": true, "most": true, "less": true, "least": true,
    "there": true, "here": true, "when": true, "where": true, "why": true,
    "how": true, "all": true, "any": true, "some": true, "each": true,
    "every": true, "other": true, "another": true, "same": true,
    "like": true, "only": true, "own": true, "even": true,
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