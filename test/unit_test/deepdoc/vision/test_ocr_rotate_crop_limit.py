"""
Regression: OpenCV warpPerspective/remap rejects src/dst dims >= SHRT_MAX.

Oversized PDF page renders (e.g. wide infographics at zoomin=3) used to crash
chunking inside OCR.get_rotate_crop_image with:

  error: (-215:Assertion failed) dst.cols < SHRT_MAX && ... in function remap
"""

from unittest.mock import MagicMock

import numpy as np

from deepdoc.vision.ocr import OCR, _OPENCV_REMAP_MAX_DIM


def _ocr_stub() -> OCR:
    """Bypass model loading; only exercise get_rotate_crop_image geometry."""
    ocr = object.__new__(OCR)
    ocr.text_recognizer = [MagicMock()]
    return ocr


def test_opencv_remap_max_dim_below_shrt_max():
    assert _OPENCV_REMAP_MAX_DIM < 32767


def test_get_rotate_crop_image_handles_oversized_source():
    """Source wider than SHRT_MAX must not raise; crop stays under the limit."""
    ocr = _ocr_stub()
    # ~10MB strip; wide enough to trip OpenCV remap without allocating a square.
    width = _OPENCV_REMAP_MAX_DIM + 100
    height = 64
    img = np.zeros((height, width, 3), dtype=np.uint8)
    img[:, :] = (40, 80, 120)
    points = np.float32([[10, 10], [200, 10], [200, 50], [10, 50]])

    cropped = ocr.get_rotate_crop_image(img, points)

    assert cropped is not None
    assert cropped.ndim == 3
    assert cropped.shape[0] <= _OPENCV_REMAP_MAX_DIM
    assert cropped.shape[1] <= _OPENCV_REMAP_MAX_DIM
    assert cropped.shape[0] > 0 and cropped.shape[1] > 0


def test_get_rotate_crop_image_normal_page_unchanged_size():
    """In-bounds crops on normal pages keep the expected output geometry."""
    ocr = _ocr_stub()
    img = np.zeros((200, 400, 3), dtype=np.uint8)
    points = np.float32([[10, 20], [110, 20], [110, 60], [10, 60]])

    cropped = ocr.get_rotate_crop_image(img, points)

    assert cropped.shape[1] == 100
    assert cropped.shape[0] == 40


def test_get_rotate_crop_image_clamps_out_of_bounds_quad():
    ocr = _ocr_stub()
    img = np.zeros((100, 100, 3), dtype=np.uint8)
    points = np.float32([[-50, -50], [500, -50], [500, 500], [-50, 500]])

    cropped = ocr.get_rotate_crop_image(img, points)

    assert cropped.shape[0] <= _OPENCV_REMAP_MAX_DIM
    assert cropped.shape[1] <= _OPENCV_REMAP_MAX_DIM
    assert cropped.shape[0] >= 1 and cropped.shape[1] >= 1
