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
import logging
import os
import time
from abc import ABC
from urllib.parse import urlsplit

import requests

from agent.tools.base import ToolBase, ToolMeta, ToolParamBase
from common.connection_utils import timeout


def _base_url() -> str:
    """Resolve the Keenable API base URL from ``KEENABLE_API_URL`` (HTTPS enforced)."""
    base = (os.environ.get("KEENABLE_API_URL") or "https://api.keenable.ai").rstrip("/")
    parsed = urlsplit(base)
    if parsed.hostname:
        if parsed.scheme == "https":
            return base
        # Permit plain http only against a loopback host (local dev).
        if parsed.scheme == "http" and parsed.hostname in {"localhost", "127.0.0.1", "::1"}:
            return base
    raise ValueError(f"KEENABLE_API_URL must be an https:// URL with a host, got {base!r}")


def _request(method: str, public_path: str, keyed_path: str, api_key: str, *, params=None, json=None, timeout_s: int = 30):
    """Call the keyed endpoint with X-API-Key when a key is set, else the keyless public one."""
    api_key = (api_key or "").strip()
    headers = {
        "User-Agent": "keenable-ragflow",
        # Attribution header the Keenable backend segments traffic by.
        "X-Keenable-Title": "RAGFlow",
    }
    if api_key:
        path = keyed_path
        headers["X-API-Key"] = api_key
    else:
        path = public_path
    resp = requests.request(method, f"{_base_url()}{path}", headers=headers, params=params, json=json, timeout=timeout_s)
    resp.raise_for_status()
    return resp.json()


class KeenableSearchParam(ToolParamBase):
    """
    Define the Keenable search component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "keenable_search",
            "description": """
Keenable is a web search API built for AI agents. It returns fresh, relevant web
results for a query and works without an API key by default (keyless free tier).
When searching:
   - Use a focused query of the most important terms (and synonyms).
   - Optionally restrict to a single site/domain.
             """,
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search keywords to execute with Keenable. The keywords should be the most important words/terms(includes synonyms) from the original request.",
                    "default": "{sys.query}",
                    "required": True,
                },
                "site": {
                    "type": "string",
                    "description": "default:''. Restrict results to a single domain, e.g. 'techcrunch.com'.",
                    "default": "",
                    "required": False,
                },
            },
        }
        super().__init__()
        # A key is optional: blank uses the keyless public endpoint (free tier);
        # setting one lifts rate limits and enables the 'realtime' mode.
        self.api_key = ""
        # "pro" (default, deeper) or "realtime" (low latency; requires a key).
        self.mode = "pro"
        self.top_n = 10

    def check(self):
        self.check_valid_value(self.mode, "Keenable search mode should be in 'pro/realtime'", ["pro", "realtime"])
        self.check_positive_integer(self.top_n, "Top N")
        # 'realtime' is not available on the keyless public endpoint, so reject
        # the invalid combination at config time instead of failing at runtime.
        if self.mode == "realtime" and not (self.api_key or "").strip():
            raise ValueError("Keenable 'realtime' mode requires an API key")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "name": "Query",
                "type": "line",
            },
            "site": {
                "name": "Site",
                "type": "line",
            },
        }


class KeenableSearch(ToolBase, ABC):
    component_name = "KeenableSearch"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("KeenableSearch processing"):
            return

        if not kwargs.get("query"):
            self.set_output("formalized_content", "")
            return ""

        payload = {"query": kwargs["query"], "mode": self._param.mode}
        if kwargs.get("site"):
            payload["site"] = kwargs["site"]

        logging.info(f"KeenableSearch: starting search (mode={self._param.mode}, keyed={bool((self._param.api_key or '').strip())})")
        last_e = None
        for _ in range(self._param.max_retries + 1):
            if self.check_if_canceled("KeenableSearch processing"):
                logging.info("KeenableSearch: cancelled before request")
                return

            try:
                data = _request("POST", "/v1/search/public", "/v1/search", self._param.api_key, json=payload)
                if self.check_if_canceled("KeenableSearch processing"):
                    logging.info("KeenableSearch: cancelled after request")
                    return

                results = (data.get("results") or [])[: self._param.top_n]
                self._retrieve_chunks(
                    results,
                    get_title=lambda r: r.get("title"),
                    get_url=lambda r: r.get("url"),
                    get_content=lambda r: r.get("description"),
                )
                self.set_output("json", results)
                logging.info(f"KeenableSearch: returned {len(results)} results")
                return self.output("formalized_content")
            except ValueError as e:
                # Config/local errors (e.g. invalid KEENABLE_API_URL) won't be
                # fixed by retrying, so fail fast instead of sleeping.
                if self.check_if_canceled("KeenableSearch processing"):
                    return
                last_e = e
                logging.exception(f"Keenable config error: {e}")
                break
            except Exception as e:
                if self.check_if_canceled("KeenableSearch processing"):
                    return

                last_e = e
                logging.exception(f"Keenable error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"Keenable error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return """
Keywords: {}
Looking for the most relevant articles.
                """.format(self.get_input().get("query", "-_-!"))
