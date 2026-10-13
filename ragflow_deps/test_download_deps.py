# Tests for the ragflow_deps dependency downloaders:
#
#   - ragflow_deps/download_native_deps.py  (native static libraries for build.sh)
#   - ragflow_deps/download_deps.py         (runtime / image resources)
#
# build.sh's build_go() fails fast when libonnxruntime.a is not linked, so these
# tests must guarantee the archives are really landed on disk — above all after
# an ORT version bump, where the zip's version-stamped top-level dir changes but
# static_lib/ still exists from the previous run.
#
# The release zip from infiniflow/ragflow-build carries a top-level dir named
# onnxruntime-v{version}-linux-x86_64; extract_onnxruntime() must rename it to
# the build.sh-expected onnxruntime-linux-x64-static_lib-{version}-glibc2_28 so
# every consumer shares one name convention. These tests pin that rename.

import http.server
import io
import os
import runpy
import shutil
import sys
import tarfile
import threading
import types
import zipfile
from pathlib import Path

import download_deps as deps
import download_native_deps as native
import pytest
import requests
import versions
from download_native_deps import (
    _extract_native_archive,
    extract_onnxruntime,
    has_static_archives,
)


def make_ort_zip(path, version):
    """Build a zip shaped like the infiniflow/ragflow-build ORT release: a
    top-level dir named onnxruntime-v{version}-linux-x86_64 holding
    lib/libonnxruntime.a. extract_onnxruntime() must rename it to the
    build.sh-expected onnxruntime-linux-x64-static_lib-{version}-glibc2_28."""
    member = f"{versions.ort_extracted_dir(version)}/lib/libonnxruntime.a"
    with zipfile.ZipFile(path, "w") as zf:
        zf.writestr(member, b"!<arch>\nort-payload")
    return path


def test_extracts_on_first_run(tmp_path):
    # static_lib/ does not exist yet: the plain first-run path.
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    version = "1.29.0"
    archive = make_ort_zip(tmp_path / versions.ort_asset_name(version), version)

    assert extract_onnxruntime(str(static_lib), str(archive), version) is True

    extracted = static_lib / versions.ort_extracted_dir(version)
    assert not extracted.exists(), "release zip top-level dir must be renamed away"
    version_dir = static_lib / versions.ort_normalized_dir(version)
    assert version_dir.is_dir()
    assert has_static_archives(str(version_dir))


def test_extracts_after_version_bump_when_static_lib_exists(tmp_path):
    """Regression: a static_lib/ left over from a previous version must not
    suppress extraction of the bumped version. Pruning the stale dir leaves
    static_lib/ in place; if extraction is then skipped no .a is ever landed and
    build.sh's ORT guard rejects the build."""
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    static_lib.mkdir(parents=True)
    stale = static_lib / versions.ort_normalized_dir("1.28.0")
    (stale / "lib").mkdir(parents=True)
    (stale / "lib" / "libonnxruntime.a").write_bytes(b"old-ort")

    version = "1.29.0"
    archive = make_ort_zip(tmp_path / versions.ort_asset_name(version), version)

    assert extract_onnxruntime(str(static_lib), str(archive), version) is True

    assert not stale.exists(), "stale ORT version should be pruned"
    extracted = static_lib / versions.ort_extracted_dir(version)
    assert not extracted.exists(), "release zip top-level dir must be renamed away"
    version_dir = static_lib / versions.ort_normalized_dir(version)
    assert version_dir.is_dir(), "bumped ORT version was silently not extracted"
    assert has_static_archives(str(version_dir)), "bumped ORT version landed no .a"


def test_skips_when_archive_missing(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    assert extract_onnxruntime(str(static_lib), str(tmp_path / "missing.zip"), "1.29.0") is False


def test_idempotent_when_matching_version_already_present(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    version = "1.29.0"
    archive = make_ort_zip(tmp_path / versions.ort_asset_name(version), version)

    assert extract_onnxruntime(str(static_lib), str(archive), version) is True
    before = sorted(os.listdir(static_lib))
    assert extract_onnxruntime(str(static_lib), str(archive), version) is True
    assert sorted(os.listdir(static_lib)) == before


@pytest.fixture
def download_server():
    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path == "/missing":
                self.send_response(404)
                self.end_headers()
                self.wfile.write(b"not an artifact")
            else:
                self.send_response(200)
                self.send_header("Content-Length", "100" if self.path == "/interrupted" else "7")
                self.end_headers()
                self.wfile.write(b"payload")

        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        thread.join()
        server.server_close()


@pytest.mark.parametrize("downloader", [deps, native], ids=["runtime", "native"])
@pytest.mark.parametrize("endpoint", ["missing", "interrupted"])
def test_failed_download_preserves_previous_file(tmp_path, download_server, downloader, endpoint):
    dest = tmp_path / "artifact"
    dest.write_bytes(b"previous valid artifact")
    with pytest.raises(requests.RequestException):
        downloader.download_with_progress(f"{download_server}/{endpoint}", dest)
    assert dest.read_bytes() == b"previous valid artifact"
    assert list(tmp_path.iterdir()) == [dest]


@pytest.mark.parametrize("downloader", [deps, native], ids=["runtime", "native"])
def test_successful_download_replaces_previous_file(tmp_path, download_server, downloader):
    dest = tmp_path / "artifact"
    dest.write_bytes(b"old")
    downloader.download_with_progress(f"{download_server}/complete", dest)
    assert dest.read_bytes() == b"payload"
    assert list(tmp_path.iterdir()) == [dest]


def _prepare_context(tmp_path, scripts, extra=()):
    """Stage a repo-shaped context: the downloader scripts plus go.mod, so
    runpy exercises the real files against fixture versions."""
    context = tmp_path / "ragflow_deps"
    context.mkdir()
    root = Path(__file__).resolve().parents[1]
    for name in scripts:
        shutil.copy(root / "ragflow_deps" / name, context / name)
    for name in extra:
        shutil.copy(root / "ragflow_deps" / name, context / name)
    (tmp_path / "go.mod").write_text(
        """module fixture

require (
    github.com/yfedoseev/office_oxide/go v0.1.12
    github.com/browserbase/stagehand-go/v3 v3.22.0
)
"""
    )
    return context


def test_native_dependency_preparation(tmp_path, monkeypatch):
    context = _prepare_context(tmp_path, ["download_native_deps.py", "versions.py"])
    script = context / "download_native_deps.py"
    required = {
        "pdfium-linux-x64-static.tgz": ("pdfium-static", ["lib/libpdfium.a", "lib/libc++.a", "lib/libc++abi.a", "include/fpdfview.h"]),
        "pdf_oxide-go-ffi-linux-amd64.tar.gz": ("pdf_oxide", ["lib/linux_amd64/libpdf_oxide.a", "include/pdf_oxide.h"]),
        "office_oxide-linux-x86_64.tar.gz": ("office_oxide", ["lib/liboffice_oxide.a", "include/office_oxide_c/office_oxide.h"]),
    }
    for archive_name, (directory, files) in required.items():
        with tarfile.open(context / archive_name, "w:gz") as archive:
            for name in files:
                item = tarfile.TarInfo("./" + name)
                payload = b"payload\x000.1.12\x00" if name.endswith("liboffice_oxide.a") else b"payload"
                item.size = len(payload)
                archive.addfile(item, io.BytesIO(payload))
    # A stale office_oxide tree (old version stamp) must be replaced, not kept.
    stale = tmp_path / "native" / "office_oxide"
    (stale / "lib").mkdir(parents=True)
    (stale / "include" / "office_oxide_c").mkdir(parents=True)
    (stale / "lib" / "liboffice_oxide.a").write_bytes(b"old\x000.1.9\x00")
    (stale / "include" / "office_oxide_c" / "office_oxide.h").write_bytes(b"old")
    # A stale ORT version dir from a previous bump must be pruned.
    (tmp_path / "native" / "onnxruntime" / "static_lib" / versions.ort_normalized_dir("1.28.0") / "lib").mkdir(parents=True)
    make_ort_zip(context / versions.ort_asset_name("1.29.0"), "1.29.0")

    def offline(*args, **kwargs):
        raise requests.ConnectionError("offline sidecar")

    monkeypatch.setattr(requests, "get", offline)
    original_expanduser = os.path.expanduser
    monkeypatch.setattr(os.path, "expanduser", lambda path: str(tmp_path / "native") if path == "~/ragflow-native-libs" else original_expanduser(path))
    monkeypatch.syspath_prepend(str(context))
    monkeypatch.setattr(sys, "argv", [str(script)])
    monkeypatch.chdir(tmp_path)
    runpy.run_path(str(script), run_name="__main__")

    for directory, files in required.values():
        for name in files:
            expected = b"payload\x000.1.12\x00" if name.endswith("liboffice_oxide.a") else b"payload"
            assert (tmp_path / "native" / directory / name).read_bytes() == expected
    assert not (tmp_path / "native" / "onnxruntime" / "static_lib" / versions.ort_normalized_dir("1.28.0")).exists()
    ort_version_dir = tmp_path / "native" / "onnxruntime" / "static_lib" / versions.ort_normalized_dir("1.29.0")
    assert has_static_archives(str(ort_version_dir))


def test_runtime_dependency_preparation(tmp_path, monkeypatch):
    context = _prepare_context(tmp_path, ["download_deps.py", "versions.py"], extra=["tokenizer_assets.txt"])
    script = context / "download_deps.py"
    (context / "cl100k_base.tiktoken").write_bytes(b"configured table")
    for arch in ["x64", "arm64"]:
        (context / f"stagehand-server-v3-linux-{arch}").write_bytes(b"\x7fELFpayload")

    def offline(*args, **kwargs):
        raise requests.ConnectionError("offline")

    def hf_download(*, repo_id, filename, local_dir):
        Path(local_dir).mkdir(parents=True, exist_ok=True)
        (Path(local_dir) / filename).write_bytes(b"model")

    monkeypatch.setitem(sys.modules, "huggingface_hub", types.SimpleNamespace(hf_hub_download=hf_download))
    monkeypatch.setattr(requests, "get", offline)
    monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
    monkeypatch.syspath_prepend(str(context))
    monkeypatch.setattr(sys, "argv", [str(script)])
    monkeypatch.chdir(tmp_path)
    runpy.run_path(str(script), run_name="__main__")

    for filename in deps.DEEPDOC_MODEL_FILES:
        assert (tmp_path / "internal" / "rag" / "res" / "deepdoc" / filename).read_bytes() == b"model"
        assert (context / "huggingface.co" / deps.DEEPDOC_REPO / filename).read_bytes() == b"model"
    for repo_id, filename, kind in deps.load_tokenizer_assets(context / "tokenizer_assets.txt"):
        assert (context / "huggingface.co" / repo_id / filename).read_bytes() == b"model"
    stagehand = tmp_path / "cache" / "stagehand" / "lib" / "go_3.22.0" / "stagehand-server-v3-linux-x64"
    assert stagehand.read_bytes() == b"\x7fELFpayload"
    assert os.access(stagehand, os.X_OK)


def test_incomplete_native_archive_preserves_existing_directory(tmp_path):
    target = tmp_path / "native"
    target.mkdir()
    (target / "existing").write_bytes(b"keep")
    archive = tmp_path / "incomplete.tar.gz"
    with tarfile.open(archive, "w:gz"):
        pass
    with pytest.raises(RuntimeError, match="missing required native file"):
        _extract_native_archive(archive, target, ["lib/library.a"])
    assert (target / "existing").read_bytes() == b"keep"


@pytest.mark.parametrize("kind", ["traversal", "absolute", "symlink", "hardlink", "fifo", "device"])
def test_native_archive_rejects_unsafe_members(tmp_path, kind):
    target = tmp_path / "native"
    target.mkdir()
    (target / "existing").write_bytes(b"keep")
    outside = tmp_path / "outside"
    outside.write_bytes(b"untouched")
    archive_path = tmp_path / "unsafe.tar.gz"
    with tarfile.open(archive_path, "w:gz") as archive:
        library = tarfile.TarInfo("./lib/library.a")
        library.size = len(b"payload")
        archive.addfile(library, io.BytesIO(b"payload"))
        member = tarfile.TarInfo("unexpected")
        if kind == "traversal":
            member.name = "../outside"
        elif kind == "absolute":
            member.name = str(outside)
        else:
            member.type = {"symlink": tarfile.SYMTYPE, "hardlink": tarfile.LNKTYPE, "fifo": tarfile.FIFOTYPE, "device": tarfile.CHRTYPE}[kind]
            member.linkname = str(outside)
        payload = b"unsafe" if member.isfile() else b""
        member.size = len(payload)
        archive.addfile(member, io.BytesIO(payload))

    with pytest.raises(tarfile.TarError, match="Unsafe native archive member"):
        _extract_native_archive(archive_path, str(target), ["lib/library.a"])

    assert outside.read_bytes() == b"untouched"
    assert (target / "existing").read_bytes() == b"keep"
    assert not (target / "lib/library.a").exists()
    assert sorted(path.name for path in tmp_path.iterdir()) == ["native", "outside", "unsafe.tar.gz"]


def test_tokenizer_asset_manifest_is_parsed(tmp_path):
    manifest = tmp_path / "tokenizer_assets.txt"
    manifest.write_text(
        """# comment line

BAAI/bge-m3 sentencepiece.bpe.model runtime
BAAI/bge-m3 tokenizer.json oracle
"""
    )
    assert deps.load_tokenizer_assets(manifest) == [
        ("BAAI/bge-m3", "sentencepiece.bpe.model", "runtime"),
        ("BAAI/bge-m3", "tokenizer.json", "oracle"),
    ]


@pytest.mark.parametrize(
    "line",
    [
        "BAAI/bge-m3 sentencepiece.bpe.model",
        "bge-m3 sentencepiece.bpe.model runtime",
    ],
)
def test_tokenizer_asset_manifest_rejects_malformed_entries(tmp_path, line):
    manifest = tmp_path / "tokenizer_assets.txt"
    manifest.write_text(f"{line}\n")
    with pytest.raises(RuntimeError, match="malformed"):
        deps.load_tokenizer_assets(manifest)


def test_manifest_rejects_unknown_kind(tmp_path):
    manifest = tmp_path / "tokenizer_assets.txt"
    manifest.write_text("BAAI/bge-m3 sentencepiece.bpe.model snapshot\n")
    with pytest.raises(RuntimeError, match="unknown kind"):
        deps.load_tokenizer_assets(manifest)


def test_url_builders_honour_mirror_env(monkeypatch):
    monkeypatch.delenv("HF_ENDPOINT", raising=False)
    monkeypatch.delenv("GH_PROXY", raising=False)
    assert versions.hf_endpoint() == "https://huggingface.co"
    assert versions.gh_proxy() == ""
    monkeypatch.setenv("HF_ENDPOINT", "https://hf-mirror.com")
    monkeypatch.setenv("GH_PROXY", "https://gh-proxy.com/")
    assert versions.hf_url("BAAI/bge-m3", "vocab.txt") == "https://hf-mirror.com/BAAI/bge-m3/resolve/main/vocab.txt"
    assert versions.stagehand_url("x64").startswith("https://gh-proxy.com/https://github.com/browserbase/stagehand/")
    assert versions.pdfium_url().startswith("https://gh-proxy.com/https://github.com/kognitos/pdfium-static/")
    assert versions.pdf_oxide_url().endswith("/v0.3.73/pdf_oxide-go-ffi-linux-amd64.tar.gz")
    assert versions.cl100k_url() == "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"


def test_go_module_version_reads_go_mod(tmp_path):
    (tmp_path / "go.mod").write_text("module fixture\n\ngithub.com/yfedoseev/office_oxide/go v0.1.12\n")
    assert versions.go_module_version("github.com/yfedoseev/office_oxide/go", str(tmp_path)) == "0.1.12"
    with pytest.raises(RuntimeError, match="not found"):
        versions.go_module_version("github.com/yfedoseev/office_oxide/go", str(tmp_path / "missing"))
