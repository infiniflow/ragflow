"""DLA adapter — wraps LayoutRecognizer and converts output to wire format."""

import io
import logging
from typing import List

from PIL import Image

from deepdoc.vision import LayoutRecognizer

logger = logging.getLogger(__name__)

# OSS model label → Go dlaClassLabels index
# Go-side (internal/parser/deepdoc.go):
#   var dlaClassLabels = []string{
#       "title", "text", "reference", "figure", "figure caption",
#       "table", "table caption", "table caption", "equation", "figure caption",
#   }
# Indices 4/6/7/9 are duplicates; OSS model only produces unique labels.
DLA_CLASS_MAP = {
    "title": 0,
    "text": 1,
    "reference": 2,
    "figure": 3,
    "figure caption": 4,
    "table": 5,
    "table caption": 6,
    "equation": 8,
}


class DLAAdapter:
    """Calls LayoutRecognizer.forward() and converts bboxes to wire format."""

    def __init__(self, model_dir: str, thr: float = 0.2):
        self.model_dir = model_dir
        self.thr = thr
        self._layouter: LayoutRecognizer | None = None

    def load(self):
        """Initialize the layout recognizer. Called once per worker."""
        self._layouter = LayoutRecognizer("layout")

    def __call__(self, image_data: bytes) -> List[List[float]]:
        """
        Args:
            image_data: JPEG image bytes.

        Returns:
            List of [x0, y0, x1, y1, score, class_id] for each detected layout region.
        """
        if self._layouter is None:
            raise RuntimeError("DLAAdapter.load() must be called before inference")

        img = Image.open(io.BytesIO(image_data)).convert("RGB")
        width, height = img.size

        # forward() returns raw Recognizer output (no OCR integration)
        raw_bboxes = self._layouter.forward([img], thr=self.thr, batch_size=1)[0]

        result = []
        for b in raw_bboxes:
            label = b["type"].lower()
            class_id = DLA_CLASS_MAP.get(label)
            if class_id is None:
                logger.warning("DLA: unknown label '%s', skipping", label)
                continue

            x0, y0, x1, y1 = b["bbox"]
            score = float(b["score"])

            # Clamp coordinates
            x0 = max(0.0, min(float(x0), width))
            y0 = max(0.0, min(float(y0), height))
            x1 = max(0.0, min(float(x1), width))
            y1 = max(0.0, min(float(y1), height))

            result.append([x0, y0, x1, y1, score, float(class_id)])

        return result
