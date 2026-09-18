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
import types
from contextlib import nullcontext
from pathlib import Path
from types import SimpleNamespace

import pytest


def _load_service(monkeypatch):
    class IntegrityError(Exception):
        pass

    class FakeDB:
        @staticmethod
        def connection_context():
            return lambda function: function

        @staticmethod
        def atomic():
            return nullcontext()

    peewee = types.ModuleType("peewee")
    peewee.IntegrityError = IntegrityError
    monkeypatch.setitem(sys.modules, "peewee", peewee)

    for name in ("api", "api.db", "common", "common.time_utils"):
        monkeypatch.setitem(sys.modules, name, types.ModuleType(name))

    db_models = types.ModuleType("api.db.db_models")
    db_models.DB = FakeDB()
    db_models.PipelineDSLVersion = object
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models)

    time_utils = sys.modules["common.time_utils"]
    time_utils.current_timestamp = lambda: 1
    time_utils.datetime_format = lambda value: value

    service_path = Path(__file__).resolve().parents[5] / "api" / "db" / "services" / "pipeline_dsl_version_service.py"
    spec = importlib.util.spec_from_file_location("_pipeline_dsl_version_service_test", service_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module, IntegrityError


class FakeVersionModel:
    rows = []
    create_hook = None

    @classmethod
    def reset(cls):
        cls.rows = []
        cls.create_hook = None

    @classmethod
    def create(cls, **kwargs):
        if cls.create_hook is not None:
            cls.create_hook(kwargs)
        row = SimpleNamespace(**kwargs)
        cls.rows.append(row)
        return row


@pytest.fixture
def version_service(monkeypatch):
    module, integrity_error = _load_service(monkeypatch)
    FakeVersionModel.reset()
    service = module.PipelineDSLVersionService
    service.model = FakeVersionModel
    monkeypatch.setattr(
        service,
        "_get_latest",
        classmethod(
            lambda cls, dsl_id: max(
                (row for row in FakeVersionModel.rows if row.dsl_id == dsl_id),
                key=lambda row: row.version,
                default=None,
            )
        ),
    )
    return module, service, integrity_error


def test_get_or_create_reuses_latest_and_appends_changes(version_service):
    _, service, _ = version_service

    first = service.get_or_create("canvas:pipeline-1", {"path": ["parser"], "revision": 1})
    reused = service.get_or_create("canvas:pipeline-1", {"revision": 1.0, "path": ["parser"]})
    second = service.get_or_create("canvas:pipeline-1", {"path": ["parser"], "revision": 2})
    reverted = service.get_or_create("canvas:pipeline-1", first.dsl)

    assert first.version == 1
    assert reused is first
    assert second.version == 2
    assert reverted.version == 3
    assert len(FakeVersionModel.rows) == 3


def test_get_or_create_reloads_after_concurrent_insert(version_service):
    _, service, integrity_error = version_service
    calls = 0

    def insert_competing_row(kwargs):
        nonlocal calls
        calls += 1
        if calls != 1:
            return
        FakeVersionModel.rows.append(SimpleNamespace(**kwargs))
        raise integrity_error("duplicate composite key")

    FakeVersionModel.create_hook = insert_competing_row
    result = service.get_or_create("canvas:pipeline-1", {"components": {}})

    assert result.version == 1
    assert result.dsl == {"components": {}}
    assert len(FakeVersionModel.rows) == 1


def test_get_or_create_bounds_conflict_retries(version_service):
    module, service, integrity_error = version_service

    def always_conflict(_kwargs):
        raise integrity_error("duplicate composite key")

    FakeVersionModel.create_hook = always_conflict
    with pytest.raises(RuntimeError, match="concurrent updates"):
        service.get_or_create("canvas:pipeline-1", {"components": {}})

    assert module.MAX_PIPELINE_DSL_VERSION_INSERT_ATTEMPTS == 16


@pytest.mark.parametrize(
    ("dsl_id", "dsl"),
    [
        ("", {}),
        ("   ", {}),
        ("canvas:pipeline-1", None),
        ("canvas:pipeline-1", []),
        ("canvas:pipeline-1", {"invalid": object()}),
    ],
)
def test_get_or_create_rejects_invalid_input(version_service, dsl_id, dsl):
    _, service, _ = version_service

    with pytest.raises(ValueError):
        service.get_or_create(dsl_id, dsl)
