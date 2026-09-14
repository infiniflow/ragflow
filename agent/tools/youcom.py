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
import time
from abc import ABC

import requests

from agent.tools.base import ToolBase, ToolMeta, ToolParamBase
from common.connection_utils import timeout

# You.com serves the same response shape from two endpoints. The keyless one is
# rate-limited but needs no credentials; the keyed one lifts those limits. The
# keyless endpoint rejects an X-API-Key header, so the endpoint and the headers
# are always chosen together.
YOUCOM_SEARCH_URL = "https://api.you.com/v1/search"
YOUCOM_KEYLESS_SEARCH_URL = "https://api.you.com/v1/agents/search"
# Identifies RAGFlow to You.com. On the keyless endpoint there is no key to
# attribute traffic to, so this is the only signal available.
YOUCOM_USER_AGENT = "RAGFlow youdotcom-integration/infiniflow-ragflow"
# You.com clamps `count` to 1-100.
YOUCOM_MAX_COUNT = 100
# You.com accepts a fixed set of recency windows. `any` is the no-restriction
# default and is never forwarded; the rest go straight into the request.
YOUCOM_FRESHNESS_ANY = "any"
YOUCOM_FRESHNESS_VALUES = [YOUCOM_FRESHNESS_ANY, "day", "week", "month", "year"]


def _search(api_key: str, params: dict, timeout_s: int = 30) -> dict:
    """GET the keyed endpoint when a key is set, else the keyless one."""
    api_key = (api_key or "").strip()
    headers = {"Accept": "application/json", "User-Agent": YOUCOM_USER_AGENT}
    if api_key:
        url = YOUCOM_SEARCH_URL
        headers["X-API-Key"] = api_key
    else:
        url = YOUCOM_KEYLESS_SEARCH_URL

    response = requests.get(url, headers=headers, params=params, timeout=timeout_s)
    response.raise_for_status()
    return response.json()


def _freshness(value) -> str:
    """Normalize a freshness value to a You.com window, or "" for no limit.

    Blank and `any` both mean no restriction. Anything else has to be one of the
    published windows, because the value is forwarded to You.com as-is.
    """
    freshness = str(value or "").strip().lower()
    if not freshness or freshness == YOUCOM_FRESHNESS_ANY:
        return ""
    if freshness not in YOUCOM_FRESHNESS_VALUES:
        raise ValueError("Freshness {} is not supported, it should be in {}".format(freshness, YOUCOM_FRESHNESS_VALUES))
    return freshness


def _result_content(result: dict) -> str:
    """Prefer the extracted page passages, falling back to the description.

    Web hits carry several `snippets` taken from the page body; news hits carry
    only a `description`.
    """
    snippets = result.get("snippets")
    if isinstance(snippets, list):
        joined = " ".join(" ".join(str(s).split()) for s in snippets if str(s).strip())
        if joined:
            return joined
    return " ".join(str(result.get("description") or "").split())


def _merge_sections(results: dict, top_n: int) -> list[dict]:
    """Web results lead, then news.

    `count` applies per section, so the two together can exceed it; the merged
    list is trimmed back to what the caller asked for.
    """
    if not isinstance(results, dict):
        return []
    merged = []
    for section in ("web", "news"):
        section_results = results.get(section)
        if isinstance(section_results, list):
            merged.extend(r for r in section_results if isinstance(r, dict))
    return merged[:top_n]


class YouComSearchParam(ToolParamBase):
    """
    Define the You.com search component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "youcom_search",
            "description": """
You.com runs its own web index and returns several extracted passages per result
rather than a single snippet, so results carry usable context without a separate
fetch. It works without an API key by default (keyless free tier).
When searching:
   - Use a focused query of the most important terms (and synonyms).
   - Optionally restrict results by how recently they were published.
             """,
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search keywords to execute with You.com. The keywords should be the most important words/terms(includes synonyms) from the original request.",
                    "default": "{sys.query}",
                    "required": True,
                },
                "freshness": {
                    "type": "string",
                    "description": "default:'any'. Restrict results by recency. One of 'day', 'week', 'month', 'year', or 'any' for no limit.",
                    "enum": YOUCOM_FRESHNESS_VALUES,
                    "default": YOUCOM_FRESHNESS_ANY,
                    "required": False,
                },
            },
        }
        super().__init__()
        # A key is optional: blank uses the keyless endpoint (free tier);
        # setting one lifts rate limits.
        self.api_key = ""
        self.top_n = 10

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "name": "Query",
                "type": "line",
            },
            "freshness": {
                "name": "Freshness",
                "type": "options",
                "value": YOUCOM_FRESHNESS_ANY,
                "options": YOUCOM_FRESHNESS_VALUES,
            },
        }


class YouComSearch(ToolBase, ABC):
    component_name = "YouComSearch"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("YouComSearch processing"):
            return

        if not kwargs.get("query"):
            self.set_output("formalized_content", "")
            return ""

        keyed = bool((self._param.api_key or "").strip())
        params = {
            "query": kwargs["query"],
            "count": min(max(1, int(self._param.top_n)), YOUCOM_MAX_COUNT),
        }
        freshness = _freshness(kwargs.get("freshness"))
        if freshness:
            params["freshness"] = freshness

        logging.info(f"YouComSearch: starting search (keyed={keyed})")
        last_e = None
        attempts = self._param.max_retries + 1
        for attempt in range(attempts):
            if self.check_if_canceled("YouComSearch processing"):
                logging.info("YouComSearch: cancelled before request")
                return

            try:
                data = _search(self._param.api_key, params)
                if self.check_if_canceled("YouComSearch processing"):
                    logging.info("YouComSearch: cancelled after request")
                    return

                results = _merge_sections(data.get("results") or {}, self._param.top_n)
                self._retrieve_chunks(
                    results,
                    get_title=lambda r: r.get("title"),
                    get_url=lambda r: r.get("url"),
                    get_content=_result_content,
                )
                self.set_output("json", results)
                logging.info(f"YouComSearch: returned {len(results)} results")
                return self.output("formalized_content")
            except Exception as e:
                if self.check_if_canceled("YouComSearch processing"):
                    return

                # Only the exception type is recorded: the query is a URL
                # parameter here, so the requests error message contains it.
                last_e = e
                logging.error(f"You.com error: {type(e).__name__}")
                if attempt < attempts - 1:
                    time.sleep(self._param.delay_after_error)

        if last_e:
            return f"You.com error: {type(last_e).__name__}"

        assert False, self.output()

    def thoughts(self) -> str:
        return """
Keywords: {}
Looking for the most relevant articles.
                """.format(self.get_input().get("query", "-_-!"))
