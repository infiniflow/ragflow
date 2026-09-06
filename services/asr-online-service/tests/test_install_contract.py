from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def test_install_bat_is_cwd_independent_and_has_cmd_contract() -> None:
    text = (ROOT / "install.bat").read_text(encoding="utf-8")

    assert "title asr-online-service" in text
    assert 'pushd "%~dp0"' in text
    assert 'set "TOOLS_DIR=%CD%\\tools"' in text
    assert 'if not exist ".env" (' in text
    assert 'copy /y ".env.example" ".env"' in text
    lock_cleanup = "if exist poetry.lock del /f /q poetry.lock"
    assert lock_cleanup in text
    assert text.index(lock_cleanup) < text.index("poetry install --all-extras")
    assert "Failed to remove poetry.lock" in text


def test_install_bat_has_tools_sanity_checks_and_poetry_fallback() -> None:
    text = (ROOT / "install.bat").read_text(encoding="utf-8")

    assert '"%FFMPEG_EXE%" -version >nul' in text
    assert '"%SOX_EXE%" --version >nul' in text
    assert "poetry install --all-extras" in text
    assert "poetry install --all-extras failed, falling back to core dependencies only." in text
    assert "poetry install" in text


def test_prefetch_wrapper_contract() -> None:
    text = (ROOT / "prefetch_models.bat").read_text(encoding="utf-8")

    assert "title asr-online-service prefetch models" in text
    assert 'pushd "%~dp0"' in text
    assert "poetry run python -m asr_service.tools.prefetch_models %*" in text
