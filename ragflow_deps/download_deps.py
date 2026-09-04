#!/usr/bin/env python3

# PEP 723 metadata
# /// script
# requires-python = ">=3.10"
# dependencies = [
#   "nltk",
#   "huggingface-hub"
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
#   uv run python3 ragflow_deps/download_deps.py            # download
#   cd ragflow_deps
#   docker build -f Dockerfile -t infiniflow/ragflow_deps .
#
# The main `Dockerfile` (built from the project root) pulls this image
# via `--mount=type=bind,from=infiniflow/ragflow_deps:latest,...` and
# is unaffected by where these files live locally.

import argparse
import os
import shutil
import urllib.request

# NLTK >=3.10 refuses proxied downloads (SSRF guard) unless opted in; the
# runners sit behind a proxy, so allow proxied fetches before importing nltk.
os.environ.setdefault("NLTK_ALLOW_PROXIED_URLOPEN", "1")

import nltk
from huggingface_hub import snapshot_download

# mirrors internal/common.DeepDocORTVersion (Go in-process backend). Single
# source for the onnxruntime native release: the download URL, .tgz name,
# extracted dir name, and SONAME below are all derived from it. The pip
# onnxruntime== pin (pyproject.toml) and the onnxruntime_go binding minor
# (go.mod) must stay on the same minor line.
ORT_VERSION = "1.23.2"


def get_urls(use_china_mirrors=False) -> list[str | list[str]]:
    if use_china_mirrors:
        return [
            "http://mirrors.tuna.tsinghua.edu.cn/ubuntu/pool/main/o/openssl/libssl1.1_1.1.1f-1ubuntu2_amd64.deb",
            "http://mirrors.tuna.tsinghua.edu.cn/ubuntu-ports/pool/main/o/openssl/libssl1.1_1.1.1f-1ubuntu2_arm64.deb",
            "https://repo.huaweicloud.com/repository/maven/org/apache/tika/tika-server-standard/3.3.0/tika-server-standard-3.3.0.jar",
            "https://repo.huaweicloud.com/repository/maven/org/apache/tika/tika-server-standard/3.3.0/tika-server-standard-3.3.0.jar.md5",
            "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken",
            ["https://registry.npmmirror.com/-/binary/chrome-for-testing/121.0.6167.85/linux64/chrome-linux64.zip", "chrome-linux64-121-0-6167-85"],
            ["https://registry.npmmirror.com/-/binary/chrome-for-testing/121.0.6167.85/linux64/chromedriver-linux64.zip", "chromedriver-linux64-121-0-6167-85"],
            "https://github.com/astral-sh/uv/releases/download/0.9.16/uv-x86_64-unknown-linux-gnu.tar.gz",
            "https://github.com/astral-sh/uv/releases/download/0.9.16/uv-aarch64-unknown-linux-gnu.tar.gz",
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
            # Native static libraries for Go build (pdfium, pdf_oxide, office_oxide)
            # Used by build.sh's check_*_deps functions — pre-downloaded to avoid
            # network access during CI.
            ["https://github.com/kognitos/pdfium-static/releases/download/chromium%2F7809/pdfium-linux-x64-static.tgz", "pdfium-linux-x64-static.tgz"],
            ["https://github.com/yfedoseev/pdf_oxide/releases/download/v0.3.73/pdf_oxide-go-ffi-linux-amd64.tar.gz", "pdf_oxide-go-ffi-linux-amd64.tar.gz"],
            ["https://github.com/yfedoseev/office_oxide/releases/download/v0.1.9/native-linux-x86_64.tar.gz", "office_oxide-linux-x86_64.tar.gz"],
            # ONNX Runtime static archives for the Go in-process (DeepDoc)
            # backend. Statically linked into the server binary (see build.sh:
            # ONNX_RUNTIME_STATIC_DIR, --whole-archive + --export-dynamic), so
            # no libonnxruntime.so is needed at runtime — OrtGetApiBase is
            # resolved via dlopen(self). csukuangfj's static_lib build is
            # CPU-only and glibc2_28-based, matching ORT_VERSION's C-API line
            # (ABI-compatible with onnxruntime_go) and the onnxruntime the
            # Python goldens were generated with.
            [
                f"https://github.com/csukuangfj/onnxruntime-libs/releases/download/v{ORT_VERSION}/onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip",
                f"onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip",
            ],
        ]
    else:
        return [
            "http://archive.ubuntu.com/ubuntu/pool/main/o/openssl/libssl1.1_1.1.1f-1ubuntu2_amd64.deb",
            "http://ports.ubuntu.com/pool/main/o/openssl/libssl1.1_1.1.1f-1ubuntu2_arm64.deb",
            "https://repo1.maven.org/maven2/org/apache/tika/tika-server-standard/3.3.0/tika-server-standard-3.3.0.jar",
            "https://repo1.maven.org/maven2/org/apache/tika/tika-server-standard/3.3.0/tika-server-standard-3.3.0.jar.md5",
            "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken",
            ["https://storage.googleapis.com/chrome-for-testing-public/121.0.6167.85/linux64/chrome-linux64.zip", "chrome-linux64-121-0-6167-85"],
            ["https://storage.googleapis.com/chrome-for-testing-public/121.0.6167.85/linux64/chromedriver-linux64.zip", "chromedriver-linux64-121-0-6167-85"],
            "https://github.com/astral-sh/uv/releases/download/0.9.16/uv-x86_64-unknown-linux-gnu.tar.gz",
            "https://github.com/astral-sh/uv/releases/download/0.9.16/uv-aarch64-unknown-linux-gnu.tar.gz",
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
            # Native static libraries for Go build (pdfium, pdf_oxide, office_oxide)
            # Used by build.sh's check_*_deps functions — pre-downloaded to avoid
            # network access during CI.
            ["https://github.com/kognitos/pdfium-static/releases/download/chromium%2F7809/pdfium-linux-x64-static.tgz", "pdfium-linux-x64-static.tgz"],
            ["https://github.com/yfedoseev/pdf_oxide/releases/download/v0.3.73/pdf_oxide-go-ffi-linux-amd64.tar.gz", "pdf_oxide-go-ffi-linux-amd64.tar.gz"],
            ["https://github.com/yfedoseev/office_oxide/releases/download/v0.1.9/native-linux-x86_64.tar.gz", "office_oxide-linux-x86_64.tar.gz"],
            # ONNX Runtime static archives for the Go in-process (DeepDoc)
            # backend. Statically linked into the server binary (see build.sh:
            # ONNX_RUNTIME_STATIC_DIR, --whole-archive + --export-dynamic), so
            # no libonnxruntime.so is needed at runtime — OrtGetApiBase is
            # resolved via dlopen(self). csukuangfj's static_lib build is
            # CPU-only and glibc2_28-based, matching ORT_VERSION's C-API line
            # (ABI-compatible with onnxruntime_go) and the onnxruntime the
            # Python goldens were generated with.
            [
                f"https://github.com/csukuangfj/onnxruntime-libs/releases/download/v{ORT_VERSION}/onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip",
                f"onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip",
            ],
        ]


repos = [
    "InfiniFlow/text_concat_xgb_v1.0",
    "InfiniFlow/deepdoc",
]


def download_model(repository_id):
    local_directory = os.path.abspath(os.path.join("huggingface.co", repository_id))
    os.makedirs(local_directory, exist_ok=True)
    snapshot_download(repo_id=repository_id, local_dir=local_directory)


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
    opener = urllib.request.build_opener()
    opener.addheaders = [("User-Agent", "Mozilla/5.0")]
    urllib.request.install_opener(opener)

    for url in urls:
        download_url = url[0] if isinstance(url, list) else url
        filename = url[1] if isinstance(url, list) else url.split("/")[-1]
        print(f"Downloading {filename} from {download_url}...")
        if not os.path.exists(filename):
            urllib.request.urlretrieve(download_url, filename)

    # Extract native static libraries to ~/ragflow-native-libs for Go build.
    # Ensures build.sh can find them without network access.
    native_deps_dir = os.path.expanduser("~/ragflow-native-libs")
    extractions = [
        ("pdfium-linux-x64-static.tgz", "pdfium-static"),
        ("pdf_oxide-go-ffi-linux-amd64.tar.gz", "pdf_oxide"),
        ("office_oxide-linux-x86_64.tar.gz", "office_oxide"),
        (f"onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28.zip", os.path.join("onnxruntime", "static_lib")),
    ]
    import tarfile
    import zipfile

    def _prune_stale_onnxruntime(static_lib_dir, version):
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

    for archive, subdir in extractions:
        archive_path = os.path.join(os.getcwd(), archive)
        if not os.path.isfile(archive_path):
            print(f"  Skipping extraction: {archive} not found")
            continue
        target = os.path.join(native_deps_dir, subdir)

        # ONNX Runtime ships a version-stamped top-level dir inside the zip
        # (onnxruntime-linux-x64-static_lib-<ORT_VERSION>-glibc2_28/). A plain
        # "any .a present?" skip would keep a STALE version in place after a
        # bump: the new zip downloads, but extraction is skipped because the
        # old .a is still under static_lib, so the bump silently does nothing.
        # Prune stale version dirs and only skip when the matching version is
        # already extracted.
        if subdir == os.path.join("onnxruntime", "static_lib"):
            _prune_stale_onnxruntime(target, ORT_VERSION)
            version_dir = os.path.join(target, f"onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28")
            if os.path.isdir(version_dir) and any(f.endswith(".a") for _, _, files in os.walk(version_dir) for f in files):
                print(f"  ✓ {subdir} ({ORT_VERSION}) already extracted to {version_dir}")
                continue

        if os.path.isdir(target) and any(f.endswith(".a") for _, _, files in os.walk(target) for f in files):
            print(f"  ✓ {subdir} already extracted to {target}")
            continue
        os.makedirs(target, exist_ok=True)
        print(f"  Extracting {archive} → {target}")
        if archive_path.endswith(".zip"):
            with zipfile.ZipFile(archive_path) as zf:
                zf.extractall(target)
        else:
            with tarfile.open(archive_path) as tf:
                tf.extractall(target)

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
        print(f"  Skipping onnxruntime static check: no .a found under {ort_static_dir}")

    local_dir = os.path.abspath("nltk_data")
    # NLTK >=3.8.2 gates `wordnet` behind `omw-1.4`; both must be provisioned
    # or tokenization-backed paths raise LookupError at runtime.
    for data in ["omw-1.4", "wordnet", "punkt", "punkt_tab"]:
        print(f"Downloading nltk {data}...")
        nltk.download(data, download_dir=local_dir)

    for repo_id in repos:
        print(f"Downloading huggingface repo {repo_id}...")
        download_model(repo_id)
