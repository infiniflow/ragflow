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
from api.utils.web_utils import is_valid_url
# from markdownify import markdownify


class CrawlerParam(ComponentParamBase):
    """
    Define the Crawler component parameters.
    """

    def __init__(self):
        super().__init__()
        self.proxy = None
        self.extract_type = "markdown"
    
    def check(self):
        self.check_valid_value(self.extract_type, "Type of content from the crawler", ['html', 'markdown', 'content'])


class Crawler(ComponentBase, ABC):
    component_name = "Crawler"

    def _run(self, history, **kwargs):
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
        from playwright.async_api import async_playwright
        
        async with async_playwright() as p:
            # 启动浏览器
            browser = await p.chromium.launch(headless=True)
            page = await browser.new_page()

            # 打开网页并等待
            await page.goto(url)
            await page.wait_for_load_state("networkidle")  # 等待网络空闲
            await page.wait_for_timeout(2000)  # 额外等待 2 秒

            # 根据 extract_type 获取内容
            if self._param.extract_type == 'html':
                content = await page.content()
            # elif self._param.extract_type == 'markdown':
            #     html = await page.content()
            #     content = markdownify(html)
            elif self._param.extract_type == 'content':
                content = await page.evaluate("() => document.body.innerText")
            else:
                content = await page.content()

            await browser.close()
            return content

    # async def get_web(self, url):
    #     proxy = self._param.proxy if self._param.proxy else None
    #     async with AsyncWebCrawler(verbose=True, proxy=proxy) as crawler:
    #         result = await crawler.arun(
    #             url=url,
    #             bypass_cache=False,
    #             wait_for_js=True,  # 等待 JavaScript 加载完成
    #             timeout=10  # 设置超时时间（秒）
    #         )
            
    #         if self._param.extract_type == 'html':
    #             return result.cleaned_html
    #         elif self._param.extract_type == 'markdown':
    #             return result.markdown
    #         elif self._param.extract_type == 'content':
    #             result.extracted_content
    #         return result.markdown
