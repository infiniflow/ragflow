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

import asyncio
import importlib
import json
import sys
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import networkx as nx
import pytest
from networkx.readwrite import json_graph

import rag.graphrag.utils as graphrag_utils
from rag.graphrag.utils import SOURCE_SUBGRAPH_VERSION, SOURCE_SUBGRAPH_VERSION_KEY, GraphChange


class FakeDocStore:
    def __init__(self, batches=None):
        self._batches = list(batches or [])
        self.deleted = []
        self.inserted = []

    def search(self, *_args, **_kwargs):
        return object()

    def get_fields(self, _res, _flds):
        return self._batches.pop(0) if self._batches else {}

    def delete(self, condition, *_args):
        self.deleted.append(condition)

    def insert(self, chunks, *_args):
        self.inserted.extend(chunks)


def _subgraph_chunk(doc_id, *, edge_weight, node_description, edge_description, stale_rank=None, source_version=SOURCE_SUBGRAPH_VERSION):
    graph = nx.Graph()
    node_attrs = {"description": node_description, "source_id": [doc_id], "entity_type": "X"}
    if stale_rank is not None:
        node_attrs["rank"] = stale_rank
    graph.add_node("A", **node_attrs)
    graph.add_node("B", description=f"B-{doc_id}", source_id=[doc_id], entity_type="Y", **({"rank": stale_rank} if stale_rank is not None else {}))
    graph.add_edge("A", "B", weight=edge_weight, description=edge_description, source_id=[doc_id], keywords=[f"k-{doc_id}"])
    graph.graph["source_id"] = [doc_id]
    if source_version is not None:
        graph.graph[SOURCE_SUBGRAPH_VERSION_KEY] = source_version
    return {
        "knowledge_graph_kwd": "subgraph",
        "content_with_weight": json.dumps(json_graph.node_link_data(graph, edges="edges"), ensure_ascii=False),
        "source_id": [doc_id],
    }


def _run_rebuild(monkeypatch, chunks, exclude=None, *, source_version=SOURCE_SUBGRAPH_VERSION, aliases=None):
    store = FakeDocStore([chunks, {}])
    settings = MagicMock()
    settings.docStoreConn = store
    monkeypatch.setattr(graphrag_utils, "settings", settings)
    return asyncio.run(
        graphrag_utils.rebuild_graph(
            "tenant",
            "kb",
            exclude,
            source_subgraph_version=source_version,
            entity_resolution_aliases=aliases,
        )
    )


def test_rebuild_accumulates_document_contributions(monkeypatch):
    graph = _run_rebuild(
        monkeypatch,
        {
            "doc1": _subgraph_chunk("doc1", edge_weight=1, node_description="node-1", edge_description="edge-1"),
            "doc2": _subgraph_chunk("doc2", edge_weight=2, node_description="node-2", edge_description="edge-2"),
        },
    )

    assert graph.edges["A", "B"]["weight"] == 3
    assert graph.edges["A", "B"]["source_id"] == ["doc1", "doc2"]
    assert graph.edges["A", "B"]["keywords"] == ["k-doc1", "k-doc2"]
    assert graph.nodes["A"]["description"] == f"node-1{graphrag_utils.GRAPH_FIELD_SEP}node-2"


def test_rebuild_excludes_all_document_contributions(monkeypatch):
    graph = _run_rebuild(
        monkeypatch,
        {
            "doc1": _subgraph_chunk("doc1", edge_weight=1, node_description="node-1", edge_description="edge-1"),
            "doc2": _subgraph_chunk("doc2", edge_weight=2, node_description="node-2", edge_description="edge-2"),
        },
        exclude=["doc2"],
    )

    assert graph.graph["source_id"] == ["doc1"]
    assert graph.edges["A", "B"]["weight"] == 1
    assert graph.edges["A", "B"]["description"] == "edge-1"
    assert graph.edges["A", "B"]["source_id"] == ["doc1"]
    assert graph.nodes["A"]["description"] == "node-1"


def test_snapshot_rebuild_does_not_reaccumulate_aggregated_attributes(monkeypatch):
    graph = _run_rebuild(
        monkeypatch,
        {
            "doc1": _subgraph_chunk("doc1", edge_weight=3, node_description="node-1<SEP>node-2", edge_description="edge-1<SEP>edge-2", source_version=None),
            "doc2": _subgraph_chunk("doc2", edge_weight=3, node_description="node-1<SEP>node-2", edge_description="edge-1<SEP>edge-2", source_version=None),
        },
        source_version=None,
    )

    assert graph.edges["A", "B"]["weight"] == 3
    assert graph.edges["A", "B"]["description"] == "edge-1<SEP>edge-2"


def test_rebuild_recomputes_rank(monkeypatch):
    graph = _run_rebuild(
        monkeypatch,
        {"doc1": _subgraph_chunk("doc1", edge_weight=1, node_description="node-1", edge_description="edge-1", stale_rank=7)},
    )

    assert graph.nodes["A"]["rank"] == 1
    assert graph.nodes["B"]["rank"] == 1


def test_rebuild_applies_alias_only_if_canonical_node_survives(monkeypatch):
    canonical = _subgraph_chunk("doc1", edge_weight=1, node_description="canonical", edge_description="edge-1")
    alias = _subgraph_chunk("doc2", edge_weight=2, node_description="alias", edge_description="edge-2")
    alias_graph = json_graph.node_link_graph(json.loads(alias["content_with_weight"]), edges="edges")
    nx.relabel_nodes(alias_graph, {"A": "ALIAS"}, copy=False)
    alias["content_with_weight"] = json.dumps(json_graph.node_link_data(alias_graph, edges="edges"), ensure_ascii=False)
    aliases = {"ALIAS": "A"}

    graph = _run_rebuild(monkeypatch, {"doc1": canonical, "doc2": alias}, aliases=aliases)
    assert "ALIAS" not in graph
    assert graph.nodes["A"]["source_id"] == ["doc1", "doc2"]

    graph_without_canonical = _run_rebuild(monkeypatch, {"doc2": alias}, aliases=aliases)
    assert "ALIAS" in graph_without_canonical
    assert "A" not in graph_without_canonical


def test_get_graph_uses_persisted_source_subgraph_version(monkeypatch):
    chunks = {
        "doc1": _subgraph_chunk("doc1", edge_weight=1, node_description="node-1", edge_description="edge-1"),
        "doc2": _subgraph_chunk("doc2", edge_weight=2, node_description="node-2", edge_description="edge-2"),
    }
    stored_graph = nx.Graph()
    stored_graph.graph["source_id"] = ["doc1", "doc2"]
    stored_graph.graph[SOURCE_SUBGRAPH_VERSION_KEY] = SOURCE_SUBGRAPH_VERSION
    response = SimpleNamespace(
        total=1,
        ids=["graph"],
        field={
            "graph": {
                "content_with_weight": json.dumps(json_graph.node_link_data(stored_graph, edges="edges")),
                "removed_kwd": "Y",
                "source_id": ["doc1", "doc2"],
            }
        },
    )
    settings = MagicMock()
    settings.retriever.search = AsyncMock(return_value=response)
    settings.docStoreConn = FakeDocStore([chunks, {}])
    monkeypatch.setattr(graphrag_utils, "settings", settings)
    monkeypatch.setattr(graphrag_utils.search, "index_name", lambda _tenant: "index")

    graph = asyncio.run(graphrag_utils.get_graph("tenant", "kb"))

    assert graph.edges["A", "B"]["weight"] == 3
    assert graph.graph["_rebuild_index"] is True
    assert graph.graph["_rebuild_graph_id"] == "graph"


def test_merge_after_document_deletion_replaces_stale_relation_index(monkeypatch):
    class IndexedDocStore(FakeDocStore):
        def __init__(self):
            super().__init__([{"doc2": _subgraph_chunk("doc2", edge_weight=2, node_description="node-2", edge_description="edge-2")}, {}])
            # The deletion path removed doc1 from source_id, but left its
            # contribution in the indexed relation's weight and description.
            self.relations = [
                {
                    "id": "old-relation",
                    "knowledge_graph_kwd": "relation",
                    "from_entity_kwd": "A",
                    "to_entity_kwd": "B",
                    "source_id": ["doc2"],
                    "removed_kwd": "N",
                    "weight_int": 3,
                    "content_with_weight": "edge-1<SEP>edge-2",
                }
            ]
            self.entities = [{"id": "old-entity", "knowledge_graph_kwd": "entity", "entity_kwd": "A", "source_id": ["doc2"], "removed_kwd": "N", "content_with_weight": "node-1<SEP>node-2"}]
            self.graph_update = None

        def search(self, _fields, _highlight, condition, _match, _order, offset, limit, *_args):
            if condition.get("removed_kwd") == "Y":
                return [row for row in self.entities + self.relations if row.get("removed_kwd") == "Y"][offset : offset + limit]
            return object()

        def get_fields(self, result, fields):
            if isinstance(result, list):
                return {row["id"]: {field: row.get(field) for field in fields} for row in result}
            return super().get_fields(result, fields)

        def get_total(self, _result):
            return len(self.entities) + len(self.relations)

        def update(self, condition, values, *_args):
            if condition.get("id") == "graph":
                self.graph_update = values.copy()
            else:
                for row in self.entities + self.relations:
                    row.update(values)
            return True

        def delete(self, condition, *_args):
            super().delete(condition)
            if "id" in condition:
                ids = set(condition["id"])
                self.entities = [row for row in self.entities if row["id"] not in ids]
                self.relations = [row for row in self.relations if row["id"] not in ids]
                return len(ids)
            if "entity" in condition.get("knowledge_graph_kwd", []):
                self.entities.clear()
            if "relation" not in condition.get("knowledge_graph_kwd", []):
                return
            if "from_entity_kwd" not in condition:
                self.relations.clear()
            else:
                self.relations = [row for row in self.relations if (row["from_entity_kwd"], row["to_entity_kwd"]) != (condition["from_entity_kwd"], condition["to_entity_kwd"])]

        def insert(self, chunks, *_args):
            super().insert(chunks)
            for chunk in chunks:
                rows = self.entities if chunk["knowledge_graph_kwd"] == "entity" else self.relations
                rows[:] = [row for row in rows if row["id"] != chunk["id"]]
                rows.append(chunk.copy())

    store = IndexedDocStore()
    old_graph = nx.Graph()
    old_graph.add_node("A", description="node-1<SEP>node-2", source_id=["doc1", "doc2"], entity_type="X")
    old_graph.add_node("B", description="peer-1<SEP>peer-2", source_id=["doc1", "doc2"], entity_type="Y")
    old_graph.add_edge("A", "B", description="edge-1<SEP>edge-2", keywords=["k-doc1", "k-doc2"], source_id=["doc1", "doc2"], weight=3)
    old_graph.graph.update(source_id=["doc1", "doc2"], source_subgraph_version=SOURCE_SUBGRAPH_VERSION)
    response = SimpleNamespace(
        total=1,
        ids=["graph"],
        field={
            "graph": {
                "content_with_weight": json.dumps(json_graph.node_link_data(old_graph, edges="edges")),
                "removed_kwd": "Y",
                "source_id": ["doc2"],
            }
        },
    )
    settings = MagicMock()
    settings.retriever.search = AsyncMock(return_value=response)
    settings.docStoreConn = store
    monkeypatch.setattr(graphrag_utils, "settings", settings)
    monkeypatch.setattr(graphrag_utils.search, "index_name", lambda _tenant: "index")

    async def run_sync(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    async def add_node(_kb, _model, name, attrs, chunks, _neighbors):
        chunks.append({"knowledge_graph_kwd": "entity", "entity_kwd": name, "source_id": attrs["source_id"], "content_with_weight": attrs["description"]})

    async def add_edge(_kb, _model, source, target, attrs, chunks):
        chunks.append(
            {
                "knowledge_graph_kwd": "relation",
                "from_entity_kwd": source,
                "to_entity_kwd": target,
                "source_id": attrs["source_id"],
                "weight_int": attrs["weight"],
                "content_with_weight": attrs["description"],
            }
        )

    async def insert_chunks(chunks, *_args, **_kwargs):
        store.insert(chunks)

    monkeypatch.setattr(graphrag_utils, "thread_pool_exec", run_sync)
    monkeypatch.setattr(graphrag_utils, "_batch_embed_cache_misses", lambda _name, names: [False] * len(names))
    monkeypatch.setattr(graphrag_utils, "_write_embed_cache_batch", lambda *_args: None)
    monkeypatch.setattr(graphrag_utils, "n_neighbor", lambda *_args: [])
    monkeypatch.setattr(graphrag_utils, "graph_node_to_chunk", add_node)
    monkeypatch.setattr(graphrag_utils, "graph_edge_to_chunk", add_edge)
    monkeypatch.setattr(graphrag_utils, "insert_chunks_bounded", insert_chunks)

    new_doc = nx.Graph()
    new_doc.add_node("C", description="node-3", source_id=["doc3"], entity_type="X")
    new_doc.add_node("D", description="peer-3", source_id=["doc3"], entity_type="Y")
    new_doc.add_edge("C", "D", description="edge-3", keywords=["k-doc3"], source_id=["doc3"], weight=1)
    new_doc.graph["source_id"] = ["doc3"]
    embedding = MagicMock(llm_name="test-embedding")
    embedding.encode.side_effect = lambda texts: ([[0.1, 0.2] for _ in texts], 0)

    async def merge_after_deletion():
        rebuilt = await graphrag_utils.get_graph("tenant", "kb", ["doc3"])
        change = GraphChange()
        merged = graphrag_utils.graph_merge(rebuilt, new_doc, change)
        pageranks = nx.pagerank(merged)
        for name, pagerank in pageranks.items():
            merged.nodes[name]["pagerank"] = pagerank
        await graphrag_utils.set_graph("tenant", "kb", embedding, merged, change, None)
        return merged

    graph = asyncio.run(merge_after_deletion())
    assert graph.edges["A", "B"]["weight"] == 2
    surviving_entity = [ent for ent in store.entities if ent["entity_kwd"] == "A"]
    assert len(surviving_entity) == 1
    assert surviving_entity[0]["content_with_weight"] == "node-2"
    remaining = [r for r in store.relations if (r["from_entity_kwd"], r["to_entity_kwd"]) == ("A", "B")]
    assert len(remaining) == 1
    assert remaining[0]["weight_int"] == 2
    assert remaining[0]["content_with_weight"] == "edge-2"
    assert any("A->B: edge-2" in call.args[0] for call in embedding.encode.call_args_list)
    assert store.graph_update is not None
    for key in ("_rebuild_index", "_rebuild_graph_id", "_rebuild_old_edges"):
        assert key not in json.loads(store.graph_update["content_with_weight"])["graph"]
        assert key not in graph.graph


def test_batch_merge_does_not_skip_a_subgraph_only_present_in_rebuild(monkeypatch):
    # The unit-test conftest stubs infrastructure dependencies. Import the
    # batch entrypoint with its unused DB/NER dependencies stubbed as well.
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", MagicMock())
    monkeypatch.setitem(sys.modules, "rag.graphrag.ner", MagicMock())
    monkeypatch.setitem(sys.modules, "rag.graphrag.ner.graph_extractor", MagicMock())
    index = importlib.import_module("rag.graphrag.general.index")

    class Lock:
        def __init__(self, *_args, **_kwargs):
            pass

        def acquire(self):
            return True

        def release(self):
            pass

    subgraph = nx.Graph()
    subgraph.add_node("C", description="new document", source_id=["doc3"])
    subgraph.graph["source_id"] = ["doc3"]
    rebuilt = subgraph.copy()
    rebuilt.graph["source_id"] = ["doc2", "doc3"]
    rebuilt.graph["_rebuild_index"] = True

    merge = AsyncMock(return_value=rebuilt)
    monkeypatch.setattr(index, "RedisDistributedLock", Lock)
    monkeypatch.setattr(index, "load_subgraph_from_store", AsyncMock(return_value=subgraph))
    monkeypatch.setattr(index, "get_graph", AsyncMock(return_value=rebuilt))
    monkeypatch.setattr(index, "does_graph_contains", AsyncMock(return_value=False))
    monkeypatch.setattr(index, "merge_subgraph", merge)
    monkeypatch.setattr(index, "clear_phase_markers", lambda _kb: None)

    def run_batch():
        return asyncio.run(
            index.run_graphrag_for_kb(
                {"tenant_id": "tenant", "kb_id": "kb", "id": "task"},
                ["doc3"],
                "English",
                {},
                MagicMock(),
                MagicMock(),
                MagicMock(),
                with_resolution=False,
                with_community=False,
            )
        )

    result = run_batch()
    assert result["ok_docs"] == ["doc3"]
    merge.assert_awaited_once()

    # A genuinely persisted, non-removed graph should still skip a retry.
    index.does_graph_contains.return_value = True
    index.get_graph.return_value = subgraph
    merge.reset_mock()
    assert run_batch()["ok_docs"] == ["doc3"]
    merge.assert_not_awaited()


def test_set_graph_preserves_source_subgraphs(monkeypatch):
    store = FakeDocStore()
    settings = MagicMock()
    settings.docStoreConn = store
    monkeypatch.setattr(graphrag_utils, "settings", settings)
    monkeypatch.setattr(graphrag_utils.search, "index_name", lambda _tenant: "index")

    graph = nx.Graph()
    graph.graph["source_id"] = ["doc1"]
    graph.graph[SOURCE_SUBGRAPH_VERSION_KEY] = SOURCE_SUBGRAPH_VERSION
    asyncio.run(graphrag_utils.set_graph("tenant", "kb", MagicMock(), graph, GraphChange(), None))

    assert store.deleted[0] == {"knowledge_graph_kwd": ["graph"]}
    assert [chunk["knowledge_graph_kwd"] for chunk in store.inserted] == ["graph"]


def test_set_graph_rewrites_snapshot_subgraphs(monkeypatch):
    store = FakeDocStore()
    settings = MagicMock()
    settings.docStoreConn = store
    monkeypatch.setattr(graphrag_utils, "settings", settings)
    monkeypatch.setattr(graphrag_utils.search, "index_name", lambda _tenant: "index")

    graph = nx.Graph()
    graph.graph["source_id"] = ["doc1"]
    asyncio.run(graphrag_utils.set_graph("tenant", "kb", MagicMock(), graph, GraphChange(), None))

    assert store.deleted[0] == {"knowledge_graph_kwd": ["graph", "subgraph"]}
    assert [chunk["knowledge_graph_kwd"] for chunk in store.inserted] == ["graph", "subgraph"]


@pytest.mark.parametrize("fail_step", ["stage", "prune", "commit", "mark_partial", "missing_row"])
def test_rebuild_index_partial_write_preserves_checkpoint_and_retries(monkeypatch, fail_step):
    class Store:
        def __init__(self):
            self.graph = {"id": "graph-old", "knowledge_graph_kwd": "graph", "removed_kwd": "Y"}
            self.injected = False
            self.rows = [
                {"id": "entity-old", "knowledge_graph_kwd": "entity", "entity_kwd": "A", "removed_kwd": "N", "content_with_weight": "node-1<SEP>node-2"},
                {"id": "relation-old", "knowledge_graph_kwd": "relation", "from_entity_kwd": "A", "to_entity_kwd": "B", "removed_kwd": "N", "weight_int": 3},
            ]

        def search(self, _fields, _highlight, condition, _match, _order, offset, limit, *_args):
            self.last_condition = condition
            kinds = condition.get("knowledge_graph_kwd", [])
            return [row for row in self.rows if row["knowledge_graph_kwd"] in kinds and ("removed_kwd" not in condition or row.get("removed_kwd") == condition["removed_kwd"])][offset : offset + limit]

        def get_fields(self, rows, fields):
            return {row["id"]: {field: row.get(field) for field in fields} for row in rows}

        def get_total(self, _rows):
            assert self.last_condition.get("removed_kwd") == "N"
            return sum(row.get("removed_kwd") == "N" for row in self.rows)

        def update(self, condition, values, *_args):
            if condition.get("id") == "graph-old":
                if fail_step == "commit" and not self.injected:
                    self.injected = True
                    raise RuntimeError("injected failure during graph commit")
                self.graph.update(values)
            else:
                if fail_step == "mark_partial" and not self.injected:
                    self.injected = True
                    self.rows[0].update(values)
                    return True
                for row in self.rows:
                    if row["knowledge_graph_kwd"] in condition.get("knowledge_graph_kwd", []):
                        row.update(values)
            return True

        def delete(self, condition, *_args):
            if "id" in condition:
                if fail_step == "prune" and not self.injected:
                    self.injected = True
                    raise RuntimeError("injected failure during stale-row pruning")
                ids = set(condition["id"])
                self.rows = [row for row in self.rows if row["id"] not in ids]
                return len(ids)
            kinds = set(condition.get("knowledge_graph_kwd", []))
            if "graph" in kinds:
                self.graph = None
            self.rows = [row for row in self.rows if row["knowledge_graph_kwd"] not in kinds]
            return 0

        def insert(self, chunks, *_args):
            for chunk in chunks:
                if chunk["knowledge_graph_kwd"] == "graph":
                    self.graph = chunk.copy()
                    continue
                self.rows = [row for row in self.rows if row["id"] != chunk["id"]]
                self.rows.append(chunk.copy())
            return []

    store = Store()
    settings = MagicMock(docStoreConn=store)
    monkeypatch.setattr(graphrag_utils, "settings", settings)
    monkeypatch.setattr(graphrag_utils.search, "index_name", lambda _tenant: "index")

    async def run_sync(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    async def add_node(_kb, _model, name, attrs, chunks, _neighbors):
        chunks.append({"id": f"new-{name}", "knowledge_graph_kwd": "entity", "entity_kwd": name, "source_id": attrs["source_id"]})

    async def add_edge(_kb, _model, source, target, attrs, chunks):
        chunks.append(
            {"id": f"new-{source}-{target}", "knowledge_graph_kwd": "relation", "from_entity_kwd": source, "to_entity_kwd": target, "weight_int": attrs["weight"], "source_id": attrs["source_id"]}
        )

    attempts = 0

    async def insert_partially_once(chunks, *_args, **_kwargs):
        nonlocal attempts
        attempts += 1
        if fail_step == "stage" and attempts == 1:
            store.insert(chunks[:1])
            raise RuntimeError("injected failure after first index chunk")
        if fail_step == "missing_row" and attempts == 1:
            store.insert(chunks[:-1])
            return
        store.insert(chunks)

    monkeypatch.setattr(graphrag_utils, "thread_pool_exec", run_sync)
    monkeypatch.setattr(graphrag_utils, "_batch_embed_cache_misses", lambda _name, names: [False] * len(names))
    monkeypatch.setattr(graphrag_utils, "_write_embed_cache_batch", lambda *_args: None)
    monkeypatch.setattr(graphrag_utils, "n_neighbor", lambda *_args: [])
    monkeypatch.setattr(graphrag_utils, "graph_node_to_chunk", add_node)
    monkeypatch.setattr(graphrag_utils, "graph_edge_to_chunk", add_edge)
    monkeypatch.setattr(graphrag_utils, "insert_chunks_bounded", insert_partially_once)

    graph = nx.Graph()
    graph.add_node("A", description="node-2", entity_type="X", source_id=["doc2"])
    graph.add_node("B", description="peer-2", entity_type="Y", source_id=["doc2"])
    graph.add_edge("A", "B", description="edge-2", keywords=["k-doc2"], source_id=["doc2"], weight=2)
    graph.graph.update(source_id=["doc2"], source_subgraph_version=SOURCE_SUBGRAPH_VERSION, _rebuild_index=True, _rebuild_graph_id="graph-old")
    embedding = MagicMock(llm_name="test-embedding")
    embedding.encode.side_effect = lambda texts: ([[0.1, 0.2] for _ in texts], 0)

    with pytest.raises(RuntimeError, match="injected failure|index row count mismatch"):
        asyncio.run(graphrag_utils.set_graph("tenant", "kb", embedding, graph, GraphChange(), None))
    assert store.graph["removed_kwd"] == "Y"
    if fail_step in ("stage", "prune"):
        assert {row["id"] for row in store.rows} >= {"entity-old", "relation-old"}
    assert graph.graph["_rebuild_index"] is True

    asyncio.run(graphrag_utils.set_graph("tenant", "kb", embedding, graph, GraphChange(), None))
    assert store.graph["removed_kwd"] == "N"
    assert {row["id"] for row in store.rows}.isdisjoint({"entity-old", "relation-old"})
    assert len(store.rows) == 3
    assert next(row for row in store.rows if row["knowledge_graph_kwd"] == "relation")["weight_int"] == 2


def test_rebuild_reembeds_only_edges_with_changed_descriptions(monkeypatch):
    async def run_sync(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    async def add_node(*_args):
        pass

    async def add_edge(*_args):
        pass

    monkeypatch.setattr(graphrag_utils, "thread_pool_exec", run_sync)
    monkeypatch.setattr(graphrag_utils, "_batch_embed_cache_misses", lambda _name, keys: [False] * len(keys))
    monkeypatch.setattr(graphrag_utils, "_write_embed_cache_batch", lambda *_args: None)
    monkeypatch.setattr(graphrag_utils, "n_neighbor", lambda *_args: [])
    monkeypatch.setattr(graphrag_utils, "graph_node_to_chunk", add_node)
    monkeypatch.setattr(graphrag_utils, "graph_edge_to_chunk", add_edge)
    monkeypatch.setattr(graphrag_utils, "_commit_rebuilt_graph_index", AsyncMock())

    graph = nx.Graph()
    for name in ("A", "B", "C", "D"):
        graph.add_node(name, description=name, entity_type="X", source_id=["doc2"])
    graph.add_edge("A", "B", description="unchanged", weight=1, keywords=[], source_id=["doc2"])
    graph.add_edge("B", "C", description="updated", weight=2, keywords=[], source_id=["doc2"])
    graph.add_edge("C", "D", description="new", weight=1, keywords=[], source_id=["doc3"])
    graph.graph.update(
        source_id=["doc2", "doc3"],
        source_subgraph_version=SOURCE_SUBGRAPH_VERSION,
        _rebuild_index=True,
        _rebuild_graph_id="graph-old",
        _rebuild_old_edges={("A", "B"): "unchanged", ("B", "C"): "original"},
    )
    embedding = MagicMock(llm_name="test-embedding")
    embedding.encode.side_effect = lambda texts: ([[0.1, 0.2] for _ in texts], 0)

    asyncio.run(graphrag_utils.set_graph("tenant", "kb", embedding, graph, GraphChange(), None))
    encoded = [text for call in embedding.encode.call_args_list for text in call.args[0]]
    assert set(encoded) == {"B->C: updated", "C->D: new"}
    assert "_rebuild_old_edges" not in graph.graph
