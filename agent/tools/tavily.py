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
from tavily import TavilyClient
from agent.tools.base import ToolParamBase, ToolBase, ToolMeta
from api.utils.api_utils import timeout


class TavilySearchParam(ToolParamBase):
    """
    Define the Retrieval component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "tavily_search",
            "description": """
Tavily is a search engine optimized for LLMs, aimed at efficient, quick and persistent search results. 
When searching:
   - Start with specific query which should focus on just a single aspect.
   - Number of keywords in query should be less than 5.
   - Broaden search terms if needed
   - Cross-reference information from multiple sources
             """,
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search keywords to execute with Tavily. The keywords should be the most important words/terms(includes synonyms) from the original request.",
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
        self.max_results = 6
        self.days = 14
        self.include_answer = False
        self.include_raw_content = False
        self.include_images = False
        self.include_image_descriptions = False

    def check(self):
        self.check_valid_value(self.topic, "Tavily topic: should be in 'general/news'", ["general", "news"])
        self.check_valid_value(self.search_depth, "Tavily search depth should be in 'basic/advanced'", ["basic", "advanced"])
        self.check_positive_integer(self.max_results, "Tavily max result number should be within [1， 20]")
        self.check_positive_integer(self.days, "Tavily days should be greater than 1")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "name": "Query",
                "type": "line"
            }
        }

class TavilySearch(ToolBase, ABC):
    component_name = "TavilySearch"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12))
    def _invoke(self, **kwargs):
        if not kwargs.get("query"):
            self.set_output("formalized_content", "")
            return ""

        self.tavily_client = TavilyClient(api_key=self._param.api_key)
        last_e = None
        for fld in ["search_depth", "topic", "max_results", "days", "include_answer", "include_raw_content", "include_images", "include_image_descriptions", "include_domains", "exclude_domains"]:
            if fld not in kwargs:
                kwargs[fld] = getattr(self._param, fld)
        for _ in range(self._param.max_retries+1):
            try:
                kwargs["include_images"] = False
                kwargs["include_raw_content"] = False
                res = self.tavily_client.search(**kwargs)
                self._retrieve_chunks(res["results"],
                                      get_title=lambda r: r["title"],
                                      get_url=lambda r: r["url"],
                                      get_content=lambda r: r["raw_content"] if r["raw_content"] else r["content"],
                                      get_score=lambda r: r["score"])
                self.set_output("json", res["results"])
                return self.output("formalized_content")
            except Exception as e:
                last_e = e
                logging.exception(f"Tavily error: {e}")
                time.sleep(self._param.delay_after_error)
        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"Tavily error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return """
Keywords: {} 
Looking for the most relevant articles.
                """.format(self.get_input().get("query", "-_-!"))


class TavilyExtractParam(ToolParamBase):
    """
    Define the Retrieval component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "tavily_extract",
            "description": "Extract web page content from one or more specified URLs using Tavily Extract.",
            "parameters": {
                "urls": {
                    "type": "array",
                    "description": "The URLs to extract content from.",
                    "default": "",
                    "items": {
                        "type": "string",
                        "description": "The URL to extract content from, e.g. www.yahoo.com"
                    },
                    "required": True
                },
                "extract_depth": {
                    "type": "string",
                    "description": "The depth of the extraction process. advanced extraction retrieves more data, including tables and embedded content, with higher success but may increase latency.basic extraction costs 1 credit per 5 successful URL extractions, while advanced extraction costs 2 credits per 5 successful URL extractions.",
                    "enum": ["basic", "advanced"],
                    "default": "basic",
                    "required": False,
                },
                "format": {
                    "type": "string",
                    "description": "The format of the extracted web page content. markdown returns content in markdown format. text returns plain text and may increase latency.",
                    "enum": ["markdown", "text"],
                    "default": "markdown",
                    "required": False,
                }
            }
        }
        super().__init__()
        self.api_key = ""
        self.extract_depth = "basic" # basic/advanced
        self.urls = []
        self.format = "markdown"
        self.include_images = False

    def check(self):
        self.check_valid_value(self.extract_depth, "Tavily extract depth should be in 'basic/advanced'", ["basic", "advanced"])
        self.check_valid_value(self.format, "Tavily extract format should be in 'markdown/text'", ["markdown", "text"])

    def get_input_form(self) -> dict[str, dict]:
        return {
            "urls": {
                "name": "URLs",
                "type": "line"
            }
        }

class TavilyExtract(ToolBase, ABC):
    component_name = "TavilyExtract"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        self.tavily_client = TavilyClient(api_key=self._param.api_key)
        last_e = None
        for fld in ["urls", "extract_depth", "format"]:
            if fld not in kwargs:
                kwargs[fld] = getattr(self._param, fld)
        if kwargs.get("urls") and isinstance(kwargs["urls"], str):
            kwargs["urls"] = kwargs["urls"].split(",")
        for _ in range(self._param.max_retries+1):
            try:
                kwargs["include_images"] = False
                res = self.tavily_client.extract(**kwargs)
                self.set_output("json", res["results"])
                return self.output("json")
            except Exception as e:
                last_e = e
                logging.exception(f"Tavily error: {e}")
        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"Tavily error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return "Opened {}—pulling out the main text…".format(self.get_input().get("urls", "-_-!"))