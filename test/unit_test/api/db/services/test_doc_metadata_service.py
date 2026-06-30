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
from types import SimpleNamespace

import pytest

from api.db.services import doc_metadata_service


class _FakeES:
    def __init__(self, responses):
        self.responses = list(responses)
        self.calls = []

    def search(self, **kwargs):
        self.calls.append(kwargs)
        return self.responses.pop(0)


@pytest.fixture
def es_metadata_env(monkeypatch):
    monkeypatch.setattr(doc_metadata_service.settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(doc_metadata_service.settings, "DOC_ENGINE_OCEANBASE", False, raising=False)
    monkeypatch.setattr(
        doc_metadata_service.Knowledgebase,
        "get_by_id",
        lambda _kb_id: SimpleNamespace(tenant_id="tenant-1"),
    )


@pytest.mark.p2
def test_get_metadata_for_documents_uses_es_direct_lookup(monkeypatch, es_metadata_env):
    es = _FakeES([
        {
            "hits": {
                "hits": [
                    {"_id": "doc-1", "_source": {"id": "doc-1", "meta_fields": {"author": "alice"}}},
                    {"_id": "doc-2", "_source": {"meta_fields": {"author": "bob"}}},
                ]
            }
        }
    ])
    doc_store = SimpleNamespace(
        es=es,
        index_exist=lambda index_name, dataset_id: True,
        search=lambda *args, **kwargs: (_ for _ in ()).throw(AssertionError("generic search should not be used")),
    )
    monkeypatch.setattr(doc_metadata_service.settings, "docStoreConn", doc_store)

    result = doc_metadata_service.DocMetadataService.get_metadata_for_documents(["doc-1", "doc-2"], "kb-1")

    assert result == {"doc-1": {"author": "alice"}, "doc-2": {"author": "bob"}}
    assert es.calls[0]["index"] == "ragflow_doc_meta_tenant-1"
    assert es.calls[0]["body"]["size"] == 2
    assert es.calls[0]["body"]["_source"] == ["id", "kb_id", "meta_fields"]
    filters = es.calls[0]["body"]["query"]["bool"]["filter"]
    assert {"term": {"kb_id": "kb-1"}} in filters
    assert {
        "bool": {
            "should": [
                {"terms": {"id": ["doc-1", "doc-2"]}},
                {"ids": {"values": ["doc-1", "doc-2"]}},
            ],
            "minimum_should_match": 1,
        }
    } in filters


@pytest.mark.p2
def test_search_metadata_es_uses_stable_search_after_pagination():
    first_hits = [
        {"_id": f"doc-{i}", "_source": {"meta_fields": {"i": i}}, "sort": [f"doc-{i:04d}"]}
        for i in range(1000)
    ]
    es = _FakeES([
        {"hits": {"hits": first_hits}},
        {"hits": {"hits": [{"_id": "doc-1000", "_source": {"meta_fields": {"i": 1000}}, "sort": ["doc-1000"]}]}},
    ])

    result = doc_metadata_service.DocMetadataService._search_metadata_es(
        es,
        "ragflow_doc_meta_tenant-1",
        {"kb_id": "kb-1"},
    )

    assert len(result) == 1001
    assert es.calls[0]["body"]["sort"] == [{"id": {"order": "asc", "unmapped_type": "keyword"}}]
    assert "search_after" not in es.calls[0]["body"]
    assert es.calls[1]["body"]["search_after"] == ["doc-0999"]
