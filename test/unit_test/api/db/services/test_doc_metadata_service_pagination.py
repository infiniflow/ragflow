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
"""Regression test for #16524: a manual metadata filter over a knowledge base
with more documents than the ES push-down cap (``filter_doc_ids_by_meta_pushdown``'s
default ``limit=10000``) must still see every document once the request falls
back to the in-memory path, not just the first page.

Exercises ``DocMetadataService.get_flatted_meta_by_kbs`` end-to-end against a
fake, paginated ``docStoreConn`` standing in for Elasticsearch, then feeds the
result into ``meta_filter`` with the same ``not in`` condition from the
original report.
"""

from types import SimpleNamespace

import pytest

from common import settings
from common.metadata_utils import meta_filter
from api.db.services.doc_metadata_service import DocMetadataService, METADATA_ID_BATCH_SIZE, MetadataPaginationError
from api.db.db_models import DB

pytestmark = pytest.mark.p2

TOTAL_DOCS = 12000
CANON_ZERO_COUNT = 30  # a small minority tagged "0"; the rest are "1"


class _FakeDocStoreConn:
    """Stands in for the ES connection's paginated ``search``.

    Mirrors the shape ``DocMetadataService._iter_search_results`` expects
    (``{"hits": {"hits": [{"_id": ..., "_source": {...}}]}}``) and actually
    honors ``offset``/``limit`` so a caller that stops paginating too early
    provably sees a truncated result, the way the reported bug did.
    """

    def __init__(self, total: int, canon_zero_count: int):
        self.conditions = []
        self._docs = []
        for i in range(total):
            canon = "0" if i < canon_zero_count else "1"
            self._docs.append({"_id": f"doc-{i}", "_source": {"meta_fields": {"canon": canon}}})

    def index_exist(self, index_name, kb_id):
        return True

    def search(self, select_fields, highlight_fields, condition, match_expressions, order_by, offset, limit, index_names, knowledgebase_ids, agg_fields=None, rank_feature=None):
        self.conditions.append(condition.copy())
        docs = self._docs
        if condition.get("id"):
            doc_ids = set(condition["id"])
            docs = [doc for doc in docs if doc["_id"] in doc_ids]
        page = [{**hit, "sort": [hit["_id"]]} for hit in docs[offset : offset + limit]]
        return {"hits": {"hits": page, "total": {"value": len(docs)}}}


def test_get_flatted_meta_by_kbs_returns_every_document_beyond_pushdown_cap(monkeypatch):
    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(settings, "docStoreConn", _FakeDocStoreConn(TOTAL_DOCS, CANON_ZERO_COUNT))
    monkeypatch.setattr(settings, "DOC_ENGINE", "oceanbase")
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    fake_kb = SimpleNamespace(tenant_id="tenant-1")
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: fake_kb)

    metas = DocMetadataService.get_flatted_meta_by_kbs(["kb-1"])

    assert len(metas["canon"]["1"]) == TOTAL_DOCS - CANON_ZERO_COUNT
    assert len(metas["canon"]["0"]) == CANON_ZERO_COUNT


def test_manual_not_in_filter_matches_every_document_beyond_pushdown_cap(monkeypatch):
    # Same scenario as the #16524 report: a "canon Not in ['0']" manual filter
    # over a KB whose match set (TOTAL_DOCS - CANON_ZERO_COUNT) exceeds the
    # push-down cap, so this exercises the in-memory fallback exclusively.
    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(settings, "docStoreConn", _FakeDocStoreConn(TOTAL_DOCS, CANON_ZERO_COUNT))
    monkeypatch.setattr(settings, "DOC_ENGINE", "oceanbase")
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    fake_kb = SimpleNamespace(tenant_id="tenant-1")
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: fake_kb)

    metas = DocMetadataService.get_flatted_meta_by_kbs(["kb-1"])
    doc_ids = meta_filter(metas, [{"key": "canon", "op": "not in", "value": ["0"]}])

    assert len(doc_ids) == TOTAL_DOCS - CANON_ZERO_COUNT


def test_get_metadata_for_documents_batches_large_id_filters(monkeypatch):
    total = METADATA_ID_BATCH_SIZE * 2 + 1
    store = _FakeDocStoreConn(total, 0)
    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(settings, "docStoreConn", store)
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    fake_kb = SimpleNamespace(tenant_id="tenant-1")
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: fake_kb)

    doc_ids = [f"doc-{index}" for index in range(total)]
    metadata = DocMetadataService.get_metadata_for_documents(doc_ids, "kb-1")

    filtered_conditions = [condition for condition in store.conditions if "id" in condition]
    requested_batches = {tuple(condition["id"]) for condition in filtered_conditions}
    assert len(metadata) == total
    assert len(requested_batches) == 3
    assert all(len(condition["id"]) <= METADATA_ID_BATCH_SIZE for condition in filtered_conditions)


def test_es_metadata_pagination_advances_cursor_without_replaying_offsets(monkeypatch):
    class CursorStore(_FakeDocStoreConn):
        def __init__(self):
            super().__init__(2501, 0)
            self.requests = []

        def search(self, *args, **kwargs):
            self.requests.append((kwargs["offset"], kwargs.get("search_after")))
            cursor = kwargs.get("search_after")
            start = int(cursor[0]) + 1 if cursor else kwargs["offset"]
            end = min(start + kwargs["limit"], len(self._docs))
            hits = []
            for i in range(start, end):
                hits.append({**self._docs[i], "sort": [str(i)]})
            return {"hits": {"hits": hits, "total": {"value": len(self._docs)}}}

    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    store = CursorStore()
    monkeypatch.setattr(settings, "docStoreConn", store)
    monkeypatch.setattr(settings, "DOC_ENGINE", "elasticsearch")
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: SimpleNamespace(tenant_id="tenant-1"))

    metas = DocMetadataService.get_flatted_meta_by_kbs(["kb-1"])
    assert len(metas["canon"]["1"]) == 2501
    assert store.requests == [(0, None), (0, ["999"]), (0, ["1999"])]


def test_es_stalled_cursor_raises_instead_of_returning_partial_metadata(monkeypatch):
    class StalledStore(_FakeDocStoreConn):
        def __init__(self):
            super().__init__(2501, 0)
            self.requests = []

        def search(self, *args, **kwargs):
            cursor = kwargs.get("search_after")
            self.requests.append((kwargs["offset"], cursor))
            start = 0 if cursor else kwargs["offset"]
            end = min(start + kwargs["limit"], len(self._docs))
            hits = [{**self._docs[i], "sort": [str(i)]} for i in range(start, end)]
            return {"hits": {"hits": hits, "total": {"value": len(self._docs)}}}

    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    store = StalledStore()
    monkeypatch.setattr(settings, "docStoreConn", store)
    monkeypatch.setattr(settings, "DOC_ENGINE", "elasticsearch")
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: SimpleNamespace(tenant_id="tenant-1"))

    with pytest.raises(MetadataPaginationError, match="did not advance"):
        DocMetadataService.get_flatted_meta_by_kbs(["kb-1"])
    assert store.requests == [(0, None), (0, ["999"])]


def test_es_full_page_without_sort_raises_instead_of_silent_truncation(monkeypatch):
    class MissingSortStore(_FakeDocStoreConn):
        def search(self, *args, **kwargs):
            response = super().search(*args, **kwargs)
            for hit in response["hits"]["hits"]:
                hit.pop("sort", None)
            return response

    store = MissingSortStore(12000, 0)
    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(settings, "docStoreConn", store)
    monkeypatch.setattr(settings, "DOC_ENGINE", "elasticsearch")
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: SimpleNamespace(tenant_id="tenant-1"))

    with pytest.raises(MetadataPaginationError, match="no sort value"):
        DocMetadataService.get_flatted_meta_by_kbs(["kb-1"])


def test_es_empty_cursor_page_with_more_total_raises(monkeypatch):
    class EmptyCursorStore(_FakeDocStoreConn):
        def search(self, *args, **kwargs):
            if kwargs.get("search_after"):
                return {"hits": {"hits": [], "total": {"value": len(self._docs)}}}
            response = super().search(*args, **kwargs)
            for i, hit in enumerate(response["hits"]["hits"]):
                hit["sort"] = [str(i)]
            return response

    store = EmptyCursorStore(2501, 0)
    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(settings, "docStoreConn", store)
    monkeypatch.setattr(settings, "DOC_ENGINE", "elasticsearch")
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: SimpleNamespace(tenant_id="tenant-1"))

    with pytest.raises(MetadataPaginationError, match="ended after 1000 of 2501"):
        DocMetadataService.get_flatted_meta_by_kbs(["kb-1"])
