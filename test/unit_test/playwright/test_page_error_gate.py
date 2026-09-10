"""Prove that browser diagnostics affect the real pytest result, without a browser."""

import os
from pathlib import Path
import subprocess
import sys

import pytest


ROOT = Path(__file__).resolve().parents[3]


@pytest.mark.parametrize(
    "fixture_name,page_errors,console_errors,allowed_patterns,body,expected_exit,expected_text",
    [
        ("page", ["uncaught error"], [], [], "pass", 1, "unhandled browser exception"),
        ("flow_page", ["uncaught error"], [], [], "pass", 1, "unhandled browser exception"),
        ("page", [], ["HTTP 403 from negative case"], [], "pass", 0, "1 passed"),
        ("page", [], [], [], "pass", 0, "1 passed"),
        (
            "page",
            ["pageerror: ResizeObserver loop completed with undelivered notifications."],
            [],
            [],
            "pass",
            0,
            "1 passed",
        ),
        (
            "page",
            ["pageerror: /127.0.0.1:1234/api/v1/system/config due to access control checks."],
            [],
            [r"^/127\.0\.0\.1:\d+/api/v1/system/config due to access control checks\.$"],
            "pass",
            0,
            "1 passed",
        ),
        ("page", ["uncaught error"], [], [], "assert False, 'original failure'", 1, "original failure"),
    ],
)
def test_page_error_gate(
    tmp_path,
    fixture_name,
    page_errors,
    console_errors,
    allowed_patterns,
    body,
    expected_exit,
    expected_text,
):
    (tmp_path / "pytest.ini").write_text("[pytest]\n", encoding="utf-8")
    (tmp_path / "conftest.py").write_text(
        "from types import SimpleNamespace\n"
        "import pytest\n"
        "@pytest.fixture\n"
        f"def {fixture_name}():\n"
        f"    return SimpleNamespace(_diag={{'page_errors': {page_errors!r}, "
        f"'console_errors': {console_errors!r}, "
        f"'allowed_page_error_patterns': {allowed_patterns!r}}})\n"
        # The real autouse artifact fixture expects a live flow context. The
        # subprocess only exercises report classification, not browser I/O.
        "@pytest.fixture(autouse=True)\n"
        "def _flow_artifacts():\n"
        "    yield\n",
        encoding="utf-8",
    )
    (tmp_path / "test_probe.py").write_text(f"def test_probe({fixture_name}):\n    {body}\n", encoding="utf-8")
    result = subprocess.run(
        [
            sys.executable,
            "-m",
            "pytest",
            "-p",
            "test.playwright.conftest",
            "-c",
            str(tmp_path / "pytest.ini"),
            "-q",
            "--color=no",
            str(tmp_path / "test_probe.py"),
        ],
        cwd=tmp_path,
        env={
            **os.environ,
            "PYTHONPATH": str(ROOT),
            "PYTEST_DISABLE_PLUGIN_AUTOLOAD": "1",
            "PYTEST_ADDOPTS": "",
            "PLAYWRIGHT_HANG_TIMEOUT_S": "0",
        },
        capture_output=True,
        text=True,
        timeout=60,
    )
    output = result.stdout + result.stderr
    assert result.returncode == expected_exit, output
    assert expected_text in output, output
