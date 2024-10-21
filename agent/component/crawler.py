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
from agent.component.base import ComponentBase, ComponentParamBase

class CrawlerParam(ComponentParamBase):
    """
    Define the Crawler component parameters.
    """

    def __init__(self):
        super().__init__()
    
    def check(self):
        return True


class Crawler(ComponentBase, ABC):
    component_name = "Crawler"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return Crawler.be_output("")
        try:
            result = asyncio.run(self.get_web(ans))

            return Crawler.be_output(result)
            
        except Exception as e:
            return Crawler.be_output(f"An unexpected error occurred: {str(e)}")


    async def get_web(self, url):
        proxy = self._param.proxy if self._param.proxy else None
        async with AsyncWebCrawler(verbose=True, proxy=proxy) as crawler:
            result = await crawler.arun(
                url=url,
                bypass_cache=True
            )
            
            match self._param.extract_type:
                case 'html':
                    return result.cleaned_html
                case 'markdown':
                    return result.markdown
                case 'content':
                    return result.extracted_content
                case _:
                    return result.markdown
                # print(result.markdown)
            


    