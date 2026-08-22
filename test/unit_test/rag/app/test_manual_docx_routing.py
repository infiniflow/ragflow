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
"""Regression tests for issue #18621.

The Manual parser's file-routing regex was `\\.docx?$` which matches
both `.doc` and `.docx`. Both formats were then passed to
`Docx()` -> `python-docx.Document()`, which only handles Office Open
XML (.docx) and raises `BadZipFile` or an OPC relationship `KeyError`
on legacy OLE/CFB .doc files. The error surfaced as a generic
500-style stack trace to the user.

The fix splits the routing:

* the `.docx` branch now matches `\\.docx$` (no optional `x`);
* a new `.doc` branch raises `NotImplementedError` with a clear
  message: legacy .doc is the pre-2007 Office format, the Manual
  parser is built on python-docx which handles .docx only, the user
  should convert to .docx or PDF and re-upload.

The test uses AST extraction rather than driving ``chunk()`` directly
because the module pulls ``deepdoc`` -> ``xgboost`` -> ``beartype``
at import time and the dev venv does not have all of those installed
(see the ``ModuleNotFoundError: No module named 'beartype'`` we hit
during the cycle-29 attempt to import ``rag.app.manual``). The AST
approach pins the routing contract without needing the full import
graph.
"""

import ast
import re
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]
MANUAL_PY = REPO_ROOT / "rag" / "app" / "manual.py"


def _chunk_function_node():
    """Parse ``rag/app/manual.py`` and return the ``chunk`` function AST node.

    Fails the test (not the import) if the source file is missing or the
    function has been renamed, so a future refactor that breaks the
    routing contract surfaces here rather than as a silent regression.
    """
    if not MANUAL_PY.is_file():
        pytest.fail(f"missing source: {MANUAL_PY}")
    tree = ast.parse(MANUAL_PY.read_text(encoding="utf-8"))
    for node in ast.walk(tree):
        if isinstance(node, ast.FunctionDef) and node.name == "chunk":
            return node
    pytest.fail("`chunk` function not found in rag/app/manual.py")


def _branch_regex_and_first_action(chunk_node):
    """Walk the top-level ``if/elif/else`` chain in ``chunk`` and yield
    ``(regex_pattern_str | None, first_action_kind)`` for each branch.

    ``first_action_kind`` is one of:
        ``"docx"``     -> branch instantiates ``Docx()`` and calls it
        ``"raise"``    -> branch raises ``NotImplementedError``
        ``"raise-other"`` -> branch is a bare ``raise`` of another exception
        ``"fallthrough"`` -> branch falls through to a different handler
    """
    # Find the top-level if/elif/else chain. chunk() has exactly one such
    # chain at its top level; the function body's other constructs are
    # nested inside it.
    for stmt in chunk_node.body:
        if isinstance(stmt, ast.If):
            current = stmt
            while isinstance(current, ast.If):
                # The condition is a Call like ``re.search(r"\.doc$", filename, ...)``.
                # ast.unparse gives the source text with escapes preserved,
                # e.g. 're.search(\\\\.doc$, filename, re.IGNORECASE)'. We need
                # the actual regex string with the escapes interpreted, so use
                # ast.literal_eval on the second argument (the pattern) when
                # possible; fall back to the unparsed source for non-literal
                # patterns (none expected here).
                pattern = _extract_re_pattern(current.test)
                kind = _first_action_kind(current)
                yield pattern, kind
                # Advance to the next elif (or end).
                if current.orelse and isinstance(current.orelse[0], ast.If):
                    current = current.orelse[0]
                else:
                    # Final else clause (or no else) terminates the chain.
                    if current.orelse:
                        kind = _first_action_kind_from_body(current.orelse)
                        yield None, kind
                    return


def _extract_re_pattern(test_node):
    """Pull the regex pattern string out of an ``re.search(pattern, ...)`` call.

    Returns the actual Python string value (with raw-string / escape
    sequences interpreted), or None if the test is not a recognisable
    ``re.search`` call.
    """
    if not (isinstance(test_node, ast.Call) and isinstance(test_node.func, ast.Attribute)):
        return None
    if test_node.func.attr != "search":
        return None
    if not test_node.args:
        return None
    pattern_arg = test_node.args[0]
    # Accept only string-literal patterns. Anything else (a variable, a
    # concat) is out of scope for this pinning test.
    if not isinstance(pattern_arg, ast.Constant) or not isinstance(pattern_arg.value, str):
        return None
    return pattern_arg.value


def _first_action_kind(if_node):
    """Determine the first action taken in an if-branch body."""
    body = if_node.body
    return _first_action_kind_from_body(body)


def _first_action_kind_from_body(body):
    """Walk the body looking for a callable instantiation, a raise, etc."""
    for stmt in body:
        # Direct raise
        if isinstance(stmt, ast.Raise):
            exc = stmt.exc
            if exc is not None and isinstance(exc, ast.Call):
                fn = exc.func
                if isinstance(fn, ast.Name) and fn.id == "NotImplementedError":
                    return "raise"
            return "raise-other"
        # Look for a Docx() instantiation (the docx branch)
        for sub in ast.walk(stmt):
            if isinstance(sub, ast.Call):
                func = sub.func
                # ``docx_parser = Docx()`` -> func is Name("Docx")
                if isinstance(func, ast.Name) and func.id == "Docx":
                    return "docx"
    return "fallthrough"


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


def test_legacy_doc_routes_to_not_implemented_error():
    """A filename ending in ``.doc`` (legacy Microsoft Word, OLE/CFB)
    must match a branch that raises NotImplementedError -- not the .docx
    branch. Pre-fix, the regex `\\.docx?$` matched both .doc and .docx
    and routed .doc to python-docx, which then failed with BadZipFile.
    """
    chunk_node = _chunk_function_node()
    branches = list(_branch_regex_and_first_action(chunk_node))

    doc_branches = [(pat, kind) for pat, kind in branches if pat and re.search(pat, "test.doc", re.IGNORECASE)]
    assert doc_branches, f"no branch in chunk() matches 'test.doc' (legacy OLE/CFB). Branches seen: {branches!r}"
    pat, kind = doc_branches[0]
    assert kind == "raise", (
        f"legacy .doc must hit a NotImplementedError-raising branch, not "
        f"the docx branch. Matched regex: {pat!r}, action kind: {kind!r}. "
        f"Pre-fix, .doc entered the docx branch and crashed inside "
        f"python-docx with BadZipFile / KeyError."
    )


def test_docx_routes_to_docx_branch():
    """A filename ending in ``.docx`` (Office Open XML) must match the
    branch that instantiates ``Docx()`` and calls the docx parser.
    """
    chunk_node = _chunk_function_node()
    branches = list(_branch_regex_and_first_action(chunk_node))

    docx_branches = [(pat, kind) for pat, kind in branches if pat and re.search(pat, "test.docx", re.IGNORECASE)]
    assert docx_branches, f"no branch in chunk() matches 'test.docx'. Branches seen: {branches!r}"
    pat, kind = docx_branches[0]
    assert kind == "docx", f"the .docx branch must instantiate Docx() and run the docx parser. Matched regex: {pat!r}, action kind: {kind!r}"


def test_pdf_routes_to_pdf_branch():
    """Sanity / regression guard: the ``.pdf`` branch must still be the
    first branch in the chain (PDF is the most common input format and
    is processed inline before the docx branch).
    """
    chunk_node = _chunk_function_node()
    branches = list(_branch_regex_and_first_action(chunk_node))

    pdf_branches = [(pat, kind) for pat, kind in branches if pat and re.search(pat, "test.pdf", re.IGNORECASE)]
    assert pdf_branches, f"no branch in chunk() matches 'test.pdf'. Branches seen: {branches!r}"
    pat, _ = pdf_branches[0]
    # The .docx branch regex must NOT match "test.doc" -- this is the
    # central invariant of the fix.
    assert not re.search(pat, "test.doc", re.IGNORECASE), (
        rf"the .docx branch regex {pat!r} must not match 'test.doc' "
        rf"(legacy OLE/CFB) -- the pre-fix \.docx?$ regex did, which is "
        rf"the bug."
    )


def test_docx_branch_regex_does_not_match_legacy_doc():
    r"""Central invariant of the fix: the regex used for the `Docx()`
    branch must be `\.docx$` (or another pattern that does not match
    `.doc`), not `\.docx?$` (which matches both). Pinned at the AST
    level so a future "loosening" of the regex is caught loudly.
    """
    chunk_node = _chunk_function_node()
    branches = list(_branch_regex_and_first_action(chunk_node))

    # The first branch that matches 'test.docx' (the .docx branch)
    docx_branch = next(
        ((pat, kind) for pat, kind in branches if pat and re.search(pat, "test.docx", re.IGNORECASE)),
        None,
    )
    assert docx_branch is not None, "no branch in chunk() matches 'test.docx'"
    pat, _ = docx_branch
    assert not re.search(pat, "test.doc", re.IGNORECASE), (
        rf"the .docx branch regex {pat!r} must not match 'test.doc' "
        rf"(legacy Microsoft Word). Pre-fix the regex was \.docx?$ which "
        rf"matched both, and legacy .doc input crashed python-docx with "
        rf"BadZipFile / KeyError. The fix narrows the regex to \.docx$ "
        rf"and adds a separate \.doc branch that raises "
        rf"NotImplementedError with a clear message."
    )


def test_legacy_doc_error_message_mentions_docx_and_pdf():
    """The NotImplementedError raised for legacy .doc must name both
    ``.docx`` and ``PDF`` as the supported conversion targets and must
    mention ``#18621`` so a maintainer reading the trace can find the
    upstream issue. The message is the user-facing interface for the
    fix; pinning it here prevents a future "cleanup" that strips the
    issue reference.
    """
    chunk_node = _chunk_function_node()
    for stmt in chunk_node.body:
        if not isinstance(stmt, ast.If):
            continue
        for sub in ast.walk(stmt):
            if not isinstance(sub, ast.Raise) or sub.exc is None:
                continue
            if not isinstance(sub.exc, ast.Call):
                continue
            fn = sub.exc.func
            if not (isinstance(fn, ast.Name) and fn.id == "NotImplementedError"):
                continue
            # Found a NotImplementedError raise. Inspect its message.
            if not sub.exc.args:
                continue
            arg = sub.exc.args[0]
            if not isinstance(arg, ast.Constant) or not isinstance(arg.value, str):
                continue
            msg = arg.value
            if "legacy" not in msg.lower() and ".doc" not in msg:
                continue
            # This is the legacy .doc error message. Pin the user-facing
            # details.
            assert ".docx" in msg, f"error message must mention .docx conversion target: {msg!r}"
            assert "PDF" in msg, f"error message must mention PDF conversion target: {msg!r}"
            assert "#18621" in msg, f"error message must reference issue #18621: {msg!r}"
            return
    pytest.fail("no NotImplementedError mentioning 'legacy' or '.doc' found in chunk()")
