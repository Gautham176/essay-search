#!/usr/bin/env bash
#
# Bootstrap: from a fresh clone to a working keyword search engine.
#
# Steps:
#   1. Start the Docker stack (Postgres + Go server)
#   2. Scrape the corpus from paulgraham.com (~5 min, skipped if cached)
#   3. Ingest documents into Postgres
#   4. Build the inverted index
#
# Skips the embedding step by design. Semantic search requires a local
# Ollama install; instructions are in README.md and printed at the end.
#
# Usage: bash scripts/bootstrap.sh
#        or: make demo

set -euo pipefail

# Move to repo root so all relative paths work regardless of where the
# script was invoked from.
cd "$(dirname "$0")/.."

# Pretty step headers. Detected color terminals get color; everyone else
# gets plain text.
if [ -t 1 ] && command -v tput >/dev/null && [ "$(tput colors 2>/dev/null || echo 0)" -ge 8 ]; then
    bold=$(tput bold); blue=$(tput setaf 4); green=$(tput setaf 2); reset=$(tput sgr0)
else
    bold=""; blue=""; green=""; reset=""
fi
step() { echo; echo "${bold}${blue}==>${reset} ${bold}$*${reset}"; }
done_msg() { echo "${green}    done.${reset}"; }

# --- Preflight: check the tools we need are present ----------------------
step "Checking prerequisites"
for cmd in docker go python3; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "    ERROR: '$cmd' is required but not installed."
        exit 1
    fi
done
if ! docker compose version >/dev/null 2>&1; then
    echo "    ERROR: 'docker compose' (v2) is required."
    exit 1
fi
done_msg

# --- 1. Docker stack -----------------------------------------------------
step "Starting Docker stack (Postgres + server)"
docker compose up -d --build

# Wait for Postgres to be ready. The healthcheck in docker-compose.yml
# usually has us covered, but we double-check from the host because the
# next steps connect to the DB directly.
echo "    waiting for Postgres to accept connections..."
for i in {1..30}; do
    if docker compose exec -T db pg_isready -U essay_search -d essay_search >/dev/null 2>&1; then
        break
    fi
    sleep 1
    if [ "$i" = 30 ]; then
        echo "    ERROR: Postgres did not become ready within 30s."
        docker compose logs db
        exit 1
    fi
done
done_msg

# --- 2. Corpus -----------------------------------------------------------
# Skip if we already have a populated corpus directory. The scraper is
# polite (1 req/sec) and takes about 5 minutes, so don't re-fetch unless
# we have to.
if [ -d corpus/clean ] && [ "$(ls -A corpus/clean 2>/dev/null | wc -l)" -gt 10 ]; then
    step "Corpus already present, skipping scrape"
    echo "    ($(ls corpus/clean | wc -l) essays found in corpus/clean/)"
    done_msg
else
    step "Scraping Paul Graham's essays (about 5 minutes)"
    pushd scripts >/dev/null

    # Create venv if missing; reuse if present.
    if [ ! -d .venv ]; then
        python3 -m venv .venv
    fi
    # shellcheck source=/dev/null
    source .venv/bin/activate
    pip install -q requests beautifulsoup4
    python scrape_pg.py --out ../corpus/clean
    deactivate
    popd >/dev/null
    done_msg
fi

# --- 3. Ingest + 4. Index ------------------------------------------------
# The container Postgres is on host port 5433 (mapped in docker-compose.yml).
# We connect from the host because our Go binaries live on the host;
# putting them in the container would mean rebuilding the image for
# every code change during development.
export DATABASE_URL="postgres://essay_search:dev_only_password@localhost:5433/essay_search?sslmode=disable"

step "Ingesting documents into Postgres"
go run ./cmd/ingest -dsn "$DATABASE_URL"
done_msg

step "Building the inverted index"
go run ./cmd/buildindex -dsn "$DATABASE_URL"
done_msg

# --- Restart server so it re-reads corpus stats --------------------------
step "Restarting server with fresh data"
docker compose restart serve >/dev/null
sleep 2
done_msg

# --- Sanity check --------------------------------------------------------
step "Verifying the server is responding"
if curl -sf http://localhost:8080/health >/dev/null; then
    docs=$(curl -s http://localhost:8080/health | grep -oP '"total_docs":\K[0-9]+' || echo "?")
    echo "    health check OK ($docs documents indexed)"
else
    echo "    WARNING: server didn't respond to /health. Check 'docker compose logs serve'."
fi
done_msg

# --- Summary -------------------------------------------------------------
cat <<EOF

${bold}${green}Bootstrap complete.${reset}

${bold}Open${reset}  http://localhost:8080
${bold}Try${reset}   "lisp"  or  "startup ideas"  in keyword mode.

Semantic and hybrid modes require a local Ollama install. To enable them:
  1. Install Ollama:        https://ollama.com/download
  2. Pull the model:        ollama pull nomic-embed-text
  3. Generate embeddings:   DATABASE_URL="\$DATABASE_URL" go run ./cmd/embed
  4. Restart the server:    docker compose restart serve

To tear everything down:
  make down       # stop containers, keep data
  make clean      # stop containers AND wipe data + corpus
EOF