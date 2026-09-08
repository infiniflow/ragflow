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
from types import ModuleType


def _install_module(monkeypatch, name, **attrs):
    module = ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    monkeypatch.setitem(sys.modules, name, module)
    return module


def _load_feedback_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[5]
    common_pkg = _install_module(monkeypatch, "common")
    constants_mod = _install_module(monkeypatch, "common.constants", PAGERANK_FLD="pagerank_fea")
    settings_mod = _install_module(monkeypatch, "common.settings", DOC_ENGINE="gaussdb")
    common_pkg.constants = constants_mod
    common_pkg.settings = settings_mod

    _install_module(monkeypatch, "rag")
    _install_module(monkeypatch, "rag.nlp")
    _install_module(monkeypatch, "rag.nlp.search", index_name=lambda tenant_id: f"ragflow_{tenant_id}")

    spec = importlib.util.spec_from_file_location(
        "api.db.services.chunk_feedback_service",
        repo_root / "api" / "db" / "services" / "chunk_feedback_service.py",
    )
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "api.db.services.chunk_feedback_service", module)
    spec.loader.exec_module(module)
    return module, settings_mod


class FakeGaussDBStore:
    def __init__(self):
        self.updated = []
        self.adjusted = []
        self.rows = {"chunk1": {"pagerank_fea": 10}}

    def db_type(self):
        return "gaussdb"

    def adjust_chunk_pagerank_fea(self, *args, **kwargs):
        self.adjusted.append((args, kwargs))
        return True

    def get(self, chunk_id, index_name, kb_ids):
        return self.rows.get(chunk_id)

    def update(self, condition, values, index_name, kb_id):
        self.updated.append((condition, values, index_name, kb_id))
        return True


class FakeGaussDBStoreWithoutAdjust:
    def __init__(self):
        self.updated = []
        self.rows = {"chunk1": {"pagerank_fea": 10}}

    def db_type(self):
        return "gaussdb"

    def get(self, chunk_id, index_name, kb_ids):
        return self.rows.get(chunk_id)

    def update(self, condition, values, index_name, kb_id):
        self.updated.append((condition, values, index_name, kb_id))
        return True


def test_update_chunk_weight_falls_back_to_update_by_id_and_kb_without_adjust(monkeypatch):
    module, settings_mod = _load_feedback_module(monkeypatch)
    fake = FakeGaussDBStoreWithoutAdjust()
    settings_mod.docStoreConn = fake

    ok = module.ChunkFeedbackService.update_chunk_weight(
        tenant_id="tenant",
        chunk_id="chunk1",
        kb_id="kb1",
        delta=2,
    )

    assert ok is True
    assert fake.updated == [({"id": "chunk1"}, {"pagerank_fea": 12}, "ragflow_tenant", "kb1")]


def test_update_chunk_weight_routes_gaussdb_through_atomic_adjust(monkeypatch):
    module, settings_mod = _load_feedback_module(monkeypatch)
    fake = FakeGaussDBStore()
    settings_mod.docStoreConn = fake

    ok = module.ChunkFeedbackService.update_chunk_weight(
        tenant_id="tenant",
        chunk_id="chunk1",
        kb_id="kb1",
        delta=2,
    )

    assert ok is True
    assert fake.adjusted == [
        (
            ("chunk1", "ragflow_tenant", "kb1", 2.0, module.MIN_PAGERANK_WEIGHT, module.MAX_PAGERANK_WEIGHT),
            {},
        )
    ]
    assert fake.updated == []
