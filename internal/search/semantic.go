package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/Gautham176/essay-search/internal/snippet"
)

// SemanticSearch returns the top-k documents most semantically similar
// to the query. Aggregation is max-pooling: each document's score is
// determined by its best-matching chunk.
//
// Unlike Search, this does NOT tokenize the query. The embedding model
// expects natural language; stemming and stopword removal would destroy
// the semantic signal it relies on.
func (e *Engine) SemanticSearch(ctx context.Context, query string, k int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	// 1. Embed the query.
	queryVec, err := e.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// 2. Nearest-neighbor search. We over-fetch chunks because multiple
	// chunks from the same document will collapse to one result; if we
	// asked for only k chunks, we might end up with fewer than k unique
	// documents. 5× is a reasonable upper bound.
	chunks, err := e.nearestChunks(ctx, queryVec, k*5)
	if err != nil {
		return nil, fmt.Errorf("nearest chunks: %w", err)
	}
	if len(chunks) == 0 {
		return nil, nil
	}

	// 3. Max-pool to documents. Walk chunks in distance order (already
	// sorted by the SQL ORDER BY). The first chunk we see for each doc
	// is its best chunk, by construction.
	type docHit struct {
		docID       int
		bestChunk   string
		bestDist    float64
	}
	seen := make(map[int]bool)
	var hits []docHit
	for _, c := range chunks {
		if seen[c.docID] {
			continue
		}
		seen[c.docID] = true
		hits = append(hits, docHit{c.docID, c.text, c.distance})
		if len(hits) == k {
			break
		}
	}

	// 4. Fetch document metadata.
	ids := make([]int, len(hits))
	for i, h := range hits {
		ids[i] = h.docID
	}
	metaByID, err := e.fetchMetadata(ctx, ids)
	if err != nil {
		return nil, err
	}

	// 5. Assemble results in ranked order. Convert cosine distance to
	// a similarity score in [0, 1] where larger = better (matches BM25
	// convention).
	out := make([]Result, 0, len(hits))
	for _, h := range hits {
		meta, ok := metaByID[h.docID]
		if !ok {
			continue
		}
		out = append(out, Result{
			DocID:   h.docID,
			Title:   meta.title,
			Author:  meta.author,
			URL:     meta.url,
			Score:   1.0 - h.bestDist/2.0,
			// Snippet from the best-matching chunk. No <mark> highlighting
			// for now — semantic matches don't have term positions.
			Snippet: snippet.Truncate(h.bestChunk, 250),
		})
	}
	return out, nil
}

// chunkHit is one row from the nearest-neighbor query.
type chunkHit struct {
	docID    int
	text     string
	distance float64
}

// nearestChunks returns the N chunks nearest to queryVec, sorted by
// cosine distance ascending.
func (e *Engine) nearestChunks(ctx context.Context, queryVec []float32, n int) ([]chunkHit, error) {
	// pgvector's <=> operator returns cosine distance.
	// We pass the query vector as a string literal: pgvector parses it.
	const q = `
		SELECT doc_id, chunk_text, embedding <=> $1 AS dist
		FROM embeddings
		ORDER BY embedding <=> $1
		LIMIT $2
	`
	rows, err := e.db.QueryContext(ctx, q, vectorLiteral(queryVec), n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []chunkHit
	for rows.Next() {
		var h chunkHit
		if err := rows.Scan(&h.docID, &h.text, &h.distance); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// docMeta is the per-document data needed to build a Result.
type docMeta struct {
	title  string
	author string
	url    string
}

// fetchMetadata pulls title/author/url for a list of doc IDs.
func (e *Engine) fetchMetadata(ctx context.Context, ids []int) (map[int]docMeta, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]byte, 0, len(ids)*4)
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '$')
		placeholders = append(placeholders, []byte(fmt.Sprintf("%d", i+1))...)
		args[i] = id
	}

	q := fmt.Sprintf(
		`SELECT id, title, author, url FROM documents WHERE id IN (%s)`,
		string(placeholders),
	)
	rows, err := e.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int]docMeta, len(ids))
	for rows.Next() {
		var id int
		var m docMeta
		if err := rows.Scan(&id, &m.title, &m.author, &m.url); err != nil {
			return nil, err
		}
		out[id] = m
	}
	return out, rows.Err()
}

// vectorLiteral converts a []float32 to pgvector's "[0.1,-0.2,...]" format.
// Same helper as in cmd/embed; small enough to duplicate.
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 12)
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%g", x)
	}
	b.WriteByte(']')
	return b.String()
}