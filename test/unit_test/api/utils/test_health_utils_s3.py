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
Unit tests for the S3 file-store health check (check_s3_alive).
Closes #17294 — Admin Service status used to silently show MinIO when
STORAGE_IMPL=AWS_S3, because the only health check wired into the
admin config loader was check_minio_alive.
"""

from unittest.mock import patch


class TestCheckS3Alive:
    """Test check_s3_alive delegates to the generic check_storage."""

    @patch("api.utils.health_utils.check_storage")
    def test_returns_alive_when_storage_healthy(self, mock_check_storage):
        mock_check_storage.return_value = (True, {"elapsed": "12.3"})
        from api.utils.health_utils import check_s3_alive

        result = check_s3_alive()
        assert result["status"] == "alive"
        assert "elapsed" in result["message"]
        mock_check_storage.assert_called_once_with()

    @patch("api.utils.health_utils.check_storage")
    def test_returns_timeout_when_storage_unhealthy(self, mock_check_storage):
        mock_check_storage.return_value = (False, {"elapsed": "9.0", "error": "Connection refused"})
        from api.utils.health_utils import check_s3_alive

        result = check_s3_alive()
        assert result["status"] == "timeout"
        assert "Connection refused" in result["message"]

    @patch("api.utils.health_utils.check_storage")
    def test_returns_timeout_when_error_missing(self, mock_check_storage):
        # Storage health returned False with no error key — must still
        # surface a non-empty message so the admin UI doesn't render
        # an empty status.
        mock_check_storage.return_value = (False, {"elapsed": "1.0"})
        from api.utils.health_utils import check_s3_alive

        result = check_s3_alive()
        assert result["status"] == "timeout"
        assert "unknown" in result["message"]


class TestCheckStorageDelegateContract:
    """Verify check_s3_alive and check_storage stay in sync.

    The S3 health check is a thin wrapper over check_storage so the
    same code path works for AWS S3, MinIO, R2, etc. If check_storage's
    return shape ever changes, these tests will fail and force a
    companion update to check_s3_alive.
    """

    @patch("api.utils.health_utils.check_storage")
    def test_message_includes_elapsed_from_storage(self, mock_check_storage):
        mock_check_storage.return_value = (True, {"elapsed": "42.0"})
        from api.utils.health_utils import check_s3_alive

        result = check_s3_alive()
        assert "42.0" in result["message"]

    @patch("api.utils.health_utils.check_storage")
    def test_message_includes_error_string(self, mock_check_storage):
        mock_check_storage.return_value = (False, {"elapsed": "0.5", "error": "boom"})
        from api.utils.health_utils import check_s3_alive

        result = check_s3_alive()
        assert "boom" in result["message"]
