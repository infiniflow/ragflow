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
"""Regression tests for ``_parse_available_flag`` (issue #19479).

The chunk PATCH/PUT routes parse the availability flag with bare ``int(...)``.
A non-integer value raised ``ValueError`` and surfaced as a generic 500
(``PATCH /datasets/<id>/documents/<id>/chunks/<chunk_id>`` with
``{"available": "abc"}``; ``PUT .../chunks`` with
``{"chunk_ids": [...], "available_int": "abc"}``). Sibling fields on the
same routes (``positions``, ``tag_kwd``, ``tag_feas``) are type-checked
and answer with data errors — this helper makes ``available`` and
``available_int`` behave the same way.

Tests pin the helper contract:

1. Valid ``0`` / ``1`` pass through as ``(True, int)``.
2. Non-integer (``"abc"``, ``"1.5"``, ``None``, ``[]``) returns the
   argument-error message instead of raising.
3. ``bool`` is rejected — ``bool`` is an ``int`` subclass so ``int(True)
   == 1`` would otherwise sneak past.
4. Out-of-range integers (``2``, ``-1``, ``100``) are rejected.

The helper is loaded directly via AST extraction so the test does not
have to import ``chunk_api.py`` (which transitively pulls in rag/peewee/
xxhash/pydantic/quart/grpcio/etc. — too heavy for a slim dev env).
"""

import ast
import textwrap
from pathlib import Path

import pytest


def _load_parse_available_flag():
    """Pull just the ``_parse_available_flag`` function out of
    ``chunk_api.py`` so we don't have to load the whole module.

    The function is a pure-Python helper (only ``int`` + ``isinstance``),
    so this isolation is safe — we test the same source that ships.
    """
    import types

    repo_root = Path(__file__).resolve().parents[5]
    source = (repo_root / "api" / "apps" / "restful_apis" / "chunk_api.py").read_text()
    tree = ast.parse(source)

    target = None
    for node in tree.body:
        if isinstance(node, ast.FunctionDef) and node.name == "_parse_available_flag":
            target = node
            break
    if target is None:
        raise RuntimeError("_parse_available_flag not found in chunk_api.py")

    func_src = textwrap.dedent(ast.get_source_segment(source, target))
    module = types.ModuleType("_chunk_api_extracted")
    code = compile(func_src, "<chunk_api._parse_available_flag>", "exec")
    exec(code, module.__dict__)  # noqa: S102 — exec is intentional here, we
    # want to test the actual source from chunk_api.py without dragging in
    # the module's heavy transitive imports (rag/peewee/xxhash/pydantic/...).
    return module._parse_available_flag


parse_flag = _load_parse_available_flag()


# --------------------------------------------------------------------------- #
# Tests
# --------------------------------------------------------------------------- #


@pytest.mark.parametrize("value", [0, 1, "0", "1", 0.0, 1.0])
def test_accepts_zero_and_one(value):
    """The valid values per the chunk schema are 0 and 1; integers,
    strings, and floats that compare equal to 0 or 1 all pass."""
    ok, result = parse_flag(value, "available")
    assert ok is True
    assert result == int(value)
    assert result in (0, 1)


@pytest.mark.parametrize(
    "bad_value",
    [
        "abc",
        "1.5",  # non-integer float string
        None,
        [],
        {},
        "1e2",  # exponential notation
        "  ",  # whitespace
        True,  # bool — rejected even though int(True) == 1
        False,  # bool — same
        2,
        -1,
        100,
    ],
)
def test_rejects_bad_values(bad_value):
    """Non-integers, bools, and out-of-range ints all return the
    data-error message instead of raising."""
    ok, message = parse_flag(bad_value, "available")
    assert ok is False
    assert isinstance(message, str)
    assert "available" in message
    assert "should be" in message


def test_float_one_point_five_is_silently_truncated_to_one():
    """Edge case: ``int(1.5) == 1`` so the float passes the validator as
    ``1``. This is the same leniency as ``int(...)`` everywhere else in
    Python; pinning the behavior so a future tightening of the helper
    surfaces as a test failure rather than a silent change."""
    ok, result = parse_flag(1.5, "available")
    # Pinning the actual behavior: 1.5 is currently accepted as 1.
    # Update this assertion if the helper is tightened.
    assert ok is True
    assert result == 1


def test_message_uses_provided_field_name():
    """The field name in the error message matches what the caller passed —
    so the same helper can validate ``available`` and ``available_int`` with
    distinct messages."""
    ok, message = parse_flag("abc", "available")
    assert ok is False
    assert "`available`" in message
    assert "`available_int`" not in message

    ok, message = parse_flag("abc", "available_int")
    assert ok is False
    assert "`available_int`" in message


def test_bool_true_explicitly_rejected():
    """Regression for ``int(True) == 1`` sneaking past the parser."""
    ok, message = parse_flag(True, "available")
    assert ok is False
    assert "should be 0 or 1" in message


def test_bool_false_explicitly_rejected():
    ok, message = parse_flag(False, "available")
    assert ok is False
    assert "should be 0 or 1" in message


def test_does_not_raise_on_any_input():
    """The original bug surfaced as ``ValueError: invalid literal for
    int()`` → 500. The helper must always return ``(bool, ...)`` and never
    raise."""
    for raw in [None, "abc", [], {}, object(), True, False, 0, 1, 2, -1, 0.5]:
        result = parse_flag(raw, "available")
        assert isinstance(result, tuple)
        assert len(result) == 2
        assert isinstance(result[0], bool)
