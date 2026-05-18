// Command ingest reads markdown essays from a directory and inserts them
// into the Postgres `documents` table. Idempotent: re-running it updates
// existing rows by slug rather than creating duplicates.
//
// Usage:
//
//	go run ./cmd/ingest -dir corpus/clean
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// essay holds the parsed contents of one markdown file.
type essay struct {
	slug      string
	title     string
	author    string
	url       string
	body      string
	wordCount int
}

func main() {
	// Flags let you override defaults from the command line without rebuilding.
	dir := flag.String("dir", "corpus/clean", "directory containing essay markdown files")
	dsn := flag.String("dsn", defaultDSN(), "Postgres connection string")
	flag.Parse()

	// Connect to Postgres. sql.Open doesn't actually connect — it sets up
	// a connection pool. Ping() forces a real connection so we fail fast
	// if the DB is unreachable.
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	// Find every .md file in the corpus directory.
	pattern := filepath.Join(*dir, "*.md")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		log.Fatalf("glob %s: %v", pattern, err)
	}
	if len(paths) == 0 {
		log.Fatalf("no markdown files found in %s", *dir)
	}
	log.Printf("found %d essay files in %s", len(paths), *dir)

	// Parse + insert one essay at a time. For 173 docs this is plenty fast;
	// we'll batch later if needed.
	inserted, updated, failed := 0, 0, 0
	for _, p := range paths {
		e, err := parseEssay(p)
		if err != nil {
			log.Printf("parse %s: %v", p, err)
			failed++
			continue
		}
		wasInsert, err := upsertEssay(db, e)
		if err != nil {
			log.Printf("upsert %s: %v", e.slug, err)
			failed++
			continue
		}
		if wasInsert {
			inserted++
		} else {
			updated++
		}
	}

	log.Printf("done. inserted=%d updated=%d failed=%d", inserted, updated, failed)
}

// defaultDSN returns a Postgres connection string. On WSL with a peer-auth
// local user, this trivial DSN works because Postgres uses the OS username.
func defaultDSN() string {
	if env := os.Getenv("DATABASE_URL"); env != "" {
		return env
	}
	return "postgres:///essay_search"
}

// parseEssay reads a markdown file with YAML-ish frontmatter and returns
// an essay struct. The frontmatter is the minimal format produced by our
// scraper: lines like `title: "..."` between two `---` separators.
func parseEssay(path string) (essay, error) {
	f, err := os.Open(path)
	if err != nil {
		return essay{}, err
	}
	defer f.Close()

	var (
		e            essay
		inFrontmatter bool
		bodyLines    []string
		seenOpener   bool
	)

	scanner := bufio.NewScanner(f)
	// Default scanner buffer is 64KB; some essays are larger.
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			if !seenOpener {
				seenOpener = true
				inFrontmatter = true
				continue
			}
			// Closing fence.
			inFrontmatter = false
			continue
		}

		if inFrontmatter {
			key, val, ok := splitFrontmatterLine(line)
			if !ok {
				continue
			}
			switch key {
			case "title":
				e.title = val
			case "author":
				e.author = val
			case "url":
				e.url = val
			case "slug":
				e.slug = val
			}
			continue
		}

		// Outside frontmatter = body.
		if seenOpener {
			bodyLines = append(bodyLines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return essay{}, fmt.Errorf("scan: %w", err)
	}

	e.body = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	// Fallback slug: filename without extension. Shouldn't be needed since
	// the scraper writes a slug, but defensive.
	if e.slug == "" {
		base := filepath.Base(path)
		e.slug = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if e.title == "" {
		return essay{}, fmt.Errorf("missing title in %s", path)
	}
	if e.body == "" {
		return essay{}, fmt.Errorf("empty body in %s", path)
	}

	e.wordCount = len(strings.Fields(e.body))
	return e, nil
}

// splitFrontmatterLine parses lines like:  key: "value"
// Returns key, value, ok. Handles unquoted values too.
func splitFrontmatterLine(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx == -1 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	val := strings.TrimSpace(line[idx+1:])
	val = strings.Trim(val, `"`)
	return key, val, key != ""
}

// upsertEssay inserts the essay or updates it if the slug already exists.
// Returns (wasInsert, err): true for new row, false for update.
func upsertEssay(db *sql.DB, e essay) (bool, error) {
	// xmax = 0 on the returned row means it was an INSERT (no prior version).
	// xmax != 0 means it was an UPDATE. Postgres-specific but reliable.
	const q = `
		INSERT INTO documents (slug, title, author, url, body, word_count)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (slug) DO UPDATE SET
			title = EXCLUDED.title,
			author = EXCLUDED.author,
			url = EXCLUDED.url,
			body = EXCLUDED.body,
			word_count = EXCLUDED.word_count
		RETURNING (xmax = 0) AS was_insert
	`
	var wasInsert bool
	err := db.QueryRow(q, e.slug, e.title, e.author, e.url, e.body, e.wordCount).
		Scan(&wasInsert)
	return wasInsert, err
}