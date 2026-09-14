from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from common.metadata_utils import apply_meta_data_filter


@pytest.mark.asyncio
async def test_manual_filter_intersects_base_doc_ids():
    metas = {"color": {"red": ["docB"], "blue": ["docA"]}}
    flt = {"method": "manual", "logic": "and", "manual": [{"key": "color", "op": "=", "value": "red"}]}

    doc_ids = await apply_meta_data_filter(flt, metas, base_doc_ids=["docA"], kb_ids=None)

    assert doc_ids == ["-999"]


@pytest.mark.asyncio
async def test_manual_filter_returns_intersection_without_duplicates():
    metas = {"color": {"red": ["docA", "docB"]}}
    flt = {"method": "manual", "logic": "and", "manual": [{"key": "color", "op": "=", "value": "red"}]}

    doc_ids = await apply_meta_data_filter(flt, metas, base_doc_ids=["docA"], kb_ids=None)

    assert doc_ids == ["docA"]


@pytest.mark.asyncio
async def test_manual_filter_keeps_base_scope_order():
    metas = {"color": {"red": ["docC", "docB"]}}
    flt = {"method": "manual", "logic": "and", "manual": [{"key": "color", "op": "=", "value": "red"}]}

    doc_ids = await apply_meta_data_filter(flt, metas, base_doc_ids=["docA", "docB", "docC", "docD"], kb_ids=None)

    assert doc_ids == ["docB", "docC"]


@pytest.mark.asyncio
async def test_manual_filter_without_base_scope_returns_all_hits():
    metas = {"color": {"red": ["docB", "docA"], "blue": ["docZ"]}}
    flt = {"method": "manual", "logic": "and", "manual": [{"key": "color", "op": "=", "value": "red"}]}

    doc_ids = await apply_meta_data_filter(flt, metas, kb_ids=None)

    assert doc_ids == ["docB", "docA"]


@pytest.mark.asyncio
async def test_auto_filter_intersects_base_scope():
    metas = {"color": {"red": ["docA", "docB"]}}
    chat_mdl = MagicMock()

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {"conditions": [{"key": "color", "op": "=", "value": "red"}], "logic": "and"}

        doc_ids = await apply_meta_data_filter({"method": "auto"}, metas, "question", chat_mdl, base_doc_ids=["docA"], kb_ids=None)

    assert doc_ids == ["docA"]


@pytest.mark.asyncio
async def test_auto_filter_returns_none_when_base_scope_excluded():
    metas = {"color": {"red": ["docB"]}}
    chat_mdl = MagicMock()

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {"conditions": [{"key": "color", "op": "=", "value": "red"}], "logic": "and"}

        doc_ids = await apply_meta_data_filter({"method": "auto"}, metas, "question", chat_mdl, base_doc_ids=["docA"], kb_ids=None)

    assert doc_ids is None
