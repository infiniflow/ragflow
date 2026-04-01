import hashlib
import ipaddress
import socket
from datetime import datetime, timezone
from email.utils import parsedate_to_datetime
from time import struct_time
from typing import Any
from urllib.parse import urlparse

import bs4
import feedparser
import requests

from common.data_source.config import INDEX_BATCH_SIZE, REQUEST_TIMEOUT_SECONDS, DocumentSource
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import Document, GenerateDocumentsOutput, SecondsSinceUnixEpoch


def _is_private_ip(ip: str) -> bool:
    try:
        ip_obj = ipaddress.ip_address(ip)
        return ip_obj.is_private or ip_obj.is_link_local or ip_obj.is_loopback
    except ValueError:
        return False


def _validate_url_no_ssrf(url: str) -> None:
    parsed = urlparse(url)
    hostname = parsed.hostname
    if not hostname:
        raise ValueError("URL must have a valid hostname")

    try:
        ip = socket.gethostbyname(hostname)
        if _is_private_ip(ip):
            raise ValueError(f"URL resolves to private/internal IP address: {ip}")
    except socket.gaierror as e:
        raise ValueError(f"Failed to resolve hostname: {hostname}") from e


class RSSConnector(LoadConnector, PollConnector):
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

    def _validate_feed_url(self) -> None:
        if not self.feed_url:
            raise ValueError("feed_url is required")

        parsed = urlparse(self.feed_url)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ValueError("feed_url must be a valid http or https URL")

        _validate_url_no_ssrf(self.feed_url)

    def _read_feed(self, require_entries: bool) -> Any:
        if self._cached_feed is not None:
            if require_entries and not self._cached_feed.entries:
                raise ValueError("RSS feed contains no entries")
            return self._cached_feed

        self._validate_feed_url()

        response = requests.get(self.feed_url, timeout=REQUEST_TIMEOUT_SECONDS, allow_redirects=True)
        response.raise_for_status()

        final_url = getattr(response, "url", self.feed_url)
        if final_url != self.feed_url and urlparse(final_url).hostname:
            _validate_url_no_ssrf(final_url)

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
        stable_key = (entry.get("id") or link or title or self.feed_url).strip()
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
            id=f"rss:{hashlib.md5(stable_key.encode('utf-8')).hexdigest()}",
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
