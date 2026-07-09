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
"""Regression tests for SyncLogsService.list_sync_tasks PostgreSQL compatibility.

Issue: infiniflow/ragflow#16776.

PostgreSQL rejects ``SELECT DISTINCT ... ORDER BY <col>`` when the ORDER BY
column is not in the select list. ``SyncLogsService.list_sync_tasks`` builds a
``fields`` array that did not include ``update_time`` while ordering by it,
which crashes the connector logs API on PostgreSQL with::

    psycopg2.errors.InvalidColumnReference:
        for SELECT DISTINCT, ORDER BY expressions must appear in select list

These tests pin the contract that ``list_sync_tasks`` includes ``update_time``
in its selected fields whenever it orders by it, so the bug stays fixed.
"""

import sys
import types
import warnings
from pathlib import Path
from types import SimpleNamespace

import pytest

warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


class _FakeOrderField:
    def desc(self):
        return self

    def asc(self):
        return self

    def alias(self, _name):
        return self


class _FakeField(_FakeOrderField):
    def __eq__(self, _other):
        return self

    def in_(self, _others):
        return self

    def not_in(self, _others):
        return self


class _FakeQuery:
    """Captures the chain of calls so we can assert what was selected and ordered."""

    def __init__(self, *select_args):
        self.select_args = list(select_args)
        self.joins = []
        self.wheres = []
        self.order_args = []
        self._distinct_applied = False
        self._paginated = None

    def join(self, *args, **kwargs):
        self.joins.append((args, kwargs))
        return self

    def where(self, *args, **kwargs):
        self.wheres.append((args, kwargs))
        return self

    def distinct(self):
        self._distinct_applied = True
        return self

    def order_by(self, *args, **kwargs):
        self.order_args.append((args, kwargs))
        return self

    def count(self):
        return 0

    def paginate(self, page, page_size):
        self._paginated = (page, page_size)
        return self

    def dicts(self):
        return []


def _fake_field(label):
    """A field whose __str__ prints a recognisable SQL fragment."""

    class _Label:
        def __init__(self, label):
            self.label = label

        def alias(self, _alias):
            return self

        def desc(self):
            return ("order-desc", self.label)

        def asc(self):
            return ("order-asc", self.label)

        def __eq__(self, _other):
            return ("eq", self.label)

        def in_(self, _others):
            return ("in", self.label)

        def __repr__(self):
            return self.label

    return _Label(label)


def _build_connector_service(monkeypatch, db_type):
    """Wire a minimal `connector_service` module surface and return the captured query.

    Strategy: build a fake ``api`` package rooted at the real repo path, but
    replace the heavy leaves (``api.db``, ``api.db.db_models``,
    ``api.db.services.common_service``) with stubs. ``common_service.CommonService``
    is loaded from the real file because it is dependency-free.
    """

    import importlib.util

    repo_root = Path(__file__).resolve().parents[5]

    # 1) Build the real `api` package with its real path so sub-packages
    # resolve correctly.
    for stale in [
        "api",
        "api.db",
        "api.db.db_models",
        "api.db.services",
        "api.db.services.connector_service",
        "api.db.services.common_service",
        "api.db.services.document_service",
        "api.utils",
        "api.utils.common",
        "common",
        "common.constants",
        "common.settings",
        "common.time_utils",
        "common.misc_utils",
    ]:
        sys.modules.pop(stale, None)

    api_pkg = types.ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    sys.modules["api"] = api_pkg

    api_db_pkg = types.ModuleType("api.db")
    api_db_pkg.__path__ = [str(repo_root / "api" / "db")]

    class _DB:
        @staticmethod
        def connection_context():
            class _Ctx:
                def __enter__(self):
                    return self

                def __exit__(self, *args):
                    return False

            return _Ctx()

    # `DB.connection_context()` is invoked as a decorator at class-definition
    # time. In peewee, ``Database.connection_context`` is a class-level
    # function (not a method). For our unit-test stub, we just need it to
    # return a decorator that wraps the function in a no-op.

    class _NoOpBoundMethod:
        def __init__(self, fn):
            self._fn = fn

        def __call__(self, *args, **kwargs):
            return self._fn(*args, **kwargs)

    def _decorator_factory(fn):
        return _NoOpBoundMethod(fn)

    class _DBCls:
        connection_context = staticmethod(lambda: _decorator_factory)

    api_db_pkg.DB = _DBCls
    api_db_pkg.InputType = SimpleNamespace(POLL="POLL")
    sys.modules["api.db"] = api_db_pkg

    # 2) Stub api.db.db_models with fake models. We need this in place before
    # `connector_service` is imported because it imports models at module
    # load time.
    captured = {}

    def _model_select(*fields):
        captured["fields"] = list(fields)
        return _FakeQuery(*fields)

    sync_model = SimpleNamespace(
        id=_fake_field("id"),
        connector_id=_fake_field("connector_id"),
        task_type=_fake_field("task_type"),
        kb_id=_fake_field("kb_id"),
        update_date=_fake_field("update_date"),
        new_docs_indexed=_fake_field("new_docs_indexed"),
        total_docs_indexed=_fake_field("total_docs_indexed"),
        docs_removed_from_index=_fake_field("docs_removed_from_index"),
        error_msg=_fake_field("error_msg"),
        error_count=_fake_field("error_count"),
        time_started=_fake_field("time_started"),
        status=_fake_field("status"),
        update_time=_fake_field("update_time"),
        select=_model_select,
    )

    connector_model = SimpleNamespace(
        id=_fake_field("c_id"),
        input_type=_fake_field("c_input_type"),
        status=_fake_field("c_status"),
        config=_fake_field("c_config"),
        refresh_freq=_fake_field("refresh_freq"),
        prune_freq=_fake_field("prune_freq"),
    )

    c2k_model = SimpleNamespace(
        kb_id=_fake_field("c2k_kb_id"),
        connector_id=_fake_field("c2k_connector_id"),
    )

    kb_model = SimpleNamespace(
        id=_fake_field("kb_id"),
        name=_fake_field("kb_name"),
    )

    db_models_pkg = types.ModuleType("api.db.db_models")
    db_models_pkg.SyncLogs = sync_model
    db_models_pkg.Connector = connector_model
    db_models_pkg.Connector2Kb = c2k_model
    db_models_pkg.Knowledgebase = kb_model
    db_models_pkg.DB = _DBCls
    sys.modules["api.db.db_models"] = db_models_pkg

    # 3) Wire api.utils.common with a stub for hash128.
    api_utils_pkg = types.ModuleType("api.utils")
    api_utils_pkg.__path__ = [str(repo_root / "api" / "utils")]
    sys.modules["api.utils"] = api_utils_pkg

    api_utils_common = types.ModuleType("api.utils.common")
    api_utils_common.hash128 = lambda *args, **kwargs: "hash"
    sys.modules["api.utils.common"] = api_utils_common

    # 4) Wire common.* stubs.
    common_pkg = types.ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    sys.modules["common"] = common_pkg

    common_constants = types.ModuleType("common.constants")
    common_constants.ConnectorTaskType = SimpleNamespace(SYNC="sync")
    common_constants.TaskStatus = SimpleNamespace(
        UNSTART="unstart",
        SCHEDULE="schedule",
        CANCEL="cancel",
        RUNNING="running",
    )
    sys.modules["common.constants"] = common_constants

    common_settings = types.ModuleType("common.settings")
    common_settings.TIMEZONE = "UTC"
    sys.modules["common.settings"] = common_settings

    common_time_utils = types.ModuleType("common.time_utils")
    common_time_utils.current_timestamp = lambda: "2026-01-01 00:00:00"
    common_time_utils.timestamp_to_date = lambda *args, **kwargs: "2026-01-01"
    sys.modules["common.time_utils"] = common_time_utils

    common_misc_utils = types.ModuleType("common.misc_utils")
    common_misc_utils.get_uuid = lambda: "uuid"
    sys.modules["common.misc_utils"] = common_misc_utils

    # 5) Wire api.db.services as a real package so submodules resolve.
    services_pkg = types.ModuleType("api.db.services")
    services_pkg.__path__ = [str(repo_root / "api" / "db" / "services")]
    sys.modules["api.db.services"] = services_pkg

    # 6) Stub api.db.services.common_service with a no-deps base class. We
    # don't need any of its helpers (they're not exercised by
    # `list_sync_tasks`).
    common_service_mod = types.ModuleType("api.db.services.common_service")

    class _CommonService:
        model = None

    common_service_mod.CommonService = _CommonService
    sys.modules["api.db.services.common_service"] = common_service_mod

    # 7) Stub api.db.services.document_service to avoid its heavy imports
    # (used only as a class reference inside `connector_service.py`).
    document_service_mod = types.ModuleType("api.db.services.document_service")
    document_service_mod.DocumentService = SimpleNamespace()
    document_service_mod.DocMetadataService = SimpleNamespace()
    sys.modules["api.db.services.document_service"] = document_service_mod

    # 8) Load the real connector_service.py.
    connector_service_path = repo_root / "api" / "db" / "services" / "connector_service.py"
    spec = importlib.util.spec_from_file_location(
        "api.db.services.connector_service", connector_service_path
    )
    connector_service_mod = importlib.util.module_from_spec(spec)
    sys.modules["api.db.services.connector_service"] = connector_service_mod
    spec.loader.exec_module(connector_service_mod)

    # Hold a reference so monkeypatch can't GC them mid-test.
    setattr(connector_service_mod, "_TEST_CAPTURED", captured)

    return connector_service_mod, captured


@pytest.mark.p2
def test_list_sync_tasks_includes_update_time_in_select(monkeypatch):
    """Regression for #16776: update_time must appear in the selected fields so
    PostgreSQL accepts ``SELECT DISTINCT ... ORDER BY update_time``."""
    monkeypatch.setenv("DB_TYPE", "postgres")
    connector_service_mod, captured = _build_connector_service(monkeypatch, "postgres")

    rows, total = connector_service_mod.SyncLogsService.list_sync_tasks(
        connector_id="connector-1", page_number=1, items_per_page=15
    )

    assert rows == []
    assert total == 0
    assert "fields" in captured, "list_sync_tasks must call cls.model.select(...)"
    select_labels = [
        getattr(f, "label", None)
        or (f[1] if isinstance(f, tuple) else getattr(f, "__repr__", lambda: str(f))())
        for f in captured["fields"]
    ]
    assert "update_time" in select_labels, (
        "SyncLogsService.list_sync_tasks must include cls.model.update_time in "
        "the SELECT list when ordering by it; otherwise PostgreSQL raises "
        "'for SELECT DISTINCT, ORDER BY expressions must appear in select list'."
    )


@pytest.mark.p2
def test_list_sync_tasks_orders_by_update_time_desc(monkeypatch):
    """Companion to the regression above: the ORDER BY clause still uses update_time."""
    monkeypatch.setenv("DB_TYPE", "postgres")

    queries = []

    def _model_select(*fields):
        q = _FakeQuery(*fields)
        queries.append(q)
        return q

    connector_service_mod, _ = _build_connector_service(monkeypatch, "postgres")

    # Patch the model.select on the imported service to record queries.
    monkeypatch.setattr(
        connector_service_mod.SyncLogs, "select", _model_select
    )

    connector_service_mod.SyncLogsService.list_sync_tasks(
        connector_id="connector-1", page_number=2, items_per_page=15
    )

    assert queries, "list_sync_tasks must build at least one select query"
    q = queries[-1]
    assert q.order_args, "list_sync_tasks must call order_by(...)"
    order_first = q.order_args[0][0][0]
    desc_marker = order_first[0] if isinstance(order_first, tuple) else None
    label = order_first[1] if isinstance(order_first, tuple) else None
    assert desc_marker == "order-desc", (
        f"Expected ORDER BY to be DESC; got descriptor {desc_marker!r}"
    )
    assert label == "update_time", (
        f"Expected ORDER BY update_time DESC; got ORDER BY {label!r}"
    )
