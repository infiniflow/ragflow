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
import logging
import os
import requests
from crawl4ai import AsyncWebCrawler
from agent.tools.base import ToolParamBase, ToolBase

_DEFAULT_REMOTE_TIMEOUT_S = 120


class CrawlerParam(ToolParamBase):
    """
    Define the Crawler component parameters.
    """

    def __init__(self):
        super().__init__()
        self.proxy = None
        self.extract_type = "markdown"

    def check(self):
        self.check_valid_value(self.extract_type, "Type of content from the crawler", ["html", "markdown", "content"])


class Crawler(ToolBase, ABC):
    component_name = "Crawler"

    def _run(self, history, **kwargs):
        from common.ssrf_guard import assert_url_is_safe, pin_dns_global

        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        try:
            _ssrf_hostname, _ssrf_ip = assert_url_is_safe(ans)
        except ValueError:
            return Crawler.be_output("URL not valid")
        try:
            server_url = (os.environ.get("CRAWL4AI_SERVER_URL", "") or "").rstrip("/")
            if server_url:
                logging.info("[Crawler] offloading to remote crawl4ai server: %s", server_url)
                return Crawler.be_output(self._fetch_remote(server_url, ans))

            # pin_dns_global is used (not thread-local) because crawl4ai resolves
            # DNS in asyncio executor threads that don't share thread-local state.
            with pin_dns_global(_ssrf_hostname, _ssrf_ip):
                result = asyncio.run(self.get_web(ans))

            return Crawler.be_output(result)

        except Exception as e:
            return Crawler.be_output(f"An unexpected error occurred: {str(e)}")

    def _fetch_remote(self, server_url: str, url: str):
        """Hand the crawl off to a standalone crawl4ai HTTP server.

        SSRF validation has already run locally in ``_run`` before this is
        called. The remote server resolves DNS itself, so ``pin_dns_global``
        is not applied here.
        """
        timeout_raw = os.environ.get("CRAWL4AI_REQUEST_TIMEOUT", str(_DEFAULT_REMOTE_TIMEOUT_S))
        try:
            timeout_s = int(timeout_raw)
            if timeout_s <= 0:
                raise ValueError
        except (TypeError, ValueError):
            logging.warning(
                "[Crawler] invalid CRAWL4AI_REQUEST_TIMEOUT=%r, falling back to %ds",
                timeout_raw, _DEFAULT_REMOTE_TIMEOUT_S,
            )
            timeout_s = _DEFAULT_REMOTE_TIMEOUT_S

        resp = requests.post(
            f"{server_url}/crawl",
            json={"urls": [url]},
            timeout=timeout_s,
        )
        resp.raise_for_status()
        payload = resp.json()
        results = payload.get("results") or []
        if not results:
            logging.warning("[Crawler] remote crawl4ai server returned no results for %s", url)
            return ""
        r = results[0]
        if self._param.extract_type == "html":
            return r.get("cleaned_html") or ""
        if self._param.extract_type == "content":
            return r.get("extracted_content")
        # markdown (default): newer crawl4ai returns a MarkdownGenerationResult-shaped
        # dict; fall back to the bare string for older server versions.
        md = r.get("markdown")
        if isinstance(md, dict):
            return md.get("raw_markdown") or md.get("fit_markdown") or ""
        return md or ""

    async def get_web(self, url):
        if self.check_if_canceled("Crawler async operation"):
            return

        proxy = self._param.proxy if self._param.proxy else None
        async with AsyncWebCrawler(verbose=True, proxy=proxy) as crawler:
            result = await crawler.arun(url=url, bypass_cache=True)

            if self.check_if_canceled("Crawler async operation"):
                return

            if self._param.extract_type == "html":
                return result.cleaned_html
            elif self._param.extract_type == "markdown":
                return result.markdown
            elif self._param.extract_type == "content":
                return result.extracted_content
            return result.markdown
