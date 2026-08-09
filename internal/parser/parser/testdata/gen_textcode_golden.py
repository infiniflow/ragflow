#!/usr/bin/env python3
"""Regenerate internal/parser/parser/testdata/textcode.python.golden.json.

Drives the REAL Python text&code parser (deepdoc.parser.txt_parser, the same
engine rag/flow/parser/parser.py:_code delegates to) so the golden is a faithful
baseline rather than a hand approximation.

Requires the project virtualenv (uv) because deepdoc needs the parser deps:

    .venv/bin/python internal/parser/parser/testdata/gen_textcode_golden.py

It mirrors _code exactly: TxtParser is called with the default chunk_token_num
(128), the default delimiter set ("\\n!?;。；！？"), and keep_delimiters=True
(the flow _code path passes keep_delimiters=True even though the parser_txt
signature defaults to False). The merged sections are then projected to the same
json item shape the Go TextParser emits:

    [{"text": section[0], "doc_type_kwd": "text"}, ...]

The Go alignment test does NOT perform the OVER_CAP token merge (chunking
ownership stays with the Go Chunker per PARSER_ALIGNMENT_HANDOFF.md §2.3,
decision 1), so the golden is merged while the Go output is the finer
delimiter-split paragraphs. The stitch-compare alignment (align_test.go) joins
both sides on whitespace and strips delimiters, reconciling the item-count
difference.
"""

import json
import os
import sys

# Make the repo root importable when run as a standalone script from testdata.
# Script lives at <repo>/internal/parser/parser/testdata/, so five dirname hops.
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))))

SAMPLE = "internal/parser/parser/testdata/textcode.sample.txt"
OUT = "internal/parser/parser/testdata/textcode.python.golden.json"
DELIM = "\n!?;。；！？"


def main():
    from deepdoc.parser.txt_parser import RAGFlowTxtParser as TxtParser

    with open(SAMPLE, "rb") as f:
        blob = f.read()

    # Mirror rag/flow/parser/parser.py:_code: keep_delimiters=True.
    sections = TxtParser()(SAMPLE, blob, 128, DELIM, keep_delimiters=True)

    items = [{"text": s[0], "doc_type_kwd": "text"} for s in sections if s[0]]

    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(items, f, ensure_ascii=False, indent=2)
    print("wrote %d items to %s" % (len(items), OUT))


if __name__ == "__main__":
    sys.exit(main())
