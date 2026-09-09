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

Locks in case-sensitive matching for the canonical delimiter parser
(#17384, #17383). The flag is currently dead code — it does not propagate
from ``re.finditer`` to ``m.group(1)`` or to downstream ``re.split`` /
``re.match`` calls — but the inconsistency with the three sibling
implementations was misleading. After #17383 all six divergent
implementations were collapsed into ``rag.nlp.delim.parse_delimiter_field``,
which is the single site these tests guard against regressing.

Affected site (after #17383 consolidation)
------------------------------------------
* ``rag.nlp.delim.parse_delimiter_field``  (the only ``re.finditer`` call
  in the canonical parser module)

Sibling sites that previously diverged (now consolidated)
--------------------------------------------------------
* ``rag.nlp.naive_merge`` custom-delimiter path  (now delegates to ``delim``)
* ``rag.nlp.naive_merge_with_images`` custom-delimiter path  (now delegates)
* ``rag.nlp._build_cks``  (now delegates)
* ``rag.nlp.get_delimiters``  (now a backwards-compat shim over ``delim``)
* ``deepdoc.parser.txt_parser.parser_txt``  (now delegates)
* ``deepdoc.parser.markdown_parser.MarkdownElementExtractor.get_delimiters``
  (now delegates)
"""

from __future__ import annotations

import ast
import re
from pathlib import Path

import pytest

pytestmark = pytest.mark.usefixtures("pdf_parser_stub")

from rag import nlp
from rag.nlp import naive_merge
from rag.nlp.delim import compile_delimiter_pattern, parse_delimiter_field

_REPO_ROOT = Path(__file__).resolve().parents[3]


def _get_delim_pattern(field: str) -> str:
    return compile_delimiter_pattern(parse_delimiter_field(field))


# --------------------------------------------------------------------------- #
# delim helper — direct pattern checks
# --------------------------------------------------------------------------- #


def test_get_delimiters_bare_char_a_returns_literal_pattern():
    """Bare-char delimiter ``a`` must produce the pattern ``a``, not ``a|A``."""
    assert _get_delim_pattern("a") == "a"


def test_get_delimiters_bare_char_A_returns_literal_pattern():
    assert _get_delim_pattern("A") == "A"


def test_get_delimiters_backtick_end_returns_exact_token():
    """Backtick-wrapped delimiter must preserve the captured group verbatim.

    A regression that introduced case-insensitive alternation would produce
    ``end|End|END|eNd|...`` instead of the literal ``end``.
    """
    assert _get_delim_pattern("`end`") == "end"


def test_get_delimiters_pattern_splits_case_sensitively():
    """The pattern returned by ``compile_delimiter_pattern`` must split case-sensitively
    when fed to ``re.split`` without any flags."""
    pat = _get_delim_pattern("a")
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
    """Bare-char ``a`` must split only at lowercase ``a``, not at ``A``.

    The delimiter produces two paragraphs ("B", "Ab") which the default
    OVER_CAP merge pairs into one chunk (pairing may exceed cap). The
    assertion therefore checks the *split point*: lowercase 'a' separates
    "B" from "Ab" while the uppercase 'A' stays inline.
    """
    chunks = naive_merge(["BaAb"], chunk_token_num=8, delimiter="a")
    joined = "".join(chunks)
    assert joined == "\nB\nAb"
    # Case-insensitive matching would have split at 'A' too -> "Ba\\nb".
    assert "Ba\nb" not in joined


def test_naive_merge_bare_char_A_splits_only_at_uppercase_A():
    chunks = naive_merge(["BaAb"], chunk_token_num=8, delimiter="A")
    joined = "".join(chunks)
    assert joined == "\nBa\nb"
    assert "B\nAb" not in joined


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
# Static source checks — guard against re.I creeping back into the canonical
# delimiter parser.
#
# After #17383, the six divergent parser implementations were collapsed into
# ``rag/nlp/delim.py`` (one ``re.finditer`` site). The previous locations
# (``rag/nlp/__init__.py`` ~1633, ``deepdoc/parser/txt_parser.py`` ~51) no
# longer have ``re.finditer`` calls — they delegate to the helper. The
# single line-number-based check below therefore targets the new helper,
# and a broader check (over the whole module) guards against re.I leaking
# into any backtick-pattern regex in the parser module.
# --------------------------------------------------------------------------- #


_BACKTICK_RE_SOURCES = [
    # The canonical helper. After #17383, this is the single place where
    # ``re.finditer`` for the `` `[^`]+` `` pattern lives.
    ("rag/nlp/delim.py", "parse_delimiter_field"),
]


def _function_source(source: str, function_name: str) -> str:
    """Return the source text of a function by AST line range."""
    tree = ast.parse(source)
    for node in ast.walk(tree):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name == function_name:
            lines = source.splitlines(keepends=True)
            return "".join(lines[node.lineno - 1 : node.end_lineno])
    raise AssertionError(f"function {function_name!r} not found")


@pytest.mark.parametrize("rel_path, function_name", _BACKTICK_RE_SOURCES)
def test_no_re_I_on_re_finditer(rel_path, function_name):
    """The ``re.finditer`` calls in the canonical delimiter parser must not
    pass ``re.I`` (or any case-insensitive flag) to the regex engine.

    Why this matters even though the flag is currently dead code: keeping
    the parser consistent makes a future refactor less likely to propagate
    the flag to a downstream ``re.split`` / ``re.match`` call where it
    would actually change behavior.
    """
    source = (_REPO_ROOT / rel_path).read_text(encoding="utf-8")
    body = _function_source(source, function_name)
    # Either `re.finditer(...)` directly, or a precompiled regex with
    # `.finditer(...)` (e.g. `_BACKTICK_RE.finditer(normalized)`).
    has_finditer = "re.finditer" in body or ".finditer(" in body
    assert has_finditer, f"expected at least one `re.finditer` (or `.finditer`) in {function_name} (see issue #17384)"
    assert "re.I" not in body and "re.IGNORECASE" not in body, f"`re.I` / `re.IGNORECASE` must not appear in {function_name} in {rel_path} (see issue #17384)"


def test_no_re_I_on_backtick_regex_anywhere_in_parser_module():
    """Broader check: the canonical parser module must not use a
    case-insensitive flag on any ``re.finditer`` / ``re.findall`` /
    ``re.compile`` that targets the backtick regex. Future refactors that
    add a new ``re.finditer`` call elsewhere in the module would otherwise
    silently regress the case-sensitive matching semantics.
    """
    source = (_REPO_ROOT / "rag/nlp/delim.py").read_text(encoding="utf-8")
    tree = ast.parse(source)
    # Collect every `re.finditer` / `re.findall` / `re.compile` call and
    # ensure none of them pass `re.I` / `re.IGNORECASE`.
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        func = node.func
        # Match `re.finditer(...)` / `re.findall(...)` / `re.compile(...)`
        if not (isinstance(func, ast.Attribute) and isinstance(func.value, ast.Name) and func.value.id == "re"):
            continue
        if func.attr not in ("finditer", "findall", "compile"):
            continue
        for kw in node.keywords:
            if kw.arg == "flags":
                flag_node = kw.value
                if isinstance(flag_node, ast.Attribute) and flag_node.attr in ("I", "IGNORECASE"):
                    raise AssertionError(f"`re.{flag_node.attr}` must not be passed as `flags=` to `re.{func.attr}` in rag/nlp/delim.py (see #17384)")
                if isinstance(flag_node, ast.BinOp):
                    for sub in ast.walk(flag_node):
                        if isinstance(sub, ast.Attribute) and sub.attr in ("I", "IGNORECASE"):
                            raise AssertionError(f"`re.{sub.attr}` must not appear in a `flags=` expression passed to `re.{func.attr}` in rag/nlp/delim.py (see #17384)")
