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
"""Pre-authentication rate limiting for /run.

Authentication dependencies run before the SlowAPI-decorated handler, so
without this layer invalid-token traffic never touches the execution rate
limit: a reachable executor manager could be made to perform unbounded
authentication work (compare_digest plus a warning log per attempt) without
ever receiving a 429. This module applies a small per-address sliding-window
limit to ALL /run traffic, before authentication, so repeated unauthenticated
requests are eventually throttled. Authenticated execution keeps the larger
SlowAPI quota as a second layer.
"""

import os
import threading
import time
from collections import deque

from core.logger import logger
from fastapi import Request
from limits import parse_many
from slowapi.errors import RateLimitExceeded

PRE_AUTH_RATE_LIMIT = os.getenv("SANDBOX_RUN_PREAUTH_RATE_LIMIT", "30/minute")


class _LimitDescriptor:
    """Minimal stand-in for slowapi's Limit that RateLimitExceeded accepts."""

    def __init__(self, limit):
        self.limit = limit
        self.error_message = "Too many requests, please try again later"


def _parse_rate_limit(value: str):
    items = list(parse_many(value))
    if len(items) != 1:
        raise ValueError(f"SANDBOX_RUN_PREAUTH_RATE_LIMIT must be a single rate like '30/minute', got {value!r}")
    return items[0]


class PreAuthRateLimiter:
    """Thread-safe sliding-window limiter keyed by client address."""

    def __init__(self, limit):
        self._limit = limit
        self._window_seconds = limit.get_expiry()
        self._max_amount = limit.amount
        self._hits: dict[str, deque] = {}
        # Bounded rejection logging: at most one warning per address per
        # window, so throttling is observable without unbounded log writes.
        self._last_logged: dict[str, float] = {}
        self._lock = threading.Lock()

    def hit(self, address: str, now: float | None = None) -> None:
        """Record one hit for `address`; raise RateLimitExceeded when over budget."""
        timestamp = time.monotonic() if now is None else now
        with self._lock:
            window = self._hits.get(address)
            if window is None:
                window = deque()
                self._hits[address] = window
            cutoff = timestamp - self._window_seconds
            while window and window[0] <= cutoff:
                window.popleft()
            if len(window) >= self._max_amount:
                if timestamp - self._last_logged.get(address, float("-inf")) >= self._window_seconds:
                    self._last_logged[address] = timestamp
                    logger.warning(
                        "Pre-auth rate limit exceeded for %s on /run (%s); further rejections from this address are throttled silently for this window",
                        address,
                        self._limit,
                    )
                raise RateLimitExceeded(_LimitDescriptor(self._limit))
            window.append(timestamp)
            # Opportunistic cleanup keeps the address maps bounded under scans.
            if len(self._hits) > 10_000:
                stale = [key for key, hits in self._hits.items() if not hits or hits[-1] <= cutoff]
                for key in stale:
                    del self._hits[key]
                    self._last_logged.pop(key, None)

    def reset(self) -> None:
        with self._lock:
            self._hits.clear()
            self._last_logged.clear()


_preauth_limit_item = _parse_rate_limit(PRE_AUTH_RATE_LIMIT)
preauth_limiter = PreAuthRateLimiter(_preauth_limit_item)


def preauth_rate_limit(request: Request) -> None:
    """FastAPI dependency: throttle /run traffic before authentication."""
    address = request.client.host if request.client else "unknown"
    preauth_limiter.hit(address)
