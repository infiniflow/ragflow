#!/usr/bin/env python3
"""Convert a DeepDoc ONNX model into the .ort FlatBuffer the Go DeepDoc backend loads.

WHY THIS EXISTS
---------------
The in-process Go DeepDoc backend links the *reduced-operator* ONNX Runtime
static library (the ~48MB libonnxruntime.a under
~/ragflow-native-libs/onnxruntime/static_lib/...). That library only implements
the modern ONNX op versions. For example, Split takes `split` as an *input
tensor* from opset 13 on — which is exactly how OSS tsr.onnx (opset 17) already
uses it, so OSS models load fine.

Some models were exported at an older opset. EE's tsr.onnx is opset 11, where
Split's `split` is an *attribute*; loading it on the reduced library fails with
"Could not find an implementation for Split(11)". This is NOT a Go bug and NOT a
broken library — the model was simply exported with an op style the library
doesn't register.

This script rewrites the model to a target opset (default 17 — the version the
reduced library supports, matching OSS) via onnx.version_converter, then
serializes it to .ort with the ORT 1.29.0 tool. After that the model runs on the
EXISTING reduced library with no rebuild.

ORT VERSION MATTERS
-------------------
The .ort FlatBuffer is version-sensitive. You MUST run this with the same
onnxruntime the Go binary links (currently 1.29.0). The script refuses to
continue on a different version.

USAGE
-----
    # EE TSR (opset 11 -> 17): writes tsr.ort next to tsr.onnx
    python convert_deepdoc_model.py --input ee_deepdoc_models/tsr/tsr.onnx

    # explicit output dir / opset
    python convert_deepdoc_model.py --input model.onnx --out-dir ./models --opset 17

The produced file is <stem>.ort (model.onnx -> model.ort), ready to drop into
DEEPDOC_MODEL_DIR. The intermediate upgraded .onnx is removed unless
--keep-onnx is given.
"""

import argparse
import os
import shutil
import subprocess
import sys
import tempfile

import onnx
from onnx import version_converter

# The ORT version the Go binary links. .ort files are not forward/backward
# compatible across ORT majors, so this MUST match.
REQUIRED_ORT_VERSION = "1.29.0"

# Opset the reduced ONNX Runtime static library actually supports for the ops
# DeepDoc models use (Split, Slice, Gather, Softmax, ...). OSS tsr.onnx already
# ships at this opset; EE tsr.onnx must be upgraded to it.
DEFAULT_TARGET_OPSET = 17


def find_ort_version():
    try:
        import onnxruntime as ort

        return ort.__version__
    except ImportError:  # pragma: no cover - defensive
        return None


def model_opset(model: onnx.ModelProto) -> int:
    """Max imported opset for the default (ai.onnx) domain."""
    best = 0
    for imp in model.opset_import:
        if imp.domain in ("", "ai.onnx"):
            best = max(best, imp.version)
    return best


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--input", required=True, help="source .onnx model")
    ap.add_argument("--out-dir", default=None, help="output directory (default: alongside --input)")
    ap.add_argument("--opset", type=int, default=DEFAULT_TARGET_OPSET, help=f"target opset (default {DEFAULT_TARGET_OPSET})")
    ap.add_argument("--keep-onnx", action="store_true", help="keep the intermediate upgraded .onnx")
    args = ap.parse_args()

    if not os.path.isfile(args.input):
        print(f"ERROR: input not found: {args.input}", file=sys.stderr)
        return 2

    ort_ver = find_ort_version()
    if ort_ver is None:
        print("ERROR: onnxruntime is not importable in this Python environment.", file=sys.stderr)
        return 2
    if ort_ver != REQUIRED_ORT_VERSION:
        print(f"ERROR: onnxruntime {ort_ver} found, but {REQUIRED_ORT_VERSION} is required (the Go backend links ORT {REQUIRED_ORT_VERSION}; .ort files are version-specific).", file=sys.stderr)
        return 2

    stem = os.path.splitext(os.path.basename(args.input))[0]
    out_dir = args.out_dir or os.path.dirname(os.path.abspath(args.input))
    os.makedirs(out_dir, exist_ok=True)
    final_ort = os.path.join(out_dir, f"{stem}.ort")

    print(f"[1/4] loading {args.input}")
    model = onnx.load(args.input)
    cur = model_opset(model)
    print(f"      current ai.onnx opset = {cur}, target = {args.opset}")

    if cur < args.opset:
        print(f"[2/4] upgrading opset {cur} -> {args.opset} via version_converter")
        model = version_converter.convert_version(model, args.opset)
        new_op = model_opset(model)
        print(f"      upgraded ai.onnx opset = {new_op}")
    else:
        print(f"[2/4] opset {cur} already >= {args.opset}; no upgrade needed")

    work = tempfile.mkdtemp(prefix="ortconv_")
    try:
        up_onnx = os.path.join(work, f"{stem}.onnx")
        print(f"[3/4] saving upgraded onnx -> {up_onnx}")
        onnx.save(model, up_onnx)

        print(f"      converting to .ort (ORT {ort_ver})")
        subprocess.run(
            [sys.executable, "-m", "onnxruntime.tools.convert_onnx_models_to_ort", up_onnx, "--output_dir", work],
            check=True,
        )
        src_ort = os.path.join(work, f"{stem}.ort")
        if not os.path.isfile(src_ort):
            # converter may name it differently; pick the single .ort produced
            orts = [f for f in os.listdir(work) if f.endswith(".ort")]
            if len(orts) != 1:
                print(f"ERROR: expected one .ort in {work}, found {orts}", file=sys.stderr)
                return 1
            src_ort = os.path.join(work, orts[0])

        shutil.move(src_ort, final_ort)
    finally:
        if not args.keep_onnx:
            shutil.rmtree(work, ignore_errors=True)

    # Sanity: confirm the .ort loads and report IO shapes.
    import onnxruntime as ort

    sess = ort.InferenceSession(final_ort, providers=["CPUExecutionProvider"])
    inp = sess.get_inputs()[0]
    out = sess.get_outputs()[0]
    print(f"[4/4] verified {final_ort}")
    print(f"      input  {inp.name} {inp.shape}")
    print(f"      output {out.name} {out.shape}")
    print(f"DONE -> {final_ort}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
