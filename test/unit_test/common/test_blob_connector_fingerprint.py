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
"""Tests for the FingerprintConnector bypass path in BlobStorageConnector."""

import importlib.util
import sys
from datetime import datetime, timezone
from pathlib import Path
from types import ModuleType

import pytest
import xxhash


def _load_blob_connector_module():
    repo_root = Path(__file__).resolve().parents[3]
    package_name = "common.data_source"
    saved_modules = {name: module for name, module in sys.modules.items() if name == package_name or name.startswith(f"{package_name}.")}
    package_stub = ModuleType(package_name)
    package_stub.__path__ = [str(repo_root / "common" / "data_source")]
    sys.modules[package_name] = package_stub

    try:
        spec = importlib.util.spec_from_file_location(
            "_blob_connector_under_test",
            repo_root / "common" / "data_source" / "blob_connector.py",
        )
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module
    finally:
        for name in list(sys.modules):
            if name == package_name or name.startswith(f"{package_name}."):
                if name in saved_modules:
                    sys.modules[name] = saved_modules[name]
                else:
                    sys.modules.pop(name, None)


blob_connector = _load_blob_connector_module()
BlobStorageConnector = blob_connector.BlobStorageConnector
_normalize_etag = blob_connector._normalize_etag


# ---------------------------------------------------------------------------
# Fake S3 client wired through a paginator-style interface.
# ---------------------------------------------------------------------------


class _FakePaginator:
    def __init__(self, pages: list[dict]) -> None:
        self._pages = pages

    def paginate(self, **_kwargs):
        for page in self._pages:
            yield page


class _FakeS3Client:
    """Captures every call on the connector's S3 client.

    Tests assert against `get_object_calls` to verify that the fingerprint
    bypass actually skips downloads when ETags haven't changed.
    """

    def __init__(self, objects: list[dict]) -> None:
        self._objects = objects
        self.get_object_calls: list[tuple[str, str]] = []
        # Hand objects to the paginator unmodified so the connector exercises
        # its own directory-placeholder filtering logic.
        self._paginator = _FakePaginator([{"Contents": list(objects)}])

    def get_paginator(self, name: str):
        assert name == "list_objects_v2"
        return self._paginator

    def list_objects_v2(self, **_kwargs):
        return {"Contents": self._objects, "KeyCount": len(self._objects)}

    def get_object(self, Bucket: str, Key: str):  # noqa: N803  (boto3 API)
        self.get_object_calls.append((Bucket, Key))
        body_text = f"body-of-{Key}".encode()
        return {
            "Body": _FakeBody(body_text),
            "ContentLength": len(body_text),
        }


class _FakeBody:
    """Minimal stand-in for botocore's StreamingBody.

    The real downloader (common.data_source.utils.download_object) consumes
    the body via iter_chunks() and then calls close(); fake out both.
    """

    def __init__(self, payload: bytes) -> None:
        self._payload = payload

    def read(self) -> bytes:
        return self._payload

    def iter_chunks(self, chunk_size: int = 65536):
        for i in range(0, len(self._payload), chunk_size):
            yield self._payload[i : i + chunk_size]

    def close(self) -> None:
        return None


def _make_connector(s3_client) -> BlobStorageConnector:
    connector = BlobStorageConnector(bucket_type="s3", bucket_name="test-bucket")
    connector.s3_client = s3_client
    return connector


def _s3_object(key: str, etag: str, size: int = 12) -> dict:
    return {
        "Key": key,
        "ETag": f'"{etag}"',
        "LastModified": datetime(2026, 1, 1, 12, tzinfo=timezone.utc),
        "Size": size,
    }


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


def test_normalize_etag_returns_32_char_hex_for_singlepart_etag():
    fp = _normalize_etag('"d41d8cd98f00b204e9800998ecf8427e"')
    assert fp is not None
    assert len(fp) == 32
    assert all(c in "0123456789abcdef" for c in fp)


def test_normalize_etag_returns_32_char_hex_for_multipart_etag():
    """Multipart ETags are 34+ chars; hashing normalizes them to 32."""
    fp = _normalize_etag('"d41d8cd98f00b204e9800998ecf8427e-7"')
    assert fp is not None
    assert len(fp) == 32


def test_normalize_etag_is_deterministic():
    raw = '"abc123def456abc123def456abc123de"'
    assert _normalize_etag(raw) == _normalize_etag(raw)


def test_normalize_etag_strips_quotes_so_quoted_and_unquoted_match():
    quoted = '"d41d8cd98f00b204e9800998ecf8427e"'
    unquoted = "d41d8cd98f00b204e9800998ecf8427e"
    assert _normalize_etag(quoted) == _normalize_etag(unquoted)


def test_normalize_etag_returns_none_for_empty_input():
    assert _normalize_etag("") is None
    assert _normalize_etag(None) is None


def test_list_keys_yields_one_keyrecord_per_object_with_fingerprint():
    s3 = _FakeS3Client(
        [
            _s3_object("foo.txt", "etag-foo"),
            _s3_object("bar/baz.txt", "etag-baz"),
        ]
    )
    connector = _make_connector(s3)

    records = list(connector.list_keys())

    assert len(records) == 2
    assert {r.key for r in records} == {
        "BlobType.S3:test-bucket:foo.txt",
        "BlobType.S3:test-bucket:bar/baz.txt",
    }
    for record in records:
        assert record.fingerprint is not None
        assert len(record.fingerprint) == 32
        assert record.deleted is False


def test_list_keys_does_not_call_get_object():
    """list_keys() must be cheap -- no body downloads during enumeration."""
    s3 = _FakeS3Client([_s3_object("foo.txt", "etag-foo")])
    connector = _make_connector(s3)

    list(connector.list_keys())

    assert s3.get_object_calls == []


def test_list_keys_skips_directory_placeholder_keys():
    """S3 'folders' are zero-byte keys ending in '/'; they shouldn't yield records."""
    s3 = _FakeS3Client(
        [
            _s3_object("real-file.txt", "etag-real"),
            _s3_object("folder/", "etag-folder"),
        ]
    )
    connector = _make_connector(s3)

    keys = [r.key for r in connector.list_keys()]

    assert keys == ["BlobType.S3:test-bucket:real-file.txt"]


def test_get_value_returns_document_with_fingerprint_set():
    s3 = _FakeS3Client([_s3_object("foo.txt", "etag-foo")])
    connector = _make_connector(s3)
    [record] = list(connector.list_keys())

    doc = connector.get_value(record.key)

    assert doc.id == "BlobType.S3:test-bucket:foo.txt"
    assert doc.fingerprint == record.fingerprint
    assert doc.fingerprint == xxhash.xxh128(b"etag-foo").hexdigest()


def test_get_value_calls_get_object_exactly_once_per_key():
    s3 = _FakeS3Client([_s3_object("foo.txt", "etag-foo")])
    connector = _make_connector(s3)
    [record] = list(connector.list_keys())

    connector.get_value(record.key)

    assert s3.get_object_calls == [("test-bucket", "foo.txt")]


def test_get_value_raises_keyerror_when_called_before_list_keys():
    s3 = _FakeS3Client([_s3_object("foo.txt", "etag-foo")])
    connector = _make_connector(s3)

    with pytest.raises(KeyError):
        connector.get_value("BlobType.S3:test-bucket:foo.txt")


def test_singlepart_and_multipart_etags_yield_different_fingerprints():
    """Sanity: distinct ETags must produce distinct fingerprints."""
    s3 = _FakeS3Client(
        [
            _s3_object("a.bin", "d41d8cd98f00b204e9800998ecf8427e"),
            _s3_object("b.bin", "d41d8cd98f00b204e9800998ecf8427e-3"),
        ]
    )
    connector = _make_connector(s3)

    records = list(connector.list_keys())

    assert records[0].fingerprint != records[1].fingerprint


def test_fingerprint_stable_across_repeated_listings():
    """Same ETag in two list_keys() calls yields the same fingerprint."""
    s3 = _FakeS3Client([_s3_object("foo.txt", "etag-stable")])
    connector = _make_connector(s3)

    fp_first = next(connector.list_keys()).fingerprint
    fp_second = next(connector.list_keys()).fingerprint

    assert fp_first == fp_second


# ---------------------------------------------------------------------------
# Bypass-logic test: simulates what the orchestrator does in
# _BlobLikeBase._fingerprint_filtered_generator. Verifies that a key whose
# fingerprint matches the persisted content_hash is NOT fetched.
# ---------------------------------------------------------------------------


def test_orchestrator_pattern_skips_get_object_when_fingerprint_matches():
    # Use distinct base names: "unchanged.txt".endswith("changed.txt") is True,
    # which would silently break endswith-based lookups in the test setup.
    s3 = _FakeS3Client(
        [
            _s3_object("static.txt", "etag-static"),
            _s3_object("modified.txt", "etag-modified"),
        ]
    )
    connector = _make_connector(s3)

    # Pre-compute the fingerprints the connector would emit, then pretend the
    # DB already stores the one for static.txt but a stale value for
    # modified.txt. This is the steady-state bypass scenario.
    listed = list(connector.list_keys())
    static_record = next(r for r in listed if r.key.endswith(":static.txt"))
    modified_record = next(r for r in listed if r.key.endswith(":modified.txt"))
    persisted = {
        static_record.key: static_record.fingerprint,
        modified_record.key: "stale-fingerprint",
    }

    # Reset the call log so we only count get_object during the bypass loop.
    s3.get_object_calls = []

    fetched = []
    for record in connector.list_keys():
        if record.fingerprint and persisted.get(record.key) == record.fingerprint:
            continue
        fetched.append(connector.get_value(record.key))

    assert [doc.id for doc in fetched] == ["BlobType.S3:test-bucket:modified.txt"]
    assert s3.get_object_calls == [("test-bucket", "modified.txt")]


def test_orchestrator_pattern_skips_deleted_records_without_calling_get_value():
    """KeyRecord(deleted=True) must short-circuit before get_value().

    Reach KeyRecord through the already-loaded blob_connector module to avoid
    triggering common.data_source.__init__'s circular imports.
    """
    KeyRecord = blob_connector.KeyRecord

    s3 = _FakeS3Client([_s3_object("foo.txt", "etag-foo")])
    connector = _make_connector(s3)

    # Manually feed a deleted KeyRecord through the bypass logic to assert the
    # short-circuit holds even when a connector emits one. (BlobStorageConnector
    # itself doesn't yield deleted records yet -- that's PR-4 -- but the
    # orchestrator must already be defensive.)
    deleted_record = KeyRecord(
        key="BlobType.S3:test-bucket:gone.txt",
        fingerprint=None,
        deleted=True,
    )

    # Mirror the orchestrator's loop body verbatim.
    fetched = []
    for record in [deleted_record]:
        if record.deleted:
            continue
        fetched.append(connector.get_value(record.key))

    assert fetched == []
    assert s3.get_object_calls == []
