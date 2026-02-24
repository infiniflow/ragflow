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
Unit tests for MinIO client SSL/secure configuration (_build_minio_http_client).
Covers issue #13158.
"""
import ssl
from unittest.mock import patch

import pytest


class TestBuildMinioHttpClient:
    """Test _build_minio_http_client helper."""

    @patch("rag.utils.minio_conn.settings")
    def test_returns_none_when_verify_true(self, mock_settings):
        mock_settings.MINIO = {"verify": True}
        from rag.utils.minio_conn import _build_minio_http_client
        client = _build_minio_http_client()
        assert client is None

    @patch("rag.utils.minio_conn.settings")
    def test_returns_none_when_verify_missing(self, mock_settings):
        mock_settings.MINIO = {}
        from rag.utils.minio_conn import _build_minio_http_client
        client = _build_minio_http_client()
        assert client is None

    @patch("rag.utils.minio_conn.settings")
    def test_returns_pool_manager_when_verify_false(self, mock_settings):
        mock_settings.MINIO = {"verify": False}
        from rag.utils.minio_conn import _build_minio_http_client
        client = _build_minio_http_client()
        assert client is not None
        assert hasattr(client, "connection_pool_kw")
        assert client.connection_pool_kw.get("cert_reqs") == ssl.CERT_NONE

    @patch("rag.utils.minio_conn.settings")
    def test_returns_pool_manager_when_verify_string_false(self, mock_settings):
        mock_settings.MINIO = {"verify": "false"}
        from rag.utils.minio_conn import _build_minio_http_client
        client = _build_minio_http_client()
        assert client is not None
        assert client.connection_pool_kw.get("cert_reqs") == ssl.CERT_NONE

    @patch("rag.utils.minio_conn.settings")
    def test_returns_none_when_verify_string_1(self, mock_settings):
        mock_settings.MINIO = {"verify": "1"}
        from rag.utils.minio_conn import _build_minio_http_client
        client = _build_minio_http_client()
        assert client is None
