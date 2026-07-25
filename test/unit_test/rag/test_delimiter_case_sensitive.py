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

"""Regression tests for case-sensitive delimiter parsing.

Locks in case-sensitive matching for the two delimiter-parsing
implementations that pass ``re.I`` to ``re.finditer`` (#17384). The flag is
currently dead code — it does not propagate from ``re.finditer`` to
``m.group(1)`` or to downstream ``re.split`` / ``re.match`` calls — but the
inconsistency with the three sibling implementations is misleading. These
tests guard against any future refactor that accidentally makes matching
case-insensitive.

Affected sites
--------------
* ``rag.nlp.get_delimiters``  (line 1633)
* ``deepdoc.parser.txt_parser.parser_txt``  (line 51)

Sibling sites that already correctly omit ``re.I``
--------------------------------------------------
* ``rag.nlp.naive_merge`` custom-delimiter path  (line 1195)
* ``rag.nlp.naive_merge_with_images`` custom-delimiter path  (line 1269)
* ``rag.nlp._build_cks``  (line 1389)
"""

from __future__ import annotations

import re
import sys
import types
from pathlib import Path

import pytest


# --------------------------------------------------------------------------- #
# Stub the heavy deepdoc + infinity import chain before touching ``rag.nlp``.
#
# ``naive_merge`` does ``from deepdoc.parser.pdf_parser import RAGFlowPdfParser``
# inside the function body, and the deepdoc package's ``__init__`` pulls in
# ``infinity`` (a native extension) plus OCR parsers that aren't relevant to
# delimiter parsing. The mock below gives ``naive_merge`` a no-op
# ``RAGFlowPdfParser.remove_tag`` so the chunking logic under test runs
# without the heavy deps.
# --------------------------------------------------------------------------- #
_pdf_parser = types.ModuleType("deepdoc.parser.pdf_parser")


class _StubPdfParser:
    @staticmethod
    def remove_tag(text):
        return text


_pdf_parser.RAGFlowPdfParser = _StubPdfParser
sys.modules.setdefault("deepdoc.parser.pdf_parser", _pdf_parser)


import rag.nlp as nlp
from rag.nlp import naive_merge, get_delimiters


_REPO_ROOT = Path(__file__).resolve().parents[3]


# --------------------------------------------------------------------------- #
# get_delimiters — direct pattern checks
# --------------------------------------------------------------------------- #


def test_get_delimiters_bare_char_a_returns_literal_pattern():
    """Bare-char delimiter ``a`` must produce the pattern ``a``, not ``a|A``."""
    assert get_delimiters("a") == "a"


def test_get_delimiters_bare_char_A_returns_literal_pattern():
    assert get_delimiters("A") == "A"


def test_get_delimiters_backtick_end_returns_exact_token():
    """Backtick-wrapped delimiter must preserve the captured group verbatim.

    A regression that introduced case-insensitive alternation would produce
    ``end|End|END|eNd|...`` instead of the literal ``end``.
    """
    assert get_delimiters("`end`") == "end"


def test_get_delimiters_pattern_splits_case_sensitively():
    """The pattern returned by ``get_delimiters`` must split case-sensitively
    when fed to ``re.split`` without any flags."""
    pat = get_delimiters("a")
    # Only the lowercase 'a' splits; uppercase 'A' is preserved intact.
    assert re.split(f"({pat})", "AaBb") == ["A", "a", "Bb"]


# --------------------------------------------------------------------------- #
# naive_merge — end-to-end (exercises get_delimiters + re.split)
# --------------------------------------------------------------------------- #


@pytest.fixture(autouse=True)
def force_every_section_above_budget(monkeypatch):
    """Mock ``num_tokens_from_string`` so every section trips the chunk-size
    guard and starts a fresh chunk. This isolates delimiter behavior from
    chunk-size heuristics."""

    def fake(_s):
        return 10**9

    monkeypatch.setattr(nlp, "num_tokens_from_string", fake)


def test_naive_merge_bare_char_a_splits_only_at_lowercase_a():
    """Bare-char ``a`` must split only at lowercase ``a``, not at ``A``."""
    chunks = naive_merge(["BaAb"], chunk_token_num=8, delimiter="a")
    assert [c.strip() for c in chunks if c.strip()] == ["B", "Ab"]


def test_naive_merge_bare_char_A_splits_only_at_uppercase_A():
    chunks = naive_merge(["BaAb"], chunk_token_num=8, delimiter="A")
    assert [c.strip() for c in chunks if c.strip()] == ["Ba", "b"]


def test_naive_merge_backtick_end_splits_only_at_lowercase_end():
    """Backtick-wrapped ``end`` must split only at the exact lowercase
    ``end``, not at ``End`` / ``END`` / ``eNd`` / etc."""
    chunks = naive_merge(
        ["the end and End and END come"],
        chunk_token_num=8,
        delimiter="`end`",
    )
    assert [c.strip() for c in chunks if c.strip()] == [
        "the",
        "and End and END come",
    ]


# --------------------------------------------------------------------------- #
# Static source checks — guard against re.I creeping back into the two sites
#
# ``parser_txt`` is not exercised directly here because importing it pulls in
# the full ``deepdoc.parser`` package (``infinity`` native extension, OCR
# parsers, etc.). The two sites share the same ``re.finditer`` pattern, so
# the behavioral tests above (which exercise ``get_delimiters`` via
# ``naive_merge``) are sufficient to lock in the chunking semantics. The
# static checks below ensure the cleanup lands in both files and cannot be
# silently undone.
# --------------------------------------------------------------------------- #


@pytest.mark.parametrize(
    "rel_path, line_number",
    [
        ("rag/nlp/__init__.py", 1633),
        ("deepdoc/parser/txt_parser.py", 51),
    ],
)
def test_no_re_I_on_re_finditer(rel_path, line_number):
    """The ``re.finditer`` calls at the two affected sites must not pass
    ``re.I`` (or any case-insensitive flag) to the regex engine.

    Why this matters even though the flag is currently dead code: the three
    sibling implementations (``naive_merge`` L1195, ``naive_merge_with_images``
    L1269, ``_build_cks`` L1389) already correctly omit ``re.I``. Keeping
    the two outlier sites consistent makes a future refactor less likely to
    propagate the flag to a downstream ``re.split`` / ``re.match`` call
    where it would actually change behavior.
    """
    source_path = _REPO_ROOT / rel_path
    lines = source_path.read_text(encoding="utf-8").splitlines()
    # Line numbers in this test are 1-based; allow ±2 to absorb whitespace
    # or comment shifts.
    window = lines[max(0, line_number - 3) : line_number + 1]
    joined = "\n".join(window)
    assert "re.finditer" in joined, (
        f"expected `re.finditer` near {rel_path}:{line_number}, got:\n{joined}"
    )
    assert "re.I" not in joined and "re.IGNORECASE" not in joined, (
        f"`re.I` / `re.IGNORECASE` must not appear in `re.finditer` at "
        f"{rel_path}:{line_number} (see issue #17384). Got:\n{joined}"
    )
