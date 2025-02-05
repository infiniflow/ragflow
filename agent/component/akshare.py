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
from agent.component.base import ComponentBase, ComponentParamBase


class AkShareParam(ComponentParamBase):
    """
    Define the AkShare component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 10

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")


class AkShare(ComponentBase, ABC):
    component_name = "AkShare"

    def _run(self, history, **kwargs):
        import akshare as ak
        ans = self.get_input()
        ans = ",".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return AkShare.be_output("")

        try:
            ak_res = []
            stock_news_em_df = ak.stock_news_em(symbol=ans)
            stock_news_em_df = stock_news_em_df.head(self._param.top_n)
            ak_res = [{"content": '<a href="' + i["新闻链接"] + '">' + i["新闻标题"] + '</a>\n 新闻内容: ' + i[
                "新闻内容"] + " \n发布时间:" + i["发布时间"] + " \n文章来源: " + i["文章来源"]} for index, i in stock_news_em_df.iterrows()]
        except Exception as e:
            return AkShare.be_output("**ERROR**: " + str(e))

        if not ak_res:
            return AkShare.be_output("")

        return pd.DataFrame(ak_res)
