#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

"""Regression tests for layout postprocess bounds-checking.

The DeepDOC layout ONNX model (``LayoutRecognizer4YOLOv10``) occasionally emits
a detection box whose predicted class id (the last output column) falls outside
``label_list``. Indexing ``self.label_list[class_ids[i]]`` without a bounds
check raised ``IndexError: list index out of range`` and crashed the whole page
task (observed on a 13-page datasheet slice, doc pages 37~49). The sibling
``postprocess`` in ``recognizer.py`` already guarded this with
``if clsid >= len(self.label_list)``; these tests pin the same guard onto the
list-comprehension variants (YOLOv10 last-column path and the base argmax path).
"""

import numpy as np
import pytest

from deepdoc.vision.layout_recognizer import LayoutRecognizer4YOLOv10
from deepdoc.vision.recognizer import Recognizer

LABELS = LayoutRecognizer4YOLOv10.labels  # 10 labels -> valid ids 0..9


def _bare(cls, labels):
    """An instance whose ``postprocess`` runs without loading the ONNX model.

    ``postprocess`` only needs ``self.label_list``; bypass ``__init__`` (which
    would download/load layout.onnx) via ``object.__new__``.
    """
    r = object.__new__(cls)
    r.label_list = labels
    r.input_names = []  # so Recognizer.postprocess takes the array (argmax) branch
    return r


def _boxes_last_col_classid(class_ids):
    # YOLOv10 output columns: x0, y0, x1, y1, score, class_id(last)
    rows = []
    for k, cid in enumerate(class_ids):
        x = 10.0 + k * 50
        rows.append([x, x, x + 20, x + 20, 0.9, float(cid)])
    return np.array(rows, dtype=np.float32)


def _boxes_argmax_classid(class_ids, n_cols):
    # Base argmax output: [x, y, w, h, <n_cols score columns>]; the recognizer
    # squeezes then transposes, so hand it shape [1, 4 + n_cols, N].
    rows = []
    for k, cid in enumerate(class_ids):
        x = 10.0 + k * 50
        cols = [x, x, 20.0, 20.0] + [0.0] * n_cols
        cols[4 + cid] = 0.9  # make argmax land on cid
        rows.append(cols)
    return np.array(rows, dtype=np.float32).T[np.newaxis, ...]


@pytest.mark.p2
class TestYOLOv10PostprocessOutOfRangeClassId:
    def test_out_of_range_class_id_does_not_crash(self):
        r = _bare(LayoutRecognizer4YOLOv10, LABELS)
        n = len(LABELS)  # 10 -> valid ids 0..9
        boxes = _boxes_last_col_classid([1, n + 5, 2])  # middle one is garbage
        inputs = {"scale_factor": [1.0, 1.0, 0.0, 0.0]}
        out = r.postprocess(boxes, inputs, 0.2)  # must not raise
        types = {o["type"] for o in out}
        assert "text" in types  # valid box survived
        assert types <= {label.lower() for label in LABELS}  # no invalid label leaked

    def test_all_out_of_range_returns_empty_not_crash(self):
        r = _bare(LayoutRecognizer4YOLOv10, LABELS)
        n = len(LABELS)
        boxes = _boxes_last_col_classid([n, n + 1])  # every box garbage
        inputs = {"scale_factor": [1.0, 1.0, 0.0, 0.0]}
        assert r.postprocess(boxes, inputs, 0.2) == []


@pytest.mark.p2
class TestRecognizerArgmaxPostprocessOutOfRange:
    def test_argmax_path_guards_out_of_range(self):
        r = _bare(Recognizer, LABELS)
        n = len(LABELS)
        # The only positive score column sits beyond label_list, so argmax
        # yields an out-of-range class id for the second box.
        boxes = _boxes_argmax_classid([1, n + 3], n_cols=n + 6)
        inputs = {"scale_factor": [1.0, 1.0, 1.0, 1.0]}
        out = r.postprocess(boxes, inputs, 0.2)  # must not raise
        assert {o["type"] for o in out} <= {label.lower() for label in LABELS}
