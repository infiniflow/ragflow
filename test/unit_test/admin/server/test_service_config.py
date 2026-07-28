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

from admin.server import config as service_config


def test_load_configurations_recognizes_s3_and_does_not_expose_credentials(monkeypatch):
    raw_config = {
        "minio": {"host": "minio:9000", "user": "minio-user", "password": "minio-password"},
        "s3": {
            "access_key": "access-key",
            "secret_key": "secret-key",
            "endpoint_url": "https://s3.example.com",
            "region": "us-east-1",
            "bucket": "ragflow",
        },
    }
    monkeypatch.setattr(service_config, "read_config", lambda _: raw_config)

    configurations = service_config.load_configurations("unused.yaml")

    assert [config.name for config in configurations] == ["minio", "s3"]
    s3 = configurations[1].to_dict()
    assert s3["host"] == "s3.example.com"
    assert s3["port"] == 443
    assert s3["extra"] == {
        "store_type": "s3",
        "endpoint_url": "https://s3.example.com",
        "scheme": "https",
        "region": "us-east-1",
        "bucket": "ragflow",
    }
    assert "access_key" not in s3["extra"]
    assert "secret_key" not in s3["extra"]


def test_s3_endpoint_supports_custom_port():
    endpoint_url, host, port, scheme = service_config._get_s3_endpoint({"endpoint_url": "http://s3.example.com:9000"})

    assert endpoint_url == "http://s3.example.com:9000"
    assert host == "s3.example.com"
    assert port == 9000
    assert scheme == "http"


def test_s3_endpoint_defaults_to_regional_aws_endpoint():
    endpoint_url, host, port, scheme = service_config._get_s3_endpoint({"region": "us-west-2"})

    assert endpoint_url == "https://s3.us-west-2.amazonaws.com"
    assert host == "s3.us-west-2.amazonaws.com"
    assert port == 443
    assert scheme == "https"


def test_service_activity_selects_s3_when_aws_s3_is_configured(monkeypatch):
    monkeypatch.setattr(
        service_config,
        "read_config",
        lambda _: {
            "minio": {"host": "minio:9000", "user": "minio-user", "password": "minio-password"},
            "s3": {"endpoint_url": "https://s3.example.com", "bucket": "ragflow"},
        },
    )
    minio, s3 = service_config.load_configurations("unused.yaml")

    assert service_config.is_service_active(minio, storage_impl="AWS_S3") is False
    assert service_config.is_service_active(s3, storage_impl="AWS_S3") is True


def test_service_activity_keeps_minio_as_default(monkeypatch):
    monkeypatch.setattr(
        service_config,
        "read_config",
        lambda _: {
            "minio": {"host": "minio:9000", "user": "minio-user", "password": "minio-password"},
            "s3": {"endpoint_url": "https://s3.example.com", "bucket": "ragflow"},
        },
    )
    minio, s3 = service_config.load_configurations("unused.yaml")

    assert service_config.is_service_active(minio, storage_impl="MINIO") is True
    assert service_config.is_service_active(s3, storage_impl="MINIO") is False
