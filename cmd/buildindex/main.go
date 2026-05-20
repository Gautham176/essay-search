// Command buildindex reads documents from Postgres, tokenizes them,
// and populates the `terms` and `postings` tables.
//
// Idempotent: truncates the index tables before rebuilding. Safe to re-run.
//
// Usage:
//
//	go run ./cmd/buildindex
package main

import (
	"database/sql"
	"flag"
	"log"
	"os"

	"github.com/Gautham176/essay-search/internal/tokenize"
	"github.com/jackc/pgx/v5/pgtype"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// postingInfo holds index entries for one (term, doc) pair as we build
// the index in memory.
type postingInfo struct {
	freq      int
	positions []int
}

// index is the in-memory inverted index, built before we write to Postgres.
// Outer key: term. Inner key: doc_id. Value: frequency and positions.
type index map[string]map[int]*postingInfo

func main() {
	dsn := flag.String("dsn", defaultDSN(), "Postgres connection string")
	flag.Parse()

	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	log.Printf("loading documents...")
	docs, err := loadDocuments(db)
	if err != nil {
		log.Fatalf("load documents: %v", err)
	}
	log.Printf("loaded %d documents", len(docs))

	log.Printf("building in-memory index...")
	idx := buildIndex(docs)
	log.Printf("indexed %d unique terms", len(idx))

	log.Printf("writing to postgres...")
	if err := writeIndex(db, idx); err != nil {
		log.Fatalf("write index: %v", err)
	}
	log.Printf("done")
}

func defaultDSN() string {
	if env := os.Getenv("DATABASE_URL"); env != "" {
		return env
	}
	return "postgres:///essay_search"
}

// document is what we need from the documents table to build the index.
type document struct {
	id   int
	body string
}

// loadDocuments reads all documents from the DB into memory.
// At 173 docs this is fine; for millions you'd stream.
func loadDocuments(db *sql.DB) ([]document, error) {
	rows, err := db.Query(`SELECT id, body FROM documents ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []document
	for rows.Next() {
		var d document
		if err := rows.Scan(&d.id, &d.body); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// buildIndex tokenizes every document and builds the in-memory inverted index.
// This is the core IR logic — write it yourself.
func buildIndex(docs []document) index {
	idx := make(index)

	for _, doc := range docs {
		tokens := tokenize.Tokenize(doc.body)

		// TODO: for each (position, token) pair, update idx[token][doc.id].
		// Remember:
		//   - If idx[token] doesn't exist yet, you need to create the inner map.
		//   - If idx[token][doc.id] doesn't exist yet, you need to create a *postingInfo.
		//   - Then increment freq and append the position.
		//
		// Hint: Go's `range` over a slice gives you (index, value).
		// Hint: check map keys with the comma-ok idiom: `if _, ok := m[k]; !ok { ... }`

		for pos, tok := range tokens{
			if _, ok := idx[tok]; !ok {
    			idx[tok] = make(map[int]*postingInfo)
			}
			if _, ok := idx[tok][doc.id]; !ok {
				idx[tok][doc.id] = &postingInfo{}
			}
			p := idx[tok][doc.id]
			p.freq++
			p.positions = append(p.positions, pos)
		}
	}

	return idx
}

// writeIndex truncates the index tables and writes the new index in one
// transaction. If anything fails, the whole thing rolls back — no half-built
// index.
func writeIndex(db *sql.DB, idx index) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	// If we return early due to error, this rollback runs. If we Commit
	// successfully below, Rollback becomes a no-op.
	defer tx.Rollback()

	// Wipe the existing index. CASCADE on the foreign keys means truncating
	// terms also clears postings.
	if _, err := tx.Exec(`TRUNCATE terms RESTART IDENTITY CASCADE`); err != nil {
		return err
	}

	// Insert one term at a time, then its postings.
	// For 10-20k terms this is fast enough. If it weren't, we'd use COPY.
	insertTerm := `INSERT INTO terms (term, doc_count) VALUES ($1, $2) RETURNING id`
	insertPosting := `INSERT INTO postings (term_id, doc_id, term_freq, positions) VALUES ($1, $2, $3, $4)`

	for term, docMap := range idx {
		var termID int
		err := tx.QueryRow(insertTerm, term, len(docMap)).Scan(&termID)
		if err != nil {
			return err
		}

		for docID, info := range docMap {
			// pgtype.Array wraps a Go []int so the pgx driver knows to send
			// it as a Postgres INTEGER[] array.
			positions := pgtype.Array[int32]{
				Elements: toInt32(info.positions),
				Dims:     []pgtype.ArrayDimension{{Length: int32(len(info.positions)), LowerBound: 1}},
				Valid:    true,
			}
			if _, err := tx.Exec(insertPosting, termID, docID, info.freq, positions); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// toInt32 converts []int to []int32 for the pgx array type.
// (Postgres INTEGER is 32-bit; Go int is platform-dependent.)
func toInt32(xs []int) []int32 {
	out := make([]int32, len(xs))
	for i, x := range xs {
		out[i] = int32(x)
	}
	return out
}
