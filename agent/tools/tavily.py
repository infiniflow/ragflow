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
from abc import ABC
from tavily import TavilyClient

from agent.tools.base import ToolParamBase, ToolBase, ToolMeta
from api.utils import get_uuid
from api.utils.api_utils import timeout
from rag.prompts import kb_prompt


class TavilySearchParam(ToolParamBase):
    """
    Define the Retrieval component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "tavily_search",
            "description": "Tavily is a search engine optimized for LLMs, aimed at efficient, quick and persistent search results. Unlike other search APIs such as Serp or Google, Tavily focuses on optimizing search for AI developers and autonomous AI agents. We take care of all the burden of searching, scraping, filtering and extracting the most relevant information from online sources. All in a single API call!",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search query to execute with Tavily. Less keywords is recommanded.",
                    "default": "{sys.query}",
                    "required": True
                },
                "topic": {
                    "type": "string",
                    "description": "default:general. The category of the search.news is useful for retrieving real-time updates, particularly about politics, sports, and major current events covered by mainstream media sources. general is for broader, more general-purpose searches that may include a wide range of sources.",
                    "enum": ["general", "news"],
                    "default": "general",
                    "required": False,
                },
                "include_domains": {
                    "type": "array",
                    "description": "default:[]. A list of domains only from which the search results can be included.",
                    "default": [],
                    "items": {
                        "type": "string",
                        "description": "Domain name that must be included, e.g. www.yahoo.com"
                    },
                    "required": False
                },
                "exclude_domains": {
                    "type": "array",
                    "description": "default:[]. A list of domains from which the search results can not be included",
                    "default": [],
                    "items": {
                        "type": "string",
                        "description": "Domain name that must be excluded, e.g. www.yahoo.com"
                    },
                    "required": False
                },
            }
        }
        super().__init__()
        self.api_key = ""
        self.search_depth = "basic" # basic/advanced
        self.max_results = 5
        self.days = 7
        self.include_answer = False
        self.include_raw_content = True
        self.include_images = False
        self.include_image_descriptions = False

    def check(self):
        self.check_valid_value(self.topic, "Tavily topic: should be in 'general/news'", ["general", "news"])
        self.check_valid_value(self.search_depth, "Tavily search depth should be in 'basic/advanced'", ["basic", "advanced"])
        self.check_positive_integer(self.max_results, "Tavily max result number should be within [1， 20]")
        self.check_positive_integer(self.days, "Tavily days should be greater than 1")


class TavilySearch(ToolBase, ABC):
    component_name = "TavilySearch"

    def _retrieve_chunks(self, response):
        chunks = []
        aggs = []
        for r in response["results"]:
            id = get_uuid()
            chunks.append({
                "chunk_id": id,
                "content_with_weight": r["content"],
                "doc_id": id,
                "docnm_kwd": r["title"],
                "similarity": r["score"],
                "url": r["url"],
                #"content_ltks": rag_tokenizer.tokenize(r["content"]),
                #"kb_id": [],
                #"important_kwd": [],
                #"image_id": "",
                #"vector_similarity": 1.,
                #"term_similarity": 0,
                #"vector": [],
                #"positions": [],
            })
            aggs.append({
                "doc_name": r["title"],
                "doc_id": id,
                "count": 1,
                "url": r["url"]
            })
        ref = {"chunks": chunks, "doc_aggs": aggs}
        self.set_output("_references", ref)
        self.set_output("formalized_content", "\n".join(kb_prompt(ref, 200000, prefix=self._id)))
        self.set_output("json", response)

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        self.tavily_client = TavilyClient(api_key=self._param.api_key)
        last_e = None
        for fld in ["search_depth", "topic", "max_results", "days", "include_answer", "include_raw_content", "include_images", "include_image_descriptions", "include_domains", "exclude_domains"]:
            if fld not in kwargs:
                kwargs[fld] = getattr(self._param, fld)
        for _ in range(self._param.max_retries+1):
            try:
                kwargs["include_images"] = False
                res = self.tavily_client.search(**kwargs)
                self._retrieve_chunks(res)
                return
            except Exception as e:
                last_e = e
                logging.error(f"Tavily error: {e}")
        if last_e:
            self.set_output("_ERROR", str(last_e))
