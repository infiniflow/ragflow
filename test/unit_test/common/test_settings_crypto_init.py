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
"""Unit tests for ``common.settings._init_crypto_storage``.

Closes the security gap reported in issue #17752: when
``RAGFLOW_CRYPTO_ENABLED=true`` and the crypto configuration is broken
(missing ``RAGFLOW_CRYPTO_KEY``, unsupported ``RAGFLOW_CRYPTO_ALGORITHM``,
broken wrapper), the original code in ``init_settings()`` caught every
exception from ``create_encrypted_storage()``, logged it once at ERROR
level, and silently fell back to the **plaintext** storage impl. The
operator's data ended up on disk unencrypted, with no health check, no
metric and no startup banner that distinguished "encrypting" from
"silently not encrypting".

The fix: extract the crypto init into ``_init_crypto_storage`` and have
it re-raise the underlying exception after a CRITICAL log line that
names the misconfigured env vars. Plaintext fallback only happens when
``crypto_enabled=False`` (the operator opted out).
"""

import logging
from unittest.mock import patch

import pytest

from common.settings import _init_crypto_storage


@pytest.fixture
def storage_impl():
    """A sentinel for the underlying plaintext storage impl."""
    return object()


# ---------------------------------------------------------------------------
# crypto_enabled=False — plaintext path (unchanged behavior)
# ---------------------------------------------------------------------------


class TestDisabled:
    """When the operator did NOT request encryption, the plaintext impl is
    returned unchanged regardless of the other arguments. No wrapper is
    ever created, so the operator's misconfigurations (missing KEY, bad
    ALGORITHM) are silently ignored — which is correct, because the
    operator opted out.
    """

    def test_returns_plaintext_unchanged(self, storage_impl):
        result = _init_crypto_storage(
            storage_impl,
            crypto_enabled=False,
            algorithm="aes-256-cbc",
            crypto_key="unused",
        )
        assert result is storage_impl

    def test_never_creates_wrapper(self, storage_impl):
        with patch("rag.utils.encrypted_storage.create_encrypted_storage") as mock_create:
            _init_crypto_storage(
                storage_impl,
                crypto_enabled=False,
                algorithm="aes-256-cbc",
                crypto_key="some-key",
            )
        mock_create.assert_not_called()

    def test_works_with_none_key_and_bad_algorithm(self, storage_impl):
        # crypto_enabled=False short-circuits before any validation, so a
        # None key or unsupported algorithm is fine here.
        result = _init_crypto_storage(
            storage_impl,
            crypto_enabled=False,
            algorithm="bogus-algorithm",
            crypto_key=None,
        )
        assert result is storage_impl


# ---------------------------------------------------------------------------
# crypto_enabled=True — wrapper path
# ---------------------------------------------------------------------------


class TestEnabledHappyPath:
    """When crypto is enabled and the config is valid, the wrapper is
    created with the supplied algorithm and key. ``create_encrypted_storage``
    receives the exact (algorithm, key, encryption_enabled) we passed.
    """

    def test_returns_wrapper(self, storage_impl):
        wrapper = object()
        with patch("rag.utils.encrypted_storage.create_encrypted_storage", return_value=wrapper) as mock_create:
            result = _init_crypto_storage(
                storage_impl,
                crypto_enabled=True,
                algorithm="aes-256-cbc",
                crypto_key="my-key",
            )
        assert result is wrapper
        mock_create.assert_called_once_with(
            storage_impl,
            algorithm="aes-256-cbc",
            key="my-key",
            encryption_enabled=True,
        )


# ---------------------------------------------------------------------------
# crypto_enabled=True + broken config — must FAIL HARD (the bug)
# ---------------------------------------------------------------------------


class TestEnabledFailsHard:
    """When crypto is enabled and the wrapper init fails for any reason,
    the original exception MUST propagate. The plaintext ``storage_impl``
    must NEVER be returned, because that is the security failure mode
    described in #17752.
    """

    def test_missing_key_raises_value_error(self, storage_impl):
        # CryptoUtil("aes-256-cbc", key=None) raises ValueError at __init__
        # because the key derivation requires a non-empty key. The wrapper
        # therefore fails to construct, and the plaintext fallback MUST NOT
        # happen.
        with pytest.raises(ValueError):
            _init_crypto_storage(
                storage_impl,
                crypto_enabled=True,
                algorithm="aes-256-cbc",
                crypto_key=None,
            )

    def test_empty_key_raises_value_error(self, storage_impl):
        with pytest.raises(ValueError):
            _init_crypto_storage(
                storage_impl,
                crypto_enabled=True,
                algorithm="aes-256-cbc",
                crypto_key="",
            )

    def test_unsupported_algorithm_raises_value_error(self, storage_impl):
        with pytest.raises(ValueError):
            _init_crypto_storage(
                storage_impl,
                crypto_enabled=True,
                algorithm="aes256-cbc",  # missing hyphen — typo case from the issue
                crypto_key="my-key",
            )

    def test_wrapper_constructor_exception_propagates(self, storage_impl):
        # Simulate any non-ValueError failure (e.g. AttributeError from a
        # broken storage_impl, ImportError from a bad wrapper). The
        # helper must re-raise unchanged, not fall back.
        with patch(
            "rag.utils.encrypted_storage.create_encrypted_storage",
            side_effect=AttributeError("Storage implementation missing required method: put"),
        ):
            with pytest.raises(AttributeError, match="missing required method"):
                _init_crypto_storage(
                    storage_impl,
                    crypto_enabled=True,
                    algorithm="aes-256-cbc",
                    crypto_key="my-key",
                )

    def test_plaintext_is_never_returned_on_failure(self, storage_impl):
        # The bug's core failure mode: ``storage_impl`` (plaintext) leaks
        # through as the active STORAGE_IMPL. Pin that this can never
        # happen — every failure must raise, and the caller never sees
        # ``storage_impl`` as a result.
        with patch(
            "rag.utils.encrypted_storage.create_encrypted_storage",
            side_effect=RuntimeError("anything"),
        ):
            with pytest.raises(RuntimeError):
                result = _init_crypto_storage(
                    storage_impl,
                    crypto_enabled=True,
                    algorithm="aes-256-cbc",
                    crypto_key="my-key",
                )
                # ``result`` is never bound because the helper raises; this
                # assertion is for documentation only.
                assert result is not storage_impl  # pragma: no cover


# ---------------------------------------------------------------------------
# CRITICAL log line — the operator's only signal
# ---------------------------------------------------------------------------


class TestCriticalLogging:
    """The fix logs at CRITICAL level before re-raising, naming the
    misconfigured env vars. The previous behavior logged once at ERROR
    level and moved on — operators reading the log had no way to tell
    "encryption failed at startup" from any other transient ERROR.
    """

    def test_logs_critical_on_failure(self, storage_impl, caplog):
        with caplog.at_level(logging.CRITICAL, logger="common.settings"):
            with patch(
                "rag.utils.encrypted_storage.create_encrypted_storage",
                side_effect=ValueError("bad key"),
            ):
                with pytest.raises(ValueError):
                    _init_crypto_storage(
                        storage_impl,
                        crypto_enabled=True,
                        algorithm="aes-256-cbc",
                        crypto_key="my-key",
                    )
        critical = [r for r in caplog.records if r.levelno == logging.CRITICAL]
        assert critical, "expected a CRITICAL log line on crypto init failure"
        msg = critical[0].getMessage()
        # The error message must name the env vars so the operator knows
        # what to check.
        assert "RAGFLOW_CRYPTO_KEY" in msg
        assert "RAGFLOW_CRYPTO_ALGORITHM" in msg
        # It must also explain WHY the server refuses to start, so the
        # operator doesn't think this is a recoverable warning.
        assert "plaintext" in msg.lower()
        assert "refus" in msg.lower()  # refuse / refusing / refused

    def test_does_not_log_when_disabled(self, storage_impl, caplog):
        # crypto_enabled=False is a normal path; no warning, no critical.
        with caplog.at_level(logging.CRITICAL, logger="common.settings"):
            _init_crypto_storage(
                storage_impl,
                crypto_enabled=False,
                algorithm="aes-256-cbc",
                crypto_key=None,
            )
        assert [r for r in caplog.records if r.levelno == logging.CRITICAL] == []

    def test_does_not_log_when_init_succeeds(self, storage_impl, caplog):
        with caplog.at_level(logging.CRITICAL, logger="common.settings"):
            with patch("rag.utils.encrypted_storage.create_encrypted_storage", return_value=object()):
                _init_crypto_storage(
                    storage_impl,
                    crypto_enabled=True,
                    algorithm="aes-256-cbc",
                    crypto_key="my-key",
                )
        assert [r for r in caplog.records if r.levelno == logging.CRITICAL] == []


# ---------------------------------------------------------------------------
# Original exception is preserved (not wrapped, not swallowed)
# ---------------------------------------------------------------------------


class TestExceptionPreserved:
    """The re-raised exception must be the SAME exception that the wrapper
    raised, not a wrapper, not a different type, and not the CRITICAL log's
    string. Operators reading the traceback need to see ``ValueError: bad
    key`` or ``AttributeError: missing method: put`` — whatever the wrapper
    actually said — to know what to fix.
    """

    def test_value_error_passes_through_unchanged(self, storage_impl):
        sentinel = ValueError("specific reason")
        with patch("rag.utils.encrypted_storage.create_encrypted_storage", side_effect=sentinel):
            with pytest.raises(ValueError) as exc_info:
                _init_crypto_storage(
                    storage_impl,
                    crypto_enabled=True,
                    algorithm="aes-256-cbc",
                    crypto_key="my-key",
                )
        assert exc_info.value is sentinel  # identity, not just type

    def test_attribute_error_passes_through_unchanged(self, storage_impl):
        sentinel = AttributeError("Storage implementation missing required method: put")
        with patch("rag.utils.encrypted_storage.create_encrypted_storage", side_effect=sentinel):
            with pytest.raises(AttributeError) as exc_info:
                _init_crypto_storage(
                    storage_impl,
                    crypto_enabled=True,
                    algorithm="aes-256-cbc",
                    crypto_key="my-key",
                )
        assert exc_info.value is sentinel


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
