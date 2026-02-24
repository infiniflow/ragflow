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
Unit tests for MinIO health check (check_minio_alive) and scheme/verify helpers.
Covers SSL/HTTPS and certificate verification (issues #13158, #13159).
"""
import pytest
from unittest.mock import patch, Mock


class TestMinioSchemeAndVerify:
    """Test _minio_scheme_and_verify helper."""

    @patch("api.utils.health_utils.settings")
    def test_scheme_http_when_secure_false(self, mock_settings):
        mock_settings.MINIO = {"host": "minio:9000", "secure": False}
        from api.utils.health_utils import _minio_scheme_and_verify
        scheme, verify = _minio_scheme_and_verify()
        assert scheme == "http"
        assert verify is True

    @patch("api.utils.health_utils.settings")
    def test_scheme_https_when_secure_true(self, mock_settings):
        mock_settings.MINIO = {"host": "minio:9000", "secure": True}
        from api.utils.health_utils import _minio_scheme_and_verify
        scheme, verify = _minio_scheme_and_verify()
        assert scheme == "https"
        assert verify is True

    @patch("api.utils.health_utils.settings")
    def test_scheme_https_when_secure_string_true(self, mock_settings):
        mock_settings.MINIO = {"host": "minio:9000", "secure": "true"}
        from api.utils.health_utils import _minio_scheme_and_verify
        scheme, verify = _minio_scheme_and_verify()
        assert scheme == "https"

    @patch("api.utils.health_utils.settings")
    def test_verify_false_for_self_signed(self, mock_settings):
        mock_settings.MINIO = {"host": "minio:9000", "secure": True, "verify": False}
        from api.utils.health_utils import _minio_scheme_and_verify
        scheme, verify = _minio_scheme_and_verify()
        assert scheme == "https"
        assert verify is False

    @patch("api.utils.health_utils.settings")
    def test_verify_string_false(self, mock_settings):
        mock_settings.MINIO = {"host": "minio:9000", "verify": "false"}
        from api.utils.health_utils import _minio_scheme_and_verify
        _, verify = _minio_scheme_and_verify()
        assert verify is False

    @patch("api.utils.health_utils.settings")
    def test_default_verify_true_when_key_missing(self, mock_settings):
        mock_settings.MINIO = {"host": "minio:9000"}
        from api.utils.health_utils import _minio_scheme_and_verify
        _, verify = _minio_scheme_and_verify()
        assert verify is True


class TestCheckMinioAlive:
    """Test check_minio_alive with mocked requests and settings."""

    @patch("api.utils.health_utils.requests.get")
    @patch("api.utils.health_utils.settings")
    def test_returns_alive_when_http_200(self, mock_settings, mock_get):
        mock_settings.MINIO = {"host": "minio:9000", "secure": False}
        mock_response = Mock()
        mock_response.status_code = 200
        mock_get.return_value = mock_response
        from api.utils.health_utils import check_minio_alive
        result = check_minio_alive()
        assert result["status"] == "alive"
        assert "elapsed" in result["message"]
        mock_get.assert_called_once()
        call_args = mock_get.call_args
        assert call_args[0][0] == "http://minio:9000/minio/health/live"
        assert call_args[1]["verify"] is True

    @patch("api.utils.health_utils.requests.get")
    @patch("api.utils.health_utils.settings")
    def test_uses_https_when_secure_true(self, mock_settings, mock_get):
        mock_settings.MINIO = {"host": "minio:9000", "secure": True}
        mock_response = Mock()
        mock_response.status_code = 200
        mock_get.return_value = mock_response
        from api.utils.health_utils import check_minio_alive
        check_minio_alive()
        call_args = mock_get.call_args
        assert call_args[0][0] == "https://minio:9000/minio/health/live"

    @patch("api.utils.health_utils.requests.get")
    @patch("api.utils.health_utils.settings")
    def test_passes_verify_false_for_self_signed(self, mock_settings, mock_get):
        mock_settings.MINIO = {"host": "minio:9000", "secure": True, "verify": False}
        mock_response = Mock()
        mock_response.status_code = 200
        mock_get.return_value = mock_response
        from api.utils.health_utils import check_minio_alive
        check_minio_alive()
        call_args = mock_get.call_args
        assert call_args[1]["verify"] is False

    @patch("api.utils.health_utils.requests.get")
    @patch("api.utils.health_utils.settings")
    def test_returns_timeout_on_non_200(self, mock_settings, mock_get):
        mock_settings.MINIO = {"host": "minio:9000"}
        mock_response = Mock()
        mock_response.status_code = 503
        mock_get.return_value = mock_response
        from api.utils.health_utils import check_minio_alive
        result = check_minio_alive()
        assert result["status"] == "timeout"

    @patch("api.utils.health_utils.requests.get")
    @patch("api.utils.health_utils.settings")
    def test_returns_timeout_on_request_exception(self, mock_settings, mock_get):
        mock_settings.MINIO = {"host": "minio:9000"}
        mock_get.side_effect = ConnectionError("Connection refused")
        from api.utils.health_utils import check_minio_alive
        result = check_minio_alive()
        assert result["status"] == "timeout"
        assert "error" in result["message"]

    @patch("api.utils.health_utils.requests.get")
    @patch("api.utils.health_utils.settings")
    def test_request_uses_timeout(self, mock_settings, mock_get):
        mock_settings.MINIO = {"host": "minio:9000"}
        mock_response = Mock()
        mock_response.status_code = 200
        mock_get.return_value = mock_response
        from api.utils.health_utils import check_minio_alive
        check_minio_alive()
        call_args = mock_get.call_args
        assert call_args[1]["timeout"] == 10
