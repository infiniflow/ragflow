"""Regression tests: a 0-byte upload must stay a real payload in the TCADP parser.

``b""`` is not ``None``. Truthiness guards (``if binary:``) route an empty
upload down the file-path branch, where the display name is opened as a path
that does not exist on disk. Sibling parsers (naive, MinerU, SoMark) already
use the explicit ``binary is not None`` form; TCADP was the leftover from
infiniflow/ragflow#18759 (closed stale).
"""

import ast
import base64
import os
import tempfile
import textwrap
from pathlib import Path

TCADP = (
    Path(__file__).resolve().parents[4] / "deepdoc" / "parser" / "tcadp_parser.py"
)
SOURCE = TCADP.read_text()
TREE = ast.parse(SOURCE)


def test_no_truthiness_binary_guards_left():
    """No ``if binary:`` / ``binary if binary else`` truthiness routing may remain."""
    bad = []
    for node in ast.walk(TREE):
        if isinstance(node, ast.If) and isinstance(node.test, ast.Name) and node.test.id == "binary":
            bad.append(f"line {node.lineno}: if binary:")
        if isinstance(node, ast.IfExp) and isinstance(node.test, ast.Name) and node.test.id == "binary":
            bad.append(f"line {node.lineno}: ... if binary else ...")
    assert not bad, "truthiness binary guards remain: " + "; ".join(bad)


def test_explicit_none_checks_present():
    assert "if binary is not None:" in SOURCE
    assert "binary if binary is not None else filepath" in SOURCE


def _load_file_to_base64():
    """Exec the REAL _file_to_base64 source with stubbed class context."""
    method = None
    for node in ast.walk(TREE):
        if isinstance(node, ast.FunctionDef) and node.name == "_file_to_base64":
            method = node
            break
    assert method is not None
    fn_src = textwrap.dedent(ast.get_source_segment(SOURCE, method))
    namespace = {"base64": base64}
    exec(fn_src, namespace)
    return namespace["_file_to_base64"]


def test_runtime_empty_bytes_stays_binary_branch():
    """b\"\" must be encoded as empty content, never opened as a file path."""
    fn = _load_file_to_base64()
    first = fn(None, "unused-display-name.pdf", binary=b"")
    second = fn(None, "unused-display-name.pdf", binary=b"")
    assert first == second == "", f"empty payload misrouted: {first!r}"


def test_runtime_none_falls_through_to_file_path():
    """None still means: read from the given path."""
    fn = _load_file_to_base64()
    with tempfile.NamedTemporaryFile(delete=False, suffix=".pdf") as f:
        f.write(b"%PDF-1.4 fake")
        path = f.name
    try:
        first = fn(None, path, binary=None)
        second = fn(None, path, binary=None)
        assert first == second == base64.b64encode(b"%PDF-1.4 fake").decode("utf-8")
    finally:
        os.unlink(path)


def test_runtime_nonempty_bytes_unchanged():
    fn = _load_file_to_base64()
    payload = b"real pdf bytes"
    assert fn(None, "unused.pdf", binary=payload) == base64.b64encode(payload).decode("utf-8")
