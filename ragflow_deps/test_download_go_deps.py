# Tests for ragflow_deps/download_go_deps.py ONNX Runtime extraction.
#
# build.sh's build_go() fails fast when libonnxruntime.a is not linked, so these
# tests must guarantee the archive is really landed on disk — above all after an
# ORT version bump, where the zip's version-stamped top-level dir changes but
# static_lib/ still exists from the previous run.
#
# The release zip from infiniflow/ragflow-build wraps everything in a top-level
# dir named onnxruntime-v{version}-{target}; extract_onnxruntime() must rename
# it to the build.sh-expected onnxruntime-<os>-<arch>-static_lib-{version}... so
# every consumer shares one name convention. These tests pin that rename.
#
# extract_onnxruntime() takes the *expected dir name* rather than just the
# version string, because that name is platform-dependent (linux vs osx, amd64
# vs arm64) and build.sh's stale-version guard keys on it.

import os
import zipfile

from download_go_deps import (
    ORT_VERSION,
    _ort_asset,
    _ort_asset_name,
    _ort_extracted_dir,
    _ort_normalized_dir,
    _ort_target,
    extract_onnxruntime,
    has_static_archives,
    host_platform,
)


def make_ort_zip(path, dir_name):
    """Build a zip shaped like the infiniflow/ragflow-build ORT release: a
    top-level dir holding lib/libonnxruntime.a. extract_onnxruntime() must
    rename it to the build.sh-expected dir (see _ort_normalized_dir)."""
    with zipfile.ZipFile(path, "w") as zf:
        zf.writestr(f"{dir_name}/lib/libonnxruntime.a", b"!<arch>\nort-payload")
    return path


def ort_archive(tmp_path, goos="linux", goarch="amd64", version=ORT_VERSION):
    """Return (zip_path, normalized_dir) for `goos/goarch` at `version`."""
    target = _ort_target(goos, goarch)
    zip_path = tmp_path / _ort_asset_name(version, target)
    return make_ort_zip(zip_path, _ort_extracted_dir(version, target)), _ort_normalized_dir(version, goos, goarch)


def test_extracts_on_first_run(tmp_path):
    # static_lib/ does not exist yet: the plain first-run path.
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    archive, dir_name = ort_archive(tmp_path)

    assert extract_onnxruntime(str(static_lib), str(archive), dir_name) is True

    assert not (static_lib / _ort_extracted_dir(ORT_VERSION, _ort_target("linux", "amd64"))).exists(), (
        "release zip top-level dir must be renamed away"
    )
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
    stale = static_lib / _ort_normalized_dir("1.28.0", "linux", "amd64")
    (stale / "lib").mkdir(parents=True)
    (stale / "lib" / "libonnxruntime.a").write_bytes(b"old-ort")

    archive, dir_name = ort_archive(tmp_path)

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
    archive, dir_name = ort_archive(tmp_path)

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
    foreign = static_lib / _ort_normalized_dir(ORT_VERSION, "darwin", "arm64")
    (foreign / "lib").mkdir(parents=True)
    (foreign / "lib" / "libonnxruntime.a").write_bytes(b"mac-ort")

    archive, dir_name = ort_archive(tmp_path)

    assert extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    assert not foreign.exists(), "foreign-platform ORT dir should be pruned"
    assert (static_lib / dir_name).is_dir()


def test_ort_asset_names_match_upstream_layout():
    # Spot-check the platform-specific asset/dir names so the download script and
    # build.sh's stale-version guard stay aligned with the upstream release.
    assert _ort_asset("linux", "amd64") == (
        f"onnxruntime-v{ORT_VERSION}-linux-x86_64.zip",
        f"onnxruntime-linux-x64-static_lib-{ORT_VERSION}-glibc2_28",
    )
    assert _ort_asset("linux", "arm64") == (
        f"onnxruntime-v{ORT_VERSION}-linux-aarch64.zip",
        f"onnxruntime-linux-aarch64-static_lib-{ORT_VERSION}-glibc2_28",
    )
    assert _ort_asset("darwin", "arm64") == (
        f"onnxruntime-v{ORT_VERSION}-osx-arm64.zip",
        f"onnxruntime-osx-arm64-static_lib-{ORT_VERSION}",
    )
    # No osx-x86_64 asset is published; Intel Macs use the universal archive.
    assert _ort_asset("darwin", "amd64") == (
        f"onnxruntime-v{ORT_VERSION}-osx-universal.zip",
        f"onnxruntime-osx-universal-static_lib-{ORT_VERSION}",
    )


def test_extracts_darwin_archive(tmp_path):
    # The macOS zip carries no glibc suffix in its normalized dir name; the
    # rename step must still land it where build.sh expects.
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    archive, dir_name = ort_archive(tmp_path, goos="darwin", goarch="arm64")

    assert extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    assert (static_lib / dir_name / "lib" / "libonnxruntime.a").is_file()


def test_host_platform_normalizes_aliases(monkeypatch):
    # build.sh's detect_target_platform accepts Linux/Darwin + x86_64/amd64
    # aliases; host_platform must normalize the same way or it KeyErrors when
    # indexing the per-platform asset maps.
    monkeypatch.setenv("RAGFLOW_TARGET_OS", "Linux")
    monkeypatch.setenv("RAGFLOW_TARGET_ARCH", "x86_64")
    assert host_platform() == ("linux", "amd64")
    monkeypatch.setenv("RAGFLOW_TARGET_OS", "Darwin")
    monkeypatch.setenv("RAGFLOW_TARGET_ARCH", "aarch64")
    assert host_platform() == ("darwin", "arm64")


def test_prunes_foreign_platform_dir_on_short_circuit(tmp_path):
    """When the expected ORT dir is already extracted, the short-circuit return
    must still prune a co-resident foreign-platform dir (defense-in-depth;
    build.sh's ort_dir_prefix guard also filters by platform at link time)."""
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    dir_name = _ort_normalized_dir(ORT_VERSION, "linux", "amd64")
    expected = static_lib / dir_name
    (expected / "lib").mkdir(parents=True)
    (expected / "lib" / "libonnxruntime.a").write_bytes(b"linux-ort")
    foreign = static_lib / _ort_normalized_dir(ORT_VERSION, "darwin", "arm64")
    (foreign / "lib").mkdir(parents=True)
    (foreign / "lib" / "libonnxruntime.a").write_bytes(b"mac-ort")

    archive, _ = ort_archive(tmp_path)
    assert extract_onnxruntime(str(static_lib), str(archive), dir_name) is True
    assert not foreign.exists(), "foreign dir must be pruned even on short-circuit"
    assert expected.is_dir()
