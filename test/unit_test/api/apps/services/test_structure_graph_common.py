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

import pytest

import common.doc_store.doc_store_base as _real_doc_store_base


def _load_module(monkeypatch):
    class OrderByExpr:
        pass

    class MatchTextExpr:
        def __init__(self, fields, matching_text, topn, extra_options=None):
            self.fields = fields
            self.matching_text = matching_text
            self.topn = topn
            self.extra_options = extra_options or {}

    async def thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    common = ModuleType("common")
    common.settings = SimpleNamespace()
    doc_store_base = ModuleType("common.doc_store.doc_store_base")
    doc_store_base.__dict__.update({k: v for k, v in vars(_real_doc_store_base).items() if not k.startswith("__")})
    doc_store_base.OrderByExpr = OrderByExpr
    doc_store_base.MatchTextExpr = MatchTextExpr
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


@pytest.mark.parametrize(
    ("base_entity_condition", "kind"),
    [
        ({"doc_id": ["doc-1"], "knowledge_graph_kwd": ["entity"]}, "tree"),
        ({"compilation_template_ids": ["template-1"], "knowledge_graph_kwd": ["entity"], "scope_kwd": ["dataset", "doc"]}, "mind_map"),
    ],
)
def test_keyword_subgraph_returns_empty_without_name_match(monkeypatch, base_entity_condition, kind):
    module = _load_module(monkeypatch)
    unrelated = _entity("Unrelated leaf")
    search_expressions = []

    async def graph_search(_index, _kb_id, _fields, condition, _order, _limit, match_expressions=None, offset=0):
        if match_expressions:
            search_expressions.extend(match_expressions)
            return {"unrelated": unrelated}, 1
        assert condition["knowledge_graph_kwd"] == ["entity"]
        return {}, 0

    module.graph_search = graph_search

    bucket, entities, relations = asyncio.run(
        module.keyword_subgraph(
            "index",
            "kb-1",
            base_entity_condition,
            "unmatched",
            lambda _row: ({"template_id": "template-1", "kind": kind}, {"doc_id": ["doc-1"], "compilation_template_ids": ["template-1"]}),
        )
    )

    assert bucket is None
    assert entities == []
    assert relations == []
    assert len(search_expressions) == 1
    assert isinstance(search_expressions[0], sys.modules["common.doc_store.doc_store_base"].MatchTextExpr)


def _as_infinity(row):
    """Mirror ``InfinityConnection.get_fields``: ``*_kwd`` -> list, absent -> ``None``."""
    converted = {}
    for key, val in row.items():
        if key.endswith("_kwd") and isinstance(val, str):
            converted[key] = [part for part in val.split("###") if part]
        else:
            converted[key] = val
    return converted


def test_build_bucket_accepts_infinity_list_shaped_kwd_columns(monkeypatch):
    module = _load_module(monkeypatch)
    rows = [_as_infinity(_entity("Root", "title")), _as_infinity(_relation("root", "Leaf"))]

    async def graph_search(_index, _kb_id, _fields, _condition, _order, limit, match_expressions=None, offset=0):
        return {str(i): row for i, row in enumerate(rows[:limit])}, len(rows)

    module.graph_search = graph_search

    entities, relations = asyncio.run(module.build_bucket("index", "kb-1", {}))

    assert [entity["name"] for entity in entities] == ["Root"]
    # normalize_relation_endpoints() cases the endpoint to the entity name.
    assert [(relation["from"], relation["to"]) for relation in relations] == [("Root", "Leaf")]


def test_project_relation_reads_list_shaped_entity_columns(monkeypatch):
    module = _load_module(monkeypatch)
    row = {
        "content_with_weight": json.dumps({"type": "parent_child"}),
        "from_entity_kwd": ["root"],
        "to_entity_kwd": ["leaf"],
    }

    assert module.project_relation(row) == {"from": "root", "to": "leaf", "type": "parent_child"}
