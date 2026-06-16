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
"""Unit tests for the audit-log service (issue #14598)."""

from unittest.mock import MagicMock, patch

import pytest

from api.db.services.audit_log_service import (
    MAX_AUDIT_PAGE_SIZE,
    VALID_AUDIT_OPERATIONS,
    AuditLogService,
)


def test_valid_operations_constant():
    assert VALID_AUDIT_OPERATIONS == {"insert", "update", "delete"}


@pytest.mark.parametrize("bad_op", ["", "read", "select", "drop", None])
def test_log_rejects_invalid_operation(bad_op):
    with pytest.raises(ValueError):
        AuditLogService.log("tenant1", bad_op, "dataset")


@pytest.mark.parametrize("tenant,rtype", [("", "dataset"), ("t1", ""), (None, "dataset")])
def test_log_requires_tenant_and_resource_type(tenant, rtype):
    with pytest.raises(ValueError):
        AuditLogService.log(tenant, "insert", rtype)


def test_log_lowercases_operation_and_builds_record():
    with patch.object(AuditLogService, "insert", return_value="row") as ins:
        out = AuditLogService.log(
            "tenant1", "INSERT", "dataset",
            user_id="u1", resource_id="kb1", resource_name="My KB", detail={"k": "v"},
        )
    assert out == "row"
    kwargs = ins.call_args.kwargs
    assert kwargs["operation"] == "insert"          # lowercased
    assert kwargs["tenant_id"] == "tenant1"
    assert kwargs["resource_type"] == "dataset"
    assert kwargs["resource_id"] == "kb1"
    assert kwargs["resource_name"] == "My KB"
    assert kwargs["detail"] == {"k": "v"}
    assert kwargs["id"]                               # an id was generated


def test_log_defaults_detail_and_name():
    with patch.object(AuditLogService, "insert", return_value="row") as ins:
        AuditLogService.log("t1", "delete", "document", resource_id="d1")
    kwargs = ins.call_args.kwargs
    assert kwargs["detail"] == {}
    assert kwargs["resource_name"] == ""
    assert kwargs["user_id"] is None


def test_log_safe_swallows_errors():
    with patch.object(AuditLogService, "insert", side_effect=RuntimeError("db down")):
        assert AuditLogService.log_safe("t1", "insert", "dataset") is None


def test_log_safe_returns_row_on_success():
    with patch.object(AuditLogService, "insert", return_value="row"):
        assert AuditLogService.log_safe("t1", "insert", "dataset") == "row"


def _mock_query():
    """A chainable mock for AuditLog.select().where().order_by() supporting
    .count() and .paginate()."""
    q = MagicMock()
    q.where.return_value = q
    q.order_by.return_value = q
    q.count.return_value = 2
    row = MagicMock()
    row.to_dict.return_value = {"id": "a1", "operation": "insert"}
    q.paginate.return_value = [row, row]
    return q


def test_list_logs_returns_rows_and_total():
    q = _mock_query()
    with patch.object(AuditLogService.model, "select", return_value=q):
        rows, total = AuditLogService.list_logs("t1", page=1, page_size=30)
    assert total == 2
    assert rows == [{"id": "a1", "operation": "insert"}, {"id": "a1", "operation": "insert"}]
    q.paginate.assert_called_once_with(1, 30)


def test_list_logs_clamps_page_size():
    q = _mock_query()
    with patch.object(AuditLogService.model, "select", return_value=q):
        AuditLogService.list_logs("t1", page=0, page_size=10_000)
    # page floored to 1, page_size clamped to the max
    q.paginate.assert_called_once_with(1, MAX_AUDIT_PAGE_SIZE)
