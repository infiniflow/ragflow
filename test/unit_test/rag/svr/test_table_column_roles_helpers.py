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

"""Unit tests for ES table metadata helpers (rag.utils.table_es_metadata)."""

from rag.utils.table_es_metadata import (
    _es_field_value_to_doc_metadata,
    _es_raw_field_key_from_typed,
    _probe_es_typed_key_for_column,
    _resolve_es_chunk_field_key,
    merge_table_parser_config_from_kb,
)


class TestProbeEsTypedKeyForColumn:
    def test_probe_es_typed_key_tks(self):
        chunk = {"country_tks": "tok", "other": 1}
        assert _probe_es_typed_key_for_column("country", chunk) == "country_tks"

    def test_probe_es_typed_key_dt(self):
        chunk = {"published_date_dt": "2024-01-01"}
        assert _probe_es_typed_key_for_column("published_date", chunk) == "published_date_dt"

    def test_probe_es_typed_key_raw(self):
        # Only raw field present (no _tks) — probe returns the raw key
        chunk = {"country_raw": "Brazil"}
        assert _probe_es_typed_key_for_column("country", chunk) == "country_raw"

    def test_probe_es_typed_key_no_match(self):
        chunk = {"other_kwd": "x"}
        assert _probe_es_typed_key_for_column("country", chunk) is None

    def test_probe_es_typed_key_empty_col(self):
        assert _probe_es_typed_key_for_column("", {"a_tks": "x"}) is None
        assert _probe_es_typed_key_for_column(None, {"a_tks": "x"}) is None


class TestResolveEsChunkFieldKey:
    def test_resolve_es_field_empty_fieldmap_uses_probe(self):
        sample = {"country_tks": ["tok"]}
        tk, src = _resolve_es_chunk_field_key("country", {}, sample)
        assert tk == "country_tks"
        assert src == "probe"

    def test_resolve_es_field_fieldmap_priority(self):
        fm = {"guojia_tks": "country"}
        sample = {"guojia_tks": ["x"], "country_tks": ["y"]}
        tk, src = _resolve_es_chunk_field_key("country", fm, sample)
        assert tk == "guojia_tks"
        assert src == "field_map"


class TestEsRawFieldKeyFromTyped:
    def test_es_raw_field_key_from_tks(self):
        assert _es_raw_field_key_from_typed("country_tks") == "country_raw"

    def test_es_raw_field_key_from_non_tks(self):
        assert _es_raw_field_key_from_typed("country_dt") is None

    def test_es_raw_field_key_from_none(self):
        assert _es_raw_field_key_from_typed(None) is None


class TestEsFieldValueToDocMetadata:
    def test_es_field_value_string(self):
        assert _es_field_value_to_doc_metadata("Brazil", from_tks_fallback=False) == "Brazil"

    def test_es_field_value_list_joined(self):
        assert (
            _es_field_value_to_doc_metadata(["hello", "world"], from_tks_fallback=True)
            == "hello world"
        )

    def test_es_field_value_empty(self):
        assert _es_field_value_to_doc_metadata(None, from_tks_fallback=True) is None
        assert _es_field_value_to_doc_metadata("", from_tks_fallback=True) is None
        assert _es_field_value_to_doc_metadata([], from_tks_fallback=True) is None


class TestMergeTableParserConfigFromKb:
    def test_merge_table_parser_config_from_kb(self):
        task = {
            "parser_id": "table",
            "parser_config": {"llm_id": "x"},
            "kb_parser_config": {
                "table_column_mode": "manual",
                "table_column_roles": {"a": "metadata"},
                "table_column_names": ["a", "b"],
            },
        }
        merged = merge_table_parser_config_from_kb(task)
        assert merged["table_column_mode"] == "manual"
        assert merged["table_column_roles"] == {"a": "metadata"}
        assert merged["table_column_names"] == ["a", "b"]
        assert merged["llm_id"] == "x"

    def test_merge_table_parser_config_auto_default(self):
        task = {
            "parser_id": "table",
            "parser_config": {"foo": 1},
            "kb_parser_config": {"llm_id": "abc"},
        }
        merged = merge_table_parser_config_from_kb(task)
        assert merged == {"foo": 1}  # no table_* keys copied from kb without kb_parser_config keys
