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
"""Regression tests for the DEFAULT_SUPERUSER_* env-var wiring in
``admin/server/auth.py``.

``admin.server.auth`` transitively imports the full api db layer
(peewee, flask, elasticsearch, ...). ``conftest.py`` installs lightweight
``sys.modules`` stubs so the real auth module can be loaded in-process,
then patches ``UserService`` / ``TenantService`` / ``UserTenantService``
with ``MagicMock`` per-test — the bootstrap functions then run against
the production source code end-to-end, without duplicating any logic in
a replica.

See infiniflow/ragflow#16876.
"""

from __future__ import annotations

import base64

import pytest


# ---------------------------------------------------------------------------
# Module-load wiring
# ---------------------------------------------------------------------------


def test_auth_module_reads_default_superuser_env_vars_at_import(reload_admin_auth, monkeypatch):
    """With no ``DEFAULT_SUPERUSER_*`` env vars set, the module must
    fall back to the documented defaults."""
    monkeypatch.delenv("DEFAULT_SUPERUSER_NICKNAME", raising=False)
    monkeypatch.delenv("DEFAULT_SUPERUSER_EMAIL", raising=False)
    monkeypatch.delenv("DEFAULT_SUPERUSER_PASSWORD", raising=False)

    module = reload_admin_auth()
    assert module.DEFAULT_SUPERUSER_NICKNAME == "admin"
    assert module.DEFAULT_SUPERUSER_EMAIL == "admin@ragflow.io"
    assert module.DEFAULT_SUPERUSER_PASSWORD == "admin"


def test_auth_module_picks_up_custom_env_values(reload_admin_auth, monkeypatch):
    """Re-importing the module after setting the env vars must surface
    the configured values as module-level constants."""
    monkeypatch.setenv("DEFAULT_SUPERUSER_NICKNAME", "ops")
    monkeypatch.setenv("DEFAULT_SUPERUSER_EMAIL", "ops@example.com")
    monkeypatch.setenv("DEFAULT_SUPERUSER_PASSWORD", "s3cret!@#")

    module = reload_admin_auth()
    assert module.DEFAULT_SUPERUSER_NICKNAME == "ops"
    assert module.DEFAULT_SUPERUSER_EMAIL == "ops@example.com"
    assert module.DEFAULT_SUPERUSER_PASSWORD == "s3cret!@#"


# ---------------------------------------------------------------------------
# init_default_admin — exercises the real production function
# ---------------------------------------------------------------------------


def test_init_default_admin_creates_row_from_env_credentials(reload_admin_auth, monkeypatch):
    """When no superuser exists, ``init_default_admin`` must call
    ``UserService.save`` with the configured ``DEFAULT_SUPERUSER_*``
    credentials. The mock asserts the production code path, not a
    replica."""
    monkeypatch.setenv("DEFAULT_SUPERUSER_NICKNAME", "ops")
    monkeypatch.setenv("DEFAULT_SUPERUSER_EMAIL", "ops@example.com")
    monkeypatch.setenv("DEFAULT_SUPERUSER_PASSWORD", "s3cret!@#")

    module = reload_admin_auth()
    user_service = module.UserService
    user_service.query.return_value = []
    user_service.save.return_value = True

    module.init_default_admin()

    user_service.save.assert_called_once()
    saved = user_service.save.call_args.kwargs
    assert saved["email"] == "ops@example.com"
    assert saved["nickname"] == "ops"
    assert saved["is_superuser"] is True
    assert base64.b64decode(saved["password"]).decode("utf-8") == "s3cret!@#"
    # id and creator are stable across the env-var wiring change.
    assert saved["creator"] == "system"
    assert saved["status"] == "1"
    assert isinstance(saved["id"], str) and saved["id"]


def test_init_default_admin_uses_defaults_when_env_unset(reload_admin_auth, monkeypatch):
    """With no ``DEFAULT_SUPERUSER_*`` env vars set, the bootstrap must
    fall back to the documented defaults."""
    monkeypatch.delenv("DEFAULT_SUPERUSER_NICKNAME", raising=False)
    monkeypatch.delenv("DEFAULT_SUPERUSER_EMAIL", raising=False)
    monkeypatch.delenv("DEFAULT_SUPERUSER_PASSWORD", raising=False)

    module = reload_admin_auth()
    user_service = module.UserService
    user_service.query.return_value = []
    user_service.save.return_value = True

    module.init_default_admin()

    saved = user_service.save.call_args.kwargs
    assert saved["email"] == "admin@ragflow.io"
    assert saved["nickname"] == "admin"
    assert base64.b64decode(saved["password"]).decode("utf-8") == "admin"


def test_init_default_admin_raises_when_no_active_admin(reload_admin_auth):
    """If superuser rows exist but none are active, ``init_default_admin``
    must raise rather than silently re-bootstrapping. This is the existing
    contract; the env-var change must not weaken it."""

    class _InactiveUser:
        is_active = "0"

    module = reload_admin_auth()
    user_service = module.UserService
    user_service.query.return_value = [_InactiveUser()]

    from api.common.exceptions import AdminException

    with pytest.raises(AdminException):
        module.init_default_admin()

    # No save call: an inactive-admin deployment should not be silently
    # recreated from env vars.
    user_service.save.assert_not_called()


def test_init_default_admin_backfills_tenant_for_existing_admin(reload_admin_auth, monkeypatch):
    """When an admin row already exists with the configured email,
    ``init_default_admin`` must backfill the tenant if missing — this is
    the path operators hit after upgrading with a custom
    ``DEFAULT_SUPERUSER_EMAIL`` set."""
    monkeypatch.setenv("DEFAULT_SUPERUSER_EMAIL", "ops@example.com")
    monkeypatch.setenv("DEFAULT_SUPERUSER_PASSWORD", "secret")
    monkeypatch.setenv("DEFAULT_SUPERUSER_NICKNAME", "ops")

    module = reload_admin_auth()
    user_service = module.UserService
    tenant_service = module.TenantService
    user_tenant_service = module.UserTenantService

    class _ExistingAdmin:
        is_active = "1"
        email = "ops@example.com"

        def to_dict(self):
            return {
                "id": "admin-id",
                "email": self.email,
                "nickname": "ops",
                "is_superuser": True,
                "status": "1",
            }

    user_service.query.return_value = [_ExistingAdmin()]
    tenant_service.get_by_id.return_value = (False, None)  # No tenant yet

    module.init_default_admin()

    user_service.save.assert_not_called()
    tenant_service.get_by_id.assert_called_once_with("admin-id")
    tenant_service.insert.assert_called_once()
    user_tenant_service.insert.assert_called_once()


# ---------------------------------------------------------------------------
# check_admin — exercises the real production function
# ---------------------------------------------------------------------------


def test_check_admin_creates_admin_from_env_on_first_login(reload_admin_auth, monkeypatch):
    """On first login with an unknown username, ``check_admin`` must
    bootstrap a new admin row using the configured
    ``DEFAULT_SUPERUSER_*`` env vars. This is the path the issue
    reports — the email and password must come from the env, not from
    a hardcoded literal."""
    monkeypatch.setenv("DEFAULT_SUPERUSER_NICKNAME", "ops")
    monkeypatch.setenv("DEFAULT_SUPERUSER_EMAIL", "ops@example.com")
    monkeypatch.setenv("DEFAULT_SUPERUSER_PASSWORD", "s3cret!@#")

    module = reload_admin_auth()
    user_service = module.UserService
    user_service.query.return_value = []  # No existing user with this email
    user_service.save.return_value = True

    # Pass the configured password to query_user so a regression in the
    # password-propagation path is caught — using ``True`` as a blanket
    # return value would mask any drift in what ``check_admin`` actually
    # hands to the credential lookup.
    user_service.query_user.return_value = True
    result = module.check_admin("ops@example.com", "s3cret!@#")
    user_service.query_user.assert_called_with("ops@example.com", "s3cret!@#")

    user_service.save.assert_called_once()
    saved = user_service.save.call_args.kwargs
    assert saved["email"] == "ops@example.com"
    assert saved["nickname"] == "ops"
    assert base64.b64decode(saved["password"]).decode("utf-8") == "s3cret!@#"
    assert result is True


def test_check_admin_uses_defaults_when_env_unset(reload_admin_auth, monkeypatch):
    """With no env vars set, the first-login bootstrap must produce the
    documented defaults."""
    monkeypatch.delenv("DEFAULT_SUPERUSER_NICKNAME", raising=False)
    monkeypatch.delenv("DEFAULT_SUPERUSER_EMAIL", raising=False)
    monkeypatch.delenv("DEFAULT_SUPERUSER_PASSWORD", raising=False)

    module = reload_admin_auth()
    user_service = module.UserService
    user_service.query.return_value = []
    user_service.save.return_value = True
    user_service.query_user.return_value = True

    module.check_admin("admin@ragflow.io", "admin")

    saved = user_service.save.call_args.kwargs
    assert saved["email"] == "admin@ragflow.io"
    assert saved["nickname"] == "admin"
    assert base64.b64decode(saved["password"]).decode("utf-8") == "admin"


@pytest.mark.parametrize(
    "env_value",
    [
        "ops@example.com",
        "me@corp.io",
        "UPPER@example.com",
    ],
)
def test_check_admin_passes_env_email_through_unchanged(reload_admin_auth, monkeypatch, env_value):
    """The configured ``DEFAULT_SUPERUSER_EMAIL`` must reach
    ``UserService.save`` exactly as set — no trimming, lowercasing, or
    other transformation."""
    monkeypatch.setenv("DEFAULT_SUPERUSER_EMAIL", env_value)
    monkeypatch.delenv("DEFAULT_SUPERUSER_NICKNAME", raising=False)
    monkeypatch.delenv("DEFAULT_SUPERUSER_PASSWORD", raising=False)

    module = reload_admin_auth()
    user_service = module.UserService
    user_service.query.return_value = []
    user_service.save.return_value = True
    user_service.query_user.return_value = True

    module.check_admin(env_value, env_value)

    saved = user_service.save.call_args.kwargs
    assert saved["email"] == env_value


def test_check_admin_skips_bootstrap_when_user_exists(reload_admin_auth):
    """If the user already exists in the database, ``check_admin`` must
    not call ``UserService.save`` — the bootstrap path is only for the
    very first login."""
    module = reload_admin_auth()
    user_service = module.UserService
    user_service.query.return_value = ["existing-user"]
    user_service.query_user.return_value = True

    result = module.check_admin("admin@ragflow.io", "admin")

    user_service.save.assert_not_called()
    assert result is True
