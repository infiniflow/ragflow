import hashlib
from datetime import datetime, timezone
from email.utils import parsedate_to_datetime
from time import struct_time
from typing import Any
from urllib.parse import urljoin, urlparse

import bs4
import feedparser
import requests

from common.data_source.config import INDEX_BATCH_SIZE, REQUEST_TIMEOUT_SECONDS, DocumentSource
from common.data_source.interfaces import LoadConnector, PollConnector, SlimConnectorWithPermSync
from common.data_source.models import (
    Document,
    GenerateDocumentsOutput,
    GenerateSlimDocumentOutput,
    SecondsSinceUnixEpoch,
    SlimDocument,
)
from common.ssrf_guard import assert_url_is_safe, pin_dns as _pin_dns

_MAX_REDIRECTS = 10


class RSSConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    def __init__(self, feed_url: str, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.feed_url = feed_url.strip()
        self.batch_size = batch_size
        self.credentials: dict[str, Any] = {}
        self._cached_feed: Any | None = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self.credentials = credentials or {}
        return None

    def validate_connector_settings(self) -> None:
        self._validate_feed_url()
        if self.batch_size < 1:
            raise ValueError("batch_size must be greater than 0")
        self._read_feed(require_entries=True)

    def load_from_state(self) -> GenerateDocumentsOutput:
        yield from self._load_entries()

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        yield from self._load_entries(start=start, end=end)

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> GenerateSlimDocumentOutput:
        del callback

        feed = self._read_feed(require_entries=False)
        batch: list[SlimDocument] = []

        for entry in feed.entries:
            batch.append(SlimDocument(id=self._build_document_id(entry)))

            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def _load_entries(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> GenerateDocumentsOutput:
        feed = self._read_feed(require_entries=False)
        batch: list[Document] = []

        for entry in feed.entries:
            updated_at = self._resolve_entry_time(entry)
            ts = updated_at.timestamp()

            if start is not None and ts <= start:
                continue
            if end is not None and ts > end:
                continue

            batch.append(self._build_document(entry, updated_at))

            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def _validate_feed_url(self) -> tuple[str, str]:
        """Validate ``self.feed_url`` and return ``(hostname, resolved_ip)``."""
        if not self.feed_url:
            raise ValueError("feed_url is required")

        parsed = urlparse(self.feed_url)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ValueError("feed_url must be a valid http or https URL")

        return assert_url_is_safe(self.feed_url)

    def _read_feed(self, require_entries: bool) -> Any:
        if self._cached_feed is not None:
            if require_entries and not self._cached_feed.entries:
                raise ValueError("RSS feed contains no entries")
            return self._cached_feed

        # Validate once to get the pinned IP for the initial request.
        current_hostname, current_ip = self._validate_feed_url()
        current_url = self.feed_url

        # Follow redirects manually: each hop is validated and DNS-pinned
        # *before* the connection is made, closing the TOCTOU rebinding window
        # that existed when allow_redirects=True was used with post-hoc checks.
        response: requests.Response | None = None
        for _ in range(_MAX_REDIRECTS + 1):
            with _pin_dns(current_hostname, current_ip):
                response = requests.get(
                    current_url,
                    timeout=REQUEST_TIMEOUT_SECONDS,
                    allow_redirects=False,
                )

            if response.status_code not in (301, 302, 303, 307, 308):
                break

            location = response.headers.get("Location")
            if not location:
                break  # broken redirect; let raise_for_status() handle it

            redirect_url = urljoin(current_url, location)
            # Validate redirect target before following it.
            current_hostname, current_ip = assert_url_is_safe(redirect_url)
            current_url = redirect_url
        else:
            raise ValueError(f"Exceeded {_MAX_REDIRECTS} redirects fetching {self.feed_url!r}")

        response.raise_for_status()

        feed = feedparser.parse(response.content)
        if getattr(feed, "bozo", False) and not feed.entries:
            error = getattr(feed, "bozo_exception", None)
            if error:
                raise ValueError(f"Failed to parse RSS feed: {error}") from error
            raise ValueError("Failed to parse RSS feed")
        if require_entries and not feed.entries:
            raise ValueError("RSS feed contains no entries")

        self._cached_feed = feed
        return feed

    def _build_document(self, entry: Any, updated_at: datetime) -> Document:
        link = (entry.get("link") or "").strip()
        title = (entry.get("title") or "").strip()
        stable_key = self._resolve_stable_key(entry)
        semantic_identifier = title or link or stable_key
        content = self._build_content(entry, semantic_identifier)
        blob = content.encode("utf-8")

        metadata: dict[str, Any] = {"feed_url": self.feed_url}
        if link:
            metadata["link"] = link
        if entry.get("author"):
            metadata["author"] = entry.get("author")

        categories = []
        for tag in entry.get("tags", []):
            if not isinstance(tag, dict):
                continue
            term = tag.get("term")
            if isinstance(term, str) and term:
                categories.append(term)
        if categories:
            metadata["categories"] = categories

        return Document(
            id=self._build_document_id(entry),
            source=DocumentSource.RSS,
            semantic_identifier=semantic_identifier,
            extension=".txt",
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            metadata=metadata,
        )

    def _build_content(self, entry: Any, semantic_identifier: str) -> str:
        parts = [semantic_identifier]
        content_blocks = entry.get("content") or []

        for block in content_blocks:
            value = block.get("value") if isinstance(block, dict) else None
            normalized = self._normalize_text(value)
            if normalized:
                parts.append(normalized)

        if len(parts) == 1:
            fallback = entry.get("summary") or entry.get("description") or ""
            normalized = self._normalize_text(fallback)
            if normalized:
                parts.append(normalized)

        return "\n\n".join(part for part in parts if part).strip()

    def _build_document_id(self, entry: Any) -> str:
        stable_key = self._resolve_stable_key(entry)
        return f"rss:{hashlib.md5(stable_key.encode('utf-8')).hexdigest()}"

    def _resolve_stable_key(self, entry: Any) -> str:
        link = (entry.get("link") or "").strip()
        title = (entry.get("title") or "").strip()
        return (entry.get("id") or link or title or self.feed_url).strip()

    def _resolve_entry_time(self, entry: Any) -> datetime:
        for field in ("updated_parsed", "published_parsed"):
            value = entry.get(field)
            if value:
                return self._struct_time_to_utc(value)

        for field in ("updated", "published"):
            value = entry.get(field)
            if isinstance(value, str) and value.strip():
                try:
                    parsed = parsedate_to_datetime(value)
                except (TypeError, ValueError, IndexError):
                    continue
                if parsed.tzinfo is None:
                    parsed = parsed.replace(tzinfo=timezone.utc)
                return parsed.astimezone(timezone.utc)

        return datetime.now(timezone.utc)

    @staticmethod
    def _normalize_text(value: Any) -> str:
        if not isinstance(value, str):
            return ""
        return bs4.BeautifulSoup(value, "html.parser").get_text("\n", strip=True)

    @staticmethod
    def _struct_time_to_utc(value: struct_time | tuple[Any, ...]) -> datetime:
        dt = datetime(*value[:6], tzinfo=timezone.utc)
        return dt.astimezone(timezone.utc)
