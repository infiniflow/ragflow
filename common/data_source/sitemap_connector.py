import hashlib
import logging
import re
from datetime import datetime, timezone
from collections.abc import Iterator
from typing import Any
from urllib.parse import urljoin, urlparse
from xml.etree import ElementTree as ET

import requests

from common.data_source.config import (
    REQUEST_TIMEOUT_SECONDS,
    DocumentSource,
)

from common.data_source.interfaces import LoadConnector, PollConnector, SlimConnectorWithPermSync
from common.data_source.models import (
    Document,
    GenerateDocumentsOutput,
    GenerateSlimDocumentOutput,
    SecondsSinceUnixEpoch,
    SlimDocument,
)
from common.ssrf_guard import assert_url_is_safe

_SITEMAP_NS = "http://www.sitemaps.org/schemas/sitemap/0.9"
_MAX_REDIRECTS = 10
_MAX_SITEMAP_DEPTH = 5

logger = logging.getLogger(__name__)


class SitemapConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    """Connector that ingests web pages listed in a sitemap.xml.

    Supports:
    - Standard urlset sitemaps
    - Sitemap index files (recursive, up to _MAX_SITEMAP_DEPTH levels)
    - Incremental polling via <lastmod>
    """

    def __init__(
        self,
        sitemap_url: str,
        batch_size: int = 10,
        user_agent: str = "RAGFlow-SitemapConnector/1.0",
        url_filter: str | None = None,
        follow_pdf_links: bool = False,
        restrict_pdf_to_domain: bool = True,
    ) -> None:
        self.sitemap_url = sitemap_url.strip()
        self.batch_size = batch_size
        self.user_agent = user_agent
        try:
            self._url_filter: re.Pattern | None = (
                re.compile(url_filter) if url_filter else None
            )
        except re.error as exc:
            raise ValueError(f"url_filter is not a valid regex: {exc}") from exc
        self.follow_pdf_links = follow_pdf_links
        self.restrict_pdf_to_domain = restrict_pdf_to_domain
        self.credentials: dict[str, Any] = {}

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self.credentials = credentials or {}
        return None

    def validate_connector_settings(self) -> None:
        self._validate_url(self.sitemap_url)
        if self.batch_size < 1:
            raise ValueError("batch_size must be greater than 0")
        if not any(self._url_matches(u[0]) for u in self._iter_sitemap_urls(self.sitemap_url, depth=0)):
            raise ValueError("Sitemap contains no URLs matching the filter")

    def load_from_state(self) -> GenerateDocumentsOutput:
        yield from self._load_urls()

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> GenerateDocumentsOutput:
        yield from self._load_urls(start=start, end=end)

    def retrieve_all_slim_docs_perm_sync(
        self, callback: Any = None
    ) -> GenerateSlimDocumentOutput:
        del callback
        batch: list[SlimDocument] = []
        for url, _lastmod in self._iter_sitemap_urls(self.sitemap_url, depth=0):
            if not self._url_matches(url):
                continue
            batch.append(SlimDocument(id=self._build_document_id(url)))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _load_urls(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> GenerateDocumentsOutput:
        seen: set[str] = set()
        pending_pdfs: list[tuple[str, str]] = []  # (pdf_url, parent_url)
        sitemap_domain = urlparse(self.sitemap_url).netloc

        # --- Pass 1: sitemap URLs ---
        batch: list[Document] = []
        for url, lastmod in self._iter_sitemap_urls(self.sitemap_url, depth=0):
            if url in seen or not self._url_matches(url):
                continue
            seen.add(url)

            if start is not None or end is not None:
                if lastmod is None:
                    if start is not None:
                        continue
                else:
                    ts = lastmod.timestamp()
                    if start is not None and ts <= start:
                        continue
                    if end is not None and ts > end:
                        continue

            doc = self._fetch_and_build_document(
                url, lastmod, seen, pending_pdfs, sitemap_domain
            )
            if doc is None:
                continue

            batch.append(doc)
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

        # --- Pass 2: PDF links discovered from HTML pages ---
        if not self.follow_pdf_links or not pending_pdfs:
            return

        batch = []
        for pdf_url, parent_url in pending_pdfs:
            doc = self._fetch_and_build_document(pdf_url, None, seen, [], sitemap_domain, parent_url)
            if doc is None:
                continue
            batch.append(doc)
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def _iter_sitemap_urls(
        self, sitemap_url: str, depth: int
    ) -> Iterator[tuple[str, datetime | None]]:
        """Recursively yield (url, lastmod) pairs from a sitemap or sitemap index."""
        if depth > _MAX_SITEMAP_DEPTH:
            logger.warning("Max sitemap depth reached, stopping at %s", sitemap_url)
            return

        try:
            self._validate_url(sitemap_url)
            content, _ = self._fetch_raw(sitemap_url)
        except Exception as exc:
            logger.warning("Failed to fetch sitemap %s: %s", sitemap_url, exc)
            return

        try:
            root = ET.fromstring(content)
        except ET.ParseError as exc:
            raise ValueError(f"Failed to parse sitemap XML from {sitemap_url!r}: {exc}") from exc

        tag = root.tag.lower()

        # Sitemap index → recurse into child sitemaps
        if "sitemapindex" in tag:
            for sitemap_el in root.iter(f"{{{_SITEMAP_NS}}}sitemap"):
                loc_el = sitemap_el.find(f"{{{_SITEMAP_NS}}}loc")
                if loc_el is None or not (loc_el.text or "").strip():
                    continue
                child_url = loc_el.text.strip()
                yield from self._iter_sitemap_urls(child_url, depth + 1)
            return

        # Standard urlset
        for url_el in root.iter(f"{{{_SITEMAP_NS}}}url"):
            loc_el = url_el.find(f"{{{_SITEMAP_NS}}}loc")
            if loc_el is None or not (loc_el.text or "").strip():
                continue
            loc = loc_el.text.strip()

            lastmod: datetime | None = None
            lastmod_el = url_el.find(f"{{{_SITEMAP_NS}}}lastmod")
            if lastmod_el is not None and (lastmod_el.text or "").strip():
                lastmod = self._parse_lastmod(lastmod_el.text.strip())

            yield loc, lastmod

    def _fetch_and_build_document(
        self,
        url: str,
        lastmod: datetime | None,
        seen: set[str] | None = None,
        pending_pdfs: list[tuple[str, str]] | None = None,
        sitemap_domain: str | None = None,
        parent_url: str | None = None,
    ) -> Document | None:
        try:
            self._validate_url(url)
            raw, content_type = self._fetch_raw(url)
        except Exception as exc:
            logger.warning("Failed to fetch page %s: %s", url, exc)
            return None

        updated_at = lastmod or datetime.now(timezone.utc)

        if "application/pdf" in content_type:
            if not raw:
                logger.debug("Empty PDF for %s, skipping", url)
                return None
            blob = raw
            extension = ".pdf"
        else:
            if self.follow_pdf_links and seen is not None and pending_pdfs is not None:
                for pdf_url in self._extract_pdf_links(raw, url):
                    if pdf_url not in seen:
                        if not self.restrict_pdf_to_domain or urlparse(pdf_url).netloc == sitemap_domain:
                            pending_pdfs.append((pdf_url, url))
                            seen.add(pdf_url)

            try:
                import trafilatura
                text = trafilatura.extract(
                    raw.decode("utf-8", errors="replace"),
                    output_format="markdown",
                    include_links=True,
                    include_tables=True,
                    include_images=False,
                    favor_recall=True,
                )
            except Exception as exc:
                logger.warning("Failed to parse page %s: %s", url, exc)
                return None

            if not text or not text.strip():
                logger.debug("Empty content for %s, skipping", url)
                return None

            blob = text.encode("utf-8")
            extension = ".md"

        return Document(
            id=self._build_document_id(url),
            source=DocumentSource.SITEMAP,
            semantic_identifier=self._url_to_identifier(url),
            extension=extension,
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            metadata={
                "sitemap_url": self.sitemap_url,
                "url": url,
                **({"parent_url": parent_url} if parent_url else {}),
            },
        )

    @staticmethod
    def _extract_pdf_links(html_bytes: bytes, base_url: str) -> list[str]:
        """Extract absolute PDF href links from an HTML page."""
        from html.parser import HTMLParser

        class _Extractor(HTMLParser):
            def __init__(self):
                super().__init__()
                self.links: list[str] = []

            def handle_starttag(self, tag, attrs):
                if tag == "a":
                    href = dict(attrs).get("href", "") or ""
                    if href.lower().endswith(".pdf"):
                        self.links.append(href)

        ex = _Extractor()
        ex.feed(html_bytes.decode("utf-8", errors="replace"))
        return [urljoin(base_url, lnk) for lnk in ex.links]

    def _fetch_raw(self, url: str) -> tuple[bytes, str]:
        """Fetch a URL with redirect following and SSRF protection.

        Returns (content, content_type).
        """
        current_url = url
        current_hostname, current_ip = assert_url_is_safe(current_url)

        response: requests.Response | None = None
        for _ in range(_MAX_REDIRECTS + 1):
            response = requests.get(
                current_url,
                timeout=REQUEST_TIMEOUT_SECONDS,
                allow_redirects=False,
                headers={"User-Agent": self.user_agent},
            )
            if response.status_code not in (301, 302, 303, 307, 308):
                break
            location = response.headers.get("Location")
            if not location:
                break
            redirect_url = urljoin(current_url, location)
            current_hostname, current_ip = assert_url_is_safe(redirect_url)
            current_url = redirect_url
        else:
            raise ValueError(f"Exceeded {_MAX_REDIRECTS} redirects fetching {url!r}")

        response.raise_for_status()
        content_type = response.headers.get("Content-Type", "").lower()
        return response.content, content_type

    @staticmethod
    def _validate_url(url: str) -> tuple[str, str]:
        if not url:
            raise ValueError("URL is required")
        parsed = urlparse(url)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ValueError(f"URL must be a valid http or https URL: {url!r}")
        return assert_url_is_safe(url)

    def _url_matches(self, url: str) -> bool:
        if self._url_filter is None:
            return True
        return bool(self._url_filter.search(url))

    @staticmethod
    def _url_to_identifier(url: str) -> str:
        parsed = urlparse(url)
        identifier = f"{parsed.netloc}{parsed.path}"
        if parsed.query:
            identifier += f"?{parsed.query}"
        if parsed.fragment:
            identifier += f"#{parsed.fragment}"
        return identifier.strip("/") or url

    @staticmethod
    def _build_document_id(url: str) -> str:
        return f"sitemap:{hashlib.md5(url.encode('utf-8')).hexdigest()}"

    @staticmethod
    def _parse_lastmod(value: str) -> datetime | None:
        for fmt in ("%Y-%m-%dT%H:%M:%S.%f%z", "%Y-%m-%dT%H:%M:%S.%fZ", "%Y-%m-%dT%H:%M:%S%z", "%Y-%m-%dT%H:%M:%SZ", "%Y-%m-%d"):
            try:
                dt = datetime.strptime(value, fmt)
                if dt.tzinfo is None:
                    dt = dt.replace(tzinfo=timezone.utc)
                return dt.astimezone(timezone.utc)
            except ValueError:
                continue
        logger.debug("Unrecognised lastmod format: %r", value)
        return None
