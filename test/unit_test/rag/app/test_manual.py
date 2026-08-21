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
import ast
import re
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]


def _file_routes():
    tree = ast.parse((REPO_ROOT / "rag/app/manual.py").read_text())
    return [
        (node.test.args[0].value, node)
        for node in ast.walk(tree)
        if isinstance(node, ast.If)
        and isinstance(node.test, ast.Call)
        and isinstance(node.test.func, ast.Attribute)
        and isinstance(node.test.func.value, ast.Name)
        and node.test.func.value.id == "re"
        and node.test.func.attr == "search"
        and node.test.args
        and isinstance(node.test.args[0], ast.Constant)
        and isinstance(node.test.args[0].value, str)
    ]


@pytest.mark.p1
@pytest.mark.parametrize("filename", ["legacy.doc", "legacy.DOC"])
def test_legacy_doc_is_rejected_with_conversion_guidance(filename):
    routes = _file_routes()

    assert any(re.search(pattern, "document.docx", re.IGNORECASE) for pattern, _ in routes)
    doc_route = next(node for pattern, node in routes if re.search(pattern, filename, re.IGNORECASE))
    error = next(node for node in doc_route.body if isinstance(node, ast.Raise))
    message = error.exc.args[0].value

    assert ".docx" in message
    assert "convert" in message.lower()
