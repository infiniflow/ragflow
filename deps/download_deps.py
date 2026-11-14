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
# ships on Linux x86_64: the cl100k BPE table, the Go DeepDoc `.ort` weights
# (det/layout/tsr/rec.ort + ocr.res), and the embedding tokenizer assets the Go
# counters load.
#
# This is the CI / image-build one-stop. It fetches the runtime resources
# and deliberately does NOT fetch the native static libraries the Go build links
# — those come from tools/scripts/download_dev_deps.py
# and are baked into the server binary by the build stage. Run this once to ready
# a dependency image; for local development, run the dev downloader instead.
# All version numbers and URLs are defined in the block below; bump a version
# there, not at the call sites.
#
# Typical workflow:
#
#   uv run deps/download_deps.py
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
# families) from `huggingface.co/<repo>/<file>` at the top of deps/ - the path
# the loaders search and the one deps/Dockerfile ships. This script fetches
# them too, per the inlined TOKENIZER_ASSETS list. Models that declare a missing
# tokenizer cannot be ingested (see internal/tokenizer/embedding_token_limits.md).

import argparse
import hashlib
import os
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

# Noto Sans CJK SC variable TTF, rendered into PDFs by the Go backend
# (internal/agent/component/io/pdf_writer.go). When non-empty, it is the pinned
# SHA-256 every downloaded/kept font must match. When empty, the hash recorded in
# deps/NotoSansCJKsc-VF.ttf.sha256 is the baseline: a present non-empty sidecar is
# verified (drift rejected), while a missing/empty sidecar is bootstrapped by
# recording the downloaded hash so the next run detects changes. Both first and
# later runs therefore run the download directly without manual setup.
NOTO_CJK_FONT_SHA256 = ""
GH_PROXY_URL = "https://gh-proxy.com/"


def gh_proxy() -> str:
    """Return the supported GitHub proxy prefix, rejecting arbitrary hosts."""
    proxy = (os.environ.get("GH_PROXY") or "").strip()
    if not proxy:
        return ""
    if proxy.rstrip("/") != GH_PROXY_URL.rstrip("/"):
        raise RuntimeError(f"GH_PROXY must be empty or {GH_PROXY_URL!r}")
    return GH_PROXY_URL


def cl100k_url() -> str:
    """cl100k BPE table. Hosted on Azure blob storage (NOT a GitHub release),
    so the GitHub mirror prefix does not apply and is deliberately omitted."""
    return "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"


def noto_cjk_font_url() -> str:
    """Noto Sans CJK SC variable TTF, rendered into PDFs by the Go backend
    (internal/agent/component/io/pdf_writer.go). It is a GitHub raw URL, so the
    gh-proxy prefix applies behind the GFW."""
    return f"{gh_proxy()}https://github.com/notofonts/noto-cjk/raw/main/Sans/Variable/TTF/NotoSansCJKsc-VF.ttf"


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


def _valid_font(path):
    """True when PATH is a TrueType font with the sfnt signature b"\\x00\\x01\\x00\\x00".

    gopdf's AddTTFFont accepts ONLY this signature; OpenType (OTTO), TrueType
    collections (ttcf), and WOFF/WOFF2 are rejected, so we must not let them
    through. A raw GitHub non-font response (e.g. a 404 HTML page) also fails this
    check, so we never copy a non-font into the dependency image."""
    if not os.path.isfile(path) or os.path.getsize(path) == 0:
        return False
    with open(path, "rb") as file:
        return file.read(4) == b"\x00\x01\x00\x00"


def _sha256_of(path):
    """SHA-256 of PATH, streamed in chunks so large files don't blow memory."""
    h = hashlib.sha256()
    with open(path, "rb") as file:
        for chunk in iter(lambda: file.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def _verify_font_checksum(font):
    """Validate FONT's SHA-256, removing it and failing on any mismatch.

    Precedence: a non-empty NOTO_CJK_FONT_SHA256 pin wins and is verified strictly.
    Otherwise the hash recorded in deps/NotoSansCJKsc-VF.ttf.sha256 is the baseline:
    a present non-empty sidecar is verified (drift is rejected), while a missing or
    empty sidecar is bootstrapped by recording the downloaded hash so subsequent runs
    detect changes. The caller runs the magic-number (_valid_font) check beforehand."""
    actual = _sha256_of(font)
    sidecar = os.path.join(SCRIPT_DIR, "NotoSansCJKsc-VF.ttf.sha256")
    if NOTO_CJK_FONT_SHA256:
        if actual != NOTO_CJK_FONT_SHA256:
            os.unlink(font)
            raise RuntimeError(f"NotoSansCJKsc-VF.ttf checksum mismatch (got {actual}, pinned {NOTO_CJK_FONT_SHA256})")
        return
    # No pinned constant: verify against the sidecar baseline, or bootstrap it.
    if os.path.isfile(sidecar):
        with open(sidecar) as file:
            content = file.read().strip()
        expected = content.split()[0] if content else ""
        if expected:
            if actual != expected:
                os.unlink(font)
                raise RuntimeError(f"NotoSansCJKsc-VF.ttf checksum changed vs recorded {expected} (got {actual})")
            return
        # Empty sidecar: fall through to (re)record the baseline below.
    with open(sidecar, "w") as file:
        file.write(actual + "\n")
    print(f"  >>> PIN THIS HASH in download_deps.py NOTO_CJK_FONT_SHA256: {actual}")


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

    The files are written into the repo's canonical model directory
    `internal/rag/res/deepdoc/` (relative to the repo root, one level up from
    this script). The Go server auto-discovers that directory, and build.sh
    --test-native defaults MODEL_DIR there too. A copy also lands under
    deps/huggingface.co/InfiniFlow/deepdoc/ to keep the dependency
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

    Written to `deps/huggingface.co/<repo>/<file>`: the path internal/tokenizer
    searches and the one deps/Dockerfile builds its image from. Fetched per file
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

    # Anchor archives, the BPE table, and tokenizer assets to deps/.
    os.chdir(SCRIPT_DIR)

    # ── cl100k_base BPE table ──────────────────────────────────────────────
    print("== cl100k_base.tiktoken ==")
    cl100k = os.path.join(SCRIPT_DIR, "cl100k_base.tiktoken")
    if os.path.isfile(cl100k) and os.path.getsize(cl100k) > 0:
        print("  ✓ cl100k_base.tiktoken already present")
    else:
        print(f"Downloading cl100k_base.tiktoken from {cl100k_url()}...")
        download_with_progress(cl100k_url(), cl100k)

    # ── Noto Sans CJK SC font (rendered into PDFs by the Go backend) ─────────
    # Verified by a font magic-number check (only the sfnt signature
    # b"\x00\x01\x00\x00" that gopdf's AddTTFFont accepts) plus a SHA-256 check
    # (_verify_font_checksum). A present file is NEVER reused unverified; a bad
    # download is removed and the build fails loudly instead of shipping a PDF
    # backend that silently loses CJK rendering.
    print("== NotoSansCJKsc-VF.ttf ==")
    font = os.path.join(SCRIPT_DIR, "NotoSansCJKsc-VF.ttf")
    if os.path.isfile(font) and os.path.getsize(font) > 0:
        if not _valid_font(font):
            print("  WARNING: existing NotoSansCJKsc-VF.ttf is not a valid font; re-downloading", file=sys.stderr)
            os.unlink(font)
        else:
            _verify_font_checksum(font)
    if not (os.path.isfile(font) and os.path.getsize(font) > 0):
        print(f"Downloading NotoSansCJKsc-VF.ttf from {noto_cjk_font_url()}...")
        download_with_progress(noto_cjk_font_url(), font)
        if not _valid_font(font):
            os.unlink(font)
            raise RuntimeError("Downloaded NotoSansCJKsc-VF.ttf is not a valid font; refusing to keep it")
        _verify_font_checksum(font)
    print("  ✓ NotoSansCJKsc-VF.ttf ready")

    # Download the Go DeepDoc `.ort` weights so this script is a one-stop for a
    # Go dev's runtime resources: models (below) from a single invocation.
    print("== DeepDoc models ==")
    download_go_models(args.china_mirrors)

    # Same one-stop idea for the token counters: their tokenizer files, in the layout
    # the loaders search and deps/Dockerfile ships.
    print("== tokenizer assets ==")
    download_tokenizer_assets(args.china_mirrors)

    print()
    print(f"✓ Runtime/image resources ready under {SCRIPT_DIR}/")
    print("  Build the dependency image with:")
    print(f"    cd {SCRIPT_DIR} && docker build -f Dockerfile -t infiniflow/ragflow_deps:latest .")


if __name__ == "__main__":
    main()
