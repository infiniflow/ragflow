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


def _file_routing_patterns():
    tree = ast.parse((REPO_ROOT / "rag/app/manual.py").read_text())
    return [
        node.args[0].value
        for node in ast.walk(tree)
        if isinstance(node, ast.Call)
        and isinstance(node.func, ast.Attribute)
        and isinstance(node.func.value, ast.Name)
        and node.func.value.id == "re"
        and node.func.attr == "search"
        and node.args
        and isinstance(node.args[0], ast.Constant)
        and isinstance(node.args[0].value, str)
    ]


@pytest.mark.p1
@pytest.mark.parametrize("filename", ["legacy.doc", "legacy.DOC"])
def test_legacy_doc_has_no_manual_parser_route(filename):
    patterns = _file_routing_patterns()

    assert any(re.search(pattern, "document.docx", re.IGNORECASE) for pattern in patterns)
    assert not any(re.search(pattern, filename, re.IGNORECASE) for pattern in patterns)
