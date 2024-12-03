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
from agent.component.base import ComponentBase, ComponentParamBase
from scholarly import scholarly


class GoogleScholarParam(ComponentParamBase):
    """
    Define the GoogleScholar component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 6
        self.sort_by = 'relevance'
        self.year_low = None
        self.year_high = None
        self.patents = True

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")
        self.check_valid_value(self.sort_by, "GoogleScholar Sort_by", ['date', 'relevance'])
        self.check_boolean(self.patents, "Whether or not to include patents, defaults to True")


class GoogleScholar(ComponentBase, ABC):
    component_name = "GoogleScholar"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return GoogleScholar.be_output("")

        scholar_client = scholarly.search_pubs(ans, patents=self._param.patents, year_low=self._param.year_low,
                                               year_high=self._param.year_high, sort_by=self._param.sort_by)
        scholar_res = []
        for i in range(self._param.top_n):
            try:
                pub = next(scholar_client)
                scholar_res.append({"content": 'Title: ' + pub['bib']['title'] + '\n_Url: <a href="' + pub[
                    'pub_url'] + '"></a> ' + "\n author: " + ",".join(pub['bib']['author']) + '\n Abstract: ' + pub[
                                                   'bib'].get('abstract', 'no abstract')})

            except StopIteration or Exception:
                logging.exception("GoogleScholar")
                break

        if not scholar_res:
            return GoogleScholar.be_output("")

        df = pd.DataFrame(scholar_res)
        logging.debug(f"df: {df}")
        return df
