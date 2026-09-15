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

"""Unit tests for RAGFlowS3.remove_bucket (issue: S3 backend could not delete buckets)."""

import importlib
import logging
from unittest.mock import Mock

import pytest
from botocore.exceptions import ClientError

# Import common.settings first: s3_conn <-> settings import cycle resolves
# only in the same order the app uses (see test_s3_conn.py).
import common.settings  # noqa: F401
from rag.utils import s3_conn


def _new_storage(monkeypatch, config):
    module = importlib.reload(s3_conn)
    client = Mock()
    monkeypatch.setattr(module.settings, "S3", config)
    monkeypatch.setattr(module.boto3, "client", Mock(return_value=client))
    return module.RAGFlowS3(), client


def _paginator(client, kind, *pages, by_prefix=None):
    """Stub one kind of list paginator. ``by_prefix`` maps a Prefix kwarg to
    its pages so single-bucket mode sees realistic prefix-filtered results.
    delete_objects defaults to an all-deleted response; tests override it."""
    paginator = Mock()

    def paginate(**kwargs):
        if by_prefix is not None:
            return list(by_prefix.get(kwargs.get("Prefix"), []))
        return list(pages)

    paginator.paginate.side_effect = paginate
    client.get_paginator.side_effect_map[kind] = paginator
    return paginator


def _wire(client):
    """Prepare a client mock with per-kind paginators and safe defaults."""
    client.get_paginator.side_effect_map = {}
    client.get_paginator.side_effect = lambda kind: client.get_paginator.side_effect_map.setdefault(kind, _default_paginator(kind))
    client.delete_objects.return_value = {"Deleted": []}


def _default_paginator(kind):
    """Empty-pages paginator for kinds a test did not register explicitly."""
    paginator = Mock()
    paginator.paginate.side_effect = lambda **kwargs: [{}]
    return paginator


def test_remove_bucket_method_matches_storage_contract(monkeypatch):
    """Callers use remove_bucket (kb/user-account deletion); it must exist."""
    storage, _ = _new_storage(monkeypatch, {})
    assert hasattr(storage, "remove_bucket")
    assert not hasattr(storage, "rm_bucket")


def test_remove_bucket_deletes_objects_then_bucket(monkeypatch):
    """Multi-bucket mode: all object versions are deleted in batches, then the
    bucket itself."""
    storage, client = _new_storage(monkeypatch, {})
    _wire(client)
    _paginator(
        client,
        "list_object_versions",
        {"Versions": [{"Key": "a"}, {"Key": "b"}], "DeleteMarkers": []},
        {"Versions": [{"Key": "c"}], "DeleteMarkers": []},
    )
    client.head_bucket.return_value = {}

    storage.remove_bucket("kb01")

    client.delete_objects.assert_any_call(Bucket="kb01", Delete={"Objects": [{"Key": "a"}, {"Key": "b"}]})
    client.delete_objects.assert_any_call(Bucket="kb01", Delete={"Objects": [{"Key": "c"}]})
    client.delete_bucket.assert_called_once_with(Bucket="kb01")


def test_remove_bucket_deletes_versions_and_markers_with_version_ids(monkeypatch):
    """Versioned buckets: every version and delete marker must be deleted with
    its VersionId, otherwise delete_bucket fails with BucketNotEmpty."""
    storage, client = _new_storage(monkeypatch, {})
    _wire(client)
    _paginator(
        client,
        "list_object_versions",
        {
            "Versions": [{"Key": "a", "VersionId": "v1"}, {"Key": "a", "VersionId": "v2"}],
            "DeleteMarkers": [{"Key": "b", "VersionId": "m1"}],
        },
    )
    client.head_bucket.return_value = {}

    storage.remove_bucket("kb01")

    client.delete_objects.assert_called_once_with(
        Bucket="kb01",
        Delete={
            "Objects": [
                {"Key": "a", "VersionId": "v1"},
                {"Key": "a", "VersionId": "v2"},
                {"Key": "b", "VersionId": "m1"},
            ]
        },
    )
    client.delete_bucket.assert_called_once_with(Bucket="kb01")


def test_remove_bucket_skips_missing_bucket(monkeypatch):
    """A 404 from head_bucket means the bucket is absent: a clean no-op."""
    storage, client = _new_storage(monkeypatch, {})
    _wire(client)
    client.head_bucket.side_effect = ClientError(
        {"Error": {"Code": "404"}},
        "HeadBucket",
    )

    storage.remove_bucket("nope")

    client.get_paginator.assert_not_called()
    client.delete_objects.assert_not_called()
    client.delete_bucket.assert_not_called()


def test_remove_bucket_propagates_head_bucket_errors(monkeypatch):
    """403/400 from head_bucket are access failures, not absence: they must
    reach the error handler, not silently skip cleanup."""
    storage, client = _new_storage(monkeypatch, {})
    _wire(client)
    client.head_bucket.side_effect = ClientError(
        {"Error": {"Code": "403", "Message": "Forbidden"}},
        "HeadBucket",
    )

    with pytest.raises(ClientError):
        storage.remove_bucket("kb01")

    client.get_paginator.assert_not_called()
    client.delete_bucket.assert_not_called()


def test_remove_bucket_raises_on_batch_delete_errors(monkeypatch):
    """delete_objects reports failed keys in its response instead of raising;
    remove_bucket must propagate the failure to lifecycle callers."""
    storage, client = _new_storage(monkeypatch, {})
    _wire(client)
    _paginator(
        client,
        "list_object_versions",
        {"Versions": [{"Key": "a"}], "DeleteMarkers": []},
        {"Versions": [{"Key": "c"}], "DeleteMarkers": []},
    )
    client.delete_objects.side_effect = [
        {"Deleted": [{"Key": "a"}]},
        {"Errors": [{"Key": "c", "Code": "InternalError", "Message": "boom"}]},
    ]
    client.head_bucket.return_value = {}

    with pytest.raises(RuntimeError, match="S3 object deletion failed for keys: c"):
        storage.remove_bucket("kb01")

    client.delete_bucket.assert_not_called()


def test_remove_bucket_handles_null_error_response(monkeypatch, caplog):
    """botocore HTTPClientError can carry response=None; the failure path must
    complete without AttributeError and still log the failure."""
    storage, client = _new_storage(monkeypatch, {})
    _wire(client)
    err = ClientError({"Error": {}}, "HeadBucket")
    err.response = None  # HTTPClientError can surface with a null response
    client.head_bucket.side_effect = err

    with caplog.at_level(logging.ERROR), pytest.raises(ClientError):
        storage.remove_bucket("kb01")

    assert any("Fail to remove bucket kb01" in r.getMessage() for r in caplog.records)


def test_remove_bucket_re_raises_to_callers(monkeypatch, caplog):
    """Lifecycle callers delete metadata after remove_bucket returns; a
    storage failure must propagate, not report silent success."""
    storage, client = _new_storage(monkeypatch, {})
    _wire(client)
    client.head_bucket.side_effect = ConnectionError("network down")

    with caplog.at_level(logging.ERROR), pytest.raises(ConnectionError):
        storage.remove_bucket("kb01")

    messages = [r.getMessage() for r in caplog.records]
    assert any("Fail to remove bucket kb01" in m for m in messages), messages
    for m in messages:
        assert "X-Amz-Signature" not in m


def test_remove_bucket_refuses_shared_bucket_without_prefix_path(monkeypatch, caplog):
    """A shared physical bucket without prefix_path hosts keys without a
    logical-bucket segment; no cleanup prefix could scope the deletion, so it
    must refuse rather than wipe the whole bucket."""
    storage, client = _new_storage(monkeypatch, {"bucket": "ragflow"})
    _wire(client)

    with caplog.at_level(logging.ERROR):
        storage.remove_bucket("kb01")

    client.get_paginator.assert_not_called()
    client.delete_bucket.assert_not_called()
    assert any("Refusing to remove logical bucket kb01" in r.getMessage() for r in caplog.records)


def test_remove_bucket_single_bucket_mode_with_prefix_path(monkeypatch):
    storage, client = _new_storage(monkeypatch, {"bucket": "ragflow", "prefix_path": "prod"})
    _wire(client)
    versions = _paginator(
        client,
        "list_object_versions",
        by_prefix={"prod/kb01/": [{"Versions": [{"Key": "prod/kb01/doc"}], "DeleteMarkers": []}]},
    )
    _paginator(client, "list_multipart_uploads", by_prefix={"prod/kb01/": []})

    storage.remove_bucket("kb01")

    versions.paginate.assert_called_once_with(Bucket="ragflow", Prefix="prod/kb01/")
    deleted = client.delete_objects.call_args
    keys = [o["Key"] for o in deleted.kwargs["Delete"]["Objects"]]
    assert keys == ["prod/kb01/doc"]
    client.delete_bucket.assert_not_called()


def test_remove_bucket_empty_bucket_deletes_only_bucket(monkeypatch):
    """A bucket that exists but has no objects: only delete_bucket fires."""
    storage, client = _new_storage(monkeypatch, {})
    _wire(client)
    _paginator(client, "list_object_versions", {})
    _paginator(client, "list_multipart_uploads", {})
    client.head_bucket.return_value = {}

    storage.remove_bucket("kb01")

    client.delete_objects.assert_not_called()
    client.delete_bucket.assert_called_once_with(Bucket="kb01")
