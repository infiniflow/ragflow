#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License on an "AS IS" BASIS, WITHOUT WARRANTIES
#  OR CONDITIONS OF ANY KIND, either express or implied. See the License
#  for the specific language governing permissions and limitations under
#  the License.
#

"""Integration-style tests for rag.app.table.chunk() column roles (mocked KB + tokenizer)."""

from __future__ import annotations

import sys
from io import BytesIO
from importlib import import_module, reload
from unittest.mock import MagicMock, patch

import warnings

# Importing rag.app.table pulls api -> rag.llm -> deepdoc -> xgboost; xgboost may warn on
# pkg_resources in a way that breaks its compat shim unless pkg_resources loads first.
warnings.filterwarnings("ignore", message=".*pkg_resources is deprecated.*", category=UserWarning)
import pkg_resources  # noqa: F401 — stabilize xgboost import during collection

import pytest

import common.settings as settings

# chunk() removes columns named id, _id, index, idx — use row_id instead of id.
TEST_CSV = b"""row_id,title,content,country,category
1,Earthquake hits Turkey,A 5.8 magnitude earthquake struck Konya,Turkey,Disaster
2,Oil prices surge,Brent crude jumped 4.2 percent,Global,Economy
3,AI regulation proposed,EU unveiled a draft regulation,EU,Technology
"""
TEST_DUPLICATE_COLUMNS_CSV = b"""name,name,name_2
Alice,Engineer,Team A
"""
TEST_PARTIAL_NUMERIC_CSV = b"""row_id,amount,note
1,100,first
2,N/A,second
3,300,third
"""

FILENAME = "test.csv"
KB_ID = "test_kb_id"


def _noop_callback(*_a, **_k):
    pass


@pytest.fixture(autouse=True)
def _es_doc_engine(monkeypatch):
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(settings, "DOC_ENGINE_OCEANBASE", False)


@pytest.fixture(autouse=True)
def _stub_rag_tokenizer(monkeypatch):
    """Avoid NLTK / infinity tokenizer deps; keep string content inspectable."""

    def fake_tokenize(line):
        return str(line)

    monkeypatch.setattr("rag.nlp.rag_tokenizer.tokenize", fake_tokenize)
    monkeypatch.setattr("rag.nlp.rag_tokenizer.fine_grained_tokenize", fake_tokenize)


@pytest.fixture(scope="module")
def table_module():
    """Load rag.app.table with heavy optional dependencies stubbed locally."""
    stub_names = [
        "deepdoc.vision.ocr",
        "deepdoc.parser.figure_parser",
        "rag.app.picture",
    ]
    original_modules = {name: sys.modules.get(name) for name in stub_names}

    try:
        for name in stub_names:
            sys.modules[name] = MagicMock()
        module = import_module("rag.app.table")
        module = reload(module)
        yield module
    finally:
        for name, original in original_modules.items():
            if original is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original


@pytest.fixture
def mock_update_kb():
    with patch("rag.app.table.KnowledgebaseService.update_parser_config") as m:
        yield m


def _run_chunk(table_module, parser_config: dict, mock_update_kb: MagicMock):
    return table_module.chunk(
        FILENAME,
        binary=TEST_CSV,
        callback=_noop_callback,
        kb_id=KB_ID,
        parser_config=parser_config,
        lang="Chinese",
    )


def test_chunk_deduplicates_repeated_column_names(table_module, mock_update_kb: MagicMock):
    chunks = table_module.chunk(
        FILENAME,
        binary=TEST_DUPLICATE_COLUMNS_CSV,
        callback=_noop_callback,
        kb_id=KB_ID,
        parser_config={},
        lang="Chinese",
    )
    assert len(chunks) == 1
    cww = chunks[0]["content_with_weight"]
    assert "- name: Alice" in cww
    assert "- name_3: Engineer" in cww
    assert "- name_2: Team A" in cww
    args, kwargs = mock_update_kb.call_args
    assert args[1]["table_column_names"] == ["name", "name_3", "name_2"]


def test_excel_image_description_string_stays_single_cell(table_module, monkeypatch):
    from openpyxl import Workbook

    wb = Workbook()
    ws = wb.active
    ws.append(["image", "note"])
    ws.append([None, "keep"])
    buf = BytesIO()
    wb.save(buf)

    monkeypatch.setattr(
        table_module.Excel,
        "_extract_images_from_worksheet",
        staticmethod(
            lambda ws, sheetname=None: [{"sheet": sheetname or ws.title, "image": None, "image_description": "", "row_from": 2, "col_from": 1, "row_to": 2, "col_to": 1, "span_type": "single_cell"}]
        ),
    )
    monkeypatch.setattr(table_module, "vision_figure_parser_figure_xlsx_wrapper", lambda images, callback=None, **kwargs: [((None, "abcdef"), [(0, 0, 0, 0, 0)])])

    dfs, tbls = table_module.Excel()("test.xlsx", binary=buf.getvalue(), callback=_noop_callback)

    assert tbls == []
    assert dfs[0].iat[0, 0] == "abcdef"


def test_chunk_auto_mode_all_columns_in_text_and_stored(table_module, mock_update_kb: MagicMock):
    parser_config: dict = {}
    chunks = _run_chunk(table_module, parser_config, mock_update_kb)
    assert len(chunks) == 3
    first = chunks[0]
    cww = first["content_with_weight"]
    assert "Earthquake hits Turkey" in cww
    assert "Konya" in cww
    assert "Turkey" in cww
    assert "Disaster" in cww
    assert "1" in cww or "row_id" in cww
    # ES path: stored typed fields for text columns include *_tks and *_raw; row_id is int -> *_long
    assert "row_id_long" in first
    assert "title_raw" in first and "country_raw" in first


def test_chunk_manual_mode_indexing_only(table_module, mock_update_kb: MagicMock):
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {
            "title": "indexing",
            "content": "indexing",
            "row_id": "metadata",
            "country": "metadata",
            "category": "metadata",
        },
    }
    chunks = _run_chunk(table_module, parser_config, mock_update_kb)
    first = chunks[0]
    cww = first["content_with_weight"]
    assert "- title:" in cww and "Earthquake" in cww
    assert "- content:" in cww and "Konya" in cww
    assert "- country:" not in cww
    assert "- category:" not in cww
    assert "- row_id:" not in cww
    # Column title/content not stored as table fields
    assert "title_raw" not in first
    assert "content_raw" not in first
    assert "country_raw" in first and "category_raw" in first
    assert "row_id_long" in first


def test_chunk_manual_mode_legacy_vectorize_role(table_module, mock_update_kb: MagicMock):
    """Stored configs may still use role *vectorize*; chunking treats it like *indexing*."""
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {
            "title": "vectorize",
            "content": "indexing",
            "row_id": "metadata",
            "country": "metadata",
            "category": "metadata",
        },
    }
    chunks = _run_chunk(table_module, parser_config, mock_update_kb)
    first = chunks[0]
    cww = first["content_with_weight"]
    assert "- title:" in cww and "Earthquake" in cww
    assert "- content:" in cww and "Konya" in cww
    assert "- country:" not in cww


def test_chunk_manual_mode_metadata_only(table_module, mock_update_kb: MagicMock):
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {
            "title": "metadata",
            "content": "metadata",
            "row_id": "metadata",
            "country": "metadata",
            "category": "metadata",
        },
    }
    chunks = _run_chunk(table_module, parser_config, mock_update_kb)
    first = chunks[0]
    assert (first.get("content_with_weight") or "").strip() == ""
    assert "country_raw" in first and "title_raw" in first


def test_chunk_manual_mode_both(table_module, mock_update_kb: MagicMock):
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {c: "both" for c in ["title", "content", "country", "category", "row_id"]},
    }
    chunks = _run_chunk(table_module, parser_config, mock_update_kb)
    first = chunks[0]
    cww = first["content_with_weight"]
    assert "Earthquake hits Turkey" in cww
    assert "Turkey" in cww
    assert "Disaster" in cww
    assert "row_id_long" in first
    assert "title_raw" in first and "country_raw" in first


def test_chunk_manual_mode_partial_roles_default_to_both(table_module, mock_update_kb: MagicMock):
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {
            "title": "indexing",
            "country": "metadata",
        },
    }
    chunks = _run_chunk(table_module, parser_config, mock_update_kb)
    first = chunks[0]
    cww = first["content_with_weight"]
    assert "- title:" in cww and "Earthquake" in cww
    assert "- country:" not in cww
    assert "- row_id:" in cww
    assert "- content:" in cww
    assert "- category:" in cww
    assert "title_raw" not in first
    assert "country_raw" in first and "country_tks" in first
    assert "content_raw" in first and "category_raw" in first


def test_chunk_manual_mode_raw_fields_for_es(table_module, mock_update_kb: MagicMock):
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {c: "both" for c in ["title", "content", "country", "category", "row_id"]},
    }
    chunks = _run_chunk(table_module, parser_config, mock_update_kb)
    first = chunks[0]
    for col in ("title", "content", "country", "category"):
        assert f"{col}_raw" in first
        assert f"{col}_tks" in first


def test_chunk_updates_table_column_names(table_module, mock_update_kb: MagicMock):
    _run_chunk(table_module, {}, mock_update_kb)
    mock_update_kb.assert_called_once()
    args, kwargs = mock_update_kb.call_args
    assert args[0] == KB_ID
    payload = args[1]
    names = payload["table_column_names"]
    assert names == ["row_id", "title", "content", "country", "category"]


def test_chunk_count_matches_row_count(table_module, mock_update_kb: MagicMock):
    chunks = _run_chunk(table_module, {}, mock_update_kb)
    assert len(chunks) == 3


# Regression for #18287: Infinity/OceanBase/SereneDB store chunk_data
# keyed by the original column name, and the SQL prompt examples
# reference those exact keys via `json_extract_string(chunk_data,
# '$.FieldName')`. Keys must therefore be the original column names
# — not the pinyin form (`py_clmns[i].lower()`), which silently
# broke non-ASCII columns because the LLM was told `xing_ming` but
# `chunk_data` was keyed by `姓名`.
def _enable_infinity(monkeypatch):
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", True)
    monkeypatch.setattr(settings, "DOC_ENGINE_OCEANBASE", False)


def _enable_oceanbase(monkeypatch):
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(settings, "DOC_ENGINE_OCEANBASE", True)


def test_chunk_infinity_field_map_uses_original_column_names(table_module, mock_update_kb: MagicMock, monkeypatch):
    _enable_infinity(monkeypatch)
    _run_chunk(table_module, {}, mock_update_kb)
    payload = mock_update_kb.call_args.args[1]
    field_map = payload["field_map"]
    # field_map keys must equal the original column names so they line up
    # with chunk_data storage keys used by the SQL prompt's
    # `json_extract_string(chunk_data, '$.FieldName')` examples.
    assert set(field_map.keys()) == {"row_id", "title", "content", "country", "category"}
    # No pinyin (e.g. "row_id_tks") should leak into field_map keys —
    # pinyin belongs in the type-suffixed ES path, not the Infinity
    # JSON-extract path.
    assert not any(k.endswith("_tks") for k in field_map.keys())
    # Keys must be the raw column name verbatim. Under the buggy
    # implementation `row_id` was being rewritten to `"row id"`, which
    # the SQL prompt's `$.row id` could not match against
    # `chunk_data["row_id"]`. Values, however, get underscore-to-space
    # formatting applied (so the LLM sees a human-readable label), but
    # the key must NOT be reformatted.
    assert "row_id" in field_map
    assert "row id" not in field_map
    assert field_map["row_id"] == "row id"


def test_chunk_oceanbase_field_map_uses_original_column_names(table_module, mock_update_kb: MagicMock, monkeypatch):
    _enable_oceanbase(monkeypatch)
    _run_chunk(table_module, {}, mock_update_kb)
    payload = mock_update_kb.call_args.args[1]
    field_map = payload["field_map"]
    assert set(field_map.keys()) == {"row_id", "title", "content", "country", "category"}
    assert not any(k.endswith("_tks") for k in field_map.keys())
    assert "row_id" in field_map
    assert "row id" not in field_map
    assert field_map["row_id"] == "row id"


def test_chunk_infinity_field_map_non_ascii_columns_are_keys(table_module, mock_update_kb: MagicMock, monkeypatch):
    """The original bug repros here: a non-ASCII column would be keyed by
    its pinyin form (`xing_ming`) under the old behavior, so the LLM
    is told a key that doesn't exist in `chunk_data`. After the fix the
    field_map key IS the original column name."""
    _enable_infinity(monkeypatch)
    test_csv = b"row_id,\xe5\xa7\x93\xe5\x90\x8d,\xe9\x95\xbf\xe5\xba\xa6\n1,Alice,42\n"  # row_id, 姓名, 长度
    table_module.chunk(
        FILENAME,
        binary=test_csv,
        callback=_noop_callback,
        kb_id=KB_ID,
        parser_config={},
        lang="Chinese",
    )
    payload = mock_update_kb.call_args.args[1]
    field_map = payload["field_map"]
    # The original column names — including the non-ASCII ones — must
    # appear verbatim as keys so the SQL prompt's `$.姓名` matches the
    # JSON extraction in chunk_data.
    assert "\u59d3\u540d" in field_map  # 姓名
    assert "\u957f\u5ea6" in field_map  # 长度
    assert not any(k.endswith("_tks") for k in field_map.keys())


@pytest.mark.parametrize(
    ("column", "expected_type"),
    [
        (["1", "2", "3", "N/A"], "int"),
        (["1.5", "2.5", "3.5", "see note"], "float"),
        (["yes", "no", "yes", "unknown"], "bool"),
        (["2025-01-01", "2025-02-01", "ongoing"], "datetime"),
    ],
)
def test_column_data_type_keeps_cells_it_cannot_convert(table_module, column, expected_type):
    converted, ty = table_module.column_data_type(list(column))
    assert ty == expected_type
    assert converted[-1] == column[-1]
    assert None not in converted


def test_chunk_keeps_unconvertible_cell_in_content(table_module, mock_update_kb: MagicMock):
    chunks = table_module.chunk(
        FILENAME,
        binary=TEST_PARTIAL_NUMERIC_CSV,
        callback=_noop_callback,
        kb_id=KB_ID,
        parser_config={},
        lang="Chinese",
    )
    assert len(chunks) == 3
    assert chunks[0]["amount_long"] == 100
    assert chunks[2]["amount_long"] == 300
    assert chunks[1].get("amount_long") != "N/A"
    assert "- amount: N/A" in chunks[1]["content_with_weight"]
