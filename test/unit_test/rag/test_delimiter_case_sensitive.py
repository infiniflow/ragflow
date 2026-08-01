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

import ast
import re
import sys
import types
from pathlib import Path

import pytest


@pytest.fixture(autouse=True)
def stub_pdf_parser(monkeypatch):
    """Stub ``deepdoc.parser.pdf_parser`` for the duration of each test.

    ``naive_merge`` does ``from deepdoc.parser.pdf_parser import
    RAGFlowPdfParser`` inside the function body, and the deepdoc package's
    ``__init__`` pulls in ``infinity`` (a native extension) plus OCR parsers
    that aren't relevant to delimiter parsing. ``monkeypatch.setitem`` (a)
    replaces any pre-existing parser entry — not just stubs one in if absent
    — and (b) restores ``sys.modules`` after the test so the mock never leaks
    across tests.
    """
    pdf_parser = types.ModuleType("deepdoc.parser.pdf_parser")

    class StubPdfParser:
        @staticmethod
        def remove_tag(text):
            return text

    pdf_parser.RAGFlowPdfParser = StubPdfParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.pdf_parser", pdf_parser)


from rag import nlp
from rag.nlp import get_delimiters, naive_merge

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
        return 9 if len(_s) >= 4 else 8

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


_CASE_INSENSITIVE_RE_ATTRS = frozenset({"I", "IGNORECASE"})


def _iter_re_finditer_calls(func_node: ast.AST):
    """Yield ``ast.Call`` nodes whose callee is ``re.finditer``."""
    for node in ast.walk(func_node):
        if not isinstance(node, ast.Call):
            continue
        func = node.func
        if isinstance(func, ast.Attribute) and func.attr == "finditer" and isinstance(func.value, ast.Name) and func.value.id == "re":
            yield node


def _is_case_insensitive_flag(arg: ast.AST) -> bool:
    """True if ``arg`` is the expression ``re.I`` or ``re.IGNORECASE``."""
    return isinstance(arg, ast.Attribute) and isinstance(arg.value, ast.Name) and arg.value.id == "re" and arg.attr in _CASE_INSENSITIVE_RE_ATTRS


def _find_function_def(tree: ast.Module, fn_name: str) -> ast.FunctionDef | None:
    """Locate a function/method named ``fn_name`` anywhere in the module AST.

    Looks at both top-level ``def`` statements and methods inside classes
    (e.g. ``parser_txt`` is a ``@classmethod`` on ``RAGFlowTxtParser``).
    """
    for node in ast.walk(tree):
        if isinstance(node, ast.FunctionDef) and node.name == fn_name:
            return node
    return None


@pytest.mark.parametrize(
    "rel_path, fn_name",
    [
        ("rag/nlp/__init__.py", "get_delimiters"),
        ("deepdoc/parser/txt_parser.py", "parser_txt"),
    ],
)
def test_no_re_I_on_re_finditer(rel_path, fn_name):
    """The ``re.finditer`` calls inside the two delimiter-parsing functions
    must not pass ``re.I`` (or any case-insensitive flag) to the regex
    engine.

    Why this matters even though the flag is currently dead code: the three
    sibling implementations (``naive_merge`` L1195, ``naive_merge_with_images``
    L1269, ``_build_cks`` L1389) already correctly omit ``re.I``. Keeping
    the two outlier sites consistent makes a future refactor less likely to
    propagate the flag to a downstream ``re.split`` / ``re.match`` call
    where it would actually change behavior.

    The check is structural (AST-based) rather than line-number-based so
    unrelated edits above either implementation cannot move the call beyond
    a fragile ±N-line window.
    """
    source = (_REPO_ROOT / rel_path).read_text(encoding="utf-8")
    tree = ast.parse(source, filename=rel_path)

    func_def = _find_function_def(tree, fn_name)
    assert func_def is not None, f"function {fn_name!r} not found in {rel_path}"

    calls = list(_iter_re_finditer_calls(func_def))
    assert calls, f"expected at least one `re.finditer(...)` call inside {fn_name!r} in {rel_path}"

    for call in calls:
        all_args = [*call.args, *(kw.value for kw in call.keywords)]
        for arg in all_args:
            assert not _is_case_insensitive_flag(arg), f"`re.I` / `re.IGNORECASE` must not be passed to `re.finditer` inside {fn_name!r} ({rel_path}). See issue #17384."
