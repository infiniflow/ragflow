# Tests for tools/scripts/native_lib_version.sh::select_native_lib_version.
#
# The function maps a (dep_name, version, system_root) triple to the directory
# build.sh should seed a native static library from. A multi-version CI runner
# image bakes each version under <system_root>/<dep>/v<version>; the function
# must prefer that version-keyed dir over the legacy flat layout, and must never
# return a different (newer) version than the one go.mod pins.
#
# These tests drive the real bash function via subprocess so they exercise the
# exact code build.sh sources.

import os
import subprocess
import tempfile

SCRIPT = os.path.join(os.path.dirname(__file__), "native_lib_version.sh")


def _resolve(dep, version, root):
    proc = subprocess.run(
        ["bash", "-c", f'source "{SCRIPT}" && select_native_lib_version "{dep}" "{version}" "{root}"'],
        capture_output=True,
        text=True,
        check=False,
    )
    return proc.stdout.strip(), proc.returncode


def _mk_version(root, dep, version, marker="lib__marker"):
    d = os.path.join(root, dep, f"v{version}")
    os.makedirs(d, exist_ok=True)
    with open(os.path.join(d, marker), "w") as fh:
        fh.write("")
    return d


def test_version_keyed_preferred_over_flat():
    with tempfile.TemporaryDirectory() as root:
        vdir = _mk_version(root, "office_oxide", "0.1.12")
        os.makedirs(os.path.join(root, "office_oxide"), exist_ok=True)  # legacy flat
        out, rc = _resolve("office_oxide", "0.1.12", root)
        assert rc == 0
        assert out == vdir


def test_version_mismatch_falls_back_to_flat():
    with tempfile.TemporaryDirectory() as root:
        _mk_version(root, "office_oxide", "0.1.16")
        flat = os.path.join(root, "office_oxide")
        os.makedirs(flat, exist_ok=True)
        out, rc = _resolve("office_oxide", "0.1.12", root)
        assert rc == 0
        assert out == flat


def test_no_local_copy_returns_empty():
    with tempfile.TemporaryDirectory() as root:
        out, rc = _resolve("office_oxide", "0.1.12", root)
        assert rc != 0
        assert out == ""


def test_adversarial_pinned_not_latest():
    # Two versions baked; requesting the OLD one must return the OLD dir, never
    # the NEW (latest) one.
    with tempfile.TemporaryDirectory() as root:
        old = _mk_version(root, "pdf_oxide", "0.3.73")
        new = _mk_version(root, "pdf_oxide", "0.3.78")
        out, rc = _resolve("pdf_oxide", "0.3.73", root)
        assert rc == 0
        assert out == old
        assert out != new


# ── go_module_version: read a Go module's pinned version from go.mod ──────────
def _module_version(module, go_mod_text):
    with tempfile.TemporaryDirectory() as root:
        with open(os.path.join(root, "go.mod"), "w") as fh:
            fh.write(go_mod_text)
        proc = subprocess.run(
            ["bash", "-c", f'source "{SCRIPT}" && go_module_version "{module}" "{root}"'],
            capture_output=True,
            text=True,
            check=False,
        )
        return proc.stdout.strip(), proc.returncode


def test_go_module_version_reads_pdf_oxide_from_gomod():
    out, rc = _module_version(
        "github.com/yfedoseev/pdf_oxide/go",
        "module example.com/x\n\ngo 1.23\n\nrequire github.com/yfedoseev/pdf_oxide/go v0.3.73\n",
    )
    assert rc == 0
    assert out == "0.3.73"


def test_go_module_version_reads_office_oxide_from_gomod():
    out, rc = _module_version(
        "github.com/yfedoseev/office_oxide/go",
        "require github.com/yfedoseev/office_oxide/go v0.1.12\n",
    )
    assert rc == 0
    assert out == "0.1.12"


def test_go_module_version_adversarial_pinned_not_latest():
    # A newer version string appears elsewhere (stray comment + an unrelated dep);
    # the reader must return the pinned require, never the stray newer string.
    out, rc = _module_version(
        "github.com/yfedoseev/pdf_oxide/go",
        "require github.com/yfedoseev/pdf_oxide/go v0.3.73\n# note: pdf_oxide 0.3.78 is now available\nrequire github.com/other/x v0.3.78\n",
    )
    assert rc == 0
    assert out == "0.3.73"  # not 0.3.78


def test_go_module_version_missing_returns_error():
    out, rc = _module_version(
        "github.com/yfedoseev/pdf_oxide/go",
        "module example.com/x\n",
    )
    assert rc != 0
    assert out == ""


# ── reseed_dep_from_system: overwrite a stale user cache from /opt ────────────
# On a self-hosted runner $HOME persists across jobs, so a stale native-lib dir
# left by an older CI image can shadow the correct /opt pre-bake. This function
# is the recovery path: copy the pinned version out of /opt over the user cache
# so build.sh never links a wrong-version .a. It must prefer the version-keyed
# subdir, fall back to flat, never pick a different version, and leave the cache
# untouched when /opt has nothing.
def _reseed(dep, version, system_root, user_cache_root):
    proc = subprocess.run(
        ["bash", "-c", f'source "{SCRIPT}" && reseed_dep_from_system "{dep}" "{version}" "{system_root}" "{user_cache_root}"'],
        capture_output=True,
        text=True,
        check=False,
    )
    return proc.returncode


def _mk_version_distinct(root, dep, version):
    d = os.path.join(root, dep, f"v{version}")
    os.makedirs(d, exist_ok=True)
    with open(os.path.join(d, f"version__{version}"), "w") as fh:
        fh.write("")
    return d


def test_reseed_copies_version_keyed_from_opt():
    with tempfile.TemporaryDirectory() as system_root:
        _mk_version_distinct(system_root, "office_oxide", "0.1.16")
        with tempfile.TemporaryDirectory() as user_root:
            user_cache = os.path.join(user_root, "office_oxide")
            os.makedirs(user_cache)
            assert _reseed("office_oxide", "0.1.16", system_root, user_cache) == 0
            assert os.path.exists(os.path.join(user_cache, "version__0.1.16"))


def test_reseed_falls_back_to_flat_opt():
    with tempfile.TemporaryDirectory() as system_root:
        flat = os.path.join(system_root, "office_oxide")
        os.makedirs(flat)
        with open(os.path.join(flat, "version__0.1.12"), "w") as fh:
            fh.write("")
        with tempfile.TemporaryDirectory() as user_root:
            user_cache = os.path.join(user_root, "office_oxide")
            os.makedirs(user_cache)
            assert _reseed("office_oxide", "0.1.12", system_root, user_cache) == 0
            assert os.path.exists(os.path.join(user_cache, "version__0.1.12"))


def test_reseed_no_opt_returns_1_and_leaves_cache():
    with tempfile.TemporaryDirectory() as system_root, tempfile.TemporaryDirectory() as user_root:
        user_cache = os.path.join(user_root, "office_oxide")
        os.makedirs(user_cache)
        with open(os.path.join(user_cache, "stale"), "w") as fh:
            fh.write("")
        assert _reseed("office_oxide", "0.1.12", system_root, user_cache) == 1
        # cache untouched: stale marker still present, no version file copied
        assert os.path.exists(os.path.join(user_cache, "stale"))
        assert not os.path.exists(os.path.join(user_cache, "version__0.1.12"))


def test_reseed_adversarial_pinned_not_latest():
    # Two versions baked; requesting the OLD one must copy the OLD dir, never
    # the NEW (latest) sibling.
    with tempfile.TemporaryDirectory() as system_root:
        _mk_version_distinct(system_root, "pdf_oxide", "0.3.73")
        _mk_version_distinct(system_root, "pdf_oxide", "0.3.78")
        with tempfile.TemporaryDirectory() as user_root:
            user_cache = os.path.join(user_root, "pdf_oxide")
            os.makedirs(user_cache)
            assert _reseed("pdf_oxide", "0.3.73", system_root, user_cache) == 0
            assert os.path.exists(os.path.join(user_cache, "version__0.3.73"))
            assert not os.path.exists(os.path.join(user_cache, "version__0.3.78"))


def test_reseed_overwrites_stale_cache():
    with tempfile.TemporaryDirectory() as system_root:
        _mk_version_distinct(system_root, "office_oxide", "0.1.16")
        with tempfile.TemporaryDirectory() as user_root:
            user_cache = os.path.join(user_root, "office_oxide")
            os.makedirs(user_cache)
            with open(os.path.join(user_cache, "stale"), "w") as fh:
                fh.write("")
            assert _reseed("office_oxide", "0.1.16", system_root, user_cache) == 0
            assert os.path.exists(os.path.join(user_cache, "version__0.1.16"))
            # stale content must be gone (overwritten, not merged)
            assert not os.path.exists(os.path.join(user_cache, "stale"))


def test_reseed_onnxruntime_nested_layout():
    # /opt/onnxruntime/static_lib/lib/libonnxruntime.a ; the user cache root is
    # ~/ragflow-native-libs/onnxruntime (the static_lib child lives under it).
    # reseed must copy static_lib/ into the cache root, not nest it.
    with tempfile.TemporaryDirectory() as system_root:
        src_static = os.path.join(system_root, "onnxruntime", "static_lib", "lib")
        os.makedirs(src_static)
        with open(os.path.join(src_static, "libonnxruntime.a"), "w") as fh:
            fh.write("")
        with tempfile.TemporaryDirectory() as user_root:
            user_cache = os.path.join(user_root, "onnxruntime")  # dep root
            os.makedirs(user_cache)
            assert _reseed("onnxruntime", "1.29.0", system_root, user_cache) == 0
            assert os.path.exists(os.path.join(user_cache, "static_lib", "lib", "libonnxruntime.a"))
