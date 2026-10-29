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
"""update_parser_config must replace table-parser derived keys, not merge them.

``field_map`` and ``table_column_names`` are recomputed from the dataset's
files on every parse. A deep merge kept columns from earlier parses forever:
once a sheet with an inflated ``<dimension>`` produced thousands of ``Column_N``
names, no later parse and no delete_field_map call could remove them.
"""

import ast
from pathlib import Path

import pytest

pytestmark = pytest.mark.p2

DERIVED = {"field_map", "table_column_names"}


def _load_update_parser_config():
    """Exec update_parser_config from the production source with a fake store."""
    src_path = Path(__file__).resolve().parents[5] / "api" / "db" / "services" / "knowledgebase_service.py"
    tree = ast.parse(src_path.read_text(encoding="utf-8"))

    cls_node = next(node for node in tree.body if isinstance(node, ast.ClassDef) and node.name == "KnowledgebaseService")
    wanted = {"update_parser_config", "delete_field_map", "document_parser_config"}
    nodes = [n for n in cls_node.body if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef)) and n.name in wanted]
    assert {n.name for n in nodes} == wanted, f"missing methods: {wanted - {n.name for n in nodes}}"

    derived = next(n for n in cls_node.body if isinstance(n, ast.Assign) and any(getattr(t, "id", "") == "PARSER_DERIVED_CONFIG_KEYS" for t in n.targets))
    nodes.append(derived)

    stored = {}
    state = {"parser_config": {}}

    class FakeModel:
        def __init__(self):
            self.parser_config = state["parser_config"]

    class FakeService:
        model = FakeModel

        @staticmethod
        def get_by_id(_id):
            return True, FakeModel()

        @staticmethod
        def update_by_id(_id, fields):
            state["parser_config"] = fields["parser_config"]
            stored.update(fields)
            return True

    module = ast.Module(body=nodes, type_ignores=[])
    ns = {"DB": type("DB", (), {"connection_context": staticmethod(lambda: (lambda fn: fn))}), "LookupError": LookupError}
    exec(compile(module, str(src_path), "exec"), ns)  # noqa: S102 - exec'ing AST-extracted methods from our own source
    cls = type(
        "KnowledgebaseService",
        (),
        {
            "get_by_id": staticmethod(FakeService.get_by_id),
            "update_by_id": staticmethod(FakeService.update_by_id),
            "PARSER_DERIVED_CONFIG_KEYS": ns["PARSER_DERIVED_CONFIG_KEYS"],
            "update_parser_config": classmethod(ns["update_parser_config"].__func__),
            "delete_field_map": classmethod(ns["delete_field_map"].__func__),
            "document_parser_config": classmethod(ns["document_parser_config"].__func__),
        },
    )
    return cls, state


def test_derived_keys_are_replaced_not_merged():
    cls, state = _load_update_parser_config()
    state["parser_config"] = {
        "field_map": {"old": "Old"},
        "table_column_names": ["Old", "Column_5", "Column_6"],
        "chunk_token_num": 128,
    }

    cls.update_parser_config("kb", {"field_map": {"new": "New"}, "table_column_names": ["New"]})

    assert state["parser_config"]["field_map"] == {"new": "New"}
    assert state["parser_config"]["table_column_names"] == ["New"]


def test_other_keys_still_deep_merge():
    cls, state = _load_update_parser_config()
    state["parser_config"] = {"raptor": {"max_token": 256, "threshold": 0.1}}

    cls.update_parser_config("kb", {"raptor": {"max_token": 512}})

    assert state["parser_config"]["raptor"] == {"max_token": 512, "threshold": 0.1}


def test_delete_field_map_clears_both_derived_keys():
    cls, state = _load_update_parser_config()
    state["parser_config"] = {
        "field_map": {"a": "A"},
        "table_column_names": ["a"],
        "chunk_token_num": 128,
    }

    cls.delete_field_map("kb")

    assert "field_map" not in state["parser_config"]
    assert "table_column_names" not in state["parser_config"]
    assert state["parser_config"]["chunk_token_num"] == 128


def test_document_parser_config_drops_derived_keys():
    cls, _ = _load_update_parser_config()

    out = cls.document_parser_config(
        {"field_map": {"a": "A"}, "table_column_names": ["a"], "chunk_token_num": 128}
    )

    assert out == {"chunk_token_num": 128}


def test_document_parser_config_handles_missing_config():
    cls, _ = _load_update_parser_config()

    assert cls.document_parser_config(None) == {}
