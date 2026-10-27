#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""Regression tests for ``RAGFlowPdfParser._merge_continuation_row_components``
in ``deepdoc/parser/pdf_parser.py``.

Issue: infiniflow/ragflow#18569.

The TSR model over-emits ``table row`` components for cross-page table
continuations with a wrapped paragraph in one column: it emits one component
per wrapped text line instead of one per physical row. Geometry alone cannot
separate those phantom splits from real adjacent rows (both abut or overlap),
so the merge decision uses the OCR text lines inside the table: a boundary is
kept when a new label-column run starts below it or when no text continues
across it at the regular line pitch, and is merged otherwise.

These tests exercise the real static method (not a re-implementation),
loaded from source with the module's heavy runtime dependencies stubbed,
following the pattern of test_pdf_parser_table_coordinates.py.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


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
    def is_caption(_box):
        return False


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

    module_name = "test_pdf_parser_row_merge_module"
    module_path = repo_root / "deepdoc" / "parser" / "pdf_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.fixture(scope="module")
def pdf_parser_module():
    class _Patch:
        def setitem(self, mapping, key, value):
            mapping[key] = value

    return _load_pdf_parser(_Patch())


def _row(top, bottom, label="table row", score=0.5, pn=0, layoutno="table-0", x0=57, x1=555):
    return {"label": label, "top": top, "bottom": bottom, "x0": x0, "x1": x1, "score": score, "pn": pn, "layoutno": layoutno}


def _col(x0, x1, pn=0, layoutno="table-0"):
    return {"label": "table column", "top": 0, "bottom": 2000, "x0": x0, "x1": x1, "score": 0.9, "pn": pn, "layoutno": layoutno}


def _text(top, bottom, x0, x1, text="txt", pn=1):
    return {"text": text, "top": top, "bottom": bottom, "x0": x0, "x1": x1, "page_number": pn, "layout_type": "table"}


# Three-column layout like the issue fixture: label | col2 | col3.
COLS = [_col(57, 118), _col(118, 294), _col(297, 553)]


def _merge(mod, tb_cpns, boxes):
    out = mod.RAGFlowPdfParser._merge_continuation_row_components(tb_cpns, boxes)
    return sorted((c for c in out if c["label"] == "table row"), key=lambda c: c["top"])


def test_issue_18569_reproduction_merges_wrapped_continuation(pdf_parser_module):
    """The cross-page continuation pattern: one component per wrapped line of
    the 拉丁美洲 paragraph collapses into a single row, while the real rows
    around it (header pair, 美國, 加拿大) stay separate."""
    rows = [
        _row(204, 217, score=0.22),  # padding strip over the header
        _row(219, 246),              # header line 1
        _row(247, 277),              # header line 2
        _row(277, 308),              # 美國
        _row(306, 357),              # 加拿大
        _row(357, 381),              # 拉丁美洲, one component per wrapped line
        _row(380, 401),
        _row(399, 421),
        _row(437, 456),
        _row(456, 476),
        _row(475, 493),
        _row(493, 512),
        _row(512, 531),
        _row(531, 550),
        _row(550, 568),
    ]
    boxes = [
        _text(229, 238, 271, 408), _text(234, 244, 68, 108),      # header 1
        _text(252, 264, 77, 99), _text(260, 269, 173, 251), _text(260, 269, 366, 494),  # header 2
        _text(287, 297, 128, 189), _text(287, 297, 308, 369),      # 美國
        _text(321, 333, 58, 89), _text(321, 332, 128, 215), _text(322, 332, 368, 395),  # 加拿大
        _text(341, 350, 127, 206), _text(341, 350, 307, 386),
        _text(371, 382, 59, 109),  # 拉丁美洲與 (wrapped label, line 1)
        _text(391, 400, 59, 98),   # 比海 (wrapped label, line 2)
        _text(368, 378, 128, 294), _text(368, 377, 313, 552),
        _text(387, 396, 127, 254), _text(387, 396, 308, 552),
        _text(406, 414, 308, 546), _text(425, 434, 308, 547),
        _text(444, 452, 308, 552), _text(463, 471, 308, 549),
        _text(481, 490, 308, 553), _text(501, 509, 309, 552),
        _text(519, 528, 308, 552), _text(538, 546, 308, 552),
        _text(557, 565, 309, 525),
    ]
    merged = _merge(pdf_parser_module, rows + COLS, boxes)
    bands = [(c["top"], c["bottom"]) for c in merged]
    assert bands == [(204, 246), (247, 277), (277, 308), (306, 357), (357, 568)]


def test_real_adjacent_rows_kept_apart(pdf_parser_module):
    """Ordinary single-line rows each start a new label run, so no boundary
    is merged even though the components abut."""
    # Real rows carry cell padding: the text gap across a real boundary is
    # about 2x the intra-cell line pitch, as in the issue fixture.
    rows = [_row(100, 130), _row(130, 160), _row(160, 190)]
    boxes = [
        _text(103, 112, 60, 110), _text(103, 112, 130, 280),   # row 1
        _text(133, 142, 60, 110), _text(133, 142, 130, 280),   # row 2
        _text(163, 172, 60, 110), _text(163, 172, 130, 280),   # row 3
    ]
    merged = _merge(pdf_parser_module, rows + COLS, boxes)
    assert [(c["top"], c["bottom"]) for c in merged] == [(100, 130), (130, 160), (160, 190)]


def test_no_column_components_pass_through(pdf_parser_module):
    rows = [_row(100, 120), _row(120, 140)]
    out = pdf_parser_module.RAGFlowPdfParser._merge_continuation_row_components(rows, [])
    assert [(c["top"], c["bottom"]) for c in sorted(out, key=lambda c: c["top"])] == [(100, 120), (120, 140)]


def test_non_row_components_pass_through(pdf_parser_module):
    comps = [
        {"label": "table", "top": 50, "bottom": 600, "x0": 50, "x1": 560, "score": 0.9, "pn": 0, "layoutno": "table-0"},
        {"label": "table spanning cell", "top": 370, "bottom": 560, "x0": 56, "x1": 555, "score": 0.8, "pn": 0, "layoutno": "table-0"},
    ]
    rows = [_row(100, 120), _row(120, 140)]
    out = pdf_parser_module.RAGFlowPdfParser._merge_continuation_row_components(comps + rows + COLS, [])
    assert sum(1 for c in out if c["label"] == "table") == 1
    assert sum(1 for c in out if c["label"] == "table spanning cell") == 1


def test_split_line_component_merges_down(pdf_parser_module):
    """A row component covering no text (split-line artifact) folds into the
    next component instead of producing an empty row."""
    rows = [_row(100, 107, score=0.2), _row(108, 130)]
    boxes = [_text(112, 122, 60, 110), _text(112, 122, 130, 280), _text(124, 129, 130, 200)]
    merged = _merge(pdf_parser_module, rows + COLS, boxes)
    assert [(c["top"], c["bottom"]) for c in merged] == [(100, 130)]


def test_tables_do_not_merge_across_layouts(pdf_parser_module):
    rows_a = [_row(100, 120), _row(120, 140)]
    cols_b = [_col(57, 118, layoutno="table-1"), _col(118, 294, layoutno="table-1")]
    rows_b = [_row(300, 330, layoutno="table-1"), _row(330, 360, layoutno="table-1")]
    boxes = [
        _text(303, 312, 60, 110), _text(303, 312, 130, 280),
        _text(333, 342, 60, 110), _text(333, 342, 130, 280),
    ]
    out = pdf_parser_module.RAGFlowPdfParser._merge_continuation_row_components(rows_a + rows_b + cols_b, boxes)
    bands = sorted((c["top"], c["bottom"]) for c in out if c["label"] == "table row")
    assert bands == [(100, 120), (120, 140), (300, 330), (330, 360)]


def test_score_kept_as_max_when_merging(pdf_parser_module):
    rows = [_row(100, 120, score=0.3), _row(120, 150, score=0.8)]
    boxes = [
        _text(105, 115, 60, 110), _text(105, 115, 130, 280),
        _text(124, 134, 60, 110), _text(124, 134, 130, 280),
    ]
    merged = _merge(pdf_parser_module, rows + COLS, boxes)
    # Wrapped label (133 follows 115 by one pitch) plus tight content gap: merge.
    assert len(merged) == 1
    assert merged[0]["score"] == 0.8


def test_gridline_keeps_compact_single_line_rows(pdf_parser_module):
    """Compact one-line rows (label + single-line cell) are geometrically
    indistinguishable from a wrapped paragraph; a visible separator between
    the components keeps them apart while unruled wrapped lines merge."""
    rows = [_row(100, 120), _row(120, 140), _row(140, 160)]
    boxes = [
        _text(103, 112, 60, 110), _text(103, 112, 130, 280),
        _text(123, 132, 60, 110), _text(123, 132, 130, 280),
        _text(143, 152, 60, 110), _text(143, 152, 130, 280),
    ]
    out = pdf_parser_module.RAGFlowPdfParser._merge_continuation_row_components(
        rows + COLS, boxes, has_separator=lambda _a, _b: True
    )
    assert [(c["top"], c["bottom"]) for c in _rows_only(out)] == [(100, 120), (120, 140), (140, 160)]
    out = pdf_parser_module.RAGFlowPdfParser._merge_continuation_row_components(
        [dict(r) for r in rows] + COLS, boxes, has_separator=lambda _a, _b: False
    )
    assert len(_rows_only(out)) == 1


def _rows_only(comps):
    return sorted((c for c in comps if c["label"] == "table row"), key=lambda c: c["top"])
