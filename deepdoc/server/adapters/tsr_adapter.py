"""TSR adapter — wraps TableStructureRecognizer and converts output to wire format."""

import io
import logging
from typing import List

from PIL import Image

from deepdoc.vision.table_structure_recognizer import TableStructureRecognizer

logger = logging.getLogger(__name__)

# OSS model label → Go tsrLabels index (labels are identical)
# Go-side (internal/parser/deepdoc.go):
#   var tsrLabels = []string{
#       "table", "table column", "table row",
#       "table column header", "table projected row header",
#       "table spanning cell",
#   }
TSR_CLASS_MAP = {
    "table": 0,
    "table column": 1,
    "table row": 2,
    "table column header": 3,
    "table projected row header": 4,
    "table spanning cell": 5,
}


class TSRAdapter:
    """Calls TableStructureRecognizer and converts elements to wire format."""

    def __init__(self, model_dir: str, thr: float = 0.2):
        self.model_dir = model_dir
        self.thr = thr
        self._tsr: TableStructureRecognizer | None = None

    def load(self):
        """Initialize the TSR model. Called once per worker."""
        self._tsr = TableStructureRecognizer()

    def __call__(self, image_data: bytes) -> List[List[float]]:
        """
        Args:
            image_data: JPEG image bytes (cropped table region).

        Returns:
            List of [x0, y0, x1, y1, score, class_id] for each structural element.
        """
        if self._tsr is None:
            raise RuntimeError("TSRAdapter.load() must be called before inference")

        img = Image.open(io.BytesIO(image_data)).convert("RGB")
        width, height = img.size

        tables = self._tsr([img], thr=self.thr)

        result = []
        for tbl_elements in tables:
            for elem in tbl_elements:
                label = elem["label"]
                class_id = TSR_CLASS_MAP.get(label)
                if class_id is None:
                    logger.warning("TSR: unknown label '%s', skipping", label)
                    continue

                x0 = max(0.0, min(float(elem["x0"]), width))
                y0 = max(0.0, min(float(elem["top"]), height))
                x1 = max(0.0, min(float(elem["x1"]), width))
                y1 = max(0.0, min(float(elem["bottom"]), height))
                score = float(elem["score"])

                result.append([x0, y0, x1, y1, score, float(class_id)])

        return result
