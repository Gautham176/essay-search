// Package search implements ranked retrieval over the inverted index.
package search

import "math"

// BM25 tuning constants. Standard defaults; tunable against an eval set later.
const (
	K1 = 1.2
	B  = 0.75
)

// IDF computes inverse document frequency for one term.
//
//	idf = log((N - df + 0.5) / (df + 0.5) + 1)
//
// Where:
//
//	N  = total documents in the corpus
//	df = documents containing this term
//
// The +0.5 and +1 are the standard BM25 smoothing tweaks — they prevent
// log(0) for rare terms and prevent negative IDF for very common terms.
// (Plain log(N/df) goes negative when df > N/2.)

func IDF(totalDocs, docsContainingTerm int) float64 {

	x := (float64(totalDocs) - float64(docsContainingTerm) + 0.5) / (float64(docsContainingTerm) + 0.5) + 1

	idf := math.Log(x)

	return idf
}

// Score computes the BM25 contribution of one (query term, document) pair.
//
//	                      tf × (k1 + 1)
//	score = idf × ─────────────────────────────────
//	              tf + k1 × ((1 - b) + b × dl/avgdl)
//
// Parameters:
//
//	tf        = term frequency in this doc
//	docLen    = length of this doc, in tokens (use word_count)
//	avgDocLen = corpus-wide average doc length
//	idf       = precomputed IDF for the term

func Score(tf int, docLen int, avgDocLen float64, idf float64) float64 {

	lengthNorm := (1 - B) + B * (float64(docLen) / avgDocLen)

	numerator := float64(tf) * (K1 + 1)

	denominator := float64(tf) + K1 * lengthNorm

	return idf * (numerator / denominator)
}