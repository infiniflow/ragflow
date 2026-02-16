from __future__ import annotations

import json
import logging
import time
from datetime import datetime, timezone
from typing import Any, Generator

import requests

from common.data_source.config import INDEX_BATCH_SIZE, REQUEST_TIMEOUT_SECONDS, DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import Document, SecondsSinceUnixEpoch


class FirecrawlConnector(LoadConnector, PollConnector):
    """Firecrawl connector using the native crawl API."""

    def __init__(
        self,
        api_url: str = "https://api.firecrawl.dev",
        start_url: str = "",
        max_pages: int | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
        include_paths: list[str] | None = None,
        exclude_paths: list[str] | None = None,
        poll_interval_seconds: int = 2,
        crawl_timeout_seconds: int = 300,
    ) -> None:
        self.api_url = (api_url or "https://api.firecrawl.dev").rstrip("/")
        self.start_url = (start_url or "").strip()
        self.max_pages = max_pages
        self.batch_size = max(1, int(batch_size or INDEX_BATCH_SIZE))
        self.include_paths = include_paths or []
        self.exclude_paths = exclude_paths or []
        self.poll_interval_seconds = max(1, int(poll_interval_seconds or 2))
        self.crawl_timeout_seconds = max(30, int(crawl_timeout_seconds or 300))
        self._api_key: str | None = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        api_key = (credentials or {}).get("firecrawl_api_key")
        if not api_key:
            raise ConnectorMissingCredentialError("Missing firecrawl_api_key in credentials")
        self._api_key = api_key
        return None

    def validate_connector_settings(self) -> None:
        if not self.start_url:
            raise ValueError("Firecrawl start_url is required")
        if not self.api_url.startswith("http://") and not self.api_url.startswith("https://"):
            raise ValueError("Firecrawl api_url must be an HTTP(S) URL")

    def _headers(self) -> dict[str, str]:
        if not self._api_key:
            raise ConnectorMissingCredentialError("Firecrawl credentials not loaded")
        return {
            "Authorization": f"Bearer {self._api_key}",
            "Content-Type": "application/json",
        }

    def _request_json(self, method: str, path: str, payload: dict[str, Any] | None = None) -> dict[str, Any]:
        url = f"{self.api_url}{path}"
        attempts = 4
        last_error: Exception | None = None

        for attempt in range(1, attempts + 1):
            try:
                response = requests.request(
                    method,
                    url,
                    headers=self._headers(),
                    json=payload,
                    timeout=REQUEST_TIMEOUT_SECONDS,
                )

                if response.status_code == 429:
                    retry_after = response.headers.get("Retry-After")
                    delay = float(retry_after) if retry_after else min(2**attempt, 15)
                    logging.warning("Firecrawl rate limited (429). Retrying in %ss", delay)
                    time.sleep(delay)
                    continue

                response.raise_for_status()
                data = response.json()
                if not isinstance(data, dict):
                    raise ValueError("Firecrawl returned non-object JSON response")
                return data
            except requests.Timeout as ex:
                last_error = ex
                if attempt == attempts:
                    raise TimeoutError("Timed out while communicating with Firecrawl") from ex
                time.sleep(min(2**attempt, 10))
            except requests.RequestException as ex:
                last_error = ex
                status_code = getattr(getattr(ex, "response", None), "status_code", None)
                body = getattr(getattr(ex, "response", None), "text", "")
                raise RuntimeError(f"Firecrawl request failed ({status_code}): {body[:300]}") from ex
            except (json.JSONDecodeError, ValueError) as ex:
                last_error = ex
                raise ValueError(f"Malformed Firecrawl response: {ex}") from ex

        raise RuntimeError(f"Firecrawl request failed after retries: {last_error}")

    @staticmethod
    def _normalize_page(page: dict[str, Any]) -> tuple[str, str, str, str]:
        url = str(page.get("url") or page.get("sourceURL") or "").strip()
        metadata = page.get("metadata") if isinstance(page.get("metadata"), dict) else {}
        title = str(metadata.get("title") or page.get("title") or url or "untitled")

        markdown = page.get("markdown")
        if markdown is None:
            markdown = page.get("content")
        if markdown is None:
            markdown = ""
        if not isinstance(markdown, str):
            markdown = str(markdown)

        updated_at = (
            page.get("updatedAt")
            or page.get("modifiedAt")
            or metadata.get("updatedAt")
            or datetime.now(timezone.utc).isoformat()
        )

        return url, title, markdown, str(updated_at)

    def _start_crawl(self) -> str:
        payload: dict[str, Any] = {
            "url": self.start_url,
            "scrapeOptions": {"formats": ["markdown"]},
        }
        if self.max_pages:
            payload["limit"] = self.max_pages
        if self.include_paths:
            payload["includePaths"] = self.include_paths
        if self.exclude_paths:
            payload["excludePaths"] = self.exclude_paths

        response = self._request_json("POST", "/v1/crawl", payload)
        if response.get("success") is False:
            raise RuntimeError(f"Firecrawl crawl start failed: {response.get('error') or response}")

        crawl_id = response.get("id") or response.get("jobId") or response.get("crawlId")
        if not crawl_id:
            raise ValueError("Firecrawl crawl response did not include an id")

        return str(crawl_id)

    def _wait_for_crawl(self, crawl_id: str) -> list[dict[str, Any]]:
        started = time.time()
        while True:
            if time.time() - started > self.crawl_timeout_seconds:
                raise TimeoutError(f"Firecrawl crawl timed out after {self.crawl_timeout_seconds}s")

            response = self._request_json("GET", f"/v1/crawl/{crawl_id}")
            if response.get("success") is False:
                raise RuntimeError(f"Firecrawl crawl failed: {response.get('error') or response}")

            status = str(response.get("status") or "").lower()
            if status in {"completed", "done", "success"}:
                data = response.get("data")
                if not isinstance(data, list):
                    raise ValueError("Firecrawl crawl result is malformed: 'data' is not a list")
                return data

            if status in {"failed", "error", "cancelled"}:
                raise RuntimeError(f"Firecrawl crawl ended with status '{status}': {response.get('error')}")

            time.sleep(self.poll_interval_seconds)

    def _build_batches(self, pages: list[dict[str, Any]]) -> Generator[list[Document], None, None]:
        now = datetime.now(timezone.utc)
        batch: list[Document] = []
        for idx, page in enumerate(pages):
            if not isinstance(page, dict):
                logging.warning("Skipping malformed Firecrawl page payload at index %s", idx)
                continue

            page_url, title, markdown, _updated_at_raw = self._normalize_page(page)
            if not markdown.strip():
                continue

            doc_id = page_url or f"{self.start_url}#page-{idx}"
            blob = markdown.encode("utf-8", errors="replace")
            doc = Document(
                id=f"firecrawl:{doc_id}",
                source=DocumentSource.FIRECRAWL,
                semantic_identifier=title,
                extension="md",
                blob=blob,
                doc_updated_at=now,
                size_bytes=len(blob),
                metadata={
                    "url": page_url,
                    "title": title,
                    "source": "firecrawl",
                },
            )
            batch.append(doc)
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def load_from_state(self) -> Generator[list[Document], None, None]:
        self.validate_connector_settings()
        crawl_id = self._start_crawl()
        pages = self._wait_for_crawl(crawl_id)
        yield from self._build_batches(pages)

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Generator[list[Document], None, None]:
        start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
        end_dt = datetime.fromtimestamp(end, tz=timezone.utc)

        for docs in self.load_from_state():
            filtered = [doc for doc in docs if start_dt <= doc.doc_updated_at < end_dt]
            if filtered:
                yield filtered
