# Tests for build.sh native-lib seeding behaviour:
#   * office_oxide must fail loud (no network) on version mismatch (no-autodownload port)
#   * onnxruntime / pdfium must recover from a stale user cache by re-seeding from
#     the /opt runner-image pre-bake, and must NOT silently link a wrong version.
#
# These drive the real build.sh by sourcing it in a subprocess and overriding the
# relevant globals (SYSTEM_DEPS, *_PREFIX) plus a stubbed `uv` so no network is
# ever touched.

import os
import subprocess

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BUILD_SH = os.path.join(REPO, "build.sh")


def _run(script_body, path_extra=None):
    full = f'source "{BUILD_SH}"\n{script_body}'
    env = os.environ.copy()
    if path_extra:
        env["PATH"] = f"{path_extra}:{env['PATH']}"
    return subprocess.run(["bash", "-c", full], capture_output=True, text=True, env=env, check=False)


# ── office_oxide: mismatch must fail loud and NEVER call `uv` ──────────────────
def test_office_oxide_mismatch_fails_loud_no_download(tmp_path):
    uv_dir = tmp_path / "uvbin"
    uv_dir.mkdir()
    marker = tmp_path / "uv_called"
    (uv_dir / "uv").write_text(f'#!/bin/bash\n touch "{marker}"\n')
    os.chmod(uv_dir / "uv", 0o755)

    cache = tmp_path / "cache" / "office_oxide"
    (cache / "lib").mkdir(parents=True)
    (cache / "include" / "office_oxide_c").mkdir(parents=True)
    # A wrong version embedded so the strings|grep version check fails.
    (cache / "lib" / "liboffice_oxide.a").write_text("9.9.9\n")
    (cache / "include" / "office_oxide_c" / "office_oxide.h").write_text("")

    script = f'OFFICE_OXIDE_PREFIX="{cache}"\ncheck_office_oxide_deps\n'
    proc = _run(script, path_extra=str(uv_dir))
    assert proc.returncode != 0, proc.stdout + proc.stderr
    # Adversarial: build.sh must NOT have attempted a network download.
    assert not marker.exists(), "office_oxide must not auto-download on mismatch"


# ── onnxruntime: stale user cache + correct /opt => recover via reseed ─────────
def _make_opt_onnxruntime(opt_root):
    static = opt_root / "onnxruntime" / "static_lib" / "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28" / "lib"
    static.mkdir(parents=True)
    (static / "libonnxruntime.a").write_text("ort-1.29.0")
    (static / "libonnxruntime_providers.a").write_text("ort-1.29.0")
    return opt_root


def test_onnxruntime_reseed_from_opt_on_stale_cache(tmp_path):
    opt = _make_opt_onnxruntime(tmp_path / "opt" / "ragflow-native-libs")

    user = tmp_path / "user" / "ragflow-native-libs"
    stale = user / "onnxruntime" / "static_lib" / "onnxruntime-linux-x64-static_lib-1.23.2-glibc2_28" / "lib"
    stale.mkdir(parents=True)
    (stale / "libonnxruntime.a").write_text("stale-1.23.2")

    script = f'SYSTEM_DEPS="{opt}"\nONNXRUNTIME_STATIC_PREFIX="{user}/onnxruntime/static_lib"\ncheck_onnxruntime_deps\n'
    proc = _run(script)
    assert proc.returncode == 0, proc.stdout + proc.stderr
    # Cache now holds the /opt version, and the stale version is gone (no silent link).
    assert (user / "onnxruntime" / "static_lib" / "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28" / "lib" / "libonnxruntime.a").exists()
    assert not (stale / "libonnxruntime.a").exists()


def test_onnxruntime_no_opt_still_fails_loud(tmp_path):
    opt = tmp_path / "opt" / "ragflow-native-libs"  # empty: no onnxruntime baked
    user = tmp_path / "user" / "ragflow-native-libs"
    stale = user / "onnxruntime" / "static_lib" / "onnxruntime-linux-x64-static_lib-1.23.2-glibc2_28" / "lib"
    stale.mkdir(parents=True)
    (stale / "libonnxruntime.a").write_text("stale-1.23.2")

    script = f'SYSTEM_DEPS="{opt}"\nONNXRUNTIME_STATIC_PREFIX="{user}/onnxruntime/static_lib"\ncheck_onnxruntime_deps\n'
    proc = _run(script)
    # Hard fail when /opt has no copy — never silently link the stale 1.23.2.
    assert proc.returncode != 0, proc.stdout + proc.stderr
    assert (stale / "libonnxruntime.a").exists()  # untouched stale cache


# ── pdfium: missing user cache + /opt present => recover via reseed ────────────
def test_pdfium_reseed_from_opt_on_missing(tmp_path):
    opt = tmp_path / "opt" / "ragflow-native-libs"
    (opt / "pdfium-static" / "lib").mkdir(parents=True)
    (opt / "pdfium-static" / "lib" / "libpdfium.a").write_text("pdfium")

    user = tmp_path / "user" / "ragflow-native-libs" / "pdfium-static"
    user.mkdir(parents=True)  # empty: no lib yet

    script = f'SYSTEM_DEPS="{opt}"\nPDFIUM_STATIC_PREFIX="{user}"\nPDFIUM_STATIC_VERSION="7809"\ncheck_pdfium_deps\n'
    proc = _run(script)
    assert proc.returncode == 0, proc.stdout + proc.stderr
    assert (user / "lib" / "libpdfium.a").exists()
