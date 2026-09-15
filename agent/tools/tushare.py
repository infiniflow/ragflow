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
import json
import logging
import os
from abc import ABC
import pandas as pd
import time
import requests
from agent.tools.base import ToolMeta, ToolParamBase, ToolBase
from common.connection_utils import timeout
from common.http_client import DEFAULT_TIMEOUT


class TuShareParam(ToolParamBase):
    """
    Define the TuShare component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "tushare_news",
            "description": "TuShare retrieves financial quick news from mainstream Chinese financial portals and filters it by keyword.",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "Keyword used to filter the returned news items by content.",
                    "default": "{sys.query}",
                    "required": True,
                }
            },
        }
        super().__init__()
        self.token = "xxx"
        self.src = "eastmoney"
        self.start_date = "2024-01-01 09:00:00"
        self.end_date = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime())
        self.keyword = ""

    def check(self):
        self.check_valid_value(self.src, "Quick News Source", ["sina", "wallstreetcn", "10jqka", "eastmoney", "yuncaijing", "fenghuang", "jinrongjie"])

    def get_input_form(self) -> dict[str, dict]:
        return {"query": {"name": "Keyword", "type": "line"}}


class TuShare(ToolBase, ABC):
    component_name = "TuShare"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("TuShare processing"):
            return

        query = kwargs.get("query")
        if not query:
            self.set_output("formalized_content", "")
            return ""

        try:
            params = {"api_name": "news", "token": self._param.token, "params": {"src": self._param.src, "start_date": self._param.start_date, "end_date": self._param.end_date}}
            response = requests.post(url="https://api.tushare.pro", data=json.dumps(params).encode("utf-8"), timeout=DEFAULT_TIMEOUT)
            response = response.json()

            if self.check_if_canceled("TuShare processing"):
                return

            if response["code"] != 0:
                msg = response["msg"]
                self.set_output("_ERROR", msg)
                return msg

            # Pass the field names to the constructor rather than assigning
            # `df.columns` afterwards: when TuShare returns no rows the frame
            # has zero columns and the assignment raises a length mismatch.
            df = pd.DataFrame(response["data"]["items"], columns=response["data"]["fields"])

            if self.check_if_canceled("TuShare processing"):
                return

            keyword = self._param.keyword or query
            logging.info(
                "TuShare news filter keyword source=%s",
                "param.keyword" if self._param.keyword else "tool_query",
            )
            res = (df[df["content"].str.contains(keyword, case=False, na=False, regex=False)]).to_markdown()
            self.set_output("formalized_content", res)
            return res
        except Exception as e:
            if self.check_if_canceled("TuShare processing"):
                return

            logging.exception(f"TuShare error: {e}")
            msg = f"TuShare error: {e}"
            self.set_output("_ERROR", msg)
            return msg

    def thoughts(self) -> str:
        return "Looking up {} financial news for: {}".format(self._param.src, self.get_input().get("query", "-_-!"))
