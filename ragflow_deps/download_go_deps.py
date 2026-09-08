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

import argparse
import os
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


def prune_stale_onnxruntime(static_lib_dir, version):
    """Remove ONNX Runtime version dirs under static_lib that do NOT match
    `version`. Without this, a version bump leaves the stale dir next to
    the new one and build.sh's `find ... -name '*.a'` links BOTH (duplicate
    symbols / wrong version, silently)."""
    if not os.path.isdir(static_lib_dir):
        return
    expected = f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28"
    for name in os.listdir(static_lib_dir):
        if not name.startswith("onnxruntime-linux-x64-static_lib-"):
            continue
        if name == expected:
            continue
        stale = os.path.join(static_lib_dir, name)
        print(f"  Removing stale ONNX Runtime dir: {stale}")
        shutil.rmtree(stale)


def has_static_archives(directory):
    """True when `directory` holds at least one static archive (.a)."""
    return any(f.endswith(".a") for _, _, files in os.walk(directory) for f in files)


def extract_onnxruntime(static_lib_dir, archive_path, version):
    """Ensure the ONNX Runtime static archives for `version` sit under
    `static_lib_dir`. Returns True when that version is available afterwards
    (extracted now or already present), False when the archive is missing.

    ORT ships a version-stamped top-level dir inside the zip
    (onnxruntime-linux-x64-static_lib-<version>-glibc2_28/), so a present
    `static_lib_dir` is NOT evidence that THIS version is extracted: after a
    version bump the stale dir is pruned and the new one must be extracted.
    """
    if not os.path.isfile(archive_path):
        print(f"  Skipping extraction: {os.path.basename(archive_path)} not found")
        return False
    prune_stale_onnxruntime(static_lib_dir, version)
    version_dir = os.path.join(static_lib_dir, f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28")
    if os.path.isdir(version_dir) and has_static_archives(version_dir):
        print(f"  ✓ onnxruntime/static_lib ({version}) already extracted to {version_dir}")
        return True
    os.makedirs(static_lib_dir, exist_ok=True)
    print(f"  Extracting {os.path.basename(archive_path)} → {static_lib_dir}")
    with zipfile.ZipFile(archive_path) as zf:
        zf.extractall(static_lib_dir)
    return True


def get_urls(use_china_mirrors=False) -> list[str | list[str]]:
    if use_china_mirrors:
        return [
            # stagehand-server-v3 Node.js SEA binaries (used by Browser
            # component in local mode).
            #
            # The stagehand-go Go module (pinned in go.mod) and the
            # stagehand-server binary (this release) are LOOSELY
            # MATCHED — both stay on the v3.x line and remain
            # protocol-compatible. The two version numbers do NOT
            # track each other: the Go SDK is at v3.21.0 while the
            # current latest server release is v3.7.2.
            #
            # On every go.mod bump, refresh this URL to the current
            # latest server release. There is no version
            # correspondence to maintain; "both on v3.x" is the
            # compatibility contract.
            "https://gh-proxy.com/https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/v3.7.2/stagehand-server-v3-linux-x64",
            "https://gh-proxy.com/https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/v3.7.2/stagehand-server-v3-linux-arm64",
            # Native static libraries for Go build (pdfium, pdf_oxide,
            # office_oxide, onnxruntime). Used by build.sh's check_*_deps
            # functions — pre-downloaded to avoid network access during CI.
            ["https://gh-proxy.com/https://github.com/kognitos/pdfium-static/releases/download/chromium%2F7809/pdfium-linux-x64-static.tgz", "pdfium-linux-x64-static.tgz"],
            ["https://gh-proxy.com/https://github.com/yfedoseev/pdf_oxide/releases/download/v0.3.73/pdf_oxide-go-ffi-linux-amd64.tar.gz", "pdf_oxide-go-ffi-linux-amd64.tar.gz"],
            ["https://gh-proxy.com/https://github.com/yfedoseev/office_oxide/releases/download/v0.1.9/native-linux-x86_64.tar.gz", "office_oxide-linux-x86_64.tar.gz"],
            [
                f"https://gh-proxy.com/https://github.com/csukuangfj/onnxruntime-libs/releases/download/v{ORT_VERSION}/onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip",
                f"onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip",
            ],
        ]
    else:
        return [
            # stagehand-server-v3 Node.js SEA binaries (used by Browser
            # component in local mode).
            #
            # The stagehand-go Go module (pinned in go.mod) and the
            # stagehand-server binary (this release) are LOOSELY
            # MATCHED — both stay on the v3.x line and remain
            # protocol-compatible. The two version numbers do NOT
            # track each other: the Go SDK is at v3.21.0 while the
            # current latest server release is v3.7.2.
            #
            # On every go.mod bump, refresh this URL to the current
            # latest server release. There is no version
            # correspondence to maintain; "both on v3.x" is the
            # compatibility contract.
            "https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/v3.7.2/stagehand-server-v3-linux-x64",
            "https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/v3.7.2/stagehand-server-v3-linux-arm64",
            # Native static libraries for Go build (pdfium, pdf_oxide,
            # office_oxide, onnxruntime). Used by build.sh's check_*_deps
            # functions — pre-downloaded to avoid network access during CI.
            ["https://github.com/kognitos/pdfium-static/releases/download/chromium%2F7809/pdfium-linux-x64-static.tgz", "pdfium-linux-x64-static.tgz"],
            ["https://github.com/yfedoseev/pdf_oxide/releases/download/v0.3.73/pdf_oxide-go-ffi-linux-amd64.tar.gz", "pdf_oxide-go-ffi-linux-amd64.tar.gz"],
            ["https://github.com/yfedoseev/office_oxide/releases/download/v0.1.9/native-linux-x86_64.tar.gz", "office_oxide-linux-x86_64.tar.gz"],
            [
                f"https://github.com/csukuangfj/onnxruntime-libs/releases/download/v{ORT_VERSION}/onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip",
                f"onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip",
            ],
        ]


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
    args = parser.parse_args()

    urls = get_urls(args.china_mirrors)

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
        ("pdfium-linux-x64-static.tgz", "pdfium-static"),
        ("pdf_oxide-go-ffi-linux-amd64.tar.gz", "pdf_oxide"),
        ("office_oxide-linux-x86_64.tar.gz", "office_oxide"),
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

    if not extract_onnxruntime(
        os.path.join(native_deps_dir, "onnxruntime", "static_lib"),
        os.path.join(os.getcwd(), f"onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip"),
        ORT_VERSION,
    ):
        # The archive was not downloaded or failed to extract, so no .a landed.
        # Fail loud instead of exiting 0: build.sh's ORT guard would otherwise
        # reject the build later with a less actionable message, and a missing
        # .a left here is exactly the "silent green" this PR is meant to prevent.
        print(
            f"  ERROR: ONNX Runtime static archives for {ORT_VERSION} were not "
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
