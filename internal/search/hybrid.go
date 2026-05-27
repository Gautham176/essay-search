package search

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/Gautham176/essay-search/internal/snippet"
	"github.com/Gautham176/essay-search/internal/tokenize"
)

// RRFConstant is the smoothing constant in Reciprocal Rank Fusion.
// 60 is the value from Cormack et al. (2009) and the standard default.
// It controls how aggressively top ranks are favored vs. consensus across
// retrievers. Higher k = more consensus, less top-rank bias.
const RRFConstant = 60

// HybridSearch runs both keyword and semantic search in parallel, then
// fuses their rankings with Reciprocal Rank Fusion (RRF).
//
// Each retriever's contribution to a doc's score is 1/(k + rank); docs
// missing from a retriever contribute nothing (no penalty, no bonus).
// This rewards docs that appear in both retrievers without letting either
// retriever's score scale dominate.
func (e *Engine) HybridSearch(ctx context.Context, query string, k int) ([]Result, error) {
	// Over-fetch from each retriever: fusion only works if both have
	// enough candidates to overlap. 5*k or 50, whichever is larger.
	candidateN := k * 5
	if candidateN < 50 {
		candidateN = 50
	}

	// Run both searches in parallel. Both are IO-bound (SQL + Ollama),
	// so concurrency is a real win.
	var (
		keywordResults  []Result
		semanticResults []Result
		keywordErr      error
		semanticErr     error
		wg              sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		keywordResults, keywordErr = e.Search(ctx, query, candidateN)
	}()
	go func() {
		defer wg.Done()
		semanticResults, semanticErr = e.SemanticSearch(ctx, query, candidateN)
	}()
	wg.Wait()

	// If both retrievers failed, return the keyword error (arbitrary
	// choice; either error is informative). If only one failed, proceed
	// with the other's results — partial answer beats no answer.
	if keywordErr != nil && semanticErr != nil {
		return nil, fmt.Errorf("both retrievers failed: keyword=%v semantic=%v", keywordErr, semanticErr)
	}

	// Fuse with RRF. Walk each retriever's ranked list, adding 1/(k+rank)
	// to each doc's score. rank is 1-indexed.
	fusedScores := make(map[int]float64)
	for rank, r := range keywordResults {
		fusedScores[r.DocID] += 1.0 / float64(RRFConstant+rank+1)
	}
	for rank, r := range semanticResults {
		fusedScores[r.DocID] += 1.0 / float64(RRFConstant+rank+1)
	}

	if len(fusedScores) == 0 {
		return nil, nil
	}

	// Sort doc IDs by fused score descending. Ties broken by doc ID for
	// determinism (same as topKByScore in bm25 path).
	type pair struct {
		id    int
		score float64
	}
	pairs := make([]pair, 0, len(fusedScores))
	for id, s := range fusedScores {
		pairs = append(pairs, pair{id, s})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		return pairs[i].id < pairs[j].id
	})
	if len(pairs) > k {
		pairs = pairs[:k]
	}

	// Build a map of the keyword/semantic results so we can reuse their
	// snippets (rather than re-running snippet generation). Keyword
	// snippets have <mark> tags; semantic snippets have chunk text.
	// Prefer keyword when both exist.
	keywordByID := make(map[int]Result, len(keywordResults))
	for _, r := range keywordResults {
		keywordByID[r.DocID] = r
	}
	semanticByID := make(map[int]Result, len(semanticResults))
	for _, r := range semanticResults {
		semanticByID[r.DocID] = r
	}

	// Assemble final results.
	out := make([]Result, 0, len(pairs))
	terms := tokenize.Tokenize(query)
	for _, p := range pairs {
		var r Result
		if kr, ok := keywordByID[p.id]; ok {
			r = kr // metadata + highlighted snippet
		} else if sr, ok := semanticByID[p.id]; ok {
			r = sr // metadata only; snippet is chunk text
		}
		// If somehow the doc isn't in either map (shouldn't happen given
		// how we built fusedScores), skip it.
		if r.DocID == 0 {
			continue
		}
		// Replace per-retriever score with the fused score so callers
		// have a uniform interpretation.
		r.Score = p.score
		// If the result came from the semantic side only, its snippet
		// has no <mark> tags. We could re-snippet here for free using
		// the body, but the semantic chunk text is often a better
		// excerpt than what keyword snippeting would produce — it's
		// the actually-most-relevant passage from the doc. Keep it.
		_ = terms // unused in this path; kept for future improvements
		out = append(out, r)
	}
	return out, nil
}

var _ = snippet.Truncate