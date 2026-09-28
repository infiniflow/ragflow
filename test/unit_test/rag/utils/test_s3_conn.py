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


def _client_error(code):
    return ClientError({"Error": {"Code": code}}, "GetObject")


def test_s3_put_succeeds_after_transient_failure(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    client.upload_fileobj.side_effect = [ConnectionError("temporary"), None]
    reconnect = Mock(return_value=True)
    monkeypatch.setattr(storage, "__open__", reconnect)
    sleep = Mock()
    monkeypatch.setattr(s3_conn.time, "sleep", sleep)

    assert storage.put("kb", "file.txt", b"data") is None
    assert client.upload_fileobj.call_count == 2
    reconnect.assert_called_once_with()
    sleep.assert_called_once_with(1)


def test_s3_put_succeeds_on_first_attempt(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})

    assert storage.put("kb", "file.txt", b"data") is client.upload_fileobj.return_value
    client.upload_fileobj.assert_called_once()


def test_s3_put_succeeds_after_two_transient_failures(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    client.upload_fileobj.side_effect = [ConnectionError("one"), ConnectionError("two"), None]
    reconnect = Mock(return_value=True)
    monkeypatch.setattr(storage, "__open__", reconnect)
    sleep = Mock()
    monkeypatch.setattr(s3_conn.time, "sleep", sleep)

    assert storage.put("kb", "file.txt", b"data") is None
    assert client.upload_fileobj.call_count == 3
    assert reconnect.call_count == 2
    assert sleep.call_args_list == [((1,), {}), ((2,), {})]


def test_s3_put_exhaustion_reraises_and_uses_expected_retries(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    failure = ConnectionError("temporary")
    client.upload_fileobj.side_effect = failure
    reconnect = Mock(return_value=True)
    monkeypatch.setattr(storage, "__open__", reconnect)
    sleep = Mock()
    monkeypatch.setattr(s3_conn.time, "sleep", sleep)

    with pytest.raises(ConnectionError) as raised:
        storage.put("kb", "file.txt", b"data")

    assert raised.value is failure
    assert client.upload_fileobj.call_count == 3
    assert reconnect.call_count == 2
    assert sleep.call_args_list == [((1,), {}), ((2,), {})]


def test_s3_put_reconnect_failure_preserves_operation_error(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    failure = ConnectionError("upload failed")
    client.upload_fileobj.side_effect = failure
    reconnect = Mock(return_value=False)
    monkeypatch.setattr(storage, "__open__", reconnect)
    sleep = Mock()
    monkeypatch.setattr(s3_conn.time, "sleep", sleep)

    with pytest.raises(ConnectionError) as raised:
        storage.put("kb", "file.txt", b"data")

    assert raised.value is failure
    assert client.upload_fileobj.call_count == 1
    sleep.assert_not_called()


def test_s3_get_succeeds_after_transient_failure(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    client.get_object.side_effect = [ConnectionError("temporary"), {"Body": Mock(read=Mock(return_value=b"data"))}]
    reconnect = Mock(return_value=True)
    monkeypatch.setattr(storage, "__open__", reconnect)
    sleep = Mock()
    monkeypatch.setattr(s3_conn.time, "sleep", sleep)

    assert storage.get("kb", "file.txt") == b"data"
    assert client.get_object.call_count == 2
    reconnect.assert_called_once_with()
    sleep.assert_called_once_with(1)


def test_s3_get_succeeds_on_first_attempt(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    client.get_object.return_value = {"Body": Mock(read=Mock(return_value=b"data"))}

    assert storage.get("kb", "file.txt") == b"data"
    client.get_object.assert_called_once()


@pytest.mark.parametrize("code", ["404", "NoSuchKey", "NotFound"])
def test_s3_get_missing_object_codes_return_none_without_retry(monkeypatch, code):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    client.get_object.side_effect = _client_error(code)
    reconnect = Mock(return_value=True)
    monkeypatch.setattr(storage, "__open__", reconnect)
    sleep = Mock()
    monkeypatch.setattr(s3_conn.time, "sleep", sleep)

    assert storage.get("kb", "missing.txt") is None
    client.get_object.assert_called_once_with(Bucket="ragflow", Key="missing.txt")
    reconnect.assert_not_called()
    sleep.assert_not_called()


def test_s3_get_non_not_found_client_error_exhausts_retries(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    failure = _client_error("AccessDenied")
    client.get_object.side_effect = failure
    reconnect = Mock(return_value=True)
    monkeypatch.setattr(storage, "__open__", reconnect)
    sleep = Mock()
    monkeypatch.setattr(s3_conn.time, "sleep", sleep)

    with pytest.raises(ClientError) as raised:
        storage.get("kb", "file.txt")

    assert raised.value is failure
    assert client.get_object.call_count == 3
    assert reconnect.call_count == 2
    assert sleep.call_args_list == [((1,), {}), ((2,), {})]


def test_s3_get_reconnect_failure_preserves_operation_error(monkeypatch):
    storage, client, _ = _new_storage(monkeypatch, {"bucket": "ragflow"})
    failure = ConnectionError("download failed")
    client.get_object.side_effect = failure
    reconnect = Mock(return_value=False)
    monkeypatch.setattr(storage, "__open__", reconnect)
    sleep = Mock()
    monkeypatch.setattr(s3_conn.time, "sleep", sleep)

    with pytest.raises(ConnectionError) as raised:
        storage.get("kb", "file.txt")

    assert raised.value is failure
    assert client.get_object.call_count == 1
    sleep.assert_not_called()
