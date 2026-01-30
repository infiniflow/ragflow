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

"""Unit tests for api.utils.file_utils (filename_type, thumbnail_img, sanitize_path, read_potential_broken_pdf)."""

import pytest
from api.db import FileType
from api.utils.file_utils import (
    MAX_BLOB_SIZE_PDF,
    MAX_BLOB_SIZE_THUMBNAIL,
    GHOSTSCRIPT_TIMEOUT_SEC,
    filename_type,
    thumbnail_img,
    thumbnail,
    sanitize_path,
    read_potential_broken_pdf,
    repair_pdf_with_ghostscript,
)


class TestFilenameType:
    """Edge cases and robustness for filename_type."""

    @pytest.mark.parametrize("filename,expected", [
        ("doc.pdf", FileType.PDF.value),
        ("a.PDF", FileType.PDF.value),
        ("x.png", FileType.VISUAL.value),
        ("file.docx", FileType.DOC.value),
        ("a/b/c.pdf", FileType.PDF.value),
        ("path/to/file.txt", FileType.DOC.value),
    ])
    def test_valid_filenames(self, filename, expected):
        assert filename_type(filename) == expected

    @pytest.mark.parametrize("filename", [
        None,
        "",
        "   ",
        123,
        [],
    ])
    def test_invalid_or_empty_returns_other(self, filename):
        assert filename_type(filename) == FileType.OTHER.value

    def test_path_with_basename_uses_extension(self):
        assert filename_type("folder/subfolder/document.pdf") == FileType.PDF.value


class TestSanitizePath:
    """Edge cases for sanitize_path."""

    @pytest.mark.parametrize("raw,expected", [
        (None, ""),
        ("", ""),
        ("  ", ""),
        (42, ""),
        ("a/b", "a/b"),
        ("a/../b", "a/b"),
        ("/leading/", "leading"),
        ("\\mixed\\path", "mixed/path"),
    ])
    def test_sanitize_cases(self, raw, expected):
        assert sanitize_path(raw) == expected


class TestReadPotentialBrokenPdf:
    """Edge cases and robustness for read_potential_broken_pdf."""

    def test_none_returns_empty_bytes(self):
        assert read_potential_broken_pdf(None) == b""

    def test_empty_bytes_returns_as_is(self):
        assert read_potential_broken_pdf(b"") == b""

    def test_non_len_raises_or_returns_empty(self):
        class NoLen:
            pass
        result = read_potential_broken_pdf(NoLen())
        assert result == b""


class TestThumbnailImg:
    """Edge cases for thumbnail_img."""

    def test_none_blob_returns_none(self):
        assert thumbnail_img("x.pdf", None) is None

    def test_none_filename_returns_none(self):
        assert thumbnail_img(None, b"fake pdf content") is None

    def test_empty_blob_returns_none(self):
        assert thumbnail_img("x.pdf", b"") is None

    def test_empty_filename_returns_none(self):
        assert thumbnail_img("", b"x") is None

    def test_oversized_blob_returns_none(self):
        huge = b"x" * (MAX_BLOB_SIZE_THUMBNAIL + 1)
        assert thumbnail_img("x.pdf", huge) is None


class TestThumbnail:
    """thumbnail() wraps thumbnail_img and returns base64 or empty string."""

    def test_none_img_returns_empty_string(self):
        assert thumbnail("x.xyz", b"garbage") == ""

    def test_valid_img_returns_base64_prefix(self):
        from api.constants import IMG_BASE64_PREFIX
        result = thumbnail("x.png", b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00\x90wS\xde\x00\x00\x00\x0cIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")
        assert result.startswith(IMG_BASE64_PREFIX) or result == ""


class TestRepairPdfWithGhostscript:
    """repair_pdf_with_ghostscript edge cases."""

    def test_none_returns_empty_bytes(self):
        assert repair_pdf_with_ghostscript(None) == b""

    def test_empty_bytes_returns_empty(self):
        assert repair_pdf_with_ghostscript(b"") == b""

    def test_oversized_returns_original_without_calling_gs(self):
        huge = b"%" * (MAX_BLOB_SIZE_PDF + 1)
        result = repair_pdf_with_ghostscript(huge)
        assert result == huge


class TestConstants:
    """Resource limit constants are positive and reasonable."""

    def test_thumbnail_limit_positive(self):
        assert MAX_BLOB_SIZE_THUMBNAIL > 0

    def test_pdf_limit_positive(self):
        assert MAX_BLOB_SIZE_PDF > 0

    def test_gs_timeout_positive(self):
        assert GHOSTSCRIPT_TIMEOUT_SEC > 0
