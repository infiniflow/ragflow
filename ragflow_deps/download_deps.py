#!/usr/bin/env python3

# PEP 723 metadata
# /// script
# requires-python = ">=3.10"
# dependencies = [
#   "huggingface-hub",
#   "requests",
# ]
# ///

# This script prepares Go dependencies on Linux x86_64: the native static
# libraries (pdfium / pdf_oxide / office_oxide / onnxruntime) for `build.sh`,
# and the Go DeepDoc `.ort` weights (det/layout/tsr/rec.ort + ocr.res) so a Go
# dev can run the in-process backend locally. It also downloads the cl100k BPE
# table and installs the local stagehand driver in the Go SDK's cache.
# Archives and tokenizer assets land under ragflow_deps/; DeepDoc weights land
# under internal/rag/res/deepdoc/, regardless of the caller's working directory.
#
# Downloaded archives and tokenizer assets land under `ragflow_deps/`.
#
# Typical workflow:
#
#   uv run ragflow_deps/download_deps.py
# Go DeepDoc weights: in addition to the native libs, this script downloads the
# five Go model files (internal/common.DeepDocModelFiles) from InfiniFlow/deepdoc
# straight into the repo's canonical model directory `internal/rag/res/deepdoc/` (one level
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
# them too. Models that declare a missing tokenizer cannot be ingested
# (see internal/tokenizer/embedding_token_limits.md).

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

# Mirrors internal/common.DeepDocORTVersion (Go in-process backend). ONE OF
# THREE places (with that Go constant and ARG ORT_VERSION in Dockerfile)
# that must carry the same ONNX Runtime
# native release for the statically-linked Go DeepDoc backend. There is no
# single source of truth — keep all three equal. build.sh --check-ort-version
# greps this file (and the other two) to fail fast on drift. (The Python pip
# onnxruntime== pin in pyproject.toml is versioned independently and is not
# part of this check.)
#
# Source of the native static archives: infiniflow/ragflow-build (our own
# ORT-only minimal build), NOT the third-party csukuangfj/onnxruntime-libs
# account. The release tag is `onnxruntime-v{ORT_VERSION}` and the asset is
# `onnxruntime-v{ORT_VERSION}-linux-x86_64.zip`. The archive is occasionally
# re-issued under this SAME tag/asset name with patched content; this script
# detects that via a `{asset}.sha256`
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
# ragflow_deps/test_tokenizer_assets.py checks these download, packaging, and pin lists.
#
# Fetched per file, not by snapshot: these repos also carry multi-GB weights we do not
# want. Missing tokenizer assets fail dependency preparation.
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
    if os.path.isdir(version_dir) and has_static_archives(version_dir):
        print(f"  ✓ onnxruntime/static_lib ({version}) already extracted to {version_dir}")
        return True
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
    return True


def _go_module_version(module):
    repo_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    with open(os.path.join(repo_root, "go.mod")) as file:
        version = re.search(rf"{re.escape(module)}\s+v([^\s]+)", file.read())
    if version is None:
        raise RuntimeError(f"{module} version not found in go.mod")
    return version.group(1)


def get_urls(use_china_mirrors=False) -> list[str | list[str]]:
    office_version = _go_module_version("github.com/yfedoseev/office_oxide/go")
    if use_china_mirrors:
        return [
            "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken",
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
            [f"https://gh-proxy.com/https://github.com/yfedoseev/office_oxide/releases/download/v{office_version}/native-linux-x86_64.tar.gz", "office_oxide-linux-x86_64.tar.gz"],
            [
                f"https://gh-proxy.com/https://github.com/infiniflow/ragflow-build/releases/download/onnxruntime-v{ORT_VERSION}/{_ort_asset_name(ORT_VERSION)}",
                _ort_asset_name(ORT_VERSION),
            ],
        ]
    else:
        return [
            "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken",
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
            [f"https://github.com/yfedoseev/office_oxide/releases/download/v{office_version}/native-linux-x86_64.tar.gz", "office_oxide-linux-x86_64.tar.gz"],
            [
                f"https://github.com/infiniflow/ragflow-build/releases/download/onnxruntime-v{ORT_VERSION}/{_ort_asset_name(ORT_VERSION)}",
                _ort_asset_name(ORT_VERSION),
            ],
        ]


def download_with_progress(url, filename):
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


def _extract_native_archive(archive_path, target, required_files, version=None):
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
    with tempfile.TemporaryDirectory(dir=os.path.dirname(target)) as staging:
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
    print(f"  ✓ extracted {os.path.basename(archive_path)} to {target}")


def _install_stagehand():
    if platform.system() != "Linux":
        raise RuntimeError("The downloaded native dependencies support Linux only")
    architecture = {"x86_64": "x64", "aarch64": "arm64"}.get(platform.machine())
    if architecture is None:
        raise RuntimeError(f"Unsupported stagehand architecture: {platform.machine()}")
    version = _go_module_version("github.com/browserbase/stagehand-go/v3")
    cache_root = os.environ.get("XDG_CACHE_HOME") or os.path.expanduser("~/.cache")
    target_dir = os.path.join(cache_root, "stagehand", "lib", f"go_{version}")
    os.makedirs(target_dir, exist_ok=True)
    filename = f"stagehand-server-v3-linux-{architecture}"
    source = os.path.join(os.path.dirname(os.path.abspath(__file__)), filename)
    target = os.path.join(target_dir, filename)
    with tempfile.TemporaryDirectory(dir=target_dir) as staging:
        prepared = os.path.join(staging, filename)
        shutil.copyfile(source, prepared)
        os.chmod(prepared, 0o755)
        os.replace(prepared, target)
    print(f"  ✓ stagehand installed to {target}")


def _valid_download(filename):
    if not os.path.isfile(filename) or os.path.getsize(filename) == 0:
        return False
    if filename.endswith((".tar.gz", ".tgz")):
        if not tarfile.is_tarfile(filename):
            return False
        if os.path.basename(filename) == "office_oxide-linux-x86_64.tar.gz":
            version = _go_module_version("github.com/yfedoseev/office_oxide/go")
            with tarfile.open(filename) as archive:
                try:
                    member = next((member for member in archive.getmembers() if os.path.normpath(member.name) == "lib/liboffice_oxide.a"), None)
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
    if os.path.basename(filename).startswith("stagehand-server-"):
        with open(filename, "rb") as file:
            return file.read(4) == b"\x7fELF"
    return True


def download_go_models(use_china_mirrors=False):
    """Download the Go DeepDoc `.ort` weights so a Go dev can run the in-process
    backend with no further setup.

    The files are written into the repo's canonical model directory
    `internal/rag/res/deepdoc/` (relative to the repo root, one level up from this
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
    target_dir = os.path.join(repo_root, "internal", "rag", "res", "deepdoc")
    os.makedirs(target_dir, exist_ok=True)

    missing = []
    for fname in DEEPDOC_MODEL_FILES:
        dest = os.path.join(target_dir, fname)
        if os.path.isfile(dest) and os.path.getsize(dest) > 0:
            print(f"  ✓ {fname} already present")
            continue
        print(f"Downloading deepdoc model {fname}...")
        try:
            hf_hub_download(repo_id=DEEPDOC_REPO, filename=fname, local_dir=target_dir)
            if not os.path.isfile(dest) or os.path.getsize(dest) == 0:
                raise RuntimeError(f"Empty or missing model: {dest}")
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
            "  - or copy the missing files into that directory by hand.",
            file=sys.stderr,
        )
        sys.exit(1)

    # Keep the dependency image's existing huggingface.co build-context layout.
    image_dir = os.path.join(repo_root, "ragflow_deps", "huggingface.co", DEEPDOC_REPO)
    os.makedirs(image_dir, exist_ok=True)
    for fname in DEEPDOC_MODEL_FILES:
        shutil.copyfile(os.path.join(target_dir, fname), os.path.join(image_dir, fname))

    print(f"  ✓ Go DeepDoc models ready under {target_dir}")
    print("    No MODEL_DIR env needed: the Go backend auto-discovers this directory.")


def download_tokenizer_assets(use_china_mirrors=False):
    """Download the embedding tokenizer assets the Go counters load.

    Written to `ragflow_deps/huggingface.co/<repo>/<file>`: the path internal/tokenizer
    searches and the one ragflow_deps/Dockerfile builds its image from. Fetched per file
    (these repos also carry multi-GB weights we do not want), routed via hf-mirror.com
    when --china-mirrors is set.

    A missing asset is fatal: models declaring its tokenizer refuse ingestion.
    Runtime images also reject missing files during their copy step.
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
        target_file = os.path.join(target_dir, filename)
        if os.path.isfile(target_file) and os.path.getsize(target_file) > 0:
            print(f"  ✓ {repo_id}/{filename} already present")
            continue
        os.makedirs(target_dir, exist_ok=True)
        print(f"Downloading tokenizer asset {repo_id}/{filename}...")
        try:
            hf_hub_download(repo_id=repo_id, filename=filename, local_dir=target_dir)
            if not os.path.isfile(target_file) or os.path.getsize(target_file) == 0:
                raise RuntimeError(f"Empty or missing tokenizer asset: {target_file}")
        except Exception as e:  # noqa: BLE001 - collected and surfaced below
            missing.append((repo_id, filename, e))

    if missing:
        for repo_id, filename, e in missing:
            print(f"  ERROR: could not fetch {repo_id}/{filename}: {e}", file=sys.stderr)
        print(
            "\n"
            "The embedding tokenizer counters in internal/tokenizer load these files from\n"
            f"  {base}\n"
            "Without one, models that declare its tokenizer cannot be ingested.\n"
            "The runtime images refuse to\n"
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
    # Anchor archives, the BPE table, and tokenizer assets to ragflow_deps/.
    os.chdir(os.path.dirname(os.path.abspath(__file__)))

    parser = argparse.ArgumentParser(description="Download dependencies with optional China mirror support")
    parser.add_argument("--china-mirrors", action="store_true", help="Use China-accessible mirrors for downloads")
    args = parser.parse_args()

    urls = get_urls(args.china_mirrors)

    for url in urls:
        download_url = url[0] if isinstance(url, list) else url
        filename = url[1] if isinstance(url, list) else url.split("/")[-1]
        print(f"Downloading {filename} from {download_url}...")

        # The ONNX Runtime archive is re-issued under the SAME release tag and
        # asset name whenever its content changes (e.g. the patched build that
        # exports SessionGetInitializer*). A pure existence check would then
        # keep a colleague's stale local copy and fail to link onnxruntime_go.
        # Verify against the published .sha256 sidecar so a re-issued archive
        # is always re-downloaded and re-extracted.
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
        if _valid_download(filename):
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
                    os.unlink(filename)
                    print(f"  ERROR: {filename} checksum mismatch after download (got {actual}, expected {expected_sha})", file=sys.stderr)
                    sys.exit(1)
                print(f"  ✓ {filename} checksum verified ({actual})")
            if not _valid_download(filename):
                os.unlink(filename)
                raise RuntimeError(f"Invalid downloaded dependency: {filename}")
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
    office_version = _go_module_version("github.com/yfedoseev/office_oxide/go")
    extractions = [
        ("pdfium-linux-x64-static.tgz", "pdfium-static", ["lib/libpdfium.a", "lib/libc++.a", "lib/libc++abi.a", "include/fpdfview.h"], None),
        ("pdf_oxide-go-ffi-linux-amd64.tar.gz", "pdf_oxide", ["lib/linux_amd64/libpdf_oxide.a", "include/pdf_oxide.h"], None),
        ("office_oxide-linux-x86_64.tar.gz", "office_oxide", ["lib/liboffice_oxide.a", "include/office_oxide_c/office_oxide.h"], office_version),
    ]

    for archive, subdir, required_files, version in extractions:
        archive_path = os.path.join(os.getcwd(), archive)
        if not os.path.isfile(archive_path):
            print(f"  Skipping extraction: {archive} not found")
            continue
        target = os.path.join(native_deps_dir, subdir)
        try:
            _extract_native_archive(archive_path, target, required_files, version)
        except (tarfile.TarError, EOFError, RuntimeError):
            os.unlink(archive_path)
            raise

    _install_stagehand()

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
