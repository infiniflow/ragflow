# Tests for ragflow_deps/download_go_deps.py ONNX Runtime extraction.
#
# build.sh's build_go() fails fast when libonnxruntime.a is not linked, so these
# tests must guarantee the archive is really landed on disk — above all after an
# ORT version bump, where the zip's version-stamped top-level dir changes but
# static_lib/ still exists from the previous run.

import os
import zipfile

from download_go_deps import extract_onnxruntime, has_static_archives


def make_ort_zip(path, version):
    """Build a zip shaped like the upstream ORT release: a version-stamped
    top-level dir holding lib/libonnxruntime.a."""
    member = f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28/lib/libonnxruntime.a"
    with zipfile.ZipFile(path, "w") as zf:
        zf.writestr(member, b"!<arch>\nort-payload")
    return path


def test_extracts_on_first_run(tmp_path):
    # static_lib/ does not exist yet: the plain first-run path.
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    version = "1.23.2"
    archive = make_ort_zip(tmp_path / f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28.zip", version)

    assert extract_onnxruntime(str(static_lib), str(archive), version) is True

    version_dir = static_lib / f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28"
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

    version = "1.23.2"
    archive = make_ort_zip(tmp_path / f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28.zip", version)

    assert extract_onnxruntime(str(static_lib), str(archive), version) is True

    assert not stale.exists(), "stale ORT version should be pruned"
    version_dir = static_lib / f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28"
    assert version_dir.is_dir(), "bumped ORT version was silently not extracted"
    assert has_static_archives(str(version_dir)), "bumped ORT version landed no .a"


def test_skips_when_archive_missing(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    assert extract_onnxruntime(str(static_lib), str(tmp_path / "missing.zip"), "1.23.2") is False


def test_idempotent_when_matching_version_already_present(tmp_path):
    static_lib = tmp_path / "onnxruntime" / "static_lib"
    version = "1.23.2"
    archive = make_ort_zip(tmp_path / f"onnxruntime-linux-x64-static_lib-{version}-glibc2_28.zip", version)

    assert extract_onnxruntime(str(static_lib), str(archive), version) is True
    before = sorted(os.listdir(static_lib))
    assert extract_onnxruntime(str(static_lib), str(archive), version) is True
    assert sorted(os.listdir(static_lib)) == before
