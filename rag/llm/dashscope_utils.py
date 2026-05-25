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

import dashscope

logger = logging.getLogger(__name__)
DASHSCOPE_CN_HOST, DASHSCOPE_INTL_HOST = "dashscope.aliyuncs.com", "dashscope-intl.aliyuncs.com"
DASHSCOPE_NATIVE_API_PATH = "/api/v1"
DASHSCOPE_CN_NATIVE_API_URL, DASHSCOPE_INTL_NATIVE_API_URL = f"https://{DASHSCOPE_CN_HOST}{DASHSCOPE_NATIVE_API_PATH}", f"https://{DASHSCOPE_INTL_HOST}{DASHSCOPE_NATIVE_API_PATH}"
_DASHSCOPE_TEXT_EMBEDDING_TYPES = {"document", "query"}
_dashscope_native_api_url_lock = threading.RLock()


def dashscope_base_url_for_log(base_url: str) -> str:
    """Log host/path only (no query string) so secrets in URLs are not printed."""
    return base_url.split("?", 1)[0].strip()[:256]


def dashscope_native_http_api_url(base_url: str | None) -> str | None:
    if not base_url:
        return None
    u = base_url.strip().rstrip("/")
    safe = dashscope_base_url_for_log(u)
    if u.endswith(DASHSCOPE_NATIVE_API_PATH):
        logger.debug("DashScope Tongyi-Qianwen embedding: using native API base as configured (%s)", safe)
        return u
    if DASHSCOPE_INTL_HOST in u:
        logger.info("DashScope Tongyi-Qianwen embedding: mapped configured base_url to intl native API (%s -> %s)", safe, DASHSCOPE_INTL_NATIVE_API_URL)
        return DASHSCOPE_INTL_NATIVE_API_URL
    if DASHSCOPE_CN_HOST in u:
        logger.info("DashScope Tongyi-Qianwen embedding: mapped configured base_url to CN native API (%s -> %s)", safe, DASHSCOPE_CN_NATIVE_API_URL)
        return DASHSCOPE_CN_NATIVE_API_URL
    logger.warning("DashScope Tongyi-Qianwen embedding: base_url is set but not recognized as a DashScope host; using SDK default endpoint (%s)", safe)
    return None


@contextmanager
def dashscope_native_api_url_scope(url: str | None):
    """Run a DashScope SDK call while holding the process-global endpoint lock."""
    with _dashscope_native_api_url_lock:
        if not url:
            yield
            return
        prev = getattr(dashscope, "base_http_api_url", None)
        dashscope.base_http_api_url = url
        try:
            yield
        finally:
            dashscope.base_http_api_url = prev


def dashscope_text_embedding_call(native_api_url: str | None, model_name: str, input_text, api_key: str, text_type: str):
    if text_type not in _DASHSCOPE_TEXT_EMBEDDING_TYPES:
        valid_types = ", ".join(sorted(_DASHSCOPE_TEXT_EMBEDDING_TYPES))
        raise ValueError(f"unsupported DashScope embedding text_type: {text_type}; expected one of: {valid_types}")
    with dashscope_native_api_url_scope(native_api_url):
        return dashscope.TextEmbedding.call(model=model_name, input=input_text, api_key=api_key, text_type=text_type)
