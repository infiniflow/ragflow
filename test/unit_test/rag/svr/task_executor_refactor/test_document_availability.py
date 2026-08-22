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

from rag.svr.task_executor_refactor.chunk_service import apply_document_availability


def test_apply_document_availability_disabled_hides_source_chunks():
    chunks = [
        {"id": "src-1", "content_with_weight": "ordinary source"},
        {"id": "struct-1", "compile_kwd": "structure", "available_int": 0},
        {"id": "src-2", "content_with_weight": "another source"},
    ]
    apply_document_availability(chunks, "0")

    assert chunks[0]["available_int"] == 0
    assert chunks[1]["available_int"] == 0
    assert chunks[2]["available_int"] == 0


def test_apply_document_availability_enabled_keeps_default():
    chunks = [{"id": "src-1", "content_with_weight": "ordinary source"}]
    apply_document_availability(chunks, "1")
    assert "available_int" not in chunks[0]

    apply_document_availability(chunks, None)
    assert "available_int" not in chunks[0]
