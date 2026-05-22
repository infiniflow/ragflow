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
"""Unit tests for the global account-role permission layer (issue #5965).

Covers ``is_readonly_account`` and the ``@require_admin_account`` decorator in
``api/common/permission.py``: read-only ("user" tier) accounts must be rejected
from mutation routes, administrators pass through.
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_permission(monkeypatch, account_role):
    """Load api/common/permission.py in isolation; ``current_user`` carries
    the given ``account_role``."""
    _stub(monkeypatch, "api.db", AccountRole=SimpleNamespace(ADMIN="admin", USER="user"))
    _stub(monkeypatch, "api.apps", current_user=SimpleNamespace(id="u-1", account_role=account_role))
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        get_error_permission_result=lambda message="Permission error": {"code": 108, "message": message, "data": None},
    )

    repo_root = Path(__file__).resolve().parents[4]
    module_path = repo_root / "api" / "common" / "permission.py"
    spec = importlib.util.spec_from_file_location("test_permission_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_permission_module", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
class TestRequireAdminAccount:
    def test_admin_passes_through(self, monkeypatch):
        module = _load_permission(monkeypatch, account_role="admin")

        @module.require_admin_account
        async def handler():
            return {"ok": True}

        assert asyncio.run(handler()) == {"ok": True}

    def test_readonly_user_is_blocked(self, monkeypatch):
        """A read-only account never reaches the handler and gets a 108 error."""
        module = _load_permission(monkeypatch, account_role="user")
        called = []

        @module.require_admin_account
        async def handler():
            called.append(True)
            return {"ok": True}

        result = asyncio.run(handler())
        assert result["code"] == 108
        assert not called

    def test_sync_handler_supported(self, monkeypatch):
        module = _load_permission(monkeypatch, account_role="admin")

        @module.require_admin_account
        def sync_handler():
            return "done"

        assert asyncio.run(sync_handler()) == "done"

    def test_is_readonly_account(self, monkeypatch):
        module = _load_permission(monkeypatch, account_role="admin")
        assert module.is_readonly_account(SimpleNamespace(account_role="user")) is True
        assert module.is_readonly_account(SimpleNamespace(account_role="admin")) is False
        # Accounts predating the feature have no account_role -> treated as admin.
        assert module.is_readonly_account(SimpleNamespace()) is False
