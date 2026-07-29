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
Unit tests for ``ServiceMgr.get_all_services`` in
``admin/server/services.py``.

Specifically covers the new ``STORAGE_IMPL`` filter that hides
inactive file_store backends. See #17294 (Admin Service status still
reports MinIO when object storage is configured as AWS S3).
"""

from unittest.mock import patch

import pytest

from config import (
    ElasticsearchConfig,
    FileStoreConfig,
    MinioConfig,
    SERVICE_CONFIGS,
)


def _minio_config():
    return MinioConfig(
        id=0,
        name="minio",
        host="minio",
        port=9000,
        user="u",
        password="p",
        service_type="file_store",
        store_type="minio",
        detail_func_name="check_minio_alive",
    )


def _s3_config():
    return FileStoreConfig(
        id=1,
        name="s3",
        host="s3.us-east-1.amazonaws.com",
        port=443,
        service_type="file_store",
        store_type="s3",
        detail_func_name="check_s3_alive",
    )


def _es_config(retrieval_type="elasticsearch"):
    return ElasticsearchConfig(
        id=2,
        name="elasticsearch",
        host="es",
        port=9200,
        service_type="retrieval",
        retrieval_type=retrieval_type,
        username="",
        password="",
        detail_func_name="get_es_cluster_stats",
    )


@pytest.fixture
def install_configs():
    """Install a clean ``SERVICE_CONFIGS.configs`` for each test, then
    restore the previous value. Required because the admin module uses
    ``SERVICE_CONFIGS`` as a mutable namespace, not an instance."""
    previous = list(getattr(SERVICE_CONFIGS, "configs", []))
    yield
    SERVICE_CONFIGS.configs = previous


class TestFileStoreFilter:
    """The new filter: only the active file_store backend is returned."""

    def test_minio_shown_when_storage_impl_is_minio(self, install_configs, monkeypatch):
        monkeypatch.setenv("STORAGE_IMPL", "MINIO")
        SERVICE_CONFIGS.configs = [_minio_config(), _s3_config()]

        with patch("services.ServiceMgr.get_service_details", return_value={"status": "alive"}):
            from services import ServiceMgr

            result = ServiceMgr.get_all_services()

        stores = [s for s in result if s["service_type"] == "file_store"]
        assert len(stores) == 1
        assert stores[0]["name"] == "minio"

    def test_s3_shown_when_storage_impl_is_aws_s3(self, install_configs, monkeypatch):
        monkeypatch.setenv("STORAGE_IMPL", "AWS_S3")
        SERVICE_CONFIGS.configs = [_minio_config(), _s3_config()]

        with patch("services.ServiceMgr.get_service_details", return_value={"status": "alive"}):
            from services import ServiceMgr

            result = ServiceMgr.get_all_services()

        stores = [s for s in result if s["service_type"] == "file_store"]
        assert len(stores) == 1
        assert stores[0]["name"] == "s3"

    def test_no_file_store_shown_when_active_backend_not_configured(self, install_configs, monkeypatch):
        """If the active backend has no corresponding config block,
        nothing is shown for file_store. We never fall back to a
        stale minio entry."""
        monkeypatch.setenv("STORAGE_IMPL", "AWS_S3")
        # Only the minio block is present.
        SERVICE_CONFIGS.configs = [_minio_config()]

        with patch("services.ServiceMgr.get_service_details", return_value={"status": "alive"}):
            from services import ServiceMgr

            result = ServiceMgr.get_all_services()

        stores = [s for s in result if s["service_type"] == "file_store"]
        assert stores == []


class TestRetrivalFilterStillWorks:
    """Regression guard: the existing DOC_ENGINE filter for retrieval
    must keep working — the new file_store filter is additive."""

    def test_elasticsearch_shown_when_doc_engine_is_elasticsearch(self, install_configs, monkeypatch):
        monkeypatch.setenv("DOC_ENGINE", "elasticsearch")
        monkeypatch.setenv("STORAGE_IMPL", "MINIO")
        from config import InfinityConfig

        SERVICE_CONFIGS.configs = [
            _es_config(retrieval_type="elasticsearch"),
            InfinityConfig(
                id=3,
                name="infinity",
                host="inf",
                port=23800,
                service_type="retrieval",
                retrieval_type="infinity",
                db_name="default_db",
                detail_func_name="get_infinity_status",
            ),
            _minio_config(),
        ]

        with patch("services.ServiceMgr.get_service_details", return_value={"status": "alive"}):
            from services import ServiceMgr

            result = ServiceMgr.get_all_services()

        retrievals = [s for s in result if s["service_type"] == "retrieval"]
        assert len(retrievals) == 1
        assert retrievals[0]["name"] == "elasticsearch"

    def test_infinity_filtered_when_doc_engine_is_elasticsearch(self, install_configs, monkeypatch):
        """When DOC_ENGINE=elasticsearch, an infinity retrieval config
        must be filtered out."""
        monkeypatch.setenv("DOC_ENGINE", "elasticsearch")
        monkeypatch.setenv("STORAGE_IMPL", "MINIO")
        from config import InfinityConfig

        SERVICE_CONFIGS.configs = [
            _es_config(retrieval_type="elasticsearch"),
            InfinityConfig(
                id=3,
                name="infinity",
                host="inf",
                port=23800,
                service_type="retrieval",
                retrieval_type="infinity",
                db_name="default_db",
                detail_func_name="get_infinity_status",
            ),
        ]

        with patch("services.ServiceMgr.get_service_details", return_value={"status": "alive"}):
            from services import ServiceMgr

            result = ServiceMgr.get_all_services()

        names = [s["name"] for s in result if s["service_type"] == "retrieval"]
        assert "infinity" not in names
        assert "elasticsearch" in names


class TestStorageImplNameMapping:
    """The STORAGE_IMPL env var uses upper-case + underscores (e.g.
    ``AWS_S3``). The ``store_type`` we record is lower-case (``s3``).
    The filter must translate correctly so ``AWS_S3`` matches
    ``s3``, not ``aws_s3``."""

    @pytest.mark.parametrize(
        "storage_impl,expected_store",
        [
            ("MINIO", "minio"),
            ("AWS_S3", "s3"),
            ("OSS", "oss"),
            ("GCS", "gcs"),
        ],
    )
    def test_env_var_maps_to_store_type(self, install_configs, monkeypatch, storage_impl, expected_store):
        monkeypatch.setenv("STORAGE_IMPL", storage_impl)
        active = FileStoreConfig(
            id=0,
            name=expected_store,
            host="x",
            port=443,
            service_type="file_store",
            store_type=expected_store,
            detail_func_name="check_storage",
        )
        inactive = FileStoreConfig(
            id=1,
            name="other",
            host="x",
            port=443,
            service_type="file_store",
            store_type="other",
            detail_func_name="check_storage",
        )
        SERVICE_CONFIGS.configs = [active, inactive]

        with patch("services.ServiceMgr.get_service_details", return_value={"status": "alive"}):
            from services import ServiceMgr

            result = ServiceMgr.get_all_services()

        stores = [s for s in result if s["service_type"] == "file_store"]
        assert len(stores) == 1
        assert stores[0]["name"] == expected_store
