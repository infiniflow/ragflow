#!/usr/bin/env python3
"""Regenerate the Python text&code final-chunk parity goldens.

Run from the repository root with the supported RAGFlow Python environment:

    python internal/ingestion/component/chunker/tool-py/capture_textcode_golden.py

The production ``parser._code`` path delegates text&code parsing to
``RAGFlowTxtParser``. Calling that parser directly here keeps the fixture
focused on its real output and avoids constructing a graph or starting a
pipeline worker.
"""

from __future__ import annotations

import json
import importlib.util
import logging
import sys
import types
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parents[4]
SAMPLE_DIR = ROOT / "internal/parser/parser/testdata"
GOLDEN_DIR = ROOT / "internal/ingestion/component/chunker/testdata/textcode"
DELIMITER = "\n!?;。；！？"
TOKEN_SIZE = 128


def load_txt_parser():
    """Load the production text parser without importing unrelated parsers.

    ``deepdoc.parser.__init__`` eagerly registers every document parser. That
    broad registry is unnecessary for this fixture and pulls in optional
    native dependencies such as NumPy. Loading the two modules used by the
    text parser keeps regeneration deterministic and still executes the
    production ``RAGFlowTxtParser`` implementation.
    """
    sys.path.insert(0, str(ROOT))
    deepdoc = sys.modules.setdefault("deepdoc", types.ModuleType("deepdoc"))
    deepdoc.__path__ = [str(ROOT / "deepdoc")]
    parser_pkg = types.ModuleType("deepdoc.parser")
    parser_pkg.__path__ = [str(ROOT / "deepdoc/parser")]
    sys.modules["deepdoc.parser"] = parser_pkg
    for name in ("utils", "txt_parser"):
        module_name = f"deepdoc.parser.{name}"
        spec = importlib.util.spec_from_file_location(module_name, ROOT / "deepdoc/parser" / f"{name}.py")
        if spec is None or spec.loader is None:
            raise RuntimeError(f"cannot load {module_name}")
        module = importlib.util.module_from_spec(spec)
        sys.modules[module_name] = module
        spec.loader.exec_module(module)
    return sys.modules["deepdoc.parser.txt_parser"].RAGFlowTxtParser


def capture(name: str) -> dict:
    sample_path = SAMPLE_DIR / f"textcode.sample.{name}.txt"
    parser = load_txt_parser()
    sections = parser()(str(sample_path), sample_path.read_bytes(), TOKEN_SIZE, DELIMITER, True)
    return {
        "meta": {
            "generator": "deepdoc.parser.txt_parser.RAGFlowTxtParser (parser._code)",
            "sample": str(sample_path.relative_to(ROOT)),
            "delimiter": DELIMITER,
            "chunk_token_size": TOKEN_SIZE,
            "keep_delimiters": True,
            "comparison": "content-equivalent after delimiter removal and whitespace collapse",
        },
        "chunks": [{"text": text, "doc_type_kwd": "text"} for text, _ in sections if text],
    }


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    GOLDEN_DIR.mkdir(parents=True, exist_ok=True)
    for name in ("en", "zh"):
        path = GOLDEN_DIR / f"python.{name}.golden.json"
        result = capture(name)
        path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        logging.info("wrote %s (%d chunks)", path.relative_to(ROOT), len(result["chunks"]))


if __name__ == "__main__":
    main()
