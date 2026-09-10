#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

from types import SimpleNamespace
from unittest.mock import patch

from rag.svr.task_executor_refactor.chunk_service import (
    apply_document_availability,
    apply_source_chunks_document_availability,
)
from rag.svr.task_executor_refactor.constants import GRAPH_RAPTOR_FAKE_DOC_ID


def test_apply_document_availability_disabled_hides_source_chunks():
    chunks = [
        {"id": "src-1", "content_with_weight": "ordinary source"},
        {"id": "struct-1", "compile_kwd": "structure", "available_int": 0},
        {"id": "src-2", "content_with_weight": "another source"},
    ]
    stamped = apply_document_availability(chunks, "0")

    assert stamped == 2
    assert chunks[0]["available_int"] == 0
    assert chunks[1]["available_int"] == 0
    assert chunks[2]["available_int"] == 0


def test_apply_document_availability_enabled_keeps_default():
    chunks = [{"id": "src-1", "content_with_weight": "ordinary source"}]
    assert apply_document_availability(chunks, "1") == 0
    assert "available_int" not in chunks[0]

    assert apply_document_availability(chunks, None) == 0
    assert "available_int" not in chunks[0]


def test_apply_source_chunks_skips_raptor_and_stamps_per_doc():
    """Mixed RAPTOR batches must not inherit status from the first document only."""
    chunks = [
        {"id": "raptor-1", "doc_id": "disabled-doc", "raptor_kwd": "raptor"},
        {"id": "src-disabled", "doc_id": "disabled-doc", "content_with_weight": "from disabled"},
        {"id": "src-enabled", "doc_id": "enabled-doc", "content_with_weight": "from enabled"},
        {"id": "fake", "doc_id": GRAPH_RAPTOR_FAKE_DOC_ID, "content_with_weight": "fake raptor doc"},
    ]

    def _get_by_id(doc_id):
        status = {"disabled-doc": "0", "enabled-doc": "1"}.get(doc_id, "1")
        return True, SimpleNamespace(status=status)

    with patch(
        "rag.svr.task_executor_refactor.chunk_service.DocumentService.get_by_id",
        side_effect=_get_by_id,
    ):
        apply_source_chunks_document_availability(chunks)

    assert "available_int" not in chunks[0]  # RAPTOR chunk untouched
    assert chunks[1]["available_int"] == 0  # disabled source stamped
    assert "available_int" not in chunks[2]  # enabled source untouched
    assert "available_int" not in chunks[3]  # fake RAPTOR doc skipped
