#!/usr/bin/env python3

# PEP 723 metadata
# /// script
# requires-python = ">=3.10"
# dependencies = [
#   "huggingface-hub",
#   "requests",
# ]
# ///

# This script prepares the runtime / image resources that the dependency image
# ships on Linux x86_64: the cl100k BPE table, the stagehand-server binaries
# (installed into the Go SDK's cache), the Go DeepDoc `.ort` weights
# (det/layout/tsr/rec.ort + ocr.res), and the embedding tokenizer assets the Go
# counters load.
#
# This is the CI / image-build one-stop. It fetches the runtime resources PLUS
# the stagehand server binary, and deliberately does NOT fetch the native static
# libraries the Go build links — those come from tools/scripts/download_dev_deps.py
# and are baked into the server binary by the build stage. Run this once to ready
# a dependency image; for local development, run the dev downloader instead.
# All version numbers and URLs are defined in the block below; bump a version
# there, not at the call sites.
#
# Typical workflow:
#
#   uv run ragflow_deps/download_deps.py
#
# Go DeepDoc weights: in addition to the resources above, this script downloads
# the five Go model files (internal/common.DeepDocModelFiles) from
# InfiniFlow/deepdoc straight into the repo's canonical model directory
# `internal/rag/res/deepdoc/` (one level up from this script). The Go server
# auto-discovers that directory via resolveDeepDocModelDir(), and build.sh
# --test-native / internal/deepdoc/native/run.sh default MODEL_DIR there too —
# so after running this script NO MODEL_DIR env needs to be set:
#
#   bash build.sh --test-native          # or: cd internal/deepdoc/native && bash run.sh
#
# Embedding tokenizer assets: the Go token counters in internal/tokenizer load a
# tokenizer file per family (XLM-R SentencePiece, BERT WordPiece, two byte-level BPE
# families) from `huggingface.co/<repo>/<file>` at the top of ragflow_deps/ - the path
# the loaders search and the one ragflow_deps/Dockerfile ships. This script fetches
# them too, per the inlined TOKENIZER_ASSETS list. Models that declare a missing
# tokenizer cannot be ingested (see internal/tokenizer/embedding_token_limits.md).

import argparse
import os
import platform
import re
import shutil
import sys
import tempfile

import requests

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(SCRIPT_DIR)

# ── Dependency versions & URLs (edit here) ────────────────────────────────
# Single source of truth for the runtime / image-resource versions and URLs
# this script fetches. build.sh greps none of these (they are image-facing);
# keep the `NAME = "value"` form greppable for consistency with
# tools/scripts/download_dev_deps.py.

# stagehand-server release binaries (used by the Browser component in local
# mode). They are LOOSELY matched to the stagehand-go Go module: both stay on
# the v3.x line and remain protocol-compatible, but the two version numbers do
# NOT track each other, so there is no cross-reference to keep in sync here.
# Kept with the "v" prefix because it is used verbatim in the release tag path.
STAGEHAND_SERVER_VERSION = "v3.7.2"


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


def cl100k_url() -> str:
    """cl100k BPE table. Hosted on Azure blob storage (NOT a GitHub release),
    so the GitHub mirror prefix does not apply and is deliberately omitted."""
    return "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"


def stagehand_url(arch: str) -> str:
    """stagehand-server release binary: stagehand_url("x64" | "arm64")."""
    return f"{gh_proxy()}https://github.com/browserbase/stagehand/releases/download/stagehand-server-v3/{STAGEHAND_SERVER_VERSION}/stagehand-server-v3-linux-{arch}"


# Mirrors internal/common.DeepDocModelFiles (Go in-process DeepDoc backend).
# These are the ONLY weights the Go backend loads; the full InfiniFlow/deepdoc
# repo also ships .onnx (Python-only), which this Go-only script deliberately
# skips to keep the download lean. The files are fetched one by one, so this
# list MUST be edited by hand when the Go slice changes.
DEEPDOC_REPO = "InfiniFlow/deepdoc"
DEEPDOC_MODEL_FILES = ["det.ort", "layout.ort", "tsr.ort", "rec.ort", "ocr.res"]


# Embedding tokenizer assets the Go counters load. Inlined here (no separate
# manifest file) as the single source of truth this script reads directly; the
# dev script tools/scripts/download_dev_deps.py carries its own copy (no shared
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


def download_with_progress(url, filename):
    """Download URL to FILENAME atomically (temp file, then move on success).

    A failed or empty download leaves any previous valid file untouched, so an
    interrupted run never destroys a working dependency tree. Mirrors
    download_dev_deps.download_with_progress."""
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


def _valid_stagehand_binary(path):
    """True when PATH is a non-empty ELF executable (the stagehand-server SEA)."""
    if not os.path.isfile(path) or os.path.getsize(path) == 0:
        return False
    with open(path, "rb") as file:
        return file.read(4) == b"\x7fELF"


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


def _install_stagehand():
    if platform.system() != "Linux":
        raise RuntimeError("The downloaded native dependencies support Linux only")
    architecture = {"x86_64": "x64", "aarch64": "arm64"}.get(platform.machine())
    if architecture is None:
        raise RuntimeError(f"Unsupported stagehand architecture: {platform.machine()}")
    version = go_module_version("github.com/browserbase/stagehand-go/v3", REPO_ROOT)
    cache_root = os.environ.get("XDG_CACHE_HOME") or os.path.expanduser("~/.cache")
    target_dir = os.path.join(cache_root, "stagehand", "lib", f"go_{version}")
    os.makedirs(target_dir, exist_ok=True)
    filename = f"stagehand-server-v3-linux-{architecture}"
    source = os.path.join(SCRIPT_DIR, filename)
    target = os.path.join(target_dir, filename)
    with tempfile.TemporaryDirectory(dir=target_dir) as staging:
        prepared = os.path.join(staging, filename)
        shutil.copyfile(source, prepared)
        os.chmod(prepared, 0o755)
        os.replace(prepared, target)
    print(f"  ✓ stagehand installed to {target}")


def download_go_models(use_china_mirrors=False):
    """Download the Go DeepDoc `.ort` weights so a Go dev can run the in-process
    backend with no further setup.

    The files are written into the repo's canonical model directory
    `internal/rag/res/deepdoc/` (relative to the repo root, one level up from
    this script). The Go server auto-discovers that directory, and build.sh
    --test-native defaults MODEL_DIR there too. A copy also lands under
    ragflow_deps/huggingface.co/InfiniFlow/deepdoc/ to keep the dependency
    image's existing build-context layout.
    """
    if use_china_mirrors:
        os.environ["HF_ENDPOINT"] = "https://hf-mirror.com"
    # Imported lazily so the module stays importable (and its unit tests stay
    # huggingface-free) without the huggingface_hub dependency installed.
    from huggingface_hub import hf_hub_download

    target_dir = os.path.join(REPO_ROOT, "internal", "rag", "res", "deepdoc")
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
    image_dir = os.path.join(SCRIPT_DIR, "huggingface.co", DEEPDOC_REPO)
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

    base = os.path.join(SCRIPT_DIR, "huggingface.co")
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


def main():
    parser = argparse.ArgumentParser(description="Download the runtime / image resources with optional China mirror support")
    parser.add_argument("--china-mirrors", action="store_true", help="Use China-accessible mirrors for downloads")
    args = parser.parse_args()

    # Anchor archives, the BPE table, and tokenizer assets to ragflow_deps/.
    os.chdir(SCRIPT_DIR)

    # ── cl100k_base BPE table ──────────────────────────────────────────────
    print("== cl100k_base.tiktoken ==")
    cl100k = os.path.join(SCRIPT_DIR, "cl100k_base.tiktoken")
    if os.path.isfile(cl100k) and os.path.getsize(cl100k) > 0:
        print("  ✓ cl100k_base.tiktoken already present")
    else:
        print(f"Downloading cl100k_base.tiktoken from {cl100k_url()}...")
        download_with_progress(cl100k_url(), cl100k)

    # ── stagehand-server binaries (image root + local cache) ────────────────
    print("== stagehand-server ==")
    for arch in ("x64", "arm64"):
        filename = f"stagehand-server-v3-linux-{arch}"
        target = os.path.join(SCRIPT_DIR, filename)
        if os.path.isfile(target) and os.path.getsize(target) > 0:
            print(f"  ✓ {filename} already present")
            continue
        print(f"Downloading {filename} from {stagehand_url(arch)}...")
        download_with_progress(stagehand_url(arch), target)
        if not _valid_stagehand_binary(target):
            os.unlink(target)
            raise RuntimeError(f"Invalid downloaded dependency: {filename}")
    _install_stagehand()

    # Download the Go DeepDoc `.ort` weights so this script is a one-stop for a
    # Go dev's runtime resources: models (below) from a single invocation.
    print("== DeepDoc models ==")
    download_go_models(args.china_mirrors)

    # Same one-stop idea for the token counters: their tokenizer files, in the layout
    # the loaders search and ragflow_deps/Dockerfile ships.
    print("== tokenizer assets ==")
    download_tokenizer_assets(args.china_mirrors)

    print()
    print(f"✓ Runtime/image resources ready under {SCRIPT_DIR}/")
    print("  Build the dependency image with:")
    print(f"    cd {SCRIPT_DIR} && docker build -f Dockerfile -t infiniflow/ragflow_deps:latest .")


if __name__ == "__main__":
    main()
