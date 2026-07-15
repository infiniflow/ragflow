import importlib.util
import sys
from pathlib import Path
from types import ModuleType


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
                "images": [{"id": "img-0.jpeg", "top_left_x": 251, "top_left_y": 72,
                            "bottom_right_x": 311, "bottom_right_y": 145}],
                "tables": [],
                "blocks": [
                    {"type": "header", "content": "Cofinanziato", "top_left_x": 135,
                     "top_left_y": 72, "bottom_right_x": 213, "bottom_right_y": 145},
                    {"type": "title", "content": "Title", "top_left_x": 40,
                     "top_left_y": 160, "bottom_right_x": 400, "bottom_right_y": 190},
                    {"type": "text", "content": "hello world", "top_left_x": 40,
                     "top_left_y": 200, "bottom_right_x": 500, "bottom_right_y": 230},
                    {"type": "image", "content": "", "top_left_x": 251, "top_left_y": 72,
                     "bottom_right_x": 311, "bottom_right_y": 145},
                ],
            },
            {
                "index": 1,
                "dimensions": {"dpi": 144, "width": 1021, "height": 681},
                "markdown": "|a|b|",
                "images": [],
                "tables": [{"id": "tbl-0.md", "content": "<table><tr><td>a</td></tr></table>",
                            "format": "html", "word_confidence_scores": None}],
                "blocks": [
                    {"type": "table", "content": "<table><tr><td>a</td></tr></table>",
                     "table_id": "tbl-0.md", "top_left_x": 49, "top_left_y": 103,
                     "bottom_right_x": 960, "bottom_right_y": 597},
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
    resp = {"pages": [{"index": 0, "dimensions": {"width": 100, "height": 100},
                       "blocks": [{"type": "image", "content": ""}]}]}
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

    tag0 = p._line_tag({"page_idx": 0, "bbox": [10, 20, 40, 60],
                        "page_size": {"w": 100, "h": 200}})
    assert tag0 == "@@1\t30.0\t120.0\t80.0\t240.0##"

    tag1 = p._line_tag({"page_idx": 1, "bbox": [5, 10, 25, 40],
                        "page_size": {"w": 50, "h": 100}})
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
        return _Resp(200, {"pages": [{"index": 5, "markdown": "x", "blocks": []}],
                           "usage_info": {"pages_processed": 1}})

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
    monkeypatch.setattr(m.requests, "post",
        lambda url, headers=None, json=None, timeout=None, **kw: captured.update(json=json)
        or _Resp(200, {"pages": []}))
    p._call_ocr(b"%PDF fake", "f.pdf", pages=None)
    assert "pages" not in captured["json"]


def test_call_ocr_raises_on_http_error(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, api_key="sk-bad")
    monkeypatch.setattr(m.requests, "post",
        lambda *a, **k: _Resp(401, text="Unauthorized"))
    try:
        p._call_ocr(b"%PDF fake", "f.pdf", pages=None)
        assert False, "expected RuntimeError"
    except RuntimeError as e:
        assert "401" in str(e)


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
    monkeypatch.setattr(m.requests, "get",
        lambda url, headers=None, params=None, timeout=None, **kw: calls["get"].append(url) or _Resp(200, {"url": "https://signed"}))
    monkeypatch.setattr(m.requests, "delete",
        lambda url, headers=None, timeout=None, **kw: calls["delete"].append(url) or _Resp(200, {}))

    out = p._call_ocr(b"%PDF-too-big", "big.pdf", pages=None)
    assert any(u.endswith("/files") for u in calls["post"])
    assert calls["get"] and calls["delete"]  # signed-url fetch + cleanup
    assert out["pages"][0]["index"] == 0
