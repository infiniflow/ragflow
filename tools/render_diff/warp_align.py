#!/usr/bin/env python3
"""WarpCrop (Go) vs an independent perspective-crop reference (cv2) pixel
alignment on REAL document text, with no model weights required.

This exercises the perspective de-skew step (layer 1 of the OCR warp work). It
answers the geometric part of the regression question: for real text regions on
a real PDF page, does Go's util.WarpCrop reproduce the same crop geometry as an
independent perspective transform?

Detection and recognition need the deepdoc ONNX weights (downloaded from
HuggingFace, unavailable offline). They are intentionally NOT used here:
  - quads come from pdfplumber word boxes (rotated to simulate skew) -- no model
  - the reference crop is pure geometry (getPerspectiveTransform +
    warpPerspective with BORDER_REPLICATE + INTER_CUBIC) -- no model

So this runs fully offline and isolates the warp geometry, which is the part
WarpCrop actually implements.

Pipeline
--------
1. genquads : from the Go-rendered page, derive real word boxes (pdfplumber),
              rotate each by a sweep of angles to build skewed quads, write
              quads.json and the reference crops (py_warp_*.png).
2. (Go)     : TestWarpAlignGo pass 2 reads quads.json and writes go_warp_*.png
              (WarpCrop, de-skewed) and go_fastcrop_*.png (FastCrop, axis-aligned bbox).
3. compare  : pixel-MSE Go-WarpCrop vs reference and Go-FastCrop vs reference,
              aggregated (layer-1 geometry; no model weights needed).
4. rec      : run recognize_batch (the actual Go->service path, NO layer-2
              rotation) on all three crops and compare the recognized text
              (layer-1 + recognizer; needs the deepdoc ONNX weights).

All outputs go under --out-dir (default /tmp/render_diff/align); the worktree
is never touched and nothing is committed.

Run
---
    WARP_PDF=test/benchmark/test_docs/Doc1.pdf \
        bash build.sh --test-manual ./internal/deepdoc/parser/pdf/ -run TestWarpAlignGo
    .venv/bin/python tools/render_diff/warp_align.py genquads \
        --pdf test/benchmark/test_docs/Doc1.pdf --go-page-png /tmp/render_diff/align/page0.png
    WARP_PDF=test/benchmark/test_docs/Doc1.pdf \
        bash build.sh --test-manual ./internal/deepdoc/parser/pdf/ -run TestWarpAlignGo
    .venv/bin/python tools/render_diff/warp_align.py compare
    # (needs HF weights in rag/res/deepdoc)
    .venv/bin/python tools/render_diff/warp_align.py rec
"""

import argparse
import difflib
import json
import logging
import math
import os
import sys

import cv2
import numpy as np
import pdfplumber

logging.basicConfig(level=logging.INFO, format="%(message)s", stream=sys.stdout)

# Go renders the page at exactly 3x (216 DPI); pdfplumber coords are in points.
SCALE = 3.0


def _sim(a, b):
    """Normalized similarity in [0,1] between two recognized strings."""
    return difflib.SequenceMatcher(None, a or "", b or "").ratio()


def _quad_from_word(w, angle_deg, W, H, margin, perturb=4.0):
    """Return [x0,y0,x1,y1,x2,y2,x3,y3] (TL,TR,BR,BL) rotated by angle_deg, or
    None if the rotated quad leaves the page.

    A small deterministic perturbation is applied to the bottom-left corner so
    the quad is a genuine trapezoid rather than a parallelogram. A plain
    rotated rectangle maps to a parallelogram, whose getPerspectiveTransform has
    no projective term -- the 8-DOF (true perspective) path in WarpCrop would
    then go untested against cv2. The perturbation makes the cv2 reference
    exercise the full homography the Go code implements.
    """
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
    # Break the parallelogram so cv2 exercises the non-zero projective terms.
    bx, by = rot[3]
    rot[3] = (bx + perturb, by - perturb)
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
            if len(boxes) >= args.max_quads:
                break
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
    logging.info("[genquads] %d quads (from %d words, angles=%s) -> quads.json + py_warp_*.png", len(boxes), len(words), angles)


def pixel_mse(a_path, b_path):
    """Pixel MSE between two crops that MUST share identical dimensions.

    Width/height are part of the WarpCrop <-> cv2 parity contract: a 1px
    dimension difference (e.g. float64 vs float32 truncation in the norm) is a
    real divergence and must surface, not be hidden by resizing. Callers that
    compare the de-skewed WarpCrop crop against the cv2 reference use this and
    treat a shape mismatch as a contract failure.
    """
    A = np.asarray(cv2.imread(a_path), dtype=np.float64)
    B = np.asarray(cv2.imread(b_path), dtype=np.float64)
    if A.shape != B.shape:
        raise ValueError(f"shape mismatch for {os.path.basename(a_path)} vs {os.path.basename(b_path)}: {A.shape} != {B.shape} (dimensions are part of the parity contract)")
    diff = A - B
    return float((diff**2).mean())


def pixel_mse_baseline(a_path, b_path):
    """Pixel MSE for the FastCrop baseline vs cv2.

    FastCrop is axis-aligned while cv2 de-skews, so the two crops legitimately
    differ in size. Resize to a common size to obtain a scalar divergence
    estimate (the baseline is known-wrong; only its magnitude matters).
    """
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
    warp_results = []  # (box_index, mw) so the per-angle summary stays aligned
    better = 0  # cases where WarpCrop is closer to cv2 than FastCrop
    warp_dim_mismatch = 0  # WarpCrop vs cv2 dimension contract failures
    for i in range(min(limit, len(boxes))):
        gw = os.path.join(args.out_dir, f"go_warp_{i}.png")
        gf = os.path.join(args.out_dir, f"go_fastcrop_{i}.png")
        pw = os.path.join(args.out_dir, f"py_warp_{i}.png")
        if not (os.path.exists(gw) and os.path.exists(gf) and os.path.exists(pw)):
            continue
        # WarpCrop must match cv2 exactly in size; a mismatch is a contract
        # failure (previously hidden by resizing) and is not counted as "closer".
        try:
            mw = pixel_mse(gw, pw)
        except ValueError as e:
            warp_dim_mismatch += 1
            logging.warning("[compare] box %d: %s", i, e)
            continue
        mf = pixel_mse_baseline(gf, pw)
        mse_warp.append(mw)
        mse_fc.append(mf)
        warp_results.append((i, mw))
        if mw < mf:
            better += 1

    n = len(mse_warp)
    if n == 0:
        logging.info("[compare] no go crops found; run TestWarpAlignGo pass 2 first")
        return
    mean_w = sum(mse_warp) / n
    mean_f = sum(mse_fc) / n
    logging.info("\n[compare] %d boxes (limit=%s)", n, limit)
    logging.info("  pixel MSE  WarpCrop vs cv2 : mean=%.2f  max=%.2f", mean_w, max(mse_warp))
    logging.info("  pixel MSE  FastCrop vs cv2 : mean=%.2f  max=%.2f", mean_f, max(mse_fc))
    logging.info("  WarpCrop closer to cv2 than FastCrop : %d/%d", better, n)
    if warp_dim_mismatch:
        logging.info("  WarpCrop vs cv2 DIMENSION MISMATCH (contract fail) : %d", warp_dim_mismatch)
    # show per-angle summary if meta present; pair each MSE with its box index
    # so a skipped/missing crop cannot misalign the angle buckets.
    by_angle = {}
    for idx, mw in warp_results:
        ang = meta[idx].get("angle", 0) if idx < len(meta) else 0
        by_angle.setdefault(ang, []).append(mw)
    if by_angle:
        logging.info("\n  -- mean MSE(WarpCrop vs cv2) by rotation angle --")
        for ang in sorted(by_angle):
            vals = by_angle[ang]
            logging.info("    angle %+6.1fdeg : n=%3d  meanMSE=%.2f", ang, len(vals), sum(vals) / len(vals))


def cmd_rec(args):
    """Recognition-level alignment (the decisive "no regression" evidence).

    The Go->Python OCR service feeds cropped image BYTES (no box) to the
    recognizer via recognize_batch(), which does NOT perform layer-2 rotation
    selection. So the apples-to-apples comparison is recognize_batch() on each
    of the three crops already on disk:

      go_warp_*.png     : Go util.WarpCrop  (de-skewed, layer-1 only)
      go_fastcrop_*.png : Go util.FastCrop  (axis-aligned bounding box)
      py_warp_*.png     : cv2.getPerspectiveTransform + warpPerspective
                          (the de-skewed crop Python produces before recognition)

    If go_warp matches py_warp at the text level and both beat go_fastcrop,
    WarpCrop reproduces the de-skewed crop geometry that FastCrop misses.
    """
    from deepdoc.vision.ocr import OCR

    ocr = OCR()
    limit = args.limit if args.limit and args.limit > 0 else 10**9
    n = 0
    warp_eq_py = 0  # WarpCrop text == cv2 text (exact)
    fc_eq_py = 0  # FastCrop text == cv2 text (exact)
    warp_sim_total = 0.0
    fc_sim_total = 0.0
    warp_closer = 0  # WarpCrop text closer to cv2 than FastCrop text
    by_angle = {}
    worst = []  # (sim_warp, i, text_warp, text_py, text_fc)

    # Per-box rotation angle, if the genquads metadata is present.
    meta = []
    meta_path = os.path.join(args.out_dir, "quads_meta.json")
    if os.path.exists(meta_path):
        with open(meta_path) as f:
            meta = json.load(f)

    for i in range(limit):
        gw = os.path.join(args.out_dir, f"go_warp_{i}.png")
        gf = os.path.join(args.out_dir, f"go_fastcrop_{i}.png")
        pw = os.path.join(args.out_dir, f"py_warp_{i}.png")
        if not (os.path.exists(gw) and os.path.exists(gf) and os.path.exists(pw)):
            break
        img_w = cv2.imread(gw)
        img_f = cv2.imread(gf)
        img_p = cv2.imread(pw)
        tw = ocr.recognize_batch([img_w])[0]
        tf = ocr.recognize_batch([img_f])[0]
        tp = ocr.recognize_batch([img_p])[0]

        sw = _sim(tw, tp)
        sf = _sim(tf, tp)
        warp_sim_total += sw
        fc_sim_total += sf
        if tw == tp:
            warp_eq_py += 1
        if tf == tp:
            fc_eq_py += 1
        if sw > sf:
            warp_closer += 1
        # per-angle bucket
        ang = meta[i].get("angle", 0.0) if i < len(meta) else 0.0
        by_angle.setdefault(ang, []).append(sw)
        worst.append((sw, i, tw, tp, tf))
        n += 1

    if n == 0:
        logging.info("[rec] no crops found; run genquads + TestWarpAlignGo pass 2 first")
        return

    logging.info("\n[rec] %d boxes  (recognize_batch, no layer-2 rotation)", n)
    logging.info("  exact text match  WarpCrop vs cv2 : %d/%d (%.1f%%)", warp_eq_py, n, 100 * warp_eq_py / n)
    logging.info("  exact text match  FastCrop vs cv2 : %d/%d (%.1f%%)", fc_eq_py, n, 100 * fc_eq_py / n)
    logging.info("  mean text similarity WarpCrop vs cv2 : %.3f", warp_sim_total / n)
    logging.info("  mean text similarity FastCrop vs cv2 : %.3f", fc_sim_total / n)
    logging.info("  WarpCrop text strictly closer to cv2 than FastCrop : %d/%d (%.1f%%)", warp_closer, n, 100 * warp_closer / n)

    if by_angle:
        logging.info("\n  -- mean text similarity(WarpCrop vs cv2) by rotation angle --")
        for ang in sorted(by_angle):
            vals = by_angle[ang]
            logging.info("    angle %+6.1fdeg : n=%3d  sim=%.3f", ang, len(vals), sum(vals) / len(vals))

    logging.info("\n  -- 8 worst WarpCrop-vs-cv2 cases --")
    worst.sort(key=lambda x: x[0])
    for sw, i, tw, tp, tf in worst[:8]:
        logging.info("    #%3d sim=%.2f | warp=%r py=%r fastcrop=%r", i, sw, tw, tp, tf)


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

    r = sub.add_parser("rec", help="recognition-level alignment (needs deepdoc weights)")
    r.add_argument("--out-dir", default="/tmp/render_diff/align")
    r.add_argument("--limit", type=int, default=0, help="max boxes to recognize (0 = all)")

    args = ap.parse_args()
    os.makedirs(args.out_dir, exist_ok=True)
    if args.cmd == "genquads":
        cmd_genquads(args)
    elif args.cmd == "rec":
        cmd_rec(args)
    else:
        cmd_compare(args)


if __name__ == "__main__":
    main()
