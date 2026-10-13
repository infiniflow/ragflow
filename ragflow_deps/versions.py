#!/usr/bin/env python3
"""Single source of truth for dependency versions and download URLs.

Imported by:
  - ragflow_deps/download_native_deps.py  (native build libraries)
  - ragflow_deps/download_deps.py         (image / runtime resources)

Pure constants plus URL-builder functions, so a version bump touches exactly
one constant here. build.sh greps the constants below (it is pure shell and
must stay Python-free), so keep the `NAME = "value"` form greppable.

The one exception is ONNX Runtime: its version is also embedded in the runtime
as internal/common.DeepDocORTVersion and in Dockerfile as ARG ORT_VERSION.
build.sh --check-ort-version fails fast when they drift, so a bump must still
touch all three — but every other version lives only here.

Mirror endpoints (HF_ENDPOINT / GH_PROXY) are read from the environment at
call time: export them, or pass --china-mirrors to either downloader.
"""

import os
import re

# ONNX Runtime (statically linked into the Go in-process DeepDoc backend).
#
# Source of the native static archives: infiniflow/ragflow-build (our own
# ORT-only minimal build), NOT the third-party csukuangfj/onnxruntime-libs
# account. The release tag is `onnxruntime-v{ORT_VERSION}` and the asset is
# `onnxruntime-v{ORT_VERSION}-linux-x86_64.zip`. The archive is occasionally
# re-issued under this SAME tag/asset name with patched content (e.g. the
# build that exports SessionGetInitializer*); the downloader detects that via
# a `{asset}.sha256` sidecar and re-downloads/re-extracts, so a stale local
# copy never silently lingers.
ORT_VERSION = "1.29.0"

# stagehand-server release binaries (used by the Browser component in local
# mode). They are LOOSELY matched to the stagehand-go Go module: both stay on
# the v3.x line and remain protocol-compatible, but the two version numbers do
# NOT track each other, so there is no cross-reference to keep in sync here.
# Kept with the "v" prefix because it is used verbatim in the release tag path.
STAGEHAND_SERVER_VERSION = "v3.7.2"

# pdf_oxide go-ffi release. Bare module version (the "v" is added by
# pdf_oxide_url); matches the value build.sh uses to validate the embedded
# .a stamp.
PDF_OXIDE_VERSION = "0.3.73"

# pdfium release, as the numeric build number after "chromium%2F" (the "v" and
# the URL-encoded slash are added by pdfium_url). Matches build.sh's
# PDFIUM_STATIC_VERSION.
PDFIUM_VERSION = "7809"

# Optional fixed Hugging Face revision (commit / branch / tag) for reproducible
# tokenizer/model downloads. Empty => resolve from the repo default branch.
HF_REVISION = ""


def hf_endpoint() -> str:
    """Hugging Face endpoint, overridable for mirrors (hf-mirror.com)."""
    return os.environ.get("HF_ENDPOINT") or "https://huggingface.co"


def gh_proxy() -> str:
    """GitHub release proxy prefix (gh-proxy.com behind the GFW), else empty."""
    return os.environ.get("GH_PROXY") or ""


def go_module_version(module: str, repo_root: str) -> str:
    """Read a Go module's pinned version (the "vX.Y.Z" after its require line)
    from `repo_root`/go.mod. Raises RuntimeError when go.mod or the module is
    missing — the caller cannot link the right library without it."""
    go_mod = os.path.join(repo_root, "go.mod")
    if not os.path.isfile(go_mod):
        raise RuntimeError(f"go.mod not found at {go_mod}")
    with open(go_mod, encoding="utf-8") as file:
        match = re.search(rf"{re.escape(module)}\s+v([^\s]+)", file.read())
    if match is None:
        raise RuntimeError(f"{module} version not found in {go_mod}")
    return match.group(1)


# ── URL builders ───────────────────────────────────────────────────────────
# Every download URL in the project is assembled from the constants above via
# one of these helpers, so a version bump touches exactly one constant.


def hf_url(repo: str, file: str, revision: str = HF_REVISION) -> str:
    """Hugging Face file URL, e.g. hf_url("BAAI/bge-m3", "sentencepiece.bpe.model")."""
    return f"{hf_endpoint()}/{repo}/resolve/{revision or 'main'}/{file}"


def cl100k_url() -> str:
    """cl100k BPE table. Hosted on Azure blob storage (NOT a GitHub release),
    so the GitHub mirror prefix does not apply and is deliberately omitted."""
    return "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"


def stagehand_url(arch: str) -> str:
    """stagehand-server release binary: stagehand_url("x64" | "arm64")."""
    return f"{gh_proxy()}https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/{STAGEHAND_SERVER_VERSION}/stagehand-server-v3-linux-{arch}"


def pdf_oxide_url() -> str:
    """pdf_oxide go-ffi tarball."""
    return f"{gh_proxy()}https://github.com/yfedoseev/pdf_oxide/releases/download/v{PDF_OXIDE_VERSION}/pdf_oxide-go-ffi-linux-amd64.tar.gz"


def pdfium_url() -> str:
    """pdfium static library tarball."""
    return f"{gh_proxy()}https://github.com/kognitos/pdfium-static/releases/download/chromium%2F{PDFIUM_VERSION}/pdfium-linux-x64-static.tgz"


def office_oxide_url(repo_root: str) -> str:
    """office_oxide native tarball. The version is derived from go.mod so it
    always matches what the Go build actually links (no second hardcoded copy
    to drift)."""
    version = go_module_version("github.com/yfedoseev/office_oxide/go", repo_root)
    return f"{gh_proxy()}https://github.com/yfedoseev/office_oxide/releases/download/v{version}/native-linux-x86_64.tar.gz"


# ── ONNX Runtime release asset helpers ─────────────────────────────────────


def ort_asset_name(version: str = ORT_VERSION) -> str:
    """Release asset filename under infiniflow/ragflow-build tag onnxruntime-v{version}."""
    return f"onnxruntime-v{version}-linux-x86_64.zip"


def ort_url(version: str = ORT_VERSION) -> str:
    return f"{gh_proxy()}https://github.com/infiniflow/ragflow-build/releases/download/onnxruntime-v{version}/{ort_asset_name(version)}"


def ort_extracted_dir(version: str = ORT_VERSION) -> str:
    """Top-level directory name INSIDE the release zip (what extractall creates)."""
    return f"onnxruntime-v{version}-linux-x86_64"


def ort_normalized_dir(version: str = ORT_VERSION) -> str:
    """Directory name build.sh's `find ... -name '*.a'` glob expects under static_lib."""
    return f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28"
