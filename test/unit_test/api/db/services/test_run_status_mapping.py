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
import sys
import types
import pytest
from api.apps.restful_apis import chunk_api  # noqa: E402
from api.apps.services import document_api_service  # noqa: E402
from common.constants import TaskStatus  # noqa: E402


class _FakeDoc:
    """Minimal stand-in for a document model exposing to_dict()."""

    def __init__(self, **fields):
        self._fields = fields

    def to_dict(self):
        return dict(self._fields)


# ---------------------------------------------------------------------------
# _process_run_mapping
# ---------------------------------------------------------------------------


def test_schedule_status_maps_to_schedule():
    doc = {"id": "d1"}
    result = document_api_service._process_run_mapping(doc, TaskStatus.SCHEDULE.value)
    assert result["run"] == "SCHEDULE"


def test_all_statuses_map_without_silent_downgrade():
    for status in TaskStatus:
        doc = {"id": "x"}
        result = document_api_service._process_run_mapping(doc, status.value)
        assert result["run"] == status.name


def test_unknown_run_status_downgrades_to_unstart():
    doc = {"id": "x"}
    result = document_api_service._process_run_mapping(doc, "999")
    assert result["run"] == "UNSTART"


# ---------------------------------------------------------------------------
# _map_doc
# ---------------------------------------------------------------------------


def test_map_doc_schedule_status():
    fake = _FakeDoc(id="d1", run=TaskStatus.SCHEDULE.value, chunk_num=0)
    mapped = chunk_api._map_doc(fake)
    assert mapped["run"] == "SCHEDULE"


def test_map_doc_unknown_run_status_is_null():
    fake = _FakeDoc(id="d1", run="999", chunk_num=0)
    mapped = chunk_api._map_doc(fake)
    assert mapped["run"] is None
