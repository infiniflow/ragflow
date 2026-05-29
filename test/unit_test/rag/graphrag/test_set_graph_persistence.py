#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#

from unittest.mock import AsyncMock, MagicMock, patch

import networkx as nx
import pytest

from common import settings
from rag.graphrag.utils import GraphChange, set_graph


@pytest.mark.p2
@pytest.mark.asyncio
async def test_set_graph_inserts_before_pruning_stale_graph_rows():
    graph = nx.Graph()
    graph.graph["source_id"] = ["doc-1"]
    graph.add_node("A", description="node", source_id=["doc-1"], entity_type="ORG")

    calls: list[str] = []

    async def fake_insert_chunks_bounded(chunks, tenant_id, kb_id, *, callback=None, label="Insert chunks"):
        calls.append("insert")

    async def fake_enumerate(_tenant_id, _kb_id):
        return ["old-graph", "old-subgraph"]

    async def fake_thread_pool_exec(func, *args, **_kwargs):
        if func is settings.docStoreConn.delete:
            calls.append(f"delete:{args[0]}")
        return None

    with (
        patch("rag.graphrag.utils.insert_chunks_bounded", fake_insert_chunks_bounded),
        patch("rag.graphrag.utils._enumerate_graph_subgraph_chunk_ids", fake_enumerate),
        patch("rag.graphrag.utils.thread_pool_exec", fake_thread_pool_exec),
        patch("rag.graphrag.utils.graph_node_to_chunk", AsyncMock()),
        patch("rag.graphrag.utils.graph_edge_to_chunk", AsyncMock()),
    ):
        await set_graph("tenant-1", "kb-1", MagicMock(), graph, GraphChange(), callback=None)

    assert calls[0] == "insert"
    assert any(call.startswith("delete:") for call in calls[1:])
    stale_delete = next(call for call in calls if call.startswith("delete:"))
    assert "'id'" in stale_delete
    assert "old-graph" in stale_delete


@pytest.mark.p2
@pytest.mark.asyncio
async def test_set_graph_skips_broad_delete_when_enumeration_succeeds():
    graph = nx.Graph()
    graph.graph["source_id"] = ["doc-1"]

    delete_conditions: list[dict] = []

    async def fake_thread_pool_exec(func, *args, **_kwargs):
        if func is settings.docStoreConn.delete:
            delete_conditions.append(args[0])
        return None

    with (
        patch("rag.graphrag.utils.insert_chunks_bounded", AsyncMock()),
        patch("rag.graphrag.utils._enumerate_graph_subgraph_chunk_ids", AsyncMock(return_value=["old-1"])),
        patch("rag.graphrag.utils.thread_pool_exec", fake_thread_pool_exec),
        patch("rag.graphrag.utils.graph_node_to_chunk", AsyncMock()),
        patch("rag.graphrag.utils.graph_edge_to_chunk", AsyncMock()),
    ):
        await set_graph("tenant-1", "kb-1", MagicMock(), graph, GraphChange(), callback=None)

    assert not any(
        cond.get("knowledge_graph_kwd") == ["graph", "subgraph"] and "id" not in cond
        for cond in delete_conditions
    )


@pytest.mark.p2
@pytest.mark.asyncio
async def test_set_graph_prunes_stale_graph_rows_in_batches():
    graph = nx.Graph()
    graph.graph["source_id"] = ["doc-1"]

    delete_batches: list[list[str]] = []

    async def fake_thread_pool_exec(func, *args, **_kwargs):
        if func is settings.docStoreConn.delete and isinstance(args[0], dict) and "id" in args[0]:
            delete_batches.append(list(args[0]["id"]))
        return None

    stale_ids = [f"old-{i}" for i in range(250)]

    with (
        patch("rag.graphrag.utils.insert_chunks_bounded", AsyncMock()),
        patch("rag.graphrag.utils._enumerate_graph_subgraph_chunk_ids", AsyncMock(return_value=stale_ids)),
        patch("rag.graphrag.utils.thread_pool_exec", fake_thread_pool_exec),
        patch("rag.graphrag.utils.graph_node_to_chunk", AsyncMock()),
        patch("rag.graphrag.utils.graph_edge_to_chunk", AsyncMock()),
    ):
        await set_graph("tenant-1", "kb-1", MagicMock(), graph, GraphChange(), callback=None)

    assert len(delete_batches) == 3
    assert len(delete_batches[0]) == 100
    assert len(delete_batches[1]) == 100
    assert len(delete_batches[2]) == 50
