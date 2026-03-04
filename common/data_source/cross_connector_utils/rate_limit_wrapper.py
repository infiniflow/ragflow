import time
import logging
from collections.abc import Callable
from functools import wraps
from typing import Any
from typing import cast
from typing import TypeVar

import requests

F = TypeVar("F", bound=Callable[..., Any])


class RateLimitTriedTooManyTimesError(Exception):
    pass


class _RateLimitDecorator:
    """Builds a generic wrapper/decorator for calls to external APIs that
    prevents making more than `max_calls` requests per `period`

    Implementation inspired by the `ratelimit` library:
    https://github.com/tomasbasham/ratelimit.

    NOTE: is not thread safe.
    """

    def __init__(
        self,
        max_calls: int,
        period: float,  # in seconds
        sleep_time: float = 2,  # in seconds
        sleep_backoff: float = 2,  # applies exponential backoff
        max_num_sleep: int = 0,
    ):
        self.max_calls = max_calls
        self.period = period
        self.sleep_time = sleep_time
        self.sleep_backoff = sleep_backoff
        self.max_num_sleep = max_num_sleep

        self.call_history: list[float] = []
        self.curr_calls = 0

    def __call__(self, func: F) -> F:
        @wraps(func)
        def wrapped_func(*args: list, **kwargs: dict[str, Any]) -> Any:
            # cleanup calls which are no longer relevant
            self._cleanup()

            # check if we've exceeded the rate limit
            sleep_cnt = 0
            while len(self.call_history) == self.max_calls:
                sleep_time = self.sleep_time * (self.sleep_backoff**sleep_cnt)
                logging.warning(
                    f"Rate limit exceeded for function {func.__name__}. "
                    f"Waiting {sleep_time} seconds before retrying."
                )
                time.sleep(sleep_time)
                sleep_cnt += 1
                if self.max_num_sleep != 0 and sleep_cnt >= self.max_num_sleep:
                    raise RateLimitTriedTooManyTimesError(
                        f"Exceeded '{self.max_num_sleep}' retries for function '{func.__name__}'"
                    )

                self._cleanup()

            # add the current call to the call history
            self.call_history.append(time.monotonic())
            return func(*args, **kwargs)

        return cast(F, wrapped_func)

    def _cleanup(self) -> None:
        curr_time = time.monotonic()
        time_to_expire_before = curr_time - self.period
        self.call_history = [
            call_time
            for call_time in self.call_history
            if call_time > time_to_expire_before
        ]


rate_limit_builder = _RateLimitDecorator


"""If you want to allow the external service to tell you when you've hit the rate limit,
use the following instead"""

R = TypeVar("R", bound=Callable[..., requests.Response])


def wrap_request_to_handle_ratelimiting(
    request_fn: R, default_wait_time_sec: int = 30, max_waits: int = 30
) -> R:
    def wrapped_request(*args: list, **kwargs: dict[str, Any]) -> requests.Response:
        for _ in range(max_waits):
            response = request_fn(*args, **kwargs)
            if response.status_code == 429:
                try:
                    wait_time = int(
                        response.headers.get("Retry-After", default_wait_time_sec)
                    )
                except ValueError:
                    wait_time = default_wait_time_sec

                time.sleep(wait_time)
                continue

            return response

        raise RateLimitTriedTooManyTimesError(f"Exceeded '{max_waits}' retries")

    return cast(R, wrapped_request)


_rate_limited_get = wrap_request_to_handle_ratelimiting(requests.get)
_rate_limited_post = wrap_request_to_handle_ratelimiting(requests.post)


class _RateLimitedRequest:
    get = _rate_limited_get
    post = _rate_limited_post


rl_requests = _RateLimitedRequest