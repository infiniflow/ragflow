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
from abc import ABC
import pandas as pd
import time
import requests
from agent.component.base import ComponentBase, ComponentParamBase


class TuShareParam(ComponentParamBase):
    """
    Define the TuShare component parameters.
    """

    def __init__(self):
        super().__init__()
        self.token = "xxx"
        self.src = "eastmoney"
        self.start_date = "2024-01-01 09:00:00"
        self.end_date = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime())
        self.keyword = ""

    def check(self):
        self.check_valid_value(self.src, "Quick News Source",
                               ["sina", "wallstreetcn", "10jqka", "eastmoney", "yuncaijing", "fenghuang", "jinrongjie"])


class TuShare(ComponentBase, ABC):
    component_name = "TuShare"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = ",".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return TuShare.be_output("")

        try:
            tus_res = []
            params = {
                "api_name": "news",
                "token": self._param.token,
                "params": {"src": self._param.src, "start_date": self._param.start_date,
                           "end_date": self._param.end_date}
            }
            response = requests.post(url="http://api.tushare.pro", data=json.dumps(params).encode('utf-8'))
            response = response.json()
            if response['code'] != 0:
                return TuShare.be_output(response['msg'])
            df = pd.DataFrame(response['data']['items'])
            df.columns = response['data']['fields']
            tus_res.append({"content": (df[df['content'].str.contains(self._param.keyword, case=False)]).to_markdown()})
        except Exception as e:
            return TuShare.be_output("**ERROR**: " + str(e))

        if not tus_res:
            return TuShare.be_output("")

        return pd.DataFrame(tus_res)
