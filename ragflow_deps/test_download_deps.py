# Tests for ragflow_deps/download_deps.py downloads and ONNX Runtime extraction.
#
# build.sh's build_go() fails fast when libonnxruntime.a is not linked, so these
# tests must guarantee the archive is really landed on disk — above all after an
# ORT version bump, where the zip's version-stamped top-level dir changes but
# static_lib/ still exists from the previous run.
#
# The release zip from infiniflow/ragflow-build carries a top-level dir named
# onnxruntime-v{version}-linux-x86_64; extract_onnxruntime() must rename it to
# the build.sh-expected onnxruntime-linux-x64-static_lib-{version}-glibc2_28 so
# every consumer shares one name convention. These tests pin that rename.

import os
import threading
import zipfile
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest
import requests

from download_deps import (
    _ort_asset_name,
    _ort_extracted_dir,
    _ort_normalized_dir,
    download_with_progress,
    extract_onnxruntime,
    has_static_archives,
)


def make_ort_zip(path, version):
    """Build a zip shaped like the infiniflow/ragflow-build ORT release: a
    top-level dir named onnxruntime-v{version}-linux-x86_64 holding
    lib/libonnxruntime.a. extract_onnxruntime() must rename it to the
    build.sh-expected onnxruntime-linux-x64-static_lib-{version}-glibc2_28."""
    member = f"{_ort_extracted_dir(version)}/lib/libonnxruntime.a"
    with zipfile.ZipFile(path, "w") as zf:
        zf.writestr(member, b"!<arch>\nort-payload")
    return path


def test_extracts_on_first_run(tmp_path):
    # static_lib/ does not exist yet: the plain first-run path.
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    version = "1.29.0"
    archive = make_ort_zip(tmp_path / _ort_asset_name(version), version)

    assert extract_onnxruntime(str(static_lib), str(archive), version) is True

    extracted = static_lib / _ort_extracted_dir(version)
    assert not extracted.exists(), "release zip top-level dir must be renamed away"
    version_dir = static_lib / _ort_normalized_dir(version)
    assert version_dir.is_dir()
    assert has_static_archives(str(version_dir))


def test_extracts_after_version_bump_when_static_lib_exists(tmp_path):
    """Regression: a static_lib/ left over from a previous version must not
    suppress extraction of the bumped version. Pruning the stale dir leaves
    static_lib/ in place; if extraction is then skipped no .a is ever landed and
    build.sh's ORT guard rejects the build."""
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    static_lib.mkdir(parents=True)
    stale = static_lib / _ort_normalized_dir("1.28.0")
    (stale / "lib").mkdir(parents=True)
    (stale / "lib" / "libonnxruntime.a").write_bytes(b"old-ort")

    version = "1.29.0"
    archive = make_ort_zip(tmp_path / _ort_asset_name(version), version)

    assert extract_onnxruntime(str(static_lib), str(archive), version) is True

    assert not stale.exists(), "stale ORT version should be pruned"
    extracted = static_lib / _ort_extracted_dir(version)
    assert not extracted.exists(), "release zip top-level dir must be renamed away"
    version_dir = static_lib / _ort_normalized_dir(version)
    assert version_dir.is_dir(), "bumped ORT version was silently not extracted"
    assert has_static_archives(str(version_dir)), "bumped ORT version landed no .a"


def test_skips_when_archive_missing(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    assert extract_onnxruntime(str(static_lib), str(tmp_path / "missing.zip"), "1.29.0") is False


def test_idempotent_when_matching_version_already_present(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    version = "1.29.0"
    archive = make_ort_zip(tmp_path / _ort_asset_name(version), version)

    assert extract_onnxruntime(str(static_lib), str(archive), version) is True
    before = sorted(os.listdir(static_lib))
    assert extract_onnxruntime(str(static_lib), str(archive), version) is True
    assert sorted(os.listdir(static_lib)) == before


def test_download_rejects_http_error_without_caching(tmp_path):
    class MissingArtifact(BaseHTTPRequestHandler):
        def do_GET(self):
            self.send_response(404)
            self.end_headers()
            self.wfile.write(b"not an archive")

        def log_message(self, *_args):
            pass

    server = HTTPServer(("127.0.0.1", 0), MissingArtifact)
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    destination = tmp_path / "archive.tar.gz"
    try:
        with pytest.raises(requests.HTTPError):
            download_with_progress(f"http://127.0.0.1:{server.server_port}/missing", destination)
    finally:
        server.shutdown()
        server.server_close()
        worker.join()

    assert not destination.exists(), "an HTTP error must not become a cached archive"


def test_download_does_not_cache_truncated_response(tmp_path):
    class TruncatedArtifact(BaseHTTPRequestHandler):
        def do_GET(self):
            self.send_response(200)
            self.send_header("Content-Length", "1024")
            self.end_headers()
            self.wfile.write(b"incomplete archive")
            self.wfile.flush()
            self.close_connection = True

        def log_message(self, *_args):
            pass

    server = HTTPServer(("127.0.0.1", 0), TruncatedArtifact)
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    destination = tmp_path / "archive.tar.gz"
    try:
        with pytest.raises(requests.RequestException):
            download_with_progress(f"http://127.0.0.1:{server.server_port}/truncated", destination)
    finally:
        server.shutdown()
        server.server_close()
        worker.join()

    assert not destination.exists(), "an interrupted transfer must not be reused"
