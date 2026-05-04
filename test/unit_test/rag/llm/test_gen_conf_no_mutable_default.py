#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""
Regression guard for mutable default `gen_conf={}` in the LLM provider
integration layer (`rag/llm/chat_model.py`, `rag/llm/cv_model.py`).

Many provider methods used to declare ``def chat_streamly(..., gen_conf={}, ...)``
and then mutate ``gen_conf`` in place (``del gen_conf["max_tokens"]``,
``gen_conf["penalty_score"] = ...``). Because Python evaluates default
argument values **once** at function-definition time, that single shared
dict accumulated mutations across calls — every later caller that omitted
``gen_conf`` saw the polluted dict from the previous call.

The fix is to default to ``None`` and copy at the call site
(``gen_conf = dict(gen_conf or {})``). This test parses both modules with
the ``ast`` module and asserts no parameter named ``gen_conf`` ever has
a mutable literal as its default.
"""
import ast
from pathlib import Path
from typing import Union

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]
TARGET_FILES = [
    REPO_ROOT / "rag" / "llm" / "chat_model.py",
    REPO_ROOT / "rag" / "llm" / "cv_model.py",
]


def _iter_param_defaults(func: Union[ast.FunctionDef, ast.AsyncFunctionDef]):
    """Yield (param_name, default_node) for every parameter with a
    non-empty default — covers positional, keyword-only, and the new
    positional-only syntax."""
    args = func.args
    pos_args = args.args
    pos_defaults = args.defaults
    # positional defaults are right-aligned with args
    for arg, default in zip(pos_args[-len(pos_defaults):], pos_defaults):
        yield arg.arg, default
    for arg, default in zip(args.kwonlyargs, args.kw_defaults):
        if default is not None:
            yield arg.arg, default


def _find_mutable_gen_conf_defaults(path: Path):
    tree = ast.parse(path.read_text(encoding="utf-8"))
    bad = []
    for node in ast.walk(tree):
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue
        for name, default in _iter_param_defaults(node):
            if name != "gen_conf":
                continue
            # An empty dict literal `{}` is the original bug. A list literal
            # `[]` would be the same class of mistake. Anything else is fine.
            if isinstance(default, (ast.Dict, ast.List)) and not default.keys and not getattr(default, "elts", None):
                bad.append((node.name, default.lineno))
    return bad


@pytest.mark.parametrize("path", TARGET_FILES, ids=lambda p: p.name)
def test_no_mutable_default_for_gen_conf(path: Path):
    """No function in chat_model.py / cv_model.py should declare
    ``gen_conf={}`` (or ``gen_conf=[]``) as a default value."""
    bad = _find_mutable_gen_conf_defaults(path)
    assert not bad, (
        f"{path.name} has functions declaring `gen_conf` with a mutable "
        f"default: {bad}. Use `gen_conf=None` and copy with "
        f"`gen_conf = dict(gen_conf or {{}})` at the top of the function."
    )


def test_target_files_exist():
    """Sanity check — if the LLM modules move, this regression guard
    must follow them."""
    for path in TARGET_FILES:
        assert path.is_file(), f"Expected target file at {path}"
