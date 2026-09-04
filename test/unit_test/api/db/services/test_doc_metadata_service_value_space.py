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
"""Tests for ``DocMetadataService.get_meta_value_space_by_kbs``.

The value space is what ``gen_meta_filter`` picks a metadata filter from, and
that filter is applied as a document scope *before* scoring -- so a value the
model was never shown cannot be chosen, and a wrong choice excludes the right
documents without raising.

``get_flatted_meta_by_kbs`` cannot supply it on a large dataset: it pages the
doc-meta index with from/size, and a doc store that refuses to page past its
result window hands back an empty page, which the loop reads as "no more data".
The fake here reproduces that refusal, so a value carried only by documents
beyond the window is provably invisible to the paged path and present in the
aggregated one.
"""

from types import SimpleNamespace

import pytest

from common import settings
from api.db.services.doc_metadata_service import DocMetadataService, ES_MAX_BUCKETS, META_VALUE_SPACE_PAGE_SIZE, MetaValueSpaceIncomplete
from api.db.db_models import DB

pytestmark = pytest.mark.p2

RESULT_WINDOW = 10000
TOTAL_DOCS = 12000
# Carried only by the last document, i.e. beyond RESULT_WINDOW. This is the
# value the paged path cannot see.
LATE_PHASE = "late-phase"


class _FakeEs:
    """Minimal stand-in for the ES client: mapping reads and composite aggregations.

    ``search`` honors ``size`` and ``after`` the way a composite aggregation
    does -- values sorted, one page at a time, ``after_key`` set while more
    remain. A caller that stops after the first page therefore provably returns
    an incomplete value list, rather than the test passing on a fake that always
    hands back everything.
    """

    def __init__(self, docs: list[dict], keys: list[str], shards: dict | None = None, timed_out: bool = False, drop_key: str | None = None):
        self._docs = docs
        self._keys = keys
        self.requested_sizes: dict[str, int] = {}
        self.searches = 0
        self.partial_kwarg = None
        self._shards = shards if shards is not None else {"total": 1, "successful": 1, "failed": 0}
        self._timed_out = timed_out
        self._drop_key = drop_key

    @property
    def indices(self):
        properties = {key: {"type": "text", "fields": {"keyword": {"type": "keyword"}}} for key in self._keys}
        mapping = {"idx": {"mappings": {"properties": {"meta_fields": {"properties": properties}}}}}
        return SimpleNamespace(get_mapping=lambda index: mapping)

    def _values(self, key: str) -> list[str]:
        values = {str(doc["_source"]["meta_fields"][key]) for doc in self._docs if doc["_source"]["meta_fields"].get(key) is not None}
        return sorted(values)

    def search(self, index, body, allow_partial_search_results=None):
        self.searches += 1
        self.partial_kwarg = allow_partial_search_results
        aggregations = {}
        for name, spec in (body.get("aggs") or {}).items():
            composite = spec["composite"]
            size = composite["size"]
            source = composite["sources"][0]
            key = next(iter(source))
            self.requested_sizes[key] = size
            values = self._values(key)
            after = composite.get("after")
            if after is not None:
                values = [value for value in values if value > after[key]]
            page = values[:size]
            aggregations[name] = {"buckets": [{"key": {key: value}} for value in page]}
            if len(page) == size and len(values) > size:
                aggregations[name]["after_key"] = {key: page[-1]}
        if self._drop_key is not None:
            aggregations.pop(f"vs_{self._drop_key}", None)
        return {"aggregations": aggregations, "_shards": self._shards, "timed_out": self._timed_out}


class _FakeDocStoreConn:
    """Paginated doc store that refuses to page past its result window.

    That refusal is the point: Elasticsearch rejects ``from + size`` beyond
    index.max_result_window, and a connector that swallows the rejection
    returns an empty page instead of raising.
    """

    def __init__(self, docs: list[dict], keys: list[str], with_es: bool = True, **es_kwargs):
        self._docs = docs
        self.paged_searches = 0
        if with_es:
            self.es = _FakeEs(docs, keys, **es_kwargs)

    def index_exist(self, index_name, kb_id):
        return True

    def search(self, select_fields, highlight_fields, condition, match_expressions, order_by, offset, limit, index_names, knowledgebase_ids, agg_fields=None, rank_feature=None):
        self.paged_searches += 1
        if offset >= RESULT_WINDOW:
            return {"hits": {"hits": [], "total": {"value": len(self._docs)}}}
        page = self._docs[offset : min(offset + limit, RESULT_WINDOW)]
        return {"hits": {"hits": page, "total": {"value": len(self._docs)}}}


def _docs(total: int = TOTAL_DOCS) -> list[dict]:
    docs = []
    for index in range(total):
        phase = LATE_PHASE if index == total - 1 else "early-phase"
        docs.append({"_id": f"doc-{index}", "_source": {"meta_fields": {"phase": phase, "project": "p1"}}})
    return docs


def _patch(monkeypatch, store):
    monkeypatch.setattr(DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(settings, "docStoreConn", store)
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr("api.db.services.doc_metadata_service.Knowledgebase.get_by_id", lambda kb_id: SimpleNamespace(tenant_id="tenant-1"))


def test_paged_path_cannot_see_values_beyond_the_result_window(monkeypatch):
    """Establishes the bug the aggregation exists to avoid."""
    _patch(monkeypatch, _FakeDocStoreConn(_docs(), ["phase", "project"]))

    metas = DocMetadataService.get_flatted_meta_by_kbs(["kb-1"])

    assert LATE_PHASE not in metas["phase"]
    assert sum(len(doc_ids) for doc_ids in metas["phase"].values()) == RESULT_WINDOW


def test_value_space_is_complete_beyond_the_result_window(monkeypatch):
    _patch(monkeypatch, _FakeDocStoreConn(_docs(), ["phase", "project"]))

    space = DocMetadataService.get_meta_value_space_by_kbs(["kb-1"])

    assert sorted(space["phase"]) == sorted(["early-phase", LATE_PHASE])
    assert space["project"] == ["p1"]


def test_bucket_budget_is_shared_across_keys(monkeypatch):
    """Many keys must not add up to an illegal request.

    One composite aggregation per key shares ES's search.max_buckets budget, so
    a fixed per-key page size would make the whole search fail once a tenant has
    enough metadata keys.
    """
    keys = [f"key{index}" for index in range(100)]
    docs = [{"_id": "doc-0", "_source": {"meta_fields": {key: "v" for key in keys}}}]
    store = _FakeDocStoreConn(docs, keys)
    _patch(monkeypatch, store)

    DocMetadataService.get_meta_value_space_by_kbs(["kb-1"])

    sizes = store.es.requested_sizes
    assert len(sizes) == len(keys)
    assert sum(sizes.values()) <= ES_MAX_BUCKETS
    assert min(sizes.values()) >= 1


def test_values_beyond_one_page_are_paged_not_dropped(monkeypatch):
    """A key with more distinct values than one page must come back whole.

    This is what composite buys over terms: terms returns the top ``size`` and
    drops the remainder silently, which is the same incompleteness this method
    exists to remove.
    """
    distinct = META_VALUE_SPACE_PAGE_SIZE * 2 + 7
    docs = [{"_id": f"doc-{index}", "_source": {"meta_fields": {"ref": f"ref-{index:05d}"}}} for index in range(distinct)]
    store = _FakeDocStoreConn(docs, ["ref"])
    _patch(monkeypatch, store)

    space = DocMetadataService.get_meta_value_space_by_kbs(["kb-1"])

    assert len(space["ref"]) == distinct
    assert len(set(space["ref"])) == distinct
    assert store.es.searches == 3  # two full pages plus the remainder


def test_low_cardinality_keys_cost_a_single_request(monkeypatch):
    """The common case must not pay per key: all keys travel in one search."""
    store = _FakeDocStoreConn(_docs(), ["phase", "project"])
    _patch(monkeypatch, store)

    DocMetadataService.get_meta_value_space_by_kbs(["kb-1"])

    assert store.es.searches == 1


def test_partial_search_results_are_refused_by_the_request(monkeypatch):
    """The client's default permits partial results; this path must opt out."""
    store = _FakeDocStoreConn(_docs(), ["phase", "project"])
    _patch(monkeypatch, store)

    DocMetadataService.get_meta_value_space_by_kbs(["kb-1"])

    assert store.es.partial_kwarg is False


@pytest.mark.parametrize(
    "kwargs",
    [
        {"shards": {"total": 2, "successful": 1, "failed": 1}},
        {"timed_out": True},
        {"drop_key": "phase"},
    ],
    ids=["failed-shard", "timed-out", "aggregation-absent"],
)
def test_incomplete_responses_raise_rather_than_returning_a_partial_space(monkeypatch, kwargs):
    """A value missing because its shard failed is indistinguishable from one
    that does not exist, so filtering on the remainder would silently drop
    matching documents. Refuse the answer instead."""
    store = _FakeDocStoreConn(_docs(), ["phase", "project"], **kwargs)
    _patch(monkeypatch, store)

    with pytest.raises(MetaValueSpaceIncomplete):
        DocMetadataService.get_meta_value_space_by_kbs(["kb-1"])


def test_incomplete_response_does_not_degrade_to_the_paged_scan(monkeypatch):
    """The result-window limited scan is incomplete in the same way, so it is
    not an acceptable substitute for a partial aggregation."""
    store = _FakeDocStoreConn(_docs(), ["phase", "project"], timed_out=True)
    _patch(monkeypatch, store)

    with pytest.raises(MetaValueSpaceIncomplete):
        DocMetadataService.get_meta_value_space_by_kbs(["kb-1"])
    # the paged path was never consulted
    assert store.paged_searches == 0


def test_falls_back_to_the_paged_path_without_an_es_client(monkeypatch):
    """Non-ES backends keep their previous behaviour."""
    _patch(monkeypatch, _FakeDocStoreConn(_docs(), ["phase", "project"], with_es=False))

    space = DocMetadataService.get_meta_value_space_by_kbs(["kb-1"])

    assert space["project"] == ["p1"]
    assert LATE_PHASE not in space["phase"]
