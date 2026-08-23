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

"""Regression tests for ``naive_merge_docx`` / ``_merge_cks``.

Guards against the accumulated-only budget check: ``_merge_cks`` decided
whether to merge an incoming text unit into the previous chunk by looking at
whether the *already-accumulated* total was already over ``chunk_token_num``,
never the *projected* total (accumulated + incoming). That is the same class
of bug #17203 fixed for ``naive_merge`` / ``RAGFlowTxtParser.parser_txt`` (the
size check fired after the append instead of before it) -- but #17203's own
"Out of scope" note claimed ``naive_merge_docx`` "already enforces the budget
... unchanged", which is incorrect: ``_merge_cks`` has an equivalent
after-the-fact check and no atom-level cap. Because the accumulated-only
check only re-evaluates once per iteration, the overshoot is not bounded to
one extra unit the way a simple soft-cap bug would be -- when text units are
sized close to chunk_token_num, a chunk can grow to nearly double the budget
before the next iteration notices.
"""

import re

import pytest

from rag import nlp
from rag.nlp import _merge_cks, _split_oversized_unit, naive_merge_docx

DEFAULT_DELIMITER = "\n。；！？"


@pytest.fixture(autouse=True)
def word_count_tokens(monkeypatch):
    """Count tokens as whitespace-delimited words. Deterministic and
    tokenizer-independent so chunk-size assertions are exact (mirrors the
    fixture in test_naive_merge.py).
    """

    def fake_num_tokens(s):
        s = re.sub(r"@@[0-9]+\t[^\t\n]*", "", s or "")
        return len(s.split())

    monkeypatch.setattr(nlp, "num_tokens_from_string", fake_num_tokens)
    return fake_num_tokens


def _nonempty_chunks(chunks):
    return [c for c in chunks if (c.get("text") or "").strip()]


def _section(text):
    # naive_merge_docx sections are (text, image, table) triples.
    return (text, None, None)


# --------------------------------------------------------------------------- #
# naive_merge_docx -- hard cap on chunk size (accumulated-only check bug)
#
# Assertions below check the ``tk_nums`` field returned on each chunk (the
# same accounting the merge decision itself uses), not a re-tokenization of
# the concatenated ``text`` -- ``_merge_cks`` joins merged text with no
# separator, so a whitespace-splitting tokenizer can fuse a trailing/leading
# word across a merge boundary (e.g. "...delta" + "alpha..." -> "deltaalpha").
# That display/formatting detail is pre-existing, orthogonal to the budget
# bug this PR fixes, and out of scope here.
# --------------------------------------------------------------------------- #


@pytest.mark.p2
def test_naive_merge_docx_many_paragraphs_do_not_overshoot_budget():
    # 20 paragraphs of 40 tokens each, default chunk_token_num=128.
    # Pre-fix: _merge_cks only starts a new chunk once the *accumulated* total
    # is already >= chunk_token_num, so every chunk grows to 4 paragraphs
    # (160 tokens, 25% over budget) before the overflow is noticed.
    paragraph = " ".join(["word"] * 40)
    sections = [_section(paragraph) for _ in range(20)]

    chunks, _ = naive_merge_docx(sections, chunk_token_num=128)
    texts = _nonempty_chunks(chunks)

    assert len(texts) > 1
    assert all(c["tk_nums"] <= 128 for c in texts), [c["tk_nums"] for c in texts]
    # Content is preserved regardless of how it was split.
    assert sum(c["tk_nums"] for c in texts) == 20 * 40


@pytest.mark.p2
def test_naive_merge_docx_near_budget_units_do_not_double_budget():
    # 21 sections of 95 tokens each, budget 100. Sections are sized close to
    # the budget, so the accumulated-only check's worst case applies: the
    # first two sections merge to 190 tokens (95 < 100 passes the stale
    # check) before a third section trips the "already over budget" branch.
    sentence = " ".join(["lorem"] * 95)
    sections = [_section(sentence) for _ in range(21)]

    chunks, _ = naive_merge_docx(sections, chunk_token_num=100)
    texts = _nonempty_chunks(chunks)

    assert len(texts) > 1
    assert all(c["tk_nums"] <= 100 for c in texts), [c["tk_nums"] for c in texts]
    # Content is preserved regardless of how it was split.
    assert sum(c["tk_nums"] for c in texts) == 21 * 95


@pytest.mark.p2
def test_naive_merge_docx_small_sections_are_still_packed():
    # The fix must not regress packing efficiency: small sections that fit
    # comfortably under the budget should still land in the same chunk.
    sections = [_section("alpha beta gamma delta") for _ in range(8)]  # 4 tokens each
    chunks, _ = naive_merge_docx(sections, chunk_token_num=50)
    texts = _nonempty_chunks(chunks)
    assert len(texts) == 1
    assert texts[0]["tk_nums"] == 32


@pytest.mark.p2
def test_naive_merge_docx_oversized_section_is_split_on_whitespace():
    # A single section with no internal delimiter is larger than the budget on
    # its own: it is split on whitespace so every emitted chunk fits.
    huge = " ".join(["word"] * 2000)
    chunks, _ = naive_merge_docx([_section(huge)], chunk_token_num=100)
    texts = _nonempty_chunks(chunks)
    assert len(texts) == 20
    assert all(c["tk_nums"] <= 100 for c in texts), [c["tk_nums"] for c in texts]
    assert sum(c["tk_nums"] for c in texts) == 2000
    assert "".join(c["text"] for c in texts) == huge


@pytest.mark.p2
def test_naive_merge_docx_custom_delimiter_section_is_not_split():
    # A wrapped delimiter bypasses chunk_token_num by contract: one segment per
    # chunk, whatever its size.
    huge = " ".join(["word"] * 500)
    chunks, _ = naive_merge_docx([_section(huge)], chunk_token_num=100, delimiter="`##`")
    texts = _nonempty_chunks(chunks)
    assert len(texts) == 1
    assert texts[0]["tk_nums"] == 500


# --------------------------------------------------------------------------- #
# _split_oversized_unit -- whitespace tier and character-window fallback
# --------------------------------------------------------------------------- #


def _char_count(s):
    return len(s or "")


@pytest.mark.p2
def test_split_oversized_unit_breaks_on_whitespace():
    text = " ".join(["alpha"] * 40)
    pieces = _split_oversized_unit(text, 10, token_count_fn=lambda s: len((s or "").split()))
    assert all(len(p.split()) <= 10 for p in pieces), [len(p.split()) for p in pieces]
    assert "".join(pieces) == text


@pytest.mark.p2
def test_split_oversized_unit_character_window_fallback():
    # One whitespace-free run larger than the budget: the character-window
    # search inside the run keeps every piece within the cap.
    text = "x" * 250
    pieces = _split_oversized_unit(text, 32, token_count_fn=_char_count)
    assert all(_char_count(p) <= 32 for p in pieces), [len(p) for p in pieces]
    assert "".join(pieces) == text


@pytest.mark.p2
def test_split_oversized_unit_mixed_words_and_long_run():
    text = "short words here " + "y" * 300 + " and more words"
    pieces = _split_oversized_unit(text, 40, token_count_fn=_char_count)
    assert all(_char_count(p) <= 40 for p in pieces), [len(p) for p in pieces]
    assert "".join(pieces) == text


@pytest.mark.p2
def test_split_oversized_unit_single_atom_over_budget_still_advances():
    # cap smaller than one character: the loop must terminate, one char per
    # piece, with no text lost.
    pieces = _split_oversized_unit("abcd", 0, token_count_fn=_char_count)
    assert pieces == ["abcd"]
    pieces = _split_oversized_unit("abcd", 1, token_count_fn=_char_count)
    assert pieces == ["a", "b", "c", "d"]


@pytest.mark.p2
def test_split_oversized_unit_within_budget_is_returned_whole():
    assert _split_oversized_unit("abc", 10, token_count_fn=_char_count) == ["abc"]
    assert _split_oversized_unit("", 10, token_count_fn=_char_count) == []


# --------------------------------------------------------------------------- #
# _merge_cks -- direct unit tests of the merge decision
# --------------------------------------------------------------------------- #


def _ck(text, tk_nums, ck_type="text"):
    return {"text": text, "image": None, "ck_type": ck_type, "tk_nums": tk_nums}


@pytest.mark.p2
def test_merge_cks_projected_total_check_no_overshoot():
    # Three text chunks of 60 tokens each, budget 100. Accumulated-only check:
    # chunk 1 (60) merges chunk 2 -> 120 (over budget, but the check that let
    # it merge only saw 60 < 100). Projected-total check must reject the
    # second merge up front, keeping every merged chunk <= 100.
    cks = [_ck("a ", 60), _ck("b ", 60), _ck("c ", 60)]
    merged, _ = _merge_cks(cks, chunk_token_num=100, has_custom=False)
    assert all(m["tk_nums"] <= 100 for m in merged)
    assert sum(m["tk_nums"] for m in merged) == 180


@pytest.mark.p2
def test_merge_cks_has_custom_bypasses_merging():
    # has_custom=True (a wrapped custom delimiter was present) must keep every
    # unit as its own chunk regardless of size, matching naive_merge's
    # custom-delimiter contract.
    cks = [_ck("a", 1), _ck("b", 1), _ck("c", 1)]
    merged, _ = _merge_cks(cks, chunk_token_num=100, has_custom=True)
    assert len(merged) == 3


@pytest.mark.p2
def test_merge_cks_image_entries_are_never_merged_into():
    # A non-text (image) entry is always appended as its own chunk and is
    # tracked in image_idxs, regardless of the projected-total check (which
    # only applies to ck_type == "text"). Text before/after an image can
    # still merge into the running text buffer -- that positional-buffer
    # behaviour is unchanged from before this fix and not what this test
    # guards; it only asserts the image entry itself is never merged.
    cks = [_ck("a", 10), _ck("", 0, ck_type="image"), _ck("b", 10)]
    merged, image_idxs = _merge_cks(cks, chunk_token_num=100, has_custom=False)
    assert image_idxs == [1]
    assert merged[1]["ck_type"] == "image"
    assert merged[1]["text"] == ""
    # The text on both sides of the image survives the pass.
    assert sum(m["tk_nums"] for m in merged if m["ck_type"] == "text") == 20
