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
3. Failed logins are throttled per client key (in-process counter).
"""

import base64
import logging
import threading
import time
from unittest import mock

# The admin conftest (test/unit_test/admin/conftest.py) already prepends
# admin/server to sys.path. The full dependency graph must be importable;
# an import failure fails test setup rather than silently stubbing the
# modules under test.
import auth
import pytest

assert auth.__file__.endswith("admin/server/auth.py"), f"unexpected auth module resolved: {auth.__file__}"


def _reset_login_state():
    auth._login_failures.clear()
    auth._login_block_until.clear()


@pytest.fixture(autouse=True)
def _clean_login_state():
    _reset_login_state()
    yield
    _reset_login_state()


@pytest.fixture(autouse=True)
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


class TestLoginThrottling:
    def test_failures_lock_out_client_key(self):
        # Each admitted attempt reserves a failure slot atomically; the
        # attempt that reaches ADMIN_LOGIN_MAX_FAILURES is still admitted
        # (it is the one that trips the lockout) and everything after it
        # is blocked.
        for _ in range(auth.ADMIN_LOGIN_MAX_FAILURES):
            remaining, marker = auth._reserve_login_attempt("1.2.3.4")
            assert remaining == 0
            assert marker is not None
        remaining, marker = auth._reserve_login_attempt("1.2.3.4")
        assert remaining > 0
        assert marker is None

    def test_success_resets_failures(self):
        for _ in range(auth.ADMIN_LOGIN_MAX_FAILURES - 1):
            assert auth._reserve_login_attempt("1.2.3.4")[0] == 0
        auth._reset_login_failures("1.2.3.4")
        assert auth._reserve_login_attempt("1.2.3.4")[0] == 0

    def test_release_keeps_genuine_failures_and_rearms_quickly(self):
        """Releasing one reservation must not wipe the genuine failures that
        armed a lockout, and must not permanently open the throttle either."""
        marker = None
        for _ in range(auth.ADMIN_LOGIN_MAX_FAILURES):
            _, marker = auth._reserve_login_attempt("1.2.3.4")
        assert auth._login_block_until.get("1.2.3.4") is not None

        # Undo exactly the last reservation: the genuine failures stay
        # recorded, the lockout armed by that reservation is lifted...
        auth._release_login_attempt("1.2.3.4", marker)
        assert len(auth._login_failures["1.2.3.4"]) == auth.ADMIN_LOGIN_MAX_FAILURES - 1

        # ...one subsequent attempt is admitted (and re-arms the lockout,
        # since the window still holds the earlier failures)...
        assert auth._reserve_login_attempt("1.2.3.4")[0] == 0
        # ...and the next attempt is blocked again.
        assert auth._reserve_login_attempt("1.2.3.4")[0] > 0

    def test_concurrent_reservations_cannot_exceed_max_admissions(self):
        """Atomic admission: a simultaneous burst gets at most MAX verifications."""
        workers = 32
        barrier = threading.Barrier(workers)
        admitted = []
        blocked = []
        lock = threading.Lock()

        def attempt():
            barrier.wait()
            was_admitted = auth._reserve_login_attempt("10.0.0.9")[0] == 0
            with lock:
                (admitted if was_admitted else blocked).append(1)

        threads = [threading.Thread(target=attempt) for _ in range(workers)]
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join()

        assert len(admitted) == auth.ADMIN_LOGIN_MAX_FAILURES
        assert len(blocked) == workers - auth.ADMIN_LOGIN_MAX_FAILURES

    def test_login_admin_locks_out_after_repeated_failures(self):
        registered = mock.MagicMock()
        with mock.patch.object(auth, "_client_key", return_value="1.2.3.4"), mock.patch.object(auth, "UserService") as user_service, mock.patch.object(auth, "decrypt", return_value="wrong"):
            user_service.query.return_value = [registered]
            user_service.query_user.return_value = None

            for _ in range(auth.ADMIN_LOGIN_MAX_FAILURES):
                with pytest.raises(auth.AdminException, match="do not match"):
                    auth.login_admin("admin@ragflow.io", "encrypted-wrong")

            with pytest.raises(auth.AdminException, match="Too many failed login attempts") as exc_info:
                auth.login_admin("admin@ragflow.io", "encrypted-wrong")

        assert exc_info.value.code == 429

    def test_login_admin_success_resets_counter(self):
        user = mock.MagicMock(is_superuser=True, is_active="1")
        user.to_json.return_value = {"email": "admin@ragflow.io"}
        response_marker = object()
        with (
            mock.patch.object(auth, "_client_key", return_value="1.2.3.4"),
            mock.patch.object(auth, "UserService") as user_service,
            mock.patch.object(auth, "decrypt", return_value="YWRtaW4="),
            mock.patch.object(auth, "login_user"),
            mock.patch.object(auth, "sync_construct_response", return_value=response_marker),
        ):
            user_service.query.return_value = [user]
            user_service.query_user.return_value = None  # first attempt fails
            with pytest.raises(auth.AdminException):
                auth.login_admin("admin@ragflow.io", "encrypted")

            user_service.query_user.return_value = user  # then correct password
            assert auth.login_admin("admin@ragflow.io", "encrypted") is response_marker

        # A fresh reservation is admitted immediately after the success reset.
        assert auth._reserve_login_attempt("1.2.3.4")[0] == 0


class TestConcurrentLoginBurst:
    def test_concurrent_login_admin_burst_limits_credential_verifications(self):
        """A simultaneous wrong-password burst from one client may run at most
        ADMIN_LOGIN_MAX_FAILURES credential verifications; the rest get 429."""
        verification_calls = []
        call_lock = threading.Lock()

        def slow_query_user(email, password):
            with call_lock:
                verification_calls.append(email)
            time.sleep(0.02)  # keep the race window open

        registered = mock.MagicMock()
        workers = 16
        barrier = threading.Barrier(workers)
        outcomes = []
        outcome_lock = threading.Lock()

        def worker():
            barrier.wait()
            try:
                auth.login_admin("admin@ragflow.io", "encrypted-wrong")
            except auth.AdminException as exc:
                with outcome_lock:
                    outcomes.append(exc.code)

        with (
            mock.patch.object(auth, "_client_key", return_value="10.0.0.8"),
            mock.patch.object(auth, "UserService") as user_service,
            mock.patch.object(auth, "decrypt", return_value="wrong"),
        ):
            user_service.query.return_value = [registered]
            user_service.query_user.side_effect = slow_query_user
            threads = [threading.Thread(target=worker) for _ in range(workers)]
            for thread in threads:
                thread.start()
            for thread in threads:
                thread.join()

        assert len(verification_calls) == auth.ADMIN_LOGIN_MAX_FAILURES
        # Exactly ADMIN_LOGIN_MAX_FAILURES verifications ran (all rejected as
        # wrong password, AdminException code 400 by default); the remainder
        # were rejected by the throttle with 429 without reaching the
        # credential check.
        assert outcomes.count(429) == workers - auth.ADMIN_LOGIN_MAX_FAILURES
        assert len(verification_calls) == auth.ADMIN_LOGIN_MAX_FAILURES


class TestDecryptFailuresCountAsCredentialRejections:
    def test_undecryptable_password_keeps_the_reserved_slot(self):
        from api.utils.crypt import CryptPayloadError

        registered = mock.MagicMock()
        with (
            mock.patch.object(auth, "_client_key", return_value="10.0.0.7"),
            mock.patch.object(auth, "UserService") as user_service,
            mock.patch.object(auth, "decrypt", side_effect=CryptPayloadError("bad payload")),
        ):
            user_service.query.return_value = [registered]
            for _ in range(auth.ADMIN_LOGIN_MAX_FAILURES):
                with pytest.raises(auth.AdminException, match="do not match"):
                    auth.login_admin("admin@ragflow.io", "not-valid-base64")
            with pytest.raises(auth.AdminException, match="Too many failed login attempts"):
                auth.login_admin("admin@ragflow.io", "not-valid-base64")

    def test_server_side_decrypt_fault_releases_the_reserved_slot(self):
        registered = mock.MagicMock()
        with (
            mock.patch.object(auth, "_client_key", return_value="10.0.0.6"),
            mock.patch.object(auth, "UserService") as user_service,
            mock.patch.object(auth, "decrypt", side_effect=RuntimeError("private key file missing")),
        ):
            user_service.query.return_value = [registered]
            for _ in range(auth.ADMIN_LOGIN_MAX_FAILURES + 3):
                with pytest.raises(RuntimeError, match="private key file missing"):
                    auth.login_admin("admin@ragflow.io", "anything")
            # Server faults never consume the budget: no lockout is armed.
            assert auth._login_block_until.get("10.0.0.6") is None
            assert auth._login_failures.get("10.0.0.6") is None
