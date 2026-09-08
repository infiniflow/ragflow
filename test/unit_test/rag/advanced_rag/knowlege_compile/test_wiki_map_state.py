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
