#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
"""Unit tests for the block-based pagination window used by Dealer.retrieval.

retrieval reuses RERANK_LIMIT both as the backend block size
(req["page"] = global_offset // RERANK_LIMIT) and as the modulus for slicing a
page out of a (re)ranked block (begin = global_offset % RERANK_LIMIT). If the
window is not an exact multiple of page_size, blocks and pages drift apart, so
deep pages silently drop results and come back short. These tests pin that
invariant and verify cross-block pagination loses nothing.
"""
import math
import sys
import types

import pytest

# Stub the heavy / circular-importing dependencies before importing search,
# mirroring test_rank_feature_scores.py so the module imports in isolation.
_fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


_fake_query.FulltextQueryer = _DummyFulltextQueryer
sys.modules.setdefault("rag.nlp.query", _fake_query)
sys.modules.setdefault("common.settings", types.ModuleType("common.settings"))

from rag.nlp.search import Dealer  # noqa: E402

_rerank_window = Dealer._rerank_window

# (page_size, top) combinations, including the common page sizes (10, 30) that
# do NOT divide 64 -- the exact case the old `min(..., 64)` clamp broke -- plus
# tiny / large / page-aligned tops.
GRID = [
    (page_size, top)
    for page_size in (1, 5, 7, 10, 30, 50, 64)
    for top in (0, 5, 30, 50, 55, 64, 100, 1024)
]


def _paginate(total, page_size, top, rerank):
    """Replay retrieval's block-fetch + in-block slice over `total` candidates.

    Returns the concatenated global positions actually surfaced across every
    page, exactly as Dealer.retrieval would emit them.
    """
    window = _rerank_window(page_size, top if rerank else 0)
    # The backend caps the candidate pool at `top` when an external reranker is
    # active; otherwise the whole result set is windowed.
    cap = min(total, top) if (rerank and top > 0) else total
    surfaced = []
    page = 1
    while (page - 1) * page_size < cap:
        global_offset = (page - 1) * page_size
        block_index = global_offset // window
        block_start = block_index * window
        block = list(range(block_start, min(block_start + window, cap)))
        begin = global_offset % window
        surfaced.extend(block[begin:begin + page_size])
        page += 1
    return window, cap, surfaced


@pytest.mark.parametrize("page_size,top", GRID)
def test_window_is_page_aligned(page_size, top):
    """The window must be a positive whole multiple of page_size."""
    for rerank in (False, True):
        window = _rerank_window(page_size, top if rerank else 0)
        assert window >= 1
        if page_size > 1:
            assert window % page_size == 0, (page_size, top, rerank, window)


@pytest.mark.parametrize("page_size,top", GRID)
def test_pagination_loses_nothing(page_size, top):
    """Walking every page reconstructs the candidate pool exactly: in order,
    no gaps, no duplicates, and no short interior pages."""
    total = 250
    for rerank in (False, True):
        window, cap, surfaced = _paginate(total, page_size, top, rerank)
        assert surfaced == list(range(cap)), (
            f"page_size={page_size} top={top} rerank={rerank} window={window} "
            f"cap={cap}: missing={sorted(set(range(cap)) - set(surfaced))[:10]} "
            f"dups={len(surfaced) != len(set(surfaced))}"
        )


@pytest.mark.p1
def test_reported_regression_page7_not_short():
    """The reported case: page_size=10, top=1024, reranker on. Page 7 (global
    offset 60) used to return only 4 of 10 results because the window was
    clamped to 64 (not a multiple of 10)."""
    page_size, top = 10, 1024
    window = _rerank_window(page_size, top)
    assert window % page_size == 0
    assert window >= 64  # still a provider-friendly ~64 candidate pool

    total = 250
    _, cap, surfaced = _paginate(total, page_size, top, rerank=True)
    # Page 7 spans global positions 60..69 and must be full and contiguous.
    assert surfaced[60:70] == list(range(60, 70))
    assert len(surfaced) == cap


@pytest.mark.p1
def test_matches_legacy_window_on_non_buggy_paths():
    """Where the old formula already produced a page-aligned value, the new
    window is unchanged (no behavioral regression on the non-buggy paths)."""
    def legacy(page_size, top, rerank):
        limit = math.ceil(64 / page_size) * page_size if page_size > 1 else 1
        limit = max(30, limit)
        if rerank and top > 0:
            limit = min(limit, top, 64)
        return limit

    for page_size in (1, 5, 7, 10, 30, 50, 64):
        # The non-rerank path was always page-aligned already -> must match.
        assert _rerank_window(page_size, 0) == legacy(page_size, 0, False)
