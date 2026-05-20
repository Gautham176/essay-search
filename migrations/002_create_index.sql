-- One row per unique term across the entire corpus.
-- doc_count is denormalized for BM25's IDF calculation in Week 3.
CREATE TABLE IF NOT EXISTS terms (
    id          SERIAL PRIMARY KEY,
    term        TEXT NOT NULL UNIQUE,
    doc_count   INTEGER NOT NULL DEFAULT 0
);

-- The inverted index itself: one row per (term, document) pair.
-- positions is a Postgres int array — small enough for our corpus.
CREATE TABLE IF NOT EXISTS postings (
    term_id     INTEGER NOT NULL REFERENCES terms(id) ON DELETE CASCADE,
    doc_id      INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    term_freq   INTEGER NOT NULL,
    positions   INTEGER[] NOT NULL,
    PRIMARY KEY (term_id, doc_id)
);

-- The critical index: "given a term, find all postings fast."
-- This is what makes lookups O(log n) instead of O(n).
CREATE INDEX IF NOT EXISTS postings_term_idx ON postings (term_id);