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
import asyncio
from crawl4ai import AsyncWebCrawler, BrowserConfig, CrawlerRunConfig, CacheMode
from agent.tools.base import ToolMeta, ToolParamBase, ToolBase
from common.connection_utils import timeout


class CrawlerParam(ToolParamBase):
    """
    Define the Crawler component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "web_crawler",
            "description": "This tool can be used to crawl a web page and return its content as HTML, Markdown, or the extracted main text.",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The absolute URL (including the http:// or https:// scheme) of the web page to crawl.",
                    "default": "{sys.query}",
                    "required": True,
                }
            },
        }
        super().__init__()
        self.proxy = None
        self.extract_type = "markdown"

    def check(self):
        self.check_valid_value(self.extract_type, "Type of content from the crawler", ["html", "markdown", "content"])

    def get_input_form(self) -> dict[str, dict]:
        return {"query": {"name": "URL", "type": "line"}}


class Crawler(ToolBase, ABC):
    component_name = "Crawler"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)))
    def _invoke(self, **kwargs):
        from common.ssrf_guard import assert_url_is_safe, pin_dns_global

        if self.check_if_canceled("Crawler processing"):
            return

        url = kwargs.get("query")
        if not url:
            self.set_output("formalized_content", "")
            return ""

        try:
            _ssrf_hostname, _ssrf_ip = assert_url_is_safe(url)
        except ValueError:
            msg = "URL not valid"
            self.set_output("_ERROR", msg)
            return msg

        try:
            # pin_dns_global is used (not thread-local) because crawl4ai resolves
            # DNS in asyncio executor threads that don't share thread-local state.
            with pin_dns_global(_ssrf_hostname, _ssrf_ip):
                result = asyncio.run(self.get_web(url))

            if self.check_if_canceled("Crawler processing"):
                return

            result = result or ""
            self.set_output("formalized_content", result)
            return result
        except Exception as e:
            if self.check_if_canceled("Crawler processing"):
                return

            logging.exception(f"Crawler error: {e}")
            msg = f"An unexpected error occurred: {str(e)}"
            self.set_output("_ERROR", msg)
            return msg

    async def get_web(self, url):
        if self.check_if_canceled("Crawler async operation"):
            return

        proxy = self._param.proxy if self._param.proxy else None

        browser_config = BrowserConfig(
            verbose=True,
            proxy_config=proxy,
        )

        run_config = CrawlerRunConfig(
            cache_mode=CacheMode.BYPASS,
        )

        async with AsyncWebCrawler(config=browser_config) as crawler:
            result = await crawler.arun(url=url, config=run_config)

            if self.check_if_canceled("Crawler async operation"):
                return

            if self._param.extract_type == "html":
                return result.cleaned_html
            elif self._param.extract_type == "markdown":
                return result.markdown
            elif self._param.extract_type == "content":
                return result.extracted_content
            return result.markdown
