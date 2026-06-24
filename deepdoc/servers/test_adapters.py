"""Adapter unit tests and golden-file regression."""

import json
import os
import sys

TESTDATA = os.path.join(os.path.dirname(__file__), "testdata")
os.makedirs(TESTDATA, exist_ok=True)


def _load_image(path):
    with open(path, "rb") as f:
        return f.read()


def test_dla():
    from deepdoc.servers.adapters.dla_adapter import DLAAdapter

    adapter = DLAAdapter(model_dir="rag/res/deepdoc")
    adapter.load()

    data = _load_image("/tmp/page0-1.png")
    result = adapter(data)

    assert len(result) > 0, "empty result"
    valid_ids = {0, 1, 2, 3, 4, 5, 6, 8}
    class_ids = set()
    for r in result:
        assert r[5] in valid_ids, f"invalid class_id {r[5]}"
        assert 0 <= r[0] < r[2], f"bad x: {r[0]}, {r[2]}"
        assert 0 <= r[1] < r[3], f"bad y: {r[1]}, {r[3]}"
        assert 0 <= r[4] <= 1.0, f"score out of range: {r[4]}"
        class_ids.add(int(r[5]))
    assert 5 in class_ids, "no table detected"
    assert 1 in class_ids, "no text detected"

    # Save golden file
    path = os.path.join(TESTDATA, "golden_dla.json")
    with open(path, "w") as f:
        json.dump(result, f)
    print(f"DLA: {len(result)} bboxes, golden saved to {path}")
    return result


def test_tsr():
    from deepdoc.servers.adapters.tsr_adapter import TSRAdapter

    adapter = TSRAdapter(model_dir="rag/res/deepdoc")
    adapter.load()

    data = _load_image("/tmp/table_crop_for_api.jpg")
    result = adapter(data)

    assert len(result) >= 8, f"expected >=8, got {len(result)}"
    valid_ids = {0, 1, 2, 3, 4, 5}
    class_ids = set()
    for r in result:
        assert r[5] in valid_ids, f"invalid class_id {r[5]}"
        class_ids.add(int(r[5]))
    assert 0 in class_ids, "no table"
    assert 1 in class_ids, "no column"
    assert 2 in class_ids, "no row"

    path = os.path.join(TESTDATA, "golden_tsr.json")
    with open(path, "w") as f:
        json.dump(result, f)
    print(f"TSR: {len(result)} bboxes, golden saved to {path}")
    return result


def test_ocr_det():
    from deepdoc.servers.adapters.ocr_adapter import OCRAdapter

    adapter = OCRAdapter(model_dir="rag/res/deepdoc")
    adapter.load()

    data = _load_image("/tmp/page0-1.png")
    result = adapter.detect(data)

    output = result["output"]
    assert len(output) == 1, "batch level"
    assert len(output[0]) == 1, "page level"
    assert len(output[0][0]) == 1, "region level"
    quads = output[0][0][0]
    assert len(quads) >= 45, f"expected >=45 boxes, got {len(quads)}"
    for q in quads:
        assert len(q) == 4, "quad must have 4 points"
        for pt in q:
            assert len(pt) == 2, "point must have 2 coords"
            assert all(isinstance(c, float) for c in pt), "coords must be float"

    path = os.path.join(TESTDATA, "golden_ocr_det.json")
    with open(path, "w") as f:
        json.dump(result, f)
    print(f"OCR det: {len(quads)} boxes, golden saved to {path}")
    return result


def test_ocr_rec():
    import cv2

    from deepdoc.servers.adapters.ocr_adapter import OCRAdapter

    adapter = OCRAdapter(model_dir="rag/res/deepdoc")
    adapter.load()

    # Detect first, then crop
    data = _load_image("/tmp/page0-1.png")
    det = adapter.detect(data)
    quads = det["output"][0][0][0]
    q0 = quads[0]
    img = cv2.imread("/tmp/page0-1.png")
    x0, y0 = int(q0[0][0]), int(q0[0][1])
    x2, y2 = int(q0[2][0]), int(q0[2][1])
    crop = img[max(0, y0):y2, max(0, x0):x2]
    _, buf = cv2.imencode(".jpg", crop)

    result = adapter.recognize(buf.tobytes())
    items = result["output"][0][0]
    assert len(items) >= 1, "no recognition results"
    assert isinstance(items[0][0], str), "text not string"
    assert len(items[0][0].strip()) > 0, "empty text"
    assert items[0][1] == 1.0, "confidence not 1.0"

    path = os.path.join(TESTDATA, "golden_ocr_rec.json")
    with open(path, "w") as f:
        json.dump(result, f)
    print(f"OCR rec: {items}, golden saved to {path}")
    return result


def regression_check(name, current, golden_path):
    """Compare current output against golden file."""
    if not os.path.exists(golden_path):
        print(f"  REGRESSION SKIP {name}: no golden file at {golden_path}")
        return True
    with open(golden_path) as f:
        golden = json.load(f)

    if name == "ocr_det":
        cur_n = len(current["output"][0][0][0])
        gold_n = len(golden["output"][0][0][0])
        ok = cur_n == gold_n
        print(f"  REGRESSION {name}: current={cur_n} golden={gold_n} {'OK' if ok else 'FAIL'}")
        return ok
    elif name == "ocr_rec":
        cur_text = current["output"][0][0][0][0]
        gold_text = golden["output"][0][0][0][0]
        ok = cur_text == gold_text
        print(f"  REGRESSION {name}: current='{cur_text}' golden='{gold_text}' {'OK' if ok else 'FAIL'}")
        return ok
    else:
        cur_n = len(current)
        gold_n = len(golden)
        if cur_n != gold_n:
            print(f"  REGRESSION {name}: count mismatch current={cur_n} golden={gold_n} FAIL")
            return False
        # Check class_id distribution
        cur_cls = sorted(set(int(r[5]) for r in current))
        gold_cls = sorted(set(int(r[5]) for r in golden))
        ok = cur_n == gold_n and cur_cls == gold_cls
        print(f"  REGRESSION {name}: count={cur_n} classes={cur_cls} {'OK' if ok else 'FAIL'}")
        return ok


if __name__ == "__main__":
    do_regression = "--regression" in sys.argv

    tests = [
        ("dla", test_dla),
        ("tsr", test_tsr),
        ("ocr_det", test_ocr_det),
        ("ocr_rec", test_ocr_rec),
    ]

    results = {}
    failed = 0
    for name, fn in tests:
        try:
            results[name] = fn()
        except Exception as e:
            print(f"FAIL {name}: {e}")
            import traceback
            traceback.print_exc()
            failed += 1

    if do_regression:
        reg_failed = 0
        golden_map = {
            "dla": "golden_dla.json",
            "tsr": "golden_tsr.json",
            "ocr_det": "golden_ocr_det.json",
            "ocr_rec": "golden_ocr_rec.json",
        }
        for name in tests:
            gn = name[0]
            if gn in results:
                ok = regression_check(gn, results[gn], os.path.join(TESTDATA, golden_map[gn]))
                if not ok:
                    reg_failed += 1
        failed += reg_failed

    print(f"\n{failed}/{len(tests)} failed")
    sys.exit(1 if failed > 0 else 0)
