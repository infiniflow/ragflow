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
import warnings

import pytest

# xgboost imports pkg_resources and emits a deprecation warning that is promoted
# to error in our pytest configuration; ignore it for this unit test module.
warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401
        return
    except Exception:
        pass

    stub = types.ModuleType("cv2")

    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1
    stub.COLOR_BGR2RGB = 0
    stub.COLOR_BGR2GRAY = 1
    stub.COLOR_GRAY2BGR = 2
    stub.IMREAD_IGNORE_ORIENTATION = 128
    stub.IMREAD_COLOR = 1
    stub.RETR_LIST = 1
    stub.CHAIN_APPROX_SIMPLE = 2

    def _missing(*_args, **_kwargs):
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    def _module_getattr(name):
        if name.isupper():
            return 0
        return _missing

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()

from api.db.services.document_service import DocumentService  # noqa: E402
from common.constants import TaskStatus  # noqa: E402

# ---------------------------------------------------------------------------
# Helpers to access the original function bypassing @DB.connection_context()
# ---------------------------------------------------------------------------

def _unwrapped_get_parsing_status():
    """Return the original (un-decorated) get_parsing_status_by_kb_ids function.

    @classmethod + @DB.connection_context() together means:
      DocumentService.get_parsing_status_by_kb_ids.__func__  -> connection_context wrapper
      ....__func__.__wrapped__                               -> original function
    """
    return DocumentService.get_parsing_status_by_kb_ids.__func__.__wrapped__


# ---------------------------------------------------------------------------
# Fake ORM helpers – mimic the minimal peewee query chain used by the function
# ---------------------------------------------------------------------------

class _FieldStub:
    """Minimal stand-in for a peewee model field used in select/where/group_by."""

    def in_(self, values):
        """Called by .where(cls.model.kb_id.in_(kb_ids)) – no-op in tests."""
        return self

    def alias(self, name):
        return self


class _FakeQuery:
    """Chains .where(), .group_by(), .dicts() without touching a real database."""

    def __init__(self, rows):
        self._rows = rows

    def where(self, *_args, **_kwargs):
        return self

    def group_by(self, *_args, **_kwargs):
        return self

    def dicts(self):
        return list(self._rows)


def _make_fake_model(rows):
    """Create a fake Document model class whose select() returns *rows*."""

    class _FakeModel:
        id = _FieldStub()
        kb_id = _FieldStub()
        run = _FieldStub()

        @classmethod
        def select(cls, *_args):
            return _FakeQuery(rows)

    return _FakeModel


# ---------------------------------------------------------------------------
# Pytest fixture – patch DocumentService.model per test
# ---------------------------------------------------------------------------

@pytest.fixture()
def call_with_rows(monkeypatch):
    """Return a helper that runs get_parsing_status_by_kb_ids with fake DB rows."""

    def _call(rows, kb_ids):
        monkeypatch.setattr(DocumentService, "model", _make_fake_model(rows))
        fn = _unwrapped_get_parsing_status()
        return fn(DocumentService, kb_ids)

    return _call


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

_ALL_STATUS_FIELDS = frozenset(
    ["unstart_count", "running_count", "cancel_count", "done_count", "fail_count"]
)


@pytest.mark.p2
class TestGetParsingStatusByKbIds:

    # ------------------------------------------------------------------
    # Edge-case: empty input list – must short-circuit before any DB call
    # ------------------------------------------------------------------

    def test_empty_kb_ids_returns_empty_dict(self, call_with_rows):
        result = call_with_rows([], [])
        assert result == {}

    # ------------------------------------------------------------------
    # A kb_id present in the input but with no matching documents
    # ------------------------------------------------------------------

    def test_single_kb_id_no_documents(self, call_with_rows):
        result = call_with_rows(rows=[], kb_ids=["kb-1"])

        assert set(result.keys()) == {"kb-1"}
        assert set(result["kb-1"].keys()) == _ALL_STATUS_FIELDS
        assert all(v == 0 for v in result["kb-1"].values())

    # ------------------------------------------------------------------
    # A single kb_id with one document in each run-status bucket
    # ------------------------------------------------------------------

    def test_single_kb_id_all_five_statuses(self, call_with_rows):
        rows = [
            {"kb_id": "kb-1", "run": TaskStatus.UNSTART.value, "cnt": 3},
            {"kb_id": "kb-1", "run": TaskStatus.RUNNING.value, "cnt": 1},
            {"kb_id": "kb-1", "run": TaskStatus.CANCEL.value, "cnt": 2},
            {"kb_id": "kb-1", "run": TaskStatus.DONE.value, "cnt": 10},
            {"kb_id": "kb-1", "run": TaskStatus.FAIL.value, "cnt": 4},
        ]
        result = call_with_rows(rows=rows, kb_ids=["kb-1"])

        assert result["kb-1"]["unstart_count"] == 3
        assert result["kb-1"]["running_count"] == 1
        assert result["kb-1"]["cancel_count"] == 2
        assert result["kb-1"]["done_count"] == 10
        assert result["kb-1"]["fail_count"] == 4

    # ------------------------------------------------------------------
    # Two kb_ids – counts must be independent per dataset
    # ------------------------------------------------------------------

    def test_multiple_kb_ids_aggregated_separately(self, call_with_rows):
        rows = [
            {"kb_id": "kb-a", "run": TaskStatus.DONE.value, "cnt": 5},
            {"kb_id": "kb-a", "run": TaskStatus.FAIL.value, "cnt": 1},
            {"kb_id": "kb-b", "run": TaskStatus.UNSTART.value, "cnt": 7},
            {"kb_id": "kb-b", "run": TaskStatus.DONE.value, "cnt": 2},
        ]
        result = call_with_rows(rows=rows, kb_ids=["kb-a", "kb-b"])

        assert set(result.keys()) == {"kb-a", "kb-b"}

        assert result["kb-a"]["done_count"] == 5
        assert result["kb-a"]["fail_count"] == 1
        assert result["kb-a"]["unstart_count"] == 0
        assert result["kb-a"]["running_count"] == 0
        assert result["kb-a"]["cancel_count"] == 0

        assert result["kb-b"]["unstart_count"] == 7
        assert result["kb-b"]["done_count"] == 2
        assert result["kb-b"]["fail_count"] == 0

    # ------------------------------------------------------------------
    # An unrecognised run value must be silently ignored
    # ------------------------------------------------------------------

    def test_unknown_run_value_ignored(self, call_with_rows):
        rows = [
            {"kb_id": "kb-1", "run": "9", "cnt": 99},   # "9" is not a TaskStatus
            {"kb_id": "kb-1", "run": TaskStatus.DONE.value, "cnt": 4},
        ]
        result = call_with_rows(rows=rows, kb_ids=["kb-1"])

        assert result["kb-1"]["done_count"] == 4
        assert all(
            result["kb-1"][f] == 0
            for f in _ALL_STATUS_FIELDS - {"done_count"}
        )

    # ------------------------------------------------------------------
    # A row whose kb_id was NOT requested must not appear in the output
    # ------------------------------------------------------------------

    def test_row_with_unrequested_kb_id_is_filtered_out(self, call_with_rows):
        rows = [
            {"kb_id": "kb-requested", "run": TaskStatus.DONE.value, "cnt": 3},
            {"kb_id": "kb-unexpected", "run": TaskStatus.DONE.value, "cnt": 100},
        ]
        result = call_with_rows(rows=rows, kb_ids=["kb-requested"])

        assert "kb-unexpected" not in result
        assert result["kb-requested"]["done_count"] == 3

    # ------------------------------------------------------------------
    # cnt values must be treated as integers regardless of DB type hints
    # ------------------------------------------------------------------

    def test_cnt_is_cast_to_int(self, call_with_rows):
        rows = [
            {"kb_id": "kb-1", "run": TaskStatus.RUNNING.value, "cnt": "7"},
        ]
        result = call_with_rows(rows=rows, kb_ids=["kb-1"])

        assert result["kb-1"]["running_count"] == 7
        assert isinstance(result["kb-1"]["running_count"], int)

    # ------------------------------------------------------------------
    # run value stored as integer in DB (some adapters may omit str cast)
    # ------------------------------------------------------------------

    def test_run_value_as_integer_is_handled(self, call_with_rows):
        rows = [
            {"kb_id": "kb-1", "run": int(TaskStatus.DONE.value), "cnt": 5},
        ]
        result = call_with_rows(rows=rows, kb_ids=["kb-1"])

        assert result["kb-1"]["done_count"] == 5

    # ------------------------------------------------------------------
    # All five status fields are initialised to 0 even when no rows exist
    # ------------------------------------------------------------------

    def test_all_five_fields_initialised_to_zero(self, call_with_rows):
        result = call_with_rows(rows=[], kb_ids=["kb-empty"])

        assert result["kb-empty"] == {
            "unstart_count": 0,
            "running_count": 0,
            "cancel_count": 0,
            "done_count": 0,
            "fail_count": 0,
        }

    # ------------------------------------------------------------------
    # Multiple kb_ids in the input – all should appear in the result
    # even when no documents exist for some of them
    # ------------------------------------------------------------------

    def test_requested_kb_ids_all_present_in_result(self, call_with_rows):
        rows = [
            {"kb_id": "kb-with-data", "run": TaskStatus.DONE.value, "cnt": 1},
        ]
        result = call_with_rows(
            rows=rows, kb_ids=["kb-with-data", "kb-empty-1", "kb-empty-2"]
        )

        assert set(result.keys()) == {"kb-with-data", "kb-empty-1", "kb-empty-2"}
        assert result["kb-empty-1"] == {f: 0 for f in _ALL_STATUS_FIELDS}
        assert result["kb-empty-2"] == {f: 0 for f in _ALL_STATUS_FIELDS}

    # ------------------------------------------------------------------
    # SCHEDULE (run=="5") is not mapped – must be silently ignored
    # ------------------------------------------------------------------

    def test_schedule_status_is_not_mapped(self, call_with_rows):
        rows = [
            {"kb_id": "kb-1", "run": TaskStatus.SCHEDULE.value, "cnt": 3},
            {"kb_id": "kb-1", "run": TaskStatus.DONE.value, "cnt": 2},
        ]
        result = call_with_rows(rows=rows, kb_ids=["kb-1"])

        assert result["kb-1"]["done_count"] == 2
        # SCHEDULE is not a tracked bucket
        assert "schedule_count" not in result["kb-1"]
        assert all(
            result["kb-1"][f] == 0
            for f in _ALL_STATUS_FIELDS - {"done_count"}
        )
