import common.settings as settings

from rag.utils.table_es_metadata import (
    aggregate_table_doc_metadata,
    merge_table_parser_config_from_kb,
    table_parser_strip_doc_metadata_keys,
)


def test_tc_sql_009_parser_config_merge_and_metadata_cleanup_keys():
    task = {
        "parser_id": "table",
        "parser_config": {"llm_id": "x"},
        "kb_parser_config": {
            "table_column_mode": "manual",
            "table_column_roles": {"a": "metadata"},
            "table_column_names": ["a", "b"],
        },
    }

    assert merge_table_parser_config_from_kb(task) == {
        "llm_id": "x",
        "table_column_mode": "manual",
        "table_column_roles": {"a": "metadata"},
        "table_column_names": ["a", "b"],
    }
    assert merge_table_parser_config_from_kb(
        {
            "parser_id": "table",
            "parser_config": {"foo": 1},
            "kb_parser_config": {"llm_id": "ignored"},
        }
    ) == {"foo": 1}
    assert table_parser_strip_doc_metadata_keys({"table_column_names": ["Region", " SKU "]}) == frozenset({"Region", "SKU"})
    assert table_parser_strip_doc_metadata_keys({"table_column_roles": {"x": "metadata", "y": "indexing"}}) == frozenset({"x", "y"})
    assert table_parser_strip_doc_metadata_keys({"table_column_names": [], "table_column_roles": {"only": "both"}}) == frozenset({"only"})


def test_tc_sql_010_es_aggregates_public_chunk_metadata(monkeypatch):
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(settings, "DOC_ENGINE_OCEANBASE", False)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", False, raising=False)
    chunks = [
        {
            "guojia_tks": ["tokenized", "country"],
            "guojia_raw": "Brazil",
            "category_tks": ["Risk", "Audit"],
            "score_long": 3,
        },
        {
            "guojia_tks": ["other"],
            "guojia_raw": "Brazil",
            "category_tks": ["Operations"],
            "score_long": 4,
        },
    ]
    task = {
        "parser_id": "table",
        "parser_config": {
            "table_column_mode": "manual",
            "table_column_names": ["country", "category", "score", "title"],
            "table_column_roles": {
                "country": "metadata",
                "category": "both",
                "score": "metadata",
                "title": "indexing",
            },
        },
        "kb_parser_config": {
            "field_map": {
                "guojia_tks": "country",
                "category_tks": "category",
                "score_long": "score",
            }
        },
    }

    assert aggregate_table_doc_metadata(chunks, task) == {
        "country": ["Brazil"],
        "category": ["Risk Audit", "Operations"],
        "score": ["3", "4"],
    }


def test_tc_sql_011_gaussdb_aggregates_chunk_data_metadata(monkeypatch):
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(settings, "DOC_ENGINE_OCEANBASE", False)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True, raising=False)
    chunks = [
        {"chunk_data": {"country": "Turkey", "category": "Disaster", "title": "Earthquake"}},
        {"chunk_data": {"country": "Turkey", "category": "Economy"}},
        {"chunk_data": {"country": "EU", "category": None}},
        {"country_raw": "must not be read on GaussDB"},
    ]
    task = {
        "parser_id": "table",
        "parser_config": {
            "table_column_names": ["country", "category", "title"],
            "table_column_roles": {
                "country": "metadata",
                "category": "both",
                "title": "indexing",
            },
        },
    }

    assert aggregate_table_doc_metadata(chunks, task) == {
        "country": ["Turkey", "EU"],
        "category": ["Disaster", "Economy"],
    }
