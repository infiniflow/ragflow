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

Guards against the regression introduced by commit db0f6840d (#11434) where the
default (non-custom-delimiter) path stopped splitting oversized sections at
sentence boundaries, and the overlap prefix was not counted toward a chunk's
token budget.
"""

import re

import pytest

import rag.nlp as nlp
from rag.nlp import naive_merge, naive_merge_with_images

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
    # No chunk should greatly exceed the budget (allow one trailing sentence of slack).
    assert all(_tok(c) <= 50 + 10 for c in chunks)
    # Content is preserved.
    assert "".join(chunks).count("word") == 200


@pytest.mark.p2
def test_small_sections_are_merged_not_oversplit():
    sentences = ["alpha beta gamma delta" for _ in range(8)]  # 4 tokens each
    chunks = _nonempty(naive_merge(sentences, chunk_token_num=50, delimiter=DEFAULT_DELIMITER))
    # All 32 tokens comfortably fit one chunk.
    assert len(chunks) == 1
    assert _tok(chunks[0]) == 32


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
    # With overlap, each chunk = overlap-prefix + new content. The fix recomputes
    # the chunk's token count after prepending the prefix, so chunks stay bounded.
    # Pre-fix, the prefix tokens were not counted, so the per-chunk budget check
    # fired late and chunks systematically overshot chunk_token_num.
    sentences = [" ".join(["w"] * 10) for _ in range(30)]
    chunks = _nonempty(naive_merge(sentences, chunk_token_num=50, delimiter=DEFAULT_DELIMITER, overlapped_percent=20))
    assert len(chunks) > 1
    # Each 10-token sentence divides chunk_token_num evenly, so a correct
    # accounting yields chunks of exactly the budget. The buggy version
    # overshot (observed up to 63). A small tolerance guards tokenizer rounding.
    assert all(_tok(c) <= 50 + 2 for c in chunks)


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
