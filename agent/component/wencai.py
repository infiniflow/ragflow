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
from abc import ABC
import pandas as pd
import pywencai
from agent.component.base import ComponentBase, ComponentParamBase


class WenCaiParam(ComponentParamBase):
    """
    Define the WenCai component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 10
        self.query_type = "stock"

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")
        self.check_valid_value(self.query_type, "Query type",
                               ['stock', 'zhishu', 'fund', 'hkstock', 'usstock', 'threeboard', 'conbond', 'insurance',
                                'futures', 'lccp',
                                'foreign_exchange'])


class WenCai(ComponentBase, ABC):
    component_name = "WenCai"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = ",".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return WenCai.be_output("")

        try:
            wencai_res = []
            res = pywencai.get(query=ans, query_type=self._param.query_type, perpage=self._param.top_n)
            if isinstance(res, pd.DataFrame):
                wencai_res.append({"content": res.to_markdown()})
            if isinstance(res, dict):
                for item in res.items():
                    if isinstance(item[1], list):
                        wencai_res.append({"content": item[0] + "\n" + pd.DataFrame(item[1]).to_markdown()})
                        continue
                    if isinstance(item[1], str):
                        wencai_res.append({"content": item[0] + "\n" + item[1]})
                        continue
                    if isinstance(item[1], dict):
                        if "meta" in item[1].keys():
                            continue
                        wencai_res.append({"content": pd.DataFrame.from_dict(item[1], orient='index').to_markdown()})
                        continue
                    if isinstance(item[1], pd.DataFrame):
                        if "image_url" in item[1].columns:
                            continue
                        wencai_res.append({"content": item[1].to_markdown()})
                        continue
                        
                    wencai_res.append({"content": item[0] + "\n" + str(item[1])})
        except Exception as e:
            return WenCai.be_output("**ERROR**: " + str(e))

        if not wencai_res:
            return WenCai.be_output("")

        return pd.DataFrame(wencai_res)
