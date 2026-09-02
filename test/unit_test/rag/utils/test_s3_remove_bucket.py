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


def _pages(client, *pages, by_prefix=None):
    """Stub the list_objects_v2 paginator. ``by_prefix`` maps a Prefix kwarg to
    its pages so single-bucket mode sees realistic prefix-filtered results.
    delete_objects defaults to an all-deleted response; tests override it."""
    paginator = Mock()

    def paginate(**kwargs):
        if by_prefix is not None:
            return list(by_prefix.get(kwargs.get("Prefix"), []))
        return list(pages)

    paginator.paginate.side_effect = paginate
    client.get_paginator.return_value = paginator
    client.delete_objects.return_value = {"Deleted": []}


def test_remove_bucket_method_matches_storage_contract(monkeypatch):
    """Callers use remove_bucket (kb/user-account deletion); it must exist."""
    storage, _ = _new_storage(monkeypatch, {})
    assert hasattr(storage, "remove_bucket")
    assert not hasattr(storage, "rm_bucket")


def test_remove_bucket_deletes_objects_then_bucket(monkeypatch):
    """Multi-bucket mode: objects are deleted in batches, then the bucket itself."""
    storage, client = _new_storage(monkeypatch, {})
    _pages(
        client,
        {"Contents": [{"Key": "a"}, {"Key": "b"}]},
        {"Contents": [{"Key": "c"}]},
    )
    client.bucket_exists.return_value = True

    storage.remove_bucket("kb01")

    client.delete_objects.assert_any_call(Bucket="kb01", Delete={"Objects": [{"Key": "a"}, {"Key": "b"}]})
    client.delete_objects.assert_any_call(Bucket="kb01", Delete={"Objects": [{"Key": "c"}]})
    client.delete_bucket.assert_called_once_with(Bucket="kb01")


def test_remove_bucket_skips_missing_bucket(monkeypatch):
    """bucket_exists goes through head_bucket; drive the real 404 path."""
    storage, client = _new_storage(monkeypatch, {})
    client.head_bucket.side_effect = ClientError(
        {"Error": {"Code": "404"}},
        "HeadBucket",
    )

    storage.remove_bucket("nope")

    client.get_paginator.assert_not_called()
    client.delete_objects.assert_not_called()
    client.delete_bucket.assert_not_called()


def test_remove_bucket_raises_on_batch_delete_errors(monkeypatch):
    """delete_objects reports failed keys in its response instead of raising;
    the helper must surface them rather than report silent success."""
    storage, client = _new_storage(monkeypatch, {})
    _pages(
        client,
        {"Contents": [{"Key": "a"}]},
        {"Contents": [{"Key": "c"}]},
    )
    client.delete_objects.side_effect = [
        {"Deleted": [{"Key": "a"}]},
        {"Errors": [{"Key": "c", "Code": "InternalError", "Message": "boom"}]},
    ]
    client.bucket_exists.return_value = True

    storage.remove_bucket("kb01")

    client.delete_bucket.assert_not_called()


def test_remove_bucket_log_omits_request_urls(monkeypatch, caplog):
    """S3 error responses can embed signed query strings; the log line must
    carry the failure mode, not a verbatim request URL."""
    storage, client = _new_storage(monkeypatch, {})
    # A non-ClientError escapes bucket_exists and reaches remove_bucket's
    # handler; botocore wraps such failures with the request URL attached.
    client.head_bucket.side_effect = ConnectionError("GET https://s3.example.com/bucket?X-Amz-Signature=SECRET failed")

    with caplog.at_level(logging.ERROR):
        storage.remove_bucket("kb01")

    client.delete_bucket.assert_not_called()
    client.get_paginator.assert_not_called()
    messages = [r.getMessage() for r in caplog.records]
    assert any("Fail to remove bucket kb01" in m for m in messages), messages
    for m in messages:
        assert "X-Amz-Signature" not in m


def test_remove_bucket_single_bucket_mode_keeps_physical_bucket(monkeypatch):
    """Default-bucket mode: scope deletion to the logical prefix, keep the bucket."""
    storage, client = _new_storage(monkeypatch, {"bucket": "ragflow"})
    _pages(
        client,
        by_prefix={"kb01/": [{"Contents": [{"Key": "kb01/x"}, {"Key": "kb01/y"}]}]},
    )

    storage.remove_bucket("kb01")

    deleted = client.delete_objects.call_args
    assert deleted.kwargs["Bucket"] == "ragflow"
    keys = [o["Key"] for o in deleted.kwargs["Delete"]["Objects"]]
    assert keys == ["kb01/x", "kb01/y"]
    client.delete_bucket.assert_not_called()


def test_remove_bucket_single_bucket_mode_with_prefix_path(monkeypatch):
    storage, client = _new_storage(monkeypatch, {"bucket": "ragflow", "prefix_path": "prod"})
    _pages(
        client,
        by_prefix={"prod/kb01/": [{"Contents": [{"Key": "prod/kb01/doc"}]}]},
    )

    storage.remove_bucket("kb01")

    deleted = client.delete_objects.call_args
    keys = [o["Key"] for o in deleted.kwargs["Delete"]["Objects"]]
    assert keys == ["prod/kb01/doc"]
    client.delete_bucket.assert_not_called()


def test_remove_bucket_empty_pages_delete_nothing_but_bucket(monkeypatch):
    """A bucket that exists but has no objects: only delete_bucket fires."""
    storage, client = _new_storage(monkeypatch, {})
    _pages(client, {})
    client.bucket_exists.return_value = True

    storage.remove_bucket("kb01")

    client.delete_objects.assert_not_called()
    client.delete_bucket.assert_called_once_with(Bucket="kb01")
