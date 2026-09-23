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
"""
The agent Retrieval tool must query with the dataset language.

A diacritic-folding dataset (Slovak, Czech) holds folded index tokens, so a
query tokenized as English cannot match them. Mirrors
``entity.KnowledgebasesLanguage`` on the Go side of the same component: one
language only when every dataset agrees, English otherwise.
"""

import asyncio
from types import SimpleNamespace

import pytest

from agent.tools import retrieval as retrieval_module
from agent.tools.retrieval import Retrieval

pytestmark = pytest.mark.p2


def _kb(kb_id, language):
    return SimpleNamespace(id=kb_id, tenant_id="tenant-1", embd_id="", language=language)


def _tool(kbs, monkeypatch):
    calls = {}

    async def _retrieval(*_args, **kwargs):
        calls["language"] = kwargs.get("language")
        return {"chunks": [], "doc_aggs": []}

    monkeypatch.setattr(retrieval_module, "validate_dataset_embedding_models", lambda _records: "")
    monkeypatch.setattr(retrieval_module, "label_question", lambda _query, _kbs: {})
    monkeypatch.setattr(retrieval_module.KnowledgebaseService, "get_by_ids", staticmethod(lambda _ids: kbs))
    monkeypatch.setattr(
        retrieval_module.settings,
        "retriever",
        SimpleNamespace(retrieval=_retrieval, retrieval_by_children=lambda chunks, _tenants: chunks),
        raising=False,
    )

    tool = Retrieval.__new__(Retrieval)
    tool._param = SimpleNamespace(
        dataset_ids=[kb.id for kb in kbs],
        document_ids=[],
        meta_data_filter={},
        cross_languages=[],
        rerank_id="",
        top_n=5,
        top_k=128,
        similarity_threshold=0.2,
        keywords_similarity_weight=0.7,
        rerank_candidates_count=0,
        toc_enhance=False,
        use_kg=False,
        empty_response="",
        outputs={},
    )
    tool._canvas = SimpleNamespace(is_reff=lambda _exp: False)
    tool.check_if_canceled = lambda _message="": False
    return tool, calls


def _language_of(kbs, monkeypatch):
    tool, calls = _tool(kbs, monkeypatch)
    asyncio.run(tool._retrieve_kb("škola"))
    return calls["language"]


def test_one_dataset_retrieves_with_its_language(monkeypatch):
    assert _language_of([_kb("kb-1", "Slovak")], monkeypatch) == "Slovak"


def test_agreeing_datasets_keep_the_language(monkeypatch):
    assert _language_of([_kb("kb-1", "Slovak"), _kb("kb-2", "Slovak")], monkeypatch) == "Slovak"


def test_mixed_datasets_retrieve_as_english(monkeypatch):
    assert _language_of([_kb("kb-1", "Slovak"), _kb("kb-2", "Czech")], monkeypatch) is None
