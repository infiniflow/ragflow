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
import requests
from agent.tools.base import ToolMeta, ToolParamBase, ToolBase
from common.connection_utils import timeout
from common.http_client import DEFAULT_TIMEOUT


class Jin10Param(ToolParamBase):
    """
    Define the Jin10 component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "jin10_market_data",
            "description": "Jin10 retrieves financial market data: newsflashes, economic calendars, commodity/forex/futures/crypto symbols, and news.",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "Keyword used to filter the returned newsflash/news items by content.",
                    "default": "{sys.query}",
                    "required": True,
                }
            },
        }
        super().__init__()
        self.type = "flash"
        self.secret_key = "xxx"
        self.flash_type = "1"
        self.calendar_type = "cj"
        self.calendar_datatype = "data"
        self.symbols_type = "GOODS"
        self.symbols_datatype = "symbols"
        self.contain = ""
        self.filter = ""

    def check(self):
        self.check_valid_value(self.type, "Type", ["flash", "calendar", "symbols", "news"])
        self.check_valid_value(self.flash_type, "Flash Type", ["1", "2", "3", "4", "5"])
        self.check_valid_value(self.calendar_type, "Calendar Type", ["cj", "qh", "hk", "us"])
        self.check_valid_value(self.calendar_datatype, "Calendar DataType", ["data", "event", "holiday"])
        self.check_valid_value(self.symbols_type, "Symbols Type", ["GOODS", "FOREX", "FUTURE", "CRYPTO"])
        self.check_valid_value(self.symbols_datatype, "Symbols DataType", ["symbols", "quotes"])

    def get_input_form(self) -> dict[str, dict]:
        return {"query": {"name": "Keyword", "type": "line"}}


class Jin10(ToolBase, ABC):
    component_name = "Jin10"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("Jin10 processing"):
            return

        contain = kwargs.get("query")
        if not contain:
            self.set_output("formalized_content", "")
            return ""

        jin10_res = []
        headers = {"secret-key": self._param.secret_key}
        try:
            if self._param.type == "flash":
                params = {"category": self._param.flash_type, "contain": contain, "filter": self._param.filter}
                response = requests.get(url="https://open-data-api.jin10.com/data-api/flash?category=" + self._param.flash_type, headers=headers, data=json.dumps(params), timeout=DEFAULT_TIMEOUT)
                response = response.json()
                for i in response["data"]:
                    if self.check_if_canceled("Jin10 processing"):
                        return
                    jin10_res.append(i["data"]["content"])
            if self._param.type == "calendar":
                params = {"category": self._param.calendar_type}
                response = requests.get(
                    url="https://open-data-api.jin10.com/data-api/calendar/" + self._param.calendar_datatype + "?category=" + self._param.calendar_type,
                    headers=headers,
                    data=json.dumps(params),
                    timeout=DEFAULT_TIMEOUT,
                )
                response = response.json()
                if self.check_if_canceled("Jin10 processing"):
                    return
                jin10_res.append(pd.DataFrame(response["data"]).to_markdown())
            if self._param.type == "symbols":
                params = {"type": self._param.symbols_type}
                if self._param.symbols_datatype == "quotes":
                    params["codes"] = "BTCUSD"
                response = requests.get(
                    url="https://open-data-api.jin10.com/data-api/" + self._param.symbols_datatype + "?type=" + self._param.symbols_type,
                    headers=headers,
                    data=json.dumps(params),
                    timeout=DEFAULT_TIMEOUT,
                )
                response = response.json()
                if self.check_if_canceled("Jin10 processing"):
                    return
                if self._param.symbols_datatype == "symbols":
                    for i in response["data"]:
                        if self.check_if_canceled("Jin10 processing"):
                            return
                        i["Commodity Code"] = i["c"]
                        i["Stock Exchange"] = i["e"]
                        i["Commodity Name"] = i["n"]
                        i["Commodity Type"] = i["t"]
                        del i["c"], i["e"], i["n"], i["t"]
                if self._param.symbols_datatype == "quotes":
                    for i in response["data"]:
                        if self.check_if_canceled("Jin10 processing"):
                            return
                        i["Selling Price"] = i["a"]
                        i["Buying Price"] = i["b"]
                        i["Commodity Code"] = i["c"]
                        i["Stock Exchange"] = i["e"]
                        i["Highest Price"] = i["h"]
                        i["Yesterday’s Closing Price"] = i["hc"]
                        i["Lowest Price"] = i["l"]
                        i["Opening Price"] = i["o"]
                        i["Latest Price"] = i["p"]
                        i["Market Quote Time"] = i["t"]
                        del i["a"], i["b"], i["c"], i["e"], i["h"], i["hc"], i["l"], i["o"], i["p"], i["t"]
                jin10_res.append(pd.DataFrame(response["data"]).to_markdown())
            if self._param.type == "news":
                params = {"contain": contain, "filter": self._param.filter}
                response = requests.get(url="https://open-data-api.jin10.com/data-api/news", headers=headers, data=json.dumps(params), timeout=DEFAULT_TIMEOUT)
                response = response.json()
                if self.check_if_canceled("Jin10 processing"):
                    return
                jin10_res.append(pd.DataFrame(response["data"]).to_markdown())
        except Exception as e:
            if self.check_if_canceled("Jin10 processing"):
                return

            logging.exception(f"Jin10 error: {e}")
            msg = f"Jin10 error: {e}"
            self.set_output("_ERROR", msg)
            return msg

        res = "\n\n".join(jin10_res)
        self.set_output("formalized_content", res)
        return res

    def thoughts(self) -> str:
        return "Fetching Jin10 {} data for: {}".format(self._param.type, self.get_input().get("query", "-_-!"))
