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

"""Empty page positions must not crash the two direct sort consumers.

``get_position`` now returns ``[]`` for an invalid or empty in-page box.
``rag.app.one.Pdf.__call__`` and ``rag.app.manual.chunk`` used to index
``x[-1][0]`` while sorting. This test executes those real sort statements
without importing either module: both pull in OCR, pdfium, and tenant
services that are unrelated to the sort key.

Scope is only the sort that consumes positions. It does not cover
``pdf_parser.get_position``, table extraction, or tag formatting.
"""

from __future__ import annotations

import ast
import textwrap
from pathlib import Path

import pytest


REPO_ROOT = Path(__file__).resolve().parents[4]

# (pn, left, right, top, bottom) — the tuple get_position emits.
_LATE = (2, 10.0, 40.0, 80.0, 90.0)
_EARLY = (1, 5.0, 20.0, 10.0, 20.0)


def _method_source(path: Path, class_name: str | None, method_name: str) -> str:
    source = path.read_text(encoding="utf-8")
    tree = ast.parse(source)
    nodes = tree.body
    if class_name is not None:
        class_node = next(node for node in nodes if isinstance(node, ast.ClassDef) and node.name == class_name)
        nodes = class_node.body
    method = next(node for node in nodes if isinstance(node, ast.FunctionDef) and node.name == method_name)
    return ast.get_source_segment(source, method)


def _uses_sort_key(node: ast.AST) -> bool:
    return any(isinstance(child, ast.Name) and child.id == "_section_sort_key" for child in ast.walk(node))


def _sort_statements(source: str) -> str:
    """Keep the real nested key and the one statement that consumes it.

    ``one`` returns the sorted list; ``manual`` iterates it inside the PDF
    branch. Both are reduced to an assignment of that sorted list so the
    fragment can run without OCR, token counting, or tag formatting.
    """
    tree = ast.parse(textwrap.dedent(source))
    body = tree.body[0].body
    helper = next(node for node in ast.walk(tree) if isinstance(node, ast.FunctionDef) and node.name == "_section_sort_key")
    consumer = next(node for node in body if node is not helper and _uses_sort_key(node))
    sorted_call = next(child for child in ast.walk(consumer) if isinstance(child, ast.Call) and isinstance(child.func, ast.Name) and child.func.id == "sorted")
    assignment = ast.Assign(targets=[ast.Name(id="sections", ctx=ast.Store())], value=sorted_call)
    fragment = ast.Module(body=[helper, assignment], type_ignores=[])
    ast.fix_missing_locations(fragment)
    return ast.unparse(fragment)


def _run_sort(relative_path: str, class_name: str | None, method_name: str, sections: list):
    source = _method_source(REPO_ROOT / relative_path, class_name, method_name)
    fragment = _sort_statements(source)
    namespace = {"sections": sections}
    exec(compile(fragment, str(REPO_ROOT / relative_path), "exec"), namespace)  # noqa: S102 - real sort fragment from this repo
    return namespace["sections"]


def test_one_pdf_keeps_text_and_sorts_empty_positions_last():
    sections = [
        ("late", [_LATE]),
        ("missing", []),
        ("early", [_EARLY]),
    ]

    result = _run_sort("rag/app/one.py", "Pdf", "__call__", sections)

    assert [item[0] for item in result] == ["early", "late", "missing"]
    assert result[2] == ("missing", [])


def test_manual_chunk_keeps_text_and_empty_positions():
    sections = [
        ("late", 1, [_LATE]),
        ("missing", 2, []),
        ("early", 0, [_EARLY]),
    ]

    result = _run_sort("rag/app/manual.py", None, "chunk", sections)

    assert [item[0] for item in result] == ["early", "late", "missing"]
    # Empty positions stay empty. The later tag join therefore emits no tag.
    assert result[2][2] == []
    assert "\t".join(f"@@{pos[0]}" for pos in result[2][2]) == ""


@pytest.mark.parametrize(
    ("relative_path", "class_name", "method_name", "sections", "expected"),
    [
        (
            "rag/app/one.py",
            "Pdf",
            "__call__",
            [("b", [(1, 9.0, 10.0, 2.0, 3.0)]), ("a", [(1, 1.0, 2.0, 2.0, 3.0)]), ("c", [(1, 1.0, 2.0, 1.0, 2.0)])],
            ["c", "a", "b"],
        ),
        (
            "rag/app/manual.py",
            None,
            "chunk",
            [("b", 0, [(1, 9.0, 10.0, 2.0, 3.0)]), ("a", 0, [(1, 1.0, 2.0, 2.0, 3.0)]), ("c", 0, [(1, 1.0, 2.0, 1.0, 2.0)])],
            ["c", "a", "b"],
        ),
    ],
)
def test_valid_position_order_is_unchanged(relative_path, class_name, method_name, sections, expected):
    result = _run_sort(relative_path, class_name, method_name, sections)

    assert [item[0] for item in result] == expected
