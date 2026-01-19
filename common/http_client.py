# Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
import asyncio
import logging
import os
import time
from typing import Any, Dict, Optional
from urllib.parse import urlparse, urlunparse

from common import settings
import httpx

logger = logging.getLogger(__name__)

# Default knobs; keep conservative to avoid unexpected behavioural changes.
DEFAULT_TIMEOUT = float(os.environ.get("HTTP_CLIENT_TIMEOUT", "15"))
# Align with requests default: follow redirects with a max of 30 unless overridden.
DEFAULT_FOLLOW_REDIRECTS = bool(
    int(os.environ.get("HTTP_CLIENT_FOLLOW_REDIRECTS", "1"))
)
DEFAULT_MAX_REDIRECTS = int(os.environ.get("HTTP_CLIENT_MAX_REDIRECTS", "30"))
DEFAULT_MAX_RETRIES = int(os.environ.get("HTTP_CLIENT_MAX_RETRIES", "2"))
DEFAULT_BACKOFF_FACTOR = float(os.environ.get("HTTP_CLIENT_BACKOFF_FACTOR", "0.5"))
DEFAULT_PROXY = os.environ.get("HTTP_CLIENT_PROXY")
DEFAULT_USER_AGENT = os.environ.get("HTTP_CLIENT_USER_AGENT", "ragflow-http-client")


def _clean_headers(
    headers: Optional[Dict[str, str]], auth_token: Optional[str] = None
) -> Optional[Dict[str, str]]:
    merged_headers: Dict[str, str] = {}
    if DEFAULT_USER_AGENT:
        merged_headers["User-Agent"] = DEFAULT_USER_AGENT
    if auth_token:
        merged_headers["Authorization"] = auth_token
    if headers is None:
        return merged_headers or None
    merged_headers.update({str(k): str(v) for k, v in headers.items() if v is not None})
    return merged_headers or None


def _get_delay(backoff_factor: float, attempt: int) -> float:
    return backoff_factor * (2**attempt)


# List of sensitive parameters to redact from URLs before logging
_SENSITIVE_QUERY_KEYS = {"client_secret", "secret", "code", "access_token", "refresh_token", "password", "token", "app_secret"}

def _redact_sensitive_url_params(url: str) -> str:
    """
    Return a version of the URL that is safe to log.

    We intentionally drop query parameters and userinfo to avoid leaking
    credentials or tokens via logs. Only scheme, host, port and path
    are preserved.
    """
    try:
        parsed = urlparse(url)
        # Remove any potential userinfo (username:password@)
        netloc = parsed.hostname or ""
        if parsed.port:
            netloc = f"{netloc}:{parsed.port}"
        # Reconstruct URL without query, params, fragment, or userinfo.
        safe_url = urlunparse(
            (
                parsed.scheme,
                netloc,
                parsed.path,
                "",  # params
                "",  # query
                "",  # fragment
            )
        )
        return safe_url
    except Exception:
        # If parsing fails, fall back to omitting the URL entirely.
        return "<redacted-url>"

def _is_sensitive_url(url: str) -> bool:
    """Return True if URL is one of the configured OAuth endpoints."""
    # Collect known sensitive endpoint URLs from settings
    oauth_urls = set()
    # GitHub OAuth endpoints
    try:
        if settings.GITHUB_OAUTH is not None:
            url_val = settings.GITHUB_OAUTH.get("url")
            if url_val:
                oauth_urls.add(url_val)
    except Exception:
        pass
    # Feishu OAuth endpoints
    try:
        if settings.FEISHU_OAUTH is not None:
            for k in ("app_access_token_url", "user_access_token_url"):
                url_val = settings.FEISHU_OAUTH.get(k)
                if url_val:
                    oauth_urls.add(url_val)
    except Exception:
        pass
    # Defensive normalization: compare only scheme+netloc+path
    url_obj = urlparse(url)
    for sensitive_url in oauth_urls:
        sensitive_obj = urlparse(sensitive_url)
        if (url_obj.scheme, url_obj.netloc, url_obj.path) == (sensitive_obj.scheme, sensitive_obj.netloc, sensitive_obj.path):
            return True
    return False

async def async_request(
    method: str,
    url: str,
    *,
    request_timeout: float | httpx.Timeout | None = None,
    follow_redirects: bool | None = None,
    max_redirects: Optional[int] = None,
    headers: Optional[Dict[str, str]] = None,
    auth_token: Optional[str] = None,
    retries: Optional[int] = None,
    backoff_factor: Optional[float] = None,
    proxy: Any = None,
    **kwargs: Any,
) -> httpx.Response:
    """Lightweight async HTTP wrapper using httpx.AsyncClient with safe defaults."""
    timeout = request_timeout if request_timeout is not None else DEFAULT_TIMEOUT
    follow_redirects = (
        DEFAULT_FOLLOW_REDIRECTS if follow_redirects is None else follow_redirects
    )
    max_redirects = DEFAULT_MAX_REDIRECTS if max_redirects is None else max_redirects
    retries = DEFAULT_MAX_RETRIES if retries is None else max(retries, 0)
    backoff_factor = (
        DEFAULT_BACKOFF_FACTOR if backoff_factor is None else backoff_factor
    )
    headers = _clean_headers(headers, auth_token=auth_token)
    proxy = DEFAULT_PROXY if proxy is None else proxy

    async with httpx.AsyncClient(
        timeout=timeout,
        follow_redirects=follow_redirects,
        max_redirects=max_redirects,
        proxy=proxy,
    ) as client:
        last_exc: Exception | None = None
        for attempt in range(retries + 1):
            try:
                start = time.monotonic()
                response = await client.request(
                    method=method, url=url, headers=headers, **kwargs
                )
                duration = time.monotonic() - start
                if not _is_sensitive_url(url):
                    log_url = _redact_sensitive_url_params(url)
                    logger.debug(f"async_request {method} {log_url} -> {response.status_code} in {duration:.3f}s")
                return response
            except httpx.RequestError as exc:
                last_exc = exc
                if attempt >= retries:
                    if not _is_sensitive_url(url):
                        log_url = _redact_sensitive_url_params(url)
                        logger.warning(f"async_request exhausted retries for {method}")
                    raise
                delay = _get_delay(backoff_factor, attempt)
                if not _is_sensitive_url(url):
                    log_url = _redact_sensitive_url_params(url)
                    logger.warning(
                        f"async_request attempt {attempt + 1}/{retries + 1} failed for {method}; retrying in {delay:.2f}s"
                    )
                    raise
                delay = _get_delay(backoff_factor, attempt)
                # Avoid including the (potentially sensitive) URL in retry logs.
                logger.warning(
                    f"async_request attempt {attempt + 1}/{retries + 1} failed for {method}; retrying in {delay:.2f}s"
                )
                await asyncio.sleep(delay)
        raise last_exc  # pragma: no cover


def sync_request(
    method: str,
    url: str,
    *,
    timeout: float | httpx.Timeout | None = None,
    follow_redirects: bool | None = None,
    max_redirects: Optional[int] = None,
    headers: Optional[Dict[str, str]] = None,
    auth_token: Optional[str] = None,
    retries: Optional[int] = None,
    backoff_factor: Optional[float] = None,
    proxy: Any = None,
    **kwargs: Any,
) -> httpx.Response:
    """Synchronous counterpart to async_request, for CLI/tests or sync contexts."""
    timeout = timeout if timeout is not None else DEFAULT_TIMEOUT
    follow_redirects = (
        DEFAULT_FOLLOW_REDIRECTS if follow_redirects is None else follow_redirects
    )
    max_redirects = DEFAULT_MAX_REDIRECTS if max_redirects is None else max_redirects
    retries = DEFAULT_MAX_RETRIES if retries is None else max(retries, 0)
    backoff_factor = (
        DEFAULT_BACKOFF_FACTOR if backoff_factor is None else backoff_factor
    )
    headers = _clean_headers(headers, auth_token=auth_token)
    proxy = DEFAULT_PROXY if proxy is None else proxy

    with httpx.Client(
        timeout=timeout,
        follow_redirects=follow_redirects,
        max_redirects=max_redirects,
        proxy=proxy,
    ) as client:
        last_exc: Exception | None = None
        for attempt in range(retries + 1):
            try:
                start = time.monotonic()
                response = client.request(
                    method=method, url=url, headers=headers, **kwargs
                )
                duration = time.monotonic() - start
                logger.debug(
                    f"sync_request {method} {url} -> {response.status_code} in {duration:.3f}s"
                )
                return response
            except httpx.RequestError as exc:
                last_exc = exc
                if attempt >= retries:
                    logger.warning(
                        f"sync_request exhausted retries for {method} {url}: {exc}"
                    )
                    raise
                delay = _get_delay(backoff_factor, attempt)
                logger.warning(
                    f"sync_request attempt {attempt + 1}/{retries + 1} failed for {method} {url}: {exc}; retrying in {delay:.2f}s"
                )
                time.sleep(delay)
        raise last_exc  # pragma: no cover


__all__ = [
    "async_request",
    "sync_request",
    "DEFAULT_TIMEOUT",
    "DEFAULT_FOLLOW_REDIRECTS",
    "DEFAULT_MAX_REDIRECTS",
    "DEFAULT_MAX_RETRIES",
    "DEFAULT_BACKOFF_FACTOR",
    "DEFAULT_PROXY",
    "DEFAULT_USER_AGENT",
]
