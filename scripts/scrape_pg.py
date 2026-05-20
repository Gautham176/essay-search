"""
Scrape Paul Graham's essays from paulgraham.com.

Usage:
    python scrape_pg.py --out ../corpus/clean

Output: one markdown file per essay in the output directory, with YAML
frontmatter (title, author, url, slug) followed by the essay body.
"""

import argparse
import re
import sys
import time
from pathlib import Path
from urllib.parse import urljoin

import requests
from bs4 import BeautifulSoup

BASE = "https://www.paulgraham.com/"
INDEX_URL = urljoin(BASE, "articles.html")
HEADERS = {
    "User-Agent": "essay-search-scraper/0.1 (educational project; contact: github.com/Gautham176)",
}
REQUEST_DELAY_SEC = 1.0
REQUEST_TIMEOUT_SEC = 20


def slugify(text: str) -> str:
    """Turn 'How to Do Great Work' -> 'how-to-do-great-work'."""
    text = text.lower().strip()
    text = re.sub(r"[^\w\s-]", "", text)
    text = re.sub(r"[\s_-]+", "-", text)
    return text.strip("-") or "untitled"


def fetch(url: str) -> str:
    """GET a URL and return decoded HTML, or raise."""
    resp = requests.get(url, headers=HEADERS, timeout=REQUEST_TIMEOUT_SEC)
    resp.raise_for_status()
    # PG's pages are latin-1-ish; let requests guess but fall back cleanly.
    resp.encoding = resp.apparent_encoding or "utf-8"
    return resp.text


def discover_essay_links(index_html: str) -> list[tuple[str, str]]:
    """
    Parse articles.html and return [(title, absolute_url), ...].

    PG's index is a flat list of <a href="essay.html">Title</a> entries
    inside a big table. We filter to links that look like essay pages
    (end in .html, not anchors or external links).
    """
    soup = BeautifulSoup(index_html, "html.parser")
    links: list[tuple[str, str]] = []
    seen: set[str] = set()

    for a in soup.find_all("a", href=True):
        href = a["href"].strip()
        title = a.get_text(strip=True)

        # Skip empty titles, anchors, mailto, external links.
        if not title or not href or href.startswith(("#", "mailto:", "javascript:")):
            continue
        # Only relative .html links on PG's own site.
        if href.startswith("http") and "paulgraham.com" not in href:
            continue
        if not href.endswith(".html"):
            continue
        # Skip the index page itself and a few known non-essay pages.
        if href in ("articles.html", "index.html", "rss.html"):
            continue

        absolute = urljoin(BASE, href)
        if absolute in seen:
            continue
        seen.add(absolute)
        links.append((title, absolute))

    return links


def extract_essay_text(essay_html: str) -> str:
    """
    Pull the essay body out of a PG essay page.

    PG's essays are wrapped in a quirky table layout from 2001-era HTML.
    The reliable pattern: the main text lives inside a <font> tag inside
    a table. We grab all text from the largest <font> block and strip
    the navigation/footer bits.
    """
    soup = BeautifulSoup(essay_html, "html.parser")

    # Strategy: find the <font> tag with the most text content.
    # PG's essay body is consistently the largest font block on the page.
    candidates = soup.find_all("font")
    if not candidates:
        # Fallback: just take body text.
        body = soup.find("body")
        return body.get_text("\n", strip=True) if body else ""

    best = max(candidates, key=lambda f: len(f.get_text(strip=True)))

    # Convert <br> to newlines, then get text.
    for br in best.find_all("br"):
        br.replace_with("\n")
    text = best.get_text("\n", strip=False)

    # Normalize whitespace: collapse runs of blank lines, strip trailing spaces.
    lines = [line.rstrip() for line in text.splitlines()]
    # Collapse 3+ blank lines into 2.
    cleaned: list[str] = []
    blank_run = 0
    for line in lines:
        if not line.strip():
            blank_run += 1
            if blank_run <= 2:
                cleaned.append("")
        else:
            blank_run = 0
            cleaned.append(line)

    return "\n".join(cleaned).strip()


def write_essay(out_dir: Path, title: str, url: str, body: str) -> Path:
    """Write one essay to disk as markdown with YAML frontmatter."""
    slug = slugify(title)
    path = out_dir / f"{slug}.md"

    # Escape quotes in title for the YAML frontmatter.
    safe_title = title.replace('"', '\\"')

    frontmatter = (
        "---\n"
        f'title: "{safe_title}"\n'
        f'author: "Paul Graham"\n'
        f'url: "{url}"\n'
        f'slug: "{slug}"\n'
        "---\n\n"
    )

    path.write_text(frontmatter + body + "\n", encoding="utf-8")
    return path


def main() -> int:
    parser = argparse.ArgumentParser(description="Scrape Paul Graham's essays.")
    parser.add_argument(
        "--out",
        type=Path,
        default=Path("corpus/clean"),
        help="Output directory for essay markdown files.",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=0,
        help="If > 0, only scrape the first N essays (for testing).",
    )
    parser.add_argument(
        "--skip-existing",
        action="store_true",
        help="Skip essays whose output file already exists.",
    )
    args = parser.parse_args()

    args.out.mkdir(parents=True, exist_ok=True)

    print(f"Fetching index: {INDEX_URL}", file=sys.stderr)
    try:
        index_html = fetch(INDEX_URL)
    except requests.RequestException as e:
        print(f"Failed to fetch index: {e}", file=sys.stderr)
        return 1

    links = discover_essay_links(index_html)
    print(f"Discovered {len(links)} essay links.", file=sys.stderr)
    if args.limit > 0:
        links = links[: args.limit]
        print(f"Limiting to first {len(links)} for testing.", file=sys.stderr)

    written = 0
    skipped = 0
    failed: list[tuple[str, str]] = []

    for i, (title, url) in enumerate(links, start=1):
        slug = slugify(title)
        out_path = args.out / f"{slug}.md"
        if args.skip_existing and out_path.exists():
            skipped += 1
            continue

        print(f"[{i}/{len(links)}] {title}", file=sys.stderr)
        try:
            html = fetch(url)
            body = extract_essay_text(html)
            if len(body) < 200:
                # Probably a stub page or extraction failure; log and skip.
                print(f"  -> body too short ({len(body)} chars), skipping", file=sys.stderr)
                failed.append((title, url))
            else:
                write_essay(args.out, title, url, body)
                written += 1
        except requests.RequestException as e:
            print(f"  -> fetch failed: {e}", file=sys.stderr)
            failed.append((title, url))
        except Exception as e:  # noqa: BLE001
            print(f"  -> unexpected error: {e}", file=sys.stderr)
            failed.append((title, url))

        time.sleep(REQUEST_DELAY_SEC)

    print(
        f"\nDone. Written: {written}, skipped: {skipped}, failed: {len(failed)}",
        file=sys.stderr,
    )
    if failed:
        print("Failed essays:", file=sys.stderr)
        for title, url in failed:
            print(f"  - {title}  {url}", file=sys.stderr)

    return 0 if written > 0 else 1


if __name__ == "__main__":
    sys.exit(main())