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

from unittest.mock import patch

import pytest

from core.config import AppConfig
from core.types.storage import ObjectStorageType


def test_storage_defaults(monkeypatch):
    monkeypatch.delenv("STORAGE_IMPL", raising=False)

    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    assert cfg.storage.active == ObjectStorageType.MINIO


def test_storage_active_override(monkeypatch):
    monkeypatch.setenv("STORAGE_IMPL", "oss")

    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    assert cfg.storage.active == ObjectStorageType.OSS


def test_minio_defaults(monkeypatch):
    monkeypatch.setenv("STORAGE_IMPL", "minio")
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    minio_cfg = cfg.storage.minio
    assert minio_cfg.host == "localhost:9000"
    assert minio_cfg.user == "rag_flow"
    assert minio_cfg.password == ""
    assert minio_cfg.bucket == ""
    assert minio_cfg.prefix_path == ""


def test_minio_override(monkeypatch):
    monkeypatch.delenv("STORAGE_IMPL", raising=False)

    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [
            {
                "storage": {
                    "active": "minio",
                    "minio": {
                        "host": "127.0.0.1",
                        "user": "minio",
                        "password": "minio",
                        "bucket": "minio",
                        "prefix_path": "minio",
                    }
                }
            },
            {}
        ]
        cfg = AppConfig()

    minio_cfg = cfg.storage.minio
    assert minio_cfg.host == "127.0.0.1"
    assert minio_cfg.user == "minio"
    assert minio_cfg.password == "minio"
    assert minio_cfg.bucket == "minio"
    assert minio_cfg.prefix_path == "minio"


def test_storage_active_types(monkeypatch):
    for impl, enum_val in [
        ("minio", ObjectStorageType.MINIO),
        ("s3", ObjectStorageType.S3),
        ("oss", ObjectStorageType.OSS),
        ("azure_sas", ObjectStorageType.AZURE_SAS),
        ("azure_spn", ObjectStorageType.AZURE_SPN),
        ("opendal", ObjectStorageType.OPENDAL),
    ]:
        monkeypatch.setenv("STORAGE_IMPL", impl)
        with patch("core.config.app.load_yaml") as mock_load:
            mock_load.side_effect = [{}, {}]
            cfg = AppConfig()
        assert cfg.storage.active == enum_val


@pytest.mark.parametrize(
    "key, override",
    [
        ("s3", {"access_key": "ak", "secret_key": "sk", "bucket": "b"}),
        ("oss", {"access_key": "ak", "secret_key": "sk", "bucket": "b"}),
        ("azure_sas", {"container_url": "url", "sas_token": "token"}),
        ("azure_spn", {"account_url": "url", "client_id": "id", "secret": "sec", "tenant_id": "tid", "container_name": "c"}),
        ("opendal", {"scheme": "mysql", "config": {"oss_table": "table"}}),
    ]
)
def test_storage_yaml_override(key, override, monkeypatch):
    monkeypatch.delenv("STORAGE_IMPL", raising=False)
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{"storage": {key: override}}, {}]
        cfg = AppConfig()
    storage_cfg = getattr(cfg.storage, key)
    for k, v in override.items():
        assert getattr(storage_cfg, k) == v


def test_storage_current(monkeypatch):
    monkeypatch.setenv("STORAGE_IMPL", "minio")
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()
    current = cfg.storage.current
    assert current == cfg.storage.minio
