"""OCR adapter — wraps OCR model and converts output to wire format.

Two modes:
- detect: 5-level nested JSON matching Go [][][][][]float64
- rec:    4-level nested JSON matching Go [][][][]any
"""

import logging
from typing import Any, Dict

import cv2
import numpy as np

from deepdoc.vision.ocr import OCR

logger = logging.getLogger(__name__)

# Confidence fill value — OSS recognize_batch does not return confidence scores.
_CONFIDENCE_FILL = 1.0


class OCRAdapter:
    """Calls OCR.detect() and OCR.recognize_batch(), converts to wire format."""

    def __init__(self, model_dir: str):
        self.model_dir = model_dir
        self._ocr: OCR | None = None

    def load(self):
        """Initialize the OCR model. Called once per worker."""
        self._ocr = OCR()

    def close(self):
        """Clean up OCR model resources."""
        if self._ocr is not None:
            try:
                # Access internal detectors and recognizers
                if hasattr(self._ocr, "detector") and self._ocr.detector is not None:
                    self._ocr.detector.close()
            except Exception:
                pass
            try:
                if hasattr(self._ocr, "text_recognizer") and self._ocr.text_recognizer is not None:
                    self._ocr.text_recognizer.close()
            except Exception:
                pass
            self._ocr = None

    def detect(self, image_data: bytes) -> Dict[str, Any]:
        """Run text detection.

        Returns:
            {"output": 5-level nested list} matching Go [][][][][]float64.
        """
        if self._ocr is None:
            raise RuntimeError("OCRAdapter.load() must be called before inference")

        img = self._decode_bgr(image_data)

        # OCR.detect() → [(quad_ndarray, ("", 0)), ...]
        det_result = self._ocr.detect(img)

        quads = []
        for quad_ndarray, _ in det_result:
            quad = quad_ndarray.tolist()  # [[x0,y0],[x1,y1],[x2,y2],[x3,y3]]
            # Convert to Python float for JSON compatibility
            quad = [[float(p[0]), float(p[1])] for p in quad]
            quads.append(quad)

        # 5-level nesting matching Go [][][][][]float64:
        # batch → page → quad → point → coord
        output = [[quads]]
        return {"output": output}

    def recognize(self, image_data: bytes) -> Dict[str, Any]:
        """Run text recognition on a cropped text region.

        Returns:
            {"output": 4-level nested list} matching Go [][][][]any.
        """
        if self._ocr is None:
            raise RuntimeError("OCRAdapter.load() must be called before inference")

        img = self._decode_bgr(image_data)

        # OCR.recognize_batch() returns List[str]; single cropped image → list of 1 image
        texts = self._ocr.recognize_batch([img])

        items = [[text, _CONFIDENCE_FILL] for text in texts]

        # 4-level nesting matching Go [][][][]any:
        # batch → page → items list → pair [text, confidence]
        output = [[items]]
        return {"output": output}

    @staticmethod
    def _decode_bgr(data: bytes) -> np.ndarray:
        """Decode JPEG bytes to BGR numpy array (OCR expects BGR)."""
        arr = np.frombuffer(data, np.uint8)
        img = cv2.imdecode(arr, cv2.IMREAD_COLOR)
        if img is None:
            raise ValueError("Failed to decode image")
        return img
