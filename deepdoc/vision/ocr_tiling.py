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

from collections import defaultdict
from collections.abc import Callable
from itertools import product

import numpy as np

DEFAULT_TILE_SIZE = 2880
DEFAULT_TILE_OVERLAP = 288
DEFAULT_TILING_THRESHOLD = 4096
DEFAULT_DUPLICATE_OVERLAP = 0.5


def tile_starts(length: int, tile_size: int, overlap: int) -> list[int]:
    """Return full-size tile starts that cover an axis, including its end."""
    if tile_size <= 0:
        raise ValueError("tile_size must be positive")
    if overlap < 0 or overlap >= tile_size:
        raise ValueError("overlap must be non-negative and smaller than tile_size")
    if length <= tile_size:
        return [0]

    step = tile_size - overlap
    starts = list(range(0, length - tile_size + 1, step))
    final_start = length - tile_size
    if starts[-1] != final_start:
        starts.append(final_start)
    return starts


def _box_bounds(box: np.ndarray) -> tuple[float, float, float, float]:
    """Return an axis-aligned bounding rectangle for a quadrilateral."""
    return float(np.min(box[:, 0])), float(np.min(box[:, 1])), float(np.max(box[:, 0])), float(np.max(box[:, 1]))


def _intersection_over_smaller(bounds_a: tuple[float, float, float, float], bounds_b: tuple[float, float, float, float]) -> float:
    """Measure intersection relative to the smaller rectangle."""
    left = max(bounds_a[0], bounds_b[0])
    top = max(bounds_a[1], bounds_b[1])
    right = min(bounds_a[2], bounds_b[2])
    bottom = min(bounds_a[3], bounds_b[3])
    intersection = max(0.0, right - left) * max(0.0, bottom - top)
    area_a = max(0.0, bounds_a[2] - bounds_a[0]) * max(0.0, bounds_a[3] - bounds_a[1])
    area_b = max(0.0, bounds_b[2] - bounds_b[0]) * max(0.0, bounds_b[3] - bounds_b[1])
    smaller_area = min(area_a, area_b)
    return intersection / smaller_area if smaller_area else 0.0


def _axis_aligned_box(bounds: tuple[float, float, float, float]) -> np.ndarray:
    """Convert rectangle bounds to a clockwise quadrilateral."""
    left, top, right, bottom = bounds
    return np.asarray([[left, top], [right, top], [right, bottom], [left, bottom]], dtype=np.float32)


def deduplicate_boxes(
    boxes: list[np.ndarray],
    sources: list[int] | None = None,
    overlap_threshold: float = DEFAULT_DUPLICATE_OVERLAP,
    cell_size: int = DEFAULT_TILE_OVERLAP,
) -> list[np.ndarray]:
    """Merge duplicate detections created by overlapping tiles."""
    if not boxes:
        return []
    if sources is None:
        sources = list(range(len(boxes)))
    if len(sources) != len(boxes):
        raise ValueError("sources and boxes must have the same length")

    bounds = [_box_bounds(box) for box in boxes]
    areas = [(right - left) * (bottom - top) for left, top, right, bottom in bounds]
    candidate_indices = sorted(range(len(boxes)), key=lambda index: areas[index], reverse=True)
    spatial_index: dict[tuple[int, int], list[int]] = defaultdict(list)
    kept_boxes: list[np.ndarray] = []
    kept_bounds: list[tuple[float, float, float, float]] = []
    kept_sources: list[set[int]] = []
    active: list[bool] = []

    for candidate_index in candidate_indices:
        candidate_bounds = bounds[candidate_index]
        left_cell = int(candidate_bounds[0] // cell_size)
        top_cell = int(candidate_bounds[1] // cell_size)
        right_cell = int(candidate_bounds[2] // cell_size)
        bottom_cell = int(candidate_bounds[3] // cell_size)
        cells = [(x, y) for x in range(left_cell, right_cell + 1) for y in range(top_cell, bottom_cell + 1)]
        nearby = {index for cell in cells for index in spatial_index[cell] if active[index]}

        duplicates = [index for index in nearby if sources[candidate_index] not in kept_sources[index] and _intersection_over_smaller(candidate_bounds, kept_bounds[index]) >= overlap_threshold]
        if duplicates:
            duplicate_index = min(duplicates)
            merged_bounds = (
                min(candidate_bounds[0], *(kept_bounds[index][0] for index in duplicates)),
                min(candidate_bounds[1], *(kept_bounds[index][1] for index in duplicates)),
                max(candidate_bounds[2], *(kept_bounds[index][2] for index in duplicates)),
                max(candidate_bounds[3], *(kept_bounds[index][3] for index in duplicates)),
            )
            if merged_bounds != kept_bounds[duplicate_index]:
                kept_boxes[duplicate_index] = _axis_aligned_box(merged_bounds)
            kept_bounds[duplicate_index] = merged_bounds
            kept_sources[duplicate_index] = {sources[candidate_index]}.union(*(kept_sources[index] for index in duplicates))
            for index in duplicates:
                if index != duplicate_index:
                    active[index] = False

            merged_left_cell = int(merged_bounds[0] // cell_size)
            merged_top_cell = int(merged_bounds[1] // cell_size)
            merged_right_cell = int(merged_bounds[2] // cell_size)
            merged_bottom_cell = int(merged_bounds[3] // cell_size)
            for x in range(merged_left_cell, merged_right_cell + 1):
                for y in range(merged_top_cell, merged_bottom_cell + 1):
                    if duplicate_index not in spatial_index[(x, y)]:
                        spatial_index[(x, y)].append(duplicate_index)
            continue

        kept_index = len(kept_boxes)
        kept_boxes.append(boxes[candidate_index])
        kept_bounds.append(candidate_bounds)
        kept_sources.append({sources[candidate_index]})
        active.append(True)
        for cell in cells:
            spatial_index[cell].append(kept_index)

    return [box for index, box in enumerate(kept_boxes) if active[index]]


def detect_tiled_boxes(
    image: np.ndarray,
    detect_tile: Callable[[np.ndarray], np.ndarray | None],
    tile_size: int = DEFAULT_TILE_SIZE,
    overlap: int = DEFAULT_TILE_OVERLAP,
) -> np.ndarray:
    """Detect text in overlapping tiles and map the boxes to image coordinates."""
    height, width = image.shape[:2]
    boxes: list[np.ndarray] = []
    sources: list[int] = []

    tile_origins = product(tile_starts(height, tile_size, overlap), tile_starts(width, tile_size, overlap))
    for tile_index, (top, left) in enumerate(tile_origins):
        tile_boxes = detect_tile(image[top : top + tile_size, left : left + tile_size])
        if tile_boxes is None:
            continue
        for box in tile_boxes:
            global_box = np.asarray(box, dtype=np.float32).copy()
            global_box[:, 0] += left
            global_box[:, 1] += top
            boxes.append(global_box)
            sources.append(tile_index)

    deduplicated = deduplicate_boxes(boxes, sources=sources, cell_size=max(overlap, 1))
    if not deduplicated:
        return np.empty((0, 4, 2), dtype=np.float32)
    return np.asarray(deduplicated, dtype=np.float32)
