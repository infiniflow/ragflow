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
from duckduckgo_search import DDGS
import pandas as pd
from agent.component.base import ComponentBase, ComponentParamBase


class DuckDuckGoParam(ComponentParamBase):
    """
    Define the DuckDuckGo component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 10
        self.channel = "text"

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")
        self.check_valid_value(self.channel, "Web Search or News", ["text", "news"])


class DuckDuckGo(ComponentBase, ABC):
    component_name = "DuckDuckGo"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return DuckDuckGo.be_output("")

        try:
            if self._param.channel == "text":
                with DDGS() as ddgs:
                    # {'title': '', 'href': '', 'body': ''}
                    duck_res = [{"content": '<a href="' + i["href"] + '">' + i["title"] + '</a>    ' + i["body"]} for i
                                in ddgs.text(ans, max_results=self._param.top_n)]
            elif self._param.channel == "news":
                with DDGS() as ddgs:
                    # {'date': '', 'title': '', 'body': '', 'url': '', 'image': '', 'source': ''}
                    duck_res = [{"content": '<a href="' + i["url"] + '">' + i["title"] + '</a>    ' + i["body"]} for i
                                in ddgs.news(ans, max_results=self._param.top_n)]
        except Exception as e:
            return DuckDuckGo.be_output("**ERROR**: " + str(e))

        if not duck_res:
            return DuckDuckGo.be_output("")

        df = pd.DataFrame(duck_res)
        logging.debug("df: {df}")
        return df
