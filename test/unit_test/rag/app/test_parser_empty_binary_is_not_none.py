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
"""Regression test for the empty-binary-truthiness fix.

The pattern ``if binary:`` is falsy on ``b""`` and would route the parser
to the file-path branch (which is wrong for a 0-byte upload). The fix
is to use ``if binary is not None:`` so an empty ``bytes`` payload
takes the binary branch, not the path branch.

PR #18712 (Fix email and presentation parsers treating an empty upload
as a missing binary) applied the fix to ``rag/app/email.py`` and
``rag/app/presentation.py``. This test pins the same fix on the four
other files the original PR explicitly left out:

  * ``deepdoc/parser/mineru_parser.py``
  * ``deepdoc/parser/somark_parser.py``
  * ``deepdoc/parser/tcadp_parser.py``
  * ``rag/app/naive.py``
"""

from __future__ import annotations

import ast
import os

import pytest


REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "..", ".."))


# (file, function/line range) for each fixed site. Each entry is the
# callable whose body must be free of the bad pattern and free of the
# old fix target (the literal "if binary:" / "if not binary:" / the
# "binary if binary else X" / "filename if not binary else binary"
# ternaries). The original PR #18712 used AST inspection; this test
# follows the same shape.
FIXED_SITES = (
    "deepdoc/parser/mineru_parser.py",
    "deepdoc/parser/somark_parser.py",
    "deepdoc/parser/tcadp_parser.py",
    "rag/app/naive.py",
)


def _source(rel_path: str) -> str:
    with open(os.path.join(REPO_ROOT, rel_path), encoding="utf-8") as f:
        return f.read()


def _module_bad_patterns(src: str) -> list[tuple[int, str]]:
    """Walk the AST and return every top-level ``if binary:`` /
    ``if not binary:`` / truthiness-ternary on the ``binary`` name that
    the fix was supposed to clean up.

    "Bad" patterns are the ones where a name ``binary`` is tested
    truthily or in a ternary where the empty-bytes value would push
    control into the wrong branch. The fix is to use the
    ``is not None`` / ``is None`` / explicit-non-None form instead.
    """
    tree = ast.parse(src)
    bad: list[tuple[int, str]] = []

    for node in ast.walk(tree):
        # ``if binary:`` / ``if not binary:`` at any depth.
        if isinstance(node, ast.If):
            test = node.test
            if _is_truthy_binary_test(test):
                bad.append((node.lineno, ast.unparse(test)))
        # Ternary: ``X if binary else Y`` and ``X if not binary else Y``
        if isinstance(node, ast.IfExp):
            if _is_truthy_binary_test(node.test):
                bad.append((node.lineno, ast.unparse(node)))

    return bad


def _is_truthy_binary_test(node: ast.AST) -> bool:
    """True when ``node`` is a truthiness test on the name ``binary``.

    Catches:
      - ``binary`` (bare Name)
      - ``not binary`` (UnaryOp Not)
      - but NOT ``binary is not None`` / ``binary is None`` /
        ``binary is None else ...`` — those are the fix target.
    """
    if isinstance(node, ast.Name) and node.id == "binary":
        return True
    if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.Not):
        if isinstance(node.operand, ast.Name) and node.operand.id == "binary":
            return True
    return False


@pytest.mark.parametrize("rel_path", FIXED_SITES)
def test_module_has_no_truthy_binary_check(rel_path: str):
    """``if binary:`` / ``if not binary:`` and the matching ternaries
    must be gone from every site PR #18712 listed as out-of-scope.
    """
    src = _source(rel_path)
    bad = _module_bad_patterns(src)
    assert not bad, (
        f"{rel_path} still has the falsy-empty-binary pattern. "
        f"Switch to `if binary is not None:` / `if binary is None:` "
        f"so a 0-byte upload routes to the binary branch, not the "
        f"file-path branch. Found: {bad}"
    )


def test_mineru_extract_pdf_takes_binary_branch_on_empty_bytes():
    """The MinerU PDF entry point must take the binary branch on
    ``binary=b""``, not the path branch. The pre-fix ``if binary:``
    pushed an empty upload to the file path, which is the bug.
    """
    src = _source("deepdoc/parser/mineru_parser.py")

    # The body of the ``if binary is not None:`` branch (after the fix)
    # must mention the temp dir creation that the path branch skips.
    # If ``if binary:`` is restored the body would still be there but
    # the test above (truthiness) would have already failed, so the
    # pre-fix verification is implicit in the AST test.
    assert 'tempfile.mkdtemp(prefix="mineru_bin_pdf_")' in src, "MinerU parser no longer matches the expected binary-branch shape; the temp-dir creation may have been removed or moved."


def test_naive_by_deepdoc_filename_picks_binary_when_present():
    """The two ``by_deepdoc``-style helpers in ``naive.py`` must pick
    the binary payload over the filename when ``binary`` is the empty
    bytes ``b""`` — i.e. the truthiness check is gone.

    This test is more permissive than the AST test above: it only
    pins the specific "filename if not binary else binary" pattern
    (the form that PR #18712 fixed in email.py / presentation.py)
    and the matching "binary if binary else filename" form.
    """
    import re

    src = _source("rag/app/naive.py")
    # The pre-fix patterns PR #18712 left out for naive.py.
    bad_patterns = [
        r"filename if not binary else binary",
        r"binary if binary else filename",
    ]
    for pat in bad_patterns:
        hits = re.findall(pat, src)
        assert not hits, f"naive.py still has the truthiness ternary: pattern={pat!r} hits={hits}. Replace with the explicit `is None` form."
