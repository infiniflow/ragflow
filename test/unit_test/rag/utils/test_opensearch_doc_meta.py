#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
Unit tests for the document-metadata helpers added to OSConnection.

Covers issue #14570: PATCH /api/v1/datasets/{ds}/documents/{doc} with
{"meta_fields": {...}} previously raised
``'OSConnection' object has no attribute 'create_doc_meta_idx'`` when the
backend was OpenSearch. These tests pin the new dispatch surface so the same
regression cannot return: every helper that DocMetadataService dispatches to
on the ES path must exist on OSConnection too, with semantically equivalent
behaviour.

The OpenSearch and Elasticsearch SDKs are imported at module load; mocking
the underlying client lets us exercise OSConnection methods in isolation
without a live cluster.
"""
from __future__ import annotations

import sys
import types
from unittest.mock import MagicMock, patch

import pytest


# Importing OSConnection touches opensearchpy at module load, so guard for
# environments where the package isn't installed.
opensearchpy = pytest.importorskip("opensearchpy")


def _install_module(name: str, **attrs) -> types.ModuleType:
    mod = sys.modules.get(name)
    if mod is None:
        mod = types.ModuleType(name)
        sys.modules[name] = mod
    for key, value in attrs.items():
        if not hasattr(mod, key):
            setattr(mod, key, value)
    return mod


def _install_module_stubs() -> None:
    """Bypass heavy optional backends for connection-only tests.

    ``rag.utils.opensearch_conn`` imports ``common.settings`` and ``rag.nlp``
    at module load. ``common.settings`` in turn pulls every storage backend
    (Infinity, OceanBase, Azure, MinIO, GCS …), which is more surface than
    these connection-only tests need. We replace just the modules opensearch_conn
    captures so the real ``OSConnection`` class loads.
    """
    _install_module(
        "common.settings",
        OS={"hosts": "stub", "username": "u", "password": "p"},
        ES={},
        DOC_ENGINE_INFINITY=False,
        DOC_ENGINE_OCEANBASE=False,
        DOC_ENGINE="opensearch",
        docStoreConn=None,
    )
    _install_module(
        "rag.nlp",
        is_english=lambda *_args, **_kwargs: False,
        rag_tokenizer=MagicMock(),
    )


_install_module_stubs()


class _FakeFile:
    """Minimal file-like stand-in supporting ``json.load``."""

    def __init__(self, content: str) -> None:
        self._content = content

    def read(self, *_args, **_kwargs) -> str:
        return self._content


def _open_returning_payload(payload: dict):
    """Build a context-manager mock for ``open`` that yields the JSON payload."""
    import json as _json

    fake_handle = MagicMock()
    fake_handle.__enter__ = MagicMock(return_value=_FakeFile(_json.dumps(payload)))
    fake_handle.__exit__ = MagicMock(return_value=False)
    return MagicMock(return_value=fake_handle)


def _resolve_os_connection_class():
    """Return the real OSConnection class.

    ``@singleton`` from ``common.decorator`` wraps the class with a closure
    that returns the cached instance on call. ``OSConnection`` at module
    scope is therefore a function, not a type. We unwrap it to recover the
    underlying class so we can call ``__new__`` directly without going through
    ``__init__`` (which would attempt a real OpenSearch handshake).
    """
    from rag.utils import opensearch_conn

    candidate = opensearch_conn.OSConnection
    if isinstance(candidate, type):
        return candidate
    closure = getattr(candidate, "__closure__", None) or ()
    for cell in closure:
        contents = cell.cell_contents
        if isinstance(contents, type):
            return contents
    raise RuntimeError("Could not locate the OSConnection class in module scope")


def _make_os_connection():
    """Build an OSConnection without invoking its real network-dependent __init__."""
    cls = _resolve_os_connection_class()
    instance = cls.__new__(cls)
    instance.os = MagicMock()
    instance.info = {"version": {"number": "2.18.0"}}
    instance.mapping = {"settings": {}, "mappings": {}}
    return instance


class TestOSConnectionMetaSurface:
    """The OSConnection class must expose the dispatch surface
    DocMetadataService relies on."""

    def test_create_doc_meta_idx_exists(self):
        cls = _resolve_os_connection_class()
        assert callable(getattr(cls, "create_doc_meta_idx", None)), (
            "OSConnection.create_doc_meta_idx is required so the metadata "
            "PATCH path does not raise AttributeError on OpenSearch backends "
            "(issue #14570)."
        )

    def test_refresh_idx_exists(self):
        cls = _resolve_os_connection_class()
        assert callable(getattr(cls, "refresh_idx", None))

    def test_count_idx_exists(self):
        cls = _resolve_os_connection_class()
        assert callable(getattr(cls, "count_idx", None))

    def test_replace_meta_fields_exists(self):
        cls = _resolve_os_connection_class()
        assert callable(getattr(cls, "replace_meta_fields", None))


class TestCreateDocMetaIdx:
    """Behavioural tests for OSConnection.create_doc_meta_idx."""

    def test_returns_true_when_index_already_exists(self):
        conn = _make_os_connection()
        with patch.object(_resolve_os_connection_class(), "index_exist", return_value=True) as exist:
            assert conn.create_doc_meta_idx("ragflow_doc_meta_t1") is True
        exist.assert_called_once_with("ragflow_doc_meta_t1", "")

    def test_creates_index_with_doc_meta_mapping(self):
        conn = _make_os_connection()
        fake_indices = MagicMock()
        fake_indices.create.return_value = {"acknowledged": True}
        cls = _resolve_os_connection_class()

        with patch.object(cls, "index_exist", return_value=False), \
             patch("rag.utils.opensearch_conn.os.path.exists", return_value=True), \
             patch(
                 "rag.utils.opensearch_conn.open",
                 new=_open_returning_payload({
                     "settings": {"index": {"number_of_shards": 2}},
                     "mappings": {"properties": {"meta_fields": {"type": "object"}}},
                 }),
                 create=True,
             ), \
             patch("opensearchpy.client.IndicesClient", return_value=fake_indices):
            result = conn.create_doc_meta_idx("ragflow_doc_meta_t1")

        assert result == {"acknowledged": True}
        fake_indices.create.assert_called_once()
        kwargs = fake_indices.create.call_args.kwargs
        assert kwargs["index"] == "ragflow_doc_meta_t1"
        body = kwargs["body"]
        assert "settings" in body and "mappings" in body
        assert body["mappings"]["properties"]["meta_fields"]["type"] == "object"

    def test_returns_false_when_mapping_file_missing(self):
        conn = _make_os_connection()
        cls = _resolve_os_connection_class()
        with patch.object(cls, "index_exist", return_value=False), \
             patch("rag.utils.opensearch_conn.os.path.exists", return_value=False):
            assert conn.create_doc_meta_idx("ragflow_doc_meta_t1") is False

    def test_returns_false_when_create_call_explodes(self):
        """If the underlying IndicesClient.create raises, the helper must
        swallow the exception and return False so the service layer can fall
        back gracefully (mirrors ESConnectionBase.create_doc_meta_idx)."""
        conn = _make_os_connection()
        cls = _resolve_os_connection_class()
        fake_indices = MagicMock()
        fake_indices.create.side_effect = RuntimeError("opensearch unreachable")

        with patch.object(cls, "index_exist", return_value=False), \
             patch("rag.utils.opensearch_conn.os.path.exists", return_value=True), \
             patch(
                 "rag.utils.opensearch_conn.open",
                 new=_open_returning_payload({"settings": {}, "mappings": {}}),
                 create=True,
             ), \
             patch("opensearchpy.client.IndicesClient", return_value=fake_indices):
            assert conn.create_doc_meta_idx("ragflow_doc_meta_t1") is False


class TestRefreshIdx:
    def test_calls_indices_refresh(self):
        conn = _make_os_connection()
        assert conn.refresh_idx("ragflow_doc_meta_t1") is True
        conn.os.indices.refresh.assert_called_once_with(index="ragflow_doc_meta_t1")

    def test_returns_false_on_not_found(self):
        conn = _make_os_connection()
        conn.os.indices.refresh.side_effect = opensearchpy.NotFoundError(
            404, "index_not_found_exception", {}
        )
        assert conn.refresh_idx("missing_idx") is False

    def test_swallows_other_errors_and_returns_false(self):
        conn = _make_os_connection()
        conn.os.indices.refresh.side_effect = RuntimeError("transient")
        assert conn.refresh_idx("ragflow_doc_meta_t1") is False


class TestCountIdx:
    def test_returns_count_value(self):
        conn = _make_os_connection()
        conn.os.count.return_value = {"count": 42}
        assert conn.count_idx("ragflow_doc_meta_t1") == 42
        conn.os.count.assert_called_once_with(index="ragflow_doc_meta_t1")

    def test_missing_index_reads_as_zero(self):
        conn = _make_os_connection()
        conn.os.count.side_effect = opensearchpy.NotFoundError(
            404, "index_not_found_exception", {}
        )
        assert conn.count_idx("ragflow_doc_meta_t1") == 0

    def test_other_failure_returns_negative_one(self):
        conn = _make_os_connection()
        conn.os.count.side_effect = RuntimeError("bad")
        assert conn.count_idx("ragflow_doc_meta_t1") == -1


class TestReplaceMetaFields:
    def test_emits_full_assignment_script(self):
        conn = _make_os_connection()
        conn.os.update.return_value = {"_id": "doc-1", "result": "updated"}
        meta = {"author": "alice", "year": 2026}

        ok = conn.replace_meta_fields("ragflow_doc_meta_t1", "doc-1", meta)

        assert ok is True
        conn.os.update.assert_called_once()
        kwargs = conn.os.update.call_args.kwargs
        assert kwargs["index"] == "ragflow_doc_meta_t1"
        assert kwargs["id"] == "doc-1"
        assert kwargs["refresh"] is True
        body = kwargs["body"]
        # The script must fully assign meta_fields, otherwise removed keys
        # would persist via deep merge.
        assert body["script"]["source"] == "ctx._source.meta_fields = params.meta_fields"
        assert body["script"]["params"]["meta_fields"] == meta

    def test_returns_false_when_doc_missing(self):
        conn = _make_os_connection()
        conn.os.update.side_effect = opensearchpy.NotFoundError(
            404, "document_missing_exception", {}
        )
        assert conn.replace_meta_fields("ragflow_doc_meta_t1", "absent", {"a": 1}) is False
