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

# This script downloads every artifact that the `infiniflow/ragflow_deps`
# Docker image bakes in. Run it from anywhere — the `__main__` block
# chdir's into this file's own directory, so all outputs land under
# `ragflow_deps/` regardless of the caller's CWD.
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
# Platform support
# -----------------
# The native static libraries (office_oxide, pdfium, pdf_oxide, onnxruntime)
# are downloaded for the *target* platform by default. The target GOOS/GOARCH
# is detected from the host `uname`; override it with `RAGFLOW_TARGET_OS` /
# `RAGFLOW_TARGET_ARCH` (or --target-os / --target-arch) when baking a
# cross-platform `ragflow_deps` image or when the host's reported arch is wrong.
#
#   linux/amd64   -> native-linux-x86_64 / pdfium-linux-x64 / ... / onnxruntime-linux-x64
#   linux/arm64   -> native-linux-aarch64 / pdfium-linux-arm64 / ... / onnxruntime-linux-aarch64
#   darwin/amd64  -> native-macos-x86_64 / pdfium-mac-x64 / ... / onnxruntime-osx-x86_64
#   darwin/arm64  -> native-macos-aarch64 / pdfium-mac-arm64 / ... / onnxruntime-osx-arm64
#
# The in-process (Go) DeepDoc backend statically links ONNX Runtime via
# `--whole-archive` (Linux) or `-force_load` (macOS); the symlink-free
# `dlopen(NULL)` resolution means no `.so`/`.dylib` ships at runtime.

import argparse
import os
import platform as _platform
import shutil
import sys
import zipfile

import requests

# Mirrors internal/common.DeepDocORTVersion (Go in-process backend). ONE OF
# THREE places (with that Go constant and the ORT_VERSION in
# ragflow_deps/download_deps.py) that must carry the same ONNX Runtime native
# release for the statically-linked Go DeepDoc backend. There is no single
# source of truth — keep all three equal. The pip onnxruntime== pin
# (pyproject.toml) and the onnxruntime_go binding minor (go.mod) must stay on
# the same minor line.
ORT_VERSION = "1.23.2"

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
    """Return (zip_filename, extracted_top_level_dir) for ONNX Runtime.

    The upstream arch token differs from GOARCH: Linux uses x64/aarch64 and
    macOS uses x86_64/arm64. The Linux static libs also carry a glibc suffix
    (-glibc2_28); the macOS ones do not. The extracted dir name is what
    build.sh's stale-version guard matches, so it must equal the zip's
    top-level directory.
    """
    if goos == "linux":
        arch_token = "x64" if goarch == "amd64" else "aarch64"
        zip_name = f"onnxruntime-linux-{arch_token}-static_lib-{ORT_VERSION}-glibc2_28.zip"
        dir_name = f"onnxruntime-linux-{arch_token}-static_lib-{ORT_VERSION}-glibc2_28"
    else:  # darwin
        arch_token = "x86_64" if goarch == "amd64" else "arm64"
        zip_name = f"onnxruntime-osx-{arch_token}-static_lib-{ORT_VERSION}.zip"
        dir_name = f"onnxruntime-osx-{arch_token}-static_lib-{ORT_VERSION}"
    return zip_name, dir_name


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
    urls.append([_release_url("csukuangfj/onnxruntime-libs", f"v{ORT_VERSION}", ort_zip, mirror), ort_zip])

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
    `static_lib_dir`. Returns True when that version is available afterwards
    (extracted now or already present), False when the archive is missing.

    ORT ships a version-stamped top-level dir inside the zip
    (onnxruntime-<platform>-static_lib-<version>[-glibc2_28]/), so a present
    `static_lib_dir` is NOT evidence that THIS version is extracted: after a
    version bump the stale dir is pruned and the new one must be extracted.
    """
    if not os.path.isfile(archive_path):
        print(f"  Skipping extraction: {os.path.basename(archive_path)} not found")
        return False
    expected_path = os.path.join(static_lib_dir, expected_dir)
    if os.path.isdir(expected_path) and has_static_archives(expected_path):
        # Already present: still prune any co-resident foreign-platform or stale
        # ORT dir so a later build.sh `find ... -name '*.a'` cannot link two ORT
        # builds. build.sh's ort_dir_prefix guard also filters by platform, but
        # trimming here keeps the cache platform-exclusive.
        prune_stale_onnxruntime(static_lib_dir, expected_dir)
        print(f"  ✓ onnxruntime/static_lib ({expected_dir}) already extracted")
        return True
    prune_stale_onnxruntime(static_lib_dir, expected_dir)
    os.makedirs(static_lib_dir, exist_ok=True)
    print(f"  Extracting {os.path.basename(archive_path)} → {static_lib_dir}")
    with zipfile.ZipFile(archive_path) as zf:
        zf.extractall(static_lib_dir)
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
