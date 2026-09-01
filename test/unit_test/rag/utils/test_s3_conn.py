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

import importlib
from unittest.mock import Mock, call

import pytest
from botocore.exceptions import ClientError

# rag.utils.s3_conn and common.settings import each other (s3_conn needs
# settings.S3; settings re-imports RAGFlowS3). The cycle resolves only when
# common.settings is imported first, the same order the app uses. Importing it
# here before s3_conn lets both modules load fully so other tests that depend
# on the real settings (e.g. db_models reads settings.DATABASE_TYPE) keep
# working in the same session.
import common.settings  # noqa: F401 (imported for load-order side effect: breaks the s3_conn <-> settings cycle)
from rag.utils import s3_conn


def _new_storage(monkeypatch, config):
    module = importlib.reload(s3_conn)
    client = Mock()
    monkeypatch.setattr(module.settings, "S3", config)
    monkeypatch.setattr(module.boto3, "client", Mock(return_value=client))
    return module.RAGFlowS3(), client, module.boto3.client


def test_s3_accepts_region_config_key(monkeypatch):
    storage, _, client_factory = _new_storage(monkeypatch, {"region": "us-east-1", "bucket": "ragflow"})

    assert storage.region_name == "us-east-1"
    client_factory.assert_called_once_with("s3", region_name="us-east-1")


def test_s3_health_uses_head_bucket_without_writing(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})

    assert storage.health() is True
    client.head_bucket.assert_called_once_with(Bucket="ragflow")
    client.create_bucket.assert_not_called()
    client.upload_fileobj.assert_not_called()


def test_s3_health_uses_list_buckets_without_default_bucket(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {})

    assert storage.health() is True
    client.list_buckets.assert_called_once_with()


def test_s3_health_returns_false_on_client_error(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    client.head_bucket.side_effect = ConnectionError("unavailable")

    assert storage.health() is False


def test_s3_remove_bucket_deletes_every_page_before_deleting_bucket(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {})
    paginator = client.get_paginator.return_value
    paginator.paginate.return_value = [
        {"Contents": [{"Key": "first"}, {"Key": "second"}]},
        {},
        {"Contents": [{"Key": "third"}]},
    ]
    client.delete_objects.side_effect = [{}, {}]

    storage.remove_bucket("dataset-id")

    client.head_bucket.assert_called_once_with(Bucket="dataset-id")
    client.get_paginator.assert_called_once_with("list_objects_v2")
    paginator.paginate.assert_called_once_with(Bucket="dataset-id", Prefix="")
    assert client.delete_objects.call_args_list == [
        call(Bucket="dataset-id", Delete={"Objects": [{"Key": "first"}, {"Key": "second"}]}),
        call(Bucket="dataset-id", Delete={"Objects": [{"Key": "third"}]}),
    ]
    client.delete_bucket.assert_called_once_with(Bucket="dataset-id")


def test_s3_remove_bucket_only_deletes_logical_prefix_in_single_bucket_mode(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "physical-bucket", "prefix_path": "ragflow"})
    paginator = client.get_paginator.return_value
    paginator.paginate.return_value = [{"Contents": [{"Key": "ragflow/dataset-id/document"}]}]
    client.delete_objects.return_value = {}

    storage.remove_bucket("dataset-id")

    client.head_bucket.assert_not_called()
    paginator.paginate.assert_called_once_with(Bucket="physical-bucket", Prefix="ragflow/dataset-id/")
    client.delete_objects.assert_called_once_with(Bucket="physical-bucket", Delete={"Objects": [{"Key": "ragflow/dataset-id/document"}]})
    client.delete_bucket.assert_not_called()


def test_s3_remove_bucket_ignores_missing_bucket(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {})
    client.head_bucket.side_effect = ClientError({"Error": {"Code": "404", "Message": "Not Found"}}, "HeadBucket")

    storage.remove_bucket("missing")

    client.get_paginator.assert_not_called()
    client.delete_bucket.assert_not_called()


def test_s3_remove_bucket_propagates_storage_errors(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {})
    client.head_bucket.side_effect = ClientError({"Error": {"Code": "403", "Message": "Forbidden"}}, "HeadBucket")

    with pytest.raises(ClientError):
        storage.remove_bucket("dataset-id")


def test_s3_remove_bucket_reports_partial_delete_failures(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {})
    paginator = client.get_paginator.return_value
    paginator.paginate.return_value = [{"Contents": [{"Key": "blocked"}]}]
    client.delete_objects.return_value = {"Errors": [{"Key": "blocked", "Code": "AccessDenied"}]}

    with pytest.raises(RuntimeError, match="blocked.*AccessDenied"):
        storage.remove_bucket("dataset-id")

    client.delete_bucket.assert_not_called()
