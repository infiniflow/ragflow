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
"""Regression tests for ``Recognizer.find_horizontally_tightest_fit`` in
``deepdoc/vision/recognizer.py``.

Issue: infiniflow/ragflow#17199.

Table columns are matched to a cell by ``find_horizontally_tightest_fit``,
which filtered candidates by ``layoutno`` only. ``layoutno`` is
``f"table-{page_table_index}"`` (see ``deepdoc/parser/pdf_parser.py`` where it
is assigned) and the index resets on every page, so ``"table-0"`` names a
different table on each page. ``clmns`` at the call site spans the whole
document, so a cell on page N could bind to a same-index column from page 1
whenever that column's x range happened to be horizontally closer, producing a
wrong column count / broken cell coordinates for multi-page multi-table PDFs.

The fix adds a vertical-extent guard: a candidate is only considered when it
shares vertical span with the box. ``top``/``bottom`` are page-cumulative at
this point (``page_cum_height`` is added to both cells and columns), so this
also rejects any same-``layoutno`` candidate that lives on a different page.

These tests exercise the real ``Recognizer.find_horizontally_tightest_fit``
(not a re-implementation), loaded from source with the module's heavy runtime
dependencies stubbed.
"""

import importlib.util
import os
import sys
from types import ModuleType

import pytest

# Project-internal / heavy modules that recognizer.py imports at module load
# time. find_horizontally_tightest_fit itself uses none of them, so an empty
# stub is enough to import the module. The fixture snapshots and restores each
# entry so neighbouring tests never receive a stub in place of the real module.
_STUB_MODULE_NAMES = (
    "cv2",
    "common",
    "common.file_utils",
    "deepdoc",
    "deepdoc.vision",
    "deepdoc.vision.operators",
    "deepdoc.vision.ocr",
)


@pytest.fixture
def recognizer_module():
    project_root = os.path.abspath(
        os.path.join(os.path.dirname(__file__), "..", "..", "..", "..")
    )

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
    # recognizer does `from .operators import *`, `from . import operators`
    # and `from .ocr import load_model`; supply just those names.
    _stub("deepdoc.vision.operators", preprocess=lambda *a, **k: None)
    _stub("deepdoc.vision.ocr", load_model=lambda *a, **k: None)

    recognizer_path = os.path.join(
        project_root, "deepdoc", "vision", "recognizer.py"
    )
    spec = importlib.util.spec_from_file_location(
        "deepdoc.vision.recognizer", recognizer_path
    )
    module = importlib.util.module_from_spec(spec)
    sys.modules["deepdoc.vision.recognizer"] = module
    spec.loader.exec_module(module)

    try:
        yield module
    finally:
        sys.modules.pop("deepdoc.vision.recognizer", None)
        for name in _STUB_MODULE_NAMES:
            if snapshot[name] is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = snapshot[name]


def _column(layoutno, x0, x1, top, bottom):
    return {"layoutno": layoutno, "x0": x0, "x1": x1, "top": top, "bottom": bottom}


def test_column_fit_does_not_bind_across_pages(recognizer_module):
    """Regression for #17199.

    A cell on page 2 must not bind to a page-1 column that merely shares the
    reset ``layoutno`` ``"table-0"`` and happens to be horizontally closer.

    Page coordinates are cumulative, so page-2 content sits below the page-1
    band. ``clmns`` here mixes both pages, as it does at the real call site.
    """
    Recognizer = recognizer_module.Recognizer

    # Page 2 cell: page height is ~1000, so its cumulative top/bottom are >1000.
    box = {"layoutno": "table-0", "x0": 100.0, "x1": 200.0, "top": 1050.0, "bottom": 1080.0}
    clmns = [
        # index 0: page-1 column, same layoutno, x range closer to the box.
        _column("table-0", 101.0, 201.0, 50.0, 90.0),
        # index 1: page-2 column, the correct match (same page), slightly farther.
        _column("table-0", 110.0, 210.0, 1000.0, 1100.0),
    ]

    assert Recognizer.find_horizontally_tightest_fit(box, clmns) == 1


def test_column_fit_still_picks_closest_within_same_page(recognizer_module):
    """The guard must not change intra-page selection: among columns that share
    the cell's page/vertical band, the horizontally tightest one still wins.
    """
    Recognizer = recognizer_module.Recognizer

    box = {"layoutno": "table-0", "x0": 100.0, "x1": 150.0, "top": 210.0, "bottom": 240.0}
    clmns = [
        _column("table-0", 10.0, 60.0, 200.0, 270.0),    # far left
        _column("table-0", 98.0, 152.0, 200.0, 270.0),   # closest, same band
        _column("table-0", 300.0, 360.0, 200.0, 270.0),  # far right
    ]

    assert Recognizer.find_horizontally_tightest_fit(box, clmns) == 1


def test_column_fit_returns_none_when_only_candidate_is_on_another_page(recognizer_module):
    """If the only same-``layoutno`` candidate is on a different page, no column
    is assigned rather than a wrong one.
    """
    Recognizer = recognizer_module.Recognizer

    box = {"layoutno": "table-0", "x0": 100.0, "x1": 200.0, "top": 1050.0, "bottom": 1080.0}
    clmns = [_column("table-0", 100.0, 200.0, 50.0, 90.0)]  # page 1 only

    assert Recognizer.find_horizontally_tightest_fit(box, clmns) is None


def test_column_fit_still_filters_by_layoutno(recognizer_module):
    """A candidate on the same page but a different table (different layoutno)
    is still excluded, as before.
    """
    Recognizer = recognizer_module.Recognizer

    box = {"layoutno": "table-0", "x0": 100.0, "x1": 200.0, "top": 210.0, "bottom": 240.0}
    clmns = [_column("table-1", 100.0, 200.0, 200.0, 270.0)]  # same page, other table

    assert Recognizer.find_horizontally_tightest_fit(box, clmns) is None
