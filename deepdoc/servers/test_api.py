"""Quick smoke test for all deepdoc HTTP endpoints."""

import json
import requests

BASE = "http://localhost:8125"

def test_health():
    r = requests.get(f"{BASE}/health", timeout=10)
    assert r.status_code == 200, f"Health failed: {r.status_code}"
    print("Health: OK")

def test_dla():
    with open("/tmp/page0-1.png", "rb") as f:
        r = requests.post(f"{BASE}/predict/dla", files={"request": f}, timeout=60)
    assert r.status_code == 200, f"DLA failed: {r.status_code} {r.text[:200]}"
    d = r.json()
    assert "bboxes" in d, f"DLA missing bboxes: {list(d.keys())}"
    assert len(d["bboxes"]) > 0, "DLA empty"
    class_ids = set(int(b[5]) for b in d["bboxes"])
    print(f"DLA: {len(d['bboxes'])} bboxes, class_ids={class_ids}")

def test_tsr():
    with open("/tmp/table_crop_for_api.jpg", "rb") as f:
        r = requests.post(f"{BASE}/predict/tsr", files={"request": f}, timeout=60)
    assert r.status_code == 200, f"TSR failed: {r.status_code} {r.text[:200]}"
    d = r.json()
    assert "bboxes" in d, f"TSR missing bboxes: {list(d.keys())}"
    assert len(d["bboxes"]) >= 8, f"TSR too few: {len(d['bboxes'])}"
    class_ids = set(int(b[5]) for b in d["bboxes"])
    print(f"TSR: {len(d['bboxes'])} bboxes, class_ids={class_ids}")

def test_ocr_det():
    with open("/tmp/page0-1.png", "rb") as f:
        r = requests.post(f"{BASE}/predict/ocr", files={"request": f, "operator": "det"}, timeout=60)
    assert r.status_code == 200, f"OCR det failed: {r.status_code} {r.text[:200]}"
    d = r.json()
    assert "output" in d, f"OCR det missing output: {list(d.keys())}"
    n = len(d["output"][0][0][0])
    print(f"OCR det: {n} boxes")

def test_ocr_rec():
    with open("/tmp/char_crop_test.jpg", "rb") as f:
        r = requests.post(f"{BASE}/predict/ocr", files={"request": f, "operator": "rec"}, timeout=60)
    assert r.status_code == 200, f"OCR rec failed: {r.status_code} {r.text[:200]}"
    d = r.json()
    text = d["output"][0][0][0]
    print(f"OCR rec: {text}")

def test_error_no_file():
    r = requests.post(f"{BASE}/predict/dla", timeout=10)
    assert r.status_code >= 400, f"Expected 4xx, got {r.status_code}"
    print(f"Error (no file): HTTP {r.status_code}")

def test_error_bad_operator():
    with open("/tmp/page0-1.png", "rb") as f:
        r = requests.post(f"{BASE}/predict/ocr", files={"request": f, "operator": "bad"}, timeout=60)
    assert r.status_code >= 400, f"Expected 4xx, got {r.status_code}"
    print(f"Error (bad operator): HTTP {r.status_code}")

if __name__ == "__main__":
    import sys
    tests = [
        ("health", test_health),
        ("dla", test_dla),
        ("tsr", test_tsr),
        ("ocr_det", test_ocr_det),
        ("ocr_rec", test_ocr_rec),
        ("error_no_file", test_error_no_file),
        ("error_bad_op", test_error_bad_operator),
    ]
    passed = 0
    for name, fn in tests:
        try:
            fn()
            passed += 1
        except Exception as e:
            print(f"FAIL {name}: {e}")
    print(f"\n{passed}/{len(tests)} passed")
    sys.exit(0 if passed == len(tests) else 1)
