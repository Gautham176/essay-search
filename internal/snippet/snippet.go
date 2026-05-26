// Package snippet generates highlighted excerpts of matched documents.
package snippet

import (
	"strings"

	"github.com/Gautham176/essay-search/internal/tokenize"
)

// windowSize is the target snippet length in bytes. Real snippets may be
// slightly longer due to word-boundary snapping, slightly shorter near
// the ends of short documents.
const windowSize = 200

func Snippet(body string, queryTerms []string) string {
	if len(queryTerms) == 0 || body == "" {
		return Truncate(body, windowSize)
	}

	// Build a set for O(1) match lookup.
	wanted := make(map[string]bool, len(queryTerms))
	for _, t := range queryTerms {
		wanted[t] = true
	}

	// Tokenize body, keep only the matches.
	tokens := tokenize.TokenizeWithOffsets(body)
	var matches []tokenize.TokenWithOffset
	for _, t := range tokens {
		if wanted[t.Stemmed] {
			matches = append(matches, t)
		}
	}

	if len(matches) == 0 {
		return Truncate(body, windowSize)
	}

	// Find the best window: the one containing the most matches.
	winStart, winEnd := bestWindow(matches, len(body))

	// Snap to word boundaries so we don't slice mid-word.
	winStart, winEnd = snapToWordBoundaries(body, winStart, winEnd)

	// Build the marked snippet by walking matches inside the window.
	return buildMarked(body, winStart, winEnd, matches)
}


func bestWindow(matches []tokenize.TokenWithOffset, bodyLen int) (int, int) {
	bestI := 0
	maxCount := 0

	for i := 0; i < len(matches); i++ {
		start := matches[i].Start
		end := start + windowSize
		count := 0

		for j := i; j < len(matches); j++ {
			if matches[j].End <= end {
				count++
			} else {
				break // matches are sequential; subsequent ones will also be out of bounds
			}
		}

		if count > maxCount {
			maxCount = count
			bestI = i
		}
	}

	winStart := matches[bestI].Start
	winEnd := winStart + windowSize
	if winEnd > bodyLen {
		winEnd = bodyLen
	}

	return winStart, winEnd
}


func snapToWordBoundaries(body string, start, end int) (int, int) {
	for start > 0 && !isSpace(body[start-1]) {
		start--
	}
	for end < len(body) && !isSpace(body[end]) {
		end++
	}
	return start, end
}

// buildMarked walks matches that fall inside [winStart, winEnd) and
// builds the output string with <mark> tags wrapped around each match.
// Prepends/appends "…" if the window doesn't reach the start/end of body.
func buildMarked(body string, winStart, winEnd int, matches []tokenize.TokenWithOffset) string {
	var b strings.Builder

	if winStart > 0 {
		b.WriteString("…")
	}

	// Walk through the window, inserting <mark> tags at each match boundary.
	// cursor tracks how much of the window we've already written.
	cursor := winStart
	for _, m := range matches {
		// Skip matches outside or straddling the window.
		if m.Start < winStart || m.End > winEnd {
			continue
		}
		b.WriteString(body[cursor:m.Start])
		b.WriteString("<mark>")
		b.WriteString(body[m.Start:m.End])
		b.WriteString("</mark>")
		cursor = m.End
	}
	// Write the tail after the last match.
	b.WriteString(body[cursor:winEnd])

	if winEnd < len(body) {
		b.WriteString("…")
	}

	return b.String()
}

// Truncate returns the first n bytes of s, snapped to a word boundary,
// with a trailing "…" if it was cut.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Snap back to the last space at or before n.
	cut := n
	for cut > 0 && !isSpace(s[cut]) {
		cut--
	}
	if cut == 0 {
		cut = n // no space found, hard-cut
	}
	return s[:cut] + "…"
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}