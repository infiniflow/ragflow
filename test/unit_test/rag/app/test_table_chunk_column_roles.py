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
from unittest.mock import MagicMock, patch

# Mock heavy modules that trigger ONNX model loading at import time
# table.py -> deepdoc.parser.figure_parser -> rag.app.picture -> OCR()
for mod in [
    "deepdoc.vision.ocr",
    "deepdoc.parser.figure_parser",
    "rag.app.picture",
]:
    if mod not in sys.modules:
        sys.modules[mod] = MagicMock()

import warnings

# Importing rag.app.table pulls api -> rag.llm -> deepdoc -> xgboost; xgboost may warn on
# pkg_resources in a way that breaks its compat shim unless pkg_resources loads first.
warnings.filterwarnings("ignore", message=".*pkg_resources is deprecated.*", category=UserWarning)
import pkg_resources  # noqa: F401 — stabilize xgboost import during collection

import pytest

import common.settings as settings
from rag.app.table import chunk

# chunk() removes columns named id, _id, index, idx — use row_id instead of id.
TEST_CSV = b"""row_id,title,content,country,category
1,Earthquake hits Turkey,A 5.8 magnitude earthquake struck Konya,Turkey,Disaster
2,Oil prices surge,Brent crude jumped 4.2 percent,Global,Economy
3,AI regulation proposed,EU unveiled a draft regulation,EU,Technology
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


@pytest.fixture
def mock_update_kb():
    with patch("rag.app.table.KnowledgebaseService.update_parser_config") as m:
        yield m


def _run_chunk(parser_config: dict, mock_update_kb: MagicMock):
    return chunk(
        FILENAME,
        binary=TEST_CSV,
        callback=_noop_callback,
        kb_id=KB_ID,
        parser_config=parser_config,
        lang="Chinese",
    )


def test_chunk_auto_mode_all_columns_in_text_and_stored(mock_update_kb: MagicMock):
    parser_config: dict = {}
    chunks = _run_chunk(parser_config, mock_update_kb)
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


def test_chunk_manual_mode_indexing_only(mock_update_kb: MagicMock):
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
    chunks = _run_chunk(parser_config, mock_update_kb)
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


def test_chunk_manual_mode_legacy_vectorize_role(mock_update_kb: MagicMock):
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
    chunks = _run_chunk(parser_config, mock_update_kb)
    first = chunks[0]
    cww = first["content_with_weight"]
    assert "- title:" in cww and "Earthquake" in cww
    assert "- content:" in cww and "Konya" in cww
    assert "- country:" not in cww


def test_chunk_manual_mode_metadata_only(mock_update_kb: MagicMock):
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
    chunks = _run_chunk(parser_config, mock_update_kb)
    first = chunks[0]
    assert (first.get("content_with_weight") or "").strip() == ""
    assert "country_raw" in first and "title_raw" in first


def test_chunk_manual_mode_both(mock_update_kb: MagicMock):
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {c: "both" for c in ["title", "content", "country", "category", "row_id"]},
    }
    chunks = _run_chunk(parser_config, mock_update_kb)
    first = chunks[0]
    cww = first["content_with_weight"]
    assert "Earthquake hits Turkey" in cww
    assert "Turkey" in cww
    assert "Disaster" in cww
    assert "row_id_long" in first
    assert "title_raw" in first and "country_raw" in first


def test_chunk_manual_mode_partial_roles_default_to_both(mock_update_kb: MagicMock):
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {
            "title": "indexing",
            "country": "metadata",
        },
    }
    chunks = _run_chunk(parser_config, mock_update_kb)
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


def test_chunk_manual_mode_raw_fields_for_es(mock_update_kb: MagicMock):
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {c: "both" for c in ["title", "content", "country", "category", "row_id"]},
    }
    chunks = _run_chunk(parser_config, mock_update_kb)
    first = chunks[0]
    for col in ("title", "content", "country", "category"):
        assert f"{col}_raw" in first
        assert f"{col}_tks" in first


def test_chunk_updates_table_column_names(mock_update_kb: MagicMock):
    _run_chunk({}, mock_update_kb)
    mock_update_kb.assert_called_once()
    args, kwargs = mock_update_kb.call_args
    assert args[0] == KB_ID
    payload = args[1]
    names = payload["table_column_names"]
    assert names == ["row_id", "title", "content", "country", "category"]


def test_chunk_count_matches_row_count(mock_update_kb: MagicMock):
    chunks = _run_chunk({}, mock_update_kb)
    assert len(chunks) == 3
