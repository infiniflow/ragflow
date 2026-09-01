import importlib.util
import sys
from pathlib import Path
from types import ModuleType


def _load_somark_parser(monkeypatch):
    """Load somark_parser.py directly, bypassing deepdoc/__init__.py's
    beartype_this_package() and the heavy deepdoc dependency chain.

    Mirrors the pattern used by test_mineru_parser.py / test_opendataloader_parser.py.
    """
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

    module_name = "test_somark_parser_unit_module"
    module_path = repo_root / "deepdoc" / "parser" / "somark_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _make_parser(m, **feature_kwargs):
    """Build a SoMarkParser instance without triggering any network call.
    __init__ only sets attributes; check_installation() is what hits the network."""
    return m.SoMarkParser(
        base_url="https://example.invalid/api/v1",
        api_key="",
        **feature_kwargs,
    )


def _sample_pages():
    """A minimal pages payload mixing text, title, figure, equation, table,
    toc (discarded), and an image block with no bbox (must be skipped)."""
    return [
        {
            "page_num": 0,
            "page_size": {"w": 600, "h": 800},
            "blocks": [
                {"type": "text", "content": "hello world", "bbox": [10, 20, 100, 40]},
                {"type": "title", "content": "Chapter 1", "title_level": 1, "bbox": [10, 5, 200, 18]},
                {"type": "figure", "content": "Figure caption from understanding", "bbox": [50, 50, 300, 300]},
                {"type": "table", "content": "<table><tr><td>a</td></tr></table>", "bbox": [10, 400, 500, 600]},
                {"type": "equation", "content": "E=mc^2", "bbox": [10, 650, 200, 680]},
                {"type": "cate_item", "content": "should be discarded", "bbox": [0, 0, 10, 10]},
                {"type": "figure", "content": "no bbox -> skip"},  # no bbox at all
            ],
        }
    ]


# ---------------------------------------------------------------------
# Type-mapping integrity (regression guard)
# ---------------------------------------------------------------------


def test_type_mapping_covers_every_non_discarded_block_type(monkeypatch):
    """Every SoMark block type that is not in ALWAYS_DISCARDED and is not a
    header/footer (which obey keep_header_footer) must have a mapping in
    SOMARK_TYPE_TO_RAGFLOW. A new SoMark type added to the enum without a
    mapping would silently fall back to "text"; this guard makes that
    omission explicit at test time."""
    m = _load_somark_parser(monkeypatch)
    header_footer = {m.SoMarkBlockType.HEADER, m.SoMarkBlockType.FOOTER}
    for btype in m.SoMarkBlockType:
        if btype in m.ALWAYS_DISCARDED or btype in header_footer:
            continue
        assert btype in m.SOMARK_TYPE_TO_RAGFLOW, f"{btype} missing from SOMARK_TYPE_TO_RAGFLOW"


def test_mapping_values_are_known_internal_layout_types(monkeypatch):
    """Mapping values must be one of the layout types that downstream
    rag/flow consumers (and chunking) understand."""
    m = _load_somark_parser(monkeypatch)
    allowed = {"text", "image", "table", "code", "equation"}
    for btype, internal in m.SOMARK_TYPE_TO_RAGFLOW.items():
        assert internal in allowed, f"{btype} -> {internal!r} is not a known internal type"


def test_always_discarded_contains_toc_and_blank(monkeypatch):
    """Table-of-contents items must be discarded; if they leaked through the
    knowledge base would be polluted with chapter titles repeated as chunks."""
    m = _load_somark_parser(monkeypatch)
    assert m.SoMarkBlockType.CATE in m.ALWAYS_DISCARDED
    assert m.SoMarkBlockType.CATE_ITEM in m.ALWAYS_DISCARDED
    assert m.SoMarkBlockType.BLANK in m.ALWAYS_DISCARDED


# ---------------------------------------------------------------------
# _resolve_internal_type — all branches
# ---------------------------------------------------------------------


def test_resolve_internal_type_discards_toc_and_blank(monkeypatch):
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)
    assert p._resolve_internal_type(m.SoMarkBlockType.CATE) is None
    assert p._resolve_internal_type(m.SoMarkBlockType.CATE_ITEM) is None
    assert p._resolve_internal_type(m.SoMarkBlockType.BLANK) is None


def test_resolve_internal_type_header_footer_dropped_by_default(monkeypatch):
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)  # keep_header_footer=False (default)
    assert p._resolve_internal_type(m.SoMarkBlockType.HEADER) is None
    assert p._resolve_internal_type(m.SoMarkBlockType.FOOTER) is None


def test_resolve_internal_type_header_footer_kept_when_flagged(monkeypatch):
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m, keep_header_footer=True)
    assert p._resolve_internal_type(m.SoMarkBlockType.HEADER) == "text"
    assert p._resolve_internal_type(m.SoMarkBlockType.FOOTER) == "text"


def test_resolve_internal_type_unknown_falls_back_to_text(monkeypatch):
    """If SoMark introduces a new block type before the mapping is updated,
    we should fall back to ``text`` (silent loss is worse than a wrong layout
    label)."""
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)
    assert p._resolve_internal_type("some_brand_new_type") == "text"


def test_resolve_internal_type_image_blocks(monkeypatch):
    """figure/cs/qrcode/stamp must all resolve to 'image' so they share the
    crop() recovery path; otherwise figures would be lost on the naive path."""
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)
    for btype in (m.SoMarkBlockType.FIGURE, m.SoMarkBlockType.CS, m.SoMarkBlockType.QRCODE, m.SoMarkBlockType.STAMP):
        assert p._resolve_internal_type(btype) == "image", btype


# ---------------------------------------------------------------------
# _block_text
# ---------------------------------------------------------------------


def test_block_text_image_returns_empty_string(monkeypatch):
    """Image-typed blocks contribute no text via _block_text; the figure is
    later recovered from the rendered page by crop()."""
    m = _load_somark_parser(monkeypatch)
    block = {"type": m.SoMarkBlockType.FIGURE.value, "content": "ignored"}
    assert m.SoMarkParser._block_text(block, "image") == ""


def test_block_text_title_prepends_markdown_hashes(monkeypatch):
    m = _load_somark_parser(monkeypatch)
    block = {"type": m.SoMarkBlockType.TITLE.value, "content": "Hello", "title_level": 2}
    assert m.SoMarkParser._block_text(block, "text") == "## Hello"


def test_block_text_title_without_level_returns_plain_content(monkeypatch):
    m = _load_somark_parser(monkeypatch)
    block = {"type": m.SoMarkBlockType.TITLE.value, "content": "Hello"}  # no title_level
    assert m.SoMarkParser._block_text(block, "text") == "Hello"


def test_block_text_text_strips_whitespace(monkeypatch):
    m = _load_somark_parser(monkeypatch)
    block = {"type": m.SoMarkBlockType.TEXT.value, "content": "  hi  "}
    assert m.SoMarkParser._block_text(block, "text") == "hi"


# ---------------------------------------------------------------------
# _transfer_to_sections — tuple shape contract
# ---------------------------------------------------------------------


def test_transfer_to_sections_naive_path_returns_2_tuples(monkeypatch):
    """parse_method=None (or anything not in {manual, pipeline}) — naive.py
    consumer — must receive 2-tuples (text, line_tag)."""
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)

    secs = p._transfer_to_sections(_sample_pages())

    assert all(isinstance(s, tuple) and len(s) == 2 for s in secs), "naive path must emit (text, line_tag) 2-tuples"
    # 7 blocks - 1 cate_item discarded - 1 no-bbox figure skipped = 5 valid
    assert len(secs) == 5


def test_transfer_to_sections_pipeline_path_returns_3_tuples(monkeypatch):
    """parse_method='pipeline' — rag/flow consumer — must receive typed
    3-tuples (text, layout_type, line_tag), mirroring MinerU's contract."""
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)

    secs = p._transfer_to_sections(_sample_pages(), parse_method="pipeline")

    assert all(isinstance(s, tuple) and len(s) == 3 for s in secs), "pipeline path must emit (text, layout_type, line_tag) 3-tuples"
    # Layout types must reflect block diversity, not collapse to all "text"
    layout_types = {s[1] for s in secs}
    assert layout_types >= {"text", "image", "table", "equation"}, f"expected diverse layout types, got {layout_types}"


def test_transfer_to_sections_naive_image_carries_caption_and_tag(monkeypatch):
    """Image sections on the naive path must carry a unique caption in the
    text field (to avoid chunk-id hash collision across figures) AND embed
    the position tag so tokenize_chunks()->crop() can still recover the
    figure. The pos field is empty by design."""
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)

    secs = p._transfer_to_sections(_sample_pages())
    image_secs = [s for s in secs if s[1] == ""]

    assert len(image_secs) == 1
    text = image_secs[0][0]
    assert "Figure caption from understanding" in text, "caption must be in text"
    assert "@@" in text and "##" in text, "position tag must be embedded in text"


def test_transfer_to_sections_pipeline_image_keeps_caption_and_typed_position(monkeypatch):
    """On the pipeline path the image block keeps its caption text for semantic
    retrieval and a real (separate) line_tag for crop(poss)."""
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)

    secs = p._transfer_to_sections(_sample_pages(), parse_method="pipeline")
    image_secs = [s for s in secs if s[1] == "image"]

    assert len(image_secs) == 1
    text, layout, line_tag = image_secs[0]
    assert text == "Figure caption from understanding"
    assert layout == "image"
    assert line_tag.startswith("@@") and line_tag.endswith("##")


def test_transfer_to_sections_discards_cate_item_in_both_modes(monkeypatch):
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)
    for mode in (None, "pipeline"):
        secs = p._transfer_to_sections(_sample_pages(), parse_method=mode)
        leaked = [s for s in secs if "should be discarded" in (s[0] or "")]
        assert leaked == [], f"cate_item leaked in mode={mode}: {leaked}"


def test_transfer_to_sections_skips_image_block_without_bbox(monkeypatch):
    """No bbox means crop() can't recover anything; emitting an empty section
    would only pollute the chunk stream."""
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)
    pages = [
        {
            "page_num": 0,
            "page_size": {"w": 600, "h": 800},
            "blocks": [{"type": "figure", "content": "no bbox"}],
        }
    ]
    assert p._transfer_to_sections(pages) == []


def test_transfer_to_sections_keeps_header_footer_when_flagged(monkeypatch):
    """With keep_header_footer=True, header/footer blocks should pass through
    as text sections."""
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m, keep_header_footer=True)
    pages = [
        {
            "page_num": 0,
            "page_size": {"w": 600, "h": 800},
            "blocks": [
                {"type": "header", "content": "doc title", "bbox": [0, 0, 100, 10]},
                {"type": "footer", "content": "page 1", "bbox": [0, 790, 100, 800]},
            ],
        }
    ]
    secs = p._transfer_to_sections(pages)
    texts = [s[0] for s in secs]
    assert any("doc title" in t for t in texts)
    assert any("page 1" in t for t in texts)


# ---------------------------------------------------------------------
# _line_tag format
# ---------------------------------------------------------------------


def test_line_tag_format(monkeypatch):
    """Tag format ``@@<page1based>\\t<x0>\\t<x1>\\t<y0>\\t<y1>##`` is the
    contract that downstream extract_positions() / crop() parse."""
    m = _load_somark_parser(monkeypatch)
    p = _make_parser(m)
    bx = {
        "page_idx": 0,
        "bbox": [10, 20, 100, 40],
        "page_size": {"w": 600, "h": 800},
    }
    tag = p._line_tag(bx)

    assert tag.startswith("@@1\t"), "page index must be 1-based"
    assert tag.endswith("##")
    parts = tag.strip("@").strip("#").split("\t")
    assert len(parts) == 5, f"expected 5 tab-separated parts, got {parts}"
    # Absent page_images, _line_tag uses raw bbox coords
    assert float(parts[1]) == 10.0  # x0
    assert float(parts[2]) == 100.0  # x1
    assert float(parts[3]) == 20.0  # y0 (top)
    assert float(parts[4]) == 40.0  # y1 (bottom)
