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

import asyncio
import base64
import functools
import hashlib
import logging
import os
import subprocess
import sys
import threading
import uuid
from urllib.parse import urljoin

from concurrent.futures import ThreadPoolExecutor

logger = logging.getLogger(__name__)


def get_uuid():
    return uuid.uuid1().hex


# OAuth avatar fetch: bounded size; each redirect hop is SSRF-checked and DNS-pinned
# (see common.ssrf_guard).
_OAUTH_AVATAR_MAX_BYTES = int(os.environ.get("RAGFLOW_OAUTH_AVATAR_MAX_BYTES", str(5 * 1024 * 1024)))
_OAUTH_AVATAR_MAX_REDIRECTS = int(os.environ.get("RAGFLOW_OAUTH_AVATAR_MAX_REDIRECTS", "5"))
_REDIRECT_STATUS = frozenset({301, 302, 303, 307, 308})


async def download_img(url):
    """Fetch an image URL and return a data URI, or empty string on failure / SSRF block.

    URLs must resolve only to globally routable addresses; redirects are followed
    only up to ``_OAUTH_AVATAR_MAX_REDIRECTS`` with each target validated.
    """
    if not url:
        return ""
    if not isinstance(url, str):
        url = str(url)
    url = url.strip()
    if not url:
        return ""

    current_url = url
    redirect_hops = 0

    # Match common/http_client.py defaults without importing http_client (avoids
    # pulling settings and keeps this path usable in lightweight test envs).
    request_timeout = float(os.environ.get("HTTP_CLIENT_TIMEOUT", "15"))
    proxy = os.environ.get("HTTP_CLIENT_PROXY")
    user_agent = os.environ.get("HTTP_CLIENT_USER_AGENT", "ragflow-http-client")

    from common.ssrf_guard import assert_url_is_safe, pin_dns_global

    while redirect_hops <= _OAUTH_AVATAR_MAX_REDIRECTS:
        try:
            hostname, pin_ip = assert_url_is_safe(current_url)
        except ValueError as exc:
            logger.warning("download_img rejected URL (SSRF guard): %s", exc)
            return ""

        import httpx

        timeout = httpx.Timeout(request_timeout)
        headers = {}
        if user_agent:
            headers["User-Agent"] = user_agent

        async def _stream_one_get() -> tuple[str, str | None]:
            """Return ``('redirect', new_url)``, ``('data', data_uri)``, or ``('fail', None)``."""
            with pin_dns_global(hostname, pin_ip):
                async with httpx.AsyncClient(
                    timeout=timeout,
                    follow_redirects=False,
                    proxy=proxy,
                ) as client:
                    async with client.stream("GET", current_url, headers=headers or None) as response:
                        if response.status_code in _REDIRECT_STATUS:
                            await response.aclose()
                            location = response.headers.get("location")
                            if not location:
                                logger.warning(
                                    "download_img redirect missing Location header: url=%r status=%s redirect_hops=%s",
                                    current_url,
                                    response.status_code,
                                    redirect_hops,
                                )
                                return ("fail", None)
                            return ("redirect", urljoin(current_url, location))
                        if response.status_code != 200:
                            logger.warning(
                                "download_img non-200 response: url=%r status=%s redirect_hops=%s",
                                current_url,
                                response.status_code,
                                redirect_hops,
                            )
                            return ("fail", None)
                        body = bytearray()
                        async for chunk in response.aiter_bytes():
                            if len(body) + len(chunk) > _OAUTH_AVATAR_MAX_BYTES:
                                logger.warning(
                                    "download_img response exceeded max size: url=%r max_bytes=%s",
                                    current_url,
                                    _OAUTH_AVATAR_MAX_BYTES,
                                )
                                await response.aclose()
                                return ("fail", None)
                            body.extend(chunk)
                        content_type = response.headers.get("Content-Type", "image/jpeg")
                        data_uri = (
                            "data:"
                            + content_type
                            + ";base64,"
                            + base64.b64encode(bytes(body)).decode("utf-8")
                        )
                        return ("data", data_uri)

        try:
            kind, payload = await asyncio.wait_for(_stream_one_get(), timeout=request_timeout)
        except asyncio.TimeoutError:
            logger.warning(
                "download_img total wall-clock timeout: url=%r redirect_hops=%s timeout=%s",
                current_url,
                redirect_hops,
                request_timeout,
            )
            return ""
        except Exception as exc:
            logger.warning(
                "download_img request failed: url=%r redirect_hops=%s err=%s",
                current_url,
                redirect_hops,
                exc,
            )
            return ""

        if kind == "redirect":
            current_url = str(payload)
            redirect_hops += 1
            continue
        if kind == "fail":
            return ""
        return str(payload)

    logger.warning(
        "download_img redirect hop limit exceeded: url=%r redirect_hops=%s max_redirects=%s",
        current_url,
        redirect_hops,
        _OAUTH_AVATAR_MAX_REDIRECTS,
    )
    return ""


def hash_str2int(line: str, mod: int = 10 ** 8) -> int:
    return int(hashlib.sha1(line.encode("utf-8")).hexdigest(), 16) % mod

def convert_bytes(size_in_bytes: int) -> str:
    """
    Format size in bytes.
    """
    if size_in_bytes == 0:
        return "0 B"

    units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
    i = 0
    size = float(size_in_bytes)

    while size >= 1024 and i < len(units) - 1:
        size /= 1024
        i += 1

    if i == 0 or size >= 100:
        return f"{size:.0f} {units[i]}"
    elif size >= 10:
        return f"{size:.1f} {units[i]}"
    else:
        return f"{size:.2f} {units[i]}"


def once(func):
    """
    A thread-safe decorator that ensures the decorated function runs exactly once,
    caching and returning its result for all subsequent calls. This prevents
    race conditions in multi-thread environments by using a lock to protect
    the execution state.

    Args:
        func (callable): The function to be executed only once.

    Returns:
        callable: A wrapper function that executes `func` on the first call
                  and returns the cached result thereafter.

    Example:
        @once
        def compute_expensive_value():
            print("Computing...")
            return 42

        # First call: executes and prints
        # Subsequent calls: return 42 without executing
    """
    executed = False
    result = None
    lock = threading.Lock()
    def wrapper(*args, **kwargs):
        nonlocal executed, result
        with lock:
            if not executed:
                executed = True
                result = func(*args, **kwargs)
        return result
    return wrapper

@once
def pip_install_torch():
    device = os.getenv("DEVICE", "cpu")
    if device=="cpu":
        return
    logging.info("Installing pytorch")
    pkg_names = ["torch>=2.5.0,<3.0.0"]
    subprocess.check_call([sys.executable, "-m", "pip", "install", *pkg_names])


@once
def _thread_pool_executor():
    max_workers_env = os.getenv("THREAD_POOL_MAX_WORKERS", "128")
    try:
        max_workers = int(max_workers_env)
    except ValueError:
        max_workers = 128
    if max_workers < 1:
        max_workers = 1
    return ThreadPoolExecutor(max_workers=max_workers)


async def thread_pool_exec(func, *args, **kwargs):
    loop = asyncio.get_running_loop()
    if kwargs:
        func = functools.partial(func, *args, **kwargs)
        return await loop.run_in_executor(_thread_pool_executor(), func)
    return await loop.run_in_executor(_thread_pool_executor(), func, *args)
