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
from serpapi import GoogleSearch
from agent.tools.base import ToolParamBase, ToolMeta, ToolBase
from api.utils.api_utils import timeout


class GoogleParam(ToolParamBase):
    """
    Define the Google component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "google_search",
            "description": """Search the world's information, including webpages, images, videos and more. Google has many special features to help you find exactly what you're looking ...""",
            "parameters": {
                "q": {
                    "type": "string",
                    "description": "The search keywords to execute with Google. The keywords should be the most important words/terms(includes synonyms) from the original request.",
                    "default": "{sys.query}",
                    "required": True
                },
                "start": {
                    "type": "integer",
                    "description": "Parameter defines the result offset. It skips the given number of results. It's used for pagination. (e.g., 0 (default) is the first page of results, 10 is the 2nd page of results, 20 is the 3rd page of results, etc.). Google Local Results only accepts multiples of 20(e.g. 20 for the second page results, 40 for the third page results, etc.) as the `start` value.",
                    "default": "0",
                    "required": False,
                },
                "num": {
                    "type": "integer",
                    "description": "Parameter defines the maximum number of results to return. (e.g., 10 (default) returns 10 results, 40 returns 40 results, and 100 returns 100 results). The use of num may introduce latency, and/or prevent the inclusion of specialized result types. It is better to omit this parameter unless it is strictly necessary to increase the number of results per page. Results are not guaranteed to have the number of results specified in num.",
                    "default": "6",
                    "required": False,
                }
            }
        }
        super().__init__()
        self.start = 0
        self.num = 6
        self.api_key = ""
        self.country = "cn"
        self.language = "en"

    def check(self):
        self.check_empty(self.api_key, "SerpApi API key")
        self.check_valid_value(self.country, "Google Country",
                               ['af', 'al', 'dz', 'as', 'ad', 'ao', 'ai', 'aq', 'ag', 'ar', 'am', 'aw', 'au', 'at',
                                'az', 'bs', 'bh', 'bd', 'bb', 'by', 'be', 'bz', 'bj', 'bm', 'bt', 'bo', 'ba', 'bw',
                                'bv', 'br', 'io', 'bn', 'bg', 'bf', 'bi', 'kh', 'cm', 'ca', 'cv', 'ky', 'cf', 'td',
                                'cl', 'cn', 'cx', 'cc', 'co', 'km', 'cg', 'cd', 'ck', 'cr', 'ci', 'hr', 'cu', 'cy',
                                'cz', 'dk', 'dj', 'dm', 'do', 'ec', 'eg', 'sv', 'gq', 'er', 'ee', 'et', 'fk', 'fo',
                                'fj', 'fi', 'fr', 'gf', 'pf', 'tf', 'ga', 'gm', 'ge', 'de', 'gh', 'gi', 'gr', 'gl',
                                'gd', 'gp', 'gu', 'gt', 'gn', 'gw', 'gy', 'ht', 'hm', 'va', 'hn', 'hk', 'hu', 'is',
                                'in', 'id', 'ir', 'iq', 'ie', 'il', 'it', 'jm', 'jp', 'jo', 'kz', 'ke', 'ki', 'kp',
                                'kr', 'kw', 'kg', 'la', 'lv', 'lb', 'ls', 'lr', 'ly', 'li', 'lt', 'lu', 'mo', 'mk',
                                'mg', 'mw', 'my', 'mv', 'ml', 'mt', 'mh', 'mq', 'mr', 'mu', 'yt', 'mx', 'fm', 'md',
                                'mc', 'mn', 'ms', 'ma', 'mz', 'mm', 'na', 'nr', 'np', 'nl', 'an', 'nc', 'nz', 'ni',
                                'ne', 'ng', 'nu', 'nf', 'mp', 'no', 'om', 'pk', 'pw', 'ps', 'pa', 'pg', 'py', 'pe',
                                'ph', 'pn', 'pl', 'pt', 'pr', 'qa', 're', 'ro', 'ru', 'rw', 'sh', 'kn', 'lc', 'pm',
                                'vc', 'ws', 'sm', 'st', 'sa', 'sn', 'rs', 'sc', 'sl', 'sg', 'sk', 'si', 'sb', 'so',
                                'za', 'gs', 'es', 'lk', 'sd', 'sr', 'sj', 'sz', 'se', 'ch', 'sy', 'tw', 'tj', 'tz',
                                'th', 'tl', 'tg', 'tk', 'to', 'tt', 'tn', 'tr', 'tm', 'tc', 'tv', 'ug', 'ua', 'ae',
                                'uk', 'gb', 'us', 'um', 'uy', 'uz', 'vu', 've', 'vn', 'vg', 'vi', 'wf', 'eh', 'ye',
                                'zm', 'zw'])
        self.check_valid_value(self.language, "Google languages",
                               ['af', 'ak', 'sq', 'ws', 'am', 'ar', 'hy', 'az', 'eu', 'be', 'bem', 'bn', 'bh',
                                'xx-bork', 'bs', 'br', 'bg', 'bt', 'km', 'ca', 'chr', 'ny', 'zh-cn', 'zh-tw', 'co',
                                'hr', 'cs', 'da', 'nl', 'xx-elmer', 'en', 'eo', 'et', 'ee', 'fo', 'tl', 'fi', 'fr',
                                'fy', 'gaa', 'gl', 'ka', 'de', 'el', 'kl', 'gn', 'gu', 'xx-hacker', 'ht', 'ha', 'haw',
                                'iw', 'hi', 'hu', 'is', 'ig', 'id', 'ia', 'ga', 'it', 'ja', 'jw', 'kn', 'kk', 'rw',
                                'rn', 'xx-klingon', 'kg', 'ko', 'kri', 'ku', 'ckb', 'ky', 'lo', 'la', 'lv', 'ln', 'lt',
                                'loz', 'lg', 'ach', 'mk', 'mg', 'ms', 'ml', 'mt', 'mv', 'mi', 'mr', 'mfe', 'mo', 'mn',
                                'sr-me', 'my', 'ne', 'pcm', 'nso', 'no', 'nn', 'oc', 'or', 'om', 'ps', 'fa',
                                'xx-pirate', 'pl', 'pt', 'pt-br', 'pt-pt', 'pa', 'qu', 'ro', 'rm', 'nyn', 'ru', 'gd',
                                'sr', 'sh', 'st', 'tn', 'crs', 'sn', 'sd', 'si', 'sk', 'sl', 'so', 'es', 'es-419', 'su',
                                'sw', 'sv', 'tg', 'ta', 'tt', 'te', 'th', 'ti', 'to', 'lua', 'tum', 'tr', 'tk', 'tw',
                                'ug', 'uk', 'ur', 'uz', 'vu', 'vi', 'cy', 'wo', 'xh', 'yi', 'yo', 'zu']
                               )

    def get_input_form(self) -> dict[str, dict]:
        return {
            "q": {
                "name": "Query",
                "type": "line"
            },
            "start": {
                "name": "From",
                "type": "integer",
                "value": 0
            },
            "num": {
                "name": "Limit",
                "type": "integer",
                "value": 12
            }
        }

class Google(ToolBase, ABC):
    component_name = "Google"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12))
    def _invoke(self, **kwargs):
        if not kwargs.get("q"):
            self.set_output("formalized_content", "")
            return ""

        params = {
            "api_key": self._param.api_key,
            "engine": "google",
            "q": kwargs["q"],
            "google_domain": "google.com",
            "gl": self._param.country,
            "hl": self._param.language
        }
        last_e = ""
        for _ in range(self._param.max_retries+1):
            try:
                search = GoogleSearch(params).get_dict()
                self._retrieve_chunks(search["organic_results"],
                                      get_title=lambda r: r["title"],
                                      get_url=lambda r: r["link"],
                                      get_content=lambda r: r.get("about_this_result", {}).get("source", {}).get("description", r["snippet"])
                                      )
                self.set_output("json", search["organic_results"])
                return self.output("formalized_content")
            except Exception as e:
                last_e = e
                logging.exception(f"Google error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"Google error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return """
Keywords: {} 
Looking for the most relevant articles.
        """.format(self.get_input().get("query", "-_-!"))