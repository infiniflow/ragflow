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
