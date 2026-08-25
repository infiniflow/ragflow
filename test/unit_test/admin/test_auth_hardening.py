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

1. ``check_admin`` (Basic-Auth verification for GET /api/v1/admin/auth) must
   never create accounts. Previously a single request with any unregistered
   username silently created a superuser ``admin@ragflow.io`` with a fixed
   password, defeating operators who had removed the default account.
2. ``init_default_admin`` must not hardcode ``admin``: the bootstrap password
   comes from ``ADMIN_DEFAULT_PASSWORD`` (or ``DEFAULT_SUPERUSER_PASSWORD``,
   shared with ``api/db/init_data.py``) or is randomly generated and printed
   once to the log.
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


def _fake_jsonify(payload=None, **kwargs):
    """Minimal stand-in for flask.jsonify: returns its payload unchanged."""
    return payload if payload is not None else kwargs


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


class TestCheckAdminNeverCreatesAccounts:
    def test_unregistered_username_is_rejected_without_creation(self):
        with mock.patch.object(auth, "UserService") as user_service:
            user_service.query.return_value = []
            user_service.query_user.return_value = None

            assert auth.check_admin("probe@attacker.example", "whatever") is False

        user_service.query.assert_called_once_with(email="probe@attacker.example")
        user_service.save.assert_not_called()
        user_service.query_user.assert_not_called()

    def test_wrong_password_is_rejected_without_creation(self):
        registered = mock.MagicMock()
        with mock.patch.object(auth, "UserService") as user_service:
            user_service.query.return_value = [registered]
            user_service.query_user.return_value = None

            assert auth.check_admin("admin@ragflow.io", "wrong-password") is False

        user_service.query_user.assert_called_once_with("admin@ragflow.io", "wrong-password")
        user_service.save.assert_not_called()

    def test_valid_credentials_pass(self):
        registered = mock.MagicMock()
        with mock.patch.object(auth, "UserService") as user_service:
            user_service.query.return_value = [registered]
            user_service.query_user.return_value = registered

            assert auth.check_admin("admin@ragflow.io", "YWRtaW4=") is True

        user_service.save.assert_not_called()


class TestInitDefaultAdminPassword:
    def test_random_password_when_env_unset_and_logged_once(self, caplog):
        with caplog.at_level(logging.WARNING), mock.patch.object(auth, "UserService") as user_service, mock.patch.object(auth, "add_tenant_for_admin") as add_tenant:
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
        # The random password is revealed exactly once, in the startup log.
        assert stored_password in caplog.text
        assert caplog.text.count(stored_password) == 1
        assert "change it immediately" in caplog.text
        add_tenant.assert_called_once()

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
            assert auth._reserve_login_attempt("1.2.3.4") == 0
        assert auth._reserve_login_attempt("1.2.3.4") > 0

    def test_success_resets_failures(self):
        for _ in range(auth.ADMIN_LOGIN_MAX_FAILURES - 1):
            assert auth._reserve_login_attempt("1.2.3.4") == 0
        auth._reset_login_failures("1.2.3.4")
        assert auth._reserve_login_attempt("1.2.3.4") == 0

    def test_release_undo_reserves_a_slot(self):
        for _ in range(auth.ADMIN_LOGIN_MAX_FAILURES):
            auth._reserve_login_attempt("1.2.3.4")
        auth._release_login_attempt("1.2.3.4")
        # The released attempt no longer counts, and a lockout set only by
        # reservations is lifted with it.
        assert auth._reserve_login_attempt("1.2.3.4") == 0

    def test_concurrent_reservations_cannot_exceed_max_admissions(self):
        """Atomic admission: a simultaneous burst gets at most MAX verifications."""
        workers = 32
        barrier = threading.Barrier(workers)
        admitted = []
        blocked = []
        lock = threading.Lock()

        def attempt():
            barrier.wait()
            was_admitted = auth._reserve_login_attempt("10.0.0.9") == 0
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
        assert auth._reserve_login_attempt("1.2.3.4") == 0

    def test_login_verify_returns_429_when_throttled(self):
        for _ in range(auth.ADMIN_LOGIN_MAX_FAILURES):
            auth._reserve_login_attempt("1.2.3.4")

        fake_request = mock.MagicMock()
        fake_request.headers = {}
        fake_request.authorization.parameters = {"username": "admin@ragflow.io", "password": "anything"}
        with (
            mock.patch.object(auth, "_client_key", return_value="1.2.3.4"),
            mock.patch.object(auth, "request", fake_request),
            mock.patch.object(auth, "jsonify", _fake_jsonify),
            mock.patch.object(auth, "check_admin") as check_admin,
        ):

            @auth.login_verify
            def endpoint():
                return "ok"

            payload, status = endpoint()

        assert status == 200
        assert payload["code"] == 429
        check_admin.assert_not_called()

    def test_login_verify_records_failure_on_denied_credentials(self):
        fake_request = mock.MagicMock()
        fake_request.headers = {}
        fake_request.authorization.parameters = {"username": "admin@ragflow.io", "password": "bad"}
        with (
            mock.patch.object(auth, "_client_key", return_value="1.2.3.4"),
            mock.patch.object(auth, "request", fake_request),
            mock.patch.object(auth, "jsonify", _fake_jsonify),
            mock.patch.object(auth, "check_admin", return_value=False),
        ):

            @auth.login_verify
            def endpoint():
                return "ok"

            payload, status = endpoint()

        assert status == 200
        assert payload["code"] == 500
        # The reserved failure slot is visible to the counter but does not
        # block the very next attempt.
        assert auth._login_failures.get("1.2.3.4") is not None
        assert auth._reserve_login_attempt("1.2.3.4") == 0


def test_throttle_client_key_uses_last_forwarded_hop():
    """Only the rightmost XFF entry (appended by our nginx) may key the limiter."""
    from flask import Flask

    from admin.server.auth import _client_key

    app = Flask(__name__)
    with app.test_request_context(
        "/", headers={"X-Forwarded-For": "1.2.3.4, 5.6.7.8"}, environ_base={"REMOTE_ADDR": "9.9.9.9"}
    ):
        assert _client_key() == "5.6.7.8"
    with app.test_request_context("/", environ_base={"REMOTE_ADDR": "9.9.9.9"}):
        assert _client_key() == "9.9.9.9"


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
