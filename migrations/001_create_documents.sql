CREATE TABLE IF NOT EXISTS documents (
    id          SERIAL PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    title       TEXT NOT NULL,
    author      TEXT NOT NULL,
    url         TEXT NOT NULL,
    body        TEXT NOT NULL,
    word_count  INTEGER NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS documents_author_idx ON documents (author);