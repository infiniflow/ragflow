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
        TableStructureRecognizer=object,
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
@pytest.mark.parametrize(("page_index", "page_offset", "zoom"), [(0, 0, 1), (1, 500, 2)])
def test_table_transformer_maps_tsr_crop_coordinates_to_page_coordinates(monkeypatch, page_index, page_offset, zoom):
    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = 0
    parser.page_cum_height = [0] if page_index == 0 else [0, page_offset]
    parser.page_images = [_FakeImage() for _ in range(page_index + 1)]
    parser.page_layout = [[] for _ in range(page_index + 1)]
    parser.page_layout[page_index] = [{"type": "table", "x0": 100, "top": 200, "x1": 220, "bottom": 270}]
    parser.tbl_det = _FakeTableDetector(zoom)
    parser.boxes = [
        {
            "text": "A1",
            "layout_type": "table",
            "layoutno": "table-0",
            "page_number": page_index,
            "x0": 105,
            "x1": 145,
            "top": page_offset + 210,
            "bottom": page_offset + 225,
        },
        {
            "text": "B2",
            "layout_type": "table",
            "layoutno": "table-0",
            "page_number": page_index,
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
def test_table_transformer_keeps_rotated_ocr_and_tsr_coordinates_aligned(monkeypatch, angle):
    module = _load_pdf_parser(monkeypatch)
    parser = module.RAGFlowPdfParser.__new__(module.RAGFlowPdfParser)
    parser.page_from = 0
    parser.page_cum_height = [0]
    parser.page_images = [_FakeImage()]
    parser.page_layout = [[{"type": "table", "x0": 100, "top": 200, "x1": 220, "bottom": 270}]]
    parser.tbl_det = _FakeTableDetector(zoom=1, angle=angle)
    parser.ocr = _FakeOcr(angle)
    parser._evaluate_table_orientation = lambda table_img: (
        angle,
        _FakeImage(table_img.size[1], table_img.size[0]) if angle in (90, 270) else _FakeImage(*table_img.size),
        {},
    )
    parser.boxes = [
        {
            "text": "old A1",
            "layout_type": "table",
            "layoutno": "table-0",
            "page_number": 0,
            "x0": 105,
            "x1": 145,
            "top": 210,
            "bottom": 225,
        },
        {
            "text": "old B2",
            "layout_type": "table",
            "layoutno": "table-0",
            "page_number": 0,
            "x0": 170,
            "x1": 215,
            "top": 240,
            "bottom": 255,
        },
    ]

    parser._table_transformer_job(ZM=1, auto_rotate=True)

    assert [box["text"] for box in parser.boxes] == ["A1", "B2"]
    assert [box["layoutno"] for box in parser.boxes] == ["table-0", "table-0"]
    assert [box["R"] for box in parser.boxes] == [0, 1]
    assert [box["R_top"] for box in parser.boxes] == [210, 240]
    assert [box["C"] for box in parser.boxes] == [0, 1]
    assert [box["C_left"] for box in parser.boxes] == [105, 170]
