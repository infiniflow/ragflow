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

    # Isolate /opt (SYSTEM_DEPS) and $HOME so the outcome does not depend on the
    # runner's pre-baked native libs. With no /opt copy, a mismatched user cache
    # must fail loud (never auto-download). This is what guards against
    # re-introducing the silent download, independent of whether CI pre-bakes
    # office_oxide into /opt (check_office_oxide_deps now reseeds from /opt when
    # present, so the recovery path is covered by the *_reseed_* tests).
    opt = tmp_path / "opt" / "ragflow-native-libs"
    script = f'HOME="{tmp_path / "home"}"\nSYSTEM_DEPS="{opt}"\nOFFICE_OXIDE_PREFIX="{cache}"\ncheck_office_oxide_deps\n'
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


# ── office_oxide: stale user cache + correct /opt => recover via reseed ────────
# Mirrors the onnxruntime recovery path. On a self-hosted runner where $HOME
# persists across jobs, an older CI image may have left a wrong-version
# office_oxide under ~/ragflow-native-libs/office_oxide; build.sh must overwrite
# it from the /opt pre-bake instead of failing loud on the stale version.
def _make_opt_office_oxide(opt_root, version="0.1.16"):
    lib = opt_root / "office_oxide" / "lib"
    inc = opt_root / "office_oxide" / "include" / "office_oxide_c"
    lib.mkdir(parents=True)
    inc.mkdir(parents=True)
    (lib / "liboffice_oxide.a").write_text(f"{version}\n")
    (inc / "office_oxide.h").write_text("")
    return opt_root


def test_office_oxide_reseed_from_opt_on_stale_cache(tmp_path):
    # Version must match go.mod's office_oxide pin (single source of truth).
    opt = _make_opt_office_oxide(tmp_path / "opt" / "ragflow-native-libs")

    user = tmp_path / "user" / "ragflow-native-libs" / "office_oxide"
    (user / "lib").mkdir(parents=True)
    (user / "include" / "office_oxide_c").mkdir(parents=True)
    (user / "lib" / "liboffice_oxide.a").write_text("9.9.9\n")  # stale
    (user / "include" / "office_oxide_c" / "office_oxide.h").write_text("")

    script = f'HOME="{tmp_path / "home"}"\nSYSTEM_DEPS="{opt}"\nOFFICE_OXIDE_PREFIX="{user}"\ncheck_office_oxide_deps\n'
    proc = _run(script)
    # Recovers from /opt: stale gone, correct version present, build proceeds.
    assert proc.returncode == 0, proc.stdout + proc.stderr
    assert (user / "lib" / "liboffice_oxide.a").read_text().strip() == "0.1.16"
    # Stale content must not linger (no silent wrong-version link).
    assert "9.9.9" not in (user / "lib" / "liboffice_oxide.a").read_text()


def test_office_oxide_no_opt_still_fails_loud(tmp_path):
    opt = tmp_path / "opt" / "ragflow-native-libs"  # empty: no office_oxide baked
    user = tmp_path / "user" / "ragflow-native-libs" / "office_oxide"
    (user / "lib").mkdir(parents=True)
    (user / "include" / "office_oxide_c").mkdir(parents=True)
    (user / "lib" / "liboffice_oxide.a").write_text("9.9.9\n")  # stale, no /opt to recover from
    (user / "include" / "office_oxide_c" / "office_oxide.h").write_text("")

    script = f'HOME="{tmp_path / "home"}"\nSYSTEM_DEPS="{opt}"\nOFFICE_OXIDE_PREFIX="{user}"\ncheck_office_oxide_deps\n'
    proc = _run(script)
    # Hard fail when /opt has no copy — never silently link the stale 9.9.9.
    assert proc.returncode != 0, proc.stdout + proc.stderr
    assert "9.9.9" in (user / "lib" / "liboffice_oxide.a").read_text()  # untouched


# ── pdf_oxide: stale user cache + correct /opt => recover via reseed ───────────
def _make_opt_pdf_oxide(opt_root, os_="linux", arch="amd64"):
    lib = opt_root / "pdf_oxide" / "lib" / f"{os_}_{arch}"
    lib.mkdir(parents=True)
    (lib / "libpdf_oxide.a").write_text("pdf_oxide 0.3.73\n")
    return opt_root


def test_pdf_oxide_reseed_from_opt_on_stale_cache(tmp_path):
    opt = _make_opt_pdf_oxide(tmp_path / "opt" / "ragflow-native-libs")

    user = tmp_path / "user" / "ragflow-native-libs" / "pdf_oxide"
    (user / "lib" / "linux_amd64").mkdir(parents=True)
    (user / "lib" / "linux_amd64" / "libpdf_oxide.a").write_text("pdf_oxide 9.9.9\n")  # stale

    script = f'HOME="{tmp_path / "home"}"\nTARGET_OS=linux\nTARGET_ARCH=amd64\nSYSTEM_DEPS="{opt}"\nPDF_OXIDE_PREFIX="{user}"\ncheck_pdf_oxide_deps\n'
    proc = _run(script)
    assert proc.returncode == 0, proc.stdout + proc.stderr
    assert (user / "lib" / "linux_amd64" / "libpdf_oxide.a").read_text().strip() == "pdf_oxide 0.3.73"
    assert "9.9.9" not in (user / "lib" / "linux_amd64" / "libpdf_oxide.a").read_text()


def test_pdf_oxide_no_opt_still_fails_loud(tmp_path):
    opt = tmp_path / "opt" / "ragflow-native-libs"  # empty: no pdf_oxide baked
    user = tmp_path / "user" / "ragflow-native-libs" / "pdf_oxide"
    (user / "lib" / "linux_amd64").mkdir(parents=True)
    (user / "lib" / "linux_amd64" / "libpdf_oxide.a").write_text("pdf_oxide 9.9.9\n")  # stale

    script = f'HOME="{tmp_path / "home"}"\nTARGET_OS=linux\nTARGET_ARCH=amd64\nSYSTEM_DEPS="{opt}"\nPDF_OXIDE_PREFIX="{user}"\ncheck_pdf_oxide_deps\n'
    proc = _run(script)
    assert proc.returncode != 0, proc.stdout + proc.stderr
    assert "9.9.9" in (user / "lib" / "linux_amd64" / "libpdf_oxide.a").read_text()  # untouched


# ── CI wiring: the new native-lib tests must actually run in CI ───────────────
# Regression guard for review finding #1: sep-tests.yml only ran
# test_download_dev_deps.py, so test_native_lib_version.py and
# test_build_sh_seeding.py were never executed in CI. This pins the "Run deps
# regression guards" step's pytest command to include both new files.
def test_ci_runs_new_native_lib_tests():
    wf = os.path.join(REPO, ".github", "workflows", "sep-tests.yml")
    with open(wf) as fh:
        text = fh.read()
    idx = text.index("Run deps regression guards")
    end = text.find("\n      - name:", idx)
    block = text[idx:end] if end != -1 else text[idx:]
    pytest_line = next((ln for ln in block.splitlines() if "pytest" in ln and "tools/scripts" in ln), "")
    assert "tools/scripts/test_native_lib_version.py" in pytest_line, "CI must run the new native_lib_version tests:\n" + block
    assert "tools/scripts/test_build_sh_seeding.py" in pytest_line, "CI must run the new build_sh seeding tests:\n" + block


# ── Dockerfile: the stage that runs build.sh must ship native_lib_version.sh ───
# build.sh sources tools/scripts/native_lib_version.sh at the top (see above), so
# the production Dockerfile's build.sh-running stage must COPY it into the image.
# Without it, `./build.sh --go` in the image build aborts on the `source` and the
# server image cannot be built. Regression guard for the native-lib versioning
# change (the file is new in that change).
def _dockerfile_build_stage_text():
    dockerfile = os.path.join(REPO, "Dockerfile")
    with open(dockerfile) as fh:
        lines = fh.readlines()
    # The stage that runs build.sh: first non-comment line that executes
    # `./build.sh` (RUN may be a multi-line continuation, and the string also
    # appears in comments, so require the `./build.sh` execution form).
    build_idx = next(i for i, ln in enumerate(lines) if ln.strip() and not ln.strip().startswith("#") and "./build.sh" in ln)
    # Most recent FROM opening that stage.
    from_idx = max(
        (i for i in range(build_idx, -1, -1) if lines[i].strip().startswith("FROM")),
        default=-1,
    )
    assert from_idx != -1, "could not locate the build.sh stage in Dockerfile"
    return "".join(lines[from_idx : build_idx + 1])


def test_dockerfile_ships_native_lib_version_to_build_stage():
    stage = _dockerfile_build_stage_text()
    assert ("tools/scripts/native_lib_version.sh" in stage) or ("COPY tools/scripts" in stage), (
        "The Dockerfile stage that runs build.sh must COPY tools/scripts/native_lib_version.sh (build.sh sources it at the top):\n" + stage
    )


def test_build_sh_sources_native_lib_version():
    # Coupling guard: if build.sh stops sourcing the file, the Dockerfile COPY is moot.
    with open(os.path.join(REPO, "build.sh")) as fh:
        build_sh = fh.read()
    assert "tools/scripts/native_lib_version.sh" in build_sh


def test_dockerignore_does_not_exclude_tools_scripts():
    # A COPY tools/scripts would fail at docker build if .dockerignore excluded it.
    dockerignore = os.path.join(REPO, ".dockerignore")
    with open(dockerignore) as fh:
        # Only inspect real exclude lines; comments may legitimately mention the path.
        exclude_lines = [ln.strip() for ln in fh if ln.strip() and not ln.strip().startswith("#")]
    for pat in exclude_lines:
        if pat == "tools/scripts" or pat == "tools/" or pat.startswith("tools/"):
            raise AssertionError(f".dockerignore excludes '{pat}'; the Dockerfile COPYs tools/scripts for build.sh")
