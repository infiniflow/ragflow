#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
Retry utilities for LLM API calls.

This module provides centralized retry logic with exponential backoff for transient
errors (rate limits, server errors). It defines error classification helpers,
decorators for synchronous and asynchronous retry, and shared error handling.

Public API:
    - retry: Decorator that retries on transient errors, re-raising on failure
    - retry_or_fallback: Decorator that retries and returns a fallback value on failure
    - is_retryable: Check if an exception is transient and worth retrying
    - classify_error: Classify an exception into an LLMErrorCode
    - async_handle_exception: Shared async error handler for retry loops
    - LLMErrorCode: Enum of error codes for LLM API failures
    - ERROR_PREFIX: String prefix for error messages

Environment variables:
    - LLM_MAX_RETRIES: Maximum retry attempts (default: 5)
    - LLM_BASE_DELAY: Base delay in seconds for exponential backoff (default: 1.0).
      Retry N waits ``base_delay * 2**N + uniform(0, base_delay)`` seconds,
      capped at 60 s.
"""

from __future__ import annotations

import asyncio
import functools
import inspect
import logging
import os
import random
import time
from collections.abc import Callable
from strenum import StrEnum
from typing import TypeVar

ERROR_PREFIX = "**ERROR**"


def is_error_result(result: object) -> bool:
    """Return True if *result* is a string carrying the LLM error marker.

    Producers encode failures as strings containing ``ERROR_PREFIX`` (either
    as a leading marker or appended after partial streamed output). Callers
    use this helper instead of ad-hoc substring checks.
    """
    return isinstance(result, str) and ERROR_PREFIX in result

_MAX_DELAY_SECONDS = 60.0

_T = TypeVar("_T")


class LLMErrorCode(StrEnum):
    ERROR_RATE_LIMIT = "RATE_LIMIT_EXCEEDED"
    ERROR_AUTHENTICATION = "AUTH_ERROR"
    ERROR_INVALID_REQUEST = "INVALID_REQUEST"
    ERROR_SERVER = "SERVER_ERROR"
    ERROR_TIMEOUT = "TIMEOUT"
    ERROR_CONNECTION = "CONNECTION_ERROR"
    ERROR_MODEL = "MODEL_ERROR"
    ERROR_MAX_ROUNDS = "MAX_ROUNDS_EXCEEDED"
    ERROR_CONTENT_FILTER = "CONTENT_FILTERED"
    ERROR_QUOTA = "QUOTA_EXCEEDED"
    ERROR_MAX_RETRIES = "MAX_RETRIES_EXCEEDED"
    ERROR_GENERIC = "GENERIC_ERROR"


_RETRYABLE_RATE_LIMIT_SIGNALS: frozenset[str] = frozenset(
    {
        "rate limit",
        "429",
        "tpm limit",
        "too many requests",
        "requests per minute",
    }
)
_RETRYABLE_SERVER_SIGNALS: frozenset[str] = frozenset(
    {
        "server",
        "503",
        "502",
        "504",
        "500",
        "unavailable",
    }
)
_RETRYABLE_CONNECTION_SIGNALS: frozenset[str] = frozenset(
    {
        "connect",
        "network",
        "unreachable",
        "dns",
    }
)

_ERROR_CLASSIFICATION: tuple[tuple[frozenset[str], LLMErrorCode], ...] = (
    (frozenset({"quota", "capacity", "credit", "billing", "balance", "欠费"}), LLMErrorCode.ERROR_QUOTA),
    (frozenset({"rate limit", "429", "tpm limit", "too many requests", "requests per minute"}), LLMErrorCode.ERROR_RATE_LIMIT),
    (frozenset({"auth", "key", "apikey", "401", "forbidden", "permission"}), LLMErrorCode.ERROR_AUTHENTICATION),
    (frozenset({"invalid", "bad request", "400", "format", "malformed", "parameter"}), LLMErrorCode.ERROR_INVALID_REQUEST),
    (frozenset({"server", "503", "502", "504", "500", "unavailable"}), LLMErrorCode.ERROR_SERVER),
    (frozenset({"timeout", "timed out"}), LLMErrorCode.ERROR_TIMEOUT),
    (frozenset({"connect", "network", "unreachable", "dns"}), LLMErrorCode.ERROR_CONNECTION),
    (frozenset({"filter", "content", "policy", "blocked", "safety", "inappropriate"}), LLMErrorCode.ERROR_CONTENT_FILTER),
    (frozenset({"model", "not found", "does not exist", "not available"}), LLMErrorCode.ERROR_MODEL),
    (frozenset({"max rounds"}), LLMErrorCode.ERROR_MAX_ROUNDS),
)


def is_retryable(error: Exception) -> bool:
    """Determine if an exception represents a transient error worth retrying.

    Retries on rate-limit errors (429, TPM limits), server errors (5xx),
    and transient connection errors (network/DNS/unreachable).
    All other errors are considered non-retryable.

    Args:
        error: The exception to evaluate.

    Returns:
        True if the error is transient and should be retried.
    """
    msg = str(error).lower()
    return (
        any(signal in msg for signal in _RETRYABLE_RATE_LIMIT_SIGNALS)
        or any(signal in msg for signal in _RETRYABLE_SERVER_SIGNALS)
        or any(signal in msg for signal in _RETRYABLE_CONNECTION_SIGNALS)
    )


def classify_error(error: Exception) -> LLMErrorCode:
    """Classify an exception into an error code for detailed error reporting.

    Matches the exception message against known error patterns to determine
    the appropriate LLMErrorCode for logging and error handling.

    Args:
        error: The exception to classify.

    Returns:
        The LLMErrorCode that best describes the error.
    """
    msg = str(error).lower()
    for signals, code in _ERROR_CLASSIFICATION:
        if any(signal in msg for signal in signals):
            return code
    return LLMErrorCode.ERROR_GENERIC


def get_retry_delay(base_delay: float, attempt: int) -> float:
    """Calculate the retry delay using exponential backoff with additive jitter.

    The delay is ``base_delay * 2**attempt + uniform(0, base_delay)``, capped
    at ``_MAX_DELAY_SECONDS``. This makes the first retry fast (roughly one
    base delay) and later retries exponentially more patient, while a small
    jitter spreads concurrent callers to avoid thundering herd.

    Args:
        base_delay: The base delay in seconds.
        attempt: The current retry attempt number (0-indexed).

    Returns:
        The delay in seconds, capped at ``_MAX_DELAY_SECONDS``.
    """
    expo = base_delay * (2 ** attempt)
    jitter = random.uniform(0, base_delay)
    return min(expo + jitter, _MAX_DELAY_SECONDS)


def retry(method: Callable[..., _T]) -> Callable[..., _T]:
    """Decorator that retries on transient errors with exponential backoff.

    Catches exceptions and retries up to max_retries times for transient
    errors (rate limits, server errors). Re-raises the exception after
    exhausting retries or for non-retryable errors.

    Uses getattr to read max_retries and base_delay from self, with
    environment variable fallbacks (LLM_MAX_RETRIES, LLM_BASE_DELAY).

    Args:
        method: The method to wrap with retry logic.

    Returns:
        The wrapped method with retry behavior.

    Limitations:
        Generator functions are rejected at decoration time. Calling a
        generator returns the generator object without raising, so the
        try/except would never see iteration errors and the decorator
        would silently be a no-op.
    """
    if inspect.isgeneratorfunction(method):
        raise TypeError(
            "@retry does not support generator functions; "
            "wrap the non-streaming call instead"
        )

    @functools.wraps(method)
    def wrapper(self, *args, **kwargs):
        max_retries = getattr(self, "max_retries", int(os.environ.get("LLM_MAX_RETRIES", 5)))
        base_delay = getattr(self, "base_delay", float(os.environ.get("LLM_BASE_DELAY", 1.0)))
        for attempt in range(max_retries + 1):
            try:
                return method(self, *args, **kwargs)
            except Exception as e:
                if attempt == max_retries or not is_retryable(e):
                    raise
                delay = get_retry_delay(base_delay, attempt)
                logging.warning(f"Retrying in {delay:.2f}s (attempt {attempt + 1}/{max_retries})")
                time.sleep(delay)

    return wrapper


def retry_or_fallback(error_result: Callable[[Exception], _T]) -> Callable[[Callable[..., _T]], Callable[..., _T]]:
    """Decorator that retries on transient errors and returns a fallback value on failure.

    Unlike retry(), which re-raises exceptions, this decorator returns a fallback
    value when retries are exhausted or the error is not retryable. Useful for
    methods that traditionally returned error tuples instead of raising.

    Args:
        error_result: A callable that takes the exception and returns the fallback value.

    Returns:
        A decorator that wraps the method with retry-or-fallback behavior.

    Limitations:
        Generator functions are rejected at decoration time for the same
        reason as ``retry``: the try/except would never see iteration errors.
    """

    def decorator(method: Callable[..., _T]) -> Callable[..., _T]:
        if inspect.isgeneratorfunction(method):
            raise TypeError(
                "@retry_or_fallback does not support generator functions; "
                "wrap the non-streaming call instead"
            )

        @functools.wraps(method)
        def wrapper(self, *args, **kwargs):
            max_retries = getattr(self, "max_retries", int(os.environ.get("LLM_MAX_RETRIES", 5)))
            base_delay = getattr(self, "base_delay", float(os.environ.get("LLM_BASE_DELAY", 1.0)))
            for attempt in range(max_retries + 1):
                try:
                    return method(self, *args, **kwargs)
                except Exception as e:
                    if attempt == max_retries or not is_retryable(e):
                        return error_result(e)
                    delay = get_retry_delay(base_delay, attempt)
                    logging.warning(f"Retrying in {delay:.2f}s (attempt {attempt + 1}/{max_retries})")
                    time.sleep(delay)

        return wrapper

    return decorator


async def async_handle_exception(
    error: Exception,
    attempt: int,
    max_retries: int,
    base_delay: float,
    log_prefix: str = "LLM",
) -> str | None:
    """Shared async error handler for retry loops in async LLM methods.

    Classifies the error, decides whether to retry, and returns appropriately:
    - Returns None to signal the caller should sleep and continue retrying
    - Returns an error message string to signal the caller should give up

    Args:
        error: The exception that was raised.
        attempt: The current retry attempt number (0-indexed).
        max_retries: Maximum number of retries allowed.
        base_delay: Base delay in seconds for backoff calculation.
        log_prefix: Prefix for log messages (e.g., "OpenAI", "LiteLLMBase").

    Returns:
        None if the error is retryable and retries remain; otherwise an error message string.
    """
    logging.exception(f"{log_prefix} async completion")
    error_code = classify_error(error)

    if attempt == max_retries:
        error_code = LLMErrorCode.ERROR_MAX_RETRIES
        msg = f"{ERROR_PREFIX}: {error_code} - {str(error)}"
        logging.error(f"{log_prefix} giving up after {max_retries} retries: {msg}")
        return msg

    if is_retryable(error):
        delay = get_retry_delay(base_delay, attempt)
        logging.warning(f"Error: {error_code}. Retrying in {delay:.2f}s (attempt {attempt + 1}/{max_retries})")
        await asyncio.sleep(delay)
        return None
    msg = f"{ERROR_PREFIX}: {error_code} - {str(error)}"
    logging.error(f"{log_prefix} giving up: {msg}")
    return msg
