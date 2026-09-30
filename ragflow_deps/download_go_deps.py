#!/usr/bin/env python3

# PEP 723 metadata
# /// script
# requires-python = ">=3.10"
# dependencies = [
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
# Embedding tokenizer assets: the Go token counters in internal/tokenizer load a
# tokenizer file per family (XLM-R SentencePiece, BERT WordPiece, two byte-level BPE
# families) from `huggingface.co/<repo>/<file>` at the top of ragflow_deps/ - the path
# the loaders search and the one ragflow_deps/Dockerfile ships. This script fetches
# them too, so one invocation gives a Go checkout everything it counts with. Without
# them every model that declares a tokenizer silently counts with the calibrated
# cl100k estimate instead (see internal/tokenizer/embedding_token_limits.md).

import argparse
import hashlib
import os
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
# account. The release tag is `onnxruntime-v{ORT_VERSION}` and the asset is
# `onnxruntime-v{ORT_VERSION}-linux-x86_64.zip`. The archive is occasionally
# re-issued under this SAME tag/asset name with patched content; both download
# scripts (this one and download_deps.py) detect that via a `{asset}.sha256`
# sidecar and re-download/re-extract, so a stale local copy never silently
# lingers.
ORT_VERSION = "1.29.0"


def _ort_asset_name(version):
    """Release asset filename under infiniflow/ragflow-build tag onnxruntime-v{version}."""
    return f"onnxruntime-v{version}-linux-x86_64.zip"


def _ort_extracted_dir(version):
    """Top-level directory name INSIDE the release zip (what extractall creates)."""
    return f"onnxruntime-v{version}-linux-x86_64"


def _ort_normalized_dir(version):
    """Directory name build.sh's `find ... -name '*.a'` glob expects under static_lib."""
    return f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28"


def _sha256_of(path):
    """sha256 of a file, streamed in chunks so large archives don't blow memory."""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


# Mirrors internal/common.DeepDocModelFiles (Go in-process DeepDoc backend).
# These are the ONLY weights the Go backend loads; the full InfiniFlow/deepdoc
# repo also ships .onnx (Python-only), which this Go-only script deliberately
# skips to keep the download lean.
DEEPDOC_REPO = "InfiniFlow/deepdoc"
DEEPDOC_MODEL_FILES = ["det.ort", "layout.ort", "tsr.ort", "rec.ort", "ocr.res"]

# Embedding tokenizer assets for the Go counters (internal/tokenizer). Each entry is
# (repo, file, kind):
#
#   "runtime" - the counters load it in production, so the runtime images must ship it
#               (the copy loop in Dockerfile / Dockerfile_base) and the loader must pin
#               its SHA-1;
#   "oracle"  - only scripts/gen_tokenizer_oracle.py needs it, to regenerate the test
#               fixtures, so it is deliberately NOT shipped to the image.
#
# ragflow_deps/test_tokenizer_assets.py holds those three places together, because a
# drift here is invisible at runtime: the counter simply becomes unavailable and ingest
# counts with the calibrated fallback.
#
# Fetched per file, not by snapshot: these repos also carry multi-GB weights we do not
# want. A missing asset is NOT fatal (unlike the DeepDoc weights above, without which
# the server cannot run at all).
TOKENIZER_ASSETS = [
    # XLM-R SentencePiece (Unigram) - the BAAI bge / multilingual-e5 / m3e /
    # gte-multilingual / jina-v3 family shares this vocabulary.
    ("BAAI/bge-m3", "sentencepiece.bpe.model", "runtime"),
    # BERT WordPiece vocab for the bge-*-en / e5-* / gte-base family.
    ("BAAI/bge-large-en-v1.5", "vocab.txt", "runtime"),
    # Byte-level BPE families (Qwen3-Embedding, Mistral/Llama embeddings).
    ("Qwen/Qwen3-Embedding-0.6B", "tokenizer.json", "runtime"),
    ("intfloat/e5-mistral-7b-instruct", "tokenizer.json", "runtime"),
    # Cross-check oracles only: the HF tokenizer.json files the same families convert to.
    ("BAAI/bge-m3", "tokenizer.json", "oracle"),
    ("BAAI/bge-large-en-v1.5", "tokenizer.json", "oracle"),
]


def prune_stale_onnxruntime(static_lib_dir, version):
    """Remove ONNX Runtime version dirs under static_lib that do NOT match
    `version`. Without this, a version bump leaves the stale dir next to
    the new one and build.sh's `find ... -name '*.a'` links BOTH (duplicate
    symbols / wrong version, silently)."""
    if not os.path.isdir(static_lib_dir):
        return
    expected = _ort_normalized_dir(version)
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

    The infiniflow/ragflow-build release zip carries a top-level dir named
    onnxruntime-v{version}-linux-x86_64, so a present
    `static_lib_dir` is NOT evidence that THIS version is extracted: after a
    version bump the stale dir is pruned and the new one must be extracted.
    """
    if not os.path.isfile(archive_path):
        print(f"  Skipping extraction: {os.path.basename(archive_path)} not found")
        return False
    prune_stale_onnxruntime(static_lib_dir, version)
    version_dir = os.path.join(static_lib_dir, _ort_normalized_dir(version))
    archive_sha = _sha256_of(archive_path)
    source_marker = os.path.join(version_dir, ".ragflow-source.sha256")
    if os.path.isdir(version_dir) and has_static_archives(version_dir):
        if os.path.isfile(source_marker):
            with open(source_marker, encoding="utf-8") as marker:
                if marker.read().strip() == archive_sha:
                    print(f"  ✓ onnxruntime/static_lib ({version}) already extracted to {version_dir}")
                    return True
        print(f"  Refreshing ONNX Runtime {version} from changed release archive")
        shutil.rmtree(version_dir)
    os.makedirs(static_lib_dir, exist_ok=True)
    print(f"  Extracting {os.path.basename(archive_path)} → {static_lib_dir}")
    with zipfile.ZipFile(archive_path) as zf:
        zf.extractall(static_lib_dir)
    # The infiniflow/ragflow-build release zip carries a top-level dir named
    # onnxruntime-v{version}-linux-x86_64, but build.sh's glob and the stale
    # checks above all expect onnxruntime-linux-x64-static_lib-{version}-glibc2_28.
    # Rename it so every consumer shares one name convention (driven by
    # ORT_VERSION).
    extracted = os.path.join(static_lib_dir, _ort_extracted_dir(version))
    normalized = os.path.join(static_lib_dir, _ort_normalized_dir(version))
    if os.path.isdir(extracted) and extracted != normalized:
        if os.path.exists(normalized):
            shutil.rmtree(normalized)
        print(f"  Renaming {os.path.basename(extracted)} → {os.path.basename(normalized)}")
        os.rename(extracted, normalized)
    with open(os.path.join(normalized, ".ragflow-source.sha256"), "w", encoding="utf-8") as marker:
        marker.write(f"{archive_sha}\n")
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
                f"https://gh-proxy.com/https://github.com/infiniflow/ragflow-build/releases/download/onnxruntime-v{ORT_VERSION}/{_ort_asset_name(ORT_VERSION)}",
                _ort_asset_name(ORT_VERSION),
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
                f"https://github.com/infiniflow/ragflow-build/releases/download/onnxruntime-v{ORT_VERSION}/{_ort_asset_name(ORT_VERSION)}",
                _ort_asset_name(ORT_VERSION),
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


def download_tokenizer_assets(use_china_mirrors=False):
    """Download the embedding tokenizer assets the Go counters load.

    Written to `ragflow_deps/huggingface.co/<repo>/<file>`: the path internal/tokenizer
    searches and the one ragflow_deps/Dockerfile builds its image from. Fetched per file
    (these repos also carry multi-GB weights we do not want), routed via hf-mirror.com
    when --china-mirrors is set.

    A missing asset is fatal, like every other asset this script provisions. The counters
    would otherwise fall back to the calibrated cl100k estimate, which under-counts some
    tokenizers - and an under-count is what makes a provider answer 400. The runtime
    images refuse to build on exactly the same condition (the Dockerfile copy loops exit
    1), so provisioning has to fail here rather than hand them a broken tree.
    """
    if use_china_mirrors:
        os.environ["HF_ENDPOINT"] = "https://hf-mirror.com"
    # Imported lazily so the module stays importable (and its unit tests stay
    # huggingface-free) without the huggingface_hub dependency installed.
    from huggingface_hub import hf_hub_download

    base = os.path.join(os.path.dirname(os.path.abspath(__file__)), "huggingface.co")
    missing = []
    for repo_id, filename, _kind in TOKENIZER_ASSETS:
        target_dir = os.path.join(base, repo_id)
        if os.path.isfile(os.path.join(target_dir, filename)):
            print(f"  ✓ {repo_id}/{filename} already present")
            continue
        os.makedirs(target_dir, exist_ok=True)
        print(f"Downloading tokenizer asset {repo_id}/{filename}...")
        try:
            hf_hub_download(repo_id=repo_id, filename=filename, local_dir=target_dir)
        except Exception as e:  # noqa: BLE001 - collected and surfaced below
            missing.append((repo_id, filename, e))

    if missing:
        for repo_id, filename, e in missing:
            print(f"  ERROR: could not fetch {repo_id}/{filename}: {e}", file=sys.stderr)
        print(
            "\n"
            "The embedding tokenizer counters in internal/tokenizer load these files from\n"
            f"  {base}\n"
            "Without one, the models that declare its counter fall back to the calibrated\n"
            "cl100k estimate: safe, but less precise - it under-counts some tokenizers, and\n"
            "an under-count is a request the provider rejects. The runtime images refuse to\n"
            "build on the same condition (their copy loops exit 1), so this script fails\n"
            "instead of handing a broken tree to the image build. To recover:\n"
            "  - re-run this script (a transient HF/network error usually clears);\n"
            "  - behind the GFW, re-run with --china-mirrors (routes via hf-mirror.com);\n"
            "  - or point MODEL_ASSETS_DIR at a directory holding the same layout.\n",
            file=sys.stderr,
        )
        sys.exit(1)

    print("  ✓ embedding tokenizer assets ready")


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

        # The ONNX Runtime archive is re-issued under the SAME release tag and
        # asset name whenever its content changes (e.g. the patched build that
        # exports SessionGetInitializer*). A pure existence check would then
        # keep a colleague's stale local copy and fail to link onnxruntime_go.
        # Verify against the published .sha256 sidecar so a re-issued archive
        # is always re-downloaded and re-extracted. Every other archive keeps
        # the legacy existence-based skip.
        is_ort = filename == _ort_asset_name(ORT_VERSION)
        expected_sha = None
        if is_ort:
            sidecar_url = download_url + ".sha256"
            try:
                resp = requests.get(sidecar_url, timeout=30)
                resp.raise_for_status()
                expected_sha = resp.text.split()[0]
            except Exception as exc:  # noqa: BLE001 - best-effort; fall back to legacy
                print(f"  WARNING: could not fetch {sidecar_url} ({exc}); skipping checksum for {filename}")

        needs_download = True
        if os.path.exists(filename):
            if expected_sha is not None:
                actual = _sha256_of(filename)
                if actual == expected_sha:
                    print(f"  ✓ {filename} checksum matches released {expected_sha}; skipping download")
                    needs_download = False
                else:
                    print(f"  {filename} checksum mismatch (local {actual} != released {expected_sha}); re-downloading")
            else:
                needs_download = False

        if needs_download:
            download_with_progress(download_url, filename)
            if expected_sha is not None:
                actual = _sha256_of(filename)
                if actual != expected_sha:
                    print(f"  ERROR: {filename} checksum mismatch after download (got {actual}, expected {expected_sha})", file=sys.stderr)
                    sys.exit(1)
                print(f"  ✓ {filename} checksum verified ({actual})")
            # Force re-extract below: drop any previously extracted version dir
            # so the same-named re-issued archive actually refreshes the .a files.
            if is_ort:
                native_libs = os.path.expanduser("~/ragflow-native-libs")
                version_dir = os.path.join(native_libs, "onnxruntime", "static_lib", _ort_normalized_dir(ORT_VERSION))
                if os.path.isdir(version_dir):
                    print(f"  Removing stale extracted ONNX Runtime dir: {version_dir}")
                    shutil.rmtree(version_dir)

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
        os.path.join(os.getcwd(), _ort_asset_name(ORT_VERSION)),
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

    # Download the Go DeepDoc `.ort` weights so this script is a one-stop for Go
    # dev: native libs (above) + model files (below) from a single invocation.
    download_go_models(args.china_mirrors)

    # Same one-stop idea for the token counters: their tokenizer files, in the layout
    # the loaders search and ragflow_deps/Dockerfile ships.
    download_tokenizer_assets(args.china_mirrors)
