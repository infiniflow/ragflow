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
from unittest.mock import Mock

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
    its pages so single-bucket mode sees realistic prefix-filtered results."""
    paginator = Mock()

    def paginate(**kwargs):
        if by_prefix is not None:
            return list(by_prefix.get(kwargs.get("Prefix"), []))
        return list(pages)

    paginator.paginate.side_effect = paginate
    client.get_paginator.return_value = paginator


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
    storage, client = _new_storage(monkeypatch, {})
    client.bucket_exists.return_value = False

    storage.remove_bucket("nope")

    client.delete_objects.assert_not_called()
    client.delete_bucket.assert_not_called()


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
