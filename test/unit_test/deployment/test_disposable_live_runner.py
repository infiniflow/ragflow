"""Guard the live runner's log decoding and bounded child-process cleanup."""

import os
import sys
from time import monotonic

from test.integration.live_ragflow import runner


def test_command_decodes_utf8_and_tolerates_invalid_log_bytes(tmp_path, monkeypatch):
    monkeypatch.setattr(runner, "OUT", tmp_path)
    result = runner.command(
        [sys.executable, "-c", "import os;os.write(1, 'Привет 😀'.encode('utf-8') + b'\\xff')"],
        env=dict(os.environ),
        log="unicode.log",
    )
    assert result.returncode == 0
    assert (tmp_path / "unicode.log").read_text(encoding="utf-8") == "Привет 😀\ufffd"


def test_command_timeout_terminates_children_holding_output_pipe(tmp_path, monkeypatch):
    monkeypatch.setattr(runner, "OUT", tmp_path)
    script = "import subprocess,sys,time;subprocess.Popen([sys.executable,'-c','import time;time.sleep(60)']);print('child started',flush=True);time.sleep(60)"
    started = monotonic()
    result = runner.command([sys.executable, "-c", script], env=dict(os.environ), log="timeout.log", timeout=1, check=False)
    assert result.returncode == 124
    assert "child started" in result.stdout
    assert monotonic() - started < 15, "A child kept the pipe open after the timeout"
    assert "Command timeout" in (tmp_path / "timeout.log").read_text(encoding="utf-8")
