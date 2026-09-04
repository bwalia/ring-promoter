"""Search Apple HIG pages (titles + abstracts) from local markdown catalog.

Examples:
  python scripts/search_hig.py "navigation bar" "toolbar"
  python scripts/search_hig.py --top 30 "apple pay"
"""

from __future__ import annotations

import argparse
import re
from pathlib import Path

CATALOG_PATH = Path(__file__).resolve().parents[1] / "references" / "hig_catalog.md"
LINE_RE = re.compile(r"^- \[(?P<title>[^\]]+)\]\((?P<url>https?://[^)]+)\)(?:\s+—\s+(?P<abstract>.*))?$")


def normalize(text: str) -> str:
    return re.sub(r"\s+", " ", (text or "").lower()).strip()


def parse_catalog(path: Path) -> list[dict[str, str]]:
    pages: list[dict[str, str]] = []
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        m = LINE_RE.match(line)
        if not m:
            continue
        pages.append(
            {
                "title": m.group("title") or "",
                "url": m.group("url") or "",
                "abstract": m.group("abstract") or "",
            }
        )
    return pages


def score(page: dict[str, str], terms: list[str]) -> int:
    title = normalize(page.get("title", ""))
    abstract = normalize(page.get("abstract", ""))
    s = 0
    for t in terms:
        s += 5 * title.count(t)
        s += 1 * abstract.count(t)
    return s


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("terms", nargs="+", help="keywords")
    ap.add_argument("--top", type=int, default=20)
    args = ap.parse_args()

    terms = [normalize(t) for t in args.terms]
    pages = parse_catalog(CATALOG_PATH)

    ranked: list[tuple[int, dict[str, str]]] = []
    for page in pages:
        s = score(page, terms)
        if s > 0:
            ranked.append((s, page))

    ranked.sort(key=lambda x: (-x[0], x[1].get("title", "")))

    for s, page in ranked[: args.top]:
        title = page.get("title", "")
        url = page.get("url", "")
        abstract = page.get("abstract", "").strip()
        if abstract:
            print(f"[{s:>3}] {title}\n      {url}\n      {abstract}\n")
        else:
            print(f"[{s:>3}] {title}\n      {url}\n")


if __name__ == "__main__":
    main()
