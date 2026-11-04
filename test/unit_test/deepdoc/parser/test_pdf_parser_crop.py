import importlib.util
import logging
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest
from PIL import Image


def _load_pdf_parser(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    _stub_module(monkeypatch, "pdfplumber")
    _stub_module(monkeypatch, "pypdf", PdfReader=object)
    _stub_module(monkeypatch, "huggingface_hub", snapshot_download=lambda **_kwargs: "")
    _stub_module(monkeypatch, "xgboost", Booster=object)
    _stub_module(monkeypatch, "sklearn")
    _stub_module(monkeypatch, "sklearn.cluster", KMeans=object)
    _stub_module(monkeypatch, "sklearn.metrics", silhouette_score=lambda *_args, **_kwargs: 0)

    common_mod = _stub_module(monkeypatch, "common")
    common_mod.__path__ = [str(repo_root / "common")]
    _stub_module(monkeypatch, "common.constants", MAXIMUM_PAGE_NUMBER=1024)
    _stub_module(monkeypatch, "common.file_utils", get_project_base_directory=lambda: str(repo_root))
    _stub_module(monkeypatch, "common.settings", PARALLEL_DEVICES=1)
    _stub_module(monkeypatch, "common.misc_utils", thread_pool_exec=lambda fn, *args, **kwargs: fn(*args, **kwargs))

    deepdoc_mod = _stub_module(monkeypatch, "deepdoc")
    deepdoc_mod.__path__ = [str(repo_root / "deepdoc")]
    parser_mod = _stub_module(monkeypatch, "deepdoc.parser")
    parser_mod.__path__ = [str(repo_root / "deepdoc" / "parser")]
    _stub_module(monkeypatch, "deepdoc.parser.utils", extract_pdf_outlines=lambda *_args, **_kwargs: [])
    _stub_module(
        monkeypatch,
        "deepdoc.vision",
        OCR=object,
        AscendLayoutRecognizer=object,
        LayoutRecognizer=object,
        Recognizer=object,
        TableStructureRecognizer=SimpleNamespace(is_caption=lambda _box: False),
    )

    rag_mod = _stub_module(monkeypatch, "rag")
    rag_mod.__path__ = [str(repo_root / "rag")]
    _stub_module(monkeypatch, "rag.nlp", rag_tokenizer=SimpleNamespace(tokenize=lambda text: text))
    prompts_mod = _stub_module(monkeypatch, "rag.prompts")
    prompts_mod.__path__ = [str(repo_root / "rag" / "prompts")]
    _stub_module(monkeypatch, "rag.prompts.generator", vision_llm_describe_prompt="")

    module_name = "test_pdf_parser_crop_module"
    module_path = repo_root / "deepdoc" / "parser" / "pdf_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _stub_module(monkeypatch, name, **attrs):
    module = ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    monkeypatch.setitem(sys.modules, name, module)
    return module


def _page(width, height, color):
    return Image.new("RGB", (width, height), color)


def _parser(module, pages, page_from=0):
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_images = pages
    parser.page_from = page_from
    return parser


def _tag(pages, left, right, top, bottom):
    labels = [page + 1 for page in pages]
    page = str(labels[0]) if len(labels) == 1 else "-".join(str(page) for page in labels)
    return f"@@{page}\t{left}\t{right}\t{top}\t{bottom}##"


def _close(actual, expected):
    assert len(actual) == len(expected)
    for got, want in zip(actual, expected):
        assert got[0] == want[0]
        assert got[1:] == pytest.approx(want[1:])


def _pixel(image, xy):
    return image.getpixel(xy)


@pytest.fixture
def parser_module(monkeypatch):
    return _load_pdf_parser(monkeypatch)


def test_single_page_normalizes_width_and_returns_crop_bounds(parser_module):
    page = _page(300, 400, (255, 255, 255))
    page.putpixel((40, 80), (1, 2, 3))
    parser = _parser(parser_module, [page], page_from=4)

    image, positions = parser.crop(_tag([0], 10, 12, 20, 40), ZM=3, need_position=True)

    _close(positions, [(4, 10, 20, 20, 40)])
    # Head: [0, 14]*3, body: [20, 40]*3, tail: [46*3, 400].
    assert image.getpixel((10, 42 + 6 + 20)) == (1, 2, 3)
    assert image.size == (30, 42 + 6 + 60 + 6 + 262 + 6)


@pytest.mark.parametrize("zoom", [1, 3])
def test_multipage_heterogeneous_pages_intersect_each_page(parser_module, zoom):
    pages = [
        _page(90 * zoom, 40 * zoom, (10, 0, 0)),
        _page(120 * zoom, 50 * zoom, (0, 10, 0)),
        _page(80 * zoom, 30 * zoom, (0, 0, 10)),
    ]
    parser = _parser(parser_module, pages, page_from=2)

    _image, positions = parser.crop(_tag([0, 1, 2], 70, 100, 10, 12), ZM=zoom, need_position=True)

    _close(
        positions,
        [
            (2, 70, 90, 10, 40),
            (3, 70, 100, 0, 50),
            (4, 70, 80, 0, 12),
        ],
    )


def test_inverted_axes_use_exact_sorted_range_before_min_width(parser_module):
    parser = _parser(parser_module, [_page(200, 200, (255, 255, 255))])

    _image, positions = parser.crop(_tag([0], 80, 20, 60, 20), ZM=1, need_position=True)

    _close(positions, [(0, 20, 80, 20, 60)])


def test_negative_top_and_out_of_page_axes_are_clamped(parser_module):
    parser = _parser(parser_module, [_page(100, 80, (255, 255, 255))])

    _image, positions = parser.crop(_tag([0], -20, 140, -15, 200), ZM=2, need_position=True)

    _close(positions, [(0, 0, 50, 0, 40)])


def test_empty_first_page_keeps_following_page(parser_module):
    parser = _parser(parser_module, [_page(100, 40, (1, 1, 1)), _page(100, 40, (2, 2, 2))])

    image, positions = parser.crop(_tag([0, 1], 10, 30, 40, 15), ZM=1, need_position=True)

    _close(positions, [(1, 10, 30, 0, 15)])
    assert image.getpixel((0, 0))[:3] != (1, 1, 1)


def test_negative_last_bottom_does_not_swap_into_a_fake_page(parser_module, caplog):
    pages = [_page(100, 40, (255, 0, 0)), _page(100, 40, (0, 255, 0))]
    parser = _parser(parser_module, pages)

    with caplog.at_level(logging.WARNING):
        image, positions = parser.crop(_tag([0, 1], 10, 30, 10, -5), ZM=1, need_position=True)

    _close(positions, [(0, 10, 30, 10, 35)])
    assert image.getpixel((0, 4 + 6)) == (255, 0, 0)
    assert all(image.getpixel((x, y)) != (0, 255, 0) for x in range(image.width) for y in range(image.height))
    assert "empty_after_clamp" in caplog.text


def test_negative_last_bottom_context_comes_from_real_body_end(parser_module):
    pages = [_page(100, 50, (255, 255, 255)), _page(100, 50, (0, 0, 255))]
    parser = _parser(parser_module, pages)

    image, positions = parser.crop(_tag([0, 1], 10, 40, 20, -20), ZM=1, need_position=True)

    _close(positions, [(0, 10, 40, 20, 30)])
    assert image.getpixel((0, 0)) == (127, 127, 127)
    assert image.getpixel((0, 14 + 6)) == (255, 255, 255)
    assert all(image.getpixel((x, y)) != (0, 0, 255) for x in range(image.width) for y in range(image.height))
    assert image.size == (30, 14 + 6 + 10 + 6 + 14 + 6)


def test_empty_context_does_not_dim_body(parser_module, caplog):
    page = _page(80, 40, (255, 255, 255))
    parser = _parser(parser_module, [page])

    with caplog.at_level(logging.WARNING):
        image, positions = parser.crop(_tag([0], 5, 25, 0, 40), ZM=1, need_position=True)

    _close(positions, [(0, 5, 25, 0, 40)])
    assert image.getpixel((0, 0)) == (255, 255, 255)
    assert "empty crop" not in caplog.text


def test_degenerate_body_returns_none_and_warns_without_text(parser_module, caplog):
    parser = _parser(parser_module, [_page(50, 50, (255, 255, 255))])
    secret = "do-not-log-body"

    with caplog.at_level(logging.WARNING):
        assert parser.crop(secret + _tag([0], 10, 20, 30, 30), need_position=False) is None
        assert parser.crop(secret + _tag([0], 10, 20, 30, 30), need_position=True) == (None, None)

    assert secret not in caplog.text
    assert "non_positive_vertical_span" in caplog.text


def test_missing_images_and_no_tags_keep_return_contract(parser_module, caplog):
    parser = _parser(parser_module, [])
    with caplog.at_level(logging.WARNING):
        assert parser.crop("no tags", need_position=False) is None
        assert parser.crop("no tags", need_position=True) == (None, None)
        assert parser.crop(_tag([0], 1, 2, 3, 4), need_position=True) == (None, None)
    assert "crop called without page images" in caplog.text


def test_subpixel_crop_collapsed_by_pil_is_skipped(parser_module):
    parser = _parser(parser_module, [_page(20, 20, (255, 255, 255))])

    assert parser.crop(_tag([0], 1, 1.1, 1, 1.1), ZM=1, need_position=True) == (None, None)


def test_extract_positions_feeds_crop_and_keeps_malformed_guard(parser_module):
    parser = _parser(parser_module, [_page(90, 40, (255, 255, 255)), _page(90, 40, (255, 255, 255))])
    text = "@@1\t10\t20\t5\t15## junk @@-\t1\t2\t3## @@2\t-4\t8\t-2\t6##"

    assert parser.extract_positions(text) == [
        ([0], 10.0, 20.0, 5.0, 15.0),
        ([1], -4.0, 8.0, -2.0, 6.0),
    ]
    _image, positions = parser.crop(text, ZM=1, need_position=True)
    _close(positions, [(0, 10, 20, 5, 15), (1, 0, 8, 0, 6)])


def test_noncontiguous_pages_are_not_guessed(parser_module, caplog):
    parser = _parser(parser_module, [_page(40, 40, (1, 1, 1)), _page(40, 40, (2, 2, 2)), _page(40, 40, (3, 3, 3))])

    with caplog.at_level(logging.WARNING):
        assert parser.crop("@@1-3\t5\t15\t10\t12##", ZM=1, need_position=True) == (None, None)

    assert "Nonconsecutive page indices" in caplog.text


@pytest.mark.parametrize("pages", [[], [-1], [3], [0, 3], [1, 0], [0, 0]])
def test_invalid_page_lists_skip_without_changing_endpoint_meaning(parser_module, monkeypatch, caplog, pages):
    parser = _parser(parser_module, [_page(100, 100, "white") for _ in range(3)])
    monkeypatch.setattr(parser, "extract_positions", lambda _text: [(pages, 10, 30, 10, 20)])
    with caplog.at_level(logging.WARNING):
        assert parser.crop("mock", ZM=1, need_position=True) == (None, None)
    assert caplog.records


def test_all_content_outside_page_returns_none_and_warns(parser_module, caplog):
    parser = _parser(parser_module, [_page(100, 100, "white")])
    with caplog.at_level(logging.WARNING):
        assert parser.crop(_tag([0], 10, 30, 200, 220), ZM=1, need_position=True) == (None, None)
    assert "empty_after_clamp" in caplog.text


def test_first_page_top_can_extend_into_second_page(parser_module):
    parser = _parser(parser_module, [_page(100, 100, "red"), _page(100, 100, "blue")])
    _image, positions = parser.crop(_tag([0, 1], 10, 30, 110, 20), ZM=1, need_position=True)
    _close(positions, [(1, 10, 30, 10, 20)])


def test_cross_page_range_with_end_before_start_is_skipped(parser_module, caplog):
    parser = _parser(parser_module, [_page(100, 50, "white"), _page(100, 50, "blue")])
    with caplog.at_level(logging.WARNING):
        assert parser.crop(_tag([0, 1], 10, 30, 20, -40), ZM=1, need_position=True) == (None, None)
    assert "non_positive_vertical_span" in caplog.text


@pytest.mark.parametrize(("top", "bottom", "visible"), [(1.4, 1.6, True), (1.6, 2.4, False)])
def test_pil_rounds_endpoints_rather_than_span_length(parser_module, top, bottom, visible):
    parser = _parser(parser_module, [_page(100, 100, "white")])
    image, positions = parser.crop(_tag([0], 10, 30, top, bottom), ZM=1, need_position=True)
    if visible:
        _close(positions, [(0, 10, 30, top, bottom)])
        assert image.getpixel((0, 0)) == (255, 255, 255)
    else:
        assert (image, positions) == (None, None)


def test_each_crop_box_is_bounded_and_matches_returned_positions(parser_module, monkeypatch):
    pages = [_page(100, 100, "red"), _page(80, 150, "blue")]
    parser = _parser(parser_module, pages, page_from=7)
    original_crop = Image.Image.crop
    calls = []

    def record_crop(image, box):
        assert any(image is page for page in pages)
        x0, y0, x1, y1 = box
        assert 0 <= x0 < x1 <= image.width
        assert 0 <= y0 < y1 <= image.height
        calls.append(box)
        return original_crop(image, box)

    monkeypatch.setattr(Image.Image, "crop", record_crop)
    _image, positions = parser.crop(_tag([0, 1], -10, 120, 0, 150), ZM=1, need_position=True)
    _close(positions, [(7, 0, 100, 0, 100), (8, 0, 80, 0, 150)])
    assert calls == [(0, 0, 100, 100), (0, 0, 80, 150)]


def test_image_only_return_is_same_as_need_position_image(parser_module):
    parser = _parser(parser_module, [_page(100, 100, "white")])
    tag = _tag([0], 10, 30, 20, 40)
    image = parser.crop(tag, ZM=1)
    with_position, _positions = parser.crop(tag, ZM=1, need_position=True)
    assert isinstance(image, Image.Image)
    assert image.size == with_position.size
    assert image.tobytes() == with_position.tobytes()
