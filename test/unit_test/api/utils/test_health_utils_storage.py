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

from unittest.mock import Mock, patch

from api.utils.health_utils import check_storage, check_storage_alive


@patch("api.utils.health_utils.settings")
def test_check_storage_alive_reports_success(mock_settings):
    mock_settings.STORAGE_IMPL = Mock()
    mock_settings.STORAGE_IMPL.health.return_value = True

    result = check_storage_alive()

    assert result["status"] == "alive"
    assert "elapsed" in result["message"]


@patch("api.utils.health_utils.settings")
def test_check_storage_alive_reports_explicit_false_as_timeout(mock_settings):
    mock_settings.STORAGE_IMPL = Mock()
    mock_settings.STORAGE_IMPL.health.return_value = False

    result = check_storage_alive()

    assert result["status"] == "timeout"


@patch("api.utils.health_utils.settings")
def test_check_storage_alive_reports_exception_as_timeout(mock_settings):
    mock_settings.STORAGE_IMPL = Mock()
    mock_settings.STORAGE_IMPL.health.side_effect = ConnectionError("unavailable")

    result = check_storage_alive()

    assert result == {"status": "timeout", "message": "error: unavailable"}


@patch("api.utils.health_utils.settings")
def test_check_storage_respects_explicit_false_result(mock_settings):
    mock_settings.STORAGE_IMPL = Mock()
    mock_settings.STORAGE_IMPL.health.return_value = False

    ok, metadata = check_storage()

    assert ok is False
    assert "elapsed" in metadata
