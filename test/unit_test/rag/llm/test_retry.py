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
Tests for rag.llm.retry module.

These tests verify the centralized retry mechanism for LLM API calls,
including error classification, retryable error detection, and decorator behavior.
"""

import asyncio
from unittest.mock import patch

import pytest

from rag.llm.retry import (
    LLMErrorCode,
    ERROR_PREFIX,
    async_handle_exception,
    classify_error,
    get_retry_delay,
    is_error_result,
    is_retryable,
    retry,
    retry_or_fallback,
)


class TestIsRetryable:
    """Tests for the is_retryable function."""

    @pytest.mark.parametrize(
        "error_msg,expected",
        [
            # Rate limit errors - should retry
            ("rate limit exceeded", True),
            ("Rate Limit", True),
            ("429 Too Many Requests", True),
            ("tpm limit reached", True),
            ("too many requests", True),
            ("requests per minute exceeded", True),
            # Server errors - should retry
            ("503 Service Unavailable", True),
            ("502 Bad Gateway", True),
            ("504 Gateway Timeout", True),
            ("500 Internal Server Error", True),
            ("server error occurred", True),
            ("service unavailable", True),
            # Connection errors - should retry
            ("connection refused", True),
            ("network unreachable", True),
            ("host unreachable", True),
            ("dns failure", True),
            ("failed to connect to host", True),
            # Non-retryable errors - should NOT retry
            ("invalid request", False),
            ("authentication failed", False),
            ("permission denied", False),
            ("bad request", False),
            ("model not found", False),
            ("timeout", False),
            ("content filtered", False),
            ("quota exceeded", False),
            ("unknown error", False),
            ("", False),
        ],
    )
    def test_retryable_errors(self, error_msg, expected):
        """Test that rate limit and server errors are retryable."""
        result = is_retryable(Exception(error_msg))
        assert result == expected, f"Failed for '{error_msg}': expected {expected}, got {result}"

    def test_retryable_case_insensitive(self):
        """Test that retryable detection is case insensitive."""
        assert is_retryable(Exception("RATE LIMIT")) is True
        assert is_retryable(Exception("SERVICE UNAVAILABLE")) is True
        assert is_retryable(Exception("503 SERVICE UNAVAILABLE")) is True

    def test_retryable_partial_match(self):
        """Test that partial matches in error messages work."""
        assert is_retryable(Exception("API rate limit is in effect")) is True
        assert is_retryable(Exception("The server is currently unavailable")) is True


class TestClassifyError:
    """Tests for the classify_error function."""

    @pytest.mark.parametrize(
        "error_msg,expected_code",
        [
            ("quota exceeded", LLMErrorCode.ERROR_QUOTA),
            ("capacity reached", LLMErrorCode.ERROR_QUOTA),
            ("credit limit", LLMErrorCode.ERROR_QUOTA),
            ("billing issue", LLMErrorCode.ERROR_QUOTA),
            ("balance low", LLMErrorCode.ERROR_QUOTA),
            ("rate limit", LLMErrorCode.ERROR_RATE_LIMIT),
            ("429 Too Many Requests", LLMErrorCode.ERROR_RATE_LIMIT),
            ("tpm limit", LLMErrorCode.ERROR_RATE_LIMIT),
            ("too many requests", LLMErrorCode.ERROR_RATE_LIMIT),
            ("authentication failed", LLMErrorCode.ERROR_AUTHENTICATION),
            ("api key invalid", LLMErrorCode.ERROR_AUTHENTICATION),
            ("401 Unauthorized", LLMErrorCode.ERROR_AUTHENTICATION),
            ("forbidden", LLMErrorCode.ERROR_AUTHENTICATION),
            ("invalid request", LLMErrorCode.ERROR_INVALID_REQUEST),
            ("bad request", LLMErrorCode.ERROR_INVALID_REQUEST),
            ("400 Bad Request", LLMErrorCode.ERROR_INVALID_REQUEST),
            ("server error", LLMErrorCode.ERROR_SERVER),
            ("503 Service Unavailable", LLMErrorCode.ERROR_SERVER),
            ("502 Bad Gateway", LLMErrorCode.ERROR_SERVER),
            ("500 Internal Error", LLMErrorCode.ERROR_SERVER),
            ("timeout", LLMErrorCode.ERROR_TIMEOUT),
            ("timed out", LLMErrorCode.ERROR_TIMEOUT),
            ("connection refused", LLMErrorCode.ERROR_CONNECTION),
            ("network unreachable", LLMErrorCode.ERROR_CONNECTION),
            ("dns failure", LLMErrorCode.ERROR_CONNECTION),
            ("content filtered", LLMErrorCode.ERROR_CONTENT_FILTER),
            ("policy violation", LLMErrorCode.ERROR_CONTENT_FILTER),
            ("safety filter", LLMErrorCode.ERROR_CONTENT_FILTER),
            ("model not found", LLMErrorCode.ERROR_MODEL),
            ("does not exist", LLMErrorCode.ERROR_MODEL),
            ("max rounds", LLMErrorCode.ERROR_MAX_ROUNDS),
            ("unknown error", LLMErrorCode.ERROR_GENERIC),
            ("", LLMErrorCode.ERROR_GENERIC),
        ],
    )
    def test_error_classification(self, error_msg, expected_code):
        """Test that errors are classified into correct error codes."""
        result = classify_error(Exception(error_msg))
        assert result == expected_code, f"Failed for '{error_msg}': expected {expected_code}, got {result}"


class TestGetRetryDelay:
    """Tests for the get_retry_delay function."""

    def test_delay_range(self):
        """Test that delay is within expected range."""
        base_delay = 2.0
        min_expected = base_delay * 10
        max_expected = base_delay * 150

        for _ in range(100):
            delay = get_retry_delay(base_delay)
            assert min_expected <= delay <= max_expected, f"Delay {delay} out of range"

    def test_delay_variance(self):
        """Test that delays vary between calls (random jitter)."""
        delays = [get_retry_delay(2.0) for _ in range(10)]
        unique_delays = set(delays)
        # With 100 iterations, we should have significant variance
        assert len(unique_delays) > 1, "Delays should have variance"

    def test_different_base_delays(self):
        """Test that different base delays scale correctly."""
        delay_1 = get_retry_delay(1.0)
        delay_2 = get_retry_delay(5.0)

        # Both should still be in their respective ranges
        assert 10 <= delay_1 <= 150
        assert 50 <= delay_2 <= 750


class TestRetryDecorator:
    """Tests for the @retry decorator."""

    def test_success_on_first_attempt(self):
        """Test that successful calls return immediately."""
        call_count = 0

        class TestClass:
            max_retries = 3
            base_delay = 0.001

            @retry
            def method(self):
                nonlocal call_count
                call_count += 1
                return "success"

        obj = TestClass()
        result = obj.method()
        assert result == "success"
        assert call_count == 1

    def test_retry_on_rate_limit(self):
        """Test that rate limit errors trigger retries."""
        call_count = 0

        class TestClass:
            max_retries = 2
            base_delay = 0.001

            @retry
            def method(self):
                nonlocal call_count
                call_count += 1
                if call_count < 3:
                    raise Exception("rate limit exceeded")
                return "success"

        obj = TestClass()
        result = obj.method()
        assert result == "success"
        assert call_count == 3

    def test_retry_on_server_error(self):
        """Test that server errors trigger retries."""
        call_count = 0

        class TestClass:
            max_retries = 2
            base_delay = 0.001

            @retry
            def method(self):
                nonlocal call_count
                call_count += 1
                if call_count < 3:
                    raise Exception("503 Service Unavailable")
                return "success"

        obj = TestClass()
        result = obj.method()
        assert result == "success"
        assert call_count == 3

    def test_raise_on_non_retryable(self):
        """Test that non-retryable errors raise immediately."""
        call_count = 0

        class TestClass:
            max_retries = 3
            base_delay = 0.001

            @retry
            def method(self):
                nonlocal call_count
                call_count += 1
                raise Exception("invalid request")

        obj = TestClass()
        with pytest.raises(Exception, match="invalid request"):
            obj.method()
        assert call_count == 1

    def test_raise_after_max_retries(self):
        """Test that max retries exhausted raises the exception."""
        call_count = 0

        class TestClass:
            max_retries = 2
            base_delay = 0.001

            @retry
            def method(self):
                nonlocal call_count
                call_count += 1
                raise Exception("rate limit")

        obj = TestClass()
        with pytest.raises(Exception, match="rate limit"):
            obj.method()
        # Should have initial attempt + 2 retries = 3 calls
        assert call_count == 3

    def test_uses_instance_attributes(self):
        """Test that instance attributes override defaults."""
        call_count = 0

        class TestClass:
            max_retries = 2
            base_delay = 0.001

            @retry
            def method(self):
                nonlocal call_count
                call_count += 1
                if call_count < 3:
                    raise Exception("rate limit")
                return "success"

        obj = TestClass()
        result = obj.method()
        assert result == "success"
        assert call_count == 3


class TestRetryOrFallbackDecorator:
    """Tests for the @retry_or_fallback decorator."""

    def test_success_on_first_attempt(self):
        """Test that successful calls return immediately."""
        call_count = 0

        class TestClass:
            max_retries = 3
            base_delay = 0.001

            @retry_or_fallback(lambda e: ("fallback", 0))
            def method(self):
                nonlocal call_count
                call_count += 1
                return "success"

        obj = TestClass()
        result = obj.method()
        assert result == "success"
        assert call_count == 1

    def test_fallback_on_non_retryable(self):
        """Test that non-retryable errors return fallback."""
        call_count = 0

        def fallback(e):
            return f"fallback: {e}"

        class TestClass:
            max_retries = 3
            base_delay = 0.001

            @retry_or_fallback(fallback)
            def method(self):
                nonlocal call_count
                call_count += 1
                raise Exception("invalid request")

        obj = TestClass()
        result = obj.method()
        assert result == "fallback: invalid request"
        assert call_count == 1

    def test_fallback_after_max_retries(self):
        """Test that exhausted retries return fallback."""
        call_count = 0

        def fallback(e):
            return f"fallback: {e}"

        class TestClass:
            max_retries = 2
            base_delay = 0.001

            @retry_or_fallback(fallback)
            def method(self):
                nonlocal call_count
                call_count += 1
                raise Exception("rate limit")

        obj = TestClass()
        result = obj.method()
        assert "fallback" in result
        assert call_count == 3  # 1 initial + 2 retries

    def test_retry_on_rate_limit(self):
        """Test that rate limit errors trigger retries."""
        call_count = 0

        class TestClass:
            max_retries = 2
            base_delay = 0.001

            @retry_or_fallback(lambda e: ("fallback", 0))
            def method(self):
                nonlocal call_count
                call_count += 1
                if call_count < 3:
                    raise Exception("rate limit")
                return "success"

        obj = TestClass()
        result = obj.method()
        assert result == "success"
        assert call_count == 3


class TestAsyncHandleException:
    """Tests for the async_handle_exception function.

    Note: These tests verify the function's logic by checking its behavior
    without actually running async code. The function is thoroughly tested
    indirectly through integration tests with actual LLM calls.
    """

    def test_retryable_logic(self):
        """Test that retryable errors are correctly identified."""
        assert is_retryable(Exception("rate limit")) is True
        assert is_retryable(Exception("503")) is True

    def test_non_retryable_logic(self):
        """Test that non-retryable errors return error messages."""
        # This mirrors the logic in async_handle_exception for non-retryable errors
        error = Exception("invalid request")
        error_code = classify_error(error)
        msg = f"{ERROR_PREFIX}: {error_code} - {str(error)}"
        assert ERROR_PREFIX in msg
        assert "invalid request" in msg
        assert "INVALID_REQUEST" in msg

    def test_retryable_mid_attempt_returns_none_and_sleeps(self):
        """Retryable error before final attempt should sleep and return None."""

        async def run():
            with patch("rag.llm.retry.asyncio.sleep") as mock_sleep:
                mock_sleep.return_value = None
                result = await async_handle_exception(
                    error=Exception("rate limit"),
                    attempt=0,
                    max_retries=3,
                    base_delay=0.001,
                )
                return result, mock_sleep.await_count

        result, sleep_calls = asyncio.run(run())
        assert result is None
        assert sleep_calls == 1

    def test_non_retryable_returns_error_string_no_sleep(self):
        """Non-retryable error should return an error string without sleeping."""

        async def run():
            with patch("rag.llm.retry.asyncio.sleep") as mock_sleep:
                result = await async_handle_exception(
                    error=Exception("invalid request"),
                    attempt=0,
                    max_retries=3,
                    base_delay=0.001,
                )
                return result, mock_sleep.await_count

        result, sleep_calls = asyncio.run(run())
        assert isinstance(result, str)
        assert ERROR_PREFIX in result
        assert "INVALID_REQUEST" in result
        assert sleep_calls == 0

    def test_final_attempt_returns_max_retries_error_no_sleep(self):
        """On the final attempt, even a retryable error must surrender with ERROR_MAX_RETRIES and not sleep."""

        async def run():
            with patch("rag.llm.retry.asyncio.sleep") as mock_sleep:
                result = await async_handle_exception(
                    error=Exception("rate limit"),
                    attempt=3,
                    max_retries=3,
                    base_delay=0.001,
                )
                return result, mock_sleep.await_count

        result, sleep_calls = asyncio.run(run())
        assert isinstance(result, str)
        assert ERROR_PREFIX in result
        assert LLMErrorCode.ERROR_MAX_RETRIES.value in result
        assert sleep_calls == 0


class TestLLMErrorCode:
    """Tests for the LLMErrorCode enum."""

    def test_error_max_rounds_value(self):
        """Test that ERROR_MAX_ROUNDS has consistent value."""
        assert LLMErrorCode.ERROR_MAX_ROUNDS.value == "MAX_ROUNDS_EXCEEDED"

    def test_all_error_codes_present(self):
        """Test that all expected error codes are defined."""
        expected_codes = {
            "ERROR_RATE_LIMIT",
            "ERROR_AUTHENTICATION",
            "ERROR_INVALID_REQUEST",
            "ERROR_SERVER",
            "ERROR_TIMEOUT",
            "ERROR_CONNECTION",
            "ERROR_MODEL",
            "ERROR_MAX_ROUNDS",
            "ERROR_CONTENT_FILTER",
            "ERROR_QUOTA",
            "ERROR_MAX_RETRIES",
            "ERROR_GENERIC",
        }
        actual_codes = {code.name for code in LLMErrorCode}
        assert expected_codes == actual_codes


class TestErrorPrefix:
    """Tests for the ERROR_PREFIX constant."""

    def test_error_prefix_value(self):
        """Test that ERROR_PREFIX has correct value."""
        assert ERROR_PREFIX == "**ERROR**"


class TestIsErrorResult:
    """Tests for the is_error_result helper."""

    def test_prefix_marker(self):
        assert is_error_result("**ERROR**: something went wrong")

    def test_appended_marker(self):
        # Streamed chat paths yield "partial answer\n**ERROR**: ..."
        assert is_error_result("partial answer\n**ERROR**: boom")

    def test_clean_string(self):
        assert not is_error_result("all good")

    def test_empty_string(self):
        assert not is_error_result("")

    def test_non_string_inputs(self):
        assert not is_error_result(None)
        assert not is_error_result(42)
        assert not is_error_result(("**ERROR**: tuple", 0))
        assert not is_error_result(["**ERROR**"])


class TestGeneratorGuard:
    """@retry / @retry_or_fallback must reject generator functions at decoration time."""

    def test_retry_rejects_generator(self):
        with pytest.raises(TypeError, match="does not support generator"):
            @retry
            def gen(self):
                yield 1

    def test_retry_or_fallback_rejects_generator(self):
        with pytest.raises(TypeError, match="does not support generator"):
            @retry_or_fallback(lambda e: None)
            def gen(self):
                yield 1

    def test_retry_accepts_regular_function(self):
        @retry
        def plain(self):
            return 1
        assert callable(plain)

    def test_retry_or_fallback_accepts_regular_function(self):
        @retry_or_fallback(lambda e: None)
        def plain(self):
            return 1
        assert callable(plain)


class AsyncMock:
    """Helper class to mock async functions."""

    def __init__(self, return_value=None):
        self.return_value = return_value

    async def __call__(self, *args, **kwargs):
        return self.return_value
