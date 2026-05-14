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
"""Unit tests verifying that ``llm_token_num`` is exposed as ``llm_token_count``
in the document API response shape.

These tests target the centralized mapping helper
``api/apps/services/document_api_service.py::map_doc_keys`` (and the variant
``map_doc_keys_with_run_status``), which is shared by all REST endpoints that
return documents (list_documents, update_document, …).  A second copy of the
same mapping is kept in ``api/apps/restful_apis/chunk_api.py::_map_doc`` for
chunk endpoints; the dict-shape assertion below is repeated against that copy
as a regression guard against drift.
"""

import sys
from types import ModuleType

# Stub heavy / partially-installed third-party modules before any RAGFlow
# import.  Without this, importing the module-under-test pulls in the
# whole RAG indexing pipeline (xgboost, umap → tensorflow, …) and any
# emitted UserWarning/ImportWarning is escalated to an error by pytest's
# ``filterwarnings = ["error"]`` config.  None of these libraries are
# exercised by the mapping helper under test.
for _stub_name in ("xgboost", "umap"):
    sys.modules.setdefault(_stub_name, ModuleType(_stub_name))

import pytest  # noqa: E402

from api.apps.restful_apis.chunk_api import _map_doc as chunk_map_doc  # noqa: E402
from api.apps.services.document_api_service import (  # noqa: E402
    _process_key_mappings,
    map_doc_keys,
    map_doc_keys_with_run_status,
)


def _doc(**fields):
    """Build a plain dict that mimics a Document row for the mappers."""
    defaults = {
        "id": "doc-1",
        "name": "report.pdf",
        "chunk_num": 5,
        "token_num": 1234,
        "llm_token_num": 0,
        "kb_id": "ds-1",
        "parser_id": "naive",
        "run": "1",
    }
    defaults.update(fields)
    return defaults


class _ModelLike:
    """Minimal stand-in for a peewee Document model: exposes ``to_dict()``
    which is what ``chunk_api._map_doc`` calls.
    """
    def __init__(self, **fields):
        self._fields = _doc(**fields)

    def to_dict(self):
        return dict(self._fields)


@pytest.mark.p2
class TestListDocsLlmTokenMapping:
    """Exercises ``map_doc_keys``, used by list_documents."""

    def test_llm_token_num_renamed_to_llm_token_count(self):
        out = map_doc_keys(_doc(llm_token_num=150))
        assert "llm_token_count" in out, "llm_token_num must be renamed to llm_token_count"
        assert out["llm_token_count"] == 150
        assert "llm_token_num" not in out, "original key must not leak through"

    def test_llm_token_count_zero_forwarded(self):
        out = map_doc_keys(_doc(llm_token_num=0))
        assert out["llm_token_count"] == 0, "zero must round-trip as zero, not be dropped"

    def test_other_key_renames_unaffected(self):
        out = map_doc_keys(_doc(llm_token_num=99))
        assert out["chunk_count"] == 5         # chunk_num  → chunk_count
        assert out["dataset_id"] == "ds-1"     # kb_id      → dataset_id
        assert out["token_count"] == 1234      # token_num  → token_count
        assert out["chunk_method"] == "naive"  # parser_id  → chunk_method
        assert out["llm_token_count"] == 99    # llm_token_num → llm_token_count

    def test_chunk_api_mapping_stays_in_sync(self):
        """The duplicated mapping in chunk_api._map_doc must keep the same keys."""
        out = chunk_map_doc(_ModelLike(llm_token_num=42))
        assert out["llm_token_count"] == 42
        assert "llm_token_num" not in out


@pytest.mark.p2
class TestUpdateDocLlmTokenMapping:
    """Exercises the rename when run-status mapping is forced (update path)."""

    def test_llm_token_num_renamed_in_update_response(self):
        # update_document uses ``map_doc_keys_with_run_status``; ``run_status``
        # is the numeric code stored in the DB (e.g. "3"), which the helper
        # turns into the public label ("DONE").
        out = map_doc_keys_with_run_status(_doc(llm_token_num=75), run_status="3")
        assert out["llm_token_count"] == 75
        assert "llm_token_num" not in out
        assert out["run"] == "DONE"

    def test_low_level_helper_matches_public_one(self):
        """Sanity check: _process_key_mappings (used by both public wrappers)
        must apply the llm_token_num rename consistently."""
        out = _process_key_mappings(_doc(llm_token_num=7))
        assert out["llm_token_count"] == 7
        assert "llm_token_num" not in out
