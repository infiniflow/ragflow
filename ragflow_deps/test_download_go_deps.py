# Tests for ragflow_deps/download_go_deps.py ONNX Runtime extraction.
#
# build.sh's build_go() fails fast when libonnxruntime.a is not linked, so these
# tests must guarantee the archive is really landed on disk — above all after an
# ORT version bump, where the zip's version-stamped top-level dir changes but
# static_lib/ still exists from the previous run.
#
# extract_onnxruntime() now takes the *expected top-level dir name* (e.g.
# "onnxruntime-linux-x64-static_lib-1.23.2-glibc2_28") rather than just the
# version string, because the dir name is platform-dependent (linux vs osx,
# amd64 vs arm64) and build.sh's stale-version guard keys on it.

import os
import zipfile

from download_go_deps import (
    _ort_asset,
    extract_onnxruntime,
    has_static_archives,
)


def make_ort_zip(path, dir_name):
    """Build a zip shaped like the upstream ORT release: a version-stamped
    top-level dir holding lib/libonnxruntime.a."""
    member = f"{dir_name}/lib/libonnxruntime.a"
    with zipfile.ZipFile(path, "w") as zf:
        zf.writestr(member, b"!<arch>\nort-payload")
    return path


def test_extracts_on_first_run(tmp_path):
    # static_lib/ does not exist yet: the plain first-run path.
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    _, dir_name = _ort_asset("linux", "amd64")
    zip_name, _ = _ort_asset("linux", "amd64")
    archive = make_ort_zip(tmp_path / zip_name, dir_name)

    assert extract_onnxruntime(str(static_lib), str(archive), dir_name) is True

    version_dir = static_lib / dir_name
    assert version_dir.is_dir()
    assert has_static_archives(str(version_dir))


def test_extracts_after_version_bump_when_static_lib_exists(tmp_path):
    """Regression: a static_lib/ left over from a previous version must not
    suppress extraction of the bumped version. Pruning the stale dir leaves
    static_lib/ in place; if extraction is then skipped no .a is ever landed and
    build.sh's ORT guard rejects the build."""
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    static_lib.mkdir(parents=True)
    stale = static_lib / "onnxruntime-linux-x64-static_lib-1.23.1-glibc2_28"
    (stale / "lib").mkdir(parents=True)
    (stale / "lib" / "libonnxruntime.a").write_bytes(b"old-ort")

    _, dir_name = _ort_asset("linux", "amd64")
    zip_name, _ = _ort_asset("linux", "amd64")
    archive = make_ort_zip(tmp_path / zip_name, dir_name)

    assert extract_onnxruntime(str(static_lib), str(archive), dir_name) is True

    assert not stale.exists(), "stale ORT version should be pruned"
    version_dir = static_lib / dir_name
    assert version_dir.is_dir(), "bumped ORT version was silently not extracted"
    assert has_static_archives(str(version_dir)), "bumped ORT version landed no .a"


def test_skips_when_archive_missing(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    _, dir_name = _ort_asset("linux", "amd64")
    assert extract_onnxruntime(str(static_lib), str(tmp_path / "missing.zip"), dir_name) is False


def test_idempotent_when_matching_version_already_present(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    _, dir_name = _ort_asset("linux", "amd64")
    zip_name, _ = _ort_asset("linux", "amd64")
    archive = make_ort_zip(tmp_path / zip_name, dir_name)

    assert extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    before = sorted(os.listdir(static_lib))
    # Same dir already present: should short-circuit, not re-extract or prune.
    assert extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    assert sorted(os.listdir(static_lib)) == before


def test_prunes_foreign_platform_dir(tmp_path):
    """A dir from a different platform/version must be pruned so build.sh's
    `find ... -name '*.a'` does not link two ORT builds."""
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    static_lib.mkdir(parents=True)
    foreign = static_lib / "onnxruntime-osx-arm64-static_lib-1.23.2"
    (foreign / "lib").mkdir(parents=True)
    (foreign / "lib" / "libonnxruntime.a").write_bytes(b"mac-ort")

    _, dir_name = _ort_asset("linux", "amd64")
    zip_name, _ = _ort_asset("linux", "amd64")
    archive = make_ort_zip(tmp_path / zip_name, dir_name)

    assert extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    assert not foreign.exists(), "foreign-platform ORT dir should be pruned"
    assert (static_lib / dir_name).is_dir()


def test_ort_asset_names_match_upstream_layout():
    # Spot-check the platform-specific asset/dir names so the download script and
    # build.sh's stale-version guard stay aligned with the upstream release.
    assert _ort_asset("linux", "amd64") == (
        "onnxruntime-linux-x64-static_lib-1.23.2-glibc2_28.zip",
        "onnxruntime-linux-x64-static_lib-1.23.2-glibc2_28",
    )
    assert _ort_asset("linux", "arm64") == (
        "onnxruntime-linux-aarch64-static_lib-1.23.2-glibc2_28.zip",
        "onnxruntime-linux-aarch64-static_lib-1.23.2-glibc2_28",
    )
    assert _ort_asset("darwin", "arm64") == (
        "onnxruntime-osx-arm64-static_lib-1.23.2.zip",
        "onnxruntime-osx-arm64-static_lib-1.23.2",
    )
