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

"""Unit tests for magic-byte helpers in rag/utils/file_utils.py."""

import io
import zipfile

from rag.utils.file_utils import _guess_ext, _is_ole, _is_pdf, _is_zip, _sha10


def _make_zip(names):
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        for n in names:
            z.writestr(n, b"payload")
    return buf.getvalue()


class TestIsZip:
    def test_local_file_header(self):
        assert _is_zip(b"PK\x03\x04rest") is True

    def test_empty_archive_header(self):
        assert _is_zip(b"PK\x05\x06rest") is True

    def test_spanned_archive_header(self):
        assert _is_zip(b"PK\x07\x08rest") is True

    def test_rejects_pdf_and_short_prefix(self):
        assert _is_zip(b"%PDF-1.7") is False
        assert _is_zip(b"PK") is False
        assert _is_zip(b"") is False


class TestIsPdf:
    def test_pdf_magic(self):
        assert _is_pdf(b"%PDF-1.7 content") is True

    def test_rejects_zip_and_empty(self):
        assert _is_pdf(b"PK\x03\x04rest") is False
        assert _is_pdf(b"") is False


class TestIsOle:
    OLE_MAGIC = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"

    def test_ole_magic(self):
        assert _is_ole(self.OLE_MAGIC + b"rest") is True

    def test_rejects_truncated_and_other(self):
        assert _is_ole(self.OLE_MAGIC[:7]) is False
        assert _is_ole(b"%PDF-1.7") is False
        assert _is_ole(b"") is False


class TestSha10:
    def test_ten_hex_chars_and_deterministic(self):
        digest = _sha10(b"hello")
        assert len(digest) == 10
        assert all(c in "0123456789abcdef" for c in digest)
        assert _sha10(b"hello") == digest

    def test_known_vector_and_distinct_inputs(self):
        assert _sha10(b"abc") == "ba7816bf8f"
        assert _sha10(b"abc") != _sha10(b"abd")
        assert _sha10(b"") != _sha10(b"\x00")


class TestGuessExt:
    def test_pdf(self):
        assert _guess_ext(b"%PDF-1.7\n%...") == ".pdf"

    def test_ole_doc(self):
        assert _guess_ext(b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1rest") == ".doc"

    def test_unknown_and_empty_fall_back_to_bin(self):
        assert _guess_ext(b"hello world") == ".bin"
        assert _guess_ext(b"") == ".bin"
        assert _guess_ext(b"PK") == ".bin"

    def test_corrupt_zip_magic_falls_back_to_zip(self):
        assert _guess_ext(b"PK\x03\x04not-a-zip") == ".zip"

    def test_docx_xlsx_pptx_routing(self):
        assert _guess_ext(_make_zip(["word/document.xml"])) == ".docx"
        assert _guess_ext(_make_zip(["xl/workbook.xml"])) == ".xlsx"
        assert _guess_ext(_make_zip(["ppt/presentation.xml"])) == ".pptx"

    def test_container_names_match_case_insensitively(self):
        assert _guess_ext(_make_zip(["WORD/document.xml"])) == ".docx"

    def test_plain_zip_without_office_dirs(self):
        assert _guess_ext(_make_zip(["readme.txt"])) == ".zip"
