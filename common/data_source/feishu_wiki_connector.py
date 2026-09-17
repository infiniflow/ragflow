"""Feishu Wiki data source connector.

The connector handles downloadable Wiki ``file`` nodes only. Native Feishu
documents require separate export APIs and are traversed as containers so that
downloadable files nested below them are still found.
"""

from __future__ import annotations

import hashlib
import json
import logging
import time
from collections.abc import Iterator
from datetime import UTC, datetime
from typing import Any
from urllib.parse import quote

import requests

from common.data_source.config import INDEX_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch
from common.data_source.models import Document, GenerateDocumentsOutput
from common.data_source.utils import get_file_ext

LOGGER = logging.getLogger(__name__)

FEISHU_OPEN_BASE_URL = "https://open.feishu.cn"
DEFAULT_REQUEST_TIMEOUT_SECONDS = 60
DEFAULT_MAX_FILE_SIZE_BYTES = 50 * 1024 * 1024
MAX_BATCH_SIZE = 10
DOWNLOAD_CHUNK_SIZE_BYTES = 1024 * 1024
MAX_DOWNLOAD_ERROR_BODY_BYTES = 8 * 1024
MAX_ERROR_DETAIL_LENGTH = 256
MAX_DOWNLOAD_RETRIES = 3
MAX_DOWNLOAD_RETRY_DELAY_SECONDS = 60
SUPPORTED_EXTENSIONS = {
    "csv",
    "doc",
    "docx",
    "eml",
    "gif",
    "html",
    "jpeg",
    "jpg",
    "json",
    "md",
    "mdx",
    "pdf",
    "png",
    "ppt",
    "pptx",
    "tif",
    "txt",
    "xls",
    "xlsx",
}


def _normalized_terms(value: Any) -> set[str]:
    if value is None:
        return set()
    if isinstance(value, str):
        raw_values = value.split(",")
    elif isinstance(value, (list, tuple, set)):
        raw_values = value
    else:
        raw_values = [value]
    return {str(item).strip().lstrip(".").casefold() for item in raw_values if str(item).strip()}


def _parse_timestamp(value: Any) -> datetime | None:
    if value in (None, ""):
        return None
    if isinstance(value, datetime):
        parsed = value
    else:
        text = str(value).strip()
        try:
            parsed = datetime.fromtimestamp(float(text), tz=UTC)
        except (TypeError, ValueError, OverflowError):
            try:
                parsed = datetime.fromisoformat(text)
            except (TypeError, ValueError):
                return None
    if parsed.tzinfo is None:
        return parsed.replace(tzinfo=UTC)
    return parsed.astimezone(UTC)


class FeishuWikiConnector(LoadConnector, PollConnector):
    """Read downloadable attachment files from one Feishu Wiki subtree."""

    def __init__(
        self,
        *,
        space_id: str,
        root_node_token: str,
        include_extensions: Any = None,
        include_keywords: Any = None,
        exclude_keywords: Any = None,
        batch_size: int = INDEX_BATCH_SIZE,
        max_file_size_bytes: int = DEFAULT_MAX_FILE_SIZE_BYTES,
        session: requests.Session | None = None,
    ) -> None:
        self.space_id = space_id.strip()
        self.root_node_token = root_node_token.strip()
        self.include_extensions = _normalized_terms(include_extensions)
        self.include_keywords = _normalized_terms(include_keywords)
        self.exclude_keywords = _normalized_terms(exclude_keywords)
        self.batch_size = batch_size
        self.max_file_size_bytes = max_file_size_bytes
        self.session = session or requests.Session()
        self.app_id = ""
        self.app_secret = ""
        self._access_token = ""
        self._access_token_expires_at = 0.0

    @classmethod
    def build_connector(cls, config: dict[str, Any]) -> FeishuWikiConnector:
        raw_batch_size = config.get("batch_size")
        if raw_batch_size in (None, ""):
            raw_batch_size = INDEX_BATCH_SIZE
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError) as exc:
            raise ConnectorValidationError("Feishu Wiki batch_size must be an integer") from exc
        raw_max_file_size_bytes = config.get("max_file_size_bytes")
        if raw_max_file_size_bytes in (None, ""):
            raw_max_file_size_bytes = DEFAULT_MAX_FILE_SIZE_BYTES
        try:
            max_file_size_bytes = int(raw_max_file_size_bytes)
        except (TypeError, ValueError) as exc:
            raise ConnectorValidationError("Feishu Wiki max_file_size_bytes must be an integer") from exc

        connector = cls(
            space_id=str(config.get("space_id") or ""),
            root_node_token=str(config.get("root_node_token") or ""),
            include_extensions=config.get("include_extensions"),
            include_keywords=config.get("include_keywords"),
            exclude_keywords=config.get("exclude_keywords"),
            batch_size=batch_size,
            max_file_size_bytes=max_file_size_bytes,
        )
        connector.load_credentials(config.get("credentials") or {})
        connector._ensure_configured()
        return connector

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self.app_id = str(credentials.get("app_id") or "").strip()
        self.app_secret = str(credentials.get("app_secret") or "").strip()
        if not self.app_id or not self.app_secret:
            raise ConnectorMissingCredentialError("Feishu Wiki")
        self._access_token = ""
        self._access_token_expires_at = 0.0
        return None

    def _ensure_configured(self) -> None:
        if not self.space_id:
            raise ConnectorValidationError("Feishu Wiki space ID is required")
        if not self.root_node_token:
            raise ConnectorValidationError("Feishu Wiki root node token is required")
        if not 1 <= self.batch_size <= MAX_BATCH_SIZE:
            raise ConnectorValidationError(f"Feishu Wiki batch_size must be between 1 and {MAX_BATCH_SIZE}")
        if self.max_file_size_bytes <= 0:
            raise ConnectorValidationError("Feishu Wiki max_file_size_bytes must be a positive integer")
        unsupported = self.include_extensions - SUPPORTED_EXTENSIONS
        if unsupported:
            values = ", ".join(sorted(unsupported))
            raise ConnectorValidationError(f"Unsupported Feishu Wiki file extensions: {values}")

    def _get_access_token(self) -> str:
        now = time.monotonic()
        if self._access_token and now < self._access_token_expires_at:
            return self._access_token

        try:
            response = self.session.request(
                "POST",
                f"{FEISHU_OPEN_BASE_URL}/open-apis/auth/v3/tenant_access_token/internal",
                json={"app_id": self.app_id, "app_secret": self.app_secret},
                timeout=DEFAULT_REQUEST_TIMEOUT_SECONDS,
            )
        except requests.RequestException as exc:
            raise ConnectorValidationError("Unable to connect to Feishu while attempting to authenticate") from exc
        try:
            payload = self._decode_api_payload(response, "authenticate")
        finally:
            response.close()
        access_token = str(payload.get("tenant_access_token") or "")
        if not access_token:
            raise ConnectorValidationError("Feishu authentication response did not contain a tenant access token")
        try:
            expires_in = max(60, int(payload.get("expire") or 7200))
        except (TypeError, ValueError):
            expires_in = 7200
        self._access_token = access_token
        self._access_token_expires_at = now + max(30, expires_in - 60)
        return access_token

    @staticmethod
    def _decode_api_payload(response: requests.Response, operation: str) -> dict[str, Any]:
        payload: Any = None
        json_error: Exception | None = None
        try:
            payload = response.json()
        except (TypeError, ValueError) as exc:
            json_error = exc

        code = payload.get("code") if isinstance(payload, dict) else None
        message = str(payload.get("msg") or "unknown Feishu API error") if isinstance(payload, dict) else ""
        if response.status_code in {401, 403}:
            detail = f": {message}" if message else ""
            raise ConnectorValidationError(f"Feishu permission denied (HTTP {response.status_code}) while attempting to {operation}{detail}")
        if response.status_code >= 400:
            if code not in (0, "0", None):
                raise ConnectorValidationError(f"Feishu API error {code} (HTTP {response.status_code}) while attempting to {operation}: {message}")
            raise ConnectorValidationError(f"Feishu HTTP {response.status_code} while attempting to {operation}")
        if json_error is not None:
            raise ConnectorValidationError(f"Feishu returned invalid JSON while attempting to {operation}") from json_error
        if not isinstance(payload, dict):
            raise ConnectorValidationError(f"Feishu returned an invalid response while attempting to {operation}")
        code = payload.get("code", 0)
        if code not in (0, "0", None):
            message = str(payload.get("msg") or "unknown Feishu API error")
            raise ConnectorValidationError(f"Feishu API error {code} while attempting to {operation}: {message}")
        return payload

    def _request_json(
        self,
        method: str,
        path: str,
        *,
        params: dict[str, Any] | None = None,
        operation: str,
    ) -> dict[str, Any]:
        try:
            response = self.session.request(
                method,
                f"{FEISHU_OPEN_BASE_URL}{path}",
                headers={"Authorization": f"Bearer {self._get_access_token()}"},
                params=params,
                timeout=DEFAULT_REQUEST_TIMEOUT_SECONDS,
            )
        except requests.RequestException as exc:
            raise ConnectorValidationError(f"Unable to connect to Feishu while attempting to {operation}") from exc
        try:
            return self._decode_api_payload(response, operation)
        finally:
            response.close()

    def _iter_child_pages(self, parent_node_token: str) -> Iterator[list[dict[str, Any]]]:
        page_token = ""
        while True:
            params: dict[str, Any] = {"page_size": 50, "parent_node_token": parent_node_token}
            if page_token:
                params["page_token"] = page_token
            payload = self._request_json(
                "GET",
                f"/open-apis/wiki/v2/spaces/{quote(self.space_id, safe='')}/nodes",
                params=params,
                operation="list Wiki nodes",
            )
            data = payload.get("data") or {}
            if not isinstance(data, dict):
                raise ConnectorValidationError("Feishu Wiki node response contained invalid data")
            items = data.get("items") or []
            if not isinstance(items, list):
                raise ConnectorValidationError("Feishu Wiki node response contained invalid items")
            yield [item for item in items if isinstance(item, dict)]
            if not data.get("has_more"):
                break
            page_token = str(data.get("page_token") or "")
            if not page_token:
                raise ConnectorValidationError("Feishu Wiki pagination indicated more data without a page token")

    def _iter_nodes(self, parent_node_token: str) -> Iterator[dict[str, Any]]:
        for items in self._iter_child_pages(parent_node_token):
            for node in items:
                yield node
                node_token = str(node.get("node_token") or "")
                if node.get("has_child") and node_token:
                    yield from self._iter_nodes(node_token)

    def _matches_filters(self, title: str) -> bool:
        normalized_title = title.casefold()
        extension = get_file_ext(title).lstrip(".").casefold()
        if extension not in SUPPORTED_EXTENSIONS:
            return False
        if self.include_extensions and extension not in self.include_extensions:
            return False
        if self.include_keywords and not any(keyword in normalized_title for keyword in self.include_keywords):
            return False
        return not any(keyword in normalized_title for keyword in self.exclude_keywords)

    @staticmethod
    def _is_within_window(
        updated_at: datetime | None,
        start: SecondsSinceUnixEpoch | None,
        end: SecondsSinceUnixEpoch | None,
    ) -> bool:
        if updated_at is None:
            return True
        timestamp = updated_at.timestamp()
        if start is not None and timestamp <= start:
            return False
        return end is None or timestamp <= end

    def _error_detail(self, value: Any) -> str:
        """Bound diagnostic fields without exposing credentials or multiline text."""
        if not isinstance(value, (str, int)):
            return ""
        text = str(value)
        for credential in (self.app_secret, self._access_token):
            if credential:
                text = text.replace(credential, "[redacted]")
        return " ".join(text.split())[:MAX_ERROR_DETAIL_LENGTH]

    def _download_error(self, response: requests.Response) -> ConnectorValidationError:
        """Extract only bounded JSON diagnostics from an unsuccessful download."""
        payload: Any = None
        body = bytearray()
        try:
            for chunk in response.iter_content(chunk_size=1024):
                body.extend(chunk[: MAX_DOWNLOAD_ERROR_BODY_BYTES - len(body)])
                if len(body) >= MAX_DOWNLOAD_ERROR_BODY_BYTES:
                    break
            payload = json.loads(body)
        except (ValueError, RecursionError, OSError, requests.RequestException):
            payload = None

        code = self._error_detail(payload.get("code")) if isinstance(payload, dict) else ""
        message = self._error_detail(payload.get("msg")) if isinstance(payload, dict) else ""
        if code and code != "0":
            detail = f"Feishu API error {code} (HTTP {response.status_code}) while downloading a Wiki file"
        elif response.status_code in {401, 403}:
            detail = f"Feishu permission denied (HTTP {response.status_code}) while downloading a Wiki file"
        else:
            detail = f"Feishu HTTP {response.status_code} while downloading a Wiki file"
        if message:
            detail += f": {message}"
        error = payload.get("error") if isinstance(payload, dict) else None
        logid = self._error_detail(response.headers.get("x-tt-logid") or response.headers.get("x-request-id"))
        if not logid and isinstance(error, dict):
            logid = self._error_detail(error.get("logid"))
        if logid:
            detail += f" (logid={logid})"
        return ConnectorValidationError(detail)

    def _download_file(self, object_token: str) -> bytes:
        """Download a file, retrying throttling without leaking streamed responses."""
        for attempt in range(MAX_DOWNLOAD_RETRIES + 1):
            try:
                response = self.session.request(
                    "GET",
                    f"{FEISHU_OPEN_BASE_URL}/open-apis/drive/v1/files/{quote(object_token, safe='')}/download",
                    headers={"Authorization": f"Bearer {self._get_access_token()}"},
                    timeout=DEFAULT_REQUEST_TIMEOUT_SECONDS,
                    stream=True,
                )
            except requests.RequestException as exc:
                raise ConnectorValidationError("Unable to connect to Feishu while downloading a Wiki file") from exc

            if response.status_code != 429 or attempt == MAX_DOWNLOAD_RETRIES:
                break
            # Feishu specifies a relative delay in seconds, not a Unix timestamp.
            try:
                delay = int(response.headers.get("x-ogw-ratelimit-reset", ""))
            except (TypeError, ValueError):
                delay = -1
            if delay < 0:
                delay = 2**attempt
            if delay > MAX_DOWNLOAD_RETRY_DELAY_SECONDS:
                # Fail rather than retry before the server's requested reset.
                break
            response.close()
            time.sleep(max(1, delay))

        try:
            if not 200 <= response.status_code < 300:
                raise self._download_error(response)

            declared_size = response.headers.get("Content-Length")
            if declared_size:
                try:
                    declared_size_bytes = int(declared_size)
                except (TypeError, ValueError):
                    declared_size_bytes = None
                if declared_size_bytes is not None and declared_size_bytes > self.max_file_size_bytes:
                    raise ConnectorValidationError(f"Feishu Wiki file size exceeds the configured {self.max_file_size_bytes}-byte limit")

            chunks: list[bytes] = []
            downloaded_size = 0
            for chunk in response.iter_content(chunk_size=DOWNLOAD_CHUNK_SIZE_BYTES):
                if not chunk:
                    continue
                downloaded_size += len(chunk)
                if downloaded_size > self.max_file_size_bytes:
                    raise ConnectorValidationError(f"Feishu Wiki file size exceeds the configured {self.max_file_size_bytes}-byte limit")
                chunks.append(chunk)
            return b"".join(chunks)
        except ConnectorValidationError:
            raise
        except (OSError, requests.RequestException) as exc:
            raise ConnectorValidationError("Failed to download a Feishu Wiki file") from exc
        finally:
            response.close()

    def _node_to_document(self, node: dict[str, Any], *, fallback_updated_at: datetime) -> Document:
        node_token = str(node.get("node_token") or "")
        object_token = str(node.get("obj_token") or "")
        title = str(node.get("title") or node_token)
        if not node_token or not object_token:
            raise ConnectorValidationError("Feishu Wiki file node is missing a node token or object token")

        blob = self._download_file(object_token)
        content_sha256 = hashlib.sha256(blob).hexdigest()
        fingerprint = content_sha256[:32]
        updated_at = _parse_timestamp(node.get("obj_edit_time")) or fallback_updated_at
        metadata = {
            "source": DocumentSource.FEISHU_WIKI.value,
            "wiki_space_id": self.space_id,
            "wiki_node_token": node_token,
            "wiki_object_token": object_token,
            "wiki_object_type": str(node.get("obj_type") or "file"),
            "wiki_parent_node_token": str(node.get("parent_node_token") or self.root_node_token),
            "wiki_url": f"https://feishu.cn/wiki/{node_token}",
            "source_updated_at": updated_at.isoformat(),
            "content_sha256": content_sha256,
        }
        return Document(
            id=f"feishu_wiki:{self.space_id}:{node_token}",
            source=DocumentSource.FEISHU_WIKI,
            semantic_identifier=title,
            extension=get_file_ext(title),
            blob=blob,
            doc_updated_at=updated_at,
            size_bytes=len(blob),
            metadata=metadata,
            fingerprint=fingerprint,
        )

    def _yield_documents(
        self,
        *,
        start: SecondsSinceUnixEpoch | None,
        end: SecondsSinceUnixEpoch | None,
    ) -> GenerateDocumentsOutput:
        fallback_updated_at = datetime.fromtimestamp(end, tz=UTC) if end is not None else datetime.now(UTC)
        batch: list[Document] = []
        for node in self._iter_nodes(self.root_node_token):
            if node.get("obj_type") != "file":
                continue
            title = str(node.get("title") or "")
            if not self._matches_filters(title):
                continue
            updated_at = _parse_timestamp(node.get("obj_edit_time"))
            if not self._is_within_window(updated_at, start, end):
                continue
            try:
                batch.append(self._node_to_document(node, fallback_updated_at=fallback_updated_at))
            except ConnectorValidationError:
                raise
            except Exception as exc:
                node_token = str(node.get("node_token") or "<unknown>")
                LOGGER.exception("Failed to read Feishu Wiki node %s", node_token)
                raise ConnectorValidationError(f"Failed to read Feishu Wiki node {node_token}") from exc
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

    def validate_connector_settings(self) -> None:
        self._ensure_configured()
        self._get_access_token()
        next(self._iter_child_pages(self.root_node_token), [])

    def load_from_state(self) -> GenerateDocumentsOutput:
        yield from self._yield_documents(start=None, end=None)

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        yield from self._yield_documents(start=start, end=end)
