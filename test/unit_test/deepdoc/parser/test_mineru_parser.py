import importlib.util
import logging
import sys
from io import BytesIO
from pathlib import Path
from types import ModuleType
from unittest.mock import Mock
import json

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


@pytest.mark.p1
@pytest.mark.parametrize(
    ("language", "expected_language"),
    [
        ("Japanese", "Japanese"),
        ("", "English"),
        (None, "English"),
    ],
)
def test_enhance_images_with_vlm_passes_dataset_language_to_prompt(monkeypatch, tmp_path, language, expected_language):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    image_path = tmp_path / "figure.png"
    module.Image.new("RGB", (1, 1)).save(image_path)

    picture_module = ModuleType("rag.app.picture")
    picture_module.vision_llm_chunk = Mock(return_value="description")
    prompt = Mock(return_value="prompt")
    generator_module = ModuleType("rag.prompts.generator")
    generator_module.vision_llm_figure_describe_prompt = prompt
    monkeypatch.setitem(sys.modules, "rag.app.picture", picture_module)
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", generator_module)

    outputs = [{"type": module.MinerUContentType.IMAGE, "img_path": str(image_path)}]
    parser._enhance_images_with_vlm(outputs, vision_model=object(), language=language)

    prompt.assert_called_once_with(language=expected_language)
    assert outputs[0]["vlm_description"] == "description"


@pytest.mark.p1
@pytest.mark.parametrize(
    ("language", "expected_language"),
    [
        ("Japanese", "Japanese"),
        ("", "English"),
        (None, "English"),
    ],
)
def test_parse_pdf_forwards_normalized_dataset_language_to_image_enhancement(monkeypatch, tmp_path, language, expected_language):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    pdf_path = tmp_path / "document.pdf"
    pdf_path.write_bytes(b"%PDF-1.4 fake")
    output_dir = tmp_path / "output"
    vision_model = object()

    monkeypatch.setattr(module, "extract_pdf_outlines", Mock(return_value=[]))
    monkeypatch.setattr(parser, "__images__", Mock())
    monkeypatch.setattr(parser, "_run_mineru", Mock(return_value=output_dir))
    monkeypatch.setattr(parser, "_read_output", Mock(return_value=[]))
    enhance = Mock()
    monkeypatch.setattr(parser, "_enhance_images_with_vlm", enhance)

    language_kwargs = {} if language is None else {"lang": language}
    parser.parse_pdf(
        filepath=pdf_path,
        binary=None,
        output_dir=str(output_dir),
        delete_output=False,
        vision_model=vision_model,
        **language_kwargs,
    )

    enhance.assert_called_once_with([], vision_model, callback=None, language=expected_language)


def test_sanitize_section_text_removes_escaped_html_tags(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    text = "&lt;table&gt;&lt;tr&gt;&lt;td&gt;Alpha&lt;/td&gt;&lt;td&gt;Beta&lt;/td&gt;&lt;/tr&gt;&lt;/table&gt;"

    sanitized = module.MinerUParser._sanitize_section_text(text)

    assert sanitized == "AlphaBeta"
    assert "<td>" not in sanitized
    assert "</td>" not in sanitized


def test_parse_pdf_emits_image_coverage_final_log_with_vlm_configured_flag(monkeypatch, tmp_path, caplog):
    """Regression for xugangqiang's review on #16978: parse_pdf must
    emit the final ``[MinerU] image_coverage final ... vlm_configured=...``
    log line so operators can see the image-coverage stamp at the parser
    boundary, including whether a vision model was configured."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    pdf_path = tmp_path / "document.pdf"
    pdf_path.write_bytes(b"%PDF-1.4 fake")
    output_dir = tmp_path / "output"
    # Three images: one captioned, one dropped, one VLM-described.
    outputs = [
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": ["Exhibit A"],
            "image_footnote": [],
            "img_path": "/tmp/a.jpg",
            "page_idx": 0,
            "bbox": (0, 0, 10, 10),
        },
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": [],
            "image_footnote": [],
            "img_path": "/tmp/b.jpg",
            "page_idx": 1,
            "bbox": (0, 0, 10, 10),
        },
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": [],
            "image_footnote": [],
            "img_path": "/tmp/c.jpg",
            "page_idx": 2,
            "bbox": (0, 0, 10, 10),
            "vlm_description": "VLM description for image C.",
        },
    ]

    monkeypatch.setattr(module, "extract_pdf_outlines", Mock(return_value=[]))
    monkeypatch.setattr(parser, "__images__", Mock())
    monkeypatch.setattr(parser, "_run_mineru", Mock(return_value=output_dir))
    monkeypatch.setattr(parser, "_read_output", Mock(return_value=outputs))
    monkeypatch.setattr(parser, "_enhance_images_with_vlm", Mock())
    monkeypatch.setattr(parser, "_transfer_to_tables", Mock(return_value=[]))
    # Do NOT mock _transfer_to_sections — the real method must run so it
    # populates parser.last_image_coverage with the actual detected/chunked/
    # described/dropped counts before the summary log line is emitted.

    with caplog.at_level(logging.INFO, logger=parser.logger.name):
        parser.parse_pdf(
            filepath=pdf_path,
            binary=None,
            output_dir=str(output_dir),
            delete_output=False,
            vision_model=object(),  # a configured vision model
        )

    # The summary log line must include the vlm_configured flag and
    # reflect the actual coverage the real _transfer_to_sections
    # accumulated.
    assert "image_coverage final" in caplog.text
    assert "vlm_configured=True" in caplog.text
    assert "detected=3" in caplog.text
    assert "described=1" in caplog.text


def test_parse_pdf_image_coverage_final_log_reflects_no_vision_model(monkeypatch, tmp_path, caplog):
    """When parse_pdf is called without a vision_model kwarg, the final
    log must reflect vlm_configured=False so operators know downstream
    chunks could NOT have been VLM-enriched."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    pdf_path = tmp_path / "document.pdf"
    pdf_path.write_bytes(b"%PDF-1.4 fake")
    output_dir = tmp_path / "output"

    monkeypatch.setattr(module, "extract_pdf_outlines", Mock(return_value=[]))
    monkeypatch.setattr(parser, "__images__", Mock())
    monkeypatch.setattr(parser, "_run_mineru", Mock(return_value=output_dir))
    monkeypatch.setattr(parser, "_read_output", Mock(return_value=[]))
    monkeypatch.setattr(parser, "_enhance_images_with_vlm", Mock())
    monkeypatch.setattr(parser, "_transfer_to_tables", Mock(return_value=[]))

    with caplog.at_level(logging.INFO, logger=parser.logger.name):
        parser.parse_pdf(
            filepath=pdf_path,
            binary=None,
            output_dir=str(output_dir),
            delete_output=False,
            # no vision_model kwarg
        )

    assert "image_coverage final" in caplog.text
    assert "vlm_configured=False" in caplog.text


def test_transfer_to_sections_logs_sections_dropped_after_sanitization(monkeypatch, caplog):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.TABLE,
            "table_body": "&lt;td&gt;&lt;/td&gt;",
            "table_caption": [],
            "table_footnote": [],
            "page_idx": 0,
            "bbox": (0, 0, 1, 1),
        }
    ]

    with caplog.at_level(logging.DEBUG, logger=parser.logger.name):
        sections = parser._transfer_to_sections(outputs, parse_method="pipeline")

    assert sections == []
    assert "Skip section after sanitization" in caplog.text
    assert f"type={module.MinerUContentType.TABLE}" in caplog.text


@pytest.mark.p1
@pytest.mark.parametrize(
    ("output", "expected"),
    [
        ({"type": "text", "text": "Use List<String> and a<b. 5 &lt; 6"}, "Use List<String> and a<b. 5 &lt; 6"),
        ({"type": "code", "code_body": "template<typename T> void f();", "code_caption": []}, "template<typename T> void f();"),
        ({"type": "equation", "text": "x < y > z"}, "x < y > z"),
    ],
)
def test_transfer_to_sections_only_sanitizes_tables(monkeypatch, output, expected):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    output = {**output, "page_idx": 0, "bbox": (0, 0, 1, 1)}

    sections = parser._transfer_to_sections([output], parse_method="raw", table_enable=False)

    assert sections[0][0] == expected


@pytest.mark.p1
@pytest.mark.parametrize("transfer", ["sections", "tables"])
def test_empty_table_fallback_is_logged(monkeypatch, caplog, transfer):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    output = {
        "type": module.MinerUContentType.TABLE,
        "table_body": "",
        "table_caption": [],
        "table_footnote": [],
        "page_idx": 2,
        "bbox": (0, 0, 1, 1),
    }

    with caplog.at_level(logging.WARNING, logger=parser.logger.name):
        if transfer == "sections":
            result = parser._transfer_to_sections([output], parse_method="raw", table_enable=True)
            fallback = result[0][0]
        else:
            result = parser._transfer_to_tables([output])
            fallback = result[0][0][1]

    assert fallback == "FAILED TO PARSE TABLE"
    assert "Empty table content at page_idx=2; using fallback text." in caplog.text


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


@pytest.mark.p1
def test_transfer_to_tables_emits_ordered_typed_media(monkeypatch, tmp_path):
    from rag.nlp import tokenize_table

    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    image_path = tmp_path / "figure.png"
    module.Image.new("RGB", (2, 2), "red").save(image_path)
    outputs = [
        {
            "type": module.MinerUContentType.TABLE,
            "table_body": "<table><tr><td>first</td></tr></table>",
            "table_caption": [],
            "table_footnote": [],
            "page_idx": 0,
            "bbox": (1, 2, 3, 4),
        },
        {
            "type": module.MinerUContentType.IMAGE,
            "img_path": str(image_path),
            "image_caption": ["Figure 1"],
            "image_footnote": ["Source"],
            "vlm_description": "A red square",
            "page_idx": 0,
            "bbox": (5, 6, 7, 8),
        },
        {
            "type": module.MinerUContentType.TABLE,
            "table_body": "second table",
            "table_caption": [],
            "table_footnote": [],
            "page_idx": 1,
            "bbox": (9, 10, 11, 12),
        },
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": ["Caption without image"],
            "image_footnote": [],
        },
    ]

    media = parser._transfer_to_tables(outputs)

    assert len(media) == 3
    assert media[0][0] == (None, "<table><tr><td>first</td></tr></table>")
    assert media[2][0] == (None, "second table")
    image, texts = media[1][0]
    image_path.unlink()
    assert isinstance(image, module.Image.Image)
    assert image.getpixel((0, 0)) == (255, 0, 0)
    assert texts == ["Figure 1", "Source", "A red square"]
    assert [chunk["doc_type_kwd"] for chunk in tokenize_table(media, {}, False)] == ["table", "image", "table"]


@pytest.mark.p1
def test_tokenize_table_uses_payload_type_instead_of_html_content():
    from PIL import Image

    from rag.nlp import tokenize_table

    image = Image.new("RGB", (1, 1))
    media = [((image, "plain table"), []), ((image, ["caption with <tr> text"]), [])]

    chunks = tokenize_table(media, {}, False)

    assert [chunk["doc_type_kwd"] for chunk in chunks] == ["table", "image"]
    assert [chunk["image"] for chunk in chunks] == [image, image]


@pytest.mark.p1
def test_media_context_preserves_media_without_positions(monkeypatch):
    from rag.nlp import append_context2table_image4pdf

    parser_module = ModuleType("deepdoc.parser")
    parser_module.PdfParser = Mock()
    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_module)

    image = object()
    media = [((None, "table"), []), ((image, ["figure"]), [])]

    assert append_context2table_image4pdf([], media, 1) == media
    assert append_context2table_image4pdf([], media, 1, return_context=True) == [("", ""), ("", "")]


@pytest.mark.p1
@pytest.mark.parametrize("parse_method", ["naive", "manual", "paper"])
def test_transfer_to_sections_routes_app_media_separately(monkeypatch, parse_method):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {"type": module.MinerUContentType.TEXT, "text": "Body", "page_idx": 0, "bbox": (0, 0, 1, 1)},
        {
            "type": module.MinerUContentType.TABLE,
            "table_body": "table",
            "table_caption": [],
            "table_footnote": [],
            "page_idx": 0,
            "bbox": (0, 1, 1, 2),
        },
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": ["figure"],
            "image_footnote": [],
            "page_idx": 0,
            "bbox": (0, 2, 1, 3),
        },
    ]

    sections = parser._transfer_to_sections(outputs, parse_method=parse_method, table_enable=True)

    assert len(sections) == 1
    assert sections[0][0].startswith("Body")
    assert len(parser._transfer_to_sections(outputs, parse_method="raw", table_enable=True)) == 3


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


def test_transfer_to_sections_tracks_image_coverage_for_captioned_image(monkeypatch, caplog):
    """An IMAGE block with a caption should count as detected and chunked, but
    not as described (no VLM). The coverage dict should be visible on the
    parser instance so callers can inspect silent loss (issue #16978)."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": ["Exhibit A: signature page"],
            "image_footnote": [],
            "img_path": "/tmp/example.jpg",
            "page_idx": 0,
            "bbox": (10, 10, 100, 100),
        }
    ]

    with caplog.at_level(logging.INFO, logger=parser.logger.name):
        sections = parser._transfer_to_sections(outputs, parse_method="raw")

    assert len(sections) == 1
    # The upstream section construction joins caption + "\n" + footnote, so a
    # captioned image with no footnote yields a trailing newline; the
    # substantive text must still be retrievable.
    assert "Exhibit A: signature page" in sections[0][0]
    coverage = parser.last_image_coverage
    assert coverage == {
        "images_detected": 1,
        "images_chunked": 1,
        "images_dropped_no_text": 0,
        "images_described": 0,
    }
    assert "image_coverage detected=1 chunked=1 described=0 dropped_no_text=0" in caplog.text


def test_transfer_to_sections_warns_when_embedded_image_has_no_text(monkeypatch, caplog):
    """When an embedded PDF image has no caption, no footnote, and no VLM
    description, it must not silently disappear from the chunk stream —
    the loss must be visible in logs and the coverage stamp."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": [],
            "image_footnote": [],
            "img_path": "/tmp/silent_drop.jpg",
            "page_idx": 2,
            "bbox": (0, 0, 50, 50),
        }
    ]

    with caplog.at_level(logging.WARNING, logger=parser.logger.name):
        sections = parser._transfer_to_sections(outputs, parse_method="raw")

    assert sections == []
    coverage = parser.last_image_coverage
    assert coverage == {
        "images_detected": 1,
        "images_chunked": 0,
        "images_dropped_no_text": 1,
        "images_described": 0,
    }
    assert "Dropped embedded image" in caplog.text
    assert "page_idx=2" in caplog.text
    assert "Configure an IMAGE2TEXT vision model" in caplog.text


def test_transfer_to_sections_counts_vlm_description(monkeypatch):
    """An IMAGE block enriched by _enhance_images_with_vlm must be counted
    as described so operators can tell the gap between detected and
    described."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": [],
            "image_footnote": [],
            "img_path": "/tmp/described.jpg",
            "page_idx": 0,
            "bbox": (0, 0, 10, 10),
            "vlm_description": "A scanned signature in cursive.",
        }
    ]

    sections = parser._transfer_to_sections(outputs, parse_method="raw")

    assert len(sections) == 1
    assert "A scanned signature in cursive." in sections[0][0]
    coverage = parser.last_image_coverage
    assert coverage["images_detected"] == 1
    assert coverage["images_chunked"] == 1
    assert coverage["images_described"] == 1
    assert coverage["images_dropped_no_text"] == 0


def test_transfer_to_sections_mixed_image_lifecycle(monkeypatch, caplog):
    """Mixed batch: one captioned, one described, one dropped. The coverage
    stamp must reflect the truth so the regression fixture can assert on
    the loss invariant (images_detected >= images_chunked)."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": ["Caption 1"],
            "image_footnote": [],
            "img_path": "/tmp/c1.jpg",
            "page_idx": 0,
            "bbox": (0, 0, 10, 10),
        },
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": [],
            "image_footnote": [],
            "img_path": "/tmp/d1.jpg",
            "page_idx": 1,
            "bbox": (0, 0, 10, 10),
            "vlm_description": "Described by VLM.",
        },
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": [],
            "image_footnote": [],
            "img_path": "/tmp/x1.jpg",
            "page_idx": 2,
            "bbox": (0, 0, 10, 10),
        },
        {
            "type": module.MinerUContentType.TEXT,
            "text": "Body text between images.",
            "page_idx": 2,
            "bbox": (0, 0, 10, 10),
        },
    ]

    with caplog.at_level(logging.INFO, logger=parser.logger.name):
        sections = parser._transfer_to_sections(outputs, parse_method="raw")

    assert len(sections) == 3  # captioned + described + text (the dropped image yields no section)
    coverage = parser.last_image_coverage
    assert coverage == {
        "images_detected": 3,
        "images_chunked": 2,
        "images_dropped_no_text": 1,
        "images_described": 1,
    }
    assert "image_coverage detected=3 chunked=2 described=1 dropped_no_text=1" in caplog.text
    assert "Dropped embedded image" in caplog.text


# --------------------------------------------------------------------------- #
# Review follow-ups (issue #16978): naive/manual/paper modes + parse_pdf log line
# --------------------------------------------------------------------------- #
#
# xugangqiang's review flagged that the previous tests only covered
# parse_method="raw" — exactly the mode where the coverage tracking was
# already working. The naive/manual/paper modes use a separate
# _transfer_to_tables path and the early-skip branch above the
# match/case block, so the original `images_detected += 1` increment
# (inside the IMAGE case) was never reached and these modes silently
# reported images_detected=0.
#
# The fix moves `images_detected += 1` above the early-skip branch.
# These tests pin the new behavior for every parse_method.


@pytest.mark.parametrize(
    "parse_method",
    ["naive", "manual", "paper"],
)
def test_transfer_to_sections_counts_images_detected_for_app_media_modes(monkeypatch, parse_method):
    """Regression for xugangqiang's review on #16978: naive, manual,
    and paper all route IMAGE blocks through _transfer_to_tables and
    skip them in _transfer_to_sections. The image_coverage stamp must
    still report images_detected correctly for these modes so operators
    can see how many images the parser saw."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": ["A"],
            "image_footnote": [],
            "img_path": "/tmp/a.jpg",
            "page_idx": 0,
            "bbox": (0, 0, 10, 10),
        },
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": ["B"],
            "image_footnote": [],
            "img_path": "/tmp/b.jpg",
            "page_idx": 1,
            "bbox": (0, 0, 10, 10),
        },
    ]

    # _transfer_to_sections returns no IMAGE sections for app-media
    # modes — they go to _transfer_to_tables instead.
    sections = parser._transfer_to_sections(outputs, parse_method=parse_method)
    assert sections == []

    # But the coverage stamp must still report both images detected so
    # operators can spot the gap between detection and chunking.
    coverage = parser.last_image_coverage
    assert coverage["images_detected"] == 2
    # The IMAGE blocks produced no section because the parser skipped
    # them — they are not "dropped_no_text" (that count tracks images
    # that reached the section-building branch with no caption/footnote),
    # they are simply not chunked via _transfer_to_sections.
    assert coverage["images_chunked"] == 0
    assert coverage["images_dropped_no_text"] == 0


def test_transfer_to_sections_app_media_mode_with_image_dropped_after_sanitization(monkeypatch, caplog):
    """Regression for #16978: even in naive/manual/paper modes, an IMAGE
    block whose caption/footnote/VLM description is empty would still
    register as `images_dropped_no_text` if it ever reached the
    section-building branch. App-media modes skip IMAGE entirely before
    the branch, so this case is a no-op — but the helper must not
    raise or misreport counters."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": [],
            "image_footnote": [],
            "img_path": "/tmp/silent.jpg",
            "page_idx": 0,
            "bbox": (0, 0, 10, 10),
        }
    ]

    with caplog.at_level(logging.WARNING, logger=parser.logger.name):
        parser._transfer_to_sections(outputs, parse_method="naive")

    coverage = parser.last_image_coverage
    # Detected is correct (1); the IMAGE block was skipped at the
    # naive/manual/paper branch, not at the empty-section branch.
    assert coverage["images_detected"] == 1
    assert coverage["images_dropped_no_text"] == 0
    # App-media modes do not warn per-image — _transfer_to_tables handles
    # them instead, so the warning channel here stays quiet.
    assert "Dropped embedded image" not in caplog.text


def test_mineruparser_initializes_last_image_coverage_in_constructor(monkeypatch):
    """Regression for xugangqiang's review on #16978: the attribute
    must exist from construction, independent of whether parse_pdf has
    run yet — no more getattr(self, ..., None) fallback in the
    caller."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()

    # Direct attribute access — not a getattr with a default — must work.
    assert isinstance(parser.last_image_coverage, dict)
    assert parser.last_image_coverage == {
        "images_detected": 0,
        "images_chunked": 0,
        "images_described": 0,
        "images_dropped_no_text": 0,
    }


def test_transfer_to_sections_app_media_image_with_only_vlm_description(monkeypatch, caplog):
    """Even when an IMAGE block has a vlm_description and no
    caption/footnote, app-media modes (naive/manual/paper) must still
    register the image as detected so the operator sees the count
    matches what the VLM model produced."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {
            "type": module.MinerUContentType.IMAGE,
            "image_caption": [],
            "image_footnote": [],
            "img_path": "/tmp/vlm.jpg",
            "page_idx": 0,
            "bbox": (0, 0, 10, 10),
            "vlm_description": "A description from the vision model.",
        }
    ]

    sections = parser._transfer_to_sections(outputs, parse_method="naive")
    assert sections == []

    coverage = parser.last_image_coverage
    # The VLM description is still counted because the IMAGE block was
    # observed (detected=1). The describe/chunked counters are not
    # incremented for app-media modes because _transfer_to_sections
    # skipped the section-building branch — the vlm_description is
    # consumed by _transfer_to_tables instead.
    assert coverage["images_detected"] == 1
