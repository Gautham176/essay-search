# Essay Search

A search engine over Paul Graham's essays. Implements classical (BM25),
semantic (vector embeddings), and hybrid (Reciprocal Rank Fusion)
retrieval from scratch in Go.

![Demo](docs/demo.gif)

## What this is

Type a query, get ranked PG essays back. Three retrieval modes that
disagree productively:

- **Keyword**: a from-scratch inverted index with Okapi BM25 scoring.
  Fast and precise on terms that appear literally — proper nouns,
  jargon, exact phrases.
- **Semantic**: each essay is chunked, embedded with a local Ollama model,
  and stored in pgvector. Queries are embedded the same way and ranked
  by cosine similarity. Strong on conceptual or vague queries where
  the words in the question don't appear in the essays.
- **Hybrid**: fuses both ranked lists with Reciprocal Rank Fusion (RRF).

Built as a summer project to learn information retrieval at
an implementation level, not just as theory.

## Try it

```bash
git clone https://github.com/Gautham176/essay-search.git
cd essay-search
make demo
```

About 7 minutes from clone to working search. Then open
http://localhost:8080. Semantic and hybrid modes require a local Ollama
install (see [Optional: enable semantic search](#optional-enable-semantic-search)
below); keyword mode works out of the box.

**Requires:** Docker, Go 1.26+, Python 3.10+.

## How it works

### Indexing pipeline
1. `scripts/scrape_pg.py` fetches essays from paulgraham.com as
   markdown with YAML frontmatter. Polite (1 req/sec) and cached.
2. `cmd/ingest` loads them into a Postgres `documents` table with
   metadata and per-essay word counts.
3. `cmd/buildindex` tokenizes each essay and builds the inverted index
   in `terms` + `postings` tables.
4. `cmd/embed` (optional) chunks each essay, embeds each chunk via
   Ollama, and stores the vectors in pgvector.

### Tokenization (`internal/tokenize`)
Lowercase → split on non-word characters → strip apostrophes →
drop tokens of length < 2 → drop ~90 English stopwords → Porter stem.
The same tokenizer runs at index time and query time, which matters:
stemming "running" → "run" at index time but not at query time would
silently miss matches.

There's also `TokenizeWithOffsets`, which keeps byte offsets into
the original text. Snippet highlighting needs to know where in the
raw text each match occurred, so the tokenizer must preserve that.

### Inverted index
Two tables. `terms` is the vocabulary with a precomputed `doc_count`
per term — denormalized so BM25's IDF doesn't need an aggregate query
per request. `postings` is one row per (term, doc) pair holding term
frequency and an `INTEGER[]` of positions for snippets. A btree on
`term_id` makes lookups O(log n).

For ~10k unique terms across ~180 documents, the index lives entirely
in Postgres without effort. At scale (10M+ docs) you'd want a custom
on-disk format with skip pointers and posting list compression —
mentioned here as a known trade-off, not a real concern at this size.

### BM25 scoring (`internal/search/bm25.go`)
Standard Okapi BM25 with k1=1.2, b=0.75:

```
                                  tf × (k1 + 1)
        score = idf × ───────────────────────────────────
                      tf + k1 × ((1 - b) + b × dl/avgdl)
```

The IDF term uses the smoothed form
`log((N - df + 0.5) / (df + 0.5) + 1)` so very common terms don't go
negative. Tests check properties — "rarer terms have higher IDF,"
"shorter docs score higher at equal TF," "scores show diminishing
returns" — rather than exact numeric outputs, so tuning k1 or b
doesn't break the suite.

### Semantic search (`internal/search/semantic.go`)
Each essay is split into ~300-word overlapping chunks (~50 word
overlap), and each chunk gets a 768-dimensional embedding from
`nomic-embed-text` via Ollama. Chunks live in a pgvector table with
an HNSW index over cosine distance.

At query time the query is embedded as natural language (no
tokenization — the model was trained on prose), nearest chunks are
retrieved with pgvector's `<=>` operator, and chunks are max-pooled
to document scores: each document's score is its best-matching
chunk's score. The over-fetch ratio is 5× the requested k, since
multiple chunks from the same document collapse to one result.

### Hybrid search (`internal/search/hybrid.go`)
Both retrievers run in parallel goroutines, then results fuse with
Reciprocal Rank Fusion: `score(doc) = Σ 1/(60 + rank_in_retriever)`.
Documents not in a retriever's list contribute nothing, which rewards
consensus without letting either retriever's score scale dominate.
The k=60 constant is the Cormack et al. (2009) default.

### Snippet generation (`internal/snippet`)
For each ranked doc, the body is re-tokenized with byte-offset
tracking to find match positions, then the densest ~200-byte window
is selected (the best match centers the window, snapped to word
boundaries). Matches are wrapped in `<mark>` tags. Algorithm is
O(matches²) per doc, which is fine for typical match counts.

### HTTP server (`cmd/serve`)
Standard library `net/http`. Endpoints:
- `GET /` — embedded HTML frontend
- `GET /search?q=...&mode=keyword|semantic|hybrid&k=N` — JSON results
- `GET /health` — corpus stats

The HTML is embedded into the binary at compile time via Go's `embed`
package, so the runtime artifact is one static binary plus the
database — no asset path tricks at deploy time.

## Evaluation

I built a small eval harness (`evals/queries.json` + `cmd/eval`) with
20 hand-labeled queries across three categories: keyword-favoring
(specific terms, proper nouns), semantic-favoring (vague conceptual),
and mixed. Each query has multiple acceptable "ideal" titles. Metrics:
P@1, P@5, and MRR.

| Mode     | P@1   | P@5   | MRR   |
|----------|-------|-------|-------|
| keyword  | 0.250 | 0.150 | 0.383 |
| semantic | 0.400 | 0.240 | 0.570 |
| hybrid   | 0.350 | 0.240 | 0.520 |

**Semantic beat both keyword and hybrid on this corpus.** That wasn't
what I expected — the standard finding is that hybrid wins — but PG's
essays are conceptually dense with high vocabulary repetition, which
suppresses BM25's IDF discrimination while playing to embeddings'
strengths. RRF's consensus-rewarding behavior then pulls semantic's
strong rankings *toward* keyword's weaker ones rather than away from
them. I swept the RRF k constant across 20/60/120 and MRR moved by
less than 0.02; the issue isn't fusion tuning.

This is the kind of result you only get by measuring. "Hybrid is better"
is a defensible default; "hybrid is better on this corpus" required
the eval.

## Design decisions

**Go, not Python.** The standard library is enough for an HTTP search
server, the type system catches a class of bugs Python wouldn't, and
the static binary deploys cleanly.

**Postgres + pgvector, not a dedicated vector DB.** One database for
documents, inverted index, and embeddings. A dedicated vector DB
would be more performant at scale but adds operational complexity
that isn't justified at 180 documents.

**No ORM.** Hand-written SQL with `database/sql` + `pgx/v5/stdlib`.
Forces engagement with the actual queries and indexes.

**Inverted index in Postgres, not on disk.** A custom on-disk format
matters at 10M+ documents; here, a normalized `postings` table with
a btree on `term_id` is correct and fast (~10-30ms per query).

**Schema and extension setup in versioned SQL migrations.** Anything
that needed to be set up by hand on the dev machine — `CREATE
EXTENSION vector` was the canonical example — is in a migration file.
The container database bootstraps identically to development.

**Max-pooling, not mean-pooling, for chunk-to-document scores.**
Essays are often about multiple topics; a user searching "procrastination"
wants the essay with the best passage on procrastination, not the
essay whose overall meaning is closest to the query.

## Corpus

~180 PG essays scraped from paulgraham.com. The scraper handles ~85%
of essays cleanly; the rest use a different page layout (a stub on
the index page that redirects to a separate file) which the scraper
doesn't follow. Skipping those was a deliberate trade-off: the engine
works fine on what's there.

After first run, `corpus/clean/` is cached locally and not re-fetched.

## Project structure

```
cmd/
  ingest/      load corpus into documents table
  buildindex/  tokenize docs and build inverted index
  embed/       chunk + embed each doc via Ollama
  serve/       HTTP search server with embedded frontend
  eval/        run labeled queries through all modes, compute metrics
internal/
  tokenize/    tokenizer with offset tracking + tests
  chunk/       overlapping word-window chunker + tests
  embed/       Ollama HTTP client
  search/      BM25, semantic, hybrid + tests
  snippet/     <mark>-wrapping highlighter + tests
migrations/    SQL schema as versioned files
scripts/       Python corpus scraper + bootstrap.sh
evals/         hand-labeled query set as JSON
docs/          demo GIF
web/           frontend, embedded into the serve binary
```

## Optional: enable semantic search

After `make demo`:

```bash
# Install Ollama: https://ollama.com/download

# Pull the embedding model
ollama pull nomic-embed-text

# Generate chunk embeddings for the corpus (~7 minutes)
DATABASE_URL="postgres://essay_search:dev_only_password@localhost:5433/essay_search?sslmode=disable" \
  go run ./cmd/embed

# Restart the server so it detects the embedder
docker compose restart serve
```

All three modes now work. Verify:

```bash
curl -s 'http://localhost:8080/search?q=how+do+i+find+work+i+love&mode=semantic' \
  | python3 -m json.tool | grep title | head -5
```

## What I'd do differently

- **A larger, more diverse corpus.** Hybrid's underperformance is
  partly an artifact of the corpus being homogeneous prose by a single
  author. A corpus mixing prose, code, and structured data would
  exercise the keyword/semantic complementarity better.
- **Better chunking.** Fixed-size word windows are fine; semantic
  paragraph boundaries would be better. Worth a comparison.
- **Re-snippet semantic-only results.** Hybrid results from the
  semantic-only path show raw chunk text instead of highlighted
  snippets. The keyword path's snippets are richer; ideally hybrid
  would re-snippet against the keyword tokenizer for any doc that has
  matching terms.
- **Approximate top-k.** Full sort of all candidates is fine at this
  size; at 100k+ docs you'd want a heap.

## Tests

```bash
make test       # or: go test ./...
```

Each non-trivial package has table-driven tests: tokenizer,
chunker, BM25, snippet. Properties over exact values where it
makes sense (BM25 scores) and exact equality where it doesn't
(tokenizer outputs).