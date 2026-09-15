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
import time
from abc import ABC

import pandas as pd
import requests

from agent.tools.base import ToolBase, ToolMeta, ToolParamBase
from common.http_client import DEFAULT_TIMEOUT


class TuShareParam(ToolParamBase):
    """
    Define the TuShare component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "tushare_quick_news",
            "description": "TuShare retrieves quick financial news from the configured source and filters it by keyword.",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The keyword to filter news by, e.g. '银行'.",
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
        self.check_valid_value(
            self.src,
            "Quick News Source",
            ["sina", "wallstreetcn", "10jqka", "eastmoney", "yuncaijing", "fenghuang", "jinrongjie"],
        )

    def get_input_form(self) -> dict[str, dict]:
        return {"query": {"name": "Keyword", "type": "line"}}


class TuShare(ToolBase, ABC):
    component_name = "TuShare"

    def _invoke(self, **kwargs):
        if self.check_if_canceled("TuShare processing"):
            return ""

        # Keyword precedence: an explicit param.keyword wins; then the
        # invoked query kwarg; then the tool's DECLARED query input (the
        # form field get_input() resolves, including {sys.query}); then any
        # upstream component content.
        upstream = self.get_input()
        upstream_content = ",".join(upstream["content"]) if "content" in upstream else ""
        declared_query = self.get_input("query")
        keyword = self._param.keyword or kwargs.get("query") or declared_query or upstream_content

        try:
            if self.check_if_canceled("TuShare processing"):
                return ""

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
                return ""
            if response["code"] != 0:
                # A non-zero code is an error, not ordinary content.
                logging.warning("TuShare API returned code %s: %s", response["code"], response["msg"])
                self.set_output("_ERROR", response["msg"])
                return f"TuShare error: {response['msg']}"

            df = pd.DataFrame(response["data"]["items"])
            df.columns = response["data"]["fields"]
            if self.check_if_canceled("TuShare processing"):
                return ""

            if keyword:
                logging.info(
                    "TuShare news filter keyword source=%s",
                    "param.keyword"
                    if self._param.keyword
                    else ("query" if kwargs.get("query") else "upstream_input"),
                )
                df = df[df["content"].str.contains(keyword, case=False, na=False, regex=False)]

            res = df.to_markdown()
        except Exception as e:
            if self.check_if_canceled("TuShare processing"):
                return ""
            logging.exception("TuShare request failed: %s", e)
            self.set_output("_ERROR", str(e))
            return f"TuShare error: {e}"

        self.set_output("formalized_content", res)
        return res

    def thoughts(self) -> str:
        # kwargs are not persisted across invoke(), so report the same
        # precedence _invoke uses minus its kwarg leg (the declared input
        # carries the invoked value in practice).
        keyword = self._param.keyword or self.get_input("query") or "-_-!"
        return "Looking up TuShare quick news for: {}".format(keyword)
