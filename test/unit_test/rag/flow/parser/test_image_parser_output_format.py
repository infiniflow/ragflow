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
"""Regression guard for #16835: Parser._image must force ``output_format='json'``.

The previous implementation read ``conf['output_format']`` and passed that to
``set_output``:

    self.set_output("output_format", conf["output_format"])

When a user (or the bundled DSL example ``rag/flow/tests/dsl_examples/
general_pdf_all.json:69``) set ``output_format='text'`` on the image setup,
the parser emitted ``output_format='text'`` to the downstream Token Chunker.
The Token Chunker then routed to the text branch, read the empty top-level
``text`` payload, and returned ``{"chunks": []}`` -- silently dropping the
image OCR results.

PR #15847 (commit ``7b8d6f34b``) pinned the runtime ``output_format`` to
``"json"`` so the image JSON payload is always reachable from the chunker.
These structural assertions make that constraint a unit test so the bug
cannot regress through a "harmless" refactor.
"""

import ast
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[5]
PARSER_PATH = REPO_ROOT / "rag" / "flow" / "parser" / "parser.py"


def _image_method_node():
    """Return the AST.FunctionDef for ``Parser._image``.

    Fails the test with a clear message if the method disappears entirely --
    a larger refactor will then be flagged for review rather than silently
    passing.
    """
    tree = ast.parse(PARSER_PATH.read_text())
    for node in tree.body:
        if isinstance(node, ast.ClassDef) and node.name == "Parser":
            for member in node.body:
                if isinstance(member, ast.FunctionDef) and member.name == "_image":
                    return member
    raise AssertionError("Parser._image not found in rag/flow/parser/parser.py")


def _set_output_calls(method_node, key_literal):
    """Yield every ``self.set_output('<key_literal>', ...)`` call in *method_node*."""
    for node in ast.walk(method_node):
        if not isinstance(node, ast.Call):
            continue
        func = node.func
        if not (isinstance(func, ast.Attribute) and func.attr == "set_output"):
            continue
        if not node.args or not isinstance(node.args[0], ast.Constant):
            continue
        if node.args[0].value != key_literal:
            continue
        yield node


def test_image_parser_forces_output_format_to_json_literal():
    """``set_output('output_format', ...)`` must be the literal ``'json'``.

    A bare string literal (not a Subscript into ``conf`` or a Name reference)
    pins the runtime value so the user-facing ``setups['image']['output_format']``
    cannot route the image OCR results to the chunker's text branch.
    """
    method = _image_method_node()
    calls = list(_set_output_calls(method, "output_format"))
    assert len(calls) == 1, f"expected exactly one set_output('output_format', ...) call in Parser._image, found {len(calls)}"

    value_arg = calls[0].args[1]
    assert isinstance(value_arg, ast.Constant), (
        f"the second arg of set_output('output_format', ...) must be a "
        f"hardcoded string literal so the runtime value cannot drift back to "
        f"the user-configurable ``conf['output_format']`` that caused #16835. "
        f"Got {type(value_arg).__name__}: {ast.dump(value_arg)}"
    )
    assert value_arg.value == "json", (
        f"Parser._image must force output_format='json' so the downstream "
        f"Token Chunker routes to the JSON branch (image OCR results live in "
        f"the ``json`` payload, not ``text``). Got {value_arg.value!r}."
    )


def test_image_parser_does_not_read_conf_output_format():
    """``conf['output_format']`` must not appear anywhere in ``_image``.

    Catches the original regression: the buggy code passed ``conf['output_format']``
    straight through, which let a user config (or the legacy DSL example)
    silently downgrade the image parser to ``output_format='text'``.
    """
    method = _image_method_node()
    for node in ast.walk(method):
        if not isinstance(node, ast.Subscript):
            continue
        if not (isinstance(node.value, ast.Name) and node.value.id == "conf"):
            continue
        if not (isinstance(node.slice, ast.Constant) and node.slice.value == "output_format"):
            continue
        raise AssertionError(
            f"Parser._image must not read conf['output_format'] at line {node.lineno}; #16835 proved that the user-configurable value can be set to 'text' and silently drop image OCR results."
        )


def test_image_parser_emits_json_payload():
    """``_image`` must call ``set_output('json', [...])`` to expose the OCR payload.

    Without this call, the JSON-list payload that downstream chunkers rely on
    (each item carrying ``text`` + ``doc_type_kwd='image'``) never reaches
    ``TokenizerFromUpstream.json_result`` / ``TokenChunkerFromUpstream.json_result``,
    and the JSON-branch dispatch cannot run.
    """
    method = _image_method_node()
    calls = list(_set_output_calls(method, "json"))
    assert calls, "Parser._image must call set_output('json', [...]) so the image OCR/VLM payload is reachable by the downstream Token Chunker."
    for call in calls:
        assert len(call.args) == 2, f"set_output('json', ...) at line {call.lineno} must pass a payload"
        assert isinstance(call.args[1], (ast.List, ast.Name, ast.Call)), (
            f"set_output('json', ...) at line {call.lineno} must be called with a list payload (or a variable holding one), got {type(call.args[1]).__name__}"
        )


def test_image_parser_runtime_output_format_is_user_config_independent():
    """Even if a user configures ``image.output_format='text'``, the runtime must still emit ``'json'``.

    This is the user-visible behaviour behind #16835. The DSL example file
    ``rag/flow/tests/dsl_examples/general_pdf_all.json`` ships with
    ``image.output_format='text'`` for historical reasons; the fix ensures
    that does not matter for the runtime payload handed to the chunker.
    """
    method = _image_method_node()
    output_format_calls = list(_set_output_calls(method, "output_format"))
    assert output_format_calls, "Parser._image does not call set_output('output_format', ...)"
    value_arg = output_format_calls[0].args[1]

    # If the second arg references ``conf`` we cannot statically assert the
    # value -- that is the bug we are guarding against. Reject any non-literal.
    if not isinstance(value_arg, ast.Constant):
        pytest.fail(
            "Parser._image's set_output('output_format', ...) second arg must "
            f"be a hardcoded string literal; got {type(value_arg).__name__}: "
            f"{ast.dump(value_arg)}. A user-configurable value would still break #16835."
        )

    assert value_arg.value == "json", (
        f"the runtime must emit 'json' so the chunker routes to the "
        f"JSON branch and reads the OCR/VLM payload. Got {value_arg.value!r}."
    )
