from __future__ import annotations

import hashlib
from collections.abc import Generator, Iterator
from datetime import datetime, timezone
from typing import Any
from urllib.parse import urljoin

import requests

from common.data_source.config import DocumentSource, INDEX_BATCH_SIZE, REQUEST_TIMEOUT_SECONDS
from common.data_source.interfaces import LoadConnector, PollConnector, SlimConnectorWithPermSync
from common.data_source.models import Document, SecondsSinceUnixEpoch, SlimDocument


class TableauConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    def __init__(
        self,
        server_url: str,
        site_content_url: str = "",
        api_version: str = "3.24",
        project_names: str = "",
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.server_url = server_url.rstrip("/")
        self.site_content_url = site_content_url
        self.api_version = api_version or "3.24"
        self.project_names = {p.strip() for p in project_names.split(",") if p.strip()}
        self.batch_size = batch_size or INDEX_BATCH_SIZE
        self.session = requests.Session()
        self._credentials: dict[str, Any] = {}
        self._site_id = ""
        self._signed_in = False

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self._credentials = credentials or {}
        return None

    def validate_connector_settings(self) -> None:
        if not self.server_url:
            raise ValueError("Tableau connector requires server_url")
        if not self._credentials.get("tableau_pat_name") or not self._credentials.get("tableau_pat_secret"):
            raise ValueError("Tableau connector requires personal access token credentials")

    def load_from_state(self) -> Generator[list[Document], None, None]:
        yield from self._batch_documents(self._iter_documents())

    def poll_source(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> Generator[list[Document], None, None]:
        start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
        end_dt = datetime.fromtimestamp(end, tz=timezone.utc)
        docs = (
            doc
            for doc in self._iter_documents()
            if start_dt <= doc.doc_updated_at <= end_dt
        )
        yield from self._batch_documents(docs)

    def retrieve_all_slim_docs_perm_sync(self, callback: Any = None) -> Generator[list[SlimDocument], None, None]:
        del callback
        batch: list[SlimDocument] = []
        for doc in self._iter_documents():
            batch.append(SlimDocument(id=doc.id))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

    def _iter_documents(self) -> Iterator[Document]:
        self._ensure_signed_in()
        for workbook in self._list_workbooks():
            project_name = (workbook.get("project") or {}).get("name") or workbook.get("projectName") or ""
            if self.project_names and project_name not in self.project_names:
                continue
            views = list(self._list_workbook_views(str(workbook.get("id"))))
            if not views:
                yield self._document_from_item("workbook", workbook, workbook)
                continue
            for view in views:
                yield self._document_from_item("view", view, workbook)

    def _ensure_signed_in(self) -> None:
        if self._signed_in:
            return
        payload = {
            "credentials": {
                "personalAccessTokenName": self._credentials.get("tableau_pat_name"),
                "personalAccessTokenSecret": self._credentials.get("tableau_pat_secret"),
                "site": {"contentUrl": self.site_content_url},
            }
        }
        response = self.session.post(
            self._api_url("auth/signin"),
            json=payload,
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        response.raise_for_status()
        credentials = response.json().get("credentials", {})
        token = credentials.get("token")
        site_id = (credentials.get("site") or {}).get("id")
        if not token or not site_id:
            raise ValueError("Tableau sign-in response did not include token and site id")
        self._site_id = site_id
        self.session.headers.update({"X-Tableau-Auth": token})
        self._signed_in = True

    def _list_workbooks(self) -> Iterator[dict[str, Any]]:
        yield from self._paged_items(
            f"sites/{self._site_id}/workbooks",
            container="workbooks",
            item_key="workbook",
        )

    def _list_workbook_views(self, workbook_id: str) -> Iterator[dict[str, Any]]:
        yield from self._paged_items(
            f"sites/{self._site_id}/workbooks/{workbook_id}/views",
            container="views",
            item_key="view",
        )

    def _paged_items(self, path: str, container: str, item_key: str) -> Iterator[dict[str, Any]]:
        page_number = 1
        page_size = 100
        while True:
            response = self.session.get(
                self._api_url(path),
                params={"pageSize": page_size, "pageNumber": page_number},
                timeout=REQUEST_TIMEOUT_SECONDS,
            )
            response.raise_for_status()
            payload = response.json()
            items = self._items_from_payload(payload, container, item_key)
            yield from items
            pagination = payload.get("pagination") or {}
            total_available = int(pagination.get("totalAvailable") or len(items))
            if page_number * page_size >= total_available or not items:
                break
            page_number += 1

    def _api_url(self, path: str) -> str:
        return urljoin(f"{self.server_url}/", f"api/{self.api_version}/{path.lstrip('/')}")

    @staticmethod
    def _items_from_payload(payload: dict[str, Any], container: str, item_key: str) -> list[dict[str, Any]]:
        value = (payload.get(container) or {}).get(item_key)
        if isinstance(value, list):
            return value
        if isinstance(value, dict):
            return [value]
        return []

    def _document_from_item(self, item_type: str, item: dict[str, Any], workbook: dict[str, Any]) -> Document:
        item_id = str(item.get("id"))
        name = str(item.get("name") or item_id)
        workbook_name = str(workbook.get("name") or "")
        project_name = (workbook.get("project") or {}).get("name") or workbook.get("projectName") or ""
        link = str(item.get("webpageUrl") or item.get("contentUrl") or workbook.get("webpageUrl") or "")
        updated_at = self._parse_datetime(item.get("updatedAt") or workbook.get("updatedAt") or item.get("createdAt"))
        text = "\n".join(
            [
                f"Type: Tableau {item_type}",
                f"Name: {name}",
                f"Workbook: {workbook_name}",
                f"Project: {project_name}",
                f"URL: {link}",
            ]
        )
        blob = text.encode("utf-8")
        return Document(
            id=f"tableau:{item_type}:{item_id}",
            source=DocumentSource.TABLEAU,
            semantic_identifier=name,
            extension="txt",
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            metadata={
                "item_type": item_type,
                "item_id": item_id,
                "workbook_id": workbook.get("id"),
                "workbook_name": workbook_name,
                "project_name": project_name,
                "url": link,
            },
            fingerprint=hashlib.sha256(blob).hexdigest(),
        )

    @staticmethod
    def _parse_datetime(value: Any) -> datetime:
        if isinstance(value, datetime):
            return value.astimezone(timezone.utc) if value.tzinfo else value.replace(tzinfo=timezone.utc)
        if isinstance(value, str) and value:
            try:
                return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)
            except ValueError:
                pass
        return datetime.fromtimestamp(0, tz=timezone.utc)

    def _batch_documents(self, docs: Iterator[Document]) -> Generator[list[Document], None, None]:
        batch: list[Document] = []
        for doc in docs:
            batch.append(doc)
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch
