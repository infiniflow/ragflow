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
import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace


def _load_module(monkeypatch):
    class OrderByExpr:
        pass

    class MatchTextExpr:
        def __init__(self, fields, matching_text, topn, extra_options=None):
            self.fields = fields
            self.matching_text = matching_text
            self.topn = topn
            self.extra_options = extra_options or {}

    class MatchDenseExpr:
        def __init__(self, *_args, **kwargs):
            self.topn = kwargs.get("topn")

    async def thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    common = ModuleType("common")
    common.settings = SimpleNamespace()
    doc_store_base = ModuleType("common.doc_store.doc_store_base")
    doc_store_base.OrderByExpr = OrderByExpr
    doc_store_base.MatchTextExpr = MatchTextExpr
    doc_store_base.MatchDenseExpr = MatchDenseExpr
    misc_utils = ModuleType("common.misc_utils")
    misc_utils.thread_pool_exec = thread_pool_exec
    structure = ModuleType("rag.advanced_rag.knowlege_compile.structure")
    structure._struct_graph_entity = lambda payload, _chunk_ids: dict(payload)
    structure._struct_graph_relation = lambda payload: dict(payload) if payload.get("from") and payload.get("to") else None
    compile_common = ModuleType("rag.advanced_rag.knowlege_compile._common")
    compile_common.tokenize_for_search = lambda text: (f"coarse:{text}", f"fine:{text}")

    monkeypatch.setitem(sys.modules, "common", common)
    monkeypatch.setitem(sys.modules, "common.doc_store", ModuleType("common.doc_store"))
    monkeypatch.setitem(sys.modules, "common.doc_store.doc_store_base", doc_store_base)
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils)
    monkeypatch.setitem(sys.modules, "rag", ModuleType("rag"))
    monkeypatch.setitem(sys.modules, "rag.advanced_rag", ModuleType("rag.advanced_rag"))
    monkeypatch.setitem(sys.modules, "rag.advanced_rag.knowlege_compile", ModuleType("rag.advanced_rag.knowlege_compile"))
    monkeypatch.setitem(sys.modules, "rag.advanced_rag.knowlege_compile._common", compile_common)
    monkeypatch.setitem(sys.modules, "rag.advanced_rag.knowlege_compile.structure", structure)

    path = Path(__file__).parents[5] / "api/apps/services/structure_graph_common.py"
    spec = importlib.util.spec_from_file_location("structure_graph_common_under_test", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _entity(name, entity_type="tree_node"):
    return {
        "knowledge_graph_kwd": "entity",
        "content_with_weight": json.dumps({"name": name, "type": entity_type}),
        "name_kwd": name.lower(),
    }


def _relation(parent, child):
    return {
        "knowledge_graph_kwd": "relation",
        "content_with_weight": json.dumps({"from": parent, "to": child, "type": "parent_child"}),
        "from_entity_kwd": parent,
        "to_entity_kwd": child,
    }


def test_tree_keyword_subgraph_returns_only_match_and_ancestor_path(monkeypatch):
    module = _load_module(monkeypatch)
    leaf = _entity("Leaf")
    fallback_expressions = []
    bucket_rows = [
        _entity("Root", "title"),
        _entity("Parent"),
        leaf,
        _entity("Child"),
        _relation("ROOT", "Parent"),
        _relation("Parent", "leaf"),
        _relation("Leaf", "Child"),
    ]

    async def graph_search(_index, _kb_id, _fields, condition, _order, _limit, match_expressions=None, offset=0):
        if match_expressions:
            fallback_expressions.extend(match_expressions)
            return {"leaf": leaf}, 1
        kind = condition["knowledge_graph_kwd"]
        if kind == ["relation"]:
            child = condition["to_entity_kwd"][0]
            rows = [row for row in bucket_rows if row["knowledge_graph_kwd"] == "relation" and row["to_entity_kwd"] == child]
        else:
            assert kind == ["entity"]
            name = condition["name_kwd"][0].lower()
            rows = [row for row in bucket_rows if row["knowledge_graph_kwd"] == "entity" and row["name_kwd"] == name]
        return {str(i): row for i, row in enumerate(rows)}, len(rows)

    module.graph_search = graph_search

    def scope_for_template(_row):
        return {"template_id": "tree-1", "kind": "tree"}, {"doc_id": ["doc-1"], "compilation_template_ids": ["tree-1"]}

    bucket, entities, relations = asyncio.run(
        module.keyword_subgraph(
            "index",
            "kb-1",
            SimpleNamespace(),
            {"doc_id": ["doc-1"], "knowledge_graph_kwd": ["entity"]},
            "Leaf",
            scope_for_template,
        )
    )

    assert bucket["template_id"] == "tree-1"
    assert {entity["name"] for entity in entities} == {"Root", "Parent", "Leaf"}
    assert {(relation["from"], relation["to"]) for relation in relations} == {("Root", "Parent"), ("Parent", "Leaf")}
    assert fallback_expressions == []


def test_keyword_subgraph_tokenizes_query_before_bm25(monkeypatch):
    module = _load_module(monkeypatch)
    matched = _entity("桃园结义初平黄巾")
    text_expressions = []

    async def graph_search(_index, _kb_id, _fields, condition, _order, _limit, match_expressions=None, offset=0):
        if match_expressions:
            text_expressions.extend(match_expressions)
            return {"matched": matched}, 1
        if condition["knowledge_graph_kwd"] == ["entity"] and "name_kwd" in condition:
            return {}, 0
        return {}, 0

    module.graph_search = graph_search

    bucket, entities, relations = asyncio.run(
        module.keyword_subgraph(
            "index",
            "kb-1",
            SimpleNamespace(),
            {"doc_id": ["doc-1"], "knowledge_graph_kwd": ["entity"]},
            "桃园结义",
            lambda _row: ({"template_id": "tree-1", "kind": "tree"}, {"doc_id": ["doc-1"], "compilation_template_ids": ["tree-1"]}),
        )
    )

    assert bucket["template_id"] == "tree-1"
    assert [entity["name"] for entity in entities] == ["桃园结义初平黄巾"]
    assert relations == []
    assert len(text_expressions) == 1
    assert text_expressions[0].matching_text == "coarse:桃园结义 fine:桃园结义"
    assert text_expressions[0].extra_options["original_query"] == text_expressions[0].matching_text


def test_keyword_subgraph_limits_knn_fallback_to_one_candidate(monkeypatch):
    module = _load_module(monkeypatch)
    matched = _entity("Semantic leaf")
    root = _entity("Root", "title")
    parent = _entity("Parent")
    relation_rows = [_relation("Root", "Parent"), _relation("Parent", "Semantic leaf")]
    dense_expressions = []
    search_limits = []

    async def graph_search(_index, _kb_id, _fields, condition, _order, limit, match_expressions=None, offset=0):
        if match_expressions and isinstance(match_expressions[0], sys.modules["common.doc_store.doc_store_base"].MatchDenseExpr):
            dense_expressions.extend(match_expressions)
            search_limits.append(limit)
            return {"matched": matched}, 1
        if condition["knowledge_graph_kwd"] == ["relation"]:
            child = condition["to_entity_kwd"][0]
            rows = [row for row in relation_rows if row["to_entity_kwd"] == child]
            return {str(i): row for i, row in enumerate(rows)}, len(rows)
        if condition["knowledge_graph_kwd"] == ["entity"] and "name_kwd" in condition:
            name = condition["name_kwd"][0]
            rows = [entity for entity in (root, parent) if entity["name_kwd"] == name]
            return {str(i): row for i, row in enumerate(rows)}, len(rows)
        return {}, 0

    module.graph_search = graph_search
    embedding = SimpleNamespace(encode_queries=lambda _query: ([0.1, 0.2], 2))

    bucket, entities, relations = asyncio.run(
        module.keyword_subgraph(
            "index",
            "kb-1",
            embedding,
            {"doc_id": ["doc-1"], "knowledge_graph_kwd": ["entity"]},
            "unmatched",
            lambda _row: ({"template_id": "tree-1", "kind": "tree"}, {"doc_id": ["doc-1"], "compilation_template_ids": ["tree-1"]}),
        )
    )

    assert bucket["template_id"] == "tree-1"
    assert {entity["name"] for entity in entities} == {"Root", "Parent", "Semantic leaf"}
    assert {(relation["from"], relation["to"]) for relation in relations} == {("Root", "Parent"), ("Parent", "Semantic leaf")}
    assert dense_expressions[0].topn == 1
    assert search_limits == [1]
