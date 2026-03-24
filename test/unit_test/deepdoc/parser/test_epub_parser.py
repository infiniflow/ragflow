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
