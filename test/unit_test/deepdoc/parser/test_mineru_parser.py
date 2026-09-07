import importlib.util
import logging
import sys
from io import BytesIO
from pathlib import Path
from types import ModuleType
from unittest.mock import Mock, call
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


def test_parse_pdf_forwards_page_range_and_callback_to_page_rendering(monkeypatch, tmp_path):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    pdf_path = tmp_path / "document.pdf"
    pdf_path.write_bytes(b"%PDF-1.4 fake")
    output_dir = tmp_path / "output"
    callback = Mock()
    render_pages = Mock()

    monkeypatch.setattr(module, "extract_pdf_outlines", Mock(return_value=[]))
    monkeypatch.setattr(parser, "__images__", render_pages)
    monkeypatch.setattr(parser, "_run_mineru", Mock(return_value=output_dir))
    monkeypatch.setattr(parser, "_read_output", Mock(return_value=[]))

    parser.parse_pdf(
        filepath=str(pdf_path),
        binary=None,
        callback=callback,
        output_dir=str(output_dir),
        delete_output=False,
        page_from=12,
        page_to=15,
    )

    render_pages.assert_called_once_with(pdf_path, zoomin=1, page_from=12, page_to=15, callback=callback)


def test_page_rendering_only_renders_requested_range(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()

    class _Page:
        def __init__(self, page_number):
            self.page_number = page_number

        def to_image(self, **_kwargs):
            rendered_pages.append(self.page_number)
            return type("RenderedPage", (), {"original": module.Image.new("RGB", (10, 20))})()

    class _Pdf:
        pages = [_Page(page_number) for page_number in range(15)]

        def __enter__(self):
            return self

        def __exit__(self, *_args):
            return False

    rendered_pages = []
    monkeypatch.setattr(module.pdfplumber, "open", lambda *_args, **_kwargs: _Pdf())

    parser.__images__(b"pdf", page_from=12, page_to=15)

    assert rendered_pages == [12, 13, 14]
    assert parser.page_images is not None
    assert len(parser.page_images) == 3
    assert parser.page_from == 12


def test_crop_converts_local_page_to_document_page(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    parser.page_from = 12
    parser.page_images = [module.Image.new("RGB", (100, 200), "white")]

    image, positions = parser.crop("@@1\t10\t40\t50\t80##", need_position=True)

    assert image is not None
    assert positions == [(12, 10, 40, 50, 80)]


def test_page_rendering_failure_is_reported_to_callback(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    callback = Mock()
    monkeypatch.setattr(module.pdfplumber, "open", Mock(side_effect=ValueError("bad pdf")))

    parser.__images__(b"pdf", page_from=2, page_to=4, callback=callback)

    assert callback.call_args_list == [
        call(0.16, "[MinerU] Rendering PDF pages..."),
        call(0.16, "[MinerU] PDF page rendering failed for pages 2:4: bad pdf"),
    ]


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
@pytest.mark.parametrize(
    ("code_body", "expected_body"),
    [
        ("```txt\nList<String> names = new ArrayList<String>();\n```", "List<String> names = new ArrayList<String>();"),
        ("```\nList<String> names = new ArrayList<String>();\n```", "List<String> names = new ArrayList<String>();"),
        ("```txt\nList<String> names = new ArrayList<String>();\n``` trailing", "```txt\nList<String> names = new ArrayList<String>();\n``` trailing"),
    ],
)
def test_transfer_to_sections_wraps_caption_and_unwrapped_body_in_fence(
    monkeypatch: pytest.MonkeyPatch,
    code_body: str,
    expected_body: str,
) -> None:
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    caption = "Java / C# style"
    output = {
        "type": module.MinerUContentType.CODE,
        "code_body": code_body,
        "code_caption": [caption],
        "page_idx": 0,
        "bbox": (97, 195, 579, 252),
    }

    sections = parser._transfer_to_sections([output], parse_method="raw")

    assert len(sections) == 1
    assert sections[0][0] == f"```{caption}\n{expected_body}\n```"


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
    parser.page_from = 12
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
            "_mineru_positions": [
                {"page_idx": 0, "bbox": (1, 2, 3, 4)},
                {"page_idx": 1, "bbox": (1, 0, 3, 2)},
            ],
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
    assert [[position[0] for position in item[1]] for item in media] == [[12, 13], [12], [13]]
    chunks = tokenize_table(media, {}, False)
    assert [chunk["doc_type_kwd"] for chunk in chunks] == ["table", "image", "table"]
    assert [chunk["page_num_int"] for chunk in chunks] == [[13, 14], [13], [14]]


@pytest.mark.p1
def test_transfer_to_tables_emits_chart_as_image_chunk(monkeypatch, tmp_path):
    """MinerU 3.4.x VLM chart blocks (chart_caption/chart_footnote/img_path)
    must surface as image chunks instead of being silently dropped (#19080)."""
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    parser.page_from = 0
    chart_path = tmp_path / "chart.png"
    module.Image.new("RGB", (2, 2), "blue").save(chart_path)
    outputs = [
        {
            "type": module.MinerUContentType.CHART,
            "img_path": str(chart_path),
            "chart_caption": ["Figure 3"],
            "chart_footnote": ["Source: dataset"],
            "sub_type": "line",
            "vlm_description": "A blue square",
            "page_idx": 0,
            "bbox": (1, 2, 3, 4),
        },
        {
            "type": module.MinerUContentType.CHART,
            "chart_caption": ["Caption without image"],
            "chart_footnote": [],
        },
    ]

    media = parser._transfer_to_tables(outputs)

    # The chart with a readable image becomes an image chunk; the chart without
    # an img_path is skipped (mirrors how IMAGE blocks behave).
    assert len(media) == 1
    image, texts = media[0][0]
    chart_path.unlink()
    assert isinstance(image, module.Image.Image)
    assert image.getpixel((0, 0)) == (0, 0, 255)
    assert texts == ["Figure 3", "Source: dataset", "A blue square"]


@pytest.mark.p1
def test_transfer_to_sections_routes_chart_like_image_per_parse_method(monkeypatch):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {"type": module.MinerUContentType.TEXT, "text": "Body", "page_idx": 0, "bbox": (0, 0, 1, 1)},
        {
            "type": module.MinerUContentType.CHART,
            "chart_caption": ["figure"],
            "chart_footnote": [],
            "page_idx": 0,
            "bbox": (0, 2, 1, 3),
        },
    ]

    # app chunkers consume media separately: the chart is excluded from text sections.
    for app_method in ("naive", "manual", "paper"):
        sections = parser._transfer_to_sections(outputs, parse_method=app_method, table_enable=True)
        assert len(sections) == 1
        assert sections[0][0].startswith("Body")

    # raw consumers keep the chart as a text section (caption/footnote), like IMAGE.
    raw_sections = parser._transfer_to_sections(outputs, parse_method="raw", table_enable=True)
    assert len(raw_sections) == 2
    assert raw_sections[1][0].strip() == "figure"


@pytest.mark.p1
def test_transfer_to_sections_warns_on_unknown_type(monkeypatch, caplog):
    module = _load_mineru_parser(monkeypatch)
    parser = module.MinerUParser()
    outputs = [
        {"type": "sidebar", "text": "ignored", "page_idx": 0, "bbox": (0, 0, 1, 1)},
    ]

    with caplog.at_level(logging.WARNING, logger=parser.logger.name):
        parser._transfer_to_sections(outputs, parse_method="raw")

    assert "Skip unsupported section type=sidebar" in caplog.text


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
def test_media_context_preserves_image_payload_type(monkeypatch):
    import rag.nlp as nlp
    from PIL import Image

    image = Image.new("RGB", (1, 1))
    sections = [("Context before.", "@@1\t0\t10\t0\t5##")]
    media = [((image, ["Figure 1"]), [(12, 0, 1, 10, 20)])]

    monkeypatch.setattr(nlp, "tokenize", lambda d, text, _eng, language="English": d.update({"content_with_weight": text}))
    contextualized = nlp.append_context2table_image4pdf(sections, media, 1, section_page_offset=12)

    rows = contextualized[0][0][1]
    assert isinstance(rows, list)
    assert "Context before." in rows[0]
    assert "Figure 1" in rows[0]
    chunks = nlp.tokenize_table(contextualized, {}, False)
    assert [chunk["doc_type_kwd"] for chunk in chunks] == ["image"]
    assert chunks[0]["page_num_int"] == [13]


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
