#!/usr/bin/env python3
"""WarpCrop (Go) vs cv2.get_rotate_crop_image (Python) pixel alignment on REAL
document text, with no model weights required.

This is the layer-1 (perspective de-skew) half of Step 4. It answers the
geometric part of the regression question: for real text regions on a real PDF
page, does Go's util.WarpCrop reproduce the exact crop Python's OCR pipeline
feeds to the recognizer?

Detection and recognition need the deepdoc ONNX weights (downloaded from
HuggingFace, unavailable offline). They are intentionally NOT used here:
  - quads come from pdfplumber word boxes (rotated to simulate skew) -- no model
  - the cv2 reference crop is pure geometry (getPerspectiveTransform +
    warpPerspective with BORDER_REPLICATE + INTER_CUBIC) -- no model

So this runs fully offline and isolates the warp geometry, which is the part
WarpCrop actually implements.

Pipeline
--------
1. genquads : from the Go-rendered page, derive real word boxes (pdfplumber),
              rotate each by a sweep of angles to build skewed quads, write
              quads.json and the cv2 reference crops (py_warp_*.png).
2. (Go)     : TestWarpAlignGo pass 2 reads quads.json and writes go_warp_*.png
              (WarpCrop) and go_fastcrop_*.png (FastCrop, the old behavior).
3. compare  : pixel-MSE Go-WarpCrop vs cv2 and Go-FastCrop vs cv2, aggregated.

All outputs go under --out-dir (default /tmp/render_diff/align); the worktree
is never touched and nothing is committed.

Run
---
    bash build.sh --test-manual ./internal/deepdoc/parser/pdf/ -run TestWarpAlignGo
    .venv/bin/python tools/render_diff/warp_align.py genquads \
        --pdf test/benchmark/test_docs/Doc1.pdf --go-page-png /tmp/render_diff/align/page0.png
    bash build.sh --test-manual ./internal/deepdoc/parser/pdf/ -run TestWarpAlignGo
    .venv/bin/python tools/render_diff/warp_align.py compare
"""

import argparse
import json
import math
import os

import cv2
import numpy as np
import pdfplumber

# Go renders the page at exactly 3x (216 DPI); pdfplumber coords are in points.
SCALE = 3.0


def _quad_from_word(w, angle_deg, W, H, margin):
    """Return [x0,y0,x1,y1,x2,y2,x3,y3] (TL,TR,BR,BL) rotated by angle_deg, or
    None if the rotated quad leaves the page."""
    x0, top, x1, bottom = w["x0"], w["top"], w["x1"], w["bottom"]
    pts = [
        (x0 * SCALE, top * SCALE),
        (x1 * SCALE, top * SCALE),
        (x1 * SCALE, bottom * SCALE),
        (x0 * SCALE, bottom * SCALE),
    ]
    cx = (x0 + x1) / 2 * SCALE
    cy = (top + bottom) / 2 * SCALE
    a = math.radians(angle_deg)
    ca, sa = math.cos(a), math.sin(a)
    rot = []
    for x, y in pts:
        dx, dy = x - cx, y - cy
        rot.append((cx + dx * ca - dy * sa, cy + dx * sa + dy * ca))
    # in-bounds check
    for x, y in rot:
        if not (margin <= x <= W - margin and margin <= y <= H - margin):
            return None
    return [v for p in rot for v in p]


def _cv2_warp(page, quad):
    p = np.array([[quad[0], quad[1]], [quad[2], quad[3]], [quad[4], quad[5]], [quad[6], quad[7]]], dtype=np.float32)
    w = int(max(np.linalg.norm(p[0] - p[1]), np.linalg.norm(p[2] - p[3])))
    h = int(max(np.linalg.norm(p[0] - p[3]), np.linalg.norm(p[1] - p[2])))
    std = np.array([[0, 0], [w, 0], [w, h], [0, h]], dtype=np.float32)
    M = cv2.getPerspectiveTransform(p, std)
    return cv2.warpPerspective(page, M, (w, h), borderMode=cv2.BORDER_REPLICATE, flags=cv2.INTER_CUBIC)


def cmd_genquads(args):
    page = cv2.imread(args.go_page_png)
    if page is None:
        raise SystemExit(f"cannot read {args.go_page_png}")
    H, W = page.shape[:2]
    with pdfplumber.open(args.pdf) as pdf:
        pg = pdf.pages[args.page]
        words = pg.extract_words()
    angles = [float(x) for x in args.angles.split(",")]
    boxes = []
    for w in words:
        if w["bottom"] - w["top"] < 6:  # skip tiny fragments
            continue
        for ang in angles:
            q = _quad_from_word(w, ang, W, H, args.margin)
            if q is None:
                continue
            idx = len(boxes)
            boxes.append({"quad": q, "text": w.get("text", ""), "angle": ang})
            cv2.imwrite(os.path.join(args.out_dir, f"py_warp_{idx}.png"), _cv2_warp(page, q))
        if len(boxes) >= args.max_quads:
            break
    with open(os.path.join(args.out_dir, "quads.json"), "w") as f:
        json.dump({"boxes": [{"quad": b["quad"]} for b in boxes]}, f, indent=2)
    with open(os.path.join(args.out_dir, "quads_meta.json"), "w") as f:
        json.dump(boxes, f, indent=2)
    print(f"[genquads] {len(boxes)} quads (from {len(words)} words, angles={angles}) -> quads.json + py_warp_*.png")


def pixel_mse(a_path, b_path):
    A = np.asarray(cv2.imread(a_path), dtype=np.float64)
    B = np.asarray(cv2.imread(b_path), dtype=np.float64)
    if A.shape != B.shape:
        B = cv2.resize(B, (A.shape[1], A.shape[0]))
    diff = A - B
    return float((diff**2).mean())


def cmd_compare(args):
    with open(os.path.join(args.out_dir, "quads.json")) as f:
        boxes = json.load(f)["boxes"]
    try:
        with open(os.path.join(args.out_dir, "quads_meta.json")) as f:
            meta = json.load(f)
    except FileNotFoundError:
        meta = [{}] * len(boxes)

    limit = args.limit if args.limit and args.limit > 0 else len(boxes)
    mse_warp, mse_fc = [], []
    better = 0  # cases where WarpCrop is closer to cv2 than FastCrop
    for i in range(min(limit, len(boxes))):
        gw = os.path.join(args.out_dir, f"go_warp_{i}.png")
        gf = os.path.join(args.out_dir, f"go_fastcrop_{i}.png")
        pw = os.path.join(args.out_dir, f"py_warp_{i}.png")
        if not (os.path.exists(gw) and os.path.exists(gf) and os.path.exists(pw)):
            continue
        mw = pixel_mse(gw, pw)
        mf = pixel_mse(gf, pw)
        mse_warp.append(mw)
        mse_fc.append(mf)
        if mw < mf:
            better += 1

    n = len(mse_warp)
    if n == 0:
        print("[compare] no go crops found; run TestWarpAlignGo pass 2 first")
        return
    mean_w = sum(mse_warp) / n
    mean_f = sum(mse_fc) / n
    print(f"\n[compare] {n} boxes (limit={limit})")
    print(f"  pixel MSE  WarpCrop vs cv2 : mean={mean_w:.2f}  max={max(mse_warp):.2f}")
    print(f"  pixel MSE  FastCrop vs cv2 : mean={mean_f:.2f}  max={max(mse_fc):.2f}")
    print(f"  WarpCrop closer to cv2 than FastCrop : {better}/{n}")
    # show per-angle summary if meta present
    by_angle = {}
    for i, m in enumerate(meta[:n]):
        ang = m.get("angle", 0)
        by_angle.setdefault(ang, []).append(mse_warp[i])
    if by_angle:
        print("\n  -- mean MSE(WarpCrop vs cv2) by rotation angle --")
        for ang in sorted(by_angle):
            vals = by_angle[ang]
            print(f"    angle {ang:+6.1f}deg : n={len(vals):3d}  meanMSE={sum(vals) / len(vals):.2f}")


def main():
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)

    g = sub.add_parser("genquads")
    g.add_argument("--pdf", required=True)
    g.add_argument("--page", type=int, default=0)
    g.add_argument("--go-page-png", required=True)
    g.add_argument("--out-dir", default="/tmp/render_diff/align")
    g.add_argument("--angles", default="0,15,-15,30,-30")
    g.add_argument("--max-quads", type=int, default=120)
    g.add_argument("--margin", type=int, default=24)

    c = sub.add_parser("compare")
    c.add_argument("--out-dir", default="/tmp/render_diff/align")
    c.add_argument("--limit", type=int, default=0, help="max boxes to compare (0 = all)")

    args = ap.parse_args()
    os.makedirs(args.out_dir, exist_ok=True)
    if args.cmd == "genquads":
        cmd_genquads(args)
    else:
        cmd_compare(args)


if __name__ == "__main__":
    main()
