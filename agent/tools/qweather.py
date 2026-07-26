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

from agent.tools.base import ToolMeta, ToolParamBase, ToolBase
from common.connection_utils import timeout
from common.http_client import DEFAULT_TIMEOUT


class QWeatherParam(ToolParamBase):
    """
    Define the QWeather component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "qweather",
            "description": "QWeather (和风天气) looks up the current weather, a multi-day forecast, life indices, or air quality for a location using the QWeather API.",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The location to look up weather for, e.g. a city name like 'Beijing' or '北京'.",
                    "default": "{sys.query}",
                    "required": True,
                }
            },
        }
        super().__init__()
        self.web_apikey = "xxx"
        self.lang = "zh"
        self.type = "weather"
        self.user_type = "free"
        self.error_code = {
            "204": "The request was successful, but the region you are querying does not have the data you need at this time.",
            "400": "Request error, may contain incorrect request parameters or missing mandatory request parameters.",
            "401": "Authentication fails, possibly using the wrong KEY, wrong digital signature, wrong type of KEY (e.g. using the SDK's KEY to access the Web API).",
            "402": "Exceeded the number of accesses or the balance is not enough to support continued access to the service, you can recharge, upgrade the accesses or wait for the accesses to be reset.",
            "403": "No access, may be the binding PackageName, BundleID, domain IP address is inconsistent, or the data that requires additional payment.",
            "404": "The queried data or region does not exist.",
            "429": "Exceeded the limited QPM (number of accesses per minute), please refer to the QPM description",
            "500": "No response or timeout, interface service abnormality please contact us",
        }
        # Weather
        self.time_period = "now"

    def check(self):
        self.check_empty(self.web_apikey, "QWeather API key")
        self.check_valid_value(self.type, "Type", ["weather", "indices", "airquality"])
        self.check_valid_value(self.user_type, "Free subscription or paid subscription", ["free", "paid"])
        self.check_valid_value(
            self.lang,
            "Use language",
            [
                "zh",
                "zh-hant",
                "en",
                "de",
                "es",
                "fr",
                "it",
                "ja",
                "ko",
                "ru",
                "hi",
                "th",
                "ar",
                "pt",
                "bn",
                "ms",
                "nl",
                "el",
                "la",
                "sv",
                "id",
                "pl",
                "tr",
                "cs",
                "et",
                "vi",
                "fil",
                "fi",
                "he",
                "is",
                "nb",
            ],
        )
        self.check_valid_value(self.time_period, "Time period", ["now", "3d", "7d", "10d", "15d", "30d"])

    def get_input_form(self) -> dict[str, dict]:
        return {"query": {"name": "Location", "type": "line"}}


class QWeather(ToolBase, ABC):
    component_name = "QWeather"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("QWeather processing"):
            return

        location = kwargs.get("query")
        if not location:
            self.set_output("formalized_content", "")
            return ""

        last_e = None
        for _ in range(self._param.max_retries + 1):
            if self.check_if_canceled("QWeather processing"):
                return

            try:
                lookup = requests.get(
                    url="https://geoapi.qweather.com/v2/city/lookup?location=" + location + "&key=" + self._param.web_apikey,
                    timeout=DEFAULT_TIMEOUT,
                ).json()

                if self.check_if_canceled("QWeather processing"):
                    return

                if lookup["code"] != "200":
                    return self._finish(self._error_message(lookup["code"]))

                location_id = lookup["location"][0]["id"]
                base_url = "https://api.qweather.com/v7/" if self._param.user_type == "paid" else "https://devapi.qweather.com/v7/"

                if self._param.type == "weather":
                    url = base_url + "weather/" + self._param.time_period + "?location=" + location_id + "&key=" + self._param.web_apikey + "&lang=" + self._param.lang
                    response = requests.get(url=url, timeout=DEFAULT_TIMEOUT).json()
                    if self.check_if_canceled("QWeather processing"):
                        return
                    if response["code"] != "200":
                        return self._finish(self._error_message(response["code"]))
                    if self._param.time_period == "now":
                        return self._finish(str(response["now"]))
                    return self._finish("\n".join(str(i) for i in response["daily"]))

                if self._param.type == "indices":
                    url = base_url + "indices/1d?type=0&location=" + location_id + "&key=" + self._param.web_apikey + "&lang=" + self._param.lang
                    response = requests.get(url=url, timeout=DEFAULT_TIMEOUT).json()
                    if self.check_if_canceled("QWeather processing"):
                        return
                    if response["code"] != "200":
                        return self._finish(self._error_message(response["code"]))
                    indices_res = response["daily"][0]["date"] + "\n" + "\n".join(i["name"] + ": " + i["category"] + ", " + i["text"] for i in response["daily"])
                    return self._finish(indices_res)

                # airquality
                url = base_url + "air/now?location=" + location_id + "&key=" + self._param.web_apikey + "&lang=" + self._param.lang
                response = requests.get(url=url, timeout=DEFAULT_TIMEOUT).json()
                if self.check_if_canceled("QWeather processing"):
                    return
                if response["code"] != "200":
                    return self._finish(self._error_message(response["code"]))
                return self._finish(str(response["now"]))
            except Exception as e:
                if self.check_if_canceled("QWeather processing"):
                    return
                last_e = e
                logging.exception(f"QWeather error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"QWeather error: {last_e}"

        assert False, self.output()

    def _error_message(self, code) -> str:
        return "**Error** " + self._param.error_code.get(str(code), f"QWeather API returned code {code}.")

    def _finish(self, content: str) -> str:
        self.set_output("formalized_content", content)
        return content

    def thoughts(self) -> str:
        return "Looking up the weather for: {}".format(self.get_input().get("query", "-_-!"))
