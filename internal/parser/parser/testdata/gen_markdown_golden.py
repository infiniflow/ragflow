#!/usr/bin/env python3
"""Regenerate internal/parser/parser/testdata/markdown.python.golden.json.

Drives the REAL Python markdown parser (deepdoc.parser.markdown_parser, the
same engine rag/flow/parser/parser.py:_markdown delegates to via
rag/app/naive.Markdown) so the golden is a faithful baseline rather than a
hand approximation.

Requires the project virtualenv (uv) because deepdoc needs markdown /
beartype / etc.:

    .venv/bin/python internal/parser/parser/testdata/gen_markdown_golden.py

It mirrors _markdown with separate_tables=False and the default delimiter
set, then assembles json items exactly as _markdown does:

  * each extracted section  -> {"text": <raw markdown section>, "doc_type_kwd": "text"}
  * each standalone table   -> {"text": <html table>,          "doc_type_kwd": "table"}
  * an image section        -> {"text": <alt>,                 "doc_type_kwd": "image"}

The Go alignment test then strips markdown syntax, html tags, and delimiters
before comparing, and ignores "table"/"image" items (those representations are
accepted divergences per PARSER_ALIGNMENT_HANDOFF.md §3.1).
"""

import json
import os
import re
import sys

# Make the repo root importable when run as a standalone script from testdata.
# Script lives at <repo>/internal/parser/parser/testdata/, so five dirname hops.
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))))

SAMPLE = "internal/parser/parser/testdata/markdown.sample.md"
OUT = "internal/parser/parser/testdata/markdown.python.golden.json"
DELIM = "\n!?;。；！？"
IMG_RE = re.compile(r"!\[([^\]]*)\]\(([^)]+)\)")
SENTINEL = "@@IMAGE@@"


def main():
    from deepdoc.parser.markdown_parser import RAGFlowMarkdownParser, MarkdownElementExtractor

    with open(SAMPLE, encoding="utf-8") as f:
        raw = f.read()

    # Model _markdown's return_section_images: the image is extracted as its
    # own item (alt text only). Replace the markdown with a delimiter-free
    # sentinel so the extractor does not split it.
    alts = []

    def _repl(m):
        alts.append(m.group(1))
        return SENTINEL

    prepared = IMG_RE.sub(_repl, raw)

    parser = RAGFlowMarkdownParser()
    remainder, tables = parser.extract_tables_and_remainder(prepared + "\n", separate_tables=False)
    extractor = MarkdownElementExtractor(remainder)
    sections = extractor.extract_elements(DELIM, include_meta=True)

    items = []
    for s in sections:
        content = s["content"]
        if SENTINEL in content:
            items.append({"text": alts.pop(0), "doc_type_kwd": "image"})
            continue
        items.append({"text": content, "doc_type_kwd": "text"})

    for tbl in tables:
        # _markdown (rag/flow/parser/parser.py:1103-1111) appends each
        # extracted table as a duplicate "table" item even when inlined,
        # carrying the table's raw text (GFM source or raw <table> HTML).
        items.append({"text": tbl, "doc_type_kwd": "table"})

    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(items, f, ensure_ascii=False, indent=2)
    print("wrote %d items to %s" % (len(items), OUT))


if __name__ == "__main__":
    sys.exit(main())
