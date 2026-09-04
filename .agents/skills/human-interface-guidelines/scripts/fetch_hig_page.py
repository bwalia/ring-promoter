"""Fetch and render an Apple HIG page (DocC JSON) to Markdown.

Apple's HIG site is served by a DocC-like JSON API under:
  https://developer.apple.com/tutorials/data/<path>.json

This script:
- Fetches that JSON
- Resolves references (doc:// identifiers, images, videos)
- Converts common DocC content blocks into Markdown

Examples:
  python fetch_hig_page.py --path "/design/human-interface-guidelines/buttons"
  python fetch_hig_page.py --url "https://developer.apple.com/design/human-interface-guidelines/buttons"
  python fetch_hig_page.py --path "/design/human-interface-guidelines/buttons" --out buttons.md

Notes:
- Output is best-effort; some rich layouts (multi-column rows) are simplified.
- For the authoritative presentation, always cite/open the canonical URL.
"""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass
from typing import Any
from urllib.parse import urlparse

import requests

BASE = "https://developer.apple.com"
DATA_BASE = "https://developer.apple.com/tutorials/data"


def abs_url(maybe_path: str) -> str:
    if maybe_path.startswith("http://") or maybe_path.startswith("https://"):
        return maybe_path
    if maybe_path.startswith("/"):
        return BASE + maybe_path
    return maybe_path


def normalize_path(path: str) -> str:
    # Accept full URL or path; strip query/fragment; ensure leading slash.
    if path.startswith("http://") or path.startswith("https://"):
        u = urlparse(path)
        path = u.path
    path = path.split("#", 1)[0].split("?", 1)[0]
    if not path.startswith("/"):
        path = "/" + path
    return path


def fetch_docc_json(path: str) -> dict[str, Any]:
    path = normalize_path(path)
    url = f"{DATA_BASE}{path}.json"
    try:
        r = requests.get(url, timeout=60, headers={"User-Agent": "Mozilla/5.0"})
        r.raise_for_status()
    except requests.exceptions.HTTPError as exc:
        status = exc.response.status_code if exc.response is not None else "unknown"
        raise RuntimeError(
            f"Failed to fetch HIG page JSON (HTTP {status}) from {url}. "
            "Check whether --path/--url is valid."
        ) from exc
    except requests.exceptions.RequestException as exc:
        raise RuntimeError(f"Network error while fetching {url}: {exc}") from exc

    try:
        return r.json()
    except ValueError as exc:
        raise RuntimeError(f"Received non-JSON response from {url}.") from exc


def choose_variant(ref: dict[str, Any]) -> str | None:
    variants = ref.get("variants") or []
    if not variants:
        return None

    # Prefer: 2x light, else first.
    def score(v: dict[str, Any]) -> int:
        traits = set(v.get("traits") or [])
        s = 0
        if "2x" in traits:
            s += 2
        if "light" in traits:
            s += 1
        return -s

    variants_sorted = sorted(variants, key=score)
    return variants_sorted[0].get("url")


@dataclass
class RenderCtx:
    references: dict[str, Any]

    def resolve_link(self, identifier: str) -> str | None:
        if not identifier:
            return None
        if identifier.startswith("http://") or identifier.startswith("https://"):
            return identifier
        # DocC topic identifier
        ref = self.references.get(identifier)
        if isinstance(ref, dict) and ref.get("url"):
            return abs_url(ref["url"])
        return None

    def resolve_media(self, identifier: str) -> str | None:
        ref = self.references.get(identifier)
        if isinstance(ref, dict):
            return choose_variant(ref)
        return None

    def media_alt(self, identifier: str) -> str:
        ref = self.references.get(identifier)
        if isinstance(ref, dict) and ref.get("alt"):
            return str(ref.get("alt"))
        return ""


def render_inline(node: Any, ctx: RenderCtx) -> str:
    if node is None:
        return ""

    if isinstance(node, str):
        return node

    if isinstance(node, list):
        return "".join(render_inline(x, ctx) for x in node)

    if not isinstance(node, dict):
        return str(node)

    t = node.get("type")

    if t == "text":
        return node.get("text", "")

    if t == "codeVoice":
        code = node.get("code", "")
        return f"`{code}`" if code else ""

    if t == "reference":
        identifier = node.get("identifier")
        title = node.get("overridingTitle")
        if not title:
            # sometimes overridingTitleInlineContent is present
            otic = node.get("overridingTitleInlineContent")
            title = render_inline(otic, ctx).strip() if otic else None
        if not title and identifier:
            # fall back to the referenced topic title
            ref = ctx.references.get(identifier)
            if isinstance(ref, dict) and ref.get("title"):
                title = str(ref.get("title"))
        if not title:
            title = identifier or "reference"

        url = ctx.resolve_link(identifier) or identifier
        return f"[{title}]({url})"

    if t == "strong":
        return f"**{render_inline(node.get('inlineContent'), ctx)}**"

    if t == "emphasis":
        return f"*{render_inline(node.get('inlineContent'), ctx)}*"

    # Fallback: try to render nested inlineContent
    if "inlineContent" in node:
        return render_inline(node.get("inlineContent"), ctx)

    return ""


def extract_text_from_block(block: Any, ctx: RenderCtx) -> str:
    # Used for table cell flattening
    if isinstance(block, dict):
        t = block.get("type")
        if t == "paragraph":
            return render_inline(block.get("inlineContent"), ctx).strip()
        if t == "heading":
            return (block.get("text") or "").strip()
        if t in ("unorderedList", "orderedList"):
            parts = []
            for it in block.get("items") or []:
                parts.append(extract_text_from_block(it.get("content") or [], ctx))
            return "; ".join([p for p in parts if p])
        if t == "aside":
            return extract_text_from_block(block.get("content") or [], ctx)
        if t == "row":
            parts = []
            for col in block.get("columns") or []:
                parts.append(extract_text_from_block(col.get("content") or [], ctx))
            return " | ".join([p for p in parts if p])
        if t == "table":
            return "[table]"
        if t in ("image", "video"):
            ident = block.get("identifier")
            return ident or t
        # best-effort
        if "content" in block:
            return extract_text_from_block(block.get("content"), ctx)
        if "inlineContent" in block:
            return render_inline(block.get("inlineContent"), ctx).strip()
        return ""

    if isinstance(block, list):
        return " ".join([extract_text_from_block(x, ctx) for x in block if extract_text_from_block(x, ctx)])

    return str(block).strip()


def render_block(block: dict[str, Any], ctx: RenderCtx) -> list[str]:
    t = block.get("type")

    if t == "heading":
        level = int(block.get("level", 2))
        level = max(1, min(level, 6))
        text = block.get("text", "")
        return [f"{'#' * level} {text}", ""]

    if t == "paragraph":
        # Image-only paragraph is common
        inline = block.get("inlineContent") or []
        if isinstance(inline, list) and len(inline) == 1 and isinstance(inline[0], dict) and inline[0].get("type") == "image":
            ident = inline[0].get("identifier")
            url = ctx.resolve_media(ident) or ""
            alt = ctx.media_alt(ident)
            if url:
                return [f"![{alt}]({url})", ""]
        text = render_inline(inline, ctx).strip()
        return [text, ""] if text else [""]

    if t == "unorderedList":
        lines: list[str] = []
        for item in block.get("items") or []:
            item_text = extract_text_from_block(item.get("content") or [], ctx)
            if item_text:
                lines.append(f"- {item_text}")
        lines.append("")
        return lines

    if t == "orderedList":
        lines: list[str] = []
        for i, item in enumerate(block.get("items") or [], start=1):
            item_text = extract_text_from_block(item.get("content") or [], ctx)
            if item_text:
                lines.append(f"{i}. {item_text}")
        lines.append("")
        return lines

    if t == "aside":
        name = block.get("name") or "Note"
        style = (block.get("style") or "note").lower()
        content = block.get("content") or []
        inner = []
        for c in content:
            if isinstance(c, dict) and c.get("type") == "paragraph":
                inner.append(render_inline(c.get("inlineContent"), ctx).strip())
            else:
                inner.append(extract_text_from_block(c, ctx))
        inner = [x for x in inner if x]
        prefix = "TIP" if style == "tip" else "NOTE"
        return [f"> **{prefix}: {name}**", *[f"> {x}" for x in inner], ""]

    if t == "codeListing":
        code = block.get("code", "")
        syntax = (block.get("syntax") or "").strip()
        fence = "```" + syntax if syntax else "```"
        return [fence, code.rstrip(), "```", ""]

    if t == "video":
        ident = block.get("identifier")
        url = ctx.resolve_media(ident) or ""
        alt = ctx.media_alt(ident)
        if url:
            return [f"[{alt or ident}]({url})", ""]
        return [f"[{ident}]", ""]

    if t == "row":
        # Multi-column layout; flatten each column.
        cols = block.get("columns") or []
        lines = ["> **Layout (columns)**"]
        for idx, col in enumerate(cols, start=1):
            col_txt = extract_text_from_block(col.get("content") or [], ctx)
            col_txt = re.sub(r"\s+", " ", col_txt).strip()
            if col_txt:
                lines.append(f"> - Col {idx}: {col_txt}")
        lines.append("")
        return lines

    if t == "table":
        rows = block.get("rows") or []
        if not rows:
            return []
        # Each row is a list of cells; each cell is list of blocks.
        rendered_rows = []
        for row in rows:
            rendered_cells = []
            for cell in row:
                cell_txt = extract_text_from_block(cell, ctx)
                cell_txt = re.sub(r"\s+", " ", cell_txt).strip()
                rendered_cells.append(cell_txt)
            rendered_rows.append(rendered_cells)

        header_mode = block.get("header")
        lines = []
        if header_mode == "row" and len(rendered_rows) >= 1:
            header = rendered_rows[0]
            body = rendered_rows[1:]
        else:
            header = [f"Column {i+1}" for i in range(len(rendered_rows[0]))]
            body = rendered_rows

        def fmt_row(r: list[str]) -> str:
            return "| " + " | ".join(r) + " |"

        lines.append(fmt_row(header))
        lines.append("| " + " | ".join(["---"] * len(header)) + " |")
        for r in body:
            lines.append(fmt_row(r))
        lines.append("")
        return lines

    # Unknown block: try some common fields
    if "inlineContent" in block:
        txt = render_inline(block.get("inlineContent"), ctx).strip()
        return [txt, ""] if txt else []

    if "content" in block:
        lines: list[str] = []
        for c in block.get("content") or []:
            if isinstance(c, dict):
                lines.extend(render_block(c, ctx))
        return lines

    return []


def render_page_md(data: dict[str, Any], canonical_url: str) -> str:
    md = data.get("metadata") or {}
    title = md.get("title") or "(Untitled)"

    refs = data.get("references") or {}
    ctx = RenderCtx(references=refs)

    abstract_items = data.get("abstract") or []
    abstract = "".join([it.get("text", "") for it in abstract_items if isinstance(it, dict)])

    out: list[str] = []
    out.append(f"# {title}")
    out.append("")
    out.append(f"- Canonical URL: {canonical_url}")
    out.append("")
    if abstract:
        out.append(abstract.strip())
        out.append("")

    # Render primary content
    pcs = data.get("primaryContentSections") or []
    for sec in pcs:
        if not isinstance(sec, dict):
            continue
        if sec.get("kind") != "content":
            continue
        for block in sec.get("content") or []:
            if isinstance(block, dict):
                out.extend(render_block(block, ctx))

    # Some pages also have additional `sections`
    for sec in data.get("sections") or []:
        if not isinstance(sec, dict):
            continue
        for block in sec.get("content") or []:
            if isinstance(block, dict):
                out.extend(render_block(block, ctx))

    # Final cleanup: remove excessive blank lines
    cleaned = []
    blank = 0
    for line in out:
        if line.strip() == "":
            blank += 1
            if blank <= 2:
                cleaned.append("")
        else:
            blank = 0
            cleaned.append(line.rstrip())

    return "\n".join(cleaned).strip() + "\n"


def main() -> None:
    try:
        ap = argparse.ArgumentParser()
        src = ap.add_mutually_exclusive_group(required=True)
        src.add_argument("--path", help="HIG path, e.g. /design/human-interface-guidelines/buttons")
        src.add_argument("--url", help="Canonical URL, e.g. https://developer.apple.com/design/human-interface-guidelines/buttons")
        ap.add_argument("--out", help="Write Markdown to file instead of stdout")
        args = ap.parse_args()

        path = normalize_path(args.path or args.url or "")
        canonical = abs_url(path)

        data = fetch_docc_json(path)
        md = render_page_md(data, canonical_url=canonical)

        if args.out:
            with open(args.out, "w", encoding="utf-8") as f:
                f.write(md)
        else:
            print(md)
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        raise SystemExit(1)


if __name__ == "__main__":
    main()
