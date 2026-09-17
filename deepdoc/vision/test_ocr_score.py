"""Offline unit tests for score-bearing OCR recognition.

These bypass model loading by constructing OCR / OCRAdapter via
object.__new__ and injecting fake recognizers, so they run without the
DeepDoc weights. They verify:
- recognize_batch_with_score returns (text, score) tuples,
- text below drop_score is blanked while the score is preserved,
- the original recognize_batch (text-only) contract is unchanged,
- OCRAdapter.recognize surfaces the real score instead of a 1.0 fill.
"""

import unittest

from deepdoc.server.adapters.ocr_adapter import OCRAdapter
from deepdoc.vision.ocr import OCR


class FakeRecognizer:
    """Callable stand-in for OCR.text_recognizer[device_id].

    results is a list of recognizer outputs, one per call; each output is a
    list of (text, score) tuples as produced by TextRecognizer.__call__.
    """

    def __init__(self, results):
        self._results = results
        self.calls = 0

    def __call__(self, img_list):
        res = self._results[self.calls]
        self.calls += 1
        return res, 0.0


def make_ocr(results, drop_score=0.5):
    ocr = object.__new__(OCR)
    ocr.text_recognizer = [FakeRecognizer(results)]
    ocr.drop_score = drop_score
    return ocr


class TestRecognizeBatchWithScore(unittest.TestCase):
    def test_returns_score_tuples(self):
        ocr = make_ocr([[["hello", 0.92]]])
        self.assertEqual(ocr.recognize_batch_with_score(["img"]), [("hello", 0.92)])

    def test_blanks_below_drop_score_keeps_score(self):
        ocr = make_ocr([[["garble", 0.10]]], drop_score=0.5)
        # text is blanked by drop_score, but the score is preserved for
        # layer-2 selection.
        self.assertEqual(ocr.recognize_batch_with_score(["img"]), [("", 0.10)])

    def test_original_recognize_batch_unchanged(self):
        # The in-process __ocr path keeps its text-only contract.
        ocr = make_ocr([[["hello", 0.92]]])
        self.assertEqual(ocr.recognize_batch(["img"]), ["hello"])


class TestOCRAdapterScore(unittest.TestCase):
    def test_recognize_surfaces_real_score(self):
        adapter = object.__new__(OCRAdapter)

        class FakeOCR:
            def recognize_batch_with_score(self, imgs):
                return [("world", 0.88)]

        adapter._ocr = FakeOCR()
        adapter._decode_bgr = lambda data: "img"
        out = adapter.recognize(b"fake")
        self.assertEqual(out, {"output": [[[["world", 0.88]]]]})


if __name__ == "__main__":
    unittest.main()
