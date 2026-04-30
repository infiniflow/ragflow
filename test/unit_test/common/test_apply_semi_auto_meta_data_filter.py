import pytest
from common.metadata_utils import apply_meta_data_filter
from unittest.mock import MagicMock, AsyncMock, patch

pytestmark = [pytest.mark.p2]

@pytest.mark.asyncio
async def test_apply_meta_data_filter_semi_auto_key():
    meta_data_filter = {
        "method": "semi_auto",
        "semi_auto": ["key1", "key2"]
    }
    metas = {
        "key1": {"val1": ["doc1"]},
        "key2": {"val2": ["doc2"]}
    }
    question = "find val1"

    chat_mdl = MagicMock()

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {"conditions": [{"key": "key1", "op": "=", "value": "val1"}], "logic": "and"}

        doc_ids = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl)
        assert doc_ids == ["doc1"]

        # Check that constraints is an empty dict by default for legacy
        mock_gen.assert_called_once()
        args, kwargs = mock_gen.call_args
        assert kwargs["constraints"] == {}

@pytest.mark.asyncio
async def test_apply_meta_data_filter_semi_auto_key_and_operator():
    meta_data_filter = {
        "method": "semi_auto",
        "semi_auto": [{"key": "key1", "op": ">"}, "key2"]
    }
    metas = {
        "key1": {"10": ["doc1"]},
        "key2": {"val2": ["doc2"]}
    }
    question = "find key1 > 5"

    chat_mdl = MagicMock()

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {"conditions": [{"key": "key1", "op": ">", "value": "5"}], "logic": "and"}

        doc_ids = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl)
        assert doc_ids == ["doc1"]

        # Check that constraints are correctly passed
        mock_gen.assert_called_once()
        args, kwargs = mock_gen.call_args
        assert kwargs["constraints"] == {"key1": ">"}


# --- Tests for https://github.com/infiniflow/ragflow/issues/13987 ---

@pytest.mark.asyncio
async def test_auto_filter_empty_metas_returns_none():
    """When metas is empty (no metadata tags), auto mode should return None
    without calling the LLM, so that normal retrieval proceeds unfiltered.

    Downstream callers (retrieval.py, dialog_service.py, chunk_app.py) pass
    the result as ``doc_ids=result`` to ``retriever.retrieval()``, and
    ``get_filters`` in ``rag/nlp/search.py`` skips the doc_id filter when
    ``doc_ids is None``."""
    meta_data_filter = {"method": "auto"}
    metas = {}
    question = "anything"

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        result = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl=MagicMock())
        assert result is None
        mock_gen.assert_not_called()


@pytest.mark.asyncio
async def test_auto_filter_empty_metas_preserves_base_doc_ids():
    """When metas is empty but base_doc_ids is provided, auto mode should
    return the original base_doc_ids unchanged."""
    meta_data_filter = {"method": "auto"}
    metas = {}
    question = "anything"
    base_doc_ids = ["doc1", "doc2"]

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        result = await apply_meta_data_filter(
            meta_data_filter, metas, question, chat_mdl=MagicMock(), base_doc_ids=base_doc_ids,
        )
        assert result == ["doc1", "doc2"]
        mock_gen.assert_not_called()


@pytest.mark.asyncio
async def test_auto_filter_no_match_falls_back_to_none():
    """When auto mode generates conditions that match no documents, the
    function should return None (no restriction) so downstream search
    proceeds without a doc_id filter."""
    meta_data_filter = {"method": "auto"}
    metas = {"author": {"alice": ["doc1"]}}
    question = "find bob"

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {
            "conditions": [{"key": "author", "op": "=", "value": "bob"}],
            "logic": "and",
        }
        result = await apply_meta_data_filter(
            meta_data_filter, metas, question, chat_mdl=MagicMock(),
        )
        # Should return None (no restriction) so retrieval proceeds normally
        assert result is None


@pytest.mark.asyncio
async def test_auto_filter_with_match_returns_matched_docs():
    """When auto mode generates conditions that match documents, those
    doc_ids should be returned."""
    meta_data_filter = {"method": "auto"}
    metas = {"author": {"alice": ["doc1", "doc2"]}}
    question = "find alice"

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {
            "conditions": [{"key": "author", "op": "=", "value": "alice"}],
            "logic": "and",
        }
        result = await apply_meta_data_filter(
            meta_data_filter, metas, question, chat_mdl=MagicMock(),
        )
        assert sorted(result) == ["doc1", "doc2"]


@pytest.mark.asyncio
async def test_auto_filter_empty_conditions_returns_none():
    """When gen_meta_filter returns empty conditions, the function should
    return None (no restriction) without filtering."""
    meta_data_filter = {"method": "auto"}
    metas = {"author": {"alice": ["doc1"]}}
    question = "hello"

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {"conditions": [], "logic": "and"}
        result = await apply_meta_data_filter(
            meta_data_filter, metas, question, chat_mdl=MagicMock(),
        )
        assert result is None


@pytest.mark.asyncio
async def test_semi_auto_filter_empty_metas_falls_back():
    """When semi_auto mode has selected keys but metas is empty, the function
    should return None (no restriction) without error."""
    meta_data_filter = {
        "method": "semi_auto",
        "semi_auto": ["key1"],
    }
    metas = {}
    question = "anything"

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        result = await apply_meta_data_filter(
            meta_data_filter, metas, question, chat_mdl=MagicMock(),
        )
        assert result is None
        mock_gen.assert_not_called()


@pytest.mark.asyncio
async def test_semi_auto_filter_no_match_falls_back():
    """When semi_auto mode generates conditions that match no documents,
    the function should return None (no restriction) instead of an empty
    list, so downstream search proceeds without a doc_id filter."""
    meta_data_filter = {
        "method": "semi_auto",
        "semi_auto": ["author"],
    }
    metas = {"author": {"alice": ["doc1"]}}
    question = "find bob"

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {
            "conditions": [{"key": "author", "op": "=", "value": "bob"}],
            "logic": "and",
        }
        result = await apply_meta_data_filter(
            meta_data_filter, metas, question, chat_mdl=MagicMock(),
        )
        assert result is None
