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
"""Test fixtures for the ``admin`` unit tests.

Loading ``admin.server.auth`` for real pulls in the full api db layer
(peewee, flask, elasticsearch, numpy, ...). That is far heavier than
the env-var wiring under test, so the :func:`reload_admin_auth` fixture
installs lightweight ``sys.modules`` stubs for every transitive import
the bootstrap module references, then re-imports ``admin.server.auth``
against those stubs.

Every stub entry is registered through ``monkeypatch.setitem`` and the
``admin.server.auth`` module itself is removed from ``sys.modules`` on
teardown, so each test starts from a clean ``sys.modules`` slate and
the stubbed surface never leaks into unrelated tests.
"""

from __future__ import annotations

import importlib
import sys
import types
from pathlib import Path
from unittest.mock import MagicMock

import pytest


_REPO_ROOT = Path(__file__).resolve().parents[3]
_AUTH_PY = _REPO_ROOT / "admin" / "server" / "auth.py"


# ---------------------------------------------------------------------------
# Stub installation
# ---------------------------------------------------------------------------


class _DummyEnum:
    """Tiny stand-in for ``enum.Enum`` values used by the bootstrap code.

    The production code accesses both ``ActiveEnum.ACTIVE.value`` (class
    attribute) and ``ActiveEnum.INACTIVE.value``, so the stub exposes
    both shapes: ``ACTIVE``/``INACTIVE``/``VALID``/``INVALID`` as class
    attributes holding a ``_DummyEnum`` instance, plus a per-instance
    ``.value`` so callers that hold an instance behave the same way.
    """

    ACTIVE = None  # populated below
    INACTIVE = None
    VALID = None
    INVALID = None

    def __init__(self, value):
        self.value = value


# Wire up the class-attribute enum members now that the class body has
# finished executing.
_DummyEnum.ACTIVE = _DummyEnum("1")
_DummyEnum.INACTIVE = _DummyEnum("0")
_DummyEnum.VALID = _DummyEnum("1")
_DummyEnum.INVALID = _DummyEnum("0")


def _build_stub_modules() -> dict[str, types.ModuleType]:
    """Build and link a fresh set of stub modules for ``admin.server.auth``.

    Returns a ``{name: module}`` mapping that the caller registers into
    ``sys.modules`` (and that the ``monkeypatch`` fixture then restores
    on teardown).

    Only the symbols ``admin.server.auth`` actually references are
    stubbed — the production code is the thing under test, so the
    stubbed surface has to match what production sees at import time.
    """

    stubs: dict[str, types.ModuleType] = {}

    # --- common.* -----------------------------------------------------
    common = types.ModuleType("common")
    stubs["common"] = common

    constants = types.ModuleType("common.constants")
    constants.ActiveEnum = _DummyEnum
    constants.StatusEnum = _DummyEnum
    stubs["common.constants"] = constants

    misc_utils = types.ModuleType("common.misc_utils")
    misc_utils.get_uuid = lambda: "test-access-token-uuid"
    stubs["common.misc_utils"] = misc_utils

    time_utils = types.ModuleType("common.time_utils")
    time_utils.current_timestamp = lambda: 1700000000
    time_utils.datetime_format = lambda dt: "2026-01-01T00:00:00Z"
    time_utils.get_format_time = lambda: "2026-01-01T00:00:00Z"
    stubs["common.time_utils"] = time_utils

    connection_utils = types.ModuleType("common.connection_utils")
    connection_utils.sync_construct_response = lambda **kwargs: {
        "code": 0,
        "data": kwargs.get("data"),
        "message": kwargs.get("message", ""),
    }
    stubs["common.connection_utils"] = connection_utils

    settings = types.ModuleType("common.settings")
    settings.CHAT_MDL = "chat-model"
    settings.EMBEDDING_MDL = "embedding-model"
    settings.ASR_MDL = "asr-model"
    settings.PARSERS = ["naive"]
    settings.IMAGE2TEXT_MDL = "image2text-model"
    settings.RERANK_MDL = "rerank-model"
    stubs["common.settings"] = settings

    # Wire the children up under their parent so ``from common import X``
    # resolves correctly.
    for child_name in (
        "constants",
        "misc_utils",
        "time_utils",
        "connection_utils",
        "settings",
    ):
        setattr(common, child_name, stubs[f"common.{child_name}"])

    # --- api.* -------------------------------------------------------
    api = types.ModuleType("api")
    stubs["api"] = api

    api_common = types.ModuleType("api.common")
    stubs["api.common"] = api_common

    exceptions = types.ModuleType("api.common.exceptions")

    class AdminException(Exception):
        def __init__(self, message: str, code: int = 500):
            super().__init__(message)
            self.code = code

    class UserNotFoundError(Exception):
        def __init__(self, email: str):
            super().__init__(email)
            self.email = email

    exceptions.AdminException = AdminException
    exceptions.UserNotFoundError = UserNotFoundError
    stubs["api.common.exceptions"] = exceptions

    base64_mod = types.ModuleType("api.common.base64")

    def encode_to_base64(s: str) -> str:
        import base64 as _b64

        return _b64.b64encode(s.encode("utf-8")).decode("utf-8")

    base64_mod.encode_to_base64 = encode_to_base64
    stubs["api.common.base64"] = base64_mod

    api_utils = types.ModuleType("api.utils")
    crypt = types.ModuleType("api.utils.crypt")
    crypt.decrypt = lambda s: s
    stubs["api.utils"] = api_utils
    stubs["api.utils.crypt"] = crypt

    api_db = types.ModuleType("api.db")

    class _UserTenantRole:
        OWNER = "owner"

    api_db.UserTenantRole = _UserTenantRole

    api_db_services = types.ModuleType("api.db.services")
    # UserService is patched per-test (see ``reload_admin_auth`` below);
    # install a MagicMock here so the ``from api.db.services import
    # UserService`` line in admin.server.auth resolves during import.
    api_db_services.UserService = MagicMock(name="UserService")
    stubs["api.db.services"] = api_db_services

    user_service_mod = types.ModuleType("api.db.services.user_service")
    user_service_mod.TenantService = MagicMock(name="TenantService")
    user_service_mod.UserTenantService = MagicMock(name="UserTenantService")
    stubs["api.db.services.user_service"] = user_service_mod

    stubs["api.db"] = api_db

    # Cross-link parent → child attributes so ``from api.X import Y``
    # resolves during import. Direct attribute assignment (rather than
    # ``setattr`` with a constant name) avoids Ruff B010 — the parent
    # modules are local to this function so the attribute names are
    # known statically.
    api.common = api_common
    api.utils = api_utils
    api.db = api_db
    api_common.exceptions = exceptions
    api_common.base64 = base64_mod
    api_utils.crypt = crypt
    api_db.services = api_db_services
    api_db_services.user_service = user_service_mod

    # --- admin.* -----------------------------------------------------
    admin = types.ModuleType("admin")
    admin_server = types.ModuleType("admin.server")
    admin_server.__path__ = [str(_REPO_ROOT / "admin" / "server")]  # type: ignore[attr-defined]
    stubs["admin"] = admin
    stubs["admin.server"] = admin_server

    admin.server = admin_server

    return stubs


def _reload_admin_auth(
    monkeypatch: pytest.MonkeyPatch,
    stubs: dict[str, types.ModuleType],
) -> types.ModuleType:
    """Drop any cached ``admin.server.auth`` and re-import it against
    the currently registered stubs.

    Re-importing is required because the bootstrap module reads
    ``DEFAULT_SUPERUSER_*`` exactly once at module load time, so tests
    that need different env values must trigger a fresh import.

    The freshly imported module is registered via
    ``monkeypatch.setitem`` so its prior identity (the real production
    module loaded elsewhere, or ``None`` if it has never been imported)
    is restored on fixture teardown — bypassing ``monkeypatch`` here
    would silently leak the freshly imported module into subsequent
    tests in the same pytest session.
    """
    sys.modules.pop("admin.server.auth", None)
    spec = importlib.util.spec_from_file_location("admin.server.auth", _AUTH_PY)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "admin.server.auth", module)
    spec.loader.exec_module(module)  # type: ignore[union-attr]
    return module


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def reload_admin_auth(monkeypatch):
    """Return a callable that re-imports ``admin.server.auth`` against
    a fresh set of ``sys.modules`` stubs registered via
    ``monkeypatch.setitem``.

    Usage::

        def test_xyz(reload_admin_auth, monkeypatch):
            monkeypatch.setenv("DEFAULT_SUPERUSER_EMAIL", "ops@example.com")
            module = reload_admin_auth()
            assert module.DEFAULT_SUPERUSER_EMAIL == "ops@example.com"

    The bootstrap module reads ``os.getenv`` exactly once at import
    time, so a fresh reload is the only way to assert different env
    values flow through ``init_default_admin`` / ``check_admin``.

    Each reload rebinds ``UserService`` / ``TenantService`` /
    ``UserTenantService`` on the freshly imported module to fresh
    ``MagicMock`` instances, so the bootstrap functions run end-to-end
    against the production source without touching the database.

    All stub modules (and the imported ``admin.server.auth`` itself)
    are removed from ``sys.modules`` on teardown via ``monkeypatch``,
    so the stubbed surface never leaks into unrelated tests.
    """

    stubs = _build_stub_modules()
    for name, module in stubs.items():
        monkeypatch.setitem(sys.modules, name, module)

    def _reload():
        module = _reload_admin_auth(monkeypatch, stubs)
        # Replace the MagicMocks from the stub install with fresh
        # per-test instances so each test gets its own assertion
        # surface. ``monkeypatch.setattr`` restores the originals
        # automatically on teardown.
        fresh_user = MagicMock(name="UserService")
        fresh_tenant = MagicMock(name="TenantService")
        fresh_user_tenant = MagicMock(name="UserTenantService")
        monkeypatch.setattr(module, "UserService", fresh_user)
        monkeypatch.setattr(module, "TenantService", fresh_tenant)
        monkeypatch.setattr(module, "UserTenantService", fresh_user_tenant)
        return module

    yield _reload
