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
from agent.tools.base import ToolParamBase, ToolMeta, ToolBase
from api.utils.api_utils import timeout


class GitHubParam(ToolParamBase):
    """
    Define the GitHub component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "github_search",
            "description": """GitHub repository search is a feature that enables users to find specific repositories on the GitHub platform. This search functionality allows users to locate projects, codebases, and other content hosted on GitHub based on various criteria.""",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search keywords to execute with GitHub. The keywords should be the most important words/terms(includes synonyms) from the original request.",
                    "default": "{sys.query}",
                    "required": True
                }
            }
        }
        super().__init__()
        self.top_n = 10

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "name": "Query",
                "type": "line"
            }
        }

class GitHub(ToolBase, ABC):
    component_name = "GitHub"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12))
    def _invoke(self, **kwargs):
        if not kwargs.get("query"):
            self.set_output("formalized_content", "")
            return ""

        last_e = ""
        for _ in range(self._param.max_retries+1):
            try:
                url = 'https://api.github.com/search/repositories?q=' + kwargs["query"] + '&sort=stars&order=desc&per_page=' + str(
                    self._param.top_n)
                headers = {"Content-Type": "application/vnd.github+json", "X-GitHub-Api-Version": '2022-11-28'}
                response = requests.get(url=url, headers=headers).json()
                self._retrieve_chunks(response['items'],
                                      get_title=lambda r: r["name"],
                                      get_url=lambda r: r["html_url"],
                                      get_content=lambda r: str(r["description"]) + '\n stars:' + str(r['watchers']))
                self.set_output("json", response['items'])
                return self.output("formalized_content")
            except Exception as e:
                last_e = e
                logging.exception(f"GitHub error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"GitHub error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return "Scanning GitHub repos related to `{}`.".format(self.get_input().get("query", "-_-!"))