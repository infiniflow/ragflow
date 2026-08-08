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
import sys
from unittest.mock import AsyncMock, MagicMock

import pytest


class _Dealer:
    def __init__(self, data_store=None):
        self.dataStore = data_store


nlp_search = sys.modules["rag.nlp.search"]
original_dealer = nlp_search.Dealer
original_index_name = nlp_search.index_name
nlp_search.Dealer = _Dealer  # type: ignore[attr-defined]
nlp_search.index_name = lambda tenant_id: f"idx-{tenant_id}"  # type: ignore[attr-defined]

import rag.graphrag.search as search_module
from rag.graphrag.search import KGSearch

nlp_search.Dealer = original_dealer  # type: ignore[attr-defined]
nlp_search.index_name = original_index_name  # type: ignore[attr-defined]


@pytest.mark.asyncio
async def test_query_rewrite_keeps_only_non_empty_string_keywords(monkeypatch):
    async def get_entity_type_samples(_idxnms, _kb_ids):
        return {}

    monkeypatch.setattr(search_module, "get_entity_type2samples", get_entity_type_samples)

    search = KGSearch()
    search._chat = AsyncMock(
        return_value='{"answer_type_keywords":[" INDUSTRY ",{"nested":true},"",42],"entities_from_query":[{"industries":[]}," entity-1 ",null,"entity-2","entity-3","entity-4","entity-5","entity-6"]}'
    )

    type_keywords, entities = await search.query_rewrite(MagicMock(), "question", ["idx"], ["kb"])

    assert type_keywords == ["INDUSTRY"]
    assert entities == ["entity-1", "entity-2", "entity-3", "entity-4", "entity-5"]


@pytest.mark.asyncio
@pytest.mark.parametrize("response", ["null", '"unexpected"', '[{"entity": "unexpected"}]'])
async def test_query_rewrite_rejects_non_object_json(monkeypatch, response):
    async def get_entity_type_samples(_idxnms, _kb_ids):
        return {}

    monkeypatch.setattr(search_module, "get_entity_type2samples", get_entity_type_samples)

    search = KGSearch()
    search._chat = AsyncMock(return_value=response)

    type_keywords, entities = await search.query_rewrite(MagicMock(), "question", ["idx"], ["kb"])

    assert type_keywords == []
    assert entities == []


@pytest.mark.asyncio
async def test_retrieval_falls_back_to_question_when_rewrite_has_no_string_entities(monkeypatch):
    monkeypatch.setattr(search_module, "index_name", lambda tenant_id: f"idx-{tenant_id}")

    search = KGSearch()
    search.get_filters = MagicMock(return_value={"kb_ids": ["kb"]})
    search.query_rewrite = AsyncMock(return_value=([], []))
    search.get_relevant_ents_by_keywords = MagicMock(return_value={})
    search.get_relevant_ents_by_types = MagicMock(return_value={})
    search.get_relevant_relations_by_txt = MagicMock(return_value={})
    search._community_retrieval_ = MagicMock(return_value="")

    await search.retrieval("original question", "tenant", ["kb"], MagicMock(), MagicMock())

    keywords = search.get_relevant_ents_by_keywords.call_args.args[0]
    assert keywords == ["original question"]
