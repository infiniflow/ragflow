# Tests for the local-dev dependency downloader (tools/scripts/download_dev_deps.py):
# the native static libraries (pdfium / pdf_oxide / office_oxide / onnxruntime)
# the Go build links, PLUS the runtime resources (cl100k / DeepDoc .ort / embedding
# tokenizer assets) the local Go server needs to run, parse and count tokens.
#
# build.sh's build_go() fails fast when libonnxruntime.a is not linked, so these
# tests must guarantee the archives are really landed on disk — above all after
# an ORT version bump, where the zip's version-stamped top-level dir changes but
# static_lib/ still exists from the previous run.
#
# The release zip from infiniflow/ragflow-build carries a top-level dir named
# onnxruntime-v{version}-linux-x86_64; extract_onnxruntime() must rename it to the
# build.sh-expected onnxruntime-linux-x64-static_lib-{version}-glibc2_28 so every
# consumer shares one name convention. These tests pin that rename, the stale-dir
# pruning, the archive member sanitization, and the atomic download behaviour.

import importlib.util
import io
import os
import tarfile
import threading
import zipfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest import mock

import download_dev_deps as native
import pytest
import requests


def make_ort_zip(path, version):
    """Build a zip shaped like the infiniflow/ragflow-build ORT release: a
    top-level dir named onnxruntime-v{version}-linux-x86_64 holding
    lib/libonnxruntime.a. extract_onnxruntime() must rename it to the
    build.sh-expected onnxruntime-linux-x64-static_lib-{version}-glibc2_28."""
    member = f"{native.ort_extracted_dir(version)}/lib/libonnxruntime.a"
    with zipfile.ZipFile(path, "w") as zf:
        zf.writestr(member, b"!<arch>\nort-payload")
    return path


def test_extracts_on_first_run(tmp_path):
    # static_lib/ does not exist yet: the plain first-run path.
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    version = "1.29.0"
    archive = make_ort_zip(tmp_path / native.ort_asset_name(version), version)

    assert native.extract_onnxruntime(str(static_lib), str(archive), version) is True

    extracted = static_lib / native.ort_extracted_dir(version)
    assert not extracted.exists(), "release zip top-level dir must be renamed away"
    version_dir = static_lib / native.ort_normalized_dir(version)
    assert version_dir.is_dir()
    assert native.has_static_archives(str(version_dir))


def test_extracts_after_version_bump_when_static_lib_exists(tmp_path):
    """Regression: a static_lib/ left over from a previous version must not
    suppress extraction of the bumped version. Pruning the stale dir leaves
    static_lib/ in place; if extraction is then skipped no .a is ever landed and
    build.sh's ORT guard rejects the build."""
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    static_lib.mkdir(parents=True)
    stale = static_lib / native.ort_normalized_dir("1.28.0")
    (stale / "lib").mkdir(parents=True)
    (stale / "lib" / "libonnxruntime.a").write_bytes(b"old-ort")

    version = "1.29.0"
    archive = make_ort_zip(tmp_path / native.ort_asset_name(version), version)

    assert native.extract_onnxruntime(str(static_lib), str(archive), version) is True

    assert not stale.exists(), "stale ORT version should be pruned"
    extracted = static_lib / native.ort_extracted_dir(version)
    assert not extracted.exists(), "release zip top-level dir must be renamed away"
    version_dir = static_lib / native.ort_normalized_dir(version)
    assert version_dir.is_dir(), "bumped ORT version was silently not extracted"
    assert native.has_static_archives(str(version_dir)), "bumped ORT version landed no .a"


def test_skips_when_archive_missing(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    assert native.extract_onnxruntime(str(static_lib), str(tmp_path / "missing.zip"), "1.29.0") is False


def test_idempotent_when_matching_version_already_present(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    version = "1.29.0"
    archive = make_ort_zip(tmp_path / native.ort_asset_name(version), version)

    assert native.extract_onnxruntime(str(static_lib), str(archive), version) is True
    before = sorted(os.listdir(static_lib))
    assert native.extract_onnxruntime(str(static_lib), str(archive), version) is True
    assert sorted(os.listdir(static_lib)) == before


@pytest.fixture
def download_server():
    class Handler(BaseHTTPRequestHandler):
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

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        thread.join()
        server.server_close()


@pytest.mark.parametrize("endpoint", ["missing", "interrupted"])
def test_failed_download_preserves_previous_file(tmp_path, download_server, endpoint):
    dest = tmp_path / "artifact"
    dest.write_bytes(b"previous valid artifact")
    with pytest.raises(requests.RequestException):
        native.download_with_progress(f"{download_server}/{endpoint}", dest)
    assert dest.read_bytes() == b"previous valid artifact"
    assert list(tmp_path.iterdir()) == [dest]


def test_successful_download_replaces_previous_file(tmp_path, download_server):
    dest = tmp_path / "artifact"
    dest.write_bytes(b"old")
    native.download_with_progress(f"{download_server}/complete", dest)
    assert dest.read_bytes() == b"payload"
    assert list(tmp_path.iterdir()) == [dest]


def test_incomplete_native_archive_preserves_existing_directory(tmp_path):
    target = tmp_path / "native"
    target.mkdir()
    (target / "existing").write_bytes(b"keep")
    archive = tmp_path / "incomplete.tar.gz"
    with tarfile.open(archive, "w:gz"):
        pass
    with pytest.raises(RuntimeError, match="missing required native file"):
        native._extract_native_archive(archive, target, ["lib/library.a"])
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
        native._extract_native_archive(archive_path, str(target), ["lib/library.a"])

    assert outside.read_bytes() == b"untouched"
    assert (target / "existing").read_bytes() == b"keep"
    assert not (target / "lib/library.a").exists()
    assert sorted(path.name for path in tmp_path.iterdir()) == ["native", "outside", "unsafe.tar.gz"]


# ── Local-dev one-stop: the dev downloader must also fetch the runtime
# resources (cl100k / DeepDoc .ort / tokenizer assets) so a developer runs THIS
# script ONCE and the Go server is ready to build, run, parse and count — but it
# must NOT fetch stagehand (that is the image script's job). ──────────────────


@pytest.fixture
def native_dirs(tmp_path, monkeypatch):
    """Point the dev script's repo-anchored paths at a temp tree so the
    runtime-resource downloads written by tests never touch the real repo."""
    deps = tmp_path / "ragflow_deps"
    deps.mkdir()
    root = tmp_path / "repo"
    root.mkdir()
    monkeypatch.setattr(native, "RAGFLOW_DEPS_DIR", str(deps))
    monkeypatch.setattr(native, "REPO_ROOT", str(root))
    return deps, root


def _fake_hf(repo_id, filename, local_dir):
    """Stand-in for huggingface_hub.hf_hub_download that writes a real file so
    downstream copy/validate steps behave like the real downloader."""
    os.makedirs(local_dir, exist_ok=True)
    with open(os.path.join(local_dir, filename), "wb") as f:
        f.write(b"hf-payload")


def test_download_cl100k_skips_when_present(native_dirs, download_server, monkeypatch):
    deps, _ = native_dirs
    existing = deps / "cl100k_base.tiktoken"
    existing.write_bytes(b"previous cl100k")
    monkeypatch.setattr(native, "cl100k_url", lambda: f"{download_server}/complete")
    native.download_cl100k()
    assert existing.read_bytes() == b"previous cl100k"


def test_download_cl100k_downloads_when_missing(native_dirs, download_server, monkeypatch):
    deps, _ = native_dirs
    monkeypatch.setattr(native, "cl100k_url", lambda: f"{download_server}/complete")
    native.download_cl100k()
    assert (deps / "cl100k_base.tiktoken").read_bytes() == b"payload"


def test_download_go_models_skips_when_present(native_dirs, monkeypatch):
    _, root = native_dirs
    deepdoc_dir = root / "internal" / "rag" / "res" / "deepdoc"
    deepdoc_dir.mkdir(parents=True)
    for fname in native.HF_DEEPDOC_MODEL_FILES:
        (deepdoc_dir / fname).write_bytes(b"already here")
    fake = mock.MagicMock(side_effect=_fake_hf)
    monkeypatch.setattr(native, "_hf_download", fake)
    native.download_go_models()
    # Already-present files means hf_hub_download is never called.
    assert fake.call_count == 0


def test_download_go_models_downloads_when_missing(native_dirs, monkeypatch):
    deps, root = native_dirs
    monkeypatch.setattr(native, "_hf_download", _fake_hf)
    native.download_go_models()
    deepdoc_dir = root / "internal" / "rag" / "res" / "deepdoc"
    for fname in native.HF_DEEPDOC_MODEL_FILES:
        assert (deepdoc_dir / fname).is_file(), f"missing DeepDoc model {fname}"
    # The image-layout copy under ragflow_deps/huggingface.co must also exist.
    for fname in native.HF_DEEPDOC_MODEL_FILES:
        assert (deps / "huggingface.co" / native.HF_DEEPDOC_REPO / fname).is_file()


def test_download_go_models_fails_loud(native_dirs, monkeypatch):
    monkeypatch.setattr(
        native,
        "_hf_download",
        lambda repo_id, filename, local_dir: (_ for _ in ()).throw(RuntimeError("hf down")),
    )
    with pytest.raises(SystemExit) as exc:
        native.download_go_models()
    assert exc.value.code == 1


def _seed_tokenizer_targets(deps, present):
    """Create (present=True) or leave absent the on-disk targets the inlined
    TOKENIZER_ASSETS list resolves to under ragflow_deps/huggingface.co/<repo>/<file>."""
    for repo, filename, _kind in native.TOKENIZER_ASSETS:
        target = deps / "huggingface.co" / repo / filename
        if present:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(b"already here")


def test_download_tokenizer_assets_skips_when_present(native_dirs, monkeypatch):
    deps, _ = native_dirs
    _seed_tokenizer_targets(deps, present=True)
    fake = mock.MagicMock(side_effect=_fake_hf)
    monkeypatch.setattr(native, "_hf_download", fake)
    native.download_tokenizer_assets()
    assert fake.call_count == 0


def test_download_tokenizer_assets_downloads_when_missing(native_dirs, monkeypatch):
    deps, _ = native_dirs
    monkeypatch.setattr(native, "_hf_download", _fake_hf)
    native.download_tokenizer_assets()
    for repo, filename, _kind in native.TOKENIZER_ASSETS:
        assert (deps / "huggingface.co" / repo / filename).is_file(), f"missing {repo}/{filename}"


def test_download_tokenizer_assets_fails_loud(native_dirs, monkeypatch):
    monkeypatch.setattr(
        native,
        "_hf_download",
        lambda repo_id, filename, local_dir: (_ for _ in ()).throw(RuntimeError("hf down")),
    )
    with pytest.raises(SystemExit) as exc:
        native.download_tokenizer_assets()
    assert exc.value.code == 1


def test_download_tokenizer_assets_fails_loud_on_bad_list(native_dirs, monkeypatch):
    # load_tokenizer_assets raises on a malformed inlined entry (bad kind);
    # download_tokenizer_assets must surface it as SystemExit 1 rather than
    # leaking a raw traceback or handing a broken tree to the image build.
    monkeypatch.setattr(
        native,
        "load_tokenizer_assets",
        lambda: (_ for _ in ()).throw(RuntimeError("unknown tokenizer asset kind 'bogus'")),
    )
    with pytest.raises(SystemExit) as exc:
        native.download_tokenizer_assets()
    assert exc.value.code == 1


def test_load_tokenizer_assets_returns_inlined_list():
    # The asset list is inlined (no separate manifest file); it must match the
    # six entries the runtime images ship and the Go loader pins.
    assert native.load_tokenizer_assets() == [
        ("BAAI/bge-m3", "sentencepiece.bpe.model", "runtime"),
        ("BAAI/bge-large-en-v1.5", "vocab.txt", "runtime"),
        ("Qwen/Qwen3-Embedding-0.6B", "tokenizer.json", "runtime"),
        ("intfloat/e5-mistral-7b-instruct", "tokenizer.json", "runtime"),
        ("BAAI/bge-m3", "tokenizer.json", "oracle"),
        ("BAAI/bge-large-en-v1.5", "tokenizer.json", "oracle"),
    ]


def test_load_tokenizer_assets_rejects_bad_kind():
    with pytest.raises(RuntimeError, match="kind"):
        native.load_tokenizer_assets([("X/y.bin", "f.bin", "bogus")])


def test_load_tokenizer_assets_rejects_empty():
    with pytest.raises(RuntimeError, match="no tokenizer assets"):
        native.load_tokenizer_assets([])


def test_load_tokenizer_assets_rejects_malformed_repo():
    with pytest.raises(RuntimeError, match="malformed"):
        native.load_tokenizer_assets([("nobadrepo", "f.bin", "runtime")])


def test_native_script_does_not_fetch_stagehand():
    # Partition contract: the local-dev one-stop must not pull the Browser
    # component's stagehand binary — that is download_deps.py's job for images.
    assert not hasattr(native, "stagehand_url")
    assert not hasattr(native, "_install_stagehand")
    assert "stagehand" not in native.main.__doc__.lower()


# ── Image script (download_deps.py) must stay runtime-resources only: it must
# NOT handle the native .a archives (the build stage bakes those into the
# binary). Imported by path inside the test so the import lives next to the
# native script and never touches module import timing. ──────────────────────


def test_image_script_does_not_handle_native_archives():
    deps_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "ragflow_deps", "download_deps.py")
    assert os.path.isfile(deps_path), f"download_deps.py not found at {deps_path}"
    spec = importlib.util.spec_from_file_location("download_deps_img", deps_path)
    img = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(img)
    assert not hasattr(img, "extract_onnxruntime")
    assert not hasattr(img, "_extract_native_archive")
    assert "onnxruntime" not in (img.__doc__ or "").lower()
