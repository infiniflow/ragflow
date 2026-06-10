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

"""Regression tests for the shared reranker score-normalization contract.

Every reranker must return scores on a single ``[0, 1]`` scale so that the
hybrid blend in ``rag/nlp/search.py`` (``tkweight * tksim + vtweight * vtsim``)
stays comparable across providers. Historically only 3 of ~17 providers
normalized, and NVIDIA returned raw, unbounded logits — which corrupted
retrieval ordering. The contract is now enforced once in ``Base.similarity``.
"""

from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from rag.llm.rerank_model import (
    Base,
    JinaRerank,
    NvidiaRerank,
)

pytestmark = pytest.mark.p1


def _mock_post(payload):
    """Patch ``requests.post`` so ``response.json()`` returns ``payload``."""
    response = MagicMock()
    response.raise_for_status.return_value = None
    response.json.return_value = payload
    return patch("rag.llm.rerank_model.requests.post", return_value=response)


class _RawRerank(Base):
    """Minimal provider that emits arbitrary raw scores via ``_compute_rank``."""

    def __init__(self, raw):
        self._raw = np.asarray(raw, dtype=float)

    def _compute_rank(self, query, texts):
        return self._raw, 0


# --- The central guarantee: every provider's output lands in [0, 1] ----------


@pytest.mark.parametrize(
    "raw, expected",
    [
        # Unbounded NVIDIA-style logits, including negatives -> rescaled.
        ([10.0, -3.0, 0.0], [1.0, 0.0, 3.0 / 13.0]),
        # Large positive logits -> rescaled.
        ([100.0, 50.0, 75.0], [1.0, 0.0, 0.5]),
        # Negative-only logits -> rescaled.
        ([-1.0, -5.0, -3.0], [1.0, 0.0, 0.5]),
    ],
)
def test_out_of_range_scores_are_rescaled(raw, expected):
    rank, _ = _RawRerank(raw).similarity("q", ["a", "b", "c"])
    assert np.allclose(rank, expected)
    assert rank.min() >= 0.0 and rank.max() <= 1.0


@pytest.mark.parametrize(
    "raw",
    [
        [0.9, 0.1, 0.5],  # spread relevance scores
        [0.8, 0.8, 0.8],  # all-equal but valid -> not zeroed
        [1.0],  # single calibrated candidate -> not zeroed
        [0.0, 1.0, 0.42],  # already spanning the full range
    ],
)
def test_in_range_scores_are_preserved(raw):
    # Calibrated [0,1] providers (Cohere/Jina/Voyage/...) keep their absolute
    # magnitudes, so similarity_threshold and reported vector_similarity stay
    # meaningful and degenerate batches are NOT collapsed to zero.
    rank, _ = _RawRerank(raw).similarity("q", ["x"] * len(raw))
    assert np.allclose(rank, raw)


def test_normalization_preserves_ordering():
    raw = [-5.0, 12.0, 3.0, -1.0]
    rank, _ = _RawRerank(raw).similarity("q", ["a", "b", "c", "d"])
    assert list(np.argsort(rank)) == list(np.argsort(raw))


@pytest.mark.parametrize(
    "raw, expected",
    [
        # Single out-of-range candidate: clamped, never zeroed and never NaN.
        ([5.0], [1.0]),
        ([-3.0], [0.0]),
        # Spreadless out-of-range batch: clamped per element, not collapsed.
        ([5.0, 5.0, 5.0], [1.0, 1.0, 1.0]),
        ([-2.0, -2.0, -2.0], [0.0, 0.0, 0.0]),
    ],
)
def test_spreadless_out_of_range_batch_is_clamped(raw, expected):
    rank, _ = _RawRerank(raw).similarity("q", ["x"] * len(raw))
    assert np.allclose(rank, expected)
    assert not np.isnan(rank).any()


# --- Empty input short-circuits before any backend call ----------------------


@pytest.mark.parametrize("query, texts", [("", ["a"]), ("q", []), ("", [])])
def test_empty_input_returns_zeros_without_backend(query, texts):
    provider = _RawRerank([1.0])
    provider._compute_rank = MagicMock(side_effect=AssertionError("backend called"))
    rank, tokens = provider.similarity(query, texts)
    assert tokens == 0
    assert rank.size == len(texts)
    assert rank.dtype == float


# --- Per-provider: raw backend payloads come out normalized ------------------


def test_nvidia_logits_are_normalized():
    """NVIDIA emits raw logits; without central normalization a negative logit
    with vtweight=0.7 would sink a relevant chunk below keyword matches."""
    nv = NvidiaRerank("key", "nvidia/rerank-qa-mistral-4b")
    payload = {"rankings": [{"index": 0, "logit": 8.0}, {"index": 1, "logit": -4.0}, {"index": 2, "logit": 1.0}]}
    with _mock_post(payload):
        rank, _ = nv.similarity("q", ["a", "b", "c"])
    # _compute_rank still returns the raw logits (no per-provider normalization)...
    with _mock_post(payload):
        raw, _ = nv._compute_rank("q", ["a", "b", "c"])
    assert raw.min() < 0  # genuinely unbounded/negative
    # ...but the public contract normalizes them.
    assert np.allclose(rank, [1.0, 0.0, 5.0 / 12.0])
    assert rank.min() >= 0.0 and rank.max() <= 1.0


def test_calibrated_relevance_scores_are_preserved():
    # A provider already returning [0,1] relevance scores keeps them verbatim;
    # min-max would have stretched these to [1.0, 0.0, 0.5].
    jina = JinaRerank("key", base_url="http://x/rerank")
    payload = {"results": [{"index": 0, "relevance_score": 0.8}, {"index": 1, "relevance_score": 0.2}, {"index": 2, "relevance_score": 0.5}]}
    with _mock_post(payload):
        rank, _ = jina.similarity("q", ["a", "b", "c"])
    assert np.allclose(rank, [0.8, 0.2, 0.5])


# --- Structural guarantee: providers override _compute_rank, not similarity --


def test_providers_share_single_similarity_entrypoint():
    import inspect

    import rag.llm.rerank_model as rm

    overrides = []
    for _, cls in inspect.getmembers(rm, inspect.isclass):
        if issubclass(cls, Base) and cls is not Base and "similarity" in cls.__dict__:
            overrides.append(cls.__name__)
    assert overrides == [], f"providers must not override similarity(): {overrides}"
