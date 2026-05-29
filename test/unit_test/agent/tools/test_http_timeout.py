#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""Guard against external-API agent tools issuing HTTP requests without a timeout.

A blocking ``requests``/``httpx`` call with no ``timeout`` will hang forever if
the upstream stalls. Because these tools run inside agent canvas execution, a
single stalled socket hangs the whole agent run with no recovery. This test
parses the tool sources and fails if any ``requests``/``httpx`` request call is
missing a ``timeout`` keyword, covering current and future call sites.
"""

import ast
from pathlib import Path

import pytest


def _repo_root() -> Path:
    """Anchor on the repo root (the dir holding pyproject.toml).

    Walking for the first ``*/agent/tools`` directory is unsafe: this test file
    itself lives under ``test/unit_test/agent/tools``, so that heuristic would
    resolve to the test directory and scan nothing.
    """
    for parent in Path(__file__).resolve().parents:
        if (parent / "pyproject.toml").is_file():
            return parent
    raise RuntimeError("Could not locate repo root (no pyproject.toml found)")


TOOLS_DIR = _repo_root() / "agent" / "tools"
# Fail loudly if we ever point at the wrong place, rather than silently
# scanning zero real tools and passing.
assert (TOOLS_DIR / "github.py").is_file(), f"agent tools not found at {TOOLS_DIR}"

# Methods on ``requests`` / ``httpx`` (and a session/client) that open a socket
# and therefore must be bounded by a timeout.
_REQUEST_METHODS = {"get", "post", "put", "patch", "delete", "head", "options", "request"}
# Modules whose request methods we police, as referenced at the call site.
_HTTP_NAMESPACES = {"requests", "httpx"}


def _iter_request_calls(tree: ast.AST):
    """Yield ``ast.Call`` nodes that look like ``<http>.<verb>(...)`` requests."""
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        func = node.func
        if not isinstance(func, ast.Attribute) or func.attr not in _REQUEST_METHODS:
            continue
        root = func.value
        # Walk back through chained attributes (e.g. requests.sessions.Session()).
        while isinstance(root, ast.Attribute):
            root = root.value
        if isinstance(root, ast.Name) and root.id in _HTTP_NAMESPACES:
            yield node


def _has_timeout(call: ast.Call) -> bool:
    if any(kw.arg == "timeout" for kw in call.keywords):
        return True
    # ``**{"timeout": ...}`` / ``**kwargs`` spread — assume the caller is explicit.
    return any(kw.arg is None for kw in call.keywords)


def _tool_files():
    return sorted(p for p in TOOLS_DIR.glob("*.py") if p.name != "__init__.py")


@pytest.mark.parametrize("path", _tool_files(), ids=lambda p: p.name)
def test_http_calls_have_timeout(path: Path):
    tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    missing = [
        f"{path.name}:{call.lineno}"
        for call in _iter_request_calls(tree)
        if not _has_timeout(call)
    ]
    assert not missing, "HTTP request(s) without timeout=: " + ", ".join(missing)
