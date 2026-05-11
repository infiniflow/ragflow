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

"""Unit tests for aggregate_table_manual_doc_metadata."""

import pytest

from rag.utils.table_es_metadata import aggregate_table_manual_doc_metadata, merge_table_parser_config_from_kb


@pytest.fixture
def es_engine(monkeypatch):
    monkeypatch.setattr("rag.utils.table_es_metadata.settings.DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr("rag.utils.table_es_metadata.settings.DOC_ENGINE_OCEANBASE", False)


@pytest.fixture
def infinity_engine(monkeypatch):
    monkeypatch.setattr("rag.utils.table_es_metadata.settings.DOC_ENGINE_INFINITY", True)
    monkeypatch.setattr("rag.utils.table_es_metadata.settings.DOC_ENGINE_OCEANBASE", False)


def _table_task(**kb_extra):
    return {
        "parser_id": "table",
        "parser_config": {},
        "kb_parser_config": {
            "table_column_mode": "manual",
            "table_column_roles": {"country": "metadata", "category": "metadata"},
            "table_column_names": ["country", "category"],
            "field_map": {
                "country_tks": "country",
                "category_tks": "category",
            },
            **kb_extra,
        },
    }


class TestAggregateTableManualDocMetadata:
    def test_aggregate_manual_mode_happy_path(self, es_engine):
        task = _table_task()
        chunks = [
            {
                "country_raw": "Brazil",
                "category_raw": "Economy",
                "country_tks": "x",
                "category_tks": "y",
            },
            {
                "country_raw": "Turkey",
                "category_raw": "Disaster",
                "country_tks": "x",
                "category_tks": "y",
            },
            {
                "country_raw": "Brazil",
                "category_raw": "Economy",
                "country_tks": "x",
                "category_tks": "y",
            },
        ]
        out = aggregate_table_manual_doc_metadata(chunks, task)
        assert out["country"] == ["Brazil", "Turkey"]
        assert out["category"] == ["Economy", "Disaster"]

    def test_aggregate_auto_mode_returns_empty(self, es_engine):
        task = {
            "parser_id": "table",
            "parser_config": {},
            "kb_parser_config": {
                "table_column_mode": "auto",
                "table_column_roles": {"country": "metadata"},
            },
        }
        assert aggregate_table_manual_doc_metadata([{"country_tks": "x"}], task) == {}

    def test_aggregate_no_mode_returns_empty(self, es_engine):
        task = {
            "parser_id": "table",
            "parser_config": {},
            "kb_parser_config": {
                "table_column_roles": {"country": "metadata"},
            },
        }
        assert aggregate_table_manual_doc_metadata([{}], task) == {}

    def test_aggregate_no_metadata_columns(self, es_engine):
        task = {
            "parser_id": "table",
            "parser_config": {},
            "kb_parser_config": {
                "table_column_mode": "manual",
                "table_column_roles": {"country": "indexing"},
                "table_column_names": ["country"],
            },
        }
        assert aggregate_table_manual_doc_metadata([{"country_tks": "x"}], task) == {}

    def test_aggregate_prefers_raw_over_tks(self, es_engine):
        task = _table_task()
        task["kb_parser_config"]["table_column_roles"] = {"country": "metadata"}
        task["kb_parser_config"]["table_column_names"] = ["country"]
        chunks = [{"country_raw": "Brazil", "country_tks": ["brazil"]}]
        out = aggregate_table_manual_doc_metadata(chunks, task)
        assert out == {"country": ["Brazil"]}

    def test_aggregate_tks_fallback(self, es_engine):
        task = _table_task()
        task["kb_parser_config"]["table_column_roles"] = {"country": "metadata"}
        task["kb_parser_config"]["table_column_names"] = ["country"]
        chunks = [{"country_tks": ["brazil"]}]
        out = aggregate_table_manual_doc_metadata(chunks, task)
        assert out == {"country": ["brazil"]}

    def test_aggregate_partial_roles_defaults_to_both(self, es_engine):
        task = {
            "parser_id": "table",
            "parser_config": {},
            "kb_parser_config": {
                "table_column_mode": "manual",
                "table_column_roles": {"country": "indexing"},
                "table_column_names": ["country", "city"],
                "field_map": {"city_tks": "city"},
            },
        }
        chunks = [{"city_raw": "SP", "city_tks": "t", "country_tks": "x"}]
        out = aggregate_table_manual_doc_metadata(chunks, task)
        assert out == {"city": ["SP"]}
        assert "country" not in out

    def test_aggregate_empty_roles_all_columns_both(self, es_engine):
        task = {
            "parser_id": "table",
            "parser_config": {},
            "kb_parser_config": {
                "table_column_mode": "manual",
                "table_column_roles": {},
                "table_column_names": ["country", "city"],
                "field_map": {"country_tks": "country", "city_tks": "city"},
            },
        }
        chunks = [
            {"country_raw": "BR", "city_raw": "SP", "country_tks": "x", "city_tks": "y"},
        ]
        out = aggregate_table_manual_doc_metadata(chunks, task)
        assert "country" in out and "city" in out

    def test_aggregate_deduplicates_values(self, es_engine):
        task = _table_task()
        task["kb_parser_config"]["table_column_roles"] = {"country": "metadata"}
        task["kb_parser_config"]["table_column_names"] = ["country"]
        chunks = [
            {"country_raw": "US", "country_tks": "x"},
            {"country_raw": "UK", "country_tks": "y"},
            {"country_raw": "US", "country_tks": "x"},
        ]
        out = aggregate_table_manual_doc_metadata(chunks, task)
        assert out["country"] == ["US", "UK"]

    def test_aggregate_kb_reload_field_map(self, es_engine, monkeypatch):
        from unittest.mock import MagicMock

        class MockKBS:
            @staticmethod
            def get_by_id(kid):
                kb = MagicMock()
                kb.parser_config = {"field_map": {"country_tks": "country"}}
                return True, kb

        monkeypatch.setattr(
            "rag.utils.table_es_metadata._knowledgebase_service_cls",
            lambda: MockKBS,
        )

        task = {
            "parser_id": "table",
            "parser_config": {},
            "kb_parser_config": {
                "table_column_mode": "manual",
                "table_column_roles": {"country": "metadata"},
                "table_column_names": ["country"],
            },
            "kb_id": "kb-1",
        }
        chunks = [{"country_raw": "X", "country_tks": "t"}]
        out = aggregate_table_manual_doc_metadata(chunks, task)
        assert out == {"country": ["X"]}

    def test_merge_infinity_chunk_data(self, infinity_engine):
        task = {
            "parser_id": "table",
            "parser_config": {},
            "kb_parser_config": {
                "table_column_mode": "manual",
                "table_column_roles": {"country": "both"},
                "table_column_names": ["country"],
            },
        }
        chunks = [
            {"chunk_data": {"country": "US"}},
            {"chunk_data": {"country": "UK"}},
        ]
        out = aggregate_table_manual_doc_metadata(chunks, task)
        assert out == {"country": ["US", "UK"]}


class TestMergeTableParserConfigFromKbExtra:
    """Merge tests also covered in helpers file; keep one explicit case for aggregation module."""

    def test_merge_preserves_parser_config_when_parser_not_table(self):
        task = {
            "parser_id": "naive",
            "parser_config": {"a": 1},
            "kb_parser_config": {"table_column_mode": "manual"},
        }
        assert merge_table_parser_config_from_kb(task) == {"a": 1}
