import importlib.util
import logging
import sys
from pathlib import Path
from types import ModuleType

import pytest


def _load_mineru_parser(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    deepdoc_mod = ModuleType("deepdoc")
    deepdoc_mod.__path__ = [str(repo_root / "deepdoc")]
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_mod)

    parser_mod = ModuleType("deepdoc.parser")
    parser_mod.__path__ = [str(repo_root / "deepdoc" / "parser")]
    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_mod)

    pdf_parser_mod = ModuleType("deepdoc.parser.pdf_parser")

    class _RAGFlowPdfParser:
        pass

    pdf_parser_mod.RAGFlowPdfParser = _RAGFlowPdfParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.pdf_parser", pdf_parser_mod)

    utils_mod = ModuleType("deepdoc.parser.utils")
    utils_mod.extract_pdf_outlines = lambda *_args, **_kwargs: []
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", utils_mod)

    module_name = "test_mineru_parser_unit_module"
    module_path = repo_root / "deepdoc" / "parser" / "mineru_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def test_sanitize_section_text_removes_escaped_html_tags(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    text = "&lt;table&gt;&lt;tr&gt;&lt;td&gt;Alpha&lt;/td&gt;&lt;td&gt;Beta&lt;/td&gt;&lt;/tr&gt;&lt;/table&gt;"

    sanitized = module.MinerUParser._sanitize_section_text(text)

    assert sanitized == "AlphaBeta"
    assert "<td>" not in sanitized
    assert "</td>" not in sanitized


def test_transfer_to_sections_logs_sections_dropped_after_sanitization(monkeypatch, caplog):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.TEXT,
            "text": "&lt;td&gt;&lt;/td&gt;",
            "page_idx": 0,
            "bbox": (0, 0, 1, 1),
        }
    ]

    with caplog.at_level(logging.DEBUG, logger=parser.logger.name):
        sections = parser._transfer_to_sections(outputs, parse_method="pipeline")

    assert sections == []
    assert "Skip section after sanitization" in caplog.text
    assert f"type={module.MinerUContentType.TEXT}" in caplog.text


def test_transfer_to_sections_skips_page_chrome_without_duplicating_text(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    fixture_path = Path(__file__).resolve().parents[3] / "fixtures" / "mineru" / "bmw_page_chrome_content_list.json"
    outputs = __import__("json").loads(fixture_path.read_text(encoding="utf-8"))

    sections = parser._transfer_to_sections(outputs, parse_method="raw")
    texts = [section[0] for section in sections]

    assert texts == ["打开和关闭", "车辆装备", "车辆钥匙", "概述", "安全提示"]
    assert texts.count("打开和关闭") == 1
    assert texts.count("概述") == 1
    assert "77" not in texts
    assert "Online Edition for Part no." not in " ".join(texts)


class _FakeImage:
    """Stand-in for a rendered page image exposing only ``.size`` (w, h)."""

    def __init__(self, width, height):
        self.size = (width, height)


def test_line_tag_scales_normalized_bbox_to_page_size(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    parser.page_images = [_FakeImage(595, 842)]

    tag = parser._line_tag({"page_idx": 0, "type": "text", "bbox": [100, 450, 500, 480]})

    # 100/1000*595=59.5, 500/1000*595=297.5, 450/1000*842=378.9, 480/1000*842=404.16
    assert tag == "@@1\t59.5\t297.5\t378.9\t404.2##"


def test_line_tag_falls_back_to_captured_page_size_when_image_missing(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    # Page render produced no images, but __images__ captured the page size.
    parser.page_images = None
    parser.page_sizes = [(595, 842)]

    tag = parser._line_tag({"page_idx": 0, "type": "text", "bbox": [100, 450, 500, 480]})

    assert tag == "@@1\t59.5\t297.5\t378.9\t404.2##"


def test_line_tag_uses_default_page_size_instead_of_raw_coords(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    # Neither images nor captured sizes available (render fully failed).
    parser.page_images = None
    parser.page_sizes = None

    tag = parser._line_tag({"page_idx": 0, "type": "text", "bbox": [100, 450, 500, 480]})

    # Must NOT emit raw 0-1000 values; scales by DEFAULT_PAGE_SIZE (595x842).
    assert tag == "@@1\t59.5\t297.5\t378.9\t404.2##"
    assert "100.0\t500.0" not in tag


def test_build_table_fragment_index_normalizes_and_groups_by_page(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    middle = {
        "pdf_info": [
            {"page_idx": 0, "page_size": [595, 842], "para_blocks": [{"type": "table", "bbox": [59.5, 421, 297.5, 757.8]}, {"type": "text", "bbox": [0, 0, 10, 10]}]},
            {"page_idx": 1, "page_size": [595, 842], "para_blocks": []},
        ]
    }
    index = parser._build_table_fragment_index(middle)
    assert set(index) == {0}  # only the page with a table fragment
    assert len(index[0]) == 1
    assert index[0][0] == pytest.approx([100.0, 500.0, 500.0, 900.0])  # normalized to 0-1000


def test_build_table_fragment_index_returns_empty_on_bad_schema(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    assert parser._build_table_fragment_index(None) == {}
    assert parser._build_table_fragment_index({"unexpected": 1}) == {}
    assert parser._build_table_fragment_index({"pdf_info": [{"no_size": True}]}) == {}


def test_cross_page_table_emits_multi_page_tags(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    parser.page_images = [_FakeImage(595, 842) for _ in range(3)]
    middle = {
        "pdf_info": [
            {"page_idx": 0, "page_size": [595, 842], "para_blocks": [{"type": "table", "bbox": [48, 560, 560, 838]}]},  # bottom of p0
            {"page_idx": 1, "page_size": [595, 842], "para_blocks": [{"type": "table", "bbox": [48, 12, 560, 840]}]},  # full p1
            {"page_idx": 2, "page_size": [595, 842], "para_blocks": [{"type": "table", "bbox": [48, 12, 560, 150]}]},  # top of p2
        ]
    }
    parser._table_fragment_index = parser._build_table_fragment_index(middle)

    # content_list keeps only the FIRST fragment's (page-0) normalized bbox.
    first = parser._normalize_bbox([48, 560, 560, 838], 595, 842)
    cl_bbox = [int(v) for v in first]
    tag = parser._line_tag({"page_idx": 0, "type": "table", "bbox": cl_bbox})

    poss = module.MinerUParser.extract_positions(tag)
    assert [p[0][0] for p in poss] == [0, 1, 2]  # spans all three pages (0-based)
    assert tag.count("@@") == 3


def test_table_without_middle_json_index_stays_single_page(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    parser.page_images = [_FakeImage(595, 842)]
    # No fragment index built -> behaves exactly like a single-page block.
    tag = parser._line_tag({"page_idx": 0, "type": "table", "bbox": [80, 150, 920, 970]})
    assert tag.count("@@") == 1
    assert tag == "@@1\t47.6\t547.4\t126.3\t816.7##"


def test_transfer_to_sections_skips_unknown_types_without_duplicating_text(monkeypatch, caplog):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.TEXT,
            "text": "Primary content",
            "page_idx": 0,
            "bbox": (0, 0, 1, 1),
        },
        {
            "type": "sidebar",
            "text": "Should not repeat previous section",
            "page_idx": 0,
            "bbox": (0, 0, 1, 1),
        },
        {
            "type": module.MinerUContentType.TEXT,
            "text": "Next content",
            "page_idx": 0,
            "bbox": (0, 0, 1, 1),
        },
    ]

    with caplog.at_level(logging.DEBUG, logger=parser.logger.name):
        sections = parser._transfer_to_sections(outputs, parse_method="raw")

    assert [section[0] for section in sections] == ["Primary content", "Next content"]
    assert "Skip unsupported section type=sidebar" in caplog.text
