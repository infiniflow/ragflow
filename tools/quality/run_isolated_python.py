"""Run one materialized quality tool without candidate import-path injection."""

from __future__ import annotations

import os
import runpy
import sys
from pathlib import Path, PurePosixPath


def sanitized_child_environment(**overrides: str) -> dict[str, str]:
    """Remove control-plane, interpreter-injection, and secret variables from child processes."""
    blocked_names = {
        "NODE_EXTRA_CA_CERTS",
        "NODE_OPTIONS",
        "NODE_PATH",
        "PYTHONHOME",
        "PYTHONPATH",
        "PYTHONSTARTUP",
        "PYTEST_ADDOPTS",
        "PYTEST_PLUGINS",
        "RUNNER_TEMP",
    }
    blocked_prefixes = ("ACTIONS_", "ARCHITECTURE_", "GITHUB_")
    secret_markers = ("CREDENTIAL", "PASSWORD", "SECRET", "TOKEN")
    environment = {
        key: value
        for key, value in os.environ.items()
        if key.upper() not in blocked_names and not key.upper().startswith(blocked_prefixes) and "EVIDENCE" not in key.upper() and not any(marker in key.upper() for marker in secret_markers)
    }
    environment.update(overrides)
    return environment


def _repository_path(value: str) -> str:
    if not value or "\\" in value or value.startswith("/"):
        raise ValueError("Tool path must be normalized and repository-relative")
    normalized = PurePosixPath(value).as_posix()
    if normalized != value or any(part in {"", ".", ".."} for part in PurePosixPath(value).parts):
        raise ValueError("Tool path must be normalized and repository-relative")
    return value


def main() -> int:
    if not sys.flags.isolated or not sys.flags.safe_path:
        raise ValueError("Trusted quality tools require Python isolated safe-path mode")
    if len(sys.argv) < 3:
        raise ValueError("Usage: run_isolated_python.py <materialized-root> <tool-path> [arguments...]")
    source_root = Path(sys.argv[1]).resolve()
    tool_path = _repository_path(sys.argv[2])
    tool = (source_root / PurePosixPath(tool_path)).resolve()
    trusted_module_root = (source_root / "tools/quality").resolve()
    if not tool.is_file() or not tool.is_relative_to(trusted_module_root) or tool.parent != trusted_module_root:
        raise ValueError(f"Tool is outside the materialized quality namespace: {tool_path}")

    inherited = [entry for entry in sys.path if entry and Path(entry).resolve() != Path.cwd().resolve()]
    sys.path[:] = [str(trusted_module_root), *inherited]
    sys.argv = [str(tool), *sys.argv[3:]]
    runpy.run_path(str(tool), run_name="__main__")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
