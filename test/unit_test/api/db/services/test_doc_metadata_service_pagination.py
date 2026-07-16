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
from api.db.services.doc_metadata_service import DocMetadataService
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
        self._docs = []
        for i in range(total):
            canon = "0" if i < canon_zero_count else "1"
            self._docs.append({"_id": f"doc-{i}", "_source": {"meta_fields": {"canon": canon}}})

    def index_exist(self, index_name, kb_id):
        return True

    def search(self, select_fields, highlight_fields, condition, match_expressions, order_by, offset, limit, index_names, knowledgebase_ids, agg_fields=None, rank_feature=None):
        page = self._docs[offset : offset + limit]
        return {"hits": {"hits": page}}


def test_get_flatted_meta_by_kbs_returns_every_document_beyond_pushdown_cap(monkeypatch):
    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(settings, "docStoreConn", _FakeDocStoreConn(TOTAL_DOCS, CANON_ZERO_COUNT))
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
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    fake_kb = SimpleNamespace(tenant_id="tenant-1")
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: fake_kb)

    metas = DocMetadataService.get_flatted_meta_by_kbs(["kb-1"])
    doc_ids = meta_filter(metas, [{"key": "canon", "op": "not in", "value": ["0"]}])

    assert len(doc_ids) == TOTAL_DOCS - CANON_ZERO_COUNT
