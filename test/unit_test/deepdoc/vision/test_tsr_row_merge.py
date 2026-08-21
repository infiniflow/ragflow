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
"""Regression tests for ``TableStructureRecognizer._merge_phantom_row_components``
in ``deepdoc/vision/table_structure_recognizer.py``.

Issue: infiniflow/ragflow#18569.

The TSR model over-emits ``table row`` components for cross-page table
continuations with a wrapped paragraph in one column: it emits one component
per wrapped text line instead of one per physical row. The bug report
(``asset-recovery-services-sd-zh-tw.pdf``, page 11, table index 2) shows 13
``table row`` components in a 268pt vertical band where the physical table has
5-7 rows. The components sit directly on top of each other (vertical gap
near 0) and the wrapped paragraph text is detected as separate rows, which
inflates the row count 2x or more in the final HTML/Markdown table.

The fix post-processes the TSR output and merges consecutive ``table row``
(plus ``table column header`` and ``table projected row header``)
components whose vertical gap is below ``_PHANTOM_ROW_MERGE_GAP`` and
whose height is at least ``_PHANTOM_ROW_MIN_HEIGHT``. Non-row components
(``table column``, ``table``, ``table spanning cell``) are passed through
unchanged.

These tests exercise the real ``_merge_phantom_row_components`` (not a
re-implementation), loaded from source with the module's heavy runtime
dependencies stubbed.
"""

import importlib.util
import os
import sys
from types import ModuleType

import pytest

# Project-internal / heavy modules that table_structure_recognizer.py imports
# at module load time. The static method _merge_phantom_row_components itself
# uses none of them, so empty stubs are enough. The fixture snapshots and
# restores each entry so neighbouring tests never receive a stub in place of
# the real module.
_STUB_MODULE_NAMES = (
    "cv2",
    "common",
    "common.file_utils",
    "deepdoc",
    "deepdoc.vision",
    "deepdoc.vision.operators",
    "deepdoc.vision.ocr",
    "huggingface_hub",
    "rag",
    "rag.nlp",
    "numpy",
)


@pytest.fixture
def tsr_module():
    project_root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "..", ".."))

    snapshot = {name: sys.modules.get(name) for name in _STUB_MODULE_NAMES}

    def _stub(name, **attrs):
        module = ModuleType(name)
        for key, value in attrs.items():
            setattr(module, key, value)
        sys.modules[name] = module
        return module

    _stub("cv2")
    common = _stub("common")
    common.__path__ = [os.path.join(project_root, "common")]
    _stub("common.file_utils", get_project_base_directory=lambda: project_root)
    deepdoc = _stub("deepdoc")
    deepdoc.__path__ = [os.path.join(project_root, "deepdoc")]
    vision = _stub("deepdoc.vision")
    vision.__path__ = [os.path.join(project_root, "deepdoc", "vision")]
    # table_structure_recognizer imports * from .operators, from . import
    # operators, and from .ocr import Recognizer; supply those names plus
    # load_model for the OCR stub.
    _stub("deepdoc.vision.operators", preprocess=lambda *a, **k: None)
    _stub("deepdoc.vision.ocr", load_model=lambda *a, **k: None)
    _stub("huggingface_hub", snapshot_download=lambda **_kwargs: "")
    rag = _stub("rag")
    rag.__path__ = [os.path.join(project_root, "rag")]

    class _StubNumpy:
        @staticmethod
        def median(values):
            values = sorted(values)
            n = len(values)
            if n == 0:
                return 0.0
            if n % 2 == 1:
                return float(values[n // 2])
            return (values[n // 2 - 1] + values[n // 2]) / 2.0

        @staticmethod
        def mean(values):
            return sum(values) / len(values) if values else 0.0

        @staticmethod
        def min(values):
            return min(values) if values else 0.0

        @staticmethod
        def max(values):
            return max(values) if values else 0.0

    numpy_stub = _StubNumpy()
    sys.modules["numpy"] = numpy_stub

    # table_structure_recognizer.py also imports rag.nlp.rag_tokenizer, but
    # only inside blockType (tokenize). _merge_phantom_row_components does not
    # call blockType, so the stub can be empty.
    class _StubRagNlp:
        class rag_tokenizer:
            @staticmethod
            def tokenize(text):
                return type("Tok", (), {"split": staticmethod(lambda self: text.split())})()

            @staticmethod
            def tag(token):
                return "n"

    _stub("rag.nlp", rag_tokenizer=_StubRagNlp.rag_tokenizer)
    sys.modules["rag.nlp"].rag_tokenizer = _StubRagNlp.rag_tokenizer
    sys.modules["rag.nlp"].__path__ = [os.path.join(project_root, "rag", "nlp")]

    tsr_path = os.path.join(project_root, "deepdoc", "vision", "table_structure_recognizer.py")
    spec = importlib.util.spec_from_file_location("deepdoc.vision.table_structure_recognizer", tsr_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules["deepdoc.vision.table_structure_recognizer"] = module
    spec.loader.exec_module(module)

    try:
        yield module
    finally:
        sys.modules.pop("deepdoc.vision.table_structure_recognizer", None)
        for name in _STUB_MODULE_NAMES:
            if snapshot[name] is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = snapshot[name]


def _row(label, top, bottom, x0=0.0, x1=100.0, score=0.5):
    return {"label": label, "top": top, "bottom": bottom, "x0": x0, "x1": x1, "score": score}


def _bug_18569_row_components():
    """The 13 ``table row`` components reported in #18569, page 11.

    Heights and y-positions taken verbatim from the issue. The first row
    (7.1pt) is the split-line / degenerate detection that the fix drops
    outright; the rest are the wrapped-paragraph lines that the fix merges
    into a single component.
    """
    y0s = [7980.1, 7989.6, 8009.1, 8026.6, 8045.8, 8064.9, 8082.4, 8134.3, 8151.8, 8169.0, 8186.5, 8203.3, 8220.0]
    heights = [7.1, 19.5, 17.2, 19.3, 19.2, 17.4, 51.6, 17.6, 17.5, 17.5, 17.0, 17.6, 28.7]
    return [_row("table row", y, y + h) for y, h in zip(y0s, heights)]


def test_merge_collapses_bug_18569_thirteen_rows_to_one(tsr_module):
    """Regression for #18569.

    The 13 ``table row`` components from the issue's reproduction are
    dropped (the 7.1pt split-line) and merged (the rest) into a single
    component. The remaining components reflect the wrapped paragraph as
    one logical row, matching the physical row count.
    """
    lts = _bug_18569_row_components()
    merged = tsr_module.TableStructureRecognizer._merge_phantom_row_components(lts)

    rows = [b for b in merged if b["label"] == "table row"]
    assert len(rows) == 1
    # The merged component spans the first kept row's top to the last
    # kept row's bottom. The 7.1pt row is dropped, so the top is the
    # second row's top (7989.6) and the bottom is the last row's
    # bottom (8220.0 + 28.7 = 8248.7).
    assert rows[0]["top"] == 7989.6
    assert rows[0]["bottom"] == 8248.7


def test_merge_preserves_well_spaced_real_rows(tsr_module):
    """A real table with normal row spacing must not be merged.

    The fix must only collapse components that are abnormally close
    together; legitimate row components with row-height-scale gaps
    (>= ``_PHANTOM_ROW_MERGE_GAP``) must be preserved as-is.
    """
    lts = [
        _row("table row", 0.0, 40.0),
        _row("table row", 80.0, 120.0),  # 40pt gap
        _row("table row", 160.0, 200.0),  # 40pt gap
        _row("table row", 240.0, 280.0),  # 40pt gap
    ]
    merged = tsr_module.TableStructureRecognizer._merge_phantom_row_components(lts)

    rows = [b for b in merged if b["label"] == "table row"]
    assert len(rows) == 4
    for kept, original in zip(rows, lts):
        assert kept["top"] == original["top"]
        assert kept["bottom"] == original["bottom"]


def test_merge_drops_split_line_below_min_height(tsr_module):
    """A row component with height below ``_PHANTOM_ROW_MIN_HEIGHT`` is
    dropped even when it is the only row component.

    This is the degenerate-detection / split-line case. Dropping it
    prevents the downstream ``construct_table`` from adding a phantom
    grid row.
    """
    lts = [_row("table row", 100.0, 105.0)]  # height 5.0
    merged = tsr_module.TableStructureRecognizer._merge_phantom_row_components(lts)

    rows = [b for b in merged if b["label"] == "table row"]
    assert rows == []


def test_merge_passes_through_non_row_components(tsr_module):
    """``table column``, ``table``, and ``table spanning cell`` components
    must be preserved unchanged regardless of their y position.

    Only ``table row``, ``table column header``, and ``table projected row
    header`` participate in the merge step.
    """
    lts = [
        _row("table column", 0.0, 1000.0),
        _row("table", 0.0, 1000.0),
        _row("table spanning cell", 100.0, 150.0),
        _row("table row", 200.0, 240.0),
        _row("table column header", 242.0, 280.0),  # 2pt gap, merges with row above
        _row("table projected row header", 320.0, 360.0),  # 40pt gap, stays separate
    ]
    merged = tsr_module.TableStructureRecognizer._merge_phantom_row_components(lts)

    by_label = {b["label"]: b for b in merged}
    assert by_label["table column"]["top"] == 0.0
    assert by_label["table column"]["bottom"] == 1000.0
    assert by_label["table"]["top"] == 0.0
    assert by_label["table"]["bottom"] == 1000.0
    assert by_label["table spanning cell"]["top"] == 100.0
    assert by_label["table spanning cell"]["bottom"] == 150.0
    # The first pair of row-like components merges (2pt gap); the third
    # stands alone (40pt gap).
    rows = [b for b in merged if b["label"] in {"table row", "table column header", "table projected row header"}]
    assert len(rows) == 2


def test_merge_combines_close_header_and_body_rows(tsr_module):
    """``table column header`` components participate in the merge step
    too, so a header row directly above a body row with negligible gap
    is collapsed (which is the correct behaviour for a continuation that
    starts with a header cell).
    """
    lts = [
        _row("table column header", 0.0, 18.0),
        _row("table row", 19.0, 36.0),  # 1pt gap
    ]
    merged = tsr_module.TableStructureRecognizer._merge_phantom_row_components(lts)
    rows = [b for b in merged if b["label"] in {"table row", "table column header"}]
    assert len(rows) == 1
    assert rows[0]["top"] == 0.0
    assert rows[0]["bottom"] == 36.0


def test_merge_keeps_max_score_of_merged_components(tsr_module):
    """When two components merge, the resulting ``score`` is the max of
    the two. Downstream code that filters by score still sees the most
    confident member of the merged group.
    """
    lts = [
        _row("table row", 0.0, 20.0, score=0.42),
        _row("table row", 22.0, 40.0, score=0.81),  # 2pt gap, higher score
    ]
    merged = tsr_module.TableStructureRecognizer._merge_phantom_row_components(lts)
    rows = [b for b in merged if b["label"] == "table row"]
    assert len(rows) == 1
    assert rows[0]["score"] == 0.81
    assert rows[0]["top"] == 0.0
    assert rows[0]["bottom"] == 40.0


def test_merge_no_op_when_only_columns_present(tsr_module):
    """If a table image has only ``table column`` components (no rows),
    the merge step is a no-op. This guards the early-return path inside
    ``__call__`` where the column snapping is skipped when there are no
    columns.
    """
    lts = [_row("table column", 0.0, 100.0)]
    merged = tsr_module.TableStructureRecognizer._merge_phantom_row_components(lts)
    assert merged == lts
