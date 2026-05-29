package search

import (
	"context"
	"database/sql"
	"fmt"
	"errors"
	"log"
	"time"

	"github.com/Gautham176/essay-search/internal/tokenize"
	"github.com/Gautham176/essay-search/internal/snippet"
	"github.com/Gautham176/essay-search/internal/embed"
)

var ErrSemanticUnavailable = errors.New("semantic search unavailable: embedder cannot be reached")

// Result is a single ranked search result.
type Result struct {
	DocID   int     `json:"doc_id"`
	Title   string  `json:"title"`
	Author  string  `json:"author"`
	URL     string  `json:"url"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

// Engine holds long-lived state needed for search: the DB connection and
// a few corpus-level statistics that we cache at startup because they're
// expensive to recompute per query.
type Engine struct {
	db                 *sql.DB
	totalDocs          int
	avgDocLen          float64
	embedder           *embed.Client
	semanticAvailable  bool
}

// NewEngine connects to the corpus statistics and returns a ready-to-query
// Engine. Call this once at server startup, not per request.
func NewEngine(db *sql.DB) (*Engine, error) {
	e := &Engine{
		db:       db,
		embedder: embed.NewClient(),
	}
 
	row := db.QueryRow(`SELECT count(*), coalesce(avg(word_count), 0) FROM documents`)
	if err := row.Scan(&e.totalDocs, &e.avgDocLen); err != nil {
		return nil, fmt.Errorf("load corpus stats: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := e.embedder.Embed(ctx, "ping"); err != nil {
		log.Printf("embedder unreachable, semantic/hybrid disabled: %v", err)
		e.semanticAvailable = false
	} else {
		log.Printf("embedder reachable, semantic/hybrid enabled")
		e.semanticAvailable = true
	}
 
	return e, nil
}

// Stats returns corpus-level numbers, mostly useful for /health endpoints.
func (e *Engine) Stats() (totalDocs int, avgDocLen float64) {
	return e.totalDocs, e.avgDocLen
}

func (e *Engine) SemanticAvailable() bool {
	return e.semanticAvailable
}

// Search returns the top-k documents for the query, ranked by BM25.
// Returns an empty slice (not an error) for queries that match nothing.
func (e *Engine) Search(ctx context.Context, query string, k int) ([]Result, error) {
	terms := tokenize.Tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}

	// scores accumulates each doc's total BM25 score across all query terms.
	// We walk postings once per query term and add contributions doc by doc.
	scores := make(map[int]float64)

	for _, term := range terms {
		if err := e.scoreOneTerm(ctx, term, scores); err != nil {
			return nil, fmt.Errorf("scoring %q: %w", term, err)
		}
	}

	if len(scores) == 0 {
		return nil, nil
	}

	// Take top-k by score. For small result sets a full sort is fine;
	// for huge ones we'd use a heap, but that's premature here.
	topIDs := topKByScore(scores, k)

	return e.fetchResults(ctx, topIDs, scores, terms)
}

// scoreOneTerm pulls all postings for a single term and adds its BM25
// contribution to each containing document's running score.
func (e *Engine) scoreOneTerm(ctx context.Context, term string, scores map[int]float64) error {
	// One query gets us the term's doc_count (for IDF) joined to every
	// posting (for TF) joined to each doc's length (for normalization).
	// If the term doesn't exist in `terms`, the query returns zero rows
	// and we silently skip — same as production search engines do.
	const q = `
		SELECT t.doc_count, p.doc_id, p.term_freq, d.word_count
		FROM terms t
		JOIN postings p ON p.term_id = t.id
		JOIN documents d ON d.id = p.doc_id
		WHERE t.term = $1
	`
	rows, err := e.db.QueryContext(ctx, q, term)
	if err != nil {
		return err
	}
	defer rows.Close()

	var idf float64
	idfComputed := false

	for rows.Next() {
		var docCount, docID, tf, docLen int
		if err := rows.Scan(&docCount, &docID, &tf, &docLen); err != nil {
			return err
		}
		if !idfComputed {
			idf = IDF(e.totalDocs, docCount)
			idfComputed = true
		}
		scores[docID] += Score(tf, docLen, e.avgDocLen, idf)
	}
	return rows.Err()
}

// fetchResults pulls metadata for the top-k doc IDs and assembles Results.
// We do this as a single IN-list query so we don't round-trip per doc.
func (e *Engine) fetchResults(ctx context.Context, ids []int, scores map[int]float64, terms []string) ([]Result, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build a parameterized query with the right number of placeholders.
	// $1, $2, $3, ... — pgx requires this style; you can't pass a slice
	// directly to `WHERE id IN (?)`.
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
		`SELECT id, title, author, url, body FROM documents WHERE id IN (%s)`,
		string(placeholders),
	)
	rows, err := e.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect into a map first so we can iterate in score order at the end.
	byID := make(map[int]Result, len(ids))
	for rows.Next() {
		var r Result
		var body string
		if err := rows.Scan(&r.DocID, &r.Title, &r.Author, &r.URL, &body); err != nil {
			return nil, err
		}
		r.Score = scores[r.DocID]
		r.Snippet = snippet.Snippet(body, terms)
		byID[r.DocID] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Return in the ranked order produced by topKByScore.
	out := make([]Result, 0, len(ids))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}