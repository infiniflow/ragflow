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
import json
import os
import sys
import types

# Load json_parser by file path so we don't trigger deepdoc/parser/__init__.py
# (which pulls in heavy parsers). json_parser imports ``decode_text`` from
# rag.nlp; stub rag.nlp only while loading, then restore sys.modules so later
# tests resolve the real package regardless of collection order.


def _find_project_root(marker="pyproject.toml"):
    d = os.path.dirname(os.path.abspath(__file__))
    while d != os.path.dirname(d):
        if os.path.exists(os.path.join(d, marker)):
            return d
        d = os.path.dirname(d)
    return None


def _load_json_parser():
    try:
        from rag.nlp import decode_text as production_decode_text
    except ImportError:

        def production_decode_text(blob, document_type="text"):
            if blob.startswith(b"\xef\xbb\xbf"):
                return blob.decode("utf-8-sig"), "utf-8-sig"
            return blob.decode("utf-8"), "utf-8"

    rag_nlp = types.ModuleType("rag.nlp")
    rag_nlp.decode_text = production_decode_text
    sys.modules["rag.nlp"] = rag_nlp
    rag = types.ModuleType("rag")
    rag.nlp = rag_nlp
    sys.modules["rag"] = rag

    project_root = _find_project_root()
    json_spec = importlib.util.spec_from_file_location(
        "deepdoc.parser.json_parser",
        os.path.join(project_root, "deepdoc", "parser", "json_parser.py"),
    )
    json_mod = importlib.util.module_from_spec(json_spec)
    sys.modules["deepdoc.parser.json_parser"] = json_mod
    json_spec.loader.exec_module(json_mod)
    return json_mod.RAGFlowJsonParser


_saved_modules = {name: sys.modules.get(name) for name in ("rag", "rag.nlp")}
try:
    RAGFlowJsonParser = _load_json_parser()
finally:
    for name, module in _saved_modules.items():
        if module is None:
            sys.modules.pop(name, None)
        else:
            sys.modules[name] = module


def test_top_level_scalars_do_not_crash():
    # Previously raised IndexError instead of returning a chunk.
    parser = RAGFlowJsonParser()
    assert parser._parse_json("42") == ["42"]
    assert parser._parse_json('"hello"') == ['"hello"']
    assert parser._parse_json("true") == ["true"]
    assert parser._parse_json("0") == ["0"]
    assert parser._parse_json("false") == ["false"]


def test_top_level_null_yields_no_chunk():
    # null carries no content; it should be dropped, not crash.
    parser = RAGFlowJsonParser()
    assert parser._parse_json("null") == []
    assert parser._parse_json('""') == []
    assert parser._parse_json("{}") == []
    assert parser._parse_json("[]") == []


def test_objects_and_arrays_still_chunk():
    parser = RAGFlowJsonParser()
    assert parser._parse_json('{"a": 1}') == ['{"a": 1}']
    assert parser._parse_json("[1, 2, 3]") != []


def test_utf8_bom_json_parses_same_as_without_bom():
    parser = RAGFlowJsonParser()
    doc = '{"title": "Quarterly report", "body": "Revenue grew 12 percent."}'
    expected = parser(doc.encode("utf-8"))
    assert expected == ['{"title": "Quarterly report", "body": "Revenue grew 12 percent."}']
    assert parser(b"\xef\xbb\xbf" + doc.encode("utf-8")) == expected


def test_utf8_bom_jsonl_keeps_all_records():
    parser = RAGFlowJsonParser()
    lines = [json.dumps({"id": i}) for i in range(1, 21)]
    jsonl = "\n".join(lines).encode("utf-8")
    with_bom = parser(b"\xef\xbb\xbf" + jsonl)
    without_bom = parser(jsonl)
    assert len(with_bom) == 20
    assert len(without_bom) == 20
    assert with_bom == without_bom
    ids = [json.loads(chunk)["id"] for chunk in with_bom]
    assert ids == list(range(1, 21))
