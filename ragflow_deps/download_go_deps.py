#!/usr/bin/env python3

# PEP 723 metadata
# /// script
# requires-python = ">=3.10"
# dependencies = [
#   "nltk",
#   "huggingface-hub",
#   "requests",
# ]
# ///

# This script downloads every artifact the Go build needs: the native static
# libraries (pdfium / pdf_oxide / office_oxide / onnxruntime) for `build.sh`,
# and the Go DeepDoc `.ort` weights (det/layout/tsr/rec.ort + ocr.res) so a Go
# dev can run the in-process backend locally without separately running
# `download_deps.py`. Run it from anywhere — the `__main__` block chdir's into
# this file's own directory, so all outputs land under `ragflow_deps/`
# regardless of the caller's CWD.
#
# Build-context relationship: `ragflow_deps/Dockerfile` is built with
# `ragflow_deps/` as its build context, so the files written here MUST
# sit at the top of `ragflow_deps/`. The Dockerfile's COPY lines assume
# top-level paths (`huggingface.co`, `nltk_data`, `cl100k_base.tiktoken`,
# `*.deb`, `*.jar`, `*.tar.gz`, `stagehand-server-v3-linux-<arch>`).
#
# Typical workflow:
#
#   uv run python3 ragflow_deps/download_go_deps.py            # download
#   cd ragflow_deps
#   docker build -f Dockerfile -t infiniflow/ragflow_deps .
#
# The main `Dockerfile` (built from the project root) pulls this image
# via `--mount=type=bind,from=infiniflow/ragflow_deps:latest,...` and
# is unaffected by where these files live locally.
#
# Go DeepDoc weights: in addition to the native libs, this script downloads the
# five Go model files (internal/common.DeepDocModelFiles) from InfiniFlow/deepdoc
# straight into the repo's canonical model directory `rag/res/deepdoc/` (one level
# up from this script). The Go server auto-discovers that directory via
# resolveDeepDocModelDir(), and build.sh --test-native / internal/deepdoc/native/run.sh
# default MODEL_DIR there too — so after running this script NO MODEL_DIR env needs
# to be set:
#
#   bash build.sh --test-native          # or: cd internal/deepdoc/native && bash run.sh
#
# Platform support
# -----------------
# The native static libraries (office_oxide, pdfium, pdf_oxide, onnxruntime)
# are downloaded for the *target* platform by default. The target GOOS/GOARCH
# is detected from the host `uname`; override it with `RAGFLOW_TARGET_OS` /
# `RAGFLOW_TARGET_ARCH` (or --target-os / --target-arch) when baking a
# cross-platform `ragflow_deps` image or when the host's reported arch is wrong.
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
import os
import platform as _platform
import shutil
import sys
import zipfile

import requests

# Mirrors internal/common.DeepDocORTVersion (Go in-process backend). ONE OF
# FOUR places (with that Go constant, ORT_VERSION in ragflow_deps/download_deps.py,
# and ARG ORT_VERSION in Dockerfile_go) that must carry the same ONNX Runtime
# native release for the statically-linked Go DeepDoc backend. There is no
# single source of truth — keep all four equal. build.sh --check-ort-version
# greps this file (and the other three) to fail fast on drift. (The Python pip
# onnxruntime== pin in pyproject.toml is versioned independently and is not
# part of this check.)
#
# Source of the native static archives: infiniflow/ragflow-build (our own
# ORT-only minimal build), NOT the third-party csukuangfj/onnxruntime-libs
# account. The release tag is `onnxruntime-v{ORT_VERSION}` and it publishes one
# asset per target: `onnxruntime-v{ORT_VERSION}-<target>.zip` (see _ort_target).
ORT_VERSION = "1.29.0"


def _ort_target(goos, goarch):
    """infiniflow/ragflow-build release target for (goos, goarch).

    The release publishes one zip per target: linux-x86_64, linux-aarch64,
    osx-arm64 and osx-universal. There is no osx-x86_64 — Intel Macs use the
    universal archive (x86_64 + arm64 in one .a).
    """
    if goos == "linux":
        return "linux-x86_64" if goarch == "amd64" else "linux-aarch64"
    return "osx-universal" if goarch == "amd64" else "osx-arm64"


def _ort_asset_name(version, target):
    """Release asset filename under infiniflow/ragflow-build tag onnxruntime-v{version}."""
    return f"onnxruntime-v{version}-{target}.zip"


def _ort_extracted_dir(version, target):
    """Top-level directory name INSIDE the release zip (what extractall creates)."""
    return f"onnxruntime-v{version}-{target}"


def _ort_normalized_dir(version, goos, goarch):
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


# Mirrors internal/common.DeepDocModelFiles (Go in-process DeepDoc backend).
# These are the ONLY weights the Go backend loads; the full InfiniFlow/deepdoc
# repo also ships .onnx (Python-only), which this Go-only script deliberately
# skips to keep the download lean.
DEEPDOC_REPO = "InfiniFlow/deepdoc"
DEEPDOC_MODEL_FILES = ["det.ort", "layout.ort", "tsr.ort", "rec.ort", "ocr.res"]

# Native static-library versions (must match build.sh's *_{VERSION} constants).
OFFICE_OXIDE_VERSION = "0.1.9"
PDFIUM_STATIC_VERSION = "7809"
PDF_OXIDE_VERSION = "0.3.73"


def host_platform():
    """Return (goos, goarch) for the build target.

    Defaults to the host machine, overridable via RAGFLOW_TARGET_OS /
    RAGFLOW_TARGET_ARCH so a CI image can be baked for a foreign arch.
    """
    raw_os = os.environ.get("RAGFLOW_TARGET_OS")
    raw_arch = os.environ.get("RAGFLOW_TARGET_ARCH")
    if not raw_os:
        raw_os = _platform.system()  # "Linux" / "Darwin" / ...
    if not raw_arch:
        raw_arch = _platform.machine().lower()  # x86_64 / arm64 / aarch64 / ...

    # Normalize the same aliases build.sh's detect_target_platform accepts, so
    # `RAGFLOW_TARGET_OS=Linux RAGFLOW_TARGET_ARCH=x86_64` (valid for build.sh)
    # does not raise KeyError when indexing the per-platform asset maps below.
    goos = {"linux": "linux", "darwin": "darwin"}.get(raw_os.lower())
    if goos is None:
        raise SystemExit(
            f"Unsupported RAGFLOW_TARGET_OS={raw_os!r}; expected linux or darwin "
            f"(aliases Linux/Darwin also accepted)."
        )
    goarch = {"amd64": "amd64", "x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(
        raw_arch.lower()
    )
    if goarch is None:
        raise SystemExit(
            f"Unsupported RAGFLOW_TARGET_ARCH={raw_arch!r}; expected amd64 or arm64 "
            f"(aliases x86_64/aarch64 also accepted)."
        )
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


def _ort_asset(goos, goarch):
    """Return (zip_filename, normalized_dir) for ONNX Runtime on this platform.

    The zip comes from infiniflow/ragflow-build tag onnxruntime-v{ORT_VERSION}
    (see _ort_target for the four published targets). `normalized_dir` is the
    dir build.sh's stale-version guard matches, so it is what the extracted
    top-level dir is renamed to.
    """
    target = _ort_target(goos, goarch)
    return _ort_asset_name(ORT_VERSION, target), _ort_normalized_dir(ORT_VERSION, goos, goarch)


def _release_url(repo, tag, asset, mirror):
    url = f"https://github.com/{repo}/releases/download/{tag}/{asset}"
    return f"https://gh-proxy.com/{url}" if mirror else url


def get_urls(use_china_mirrors=False, goos=None, goarch=None) -> list[str | list[str]]:
    if goos is None or goarch is None:
        goos, goarch = host_platform()
    mirror = use_china_mirrors
    urls: list[str | list[str]] = []

    # stagehand-server-v3 Node.js SEA binaries (used by Browser component in
    # local mode). Linux-only; on a macOS build host they are irrelevant, so
    # skip them there to avoid downloading useless artifacts.
    if goos == "linux":
        urls += [
            "https://gh-proxy.com/https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/v3.7.2/stagehand-server-v3-linux-x64"
            if mirror
            else "https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/v3.7.2/stagehand-server-v3-linux-x64",
            "https://gh-proxy.com/https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/v3.7.2/stagehand-server-v3-linux-arm64"
            if mirror
            else "https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/v3.7.2/stagehand-server-v3-linux-arm64",
        ]

    # Native static libraries for Go build (pdfium, pdf_oxide, office_oxide,
    # onnxruntime). Used by build.sh's check_*_deps functions — pre-downloaded
    # to avoid network access during CI.
    oa = OFFICE_OXIDE_ASSETS[(goos, goarch)]
    urls.append([_release_url("yfedoseev/office_oxide", f"v{OFFICE_OXIDE_VERSION}", oa, mirror), oa])

    pf = PDFIUM_STATIC_ASSETS[(goos, goarch)]
    urls.append([_release_url("kognitos/pdfium-static", f"chromium%2F{PDFIUM_STATIC_VERSION}", pf, mirror), pf])

    po = PDF_OXIDE_ASSETS[(goos, goarch)]
    urls.append([_release_url("yfedoseev/pdf_oxide", f"v{PDF_OXIDE_VERSION}", po, mirror), po])

    ort_zip, _ = _ort_asset(goos, goarch)
    urls.append([_release_url("infiniflow/ragflow-build", f"onnxruntime-v{ORT_VERSION}", ort_zip, mirror), ort_zip])

    return urls


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
    (onnxruntime-v{version}-<target>/, see _ort_extracted_dir), so a present
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


def download_with_progress(url, filename):
    response = requests.get(url, stream=True)
    total_size = int(response.headers.get("content-length", 0))
    block_size = 1024

    with open(filename, "wb") as file:
        downloaded = 0
        for data in response.iter_content(block_size):
            file.write(data)
            downloaded += len(data)

            if total_size > 0:
                progress = (downloaded / total_size) * 100
                sys.stdout.write(f"\rProgress: {progress:.1f}% ({downloaded}/{total_size} bytes)")
                sys.stdout.flush()

    print()


def download_go_models(use_china_mirrors=False):
    """Download the Go DeepDoc `.ort` weights so a Go dev can run the in-process
    backend with no further setup.

    The files are written into the repo's canonical model directory
    `rag/res/deepdoc/` (relative to the repo root, one level up from this
    script). The Go server auto-discovers that directory via
    resolveDeepDocModelDir(), and build.sh --test-native / run.sh default
    MODEL_DIR there too — so after this script runs, no MODEL_DIR env needs to
    be set.

    Uses hf_hub_download per-file (not snapshot_download) to fetch only the
    five Go model files; the `.onnx` siblings are Python-only and skipped.
    On China mirrors, route through hf-mirror.com via HF_ENDPOINT.
    """
    if use_china_mirrors:
        os.environ["HF_ENDPOINT"] = "https://hf-mirror.com"
    # Imported lazily so the module stays importable (and its unit tests stay
    # huggingface-free) without the huggingface_hub dependency installed.
    from huggingface_hub import hf_hub_download

    # Canonical local-dev model dir the Go backend auto-discovers.
    repo_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    target_dir = os.path.join(repo_root, "rag", "res", "deepdoc")
    os.makedirs(target_dir, exist_ok=True)

    missing = []
    for fname in DEEPDOC_MODEL_FILES:
        dest = os.path.join(target_dir, fname)
        if os.path.isfile(dest):
            print(f"  ✓ {fname} already present")
            continue
        print(f"Downloading deepdoc model {fname}...")
        try:
            hf_hub_download(repo_id=DEEPDOC_REPO, filename=fname, local_dir=target_dir)
        except Exception as e:  # noqa: BLE001 - collected and surfaced below
            missing.append((fname, e))

    if missing:
        for fname, e in missing:
            print(f"  ERROR: failed to download {fname}: {e}", file=sys.stderr)
        print(
            "\n"
            "The Go in-process DeepDoc backend loads ONLY the .ort weights listed in\n"
            "internal/common.DeepDocModelFiles. They are fetched from "
            f"{DEEPDOC_REPO} into:\n"
            f"  {target_dir}\n"
            "Without them the backend cannot serve — the server exits with a fatal\n"
            '"no in-process DeepDoc backend serving". To recover:\n'
            "  - re-run this script (a transient HF/network error usually clears);\n"
            "  - behind the GFW, re-run with --china-mirrors (routes via hf-mirror.com);\n"
            "  - or run `uv run python3 ragflow_deps/download_deps.py`, which snapshots\n"
            f"    all of {DEEPDOC_REPO} (it also provides the Python-side .onnx);\n"
            "  - or copy the missing files into that directory by hand.",
            file=sys.stderr,
        )
        sys.exit(1)

    print(f"  ✓ Go DeepDoc models ready under {target_dir}")
    print("    No MODEL_DIR env needed: the Go backend auto-discovers this directory.")


if __name__ == "__main__":
    # Anchor CWD to this file's directory so all relative outputs
    # (huggingface.co/, nltk_data/, *.deb, *.jar, *.tar.gz, etc.) land
    # at the top of ragflow_deps/ regardless of where the user invokes
    # the script from. This is the build context for `ragflow_deps/Dockerfile`.
    os.chdir(os.path.dirname(os.path.abspath(__file__)))

    parser = argparse.ArgumentParser(description="Download dependencies with optional China mirror support")
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

    goos, goarch = host_platform()
    if args.target_os:
        goos = args.target_os
    if args.target_arch:
        goarch = args.target_arch
    print(f"Target platform: {goos}/{goarch}")

    urls = get_urls(args.china_mirrors, goos, goarch)

    # Some mirrors (e.g. archive.ubuntu.com) reject the default urllib
    # User-Agent with HTTP 403, so install an opener with a browser-like UA.
    #     opener = urllib.request.build_opener()
    #     opener.addheaders = [("User-Agent", "Mozilla/5.0")]
    #     urllib.request.install_opener(opener)

    for url in urls:
        download_url = url[0] if isinstance(url, list) else url
        filename = url[1] if isinstance(url, list) else url.split("/")[-1]
        print(f"Downloading {filename} from {download_url}...")
        if not os.path.exists(filename):
            download_with_progress(download_url, filename)

    # Extract native static libraries to ~/ragflow-native-libs for Go build.
    # Ensures build.sh can find them without network access.
    native_deps_dir = os.path.expanduser("~/ragflow-native-libs")
    import tarfile

    extractions = [
        (OFFICE_OXIDE_ASSETS[(goos, goarch)], "office_oxide"),
        (PDFIUM_STATIC_ASSETS[(goos, goarch)], "pdfium-static"),
        (PDF_OXIDE_ASSETS[(goos, goarch)], "pdf_oxide"),
    ]

    for archive, subdir in extractions:
        archive_path = os.path.join(os.getcwd(), archive)
        if not os.path.isfile(archive_path):
            print(f"  Skipping extraction: {archive} not found")
            continue
        target = os.path.join(native_deps_dir, subdir)
        if os.path.isdir(target):
            print(f"  ✓ {subdir} already extracted to {target}")
            continue
        os.makedirs(target, exist_ok=True)
        print(f"  Extracting {archive} → {target}")
        with tarfile.open(archive_path) as tf:
            tf.extractall(target)

    ort_zip, ort_dir = _ort_asset(goos, goarch)
    if not extract_onnxruntime(
        os.path.join(native_deps_dir, "onnxruntime", "static_lib"),
        os.path.join(os.getcwd(), ort_zip),
        ort_dir,
    ):
        # The archive was not downloaded or failed to extract, so no .a landed.
        # Fail loud instead of exiting 0: build.sh's ORT guard would otherwise
        # reject the build later with a less actionable message, and a missing
        # .a left here is exactly the "silent green" this PR is meant to prevent.
        print(
            f"  ERROR: ONNX Runtime static archives for {ort_dir} were not "
            f"extracted to {os.path.join(native_deps_dir, 'onnxruntime', 'static_lib')}. "
            f"Check the download above; build.sh will refuse to link without them.",
            file=sys.stderr,
        )
        sys.exit(1)

    # ONNX Runtime is statically linked into the server binary, so there is no
    # runtime .so to surface. Log where build.sh (ONNXRUNTIME_STATIC_PREFIX) will
    # find the archives — the .a files live under
    # ~/ragflow-native-libs/onnxruntime/static_lib. The in-process backend
    # resolves OrtGetApiBase via dlopen(NULL); there is no dynamic .so fallback.
    ort_static_dir = os.path.join(native_deps_dir, "onnxruntime", "static_lib")
    ort_a_files = [os.path.join(root, f) for root, _, files in os.walk(ort_static_dir) for f in files if f.endswith(".a")]
    if ort_a_files:
        print(f"  ✓ onnxruntime static archives ready: {len(ort_a_files)} .a under {ort_static_dir}")
    else:
        print(f"  ERROR: ONNX Runtime .a files still missing under {ort_static_dir} after extraction; build.sh will refuse to link.", file=sys.stderr)
        sys.exit(1)

    # Download the Go DeepDoc `.ort` weights so this script is a one-stop for Go
    # dev: native libs (above) + model files (below) from a single invocation.
    download_go_models(args.china_mirrors)
