#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""Unit tests for HtmlParser.chunk_block.

These cover the splitting of oversized text blocks, which must preserve the
original source text (the tokenizer lowercases / stems / segments text, so the
stored chunk must not be built from the tokenized form) and must split text in
scripts that have no whitespace word boundaries (e.g. Chinese).
"""

import importlib.util
import json
import os
import sys
from pathlib import Path
from unittest import mock

import pytest
from bs4 import BeautifulSoup

# Load html_parser by file path so we don't trigger deepdoc/parser/__init__.py
# (which pulls in heavy parsers) or the real rag.nlp tokenizer. The heavy
# optional modules are stubbed; rag.nlp is stubbed so the module imports, and
# the tokenizer is replaced after load with a deterministic fake below.
_MOCK_MODULES = [
    "xgboost",
    "pdfplumber",
    "huggingface_hub",
    "PIL",
    "PIL.Image",
    "pypdf",
    "sklearn",
    "deepdoc.vision",
    "infinity",
    "infinity.rag_tokenizer",
]
for _m in _MOCK_MODULES:
    if _m not in sys.modules:
        sys.modules[_m] = mock.MagicMock()

if "rag" not in sys.modules:
    sys.modules["rag"] = mock.MagicMock()
if "rag.nlp" not in sys.modules:
    sys.modules["rag.nlp"] = mock.MagicMock()


def _find_project_root(marker="pyproject.toml"):
    d = os.path.dirname(os.path.abspath(__file__))
    while d != os.path.dirname(d):
        if os.path.exists(os.path.join(d, marker)):
            return d
        d = os.path.dirname(d)
    return None


_PROJECT_ROOT = _find_project_root()

_html_spec = importlib.util.spec_from_file_location(
    "deepdoc.parser.html_parser",
    os.path.join(_PROJECT_ROOT, "deepdoc", "parser", "html_parser.py"),
)
_html_mod = importlib.util.module_from_spec(_html_spec)
sys.modules["deepdoc.parser.html_parser"] = _html_mod
_html_spec.loader.exec_module(_html_mod)

RAGFlowHtmlParser = _html_mod.RAGFlowHtmlParser


class _FakeTokenizer:
    """Deterministic stand-in for rag.nlp.rag_tokenizer.

    Mirrors the two behaviours the real tokenizer applies on the default
    (Elasticsearch) backend and that this test depends on: it transforms the
    text (lowercases Latin tokens) and segments spaceless scripts (CJK) into
    per-character, space-separated tokens. tokenize() returns the same
    space-joined string shape the real tokenizer returns.
    """

    @staticmethod
    def tokenize(text):
        spaced = []
        for ch in text:
            if "一" <= ch <= "鿿":
                spaced.append(" " + ch + " ")
            else:
                spaced.append(ch)
        return " ".join(t.lower() for t in "".join(spaced).split())


# Bind the deterministic tokenizer regardless of how rag.nlp resolved.
_html_mod.rag_tokenizer = _FakeTokenizer()


def _token_count(text):
    return RAGFlowHtmlParser._token_count(text)


def test_oversized_english_block_preserves_original_text():
    # 8 latin tokens, budget 3 -> must be split into multiple chunks that keep
    # the original casing (the tokenizer lowercases, so a tokenized-form chunk
    # would be "hello world ...").
    block = "Hello World FOO Bar Baz Qux Lazy Dogs"
    chunks = RAGFlowHtmlParser.chunk_block([block], chunk_token_num=3)

    assert len(chunks) > 1
    # Original text is preserved exactly (atoms partition the source).
    assert "".join(chunks) == block
    # Case is not mangled.
    assert "Hello" in chunks[0]
    assert all(c.lower() != c for c in chunks if any(ch.isalpha() for ch in c))
    # No chunk exceeds the token budget.
    assert all(_token_count(c) <= 3 for c in chunks)


def test_oversized_chinese_block_is_split_and_preserved():
    # Chinese has no whitespace; a naive whitespace split would leave this as a
    # single un-splittable chunk. It must still be split, with no spurious
    # spaces inserted between characters.
    block = "你好世界这是一个测试用例需要被切分"
    chunks = RAGFlowHtmlParser.chunk_block([block], chunk_token_num=3)

    assert len(chunks) > 1
    assert "".join(chunks) == block
    assert all(" " not in c for c in chunks)
    assert all(_token_count(c) <= 3 for c in chunks)


def test_small_blocks_are_merged_unchanged():
    # Blocks under the budget keep their original text and are merged.
    chunks = RAGFlowHtmlParser.chunk_block(["Alpha Beta", "Gamma"], chunk_token_num=512)

    assert "Alpha Beta" in "".join(chunks)
    assert "Gamma" in "".join(chunks)


def test_parser_txt_extracts_bodyless_html_fragment():
    chunks = RAGFlowHtmlParser.parser_txt("<h1>Title</h1><p>Fragment text</p>", chunk_token_num=512)

    joined = "\n".join(chunks)
    assert "# Title" in joined
    assert "Fragment text" in joined


def test_parser_txt_keeps_text_before_first_block():
    # Loose text (or an inline element) that precedes the first block element
    # must not be dropped. The block-change flush was guarded on there being a
    # previous block, so the buffered preamble was overwritten and lost.
    joined = "\n".join(RAGFlowHtmlParser.parser_txt("<body>Preamble text.<h1>Title</h1><p>Body.</p></body>", chunk_token_num=512))
    assert "Preamble text." in joined
    assert "# Title" in joined
    assert "Body." in joined

    inline = "\n".join(RAGFlowHtmlParser.parser_txt("<body><span>Notice.</span><h2>Section</h2><p>Body.</p></body>", chunk_token_num=512))
    assert "Notice." in inline


def test_parser_txt_keeps_loose_text_between_and_after_blocks():
    # Control: loose text surrounded by / following blocks was already kept and
    # must stay that way.
    between = "\n".join(RAGFlowHtmlParser.parser_txt("<p>One.</p>loose<p>Two.</p>", chunk_token_num=512))
    assert "loose" in between
    trailing = "\n".join(RAGFlowHtmlParser.parser_txt("<p>Only.</p>tail", chunk_token_num=512))
    assert "tail" in trailing


# Unified HTML semantics: the browser-faithful rules that BOTH the Python and
# Go parsers must converge on. The cases are the single source of truth shared
# with the Go engine, loaded from the JSON fixture below (so the two engines
# can no longer drift). They must all currently PASS on both engines.
#
# The fixture lives in the Go package's testdata because //go:embed requires
# the file to sit inside the package directory tree; the Python mirror reads
# the same file by absolute repo-root path.
_REPO_ROOT = Path(__file__).resolve().parents[4]
_UNIFIED_HTML_FIXTURE = _REPO_ROOT / "internal/parser/parser/testdata/unified_html_cases.json"
_UNIFIED_HTML_CASES = [(c["name"], c["html"], c["want"]) for c in json.loads(_UNIFIED_HTML_FIXTURE.read_text())]


def _merge_one_block(html):
    # Use read_text_recursively + merge_block_text (not parser_txt) so block
    # boundaries survive as separate list entries and no "#" heading prefix or
    # chunk-merge join is applied — enabling a 1:1 comparison with the Go
    # per-block output.
    soup = BeautifulSoup(html, "html.parser")
    temp = []
    RAGFlowHtmlParser.read_text_recursively(soup, temp)
    blocks, _ = RAGFlowHtmlParser.merge_block_text(temp)
    return blocks


@pytest.mark.parametrize("name,html,want", _UNIFIED_HTML_CASES)
def test_unified_html_semantics_parity(name, html, want):
    assert _merge_one_block(html) == [want]


def test_merge_block_text_loose_text_newline_join():
    # The block_id is None branch (loose text, e.g. text directly under
    # <body>) must join fragments with hard breaks, not silently
    # concatenate. Regression guard for the inline-verbatim / <br> rewrite in
    # merge_block_text (this branch is not exercised by the block_id cases
    # above, which all carry a block_id).
    # Single hard break between two loose fragments.
    blocks, _ = RAGFlowHtmlParser.merge_block_text(
        [
            {"content": "Loose one", "tag_name": "inner_text", "metadata": {}},
            {"content": "Loose two", "tag_name": "inner_text", "metadata": {"newline_before": 1}},
        ]
    )
    assert blocks == ["Loose one\nLoose two"]

    # Consecutive breaks between loose fragments are preserved (1:1 with the
    # <br><br> rule for blocks).
    blocks2, _ = RAGFlowHtmlParser.merge_block_text(
        [
            {"content": "A", "tag_name": "inner_text", "metadata": {}},
            {"content": "B", "tag_name": "inner_text", "metadata": {"newline_before": 2}},
        ]
    )
    assert blocks2 == ["A\n\nB"]
