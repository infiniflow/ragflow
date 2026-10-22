import asyncio
import importlib.util
import json
import os
import sys
from types import ModuleType, SimpleNamespace

import pytest


def _load_wiki_module(monkeypatch):
    token_utils = ModuleType("common.token_utils")
    token_utils.num_tokens_from_string = lambda text: len(text)
    monkeypatch.setitem(sys.modules, "common.token_utils", token_utils)
    xxhash = ModuleType("xxhash")
    xxhash.xxh64 = lambda value: SimpleNamespace(hexdigest=lambda: "test-hash")
    monkeypatch.setitem(sys.modules, "xxhash", xxhash)

    generator = sys.modules["rag.prompts.generator"]
    monkeypatch.setattr(generator, "gen_json", lambda *args, **kwargs: {}, raising=False)

    common = sys.modules["rag.advanced_rag.knowlege_compile._common"]
    monkeypatch.setattr(common, "build_chunk_batches", lambda *args, **kwargs: ([], {}), raising=False)
    monkeypatch.setattr(common, "bulk_dedup_items", lambda items, *args, **kwargs: items, raising=False)
    monkeypatch.setattr(common, "ensure_llm_bundle", lambda model: model, raising=False)
    monkeypatch.setattr(common, "knowledge_compile_gen_conf", lambda *args, **kwargs: {}, raising=False)
    monkeypatch.setattr(common, "run_chunked_pipeline", lambda *args, **kwargs: {}, raising=False)
    monkeypatch.setattr(common, "stable_row_id", lambda *parts: "|".join(str(part) for part in parts), raising=False)

    structure = sys.modules["rag.advanced_rag.knowlege_compile.structure"]
    monkeypatch.setattr(structure, "_struct_get", lambda *args, **kwargs: None, raising=False)
    monkeypatch.setattr(structure, "_struct_localize", lambda value, *args, **kwargs: value, raising=False)

    module_name = "rag.advanced_rag.knowlege_compile.wiki"
    module_path = os.path.normpath(os.path.join(os.path.dirname(__file__), "../../../../../rag/advanced_rag/knowlege_compile/wiki.py"))
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


class _StateDocStore:
    def __init__(self, rows=None):
        self.rows = {row["id"]: dict(row) for row in rows or []}
        self.insert_calls = []

    def search(self, fields, _highlights, condition, _matches, _order, offset, limit, _index, _dataset_ids):
        rows = []
        for row in self.rows.values():
            if any(condition.get(key) and row.get(key) not in condition[key] for key in ("id", "compile_kwd", "type_kwd")):
                continue
            if condition.get("doc_id") and row.get("doc_id") not in condition["doc_id"]:
                continue
            if condition.get("source_chunk_ids") and not set(row.get("source_chunk_ids") or []) & set(condition["source_chunk_ids"]):
                continue
            if condition.get("chunk_hash_kwd") and row.get("chunk_hash_kwd") not in condition["chunk_hash_kwd"]:
                continue
            rows.append(row)
        return rows[offset : offset + limit]

    def get_fields(self, result, fields):
        return {row["id"]: {field: row.get(field) for field in fields} for row in result}

    def insert(self, rows, _index, _dataset_id):
        self.insert_calls.append([dict(row) for row in rows])
        for row in rows:
            self.rows[row["id"]] = dict(row)

    def delete(self, condition, _index, _dataset_id):
        for row_id, row in list(self.rows.items()):
            if all(not condition.get(key) or row.get(key) in condition[key] for key in ("compile_kwd", "type_kwd")):
                del self.rows[row_id]


class _KbScopedDocStore(_StateDocStore):
    """Mirrors ES/OpenSearch search: every query is pinned to ``kb_id``."""

    def search(self, fields, highlights, condition, matches, order, offset, limit, index, dataset_ids):
        scoped = dict(condition or {})
        if dataset_ids:
            scoped["kb_id"] = list(dataset_ids)
        rows = []
        for row in self.rows.values():
            if scoped.get("kb_id") and row.get("kb_id") not in scoped["kb_id"]:
                continue
            if any(scoped.get(key) and row.get(key) not in scoped[key] for key in ("id", "compile_kwd", "type_kwd")):
                continue
            if scoped.get("doc_id") and row.get("doc_id") not in scoped["doc_id"]:
                continue
            if scoped.get("source_chunk_ids") and not set(row.get("source_chunk_ids") or []) & set(scoped["source_chunk_ids"]):
                continue
            if scoped.get("chunk_hash_kwd") and row.get("chunk_hash_kwd") not in scoped["chunk_hash_kwd"]:
                continue
            rows.append(row)
        return rows[offset : offset + limit]


class _MaxFilterValuesDocStore(_KbScopedDocStore):
    """Rejects a query whose filter list exceeds the backend's predicate budget.

    Infinity emits one predicate per filter value and nests them into an OR
    chain, which Thrift caps at 64 levels ("Maximum recursion depth exceeded").
    """

    def __init__(self, rows=None, max_values=8):
        super().__init__(rows)
        self.max_values = max_values
        self.widest_filter = 0
        self.query_count = 0
        self.hash_filter_queries = 0

    def search(self, fields, highlights, condition, matches, order, offset, limit, index, dataset_ids):
        if condition.get("chunk_hash_kwd"):
            self.hash_filter_queries += 1
        for key in ("doc_id", "source_chunk_ids", "chunk_hash_kwd"):
            values = condition.get(key)
            if values:
                self.widest_filter = max(self.widest_filter, len(values))
                if len(values) > self.max_values:
                    raise RuntimeError("Maximum recursion depth exceeded")
        self.query_count += 1
        return super().search(fields, highlights, condition, matches, order, offset, limit, index, dataset_ids)


# Reporter chunk ids from infiniflow/ragflow#19092. MAP logged extracted=9 then
# immediately listed these same ids as missing_chunks.
_REPORTER_CHUNK_IDS = [
    "0b9beb4afcaf7717",
    "2382483997f1183d",
    "31c63a0feb6b8207",
    "39feff69089a3d59",
    "5467040776c88761",
    "60ffd6504434bae2",
    "87b76b7a1ebb3dc5",
    "d96c875495acd52c",
    "e96857ea65980dec",
]


def _missing_after_resolve(wiki, tenant_id, kb_id, state, target_chunk_ids):
    resolved = asyncio.run(wiki._wiki_load_map_extracts_for_state(tenant_id, kb_id, state, target_chunk_ids))
    resolved_ids = {str((extract.get("_map_version") or {}).get("chunk_id") or "") for extract in resolved}
    return set(target_chunk_ids) - resolved_ids


def test_active_map_state_switches_marker_after_new_generation(monkeypatch):
    wiki = _load_wiki_module(monkeypatch)
    marker_id = wiki._stable_row_id(wiki.WIKI_MAP_STATE_META_COMPILE_KWD, "kb-1")
    store = _StateDocStore(
        [
            {
                "id": marker_id,
                "compile_kwd": wiki.WIKI_MAP_STATE_META_COMPILE_KWD,
                "type_kwd": "old",
            },
            {
                "id": "old-state",
                "doc_id": "doc-old",
                "compile_kwd": wiki.WIKI_MAP_STATE_COMPILE_KWD,
                "type_kwd": "old",
                "source_chunk_ids": ["chunk-old"],
                "chunk_hash_kwd": "hash-old",
            },
        ]
    )
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    asyncio.run(wiki._wiki_commit_active_map_state("tenant-1", "kb-1", {"chunk-new": {"doc_id": "doc-new", "hash": "hash-new"}}))

    assert store.insert_calls[0][0]["compile_kwd"] == wiki.WIKI_MAP_STATE_COMPILE_KWD
    assert store.insert_calls[1][0]["compile_kwd"] == wiki.WIKI_MAP_STATE_META_COMPILE_KWD
    assert asyncio.run(wiki._wiki_load_active_map_state("tenant-1", "kb-1")) == {"chunk-new": {"doc_id": "doc-new", "hash": "hash-new"}}
    assert "old-state" not in store.rows


def test_map_version_query_is_limited_to_requested_chunk_hash(monkeypatch):
    wiki = _load_wiki_module(monkeypatch)
    store = _StateDocStore(
        [
            {
                "id": "wanted",
                "doc_id": "doc-1",
                "compile_kwd": wiki.WIKI_MAP_COMPILE_KWD,
                "source_chunk_ids": ["chunk-1"],
                "chunk_hash_kwd": "hash-b",
                "content_with_weight": json.dumps({"entities": [{"name": "B"}]}),
            },
            {
                "id": "historical",
                "doc_id": "doc-1",
                "compile_kwd": wiki.WIKI_MAP_COMPILE_KWD,
                "source_chunk_ids": ["chunk-1"],
                "chunk_hash_kwd": "hash-a",
                "content_with_weight": json.dumps({"entities": [{"name": "A"}]}),
            },
        ]
    )
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    versions = asyncio.run(wiki._wiki_load_map_versions("doc-1", "tenant-1", "kb-1", {"chunk-1": "hash-b"}))

    assert set(versions["chunk-1"]) == {"hash-b"}


def test_failed_state_write_keeps_previous_generation_active(monkeypatch):
    wiki = _load_wiki_module(monkeypatch)
    marker_id = wiki._stable_row_id(wiki.WIKI_MAP_STATE_META_COMPILE_KWD, "kb-1")

    class _FailingStore(_StateDocStore):
        def insert(self, rows, index, dataset_id):
            if rows[0].get("compile_kwd") == wiki.WIKI_MAP_STATE_COMPILE_KWD:
                raise RuntimeError("state write failed")
            super().insert(rows, index, dataset_id)

    store = _FailingStore(
        [
            {
                "id": marker_id,
                "compile_kwd": wiki.WIKI_MAP_STATE_META_COMPILE_KWD,
                "type_kwd": "old",
            },
            {
                "id": "old-state",
                "doc_id": "doc-old",
                "compile_kwd": wiki.WIKI_MAP_STATE_COMPILE_KWD,
                "type_kwd": "old",
                "source_chunk_ids": ["chunk-old"],
                "chunk_hash_kwd": "hash-old",
            },
        ]
    )
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    with pytest.raises(RuntimeError, match="state write failed"):
        asyncio.run(wiki._wiki_commit_active_map_state("tenant-1", "kb-1", {"chunk-new": {"doc_id": "doc-new", "hash": "hash-new"}}))

    assert asyncio.run(wiki._wiki_load_active_map_state("tenant-1", "kb-1")) == {"chunk-old": {"doc_id": "doc-old", "hash": "hash-old"}}


def test_persisted_map_extracts_are_not_missing_from_completeness_check(monkeypatch):
    """Successful MAP persist must satisfy the incremental completeness check.

    Issue 19092: wiki_map logged requested=9 cache_hits=0 extracted=9, then the
    follow-up cache resolution reported the same 9 source-chunk ids as missing.
    The rows were written, but searches always filter by kb_id and the resume
    doc omitted that field, so every extracted chunk looked absent.
    """
    wiki = _load_wiki_module(monkeypatch)
    store = _KbScopedDocStore()
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    doc_id = "6dc70e7aa5a411f19c0109b856bc47ac"
    kb_id = "ff675c34a5a111f19c0109b856bc47ac"
    per_chunk = {chunk_id: wiki._wiki_empty_extract() for chunk_id in _REPORTER_CHUNK_IDS}
    hashes = {chunk_id: f"hash-{chunk_id}" for chunk_id in _REPORTER_CHUNK_IDS}
    state = {chunk_id: {"doc_id": doc_id, "hash": hashes[chunk_id]} for chunk_id in _REPORTER_CHUNK_IDS}
    target = set(_REPORTER_CHUNK_IDS)

    asyncio.run(wiki._wiki_persist_extracts(per_chunk, doc_id, "tenant-1", kb_id, chunk_hashes=hashes))

    written = [row for batch in store.insert_calls for row in batch]
    assert {row["kb_id"] for row in written} == {kb_id}
    assert {row["source_chunk_ids"][0] for row in written} == target
    assert _missing_after_resolve(wiki, "tenant-1", kb_id, state, target) == set()


def test_completeness_check_still_reports_chunks_that_were_never_persisted(monkeypatch):
    wiki = _load_wiki_module(monkeypatch)
    store = _KbScopedDocStore()
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    doc_id = "doc-1"
    kb_id = "kb-1"
    persisted = _REPORTER_CHUNK_IDS[:-1]
    missing_id = _REPORTER_CHUNK_IDS[-1]
    hashes = {chunk_id: f"hash-{chunk_id}" for chunk_id in _REPORTER_CHUNK_IDS}
    state = {chunk_id: {"doc_id": doc_id, "hash": hashes[chunk_id]} for chunk_id in _REPORTER_CHUNK_IDS}
    per_chunk = {chunk_id: wiki._wiki_empty_extract() for chunk_id in persisted}

    asyncio.run(wiki._wiki_persist_extracts(per_chunk, doc_id, "tenant-1", kb_id, chunk_hashes=hashes))

    assert _missing_after_resolve(wiki, "tenant-1", kb_id, state, set(_REPORTER_CHUNK_IDS)) == {missing_id}


def test_map_extract_without_kb_id_is_invisible_to_completeness_check(monkeypatch):
    """Rows that omit kb_id must not count as resolved under a kb-scoped search."""
    wiki = _load_wiki_module(monkeypatch)
    store = _KbScopedDocStore()
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    chunk_id = _REPORTER_CHUNK_IDS[0]
    doc_id = "doc-1"
    kb_id = "kb-1"
    chunk_hash = "hash-1"
    store.rows["orphan"] = {
        "id": "orphan",
        "doc_id": doc_id,
        "compile_kwd": wiki.WIKI_MAP_COMPILE_KWD,
        "source_chunk_ids": [chunk_id],
        "chunk_hash_kwd": chunk_hash,
        "content_with_weight": json.dumps({"entities": []}),
    }
    state = {chunk_id: {"doc_id": doc_id, "hash": chunk_hash}}

    assert _missing_after_resolve(wiki, "tenant-1", kb_id, state, {chunk_id}) == {chunk_id}


def test_wiki_map_from_chunks_successful_extract_is_not_reported_missing(monkeypatch):
    """Successful MAP through wiki_map_from_chunks must not look missing afterward.

    Issue 19092: wiki_map logged extracted=9 then the generator completeness
    check listed the same source-chunk ids as missing_chunks. The isolated
    persist + lookup tests cover the row shape; this drives the public MAP
    entry so extract → persist → kb-scoped completeness run as one flow.
    Extraction is stubbed; persist and completeness use the real helpers
    against a kb-scoped fake doc store.
    """
    wiki = _load_wiki_module(monkeypatch)
    store = _KbScopedDocStore()
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    def _pack_batches(chunks, _chat_mdl, **kwargs):
        resume = kwargs.get("resume_chunk_ids") or set()
        picker = kwargs.get("chunk_text_picker") or (lambda chunk: chunk.get("text") or "")
        packed = []
        for idx, chunk in enumerate(chunks):
            chunk_id = chunk.get("id") or chunk.get("chunk_id")
            if not isinstance(chunk_id, str) or not chunk_id or chunk_id in resume:
                continue
            packed.append({"label": f"C{idx + 1}", "chunk_id": chunk_id, "text": picker(chunk) or ""})
        return ([packed] if packed else [], {"n_batches": int(bool(packed))})

    async def _run_pipeline(batches, *, process_batch, aggregate=None, **_kwargs):
        if not batches:
            return aggregate([]) if aggregate else []
        results = [await process_batch(batch, idx, len(batches)) for idx, batch in enumerate(batches)]
        return aggregate(results) if aggregate else results

    extracted_from_llm = []

    async def _extract_ok(packed, *_args, **_kwargs):
        extracted_from_llm.extend(entry["chunk_id"] for entry in packed)
        return wiki._wiki_empty_extract()

    monkeypatch.setattr(wiki, "_build_chunk_batches", _pack_batches)
    monkeypatch.setattr(wiki, "_run_chunked_pipeline", _run_pipeline)
    monkeypatch.setattr(wiki, "_wiki_extract_one_batch", _extract_ok)

    doc_id = "6dc70e7aa5a411f19c0109b856bc47ac"
    kb_id = "ff675c34a5a111f19c0109b856bc47ac"
    tenant_id = "tenant-1"
    never_extracted_id = "never-extracted-chunk"
    chunks = [{"id": chunk_id, "text": f"body of {chunk_id}"} for chunk_id in _REPORTER_CHUNK_IDS]
    chat_mdl = SimpleNamespace(max_length=8192, llm_name="mock-chat")

    first = asyncio.run(
        wiki.wiki_map_from_chunks(
            chunks=chunks,
            chat_mdl=chat_mdl,
            embd_mdl=None,
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
        )
    )

    assert first["_meta"]["requested"] == len(_REPORTER_CHUNK_IDS)
    assert first["_meta"]["extracted"] == len(_REPORTER_CHUNK_IDS)
    assert first["_meta"]["cache_hits"] == 0
    assert set(extracted_from_llm) == set(_REPORTER_CHUNK_IDS)

    written = [row for batch in store.insert_calls for row in batch]
    assert {row["kb_id"] for row in written} == {kb_id}
    assert {row["source_chunk_ids"][0] for row in written} == set(_REPORTER_CHUNK_IDS)

    hashes = {chunk_id: wiki._chunk_hash(f"body of {chunk_id}") for chunk_id in _REPORTER_CHUNK_IDS}
    state = {chunk_id: {"doc_id": doc_id, "hash": hashes[chunk_id]} for chunk_id in _REPORTER_CHUNK_IDS}
    state[never_extracted_id] = {"doc_id": doc_id, "hash": "hash-never"}
    target = set(_REPORTER_CHUNK_IDS) | {never_extracted_id}

    # Same completeness path as run_wiki_incremental after MAP: load extracts
    # for the current chunk state and treat unresolved ids as missing_chunks.
    assert _missing_after_resolve(wiki, tenant_id, kb_id, state, target) == {never_extracted_id}

    second = asyncio.run(
        wiki.wiki_map_from_chunks(
            chunks=chunks,
            chat_mdl=chat_mdl,
            embd_mdl=None,
            doc_id=doc_id,
            tenant_id=tenant_id,
            kb_id=kb_id,
        )
    )
    assert second["_meta"]["cache_hits"] == len(_REPORTER_CHUNK_IDS)
    assert second["_meta"]["extracted"] == 0
    assert extracted_from_llm == list(_REPORTER_CHUNK_IDS)


def test_map_version_query_never_sized_by_chunk_count(monkeypatch):
    """Resolving cached MAP versions must not build one unbounded filter.

    Infinity serializes every predicate as a nested Thrift expression, so a
    filter sized for a whole knowledge base trips the 64-level recursion cap
    ("Maximum recursion depth exceeded") and the lookup reports every chunk of
    the run as missing. Only doc ids go into the filter — chunk ids and hashes
    are matched in Python — so a document's chunk count cannot blow the budget.
    """
    wiki = _load_wiki_module(monkeypatch)
    monkeypatch.setattr(wiki, "_WIKI_MAP_VERSION_DOC_BATCH", 2)
    store = _MaxFilterValuesDocStore(max_values=2)
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    doc_ids = [f"doc-{idx}" for idx in range(5)]
    kb_id = "kb-1"
    chunk_ids = []
    hashes = {}
    state = {}
    for doc_id in doc_ids:
        for idx in range(4):  # many chunks per document, all resolved
            chunk_id = f"chunk-{doc_id}-{idx}"
            chunk_ids.append(chunk_id)
            hashes[chunk_id] = f"hash-{chunk_id}"
            state[chunk_id] = {"doc_id": doc_id, "hash": hashes[chunk_id]}
        asyncio.run(wiki._wiki_persist_extracts({cid: wiki._wiki_empty_extract() for cid in chunk_ids if cid.startswith(f"chunk-{doc_id}-")}, doc_id, "tenant-1", kb_id, chunk_hashes=hashes))

    versions = asyncio.run(wiki._wiki_load_map_versions(set(doc_ids), "tenant-1", kb_id, hashes))

    assert store.widest_filter <= 2
    assert store.hash_filter_queries == 0
    assert store.query_count > 1
    assert set(versions) == set(chunk_ids)
    assert all(set(versions[chunk_id]) == {hashes[chunk_id]} for chunk_id in chunk_ids)
    assert _missing_after_resolve(wiki, "tenant-1", kb_id, state, set(chunk_ids)) == set()


class _OversizedBatchDocStore(_KbScopedDocStore):
    """Rejects any query whose doc list is wider than one document.

    Mirrors an oversized result set (ES ``max_result_window``, deep pagination)
    to prove the lookup degrades to per-doc retries instead of dropping every
    cached extract of the run.
    """

    def __init__(self, rows=None):
        super().__init__(rows)
        self.doc_query_widths = []

    def search(self, fields, highlights, condition, matches, order, offset, limit, index, dataset_ids):
        docs = condition.get("doc_id") or []
        self.doc_query_widths.append(len(docs))
        if len(docs) > 1:
            raise RuntimeError("result window too large")
        return super().search(fields, highlights, condition, matches, order, offset, limit, index, dataset_ids)


def test_version_lookup_degrades_to_per_doc_retry(monkeypatch):
    """A batch that cannot be paged through must not cost the whole lookup.

    Returning on the first failure (the old behaviour) silently marked every
    cached extract as missing — the very symptom this fix is about.
    """
    wiki = _load_wiki_module(monkeypatch)
    monkeypatch.setattr(wiki, "_WIKI_MAP_VERSION_DOC_BATCH", 3)
    doc_ids = [f"doc-{idx}" for idx in range(3)]
    rows = []
    hashes = {}
    for doc_id in doc_ids:
        chunk_id = f"chunk-{doc_id}"
        hashes[chunk_id] = f"hash-{chunk_id}"
        rows.append(
            {
                "id": f"row-{doc_id}",
                "kb_id": "kb-1",
                "doc_id": doc_id,
                "compile_kwd": wiki.WIKI_MAP_COMPILE_KWD,
                "source_chunk_ids": [chunk_id],
                "chunk_hash_kwd": hashes[chunk_id],
                "content_with_weight": json.dumps({"entities": []}),
            }
        )
    store = _OversizedBatchDocStore(rows)
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    versions = asyncio.run(wiki._wiki_load_map_versions(set(doc_ids), "tenant-1", "kb-1", hashes))

    assert set(versions) == set(hashes)
    assert max(store.doc_query_widths) == 3  # tried as one batch first
    assert store.doc_query_widths.count(1) >= 3  # then degraded to per-doc


def test_map_version_query_batches_every_history_lookup(monkeypatch):
    """The KB-wide resolve path (no requested versions) batches by doc id."""
    wiki = _load_wiki_module(monkeypatch)
    monkeypatch.setattr(wiki, "_WIKI_MAP_VERSION_DOC_BATCH", 3)
    store = _MaxFilterValuesDocStore(max_values=3)
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    doc_ids = [f"doc-{idx}" for idx in range(7)]
    rows = []
    for doc_id in doc_ids:
        for chunk_idx in range(2):
            chunk_id = f"chunk-{doc_id}-{chunk_idx}"
            rows.append(
                {
                    "id": f"row-{doc_id}-{chunk_idx}",
                    "kb_id": "kb-1",
                    "doc_id": doc_id,
                    "compile_kwd": wiki.WIKI_MAP_COMPILE_KWD,
                    "source_chunk_ids": [chunk_id],
                    "chunk_hash_kwd": f"hash-{chunk_id}",
                    "content_with_weight": json.dumps({"entities": []}),
                }
            )
    store.rows = {row["id"]: row for row in rows}

    versions = asyncio.run(wiki._wiki_load_map_versions(set(doc_ids), "tenant-1", "kb-1"))

    assert store.query_count > 1
    assert set(versions) == {f"chunk-{doc_id}-{chunk_idx}" for doc_id in doc_ids for chunk_idx in range(2)}


def _packed(chunk_ids):
    return [{"label": f"C{idx + 1}", "chunk_id": chunk_id, "text": f"body of {chunk_id}"} for idx, chunk_id in enumerate(chunk_ids)]


def test_timed_out_map_batch_is_retried_as_smaller_batches(monkeypatch):
    """A batch whose extraction times out is re-run split in half.

    Without the retry the whole batch's chunks are dropped for the run and
    the generator reports them as missing MAP output. Halving the prompt
    gives the smaller calls a real chance to finish.
    """
    wiki = _load_wiki_module(monkeypatch)
    store = _KbScopedDocStore()
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    seen_sizes: list[int] = []

    async def _extract(packed, *_args, **_kwargs):
        seen_sizes.append(len(packed))
        if len(packed) > 1:
            raise wiki._WikiMapBatchTimeout("timed out")
        extract = wiki._wiki_empty_extract()
        extract["entities"] = [{"name": "Solo", "source_chunk_id": packed[0]["label"]}]
        return extract

    monkeypatch.setattr(wiki, "_wiki_extract_one_batch", _extract)

    chunk_ids = [f"chunk-{idx}" for idx in range(4)]
    hashes = {chunk_id: f"hash-{chunk_id}" for chunk_id in chunk_ids}

    merged = asyncio.run(
        wiki._wiki_process_batch(
            packed=_packed(chunk_ids),
            batch_idx=0,
            total_batches=1,
            doc_id="doc-1",
            tenant_id="tenant-1",
            kb_id="kb-1",
            chat_mdl=SimpleNamespace(max_length=8192, llm_name="mock-chat"),
            language="en",
            llm_timeout=1,
            semaphore=None,
            callback=None,
            chunk_hashes=hashes,
        )
    )

    assert sorted(seen_sizes) == [2, 2, 4]
    assert len(merged["entities"]) == 0
    assert store.insert_calls == []

    # One more split level: the halves are re-tried down to single chunks and
    # every chunk is extracted and persisted.
    monkeypatch.setattr(wiki, "DEFAULT_WIKI_MAP_TIMEOUT_SPLIT_DEPTH", 2)
    seen_sizes.clear()
    merged = asyncio.run(
        wiki._wiki_process_batch(
            packed=_packed(chunk_ids),
            batch_idx=0,
            total_batches=1,
            doc_id="doc-1",
            tenant_id="tenant-1",
            kb_id="kb-1",
            chat_mdl=SimpleNamespace(max_length=8192, llm_name="mock-chat"),
            language="en",
            llm_timeout=1,
            semaphore=None,
            callback=None,
            chunk_hashes=hashes,
        )
    )

    assert sorted(seen_sizes) == [1, 1, 1, 1, 2, 2, 4]
    assert len(merged["entities"]) == 4
    written = [row for batch in store.insert_calls for row in batch]
    assert {row["source_chunk_ids"][0] for row in written} == set(chunk_ids)


def test_split_retry_can_be_disabled(monkeypatch):
    """With the split depth at 0 a timed-out batch is abandoned untouched.

    The chunks must stay unpersisted so the next incremental run re-MAPs
    them instead of treating the batch as a cached empty extraction.
    """
    wiki = _load_wiki_module(monkeypatch)
    monkeypatch.setattr(wiki, "DEFAULT_WIKI_MAP_TIMEOUT_SPLIT_DEPTH", 0)
    store = _KbScopedDocStore()
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    seen_sizes: list[int] = []

    async def _timeout(packed, *_args, **_kwargs):
        seen_sizes.append(len(packed))
        raise wiki._WikiMapBatchTimeout("timed out")

    monkeypatch.setattr(wiki, "_wiki_extract_one_batch", _timeout)

    chunk_ids = [f"chunk-{idx}" for idx in range(4)]
    merged = asyncio.run(
        wiki._wiki_process_batch(
            packed=_packed(chunk_ids),
            batch_idx=0,
            total_batches=1,
            doc_id="doc-1",
            tenant_id="tenant-1",
            kb_id="kb-1",
            chat_mdl=SimpleNamespace(max_length=8192, llm_name="mock-chat"),
            language="en",
            llm_timeout=1,
            semaphore=None,
            callback=None,
            chunk_hashes={chunk_id: f"hash-{chunk_id}" for chunk_id in chunk_ids},
        )
    )

    assert seen_sizes == [4]
    assert merged == wiki._wiki_empty_extract()
    assert store.insert_calls == []


def test_llm_error_still_returns_none_without_split_retry(monkeypatch):
    """A non-timeout failure is not retried: retrying would double the load
    on an already broken model."""
    wiki = _load_wiki_module(monkeypatch)
    store = _KbScopedDocStore()
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    seen_sizes: list[int] = []

    async def _fail(packed, *_args, **_kwargs):
        seen_sizes.append(len(packed))

    monkeypatch.setattr(wiki, "_wiki_extract_one_batch", _fail)

    chunk_ids = [f"chunk-{idx}" for idx in range(4)]
    merged = asyncio.run(
        wiki._wiki_process_batch(
            packed=_packed(chunk_ids),
            batch_idx=0,
            total_batches=1,
            doc_id="doc-1",
            tenant_id="tenant-1",
            kb_id="kb-1",
            chat_mdl=SimpleNamespace(max_length=8192, llm_name="mock-chat"),
            language="en",
            llm_timeout=1,
            semaphore=None,
            callback=None,
            chunk_hashes={chunk_id: f"hash-{chunk_id}" for chunk_id in chunk_ids},
        )
    )

    assert seen_sizes == [4]
    assert merged == wiki._wiki_empty_extract()
    assert store.insert_calls == []


class _ListValuedKwdDocStore(_StateDocStore):
    """Mirrors Infinity/Vastbase ``get_fields``: ``*_kwd`` columns come back as lists.

    Those backends split every ``*_kwd`` value on read, so ``chunk_hash_kwd``
    arrives as ``["<hash>"]`` while ES/OpenSearch hand back ``"<hash>"``. Both
    shapes must resolve to the same cached version.
    """

    def get_fields(self, result, fields):
        mapped = {}
        for row in result:
            values = {}
            for field in fields:
                value = row.get(field)
                if field.endswith("_kwd") and isinstance(value, str):
                    value = [value] if value else []
                values[field] = value
            mapped[row["id"]] = values
        return mapped


def test_map_versions_resolve_when_chunk_hash_comes_back_as_list(monkeypatch):
    """Regression: Infinity returns ``chunk_hash_kwd`` as a one-element list.

    Requiring a plain string dropped every row, so MAP extraction reported
    ``cache_hits=0`` forever and then failed its own resolution check with
    "MAP extraction/cache resolution incomplete ... missing_chunks=[...]".
    """
    wiki = _load_wiki_module(monkeypatch)
    store = _ListValuedKwdDocStore(
        [
            {
                "id": "wanted",
                "doc_id": "doc-1",
                "compile_kwd": wiki.WIKI_MAP_COMPILE_KWD,
                "source_chunk_ids": ["chunk-1"],
                "chunk_hash_kwd": "hash-b",
                "content_with_weight": json.dumps({"entities": [{"name": "B"}]}),
            }
        ]
    )
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    versions = asyncio.run(wiki._wiki_load_map_versions("doc-1", "tenant-1", "kb-1", {"chunk-1": "hash-b"}))
    assert set(versions["chunk-1"]) == {"hash-b"}

    state = {"chunk-1": {"doc_id": "doc-1", "hash": "hash-b"}}
    assert _missing_after_resolve(wiki, "tenant-1", "kb-1", state, {"chunk-1"}) == set()


def test_active_map_state_reads_list_valued_chunk_hash(monkeypatch):
    """The incremental baseline must restore too, otherwise every run re-MAPs
    the whole KB instead of only the changed chunks."""
    wiki = _load_wiki_module(monkeypatch)
    marker_id = wiki._stable_row_id(wiki.WIKI_MAP_STATE_META_COMPILE_KWD, "kb-1")
    store = _ListValuedKwdDocStore(
        [
            {
                "id": marker_id,
                "compile_kwd": wiki.WIKI_MAP_STATE_META_COMPILE_KWD,
                "type_kwd": "gen-1",
            },
            {
                "id": "state-row",
                "doc_id": "doc-1",
                "compile_kwd": wiki.WIKI_MAP_STATE_COMPILE_KWD,
                "type_kwd": "gen-1",
                "source_chunk_ids": ["chunk-1"],
                "chunk_hash_kwd": "hash-1",
            },
        ]
    )
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    assert asyncio.run(wiki._wiki_load_active_map_state("tenant-1", "kb-1")) == {"chunk-1": {"doc_id": "doc-1", "hash": "hash-1"}}


def test_provider_error_is_not_cached_as_empty_extract(monkeypatch):
    """A provider failure raised by ``gen_json`` must not be persisted as an extract.

    Persisting it marked every chunk in the batch as extracted, so all later runs
    skipped them and the wiki stayed empty.
    """
    wiki = _load_wiki_module(monkeypatch)

    async def _error_gen(*args, **kwargs):
        raise RuntimeError("**ERROR**: MODEL_ERROR - litellm.NotFoundError: OpenAIException - 404")

    monkeypatch.setattr(wiki, "gen_json", _error_gen)
    store = _StateDocStore([])
    monkeypatch.setattr(sys.modules["common.settings"], "docStoreConn", store)

    merged = asyncio.run(
        wiki._wiki_process_batch(
            packed=_packed(["chunk-1", "chunk-2"]),
            batch_idx=0,
            total_batches=1,
            doc_id="doc-1",
            tenant_id="tenant-1",
            kb_id="kb-1",
            chat_mdl=SimpleNamespace(max_length=8192, llm_name="mock-chat"),
            language="en",
            llm_timeout=1,
            semaphore=None,
            callback=None,
            chunk_hashes={"chunk-1": "hash-1", "chunk-2": "hash-2"},
        )
    )

    assert merged == wiki._wiki_empty_extract()
    assert store.insert_calls == []
