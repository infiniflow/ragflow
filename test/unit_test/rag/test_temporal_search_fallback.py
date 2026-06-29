import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest

_fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


_fake_query.FulltextQueryer = _DummyFulltextQueryer
sys.modules.setdefault("rag.nlp.query", _fake_query)
sys.modules.setdefault("common.settings", types.ModuleType("common.settings"))

from rag.nlp.search import Dealer  # noqa: E402


@pytest.mark.p1
@pytest.mark.asyncio
@patch("rag.nlp.search.Dealer.__init__", return_value=None)
async def test_temporal_sort_scores_fallback_when_metadata_load_fails(mock_init):
    dealer = Dealer()
    dealer.qryr = _DummyFulltextQueryer()
    dealer.dataStore = None

    sim_np = np.array([0.8, 0.6], dtype=np.float64)
    sres = MagicMock()
    sres.ids = ["chunk-1", "chunk-2"]
    sres.field = {
        "chunk-1": {"doc_id": "doc-1", "kb_id": "kb-1"},
        "chunk-2": {"doc_id": "doc-2", "kb_id": "kb-1"},
    }
    rank_plan = MagicMock(
        enabled=True,
        temporal_field="post_date",
        half_life_days=14.0,
        freshness_offset_days=0.0,
        freshness_weight=0.15,
        future_date_policy="include_without_boost",
        shadow_mode=False,
    )

    with patch("rag.nlp.search.thread_pool_exec", new=AsyncMock(side_effect=RuntimeError("metadata down"))):
        with patch("rag.nlp.search.logging.warning") as warning_log:
            result = await dealer._temporal_sort_scores(sim_np, sres, rank_plan)

    assert np.array_equal(result, sim_np)
    warning_log.assert_called_once()
