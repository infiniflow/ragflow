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

"""Unit tests for the EPUB parser.

Tests cover:
- Parsing a well-formed EPUB with OPF spine ordering
- Fallback parsing when META-INF/container.xml is missing
- Handling of empty or content-less EPUB files
- Spine ordering respects the OPF itemref sequence
- Malformed XML graceful fallback
- Empty binary input handling
"""

import importlib.util
import os
import sys
import zipfile
from io import BytesIO
from unittest import mock

import pytest
from bs4 import ParserRejectedMarkup

# Import RAGFlowEpubParser directly by file path to avoid triggering
# deepdoc/parser/__init__.py which pulls in heavy dependencies
# (pdfplumber, xgboost, etc.) that may not be available in test environments.
_MOCK_MODULES = [
    "xgboost",
    "xgb",
    "pdfplumber",
    "huggingface_hub",
    "PIL",
    "PIL.Image",
    "pypdf",
    "sklearn",
    "sklearn.cluster",
    "sklearn.metrics",
    "deepdoc.vision",
    "infinity",
    "infinity.rag_tokenizer",
]
for _m in _MOCK_MODULES:
    if _m not in sys.modules:
        sys.modules[_m] = mock.MagicMock()


def _find_project_root(marker="pyproject.toml"):
    d = os.path.dirname(os.path.abspath(__file__))
    while d != os.path.dirname(d):
        if os.path.exists(os.path.join(d, marker)):
            return d
        d = os.path.dirname(d)
    return None


_PROJECT_ROOT = _find_project_root()

# Load html_parser first (epub_parser depends on it via relative import)
_html_spec = importlib.util.spec_from_file_location(
    "deepdoc.parser.html_parser",
    os.path.join(_PROJECT_ROOT, "deepdoc", "parser", "html_parser.py"),
)
_html_mod = importlib.util.module_from_spec(_html_spec)
sys.modules["deepdoc.parser.html_parser"] = _html_mod
_html_spec.loader.exec_module(_html_mod)

_epub_spec = importlib.util.spec_from_file_location(
    "deepdoc.parser.epub_parser",
    os.path.join(_PROJECT_ROOT, "deepdoc", "parser", "epub_parser.py"),
)
_epub_mod = importlib.util.module_from_spec(_epub_spec)
sys.modules["deepdoc.parser.epub_parser"] = _epub_mod
_epub_spec.loader.exec_module(_epub_mod)

RAGFlowEpubParser = _epub_mod.RAGFlowEpubParser


def _make_epub(chapters, include_container=True, spine_order=None):
    """Build a minimal EPUB ZIP in memory.

    Args:
        chapters: list of (filename, html_content) tuples.
        include_container: whether to include META-INF/container.xml.
        spine_order: optional list of filenames for spine ordering.
                     Defaults to the order of `chapters`.
    """
    buf = BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("mimetype", "application/epub+zip")

        if include_container:
            container_xml = (
                '<?xml version="1.0" encoding="UTF-8"?>'
                '<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">'
                "  <rootfiles>"
                '    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>'
                "  </rootfiles>"
                "</container>"
            )
            zf.writestr("META-INF/container.xml", container_xml)

            if spine_order is None:
                spine_order = [fn for fn, _ in chapters]

            manifest_items = ""
            for i, (fn, _) in enumerate(chapters):
                manifest_items += f'<item id="ch{i}" href="{fn}" media-type="application/xhtml+xml"/>'

            spine_refs = ""
            fn_to_id = {fn: f"ch{i}" for i, (fn, _) in enumerate(chapters)}
            for fn in spine_order:
                spine_refs += f'<itemref idref="{fn_to_id[fn]}"/>'

            opf_xml = (
                f'<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0">  <manifest>{manifest_items}</manifest>  <spine>{spine_refs}</spine></package>'
            )
            zf.writestr("OEBPS/content.opf", opf_xml)

        for fn, content in chapters:
            path = f"OEBPS/{fn}" if include_container else fn
            zf.writestr(path, content)

    return buf.getvalue()


def _simple_html(body_text):
    return f"<?xml version='1.0' encoding='utf-8'?><html xmlns='http://www.w3.org/1999/xhtml'><head><title>Test</title></head><body><p>{body_text}</p></body></html>"


class TestEpubParserBasic:
    def test_parse_single_chapter(self):
        epub_bytes = _make_epub([("ch1.xhtml", _simple_html("Hello World"))])
        parser = RAGFlowEpubParser()
        sections = parser(None, binary=epub_bytes, chunk_token_num=512)
        assert len(sections) >= 1
        combined = " ".join(sections)
        assert "Hello World" in combined

    def test_parse_multiple_chapters(self):
        chapters = [
            ("ch1.xhtml", _simple_html("Chapter One")),
            ("ch2.xhtml", _simple_html("Chapter Two")),
            ("ch3.xhtml", _simple_html("Chapter Three")),
        ]
        epub_bytes = _make_epub(chapters)
        parser = RAGFlowEpubParser()
        sections = parser(None, binary=epub_bytes, chunk_token_num=512)
        combined = " ".join(sections)
        assert "Chapter One" in combined
        assert "Chapter Two" in combined
        assert "Chapter Three" in combined

    def test_spine_ordering(self):
        """Chapters should be returned in spine order, not filename order."""
        chapters = [
            ("ch1.xhtml", _simple_html("First")),
            ("ch2.xhtml", _simple_html("Second")),
            ("ch3.xhtml", _simple_html("Third")),
        ]
        epub_bytes = _make_epub(chapters, spine_order=["ch3.xhtml", "ch1.xhtml", "ch2.xhtml"])
        parser = RAGFlowEpubParser()
        sections = parser(None, binary=epub_bytes, chunk_token_num=512)
        combined = " ".join(sections)
        assert combined.index("Third") < combined.index("First")
        assert combined.index("First") < combined.index("Second")

    def test_empty_epub(self):
        epub_bytes = _make_epub([])
        parser = RAGFlowEpubParser()
        sections = parser(None, binary=epub_bytes, chunk_token_num=512)
        assert sections == []

    def test_empty_binary(self):
        """Empty bytes should raise ValueError, not trigger file open."""
        parser = RAGFlowEpubParser()
        try:
            parser(None, binary=b"", chunk_token_num=512)
            assert False, "Expected ValueError for empty binary"
        except ValueError:
            pass


class TestEpubParserFallback:
    def test_fallback_without_container(self):
        """When META-INF/container.xml is missing, should fall back to finding .xhtml files."""
        chapters = [
            ("chapter1.xhtml", _simple_html("Fallback Content")),
        ]
        epub_bytes = _make_epub(chapters, include_container=False)
        parser = RAGFlowEpubParser()
        sections = parser(None, binary=epub_bytes, chunk_token_num=512)
        combined = " ".join(sections)
        assert "Fallback Content" in combined

    def test_fallback_on_malformed_container_xml(self):
        """Malformed container.xml should fall back, not raise."""
        buf = BytesIO()
        with zipfile.ZipFile(buf, "w") as zf:
            zf.writestr("mimetype", "application/epub+zip")
            zf.writestr("META-INF/container.xml", "THIS IS NOT XML <><><>")
            zf.writestr("chapter.xhtml", _simple_html("Recovered Content"))

        parser = RAGFlowEpubParser()
        sections = parser(None, binary=buf.getvalue(), chunk_token_num=512)
        combined = " ".join(sections)
        assert "Recovered Content" in combined

    def test_fallback_on_malformed_opf_xml(self):
        """Malformed OPF file should fall back, not raise."""
        buf = BytesIO()
        with zipfile.ZipFile(buf, "w") as zf:
            zf.writestr("mimetype", "application/epub+zip")
            container_xml = (
                '<?xml version="1.0"?>'
                '<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">'
                "  <rootfiles>"
                '    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>'
                "  </rootfiles>"
                "</container>"
            )
            zf.writestr("META-INF/container.xml", container_xml)
            zf.writestr("content.opf", "BROKEN OPF {{{")
            zf.writestr("chapter.xhtml", _simple_html("OPF Fallback"))

        parser = RAGFlowEpubParser()
        sections = parser(None, binary=buf.getvalue(), chunk_token_num=512)
        combined = " ".join(sections)
        assert "OPF Fallback" in combined


class TestEpubParserEdgeCases:
    def test_non_xhtml_spine_items_skipped(self):
        """Non-XHTML items in the spine should be skipped."""
        buf = BytesIO()
        with zipfile.ZipFile(buf, "w") as zf:
            zf.writestr("mimetype", "application/epub+zip")
            container_xml = (
                '<?xml version="1.0"?>'
                '<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">'
                "  <rootfiles>"
                '    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>'
                "  </rootfiles>"
                "</container>"
            )
            zf.writestr("META-INF/container.xml", container_xml)
            opf_xml = (
                '<?xml version="1.0"?>'
                '<package xmlns="http://www.idpf.org/2007/opf" version="3.0">'
                "  <manifest>"
                '    <item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/>'
                '    <item id="img1" href="cover.png" media-type="image/png"/>'
                "  </manifest>"
                "  <spine>"
                '    <itemref idref="ch1"/>'
                '    <itemref idref="img1"/>'
                "  </spine>"
                "</package>"
            )
            zf.writestr("content.opf", opf_xml)
            zf.writestr("ch1.xhtml", _simple_html("Real Content"))
            zf.writestr("cover.png", b"\x89PNG fake image data")

        epub_bytes = buf.getvalue()
        parser = RAGFlowEpubParser()
        sections = parser(None, binary=epub_bytes, chunk_token_num=512)
        combined = " ".join(sections)
        assert "Real Content" in combined

    def test_missing_spine_file(self):
        """If a spine item references a file not in the ZIP, it should be skipped."""
        buf = BytesIO()
        with zipfile.ZipFile(buf, "w") as zf:
            zf.writestr("mimetype", "application/epub+zip")
            container_xml = (
                '<?xml version="1.0"?>'
                '<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">'
                "  <rootfiles>"
                '    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>'
                "  </rootfiles>"
                "</container>"
            )
            zf.writestr("META-INF/container.xml", container_xml)
            opf_xml = (
                '<?xml version="1.0"?>'
                '<package xmlns="http://www.idpf.org/2007/opf" version="3.0">'
                "  <manifest>"
                '    <item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/>'
                '    <item id="ch2" href="missing.xhtml" media-type="application/xhtml+xml"/>'
                "  </manifest>"
                "  <spine>"
                '    <itemref idref="ch1"/>'
                '    <itemref idref="ch2"/>'
                "  </spine>"
                "</package>"
            )
            zf.writestr("content.opf", opf_xml)
            zf.writestr("ch1.xhtml", _simple_html("Existing Chapter"))

        epub_bytes = buf.getvalue()
        parser = RAGFlowEpubParser()
        sections = parser(None, binary=epub_bytes, chunk_token_num=512)
        combined = " ".join(sections)
        assert "Existing Chapter" in combined

    def test_empty_xhtml_file_skipped(self):
        """Empty XHTML files in the EPUB should be skipped without error."""
        buf = BytesIO()
        with zipfile.ZipFile(buf, "w") as zf:
            zf.writestr("mimetype", "application/epub+zip")
            container_xml = (
                '<?xml version="1.0"?>'
                '<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">'
                "  <rootfiles>"
                '    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>'
                "  </rootfiles>"
                "</container>"
            )
            zf.writestr("META-INF/container.xml", container_xml)
            opf_xml = (
                '<?xml version="1.0"?>'
                '<package xmlns="http://www.idpf.org/2007/opf" version="3.0">'
                "  <manifest>"
                '    <item id="ch1" href="empty.xhtml" media-type="application/xhtml+xml"/>'
                '    <item id="ch2" href="real.xhtml" media-type="application/xhtml+xml"/>'
                "  </manifest>"
                "  <spine>"
                '    <itemref idref="ch1"/>'
                '    <itemref idref="ch2"/>'
                "  </spine>"
                "</package>"
            )
            zf.writestr("content.opf", opf_xml)
            zf.writestr("empty.xhtml", b"")
            zf.writestr("real.xhtml", _simple_html("Has Content"))

        parser = RAGFlowEpubParser()
        sections = parser(None, binary=buf.getvalue(), chunk_token_num=512)
        combined = " ".join(sections)
        assert "Has Content" in combined


def _set_encrypted_flag(payload: bytes, member: bytes) -> bytes:
    """Set general purpose bit 0 on `member` in both of its ZIP headers.

    `zipfile` reads an encrypted archive but cannot write one, so the flag is set
    on the raw bytes: offset 6 in the local file header and offset 8 in the
    central directory header.
    """
    import struct

    data = bytearray(payload)
    for signature, name_len_offset, flag_offset, header_len in (
        (b"PK\x03\x04", 26, 6, 30),
        (b"PK\x01\x02", 28, 8, 46),
    ):
        position = 0
        while True:
            position = data.find(signature, position)
            if position < 0:
                break
            name_len = struct.unpack_from("<H", data, position + name_len_offset)[0]
            name = bytes(data[position + header_len : position + header_len + name_len])
            if name == member:
                flags = struct.unpack_from("<H", data, position + flag_offset)[0]
                struct.pack_into("<H", data, position + flag_offset, flags | 0x1)
            position += 4
    return bytes(data)


class TestEpubParserUnreadableChapter:
    """One chapter the parser cannot read must not cost the whole book."""

    _CHAPTERS = [
        ("ch1.xhtml", _simple_html("ALPHA chapter")),
        ("ch2.xhtml", _simple_html("BRAVO chapter")),
        ("ch3.xhtml", _simple_html("CHARLIE chapter")),
    ]

    def _parse(self, epub_bytes):
        """Parse `epub_bytes` and join the sections into one string."""
        return " ".join(RAGFlowEpubParser()(None, binary=epub_bytes, chunk_token_num=512))

    def test_all_three_chapters_when_the_book_is_intact(self):
        """The fixture book parses in full when nothing is broken."""
        combined = self._parse(_make_epub(self._CHAPTERS))

        assert "ALPHA" in combined
        assert "BRAVO" in combined
        assert "CHARLIE" in combined

    def test_a_damaged_chapter_is_skipped(self):
        """`zipfile.read` raises BadZipFile for a member whose CRC does not match."""
        # ZIP_STORED so the payload is findable in the archive bytes.
        buf = BytesIO()
        with zipfile.ZipFile(buf, "w", zipfile.ZIP_STORED) as zf:
            zf.writestr("mimetype", "application/epub+zip")
            zf.writestr(
                "META-INF/container.xml",
                '<?xml version="1.0"?>'
                '<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">'
                '<rootfiles><rootfile full-path="OEBPS/content.opf" '
                'media-type="application/oebps-package+xml"/></rootfiles></container>',
            )
            zf.writestr(
                "OEBPS/content.opf",
                '<?xml version="1.0"?>'
                '<package xmlns="http://www.idpf.org/2007/opf" version="3.0"><manifest>'
                '<item id="ch0" href="ch1.xhtml" media-type="application/xhtml+xml"/>'
                '<item id="ch1" href="ch2.xhtml" media-type="application/xhtml+xml"/>'
                '<item id="ch2" href="ch3.xhtml" media-type="application/xhtml+xml"/>'
                "</manifest><spine>"
                '<itemref idref="ch0"/><itemref idref="ch1"/><itemref idref="ch2"/>'
                "</spine></package>",
            )
            for name, html in self._CHAPTERS:
                zf.writestr(f"OEBPS/{name}", html)

        payload = bytearray(buf.getvalue())
        index = payload.index(b"BRAVO")
        payload[index] ^= 0xFF

        combined = self._parse(bytes(payload))

        assert "ALPHA" in combined
        assert "CHARLIE" in combined
        assert "BRAVO" not in combined

    def test_an_encrypted_chapter_is_skipped(self):
        """`zipfile.read` raises RuntimeError for an entry with the encrypted flag."""
        epub_bytes = _set_encrypted_flag(_make_epub(self._CHAPTERS), b"OEBPS/ch2.xhtml")

        combined = self._parse(epub_bytes)

        assert "ALPHA" in combined
        assert "CHARLIE" in combined

    def test_an_undecodable_chapter_is_skipped(self):
        """`decode_text` refuses a weak codec guess rather than mangling the text."""
        chapters = [
            ("ch1.xhtml", _simple_html("ALPHA chapter")),
            ("ch2.xhtml", bytes(range(256)) * 8),
            ("ch3.xhtml", _simple_html("CHARLIE chapter")),
        ]

        combined = self._parse(_make_epub(chapters))

        assert "ALPHA" in combined
        assert "CHARLIE" in combined

    def test_a_chapter_with_rejected_markup_is_skipped(self):
        """bs4 raises ParserRejectedMarkup when html.parser rejects the markup. Which markup that is depends on the CPython version, so the rejection is simulated."""
        parser_txt = _epub_mod.RAGFlowHtmlParser.parser_txt

        def reject_bravo(txt, chunk_token_num):
            if "BRAVO" in txt:
                raise ParserRejectedMarkup("rejected")
            return parser_txt(txt, chunk_token_num)

        with mock.patch.object(_epub_mod.RAGFlowHtmlParser, "parser_txt", side_effect=reject_bravo):
            combined = self._parse(_make_epub(self._CHAPTERS))

        assert "ALPHA" in combined
        assert "CHARLIE" in combined
        assert "BRAVO" not in combined

    def test_a_chapter_nested_too_deep_to_walk_is_skipped(self):
        """The HTML walker recurses once per element, so enough unclosed tags raise RecursionError."""
        chapters = [
            ("ch1.xhtml", _simple_html("ALPHA chapter")),
            ("ch2.xhtml", _simple_html("<span>BRAVO " * sys.getrecursionlimit())),
            ("ch3.xhtml", _simple_html("CHARLIE chapter")),
        ]

        combined = self._parse(_make_epub(chapters))

        assert "ALPHA" in combined
        assert "CHARLIE" in combined
        assert "BRAVO" not in combined

    def test_a_parser_bug_is_not_skipped_as_an_unreadable_chapter(self):
        """Only failures caused by the chapter itself are skipped; any other error propagates."""
        with mock.patch.object(_epub_mod.RAGFlowHtmlParser, "parser_txt", side_effect=TypeError("parser bug")), pytest.raises(TypeError, match="parser bug"):
            self._parse(_make_epub(self._CHAPTERS))
