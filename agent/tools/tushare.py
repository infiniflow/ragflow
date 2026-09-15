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
import time
from abc import ABC

import pandas as pd
import requests

from agent.tools.base import ToolBase, ToolMeta, ToolParamBase
from common.connection_utils import timeout
from common.http_client import DEFAULT_TIMEOUT


class TuShareParam(ToolParamBase):
    """
    Define the TuShare component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "tushare_news",
            "description": (
                "TuShare is a Chinese financial data service. This tool fetches a quick-news "
                "feed (api_name=news) for a configurable source and date range, then filters by "
                "the configured keyword (or the user query if no keyword is set). Use this to "
                "retrieve recent Chinese financial news for a specific source."
            ),
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "Optional user query. Used as the news-filter keyword when the tool's `keyword` field is empty; otherwise the tool's `keyword` config wins.",
                    "default": "{sys.query}",
                    "required": False,
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
        self.check_valid_value(
            self.src,
            "Quick News Source",
            ["sina", "wallstreetcn", "10jqka", "eastmoney", "yuncaijing", "fenghuang", "jinrongjie"],
        )

    def get_input_form(self) -> dict[str, dict]:
        return {"query": {"name": "Query", "type": "line"}}


class TuShare(ToolBase, ABC):
    component_name = "TuShare"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("TuShare processing"):
            return

        try:
            if self.check_if_canceled("TuShare processing"):
                return

            params = {
                "api_name": "news",
                "token": self._param.token,
                "params": {
                    "src": self._param.src,
                    "start_date": self._param.start_date,
                    "end_date": self._param.end_date,
                },
            }
            response = requests.post(
                url="http://api.tushare.pro",
                data=json.dumps(params).encode("utf-8"),
                timeout=DEFAULT_TIMEOUT,
            )
            response = response.json()
            if self.check_if_canceled("TuShare processing"):
                return
            if response["code"] != 0:
                self.set_output("_ERROR", response["msg"])
                return response["msg"]
            df = pd.DataFrame(response["data"]["items"])
            df.columns = response["data"]["fields"]
            if self.check_if_canceled("TuShare processing"):
                return
            keyword = self._param.keyword or kwargs.get("query", "")
            logging.info(
                "TuShare news filter keyword source=%s",
                "param.keyword" if self._param.keyword else "upstream_input",
            )
            filtered = df[df["content"].str.contains(keyword, case=False, na=False, regex=False)]
            markdown = filtered.to_markdown() if not filtered.empty else ""
            self.set_output("json", filtered.to_dict(orient="records"))
            self.set_output("formalized_content", markdown)
            return self.output("formalized_content")
        except Exception as e:
            if self.check_if_canceled("TuShare processing"):
                return
            self.set_output("_ERROR", str(e))
            return "**ERROR**: " + str(e)
