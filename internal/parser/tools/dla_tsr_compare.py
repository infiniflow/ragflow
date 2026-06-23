#!/usr/bin/env python3
"""Call DeepDoc DLA/TSR with the same images Go used, save results as JSON.

Reads:  testdata/output/render_compare/dla_input.png  (Go-rendered page)
        testdata/output/render_compare/tsr_input.png  (Go-cropped table region)
Writes: testdata/output/render_compare/py_dla.json
        testdata/output/render_compare/py_tsr.json

Compare with Go results:
    diff testdata/output/render_compare/go_dla.json testdata/output/render_compare/py_dla.json
    diff testdata/output/render_compare/go_tsr.json testdata/output/render_compare/py_tsr.json

Usage:
    cd /home/shenyushi/cc-workspace/ragflow/internal/parser
    python3 tools/dla_tsr_compare.py [--deepdoc-url http://localhost:8000]
"""
import json, os, sys, argparse, requests
from pathlib import Path

SCRIPT_DIR = Path(__file__).parent
PARSER_DIR = SCRIPT_DIR / ".."
COMPARE_DIR = PARSER_DIR / "testdata" / "output" / "render_compare"

def call_deepdoc(url, endpoint, image_path, extra_fields=None):
    """Send image to DeepDoc and return parsed JSON response."""
    with open(image_path, "rb") as f:
        files = {"request": (os.path.basename(image_path), f, "image/png")}
        data = extra_fields or {}
        resp = requests.post(f"{url}{endpoint}", files=files, data=data, timeout=60)
    resp.raise_for_status()
    return resp.json()

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--deepdoc-url", default="http://localhost:8000")
    args = ap.parse_args()
    url = args.deepdoc_url

    # Health check
    try:
        r = requests.get(f"{url}/health", timeout=5)
        if r.status_code != 200:
            print(f"DeepDoc not healthy at {url}")
            sys.exit(1)
    except Exception as e:
        print(f"DeepDoc not reachable at {url}: {e}")
        sys.exit(1)

    # DLA
    dla_input = COMPARE_DIR / "dla_input.jpeg"
    if not dla_input.exists():
        print(f"Missing {dla_input} — run Go test first: go test -v -tags=integration -run TestDLATSRResponseCompare")
        sys.exit(1)

    print(f"DLA: sending {dla_input} to {url}/predict/dla ...")
    dla_resp = call_deepdoc(url, "/predict/dla", dla_input)
    bboxes = dla_resp.get("bboxes", [])

    # Map class IDs to labels (same as Go's dlaClassLabels)
    CLASS_LABELS = [
        "title", "text", "reference", "figure", "figure caption",
        "table", "table caption", "table caption", "equation", "figure caption",
    ]
    regions = []
    for b in bboxes:
        if len(b) < 6:
            continue
        cls_id = int(b[5])
        label = CLASS_LABELS[cls_id] if 0 <= cls_id < len(CLASS_LABELS) else ""
        regions.append({
            "X0": b[0], "Y0": b[1], "X1": b[2], "Y1": b[3],
            "Confidence": b[4],
            "Label": label,
        })

    dla_out = COMPARE_DIR / "py_dla.json"
    with open(dla_out, "w") as f:
        json.dump(regions, f, indent=2)
    print(f"  {len(regions)} regions → {dla_out}")
    for i, r in enumerate(regions):
        print(f"  region[{i}]: label={r['Label']} conf={r['Confidence']:.3f} bbox=[{r['X0']:.1f}, {r['Y0']:.1f}, {r['X1']:.1f}, {r['Y1']:.1f}]")

    # TSR
    tsr_input = COMPARE_DIR / "tsr_input.jpeg"
    if not tsr_input.exists():
        print(f"\nNo TSR input image — Go test found no table region")
        return

    print(f"\nTSR: sending {tsr_input} to {url}/predict/tsr ...")
    tsr_resp = call_deepdoc(url, "/predict/tsr", tsr_input)
    tsr_bboxes = tsr_resp.get("bboxes", [])

    cells = []
    for b in tsr_bboxes:
        if len(b) < 5:
            continue
        cells.append({"X0": b[0], "Y0": b[1], "X1": b[2], "Y1": b[3]})

    tsr_out = COMPARE_DIR / "py_tsr.json"
    with open(tsr_out, "w") as f:
        json.dump(cells, f, indent=2)
    print(f"  {len(cells)} cells → {tsr_out}")
    for i, c in enumerate(cells):
        print(f"  cell[{i}]: [{c['X0']:.1f}, {c['Y0']:.1f}, {c['X1']:.1f}, {c['Y1']:.1f}]")

    # Quick diff
    print("\n=== Quick comparison ===")
    go_dla = COMPARE_DIR / "go_dla.json"
    if go_dla.exists():
        with open(go_dla) as f:
            go_regions = json.load(f)
        if len(go_regions) == len(regions):
            match = all(
                g["Label"] == p["Label"] and
                abs(g["X0"] - p["X0"]) < 1 and abs(g["Y0"] - p["Y0"]) < 1
                for g, p in zip(go_regions, regions)
            )
            print(f"DLA: Go={len(go_regions)} Py={len(regions)} {'✅ MATCH' if match else '❌ DIFFER'}")
        else:
            print(f"DLA: Go={len(go_regions)} Py={len(regions)} ❌ COUNT DIFFERS")
    else:
        print(f"DLA: Go results not found at {go_dla}")

    go_tsr = COMPARE_DIR / "go_tsr.json"
    if go_tsr.exists() and tsr_input.exists():
        with open(go_tsr) as f:
            go_cells = json.load(f)
        if len(go_cells) == len(cells):
            match = all(
                abs(g["X0"] - p["X0"]) < 1 and abs(g["Y0"] - p["Y0"]) < 1
                for g, p in zip(go_cells, cells)
            )
            print(f"TSR: Go={len(go_cells)} Py={len(cells)} {'✅ MATCH' if match else '❌ DIFFER'}")
        else:
            print(f"TSR: Go={len(go_cells)} Py={len(cells)} ❌ COUNT DIFFERS")

    # OCR Detect
    print(f"\nOCR Detect: sending {dla_input} to {url}/predict/ocr?operator=det ...")
    det_resp = call_deepdoc(url, "/predict/ocr", dla_input, {"operator": "det"})
    det_boxes = []
    for outer in det_resp.get("output", []):
        for page in outer:
            for box in page:
                if len(box) >= 4:
                    det_boxes.append({
                        "X0": box[0][0], "Y0": box[0][1],
                        "X1": box[1][0], "Y1": box[1][1],
                        "X2": box[2][0], "Y2": box[2][1],
                        "X3": box[3][0], "Y3": box[3][1],
                    })
    det_out = COMPARE_DIR / "py_ocr_detect.json"
    with open(det_out, "w") as f:
        json.dump(det_boxes, f, indent=2)
    print(f"  {len(det_boxes)} boxes → {det_out}")

    go_det = COMPARE_DIR / "go_ocr_detect.json"
    if go_det.exists():
        with open(go_det) as f:
            go_boxes = json.load(f)
        if len(go_boxes) == len(det_boxes):
            match = all(
                abs(g["X0"] - p["X0"]) < 1 and abs(g["Y0"] - p["Y0"]) < 1
                for g, p in zip(go_boxes, det_boxes)
            )
            print(f"OCR Detect: Go={len(go_boxes)} Py={len(det_boxes)} {'✅ MATCH' if match else '❌ DIFFER'}")
        else:
            print(f"OCR Detect: Go={len(go_boxes)} Py={len(det_boxes)} ❌ COUNT DIFFERS")

    # OCR Recognize
    rec_input = COMPARE_DIR / "ocr_rec_input.jpeg"
    if rec_input.exists():
        print(f"\nOCR Recognize: sending {rec_input} to {url}/predict/ocr?operator=rec ...")
        rec_resp = call_deepdoc(url, "/predict/ocr", rec_input, {"operator": "rec"})
        texts = []
        for page in rec_resp.get("output", []):
            for item in page:
                for pair in item:
                    if len(pair) >= 2:
                        texts.append({"Text": pair[0], "Confidence": pair[1]})
        rec_out = COMPARE_DIR / "py_ocr_rec.json"
        with open(rec_out, "w") as f:
            json.dump(texts, f, indent=2)
        print(f"  {len(texts)} texts → {rec_out}")

        go_rec = COMPARE_DIR / "go_ocr_rec.json"
        if go_rec.exists():
            with open(go_rec) as f:
                go_texts = json.load(f)
            if len(go_texts) == len(texts):
                match = all(g["Text"] == p["Text"] for g, p in zip(go_texts, texts))
                print(f"OCR Recognize: Go={len(go_texts)} Py={len(texts)} {'✅ MATCH' if match else '❌ DIFFER'}")
            else:
                print(f"OCR Recognize: Go={len(go_texts)} Py={len(texts)} ❌ COUNT DIFFERS")

if __name__ == "__main__":
    main()
