"""Generate golden data for the Go util.WarpCrop unit test.

Produces, under this directory:
  * warp_src.png      - a synthetic source image with high-frequency content
  * warp_expected.png - the perspective-de-skewed crop, computed with PIL's
                        PERSPECTIVE transform (BICUBIC)
  * warp_meta.json    - the 4 source corners (TL,TR,BR,BL) and the expected
                        output size (w,h) consumed by warp_test.go.

The reference perspective transform and the Go WarpCrop implementation compute
the same homogeneous mapping (destination -> source for the backward sampler);
any minor resampling-kernel difference (PIL-bicubic vs the Go Catmull-Rom
sampler) is absorbed by the MSE tolerance in the test.
"""

import base64
import io
import json
import math
import os

from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))

# A general quadrilateral (true perspective, not a parallelogram) inside the
# source image. Order: top-left, top-right, bottom-right, bottom-left.
SRC = [(50, 40), (260, 25), (250, 170), (40, 150)]


def dist(a, b):
    return math.hypot(a[0] - b[0], a[1] - b[1])


def out_size(src):
    w = int(max(dist(src[0], src[1]), dist(src[2], src[3])))
    h = int(max(dist(src[0], src[3]), dist(src[1], src[2])))
    return w, h


def solve_homography(src, dst):
    """Solve the 8-DOF homography mapping src->dst with bottom-right fixed to 1.

    Returns coeffs [a,b,c,d,e,f,g,h] for PIL's PERSPECTIVE:
        x' = (a*x + b*y + c) / (g*x + h*y + 1)
        y' = (d*x + e*y + f) / (g*x + h*y + 1)
    """
    A = [[0.0] * 9 for _ in range(8)]
    b = [0.0] * 8
    for i in range(4):
        sx, sy = src[i]
        dx, dy = dst[i]
        # x' equation.
        A[2 * i][0] = sx
        A[2 * i][1] = sy
        A[2 * i][2] = 1.0
        A[2 * i][6] = -sx * dx
        A[2 * i][7] = -sy * dx
        b[2 * i] = dx
        # y' equation.
        A[2 * i + 1][3] = sx
        A[2 * i + 1][4] = sy
        A[2 * i + 1][5] = 1.0
        A[2 * i + 1][6] = -sx * dy
        A[2 * i + 1][7] = -sy * dy
        b[2 * i + 1] = dy
    # Gaussian elimination with partial pivoting.
    for col in range(8):
        pivot = max(range(col, 8), key=lambda r: abs(A[r][col]))
        A[col], A[pivot] = A[pivot], A[col]
        b[col], b[pivot] = b[pivot], b[col]
        piv = A[col][col]
        for r in range(col + 1, 8):
            f = A[r][col] / piv
            for c in range(col, 9):
                A[r][c] -= f * A[col][c]
            b[r] -= f * b[col]
    x = [0.0] * 8
    for r in range(7, -1, -1):
        s = b[r]
        for c in range(r + 1, 8):
            s -= A[r][c] * x[c]
        x[r] = s / A[r][r]
    return x  # [a,b,c,d,e,f,g,h]


def make_source(path):
    img = Image.new("RGB", (320, 210), (255, 255, 255))
    d = ImageDraw.Draw(img)
    # Border.
    d.rectangle([4, 4, 315, 205], outline=(0, 0, 0), width=2)
    # Solid color blocks (smooth edges -> small resampling-kernel differences).
    d.rectangle([20, 20, 90, 90], fill=(200, 30, 30))
    d.rectangle([110, 30, 170, 100], fill=(30, 160, 40))
    d.rectangle([200, 20, 300, 80], fill=(30, 60, 200))
    # Circle outline (interpolation signal, smooth curvature).
    d.ellipse([40, 120, 130, 200], outline=(0, 0, 0), width=3)
    # A few thick diagonal bars (width 3) to exercise bicubic sampling without
    # pushing content to the Nyquist limit.
    for k in range(0, 160, 28):
        d.line([(175 + k, 110), (175 + k + 60, 200)], fill=(0, 0, 0), width=3)
    img.save(path)


def png_b64(img):
    """Encode a PIL image as a single-line base64 PNG string.

    Golden fixtures are committed as base64 TEXT rather than binary PNG so the
    repo's pre-commit text filters (mixed-line-ending / end-of-file-fixer) can
    never corrupt the binary signature. A trailing newline added to the .b64
    file is harmless: base64 decode ignores surrounding whitespace.
    """
    buf = io.BytesIO()
    img.save(buf, format="PNG")
    return base64.b64encode(buf.getvalue()).decode("ascii")


def main():
    src_path = os.path.join(HERE, "warp_src.png")
    exp_path = os.path.join(HERE, "warp_expected.png")
    src_b64 = os.path.join(HERE, "warp_src.b64")
    exp_b64 = os.path.join(HERE, "warp_expected.b64")
    meta_path = os.path.join(HERE, "warp_meta.json")

    make_source(src_path)

    w, h = out_size(SRC)
    dst = [(0, 0), (w, 0), (w, h), (0, h)]
    # PIL's PERSPECTIVE coeffs map DESTINATION -> SOURCE directly. So solve the
    # homography dst->src, matching the Go WarpCrop implementation (which
    # computes src->dst, then uses its inverse for backward mapping).
    coeffs = solve_homography(dst, SRC)

    img = Image.open(src_path).convert("RGB")
    warped = img.transform((w, h), Image.PERSPECTIVE, coeffs, resample=Image.BICUBIC)
    warped.save(exp_path)

    # Committed (text) golden fixtures.
    with open(src_b64, "w") as f:
        f.write(png_b64(img))
    with open(exp_b64, "w") as f:
        f.write(png_b64(warped))

    with open(meta_path, "w") as f:
        json.dump({"src": SRC, "w": w, "h": h}, f, indent=2)

    print(f"wrote {src_path} ({img.size}), {exp_path} ({warped.size}), {src_b64}, {exp_b64}, {meta_path}")


if __name__ == "__main__":
    main()
