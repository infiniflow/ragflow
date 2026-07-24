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
from unittest.mock import Mock

import rag.utils.s3_conn as s3_conn


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
