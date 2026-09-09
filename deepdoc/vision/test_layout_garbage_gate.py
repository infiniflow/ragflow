"""Offline unit tests for the DLA 0.4 garbage gate.

LayoutRecognizer.forward (deepdoc/vision/layout_recognizer.py:171) is the
shared model-output path used by the /predict/dla HTTP endpoint (via
DLAAdapter). It previously applied only the detection threshold + NMS and
returned raw low-confidence footer/header/reference boxes, diverging from
LayoutRecognizer.__call__ which applies a 0.4 garbage gate
(layout_recognizer.py:97 and :379). These tests pin the gate on forward
without loading the DeepDoc weights: the recognizer is built via
object.__new__ and the base Recognizer.__call__ that forward delegates to is
stubbed.
"""

import unittest
from unittest.mock import patch

import numpy as np

from deepdoc.vision.layout_recognizer import LayoutRecognizer


def _make_rec():
    """Build a LayoutRecognizer without loading the ONNX model."""
    rec = LayoutRecognizer.__new__(LayoutRecognizer)
    rec.garbage_layouts = ["footer", "header", "reference"]
    rec.label_list = [
        "title",
        "text",
        "reference",
        "figure",
        "figure caption",
        "table",
        "table caption",
        "table caption",
        "equation",
        "figure caption",
    ]
    return rec


# Boxes returned by the stubbed base Recognizer.__call__ (one page).
RAW_BOXES = [
    {"type": "reference", "bbox": [1, 2, 3, 4], "score": 0.30},  # low-conf garbage -> dropped
    {"type": "text", "bbox": [1, 2, 3, 4], "score": 0.10},  # low-conf non-garbage -> kept
    {"type": "reference", "bbox": [1, 2, 3, 4], "score": 0.90},  # high-conf garbage -> kept
    {"type": "reference", "bbox": [1, 2, 3, 4], "score": 0.40},  # boundary 0.4 -> kept
]


class TestFilterGarbageLayouts(unittest.TestCase):
    def test_drops_low_conf_garbage_only(self):
        rec = _make_rec()
        kept = rec._filter_garbage_layouts(RAW_BOXES)
        kept_sig = [(b["type"], b["score"]) for b in kept]
        self.assertNotIn(("reference", 0.30), kept_sig)
        self.assertIn(("text", 0.10), kept_sig)
        self.assertIn(("reference", 0.90), kept_sig)
        self.assertIn(("reference", 0.40), kept_sig)


class TestForwardGarbageGate(unittest.TestCase):
    def test_forward_drops_low_conf_garbage(self):
        rec = _make_rec()
        # Stub the base Recognizer.__call__ that forward() delegates to, so no
        # model inference runs.
        with patch("deepdoc.vision.recognizer.Recognizer.__call__", return_value=[RAW_BOXES]):
            out = rec.forward([np.zeros((10, 10, 3), dtype=np.uint8)])
        self.assertEqual(len(out), 1)
        kept_sig = [(b["type"], b["score"]) for b in out[0]]
        self.assertNotIn(("reference", 0.30), kept_sig)
        self.assertIn(("text", 0.10), kept_sig)
        self.assertIn(("reference", 0.90), kept_sig)
        self.assertIn(("reference", 0.40), kept_sig)


if __name__ == "__main__":
    unittest.main()
