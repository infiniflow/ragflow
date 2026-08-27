#!/usr/bin/env bash
# Run the local Go DeepDoc prototype (DLA / TSR / OCR-rec) and compare each
# task against its Python reference script. Proves the Go ports match the
# Python DeepDoc pipeline on CPU via ONNX Runtime.
#
#   bash run.sh            # uses default MODEL_DIR below
#   MODEL_DIR=... bash run.sh
set -euo pipefail
cd "$(dirname "$0")"

# ONNX Runtime is statically linked (libonnxruntime.a) and resolved via
# dlopen(NULL); no ORT_LIB is needed.
MODEL_DIR="${MODEL_DIR:-/home/shenyushi/workspace/ragflow/rag/res/deepdoc}"
PY="${PY:-/home/shenyushi/workspace/ragflow/.venv/bin/python}"
PYTHONPATH="${PYTHONPATH:-/home/shenyushi/workspace/ragflow}"
export MODEL_DIR PYTHONPATH

compare_boxes() {
  # $1 = go json file, $2 = ref json file. Both are a list of
  # [x0,y0,x1,y1,score,class_id] (Go wraps in {"bboxes":[...]}).
  "$PY" - "$1" "$2" <<'PY'
import json, sys, math
go=json.load(open(sys.argv[1]))
if isinstance(go, dict): go=go["bboxes"]
ref=json.load(open(sys.argv[2]))
if isinstance(ref, dict): ref=ref["bboxes"]
def c(b): return (b[0]+b[2])/2,(b[1]+b[3])/2
used=set(); maxd=0.0; n=0
for rb in ref:
    cls=int(rb[5]); best=None; bd=1e18
    for i,vb in enumerate(go):
        if i in used or int(vb[5])!=cls: continue
        d=(c(rb)[0]-c(vb)[0])**2+(c(rb)[1]-c(vb)[1])**2
        if d<bd: bd=1e18 if d>1e17 else d; best=i
    if best is None:
        print("  UNMATCHED ref box class",cls,"center",c(rb)); continue
    used.add(best); n+=1
    for j in range(6):
        if j!=4: maxd=max(maxd, abs(rb[j]-go[best][j]))
print("  matched %d/%d  MAX COORD DIFF=%.4f px"%(n,len(ref),maxd))
PY
}

echo "================ DLA (layout detection, page0.png) ================"
go run . -task dla -image testdata/page0.png | tee /tmp/go_dla.json >/dev/null
"$PY" ref_dla.py testdata/page0.png | tee /tmp/ref_dla.json >/dev/null
echo "Go:"; cat /tmp/go_dla.json
echo "Ref:"; cat /tmp/ref_dla.json
# ref_dla prints a bare list; wrap for the comparator.
echo '{"bboxes":'"$(cat /tmp/go_dla.json | sed 's/^{"bboxes"://;s/}$//')"'}' > /tmp/go_dla_w.json
echo '{"bboxes":'"$(cat /tmp/ref_dla.json)"'}' > /tmp/ref_dla_w.json
echo "diff:"; compare_boxes /tmp/go_dla_w.json /tmp/ref_dla_w.json

echo "================ TSR (table structure, table0.png) ================"
go run . -task tsr -image testdata/table0.png | tee /tmp/go_tsr.json >/dev/null
"$PY" ref_tsr.py testdata/table0.png | tee /tmp/ref_tsr.json >/dev/null
echo "Go:"; cat /tmp/go_tsr.json
echo "Ref:"; cat /tmp/ref_tsr.json
echo '{"bboxes":'"$(cat /tmp/go_tsr.json | sed 's/^{"bboxes"://;s/}$//')"'}' > /tmp/go_tsr_w.json
echo '{"bboxes":'"$(cat /tmp/ref_tsr.json)"'}' > /tmp/ref_tsr_w.json
echo "diff:"; compare_boxes /tmp/go_tsr_w.json /tmp/ref_tsr_w.json

echo "================ OCR-rec (text recognition, line0.png) ================"
go run . -task ocr-rec -image testdata/line0.png | tee /tmp/go_ocr.json >/dev/null
"$PY" ref_ocr_rec.py testdata/line0.png | tee /tmp/ref_ocr.json >/dev/null
echo "Go:"; cat /tmp/go_ocr.json
echo "Ref:"; cat /tmp/ref_ocr.json
"$PY" - <<'PY'
import json
g=json.load(open('/tmp/go_ocr.json'))['output'][0][0][0][0][0]
r=json.load(open('/tmp/ref_ocr.json'))['output'][0][0][0][0][0]
print("TEXT MATCH:" , "OK" if g==r else "MISMATCH")
print("  go :", g)
print("  ref:", r)
PY

echo "================ DET (text detection, DB, page0.png) ================"
# Pure-Go DBPostProcess: expect box COUNT to match and coords within a few px
# (miter offset vs Clipper round offset, convex hull vs exact contours).
go run . -task det -image testdata/page0.png | tee /tmp/go_det.json >/dev/null
"$PY" ref_det.py testdata/page0.png | tee /tmp/ref_det.json >/dev/null
echo "Go:"; cat /tmp/go_det.json
echo "Ref:"; cat /tmp/ref_det.json
"$PY" - <<'PY'
import json, sys
go=json.load(open('/tmp/go_det.json'))['output'][0][0]
ref=json.load(open('/tmp/ref_det.json'))['output'][0][0]
def c(q):
    sx=sum(p[0] for p in q); sy=sum(p[1] for p in q); return sx/4, sy/4
used=set(); maxd=0.0; n=0
for rb in ref:
    rc=c(rb); best=None; bd=1e18
    for i,vb in enumerate(go):
        if i in used: continue
        d=(c(rb)[0]-c(vb)[0])**2+(c(rb)[1]-c(vb)[1])**2
        if d<bd: bd=d; best=i
    if best is None:
        print("  UNMATCHED ref quad center", c(rb)); continue
    used.add(best); n+=1
    for j in range(4):
        for k in range(2):
            maxd=max(maxd, abs(rb[j][k]-go[best][j][k]))
print("  go boxes=%d ref boxes=%d"%(len(go),len(ref)))
print("  matched %d/%d  MAX COORD DIFF=%.4f px"%(n,len(ref),maxd))
PY

echo "================ DONE ================"
