import importlib.util
import logging
import sys
from pathlib import Path
from types import ModuleType


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
