import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


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
        Recognizer=_FakeRecognizer,
        TableStructureRecognizer=SimpleNamespace(is_caption=lambda _box: False),
    )

    rag_mod = _stub_module(monkeypatch, "rag")
    rag_mod.__path__ = [str(repo_root / "rag")]
    _stub_module(monkeypatch, "rag.nlp", rag_tokenizer=SimpleNamespace(tokenize=lambda text: text))
    prompts_mod = _stub_module(monkeypatch, "rag.prompts")
    prompts_mod.__path__ = [str(repo_root / "rag" / "prompts")]
    _stub_module(monkeypatch, "rag.prompts.generator", vision_llm_describe_prompt="")

    module_name = "test_pdf_parser_unit_module"
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


class _FakeRecognizer:
    @staticmethod
    def sort_Y_firstly(arr, _threshold):
        return sorted(arr, key=lambda item: (item["top"], item["x0"]))

    @staticmethod
    def layouts_cleanup(_boxes, layouts, _far=2, _thr=0.7):
        return layouts

    @staticmethod
    def overlapped_area(a, b, ratio=True):
        x0 = max(a["x0"], b["x0"])
        x1 = min(a["x1"], b["x1"])
        top = max(a["top"], b["top"])
        bottom = min(a["bottom"], b["bottom"])
        if x1 <= x0 or bottom <= top:
            return 0
        area = (x1 - x0) * (bottom - top)
        if ratio:
            area /= (a["x1"] - a["x0"]) * (a["bottom"] - a["top"])
        return area

    @staticmethod
    def find_overlapped_with_threshold(box, boxes, thr=0.3):
        best_i = None
        best = thr
        best_reverse = 0
        for i, candidate in enumerate(boxes):
            overlap = _FakeRecognizer.overlapped_area(box, candidate)
            reverse = _FakeRecognizer.overlapped_area(candidate, box)
            if (overlap, reverse) < (best, best_reverse):
                continue
            best_i = i
            best = overlap
            best_reverse = reverse
        return best_i

    @staticmethod
    def find_overlapped(box, boxes, naive=False):
        for i, candidate in enumerate(boxes):
            if _FakeRecognizer.overlapped_area(box, candidate) > 0.1:
                return i
        return 0 if boxes else None

    @staticmethod
    def find_horizontally_tightest_fit(box, boxes):
        min_distance = 1000000
        min_i = None
        for i, candidate in enumerate(boxes):
            if box.get("layoutno", "0") != candidate.get("layoutno", "0"):
                continue
            distance = min(
                abs(box["x0"] - candidate["x0"]),
                abs(box["x1"] - candidate["x1"]),
                abs(box["x0"] + box["x1"] - candidate["x1"] - candidate["x0"]) / 2,
            )
            if distance < min_distance:
                min_distance = distance
                min_i = i
        return min_i


class _FakeImage:
    def __init__(self, width=300, height=400):
        self.size = (width, height)

    def crop(self, box):
        left, top, right, bottom = box
        return _FakeImage(right - left, bottom - top)

    def __array__(self, dtype=None):
        import numpy as np

        return np.zeros((int(self.size[1]), int(self.size[0]), 3), dtype=dtype or np.uint8)


class _FakeTableDetector:
    def __init__(self, zoom, angle=0, crop_width=140, crop_height=90):
        self.zoom = zoom
        self.angle = angle
        self.crop_width = crop_width * zoom
        self.crop_height = crop_height * zoom

    def __call__(self, _imgs):
        z = self.zoom
        rows = [
            _scale_bbox((15, 20, 125, 35), z),
            _scale_bbox((15, 50, 125, 65), z),
        ]
        columns = [
            _scale_bbox((15, 20, 55, 65), z),
            _scale_bbox((80, 20, 125, 65), z),
        ]
        rows = [_rotate_bbox_clockwise(row, self.angle, self.crop_width, self.crop_height) for row in rows]
        columns = [_rotate_bbox_clockwise(column, self.angle, self.crop_width, self.crop_height) for column in columns]
        return [
            [
                _component("table row", rows[0]),
                _component("table row", rows[1]),
                _component("table column", columns[0]),
                _component("table column", columns[1]),
            ]
        ]


class _FakeOcr:
    def __init__(self, angle, crop_width=140, crop_height=90):
        self.angle = angle
        self.crop_width = crop_width
        self.crop_height = crop_height

    def __call__(self, _img_array):
        boxes = [
            ("A1", (15, 20, 55, 35)),
            ("B2", (80, 50, 125, 65)),
        ]
        return [
            (
                _bbox_points(_rotate_bbox_clockwise(bbox, self.angle, self.crop_width, self.crop_height)),
                (text, 0.99),
            )
            for text, bbox in boxes
        ]


def _component(label, bbox):
    x0, top, x1, bottom = bbox
    return {"label": label, "x0": x0, "x1": x1, "top": top, "bottom": bottom}


def _scale_bbox(bbox, zoom):
    x0, top, x1, bottom = bbox
    return x0 * zoom, top * zoom, x1 * zoom, bottom * zoom


def _bbox_points(bbox):
    x0, top, x1, bottom = bbox
    return [(x0, top), (x1, top), (x1, bottom), (x0, bottom)]


def _rotate_bbox_clockwise(bbox, angle, width, height):
    points = [_rotate_point_clockwise(x, y, angle, width, height) for x, y in _bbox_points(bbox)]
    xs = [p[0] for p in points]
    ys = [p[1] for p in points]
    return min(xs), min(ys), max(xs), max(ys)


def _rotate_point_clockwise(x, y, angle, width, height):
    if angle == 0:
        return x, y
    if angle == 90:
        return height - y, x
    if angle == 180:
        return width - x, height - y
    if angle == 270:
        return y, width - x
    raise ValueError(f"unsupported angle: {angle}")


@pytest.mark.p1
@pytest.mark.parametrize(
    ("env_value", "explicit_value", "expected_orientation_calls"),
    [
        (None, None, 1),
        ("false", None, 0),
        ("true", None, 1),
        ("1", None, 1),
        ("yes", None, 1),
        ("true", False, 0),
        ("false", True, 1),
    ],
)
def test_table_transformer_resolves_auto_rotate_from_argument_or_environment(monkeypatch, env_value, explicit_value, expected_orientation_calls):
    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_layout = [[{"type": "table", "x0": 0, "top": 0, "x1": 10, "bottom": 10}]]
    parser.page_images = [_FakeImage()]
    parser.boxes = []
    parser.tbl_det = lambda imgs: [[] for _ in imgs]
    parser._ocr_rotated_tables = lambda *_args, **_kwargs: None

    orientation_calls = []

    def evaluate_orientation(table_img):
        orientation_calls.append(table_img)
        return 0, table_img, {}

    parser._evaluate_table_orientation = evaluate_orientation

    if env_value is None:
        monkeypatch.delenv("TABLE_AUTO_ROTATE", raising=False)
    else:
        monkeypatch.setenv("TABLE_AUTO_ROTATE", env_value)

    kwargs = {} if explicit_value is None else {"auto_rotate": explicit_value}
    parser._table_transformer_job(1, **kwargs)

    assert len(orientation_calls) == expected_orientation_calls


@pytest.mark.p1
@pytest.mark.parametrize(("page_index", "page_offset", "zoom"), [(0, 0, 1), (1, 500, 2)])
def test_table_transformer_maps_tsr_crop_coordinates_to_page_coordinates(monkeypatch, page_index, page_offset, zoom):
    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = 0
    parser.page_images = [_FakeImage() for _ in range(page_index + 1)]
    if page_index:
        # The first page contributes page_offset pre-zoom units.
        parser.page_images[0] = _FakeImage(height=page_offset * zoom)
    parser.page_cum_height = [0]
    for page in parser.page_images:
        parser.page_cum_height.append(parser.page_cum_height[-1] + page.size[1] / zoom)
    assert len(parser.page_cum_height) == len(parser.page_images) + 1
    assert parser.page_cum_height[page_index] == page_offset
    parser.page_layout = [[] for _ in range(page_index + 1)]
    parser.page_layout[page_index] = [{"type": "table", "x0": 100, "top": 200, "x1": 220, "bottom": 270}]
    parser.tbl_det = _FakeTableDetector(zoom)
    parser.boxes = [
        {
            "text": "A1",
            "layout_type": "table",
            "layoutno": "table-0",
            "page_number": page_index + 1,
            "x0": 105,
            "x1": 145,
            "top": page_offset + 210,
            "bottom": page_offset + 225,
        },
        {
            "text": "B2",
            "layout_type": "table",
            "layoutno": "table-0",
            "page_number": page_index + 1,
            "x0": 170,
            "x1": 215,
            "top": page_offset + 240,
            "bottom": page_offset + 255,
        },
    ]

    parser._table_transformer_job(ZM=zoom, auto_rotate=False)

    assert [box["R"] for box in parser.boxes] == [0, 1]
    assert [box["R_top"] for box in parser.boxes] == [page_offset + 210, page_offset + 240]
    assert [box["C"] for box in parser.boxes] == [0, 1]
    assert [box["C_left"] for box in parser.boxes] == [105, 170]


@pytest.mark.p1
@pytest.mark.parametrize("angle", [90, 180, 270])
@pytest.mark.parametrize(("page_index", "page_from", "zoom"), [(0, 0, 1), (0, 7, 3), (1, 0, 1), (1, 7, 3)])
def test_table_transformer_keeps_rotated_ocr_and_tsr_coordinates_aligned(monkeypatch, angle, page_index, page_from, zoom):
    from PIL import Image

    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = page_from
    parser.page_cum_height = [400 * i for i in range(page_index + 2)]
    offset = parser.page_cum_height[page_index]
    parser.page_images = [Image.new("RGB", (300 * zoom, 400 * zoom), "white") for _ in range(page_index + 1)]
    parser.page_layout = [[] for _ in parser.page_images]
    parser.page_layout[page_index] = [{"type": "table", "x0": 100, "top": 200, "x1": 220, "bottom": 270}]
    parser.tbl_det = _FakeTableDetector(zoom=zoom, angle=angle)
    original_ocr = _FakeOcr(angle)
    parser.ocr = lambda img: [([(x * zoom, y * zoom) for x, y in points], result) for points, result in original_ocr(img)]
    parser._evaluate_table_orientation = lambda table_img: (
        angle,
        table_img.rotate(-angle, expand=True),
        {},
    )
    parser.boxes = [
        {
            "text": "old A1",
            "layout_type": "table",
            "layoutno": "table-0",
            "page_number": page_index + 1,
            "x0": 105,
            "x1": 145,
            "top": offset + 210,
            "bottom": offset + 225,
        },
        {
            "text": "old B2",
            "layout_type": "table",
            "layoutno": "table-0",
            "page_number": page_index + 1,
            "x0": 170,
            "x1": 215,
            "top": offset + 240,
            "bottom": offset + 255,
        },
    ]

    parser._table_transformer_job(ZM=zoom, auto_rotate=True)

    assert [box["text"] for box in parser.boxes] == ["A1", "B2"]
    assert [box["layoutno"] for box in parser.boxes] == ["table-0", "table-0"]
    assert [box["R"] for box in parser.boxes] == [0, 1]
    assert [box["R_top"] for box in parser.boxes] == [offset + 210, offset + 240]
    assert [box["C"] for box in parser.boxes] == [0, 1]
    assert [box["C_left"] for box in parser.boxes] == [105, 170]
    assert [box["page_number"] for box in parser.boxes] == [page_index + 1, page_index + 1]
    assert parser.get_position(parser.boxes[0], zoom) == [(page_index + 1, 105, 145, 210, 225)]
    tag = parser._line_tag(parser.boxes[0], zoom)
    assert parser.extract_positions(tag) == [([page_index], 105, 145, 210, 225)]
    image, positions = parser.crop(tag, ZM=zoom, need_position=True)
    assert image is not None
    assert positions == [(page_index + page_from, 105, 145, 210, 225)]


@pytest.mark.p1
def test_extract_table_figure_preserves_textless_figures(monkeypatch):
    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = 0
    parser.page_cum_height = [0]
    parser.page_images = [_FakeImage()]
    parser.page_layout = [[{"type": "figure", "x0": 50, "top": 50, "x1": 200, "bottom": 200}]]
    parser.tbl_det = SimpleNamespace(construct_table=lambda *args, **kwargs: "")
    parser.is_english = False

    # Figure box with empty OCR text (e.g. textless diagram, chart, or photo)
    parser.boxes = [
        {
            "text": "",
            "layout_type": "figure",
            "layoutno": "figure-0",
            "page_number": 0,
            "x0": 50,
            "x1": 200,
            "top": 50,
            "bottom": 200,
        }
    ]

    items = parser._extract_table_figure(need_image=True, ZM=1, return_html=False, need_position=True)
    assert len(items) == 1
    (img, descriptions), poss = items[0]
    assert img is not None
    assert isinstance(descriptions, list)
    assert descriptions == [""]
    assert poss == [(0, 50, 200, 50, 200)]


def test_extract_positions_accepts_negative_coordinates(monkeypatch):
    """A content box may extend above the page origin (top < 0).

    ``_line_tag`` writes such tags (``top`` is offset by ``page_cum_height``),
    so ``extract_positions`` must parse them instead of silently dropping the
    section image -- mirrors the Go-side fix in #19945.
    """
    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser
    tag = "@@50\t45.0\t549.7\t-3.0\t737.9##"
    assert parser.extract_positions(tag) == [([49], 45.0, 549.7, -3.0, 737.9)]
    assert parser.remove_tag(f"body {tag} tail") == "body  tail"


def test_extract_positions_accepts_negative_coordinates_with_page_range(monkeypatch):
    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser
    tag = "@@3-4\t1.5\t2.5\t-0.5\t3.5##"
    assert parser.extract_positions(tag) == [([2, 3], 1.5, 2.5, -0.5, 3.5)]


@pytest.mark.parametrize("ocr_kind", ["outside", "low_confidence", "empty", "valid_clipped", "nonfinite"])
def test_rotated_ocr_clamps_to_page_and_restores_if_no_valid_replacement(monkeypatch, ocr_kind):
    import numpy as np
    from PIL import Image

    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = 7
    parser.page_cum_height = np.array([0, 100])
    parser.page_images = [Image.new("RGB", (100, 100), "white")]
    parser.table_rotations = {0: {"best_angle": 180}}
    parser.rotated_table_imgs = {0: Image.new("RGB", (120, 120), "white")}
    original = {"text": "original", "page_number": 1, "layout_type": "table", "x0": 10, "x1": 30, "top": 10, "bottom": 30}
    parser.boxes = [original.copy()]
    if ocr_kind == "outside":
        results = [(_bbox_points((200, 200, 220, 220)), ("outside", 0.99))]
    elif ocr_kind == "low_confidence":
        results = [(_bbox_points((10, 10, 30, 30)), ("low confidence", 0.1))]
    elif ocr_kind == "empty":
        results = []
    elif ocr_kind == "nonfinite":
        results = [(_bbox_points((float("nan"), 0, 30, 30)), ("invalid", 0.99))]
    else:
        results = [(_bbox_points((0, 0, 120, 120)), ("clipped", 0.99))]
    parser.ocr = lambda _image: results
    layouts = [{"table_index": 0, "page": 0, "layout": {"x0": 0, "x1": 100, "top": 0, "bottom": 100}, "layoutno": "table-0", "coords": (-10, -10, 110, 110)}]

    parser._ocr_rotated_tables(1, layouts, [], [0, 1])

    if ocr_kind == "valid_clipped":
        assert len(parser.boxes) == 1
        assert parser.boxes[0]["text"] == "clipped"
        assert parser.get_position(parser.boxes[0], 1) == [(1, 0, 100, 0, 100)]
    else:
        assert parser.boxes == [original]


def test_orientation_confidence_gate_is_retained(monkeypatch):
    from PIL import Image

    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    scores = iter([0.8, 0.99, 0.1, 0.1])
    parser.ocr = lambda _image: [(_bbox_points((1, 1, 2, 2)), ("text", next(scores)))]
    page = Image.new("RGB", (50, 80), "white")
    angle, image, _scores = parser._evaluate_table_orientation(page)
    assert angle == 0
    assert image is page


def test_structured_boxes_keep_each_page_position_before_global_offset(monkeypatch):
    import numpy as np
    from PIL import Image

    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = 7
    parser.page_images = [Image.new("RGB", (100, 100), "white"), Image.new("RGB", (80, 150), "white")]
    parser.page_cum_height = np.array([0, 100, 250])
    parser.boxes = [{"text": "cross page", "page_number": 1, "x0": 10, "x1": 90, "top": 90, "bottom": 120}]
    for name in ("_layouts_rec", "_table_transformer_job", "_text_merge", "_concat_downward", "_naive_vertical_merge"):
        monkeypatch.setattr(parser, name, lambda *_args: None)
    monkeypatch.setattr(parser, "_extract_table_figure", lambda *_args: ([], []))

    boxes = parser._parse_loaded_window_into_bboxes(zoomin=1)

    assert boxes[0]["positions"] == [[1, 10, 90, 90, 100], [2, 10, 80, 0, 20]]
    assert boxes[0]["image"] is not None
    assert parser.extract_positions(boxes[0]["position_tag"]) == [([0], 10, 90, 90, 100), ([1], 10, 80, 0, 20)]
    external = parser._to_global_boxes(boxes)
    assert external[0]["page_number"] == 8
    assert external[0]["positions"] == [[8, 10, 90, 90, 100], [9, 10, 80, 0, 20]]


@pytest.mark.parametrize("layout_type", ["table", "figure"])
@pytest.mark.parametrize("page_from", [0, 7])
@pytest.mark.parametrize("page_count", [1, 2])
def test_insert_table_figures_preserves_all_page_positions(monkeypatch, layout_type, page_from, page_count):
    import numpy as np
    from PIL import Image

    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = page_from
    parser.page_images = [Image.new("RGB", (100, 100), "white"), Image.new("RGB", (80, 150), "white")]
    parser.page_cum_height = np.array([0, 100, 250])
    parser.boxes = [{"text": "anchor", "page_number": 1, "x0": 5, "x1": 20, "top": 50, "bottom": 70}]
    for name in ("_layouts_rec", "_table_transformer_job", "_text_merge", "_concat_downward", "_naive_vertical_merge"):
        monkeypatch.setattr(parser, name, lambda *_args: None)
    positions = [(page_from, 10.9, 90.8, 80.7, 100.0), (page_from + 1, 12.5, 70.2, 0.0, 30.9)][:page_count]
    entry = ((Image.new("RGB", (80, 50), "red"), ["content"]), positions)
    monkeypatch.setattr(parser, "_extract_table_figure", lambda *_args: ([entry], []) if layout_type == "table" else ([], [entry]))

    boxes = parser._parse_loaded_window_into_bboxes(zoomin=1)
    block = next(box for box in boxes if box.get("layout_type") == layout_type)

    expected_local = [[1, 10, 90, 80, 100], [2, 12, 70, 0, 30]][:page_count]
    assert block["positions"] == expected_local
    # Placement still uses the first valid page; only position coverage changes.
    assert block["page_number"] == 1
    assert (block["x0"], block["x1"], block["top"], block["bottom"]) == positions[0][1:]
    assert block["image"].size == (80, 50)
    external = parser._to_global_boxes(boxes)
    block = next(box for box in external if box.get("layout_type") == layout_type)
    assert block["positions"] == [[pn + page_from, *rest] for pn, *rest in expected_local]
    assert block["page_number"] == page_from + 1


@pytest.mark.parametrize("layout_type", ["table", "figure"])
def test_insert_table_figures_filters_invalid_pages_without_losing_valid_ones(monkeypatch, layout_type):
    import numpy as np
    from PIL import Image

    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = 7
    parser.page_images = [Image.new("RGB", (100, 100), "white"), Image.new("RGB", (100, 100), "white")]
    parser.page_cum_height = np.array([0, 100, 200])
    parser.boxes = []
    for name in ("_layouts_rec", "_table_transformer_job", "_text_merge", "_concat_downward", "_naive_vertical_merge"):
        monkeypatch.setattr(parser, name, lambda *_args: None)
    entry = ((Image.new("RGB", (30, 50)), "content"), [(6, 0, 10, 0, 10), (7, 10, 40, 80, 100), (8, 10, 40, 0, 30), (9, 0, 10, 0, 10)])
    monkeypatch.setattr(parser, "_extract_table_figure", lambda *_args: ([entry], []) if layout_type == "table" else ([], [entry]))

    boxes = parser._parse_loaded_window_into_bboxes(zoomin=1)

    assert len(boxes) == 1
    assert boxes[0]["positions"] == [[1, 10, 40, 80, 100], [2, 10, 40, 0, 30]]


@pytest.mark.parametrize("layout_type", ["table", "figure"])
def test_insert_table_figures_with_no_valid_positions_is_skipped(monkeypatch, layout_type):
    import numpy as np
    from PIL import Image

    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = 7
    parser.page_images = [Image.new("RGB", (100, 100), "white")]
    parser.page_cum_height = np.array([0, 100])
    parser.boxes = []
    for name in ("_layouts_rec", "_table_transformer_job", "_text_merge", "_concat_downward", "_naive_vertical_merge"):
        monkeypatch.setattr(parser, name, lambda *_args: None)
    entry = ((Image.new("RGB", (30, 50)), "content"), [(6, 0, 10, 0, 10), (8, 0, 10, 0, 10)])
    monkeypatch.setattr(parser, "_extract_table_figure", lambda *_args: ([entry], []) if layout_type == "table" else ([], [entry]))

    assert parser._parse_loaded_window_into_bboxes(zoomin=1) == []
