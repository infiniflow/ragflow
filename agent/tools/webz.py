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

WEBZ_NEWS_SEARCH_URL = "https://api.webz.io/newsApiLite"
WEBZ_USER_AGENT = "RAGFlow webz-integration/infiniflow-ragflow"
WEBZ_MAX_COUNT = 10


def _search(api_key: str, params: dict, timeout_s: int = 30) -> dict:
    """GET Webz.io news API endpoint."""
    api_key = (api_key or os.environ.get("WEBZ_API_KEY", "")).strip()
    if not api_key:
        raise ValueError("Webz.io API token is required. Please set api_key or WEBZ_API_KEY environment variable.")

    headers = {"Accept": "application/json", "User-Agent": WEBZ_USER_AGENT}

    query_params = {
        "token": api_key,
        "q": params.get("query", ""),
        "size": params.get("size", 10),
    }

    response = requests.get(WEBZ_NEWS_SEARCH_URL, headers=headers, params=query_params, timeout=timeout_s)
    response.raise_for_status()
    return response.json()


def _result_content(post: dict) -> str:
    """Extract snippet/text content from Webz.io post dictionary."""
    text = post.get("text") or post.get("highlightText") or ""
    return " ".join(str(text).split())


class WebzSearchParam(ToolParamBase):
    """
    Define the Webz.io search component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "webz_search",
            "description": """
Webz.io provides live and historical news search across thousands of global publishers,
returning structured articles, full text, sentiment, and rich metadata.
When searching:
   - Use focused keywords or Boolean search queries.
   - Ideal for news monitoring, financial intelligence, and media research.
             """,
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search keywords or query to execute with Webz.io News API.",
                    "default": "{sys.query}",
                    "required": True,
                },
            },
        }
        super().__init__()
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
        }


class WebzSearch(ToolBase, ABC):
    component_name = "WebzSearch"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("WebzSearch processing"):
            return

        if not kwargs.get("query"):
            self.set_output("formalized_content", "")
            return ""

        params = {
            "query": kwargs["query"],
            "size": min(max(1, int(self._param.top_n)), WEBZ_MAX_COUNT),
        }

        logging.info(f"WebzSearch: starting news search query={params['query']}")
        last_e = None
        attempts = self._param.max_retries + 1
        for attempt in range(attempts):
            if self.check_if_canceled("WebzSearch processing"):
                logging.info("WebzSearch: cancelled before request")
                return

            try:
                data = _search(self._param.api_key, params)
                if self.check_if_canceled("WebzSearch processing"):
                    logging.info("WebzSearch: cancelled after request")
                    return

                if not isinstance(data, dict):
                    data = {}
                raw_posts = data.get("posts")
                posts = [p for p in raw_posts if isinstance(p, dict)] if isinstance(raw_posts, list) else []
                results = posts[: self._param.top_n]

                self._retrieve_chunks(
                    results,
                    get_title=lambda r: r.get("title") or r.get("thread", {}).get("title"),
                    get_url=lambda r: r.get("url") or r.get("thread", {}).get("url"),
                    get_content=_result_content,
                )
                self.set_output("json", results)
                logging.info(f"WebzSearch: returned {len(results)} results")
                return self.output("formalized_content")
            except Exception as e:
                if self.check_if_canceled("WebzSearch processing"):
                    return

                last_e = e
                logging.error(f"Webz.io error: {type(e).__name__}")
                # Do not retry non-transient validation errors or client 4xx (except rate-limit 429)
                if isinstance(e, ValueError) or (
                    isinstance(e, requests.HTTPError)
                    and e.response is not None
                    and 400 <= e.response.status_code < 500
                    and e.response.status_code != 429
                ):
                    break
                if attempt < attempts - 1:
                    time.sleep(self._param.delay_after_error)

        if last_e:
            return f"Webz.io error: {type(last_e).__name__}"

        assert False, self.output()

    def thoughts(self) -> str:
        return """
Keywords: {}
Looking for the most relevant news articles via Webz.io.
                """.format(self.get_input().get("query", "-_-!"))
