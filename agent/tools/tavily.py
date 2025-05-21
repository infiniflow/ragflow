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
import logging
import re
from abc import ABC

import pandas as pd
from tavily import TavilyClient

from agent.tools.base import ToolParamBase, ToolBase, ToolMeta
from api.db import LLMType
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api import settings
from agent.component.base import ComponentBase, ComponentParamBase
from api.utils import get_uuid
from rag.app.tag import label_question
from rag.nlp import rag_tokenizer
from rag.prompts import kb_prompt
from rag.utils.tavily_conn import Tavily


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
                    "type": "str",
                    "description": "The search query to execute with Tavily.",
                    "required": True
                },
                "topic": {
                    "type": "enum",
                    "description": "default:general. The category of the search.news is useful for retrieving real-time updates, particularly about politics, sports, and major current events covered by mainstream media sources. general is for broader, more general-purpose searches that may include a wide range of sources.",
                    "required": False,
                    "enum": ["general", "news"]
                },
                "include_domains": {
                    "type": "List[str]",
                    "description": "default:[]. A list of domains to specifically include in the search results.",
                    "required": False
                },
                "exclude_domains": {
                    "type": "List[str]",
                    "description": "default:[]. A list of domains to specifically exclude from the search results.",
                    "required": False
                },
            }
        }
        super().__init__()
        self.api_key = ""
        self.search_depth = "basic" # basic/advanced
        self.topic = "general"
        self.max_results = 5
        self.days = 7
        self.include_answer = False
        self.include_raw_content = False
        self.include_images = False
        self.include_images = False
        self.include_image_descriptions = False
        self.include_domains = []
        self.exclude_domains = []

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
                "content_ltks": rag_tokenizer.tokenize(r["content"]),
                "content_with_weight": r["content"],
                "doc_id": id,
                "docnm_kwd": r["title"],
                "kb_id": [],
                "important_kwd": [],
                "image_id": "",
                "similarity": r["score"],
                "vector_similarity": 1.,
                "term_similarity": 0,
                "vector": [],
                "positions": [],
                "url": r["url"]
            })
            aggs.append({
                "doc_name": r["title"],
                "doc_id": id,
                "count": 1,
                "url": r["url"]
            })
        ref = {"chunks": chunks, "doc_aggs": aggs}
        self._param.outputs["result"] = {"_references": ref, "formalized_content": kb_prompt(ref, 200000, prefix=self._id), "json": response}

    async def _invoke(self, **kwargs):
        self.tavily_client = TavilyClient(api_key=self._param.api_key)
        last_e = None
        for _ in range(self._param.retry_times):
            try:
                res = self.tavily_client.search(**kwargs)
                self._retrieve_chunks(res)
                return
            except Exception as e:
                last_e = e
                logging.error(f"Tavily error: {e}")
        if last_e:
            raise last_e
