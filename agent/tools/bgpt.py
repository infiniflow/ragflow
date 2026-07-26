#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

BGPT_SEARCH_URL = "https://bgpt.pro/api/mcp-search"


class BGPTParam(ToolParamBase):
    """Define the BGPT component parameters."""

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "bgpt_search",
            "description": (
                "BGPT searches scientific papers and returns structured evidence extracted from full-text studies: "
                "methods, sample sizes, results, limitations, conflicts of interest, data/code availability, "
                "study blind spots, quality scores, and falsification prompts. "
                "Useful when the agent must judge a scientific claim, not just find abstracts."
            ),
            "parameters": {
                "query": {
                    "type": "string",
                    "description": ("Natural-language scientific search query. Use the most important terms from the user's request."),
                    "default": "{sys.query}",
                    "required": True,
                }
            },
        }
        super().__init__()
        self.top_n = 10
        self.api_key = ""
        self.days_back = None

    def check(self):
        try:
            if isinstance(self.top_n, str):
                self.top_n = int(self.top_n.strip())
        except Exception:
            pass
        self.check_positive_integer(self.top_n, "Top N")
        if self.days_back not in (None, ""):
            try:
                self.days_back = int(self.days_back)
            except (TypeError, ValueError) as exc:
                raise ValueError("days_back must be an integer") from exc
            self.check_positive_integer(self.days_back, "Days back")

    def get_input_form(self) -> dict[str, dict]:
        return {"query": {"name": "Query", "type": "line"}}


class BGPT(ToolBase, ABC):
    component_name = "BGPT"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 30)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("BGPT processing"):
            return

        query = kwargs.get("query")
        if not query or not isinstance(query, str) or not query.strip():
            self.set_output("formalized_content", "")
            return ""

        payload = {"query": query.strip(), "num_results": self._param.top_n}
        if self._param.api_key:
            payload["api_key"] = self._param.api_key
        if self._param.days_back:
            payload["days_back"] = self._param.days_back

        last_e = ""
        for _ in range(self._param.max_retries + 1):
            if self.check_if_canceled("BGPT processing"):
                return

            try:
                response = requests.post(BGPT_SEARCH_URL, json=payload, timeout=25)
                response.raise_for_status()
                data = response.json()

                if not data or not isinstance(data, dict):
                    raise ValueError("Invalid response from BGPT")

                results = data.get("results", [])
                if not isinstance(results, list):
                    raise ValueError("Invalid results format from BGPT")

                if self.check_if_canceled("BGPT processing"):
                    return

                self._retrieve_chunks(
                    results,
                    get_title=lambda paper: paper.get("title") or "Untitled",
                    get_url=lambda paper: paper.get("url") or paper.get("doi") or "",
                    get_content=lambda paper: self._format_bgpt_paper(paper),
                )
                self.set_output("json", results)
                return self.output("formalized_content")

            except requests.HTTPError as e:
                # Non-retryable 4xx (e.g. 400/401/403/404) should fail fast
                # rather than wasting retries on bad requests or auth failures.
                status = e.response.status_code if e.response is not None else None
                if status is not None and 400 <= status < 500 and status != 429:
                    last_e = f"HTTP error: {e}"
                    logging.exception("BGPT non-retryable HTTP error: %s", e)
                    break
                if self.check_if_canceled("BGPT processing"):
                    return

                last_e = f"Network error: {e}"
                logging.exception("BGPT network error: %s", e)
                time.sleep(self._param.delay_after_error)
            except requests.RequestException as e:
                if self.check_if_canceled("BGPT processing"):
                    return

                last_e = f"Network error: {e}"
                logging.exception("BGPT network error: %s", e)
                time.sleep(self._param.delay_after_error)
            except Exception as e:
                if self.check_if_canceled("BGPT processing"):
                    return

                last_e = str(e)
                logging.exception("BGPT error: %s", e)
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", last_e)
            return f"BGPT error: {last_e}"

        assert False, self.output()

    def _format_bgpt_paper(self, paper: dict) -> str:
        def field(*names: str) -> str:
            for name in names:
                value = paper.get(name)
                if value is None:
                    continue
                if isinstance(value, (dict, list)):
                    value = str(value)
                text = str(value).strip()
                if text:
                    return text
            return "-"

        lines = [
            f"Title: {field('title')}",
            f"Authors: {field('authors')}",
            f"Journal: {field('journal')}",
            f"Year: {field('year')}",
            f"DOI: {field('doi')}",
            f"Abstract: {field('abstract')}",
            f"Methods: {field('methods_and_experimental_techniques', 'methods')}",
            f"Sample size / population: {field('sample_size_and_population_characteristics', 'sample_size_and_population')}",
            f"Results: {field('results_and_conclusions', 'results')}",
            f"Limitations: {field('paper_limitations_and_biases', 'limitations')}",
            f"Conflicts of interest: {field('conflict_of_interest_statements', 'conflict_of_interest')}",
            f"Data availability: {field('data_availability_statements', 'data_availability')}",
            f"Blind spots: {field('study_blindspots')}",
            f"How to falsify: {field('how_to_falsify')}",
        ]
        return "\n".join(lines)

    def thoughts(self) -> str:
        return "Searching BGPT for structured scientific evidence on `{}`.".format(self.get_input().get("query", "-_-!"))
