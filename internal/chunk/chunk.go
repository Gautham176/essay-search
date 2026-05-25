// Package chunk splits documents into overlapping word windows suitable
// for embedding. Each chunk is small enough to fit in an embedding model's
// context window, and consecutive chunks overlap so that ideas spanning
// boundaries appear in both.
package chunk

import "strings"

// Default chunk parameters. These are reasonable for an essay corpus —
// ~300 words is a couple of paragraphs, ~50 words is enough overlap to
// preserve ideas that straddle a boundary.
const (
	DefaultChunkSize = 300
	DefaultOverlap   = 50
)

// Chunk splits text into overlapping word-based chunks.
//
// Parameters:
//
//	text       = the document body
//	chunkSize  = words per chunk
//	overlap    = words shared between consecutive chunks (must be < chunkSize)
//
// Returns a slice of strings, each ~chunkSize words. The last chunk may
// be shorter. Returns a single chunk if text is shorter than chunkSize.
// Returns nil for empty input.
func Chunk(text string, chunkSize, overlap int) []string {
	if text == "" || chunkSize <= 0 {
		return nil
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 2 // defensive: silently fix bad input
	}

	// Split on whitespace. strings.Fields handles any whitespace and
	// collapses runs of spaces/tabs/newlines into single separators.
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	// Short documents fit in one chunk.
	if len(words) <= chunkSize {
		return []string{strings.Join(words, " ")}
	}

	step := chunkSize - overlap

	var chunks []string

	for start := 0; start < len(words); start += step {
		end := start + chunkSize
		if end > len(words) {
			end = len(words)
		}

		chunks = append(chunks, strings.Join(words[start:end], " "))

		if end == len(words) {
			break
		}
	}

	return chunks
}