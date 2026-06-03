import hashlib
from datetime import datetime, timezone
from typing import Any

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

_USER_AGENT = "RAGFlow"


class TrelloConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    def __init__(
        self,
        board_ids: list[str] | str | None = None,
        include_comments: bool = True,
        include_attachments: bool = True,
        batch_size: int = INDEX_BATCH_SIZE,
        api_base: str = "https://api.trello.com/1",
    ) -> None:
        self.board_ids = self._normalize_id_list(board_ids)
        self.include_comments = include_comments
        self.include_attachments = include_attachments
        self.batch_size = batch_size
        self.api_base = api_base.rstrip("/")
        self.credentials: dict[str, Any] = {}
        self.session = requests.Session()
        self.session.headers.update({"User-Agent": _USER_AGENT})

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self.credentials = credentials or {}
        return None

    def validate_connector_settings(self) -> None:
        self._require_credentials()
        if self.batch_size < 1:
            raise ValueError("batch_size must be greater than 0")
        if self.board_ids:
            for board_id in self.board_ids:
                self._get(f"boards/{board_id}", {"fields": "id,name"})
            return
        self._get("members/me/boards", {"fields": "id,name", "filter": "open"})

    def load_from_state(self) -> GenerateDocumentsOutput:
        yield from self._load_cards()

    def poll_source(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> GenerateDocumentsOutput:
        yield from self._load_cards(start=start, end=end)

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> GenerateSlimDocumentOutput:
        del callback

        batch: list[SlimDocument] = []
        for board in self._iter_boards():
            for card in self._iter_cards(board["id"]):
                batch.append(SlimDocument(id=self._build_document_id(card)))
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

        if batch:
            yield batch

    def _load_cards(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> GenerateDocumentsOutput:
        batch: list[Document] = []
        for board in self._iter_boards():
            lists_by_id = self._lists_by_id(board["id"])
            for card in self._iter_cards(board["id"]):
                updated_at = self._resolve_card_updated_at(card)
                ts = updated_at.timestamp()
                if start is not None and ts <= start:
                    continue
                if end is not None and ts > end:
                    continue

                comments = self._comments_for_card(card["id"]) if self.include_comments else []
                attachments = self._attachments_for_card(card["id"]) if self.include_attachments else []
                batch.append(self._build_document(board, lists_by_id, card, comments, attachments, updated_at))

                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

        if batch:
            yield batch

    def _iter_boards(self) -> list[dict[str, Any]]:
        params = {"fields": "id,name,url,dateLastActivity", "filter": "open"}
        if not self.board_ids:
            boards = self._get("members/me/boards", params)
            return [board for board in boards if board.get("id")]

        boards: list[dict[str, Any]] = []
        for board_id in self.board_ids:
            board = self._get(f"boards/{board_id}", params)
            if board.get("id"):
                boards.append(board)
        return boards

    def _lists_by_id(self, board_id: str) -> dict[str, str]:
        lists = self._get(f"boards/{board_id}/lists", {"fields": "id,name", "filter": "open"})
        return {item["id"]: item.get("name", "") for item in lists if item.get("id")}

    def _iter_cards(self, board_id: str) -> list[dict[str, Any]]:
        fields = ",".join(
            [
                "id",
                "name",
                "desc",
                "url",
                "shortUrl",
                "dateLastActivity",
                "due",
                "dueComplete",
                "idList",
                "labels",
                "closed",
            ]
        )
        cards = self._get(f"boards/{board_id}/cards", {"fields": fields, "filter": "open"})
        return [card for card in cards if card.get("id") and not card.get("closed")]

    def _comments_for_card(self, card_id: str) -> list[dict[str, Any]]:
        actions = self._get(
            f"cards/{card_id}/actions",
            {
                "filter": "commentCard",
                "fields": "data,date,memberCreator",
                "limit": "1000",
            },
        )
        return [action for action in actions if action.get("data", {}).get("text")]

    def _attachments_for_card(self, card_id: str) -> list[dict[str, Any]]:
        attachments = self._get(
            f"cards/{card_id}/attachments",
            {"fields": "id,name,url,mimeType,bytes,date"},
        )
        return [attachment for attachment in attachments if attachment.get("id")]

    def _build_document(
        self,
        board: dict[str, Any],
        lists_by_id: dict[str, str],
        card: dict[str, Any],
        comments: list[dict[str, Any]],
        attachments: list[dict[str, Any]],
        updated_at: datetime,
    ) -> Document:
        content = self._build_content(board, lists_by_id, card, comments, attachments)
        blob = content.encode("utf-8")
        labels = [label.get("name") or label.get("color") for label in card.get("labels", []) if label.get("name") or label.get("color")]
        metadata = {
            "board_id": board.get("id"),
            "board_name": board.get("name"),
            "list_id": card.get("idList"),
            "list_name": lists_by_id.get(card.get("idList", ""), ""),
            "card_id": card.get("id"),
            "card_url": card.get("url") or card.get("shortUrl"),
            "labels": labels,
            "due": card.get("due"),
            "due_complete": card.get("dueComplete"),
        }
        return Document(
            id=self._build_document_id(card),
            source=DocumentSource.TRELLO,
            semantic_identifier=card.get("name") or card.get("id"),
            extension=".txt",
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            metadata=metadata,
            fingerprint=hashlib.md5(blob).hexdigest(),
        )

    def _build_content(
        self,
        board: dict[str, Any],
        lists_by_id: dict[str, str],
        card: dict[str, Any],
        comments: list[dict[str, Any]],
        attachments: list[dict[str, Any]],
    ) -> str:
        labels = [label.get("name") or label.get("color") for label in card.get("labels", []) if label.get("name") or label.get("color")]
        parts = [
            f"Card: {card.get('name', '')}",
            f"Board: {board.get('name', '')}",
            f"List: {lists_by_id.get(card.get('idList', ''), '')}",
        ]
        if card.get("url") or card.get("shortUrl"):
            parts.append(f"URL: {card.get('url') or card.get('shortUrl')}")
        if card.get("due"):
            parts.append(f"Due: {card.get('due')}")
        if labels:
            parts.append("Labels: " + ", ".join(labels))
        if card.get("desc"):
            parts.append("Description:\n" + card["desc"].strip())
        if comments:
            parts.append("Comments:")
            for comment in comments:
                author = (comment.get("memberCreator") or {}).get("fullName") or "Unknown"
                text = (comment.get("data") or {}).get("text", "")
                date = comment.get("date", "")
                parts.append(f"- {author} ({date}): {text}")
        if attachments:
            parts.append("Attachments:")
            for attachment in attachments:
                name = attachment.get("name") or attachment.get("url") or attachment.get("id")
                url = attachment.get("url", "")
                parts.append(f"- {name}: {url}".strip())
        return "\n\n".join(part for part in parts if part).strip()

    def _get(self, path: str, params: dict[str, Any] | None = None) -> Any:
        self._require_credentials()
        request_params = dict(params or {})
        request_params["key"] = self.credentials["trello_api_key"]
        request_params["token"] = self.credentials["trello_api_token"]
        response = self.session.get(
            f"{self.api_base}/{path.lstrip('/')}",
            params=request_params,
            timeout=REQUEST_TIMEOUT_SECONDS,
        )
        response.raise_for_status()
        return response.json()

    def _require_credentials(self) -> None:
        if not self.credentials.get("trello_api_key"):
            raise ValueError("Missing trello_api_key in credentials")
        if not self.credentials.get("trello_api_token"):
            raise ValueError("Missing trello_api_token in credentials")

    @staticmethod
    def _build_document_id(card: dict[str, Any]) -> str:
        return f"trello:{card['id']}"

    @staticmethod
    def _parse_trello_datetime(value: Any) -> datetime | None:
        if not isinstance(value, str) or not value.strip():
            return None
        normalized = value.replace("Z", "+00:00")
        try:
            parsed = datetime.fromisoformat(normalized)
        except ValueError:
            return None
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed.astimezone(timezone.utc)

    def _resolve_card_updated_at(self, card: dict[str, Any]) -> datetime:
        parsed = self._parse_trello_datetime(card.get("dateLastActivity"))
        if parsed is not None:
            return parsed
        return self._datetime_from_card_id(card.get("id")) or datetime.now(timezone.utc)

    @staticmethod
    def _datetime_from_card_id(card_id: Any) -> datetime | None:
        if not isinstance(card_id, str) or len(card_id) < 8:
            return None
        try:
            timestamp = int(card_id[:8], 16)
        except ValueError:
            return None
        return datetime.fromtimestamp(timestamp, tz=timezone.utc)

    @staticmethod
    def _normalize_id_list(value: list[str] | str | None) -> list[str]:
        if value is None:
            return []
        if isinstance(value, str):
            raw_items = value.split(",")
        else:
            raw_items = value
        return [str(item).strip() for item in raw_items if str(item).strip()]
