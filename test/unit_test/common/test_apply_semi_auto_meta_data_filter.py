import pytest
from common.metadata_utils import apply_meta_data_filter
from unittest.mock import MagicMock, AsyncMock, patch


@pytest.mark.asyncio
async def test_apply_meta_data_filter_semi_auto_key():
    meta_data_filter = {"method": "semi_auto", "semi_auto": ["key1", "key2"]}
    metas = {"key1": {"val1": ["doc1"]}, "key2": {"val2": ["doc2"]}}
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
    meta_data_filter = {"method": "semi_auto", "semi_auto": [{"key": "key1", "op": ">"}, "key2"]}
    metas = {"key1": {"10": ["doc1"]}, "key2": {"val2": ["doc2"]}}
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


@pytest.mark.asyncio
async def test_apply_meta_data_filter_semi_auto_no_match_returns_no_results_sentinel():
    # A generated filter that matches no document must yield the ["-999"] "no results"
    # sentinel, not None (which would fall back to unrestricted retrieval).
    meta_data_filter = {"method": "semi_auto", "semi_auto": ["date"]}
    metas = {"date": {"2026-04-23": ["doc1"]}}
    question = "what happened on 2026-04-30"

    chat_mdl = MagicMock()

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {"conditions": [{"key": "date", "op": "=", "value": "2026-04-30"}], "logic": "and"}

        doc_ids = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl)
        assert doc_ids == ["-999"]


@pytest.mark.asyncio
async def test_apply_meta_data_filter_auto_no_match_returns_no_results_sentinel():
    # Same as above for the `auto` method.
    meta_data_filter = {"method": "auto"}
    metas = {"date": {"2026-04-23": ["doc1"]}}
    question = "what happened on 2026-04-30"

    chat_mdl = MagicMock()

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {"conditions": [{"key": "date", "op": "=", "value": "2026-04-30"}], "logic": "and"}

        doc_ids = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl)
        assert doc_ids == ["-999"]


@pytest.mark.asyncio
async def test_apply_meta_data_filter_semi_auto_no_conditions_returns_none():
    # When the LLM produces no conditions there is nothing to filter on: return None
    # (no metadata filter), leaving retrieval unrestricted.
    meta_data_filter = {"method": "semi_auto", "semi_auto": ["date"]}
    metas = {"date": {"2026-04-23": ["doc1"]}}
    question = "hello"

    chat_mdl = MagicMock()

    with patch("rag.prompts.generator.gen_meta_filter", new_callable=AsyncMock) as mock_gen:
        mock_gen.return_value = {"conditions": [], "logic": "and"}

        doc_ids = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl)
        assert doc_ids is None
