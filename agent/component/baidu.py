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
import pandas as pd
import requests
from bs4 import BeautifulSoup
import re
from agent.component.base import ComponentBase, ComponentParamBase


class BaiduParam(ComponentParamBase):
    """
    Define the Baidu component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 10

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")


class Baidu(ComponentBase, ABC):
    component_name = "Baidu"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return Baidu.be_output("")

        try:
            url = 'https://www.baidu.com/s?wd=' + ans + '&rn=' + str(self._param.top_n)
            headers = {
                'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36',
                'Accept': 'text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8',
                'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8',
                'Connection': 'keep-alive',
            }
            response = requests.get(url=url, headers=headers)
            # check if request success
            if response.status_code == 200:
                soup = BeautifulSoup(response.text, 'html.parser')
                url_res = []
                title_res = []
                body_res = []
                for item in soup.select('.result.c-container'):
                    # extract title
                    title_res.append(item.select_one('h3 a').get_text(strip=True))
                    url_res.append(item.select_one('h3 a')['href'])
                    body_res.append(item.select_one('.c-abstract').get_text(strip=True) if item.select_one('.c-abstract') else '')
                baidu_res = [{"content": re.sub('<em>|</em>', '', '<a href="' + url + '">' + title + '</a>    ' + body)} for
                             url, title, body in zip(url_res, title_res, body_res)]
                del body_res, url_res, title_res
        except Exception as e:
            return Baidu.be_output("**ERROR**: " + str(e))

        if not baidu_res:
            return Baidu.be_output("")

        df = pd.DataFrame(baidu_res)
        logging.debug(f"df: {str(df)}")
        return df

