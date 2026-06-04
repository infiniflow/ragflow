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


class OpenTextConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    def __init__(
        self,
        base_url: str,
        root_node_ids: str,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.root_node_ids = [node.strip() for node in root_node_ids.split(",") if node.strip()]
        self.batch_size = batch_size or INDEX_BATCH_SIZE
        self.session = requests.Session()
        self._credentials: dict[str, Any] = {}

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self._credentials = credentials or {}
        ticket = self._credentials.get("opentext_ticket")
        token = self._credentials.get("opentext_api_token")
        username = self._credentials.get("opentext_username")
        password = self._credentials.get("opentext_password")
        if ticket:
            self.session.headers.update({"OTCSTicket": ticket})
        elif token:
            self.session.headers.update({"Authorization": f"Bearer {token}"})
        elif username and password:
            self.session.auth = (username, password)
        return None

    def validate_connector_settings(self) -> None:
        if not self.base_url:
            raise ValueError("OpenText connector requires base_url")
        if not self.root_node_ids:
            raise ValueError("OpenText connector requires at least one root_node_id")

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
        for root_node_id in self.root_node_ids:
            for node in self._walk_node(root_node_id):
                if self._is_folder(node):
                    continue
                detail = self._node_detail(str(node.get("id")))
                content = self._node_content(str(node.get("id")))
                yield self._document_from_node({**node, **detail}, content)

    def _walk_node(self, node_id: str) -> Iterator[dict[str, Any]]:
        node = self._node_detail(node_id)
        if not self._is_folder(node):
            yield node
            return
        for child in self._child_nodes(node_id):
            yield child
            if self._is_folder(child):
                yield from self._walk_node(str(child.get("id")))

    def _node_detail(self, node_id: str) -> dict[str, Any]:
        return self._data_from_payload(self._get_json(f"api/v1/nodes/{node_id}"))

    def _child_nodes(self, node_id: str) -> Iterator[dict[str, Any]]:
        payload = self._get_json(f"api/v1/nodes/{node_id}/nodes")
        for item in self._items_from_payload(payload):
            data = self._data_from_payload(item)
            if isinstance(data, dict):
                yield data

    def _node_content(self, node_id: str) -> bytes:
        response = self.session.get(
            urljoin(f"{self.base_url}/", f"api/v1/nodes/{node_id}/content"),
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        response.raise_for_status()
        return response.content

    def _get_json(self, path: str) -> dict[str, Any]:
        response = self.session.get(urljoin(f"{self.base_url}/", path), timeout=REQUEST_TIMEOUT_SECONDS)
        response.raise_for_status()
        return response.json()

    @staticmethod
    def _data_from_payload(payload: dict[str, Any]) -> dict[str, Any]:
        data = payload.get("data")
        if isinstance(data, dict) and isinstance(data.get("properties"), dict):
            return data["properties"]
        if isinstance(data, dict):
            return data
        return payload

    @staticmethod
    def _items_from_payload(payload: dict[str, Any]) -> list[dict[str, Any]]:
        data = payload.get("data")
        if isinstance(data, list):
            return data
        if isinstance(data, dict):
            for key in ("items", "children", "nodes"):
                value = data.get(key)
                if isinstance(value, list):
                    return value
        for key in ("items", "children", "nodes", "results"):
            value = payload.get(key)
            if isinstance(value, list):
                return value
        return []

    @staticmethod
    def _is_folder(node: dict[str, Any]) -> bool:
        type_name = str(node.get("type_name") or node.get("container") or "").lower()
        return bool(node.get("container")) or type_name in {"folder", "container"} or node.get("type") in (0, 1)

    def _document_from_node(self, node: dict[str, Any], blob: bytes) -> Document:
        node_id = str(node.get("id"))
        name = str(node.get("name") or node.get("title") or node_id)
        updated_at = self._parse_datetime(node.get("modify_date") or node.get("modified") or node.get("create_date"))
        link = str(node.get("url") or node.get("web_url") or urljoin(f"{self.base_url}/", f"app/nodes/{node_id}"))
        metadata = {
            "node_id": node_id,
            "name": name,
            "mime_type": node.get("mime_type") or node.get("mimeType") or "",
            "url": link,
        }
        return Document(
            id=f"opentext:{node_id}",
            source=DocumentSource.OPENTEXT,
            semantic_identifier=name,
            extension=self._extension(name, metadata["mime_type"]),
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            metadata=metadata,
            fingerprint=hashlib.sha256(blob).hexdigest(),
        )

    @staticmethod
    def _extension(name: str, mime_type: str) -> str:
        if "." in name.rsplit("/", 1)[-1]:
            return name.rsplit(".", 1)[-1].lower()
        if mime_type == "application/pdf":
            return "pdf"
        if mime_type.startswith("text/"):
            return "txt"
        return "bin"

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
