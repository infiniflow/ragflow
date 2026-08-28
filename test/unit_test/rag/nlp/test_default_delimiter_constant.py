#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
"""Regression tests for issue #18562.

The shipped default for ``parser_config.delimiter`` was copy-pasted in
at least 5 sites with 3 distinct values. The divergence was undocumented
-- the most impactful gap was that ``docx`` / ``image`` / ``email`` omitted
``;`` (so an English paragraph like "The rain; the sun; the wind" parsed
as one oversized chunk instead of three), while ``txt`` / ``markdown``
included it.

The fix introduces ``rag.nlp.delim.DEFAULT_DELIMITER`` (the full set
``'\n!?;。；！？'``) and uses it in the 5 standard parser entry points.
``book.py`` is left unchanged because its ``'\n。；！？'`` default is a
deliberate Chinese-text audience choice, not a drift.

These tests pin:
- the constant exists in ``rag.nlp.delim`` with the documented value,
- the constant is the one referenced in each of the 5 main-parser
  ``parser_config.get("delimiter", ...)`` call sites,
- the ``book.py`` site is unchanged (regression guard),
- a behavioural test confirms the fixed default splits English
  semicolons (the original bug's user-visible symptom).
"""

import ast
from pathlib import Path

import pytest

from rag.nlp.delim import DEFAULT_DELIMITER, parse_delimiter_field

REPO_ROOT = Path(__file__).resolve().parents[4]


# ---------------------------------------------------------------------------
# Constant
# ---------------------------------------------------------------------------


def test_default_delimiter_constant_value():
    """The constant must be the full set: newline + ``!?;`` (English
    punctuation) + ``。，！？`` (Chinese punctuation). The ``;`` is
    the one the pre-fix defaults in docx / image / email dropped --
    see issue #18562.
    """
    assert DEFAULT_DELIMITER == "\n!?;。；！？", (
        f"DEFAULT_DELIMITER is {DEFAULT_DELIMITER!r}; expected "
        f"'\\n!?;。；？' (newline + English punctuation ;!? + Chinese "
        f"punctuation 。，！？). The ; is the one the pre-fix docx / "
        f"image / email defaults dropped."
    )


def test_default_delimiter_parses_to_full_set():
    """The parsed delim list must include ``;`` (and the rest of the
    English+Chinese punctuation set) -- the full set a user might
    expect on English+Chinese text.
    """
    parsed = parse_delimiter_field(DEFAULT_DELIMITER)
    for delim in ["\n", "!", "?", ";", "。", "；", "！", "？"]:
        assert delim in parsed, f"DEFAULT_DELIMITER is missing {delim!r} after parsing; parsed: {parsed}. The ; omission is the original bug."


# ---------------------------------------------------------------------------
# Call sites in naive.py and email.py
# ---------------------------------------------------------------------------


# ``parser_config.get("delimiter", "<literal>")`` in each main-parser
# entry point. The test reads each file's source, walks the AST, and
# asserts the literal is now ``DEFAULT_DELIMITER`` (the constant name)
# and not any of the 3 pre-fix hard-coded strings.
PRE_FIX_DELIMITER_LITERALS = frozenset(
    {
        "\n!?;。；！？",  # pre-fix txt/markdown (correct, now constant)
        "\n!?。；！？",  # pre-fix docx/image/email (missing ;)
        "\n。；！？",  # pre-fix book (Chinese-only, deliberately preserved)
    }
)


def _parser_config_default_args_in_file(path: Path):
    """Yield every ``parser_config.get("delimiter", X)`` call's second
    argument ``X`` (a literal AST node) in ``path``. The test then
    asserts each ``X`` is the constant name ``DEFAULT_DELIMITER``.
    """
    source = path.read_text(encoding="utf-8")
    tree = ast.parse(source)
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        # Match keyword-style: parser_config.get("delimiter", X) and
        # keyword-style parser_config.get(key="delimiter", default=X).
        kwargs = {kw.arg: kw.value for kw in node.keywords}
        positional = list(node.args)
        is_target = False
        if len(positional) >= 2 and isinstance(positional[0], ast.Constant) and positional[0].value == "delimiter":
            is_target = True
            default_node = positional[1]
        elif kwargs.get("key") is not None and isinstance(kwargs["key"], ast.Constant) and kwargs["key"].value == "delimiter":
            is_target = True
            default_node = kwargs.get("default")
        if is_target and default_node is not None:
            yield default_node


@pytest.mark.parametrize(
    ("relpath", "min_count"),
    [
        ("rag/app/naive.py", 5),
        ("rag/app/email.py", 1),
    ],
)
def test_main_parser_call_sites_use_default_delimiter_constant(relpath, min_count):
    """Every ``parser_config.get("delimiter", X)`` call in the main
    parsers must use the ``DEFAULT_DELIMITER`` constant -- not a
    hard-coded literal. The pre-fix code had at least 5 sites in
    ``naive.py`` (docx, txt, markdown, image x2) and 1 in ``email.py``
    that copy-pasted a string literal; a future refactor that
    re-introduces a literal is caught here.
    """
    path = REPO_ROOT / relpath
    if not path.is_file():
        pytest.fail(f"missing source: {path}")

    defaults = list(_parser_config_default_args_in_file(path))
    assert len(defaults) >= min_count, (
        f"expected at least {min_count} parser_config.get('delimiter', X) call(s) in {relpath}; found {len(defaults)}. If the call sites were moved or removed, the marker list needs to be updated."
    )
    for default_node in defaults:
        if isinstance(default_node, ast.Name) and default_node.id == "DEFAULT_DELIMITER":
            continue
        if isinstance(default_node, ast.Constant) and isinstance(default_node.value, str):
            assert default_node.value not in PRE_FIX_DELIMITER_LITERALS, (
                f"{relpath} contains a parser_config.get('delimiter', X) "
                f"call with the pre-fix hard-coded literal {default_node.value!r}. "
                f"Use the DEFAULT_DELIMITER constant from rag.nlp.delim instead "
                f"-- the issue #18562 root cause is this exact divergence."
            )
            # Any other literal is also a regression risk: every site
            # should go through the constant so a future fix in one
            # place propagates everywhere.
            pytest.fail(
                f"{relpath} has a parser_config.get('delimiter', {default_node.value!r}) call that does not reference DEFAULT_DELIMITER. Use the constant to keep the shipped default in one place."
            )
        pytest.fail(f"{relpath} has a parser_config.get('delimiter', X) call where X is not a string literal or a Name; got {ast.dump(default_node)}")


# ---------------------------------------------------------------------------
# Regression guard: book.py must keep its own Chinese-only default
# ---------------------------------------------------------------------------


def test_book_py_keeps_chinese_only_default():
    """The book parser keeps ``'\n。；？'`` (Chinese-only) as its
    own deliberate choice for the Chinese-text book audience. A
    refactor that "harmonises" book.py onto DEFAULT_DELIMITER would
    be a regression for Chinese book users.
    """
    path = REPO_ROOT / "rag/app/book.py"
    if not path.is_file():
        pytest.fail(f"missing source: {path}")

    found = False
    for default_node in _parser_config_default_args_in_file(path):
        if isinstance(default_node, ast.Constant) and isinstance(default_node.value, str):
            assert default_node.value == "\n。；！？", (
                f"book.py parser_config.get('delimiter', X) has X="
                f"{default_node.value!r}; expected the Chinese-only "
                f"book default '\\n。；？'. The book parser's audience "
                f"is Chinese books; a wider delimiter set would split "
                f"on English punctuation that is not present in the "
                f"target text. See issue #18562."
            )
            found = True
            break
    assert found, (
        "book.py no longer has a parser_config.get('delimiter', ...) call. "
        "If the book parser was refactored away, this regression guard "
        "should be removed; otherwise the Chinese-only default is missing."
    )


# ---------------------------------------------------------------------------
# Behavioural: the fixed default splits English semicolons (the bug's
# user-visible symptom)
# ---------------------------------------------------------------------------


def test_default_delimiter_splits_english_semicolons():
    """The user-visible symptom from issue #18552/#18562: a paragraph
    with English semicolons must split on ``;``. With the fixed
    default, the resulting segments are visible (the merge step may
    then re-merge them if they all fit in chunk_token_num, but the
    split itself must occur -- otherwise the user's prose is one
    oversized chunk).
    """
    from rag.nlp import naive_merge

    # Use a very long input with many semicolons so the merge step
    # cannot re-merge. chunk_token_num is small enough that any single
    # clause exceeds it. The pre-fix defaults that dropped ';' produced
    # one huge chunk; the post-fix default produces one chunk per
    # semicolon-separated clause.
    paragraph = ("The rain; " * 20) + "the wind."
    sections = [paragraph]

    chunks = naive_merge(sections, chunk_token_num=8, delimiter=DEFAULT_DELIMITER)
    # 20 semicolons + 1 final clause = 21 expected splits (the
    # delimiter alone, not counting the merge step's coalescing of
    # tiny adjacent segments under chunk_token_num=8 -- but since
    # each "The rain; " is itself > 8 tokens, no merge should happen).
    assert len(chunks) > 5, (
        f"DEFAULT_DELIMITER failed to split on ';': a 250-char paragraph "
        f"with 20 semicolons produced {len(chunks)} chunk(s). The pre-fix "
        f"docx/image/email defaults dropped ';' and produced 1 oversized "
        f"chunk. Expected many chunks, one per semicolon-separated clause."
    )
