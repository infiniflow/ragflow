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
from agent.tools.base import ToolMeta, ToolParamBase, ToolBase
from common.connection_utils import timeout


class AkShareParam(ToolParamBase):
    """
    Define the AkShare component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "akshare_stock_news",
            "description": "AkShare retrieves the latest news articles for a given Chinese A-share stock from East Money (东方财富).",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The stock symbol/code to fetch news for, e.g. '600519'.",
                    "default": "{sys.query}",
                    "required": True,
                }
            },
        }
        super().__init__()
        self.top_n = 10

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")

    def get_input_form(self) -> dict[str, dict]:
        return {"query": {"name": "Stock symbol", "type": "line"}}


class AkShare(ToolBase, ABC):
    component_name = "AkShare"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("AkShare processing"):
            return

        symbol = kwargs.get("query")
        if not symbol:
            self.set_output("formalized_content", "")
            return ""

        last_e = None
        for _ in range(self._param.max_retries + 1):
            if self.check_if_canceled("AkShare processing"):
                return

            try:
                import akshare as ak

                df = ak.stock_news_em(symbol=symbol).head(self._param.top_n)

                if self.check_if_canceled("AkShare processing"):
                    return

                items = ['<a href="{}">{}</a>\n 新闻内容: {} \n发布时间:{} \n文章来源: {}'.format(i["新闻链接"], i["新闻标题"], i["新闻内容"], i["发布时间"], i["文章来源"]) for _, i in df.iterrows()]
                res = "\n\n".join(items)
                self.set_output("formalized_content", res)
                return res
            except Exception as e:
                if self.check_if_canceled("AkShare processing"):
                    return

                last_e = e
                logging.exception(f"AkShare error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"AkShare error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return "Looking up the latest stock news for: {}".format(self.get_input().get("query", "-_-!"))
