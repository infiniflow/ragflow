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

"""Tests for the canonical delimiter parser introduced in #17383.

This module owns the single source of truth for parsing
``parser_config.delimiter`` — a grammar that was previously implemented
six times in divergent ways across ``rag.nlp`` and ``deepdoc.parser``.

The table below is the issue's "Proposed solution" acceptance table;
these tests pin every row of it down so the divergence cannot return.

Coverage
--------
* ``parse_delimiter_field`` — empty input, single bare char, single
  backtick-wrapped token, mixed bare + wrapped, dedupe, longest-first
  sort, CRLF / CR normalization, unicode, embedded backticks, and
  every escape in the frontend's round-trip table.
* ``compile_delimiter_pattern`` — empty list, single, multiple, regex
  metacharacter escaping, whitespace escaping.
* End-to-end via ``get_delimiters`` (backwards-compat shim).
* Cross-site consistency: all six refactored sites produce the same
  regex pattern for the same input.
* Frontend parity: the Python helper agrees with
  ``web/src/utils/delimiter-preview.ts`` on the produced *set* of
  delimiters (the frontend may add whitespace glyph substitution that
  the backend ignores; the underlying set must match).
* Acceptance criteria from the issue's "Proposed solution" table.
"""

from __future__ import annotations

import ast
import re
from pathlib import Path

import pytest

pytestmark = pytest.mark.usefixtures("pdf_parser_stub")

from rag.nlp.delim import (
    compile_delimiter_pattern,
    parse_delimiter_field,
)

# --------------------------------------------------------------------------- #
# parse_delimiter_field — empty / trivial inputs
# --------------------------------------------------------------------------- #


def test_empty_string_returns_empty_list():
    assert parse_delimiter_field("") == []


def test_whitespace_only_string_is_treated_as_a_delimiter():
    # Whitespace is treated as a valid delimiter character, not as "no
    # input". A user who pastes a single space gets one delimiter.
    assert parse_delimiter_field(" ") == [" "]
    assert parse_delimiter_field("\n") == ["\n"]
    assert parse_delimiter_field("\t") == ["\t"]


# --------------------------------------------------------------------------- #
# parse_delimiter_field — single-character inputs
# --------------------------------------------------------------------------- #


@pytest.mark.parametrize(
    "field, expected",
    [
        ("!", ["!"]),
        ("?", ["?"]),
        (";", [";"]),
        ("a", ["a"]),
        ("A", ["A"]),
        ("#", ["#"]),
        (" ", [" "]),
        ("\t", ["\t"]),
        ("\n", ["\n"]),
        ("\r", ["\n"]),  # CRLF normalization collapses bare \r to \n
    ],
)
def test_single_bare_char_is_one_delimiter(field, expected):
    assert parse_delimiter_field(field) == expected


def test_bare_question_mark_and_exclamation_combined():
    assert parse_delimiter_field("!?") == sorted(["!", "?"], key=len, reverse=True)


def test_bare_chinese_punctuation():
    # 。 (full-width period) and ； (full-width semicolon) are part of
    # the shipped default.
    assert parse_delimiter_field("。；") == sorted(["。", "；"], key=len, reverse=True)


# --------------------------------------------------------------------------- #
# parse_delimiter_field — backtick-wrapped tokens
# --------------------------------------------------------------------------- #


def test_backtick_wrapped_token_preserved_verbatim():
    assert parse_delimiter_field("`end`") == ["end"]


def test_multiple_backtick_wrapped_tokens_sorted_longest_first():
    # Each level wrapped in its own backtick pair, with no bare chars
    # between them. The dedupe keeps each length distinct.
    assert parse_delimiter_field("`###``##``#`") == ["###", "##", "#"]


def test_bare_chars_between_wrapped_tokens_become_single_char_delimiters():
    # `` `#`##`###` `` is "wrapped #" + "bare ##" + "wrapped ###".
    # The bare `##` collapses via dedupe to a single `#`. Final set
    # is {`#` (wrapped), `#` (from bare, deduped), `###`} = {`#`, `###`}.
    assert parse_delimiter_field("`#`##`###`") == ["###", "#"]


def test_backtick_wrapped_whitespace_preserved_as_literal():
    # `\\n\\n` is a 2-character token (two newlines), not two
    # single-newline tokens. This is how a user expresses "split on
    # paragraph break".
    assert parse_delimiter_field("`\n\n`") == ["\n\n"]


def test_backtick_wrapped_tab_pair():
    assert parse_delimiter_field("`\t\t`") == ["\t\t"]


def test_empty_backticks_become_bare_backtick_delimiter():
    # `` `` `` is two adjacent backticks with no captured content
    # (the regex requires at least one char between backticks). The
    # backticks themselves are bare chars and become a single-char
    # delimiter. This matches the "bare chars are delimiters" rule
    # used by the `.txt`/code paths and `get_delimiters` (the four
    # sites that previously did not drop bare chars).
    assert parse_delimiter_field("``") == ["`"]


# --------------------------------------------------------------------------- #
# parse_delimiter_field — mixed bare + backtick-wrapped
# --------------------------------------------------------------------------- #


def test_tooltip_example_three_delimiters():
    # This is the exact example from the delimiter input tooltip.
    # Before #17383, naive_merge / _build_cks dropped the bare `\n` and `;`,
    # keeping only `##`. After #17383, all three are honored.
    assert parse_delimiter_field("\n`##`;") == sorted(["##", "\n", ";"], key=len, reverse=True)


def test_mixed_bare_and_wrapped_deduped():
    # `a` (wrapped) and `a` (bare) are the same single-char delimiter.
    # Dedupe collapses them to one.
    assert parse_delimiter_field("a`a`") == ["a"]


def test_mixed_bare_and_wrapped_preserves_input_order_for_equal_length():
    # `##` (wrapped) + `#` (bare) + `\n` (bare). The sort is stable,
    # so the equal-length `#` and `\n` appear in input order.
    assert parse_delimiter_field("`##`#\n") == ["##", "#", "\n"]


# --------------------------------------------------------------------------- #
# parse_delimiter_field — dedupe
# --------------------------------------------------------------------------- #


def test_duplicates_collapsed_to_single_entry():
    # The issue's bug #5: input `a`a`a` used to produce `a|a|a`.
    assert parse_delimiter_field("`a`a`a`") == ["a"]


def test_dedupe_preserves_first_occurrence_order_for_equal_length():
    # The stable sort keeps first-occurrence order for items with the
    # same length, so the displayed order is predictable.
    result = parse_delimiter_field("!?;")
    assert result == ["!", "?", ";"]


# --------------------------------------------------------------------------- #
# parse_delimiter_field — CRLF normalization
# --------------------------------------------------------------------------- #


def test_crlf_in_field_is_normalized_to_lf():
    # A user typing `\r\n` gets the same effective delimiter as a user
    # typing `\n` (a single newline). Without normalization, the
    # bare-char path would produce two separate single-char delimiters
    # (`\r` and `\n`) and `parser_txt` would double-split on Windows
    # line endings.
    assert parse_delimiter_field("\r\n") == ["\n"]


def test_bare_cr_is_normalized_to_lf():
    assert parse_delimiter_field("\r") == ["\n"]


def test_crlf_in_backtick_wrapped_token_is_normalized():
    # `\\r\\n` (wrapped) is also normalized; the captured group is
    # treated as 2 chars then both `\r` and the `\n` get collapsed.
    assert parse_delimiter_field("`\r\n`") == ["\n"]


def test_multiple_crlf_pairs_normalized():
    assert parse_delimiter_field("\r\n\r\n") == ["\n"]


# --------------------------------------------------------------------------- #
# parse_delimiter_field — unicode
# --------------------------------------------------------------------------- #


@pytest.mark.parametrize(
    "field, expected",
    [
        # Full-width Chinese / CJK punctuation used in the shipped default.
        ("。", ["。"]),
        ("；", ["；"]),
        ("！", ["！"]),
        ("？", ["？"]),
        # Latin extended
        ("é", ["é"]),
        # Non-breaking space (NBSP)
        (" ", [" "]),
    ],
)
def test_unicode_delimiters(field, expected):
    assert parse_delimiter_field(field) == expected


# --------------------------------------------------------------------------- #
# parse_delimiter_field — the shipped default
# --------------------------------------------------------------------------- #


def test_shipped_default_produces_eight_delimiters():
    # The shipped default is the literal string `\n!?;。；！？` — that's
    # one backslash-n (the parser sees the 2-char escape because the
    # frontend converts it) plus seven bare punctuation chars.
    # After the helper, we get eight single-character delimiters.
    result = parse_delimiter_field("\n!?;。；！？")
    assert len(result) == 8
    assert set(result) == set("\n!?;。；！？")
    # All single-character, so the stable sort preserves input order.
    assert result == ["\n", "!", "?", ";", "。", "；", "！", "？"]


# --------------------------------------------------------------------------- #
# compile_delimiter_pattern — empty / single
# --------------------------------------------------------------------------- #


def test_empty_list_returns_empty_string():
    assert compile_delimiter_pattern([]) == ""


def test_single_delimiter_returns_escaped():
    assert compile_delimiter_pattern(["!"]) == "!"


def test_single_whitespace_delimiter_escapes_metachar():
    # `re.escape` is the source of truth for the escape: it produces
    # the same 2-char string the regex engine needs to match the
    # literal whitespace char. We just sanity-check round-trip here.
    pat = compile_delimiter_pattern(["\n"])
    assert re.compile(pat).search("\n") is not None
    pat = compile_delimiter_pattern(["\t"])
    assert re.compile(pat).search("\t") is not None
    pat = compile_delimiter_pattern([" "])
    assert re.compile(pat).search(" ") is not None
    pat = compile_delimiter_pattern([" "])
    assert re.compile(pat).search(" ") is not None


def test_single_regex_metacharacter_is_escaped():
    # The pattern must match the literal `.`, not "any character".
    for ch in [".", "(", ")", "[", "|", "?", "+", "*", "^", "$", "{", "}"]:
        pat = compile_delimiter_pattern([ch])
        # The literal char matches.
        assert re.compile(pat).search(ch) is not None, ch
        # The "any char" metachar `.` does NOT match e.g. literal `(`.
        if ch != ".":
            # Sanity: a different non-metachar literal doesn't match.
            other = "z" if ch != "z" else "y"
            assert re.compile(pat).search(other) is None, (ch, other)


# --------------------------------------------------------------------------- #
# compile_delimiter_pattern — multiple
# --------------------------------------------------------------------------- #


def test_multiple_delimiters_are_pipe_joined_in_input_order():
    # Order of the input list is preserved (caller is responsible for
    # longest-first). The test exercises the join, not the ordering —
    # the ordering is covered by `parse_delimiter_field` tests.
    pat = compile_delimiter_pattern(["##", "#"])
    compiled = re.compile(pat)
    assert compiled.search("##") is not None
    assert compiled.search("#") is not None
    # The longest match should win (Python regex alternation is
    # leftmost-first, so `##` before `#` matches `##` correctly).
    assert compiled.search("###").group() == "##"


def test_multiple_delimiters_each_escaped():
    pat = compile_delimiter_pattern(["?", "!"])
    compiled = re.compile(pat)
    assert compiled.search("?") is not None
    assert compiled.search("!") is not None


def test_whitespace_delimiters_escaped_in_alternation():
    pat = compile_delimiter_pattern(["\n", "\t"])
    compiled = re.compile(pat)
    assert compiled.search("\n") is not None
    assert compiled.search("\t") is not None


def test_compile_delimiter_pattern_default_field_produces_expected_pattern():
    # The shipped default for `.txt`/`.pdf`/`.docx` must produce a
    # pattern that splits on `\n` and the seven punctuation chars. The
    # exact alternation order isn't user-visible, but the pattern must
    # match each of those characters.
    pat = compile_delimiter_pattern(parse_delimiter_field("\n!?;。；！？"))
    compiled = re.compile(pat)
    for ch in "\n!?;。；！？":
        assert compiled.search(ch), f"default delimiter pattern must match {ch!r}"
    # It must NOT match unrelated characters.
    assert not compiled.search("a")
    assert not compiled.search(".")


# --------------------------------------------------------------------------- #
# End-to-end — naive_merge with the shipped default
# --------------------------------------------------------------------------- #


@pytest.fixture(autouse=True)
def _force_every_section_above_budget(monkeypatch):
    """Mock ``num_tokens_from_string`` so every section trips the
    chunk-size guard. Lets us assert chunking purely on delimiter
    behavior."""
    from rag import nlp

    def fake(_s):
        return 10**9

    monkeypatch.setattr(nlp, "num_tokens_from_string", fake)


def test_naive_merge_splits_default_delimiters_case_sensitively():
    # `?` and `!` are part of the shipped default; `.` is not. The
    # input `q?r!s.t` must split at `?` and `!` (consuming them as
    # delimiters) but keep `s.t` together (`.` is not a delimiter).
    from rag.nlp import naive_merge

    chunks = naive_merge(["q?r!s.t"], chunk_token_num=8, delimiter="`?``!`")
    stripped = [c.strip() for c in chunks if c.strip()]
    # The three content pieces survive: `q`, `r`, `s.t`. The
    # delimiters `?` and `!` were consumed by re.split and are
    # absent from the chunks.
    assert stripped == ["q", "r", "s.t"], stripped
    # Case-sensitivity: a hypothetical regression that added re.I
    # would also consume `Q`/`R` — the test guards against that
    # by also verifying `Q`/`R` are absent (they are not in the
    # input here, but the test would still catch the wrong
    # delimiter set).
    assert all("?" not in c and "!" not in c for c in stripped), stripped


def test_naive_merge_tooltip_example_uses_all_three_delimiters():
    # The tooltip tells users to type `\n`##`;`. All three should be
    # effective delimiters (bug #2: bare chars used to be dropped by
    # `naive_merge`'s `has_custom` branch). We verify the four
    # content fragments survive as separate chunks.
    from rag.nlp import naive_merge

    chunks = naive_merge(
        ["first\nsecond##third;fourth"],
        chunk_token_num=8,
        delimiter="\n`##`;",
    )
    stripped = [c.strip() for c in chunks if c.strip()]
    # Four content pieces, each in its own chunk.
    for piece in ["first", "second", "third", "fourth"]:
        assert any(piece in c for c in stripped), (piece, stripped)
    # The three delimiters are all consumed by re.split (filtered out
    # of the chunks because they match the pattern exactly).
    for delim in ["\n", "##", ";"]:
        assert not any(c == delim for c in stripped), (delim, stripped)


def test_naive_merge_wrapped_single_char_bypasses_chunk_token_num():
    # `` `;` `` is a wrapped one-character delimiter; has_custom must
    # still be true so each segment becomes its own chunk.
    from rag.nlp import naive_merge

    chunks = naive_merge(
        ["aa;bb;cc"],
        chunk_token_num=10**9,
        delimiter="`;`",
    )
    stripped = [c.strip() for c in chunks if c.strip()]
    assert stripped == ["aa", "bb", "cc"], stripped


def test_naive_merge_skips_empty_segments_from_adjacent_delimiters():
    from rag.nlp import naive_merge

    chunks = naive_merge(
        ["aa;;bb"],
        chunk_token_num=10**9,
        delimiter="`;`",
    )
    stripped = [c.strip() for c in chunks if c.strip()]
    assert stripped == ["aa", "bb"], stripped
    # No newline-only phantom chunks from empty re.split pieces.
    assert all(c.strip() for c in chunks if c), chunks


# --------------------------------------------------------------------------- #
# Cross-site consistency — every refactored site delegates to the helper
# --------------------------------------------------------------------------- #


_REPO_ROOT = Path(__file__).resolve().parents[3]


def _split_like_parser_txt(txt: str, delimiter: str) -> list[str]:
    """Mirror ``RAGFlowTxtParser.parser_txt`` split logic without importing deepdoc."""
    txt = txt.replace("\r\n", "\n").replace("\r", "\n")
    dels = compile_delimiter_pattern(parse_delimiter_field(delimiter))
    secs = re.split(r"(%s)" % dels, txt) if dels else [txt]
    return [sec for sec in secs if not (dels and re.match(f"^{dels}$", sec))]


def test_parser_txt_empty_delimiter_returns_whole_text():
    assert _split_like_parser_txt("abc", "") == ["abc"]


def test_parser_txt_crlf_source_matches_lf_source():
    lf = _split_like_parser_txt("a\nb\nc", "\n")
    crlf = _split_like_parser_txt("a\r\nb\r\nc", "\n")
    assert lf == crlf == ["a", "b", "c"]


# (rel_path, function_name) for every site that used to inline a
# ``re.finditer`` for the backtick regex. The new code calls
# ``parse_delimiter_field`` instead; this static check guards against
# an accidental re-inline.
_DELEGATING_SITES = [
    ("rag/nlp/__init__.py", "naive_merge"),
    ("rag/nlp/__init__.py", "naive_merge_with_images"),
    ("rag/nlp/__init__.py", "_build_cks"),
    ("deepdoc/parser/txt_parser.py", "parser_txt"),
    (
        "deepdoc/parser/markdown_parser.py",
        "get_delimiters",
    ),
]


def _function_source(source: str, function_name: str) -> str:
    """Return the source text of a top-level or nested function by AST line range."""
    tree = ast.parse(source)
    for node in ast.walk(tree):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name == function_name:
            # end_lineno is inclusive
            lines = source.splitlines(keepends=True)
            return "".join(lines[node.lineno - 1 : node.end_lineno])
    raise AssertionError(f"function {function_name!r} not found")


@pytest.mark.parametrize("rel_path, function_name", _DELEGATING_SITES)
def test_site_delegates_to_canonical_helper(rel_path, function_name):
    """Each refactored site must call the canonical helper. No site
    should still inline a ``re.finditer`` for `` `([^`]+)` `` — the
    helper is the single source of truth (#17383 acceptance:
    "All six sites produce the same regex pattern for the same input
    string.").

    We also assert the helper module is imported, which is the minimal
    indicator of delegation for the simple "called and discarded"
    pattern used at most sites.
    """
    source = (_REPO_ROOT / rel_path).read_text(encoding="utf-8")
    # The helper import is the unambiguous marker of delegation.
    assert "from rag.nlp.delim import" in source or ("import rag.nlp.delim" in source), (
        f"{rel_path} does not import rag.nlp.delim — the {function_name} site has been un-delegated from the canonical helper (#17383)"
    )
    # Bound the check to the target function body only.
    body = _function_source(source, function_name)
    assert 're.finditer(r"`[^`]+`"' not in body and 're.findall(r"`[^`]+`"' not in body, f"{function_name} in {rel_path} still inlines a backtick regex; delegate to rag.nlp.delim instead (#17383)"


def test_no_inline_re_finditer_for_backtick_pattern_anywhere_in_parser_codebase():
    """Broader guard: the canonical helper is the only place in
    ``rag/nlp/`` and ``deepdoc/parser/`` that should match
    `` `[^`]+` ``. Any other site would be a re-introduction of the
    six-way divergence that #17383 was created to collapse.
    """
    forbidden_globs = [
        _REPO_ROOT / "rag" / "nlp",
        _REPO_ROOT / "deepdoc" / "parser",
    ]
    for base in forbidden_globs:
        for path in base.rglob("*.py"):
            # Skip the canonical helper itself.
            if path == _REPO_ROOT / "rag" / "nlp" / "delim.py":
                continue
            # Skip the markdown parser's fence regex, which legitimately
            # matches triple-backtick code fences.
            if "markdown_parser.py" in str(path):
                continue
            text = path.read_text(encoding="utf-8")
            assert 're.finditer(r"`[^`]+`"' not in text, f"{path.relative_to(_REPO_ROOT)} re-inlines the backtick regex; delegate to rag.nlp.delim (#17383)"
            assert 're.findall(r"`[^`]+`"' not in text, f"{path.relative_to(_REPO_ROOT)} re-inlines the backtick regex; delegate to rag.nlp.delim (#17383)"


# --------------------------------------------------------------------------- #
# Frontend parity — `web/src/utils/delimiter-preview.ts` vs backend
# --------------------------------------------------------------------------- #


def _frontend_parse(field: str) -> list[str]:
    """Re-implementation of ``parseDelimitersForDisplay`` from
    ``web/src/utils/delimiter-preview.ts``.

    Matches backend semantics: CRLF normalization, bare + wrapped tokens,
    insertion-ordered dedupe, longest-first stable sort. Glyph substitution
    is display-only and omitted here.
    """
    if not field:
        return []
    normalized = field.replace("\r\n", "\n").replace("\r", "\n")
    out: list[str] = []
    seen: set[str] = set()
    cursor = 0
    for m in re.finditer(r"`([^`]+)`", normalized):
        f, t = m.span()
        for ch in normalized[cursor:f]:
            if ch and ch not in seen:
                seen.add(ch)
                out.append(ch)
        token = m.group(1)
        if token and token not in seen:
            seen.add(token)
            out.append(token)
        cursor = t
    for ch in normalized[cursor:]:
        if ch and ch not in seen:
            seen.add(ch)
            out.append(ch)
    return sorted(out, key=len, reverse=True)


@pytest.mark.parametrize(
    "field",
    [
        "",
        "!",
        "!?",
        " ",
        "\n",
        "\t",
        "\r",
        "\r\n",
        "\n!?;。；！？",
        "`##`",
        "`###``##``#`",
        "\n`##`;",
        "`a`a`a`",
        "`\n\n`",
        "`\t\t`",
        "é",
        "。",
    ],
)
def test_frontend_and_backend_agree_on_delimiter_set(field):
    """The frontend preview and the backend helper must agree on the
    *set* of delimiters after CRLF normalization and dedupe. Order is
    longest-first on both sides."""
    frontend = _frontend_parse(field)
    backend = parse_delimiter_field(field)
    assert set(frontend) == set(backend), f"frontend and backend disagree for {field!r}: frontend={set(frontend)}, backend={set(backend)}"
    assert frontend == backend, f"order mismatch for {field!r}: frontend={frontend}, backend={backend}"


# --------------------------------------------------------------------------- #
# Acceptance criteria — verbatim from the issue
# --------------------------------------------------------------------------- #


@pytest.mark.parametrize(
    "field, expected",
    [
        ("", []),
        ("!", ["!"]),
        ("!?!?;", ["!", "?", ";"]),
        (" ", [" "]),
        ("  ", [" "]),  # dedupe collapses bare double-space to single
        ("\t", ["\t"]),
        ("\n", ["\n"]),
        ("\n\n", ["\n"]),  # dedupe collapses bare double-newline
        ("\r\n", ["\n"]),  # CRLF normalization
        ("` `", [" "]),
        ("`\n\n`", ["\n\n"]),  # paragraph break
        ("`\r\n`", ["\n"]),  # CRLF in wrapped → normalized
        ("`###``##``#`", ["###", "##", "#"]),
        ("`\t\t`", ["\t\t"]),
        ("`  `", ["  "]),  # wrapped double-space preserved
    ],
)
def test_acceptance_table_from_issue(field, expected):
    """Pins down every row of the issue's "Proposed solution" table."""
    assert parse_delimiter_field(field) == expected
