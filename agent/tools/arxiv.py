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
import arxiv
from agent.tools.base import ToolParamBase, ToolMeta, ToolBase
from common.connection_utils import timeout


class ArXivParam(ToolParamBase):
    """
    Define the ArXiv component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "arxiv_search",
            "description": """arXiv is a free distribution service and an open-access archive for nearly 2.4 million scholarly articles in the fields of physics, mathematics, computer science, quantitative biology, quantitative finance, statistics, electrical engineering and systems science, and economics. Materials on this site are not peer-reviewed by arXiv.""",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search keywords to execute with arXiv. The keywords should be the most important words/terms(includes synonyms) from the original request.",
                    "default": "{sys.query}",
                    "required": True
                }
            }
        }
        super().__init__()
        self.top_n = 12
        self.sort_by = 'submittedDate'

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")
        self.check_valid_value(self.sort_by, "ArXiv Search Sort_by",
                               ['submittedDate', 'lastUpdatedDate', 'relevance'])

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "name": "Query",
                "type": "line"
            }
        }


class ArXiv(ToolBase, ABC):
    component_name = "ArXiv"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("ArXiv processing"):
            return

        if not kwargs.get("query"):
            self.set_output("formalized_content", "")
            return ""

        last_e = ""
        for _ in range(self._param.max_retries+1):
            if self.check_if_canceled("ArXiv processing"):
                return

            try:
                sort_choices = {"relevance": arxiv.SortCriterion.Relevance,
                                "lastUpdatedDate": arxiv.SortCriterion.LastUpdatedDate,
                                'submittedDate': arxiv.SortCriterion.SubmittedDate}
                arxiv_client = arxiv.Client()
                search = arxiv.Search(
                    query=kwargs["query"],
                    max_results=self._param.top_n,
                    sort_by=sort_choices[self._param.sort_by]
                )
                results = list(arxiv_client.results(search))

                if self.check_if_canceled("ArXiv processing"):
                    return

                self._retrieve_chunks(results,
                                      get_title=lambda r: r.title,
                                      get_url=lambda r: r.pdf_url,
                                      get_content=lambda r: r.summary)
                return self.output("formalized_content")
            except Exception as e:
                if self.check_if_canceled("ArXiv processing"):
                    return

                last_e = e
                logging.exception(f"ArXiv error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"ArXiv error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return """
Keywords: {}
Looking for the most relevant articles.
                """.format(self.get_input().get("query", "-_-!"))
