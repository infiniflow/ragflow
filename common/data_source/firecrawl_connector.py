#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""
Firecrawl connector for RAGFlow.

This connector integrates Firecrawl's web scraping capabilities into RAGFlow,
allowing users to import web content directly into their RAG workflows.

Firecrawl API documentation: https://docs.firecrawl.dev/
"""

import logging
import time
from collections.abc import Generator
from datetime import datetime, timezone
from typing import Any, Optional
from urllib.parse import urlparse

import requests
from retry import retry

from common.data_source.config import (
    INDEX_BATCH_SIZE,
    DocumentSource,
)
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    UnexpectedValidationError,
)
from common.data_source.interfaces import (
    LoadConnector,
    PollConnector,
    SecondsSinceUnixEpoch,
)
from common.data_source.models import (
    Document,
    GenerateDocumentsOutput,
)
from common.data_source.utils import batch_generator


# Firecrawl API base URL
FIRECRAWL_API_BASE = "https://api.firecrawl.dev/v1"

# Rate limiting configuration
RATE_LIMIT_DELAY = 1.0  # Delay between requests in seconds
MAX_RETRIES = 3
RETRY_DELAY = 2  # Initial delay for retries

# Crawl job polling configuration
CRAWL_POLL_INTERVAL = 5  # Seconds between status checks
CRAWL_MAX_WAIT_TIME = 3600  # Maximum time to wait for crawl completion (1 hour)


class FirecrawlConnector(LoadConnector, PollConnector):
    """Firecrawl connector for web scraping and content import.

    This connector supports:
    - Single URL scraping
    - Batch/crawl operations for multiple pages
    - Rate limit handling with automatic retries
    - Error handling for failed requests

    Arguments:
        urls (list[str] | None): List of URLs to scrape
        crawl_url (str | None): Base URL for crawl operation
        max_depth (int): Maximum crawl depth (default: 2)
        include_paths (list[str] | None): URL patterns to include in crawl
        exclude_paths (list[str] | None): URL patterns to exclude from crawl
        batch_size (int): Number of documents per batch
        scrape_options (dict | None): Additional options for scraping
    """

    def __init__(
        self,
        urls: Optional[list[str]] = None,
        crawl_url: Optional[str] = None,
        max_depth: int = 2,
        include_paths: Optional[list[str]] = None,
        exclude_paths: Optional[list[str]] = None,
        batch_size: int = INDEX_BATCH_SIZE,
        scrape_options: Optional[dict] = None,
    ) -> None:
        self.urls = urls or []
        self.crawl_url = crawl_url
        self.max_depth = max_depth
        self.include_paths = include_paths or []
        self.exclude_paths = exclude_paths or []
        self.batch_size = batch_size
        self.scrape_options = scrape_options or {}
        self.api_key: Optional[str] = None
        self.headers: dict[str, str] = {
            "Content-Type": "application/json",
        }
        self._last_request_time: float = 0
        self._scraped_urls: set[str] = set()

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load Firecrawl API credentials.

        Args:
            credentials: Dictionary containing 'firecrawl_api_key'

        Returns:
            None (credentials are applied to headers)

        Raises:
            ConnectorMissingCredentialError: If API key is missing
        """
        api_key = credentials.get("firecrawl_api_key")
        if not api_key:
            raise ConnectorMissingCredentialError("Firecrawl API key is required")

        self.api_key = api_key
        self.headers["Authorization"] = f"Bearer {api_key}"
        return None

    def _rate_limit(self) -> None:
        """Apply rate limiting between requests."""
        elapsed = time.time() - self._last_request_time
        if elapsed < RATE_LIMIT_DELAY:
            time.sleep(RATE_LIMIT_DELAY - elapsed)
        self._last_request_time = time.time()

    @retry(tries=MAX_RETRIES, delay=RETRY_DELAY, backoff=2, logger=logging)
    def _scrape_url(self, url: str) -> dict[str, Any] | None:
        """Scrape a single URL using Firecrawl API.

        Args:
            url: The URL to scrape

        Returns:
            Scraped content as a dictionary, or None if failed
        """
        self._rate_limit()

        logging.info(f"[Firecrawl] Scraping URL: {url}")

        payload = {
            "url": url,
            "formats": ["markdown", "html"],
            **self.scrape_options,
        }

        try:
            response = requests.post(
                f"{FIRECRAWL_API_BASE}/scrape",
                headers=self.headers,
                json=payload,
                timeout=60,
            )

            if response.status_code == 429:
                # Rate limited - extract retry delay from headers if available
                retry_after = int(response.headers.get("Retry-After", RETRY_DELAY * 2))
                logging.warning(f"[Firecrawl] Rate limited. Waiting {retry_after}s...")
                time.sleep(retry_after)
                raise Exception("Rate limited, retrying...")

            response.raise_for_status()
            data = response.json()

            if data.get("success"):
                return data.get("data", {})
            else:
                logging.warning(f"[Firecrawl] Scrape failed for {url}: {data.get('error', 'Unknown error')}")
                return None

        except requests.exceptions.HTTPError as e:
            status_code = e.response.status_code if e.response else None
            if status_code == 401:
                raise CredentialExpiredError("Firecrawl API key is invalid or expired")
            elif status_code == 403:
                logging.warning(f"[Firecrawl] Access forbidden for {url}")
                return None
            elif status_code == 404:
                logging.warning(f"[Firecrawl] URL not found: {url}")
                return None
            else:
                logging.error(f"[Firecrawl] HTTP error scraping {url}: {e}")
                raise

        except Exception as e:
            logging.error(f"[Firecrawl] Error scraping {url}: {e}")
            raise

    @retry(tries=MAX_RETRIES, delay=RETRY_DELAY, backoff=2, logger=logging)
    def _start_crawl(self, url: str) -> str | None:
        """Start a crawl job for a URL.

        Args:
            url: Base URL to start crawling from

        Returns:
            Crawl job ID, or None if failed
        """
        self._rate_limit()

        logging.info(f"[Firecrawl] Starting crawl for: {url}")

        payload = {
            "url": url,
            "maxDepth": self.max_depth,
            "limit": 100,  # Maximum pages to crawl
        }

        if self.include_paths:
            payload["includePaths"] = self.include_paths
        if self.exclude_paths:
            payload["excludePaths"] = self.exclude_paths

        try:
            response = requests.post(
                f"{FIRECRAWL_API_BASE}/crawl",
                headers=self.headers,
                json=payload,
                timeout=60,
            )

            if response.status_code == 429:
                retry_after = int(response.headers.get("Retry-After", RETRY_DELAY * 2))
                logging.warning(f"[Firecrawl] Rate limited. Waiting {retry_after}s...")
                time.sleep(retry_after)
                raise Exception("Rate limited, retrying...")

            response.raise_for_status()
            data = response.json()

            if data.get("success"):
                job_id = data.get("id")
                logging.info(f"[Firecrawl] Crawl job started: {job_id}")
                return job_id
            else:
                logging.error(f"[Firecrawl] Failed to start crawl: {data.get('error', 'Unknown error')}")
                return None

        except requests.exceptions.HTTPError as e:
            status_code = e.response.status_code if e.response else None
            if status_code == 401:
                raise CredentialExpiredError("Firecrawl API key is invalid or expired")
            else:
                logging.error(f"[Firecrawl] HTTP error starting crawl: {e}")
                raise

        except Exception as e:
            logging.error(f"[Firecrawl] Error starting crawl: {e}")
            raise

    def _get_crawl_status(self, job_id: str) -> dict[str, Any]:
        """Get the status of a crawl job.

        Args:
            job_id: The crawl job ID

        Returns:
            Crawl status and results
        """
        self._rate_limit()

        try:
            response = requests.get(
                f"{FIRECRAWL_API_BASE}/crawl/{job_id}",
                headers=self.headers,
                timeout=30,
            )
            response.raise_for_status()
            return response.json()

        except Exception as e:
            logging.error(f"[Firecrawl] Error checking crawl status: {e}")
            return {"status": "error", "error": str(e)}

    def _wait_for_crawl(self, job_id: str) -> list[dict[str, Any]]:
        """Wait for a crawl job to complete and return results.

        Args:
            job_id: The crawl job ID

        Returns:
            List of scraped page data
        """
        start_time = time.time()
        results = []

        while time.time() - start_time < CRAWL_MAX_WAIT_TIME:
            status = self._get_crawl_status(job_id)

            if status.get("status") == "completed":
                results = status.get("data", [])
                logging.info(f"[Firecrawl] Crawl completed. Retrieved {len(results)} pages.")
                break
            elif status.get("status") == "failed":
                logging.error(f"[Firecrawl] Crawl failed: {status.get('error', 'Unknown error')}")
                break
            elif status.get("status") == "cancelled":
                logging.warning("[Firecrawl] Crawl was cancelled")
                break
            else:
                # Still in progress
                completed = status.get("completed", 0)
                total = status.get("total", 0)
                logging.info(f"[Firecrawl] Crawl in progress: {completed}/{total} pages")
                time.sleep(CRAWL_POLL_INTERVAL)

        return results

    def _create_document(self, scraped_data: dict[str, Any]) -> Document | None:
        """Create a Document from scraped data.

        Args:
            scraped_data: Dictionary containing scraped content

        Returns:
            Document object, or None if data is invalid
        """
        url = scraped_data.get("url") or scraped_data.get("sourceURL", "")
        if not url:
            logging.warning("[Firecrawl] Scraped data missing URL, skipping")
            return None

        # Skip if already processed
        if url in self._scraped_urls:
            return None
        self._scraped_urls.add(url)

        # Extract content - prefer markdown, fall back to HTML
        content = scraped_data.get("markdown") or scraped_data.get("content", "")
        if not content:
            html_content = scraped_data.get("html", "")
            if html_content:
                # Use HTML as fallback
                content = html_content
            else:
                logging.warning(f"[Firecrawl] No content found for {url}, skipping")
                return None

        # Extract metadata
        metadata = scraped_data.get("metadata", {})
        title = metadata.get("title") or metadata.get("ogTitle") or urlparse(url).path or "Untitled"
        description = metadata.get("description") or metadata.get("ogDescription", "")

        # Build semantic identifier
        parsed_url = urlparse(url)
        semantic_id = f"{parsed_url.netloc}{parsed_url.path}"
        if len(semantic_id) > 200:
            semantic_id = semantic_id[:200]

        # Create document blob
        full_text = f"# {title}\n\n"
        if description:
            full_text += f"{description}\n\n"
        full_text += content
        blob = full_text.encode("utf-8")

        # Parse timestamp from metadata if available
        doc_updated_at = datetime.now(timezone.utc)
        if "publishedTime" in metadata:
            try:
                doc_updated_at = datetime.fromisoformat(metadata["publishedTime"].replace("Z", "+00:00"))
            except (ValueError, TypeError):
                pass

        return Document(
            id=url,  # Use URL as document ID
            blob=blob,
            source=DocumentSource.FIRECRAWL,
            semantic_identifier=semantic_id,
            extension=".md",
            size_bytes=len(blob),
            doc_updated_at=doc_updated_at,
            metadata={
                "url": url,
                "title": title,
                "description": description,
                **{k: v for k, v in metadata.items() if isinstance(v, (str, int, float, bool))},
            },
        )

    def _scrape_urls(self) -> Generator[Document, None, None]:
        """Scrape individual URLs from the configured list.

        Yields:
            Document objects for each successfully scraped URL
        """
        for url in self.urls:
            url = url.strip()
            if not url:
                continue

            scraped_data = self._scrape_url(url)
            if scraped_data:
                doc = self._create_document(scraped_data)
                if doc:
                    yield doc

    def _crawl_and_scrape(self) -> Generator[Document, None, None]:
        """Start a crawl job and yield documents from results.

        Yields:
            Document objects for each page in the crawl
        """
        if not self.crawl_url:
            return

        job_id = self._start_crawl(self.crawl_url)
        if not job_id:
            logging.error("[Firecrawl] Failed to start crawl job")
            return

        results = self._wait_for_crawl(job_id)
        for scraped_data in results:
            doc = self._create_document(scraped_data)
            if doc:
                yield doc

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Load all documents from configured URLs/crawl.

        This method handles both single URL scraping and batch crawl operations.

        Yields:
            Batches of Document objects
        """
        logging.info("[Firecrawl] Starting full load")

        def generate_documents() -> Generator[Document, None, None]:
            # First, scrape individual URLs
            if self.urls:
                logging.info(f"[Firecrawl] Scraping {len(self.urls)} individual URLs")
                yield from self._scrape_urls()

            # Then, perform crawl if configured
            if self.crawl_url:
                logging.info(f"[Firecrawl] Starting crawl from: {self.crawl_url}")
                yield from self._crawl_and_scrape()

        yield from batch_generator(generate_documents(), self.batch_size)

    def poll_source(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> GenerateDocumentsOutput:
        """Poll for updated content.

        Note: Firecrawl doesn't have built-in change detection, so this
        performs a full rescrape. In the future, this could be enhanced
        with caching and comparison logic.

        Args:
            start: Start timestamp (unused for now)
            end: End timestamp (unused for now)

        Yields:
            Batches of Document objects
        """
        logging.info(f"[Firecrawl] Polling for updates (start={start}, end={end})")
        # For now, just do a full rescrape
        # Future enhancement: implement change detection
        yield from self.load_from_state()

    def validate_connector_settings(self) -> None:
        """Validate Firecrawl connector settings and credentials.

        Raises:
            ConnectorMissingCredentialError: If credentials are not loaded
            ConnectorValidationError: If validation fails
        """
        if not self.headers.get("Authorization"):
            raise ConnectorMissingCredentialError("Firecrawl API key not loaded")

        if not self.urls and not self.crawl_url:
            raise ConnectorValidationError("At least one URL or crawl URL must be specified")

        # Test API connectivity
        try:
            response = requests.get(
                f"{FIRECRAWL_API_BASE}/",
                headers=self.headers,
                timeout=10,
            )
            # Any 2xx or 4xx response (except 401) indicates the API is reachable
            if response.status_code == 401:
                raise CredentialExpiredError("Firecrawl API key is invalid")

        except requests.exceptions.ConnectionError:
            raise ConnectorValidationError("Unable to connect to Firecrawl API")
        except requests.exceptions.Timeout:
            raise ConnectorValidationError("Firecrawl API connection timed out")
        except CredentialExpiredError:
            raise
        except Exception as e:
            raise UnexpectedValidationError(f"Unexpected error validating Firecrawl settings: {e}")


if __name__ == "__main__":
    import os

    # Test the connector
    connector = FirecrawlConnector(
        urls=["https://example.com"],
    )
    connector.load_credentials({"firecrawl_api_key": os.environ.get("FIRECRAWL_API_KEY", "")})

    try:
        connector.validate_connector_settings()
        for doc_batch in connector.load_from_state():
            for doc in doc_batch:
                print(f"Document: {doc.semantic_identifier} ({doc.size_bytes} bytes)")
    except Exception as e:
        print(f"Error: {e}")
