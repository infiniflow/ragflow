from pathlib import Path
from subprocess import CompletedProcess

from tools.hooks import check_files


def test_staged_paths_preserve_whitespace(monkeypatch):
    paths = b"space name.py\0line\nbreak.go\0"

    def run(command, **kwargs):
        assert command == ["git", "diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z"]
        assert kwargs == {"check": True, "capture_output": True}
        return CompletedProcess(command, 0, stdout=paths)

    monkeypatch.setattr(check_files.subprocess, "run", run)

    assert check_files._staged_paths() == [Path("space name.py"), Path("line\nbreak.go")]


def test_case_conflicts_preserve_newlines(monkeypatch, capsys):
    paths = b"dir/A\nfile.py\0dir/a\nfile.py\0"

    def run(command, **kwargs):
        assert command == ["git", "ls-files", "-z"]
        assert kwargs == {"check": True, "capture_output": True}
        return CompletedProcess(command, 0, stdout=paths)

    monkeypatch.setattr(check_files.subprocess, "run", run)

    assert check_files.check_case_conflicts([]) == 1
    assert "case conflict:" in capsys.readouterr().err
