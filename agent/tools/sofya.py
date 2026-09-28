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

SOFYA_SEARCH_URL = "https://sofya.co/v1/search"
# Sofya returns at most 20 results per search.
SOFYA_MAX_RESULTS = 20
# "basic" fetches the result pages and returns their content; "snippets"
# returns the search snippets only, which is faster and cheaper.
SOFYA_SEARCH_DEPTHS = ["basic", "snippets"]
SOFYA_DEFAULT_SEARCH_DEPTH = "basic"
SOFYA_TOPICS = ["general", "news"]
SOFYA_DEFAULT_TOPIC = "general"
# Sofya also accepts a date range for freshness. Only the named windows are
# offered here, so the value can be checked against a fixed list.
SOFYA_FRESHNESS_ANY = "any"
SOFYA_FRESHNESS_VALUES = [SOFYA_FRESHNESS_ANY, "day", "week", "month", "year"]
# Statuses worth another attempt. Anything else (bad request, bad key, no
# credits) will fail the same way again, so it is returned at once.
SOFYA_RETRY_STATUSES = {429, 500, 502, 503, 504}


def _search(api_key: str, payload: dict, timeout_s: int = 30) -> dict:
    """POST a search to Sofya and return the decoded body."""
    headers = {
        "Authorization": f"Bearer {(api_key or '').strip()}",
        "Content-Type": "application/json",
        "Accept": "application/json",
        # Identifies RAGFlow to Sofya.
        "User-Agent": "RAGFlow sofya-integration/infiniflow-ragflow",
    }
    response = requests.post(SOFYA_SEARCH_URL, headers=headers, json=payload, timeout=timeout_s)
    response.raise_for_status()
    return response.json()


def _is_transient(error: Exception) -> bool:
    """Whether a failed request might succeed if repeated."""
    if isinstance(error, requests.HTTPError):
        response = getattr(error, "response", None)
        return getattr(response, "status_code", None) in SOFYA_RETRY_STATUSES
    return isinstance(error, requests.RequestException)


def _search_depth(value) -> str:
    """Normalize the node's search depth.

    This one is set in the node config rather than by the caller, so an unknown
    value falls back to the default instead of failing the run.
    """
    depth = str(value or "").strip().lower()
    if depth in SOFYA_SEARCH_DEPTHS:
        return depth
    if depth:
        logging.warning(f"Sofya search depth {depth} is not supported, using {SOFYA_DEFAULT_SEARCH_DEPTH}")
    return SOFYA_DEFAULT_SEARCH_DEPTH


def _topic(value) -> str:
    """Normalize a topic. Blank means the default, anything else must be known."""
    topic = str(value or "").strip().lower()
    if not topic:
        return SOFYA_DEFAULT_TOPIC
    if topic not in SOFYA_TOPICS:
        raise ValueError("Topic {} is not supported, it should be in {}".format(topic, SOFYA_TOPICS))
    return topic


def _freshness(value) -> str:
    """Normalize a freshness value to a Sofya window, or "" for no limit.

    Blank and `any` both mean no restriction. Anything else has to be one of the
    named windows, because the value is forwarded to Sofya as-is.
    """
    freshness = str(value or "").strip().lower()
    if not freshness or freshness == SOFYA_FRESHNESS_ANY:
        return ""
    if freshness not in SOFYA_FRESHNESS_VALUES:
        raise ValueError("Freshness {} is not supported, it should be in {}".format(freshness, SOFYA_FRESHNESS_VALUES))
    return freshness


def _max_results(value, fallback) -> int:
    """Result count: the caller's if it gave one, else the node's Top N.

    A blank or unparseable caller value is treated as "not given" so a bad
    argument falls back rather than failing the run.
    """
    for candidate in (value, fallback):
        try:
            count = int(candidate)
        except (TypeError, ValueError):
            continue
        return min(max(1, count), SOFYA_MAX_RESULTS)
    return 1


def _result_content(result: dict) -> str:
    """Prefer the page content, falling back to the search snippet.

    `content` carries the text of the result page and is empty when the page
    was not read; `description` is the search snippet. Either can be missing, so
    a result with no text at all returns "" and is dropped rather than stored as
    a blank chunk.
    """
    content = " ".join(str(result.get("content") or "").split())
    if content:
        return content
    return " ".join(str(result.get("description") or "").split())


def _results(data, top_n: int) -> list[dict]:
    """Pull the result list out of the response body."""
    if not isinstance(data, dict):
        return []
    results = data.get("results")
    if not isinstance(results, list):
        return []
    return [r for r in results if isinstance(r, dict)][:top_n]


class SofyaSearchParam(ToolParamBase):
    """
    Define the Sofya search component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "sofya_search",
            "description": """
Sofya is a web search API for AI agents. It returns the content of the result
pages, not just a snippet, so results carry usable context without a separate
fetch. An API key is required.
When searching:
   - Use a focused query of the most important terms (and synonyms).
   - Use the news topic for current events, and the general topic otherwise.
   - Optionally restrict results by how recently they were published.
             """,
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search keywords to execute with Sofya. The keywords should be the most important words/terms(includes synonyms) from the original request.",
                    "default": "{sys.query}",
                    "required": True,
                },
                "topic": {
                    "type": "string",
                    "description": "default:'general'. The category of the search. 'news' searches news sources and suits current events; 'general' is a broader web search.",
                    "enum": SOFYA_TOPICS,
                    "default": SOFYA_DEFAULT_TOPIC,
                    "required": False,
                },
                "freshness": {
                    "type": "string",
                    "description": "default:'any'. Restrict results by recency. One of 'day', 'week', 'month', 'year', or 'any' for no limit.",
                    "enum": SOFYA_FRESHNESS_VALUES,
                    "default": SOFYA_FRESHNESS_ANY,
                    "required": False,
                },
                "max_results": {
                    "type": "integer",
                    "description": "Number of results to return, 1 to 20. Leave it out to use the Top N set on the node.",
                    "default": "",
                    "required": False,
                },
            },
        }
        super().__init__()
        self.api_key = ""
        self.search_depth = SOFYA_DEFAULT_SEARCH_DEPTH
        self.top_n = 10

    def check(self):
        self.check_empty(self.api_key, "Sofya API key")
        self.check_valid_value(self.search_depth, "Sofya search depth should be in 'basic/snippets'", ["basic", "snippets"])
        self.check_positive_integer(self.top_n, "Top N")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "name": "Query",
                "type": "line",
            },
            "topic": {
                "name": "Topic",
                "type": "options",
                "value": SOFYA_DEFAULT_TOPIC,
                "options": SOFYA_TOPICS,
            },
            "freshness": {
                "name": "Freshness",
                "type": "options",
                "value": SOFYA_FRESHNESS_ANY,
                "options": SOFYA_FRESHNESS_VALUES,
            },
        }


class SofyaSearch(ToolBase, ABC):
    component_name = "SofyaSearch"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("SofyaSearch processing"):
            return

        if not kwargs.get("query"):
            self.set_output("formalized_content", "")
            return ""

        search_depth = _search_depth(self._param.search_depth)
        top_n = _max_results(kwargs.get("max_results"), self._param.top_n)
        payload = {
            "query": kwargs["query"],
            "search_depth": search_depth,
            "max_results": top_n,
            "topic": _topic(kwargs.get("topic")),
        }
        freshness = _freshness(kwargs.get("freshness"))
        if freshness:
            payload["freshness"] = freshness

        logging.info(f"SofyaSearch: starting search (search_depth={search_depth}, max_results={top_n})")
        last_e = None
        attempts = self._param.max_retries + 1
        for attempt in range(attempts):
            if self.check_if_canceled("SofyaSearch processing"):
                logging.info("SofyaSearch: cancelled before request")
                return

            try:
                data = _search(self._param.api_key, payload)
                if self.check_if_canceled("SofyaSearch processing"):
                    logging.info("SofyaSearch: cancelled after request")
                    return

                results = _results(data, top_n)
                self._retrieve_chunks(
                    results,
                    get_title=lambda r: str(r.get("title") or ""),
                    get_url=lambda r: str(r.get("url") or ""),
                    get_content=_result_content,
                )
                self.set_output("json", results)
                logging.info(f"SofyaSearch: returned {len(results)} results")
                return self.output("formalized_content")
            except Exception as e:
                if self.check_if_canceled("SofyaSearch processing"):
                    return

                # Only the exception type is recorded, so the query and the key
                # stay out of the log whatever the error message holds.
                last_e = e
                logging.error(f"Sofya error: {type(e).__name__}")
                if not _is_transient(e):
                    break
                if attempt < attempts - 1:
                    time.sleep(self._param.delay_after_error)

        if last_e:
            return f"Sofya error: {type(last_e).__name__}"

        assert False, self.output()

    def thoughts(self) -> str:
        return """
Keywords: {}
Looking for the most relevant articles.
                """.format(self.get_input().get("query", "-_-!"))
