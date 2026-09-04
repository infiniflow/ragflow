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


# The BOM tests below call ``__call__`` which routes through ``find_codec``;
# the rest of the test file calls ``_parse_json`` / ``_parse_jsonl`` directly
# and so does not need the codec stub. Replace the rag.nlp MagicMock with a
# thin shim that exposes a working ``find_codec`` so the new tests can
# exercise the byte-decode + BOM-strip path.
def _stub_find_codec(blob):
    for enc in ("utf-8", "utf-8-sig", "gb18030", "latin-1"):
        try:
            blob.decode(enc)
            return enc
        except UnicodeDecodeError:
            continue
    return "utf-8"


_nlp_stub = mock.MagicMock()
_nlp_stub.find_codec = _stub_find_codec
sys.modules["rag.nlp"] = _nlp_stub
_json_mod.find_codec = _stub_find_codec


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


# Regression for #19179: a leading UTF-8 BOM (U+FEFF) used to silently zero
# out a .json / .jsonl / .ldjson upload because ``json.loads`` rejects the
# BOM as garbage and the bare ``except json.JSONDecodeError`` in
# ``_parse_json`` / ``_parse_jsonl`` swallowed the failure. The fix strips
# a leading BOM at the top of ``__call__`` so the rest of the pipeline
# never sees it.
UTF8_BOM = b"\xef\xbb\xbf"


def test_json_with_bom_is_chunked():
    parser = RAGFlowJsonParser()
    # .json object, with BOM -> one chunk
    chunks = parser(UTF8_BOM + b'{"a": 1}')
    assert chunks == ['{"a": 1}']
    # .json top-level array, with BOM -> the array is converted to a dict
    # by the parser (convert_lists=True), so both elements fit in one chunk
    # together. The point of this test is that the BOM no longer zeros
    # the result, not the chunk boundary.
    chunks = parser(UTF8_BOM + b'[{"a": 1}, {"b": 2}]')
    assert len(chunks) == 1
    assert chunks[0] == '{"0": {"a": 1}, "1": {"b": 2}}'


def test_jsonl_with_bom_keeps_every_record():
    parser = RAGFlowJsonParser()
    # Previously, the first record was dropped because its leading BOM made
    # ``json.loads`` raise on it (the bare ``except: continue`` swallowed
    # that record only). After the fix, every record comes through.
    for n in (2, 3, 5, 20):
        payload = UTF8_BOM + b"\n".join((b'{"i": ' + str(i).encode() + b"}") for i in range(n))
        chunks = parser(payload)
        # split_json produces one chunk per object for small inputs.
        assert len(chunks) == n, f"expected {n} chunks, got {len(chunks)}"


def test_bom_only_input_does_not_crash():
    parser = RAGFlowJsonParser()
    # A BOM with no payload is a degenerate file, but should not raise.
    # It falls through both parser branches and returns an empty list.
    assert parser(UTF8_BOM) == []
    assert parser(UTF8_BOM + b"\n") == []


def test_no_bom_path_unchanged():
    parser = RAGFlowJsonParser()
    # Sanity: the BOM-stripping branch is a no-op for clean input.
    assert parser(b'{"a": 1}') == ['{"a": 1}']
    assert parser(b'{"a": 1}\n{"b": 2}') == parser(UTF8_BOM + b'{"a": 1}\n{"b": 2}')
