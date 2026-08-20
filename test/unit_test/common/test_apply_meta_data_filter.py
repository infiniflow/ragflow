"""Unit tests for apply_meta_data_filter – covers the #13987 fix.

The bug: when a knowledge base had no metadata tags (empty ``metas``),
``apply_meta_data_filter`` still called ``gen_meta_filter`` and then returned
``None`` on the empty conditions.  Downstream code treated ``None`` as
"skip vector search entirely", so the user got zero results.

The fix (PR #13989) makes three changes:
1. Auto mode with empty ``metas`` -> skip the LLM call, return ``doc_ids``.
2. Auto/semi_auto mode where LLM conditions match nothing -> fall back to
   unfiltered retrieval (return base ``doc_ids``) instead of ``None``.
3. ``None`` is returned **only** when ``meta_data_filter`` is falsy.
"""

import sys
import types
import pytest
from unittest.mock import MagicMock, AsyncMock

# Inject a stub ``rag.prompts.generator`` module so the lazy import inside
# ``apply_meta_data_filter`` resolves without pulling in the full rag/nlp
# dependency chain (tiktoken, transformers, etc.).
_gen_meta_filter_mock = AsyncMock()
if "rag.prompts.generator" not in sys.modules:
    _stub = types.ModuleType("rag.prompts.generator")
    _stub.gen_meta_filter = _gen_meta_filter_mock
    sys.modules["rag.prompts.generator"] = _stub

from common.metadata_utils import apply_meta_data_filter


def _set_gen_filter(return_value=None):
    """Configure the stub gen_meta_filter and return the mock for assertions."""
    m = AsyncMock()
    if return_value is not None:
        m.return_value = return_value
    sys.modules["rag.prompts.generator"].gen_meta_filter = m
    return m


def _reset_gen_filter():
    """Set gen_meta_filter to a fresh mock that should never be called."""
    m = AsyncMock()
    sys.modules["rag.prompts.generator"].gen_meta_filter = m
    return m


# ---------------------------------------------------------------------------
# Falsy / unconfigured meta_data_filter
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_falsy_meta_data_filter_returns_empty_list():
    """When meta_data_filter is None, return [] (not None)."""
    result = await apply_meta_data_filter(None, metas={})
    assert result == []


@pytest.mark.asyncio
async def test_empty_dict_meta_data_filter_returns_empty_list():
    """When meta_data_filter is {}, return [] (not None)."""
    result = await apply_meta_data_filter({}, metas={})
    assert result == []


@pytest.mark.asyncio
async def test_falsy_meta_data_filter_preserves_base_doc_ids():
    """When meta_data_filter is None but base_doc_ids given, return them."""
    result = await apply_meta_data_filter(None, metas={}, base_doc_ids=["d1", "d2"])
    assert result == ["d1", "d2"]


# ---------------------------------------------------------------------------
# Auto mode – the core #13987 fix
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_auto_mode_empty_metas_skips_llm_call():
    """KB has no metadata tags -> skip gen_meta_filter entirely, return doc_ids."""
    meta_data_filter = {"method": "auto"}
    chat_mdl = MagicMock()
    mock_gen = _reset_gen_filter()

    result = await apply_meta_data_filter(meta_data_filter, metas={}, question="test", chat_mdl=chat_mdl)
    assert result == []
    mock_gen.assert_not_called()


@pytest.mark.asyncio
async def test_auto_mode_empty_metas_preserves_base_doc_ids():
    """KB has no tags but base_doc_ids exist -> return base_doc_ids, skip LLM."""
    meta_data_filter = {"method": "auto"}
    chat_mdl = MagicMock()
    mock_gen = _reset_gen_filter()

    result = await apply_meta_data_filter(
        meta_data_filter, metas={}, question="test", chat_mdl=chat_mdl, base_doc_ids=["base1"]
    )
    assert result == ["base1"]
    mock_gen.assert_not_called()


@pytest.mark.asyncio
async def test_auto_mode_llm_conditions_match_returns_matched_docs():
    """LLM generates conditions that match documents -> return matched doc_ids."""
    meta_data_filter = {"method": "auto"}
    metas = {"tag": {"python": ["doc1", "doc2"], "java": ["doc3"]}}
    chat_mdl = MagicMock()
    mock_gen = _set_gen_filter({
        "conditions": [{"key": "tag", "op": "=", "value": "python"}],
        "logic": "and",
    })

    result = await apply_meta_data_filter(meta_data_filter, metas, "python docs", chat_mdl)
    assert set(result) == {"doc1", "doc2"}


@pytest.mark.asyncio
async def test_auto_mode_llm_conditions_match_nothing_falls_back():
    """LLM conditions match nothing -> fall back to unfiltered (not None).

    This is the key regression test for #13987: previously the code returned
    None, causing vector search to be skipped entirely.
    """
    meta_data_filter = {"method": "auto"}
    metas = {"tag": {"python": ["doc1"], "java": ["doc2"]}}
    chat_mdl = MagicMock()
    mock_gen = _set_gen_filter({
        "conditions": [{"key": "tag", "op": "=", "value": "rust"}],
        "logic": "and",
    })

    result = await apply_meta_data_filter(meta_data_filter, metas, "rust docs", chat_mdl)
    # Should return [] (unfiltered fallback), NOT None
    assert result == []


@pytest.mark.asyncio
async def test_auto_mode_llm_conditions_match_nothing_preserves_base_doc_ids():
    """LLM conditions match nothing -> fall back, preserving base_doc_ids."""
    meta_data_filter = {"method": "auto"}
    metas = {"tag": {"python": ["doc1"]}}
    chat_mdl = MagicMock()
    mock_gen = _set_gen_filter({
        "conditions": [{"key": "tag", "op": "=", "value": "rust"}],
        "logic": "and",
    })

    result = await apply_meta_data_filter(
        meta_data_filter, metas, "rust", chat_mdl, base_doc_ids=["base1", "base2"]
    )
    assert result == ["base1", "base2"]


@pytest.mark.asyncio
async def test_auto_mode_llm_returns_empty_conditions_falls_back():
    """LLM returns empty conditions list -> fall back to unfiltered."""
    meta_data_filter = {"method": "auto"}
    metas = {"tag": {"python": ["doc1"]}}
    chat_mdl = MagicMock()
    mock_gen = _set_gen_filter({"conditions": [], "logic": "and"})

    result = await apply_meta_data_filter(meta_data_filter, metas, "test", chat_mdl)
    assert result == []


# ---------------------------------------------------------------------------
# Semi-auto mode – same fall-back behaviour
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_semi_auto_mode_conditions_match_nothing_falls_back():
    """Semi-auto LLM conditions match nothing -> fall back to unfiltered."""
    meta_data_filter = {"method": "semi_auto", "semi_auto": ["tag"]}
    metas = {"tag": {"python": ["doc1"]}}
    chat_mdl = MagicMock()
    mock_gen = _set_gen_filter({
        "conditions": [{"key": "tag", "op": "=", "value": "rust"}],
        "logic": "and",
    })

    result = await apply_meta_data_filter(meta_data_filter, metas, "rust", chat_mdl)
    assert result == []


@pytest.mark.asyncio
async def test_semi_auto_mode_no_selected_keys_in_metas_falls_back():
    """Semi-auto with selected keys not present in metas -> no LLM call, return base."""
    meta_data_filter = {"method": "semi_auto", "semi_auto": ["nonexistent_key"]}
    metas = {"tag": {"python": ["doc1"]}}
    chat_mdl = MagicMock()
    mock_gen = _reset_gen_filter()

    result = await apply_meta_data_filter(
        meta_data_filter, metas, "test", chat_mdl, base_doc_ids=["b1"]
    )
    assert result == ["b1"]
    mock_gen.assert_not_called()


# ---------------------------------------------------------------------------
# Manual mode
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_manual_mode_matching_conditions_return_docs():
    """Manual filters that match -> return matched doc_ids."""
    meta_data_filter = {
        "method": "manual",
        "manual": [{"key": "tag", "op": "=", "value": "python"}],
        "logic": "and",
    }
    metas = {"tag": {"python": ["doc1", "doc2"], "java": ["doc3"]}}

    result = await apply_meta_data_filter(meta_data_filter, metas)
    assert set(result) == {"doc1", "doc2"}


@pytest.mark.asyncio
async def test_manual_mode_no_match_returns_sentinel():
    """Manual filters that match nothing -> return ["-999"] sentinel."""
    meta_data_filter = {
        "method": "manual",
        "manual": [{"key": "tag", "op": "=", "value": "rust"}],
        "logic": "and",
    }
    metas = {"tag": {"python": ["doc1"]}}

    result = await apply_meta_data_filter(meta_data_filter, metas)
    assert result == ["-999"]


@pytest.mark.asyncio
async def test_manual_mode_no_filters_returns_empty():
    """Manual mode with empty filters list -> return [] (no sentinel)."""
    meta_data_filter = {"method": "manual", "manual": [], "logic": "and"}
    metas = {"tag": {"python": ["doc1"]}}

    result = await apply_meta_data_filter(meta_data_filter, metas)
    assert result == []


# ---------------------------------------------------------------------------
# metas_loader lazy evaluation
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_metas_loader_not_called_when_metas_provided():
    """When metas is provided directly, metas_loader is never invoked."""
    meta_data_filter = {"method": "auto"}
    metas = {"tag": {"python": ["doc1"]}}
    chat_mdl = MagicMock()
    loader_called = False

    def loader():
        nonlocal loader_called
        loader_called = True
        return {}

    _set_gen_filter({"conditions": [], "logic": "and"})
    await apply_meta_data_filter(meta_data_filter, metas, "test", chat_mdl, metas_loader=loader)
    assert not loader_called


@pytest.mark.asyncio
async def test_metas_loader_called_when_metas_is_none():
    """When metas is None, metas_loader is invoked lazily."""
    meta_data_filter = {"method": "auto"}
    chat_mdl = MagicMock()
    loader_call_count = 0

    def loader():
        nonlocal loader_call_count
        loader_call_count += 1
        return {"tag": {"python": ["doc1"]}}

    _set_gen_filter({
        "conditions": [{"key": "tag", "op": "=", "value": "python"}],
        "logic": "and",
    })
    result = await apply_meta_data_filter(
        meta_data_filter, metas=None, question="test", chat_mdl=chat_mdl, metas_loader=loader
    )
    assert loader_call_count == 1
    assert "doc1" in result


@pytest.mark.asyncio
async def test_metas_loader_returns_empty_skips_llm():
    """metas_loader returns {} -> skip LLM, return doc_ids (the #13987 fix via loader)."""
    meta_data_filter = {"method": "auto"}
    chat_mdl = MagicMock()
    mock_gen = _reset_gen_filter()

    result = await apply_meta_data_filter(
        meta_data_filter,
        metas=None,
        question="test",
        chat_mdl=chat_mdl,
        base_doc_ids=["b1"],
        metas_loader=lambda: {},
    )
    assert result == ["b1"]
    mock_gen.assert_not_called()


# ---------------------------------------------------------------------------
# Regression: ensure None is never returned when meta_data_filter is truthy
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_auto_mode_never_returns_none():
    """Regression: auto mode must never return None (would skip vector search)."""
    meta_data_filter = {"method": "auto"}
    metas = {"tag": {"python": ["doc1"]}}
    chat_mdl = MagicMock()

    # LLM returns conditions that match nothing
    _set_gen_filter({
        "conditions": [{"key": "tag", "op": "=", "value": "nonexistent"}],
        "logic": "and",
    })
    result = await apply_meta_data_filter(meta_data_filter, metas, "test", chat_mdl)
    assert result is not None, "auto mode must never return None (regression for #13987)"

    # LLM returns empty conditions
    _set_gen_filter({"conditions": [], "logic": "and"})
    result = await apply_meta_data_filter(meta_data_filter, metas, "test", chat_mdl)
    assert result is not None, "auto mode must never return None (regression for #13987)"
