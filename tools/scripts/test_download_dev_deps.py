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
# The release zip from infiniflow/ragflow-build wraps everything in a top-level
# dir named onnxruntime-v{version}-{target}; extract_onnxruntime() must rename
# it to the build.sh-expected onnxruntime-<os>-<arch>-static_lib-{version}... so
# every consumer shares one name convention. These tests pin that rename, the
# stale-dir pruning, the archive member sanitization, and the atomic download
# behaviour.
#
# extract_onnxruntime() takes the *expected dir name* rather than just the
# version string, because that name is platform-dependent (linux vs osx, amd64
# vs arm64) and build.sh's stale-version guard keys on it.

import importlib.util
import io
import os
import subprocess
import tarfile
import threading
import zipfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest import mock

import download_dev_deps as native
import pytest
import requests


def make_ort_zip(path, dir_name):
    """Build a zip shaped like the infiniflow/ragflow-build ORT release: a
    top-level dir holding lib/libonnxruntime.a. extract_onnxruntime() must
    rename it to the build.sh-expected dir (see ort_normalized_dir)."""
    with zipfile.ZipFile(path, "w") as zf:
        zf.writestr(f"{dir_name}/lib/libonnxruntime.a", b"!<arch>\nort-payload")
    return path


def ort_archive(tmp_path, goos="linux", goarch="amd64", version="1.29.0"):
    """Return (zip_path, normalized_dir) for `goos/goarch` at `version`."""
    target = native.ort_target(goos, goarch)
    zip_path = tmp_path / native.ort_asset_name(version, target)
    return make_ort_zip(zip_path, native.ort_extracted_dir(version, target)), native.ort_normalized_dir(version, goos, goarch)


def test_extracts_on_first_run(tmp_path):
    # static_lib/ does not exist yet: the plain first-run path.
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    archive, dir_name = ort_archive(tmp_path)

    assert native.extract_onnxruntime(str(static_lib), str(archive), dir_name) is True

    extracted = static_lib / native.ort_extracted_dir("1.29.0", native.ort_target("linux", "amd64"))
    assert not extracted.exists(), "release zip top-level dir must be renamed away"
    version_dir = static_lib / dir_name
    assert version_dir.is_dir()
    assert native.has_static_archives(str(version_dir))


def test_extracts_after_version_bump_when_static_lib_exists(tmp_path):
    """Regression: a static_lib/ left over from a previous version must not
    suppress extraction of the bumped version. Pruning the stale dir leaves
    static_lib/ in place; if extraction is then skipped no .a is ever landed and
    build.sh's ORT guard rejects the build."""
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    static_lib.mkdir(parents=True)
    stale = static_lib / native.ort_normalized_dir("1.28.0", "linux", "amd64")
    (stale / "lib").mkdir(parents=True)
    (stale / "lib" / "libonnxruntime.a").write_bytes(b"old-ort")

    archive, dir_name = ort_archive(tmp_path)

    assert native.extract_onnxruntime(str(static_lib), str(archive), dir_name) is True

    assert not stale.exists(), "stale ORT version should be pruned"
    extracted = static_lib / native.ort_extracted_dir("1.29.0", native.ort_target("linux", "amd64"))
    assert not extracted.exists(), "release zip top-level dir must be renamed away"
    version_dir = static_lib / dir_name
    assert version_dir.is_dir(), "bumped ORT version was silently not extracted"
    assert native.has_static_archives(str(version_dir)), "bumped ORT version landed no .a"


def test_skips_when_archive_missing(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    _, dir_name = ort_archive(tmp_path)
    assert native.extract_onnxruntime(str(static_lib), str(tmp_path / "missing.zip"), dir_name) is False


def test_idempotent_when_matching_version_already_present(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    archive, dir_name = ort_archive(tmp_path)

    assert native.extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    before = sorted(os.listdir(static_lib))
    # Same dir already present: should short-circuit, not re-extract.
    assert native.extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    assert sorted(os.listdir(static_lib)) == before


def test_prunes_foreign_platform_dir(tmp_path):
    """A dir from a different platform/version must be pruned so build.sh's
    `find ... -name '*.a'` does not link two ORT builds."""
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    static_lib.mkdir(parents=True)
    foreign = static_lib / native.ort_normalized_dir("1.29.0", "darwin", "arm64")
    (foreign / "lib").mkdir(parents=True)
    (foreign / "lib" / "libonnxruntime.a").write_bytes(b"mac-ort")

    archive, dir_name = ort_archive(tmp_path)

    assert native.extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    assert not foreign.exists(), "foreign-platform ORT dir should be pruned"
    assert (static_lib / dir_name).is_dir()


def test_ort_asset_names_match_upstream_layout():
    # Spot-check the platform-specific asset/dir names so the download script and
    # build.sh's stale-version guard stay aligned with the upstream release.
    assert native.ort_asset_name("1.29.0", "linux-x86_64") == "onnxruntime-v1.29.0-linux-x86_64.zip"
    assert native.ort_asset_name("1.29.0", "linux-aarch64") == "onnxruntime-v1.29.0-linux-aarch64.zip"
    assert native.ort_asset_name("1.29.0", "osx-arm64") == "onnxruntime-v1.29.0-osx-arm64.zip"
    # No osx-x86_64 asset is published; Intel Macs use the universal archive.
    assert native.ort_asset_name("1.29.0", "osx-universal") == "onnxruntime-v1.29.0-osx-universal.zip"
    assert native.ort_normalized_dir("1.29.0", "linux", "amd64") == "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28"
    assert native.ort_normalized_dir("1.29.0", "linux", "arm64") == "onnxruntime-linux-aarch64-static_lib-1.29.0-glibc2_28"
    assert native.ort_normalized_dir("1.29.0", "darwin", "arm64") == "onnxruntime-osx-arm64-static_lib-1.29.0"
    assert native.ort_normalized_dir("1.29.0", "darwin", "amd64") == "onnxruntime-osx-universal-static_lib-1.29.0"
    assert native.ort_target("linux", "amd64") == "linux-x86_64"
    assert native.ort_target("linux", "arm64") == "linux-aarch64"
    assert native.ort_target("darwin", "arm64") == "osx-arm64"
    assert native.ort_target("darwin", "amd64") == "osx-universal"


def test_extracts_darwin_archive(tmp_path):
    # The macOS zip carries no glibc suffix in its normalized dir name; the
    # rename step must still land it where build.sh expects.
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    archive, dir_name = ort_archive(tmp_path, goos="darwin", goarch="arm64")

    assert native.extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    assert (static_lib / dir_name / "lib" / "libonnxruntime.a").is_file()


def test_prunes_foreign_platform_dir_on_short_circuit(tmp_path):
    """When the expected ORT dir is already extracted, the short-circuit return
    must still prune a co-resident foreign-platform dir (defense-in-depth;
    build.sh's ort_dir_prefix guard also filters by platform at link time)."""
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    dir_name = native.ort_normalized_dir("1.29.0", "linux", "amd64")
    expected = static_lib / dir_name
    (expected / "lib").mkdir(parents=True)
    (expected / "lib" / "libonnxruntime.a").write_bytes(b"linux-ort")
    foreign = static_lib / native.ort_normalized_dir("1.29.0", "darwin", "arm64")
    (foreign / "lib").mkdir(parents=True)
    (foreign / "lib" / "libonnxruntime.a").write_bytes(b"mac-ort")

    archive, _ = ort_archive(tmp_path)
    assert native.extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    assert not foreign.exists(), "foreign dir must be pruned even on short-circuit"
    assert expected.is_dir()


def test_host_platform_normalizes_aliases(monkeypatch):
    # build.sh's detect_target_platform accepts Linux/Darwin + x86_64/amd64
    # aliases; host_platform must normalize the same way or it KeyErrors when
    # indexing the per-platform asset maps.
    monkeypatch.setenv("RAGFLOW_TARGET_OS", "Linux")
    monkeypatch.setenv("RAGFLOW_TARGET_ARCH", "x86_64")
    assert native.host_platform() == ("linux", "amd64")
    monkeypatch.setenv("RAGFLOW_TARGET_OS", "Darwin")
    monkeypatch.setenv("RAGFLOW_TARGET_ARCH", "aarch64")
    assert native.host_platform() == ("darwin", "arm64")


def test_host_platform_normalizes_explicit_arguments():
    # The --target-os / --target-arch flags must go through the same
    # normalization as the environment variables, or an alias (e.g.
    # --target-os Linux) KeyErrors on the per-platform asset maps.
    assert native.host_platform("Linux", "x86_64") == ("linux", "amd64")
    assert native.host_platform("Darwin", "aarch64") == ("darwin", "arm64")


def test_host_platform_rejects_unsupported(monkeypatch):
    monkeypatch.setenv("RAGFLOW_TARGET_OS", "windows")
    with pytest.raises(SystemExit):
        native.host_platform()
    monkeypatch.setenv("RAGFLOW_TARGET_OS", "linux")
    monkeypatch.setenv("RAGFLOW_TARGET_ARCH", "riscv64")
    with pytest.raises(SystemExit):
        native.host_platform()


def test_url_builders_select_platform_assets(monkeypatch):
    monkeypatch.delenv("GH_PROXY", raising=False)
    assert native.pdfium_url("linux", "amd64").endswith("/pdfium-linux-x64-static.tgz")
    assert native.pdfium_url("linux", "arm64").endswith("/pdfium-linux-arm64-static.tgz")
    assert native.pdfium_url("darwin", "arm64").endswith("/pdfium-mac-arm64-static.tgz")
    assert native.pdf_oxide_url(native.REPO_ROOT, "linux", "amd64").endswith("/pdf_oxide-go-ffi-linux-amd64.tar.gz")
    assert native.pdf_oxide_url(native.REPO_ROOT, "darwin", "amd64").endswith("/pdf_oxide-go-ffi-darwin-amd64.tar.gz")
    assert native.ort_url("1.29.0", native.ort_target("darwin", "amd64")).endswith("/onnxruntime-v1.29.0-osx-universal.zip")


def test_office_oxide_url_selects_platform_asset(tmp_path):
    (tmp_path / "go.mod").write_text("module fixture\n\nrequire github.com/yfedoseev/office_oxide/go v0.1.12\n")
    assert native.office_oxide_url(str(tmp_path), "linux", "amd64").endswith("/v0.1.12/native-linux-x86_64.tar.gz")
    assert native.office_oxide_url(str(tmp_path), "darwin", "arm64").endswith("/v0.1.12/native-macos-aarch64.tar.gz")


def test_pdf_oxide_url_derives_from_go_mod(tmp_path):
    # pdf_oxide version MUST come from go.mod (the single source of truth
    # build.sh also reads), never a hardcoded constant. Otherwise build.sh and
    # the downloader silently drift apart. A fixture go.mod pins a non-default
    # version; the produced URL must reflect exactly that version.
    (tmp_path / "go.mod").write_text("module fixture\n\nrequire github.com/yfedoseev/pdf_oxide/go v0.4.1\n")
    url = native.pdf_oxide_url(str(tmp_path), "linux", "amd64")
    assert "/v0.4.1/pdf_oxide-go-ffi-linux-amd64.tar.gz" in url


def test_pdf_oxide_url_reads_its_own_module_line(tmp_path):
    # Adversarial: a go.mod that pins office_oxide and pdf_oxide to DIFFERENT
    # versions. pdf_oxide_url must read the pdf_oxide line, not the
    # office_oxide one (a naive "read the first version in go.mod" would pick
    # the wrong library). office_oxide_url must likewise stay on its own line.
    (tmp_path / "go.mod").write_text("module fixture\n\nrequire github.com/yfedoseev/office_oxide/go v0.1.12\nrequire github.com/yfedoseev/pdf_oxide/go v0.4.1\n")
    pdf_url = native.pdf_oxide_url(str(tmp_path), "linux", "amd64")
    office_url = native.office_oxide_url(str(tmp_path), "linux", "amd64")
    assert "/v0.4.1/" in pdf_url  # pdf_oxide's own version
    assert "/v0.1.12/" in office_url  # office_oxide's own version
    assert "/v0.1.12/" not in pdf_url  # not coupled to office_oxide's line


def test_pdf_oxide_current_go_mod_version_is_0_3_78():
    # TDD contract for the pdf_oxide upgrade: the real repo go.mod must pin
    # v0.3.78 (not the prior 0.3.73). build.sh and the downloader both read
    # this single source, so the native-lib cache path and release URL must
    # track 0.3.78 after the bump. Adversarial: explicitly NOT 0.3.73, so a
    # forgotten / partial bump (go.mod left at the old pin) fails loudly.
    version = native.go_module_version("github.com/yfedoseev/pdf_oxide/go", native.REPO_ROOT)
    assert version == "0.3.78"
    assert version != "0.3.73"


def test_pdf_oxide_url_targets_0_3_78_release():
    # The downloader's release URL must reference v0.3.78 (derived from go.mod);
    # it must NOT still point at the prior 0.3.73 asset. Adversarial: the old
    # version string must be absent, otherwise the versioned native-lib cache
    # would be populated from the wrong release.
    url = native.pdf_oxide_url(native.REPO_ROOT, "linux", "amd64")
    assert "/v0.3.78/" in url
    assert "/v0.3.73/" not in url


def test_url_builders_honor_gh_proxy(monkeypatch):
    monkeypatch.setenv("GH_PROXY", "https://gh-proxy.com/")
    assert native.pdfium_url("linux", "amd64").startswith("https://gh-proxy.com/https://github.com/")
    assert native.pdf_oxide_url(native.REPO_ROOT, "linux", "arm64").startswith("https://gh-proxy.com/https://github.com/")


def test_dev_downloader_rejects_untrusted_gh_proxy(monkeypatch):
    monkeypatch.setenv("GH_PROXY", "https://attacker.invalid/")
    with pytest.raises(RuntimeError, match="GH_PROXY"):
        native.gh_proxy()


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


def test_pdfium_required_files_by_platform():
    """Verify PDFium required archive entries for Linux vs. macOS."""
    assert native.pdfium_required_files("linux") == [
        "lib/libpdfium.a",
        "lib/libc++.a",
        "lib/libc++abi.a",
        "include/fpdfview.h",
    ]
    assert native.pdfium_required_files("darwin") == [
        "lib/libpdfium.a",
        "include/fpdfview.h",
    ]


def test_extracts_darwin_pdfium_archive_without_bundled_libcxx(tmp_path):
    """Darwin PDFium archives without bundled libc++ extract on macOS and fail Linux checks."""
    archive = tmp_path / "pdfium-mac-arm64-static.tgz"
    with tarfile.open(archive, "w:gz") as tf:
        for rel, payload in [
            ("lib/libpdfium.a", b"!<arch>\npdfium"),
            ("include/fpdfview.h", b"// header"),
        ]:
            info = tarfile.TarInfo(rel)
            info.size = len(payload)
            tf.addfile(info, io.BytesIO(payload))

    darwin_target = tmp_path / "pdfium-darwin"
    native._extract_native_archive(
        str(archive),
        str(darwin_target),
        native.pdfium_required_files("darwin"),
        platform=("darwin", "arm64"),
    )
    assert (darwin_target / "lib" / "libpdfium.a").is_file()
    assert (darwin_target / "include" / "fpdfview.h").is_file()

    linux_target = tmp_path / "pdfium-linux"
    with pytest.raises(RuntimeError, match="missing required native file"):
        native._extract_native_archive(str(archive), str(linux_target), native.pdfium_required_files("linux"))


def test_extract_native_archive_replaces_cached_tree_when_platform_changes(tmp_path):
    """Switching target platform/arch replaces a cached PDFium tree instead of reusing it."""

    def write_pdfium_tar(path, payload):
        """Create a minimal Darwin PDFium tar.gz with the given libpdfium.a payload."""
        with tarfile.open(path, "w:gz") as tf:
            for rel, data in [
                ("lib/libpdfium.a", payload),
                ("include/fpdfview.h", b"// header"),
            ]:
                info = tarfile.TarInfo(rel)
                info.size = len(data)
                tf.addfile(info, io.BytesIO(data))

    arm64_archive = tmp_path / "pdfium-mac-arm64-static.tgz"
    amd64_archive = tmp_path / "pdfium-mac-x64-static.tgz"
    write_pdfium_tar(arm64_archive, b"!<arch>\narm64-pdfium")
    write_pdfium_tar(amd64_archive, b"!<arch>\namd64-pdfium")

    target = tmp_path / "pdfium-static"
    required = native.pdfium_required_files("darwin")
    native._extract_native_archive(str(arm64_archive), str(target), required, platform=("darwin", "arm64"))
    assert (target / "lib" / "libpdfium.a").read_bytes() == b"!<arch>\narm64-pdfium"

    native._extract_native_archive(str(amd64_archive), str(target), required, platform=("darwin", "amd64"))
    assert (target / "lib" / "libpdfium.a").read_bytes() == b"!<arch>\namd64-pdfium"


def test_extract_native_lib_replaces_cached_pdfium_when_hash_changes(tmp_path, monkeypatch):
    """A changed archive replaces PDFium cached under the same version and platform."""

    def write_pdfium_tar(path, payload):
        with tarfile.open(path, "w:gz") as tf:
            for rel, data in [
                ("lib/libpdfium.a", payload),
                ("include/fpdfview.h", b"// header"),
            ]:
                info = tarfile.TarInfo(rel)
                info.size = len(data)
                tf.addfile(info, io.BytesIO(data))

    archive_v1 = tmp_path / "pdfium-v1.tgz"
    archive_v2 = tmp_path / "pdfium-v2.tgz"
    write_pdfium_tar(archive_v1, b"!<arch>\npdfium-v1")
    write_pdfium_tar(archive_v2, b"!<arch>\npdfium-v2")

    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", str(tmp_path / "native-libs"))
    dir_version = f"v{native.PDFIUM_VERSION}"
    target = tmp_path / "native-libs" / "pdfium-static" / dir_version
    required = native.pdfium_required_files("darwin")
    native.extract_native_lib(
        str(archive_v1),
        "pdfium-static",
        dir_version,
        required,
        platform=("darwin", "arm64"),
    )
    assert (target / "lib" / "libpdfium.a").read_bytes() == b"!<arch>\npdfium-v1"

    native.extract_native_lib(
        str(archive_v2),
        "pdfium-static",
        dir_version,
        required,
        platform=("darwin", "arm64"),
    )
    assert (target / "lib" / "libpdfium.a").read_bytes() == b"!<arch>\npdfium-v2"


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


# ── Version-stamped extraction (Slice 2): native .a must land under a
# version-keyed subdir (v<version>/) of the user cache, never a flat layout, so a
# single mounted cache can hold several versions at once (different PRs pinned to
# different go.mod versions all build offline). extract_native_lib() is the
# single extraction seam main() calls; it is idempotent (skips when the version
# dir already holds the required files). ─────────────────────────────────────


def _make_pdfium_archive(path, names):
    with tarfile.open(path, "w:gz") as tf:
        for name in names:
            data = b"pdfium-payload-1234"  # non-empty so complete() accepts it
            info = tarfile.TarInfo(name)
            info.size = len(data)
            tf.addfile(info, io.BytesIO(data))


def test_extracts_pdfium_into_versioned_dir(tmp_path, monkeypatch):
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", str(tmp_path / "native-libs"))
    archive = tmp_path / "pdfium-linux-x64-static.tgz"
    _make_pdfium_archive(archive, ["lib/libpdfium.a", "lib/libc++.a", "lib/libc++abi.a", "include/fpdfview.h"])
    native.extract_native_lib(
        str(archive),
        "pdfium-static",
        "v7809",
        ["lib/libpdfium.a", "lib/libc++.a", "lib/libc++abi.a", "include/fpdfview.h"],
        None,
    )
    target = tmp_path / "native-libs" / "pdfium-static" / "v7809"
    assert (target / "lib" / "libpdfium.a").is_file()
    assert (target / "include" / "fpdfview.h").is_file()


def test_extract_native_lib_idempotent_on_rerun(tmp_path, monkeypatch):
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", str(tmp_path / "native-libs"))
    archive = tmp_path / "pdfium-linux-x64-static.tgz"
    _make_pdfium_archive(archive, ["lib/libpdfium.a"])
    native.extract_native_lib(str(archive), "pdfium-static", "v7809", ["lib/libpdfium.a"], None)
    before = sorted(os.listdir(tmp_path / "native-libs" / "pdfium-static" / "v7809" / "lib"))
    native.extract_native_lib(str(archive), "pdfium-static", "v7809", ["lib/libpdfium.a"], None)
    after = sorted(os.listdir(tmp_path / "native-libs" / "pdfium-static" / "v7809" / "lib"))
    assert before == after


def _make_stamped_archive(path, lib_name, stamp, header_name):
    """Build a tar.gz with the .a (carrying STAMP, a version-bearing byte
    string) plus an include header, so complete()'s file-list AND stamp checks
    can both be exercised per library format."""
    with tarfile.open(path, "w:gz") as tf:
        info = tarfile.TarInfo(lib_name)
        payload = b"!<arch>\n" + stamp
        info.size = len(payload)
        tf.addfile(info, io.BytesIO(payload))
        header = tarfile.TarInfo(header_name)
        header.size = len(b"// header")
        tf.addfile(header, io.BytesIO(b"// header"))


def test_pdf_oxide_stamp_text_accepts_matching_version(tmp_path, monkeypatch):
    # pdf_oxide embeds a plain-text `pdf_oxide <version>` token (not the
    # null-delimited token office_oxide uses). complete() must accept it when
    # stamp_format="text", or auto-download would reject a good archive and
    # build.sh's controlled download path would fail closed.
    version = native.go_module_version("github.com/yfedoseev/pdf_oxide/go", native.REPO_ROOT)
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", str(tmp_path / "native-libs"))
    archive = tmp_path / "pdf_oxide-go-ffi-linux-amd64.tar.gz"
    _make_stamped_archive(archive, "lib/linux_amd64/libpdf_oxide.a", f"pdf_oxide {version}".encode(), "include/pdf_oxide.h")
    native.extract_native_lib(
        str(archive),
        "pdf_oxide",
        f"v{version}",
        ["lib/linux_amd64/libpdf_oxide.a", "include/pdf_oxide.h"],
        version,
        "text",
    )
    assert (tmp_path / "native-libs" / "pdf_oxide" / f"v{version}" / "lib" / "linux_amd64" / "libpdf_oxide.a").is_file()


def test_pdf_oxide_stamp_text_rejects_wrong_version(tmp_path, monkeypatch):
    # A pdf_oxide .a stamped with a different version must be rejected, not
    # silently accepted (the whole point of the stamp check is to catch a
    # mismatched/wrong native build).
    version = native.go_module_version("github.com/yfedoseev/pdf_oxide/go", native.REPO_ROOT)
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", str(tmp_path / "native-libs"))
    archive = tmp_path / "pdf_oxide-go-ffi-linux-amd64.tar.gz"
    _make_stamped_archive(archive, "lib/linux_amd64/libpdf_oxide.a", b"pdf_oxide 0.9.9", "include/pdf_oxide.h")
    with pytest.raises(RuntimeError, match=f"missing required native files or version {version}"):
        native.extract_native_lib(
            str(archive),
            "pdf_oxide",
            f"v{version}",
            ["lib/linux_amd64/libpdf_oxide.a", "include/pdf_oxide.h"],
            version,
            "text",
        )


def test_valid_download_accepts_pdf_oxide_tarball_with_current_version(tmp_path):
    # Regression for the stale-archive reuse bug: a pdf_oxide tarball whose
    # embedded .a carries the version go.mod pins must be treated as a valid
    # download, so main() skips re-downloading it (the "already present" fast
    # path). _valid_download must read the SAME version go.mod pins, via the
    # same go_module_version() the downloader URL builder uses.
    version = native.go_module_version("github.com/yfedoseev/pdf_oxide/go", native.REPO_ROOT)
    archive = tmp_path / "pdf_oxide-go-ffi-linux-amd64.tar.gz"
    _make_stamped_archive(archive, "lib/linux_amd64/libpdf_oxide.a", f"pdf_oxide {version}".encode(), "include/pdf_oxide.h")
    assert native._valid_download(str(archive)) is True


def test_valid_download_rejects_pdf_oxide_tarball_with_stale_version(tmp_path):
    # The exact failure the user hit: a pdf_oxide tarball left over from a prior
    # version sitting in deps/ must NOT be reused. Before this fix _valid_download
    # accepted ANY non-empty tarball and the version mismatch only surfaced later
    # as a RuntimeError at extraction ("missing required native files or version
    # <current>"). Rejecting it here makes the downloader re-fetch the correct
    # release and keeps the error close to the cause.
    current = native.go_module_version("github.com/yfedoseev/pdf_oxide/go", native.REPO_ROOT)
    stale = "0.0.0" if current != "0.0.0" else "0.0.1"
    archive = tmp_path / "pdf_oxide-go-ffi-linux-amd64.tar.gz"
    _make_stamped_archive(archive, "lib/linux_amd64/libpdf_oxide.a", f"pdf_oxide {stale}".encode(), "include/pdf_oxide.h")
    assert native._valid_download(str(archive)) is False


def test_office_oxide_stamp_null_accepts_matching_version(tmp_path, monkeypatch):
    # Regression guard: office_oxide keeps its null-delimited stamp token; the
    # "text" vs "null" split must not break it.
    version = native.go_module_version("github.com/yfedoseev/office_oxide/go", native.REPO_ROOT)
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", str(tmp_path / "native-libs"))
    archive = tmp_path / "native-linux-x86_64.tar.gz"
    _make_stamped_archive(archive, "lib/liboffice_oxide.a", f"\x00{version}\x00".encode(), "include/office_oxide_c/office_oxide.h")
    native.extract_native_lib(
        str(archive),
        "office_oxide",
        f"v{version}",
        ["lib/liboffice_oxide.a", "include/office_oxide_c/office_oxide.h"],
        version,
        "null",
    )
    assert (tmp_path / "native-libs" / "office_oxide" / f"v{version}" / "lib" / "liboffice_oxide.a").is_file()


# ── Cache-aware download: the download pass must skip re-fetching a native lib
# whose version-keyed cache dir (~/ragflow-native-libs/<lib>/v<ver>/) is already
# provisioned (e.g. a CI builder's mounted pre-baked native-libs cache), not
# just the transient deps/ archive. native_lib_provisioned is the shared check
# _extract_native_archive.complete() also uses, lifted to module scope. ──────


def test_native_lib_provisioned_true_when_files_present(tmp_path):
    d = tmp_path / "pdfium-static" / "v7809"
    for name in ["lib/libpdfium.a", "lib/libc++.a", "include/fpdfview.h"]:
        p = d / name
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_bytes(b"pdfium-payload")
    assert native.native_lib_provisioned(str(d), ["lib/libpdfium.a", "lib/libc++.a", "include/fpdfview.h"], None, "null") is True


def test_native_lib_provisioned_false_when_required_file_missing(tmp_path):
    d = tmp_path / "pdfium-static" / "v7809"
    d.mkdir(parents=True)
    (d / "lib").mkdir()
    (d / "lib" / "libpdfium.a").write_bytes(b"x")
    # libc++.a and the header are absent -> not provisioned.
    assert native.native_lib_provisioned(str(d), ["lib/libpdfium.a", "lib/libc++.a", "include/fpdfview.h"], None, "null") is False


def test_native_lib_provisioned_rejects_empty_file(tmp_path):
    d = tmp_path / "pdfium-static" / "v7809"
    d.mkdir(parents=True)
    (d / "lib").mkdir()
    (d / "lib" / "libpdfium.a").write_bytes(b"")  # zero bytes -> unusable
    assert native.native_lib_provisioned(str(d), ["lib/libpdfium.a"], None, "null") is False


def test_native_lib_provisioned_pdf_oxide_true_on_correct_stamp(tmp_path):
    version = native.go_module_version("github.com/yfedoseev/pdf_oxide/go", native.REPO_ROOT)
    d = tmp_path / "pdf_oxide" / f"v{version}"
    (d / "lib" / "linux_amd64").mkdir(parents=True)
    (d / "lib" / "linux_amd64" / "libpdf_oxide.a").write_bytes(b"!<arch>\n" + f"pdf_oxide {version}".encode())
    (d / "include").mkdir()
    (d / "include" / "pdf_oxide.h").write_bytes(b"// header")
    assert native.native_lib_provisioned(str(d), ["lib/linux_amd64/libpdf_oxide.a", "include/pdf_oxide.h"], version, "text") is True


def test_native_lib_provisioned_pdf_oxide_false_on_stamp_mismatch(tmp_path):
    version = native.go_module_version("github.com/yfedoseev/pdf_oxide/go", native.REPO_ROOT)
    d = tmp_path / "pdf_oxide" / f"v{version}"
    d.mkdir(parents=True)
    (d / "lib" / "linux_amd64").mkdir(parents=True)
    # Stamped with a different version -> must be rejected, else a wrong native
    # build would be silently treated as provisioned.
    (d / "lib" / "linux_amd64" / "libpdf_oxide.a").write_bytes(b"!<arch>\n" + b"pdf_oxide 0.9.9")
    (d / "include").mkdir()
    (d / "include" / "pdf_oxide.h").write_bytes(b"// header")
    assert native.native_lib_provisioned(str(d), ["lib/linux_amd64/libpdf_oxide.a", "include/pdf_oxide.h"], version, "text") is False


def test_native_lib_provisioned_office_oxide_true_on_null_stamp(tmp_path):
    version = native.go_module_version("github.com/yfedoseev/office_oxide/go", native.REPO_ROOT)
    d = tmp_path / "office_oxide" / f"v{version}"
    (d / "lib").mkdir(parents=True)
    (d / "lib" / "liboffice_oxide.a").write_bytes(b"!<arch>\n" + f"\x00{version}\x00".encode())
    (d / "include" / "office_oxide_c").mkdir(parents=True)
    (d / "include" / "office_oxide_c" / "office_oxide.h").write_bytes(b"// header")
    assert native.native_lib_provisioned(str(d), ["lib/liboffice_oxide.a", "include/office_oxide_c/office_oxide.h"], version, "null") is True


def test_native_lib_provisioned_office_oxide_false_on_null_stamp_mismatch(tmp_path):
    version = native.go_module_version("github.com/yfedoseev/office_oxide/go", native.REPO_ROOT)
    d = tmp_path / "office_oxide" / f"v{version}"
    d.mkdir(parents=True)
    (d / "lib").mkdir()
    (d / "lib" / "liboffice_oxide.a").write_bytes(b"!<arch>\n" + b"\x00" + b"0.0.0" + b"\x00")
    (d / "include" / "office_oxide_c").mkdir(parents=True)
    (d / "include" / "office_oxide_c" / "office_oxide.h").write_bytes(b"// header")
    assert native.native_lib_provisioned(str(d), ["lib/liboffice_oxide.a", "include/office_oxide_c/office_oxide.h"], version, "null") is False


def test_download_native_archives_skips_when_provisioned(tmp_path, monkeypatch):
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", str(tmp_path / "native-libs"))
    monkeypatch.setattr(native, "native_lib_provisioned", lambda *a, **k: True)
    spy = mock.MagicMock()
    monkeypatch.setattr(native, "download_with_progress", spy)
    native.download_native_archives("linux", "amd64")
    assert spy.call_count == 0


def test_download_native_archives_downloads_when_not_provisioned(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", str(tmp_path / "native-libs"))
    monkeypatch.setattr(native, "native_lib_provisioned", lambda *a, **k: False)
    # Treat any on-disk file as a valid (already-present) deps/ archive so the
    # post-download validation passes without a real tarball.
    monkeypatch.setattr(native, "_valid_download", lambda f: os.path.isfile(f) and os.path.getsize(f) > 0)

    def _spy(url, filename):
        with open(filename, "wb") as fh:
            fh.write(b"archive")

    spy = mock.MagicMock(side_effect=_spy)
    monkeypatch.setattr(native, "download_with_progress", spy)
    native.download_native_archives("linux", "amd64")
    # pdfium, pdf_oxide, office_oxide — all fetched when the cache is empty.
    assert spy.call_count == 3


# ── Local-dev one-stop: the dev downloader must also fetch the runtime
# resources (cl100k / DeepDoc .ort / tokenizer assets) so a developer runs THIS
# script ONCE and the Go server is ready to build, run, parse and count. ─────


@pytest.fixture
def native_dirs(tmp_path, monkeypatch):
    """Point the dev script's repo-anchored paths at a temp tree so the
    runtime-resource downloads written by tests never touch the real repo."""
    deps = tmp_path / "deps"
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
    # The image-layout copy under deps/huggingface.co must also exist.
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


def test_download_go_models_reuses_model_dir_when_missing(native_dirs, tmp_path, monkeypatch):
    # CI bakes the DeepDoc weights into MODEL_DIR (the ragflow_deps image's
    # /opt/ragflow-deepdoc-models). When the repo tree copy is absent, reuse
    # from there instead of fetching ~100MB from Hugging Face.
    _, root = native_dirs
    baked = tmp_path / "baked"
    baked.mkdir()
    for fname in native.HF_DEEPDOC_MODEL_FILES:
        (baked / fname).write_bytes(b"baked weights")
    monkeypatch.setenv("MODEL_DIR", str(baked))
    fake = mock.MagicMock(side_effect=_fake_hf)
    monkeypatch.setattr(native, "_hf_download", fake)
    native.download_go_models()
    assert fake.call_count == 0  # no Hugging Face fetch
    deepdoc_dir = root / "internal" / "rag" / "res" / "deepdoc"
    for fname in native.HF_DEEPDOC_MODEL_FILES:
        assert (deepdoc_dir / fname).read_bytes() == b"baked weights"


def test_download_go_models_falls_back_to_hf_when_model_dir_empty(native_dirs, tmp_path, monkeypatch):
    _, _ = native_dirs
    baked = tmp_path / "baked"  # set but contains no weights
    baked.mkdir()
    monkeypatch.setenv("MODEL_DIR", str(baked))
    fake = mock.MagicMock(side_effect=_fake_hf)
    monkeypatch.setattr(native, "_hf_download", fake)
    native.download_go_models()
    assert fake.call_count == len(native.HF_DEEPDOC_MODEL_FILES)


def test_download_go_models_falls_back_to_hf_when_model_dir_unset(native_dirs, tmp_path, monkeypatch):
    _, _ = native_dirs
    monkeypatch.delenv("MODEL_DIR", raising=False)
    fake = mock.MagicMock(side_effect=_fake_hf)
    monkeypatch.setattr(native, "_hf_download", fake)
    native.download_go_models()
    assert fake.call_count == len(native.HF_DEEPDOC_MODEL_FILES)


def test_download_go_models_falls_back_to_hf_when_model_dir_file_empty(native_dirs, tmp_path, monkeypatch):
    _, _ = native_dirs
    baked = tmp_path / "baked"
    baked.mkdir()
    for fname in native.HF_DEEPDOC_MODEL_FILES:
        (baked / fname).write_bytes(b"")  # zero bytes -> not a usable copy
    monkeypatch.setenv("MODEL_DIR", str(baked))
    fake = mock.MagicMock(side_effect=_fake_hf)
    monkeypatch.setattr(native, "_hf_download", fake)
    native.download_go_models()
    assert fake.call_count == len(native.HF_DEEPDOC_MODEL_FILES)


def _seed_tokenizer_targets(deps, present):
    """Create (present=True) or leave absent the on-disk targets the inlined
    TOKENIZER_ASSETS list resolves to under deps/huggingface.co/<repo>/<file>."""
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


# ── Image script (download_deps.py) must stay runtime-resources only: it must
# NOT handle the native .a archives (the build stage bakes those into the
# binary). Imported by path inside the test so the import lives next to the
# native script and never touches module import timing. ──────────────────────


def test_image_script_does_not_handle_native_archives():
    deps_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "deps", "download_deps.py")
    assert os.path.isfile(deps_path), f"download_deps.py not found at {deps_path}"
    spec = importlib.util.spec_from_file_location("download_deps_img", deps_path)
    img = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(img)
    assert not hasattr(img, "extract_onnxruntime")
    assert not hasattr(img, "_extract_native_archive")
    assert "onnxruntime" not in (img.__doc__ or "").lower()


def _image_deps_module():
    deps_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "deps", "download_deps.py")
    spec = importlib.util.spec_from_file_location("download_deps_image_security", deps_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_image_proxy_is_restricted_to_the_configured_mirror(monkeypatch):
    image_deps = _image_deps_module()
    monkeypatch.setenv("GH_PROXY", "https://attacker.invalid/")
    with pytest.raises(RuntimeError, match="GH_PROXY"):
        image_deps.gh_proxy()

    monkeypatch.setenv("GH_PROXY", "https://gh-proxy.com/")
    assert image_deps.gh_proxy() == "https://gh-proxy.com/"


# ── Documentation contract: ARM64 build-host minimums ─────────────────────
# The ARM64 host requirements are empirical findings from real hardware
# (glibc 2.38 __isoc23_* symbols, GCC 13 libstdc++, SIMDe headers, cmake 4.x,
# lld 20 — where lld-18 LINKS but hangs at startup). They cost half a day to
# diagnose, so a doc refactor must not silently drop them: these tests pin the
# section and its failure signatures.


def _arm64_docs_section():
    """Return the ARM64 host-requirements block of internal/development.md.

    Scoped to the section (anchor → next "###" heading) so the assertions
    below check the requirements block itself, not unrelated version numbers
    elsewhere in an 800-line document.
    """
    doc = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "internal", "development.md")
    assert os.path.isfile(doc), f"development.md not found at {doc}"
    with open(doc, encoding="utf-8") as file:
        text = file.read()
    start = text.find("ARM64 (aarch64) build hosts")
    assert start != -1, "development.md no longer has an ARM64 build-hosts section"
    end = text.find("\n### ", start)
    return text[start:] if end == -1 else text[start:end]


def test_development_docs_document_arm64_host_requirements():
    section = _arm64_docs_section()
    # Each minimum, with its concrete failure signature, must stay documented.
    for token in (
        "glibc",
        "2.38",
        "__isoc23_strtoll",
        "GCC",
        "13",
        "libsimde-dev",
        "simde/x86/sse4.1.h",
        "cmake",
        "4.0",
        "lld",
        "20",
        "inittrace",
    ):
        assert token in section, f"development.md ARM64 section lost requirement token {token!r}"


def test_arm64_docs_section_warns_that_lld18_links_but_hangs():
    # The subtle failure mode: lld-18 produces a binary that links fine and
    # then spins at startup. Drop this warning and an ARM64 host silently
    # keeping the distro-default lld hits an undebuggable hang.
    section = _arm64_docs_section()
    assert "lld-18" in section, "lld-18 is no longer called out as insufficient"
    assert "hang" in section.lower() or "spin" in section.lower(), "lld-18 startup-hang failure mode missing"


# ── Single source of truth for native versions (Slice 1) ───────────────────────
# build.sh and download_dev_deps.py must agree on every native version. The
# authoritative values live in tools/scripts/native_lib_version.sh (office_oxide
# / pdf_oxide from go.mod, ORT from environments.go, pdfium as one shared const).
# These tests pin that contract: no second hardcoded copy may drift.


def test_deepdoc_ort_version_reads_environments_go(tmp_path):
    # Fixture environments.go with a deliberately non-default version.
    common = tmp_path / "internal" / "common"
    common.mkdir(parents=True)
    (common / "environments.go").write_text('package common\n\n// DeepDocORTVersion is ...\nconst DeepDocORTVersion = "9.9.9"\n')
    assert native.deepdoc_ort_version(str(tmp_path)) == "9.9.9"


def test_deepdoc_ort_version_against_real_repo():
    # Sanity: parses the checked-out repo's actual pin. Assert the shape (semver)
    # rather than a hardcoded literal so the test tracks environments.go on bumps.
    import re

    version = native.deepdoc_ort_version(native.REPO_ROOT)
    assert re.match(r"^\d+\.\d+\.\d+", version), version


def test_pdfium_static_version_reads_single_source(tmp_path):
    # Fixture native_lib_version.sh exporting a sentinel (NOT the real 7809).
    scripts = tmp_path / "tools" / "scripts"
    scripts.mkdir(parents=True)
    (scripts / "native_lib_version.sh").write_text("pdfium_static_version() { echo 1234; }\n")
    assert native.pdfium_static_version(str(tmp_path)) == "1234"


def test_pdfium_static_version_against_real_repo():
    # Sanity: the shared const on the checked-out repo.
    assert native.pdfium_static_version(native.REPO_ROOT) == "7809"


def test_pdfium_local_asset_name_carries_version_and_platform():
    # The on-disk pdfium tarball name must encode the resolved chromium build
    # number so two pdfium versions never share a deps/ path (the silent-reuse
    # bug). It must NOT equal the version-agnostic upstream asset name, and must
    # keep the platform token so the right arch is selected.
    version = native.pdfium_static_version(native.REPO_ROOT)
    name = native.pdfium_local_asset_name("linux", "amd64")
    assert name == f"pdfium-linux-x64-static-{version}.tgz"
    # Adversarial: if this were still the bare asset name, a stale tarball would
    # be silently reused — the whole point is that the name differs.
    assert name != native.PDFIUM_STATIC_ASSETS[("linux", "amd64")]
    assert version in name
    assert "linux-x64" in name


def test_pdfium_local_asset_name_is_version_driven_not_hardcoded(monkeypatch):
    # Guard against a wrong implementation that hardcodes "7809" or otherwise
    # ignores PDFIUM_VERSION: when the bound version changes, the local name
    # must change with it.
    monkeypatch.setattr(native, "PDFIUM_VERSION", "9999")
    assert native.pdfium_local_asset_name("linux", "amd64") == "pdfium-linux-x64-static-9999.tgz"
    assert native.pdfium_local_asset_name("darwin", "arm64") == "pdfium-mac-arm64-static-9999.tgz"


def test_pdfium_url_basename_stays_version_agnostic():
    # The download URL must keep pointing at the upstream version-agnostic
    # asset (.../chromium/7809/pdfium-linux-x64-static.tgz). Only the LOCAL name
    # is versioned; versioning the URL instead would break the download, since
    # the upstream release asset name does not contain the build number.
    url = native.pdfium_url("linux", "amd64")
    assert url.endswith("/" + native.PDFIUM_STATIC_ASSETS[("linux", "amd64")])
    assert native.pdfium_local_asset_name("linux", "amd64") not in url


def test_valid_download_accepts_versioned_pdfium_tarball(tmp_path):
    # Regression: the renamed pdfium tarball is still a valid .tgz, so the
    # downloader's "already present" fast path still fires for it (it is not in
    # the OFFICE/PDF_OXIDE asset sets, so _valid_download returns True for any
    # non-empty valid tarball).
    version = native.pdfium_static_version(native.REPO_ROOT)
    archive = tmp_path / f"pdfium-linux-x64-static-{version}.tgz"
    _make_pdfium_archive(archive, ["lib/libpdfium.a"])
    assert native._valid_download(str(archive)) is True


# ── deepdoc_ort_version (bash) single source ───────────────────────────────
# build.sh must read DeepDocORTVersion from environments.go through the SAME
# native_lib_version.sh helper the Python downloader mirrors (deepdoc_ort_version
# in download_dev_deps.py), not an inline grep repeated at every call site. These
# tests pin the bash function contract so the 4 inline greps in build.sh can be
# collapsed onto one source.


def test_deepdoc_ort_version_bash_reads_environments_go(tmp_path):
    # Fixture environments.go with a deliberately non-default version.
    common = tmp_path / "internal" / "common"
    common.mkdir(parents=True)
    (common / "environments.go").write_text('package common\n\n// DeepDocORTVersion is ...\nconst DeepDocORTVersion = "9.9.9"\n')
    res = _sh_eval(f"deepdoc_ort_version {tmp_path}", _env_with("/home/alice"))
    assert res.returncode == 0, res.stderr
    assert res.stdout.strip() == "9.9.9"


def test_deepdoc_ort_version_bash_against_real_repo():
    # Sanity: parses the checked-out repo's actual pin. Assert the shape (semver)
    # rather than a hardcoded literal so the test tracks environments.go on bumps.
    import re

    res = _sh_eval(f"deepdoc_ort_version {native.REPO_ROOT}", _env_with("/home/alice"))
    assert res.returncode == 0, res.stderr
    assert re.match(r"^\d+\.\d+\.\d+", res.stdout.strip()), res.stdout


def test_deepdoc_ort_version_bash_missing_file(tmp_path):
    # Adversarial: an empty repo root with no environments.go must NOT echo a
    # version and must signal failure (so callers fail loud, mirroring the
    # Python RuntimeError), not silently return an empty string with rc 0.
    res = _sh_eval(f"deepdoc_ort_version {tmp_path}", _env_with("/home/alice"))
    assert res.returncode == 1, res.stdout
    assert res.stdout.strip() == ""


# ── Slice 3: CI mounts the native-lib user cache (RAGFLOW_NATIVE_LIBS_DIR) ──
# build.sh resolves the native static libs from a single cache root
# (default ~/ragflow-native-libs, overridable via RAGFLOW_NATIVE_LIBS_DIR so CI
# can mount the cache at a fixed host path instead of the build user's $HOME).
# These tests pin that resolution contract (native_lib_version.sh) and the
# sep-tests.yml wiring that mounts the cache into every BUILDER_CONTAINER.


def _sh_eval(func_call, env):
    """Source the real native_lib_version.sh and call FUNC_CALL in a throwaway
    bash, returning the CompletedProcess. ENV is the full env (HOME/PATH must be
    supplied), mirroring how build.sh sources the file."""
    sh = os.path.join(native.REPO_ROOT, "tools", "scripts", "native_lib_version.sh")
    assert os.path.isfile(sh), f"native_lib_version.sh not found at {sh}"
    cmd = f'source "{sh}" && {func_call}'
    return subprocess.run(["bash", "-c", cmd], env=env, capture_output=True, text=True, check=False)


def _env_with(home, override=None):
    env = {"HOME": home, "PATH": os.environ.get("PATH", "")}
    if override is not None:
        env["RAGFLOW_NATIVE_LIBS_DIR"] = override
    return env


def test_native_libs_dir_defaults_to_home():
    # No override: resolve to $HOME/ragflow-native-libs.
    res = _sh_eval("ragflow_native_libs_dir", _env_with("/home/alice"))
    assert res.returncode == 0, res.stderr
    assert res.stdout.strip() == "/home/alice/ragflow-native-libs"


def test_native_libs_dir_honors_override():
    # Override set: CI mounts the cache at a fixed host path; build.sh must use it.
    res = _sh_eval("ragflow_native_libs_dir", _env_with("/home/alice", "/mnt/cache"))
    assert res.returncode == 0, res.stderr
    assert res.stdout.strip() == "/mnt/cache"


def test_native_libs_dir_empty_override_falls_back():
    # Adversarial: an explicitly EMPTY override must fall back to the HOME default
    # (the ${VAR:-default} expansion), never resolve to an empty cache root.
    res = _sh_eval("ragflow_native_libs_dir", _env_with("/home/alice", ""))
    assert res.returncode == 0, res.stderr
    assert res.stdout.strip() == "/home/alice/ragflow-native-libs"


SEP_TESTS_YML = os.path.join(native.REPO_ROOT, ".github", "workflows", "sep-tests.yml")


def test_sep_tests_mounts_native_libs_cache():
    # Slice 3 contract: the native static-lib user cache is mounted into every
    # BUILDER_CONTAINER (4 docker runs) and exposed via RAGFLOW_NATIVE_LIBS_DIR,
    # replacing the old /opt pre-bake. The YAML must also stay parseable.
    assert os.path.isfile(SEP_TESTS_YML), f"sep-tests.yml not found at {SEP_TESTS_YML}"
    with open(SEP_TESTS_YML, encoding="utf-8") as f:
        content = f.read()
    # YAML must still parse (guards against a malformed edit); skip when pyyaml
    # is unavailable in the test env (the CI runner has it).
    try:
        import yaml

        yaml.safe_load(content)
    except ImportError:
        pass
    # The cache root is declared as a job env so runner and builder container agree.
    assert "RAGFLOW_NATIVE_LIBS_DIR" in content
    # Each of the 4 BUILDER_CONTAINER docker runs bind-mounts the cache.
    assert content.count("${RAGFLOW_NATIVE_LIBS_DIR}:${RAGFLOW_NATIVE_LIBS_DIR}") >= 4
    # A fail-fast guard catches a missing/partial mount before a cryptic cgo link error.
    assert "native-lib cache not mounted" in content


# ── downloader root resolution must honour RAGFLOW_NATIVE_LIBS_DIR (P1#2) ─────
# build.sh resolves the cache root through native_lib_version.ragflow_native_libs_dir()
# (${RAGFLOW_NATIVE_LIBS_DIR:-~/ragflow-native-libs}); the downloader must agree so a
# runner that overrides the mount path does not silently download into the default.


def test_python_native_libs_dir_default(monkeypatch):
    monkeypatch.delenv("RAGFLOW_NATIVE_LIBS_DIR", raising=False)
    assert native.native_libs_dir() == os.path.expanduser("~/ragflow-native-libs")


def test_python_native_libs_dir_honors_override(monkeypatch):
    # Adversarial: a custom mount path must be returned verbatim, NOT the default.
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", "/srv/custom/native-libs")
    assert native.native_libs_dir() == "/srv/custom/native-libs"


def test_python_native_libs_dir_empty_override_falls_back(monkeypatch):
    # Adversarial: an empty override must fall back to the default, not resolve to "".
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", "")
    assert native.native_libs_dir() == os.path.expanduser("~/ragflow-native-libs")


# ── downloader must not crash when deps/ is absent (P1#3) ─────────────────────


def test_ensure_deps_dir_creates_missing(monkeypatch, tmp_path):
    # In the Docker go-builder stage nothing COPYs deps/, so the downloader's
    # os.chdir(deps) used to raise FileNotFoundError and abort the image build.
    # ensure_deps_dir() must create it first.
    missing = tmp_path / "no_deps_dir"
    monkeypatch.setattr(native, "RAGFLOW_DEPS_DIR", str(missing))
    native.ensure_deps_dir()
    assert missing.is_dir()


# ── ORT version-stamp symlink must be RELATIVE and read-only-safe (Fix #1/#2) ──
# build.sh stamps the ORT archive with a v<version>-<sha16> symlink so an
# in-place .a upgrade invalidates Go's build cache. The symlink MUST be relative
# (resolves under any cache mount: build host, runner, CI builder) and build.sh
# MUST skip the write when an existing link already resolves to the same archive
# (so a read-only CI mount is a no-op instead of a fatal `ln` on a read-only fs).


_ORT_STAMP_SH = os.path.join(native.REPO_ROOT, "tools", "scripts", "ort_stamp.sh")


def _stamp_eval(versioned_dir, archive_file):
    assert os.path.isfile(_ORT_STAMP_SH), f"ort_stamp.sh not found at {_ORT_STAMP_SH}"
    cmd = f'source "{_ORT_STAMP_SH}" && ragflow_ort_stamp_target "$@"'
    return subprocess.run(
        ["bash", "-c", cmd, "_", versioned_dir, archive_file],
        capture_output=True,
        text=True,
        check=False,
    )


def test_ort_stamp_target_is_relative(tmp_path):
    versioned = tmp_path / "cache" / "onnxruntime" / "static_lib" / "v1.29.0-abc123"
    versioned.mkdir(parents=True)
    archive = tmp_path / "cache" / "onnxruntime" / "static_lib" / "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28" / "lib" / "libonnxruntime.a"
    archive.parent.mkdir(parents=True)
    archive.write_bytes(b"ort")
    res = _stamp_eval(str(versioned), str(archive))
    assert res.returncode == 0, res.stderr
    needs, target = res.stdout.strip().split("|")
    assert needs == "yes"
    assert not os.path.isabs(target), f"stamp target must be relative, got {target!r}"
    assert (versioned / target).resolve() == archive.resolve()


def test_ort_stamp_target_skips_when_existing_link_correct(tmp_path):
    # ro-safe: an existing symlink already pointing at the archive means no write.
    versioned = tmp_path / "cache" / "onnxruntime" / "static_lib" / "v1.29.0-abc123"
    versioned.mkdir(parents=True)
    archive = tmp_path / "cache" / "onnxruntime" / "static_lib" / "norm" / "lib" / "libonnxruntime.a"
    archive.parent.mkdir(parents=True)
    archive.write_bytes(b"ort")
    rel = os.path.relpath(archive, versioned)
    os.symlink(rel, versioned / "libonnxruntime.a")
    res = _stamp_eval(str(versioned), str(archive))
    assert res.returncode == 0, res.stderr
    needs, target = res.stdout.strip().split("|")
    assert needs == "no", "existing correct symlink must need no write (ro-safe)"
    assert target == rel


def test_ort_stamp_target_writes_when_link_missing(tmp_path):
    versioned = tmp_path / "v1.29.0-abc123"
    versioned.mkdir(parents=True)
    archive = tmp_path / "norm" / "lib" / "libonnxruntime.a"
    archive.parent.mkdir(parents=True)
    archive.write_bytes(b"ort")
    res = _stamp_eval(str(versioned), str(archive))
    assert res.returncode == 0, res.stderr
    needs, _ = res.stdout.strip().split("|")
    assert needs == "yes"


# ── download_dev_deps.py pre-creates the ORT stamp dir during population ──────
# build.sh's stamp step is a no-op on a read-only CI mount ONLY if the stamp dir
# already exists in the cache. The downloader must therefore create the
# v<version>-<sha16> dir (relative symlink) while it still has write access.


def test_stamp_onnxruntime_version_creates_relative_symlink(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    static_lib.mkdir(parents=True)
    norm = static_lib / "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28"
    (norm / "lib").mkdir(parents=True)
    (norm / "lib" / "libonnxruntime.a").write_bytes(b"!<arch>\nort")
    native.stamp_onnxruntime_version(
        str(static_lib),
        "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28",
        "1.29.0",
    )
    stamp_dirs = list(static_lib.glob("v1.29.0-*"))
    assert len(stamp_dirs) == 1, f"expected exactly one stamp dir, found {stamp_dirs}"
    link = stamp_dirs[0] / "libonnxruntime.a"
    assert link.is_symlink(), "stamp must be a symlink"
    assert not os.path.isabs(os.readlink(link)), "stamp symlink must be relative"
    assert link.resolve() == (norm / "lib" / "libonnxruntime.a").resolve()


def test_stamp_onnxruntime_version_idempotent(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    static_lib.mkdir(parents=True)
    norm_dir = "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28"
    norm = static_lib / norm_dir
    (norm / "lib").mkdir(parents=True)
    (norm / "lib" / "libonnxruntime.a").write_bytes(b"!<arch>\nort")
    native.stamp_onnxruntime_version(str(static_lib), norm_dir, "1.29.0")
    before = sorted(p.name for p in static_lib.iterdir())
    native.stamp_onnxruntime_version(str(static_lib), norm_dir, "1.29.0")
    after = sorted(p.name for p in static_lib.iterdir())
    assert before == after


# ── _native_libs_present ORT gate must be version+platform aware ──────────────
# build.sh's _native_libs_present decides whether the controlled auto-downloader
# runs. Its ORT check historically matched ANY .a under the static_lib prefix
# (find ... -name '*.a'), so a shared cache holding only an OLD version or a
# DIFFERENT platform's ORT was reported "present" and the downloader was skipped
# — the missing lib then failed only at link time. The gate must match the SAME
# <platform-prefix><version> dir that check_onnxruntime_deps links
# (*/${ort_dir_prefix}${ort_version}*/lib/*.a). ragflow_ort_present is that pure
# decision so it is unit-testable in isolation.

_ORT_PRESENT_SH = os.path.join(native.REPO_ROOT, "tools", "scripts", "ort_present.sh")


def _present_eval(prefix, ort_dir_prefix, ort_version):
    """Source the real ort_present.sh and call ragflow_ort_present."""
    assert os.path.isfile(_ORT_PRESENT_SH), f"ort_present.sh not found at {_ORT_PRESENT_SH}"
    cmd = f'source "{_ORT_PRESENT_SH}" && ragflow_ort_present "$@"'
    return subprocess.run(
        ["bash", "-c", cmd, "_", prefix, ort_dir_prefix, ort_version],
        capture_output=True,
        text=True,
        check=False,
    )


def _seed_ort(prefix, dir_name):
    """Create a version-keyed ORT dir with a real libonnxruntime.a inside lib/."""
    lib = prefix / dir_name / "lib"
    lib.mkdir(parents=True)
    (lib / "libonnxruntime.a").write_bytes(b"!<arch>\nort")
    return prefix / dir_name


def test_ort_present_happy_path_matches_version_and_platform(tmp_path):
    prefix = tmp_path / "static_lib"
    _seed_ort(prefix, "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28")
    res = _present_eval(str(prefix), "onnxruntime-linux-x64-static_lib-", "1.29.0")
    assert res.returncode == 0, res.stderr


def test_ort_present_rejects_old_version(tmp_path):
    # Adversarial: cache holds ONLY an older ORT version; required version differs.
    # The old any-.a check wrongly returned present and skipped the downloader.
    prefix = tmp_path / "static_lib"
    _seed_ort(prefix, "onnxruntime-linux-x64-static_lib-1.23.2-glibc2_28")
    res = _present_eval(str(prefix), "onnxruntime-linux-x64-static_lib-", "1.29.0")
    assert res.returncode == 1, "old-version ORT must NOT satisfy the required version"
    assert res.stderr == ""


def test_ort_present_rejects_wrong_platform(tmp_path):
    # Adversarial: cache holds ORT for aarch64 but the target is amd64.
    prefix = tmp_path / "static_lib"
    _seed_ort(prefix, "onnxruntime-linux-aarch64-static_lib-1.29.0-glibc2_28")
    res = _present_eval(str(prefix), "onnxruntime-linux-x64-static_lib-", "1.29.0")
    assert res.returncode == 1, "other-platform ORT must NOT satisfy the target platform"


def test_ort_present_rejects_empty_cache(tmp_path):
    prefix = tmp_path / "static_lib"
    prefix.mkdir()
    res = _present_eval(str(prefix), "onnxruntime-linux-x64-static_lib-", "1.29.0")
    assert res.returncode == 1


def test_ort_present_rejects_stray_top_level_archive(tmp_path):
    # Adversarial: a .a sitting directly in the prefix root (not under a
    # versioned .../lib/ dir) must NOT count as present.
    prefix = tmp_path / "static_lib"
    prefix.mkdir()
    (prefix / "libonnxruntime.a").write_bytes(b"!<arch>\nort")
    res = _present_eval(str(prefix), "onnxruntime-linux-x64-static_lib-", "1.29.0")
    assert res.returncode == 1, "stray top-level .a must not satisfy the versioned gate"


def test_ort_present_ignores_provider_specific_archives(tmp_path):
    # Adversarial: a cuda provider archive inside the correct version dir must be
    # excluded (it is not the base libonnxruntime.a), matching the link-time
    # selection. Only the provider lib present => not present.
    prefix = tmp_path / "static_lib"
    lib = prefix / "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28" / "lib"
    lib.mkdir(parents=True)
    (lib / "libonnxruntime_providers_cuda.a").write_bytes(b"!<arch>\nort")
    res = _present_eval(str(prefix), "onnxruntime-linux-x64-static_lib-", "1.29.0")
    assert res.returncode == 1, "provider-specific archive must not satisfy the base lib gate"


# ── Documentation contract: IDE CGO snippet uses the versioned layout ─────────
# The native libs live under <lib>/v<version>/ (one mounted cache holds several
# versions). The IDE CGO snippet must point there, or a developer following the
# doc links a .a that does not exist at the legacy flat path.


def test_development_doc_ide_cgo_uses_versioned_layout():
    doc = os.path.join(native.REPO_ROOT, "internal", "development.md")
    with open(doc, encoding="utf-8") as file:
        text = file.read()
    # Version-keyed paths must appear for every native lib the IDE snippet links
    # (resolved via v*/ glob, matching download_dev_deps.py / build.sh).
    assert "office_oxide/v" in text, "office_oxide must use v<version> layout"
    assert "pdfium-static/v" in text, "pdfium-static must use v<version> layout"
    assert "pdf_oxide/v" in text, "pdf_oxide must use v<version> layout"
    # The legacy flat layout must be gone so it cannot mislead.
    assert "office_oxide/lib/liboffice_oxide.a" not in text
    assert "pdfium-static/lib/libpdfium.a" not in text


# ── Versioned-extraction fixtures must isolate via env, not the module attr ───
# native_libs_dir() reads RAGFLOW_NATIVE_LIBS_DIR (env); monkeypatching the
# NATIVE_LIBS_DIR module attribute only isolates when that env is unset, so under
# CI (where the env is set to the real cache) the attribute stub is ignored and
# files leak into the real cache. The fixtures must isolate via env.


def test_extract_native_lib_honors_ragflow_native_libs_dir_env(tmp_path, monkeypatch):
    # Adversarial: with the env set (as CI does) AND the module attribute stubbed
    # to a different path, extraction must follow the env, not the attribute.
    monkeypatch.setenv("RAGFLOW_NATIVE_LIBS_DIR", str(tmp_path / "isolated"))
    monkeypatch.setattr(native, "NATIVE_LIBS_DIR", "/attribute-path-ignored")
    archive = tmp_path / "pdfium-linux-x64-static.tgz"
    _make_pdfium_archive(archive, ["lib/libpdfium.a"])
    native.extract_native_lib(str(archive), "pdfium-static", "v7809", ["lib/libpdfium.a"], None)
    assert (tmp_path / "isolated" / "pdfium-static" / "v7809" / "lib" / "libpdfium.a").is_file()
    assert not os.path.exists("/attribute-path-ignored")
