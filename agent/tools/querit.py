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
import os
import re
import time
from abc import ABC
from typing import Any
from urllib.parse import urlparse

import requests

from agent.tools.base import ToolBase, ToolMeta, ToolParamBase
from common.connection_utils import timeout
from common.http_client import DEFAULT_TIMEOUT

QUERIT_SEARCH_URL = "https://api.querit.ai/v1/search"
QUERIT_CONTENTS_URL = "https://api.querit.ai/v1/contents"
QUERIT_MAX_ATTEMPTS = 3
QUERIT_RETRYABLE_STATUS_CODES = {429, 500, 502, 503, 504}
QUERIT_CONTENT_FORMATS = {"text", "markdown", "html"}
TIME_RANGE_PATTERN = re.compile(r"^([dwmy][1-9][0-9]*|\d{4}-\d{2}-\d{2}to\d{4}-\d{2}-\d{2})$")
logger = logging.getLogger(__name__)


class _QueritCanceled(Exception):
    pass


class QueritSearchParam(ToolParamBase):
    def __init__(self):
        self.meta: ToolMeta = {
            "name": "querit_search",
            "description": "Search the live web with Querit and return the complete Querit API response.",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search query to execute with Querit.",
                    "default": "{sys.query}",
                    "required": True,
                },
                "count": {
                    "type": "integer",
                    "description": "The maximum number of results to return. Defaults to 10.",
                    "default": 10,
                    "required": False,
                },
                "chunks_per_doc": {
                    "type": "integer",
                    "description": "The number of summary chunks per document. Supports values from 1 to 3.",
                    "default": 3,
                    "required": False,
                },
                "site_include": {
                    "type": "array",
                    "description": "Sites that search results must include.",
                    "default": [],
                    "items": {"type": "string"},
                    "required": False,
                },
                "site_exclude": {
                    "type": "array",
                    "description": "Sites that search results must exclude.",
                    "default": [],
                    "items": {"type": "string"},
                    "required": False,
                },
                "time_range": {
                    "type": "string",
                    "description": "A Querit time range such as d7, w1, m3, y1, or YYYY-MM-DDtoYYYY-MM-DD.",
                    "default": "",
                    "required": False,
                },
                "country_include": {
                    "type": "array",
                    "description": "Return results associated with the specified countries.",
                    "default": [],
                    "items": {"type": "string"},
                    "required": False,
                },
                "language_include": {
                    "type": "array",
                    "description": "Languages that search results must include.",
                    "default": [],
                    "items": {"type": "string"},
                    "required": False,
                },
            },
        }
        super().__init__()
        self.api_key = ""

    def check(self):
        _validate_search_inputs(
            count=self.count,
            chunks_per_doc=self.chunks_per_doc,
            time_range=self.time_range,
            site_include=self.site_include,
            site_exclude=self.site_exclude,
            country_include=self.country_include,
            language_include=self.language_include,
        )

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {"name": "Query", "type": "line"},
            "count": {"name": "Count", "type": "line"},
            "chunks_per_doc": {"name": "Chunks per document", "type": "line"},
            "site_include": {"name": "Include sites", "type": "line"},
            "site_exclude": {"name": "Exclude sites", "type": "line"},
            "time_range": {"name": "Time range", "type": "line"},
            "country_include": {"name": "Include countries", "type": "line"},
            "language_include": {"name": "Include languages", "type": "line"},
        }


class QueritSearch(ToolBase, ABC):
    component_name = "QueritSearch"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", "12")))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("QueritSearch processing"):
            return

        query = kwargs.get("query")
        if not isinstance(query, str):
            return self._fail("Querit query must be a string.")
        if not query:
            self.set_output("formalized_content", "")
            self.set_output("json", {})
            return ""

        node_api_key = (self._param.api_key or "").strip()
        api_key = node_api_key or (os.environ.get("QUERIT_API_KEY") or "").strip()
        if not api_key:
            return self._fail("Querit API key is required. Configure api_key or set QUERIT_API_KEY.")

        values = {
            name: kwargs[name] if name in kwargs else getattr(self._param, name)
            for name in (
                "count",
                "chunks_per_doc",
                "site_include",
                "site_exclude",
                "time_range",
                "country_include",
                "language_include",
            )
        }

        try:
            _validate_search_inputs(**values)
            payload = _build_payload(query, **values)
            response_data = self._search(payload, api_key)
            if not isinstance(response_data, dict):
                raise TypeError("Querit API response must be a JSON object.")

            result_container = response_data.get("results", {})
            if not isinstance(result_container, dict):
                raise TypeError("Querit API response field results must be an object.")
            results = result_container.get("result", [])
            if not isinstance(results, list):
                raise TypeError("Querit API response field results.result must be an array.")

            reference_results = [item for item in results if isinstance(item, dict)]
            if reference_results:
                self._retrieve_chunks(
                    reference_results,
                    get_title=lambda item: _querit_text(item.get("title")),
                    get_url=lambda item: _querit_text(item.get("url")),
                    get_content=lambda item: _querit_text(item.get("snippet")),
                    get_score=lambda _item: 1,
                )
            else:
                self.set_output("formalized_content", "")
            self.set_output("json", response_data)
            return self.output("formalized_content")
        except _QueritCanceled:
            return
        except (requests.RequestException, RuntimeError, TypeError, ValueError) as error:
            return self._fail(_safe_error_message(error, api_key))

    def _search(self, payload: dict[str, Any], api_key: str) -> Any:
        return _post_querit(self, QUERIT_SEARCH_URL, payload, api_key, "QueritSearch")

    def _wait_before_retry(self) -> None:
        if self.check_if_canceled("QueritSearch processing"):
            raise _QueritCanceled
        time.sleep(self._param.delay_after_error)

    def _fail(self, message: str) -> str:
        self.set_output("_ERROR", message)
        logger.error("Querit search failed: %s", message)
        return f"Querit error: {message}"

    def thoughts(self) -> str:
        return "Searching Querit for `{}`.".format(self.get_input().get("query", "-_-!"))


class QueritContentsParam(ToolParamBase):
    def __init__(self):
        self.meta: ToolMeta = {
            "name": "querit_contents",
            "description": "Crawl one or more web pages with Querit and return their contents.",
            "parameters": {
                "urls": {
                    "type": "array",
                    "description": "The absolute HTTP or HTTPS URLs to crawl. Supports 1 to 10 URLs.",
                    "default": [],
                    "items": {"type": "string"},
                    "required": True,
                },
                "format": {
                    "type": "string",
                    "description": "Content format: text, markdown, or html. Defaults to markdown.",
                    "enum": ["text", "markdown", "html"],
                    "default": "markdown",
                    "required": False,
                },
                "crawl_timeout": {
                    "type": "integer",
                    "description": "Per-page crawl timeout in seconds. Must be between 1 and 60.",
                    "default": 10,
                    "required": False,
                },
                "extras_meta": {
                    "type": "boolean",
                    "description": "Whether to include page metadata in each result.",
                    "default": False,
                    "required": False,
                },
            },
        }
        super().__init__()
        self.api_key = ""

    def check(self):
        self.urls = _normalize_contents_urls(self.urls)
        _validate_contents_inputs(self.urls, self.format, self.crawl_timeout, self.extras_meta)

    def get_input_form(self) -> dict[str, dict]:
        return {"urls": {"name": "URLs", "type": "line"}}


class QueritContents(ToolBase, ABC):
    component_name = "QueritContents"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", "70")))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("QueritContents processing"):
            return

        values = {name: kwargs[name] if name in kwargs else getattr(self._param, name) for name in ("urls", "format", "crawl_timeout", "extras_meta")}
        values["urls"] = _normalize_contents_urls(values["urls"])

        node_api_key = (self._param.api_key or "").strip()
        api_key = node_api_key or (os.environ.get("QUERIT_API_KEY") or "").strip()
        if not api_key:
            return self._fail("Querit API key is required. Configure api_key or set QUERIT_API_KEY.")

        try:
            _validate_contents_inputs(**values)
            response_data = self._request(_build_contents_payload(**values), api_key)
            _validate_contents_response(response_data)
            self.set_output("json", response_data)
            return self.output("json")
        except _QueritCanceled:
            return
        except (requests.RequestException, RuntimeError, TypeError, ValueError) as error:
            return self._fail(_safe_error_message(error, api_key))

    def _request(self, payload: dict[str, Any], api_key: str) -> Any:
        request_timeout = max(DEFAULT_TIMEOUT, payload["crawlTimeout"] + 5)
        return _post_querit(self, QUERIT_CONTENTS_URL, payload, api_key, "QueritContents", request_timeout)

    def _wait_before_retry(self) -> None:
        if self.check_if_canceled("QueritContents processing"):
            raise _QueritCanceled
        time.sleep(self._param.delay_after_error)

    def _fail(self, message: str) -> str:
        self.set_output("_ERROR", message)
        logger.error("Querit contents failed: %s", message)
        return f"Querit contents error: {message}"

    def thoughts(self) -> str:
        return "Reading web page contents with Querit."


def _build_payload(query: str, **values: Any) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "query": query,
        "count": values["count"],
    }
    if values["chunks_per_doc"] is not None:
        payload["chunksPerDoc"] = values["chunks_per_doc"]

    filters: dict[str, Any] = {}
    if values["site_include"] or values["site_exclude"]:
        filters["sites"] = {}
        if values["site_include"]:
            filters["sites"]["include"] = values["site_include"]
        if values["site_exclude"]:
            filters["sites"]["exclude"] = values["site_exclude"]
    if values["time_range"]:
        filters["timeRange"] = {"date": values["time_range"]}
    if values["country_include"]:
        filters["geo"] = {"countries": {"include": values["country_include"]}}
    if values["language_include"]:
        filters["languages"] = {"include": values["language_include"]}
    if filters:
        payload["filters"] = filters
    return payload


def _post_querit(tool: Any, endpoint: str, payload: dict[str, Any], api_key: str, operation: str, request_timeout: float = DEFAULT_TIMEOUT) -> Any:
    headers = {
        "Accept": "application/json",
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }
    for attempt in range(QUERIT_MAX_ATTEMPTS):
        if tool.check_if_canceled(f"{operation} processing"):
            raise _QueritCanceled
        try:
            response = requests.post(endpoint, headers=headers, json=payload, timeout=request_timeout)
            if response.status_code in QUERIT_RETRYABLE_STATUS_CODES and attempt + 1 < QUERIT_MAX_ATTEMPTS:
                tool._wait_before_retry()
                continue
            response.raise_for_status()
            return response.json()
        except requests.JSONDecodeError:
            raise
        except requests.HTTPError:
            raise
        except requests.RequestException:
            if attempt + 1 >= QUERIT_MAX_ATTEMPTS:
                raise
            tool._wait_before_retry()
    raise RuntimeError("Querit request failed after three attempts.")


def _build_contents_payload(urls: list[str], format: str, crawl_timeout: int, extras_meta: bool) -> dict[str, Any]:
    return {
        "urls": urls,
        "format": format,
        "crawlTimeout": crawl_timeout,
        "extrasMeta": extras_meta,
    }


def _normalize_contents_urls(urls: Any) -> Any:
    if isinstance(urls, str):
        return [url.strip() for url in urls.split(",") if url.strip()]
    return urls


def _validate_contents_inputs(urls: Any, format: Any, crawl_timeout: Any, extras_meta: Any) -> None:
    if not isinstance(urls, list) or not 1 <= len(urls) <= 10 or any(not isinstance(url, str) or not url.strip() for url in urls):
        raise ValueError("Querit urls must contain between 1 and 10 non-empty strings.")
    for url in urls:
        parsed = urlparse(url)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ValueError("Querit urls must be absolute HTTP or HTTPS URLs.")
    if format not in QUERIT_CONTENT_FORMATS:
        raise ValueError("Querit format must be text, markdown, or html.")
    if type(crawl_timeout) is not int or not 1 <= crawl_timeout <= 60:
        raise ValueError("Querit crawl_timeout must be an integer from 1 to 60.")
    if type(extras_meta) is not bool:
        raise ValueError("Querit extras_meta must be a boolean.")


def _validate_contents_response(response_data: Any) -> None:
    if not isinstance(response_data, dict):
        raise TypeError("Querit API response must be a JSON object.")
    if "results" in response_data and not isinstance(response_data["results"], list):
        raise TypeError("Querit API response field results must be an array.")
    if "statuses" in response_data and not isinstance(response_data["statuses"], list):
        raise TypeError("Querit API response field statuses must be an array.")


def _validate_search_inputs(
    count: Any,
    chunks_per_doc: Any,
    time_range: Any,
    site_include: Any,
    site_exclude: Any,
    country_include: Any,
    language_include: Any,
) -> None:
    if type(count) is not int or count < 1:
        raise ValueError("Querit count must be an integer greater than or equal to 1.")
    if chunks_per_doc is not None and (type(chunks_per_doc) is not int or not 1 <= chunks_per_doc <= 3):
        raise ValueError("Querit chunks_per_doc must be an integer from 1 to 3.")
    if type(time_range) is not str:
        raise ValueError("Querit time_range must be a string.")
    if time_range and not TIME_RANGE_PATTERN.fullmatch(time_range):
        raise ValueError("Querit time_range must use dN, wN, mN, yN, or YYYY-MM-DDtoYYYY-MM-DD.")
    for name, value in (
        ("site_include", site_include),
        ("site_exclude", site_exclude),
        ("country_include", country_include),
        ("language_include", language_include),
    ):
        if not isinstance(value, list) or any(not isinstance(item, str) for item in value):
            raise ValueError(f"Querit {name} must be an array of strings.")


def _safe_error_message(error: Exception, api_key: str) -> str:
    message = str(error) or error.__class__.__name__
    return message.replace(api_key, "[REDACTED]") if api_key else message


def _querit_text(value: Any) -> str:
    return "" if value is None else str(value)
