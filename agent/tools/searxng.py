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
import requests
from agent.tools.base import ToolMeta, ToolParamBase, ToolBase
from api.utils.api_utils import timeout


class SearXNGParam(ToolParamBase):
    """
    Define the SearXNG component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "searxng_search",
            "description": "SearXNG is a privacy-focused metasearch engine that aggregates results from multiple search engines without tracking users. It provides comprehensive web search capabilities.",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search keywords to execute with SearXNG. The keywords should be the most important words/terms(includes synonyms) from the original request.",
                    "default": "{sys.query}",
                    "required": True
                },
                "searxng_url": {
                    "type": "string",
                    "description": "The base URL of your SearXNG instance (e.g., http://localhost:4000). This is required to connect to your SearXNG server.",
                    "required": False,
                    "default": ""
                }
            }
        }
        super().__init__()
        self.top_n = 10
        self.searxng_url = ""

    def check(self):
        # Keep validation lenient so opening try-run panel won't fail without URL.
        # Coerce top_n to int if it comes as string from UI.
        try:
            if isinstance(self.top_n, str):
                self.top_n = int(self.top_n.strip())
        except Exception:
            pass
        self.check_positive_integer(self.top_n, "Top N")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "name": "Query",
                "type": "line"
            },
            "searxng_url": {
                "name": "SearXNG URL",
                "type": "line",
                "placeholder": "http://localhost:4000"
            }
        }


class SearXNG(ToolBase, ABC):
    component_name = "SearXNG"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12))
    def _invoke(self, **kwargs):
        # Gracefully handle try-run without inputs
        query = kwargs.get("query")
        if not query or not isinstance(query, str) or not query.strip():
            self.set_output("formalized_content", "")
            return ""

        searxng_url = (kwargs.get("searxng_url") or getattr(self._param, "searxng_url", "") or "").strip()
        # In try-run, if no URL configured, just return empty instead of raising
        if not searxng_url:
            self.set_output("formalized_content", "")
            return ""

        last_e = ""
        for _ in range(self._param.max_retries+1):
            try:
                # 构建搜索参数
                search_params = {
                    'q': query,
                    'format': 'json',
                    'categories': 'general',
                    'language': 'auto',
                    'safesearch': 1,
                    'pageno': 1
                }

                # 发送搜索请求
                response = requests.get(
                    f"{searxng_url}/search",
                    params=search_params,
                    timeout=10
                )
                response.raise_for_status()
                
                data = response.json()
                
                # 验证响应数据
                if not data or not isinstance(data, dict):
                    raise ValueError("Invalid response from SearXNG")
                
                results = data.get("results", [])
                if not isinstance(results, list):
                    raise ValueError("Invalid results format from SearXNG")
                
                # 限制结果数量
                results = results[:self._param.top_n]
                
                # 处理搜索结果
                self._retrieve_chunks(results,
                                      get_title=lambda r: r.get("title", ""),
                                      get_url=lambda r: r.get("url", ""),
                                      get_content=lambda r: r.get("content", ""))
                
                self.set_output("json", results)
                return self.output("formalized_content")

            except requests.RequestException as e:
                last_e = f"Network error: {e}"
                logging.exception(f"SearXNG network error: {e}")
                time.sleep(self._param.delay_after_error)
            except Exception as e:
                last_e = str(e)
                logging.exception(f"SearXNG error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", last_e)
            return f"SearXNG error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return """
Keywords: {} 
Searching with SearXNG for relevant results...
                """.format(self.get_input().get("query", "-_-!"))
