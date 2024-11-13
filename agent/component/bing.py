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
import requests
import pandas as pd
from agent.component.base import ComponentBase, ComponentParamBase

class BingParam(ComponentParamBase):
    """
    Define the Bing component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 10
        self.channel = "Webpages"
        self.api_key = "YOUR_ACCESS_KEY"
        self.country = "CN"
        self.language = "en"

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")
        self.check_valid_value(self.channel, "Bing Web Search or Bing News", ["Webpages", "News"])
        self.check_empty(self.api_key, "Bing subscription key")
        self.check_valid_value(self.country, "Bing Country",
                               ['AR', 'AU', 'AT', 'BE', 'BR', 'CA', 'CL', 'DK', 'FI', 'FR', 'DE', 'HK', 'IN', 'ID',
                                'IT', 'JP', 'KR', 'MY', 'MX', 'NL', 'NZ', 'NO', 'CN', 'PL', 'PT', 'PH', 'RU', 'SA',
                                'ZA', 'ES', 'SE', 'CH', 'TW', 'TR', 'GB', 'US'])
        self.check_valid_value(self.language, "Bing Languages",
                               ['ar', 'eu', 'bn', 'bg', 'ca', 'ns', 'nt', 'hr', 'cs', 'da', 'nl', 'en', 'gb', 'et',
                                'fi', 'fr', 'gl', 'de', 'gu', 'he', 'hi', 'hu', 'is', 'it', 'jp', 'kn', 'ko', 'lv',
                                'lt', 'ms', 'ml', 'mr', 'nb', 'pl', 'br', 'pt', 'pa', 'ro', 'ru', 'sr', 'sk', 'sl',
                                'es', 'sv', 'ta', 'te', 'th', 'tr', 'uk', 'vi'])


class Bing(ComponentBase, ABC):
    component_name = "Bing"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return Bing.be_output("")

        try:
            headers = {"Ocp-Apim-Subscription-Key": self._param.api_key, 'Accept-Language': self._param.language}
            params = {"q": ans, "textDecorations": True, "textFormat": "HTML", "cc": self._param.country,
                      "answerCount": 1, "promote": self._param.channel}
            if self._param.channel == "Webpages":
                response = requests.get("https://api.bing.microsoft.com/v7.0/search", headers=headers, params=params)
                response.raise_for_status()
                search_results = response.json()
                bing_res = [{"content": '<a href="' + i["url"] + '">' + i["name"] + '</a>    ' + i["snippet"]} for i in
                            search_results["webPages"]["value"]]
            elif self._param.channel == "News":
                response = requests.get("https://api.bing.microsoft.com/v7.0/news/search", headers=headers,
                                        params=params)
                response.raise_for_status()
                search_results = response.json()
                bing_res = [{"content": '<a href="' + i["url"] + '">' + i["name"] + '</a>    ' + i["description"]} for i
                            in search_results['news']['value']]
        except Exception as e:
            return Bing.be_output("**ERROR**: " + str(e))

        if not bing_res:
            return Bing.be_output("")

        df = pd.DataFrame(bing_res)
        logging.debug(f"df: {str(df)}")
        return df
