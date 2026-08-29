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

import importlib.util
import sys
from itertools import pairwise
from pathlib import Path
from types import ModuleType

import numpy as np
import pytest

module_path = Path(__file__).resolve().parents[4] / "deepdoc" / "vision" / "ocr_tiling.py"
spec = importlib.util.spec_from_file_location("ocr_tiling_under_test", module_path)
ocr_tiling = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ocr_tiling)

deduplicate_boxes = ocr_tiling.deduplicate_boxes
detect_tiled_boxes = ocr_tiling.detect_tiled_boxes
tile_starts = ocr_tiling.tile_starts


@pytest.fixture
def ocr_module():
    project_root = Path(__file__).resolve().parents[4]
    stub_names = (
        "cv2",
        "onnxruntime",
        "huggingface_hub",
        "common",
        "common.file_utils",
        "common.misc_utils",
        "common.settings",
        "deepdoc",
        "deepdoc.vision",
        "deepdoc.vision.operators",
        "deepdoc.vision.postprocess",
        "deepdoc.vision.ocr_tiling",
        "deepdoc.vision.ocr",
    )
    snapshot = {name: sys.modules.get(name) for name in stub_names}

    def stub(name, **attrs):
        module = ModuleType(name)
        for key, value in attrs.items():
            setattr(module, key, value)
        sys.modules[name] = module
        return module

    stub("cv2")
    stub("onnxruntime")
    stub("huggingface_hub", snapshot_download=lambda *args, **kwargs: None)
    common = stub("common")
    common.__path__ = [str(project_root / "common")]
    stub("common.file_utils", get_project_base_directory=lambda: str(project_root))
    stub("common.misc_utils", pip_install_torch=lambda: None)
    stub("common.settings", PARALLEL_DEVICES=0)
    deepdoc = stub("deepdoc")
    deepdoc.__path__ = [str(project_root / "deepdoc")]
    vision = stub("deepdoc.vision")
    vision.__path__ = [str(project_root / "deepdoc" / "vision")]
    stub("deepdoc.vision.operators")
    stub("deepdoc.vision.postprocess", build_post_process=lambda *_args, **_kwargs: None)
    sys.modules["deepdoc.vision.ocr_tiling"] = ocr_tiling

    ocr_path = project_root / "deepdoc" / "vision" / "ocr.py"
    ocr_spec = importlib.util.spec_from_file_location("deepdoc.vision.ocr", ocr_path)
    module = importlib.util.module_from_spec(ocr_spec)
    sys.modules["deepdoc.vision.ocr"] = module
    ocr_spec.loader.exec_module(module)

    try:
        yield module
    finally:
        for name, previous in snapshot.items():
            if previous is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = previous


def _box(left, top, right, bottom):
    return np.asarray([[left, top], [right, top], [right, bottom], [left, bottom]], dtype=np.float32)


def test_tile_starts_cover_axis_without_small_edge_tiles():
    starts = tile_starts(length=5000, tile_size=2880, overlap=288)

    assert starts == [0, 2120]
    assert starts[-1] + 2880 == 5000
    assert all(next_start <= start + 2880 for start, next_start in pairwise(starts))


def test_text_detector_preserves_single_pass_for_regular_document_images(ocr_module):
    detector = object.__new__(ocr_module.TextDetector)
    expected_boxes = np.asarray([_box(10, 20, 30, 40)])
    calls = []

    def detect(image):
        calls.append(image.shape)
        return expected_boxes, 0.25

    detector._detect = detect
    boxes, elapsed = detector(np.zeros((3508, 2480, 3), dtype=np.uint8))

    assert calls == [(3508, 2480, 3)]
    assert boxes is expected_boxes
    assert elapsed == 0.25


def test_text_detector_tiles_large_images(ocr_module):
    detector = object.__new__(ocr_module.TextDetector)
    calls = []

    def detect(image):
        calls.append(image.shape)
        return np.asarray([_box(0, 0, 20, 10)]), 0.25

    detector._detect = detect
    boxes, elapsed = detector(np.zeros((1000, 5000, 3), dtype=np.uint8))

    assert calls == [(1000, 2880, 3)] * 2
    assert {(int(box[0][0]), int(box[0][1])) for box in boxes} == {(0, 0), (2120, 0)}
    assert elapsed == 0.5


@pytest.mark.parametrize(
    ("width", "expected_calls"),
    [
        (4096, [(100, 4096, 3)]),
        (4097, [(100, 2880, 3)] * 2),
    ],
)
def test_text_detector_uses_strict_tiling_threshold(ocr_module, width, expected_calls):
    detector = object.__new__(ocr_module.TextDetector)
    calls = []

    def detect(image):
        calls.append(image.shape)
        return np.asarray([_box(0, 0, 20, 10)]), 0.25

    detector._detect = detect
    detector(np.zeros((100, width, 3), dtype=np.uint8))

    assert calls == expected_calls


def test_tiled_detection_maps_every_tile_back_to_image_coordinates():
    image = np.zeros((4000, 5000, 3), dtype=np.uint8)
    tile_shapes = []

    def detect_tile(tile):
        tile_shapes.append(tile.shape)
        return np.asarray([_box(0, 0, 20, 10)])

    boxes = detect_tiled_boxes(image, detect_tile)

    assert tile_shapes == [(2880, 2880, 3)] * 4
    assert len(boxes) == 4
    assert {(int(box[0][0]), int(box[0][1])) for box in boxes} == {
        (0, 0),
        (2120, 0),
        (0, 1120),
        (2120, 1120),
    }


def test_tiled_detection_deduplicates_boxes_from_overlap():
    image = np.zeros((1000, 5472, 3), dtype=np.uint8)
    calls = 0

    def detect_tile(_tile):
        nonlocal calls
        calls += 1
        if calls == 1:
            return np.asarray([_box(2600, 100, 2700, 140)])
        return np.asarray([_box(8, 100, 108, 140)])

    boxes = detect_tiled_boxes(image, detect_tile)

    assert calls == 2
    assert len(boxes) == 1
    np.testing.assert_array_equal(boxes[0], _box(2600, 100, 2700, 140))


def test_tiled_detection_preserves_overlapping_boxes_from_one_tile():
    image = np.zeros((1000, 1000, 3), dtype=np.uint8)

    def detect_tile(_tile):
        return np.asarray([_box(100, 100, 200, 140), _box(120, 105, 180, 135)])

    boxes = detect_tiled_boxes(image, detect_tile)

    assert len(boxes) == 2


def test_deduplication_keeps_complete_detection():
    partial_box = _box(100, 100, 180, 140)
    complete_box = _box(90, 95, 190, 145)

    boxes = deduplicate_boxes([partial_box, complete_box])

    assert len(boxes) == 1
    np.testing.assert_array_equal(boxes[0], complete_box)


def test_deduplication_merges_detections_clipped_at_opposite_tile_edges():
    left_tile_box = _box(2500, 100, 2879, 140)
    right_tile_box = _box(2592, 100, 2992, 140)

    boxes = deduplicate_boxes([left_tile_box, right_tile_box])

    assert len(boxes) == 1
    np.testing.assert_array_equal(boxes[0], _box(2500, 100, 2992, 140))


def test_deduplication_merges_every_box_in_a_cross_tile_overlap_chain():
    left_box = _box(0, 0, 100, 40)
    right_box = _box(100, 0, 200, 40)
    bridge_box = _box(50, 0, 150, 40)

    boxes = deduplicate_boxes([left_box, right_box, bridge_box])

    assert len(boxes) == 1
    np.testing.assert_array_equal(boxes[0], _box(0, 0, 200, 40))
