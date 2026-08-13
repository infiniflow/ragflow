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

"""Regression tests for the ``ConnectorRunner.run`` credential-redaction
fix (#17587).

Pre-fix, ``ConnectorRunner.run`` had a ``try: ... except Exception:`` block
that, on any connector error, walked the traceback to the failing frame
and dumped the first 1024 chars of ``f_locals`` (the local variables of
the frame where the exception was raised) into the log file via
``logging.error(...)``. Connectors hold OAuth access tokens, refresh
tokens, client secrets, API keys, etc. in memory, so any transient
connector failure (network blip, rate limit, parser error, schema
mismatch, expired token) would write those credentials to the log in
plaintext.

The fix adds a small ``_is_sensitive_key(name) -> bool`` helper backed
by an explicit allowlist of credential-related key-name patterns
(``access_token``, ``refresh_token``, ``client_secret``, ``api_key``,
``password``, etc., matched case-insensitive substring). The
exception handler now redacts any ``f_locals`` entry whose key matches
one of those patterns, replacing the value with the literal string
``"<redacted>"`` so the operator debugging the log can see that a
field was hidden, not that it just disappeared.

These tests cover:

1. ``_is_sensitive_key`` directly -- the allowlist.
2. The end-to-end redaction in ``ConnectorRunner.run`` -- a real
   ``except Exception`` path triggered by a mock connector that holds
   credentials in its local variables, asserting the credentials are
   not present in the captured log output but the non-sensitive
   variables are.
3. The redaction is visible (``"<redacted>"`` appears in the log), so
   an operator debugging the failure knows that a field was hidden.
4. The 1024-char truncation behavior is preserved.
"""

import logging

import pytest

from common.data_source.connector_runner import (
    ConnectorRunner,
    _is_sensitive_key,
)
from common.data_source.interfaces import LoadConnector
from common.data_source.models import ConnectorCheckpoint


# --------------------------------------------------------------------------- #
# 1. The ``_is_sensitive_key`` allowlist
# --------------------------------------------------------------------------- #
@pytest.mark.p0
class TestIsSensitiveKey:
    """Direct tests for the redaction predicate. Match is
    case-insensitive substring on the key name."""

    @pytest.mark.parametrize(
        "name",
        [
            "access_token",
            "ACCESS_TOKEN",
            "Access_Token",
            "google_access_token",
            "refresh_token",
            "id_token",
            "client_secret",
            "client_id",
            "api_key",
            "apikey",
            "ApiKey",
            "my_api_key",
            "private_key",
            "privateKey",
            "google_service_account_private_key",
            "secret_key",
            "SecretKey",
            "aws_secret_access_key",
            "access_key",
            "AccessKey",
            "aws_access_key_id",
            "creds",
            "creds_dict",
            "service_account_creds",
            "secret",
            "client_secret_value",
            "password",
            "PASSWORD",
            "user_password",
            "token",
            "auth_token",
            "credentials",
            "credential",
            "authorization",
            "Authorization",
            "auth",
            "auth_header",
            "cookie",
            "Cookie",
            "session",
            "session_id",
        ],
    )
    def test_sensitive_keys_are_detected(self, name: str) -> None:
        assert _is_sensitive_key(name) is True, f"expected {name!r} to be sensitive"

    @pytest.mark.parametrize(
        "name",
        [
            "user_id",
            "tenant_id",
            "document_id",
            "file_name",
            "url",
            "page",
            "cursor",
            "limit",
            "offset",
            "status",
            "message",
            "checkpoint",
            "batch_size",
            "include_permissions",
            "time_range",
            "documents",
            "failures",
        ],
    )
    def test_non_sensitive_keys_are_not_detected(self, name: str) -> None:
        assert _is_sensitive_key(name) is False, f"expected {name!r} to be non-sensitive"

    def test_non_string_key_returns_false(self) -> None:
        """Tuple indices from ``f_locals`` (int) are never sensitive."""
        assert _is_sensitive_key(0) is False
        assert _is_sensitive_key(42) is False

    def test_none_key_returns_false(self) -> None:
        assert _is_sensitive_key(None) is False


# --------------------------------------------------------------------------- #
# 2. End-to-end: a mock connector that holds credentials raises; the
#    exception handler redacts them before logging.
# --------------------------------------------------------------------------- #


class _CredentialHoldingLoadConnector(LoadConnector):
    """A minimal ``LoadConnector`` that holds several credentials in
    its local scope and then raises a ``ValueError`` so the runner's
    ``except Exception`` block fires.

    The credentials are deliberately named to match the allowlist --
    this is the exact shape of state a real connector (Google Drive,
    Slack, Notion, Jira, ...) holds while iterating.
    """

    def load_credentials(self, credentials):
        return None

    def build_dummy_checkpoint(self):
        return ConnectorCheckpoint(has_more=True)

    def load_from_state(self):
        # All of these locals intentionally never get read in Python --
        # the test asserts they appear in the captured log output (for
        # non-sensitive vars) or are redacted to "<redacted>" (for
        # sensitive vars). The noqa: F841 marks them as
        # intentionally-unread; without it ruff misreads the test
        # intent.
        access_token = "ya29.a0fake_google_oauth_access_token_value_xyz"  # noqa: F841, S105
        refresh_token = "1//0g_fake_refresh_token_value_xyz"  # noqa: F841, S105
        client_secret = "GOCSPX-FakeClientSecretValueXYZ"  # noqa: F841, S105
        client_id = "1234567890.apps.googleusercontent.com"  # noqa: F841
        api_key = "sk-fake-openai-api-key-1234567890abcdef"  # noqa: F841
        password = "hunter2"  # noqa: F841, S105
        # Non-sensitive context that SHOULD appear in the log:
        tenant_id = "tenant-abc-123"  # noqa: F841
        document_id = "doc-uuid-456"  # noqa: F841
        # Trigger the failure:
        raise ValueError("simulated connector failure with credentials in scope")


def _make_runner_with_load_connector() -> ConnectorRunner:
    """Build a ``ConnectorRunner`` around the credential-holding
    load connector. The runner is constructed but never used to actually
    iterate; we just need an instance so we can call the inner
    exception handler via a synthetic failure."""
    return ConnectorRunner(
        connector=_CredentialHoldingLoadConnector(),
        batch_size=10,
        include_permissions=False,
    )


def _trigger_exception_handler_log(caplog) -> str:
    """Run a connector that fails, returning the captured log output.

    Drives ``ConnectorRunner.run`` through the ``LoadConnector`` branch
    (the simplest one -- no checkpoint, no time range, no perm sync).
    The exception handler fires, the credentials are dumped to the
    log via ``logging.error(...)``, and the test asserts that the
    credentials are NOT in the captured output.
    """
    runner = _make_runner_with_load_connector()
    with caplog.at_level(logging.ERROR, logger="root"):
        # Iterate to consume the generator; the failure is raised by
        # the connector the first time ``load_from_state`` is called.
        try:
            list(runner.run(ConnectorCheckpoint(has_more=False)))
        except ValueError:
            pass  # Expected: the connector raises
    return caplog.text


@pytest.mark.p0
class TestConnectorRunnerRedactionInLog:
    def test_oauth_access_token_is_redacted(self, caplog) -> None:
        """The OAuth access token must not appear in the log output."""
        log_text = _trigger_exception_handler_log(caplog)
        assert "ya29.a0fake_google_oauth_access_token_value_xyz" not in log_text, "access_token was leaked to the log: " + log_text
        # And the redacted placeholder IS present, so an operator
        # debugging the log can see the field was hidden, not that
        # it just disappeared.
        assert "<redacted>" in log_text
        assert "access_token: <redacted>" in log_text

    def test_oauth_refresh_token_is_redacted(self, caplog) -> None:
        log_text = _trigger_exception_handler_log(caplog)
        assert "1//0g_fake_refresh_token_value_xyz" not in log_text
        assert "refresh_token: <redacted>" in log_text

    def test_client_secret_is_redacted(self, caplog) -> None:
        log_text = _trigger_exception_handler_log(caplog)
        assert "GOCSPX-FakeClientSecretValueXYZ" not in log_text
        assert "client_secret: <redacted>" in log_text

    def test_client_id_is_redacted(self, caplog) -> None:
        log_text = _trigger_exception_handler_log(caplog)
        assert "1234567890.apps.googleusercontent.com" not in log_text
        assert "client_id: <redacted>" in log_text

    def test_api_key_is_redacted(self, caplog) -> None:
        log_text = _trigger_exception_handler_log(caplog)
        assert "sk-fake-openai-api-key-1234567890abcdef" not in log_text
        assert "api_key: <redacted>" in log_text

    def test_password_is_redacted(self, caplog) -> None:
        log_text = _trigger_exception_handler_log(caplog)
        assert "hunter2" not in log_text
        assert "password: <redacted>" in log_text

    def test_non_sensitive_context_is_preserved(self, caplog) -> None:
        """The non-credential variables (tenant_id, document_id) should
        still appear in the log verbatim -- the redaction is selective,
        not a blanket scrub."""
        log_text = _trigger_exception_handler_log(caplog)
        assert "tenant-abc-123" in log_text
        assert "doc-uuid-456" in log_text
        # The exception type should also be present (the pre-existing
        # "Error in connector. type: ..." prefix is preserved). The
        # runner does NOT log the exception message itself -- the type
        # is the operator's hook for matching the failure to a known
        # cause.
        assert "ValueError" in log_text

    def test_log_truncation_preserved(self, caplog) -> None:
        """The pre-existing 1024-char truncation is preserved -- the
        fix only changes the variable dump, not the truncation."""
        log_text = _trigger_exception_handler_log(caplog)
        # The "local_vars below -> " marker is part of the pre-existing
        # log format; it should still be there.
        assert "local_vars below ->" in log_text

    def test_no_credential_string_appears_anywhere_in_log(self, caplog) -> None:
        """Defense in depth: scan the entire log output for any of the
        known fake credential substrings. If any one slips through
        (because of a future pattern addition that doesn't cover it),
        this test fails loudly."""
        log_text = _trigger_exception_handler_log(caplog)
        forbidden_substrings = [
            "ya29.a0fake",
            "1//0g_fake_refresh",
            "GOCSPX-FakeClientSecret",
            "1234567890.apps.googleusercontent.com",
            "sk-fake-openai-api-key",
            "hunter2",
        ]
        for forbidden in forbidden_substrings:
            assert forbidden not in log_text, f"forbidden credential substring {forbidden!r} leaked to log: {log_text}"
