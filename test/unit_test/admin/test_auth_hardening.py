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
"""
Unit tests for the admin auth bootstrap hardening in ``admin/server/auth.py``.

Covers the security fixes for the "anonymous probe resurrects the superuser"
chain:

1. The Basic-Auth probe endpoint (GET /api/v1/admin/auth, ``login_verify`` /
   ``check_admin``) — the historic silent-superuser-creation path — is dead
   code (the admin UI uses the JSON login, ragflow-cli uses POST
   /admin/login) and has been removed outright instead of hardened.
2. ``init_default_admin`` must not hardcode ``admin``: the bootstrap password
   comes from ``ADMIN_DEFAULT_PASSWORD`` (or ``DEFAULT_SUPERUSER_PASSWORD``,
   shared with ``api/db/init_data.py``) or is randomly generated and written
   once to a 0600 bootstrap file, never the log.
3. Decrypt failures are classified: client payload faults answer with the
   generic credential mismatch, while server-side key faults propagate
   instead of masquerading as wrong-password verdicts.

The per-address login throttle that earlier revisions carried was removed
on review: its counters were process-local, so under a multi-worker
deployment the limit would silently multiply and the control would become
an unreliable mix of false protection and unpredictable 429s for real
users. The admin server deliberately avoids shared-state dependencies
(filesystem sessions, no Redis), so the honest options were a shared-store
implementation or no throttle; rag/utils/redis_conn.py already ships an
atomic token-bucket Lua script if a Redis-backed version is ever wanted.
"""

import base64
import logging
from unittest import mock

# The admin conftest (test/unit_test/admin/conftest.py) already prepends
# admin/server to sys.path. The full dependency graph must be importable;
# an import failure fails test setup rather than silently stubbing the
# modules under test.
import auth
import pytest

assert auth.__file__.endswith("admin/server/auth.py"), f"unexpected auth module resolved: {auth.__file__}"


def _no_bootstrap_password_env(monkeypatch):
    monkeypatch.delenv("ADMIN_DEFAULT_PASSWORD", raising=False)
    monkeypatch.delenv("DEFAULT_SUPERUSER_PASSWORD", raising=False)


class TestInitDefaultAdminPassword:
    def test_random_password_when_env_unset_is_not_logged(self, tmp_path, caplog):
        with (
            caplog.at_level(logging.WARNING),
            mock.patch.object(auth, "get_project_base_directory", return_value=str(tmp_path)),
            mock.patch.object(auth, "UserService") as user_service,
            mock.patch.object(auth, "add_tenant_for_admin") as add_tenant,
        ):
            user_service.query.return_value = []
            user_service.save.return_value = object()

            auth.init_default_admin()

        user_service.save.assert_called_once()
        saved = user_service.save.call_args.kwargs
        assert saved["email"] == "admin@ragflow.io"
        assert saved["is_superuser"] is True
        # Stored value is base64(<plain password>) and must not be the old default.
        stored_password = base64.b64decode(saved["password"]).decode("utf-8")
        assert stored_password != "admin"
        assert len(stored_password) >= 18
        # The random password goes to the 0600 bootstrap file and never the log.
        assert stored_password not in caplog.text
        assert "admin_bootstrap_password.txt" in caplog.text
        assert "0600" in caplog.text
        add_tenant.assert_called_once()

    def test_random_password_written_to_0600_file_matching_the_saved_hash(self, tmp_path, caplog):
        with (
            caplog.at_level(logging.WARNING),
            mock.patch.object(auth, "get_project_base_directory", return_value=str(tmp_path)),
            mock.patch.object(auth, "UserService") as user_service,
            mock.patch.object(auth, "add_tenant_for_admin"),
        ):
            user_service.query.return_value = []
            user_service.save.return_value = object()

            auth.init_default_admin()

        saved = user_service.save.call_args.kwargs
        stored_password = base64.b64decode(saved["password"]).decode("utf-8")

        password_file = tmp_path / "logs" / "admin_bootstrap_password.txt"
        assert password_file.exists()
        assert password_file.read_text(encoding="utf-8").strip() == stored_password
        assert (password_file.stat().st_mode & 0o777) == 0o600
        # The log announces the file, not the credential itself.
        assert stored_password not in caplog.text
        assert str(password_file) in caplog.text

    def test_unwritable_bootstrap_dir_fails_closed_without_leaking(self, tmp_path, caplog):
        locked_dir = tmp_path / "logs"
        locked_dir.mkdir()
        locked_dir.chmod(0o500)  # read+execute, no write
        try:
            with (
                caplog.at_level(logging.ERROR),
                mock.patch.object(auth, "get_project_base_directory", return_value=str(tmp_path)),
                mock.patch.object(auth, "UserService") as user_service,
                mock.patch.object(auth, "add_tenant_for_admin"),
            ):
                user_service.query.return_value = []
                user_service.save.return_value = object()

                auth.init_default_admin()

            saved = user_service.save.call_args.kwargs
            stored_password = base64.b64decode(saved["password"]).decode("utf-8")
            # Fail closed: no file, and the password never reaches the log.
            assert not (locked_dir / "admin_bootstrap_password.txt").exists()
            assert stored_password not in caplog.text
            assert "NOT recoverable" in caplog.text
        finally:
            locked_dir.chmod(0o700)

    def test_random_passwords_differ_between_bootstraps(self):
        first, first_env = auth._resolve_bootstrap_admin_password()
        second, second_env = auth._resolve_bootstrap_admin_password()
        assert first_env is None and second_env is None
        assert first != second

    def test_admin_default_password_env_wins_and_is_not_logged(self, monkeypatch, caplog):
        monkeypatch.setenv("ADMIN_DEFAULT_PASSWORD", "s3cr3t-bootstrap")
        with caplog.at_level(logging.INFO), mock.patch.object(auth, "UserService") as user_service, mock.patch.object(auth, "add_tenant_for_admin"):
            user_service.query.return_value = []
            user_service.save.return_value = object()

            auth.init_default_admin()

        saved = user_service.save.call_args.kwargs
        assert base64.b64decode(saved["password"]).decode("utf-8") == "s3cr3t-bootstrap"
        assert "s3cr3t-bootstrap" not in caplog.text

    def test_falls_back_to_default_superuser_password_env(self, monkeypatch):
        monkeypatch.setenv("DEFAULT_SUPERUSER_PASSWORD", "web-side-bootstrap")
        with mock.patch.object(auth, "UserService") as user_service, mock.patch.object(auth, "add_tenant_for_admin"):
            user_service.query.return_value = []
            user_service.save.return_value = object()

            auth.init_default_admin()

        saved = user_service.save.call_args.kwargs
        assert base64.b64decode(saved["password"]).decode("utf-8") == "web-side-bootstrap"

    def test_existing_superuser_skips_creation(self):
        existing = mock.MagicMock(is_active="1", email="admin@ragflow.io", id="x" * 32)
        existing.to_dict.return_value = {"id": "x" * 32, "email": "admin@ragflow.io", "nickname": "admin"}
        with mock.patch.object(auth, "UserService") as user_service, mock.patch.object(auth, "TenantService") as tenant_service:
            user_service.query.return_value = [existing]
            tenant_service.get_by_id.return_value = (True, mock.MagicMock())

            auth.init_default_admin()

        user_service.save.assert_not_called()


class TestDecryptFailureClassification:
    def test_client_payload_fault_is_a_generic_credential_mismatch(self):
        from api.utils.crypt import CryptPayloadError

        registered = mock.MagicMock()
        with (
            mock.patch.object(auth, "UserService") as user_service,
            mock.patch.object(auth, "decrypt", side_effect=CryptPayloadError("bad payload")),
        ):
            user_service.query.return_value = [registered]

            with pytest.raises(auth.AdminException, match="do not match"):
                auth.login_admin("admin@ragflow.io", "not-valid-base64")

        # A payload the client could not encode correctly must not surface
        # as a server error (which would hint at key-management problems).
        user_service.query_user.assert_not_called()

    def test_server_side_decrypt_fault_propagates(self):
        registered = mock.MagicMock()
        with (
            mock.patch.object(auth, "UserService") as user_service,
            mock.patch.object(auth, "decrypt", side_effect=RuntimeError("private key file missing")),
        ):
            user_service.query.return_value = [registered]

            # Key-management faults are server errors, not credential
            # verdicts: they propagate instead of answering "wrong password".
            with pytest.raises(RuntimeError, match="private key file missing"):
                auth.login_admin("admin@ragflow.io", "anything")


class TestDecryptAcceptsLineWrappedBase64:
    def test_round_trip_with_embedded_newlines(self):
        """b64decode(validate=True) rejects embedded newlines; decrypt must
        strip internal whitespace first or line-wrapped base64 payloads
        (test fixtures, PEM-style senders) are treated as bad credentials."""
        import base64 as b64

        from api.utils.crypt import crypt, decrypt

        blob = crypt("123")
        wrapped = "\n".join(blob[i : i + 16] for i in range(0, len(blob), 16))

        assert decrypt(wrapped) == b64.b64encode(b"123").decode()
        assert decrypt(blob) == b64.b64encode(b"123").decode()
