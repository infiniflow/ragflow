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
from abc import ABC
import pandas as pd
import requests
from agent.component.base import ComponentBase, ComponentParamBase


class GitHubParam(ComponentParamBase):
    """
    Define the GitHub component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 10

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")


class GitHub(ComponentBase, ABC):
    component_name = "GitHub"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return GitHub.be_output("")

        try:
            url = 'https://api.github.com/search/repositories?q=' + ans + '&sort=stars&order=desc&per_page=' + str(
                self._param.top_n)
            headers = {"Content-Type": "application/vnd.github+json", "X-GitHub-Api-Version": '2022-11-28'}
            response = requests.get(url=url, headers=headers).json()

            github_res = [{"content": '<a href="' + i["html_url"] + '">' + i["name"] + '</a>' + str(
                i["description"]) + '\n stars:' + str(i['watchers'])} for i in response['items']]
        except Exception as e:
            return GitHub.be_output("**ERROR**: " + str(e))

        if not github_res:
            return GitHub.be_output("")

        df = pd.DataFrame(github_res)
        logging.debug(f"df: {df}")
        return df
