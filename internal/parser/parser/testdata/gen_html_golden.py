"""Regenerate internal/parser/parser/testdata/html.python.golden.json.

Drives the REAL Python HTML parser (deepdoc.parser.html_parser.
RAGFlowHtmlParser, the same engine rag/flow/parser/parser.py:_html
delegates to) so the golden is a faithful baseline rather than a hand
approximation.

Requires the project virtualenv (uv) because deepdoc needs bs4 /
rag.nlp / etc.:

    .venv/bin/python internal/parser/parser/testdata/gen_html_golden.py

How the golden is built (mirrors gen_markdown_golden.py's
separate_tables=False behaviour, NOT the flow's chunk_block path):

  * read_text_recursively walks the DOM and yields one record per block in
    DOCUMENT ORDER, including <table> elements (kept inline, with their full
    <table>…</table> HTML as the record content).
  * We deliberately skip the flow's chunk_block(512) merging so the golden
    stays at parser-block granularity — chunking belongs to the Chunker, not
    the parser (PARSER_ALIGNMENT_HANDOFF.md §2.3).
  * Every record is emitted as {"text": <block>, "doc_type_kwd": "text"}.
    The <table> record is kept INLINE at its document position (this is what
    separate_tables=False does for markdown), so the alignment test actually
    exercises the table markup rather than dropping it via FilterByDocType.

The Go alignment test filters to doc_type_kwd:"text" on BOTH sides, strips
the heading marker / HTML tags / collapses whitespace, and compares. The Go
parser additionally emits a structured doc_type_kwd:"table" item (a downstream
signal Python does not); that extra item is excluded by the filter, exactly
like the markdown convention.
"""

import json
import os
import sys

# Make the repo root importable when run as a standalone script from testdata.
# Script lives at <repo>/internal/parser/parser/testdata/, so five dirname hops.
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))))

SAMPLE = "internal/parser/parser/testdata/html.sample.html"
OUT = "internal/parser/parser/testdata/html.python.golden.json"


def main():
    import bs4

    from deepdoc.parser.html_parser import TITLE_TAGS, RAGFlowHtmlParser

    with open(SAMPLE, "rb") as f:
        binary = f.read()
    txt = binary.decode("utf-8", errors="ignore")

    soup = bs4.BeautifulSoup(txt, "html.parser")
    for style in soup.find_all(["style", "script"]):
        style.decompose()
    root = soup.body or soup

    # Per-block records in DOCUMENT ORDER, tables kept inline.
    temp_sections = []
    RAGFlowHtmlParser.read_text_recursively(root, temp_sections, chunk_token_num=512)

    # Walk temp_sections, applying the heading prefix (TITLE_TAGS) and emitting
    # every block (including the inline <table> markup) as a text record.
    items = []
    for info in temp_sections:
        tag = info.get("tag_name")
        content = info.get("content", "")
        if tag in TITLE_TAGS:
            content = TITLE_TAGS[tag] + " " + content
        if content.strip():
            items.append({"text": content, "doc_type_kwd": "text"})

    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(items, f, ensure_ascii=False, indent=2)
    print(f"wrote {len(items)} items to {OUT}")


if __name__ == "__main__":
    sys.exit(main())
