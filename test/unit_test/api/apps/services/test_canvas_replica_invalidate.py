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
"""CanvasReplicaService.invalidate_canvas drops every tenant's replica of one canvas."""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _FakeRedis:
    def __init__(self, keys):
        self.store = set(keys)

    def scan_iter(self, match, count=10):
        prefix = match[:-1]
        return iter([k for k in sorted(self.store) if k.startswith(prefix)])

    def delete(self, key):
        if key not in self.store:
            return 0
        self.store.remove(key)
        return 1


def _load_service(monkeypatch, fake_redis):
    def stub(name, **attrs):
        mod = ModuleType(name)
        for key, value in attrs.items():
            setattr(mod, key, value)
        monkeypatch.setitem(sys.modules, name, mod)

    stub("api.db", CanvasCategory=SimpleNamespace(Agent="agent"))
    stub("agent.dsl_migration", normalize_chunker_dsl=lambda dsl: dsl)
    stub("rag.utils.redis_conn", REDIS_CONN=SimpleNamespace(REDIS=fake_redis), RedisDistributedLock=object)
    module_path = Path(__file__).resolve().parents[5] / "api" / "apps" / "services" / "canvas_replica_service.py"
    spec = importlib.util.spec_from_file_location("test_canvas_replica_service", module_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.CanvasReplicaService


@pytest.mark.p1
def test_invalidate_canvas_removes_only_that_canvas(monkeypatch):
    redis = _FakeRedis(
        [
            "canvas:replica:canvas-a:tenant-owner:tenant-owner",
            "canvas:replica:canvas-a:tenant-api:tenant-api",
            "canvas:replica:canvas-a:tenant-member:tenant-member",
            "canvas:replica:canvas-b:tenant-owner:tenant-owner",
            "canvas:replica:lock:canvas-a:tenant-owner:tenant-owner",
        ]
    )
    service = _load_service(monkeypatch, redis)

    assert service.invalidate_canvas("canvas-a") == 3
    assert redis.store == {
        "canvas:replica:canvas-b:tenant-owner:tenant-owner",
        "canvas:replica:lock:canvas-a:tenant-owner:tenant-owner",
    }


@pytest.mark.p1
def test_invalidate_canvas_without_redis_is_noop(monkeypatch):
    service = _load_service(monkeypatch, None)
    assert service.invalidate_canvas("canvas-a") == 0
