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

"""Regression tests for ``naive_merge`` / ``naive_merge_with_images``.

Guards against:

* the regression introduced by commit db0f6840d (#11434) where the default
  (non-custom-delimiter) path stopped splitting oversized sections at sentence
  boundaries, and the overlap prefix was not counted toward a chunk's token
  budget;
* the soft-cap bug where chunks systematically overshot ``chunk_token_num`` by
  up to one unit (sentence / line) because the size check fired *after* the
  append instead of using a projected-total check.
"""

import re

import pytest

from rag import nlp
from rag.nlp import naive_merge, naive_merge_with_images, MergeStrategy

DEFAULT_DELIMITER = "\n!?。；！？"


@pytest.fixture(autouse=True)
def word_count_tokens(monkeypatch):
    """Count tokens as whitespace-delimited words (ignoring ``@@..`` position tags).

    Deterministic and tokenizer-independent so chunk-size assertions are exact.
    """

    def fake_num_tokens(s):
        s = re.sub(r"@@[0-9]+\t[^\t\n]*", "", s or "")
        return len(s.split())

    monkeypatch.setattr(nlp, "num_tokens_from_string", fake_num_tokens)
    return fake_num_tokens


def _tok(s):
    return len(re.sub(r"@@[0-9]+\t[^\t\n]*", "", s or "").split())


def _nonempty(chunks):
    return [c for c in chunks if c.strip()]


# --------------------------------------------------------------------------- #
# naive_merge — text path
# --------------------------------------------------------------------------- #


@pytest.mark.p2
def test_oversized_section_is_split_at_sentence_boundaries():
    # One section far larger than chunk_token_num, sentences separated by '\n'.
    sentence = " ".join(["word"] * 10)  # 10 tokens
    section = "\n".join([sentence] * 20)  # 200 tokens, single section
    assert _tok(section) == 200

    chunks = _nonempty(naive_merge([section], chunk_token_num=50, delimiter=DEFAULT_DELIMITER))

    # Pre-regression behaviour: the section is broken into several chunks
    # instead of a single oversized one.
    assert len(chunks) > 1
    # OVER_CAP (default) allows at most one boundary paragraph to overflow the
    # soft cap: a chunk may exceed ``chunk_token_num`` by one paragraph (10
    # tokens here) but never more. The old pairwise code packed to strictly
    # ``<= cap``; greedy OVER_CAP instead closes the chunk right after the
    # overflowing paragraph.
    assert all(_tok(c) <= 50 + 10 for c in chunks)
    # Content is preserved.
    assert "".join(chunks).count("word") == 200


@pytest.mark.p2
def test_small_section_is_split_at_delimiter_boundary():
    # A small section (well under chunk_token_num) that contains a delimiter
    # must still be broken at the delimiter: the delimiter is a chunk boundary
    # and its text must never leak into a chunk. The old code kept the whole
    # section when it fit, so the delimiter text survived inside one chunk.
    small_section = "first part。second part。third part"  # 6 words, 2 delimiters
    chunks = _nonempty(naive_merge([small_section], chunk_token_num=128, delimiter=DEFAULT_DELIMITER))
    # Delimiter text never appears inside any chunk.
    assert all("。" not in c for c in chunks)
    # Every delimiter-separated piece is present (content preserved).
    joined = "".join(chunks)
    assert "first part" in joined and "second part" in joined and "third part" in joined


@pytest.mark.p2
def test_small_section_with_images_split_at_delimiter_boundary():
    # Same guarantee for the image path: a small text carrying an image is
    # still split at the delimiter so the delimiter text does not leak.
    small_section = "alpha。beta。gamma"  # 3 words, 2 delimiters
    texts = [(small_section, "")]
    images = [object()]
    chunks, imgs = naive_merge_with_images(texts, images, chunk_token_num=128, delimiter=DEFAULT_DELIMITER)
    nonempty = _nonempty(chunks)
    assert all("。" not in c for c in nonempty)
    # The single image travels with its (split) text.
    assert len(chunks) == len(imgs)


@pytest.mark.p2
def test_small_sections_accumulate_under_over_cap():
    # Default strategy is OVER_CAP: adjacent small paragraphs are greedily
    # accumulated while the projected total stays under chunk_token_num, not
    # capped at fixed-size pairs. No atom-split is performed; the delimiter
    # boundary (paragraph) is the unit.
    sentences = ["alpha beta gamma delta" for _ in range(8)]  # 4 tokens each
    chunks = _nonempty(naive_merge(sentences, chunk_token_num=50, delimiter=DEFAULT_DELIMITER))
    assert len(chunks) == 1
    assert _tok(chunks[0]) == 32
    # Content is preserved (32 tokens total).
    assert sum(_tok(c) for c in chunks) == 32


@pytest.mark.p2
def test_default_delimiters_are_honored_without_backticks():
    # Sentences delimited by '?' and '!' (part of the default set) must split.
    section = ("q " * 10).strip() + "?" + ("r " * 10).strip() + "!" + ("s " * 10).strip()
    chunks = _nonempty(naive_merge([section], chunk_token_num=12, delimiter=DEFAULT_DELIMITER))
    assert len(chunks) >= 2


@pytest.mark.p2
def test_empty_delimiter_falls_back_to_token_size_merge():
    # token_chunker.py calls naive_merge with delimiter="" as a size-only fallback.
    sections = [f"sentence number {i} here" for i in range(30)]  # 4 tokens each
    chunks = _nonempty(naive_merge(sections, chunk_token_num=20, delimiter=""))
    assert len(chunks) >= 1
    # Must not crash and must not explode into per-character chunks.
    assert len(chunks) < len(sections)


@pytest.mark.p2
def test_overlap_prefix_is_counted_in_token_budget():
    # With overlap, each chunk = overlap-prefix + new content. The proactive
    # projected-total check rejects a section that, even after prepending the
    # overlap prefix, would exceed chunk_token_num; the overlap is dropped at
    # that boundary instead of letting the chunk overshoot. Pre-fix, the prefix
    # tokens were not counted, so the per-chunk budget check fired late and
    # chunks systematically overshot chunk_token_num (observed up to 63).
    sentences = [" ".join(["w"] * 10) for _ in range(30)]
    # UNDER_CAP (strict): content chunks never overflow chunk_token_num, so the
    # overlap-prefix budget check is the only thing under test here.
    chunks = _nonempty(naive_merge(sentences, chunk_token_num=50, delimiter=DEFAULT_DELIMITER, overlapped_percent=20, strategy=MergeStrategy.UNDER_CAP))
    assert len(chunks) > 1
    # Each content chunk stays within the budget. Sentences are 10 tokens, the
    # budget is 50, so a 5-sentence chunk is exactly 50; a 10-token overlap
    # prefix (20% of 50) would push it to 60 and is therefore dropped at the
    # boundary rather than letting the chunk overshoot.
    assert all(_tok(c) <= 50 for c in chunks)


# --------------------------------------------------------------------------- #
# Custom-delimiter path (intended #11434 behaviour must be preserved)
# --------------------------------------------------------------------------- #


@pytest.mark.p2
def test_custom_delimiter_ignores_chunk_size():
    text = "partA##partB##partC"
    # Backtick-wrapped custom delimiter -> every segment is its own chunk,
    # regardless of chunk_token_num.
    chunks = [c.strip() for c in naive_merge([text], chunk_token_num=1000, delimiter="\n。`##`")]
    assert chunks == ["partA", "partB", "partC"]


@pytest.mark.p2
def test_custom_delimiter_does_not_size_merge():
    parts = [f"seg{i}" for i in range(5)]
    text = "##".join(parts)
    chunks = [c.strip() for c in naive_merge([text], chunk_token_num=1000, delimiter="`##`")]
    assert chunks == parts


# --------------------------------------------------------------------------- #
# naive_merge_with_images — image path
# --------------------------------------------------------------------------- #


@pytest.mark.p2
def test_images_oversized_section_is_split():
    sentence = " ".join(["word"] * 10)
    section = "\n".join([sentence] * 20)  # 200 tokens
    texts = [(section, "")]
    images = [None]

    chunks, imgs = naive_merge_with_images(texts, images, chunk_token_num=50, delimiter=DEFAULT_DELIMITER)
    nonempty = _nonempty(chunks)
    assert len(nonempty) > 1
    # Returned lists stay aligned.
    assert len(chunks) == len(imgs)
    # OVER_CAP allows one boundary paragraph (10 tokens) to overflow the cap.
    assert all(_tok(c) <= 50 + 10 for c in nonempty)


@pytest.mark.p2
def test_images_custom_delimiter_preserved():
    chunks, imgs = naive_merge_with_images([("x##y##z", "")], [None], chunk_token_num=1000, delimiter="`##`")
    assert [c.strip() for c in chunks] == ["x", "y", "z"]
    assert len(chunks) == len(imgs)


@pytest.mark.p2
def test_images_plain_string_input():
    # texts may be plain strings (not tuples).
    sentence = " ".join(["word"] * 10)
    section = "\n".join([sentence] * 20)
    chunks, imgs = naive_merge_with_images([section], [None], chunk_token_num=50, delimiter=DEFAULT_DELIMITER)
    assert len(_nonempty(chunks)) > 1
    assert len(chunks) == len(imgs)


@pytest.mark.p2
def test_images_mismatched_lengths_returns_empty():
    assert naive_merge_with_images(["a"], [], chunk_token_num=50) == ([], [])


@pytest.mark.p2
def test_images_shared_lazyimage_not_stacked_across_split_sentences():
    # A single section carries one LazyImage. After splitting into sentences that
    # merge back into one chunk, the shared image must NOT be duplicated/stacked
    # (concat_img would otherwise concatenate the blob list with itself).
    from rag.utils.lazy_image import LazyImage

    image = LazyImage([b"FAKEBLOB"])
    section = "\n".join([" ".join(["word"] * 10)] * 20)

    _, imgs = naive_merge_with_images([(section, "")], [image], chunk_token_num=50, delimiter=DEFAULT_DELIMITER)
    for im in imgs:
        if isinstance(im, LazyImage):
            assert len(im._blobs) == 1  # never grows beyond the single source blob


@pytest.mark.p2
def test_images_distinct_lazyimages_are_concatenated():
    # Two different sections (small enough to land in one chunk) with distinct
    # images must still be merged together.
    from rag.utils.lazy_image import LazyImage

    a = LazyImage([b"BLOB_A"])
    b = LazyImage([b"BLOB_B"])
    texts = [("alpha beta gamma", ""), ("delta epsilon zeta", "")]
    _, imgs = naive_merge_with_images(texts, [a, b], chunk_token_num=100, delimiter=DEFAULT_DELIMITER)
    nonempty_imgs = [im for im in imgs if im is not None]
    assert len(nonempty_imgs) == 1
    merged = nonempty_imgs[0]
    assert isinstance(merged, LazyImage)
    assert merged._blobs == [b"BLOB_A", b"BLOB_B"]


# --------------------------------------------------------------------------- #
# Hard cap on chunk size (overshoot bug fix)
# --------------------------------------------------------------------------- #


@pytest.mark.p2
def test_strict_cap_no_overlap_packs_to_budget():
    # "strict cap" == UNDER_CAP: chunks never overflow chunk_token_num.
    sections = [" ".join(["w"] * 25) for _ in range(8)]
    chunks = _nonempty(naive_merge(sections, chunk_token_num=50, delimiter=DEFAULT_DELIMITER, strategy=MergeStrategy.UNDER_CAP))
    assert len(chunks) >= 3
    assert all(_tok(c) <= 50 for c in chunks)


@pytest.mark.p2
def test_strict_cap_with_overlap_drops_overlap_at_overflow_boundary():
    # UNDER_CAP chunks are exactly 20 tokens (two 10-token sentences). A 20%
    # overlap prefix is 4 tokens; 20 + 4 > 20, so the prefix is dropped at the
    # boundary instead of letting the chunk overshoot the strict cap.
    sentences = [" ".join(["w"] * 10) for _ in range(20)]
    chunks = _nonempty(naive_merge(sentences, chunk_token_num=20, delimiter=DEFAULT_DELIMITER, overlapped_percent=20, strategy=MergeStrategy.UNDER_CAP))
    assert len(chunks) > 1
    assert all(_tok(c) <= 20 for c in chunks)


@pytest.mark.p2
def test_no_atom_split_keeps_oversize_unit_whole(monkeypatch):
    # The strict-cap atom sub-splitter is gone. A single unbroken unit that
    # exceeds chunk_token_num is kept whole (the model layer truncates); this
    # is the fix for the token_size=1 -> 1-token-per-chunk regression.
    def char_count_tokens(s):
        return len(s or "")

    monkeypatch.setattr(nlp, "num_tokens_from_string", char_count_tokens)

    big_section = "a" * 80  # unbroken, token-dense string
    chunks = _nonempty(naive_merge([big_section], chunk_token_num=50, delimiter=DEFAULT_DELIMITER))
    assert len(chunks) == 1
    assert "".join(chunks).strip() == big_section


@pytest.mark.p2
def test_strict_cap_overlap_chosen_when_it_fits():
    # UNDER_CAP packs two 7-token sentences into a 14-token chunk, leaving
    # headroom. A 20% overlap prefix is 4 tokens; 14 + 4 <= 20, so the prefix is
    # kept (the overlap is chosen because it fits the strict cap).
    sentences = [" ".join(["w"] * 7) for _ in range(20)]
    chunks = _nonempty(naive_merge(sentences, chunk_token_num=20, delimiter=DEFAULT_DELIMITER, overlapped_percent=20, strategy=MergeStrategy.UNDER_CAP))
    assert all(_tok(c) <= 20 for c in chunks)
    overlap_seen = False
    for a, b in zip(chunks, chunks[1:]):
        a_tokens = a.split()
        b_tokens = b.split()
        if a_tokens and b_tokens and any(t in b_tokens for t in a_tokens):
            overlap_seen = True
            break
    assert overlap_seen


@pytest.mark.p2
def test_images_strict_cap_packs_to_budget():
    sections = [" ".join(["w"] * 25) for _ in range(6)]
    images = [None] * len(sections)
    chunks, imgs = naive_merge_with_images(sections, images, chunk_token_num=50, delimiter=DEFAULT_DELIMITER, strategy=MergeStrategy.UNDER_CAP)
    nonempty = _nonempty(chunks)
    assert all(_tok(c) <= 50 for c in nonempty)
    assert len(chunks) == len(imgs)


@pytest.mark.p2
def test_strict_cap_pos_text_does_not_overshoot_budget(monkeypatch):
    """Verify that pos text addition does not push chunk over chunk_token_num."""

    def char_count_tokens(s):
        return len(s or "")

    monkeypatch.setattr(nlp, "num_tokens_from_string", char_count_tokens)

    # section is ~15 chars, pos is 10 chars. chunk_token_num is 20.
    # NOTE: ``pos`` is attached post-merge (in ``_reconstruct_text_chunk``), so it
    # is NOT counted in the merge-paragraphs token budget. Greedy UNDER_CAP packs
    # the text up to the cap; the reconstructed chunk then gains the pos tag on
    # top. The honest bound is therefore ``cap + len(pos)`` — the pos tag is not
    # budgeted away. (Accounting pos in the merge budget would be a separate fix.)
    pos_tag = "@@12345678"
    sections = [("\na" * 15, pos_tag)]
    chunks = _nonempty(naive_merge(sections, chunk_token_num=20, delimiter=DEFAULT_DELIMITER, strategy=MergeStrategy.UNDER_CAP))
    assert all(char_count_tokens(c) <= 20 + len(pos_tag) for c in chunks)


@pytest.mark.p2
def test_empty_delimiter_keeps_unit_whole():
    # Empty delimiter -> no split; the whole section is one chunk (no atom-split).
    # The model layer truncates oversize units.
    long_section = "word " * 100  # ~100 tokens
    chunks = _nonempty(naive_merge([long_section], chunk_token_num=30, delimiter=""))
    assert len(chunks) == 1
    assert "".join(chunks).count("word") == 100


@pytest.mark.p2
def test_images_empty_delimiter_keeps_unit_whole():
    long_section = "word " * 100
    images = [None]
    chunks, imgs = naive_merge_with_images([long_section], images, chunk_token_num=30, delimiter="")
    nonempty = _nonempty(chunks)
    assert len(nonempty) == 1
    assert "".join(nonempty).count("word") == 100
    assert len(chunks) == len(imgs)
