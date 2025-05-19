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
from Bio import Entrez
import re
import pandas as pd
import xml.etree.ElementTree as ET
from agent.component.base import ComponentBase, ComponentParamBase


class PubMedParam(ComponentParamBase):
    """
    Define the PubMed component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 5
        self.email = "A.N.Other@example.com"

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")


class PubMed(ComponentBase, ABC):
    component_name = "PubMed"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return PubMed.be_output("")

        try:
            Entrez.email = self._param.email
            pubmedids = Entrez.read(Entrez.esearch(db='pubmed', retmax=self._param.top_n, term=ans))['IdList']
            pubmedcnt = ET.fromstring(re.sub(r'<(/?)b>|<(/?)i>', '', Entrez.efetch(db='pubmed', id=",".join(pubmedids),
                                                                                   retmode="xml").read().decode(
                "utf-8")))
            pubmed_res = [{"content": 'Title:' + child.find("MedlineCitation").find("Article").find(
                "ArticleTitle").text + '\nUrl:<a href=" https://pubmed.ncbi.nlm.nih.gov/' + child.find(
                "MedlineCitation").find("PMID").text + '">' + '</a>\n' + 'Abstract:' + (
                                          child.find("MedlineCitation").find("Article").find("Abstract").find(
                                              "AbstractText").text if child.find("MedlineCitation").find(
                                              "Article").find("Abstract") else "No abstract available")} for child in
                          pubmedcnt.findall("PubmedArticle")]
        except Exception as e:
            return PubMed.be_output("**ERROR**: " + str(e))

        if not pubmed_res:
            return PubMed.be_output("")

        df = pd.DataFrame(pubmed_res)
        logging.debug(f"df: {df}")
        return df
