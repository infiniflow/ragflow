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
import os
import sys
from unittest import mock

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
