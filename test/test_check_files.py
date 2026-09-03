# Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Tests for tools/hooks/check_files.py.

These lock the strict UTF-8 decode behaviour introduced when the hook stopped
using errors="ignore": a file whose bytes are not valid UTF-8 (and which does
not contain a NUL byte, so the binary guard misses it) must be skipped rather
than silently rewritten with dropped bytes when running in --fix mode.
"""

import sys
from pathlib import Path

# pytest's pythonpath includes "." but be defensive about import location.
_ROOT = Path(__file__).resolve().parents[1]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))

from tools.hooks.check_files import (
    check_merge_conflicts,
    check_trailing_whitespace,
)


def test_trailing_whitespace_fix_skips_invalid_utf8_without_corruption(tmp_path):
    # Invalid UTF-8 (no NUL byte) carrying trailing whitespace.
    f = tmp_path / "blob.bin"
    original = b"data \xff\xfe trailing   \n"
    f.write_bytes(original)

    rc = check_trailing_whitespace([f], fix=True)

    # Skipped (no error reported) and, crucially, the bytes are untouched:
    # the old errors="ignore" path would have dropped the invalid bytes and
    # rewritten the file, corrupting it.
    assert rc == 0
    assert f.read_bytes() == original


def test_trailing_whitespace_fix_strips_valid_utf8(tmp_path):
    f = tmp_path / "ok.txt"
    f.write_bytes(b"hello   \n")

    rc = check_trailing_whitespace([f], fix=True)

    assert rc == 0
    assert f.read_bytes() == b"hello\n"


def test_trailing_whitespace_fix_is_noop_on_clean_file(tmp_path):
    f = tmp_path / "ok.txt"
    f.write_bytes(b"clean\n")

    check_trailing_whitespace([f], fix=True)

    assert f.read_bytes() == b"clean\n"


def test_merge_conflicts_skips_binary_file_with_markers(tmp_path):
    # A binary file whose bytes happen to contain ASCII conflict markers.
    f = tmp_path / "blob.bin"
    f.write_bytes(b"\x00<<<<<<< HEAD\n>>>>>>> branch\n")

    rc = check_merge_conflicts([f])

    # Skipped (binary guard), so no false-positive conflict report.
    assert rc == 0


def test_merge_conflicts_detects_real_conflict(tmp_path):
    f = tmp_path / "conflicted.txt"
    f.write_text(
        "<<<<<<< HEAD\nlocal\n=======\nincoming\n>>>>>>> branch\n",
        encoding="utf-8",
    )

    rc = check_merge_conflicts([f])

    # Real conflict markers are still detected.
    assert rc == 1
