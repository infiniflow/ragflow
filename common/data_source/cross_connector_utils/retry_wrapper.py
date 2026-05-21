from collections.abc import Callable
import logging
from logging import Logger
from typing import Any
from typing import cast
from typing import TypeVar
import requests
from retry import retry

from common.data_source.config import REQUEST_TIMEOUT_SECONDS


F = TypeVar("F", bound=Callable[..., Any])
logger = logging.getLogger(__name__)

def retry_builder(
    tries: int = 20,
    delay: float = 0.1,
    max_delay: float | None = 60,
    backoff: float = 2,
    jitter: tuple[float, float] | float = 1,
    exceptions: type[Exception] | tuple[type[Exception], ...] = (Exception,),
) -> Callable[[F], F]:
    """Builds a generic wrapper/decorator for calls to external APIs that
    may fail due to rate limiting, flakes, or other reasons. Applies exponential
    backoff with jitter to retry the call."""

    def retry_with_default(func: F) -> F:
        @retry(
            tries=tries,
            delay=delay,
            max_delay=max_delay,
            backoff=backoff,
            jitter=jitter,
            logger=cast(Logger, logger),
            exceptions=exceptions,
        )
        def wrapped_func(*args: list, **kwargs: dict[str, Any]) -> Any:
            return func(*args, **kwargs)

        return cast(F, wrapped_func)

    return retry_with_default


def request_with_retries(
    method: str,
    url: str,
    *,
    data: dict[str, Any] | None = None,
    headers: dict[str, Any] | None = None,
    params: dict[str, Any] | None = None,
    timeout: int = REQUEST_TIMEOUT_SECONDS,
    stream: bool = False,
    tries: int = 8,
    delay: float = 1,
    backoff: float = 2,
) -> requests.Response:
    @retry(tries=tries, delay=delay, backoff=backoff, logger=cast(Logger, logger))
    def _make_request() -> requests.Response:
        response = requests.request(
            method=method,
            url=url,
            data=data,
            headers=headers,
            params=params,
            timeout=timeout,
            stream=stream,
        )
        try:
            response.raise_for_status()
        except requests.exceptions.HTTPError:
            logging.exception(
                "Request failed:\n%s",
                {
                    "method": method,
                    "url": url,
                    "data": data,
                    "headers": headers,
                    "params": params,
                    "timeout": timeout,
                    "stream": stream,
                },
            )
            raise
        return response

    return _make_request()