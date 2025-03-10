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
from serpapi import GoogleSearch as SerpApiSearch
import pandas as pd
from agent.component.base import ComponentBase, ComponentParamBase
import requests
from googlesearch import search


class GoogleParam(ComponentParamBase):
    """
    Define the Google component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 10
        self.api_key = "xxx"
        self.country = "cn"
        self.language = "en"
        self.provider = "OpenSearch" 

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")
        self.check_empty(self.api_key, "API key")
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
        self.check_valid_value(self.provider, "Provider type", ['SerpApi', 'GoogleCustomSearch','OpenSearch'])  

class Google(ComponentBase, ABC):
    component_name = "Google"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return Google.be_output("")
        logging.info(f"self._param: {self._param}")
        try:
            if self._param.provider == "SerpApi":
                google_res = self.search_serpapi(ans)
            elif self._param.provider == "GoogleCustomSearch":
                google_res = self.search_google_custom(ans)
            elif self._param.provider == "OpenSearch":
                google_res = self.search_opensearch(ans)
            else:
                return Google.be_output("**ERROR**: Unsupported provider!")
        except Exception as e:
            logging.info(f"Search error: {e}")
            return Google.be_output(f"**ERROR**: {e}!")

        if not google_res:
            return Google.be_output("")

        df = pd.DataFrame(google_res)
        logging.debug(f"df: {df}")
        return df

    def search_serpapi(self, query):
        """
        Perform a search using the SerpApi and return the results.
        """
        client = SerpApiSearch(
            {"engine": "google", "q": query, "api_key": self._param.api_key, "gl": self._param.country,
             "hl": self._param.language, "num": self._param.top_n})
        results = [{"content": '<a href="' + i["link"] + '">' + i["title"] + '</a>    ' + i["snippet"]} for i in
                   client.get_dict()["organic_results"]]
        logging.info(f"{results}")

        return results

    def search_google_custom(self, query):
        """
        Perform a search using the Google Custom Search API and return the results.
        """
        url = f"https://www.googleapis.com/customsearch/v1?q={query}&key={self._param.api_key}&cx=YOUR_CX_ID&gl={self._param.country}&hl={self._param.language}&num={self._param.top_n}"
        response = requests.get(url)
        response.raise_for_status()
        data = response.json()
        results = [{"content": '<a href="' + item["link"] + '">' + item["title"] + '</a>    ' + item["snippet"]} for item in data.get("items", [])]
        logging.info(f"{results}")

        return results

    def search_opensearch(self, query):
        """
        Perform a search using the OpenSearch and return the results.
        """

        results = []
        for url in search(query, num_results=self._param.top_n, lang=self._param.language, advanced=True ):
            try:
                title = url.title
                snippet = url.description if url.description else 'No description available'
                results.append({"content": f'<a href="{url.url}">{title}</a>    {snippet}'})
            except Exception as e:
                logging.error(f"Error processing search result {url}: {e}")
                results.append({"content": f'<a href="{url.url}">{url.url}</a>    Error processing details'})
        logging.info(f"{results}")
        return results
