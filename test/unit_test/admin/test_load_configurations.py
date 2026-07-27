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
Unit tests for ``load_configurations`` in ``admin/server/config.py``.

Specifically covers the new ``case "s3"`` branch that the admin status
page needs to render AWS S3 as the active file_store backend. See #17294
(``Admin Service status still reports MinIO when object storage is
configured as AWS S3``).

``read_config`` in ``common.config_utils`` interprets its argument as a
filename relative to the project ``conf/`` directory, so we patch it
to return a controlled mapping per test.
"""

from unittest.mock import patch

import pytest

from config import FileStoreConfig, MinioConfig, load_configurations


def _read_config_returns(mapping):
    """Build a patch that makes ``read_config`` return ``mapping``."""
    return patch("config.read_config", return_value=mapping)


class TestLoadS3Configuration:
    """``case "s3"`` branch of load_configurations."""

    def test_s3_endpoint_with_explicit_https(self):
        with _read_config_returns(
            {
                "s3": {
                    "access_key": "AKIA...",
                    "secret_key": "...",
                    "region_name": "us-east-1",
                    "endpoint_url": "https://s3.us-east-1.amazonaws.com",
                    "bucket": "my-bucket",
                }
            }
        ):
            configs = load_configurations("/unused/service_conf.yaml")
        s3 = next(c for c in configs if isinstance(c, FileStoreConfig) and c.store_type == "s3")
        assert s3.name == "s3"
        assert s3.service_type == "file_store"
        assert s3.detail_func_name == "check_s3_alive"
        assert s3.host == "s3.us-east-1.amazonaws.com"
        assert s3.port == 443

    def test_s3_endpoint_with_explicit_http(self):
        with _read_config_returns(
            {
                "s3": {
                    "endpoint_url": "http://minio.local:9000",
                    "bucket": "test",
                }
            }
        ):
            configs = load_configurations("/unused/service_conf.yaml")
        s3 = next(c for c in configs if isinstance(c, FileStoreConfig) and c.store_type == "s3")
        assert s3.host == "minio.local"
        assert s3.port == 9000

    def test_s3_endpoint_with_non_default_https_port(self):
        with _read_config_returns(
            {
                "s3": {
                    "endpoint_url": "https://s3.custom.example.com:8443",
                    "bucket": "x",
                }
            }
        ):
            configs = load_configurations("/unused/service_conf.yaml")
        s3 = next(c for c in configs if isinstance(c, FileStoreConfig) and c.store_type == "s3")
        assert s3.host == "s3.custom.example.com"
        assert s3.port == 8443

    def test_s3_endpoint_omitted_uses_aws_default(self):
        with _read_config_returns({"s3": {"bucket": "my-bucket"}}):
            configs = load_configurations("/unused/service_conf.yaml")
        s3 = next(c for c in configs if isinstance(c, FileStoreConfig) and c.store_type == "s3")
        # No endpoint_url → fall back to standard AWS S3 endpoint.
        assert s3.host == "s3.amazonaws.com"
        assert s3.port == 443

    def test_s3_id_count_strictly_increments(self):
        """Each backend added by the loader must receive a unique id."""
        with _read_config_returns(
            {
                "minio": {"host": "minio:9000", "user": "u", "password": "p"},
                "s3": {"endpoint_url": "https://s3.us-east-1.amazonaws.com", "bucket": "x"},
                "redis": {"host": "redis:6379", "password": "", "db": 0},
            }
        ):
            configs = load_configurations("/unused/service_conf.yaml")
        ids = [c.id for c in configs]
        assert ids == sorted(set(ids)), f"Duplicate ids: {ids}"


class TestLoadS3Regressions:
    """Make sure the new branch does not break existing backends."""

    def test_minio_still_creates_minio_config(self):
        with _read_config_returns(
            {
                "minio": {
                    "user": "rag_flow",
                    "password": "infini_rag_flow",
                    "host": "minio:9000",
                }
            }
        ):
            configs = load_configurations("/unused/service_conf.yaml")
        minio = next(c for c in configs if isinstance(c, MinioConfig))
        assert minio.store_type == "minio"
        assert minio.service_type == "file_store"
        assert minio.detail_func_name == "check_minio_alive"
        assert minio.host == "minio"
        assert minio.port == 9000

    def test_both_minio_and_s3_in_same_config(self):
        """When the user has both backends configured, the admin loader
        surfaces both. The service-layer filter at #17294 is responsible
        for hiding the inactive one."""
        with _read_config_returns(
            {
                "minio": {"host": "minio:9000", "user": "u", "password": "p"},
                "s3": {"endpoint_url": "https://s3.us-east-1.amazonaws.com", "bucket": "x"},
            }
        ):
            configs = load_configurations("/unused/service_conf.yaml")
        minio = [c for c in configs if isinstance(c, MinioConfig)]
        s3 = [c for c in configs if isinstance(c, FileStoreConfig) and c.store_type == "s3"]
        assert len(minio) == 1
        assert len(s3) == 1


class TestLoadS3HandlesBadEndpoint:
    """The endpoint_url parser must never raise on bad input — the admin
    UI would otherwise crash and stop showing all services. The fix
    degrades to a best-effort host string."""

    @pytest.mark.parametrize(
        "endpoint",
        [
            "not a url",
            "://no-scheme",
            "https://",
            # `parsed.port` raises ValueError on these:
            "https://s3.example.com:not-a-port",
            "https://s3.example.com:99999999999999",
            # `urlparse` itself raises ValueError on malformed IPv6:
            "https://[::1",
        ],
    )
    def test_malformed_endpoint_does_not_raise(self, endpoint):
        with _read_config_returns({"s3": {"endpoint_url": endpoint, "bucket": "x"}}):
            configs = load_configurations("/unused/service_conf.yaml")
        s3 = next(c for c in configs if isinstance(c, FileStoreConfig) and c.store_type == "s3")
        # The contract is "does not raise". The exact host/port for
        # pathological input is best-effort.
        assert s3 is not None
        # port is always an int (or 0 for empty string) — never raises.
        assert isinstance(s3.port, int)
