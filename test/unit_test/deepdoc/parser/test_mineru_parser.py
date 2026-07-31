import importlib.util
import logging
import sys
from io import BytesIO
from pathlib import Path
from types import ModuleType
import json


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


class _FakeZipResponse:
    """Stand-in for the streaming response returned by requests.post.

    Provides the minimum surface that _run_mineru_api touches: status code,
    headers (Content-Type), and a `.raw` stream that copyfileobj can drain.
    """

    def __init__(self, body: bytes = b"zip-bytes"):
        self._body = body
        self.headers = {"Content-Type": "application/zip"}
        self.raw = BytesIO(body)

    def raise_for_status(self):
        return None


class _FakePostContext:
    def __init__(self, response: _FakeZipResponse, captured: dict):
        self._response = response
        self._captured = captured

    def __enter__(self):
        return self._response

    def __exit__(self, exc_type, exc, tb):
        return False


def _capture_run_mineru_api(monkeypatch, module, *, pdf_path: Path, extracted_dir: Path):
    """Stub everything around requests.post so _run_mineru_api runs end-to-end
    against an in-memory response. Returns the captured kwargs dict.
    """
    captured: dict = {}

    def fake_post(url, files, data, headers, timeout, stream):
        captured["url"] = url
        captured["data"] = data
        captured["files"] = files
        return _FakePostContext(_FakeZipResponse(), captured)

    monkeypatch.setattr(module.requests, "post", fake_post)
    monkeypatch.setattr(module.os.path, "exists", lambda _p: True)
    monkeypatch.setattr(
        module.MinerUParser,
        "_extract_zip_no_root",
        lambda self, *_a, **_kw: None,
    )
    monkeypatch.setattr(
        module.shutil,
        "copyfileobj",
        lambda _src, _dst: None,
    )
    import tempfile

    monkeypatch.setattr(tempfile, "mkdtemp", lambda prefix="", dir=None: str(extracted_dir))
    return captured


def test_run_mineru_api_threads_page_range_into_request_payload(monkeypatch, tmp_path):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser(mineru_api="http://mineru.local")
    parser.mineru_server_url = ""

    pdf_path = tmp_path / "sample.pdf"
    pdf_path.write_bytes(b"%PDF-1.4 fake")
    extracted_dir = tmp_path / "out"
    extracted_dir.mkdir()

    captured = _capture_run_mineru_api(monkeypatch, module, pdf_path=pdf_path, extracted_dir=extracted_dir)
    options = module.MinerUParseOptions()

    # Mid-document range: pages 0..12 inclusive in RAGFlow slice terms.
    parser._run_mineru_api(
        pdf_path,
        extracted_dir,
        options,
        callback=None,
        page_from=0,
        page_to=13,
    )

    assert captured["data"]["start_page_id"] == 0
    assert captured["data"]["end_page_id"] == 12

    # End-of-document range: still need the full doc to come back.
    captured.clear()
    parser._run_mineru_api(
        pdf_path,
        extracted_dir,
        options,
        callback=None,
        page_from=5,
        page_to=20,
    )

    assert captured["data"]["start_page_id"] == 5
    assert captured["data"]["end_page_id"] == 19


def test_run_mineru_api_uses_full_document_when_no_range_given(monkeypatch, tmp_path):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser(mineru_api="http://mineru.local")
    parser.mineru_server_url = ""

    pdf_path = tmp_path / "sample.pdf"
    pdf_path.write_bytes(b"%PDF-1.4 fake")
    extracted_dir = tmp_path / "out"
    extracted_dir.mkdir()

    captured = _capture_run_mineru_api(monkeypatch, module, pdf_path=pdf_path, extracted_dir=extracted_dir)
    options = module.MinerUParseOptions()

    # No page_from/page_to: defaults should keep the prior behavior (0 / 99999).
    parser._run_mineru_api(pdf_path, extracted_dir, options, callback=None)

    assert captured["data"]["start_page_id"] == 0
    assert captured["data"]["end_page_id"] == 99999


def test_end_page_minus_one_normalizes_for_mineru_api(monkeypatch, tmp_path):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser(mineru_api="http://mineru.local")
    parser.mineru_server_url = ""

    pdf_path = tmp_path / "sample.pdf"
    pdf_path.write_bytes(b"%PDF-1.4 fake")
    extracted_dir = tmp_path / "out"
    extracted_dir.mkdir()

    captured = _capture_run_mineru_api(monkeypatch, module, pdf_path=pdf_path, extracted_dir=extracted_dir)
    options = module.MinerUParseOptions()

    # RAGFlow to_page is exclusive (Python slice stop); MinerU end_page_id is
    # 0-based inclusive, so to_page - 1 is the correct translation.
    parser._run_mineru_api(
        pdf_path,
        extracted_dir,
        options,
        callback=None,
        page_from=0,
        page_to=13,
    )

    assert captured["data"]["end_page_id"] == 12


class _FakePageImage:
    def __init__(self, width: int, height: int):
        self.size = (width, height)


def test_read_output_enriches_cross_page_table_positions_from_middle_json(monkeypatch, tmp_path):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    parser.page_images = [_FakePageImage(200, 400), _FakePageImage(200, 400)]

    content_list = [
        {
            "type": module.MinerUContentType.TABLE,
            "table_body": "<table><tr><td>first page row</td></tr><tr><td>second page row</td></tr></table>",
            "table_caption": [],
            "table_footnote": [],
            "bbox": [100, 100, 900, 900],
            "page_idx": 0,
        }
    ]
    middle_json = {
        "pdf_info": [
            {
                "page_idx": 0,
                "page_size": [200, 400],
                "para_blocks": [
                    {
                        "type": "table",
                        "bbox": [20, 40, 180, 360],
                        "blocks": [
                            {
                                "type": "table_body",
                                "lines": [
                                    {
                                        "spans": [
                                            {"type": "table", "content": "first page row", "bbox": [20, 40, 180, 360]},
                                        ]
                                    }
                                ],
                            }
                        ],
                    }
                ],
            },
            {
                "page_idx": 1,
                "page_size": [200, 400],
                "para_blocks": [
                    {
                        "type": "table",
                        "bbox": [20, 0, 180, 80],
                        "blocks": [
                            {
                                "type": "table_body",
                                "lines": [
                                    {
                                        "spans": [
                                            {"type": "table", "content": "second page row", "bbox": [20, 0, 180, 80]},
                                        ]
                                    }
                                ],
                            }
                        ],
                    }
                ],
            },
        ],
    }
    (tmp_path / "sample_content_list.json").write_text(json.dumps(content_list), encoding="utf-8")
    (tmp_path / "sample_middle.json").write_text(json.dumps(middle_json), encoding="utf-8")

    outputs = parser._read_output(tmp_path, "sample", method="auto", backend="pipeline")
    sections = parser._transfer_to_sections(outputs, parse_method="raw", table_enable=True)

    assert len(sections) == 1
    _, line_tag = sections[0]
    assert module.MinerUParser.extract_positions(line_tag) == [
        ([0], 20.0, 180.0, 40.0, 360.0),
        ([1], 20.0, 180.0, 0.0, 80.0),
    ]


def test_read_output_does_not_enrich_non_table_positions_from_middle_json(monkeypatch, tmp_path):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    parser.page_images = [_FakePageImage(200, 400), _FakePageImage(200, 400)]

    content_list = [
        {
            "type": module.MinerUContentType.TEXT,
            "text": "first page row second page row",
            "bbox": [100, 100, 900, 900],
            "page_idx": 0,
        }
    ]
    middle_json = {
        "pdf_info": [
            {
                "page_idx": 0,
                "page_size": [200, 400],
                "para_blocks": [
                    {
                        "type": "text",
                        "bbox": [20, 40, 180, 360],
                        "lines": [{"spans": [{"content": "first page row"}]}],
                    }
                ],
            },
            {
                "page_idx": 1,
                "page_size": [200, 400],
                "para_blocks": [
                    {
                        "type": "text",
                        "bbox": [20, 0, 180, 80],
                        "lines": [{"spans": [{"content": "second page row"}]}],
                    }
                ],
            },
        ],
    }
    (tmp_path / "sample_content_list.json").write_text(json.dumps(content_list), encoding="utf-8")
    (tmp_path / "sample_middle.json").write_text(json.dumps(middle_json), encoding="utf-8")

    outputs = parser._read_output(tmp_path, "sample", method="auto", backend="pipeline")
    sections = parser._transfer_to_sections(outputs, parse_method="raw", table_enable=True)

    assert len(sections) == 1
    _, line_tag = sections[0]
    assert module.MinerUParser.extract_positions(line_tag) == [
        ([0], 20.0, 180.0, 40.0, 360.0),
    ]


def test_middle_positions_ignore_malformed_output_bbox(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    parser.page_images = [_FakePageImage(200, 400)]

    positions = parser._middle_positions_for_output(
        {
            "type": module.MinerUContentType.TABLE,
            "table_body": "<table><tr><td>row</td></tr></table>",
            "table_caption": [],
            "table_footnote": [],
            "bbox": [100, 100, 900],
            "page_idx": 0,
        },
        [
            {
                "type": "table",
                "page_idx": 0,
                "bbox": (20, 40, 180, 360),
                "text": "row",
            }
        ],
    )

    assert positions == []


def test_read_output_keeps_original_tag_when_middle_json_has_single_table_position(monkeypatch, tmp_path):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    parser.page_images = [_FakePageImage(200, 400)]

    content_list = [
        {
            "type": module.MinerUContentType.TABLE,
            "table_body": "<table><tr><td>only row</td></tr></table>",
            "table_caption": [],
            "table_footnote": [],
            "bbox": [100, 100, 850, 850],
            "page_idx": 0,
        }
    ]
    middle_json = {
        "pdf_info": [
            {
                "page_idx": 0,
                "page_size": [200, 400],
                "para_blocks": [
                    {
                        "type": "table",
                        "bbox": [20, 40, 180, 360],
                        "blocks": [
                            {
                                "type": "table_body",
                                "lines": [{"spans": [{"type": "table", "content": "only row"}]}],
                            }
                        ],
                    }
                ],
            }
        ],
    }
    (tmp_path / "sample_content_list.json").write_text(json.dumps(content_list), encoding="utf-8")
    (tmp_path / "sample_middle.json").write_text(json.dumps(middle_json), encoding="utf-8")

    outputs = parser._read_output(tmp_path, "sample", method="auto", backend="pipeline")
    sections = parser._transfer_to_sections(outputs, parse_method="raw", table_enable=True)

    assert "_mineru_positions" not in outputs[0]
    assert len(sections) == 1
    _, line_tag = sections[0]
    assert module.MinerUParser.extract_positions(line_tag) == [
        ([0], 20.0, 170.0, 40.0, 340.0),
    ]
