"""Tests for the Google Drive sync time buffer feature.

Verifies that incremental syncs pull poll_range_start backward by
GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS so that files whose modifiedTime
predates the previous cutoff are not silently excluded.

See: https://github.com/infiniflow/ragflow/issues/13939
"""

from datetime import datetime, timezone
import importlib
import sys
from pathlib import Path
from types import ModuleType

import pytest

pytestmark = pytest.mark.p2

import common

# Bypass common.data_source.__init__.py which eagerly imports heavy deps.
repo_root = Path(__file__).resolve().parents[3]
data_source_pkg = ModuleType("common.data_source")
data_source_pkg.__path__ = [str(repo_root / "common" / "data_source")]
sys.modules["common.data_source"] = data_source_pkg
setattr(common, "data_source", data_source_pkg)

_config = importlib.import_module("common.data_source.config")
ONE_DAY = _config.ONE_DAY
GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS = _config.GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS


# ---------------------------------------------------------------------------
# Config constant
# ---------------------------------------------------------------------------

def test_google_drive_sync_buffer_default_is_one_day():
    """GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS should default to 86400 (1 day)."""
    assert GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS == ONE_DAY
    assert GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS == 86400


def test_google_drive_sync_buffer_is_configurable(monkeypatch):
    """The buffer should be overridable via environment variable."""
    monkeypatch.setenv("GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS", "3600")
    reloaded = importlib.reload(_config)
    assert reloaded.GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS == 3600
    # Restore default
    monkeypatch.delenv("GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS")
    importlib.reload(_config)


# ---------------------------------------------------------------------------
# Buffer arithmetic (mirrors sync_data_source.py GoogleDrive._generate logic)
#
#   raw_start = task["poll_range_start"].timestamp()
#   start_time = max(0.0, raw_start - GOOGLE_DRIVE_SYNC_TIME_BUFFER_SECONDS)
# ---------------------------------------------------------------------------

def _apply_buffer(poll_range_start_ts: float, buffer_seconds: int) -> float:
    """Replicate the buffer logic from GoogleDrive._generate."""
    return max(0.0, poll_range_start_ts - buffer_seconds)


def test_buffer_shifts_start_time_backward():
    """With default 1-day buffer, start should shift back by exactly 86400s."""
    poll_start = datetime(2025, 3, 15, tzinfo=timezone.utc).timestamp()
    buffered = _apply_buffer(poll_start, ONE_DAY)
    expected = datetime(2025, 3, 14, tzinfo=timezone.utc).timestamp()
    assert buffered == pytest.approx(expected)


def test_buffer_does_not_go_negative():
    """If poll_range_start < buffer, clamp to 0.0 (full reindex territory)."""
    small_start = 100.0  # 100 seconds after epoch
    buffered = _apply_buffer(small_start, ONE_DAY)
    assert buffered == 0.0


def test_buffer_zero_start_stays_zero():
    """A start of 0.0 (full reindex) should remain 0.0 after buffering."""
    buffered = _apply_buffer(0.0, ONE_DAY)
    assert buffered == 0.0


def test_buffer_with_custom_value():
    """A 1-hour buffer should shift start back by 3600s."""
    poll_start = datetime(2025, 6, 15, 12, 0, 0, tzinfo=timezone.utc).timestamp()
    one_hour = 3600
    buffered = _apply_buffer(poll_start, one_hour)
    expected = datetime(2025, 6, 15, 11, 0, 0, tzinfo=timezone.utc).timestamp()
    assert buffered == pytest.approx(expected)


def test_buffer_zero_means_no_overlap():
    """Buffer of 0 should leave start_time unchanged."""
    poll_start = datetime(2025, 3, 15, tzinfo=timezone.utc).timestamp()
    buffered = _apply_buffer(poll_start, 0)
    assert buffered == pytest.approx(poll_start)
