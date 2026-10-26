#!/usr/bin/env python3

# PEP 723 metadata
# /// script
# requires-python = ">=3.10"
# dependencies = [
#   "requests",
#   "huggingface-hub",
# ]
# ///

# Download the native static libraries the Go build links (pdfium / pdf_oxide /
# office_oxide / onnxruntime) and unpack them into ~/ragflow-native-libs, so
# build.sh can compile the Go server without network access.
#
# This is the LOCAL-DEV one-stop: it also fetches the runtime resources the Go
# server needs to actually RUN, parse documents and count tokens (the cl100k
# BPE table, the DeepDoc `.ort` weights for the in-process PDF/table backend,
# and the embedding tokenizer assets). Run it ONCE and the local environment is
# ready — you do NOT also need download_deps.py for local development.
#
# The image-facing counterpart deps/download_deps.py fetches the same
# runtime resources PLUS the stagehand server binary (used by the Browser
# component) and skips the native `.a` files, because the build stage bakes
# those into the server binary. Both scripts land the shared runtime resources
# at the SAME paths, so running one after the other never re-downloads an
# already-present asset.
#
# All version numbers and URLs are defined in the block below; bump a version
# there, not at the call sites.
#
# Typical workflow:
#
#   uv run tools/scripts/download_dev_deps.py
#
# Behind the GFW, route GitHub release downloads through a mirror with
# --china-mirrors (or export GH_PROXY=https://gh-proxy.com/).
#
# Platform support
# ----------------
# The native static libraries are downloaded for the *target* platform by
# default. The target GOOS/GOARCH is detected from the host `uname`; override it
# with RAGFLOW_TARGET_OS / RAGFLOW_TARGET_ARCH (or --target-os / --target-arch)
# when baking a cross-platform environment or when the host's reported arch is
# wrong.
#
#   linux/amd64   -> native-linux-x86_64 / pdfium-linux-x64 / ... / onnxruntime-v{VER}-linux-x86_64
#   linux/arm64   -> native-linux-aarch64 / pdfium-linux-arm64 / ... / onnxruntime-v{VER}-linux-aarch64
#   darwin/amd64  -> native-macos-x86_64 / pdfium-mac-x64 / ... / onnxruntime-v{VER}-osx-universal
#   darwin/arm64  -> native-macos-aarch64 / pdfium-mac-arm64 / ... / onnxruntime-v{VER}-osx-arm64
#
# The in-process (Go) DeepDoc backend statically links ONNX Runtime; the
# symlink-free `dlopen(NULL)` resolution means no `.so`/`.dylib` ships at
# runtime.

import argparse
import hashlib
import os
import platform
import re
import shutil
import sys
import tarfile
import tempfile
import zipfile

import requests

# ── Dependency versions & URLs (edit here) ────────────────────────────────
# This script's own copy of the native static-library versions and download URLs
# it fetches. build.sh does NOT grep these (it pins PDFIUM/PDF_OXIDE itself and
# reads ORT from internal/common/environments.go + the Dockerfile ARG, so it
# stays tied to the checked-out source tree and the Docker build needs no COPY
# of this script) — keep the two in sync when bumping a version.

# ONNX Runtime (statically linked into the Go in-process DeepDoc backend).
#
# Source of the native static archives: infiniflow/ragflow-build (our own
# ORT-only minimal build), NOT the third-party csukuangfj/onnxruntime-libs
# account. The release tag is `ORT_RELEASE_TAG` (see below; currently
# onnxruntime-v1.29.0-patch2) and it publishes one asset per target:
# `onnxruntime-v{ORT_VERSION}-<target>.zip` (see ort_target). The archive is
# occasionally re-issued under this SAME tag/asset name with
# patched content (e.g. the build that exports SessionGetInitializer*); the
# downloader detects that via a `{asset}.sha256` sidecar and
# re-downloads/re-extracts, so a stale local copy never silently lingers.
ORT_VERSION = "1.29.0"

# Release tag on infiniflow/ragflow-build. v1.29.0's Linux x86_64 minimal build
# reduced operators to required_operators.config, which omitted Clip@13 — so
# Clip(opset 13) models (e.g. the DeepDoc rec/tsr weights) failed to load with
# "Could not find an implementation for Clip(13)". onnxruntime-v1.29.0-patch2
# re-adds Clip to the ai.onnx;13 allow list and rebuilds. Its asset name,
# extracted dir and normalized dir all keep the bare version, so ONLY the
# release tag changes here (revert to onnxruntime-v{ORT_VERSION} once a stock
# build ships Clip@13).
ORT_RELEASE_TAG = "onnxruntime-v1.29.0-patch2"

# pdf_oxide go-ffi release. Bare module version (the "v" is added by
# pdf_oxide_url); matches the value build.sh uses to validate the embedded
# .a stamp.
PDF_OXIDE_VERSION = "0.3.73"

# pdfium release, as the numeric build number after "chromium%2F" (the "v" and
# the URL-encoded slash are added by pdfium_url). Matches build.sh's
# PDFIUM_STATIC_VERSION.
PDFIUM_VERSION = "7809"


def host_platform(target_os=None, target_arch=None):
    """Return (goos, goarch) for the build target.

    Defaults to the host machine, overridable via the RAGFLOW_TARGET_OS /
    RAGFLOW_TARGET_ARCH environment variables or the explicit arguments (the
    --target-os / --target-arch CLI flags) so a cross-platform environment can
    be baked for a foreign arch.
    """
    raw_os = target_os or os.environ.get("RAGFLOW_TARGET_OS")
    raw_arch = target_arch or os.environ.get("RAGFLOW_TARGET_ARCH")
    if not raw_os:
        raw_os = platform.system()  # "Linux" / "Darwin" / ...
    if not raw_arch:
        raw_arch = platform.machine().lower()  # x86_64 / arm64 / aarch64 / ...

    # Normalize the same aliases build.sh's detect_target_platform accepts, so
    # `RAGFLOW_TARGET_OS=Linux RAGFLOW_TARGET_ARCH=x86_64` (valid for build.sh)
    # does not raise KeyError when indexing the per-platform asset maps below.
    goos = {"linux": "linux", "darwin": "darwin"}.get(raw_os.lower())
    if goos is None:
        raise SystemExit(f"Unsupported target OS={raw_os!r}; expected linux or darwin (aliases Linux/Darwin also accepted).")
    goarch = {"amd64": "amd64", "x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(raw_arch.lower())
    if goarch is None:
        raise SystemExit(f"Unsupported target arch={raw_arch!r}; expected amd64 or arm64 (aliases x86_64/aarch64 also accepted).")
    return goos, goarch


# Per-(goos, goarch) release asset filenames. The tarballs extract flat into
# `~/ragflow-native-libs/<lib>/` with a platform-independent internal layout
# (office_oxide: lib/liboffice_oxide.a + include/office_oxide_c/; pdfium:
# lib/libpdfium.a; pdf_oxide: lib/<platform_subdir>/libpdf_oxide.a +
# include/), so build.sh resolves the same paths on every platform.
OFFICE_OXIDE_ASSETS = {
    ("linux", "amd64"): "native-linux-x86_64.tar.gz",
    ("linux", "arm64"): "native-linux-aarch64.tar.gz",
    ("darwin", "amd64"): "native-macos-x86_64.tar.gz",
    ("darwin", "arm64"): "native-macos-aarch64.tar.gz",
}
PDFIUM_STATIC_ASSETS = {
    ("linux", "amd64"): "pdfium-linux-x64-static.tgz",
    ("linux", "arm64"): "pdfium-linux-arm64-static.tgz",
    ("darwin", "amd64"): "pdfium-mac-x64-static.tgz",
    ("darwin", "arm64"): "pdfium-mac-arm64-static.tgz",
}
PDF_OXIDE_ASSETS = {
    ("linux", "amd64"): "pdf_oxide-go-ffi-linux-amd64.tar.gz",
    ("linux", "arm64"): "pdf_oxide-go-ffi-linux-arm64.tar.gz",
    ("darwin", "amd64"): "pdf_oxide-go-ffi-darwin-amd64.tar.gz",
    ("darwin", "arm64"): "pdf_oxide-go-ffi-darwin-arm64.tar.gz",
}


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


# ── URL builders ──────────────────────────────────────────────────────────
# Every download URL is assembled from the constants above via one of these
# helpers, so a version bump touches exactly one constant. Each takes the target
# (goos, goarch) and resolves the release asset filename from the per-platform
# maps above.


def pdf_oxide_url(goos: str, goarch: str) -> str:
    """pdf_oxide go-ffi tarball for the target platform."""
    return f"{gh_proxy()}https://github.com/yfedoseev/pdf_oxide/releases/download/v{PDF_OXIDE_VERSION}/{PDF_OXIDE_ASSETS[(goos, goarch)]}"


def pdfium_url(goos: str, goarch: str) -> str:
    """pdfium static library tarball for the target platform."""
    return f"{gh_proxy()}https://github.com/kognitos/pdfium-static/releases/download/chromium%2F{PDFIUM_VERSION}/{PDFIUM_STATIC_ASSETS[(goos, goarch)]}"


def office_oxide_url(repo_root: str, goos: str, goarch: str) -> str:
    """office_oxide native tarball for the target platform. The version is
    derived from go.mod so it always matches what the Go build actually links
    (no second hardcoded copy to drift)."""
    version = go_module_version("github.com/yfedoseev/office_oxide/go", repo_root)
    return f"{gh_proxy()}https://github.com/yfedoseev/office_oxide/releases/download/v{version}/{OFFICE_OXIDE_ASSETS[(goos, goarch)]}"


# ── ONNX Runtime release asset helpers ─────────────────────────────────────


def ort_target(goos: str, goarch: str) -> str:
    """infiniflow/ragflow-build release target for (goos, goarch).

    The release publishes one zip per target: linux-x86_64, linux-aarch64,
    osx-arm64 and osx-universal. There is no osx-x86_64 — Intel Macs use the
    universal archive (x86_64 + arm64 in one .a).
    """
    if goos == "linux":
        return "linux-x86_64" if goarch == "amd64" else "linux-aarch64"
    return "osx-universal" if goarch == "amd64" else "osx-arm64"


def ort_asset_name(version: str, target: str) -> str:
    """Release asset filename under infiniflow/ragflow-build tag onnxruntime-v{version}."""
    return f"onnxruntime-v{version}-{target}.zip"


def ort_url(version: str, target: str) -> str:
    return f"{gh_proxy()}https://github.com/infiniflow/ragflow-build/releases/download/{ORT_RELEASE_TAG}/{ort_asset_name(version, target)}"


def ort_extracted_dir(version: str, target: str) -> str:
    """Top-level directory name INSIDE the release zip (what extractall creates)."""
    return f"onnxruntime-v{version}-{target}"


def ort_normalized_dir(version: str, goos: str, goarch: str) -> str:
    """Directory name build.sh's `find ... -name '*.a'` glob expects under static_lib.

    Not the zip's own top-level dir: build.sh matches the onnxruntime release
    layout (onnxruntime-<os>-<arch>-static_lib-<version>[-glibc2_28]), so
    extract_onnxruntime() renames the extracted dir to this. The glibc suffix
    exists on Linux only.
    """
    if goos == "linux":
        arch_token = "x64" if goarch == "amd64" else "aarch64"
        return f"onnxruntime-linux-{arch_token}-static_lib-{version}-glibc2_28"
    arch_token = "universal" if goarch == "amd64" else "arm64"
    return f"onnxruntime-osx-{arch_token}-static_lib-{version}"


def _repo_root() -> str:
    """Walk up from this file to the repo root (the directory holding go.mod).

    The script lives at tools/scripts/, two levels below the repo root, so a
    fixed parent-count would break if it is ever moved again; searching for
    go.mod keeps REPO_ROOT correct regardless of nesting."""
    d = os.path.dirname(os.path.abspath(__file__))
    while d and d != os.path.dirname(d):
        if os.path.isfile(os.path.join(d, "go.mod")):
            return d
        d = os.path.dirname(d)
    raise RuntimeError("go.mod not found: cannot locate repo root")


REPO_ROOT = _repo_root()
# Downloaded archives are anchored under deps/ (ignored by .gitignore
# via *.tgz / deps/*.zip) regardless of the caller's working directory,
# so a move of this script does not scatter large files into tools/scripts/.
RAGFLOW_DEPS_DIR = os.path.join(REPO_ROOT, "deps")
NATIVE_LIBS_DIR = os.path.expanduser("~/ragflow-native-libs")


def _sha256_of(path):
    """sha256 of a file, streamed in chunks so large archives don't blow memory."""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def download_with_progress(url, filename):
    """Download URL to FILENAME atomically (temp file, then move on success).

    A failed or empty download leaves any previous valid file untouched, so an
    interrupted run never destroys a working dependency tree."""
    filename = os.fspath(filename)
    temporary = None
    try:
        with requests.get(url, stream=True, timeout=(15, 60)) as response:
            response.raise_for_status()
            total_size = int(response.headers.get("content-length", 0))
            with tempfile.NamedTemporaryFile(dir=os.path.dirname(os.path.abspath(filename)), delete=False) as file:
                temporary = file.name
                downloaded = 0
                for data in response.iter_content(1 << 20):
                    file.write(data)
                    downloaded += len(data)
                    if total_size > 0:
                        progress = (downloaded / total_size) * 100
                        sys.stdout.write(f"\rProgress: {progress:.1f}% ({downloaded}/{total_size} bytes)")
                        sys.stdout.flush()
            if downloaded == 0:
                raise requests.RequestException(f"Empty dependency download: {url}")
            os.replace(temporary, filename)
    finally:
        if temporary is not None and os.path.exists(temporary):
            os.unlink(temporary)
    print()


def _valid_download(filename):
    """True when FILENAME is a non-empty, structurally valid archive of its kind."""
    if not os.path.isfile(filename) or os.path.getsize(filename) == 0:
        return False
    if filename.endswith((".tar.gz", ".tgz")):
        if not tarfile.is_tarfile(filename):
            return False
        if os.path.basename(filename) in OFFICE_OXIDE_ASSETS.values():
            version = go_module_version("github.com/yfedoseev/office_oxide/go", REPO_ROOT)
            with tarfile.open(filename) as archive:
                try:
                    member = next((m for m in archive.getmembers() if os.path.normpath(m.name) == "lib/liboffice_oxide.a"), None)
                    if member is None:
                        return False
                    library = archive.extractfile(member)
                    if library is None:
                        return False
                    with library:
                        return b"\x00" + version.encode() + b"\x00" in library.read()
                except (KeyError, tarfile.TarError, EOFError):
                    return False
        return True
    if filename.endswith(".zip"):
        return zipfile.is_zipfile(filename)
    return True


def _extract_native_archive(archive_path, target, required_files, version=None):
    """Extract ARCHIVE_PATH into TARGET, verifying REQUIRED_FILES (and, for .a
    files, an optional null-delimited VERSION stamp) before promoting the
    staged tree.

    Path-traversal / absolute / non-regular members are rejected before
    extraction, and an incomplete archive raises instead of replacing a
    previously good directory."""

    def complete(directory):
        for name in required_files:
            path = os.path.join(directory, name)
            if not os.path.isfile(path) or os.path.getsize(path) == 0:
                return False
            if version is not None and name.endswith(".a"):
                with open(path, "rb") as file:
                    if b"\x00" + version.encode() + b"\x00" not in file.read():
                        return False
        return True

    if complete(target):
        print(f"  ✓ {os.path.basename(target)} already extracted to {target}")
        return
    os.makedirs(os.path.dirname(target), exist_ok=True)
    staging = tempfile.mkdtemp(dir=os.path.dirname(target))
    try:
        with tarfile.open(archive_path) as archive:
            members = archive.getmembers()
            for member in members:
                if os.path.isabs(member.name) or ".." in member.name.split("/") or not (member.isfile() or member.isdir()):
                    raise tarfile.TarError(f"Unsafe native archive member: {member.name}")
            archive.extractall(staging, members=members)
        if not complete(staging):
            raise RuntimeError(f"{archive_path} is missing required native files or version {version}")
        if os.path.isdir(target):
            shutil.rmtree(target)
        os.replace(staging, target)
        staging = None
    finally:
        # No-op after a successful replace (the dir was renamed away); cleans
        # up the partial tree when extraction or validation failed.
        if staging is not None:
            shutil.rmtree(staging, ignore_errors=True)
    print(f"  ✓ extracted {os.path.basename(archive_path)} to {target}")


def prune_stale_onnxruntime(static_lib_dir, expected_dir):
    """Remove ONNX Runtime version dirs under static_lib that do NOT match
    `expected_dir`. Without this, a version bump (or a foreign-arch run on a
    shared cache) leaves a stale dir next to the new one and build.sh's
    `find ... -name '*.a'` links BOTH (duplicate symbols / wrong version,
    silently)."""
    if not os.path.isdir(static_lib_dir):
        return
    for name in os.listdir(static_lib_dir):
        if not name.startswith("onnxruntime-"):
            continue
        if name == expected_dir:
            continue
        stale = os.path.join(static_lib_dir, name)
        print(f"  Removing stale ONNX Runtime dir: {stale}")
        shutil.rmtree(stale)


def has_static_archives(directory):
    """True when `directory` holds at least one static archive (.a)."""
    return any(f.endswith(".a") for _, _, files in os.walk(directory) for f in files)


def extract_onnxruntime(static_lib_dir, archive_path, expected_dir):
    """Ensure the ONNX Runtime static archives for `expected_dir` sit under
    `static_lib_dir`. Returns True when that dir is available afterwards
    (extracted now or already present), False when the archive is missing.

    The zip wraps everything in a version-stamped top-level dir
    (onnxruntime-v{version}-<target>/, see ort_extracted_dir), so a present
    `static_lib_dir` is NOT evidence that THIS version is extracted: after a
    version bump the stale dir is pruned and the new one must be extracted.
    """
    if not os.path.isfile(archive_path):
        print(f"  Skipping extraction: {os.path.basename(archive_path)} not found")
        return False
    prune_stale_onnxruntime(static_lib_dir, expected_dir)
    expected_path = os.path.join(static_lib_dir, expected_dir)
    if os.path.isdir(expected_path) and has_static_archives(expected_path):
        print(f"  ✓ onnxruntime/static_lib ({expected_dir}) already extracted to {expected_path}")
        return True
    os.makedirs(static_lib_dir, exist_ok=True)
    print(f"  Extracting {os.path.basename(archive_path)} → {static_lib_dir}")
    with zipfile.ZipFile(archive_path) as zf:
        zf.extractall(static_lib_dir)
        top_level = {name.split("/", 1)[0] for name in zf.namelist() if name.strip("/")}
    # The zip's top-level dir (onnxruntime-v{version}-<target>) is not the name
    # build.sh's glob and the stale-dir check above expect, so rename it to
    # `expected_dir` — every consumer then shares one name convention.
    normalized = os.path.join(static_lib_dir, expected_dir)
    if len(top_level) == 1:
        extracted = os.path.join(static_lib_dir, top_level.pop())
        if os.path.isdir(extracted) and extracted != normalized:
            if os.path.exists(normalized):
                shutil.rmtree(normalized)
            print(f"  Renaming {os.path.basename(extracted)} → {os.path.basename(normalized)}")
            os.rename(extracted, normalized)
    return True


def _fetch_ort_sidecar_sha(url):
    """Fetch the published `<asset>.sha256` sidecar for the ORT archive.

    The infiniflow/ragflow-build zip is occasionally re-issued under the SAME
    tag/asset name with patched content, so a stale local copy would link the
    wrong build. Returns the expected digest, or None when the sidecar cannot
    be fetched (best-effort; the downloader then falls back to the legacy
    existence check)."""
    try:
        resp = requests.get(url, timeout=30)
        resp.raise_for_status()
        return resp.text.split()[0]
    except Exception as exc:  # noqa: BLE001 - best-effort; fall back to legacy
        print(f"  WARNING: could not fetch {url} ({exc}); skipping checksum")
        return None


def _download_ort_archive(archive_path, goos, goarch):
    """Download the ORT release zip, verifying it against the published
    .sha256 sidecar so a re-issued archive is always refreshed.

    A (re-)download drops any previously extracted version dir, so the
    same-named re-issued archive actually refreshes the .a files instead of
    being masked by the extraction-time idempotence check."""
    target = ort_target(goos, goarch)
    url = ort_url(ORT_VERSION, target)
    expected_sha = _fetch_ort_sidecar_sha(url + ".sha256")
    if os.path.isfile(archive_path) and os.path.getsize(archive_path) > 0:
        if expected_sha is not None:
            actual = _sha256_of(archive_path)
            if actual == expected_sha:
                print(f"  ✓ {os.path.basename(archive_path)} checksum matches released {expected_sha}; skipping download")
                return
            print(f"  {os.path.basename(archive_path)} checksum mismatch (local {actual} != released {expected_sha}); re-downloading")
        elif _valid_download(archive_path):
            return
    download_with_progress(url, archive_path)
    if expected_sha is not None:
        actual = _sha256_of(archive_path)
        if actual != expected_sha:
            os.unlink(archive_path)
            print(f"  ERROR: {os.path.basename(archive_path)} checksum mismatch after download (got {actual}, expected {expected_sha})", file=sys.stderr)
            sys.exit(1)
        print(f"  ✓ {os.path.basename(archive_path)} checksum verified ({actual})")
    if not _valid_download(archive_path):
        os.unlink(archive_path)
        raise RuntimeError(f"Invalid downloaded dependency: {os.path.basename(archive_path)}")
    version_dir = os.path.join(NATIVE_LIBS_DIR, "onnxruntime", "static_lib", ort_normalized_dir(ORT_VERSION, goos, goarch))
    if os.path.isdir(version_dir):
        print(f"  Removing stale extracted ONNX Runtime dir: {version_dir}")
        shutil.rmtree(version_dir)


# ── Runtime resources the local Go dev server also needs ───────────────────
# The build-facing native `.a` libs above let the server LINK; these let it
# RUN, parse documents and count tokens. Fetched here too so a developer runs
# THIS script once and the environment is ready. Landing paths match
# deps/download_deps.py, so re-running that image script never
# re-downloads an already-present asset. stagehand is deliberately NOT fetched
# here (Browser component only) — that is download_deps.py's job for images.


# cl100k BPE table. Hosted on Azure blob storage (NOT a GitHub release), so the
# GitHub mirror prefix does not apply and is deliberately omitted.
def cl100k_url() -> str:
    return "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"


HF_DEEPDOC_REPO = "InfiniFlow/deepdoc"
HF_DEEPDOC_MODEL_FILES = ["det.ort", "layout.ort", "tsr.ort", "rec.ort", "ocr.res"]


# Embedding tokenizer assets the Go counters load. Inlined here (no separate
# manifest file) as the single source of truth this script reads directly; the
# image script deps/download_deps.py carries its own copy (no shared
# module, by design). One entry per tuple: (repo, file, kind).
#   runtime - the Go counters load it in production; the runtime images must ship
#             it and the loader must pin its SHA-1.
#   oracle  - only scripts/gen_tokenizer_oracle.py needs it to regenerate the
#             test fixtures, so it is deliberately NOT shipped to the image.
TOKENIZER_ASSETS = [
    ("BAAI/bge-m3", "sentencepiece.bpe.model", "runtime"),
    ("BAAI/bge-large-en-v1.5", "vocab.txt", "runtime"),
    ("Qwen/Qwen3-Embedding-0.6B", "tokenizer.json", "runtime"),
    ("intfloat/e5-mistral-7b-instruct", "tokenizer.json", "runtime"),
    ("BAAI/bge-m3", "tokenizer.json", "oracle"),
    ("BAAI/bge-large-en-v1.5", "tokenizer.json", "oracle"),
]


# Single huggingface_hub import seam, so the module stays importable (and its
# unit tests stay huggingface-free) without the dep installed.
def _hf_download(repo_id, filename, local_dir):
    """Lazy huggingface_hub import + download into LOCAL_DIR."""
    from huggingface_hub import hf_hub_download

    return hf_hub_download(repo_id=repo_id, filename=filename, local_dir=local_dir)


def download_cl100k():
    """Fetch the cl100k BPE table into deps/ (skip if already present)."""
    target = os.path.join(RAGFLOW_DEPS_DIR, "cl100k_base.tiktoken")
    if os.path.isfile(target) and os.path.getsize(target) > 0:
        print("  ✓ cl100k_base.tiktoken already present")
        return
    print(f"Downloading cl100k_base.tiktoken from {cl100k_url()}...")
    download_with_progress(cl100k_url(), target)


def load_tokenizer_assets(assets=None):
    """Validate and return the inlined tokenizer asset list as (repo, file, kind)
    tuples. When ASSETS is None, uses the module-level TOKENIZER_ASSETS.

    Fetched per file, not by snapshot: these repos also carry multi-GB weights
    we do not want. A malformed entry (bad kind, malformed repo, or an empty
    list) fails dependency preparation loudly instead of handing a broken tree
    to the image build."""
    items = TOKENIZER_ASSETS if assets is None else assets
    result = []
    for repo, filename, kind in items:
        if kind not in ("runtime", "oracle"):
            raise RuntimeError(f"unknown tokenizer asset kind {kind!r} for {repo}/{filename}")
        if "/" not in repo or not filename:
            raise RuntimeError(f"malformed tokenizer asset repo/file {repo}/{filename}")
        result.append((repo, filename, kind))
    if not result:
        raise RuntimeError("no tokenizer assets listed")
    return result


def download_go_models(use_china_mirrors=False):
    """Download the Go DeepDoc `.ort` weights so a Go dev can run the in-process
    backend with no further setup.

    Written into the repo's canonical model directory
    internal/rag/res/deepdoc/ (one level up from deps/). The Go server
    auto-discovers that directory, and build.sh --test-native defaults MODEL_DIR
    there too. A copy also lands under deps/huggingface.co/InfiniFlow/deepdoc/
    to keep the dependency image's existing build-context layout."""
    if use_china_mirrors:
        os.environ["HF_ENDPOINT"] = "https://hf-mirror.com"
    target_dir = os.path.join(REPO_ROOT, "internal", "rag", "res", "deepdoc")
    missing = []
    for fname in HF_DEEPDOC_MODEL_FILES:
        dest = os.path.join(target_dir, fname)
        if os.path.isfile(dest) and os.path.getsize(dest) > 0:
            print(f"  ✓ {fname} already present")
            continue
        print(f"Downloading deepdoc model {fname}...")
        try:
            _hf_download(repo_id=HF_DEEPDOC_REPO, filename=fname, local_dir=target_dir)
            if not os.path.isfile(dest) or os.path.getsize(dest) == 0:
                raise RuntimeError(f"Empty or missing model: {dest}")
        except Exception as e:  # noqa: BLE001 - collected and surfaced below
            missing.append((fname, e))
    if missing:
        for fname, e in missing:
            print(f"  ERROR: failed to download {fname}: {e}", file=sys.stderr)
        print(
            "\nThe Go in-process DeepDoc backend loads ONLY the .ort weights listed in\n"
            "HF_DEEPDOC_MODEL_FILES. They are fetched from "
            f"{HF_DEEPDOC_REPO} into:\n  {target_dir}\n"
            "Without them the backend cannot serve — the server exits with a fatal\n"
            '"no in-process DeepDoc backend serving". To recover:\n'
            "  - re-run this script (a transient HF/network error usually clears);\n"
            "  - behind the GFW, re-run with --china-mirrors (routes via hf-mirror.com);\n"
            "  - or copy the missing files into that directory by hand.",
            file=sys.stderr,
        )
        sys.exit(1)
    # Keep the dependency image's existing huggingface.co build-context layout.
    image_dir = os.path.join(RAGFLOW_DEPS_DIR, "huggingface.co", HF_DEEPDOC_REPO)
    os.makedirs(image_dir, exist_ok=True)
    for fname in HF_DEEPDOC_MODEL_FILES:
        shutil.copyfile(os.path.join(target_dir, fname), os.path.join(image_dir, fname))
    print(f"  ✓ Go DeepDoc models ready under {target_dir}")


def download_tokenizer_assets(use_china_mirrors=False):
    """Download the embedding tokenizer assets the Go counters load, into
    deps/huggingface.co/<repo>/<file> (the path internal/tokenizer
    searches and the one deps/Dockerfile builds its image from).

    A missing asset is fatal: models declaring its tokenizer refuse ingestion."""
    if use_china_mirrors:
        os.environ["HF_ENDPOINT"] = "https://hf-mirror.com"
    base = os.path.join(RAGFLOW_DEPS_DIR, "huggingface.co")
    missing = []
    try:
        entries = load_tokenizer_assets()
    except Exception as e:  # noqa: BLE001 - manifest missing/malformed is fatal
        missing.append(("<manifest>", "<manifest>", e))
    else:
        for repo_id, filename, _kind in entries:
            target_dir = os.path.join(base, repo_id)
            target_file = os.path.join(target_dir, filename)
            if os.path.isfile(target_file) and os.path.getsize(target_file) > 0:
                print(f"  ✓ {repo_id}/{filename} already present")
                continue
            os.makedirs(target_dir, exist_ok=True)
            print(f"Downloading tokenizer asset {repo_id}/{filename}...")
            try:
                _hf_download(repo_id=repo_id, filename=filename, local_dir=target_dir)
                if not os.path.isfile(target_file) or os.path.getsize(target_file) == 0:
                    raise RuntimeError(f"Empty or missing tokenizer asset: {target_file}")
            except Exception as e:  # noqa: BLE001 - collected and surfaced below
                missing.append((repo_id, filename, e))
    if missing:
        for repo_id, filename, e in missing:
            print(f"  ERROR: could not fetch {repo_id}/{filename}: {e}", file=sys.stderr)
        print(
            "\nThe embedding tokenizer counters in internal/tokenizer load these files from\n"
            f"  {base}\n"
            "Without one, models that declare its tokenizer cannot be ingested.\n"
            "The runtime images refuse to build on the same condition, so this script fails\n"
            "instead of handing a broken tree to the image build. To recover:\n"
            "  - re-run this script (a transient HF/network error usually clears);\n"
            "  - behind the GFW, re-run with --china-mirrors (routes via hf-mirror.com);\n"
            "  - or point MODEL_ASSETS_DIR at a directory holding the same layout.\n",
            file=sys.stderr,
        )
        sys.exit(1)
    print("  ✓ embedding tokenizer assets ready")


def main():
    """Download the native static libraries the Go build links, then the runtime
    resources (cl100k BPE table, DeepDoc .ort weights, embedding tokenizer assets)
    the local Go dev server needs to run, parse and count tokens.

    This is the local-dev one-stop: one invocation readies the environment. The
    Browser component's server binary is intentionally NOT fetched here — that
    is the image script's job. Run `uv run deps/download_deps.py` for the
    full image resources (it adds that binary and skips the native .a, which the
    build stage bakes into the binary)."""
    parser = argparse.ArgumentParser(description="Download the native static libraries and runtime resources the local Go dev server needs")
    parser.add_argument("--china-mirrors", action="store_true", help="Use China-accessible mirrors for downloads")
    parser.add_argument(
        "--target-os",
        default=os.environ.get("RAGFLOW_TARGET_OS"),
        help="Override the target GOOS (linux/darwin). Defaults to host.",
    )
    parser.add_argument(
        "--target-arch",
        default=os.environ.get("RAGFLOW_TARGET_ARCH"),
        help="Override the target GOARCH (amd64/arm64). Defaults to host.",
    )
    args = parser.parse_args()
    if args.china_mirrors:
        os.environ["GH_PROXY"] = "https://gh-proxy.com/"

    goos, goarch = host_platform(args.target_os, args.target_arch)
    print(f"Target platform: {goos}/{goarch}")
    ort_zip = ort_asset_name(ORT_VERSION, ort_target(goos, goarch))
    ort_dir = ort_normalized_dir(ORT_VERSION, goos, goarch)

    # Anchor the downloaded archives to deps/ regardless of the
    # caller's working directory (build.sh invokes this from the repo root).
    os.chdir(RAGFLOW_DEPS_DIR)

    # ONNX Runtime archive (+ checksum sidecar), then the other native archives.
    _download_ort_archive(os.path.join(RAGFLOW_DEPS_DIR, ort_zip), goos, goarch)
    archives = [
        (pdfium_url(goos, goarch), PDFIUM_STATIC_ASSETS[(goos, goarch)]),
        (pdf_oxide_url(goos, goarch), PDF_OXIDE_ASSETS[(goos, goarch)]),
        (office_oxide_url(REPO_ROOT, goos, goarch), OFFICE_OXIDE_ASSETS[(goos, goarch)]),
    ]
    for url, filename in archives:
        print(f"Downloading {filename} from {url}...")
        if _valid_download(filename):
            print(f"  ✓ {filename} already present")
            continue
        download_with_progress(url, filename)
        if not _valid_download(filename):
            os.unlink(filename)
            raise RuntimeError(f"Invalid downloaded dependency: {filename}")

    # Extract native static libraries to ~/ragflow-native-libs for the Go build.
    # Ensures build.sh can find them without network access.
    office_version = go_module_version("github.com/yfedoseev/office_oxide/go", REPO_ROOT)
    extractions = [
        (PDFIUM_STATIC_ASSETS[(goos, goarch)], "pdfium-static", ["lib/libpdfium.a", "lib/libc++.a", "lib/libc++abi.a", "include/fpdfview.h"], None),
        (PDF_OXIDE_ASSETS[(goos, goarch)], "pdf_oxide", [f"lib/{goos}_{goarch}/libpdf_oxide.a", "include/pdf_oxide.h"], None),
        (OFFICE_OXIDE_ASSETS[(goos, goarch)], "office_oxide", ["lib/liboffice_oxide.a", "include/office_oxide_c/office_oxide.h"], office_version),
    ]

    for archive, subdir, required_files, version in extractions:
        archive_path = os.path.join(RAGFLOW_DEPS_DIR, archive)
        if not os.path.isfile(archive_path):
            print(f"  Skipping extraction: {archive} not found")
            continue
        target = os.path.join(NATIVE_LIBS_DIR, subdir)
        try:
            _extract_native_archive(archive_path, target, required_files, version)
        except (tarfile.TarError, EOFError, RuntimeError):
            os.unlink(archive_path)
            raise

    if not extract_onnxruntime(
        os.path.join(NATIVE_LIBS_DIR, "onnxruntime", "static_lib"),
        os.path.join(RAGFLOW_DEPS_DIR, ort_zip),
        ort_dir,
    ):
        # The archive was not downloaded or failed to extract, so no .a landed.
        # Fail loud instead of exiting 0: build.sh's ORT guard would otherwise
        # reject the build later with a less actionable message, and a missing
        # .a left here is exactly the "silent green" this split must prevent.
        print(
            f"  ERROR: ONNX Runtime static archives for {ort_dir} were not "
            f"extracted to {os.path.join(NATIVE_LIBS_DIR, 'onnxruntime', 'static_lib')}. "
            f"Check the download above; build.sh will refuse to link without them.",
            file=sys.stderr,
        )
        sys.exit(1)

    # ONNX Runtime is statically linked into the server binary, so there is no
    # runtime .so to surface. Log where build.sh (ONNXRUNTIME_STATIC_PREFIX) will
    # find the archives — the .a files live under
    # ~/ragflow-native-libs/onnxruntime/static_lib. The in-process backend
    # resolves OrtGetApiBase via dlopen(NULL); there is no dynamic .so fallback.
    ort_static_dir = os.path.join(NATIVE_LIBS_DIR, "onnxruntime", "static_lib")
    ort_a_files = [os.path.join(root, f) for root, _, files in os.walk(ort_static_dir) for f in files if f.endswith(".a")]
    if ort_a_files:
        print(f"  ✓ onnxruntime static archives ready: {len(ort_a_files)} .a under {ort_static_dir}")
    else:
        print(f"  ERROR: ONNX Runtime .a files still missing under {ort_static_dir} after extraction; build.sh will refuse to link.", file=sys.stderr)
        sys.exit(1)

    # The Go server also needs the runtime resources (cl100k BPE table, the
    # DeepDoc .ort weights for in-process PDF/table parsing, the embedding
    # tokenizer assets for token counting) to actually RUN and ingest — not
    # just link. Fetch them here too so a local developer runs THIS script
    # once and the environment is ready. The Browser component's server binary
    # is fetched by the image script, not here.
    print("== cl100k_base.tiktoken ==")
    download_cl100k()
    print("== DeepDoc models ==")
    download_go_models(args.china_mirrors)
    print("== tokenizer assets ==")
    download_tokenizer_assets(args.china_mirrors)

    print()
    print(f"✓ Native build libraries ready under {NATIVE_LIBS_DIR}/")
    print("  Run: bash build.sh (it will find them without network access).")
    print("  Runtime resources (cl100k / DeepDoc .ort / tokenizer) are also in place,")
    print("  so the local Go server is ready to build, run, parse and count tokens.")


if __name__ == "__main__":
    main()
