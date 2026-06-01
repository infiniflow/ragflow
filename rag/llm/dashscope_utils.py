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

import logging
import threading
from contextlib import contextmanager
from urllib.parse import urlparse, urlunparse

import dashscope

logger = logging.getLogger(__name__)
DASHSCOPE_CN_HOST, DASHSCOPE_INTL_HOST = "dashscope.aliyuncs.com", "dashscope-intl.aliyuncs.com"
DASHSCOPE_NATIVE_API_PATH = "/api/v1"
DASHSCOPE_CN_NATIVE_API_URL, DASHSCOPE_INTL_NATIVE_API_URL = f"https://{DASHSCOPE_CN_HOST}{DASHSCOPE_NATIVE_API_PATH}", f"https://{DASHSCOPE_INTL_HOST}{DASHSCOPE_NATIVE_API_PATH}"
_DASHSCOPE_TEXT_EMBEDDING_TYPES = {"document", "query"}
_dashscope_native_api_url_lock = threading.RLock()


def dashscope_base_url_for_log(base_url: str) -> str:
    """Log host/path only (no query string) so secrets in URLs are not printed."""
    raw = base_url.strip().rstrip("/")
    parsed = urlparse(raw if "://" in raw else f"https://{raw}")
    if not parsed.hostname:
        return raw.split("?", 1)[0].split("#", 1)[0][:256]
    netloc = parsed.hostname.lower().rstrip(".")
    try:
        if parsed.port:
            netloc = f"{netloc}:{parsed.port}"
    except ValueError:
        pass
    return urlunparse((parsed.scheme or "https", netloc, parsed.path.rstrip("/"), "", "", ""))[:256]


def _dashscope_host_matches(hostname: str | None, expected: str) -> bool:
    if not hostname:
        return False
    hostname = hostname.lower().rstrip(".")
    expected = expected.lower()
    return hostname == expected or hostname.endswith(f".{expected}")


def _dashscope_native_api_url_from_parts(parsed) -> str | None:
    if not parsed.hostname:
        return None
    netloc = parsed.hostname.lower().rstrip(".")
    try:
        if parsed.port:
            netloc = f"{netloc}:{parsed.port}"
    except ValueError:
        pass
    return urlunparse((parsed.scheme or "https", netloc, DASHSCOPE_NATIVE_API_PATH, "", "", ""))


def _dashscope_endpoint_for_log(url: str | None) -> str | None:
    return dashscope_base_url_for_log(str(url)) if url else None


def dashscope_native_http_api_url(base_url: str | None) -> str | None:
    if not base_url:
        return None
    u = base_url.strip().rstrip("/")
    parsed = urlparse(u if "://" in u else f"https://{u}")
    safe = dashscope_base_url_for_log(u)
    if parsed.path.rstrip("/") == DASHSCOPE_NATIVE_API_PATH:
        native_url = _dashscope_native_api_url_from_parts(parsed)
        if native_url:
            logger.debug("DashScope Tongyi-Qianwen embedding: using native API base as configured (%s)", dashscope_base_url_for_log(native_url))
            return native_url
    if _dashscope_host_matches(parsed.hostname, DASHSCOPE_INTL_HOST):
        logger.info("DashScope Tongyi-Qianwen embedding: mapped configured base_url to intl native API (%s -> %s)", safe, DASHSCOPE_INTL_NATIVE_API_URL)
        return DASHSCOPE_INTL_NATIVE_API_URL
    if _dashscope_host_matches(parsed.hostname, DASHSCOPE_CN_HOST):
        logger.info("DashScope Tongyi-Qianwen embedding: mapped configured base_url to CN native API (%s -> %s)", safe, DASHSCOPE_CN_NATIVE_API_URL)
        return DASHSCOPE_CN_NATIVE_API_URL
    logger.warning("DashScope Tongyi-Qianwen embedding: base_url is set but not recognized as a DashScope host; using SDK default endpoint (%s)", safe)
    return None


@contextmanager
def dashscope_native_api_url_scope(url: str | None):
    """Run a DashScope SDK call while holding the process-global endpoint lock."""
    with _dashscope_native_api_url_lock:
        prev = getattr(dashscope, "base_http_api_url", None)
        logger.debug("DashScope native endpoint scope acquired: requested=%s previous=%s", _dashscope_endpoint_for_log(url), _dashscope_endpoint_for_log(prev))
        if not url:
            try:
                yield
            finally:
                current = getattr(dashscope, "base_http_api_url", None)
                logger.debug("DashScope native endpoint scope released without override: current=%s", _dashscope_endpoint_for_log(current))
            return
        dashscope.base_http_api_url = url
        logger.debug("DashScope native endpoint override set: current=%s", _dashscope_endpoint_for_log(url))
        try:
            yield
        finally:
            current = getattr(dashscope, "base_http_api_url", None)
            logger.debug("DashScope native endpoint override restoring: current=%s previous=%s", _dashscope_endpoint_for_log(current), _dashscope_endpoint_for_log(prev))
            dashscope.base_http_api_url = prev
            logger.debug("DashScope native endpoint override restored: current=%s", _dashscope_endpoint_for_log(getattr(dashscope, "base_http_api_url", None)))


def dashscope_text_embedding_call(native_api_url: str | None, model_name: str, input_text, api_key: str, text_type: str):
    if text_type not in _DASHSCOPE_TEXT_EMBEDDING_TYPES:
        valid_types = ", ".join(sorted(_DASHSCOPE_TEXT_EMBEDDING_TYPES))
        raise ValueError(f"unsupported DashScope embedding text_type: {text_type}; expected one of: {valid_types}")
    with dashscope_native_api_url_scope(native_api_url):
        return dashscope.TextEmbedding.call(model=model_name, input=input_text, api_key=api_key, text_type=text_type)
