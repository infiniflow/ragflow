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

import importlib.util
import inspect
import json
import sys
import types
from pathlib import Path

import pytest


class _FakeOrderByExpr:
    def desc(self, *_args, **_kwargs):
        return self


class _FakeDealer:
    def __init__(self, data_store):
        self.dataStore = data_store

    def get_filters(self, req):
        return {"kb_id": req["kb_ids"]}

    async def get_vector(self, txt, emb_mdl, topk=10, similarity=0.1):
        return {
            "text": txt,
            "embedding_model": emb_mdl,
            "topk": topk,
            "similarity": similarity,
        }


class _FakeDataFrame:
    def __init__(self, rows):
        self.rows = rows

    def to_csv(self):
        return json.dumps(self.rows)


def _load_graphrag_search_module():
    module_name = "test_graphrag_search_runtime"
    module_path = Path(__file__).resolve().parents[3] / "rag" / "graphrag" / "search.py"

    for pkg_name in ["rag", "rag.graphrag", "common"]:
        if pkg_name not in sys.modules:
            pkg = types.ModuleType(pkg_name)
            pkg.__path__ = []
            sys.modules[pkg_name] = pkg

    stubbed_modules = {
        "common.misc_utils": types.SimpleNamespace(get_uuid=lambda: "uuid"),
        "rag.graphrag.query_analyze_prompt": types.SimpleNamespace(
            PROMPTS={"minirag_query2kwd": "{query}{TYPE_POOL}"}
        ),
        "rag.graphrag.utils": types.SimpleNamespace(
            get_entity_type2samples=lambda *_args, **_kwargs: {},
            get_llm_cache=lambda *_args, **_kwargs: None,
            set_llm_cache=lambda *_args, **_kwargs: None,
            get_relation=lambda *_args, **_kwargs: None,
        ),
        "common.token_utils": types.SimpleNamespace(num_tokens_from_string=lambda txt: len(str(txt))),
        "common.float_utils": types.SimpleNamespace(get_float=float),
        "common.settings": types.SimpleNamespace(),
        "common.doc_store.doc_store_base": types.SimpleNamespace(OrderByExpr=_FakeOrderByExpr),
        "rag.nlp.search": types.SimpleNamespace(Dealer=_FakeDealer, index_name=lambda tenant_id: f"idx_{tenant_id}"),
        "json_repair": types.SimpleNamespace(loads=json.loads, JSONDecodeError=json.JSONDecodeError),
        "pandas": types.SimpleNamespace(DataFrame=_FakeDataFrame),
    }

    previous = {name: sys.modules.get(name) for name in stubbed_modules}
    try:
        sys.modules.update(stubbed_modules)
        sys.modules.pop(module_name, None)
        spec = importlib.util.spec_from_file_location(module_name, module_path)
        module = importlib.util.module_from_spec(spec)
        sys.modules[module_name] = module
        spec.loader.exec_module(module)
        return module
    finally:
        for name, old_module in previous.items():
            if old_module is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = old_module


class _FakeDataStore:
    def __init__(self):
        self.search_calls = []

    def search(self, _fields, _queries, filters, dense_exprs, _order, _offset, _limit, _idxnms, _kb_ids):
        self.search_calls.append({"filters": dict(filters), "dense_exprs": dense_exprs})
        if filters["knowledge_graph_kwd"] == "entity":
            assert dense_exprs
            assert not inspect.iscoroutine(dense_exprs[0])
            return {
                "ent-1": {
                    "content_with_weight": json.dumps({"description": "Alice"}),
                    "_score": 0.9,
                    "entity_kwd": "ALICE",
                    "rank_flt": 2.0,
                    "n_hop_with_weight": "[]",
                }
            }
        if filters["knowledge_graph_kwd"] == "community_report":
            assert dense_exprs == []
            return {}
        assert dense_exprs
        assert not inspect.iscoroutine(dense_exprs[0])
        return {
            "rel-1": {
                "content_with_weight": json.dumps({"description": "knows"}),
                "_score": 0.8,
                "from_entity_kwd": "ALICE",
                "to_entity_kwd": "BOB",
                "weight_int": 3.0,
            }
        }

    def get_fields(self, es_res, _fields):
        return es_res


@pytest.mark.asyncio
async def test_graphrag_retrieval_awaits_vector_search_helpers(monkeypatch):
    search_module = _load_graphrag_search_module()
    assert inspect.iscoroutinefunction(search_module.KGSearch.get_relevant_ents_by_keywords)
    assert inspect.iscoroutinefunction(search_module.KGSearch.get_relevant_relations_by_txt)

    data_store = _FakeDataStore()
    searcher = search_module.KGSearch(data_store)
    awaited_inputs = []

    async def fake_get_vector(txt, emb_mdl, topk=10, similarity=0.1):
        awaited_inputs.append((txt, emb_mdl, topk, similarity))
        return {
            "text": txt,
            "embedding_model": emb_mdl,
            "topk": topk,
            "similarity": similarity,
        }

    async def fake_query_rewrite(_llm, _question, _idxnms, _kb_ids):
        return ["Person"], ["Alice"]

    monkeypatch.setattr(searcher, "get_vector", fake_get_vector)
    monkeypatch.setattr(searcher, "query_rewrite", fake_query_rewrite)

    result = await searcher.retrieval(
        question="Who knows Alice?",
        tenant_ids="tenant-1",
        kb_ids=["kb-1"],
        emb_mdl="embedder",
        llm=object(),
    )

    assert awaited_inputs == [
        ("Alice", "embedder", 1024, 0.3),
        ("Who knows Alice?", "embedder", 1024, 0.3),
    ]
    assert [call["filters"]["knowledge_graph_kwd"] for call in data_store.search_calls] == [
        "entity",
        "entity",
        "relation",
        "community_report",
    ]
    assert "Related content in Knowledge Graph" == result["docnm_kwd"]
    assert '"Entity": "ALICE"' in result["content_with_weight"]
    assert '"From Entity": "ALICE"' in result["content_with_weight"]
    assert '"To Entity": "BOB"' in result["content_with_weight"]
