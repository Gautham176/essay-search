// Command serve runs the HTTP search API.
//
// Usage:
//
//	go run ./cmd/serve
//	# then:
//	curl 'http://localhost:8080/search?q=startup+ideas'
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Gautham176/essay-search/internal/search"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
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

	engine, err := search.NewEngine(db)
	if err != nil {
		log.Fatalf("init engine: %v", err)
	}
	totalDocs, avgLen := engine.Stats()
	log.Printf("engine ready: %d docs, avg length %.0f tokens", totalDocs, avgLen)

	mux := http.NewServeMux()
	mux.HandleFunc("/search", searchHandler(engine))
	mux.HandleFunc("/health", healthHandler(engine))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second, // basic protection against slowloris
	}

	log.Printf("listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func defaultDSN() string {
	if env := os.Getenv("DATABASE_URL"); env != "" {
		return env
	}
	return "postgres:///essay_search"
}

// searchResponse is the JSON envelope returned to clients. Wrapping the
// results in an object (vs returning a bare array) lets us add fields
// later without breaking clients.
type searchResponse struct {
	Query    string           `json:"query"`
	Count    int              `json:"count"`
	Results  []search.Result  `json:"results"`
	LatencyMs int             `json:"latency_ms"`
}

func searchHandler(engine *search.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
 
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, `missing "q" parameter`, http.StatusBadRequest)
			return
		}
 
		k := 10
		if v := r.URL.Query().Get("k"); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 1 || parsed > 100 {
				http.Error(w, `"k" must be an integer between 1 and 100`, http.StatusBadRequest)
				return
			}
			k = parsed
		}
 
		// Default to keyword search. Recognized modes: "keyword", "semantic".
		mode := r.URL.Query().Get("mode")
		if mode == "" {
			mode = "keyword"
		}
 
		var (
			results []search.Result
			err     error
		)
		switch mode {
		case "keyword":
			results, err = engine.Search(r.Context(), query, k)
		case "semantic":
			results, err = engine.SemanticSearch(r.Context(), query, k)
		default:
			http.Error(w, `"mode" must be "keyword" or "semantic"`, http.StatusBadRequest)
			return
		}
 
		if err != nil {
			log.Printf("search error (%s): %v", mode, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if results == nil {
			results = []search.Result{}
		}
 
		resp := searchResponse{
			Query:     query,
			Count:     len(results),
			Results:   results,
			LatencyMs: int(time.Since(start).Milliseconds()),
		}
 
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("encode response: %v", err)
		}
	}
}

func healthHandler(engine *search.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		totalDocs, avgLen := engine.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":      "ok",
			"total_docs":  totalDocs,
			"avg_doc_len": avgLen,
		})
	}
}

// logRequests is a middleware that prints one line per HTTP request
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %s", r.Method, r.URL.Path, r.URL.RawQuery, time.Since(start))
	})
}