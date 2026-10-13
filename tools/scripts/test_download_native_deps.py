# Tests for the native static-library downloader (tools/scripts/download_native_deps.py):
# the pdfium / pdf_oxide / office_oxide / onnxruntime archives the Go build links.
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

import io
import os
import tarfile
import threading
import zipfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import download_native_deps as native
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
