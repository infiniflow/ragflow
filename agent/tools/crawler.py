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
import asyncio
from crawl4ai import AsyncWebCrawler
from agent.tools.base import ToolParamBase, ToolBase



class CrawlerParam(ToolParamBase):
    """
    Define the Crawler component parameters.
    """

    def __init__(self):
        super().__init__()
        self.proxy = None
        self.extract_type = "markdown"

    def check(self):
        self.check_valid_value(self.extract_type, "Type of content from the crawler", ['html', 'markdown', 'content'])


class Crawler(ToolBase, ABC):
    component_name = "Crawler"

    def _run(self, history, **kwargs):
        from api.utils.web_utils import is_valid_url
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not is_valid_url(ans):
            return Crawler.be_output("URL not valid")
        try:
            result = asyncio.run(self.get_web(ans))

            return Crawler.be_output(result)

        except Exception as e:
            return Crawler.be_output(f"An unexpected error occurred: {str(e)}")

    async def get_web(self, url):
        if self.check_if_canceled("Crawler async operation"):
            return

        proxy = self._param.proxy if self._param.proxy else None
        async with AsyncWebCrawler(verbose=True, proxy=proxy) as crawler:
            result = await crawler.arun(
                url=url,
                bypass_cache=True
            )

            if self.check_if_canceled("Crawler async operation"):
                return

            if self._param.extract_type == 'html':
                return result.cleaned_html
            elif self._param.extract_type == 'markdown':
                return result.markdown
            elif self._param.extract_type == 'content':
                return result.extracted_content
            return result.markdown
