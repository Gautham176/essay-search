// Command embed reads documents from Postgres, chunks each body, generates
// an embedding per chunk via Ollama, and stores everything in the
// `embeddings` table.
//
// Idempotent: truncates the embeddings table before rebuilding. Safe to
// re-run. Embeddings are deterministic for a given model, so a re-run
// produces the same vectors.
//
// Usage:
//
//	go run ./cmd/embed
//
// Takes 5-15 minutes for a corpus of ~170 essays at default chunk sizes.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Gautham176/essay-search/internal/chunk"
	"github.com/Gautham176/essay-search/internal/embed"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := flag.String("dsn", defaultDSN(), "Postgres connection string")
	ollamaURL := flag.String("ollama", embed.DefaultBaseURL, "Ollama base URL")
	model := flag.String("model", embed.DefaultModel, "Ollama embedding model")
	chunkSize := flag.Int("chunk-size", chunk.DefaultChunkSize, "words per chunk")
	overlap := flag.Int("overlap", chunk.DefaultOverlap, "words shared between chunks")
	flag.Parse()

	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	client := embed.NewClient()
	client.BaseURL = *ollamaURL
	client.Model = *model

	// Sanity check: one quick call to fail fast if Ollama isn't reachable
	// or the model isn't pulled, rather than failing on the first document.
	log.Printf("checking ollama at %s with model %s...", client.BaseURL, client.Model)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if _, err := client.Embed(ctx, "ping"); err != nil {
		cancel()
		log.Fatalf("ollama check failed: %v", err)
	}
	cancel()

	log.Printf("loading documents...")
	docs, err := loadDocuments(db)
	if err != nil {
		log.Fatalf("load documents: %v", err)
	}
	log.Printf("loaded %d documents", len(docs))

	log.Printf("truncating embeddings table...")
	if _, err := db.Exec(`TRUNCATE embeddings RESTART IDENTITY`); err != nil {
		log.Fatalf("truncate: %v", err)
	}

	totalChunks := 0
	start := time.Now()

	// Process docs one at a time. We commit per document so a crash
	// halfway through doesn't lose all work — you can re-run and skip
	// docs that already have embeddings (we don't implement that here
	// since TRUNCATE + full rebuild is the simpler default, but the
	// per-doc transaction is the right shape).
	for i, doc := range docs {
		chunks := chunk.Chunk(doc.body, *chunkSize, *overlap)
		if len(chunks) == 0 {
			log.Printf("[%d/%d] %s: skipped (empty body)", i+1, len(docs), doc.title)
			continue
		}

		docStart := time.Now()
		if err := embedAndStore(context.Background(), db, client, doc.id, chunks); err != nil {
			log.Fatalf("[%d/%d] %s: %v", i+1, len(docs), doc.title, err)
		}
		totalChunks += len(chunks)
		log.Printf("[%d/%d] %s: %d chunks (%.1fs)",
			i+1, len(docs), doc.title, len(chunks), time.Since(docStart).Seconds())
	}

	log.Printf("done. %d documents, %d chunks, %.1fs total",
		len(docs), totalChunks, time.Since(start).Seconds())
}

func defaultDSN() string {
	if env := os.Getenv("DATABASE_URL"); env != "" {
		return env
	}
	return "postgres:///essay_search"
}

type document struct {
	id    int
	title string
	body  string
}

func loadDocuments(db *sql.DB) ([]document, error) {
	rows, err := db.Query(`SELECT id, title, body FROM documents ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []document
	for rows.Next() {
		var d document
		if err := rows.Scan(&d.id, &d.title, &d.body); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// embedAndStore embeds every chunk of one document and writes them to
// the embeddings table in a single transaction. Per-doc transactions
// strike a balance: each one is small enough to be cheap, but big
// enough that we don't pay commit overhead per chunk.
func embedAndStore(
	ctx context.Context,
	db *sql.DB,
	client *embed.Client,
	docID int,
	chunks []string,
) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const insertQ = `
		INSERT INTO embeddings (doc_id, chunk_index, chunk_text, embedding)
		VALUES ($1, $2, $3, $4)
	`
	for i, c := range chunks {
		vec, err := client.Embed(ctx, c)
		if err != nil {
			return fmt.Errorf("embed chunk %d: %w", i, err)
		}
		if _, err := tx.ExecContext(ctx, insertQ, docID, i, c, vectorLiteral(vec)); err != nil {
			return fmt.Errorf("insert chunk %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// vectorLiteral converts a []float32 to pgvector's string literal format:
// "[0.123,-0.456,...]" — pgvector parses this directly on insert.
//
// We send as a string rather than wrestling with pgx custom types. It's
// simpler, plenty fast for our size, and easy to debug.
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