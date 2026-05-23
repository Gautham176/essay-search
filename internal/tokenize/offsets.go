package tokenize

import (
	"strings"
	"unicode"

	stemmer "github.com/blevesearch/go-porterstemmer"
)

// TokenWithOffset records a normalized token alongside its byte offsets
// in the *original* text. Offsets are inclusive-start, exclusive-end
// (the same convention as Go's string slicing: text[Start:End]).
type TokenWithOffset struct {
	Stemmed string // the normalized + stemmed form, suitable for matching
	Start   int    // byte offset of the raw token in the original text
	End     int    // byte offset just past the raw token
}

// TokenizeWithOffsets scans text once, identifying word-like runs and
// returning a TokenWithOffset for each one that survives normalization.
//
// Offsets are byte offsets, not rune offsets. This is what you want for
// slicing back into the original string: original[t.Start:t.End] gives
// you the raw form of the token as it appeared.
func TokenizeWithOffsets(text string) []TokenWithOffset {
	var tokens []TokenWithOffset

	// We walk the text rune-by-rune, accumulating a "raw token" whenever
	// we're inside a word, and emitting it when we hit a separator.
	//
	// range over a string yields (byteOffset, rune) — exactly what we
	// need to track where each token started in the original.
	tokenStart := -1 // -1 means "not currently inside a token"

	for i, r := range text {
		if isWordChar(r) {
			if tokenStart == -1 {
				tokenStart = i
			}
			continue
		}
		// Hit a separator. If we were building a token, emit it.
		if tokenStart != -1 {
			raw := text[tokenStart:i]
			
			lowered := strings.ToLower(raw)
            normalized := normalizeWord(lowered)

            if len(normalized) >= 2 && !Stopwords[normalized] {
                stemmed := stemString(normalized)
                tokens = append(tokens, TokenWithOffset{
                    Stemmed: stemmed,
                    Start:   tokenStart,
                    End:     i,
                })
            }

			tokenStart = -1
		}
	}
	// Handle a token at the very end of the text (no trailing separator).
	if tokenStart != -1 {
		raw := text[tokenStart:len(text)]
        
        lowered := strings.ToLower(raw)
        normalized := normalizeWord(lowered)

        if len(normalized) >= 2 && !Stopwords[normalized] {
            stemmed := stemString(normalized)
            tokens = append(tokens, TokenWithOffset{
                Stemmed: stemmed,
                Start:   tokenStart,
                End:     len(text),
            })
        }
	}

	return tokens
}

// isWordChar returns true for characters that can be inside a word:
// letters, digits, and apostrophes (so "don't" stays one token).
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\''
}

// stemString is a small helper: same as the stemmer call you used in Tokenize.
func stemString(s string) string {
	return string(stemmer.Stem([]rune(s)))
}