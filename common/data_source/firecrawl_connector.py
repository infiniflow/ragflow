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
"""Firecrawl connector – scrapes/crawls web pages via the Firecrawl API
and yields RAGFlow ``Document`` batches.

Supports two modes controlled by ``crawl_mode``:

* **scrape** (default) – scrape a list of individual URLs.
* **crawl** – start from a single URL and follow links up to ``crawl_limit``.
"""

import hashlib
import logging
import time
from collections.abc import Generator
from datetime import datetime, timezone
from typing import Any, Optional

from retry import retry

from common.data_source.config import INDEX_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    UnexpectedValidationError,
)
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch
from common.data_source.models import Document
from common.data_source.utils import batch_generator

logger = logging.getLogger(__name__)


def _doc_id(url: str) -> str:
    """Deterministic document id from URL."""
    return hashlib.sha256(url.encode()).hexdigest()


def _to_document(url: str, markdown: str, metadata: dict) -> Document:
    """Convert a single scraped result into a RAGFlow ``Document``."""
    title = metadata.get("title") or url
    blob = markdown.encode("utf-8") if markdown else b""
    updated_at = datetime.now(timezone.utc)

    return Document(
        id=_doc_id(url),
        source=DocumentSource.FIRECRAWL,
        semantic_identifier=title,
        extension=".md",
        blob=blob,
        doc_updated_at=updated_at,
        size_bytes=len(blob),
        metadata={
            "url": url,
            "title": title,
            "description": metadata.get("description", ""),
            "status_code": metadata.get("statusCode"),
            "source_url": metadata.get("sourceURL", url),
        },
    )


class FirecrawlConnector(LoadConnector, PollConnector):
    """Firecrawl data-source connector for RAGFlow.

    Arguments:
        urls: Comma-separated URLs to scrape (used when ``crawl_mode`` is ``scrape``).
        crawl_url: Starting URL for crawl mode.
        crawl_mode: ``"scrape"`` or ``"crawl"``.
        crawl_limit: Max pages to crawl (crawl mode only).
        batch_size: Number of documents per yielded batch.
        api_url: Firecrawl API base URL (for self-hosted instances).
    """

    def __init__(
        self,
        urls: str = "",
        crawl_url: str = "",
        crawl_mode: str = "scrape",
        crawl_limit: int = 100,
        batch_size: int = INDEX_BATCH_SIZE,
        api_url: str = "https://api.firecrawl.dev",
    ) -> None:
        self.urls = [u.strip() for u in urls.split(",") if u.strip()] if urls else []
        self.crawl_url = crawl_url.strip() if crawl_url else ""
        self.crawl_mode = crawl_mode
        self.crawl_limit = crawl_limit
        self.batch_size = batch_size
        self.api_url = api_url.rstrip("/")
        self.api_key: Optional[str] = None
        self._client: Any = None  # FirecrawlApp instance

    # ------------------------------------------------------------------
    # Credentials
    # ------------------------------------------------------------------
    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        api_key = credentials.get("firecrawl_api_key", "")
        if not api_key:
            raise ConnectorMissingCredentialError("Firecrawl API key is required.")
        self.api_key = api_key

        # Lazy-import so the dependency is only required at runtime.
        try:
            from firecrawl import FirecrawlApp  # type: ignore[import-untyped]
        except ImportError:
            raise ConnectorMissingCredentialError(
                "firecrawl-py package is not installed.  "
                "Run: pip install firecrawl-py"
            )

        self._client = FirecrawlApp(api_key=self.api_key, api_url=self.api_url)
        return None

    # ------------------------------------------------------------------
    # Scrape helpers
    # ------------------------------------------------------------------
    @retry(tries=3, delay=2, backoff=2, logger=logger)
    def _scrape_url(self, url: str) -> Optional[Document]:
        """Scrape a single URL and return a Document (or None on failure)."""
        try:
            result = self._client.scrape_url(url, params={"formats": ["markdown"]})
            if not result:
                logger.warning("[Firecrawl] Empty response for %s", url)
                return None

            markdown = result.get("markdown", "")
            metadata = result.get("metadata", {})
            if not markdown:
                logger.warning("[Firecrawl] No markdown content for %s", url)
                return None

            return _to_document(url, markdown, metadata)

        except Exception as exc:
            logger.error("[Firecrawl] Failed to scrape %s: %s", url, exc)
            raise

    def _scrape_urls(self, urls: list[str]) -> Generator[list[Document], None, None]:
        """Scrape a list of URLs and yield in batches."""
        docs: list[Document] = []
        for url in urls:
            try:
                doc = self._scrape_url(url)
                if doc is not None:
                    docs.append(doc)
            except Exception:
                logger.error("[Firecrawl] Giving up on %s after retries", url)
                continue

            if len(docs) >= self.batch_size:
                yield docs
                docs = []

        if docs:
            yield docs

    def _crawl_and_yield(self, start_url: str) -> Generator[list[Document], None, None]:
        """Start a crawl job and yield documents as they arrive."""
        try:
            logger.info("[Firecrawl] Starting crawl from %s (limit=%d)", start_url, self.crawl_limit)
            crawl_result = self._client.crawl_url(
                start_url,
                params={"limit": self.crawl_limit, "scrapeOptions": {"formats": ["markdown"]}},
                poll_interval=5,
            )

            if not crawl_result:
                logger.warning("[Firecrawl] Crawl returned empty result for %s", start_url)
                return

            # crawl_url returns the full result with 'data' key when polling completes
            data = crawl_result if isinstance(crawl_result, list) else crawl_result.get("data", [])
            if not data:
                logger.warning("[Firecrawl] No pages found for crawl of %s", start_url)
                return

            docs: list[Document] = []
            for item in data:
                metadata = item.get("metadata", {})
                url = metadata.get("sourceURL", metadata.get("url", start_url))
                markdown = item.get("markdown", "")
                if not markdown:
                    continue

                docs.append(_to_document(url, markdown, metadata))
                if len(docs) >= self.batch_size:
                    yield docs
                    docs = []

            if docs:
                yield docs

        except Exception as exc:
            logger.error("[Firecrawl] Crawl failed for %s: %s", start_url, exc)
            raise

    # ------------------------------------------------------------------
    # LoadConnector interface
    # ------------------------------------------------------------------
    def load_from_state(self) -> Generator[list[Document], None, None]:
        """Load all configured URLs / crawl results."""
        if not self._client:
            raise ConnectorMissingCredentialError("Firecrawl credentials not loaded.")

        if self.crawl_mode == "crawl" and self.crawl_url:
            yield from self._crawl_and_yield(self.crawl_url)
        else:
            all_urls = list(self.urls)
            if self.crawl_url and self.crawl_url not in all_urls:
                all_urls.append(self.crawl_url)
            if not all_urls:
                logger.warning("[Firecrawl] No URLs configured.")
                return
            yield from self._scrape_urls(all_urls)

    # ------------------------------------------------------------------
    # PollConnector interface
    # ------------------------------------------------------------------
    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> Generator[list[Document], None, None]:
        """Re-scrape all configured URLs (web content has no reliable
        modification timestamp, so we always re-fetch)."""
        yield from self.load_from_state()

    # ------------------------------------------------------------------
    # Validation
    # ------------------------------------------------------------------
    def validate_connector_settings(self) -> None:
        if not self._client:
            raise ConnectorMissingCredentialError("Firecrawl credentials not loaded.")

        if self.crawl_mode not in ("scrape", "crawl"):
            raise ConnectorValidationError(
                f"Invalid crawl_mode '{self.crawl_mode}'. Must be 'scrape' or 'crawl'."
            )

        if self.crawl_mode == "scrape" and not self.urls and not self.crawl_url:
            raise ConnectorValidationError("At least one URL is required in scrape mode.")

        if self.crawl_mode == "crawl" and not self.crawl_url:
            raise ConnectorValidationError("crawl_url is required in crawl mode.")

        # Quick connectivity check – scrape a lightweight URL
        try:
            result = self._client.scrape_url("https://httpbin.org/json", params={"formats": ["markdown"]})
            if not result:
                raise ConnectorValidationError("Test scrape returned empty result.")
        except ConnectorValidationError:
            raise
        except Exception as exc:
            raise UnexpectedValidationError(f"Firecrawl connectivity test failed: {exc}")


if __name__ == "__main__":
    import os

    connector = FirecrawlConnector(urls="https://docs.firecrawl.dev")
    connector.load_credentials({"firecrawl_api_key": os.environ.get("FIRECRAWL_API_KEY", "")})
    for batch in connector.load_from_state():
        for doc in batch:
            print(f"  {doc.semantic_identifier} ({doc.size_bytes} bytes)")
