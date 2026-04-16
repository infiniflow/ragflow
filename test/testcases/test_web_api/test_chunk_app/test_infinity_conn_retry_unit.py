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

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _FakeLogger:
    def __init__(self):
        self.messages = {"debug": [], "warning": [], "error": [], "info": []}

    def debug(self, *args, **kwargs):
        self.messages["debug"].append((args, kwargs))

    def warning(self, *args, **kwargs):
        self.messages["warning"].append((args, kwargs))

    def error(self, *args, **kwargs):
        self.messages["error"].append((args, kwargs))

    def info(self, *args, **kwargs):
        self.messages["info"].append((args, kwargs))


class _FakeColumns:
    def rows(self):
        return []


class _FakeTable:
    def __init__(self, outcomes):
        self._outcomes = list(outcomes)
        self.update_calls = []

    def show_columns(self):
        return _FakeColumns()

    def update(self, filter_expr, payload):
        self.update_calls.append((filter_expr, payload))
        outcome = self._outcomes.pop(0)
        if isinstance(outcome, Exception):
            raise outcome
        return outcome


class _FakeDatabase:
    def __init__(self, table):
        self._table = table

    def get_table(self, _name):
        return self._table


class _FakeConn:
    def __init__(self, table):
        self._db = _FakeDatabase(table)

    def get_database(self, _db_name):
        return self._db


class _FakeConnPool:
    def __init__(self, table):
        self._conn = _FakeConn(table)
        self.release_count = 0

    def get_conn(self):
        return self._conn

    def release_conn(self, _conn):
        self.release_count += 1


def _load_infinity_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    decorator_mod = ModuleType("common.decorator")
    decorator_mod.singleton = lambda cls, *args, **kwargs: cls
    monkeypatch.setitem(sys.modules, "common.decorator", decorator_mod)

    doc_store_pkg = ModuleType("common.doc_store")
    doc_store_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "common.doc_store", doc_store_pkg)

    doc_store_base_mod = ModuleType("common.doc_store.doc_store_base")
    doc_store_base_mod.MatchExpr = type("MatchExpr", (), {})
    doc_store_base_mod.MatchTextExpr = type("MatchTextExpr", (), {})
    doc_store_base_mod.MatchDenseExpr = type("MatchDenseExpr", (), {})
    doc_store_base_mod.FusionExpr = type("FusionExpr", (), {})
    doc_store_base_mod.OrderByExpr = type("OrderByExpr", (), {})
    monkeypatch.setitem(sys.modules, "common.doc_store.doc_store_base", doc_store_base_mod)

    infinity_base_mod = ModuleType("common.doc_store.infinity_conn_base")

    class _InfinityConnectionBase:
        @staticmethod
        def list2str(lst, sep=" "):
            if isinstance(lst, str):
                return lst
            return sep.join(lst)

        def equivalent_condition_to_str(self, condition, _table_instance=None):
            clauses = []
            for key, value in condition.items():
                if isinstance(value, str):
                    clauses.append(f"{key}='{value}'")
                else:
                    clauses.append(f"{key}={value}")
            return " AND ".join(clauses) if clauses else "1=1"

    infinity_base_mod.InfinityConnectionBase = _InfinityConnectionBase
    monkeypatch.setitem(sys.modules, "common.doc_store.infinity_conn_base", infinity_base_mod)

    constants_mod = ModuleType("common.constants")
    constants_mod.PAGERANK_FLD = "pagerank_flt"
    constants_mod.TAG_FLD = "tag_kwd"
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    infinity_pkg = ModuleType("infinity")
    monkeypatch.setitem(sys.modules, "infinity", infinity_pkg)

    infinity_common_mod = ModuleType("infinity.common")

    class _FakeInfinityException(Exception):
        def __init__(self, error_code, error_msg):
            super().__init__(error_msg)
            self.error_code = error_code
            self.error_msg = error_msg

    infinity_common_mod.InfinityException = _FakeInfinityException
    infinity_common_mod.SortType = SimpleNamespace(Asc="asc", Desc="desc")
    monkeypatch.setitem(sys.modules, "infinity.common", infinity_common_mod)

    infinity_errors_mod = ModuleType("infinity.errors")
    infinity_errors_mod.ErrorCode = SimpleNamespace(OK=0)
    monkeypatch.setitem(sys.modules, "infinity.errors", infinity_errors_mod)

    module_name = "test_infinity_conn_retry_unit_target"
    spec = importlib.util.spec_from_file_location(
        module_name, repo_root / "rag" / "utils" / "infinity_conn.py"
    )
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


def _build_connection(module, table):
    conn = module.InfinityConnection.__new__(module.InfinityConnection)
    conn.dbName = "default_db"
    conn.connPool = _FakeConnPool(table)
    conn.logger = _FakeLogger()
    return conn


def test_update_retries_on_transaction_conflict(monkeypatch):
    module = _load_infinity_module(monkeypatch)
    table = _FakeTable(
        [
            module.InfinityException(4002, "Txn conflict reason: Conflict with candidate_txn 10168"),
            True,
        ]
    )
    conn = _build_connection(module, table)

    ok = conn.update(
        {"id": "chunk-1"},
        {"content_with_weight": "updated content"},
        "ragflow_tenant",
        "kb-1",
        max_retries=2,
        retry_delay=0,
    )

    assert ok is True
    assert table.update_calls == [
        ("id='chunk-1'", {"content": "updated content"}),
        ("id='chunk-1'", {"content": "updated content"}),
    ]
    assert len(conn.logger.messages["warning"]) == 1
    assert conn.connPool.release_count == 2


def test_update_does_not_retry_non_conflict_exception(monkeypatch):
    module = _load_infinity_module(monkeypatch)
    table = _FakeTable(
        [module.InfinityException(5001, "table not found")]
    )
    conn = _build_connection(module, table)

    with pytest.raises(module.InfinityException, match="table not found"):
        conn.update(
            {"id": "chunk-1"},
            {"content_with_weight": "updated content"},
            "ragflow_tenant",
            "kb-1",
            max_retries=2,
            retry_delay=0,
        )

    assert table.update_calls == [
        ("id='chunk-1'", {"content": "updated content"})
    ]
    assert conn.logger.messages["warning"] == []
    assert conn.connPool.release_count == 1
