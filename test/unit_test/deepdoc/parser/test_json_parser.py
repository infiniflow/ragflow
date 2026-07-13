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

"""Unit tests for RAGFlowJsonParser.

Regression for the case where a .json upload whose top-level value is a bare
JSON scalar (a number, string, boolean, or null - all valid JSON) reached
``_json_split`` with an empty ``current_path``. ``_set_nested_dict`` then indexed
``path[-1]`` on an empty list and raised ``IndexError``, which ``_parse_json``
does not catch (it only guards ``json.JSONDecodeError``), so the whole upload
crashed. A top-level scalar has no key to nest under and must be stored as the
chunk directly.
"""

import importlib.util
import os
import sys
from unittest import mock

# Load json_parser by file path so we don't trigger deepdoc/parser/__init__.py
# (which pulls in heavy parsers). json_parser only imports ``find_codec`` from
# rag.nlp, and only inside ``__call__``; stub rag.nlp so the module imports.
if "rag" not in sys.modules:
    sys.modules["rag"] = mock.MagicMock()
if "rag.nlp" not in sys.modules:
    sys.modules["rag.nlp"] = mock.MagicMock()


def _find_project_root(marker="pyproject.toml"):
    d = os.path.dirname(os.path.abspath(__file__))
    while d != os.path.dirname(d):
        if os.path.exists(os.path.join(d, marker)):
            return d
        d = os.path.dirname(d)
    return None


_PROJECT_ROOT = _find_project_root()

_json_spec = importlib.util.spec_from_file_location(
    "deepdoc.parser.json_parser",
    os.path.join(_PROJECT_ROOT, "deepdoc", "parser", "json_parser.py"),
)
_json_mod = importlib.util.module_from_spec(_json_spec)
sys.modules["deepdoc.parser.json_parser"] = _json_mod
_json_spec.loader.exec_module(_json_mod)

RAGFlowJsonParser = _json_mod.RAGFlowJsonParser


def test_top_level_scalars_do_not_crash():
    # Previously raised IndexError instead of returning a chunk.
    parser = RAGFlowJsonParser()
    assert parser._parse_json("42") == ["42"]
    assert parser._parse_json('"hello"') == ['"hello"']
    assert parser._parse_json("true") == ["true"]


def test_top_level_null_yields_no_chunk():
    # null carries no content; it should be dropped, not crash.
    parser = RAGFlowJsonParser()
    assert parser._parse_json("null") == []


def test_objects_and_arrays_still_chunk():
    parser = RAGFlowJsonParser()
    assert parser._parse_json('{"a": 1}') == ['{"a": 1}']
    assert parser._parse_json("[1, 2, 3]") != []
