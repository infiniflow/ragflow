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
"""Tests for ``DocMetadataService.get_metadata_facets``.

The facet behind the file list's filter menu used to be counted by reading the
metadata of every document in the dataset: ``get_filter_by_kb_id`` handed the
whole id list to ``get_metadata_for_documents``, which pages the doc-meta index
1000 rows at a time. Measured on a 113,492-document dataset that is 114 searches
per request and 11.6 s of a single-process server that serves nothing else
meanwhile.

An aggregation counts the same values inside the store, so the cost follows the
metadata's cardinality rather than the document count. The fake here counts the
paged searches too, so a change that quietly reintroduces the scan fails.
"""

from types import SimpleNamespace

import pytest

from api.db.db_models import DB
from api.db.services.doc_metadata_service import (
    META_VALUE_SPACE_PAGE_SIZE,
    METADATA_FACET_ID_BATCH,
    DocMetadataService,
)
from common import settings

pytestmark = pytest.mark.p2


class _FakeEs:
    """Stand-in for the ES client: mapping reads, composite and filter aggregations.

    ``search`` honours the query's id scope, and ``size``/``after`` the way a
    composite aggregation does, so a caller that ignores either provably
    returns the wrong facet rather than the test passing on a fake that always
    hands back everything.
    """

    def __init__(self, docs, keys, shards=None, timed_out=False, unindexed=None):
        self._docs = docs
        self._keys = keys
        self._unindexed = unindexed or {}
        self._shards = shards if shards is not None else {"total": 1, "successful": 1, "failed": 0}
        self._timed_out = timed_out
        self.searches = 0
        self.queries = []

    @property
    def indices(self):
        properties = {key: {"type": "text", "fields": {"keyword": {"type": "keyword"}}} for key in self._keys}
        mapping = {"idx": {"mappings": {"properties": {"meta_fields": {"properties": properties}}}}}
        return SimpleNamespace(get_mapping=lambda index: mapping)

    def _scoped(self, query):
        for clause in query["bool"]["filter"]:
            should = clause.get("bool", {}).get("should")
            if should:
                scope = set(should[0]["terms"]["id"])
                return [doc for doc in self._docs if doc["_id"] in scope]
        return list(self._docs)

    @staticmethod
    def _values_of(doc, key):
        value = doc["_source"]["meta_fields"].get(key)
        if value is None:
            return []
        return [str(item) for item in (value if isinstance(value, list) else [value])]

    def _counted(self, docs, key):
        counts = {}
        for doc in docs:
            for value in set(self._values_of(doc, key)):
                counts[value] = counts.get(value, 0) + 1
        return dict(sorted(counts.items()))

    def search(self, index, body, allow_partial_search_results=None):
        self.searches += 1
        self.queries.append(body["query"])
        docs = self._scoped(body["query"])
        aggregations = {}
        for name, spec in (body.get("aggs") or {}).items():
            if name == "carrying":
                aggregations[name] = {"doc_count": sum(1 for doc in docs if any(self._values_of(doc, key) for key in self._keys))}
            elif name == "unindexed":
                aggregations[name] = {"buckets": {key: {"doc_count": self._unindexed.get(key, 0)} for key in spec["filters"]["filters"]}}
            else:
                composite = spec["composite"]
                size = composite["size"]
                key = next(iter(composite["sources"][0]))
                counts = self._counted(docs, key)
                values = list(counts)
                after = composite.get("after")
                if after is not None:
                    values = [value for value in values if value > after[key]]
                page = values[:size]
                aggregations[name] = {"buckets": [{"key": {key: value}, "doc_count": counts[value]} for value in page]}
                if len(page) == size and len(values) > size:
                    aggregations[name]["after_key"] = {key: page[-1]}
        return {"aggregations": aggregations, "_shards": self._shards, "timed_out": self._timed_out}


class _FakeDocStoreConn:
    """Doc store that records how many documents the caller read to count a facet."""

    def __init__(self, docs, keys, with_es=True, index_exists=True, **es_kwargs):
        self._docs = docs
        self._index_exists = index_exists
        self.paged_searches = 0
        self.created_indices = []
        if with_es:
            self.es = _FakeEs(docs, keys, **es_kwargs)

    def index_exist(self, index_name, kb_id):
        return self._index_exists

    def create_doc_meta_idx(self, index_name):
        self.created_indices.append(index_name)
        return True

    def search(self, select_fields, highlight_fields, condition, match_expressions, order_by, offset, limit, index_names, knowledgebase_ids, agg_fields=None, rank_feature=None):
        self.paged_searches += 1
        page = self._docs[offset : offset + limit]
        return {"hits": {"hits": page, "total": {"value": len(self._docs)}}}


def _docs(*metas):
    return [{"_id": f"doc-{index}", "_source": {"meta_fields": meta}} for index, meta in enumerate(metas)]


def _ids(docs):
    return [doc["_id"] for doc in docs]


def _patch(monkeypatch, store):
    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(settings, "docStoreConn", store)
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: SimpleNamespace(tenant_id="tenant-1"))


def test_values_are_counted_without_reading_the_documents(monkeypatch):
    docs = _docs(
        {"phase": "DRP", "project": "p1"},
        {"phase": "DRP", "project": "p2"},
        {"phase": "DSP", "project": "p1"},
    )
    store = _FakeDocStoreConn(docs, ["phase", "project"])
    _patch(monkeypatch, store)

    counts, carrying = DocMetadataService.get_metadata_facets("kb-1", _ids(docs))

    assert counts == {"phase": {"DRP": 2, "DSP": 1}, "project": {"p1": 2, "p2": 1}}
    assert carrying == 3
    # Two searches whatever the dataset holds: one for the coverage count, one
    # for every key's values.
    assert store.es.searches == 2
    assert store.paged_searches == 0


def test_metadata_of_documents_outside_the_dataset_is_not_counted(monkeypatch):
    """The doc-meta index outlives the documents it describes -- a row whose
    document is gone (or was never created) must not put a value in the filter
    menu that matches nothing."""
    docs = _docs({"phase": "DRP"}, {"phase": "orphaned"})
    store = _FakeDocStoreConn(docs, ["phase"])
    _patch(monkeypatch, store)

    counts, carrying = DocMetadataService.get_metadata_facets("kb-1", ["doc-0"])

    assert counts == {"phase": {"DRP": 1}}
    assert carrying == 1


def test_a_document_counts_once_per_distinct_value_it_carries(monkeypatch):
    docs = _docs({"tag": ["a", "b"]}, {"tag": "b"})
    store = _FakeDocStoreConn(docs, ["tag"])
    _patch(monkeypatch, store)

    counts, carrying = DocMetadataService.get_metadata_facets("kb-1", _ids(docs))

    assert counts == {"tag": {"a": 1, "b": 2}}
    assert carrying == 2


def test_documents_without_metadata_are_reported_as_uncovered(monkeypatch):
    """The empty-metadata bucket is the dataset minus this count, so a document
    whose metadata row holds nothing must not be counted as carrying any."""
    docs = _docs({"phase": "DRP"}, {}, {"phase": None})
    store = _FakeDocStoreConn(docs, ["phase"])
    _patch(monkeypatch, store)

    counts, carrying = DocMetadataService.get_metadata_facets("kb-1", _ids(docs))

    assert counts == {"phase": {"DRP": 1}}
    assert carrying == 1


def test_blank_values_do_not_become_facet_entries(monkeypatch):
    """The document scan skips them, and a facet entry with an empty label
    cannot be selected anyway."""
    docs = _docs({"phase": "DRP"}, {"phase": "   "})
    store = _FakeDocStoreConn(docs, ["phase"])
    _patch(monkeypatch, store)

    counts, _ = DocMetadataService.get_metadata_facets("kb-1", _ids(docs))

    assert counts == {"phase": {"DRP": 1}}


def test_values_beyond_one_page_are_paged_not_dropped(monkeypatch):
    """What composite buys over terms: a terms aggregation returns the top
    ``size`` values and drops the rest silently."""
    distinct = META_VALUE_SPACE_PAGE_SIZE + 5
    docs = _docs(*({"ref": f"ref-{index:05d}"} for index in range(distinct)))
    store = _FakeDocStoreConn(docs, ["ref"])
    _patch(monkeypatch, store)

    counts, _ = DocMetadataService.get_metadata_facets("kb-1", _ids(docs))

    assert len(counts["ref"]) == distinct
    assert set(counts["ref"].values()) == {1}


def test_a_dataset_over_the_terms_limit_is_asked_for_in_batches(monkeypatch):
    """ES rejects a terms query with more clauses than index.max_terms_count, so
    the ids go in batches -- and the counts have to add up across them."""
    total = METADATA_FACET_ID_BATCH + 10
    docs = _docs(*({"phase": "DRP"} for _ in range(total)))
    store = _FakeDocStoreConn(docs, ["phase"])
    _patch(monkeypatch, store)

    counts, carrying = DocMetadataService.get_metadata_facets("kb-1", _ids(docs))

    assert counts == {"phase": {"DRP": total}}
    assert carrying == total
    assert max(len(query["bool"]["filter"][1]["bool"]["should"][0]["terms"]["id"]) for query in store.es.queries) <= METADATA_FACET_ID_BATCH


def test_the_scope_names_the_documents_and_the_dataset(monkeypatch):
    docs = _docs({"phase": "DRP"})
    store = _FakeDocStoreConn(docs, ["phase"])
    _patch(monkeypatch, store)

    DocMetadataService.get_metadata_facets("kb-1", ["doc-0", "doc-1"])

    scope = store.es.queries[0]["bool"]["filter"]
    assert {"term": {"kb_id": "kb-1"}} in scope
    assert scope[1]["bool"]["should"] == [{"terms": {"id": ["doc-0", "doc-1"]}}, {"terms": {"_id": ["doc-0", "doc-1"]}}]


def test_values_too_long_to_aggregate_fall_back_to_the_scan(monkeypatch):
    """A dynamically mapped string aggregates through its ``.keyword`` subfield,
    which drops values longer than ignore_above (256 by default). Such a value
    has no bucket, so the facet would be missing an entry the file list can
    filter by -- hand the question back to the caller instead."""
    docs = _docs({"phase": "DRP"})
    store = _FakeDocStoreConn(docs, ["phase"], unindexed={"phase": 3})
    _patch(monkeypatch, store)

    assert DocMetadataService.get_metadata_facets("kb-1", _ids(docs)) is None


@pytest.mark.parametrize(
    "kwargs",
    [
        {"shards": {"total": 2, "successful": 1, "failed": 1}},
        {"timed_out": True},
    ],
    ids=["failed-shard", "timed-out"],
)
def test_a_partial_response_falls_back_rather_than_undercounting(monkeypatch, kwargs):
    """A value absent because its shard failed is indistinguishable from one
    that does not exist, and a facet count is read as exact."""
    docs = _docs({"phase": "DRP"})
    store = _FakeDocStoreConn(docs, ["phase"], **kwargs)
    _patch(monkeypatch, store)

    assert DocMetadataService.get_metadata_facets("kb-1", _ids(docs)) is None


def test_without_an_es_client_the_caller_keeps_the_scan(monkeypatch):
    """Infinity, OceanBase and the rest are unchanged by this path."""
    docs = _docs({"phase": "DRP"})
    store = _FakeDocStoreConn(docs, ["phase"], with_es=False)
    _patch(monkeypatch, store)

    assert DocMetadataService.get_metadata_facets("kb-1", _ids(docs)) is None
    assert store.paged_searches == 0


def test_an_empty_dataset_is_answered_without_asking_the_store(monkeypatch):
    store = _FakeDocStoreConn(_docs({"phase": "DRP"}), ["phase"])
    _patch(monkeypatch, store)

    assert DocMetadataService.get_metadata_facets("kb-1", []) == ({}, 0)
    assert store.es.searches == 0


def test_a_missing_index_reports_no_metadata_without_creating_one(monkeypatch):
    """This is a read; the write paths own creating the index."""
    docs = _docs({"phase": "DRP"})
    store = _FakeDocStoreConn(docs, ["phase"], index_exists=False)
    _patch(monkeypatch, store)

    assert DocMetadataService.get_metadata_facets("kb-1", _ids(docs)) == ({}, 0)
    assert store.created_indices == []
