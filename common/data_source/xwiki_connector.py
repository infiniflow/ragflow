from __future__ import annotations

import hashlib
import logging
from collections.abc import Generator, Iterator
from datetime import datetime, timezone
from typing import Any
from urllib.parse import quote, urljoin

import requests

from common.data_source.config import DocumentSource, INDEX_BATCH_SIZE, REQUEST_TIMEOUT_SECONDS
from common.data_source.interfaces import LoadConnector, PollConnector, SlimConnectorWithPermSync
from common.data_source.models import Document, SecondsSinceUnixEpoch, SlimDocument

logger = logging.getLogger(__name__)


class XWikiConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    _INVALID_TIMESTAMP = datetime.fromtimestamp(0, tz=timezone.utc)

    def __init__(
        self,
        base_url: str,
        wiki: str = "xwiki",
        space: str = "Main",
        page_ids: str = "",
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.wiki = wiki.strip() or "xwiki"
        self.space = space.strip() or "Main"
        self.page_ids = [p.strip() for p in page_ids.split(",") if p.strip()]
        self.batch_size = batch_size or INDEX_BATCH_SIZE
        self.session = requests.Session()
        self.session.headers.update({"Accept": "application/json"})
        self._credentials: dict[str, Any] = {}

    def __enter__(self) -> XWikiConnector:
        return self

    def __exit__(self, exc_type: Any, exc: Any, traceback: Any) -> None:
        del exc_type, exc, traceback
        self.close()

    def __del__(self) -> None:
        self.close()

    def close(self) -> None:
        close = getattr(getattr(self, "session", None), "close", None)
        if close is not None:
            close()

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self._credentials = credentials or {}
        token = self._credentials.get("xwiki_api_token")
        username = self._credentials.get("xwiki_username")
        password = self._credentials.get("xwiki_password")
        if token:
            self.session.headers.update({"Authorization": f"Bearer {token}"})
            logger.info("XWiki connector using bearer token authentication")
        elif username and password:
            self.session.auth = (username, password)
            logger.info("XWiki connector using basic authentication")
        self.session.headers.setdefault("Accept", "application/json")
        return None

    def validate_connector_settings(self) -> None:
        if not self.base_url:
            raise ValueError("XWiki connector requires base_url")
        if not self.space and not self.page_ids:
            raise ValueError("XWiki connector requires a space or page IDs")

    def load_from_state(self) -> Generator[list[Document], None, None]:
        logger.debug("XWiki load_from_state started")
        yield from self._batch_documents(self._iter_documents())

    def poll_source(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> Generator[list[Document], None, None]:
        logger.debug("XWiki poll_source started start=%s end=%s", start, end)
        start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
        end_dt = datetime.fromtimestamp(end, tz=timezone.utc)
        docs = (
            doc
            for doc in self._iter_documents()
            if doc.doc_updated_at != self._INVALID_TIMESTAMP and start_dt <= doc.doc_updated_at <= end_dt
        )
        yield from self._batch_documents(docs)

    def retrieve_all_slim_docs_perm_sync(self, callback: Any = None) -> Generator[list[SlimDocument], None, None]:
        del callback
        docs = (SlimDocument(id=doc.id) for doc in self._iter_documents())
        batch: list[SlimDocument] = []
        for doc in docs:
            batch.append(doc)
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

    def _iter_documents(self) -> Iterator[Document]:
        logger.debug(
            "XWiki iterating documents wiki=%s space=%s configured_pages=%s",
            self.wiki,
            self.space,
            len(self.page_ids),
        )
        summaries = self._configured_page_summaries() if self.page_ids else self._list_space_pages(self.space)
        for summary in summaries:
            page = self._page_detail(summary)
            yield self._document_from_page(page)

    def _configured_page_summaries(self) -> Iterator[dict[str, Any]]:
        for page_id in self.page_ids:
            yield {"id": page_id, "fullName": page_id.split(":", 1)[-1]}

    def _list_space_pages(self, space: str) -> Iterator[dict[str, Any]]:
        path = f"rest/wikis/{quote(self.wiki)}/{self._space_path(space)}/pages"
        logger.debug("XWiki listing pages path=%s", path)
        for item in self._items_from_payload(self._get(path)):
            yield item

    def _page_detail(self, summary: dict[str, Any]) -> dict[str, Any]:
        full_name = (
            summary.get("fullName")
            or summary.get("name")
            or summary.get("id", "").split(":", 1)[-1]
        )
        if "." not in full_name:
            return summary
        space, page = full_name.rsplit(".", 1)
        path = f"rest/wikis/{quote(self.wiki)}/{self._space_path(space)}/pages/{quote(page)}"
        logger.debug("XWiki fetching page detail full_name=%s path=%s", full_name, path)
        detail = self._get(path)
        detail.setdefault("fullName", full_name)
        return {**summary, **detail}

    def _get(self, path: str) -> dict[str, Any]:
        url = urljoin(f"{self.base_url}/", path)
        logger.debug("XWiki request url=%s", url)
        try:
            response = self.session.get(url, timeout=REQUEST_TIMEOUT_SECONDS)
            response.raise_for_status()
        except requests.RequestException:
            logger.exception("XWiki request failed url=%s", url)
            raise
        return response.json()

    @staticmethod
    def _space_path(space: str) -> str:
        return "/".join(f"spaces/{quote(part)}" for part in space.split(".") if part)

    @staticmethod
    def _items_from_payload(payload: dict[str, Any]) -> list[dict[str, Any]]:
        for key in ("pageSummaries", "pageSummary", "pages", "items", "results"):
            value = payload.get(key)
            if isinstance(value, list):
                return value
            if isinstance(value, dict):
                return [value]
        return []

    def _document_from_page(self, page: dict[str, Any]) -> Document:
        full_name = page.get("fullName") or page.get("name") or page.get("id", "page")
        title = page.get("title") or full_name.rsplit(".", 1)[-1]
        content = page.get("content") or page.get("renderedContent") or ""
        updated_at = self._parse_datetime(page.get("modified") or page.get("updated") or page.get("created"))
        link = self._page_link(page)
        text = "\n".join(
            part
            for part in (
                f"Title: {title}",
                f"Page: {full_name}",
                f"URL: {link}" if link else "",
                "",
                str(content).strip(),
            )
            if part is not None
        ).strip()
        blob = text.encode("utf-8")
        doc_id = f"xwiki:{self.wiki}:{full_name}"
        fingerprint = hashlib.md5(blob).hexdigest()
        logger.debug(
            "XWiki built document id=%s wiki=%s full_name=%s size=%s fingerprint=%s",
            doc_id,
            self.wiki,
            full_name,
            len(blob),
            fingerprint,
        )
        return Document(
            id=doc_id,
            source=DocumentSource.XWIKI,
            semantic_identifier=title,
            extension="txt",
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            metadata={
                "wiki": self.wiki,
                "page": full_name,
                "space": full_name.rsplit(".", 1)[0] if "." in full_name else "",
                "url": link,
            },
            fingerprint=fingerprint,
        )

    def _page_link(self, page: dict[str, Any]) -> str:
        for key in ("xwikiAbsoluteUrl", "absoluteUrl", "url"):
            if page.get(key):
                return str(page[key])
        relative = page.get("xwikiRelativeUrl") or page.get("relativeUrl")
        if relative:
            return urljoin(f"{self.base_url}/", str(relative).lstrip("/"))
        links = page.get("links") or page.get("_links") or {}
        href = links.get("http://www.xwiki.org/rel/page") or links.get("self")
        if isinstance(href, dict):
            href = href.get("href")
        return urljoin(f"{self.base_url}/", str(href).lstrip("/")) if href else ""

    @staticmethod
    def _parse_datetime(value: Any) -> datetime:
        if isinstance(value, datetime):
            return value.astimezone(timezone.utc) if value.tzinfo else value.replace(tzinfo=timezone.utc)
        if isinstance(value, str) and value:
            try:
                return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)
            except ValueError:
                pass
        return XWikiConnector._INVALID_TIMESTAMP

    def _batch_documents(self, docs: Iterator[Document]) -> Generator[list[Document], None, None]:
        batch: list[Document] = []
        for doc in docs:
            batch.append(doc)
            if len(batch) >= self.batch_size:
                logger.debug("XWiki yielding document batch size=%s", len(batch))
                yield batch
                batch = []
        if batch:
            logger.debug("XWiki yielding final document batch size=%s", len(batch))
            yield batch
