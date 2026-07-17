import importlib.util
import sys
from pathlib import Path
from types import ModuleType

import pytest


def _load_mistral_parser(monkeypatch):
    """Load mistral_parser.py directly, bypassing deepdoc/__init__.py's
    beartype_this_package() and the heavy deepdoc dependency chain.
    Mirrors test_somark_parser.py."""
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
    pdf_parser_mod.MAXIMUM_PAGE_NUMBER = 100000
    monkeypatch.setitem(sys.modules, "deepdoc.parser.pdf_parser", pdf_parser_mod)

    utils_mod = ModuleType("deepdoc.parser.utils")
    utils_mod.extract_pdf_outlines = lambda *_a, **_k: []
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", utils_mod)

    module_name = "test_mistral_parser_unit_module"
    module_path = repo_root / "deepdoc" / "parser" / "mistral_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _make_parser(m, api_key="", **kwargs):
    """Build a MistralParser without any network call. __init__ only sets attributes."""
    return m.MistralParser(base_url="https://api.mistral.ai/v1", api_key=api_key, **kwargs)


def test_resolve_internal_type_maps_known_types(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    assert p._resolve_internal_type("text") == "text"
    assert p._resolve_internal_type("title") == "text"
    assert p._resolve_internal_type("list") == "text"
    assert p._resolve_internal_type("image") == "image"
    assert p._resolve_internal_type("table") == "table"


def test_resolve_internal_type_drops_header_footer_by_default(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    assert p._resolve_internal_type("header") is None
    assert p._resolve_internal_type("footer") is None


def test_resolve_internal_type_keeps_header_footer_when_flagged(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, keep_header_footer=True)
    assert p._resolve_internal_type("header") == "text"
    assert p._resolve_internal_type("footer") == "text"


def test_resolve_internal_type_unknown_falls_back_to_text(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    assert p._resolve_internal_type("brand_new_type") == "text"


def test_block_text_image_returns_empty(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    assert m.MistralParser._block_text({"content": "x"}, "image") == ""


def test_block_text_strips_and_returns_content(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    assert m.MistralParser._block_text({"content": "  hi  "}, "text") == "hi"


def _ocr_response():
    """A real-shaped /v1/ocr response (from a live probe): two pages, a header
    (dropped), text, title, an image with bbox, and a table block. dimensions
    differ per page to guard against a constant rescale factor."""
    return {
        "model": "mistral-ocr-latest",
        "usage_info": {"pages_processed": 2, "doc_size_bytes": 211291},
        "pages": [
            {
                "index": 0,
                "dimensions": {"dpi": 87, "width": 720, "height": 1018},
                "markdown": "# Title\n\nhello",
                "images": [{"id": "img-0.jpeg", "top_left_x": 251, "top_left_y": 72, "bottom_right_x": 311, "bottom_right_y": 145}],
                "tables": [],
                "blocks": [
                    {"type": "header", "content": "Cofinanziato", "top_left_x": 135, "top_left_y": 72, "bottom_right_x": 213, "bottom_right_y": 145},
                    {"type": "title", "content": "Title", "top_left_x": 40, "top_left_y": 160, "bottom_right_x": 400, "bottom_right_y": 190},
                    {"type": "text", "content": "hello world", "top_left_x": 40, "top_left_y": 200, "bottom_right_x": 500, "bottom_right_y": 230},
                    {"type": "image", "content": "", "top_left_x": 251, "top_left_y": 72, "bottom_right_x": 311, "bottom_right_y": 145},
                ],
            },
            {
                "index": 1,
                "dimensions": {"dpi": 144, "width": 1021, "height": 681},
                "markdown": "|a|b|",
                "images": [],
                "tables": [{"id": "tbl-0.md", "content": "<table><tr><td>a</td></tr></table>", "format": "html", "word_confidence_scores": None}],
                "blocks": [
                    {"type": "table", "content": "<table><tr><td>a</td></tr></table>", "table_id": "tbl-0.md", "top_left_x": 49, "top_left_y": 103, "bottom_right_x": 960, "bottom_right_y": 597},
                ],
            },
        ],
    }


def test_normalize_pages_maps_bbox_and_page_size(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    pages = p._normalize_pages(_ocr_response())
    assert [pg["page_num"] for pages_ in [pages] for pg in pages_] == [0, 1]
    assert pages[0]["page_size"] == {"w": 720, "h": 1018}
    assert pages[1]["page_size"] == {"w": 1021, "h": 681}
    first_text = pages[0]["blocks"][2]
    assert first_text["bbox"] == [40, 200, 500, 230]  # [x0, top, x1, bott]


def test_normalize_pages_bbox_none_when_no_coords(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    resp = {"pages": [{"index": 0, "dimensions": {"width": 100, "height": 100}, "blocks": [{"type": "image", "content": ""}]}]}
    pages = p._normalize_pages(resp)
    assert pages[0]["blocks"][0]["bbox"] is None
    # geometry-less image must be skipped, not emitted as a zero-area crop
    assert p._transfer_to_sections(pages) == []


def test_transfer_naive_path_returns_2_tuples_without_header(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    pages = p._normalize_pages(_ocr_response())
    secs = p._transfer_to_sections(pages)  # parse_method None -> naive
    assert all(isinstance(s, tuple) and len(s) == 2 for s in secs)
    joined = " ".join(s[0] for s in secs)
    assert "Cofinanziato" not in joined  # header dropped
    assert "hello world" in joined
    assert "<table>" in joined  # table inlined as text


def test_transfer_pipeline_path_returns_typed_3_tuples(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    pages = p._normalize_pages(_ocr_response())
    secs = p._transfer_to_sections(pages, parse_method="pipeline")
    assert all(isinstance(s, tuple) and len(s) == 3 for s in secs)
    layout_types = {s[1] for s in secs}
    assert {"text", "image", "table"} <= layout_types


def test_transfer_naive_image_carries_caption_and_tag(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    pages = p._normalize_pages(_ocr_response())
    secs = p._transfer_to_sections(pages)
    img = [s for s in secs if "@@" in s[0] and "##" in s[0]]
    assert len(img) == 1  # exactly the image block, tag embedded in text


def test_line_tag_rescales_per_page_dimensions(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)

    class _Img:
        def __init__(self, size):
            self.size = size

    # Distinct scale factors per page and per axis so a swapped-axis or
    # wrong-page-index bug cannot pass: page 0 scales x by 3.0, y by 4.0;
    # page 1 scales x by 4.0, y by 6.0.
    p.page_images = [_Img((300, 800)), _Img((200, 600))]

    tag0 = p._line_tag({"page_idx": 0, "bbox": [10, 20, 40, 60], "page_size": {"w": 100, "h": 200}})
    assert tag0 == "@@1\t30.0\t120.0\t80.0\t240.0##"

    tag1 = p._line_tag({"page_idx": 1, "bbox": [5, 10, 25, 40], "page_size": {"w": 50, "h": 100}})
    assert tag1 == "@@2\t20.0\t100.0\t60.0\t240.0##"


def test_transfer_to_tables_is_empty(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    pages = p._normalize_pages(_ocr_response())
    assert p._transfer_to_tables(pages) == []


def test_crop_returns_none_without_positions(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    assert p.crop("plain text with no tag") is None


def test_crop_reads_page_images_for_tagged_text(monkeypatch):
    from PIL import Image

    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    p.page_images = [Image.new("RGB", (200, 300), "white")]
    tag = "@@1\t10.0\t100.0\t20.0\t60.0##"
    out = p.crop("caption" + tag, need_position=True)
    assert isinstance(out, tuple) and len(out) == 2


class _Resp:
    def __init__(self, status, payload=None, text=""):
        self.status_code = status
        self._payload = payload or {}
        self.text = text

    def json(self):
        return self._payload


def test_check_installation_requires_api_key(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="")
    ok, reason = p.check_installation()
    assert ok is False and "key" in reason.lower()


def test_call_ocr_inline_posts_pages_selector(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test")
    captured = {}

    def fake_post(url, headers=None, json=None, timeout=None, **kw):
        captured["url"] = url
        captured["json"] = json
        return _Resp(200, {"pages": [{"index": 5, "markdown": "x", "blocks": []}], "usage_info": {"pages_processed": 1}})

    monkeypatch.setattr(m.requests, "post", fake_post)
    out = p._call_ocr(b"%PDF-1.4 fake", "f.pdf", pages=[5])
    assert captured["url"].endswith("/ocr")
    assert captured["json"]["pages"] == [5]
    assert captured["json"]["include_blocks"] is True
    assert out["pages"][0]["index"] == 5


def test_call_ocr_omits_pages_when_none(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test")
    captured = {}
    monkeypatch.setattr(m.requests, "post", lambda url, headers=None, json=None, timeout=None, **kw: captured.update(json=json) or _Resp(200, {"pages": []}))
    p._call_ocr(b"%PDF fake", "f.pdf", pages=None)
    assert "pages" not in captured["json"]


def test_call_ocr_raises_on_http_error(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-bad")
    monkeypatch.setattr(m.requests, "post", lambda *a, **k: _Resp(401, text="Unauthorized"))
    with pytest.raises(RuntimeError, match="401"):
        p._call_ocr(b"%PDF fake", "f.pdf", pages=None)


def test_call_ocr_uploads_when_over_inline_limit(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test", inline_max_bytes=4)
    calls = {"post": [], "get": [], "delete": []}

    def fake_post(url, headers=None, json=None, data=None, files=None, timeout=None, **kw):
        calls["post"].append(url)
        if url.endswith("/files"):
            return _Resp(200, {"id": "file-1"})
        return _Resp(200, {"pages": [{"index": 0, "blocks": []}]})

    monkeypatch.setattr(m.requests, "post", fake_post)
    monkeypatch.setattr(m.requests, "get", lambda url, headers=None, params=None, timeout=None, **kw: calls["get"].append(url) or _Resp(200, {"url": "https://signed"}))
    monkeypatch.setattr(m.requests, "delete", lambda url, headers=None, timeout=None, **kw: calls["delete"].append(url) or _Resp(200, {}))

    out = p._call_ocr(b"%PDF-too-big", "big.pdf", pages=None)
    assert any(u.endswith("/files") for u in calls["post"])
    assert calls["get"] and calls["delete"]  # signed-url fetch + cleanup
    assert out["pages"][0]["index"] == 0


def test_call_ocr_upload_files_error_no_delete(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test", inline_max_bytes=4)
    calls = {"delete": []}
    monkeypatch.setattr(m.requests, "post", lambda url, headers=None, json=None, data=None, files=None, timeout=None, **kw: _Resp(500, text="boom"))
    monkeypatch.setattr(m.requests, "delete", lambda url, headers=None, timeout=None, **kw: calls["delete"].append(url) or _Resp(200, {}))
    with pytest.raises(RuntimeError, match="500"):
        p._call_ocr(b"%PDF-too-big", "big.pdf", pages=None)
    assert calls["delete"] == []  # file_id never set -> nothing to clean up


def test_call_ocr_signed_url_error_triggers_cleanup(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test", inline_max_bytes=4)
    calls = {"delete": []}
    monkeypatch.setattr(m.requests, "post", lambda url, headers=None, json=None, data=None, files=None, timeout=None, **kw: _Resp(200, {"id": "file-1"}))
    monkeypatch.setattr(m.requests, "get", lambda url, headers=None, params=None, timeout=None, **kw: _Resp(403, text="nope"))
    monkeypatch.setattr(m.requests, "delete", lambda url, headers=None, timeout=None, **kw: calls["delete"].append(url) or _Resp(200, {}))
    with pytest.raises(RuntimeError, match="403"):
        p._call_ocr(b"%PDF-too-big", "big.pdf", pages=None)
    assert any("file-1" in u for u in calls["delete"])  # cleanup ran


def test_call_ocr_post_upload_ocr_error_triggers_cleanup(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test", inline_max_bytes=4)
    calls = {"delete": []}

    def fake_post(url, headers=None, json=None, data=None, files=None, timeout=None, **kw):
        if url.endswith("/files"):
            return _Resp(200, {"id": "file-9"})
        return _Resp(422, text="bad ocr")

    monkeypatch.setattr(m.requests, "post", fake_post)
    monkeypatch.setattr(m.requests, "get", lambda url, headers=None, params=None, timeout=None, **kw: _Resp(200, {"url": "https://signed"}))
    monkeypatch.setattr(m.requests, "delete", lambda url, headers=None, timeout=None, **kw: calls["delete"].append(url) or _Resp(200, {}))
    with pytest.raises(RuntimeError, match="422"):
        p._call_ocr(b"%PDF-too-big", "big.pdf", pages=None)
    assert any("file-9" in u for u in calls["delete"])  # finally cleanup ran despite OCR failure


def test_call_ocr_delete_failure_does_not_mask_error(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test", inline_max_bytes=4)

    def fake_post(url, headers=None, json=None, data=None, files=None, timeout=None, **kw):
        if url.endswith("/files"):
            return _Resp(200, {"id": "file-x"})
        return _Resp(422, text="bad ocr")

    def boom_delete(url, headers=None, timeout=None, **kw):
        raise RuntimeError("delete network error")

    monkeypatch.setattr(m.requests, "post", fake_post)
    monkeypatch.setattr(m.requests, "get", lambda url, headers=None, params=None, timeout=None, **kw: _Resp(200, {"url": "https://signed"}))
    monkeypatch.setattr(m.requests, "delete", boom_delete)
    # the real OCR error, not the swallowed delete error
    with pytest.raises(RuntimeError, match="422"):
        p._call_ocr(b"%PDF-too-big", "big.pdf", pages=None)


def _patch_render(m, p, n_pages):
    from PIL import Image

    p.page_images = [Image.new("RGB", (100, 140), "white") for _ in range(n_pages)]
    # neuter __images__ so parse_pdf doesn't touch pdfplumber
    p.__images__ = lambda *a, **k: None


def test_parse_pdf_whole_document_omits_pages(monkeypatch, tmp_path):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test")
    seen = {}
    p._call_ocr = lambda pdf_bytes, filename, pages, callback=None: seen.update(pages=pages) or _ocr_response()
    _patch_render(m, p, 2)
    pdf = tmp_path / "x.pdf"
    pdf.write_bytes(b"%PDF-1.4 minimal")
    secs, tables = p.parse_pdf(str(pdf))
    assert seen["pages"] is None  # whole doc -> no selector
    assert tables == []
    assert any("hello world" in s[0] for s in secs)


def test_parse_pdf_restricted_range_sends_absolute_pages(monkeypatch, tmp_path):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test")
    seen = {}
    p._call_ocr = lambda pdf_bytes, filename, pages, callback=None: seen.update(pages=pages) or {"pages": []}
    _patch_render(m, p, 10)
    pdf = tmp_path / "x.pdf"
    pdf.write_bytes(b"%PDF-1.4 minimal")
    p.parse_pdf(str(pdf), from_page=4, to_page=8)
    assert seen["pages"] == [4, 5, 6, 7]  # 0-based, half-open, absolute


def test_parse_pdf_pipeline_returns_3_tuples(monkeypatch, tmp_path):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test")
    p._call_ocr = lambda *a, **k: _ocr_response()
    _patch_render(m, p, 2)
    pdf = tmp_path / "x.pdf"
    pdf.write_bytes(b"%PDF-1.4 minimal")
    secs, _ = p.parse_pdf(str(pdf), parse_method="pipeline")
    assert all(len(s) == 3 for s in secs)


def test_parse_pdf_empty_range_short_circuits(monkeypatch, tmp_path):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test")
    called = {"ocr": 0}

    def fake_ocr(pdf_bytes, filename, pages, callback=None):
        called["ocr"] += 1
        return {"pages": []}

    p._call_ocr = fake_ocr
    _patch_render(m, p, 3)  # only 3 pages rendered
    pdf = tmp_path / "x.pdf"
    pdf.write_bytes(b"%PDF-1.4 minimal")
    # from_page beyond the rendered page count -> empty selector after clamp
    secs, tables = p.parse_pdf(str(pdf), from_page=5, to_page=10)
    assert (secs, tables) == ([], [])
    assert called["ocr"] == 0  # short-circuited, no OCR call made


def test_parse_pdf_binary_bytes_path(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test")
    p._call_ocr = lambda pdf_bytes, filename, pages, callback=None: _ocr_response()
    _patch_render(m, p, 2)
    secs, tables = p.parse_pdf("x.pdf", binary=b"%PDF-1.4 minimal")
    assert any("hello world" in s[0] for s in secs)
    assert tables == []


def test_parse_pdf_binary_stream_normalized(monkeypatch):
    from io import BytesIO

    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test")
    seen = {}
    p._call_ocr = lambda pdf_bytes, filename, pages, callback=None: seen.update(pdf_bytes=pdf_bytes) or _ocr_response()
    _patch_render(m, p, 2)
    p.parse_pdf("x.pdf", binary=BytesIO(b"%PDF-1.4 stream"))
    assert seen["pdf_bytes"] == b"%PDF-1.4 stream"  # BytesIO normalized to raw bytes


def test_image_description_injected_when_vision_model_present(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    p.vision_model = object()  # truthy -> enrichment path taken
    p._describe_image = lambda line_tag: "A scatter plot of X versus Y"
    pages = p._normalize_pages(_ocr_response())
    secs = p._transfer_to_sections(pages)
    img = [s for s in secs if "@@" in s[0] and "scatter plot" in s[0]]
    assert len(img) == 1  # VLM caption injected into the image chunk text


def test_image_description_skipped_without_vision_model(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    assert p.vision_model is None  # default: no enrichment
    calls = {"n": 0}

    def _boom(line_tag):
        calls["n"] += 1
        return "SHOULD NOT APPEAR"

    p._describe_image = _boom
    pages = p._normalize_pages(_ocr_response())
    secs = p._transfer_to_sections(pages)
    assert calls["n"] == 0  # _describe_image not called when vision_model is None
    assert not any("SHOULD NOT APPEAR" in s[0] for s in secs)
    # the image chunk still exists, labelled by fallback (empty caption -> "image N")
    assert any("@@" in s[0] and "image 1" in s[0] for s in secs)


def test_describe_image_returns_empty_when_crop_none(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    p.vision_model = object()
    p.crop = lambda *a, **k: None  # no image to crop
    assert p._describe_image("@@1\t0\t0\t0\t0##") == ""


def test_parse_pdf_consumes_vision_model_kwarg(monkeypatch, tmp_path):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-test")
    seen = {}
    p._call_ocr = lambda pdf_bytes, filename, pages, callback=None: seen.setdefault("pages", pages) or _ocr_response()
    _patch_render(m, p, 2)
    pdf = tmp_path / "x.pdf"
    pdf.write_bytes(b"%PDF-1.4 minimal")
    p.parse_pdf(str(pdf), vision_model="VM")
    assert p.vision_model == "VM"  # popped from kwargs into self, not forwarded to _call_ocr


def test_describe_image_passes_pil_image_not_bytes(monkeypatch):
    # Regression: vision_llm_chunk expects a PIL Image (it calls img.size/img.save),
    # so _describe_image must pass the crop directly, not its bytes.
    import sys as _sys
    from types import ModuleType
    from PIL import Image

    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    p.vision_model = object()
    p.crop = lambda *a, **k: Image.new("RGB", (40, 40), "white")

    captured = {}
    for name in ("rag", "rag.app", "rag.prompts"):
        if name not in _sys.modules:
            monkeypatch.setitem(_sys.modules, name, ModuleType(name))
    pic = ModuleType("rag.app.picture")
    pic.vision_llm_chunk = lambda binary, vision_model, prompt=None, callback=None: (captured.update(kind=type(binary).__name__), "a white square")[1]
    gen = ModuleType("rag.prompts.generator")
    gen.vision_llm_figure_describe_prompt = lambda: "describe"
    monkeypatch.setitem(_sys.modules, "rag.app.picture", pic)
    monkeypatch.setitem(_sys.modules, "rag.prompts.generator", gen)

    out = p._describe_image("@@1\t0\t0\t40\t40##")
    assert out == "a white square"
    assert captured["kind"] == "Image"  # PIL Image, not 'bytes'
