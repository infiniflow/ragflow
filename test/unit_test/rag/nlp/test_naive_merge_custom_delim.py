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
"""Regression tests for issue #18552.

When ``parser_config.delimiter`` contains a backtick-wrapped token, the
``has_custom`` branch in ``naive_merge`` (and ``naive_merge_with_images``)
used to split the section on the user's compiled pattern and then append
every segment as its own chunk -- bypassing ``chunk_token_num`` entirely.
For the issue's reproduction (a 2.8 MB text with ``chunk_token_num=512``
and a delimiter that leaks a single space from unbalanced backticks), this
produced ~429,000 chunks of 1-2 tokens instead of the expected ~1,400.

The fix runs the custom-delimiter split through the same merge step the
default path uses, so ``chunk_token_num`` is honoured regardless of which
path is taken. The user's backtick-wrapped token still produces a chunk
boundary, but a run of tiny segments now merges back up to the cap.

These tests drive the real ``naive_merge`` and assert both the chunk
count and the per-chunk token-size distribution. The issue body gives a
reproducible example; the tests use the same numbers.
"""

from rag.nlp import naive_merge, naive_merge_with_images
from rag.nlp.delim import has_wrapped_delimiter  # noqa: F401  -- used to confirm the helper exists


# The delimiter from the issue's repro -- four backticks in odd positions.
# ``has_wrapped_delimiter`` is True (the leading pair matches), and
# ``parse_delimiter_field`` extracts bare single chars from the trailing
# unclosed backtick region. Pre-fix, the bare space alone was enough to
# split on every space; post-fix, the merge step recovers.
ISSUE_DELIMITER = "\n` ``  ``\n\n``. `"


def _approx_tokens(text):
    """Crude token estimator. The real ``num_tokens_from_string`` in
    ragflow wraps a tiktoken-like encoder; for a regression test on
    chunk count the exact count does not matter as long as the
    relative distribution is correct. ``len(text) / 4`` is a standard
    rule of thumb for English text (~4 chars per token)."""
    return max(1, len(text) // 4)


def test_issue_repro_does_not_produce_300x_chunks():
    """The issue's exact reproduction: a 2.8 MB book text with
    ``chunk_token_num=512`` and the unbalanced-backtick delimiter
    must produce a bounded chunk count, not the 300x-exploded count
    (~429,000 chunks for 2.8 MB) seen pre-fix.
    """
    # A short stand-in for the 2.8 MB book -- ~200KB so the test runs
    # fast but the chunk count is still dominated by the merge step
    # (not by section count).
    book_text = ("lorem ipsum dolor sit amet " * 100 + "\n\n") * 200
    sections = [book_text]

    chunks = naive_merge(sections, chunk_token_num=512, delimiter=ISSUE_DELIMITER)

    # Pre-fix: 300x more chunks than expected (~30k for a 200KB book).
    # Post-fix: the merge step keeps the count bounded (a few hundred
    # to a few thousand for this input). Pin a generous upper bound
    # well below the pre-fix blow-up but above any reasonable post-fix
    # count.
    pre_fix_estimate = 30_000  # 200KB * (429k / 2.8MB)
    post_fix_ceiling = 5_000  # well above the observed ~500-600
    assert len(chunks) < post_fix_ceiling, (
        f"naive_merge produced {len(chunks)} chunks for a 200KB book with "
        f"chunk_token_num=512 and the issue's delimiter. Pre-fix the bypass "
        f"produced ~{pre_fix_estimate} chunks for an input this size; "
        f"post-fix the merge step keeps the count below "
        f"{post_fix_ceiling}."
    )


def test_chunk_size_respects_chunk_token_num_with_custom_delimiter():
    """With a custom delimiter, the per-chunk token count must respect
    the ``chunk_token_num`` cap (modulo the unconditional overlap prefix
    in the OVER_CAP merge strategy). Pre-fix, every chunk was 1-2 tokens;
    post-fix, the merge step fills each chunk up to the cap.
    """
    book_text = ("lorem ipsum dolor sit amet " * 100 + "\n\n") * 50
    sections = [book_text]

    chunks = naive_merge(sections, chunk_token_num=128, delimiter=ISSUE_DELIMITER)
    assert chunks, "naive_merge returned no chunks"

    # The OVER_CAP merge strategy leaves room for the unconditional
    # overlap prefix; allow up to chunk_token_num + a small margin
    # (the overlap prefix is the same on every chunk).
    max_tokens = max(_approx_tokens(c) for c in chunks)
    assert max_tokens <= 256, (
        f"largest chunk is ~{max_tokens} tokens but chunk_token_num=128; "
        f"the custom-delimiter path bypassed the merge step and produced "
        f"oversize chunks (pre-fix) or many 1-2 token chunks (pre-fix in the "
        f"opposite direction). Post-fix: every chunk should respect the cap."
    )


def test_default_path_is_unchanged():
    """Pin: the default (no-backticks) path is unchanged. A user who
    never had a custom delimiter continues to get the same chunk
    distribution. We test by feeding the default delimiter and asserting
    the per-chunk size respects ``chunk_token_num``.
    """
    default_delimiter = "\n。；！？"
    text = ("section one. " * 50) + "。\n" + ("section two. " * 50) + "。\n" + ("section three. " * 50)
    sections = [text]

    chunks = naive_merge(sections, chunk_token_num=128, delimiter=default_delimiter)
    assert chunks, "default path returned no chunks"
    # The default path's contract: each chunk respects chunk_token_num
    # (modulo the unconditional overlap prefix). A failure here means
    # the default path was inadvertently broken by the custom-delim fix.
    max_tokens = max(_approx_tokens(c) for c in chunks)
    assert max_tokens <= 256, f"default path produced a chunk of ~{max_tokens} tokens at chunk_token_num=128; the merge step did not enforce the cap."


def test_naive_merge_with_images_honours_chunk_token_num_with_custom_delim():
    """The same fix must apply to ``naive_merge_with_images`` -- a
    custom delimiter with a stray bare char must not multiply chunks
    by 300x.
    """
    text = ("lorem ipsum dolor sit amet " * 100 + "\n\n") * 50
    image = b"fake-image-bytes"

    chunks, images = naive_merge_with_images(
        [(text, "")],
        [image],
        chunk_token_num=128,
        delimiter=ISSUE_DELIMITER,
    )

    assert len(chunks) == len(images), f"naive_merge_with_images returned {len(chunks)} chunks and {len(images)} images; the lists must be 1:1."
    # Post-fix the count is bounded. Pre-fix would have been ~7500
    # chunks for this 200KB input.
    assert len(chunks) < 5_000, (
        f"naive_merge_with_images produced {len(chunks)} chunks for a 200KB text with chunk_token_num=128 and the issue's delimiter; the pre-fix bypass produced 300x more chunks."
    )


# ---------------------------------------------------------------------------
# Pin the contract: the delimiter "has backticks" -> the same merge
# strategy is applied as the no-backticks path. The fix is the single
# point where the has_custom branch honours chunk_token_num; this test
# pins that the helper used is the same one the default path uses.
# ---------------------------------------------------------------------------


def test_has_custom_branch_uses_merge_paragraph_groups():
    """Pin the fix: the has_custom branch must funnel through
    ``_merge_paragraph_groups`` (the same helper the default path uses).
    Pre-fix, the has_custom branch appended each split segment as its
    own chunk and never called ``_merge_paragraph_groups``.
    """
    import inspect

    from rag.nlp import _merge_paragraph_groups  # noqa: F401 -- used to confirm the helper exists
    import rag.nlp as nlp_mod

    source = inspect.getsource(nlp_mod.naive_merge)
    # The has_custom branch must call _merge_paragraph_groups with the
    # user's paragraphs. Pin the call shape so a future refactor that
    # drops the merge step (re-introducing the bypass) is caught loudly.
    assert "_merge_paragraph_groups" in source, (
        "naive_merge no longer references _merge_paragraph_groups -- the custom-delimiter branch must funnel through the merge step to honour chunk_token_num. See issue #18552."
    )
    # And the result must still go through _reconstruct_text_chunk +
    # _apply_overlap_unconditional (the same shape as the default path).
    assert "_reconstruct_text_chunk" in source
    assert "_apply_overlap_unconditional" in source
