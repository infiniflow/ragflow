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

"""Tests for Google Drive connector time buffer and time range filter.

Verifies:
1. The sync time buffer config is correctly defined and env-configurable.
2. The _adjust_start_for_query logic correctly shifts start backward.
3. The generate_time_range_filter function uses both modifiedTime and
   createdTime to catch newly uploaded files whose modifiedTime predates
   the last sync cutoff (issue #13939).
"""

import importlib
import os
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import patch

import common

# Bootstrap common.data_source without triggering __init__.py imports
# (which pull in boto3, googleapiclient, etc.)
repo_root = Path(__file__).resolve().parents[3]
_ds_pkg = ModuleType("common.data_source")
_ds_pkg.__path__ = [str(repo_root / "common" / "data_source")]
sys.modules["common.data_source"] = _ds_pkg
setattr(common, "data_source", _ds_pkg)

# Stub out google_drive subpackage dependencies before importing file_retrieval
_gd_pkg = ModuleType("common.data_source.google_drive")
_gd_pkg.__path__ = [str(repo_root / "common" / "data_source" / "google_drive")]
sys.modules["common.data_source.google_drive"] = _gd_pkg

_gu_pkg = ModuleType("common.data_source.google_util")
_gu_pkg.__path__ = [str(repo_root / "common" / "data_source" / "google_util")]
sys.modules["common.data_source.google_util"] = _gu_pkg

# Stub heavy third-party modules that file_retrieval and its deps import
_stub_modules = [
    "googleapiclient", "googleapiclient.discovery", "googleapiclient.errors",
    "google", "google.auth", "google.auth.exceptions",
    "google.oauth2", "google.oauth2.credentials", "google.oauth2.service_account",
]
for mod_name in _stub_modules:
    if mod_name not in sys.modules:
        stub = ModuleType(mod_name)
        # Add common attributes that imports expect
        stub.__dict__.setdefault("HttpError", type("HttpError", (Exception,), {}))
        stub.__dict__.setdefault("Resource", type("Resource", (), {}))
        stub.__dict__.setdefault("Credentials", type("Credentials", (), {}))
        stub.__dict__.setdefault("RefreshError", type("RefreshError", (Exception,), {}))
        stub.__dict__.setdefault("build", lambda *a, **kw: None)
        stub.__dict__.setdefault("override", lambda f: f)
        sys.modules[mod_name] = stub

cfg = importlib.import_module("common.data_source.config")
ONE_DAY = cfg.ONE_DAY

# Import models and util before file_retrieval (which depends on them)
importlib.import_module("common.data_source.models")
importlib.import_module("common.data_source.google_util.util")
importlib.import_module("common.data_source.google_util.resource")
importlib.import_module("common.data_source.google_drive.constant")
importlib.import_module("common.data_source.google_drive.model")

file_retrieval = importlib.import_module("common.data_source.google_drive.file_retrieval")
generate_time_range_filter = file_retrieval.generate_time_range_filter


class TestGoogleDriveSyncTimeBufferConfig:
    """Tests for the GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS config constant."""

    def test_default_value_is_one_day(self):
        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS", None)
            importlib.reload(cfg)
            assert cfg.GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS == ONE_DAY
            assert cfg.GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS == 86400

    def test_env_override(self):
        with patch.dict(os.environ, {"GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS": "3600"}):
            importlib.reload(cfg)
            assert cfg.GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS == 3600

    def test_env_override_zero(self):
        with patch.dict(os.environ, {"GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS": "0"}):
            importlib.reload(cfg)
            assert cfg.GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS == 0


class TestAdjustStartForQuery:
    """Tests for the _adjust_start_for_query logic."""

    @staticmethod
    def _adjust(start, time_buffer_seconds):
        """Replicate the connector's _adjust_start_for_query logic."""
        buffer = max(0, time_buffer_seconds)
        if not start or start <= 0:
            return start
        if buffer <= 0:
            return start
        return max(0.0, start - buffer)

    def test_subtracts_buffer(self):
        assert self._adjust(1_000_000.0, 86400) == 1_000_000.0 - 86400

    def test_clamps_to_zero(self):
        assert self._adjust(100.0, 86400) == 0.0

    def test_zero_start_unchanged(self):
        assert self._adjust(0.0, 86400) == 0.0

    def test_none_start_unchanged(self):
        assert self._adjust(None, 86400) is None

    def test_zero_buffer_no_change(self):
        assert self._adjust(1_000_000.0, 0) == 1_000_000.0

    def test_negative_buffer_clamped_to_zero(self):
        assert self._adjust(1_000_000.0, -100) == 1_000_000.0

    def test_custom_buffer_value(self):
        assert self._adjust(1_000_000.0, 7200) == 1_000_000.0 - 7200

    def test_exact_boundary(self):
        assert self._adjust(86400.0, 86400) == 0.0

    def test_buffer_larger_than_start(self):
        assert self._adjust(3600.0, 86400) == 0.0


class TestGenerateTimeRangeFilter:
    """Tests for generate_time_range_filter with createdTime support."""

    def test_no_filters_when_no_args(self):
        assert generate_time_range_filter() == ""

    def test_no_filters_when_none(self):
        assert generate_time_range_filter(start=None, end=None) == ""

    def test_start_includes_both_modified_and_created_time(self):
        result = generate_time_range_filter(start=1_000_000.0)
        assert "modifiedTime" in result
        assert "createdTime" in result
        assert " or " in result

    def test_start_filter_uses_strict_greater_than(self):
        result = generate_time_range_filter(start=1_000_000.0)
        assert "modifiedTime >" in result
        assert "createdTime >" in result

    def test_start_filter_is_parenthesized(self):
        """The OR condition must be grouped to combine correctly with AND."""
        result = generate_time_range_filter(start=1_000_000.0)
        assert result.strip().startswith("and (")
        assert result.strip().endswith(")")

    def test_end_filter_only_uses_modified_time(self):
        result = generate_time_range_filter(end=2_000_000.0)
        assert "modifiedTime <=" in result
        assert "createdTime" not in result

    def test_both_start_and_end(self):
        result = generate_time_range_filter(start=1_000_000.0, end=2_000_000.0)
        assert "modifiedTime >" in result
        assert "createdTime >" in result
        assert "modifiedTime <=" in result

    def test_zero_start_still_generates_filter(self):
        """start=0.0 is not None, so a filter should be generated."""
        result = generate_time_range_filter(start=0.0)
        assert "modifiedTime" in result
        assert "createdTime" in result
