"""Generic, configuration-driven REST API data source connector.

Connect any REST API as a RAGFlow data source without code changes.
All behaviour — URL, auth, pagination, field mapping — is controlled
via the ``RestAPIConnectorConfig`` schema exposed by the UI.
"""

from __future__ import annotations

import json
import logging
import re
from datetime import datetime, timezone
from typing import Any, Dict, Generator, Iterable, List, Mapping, Optional
from urllib.parse import parse_qs, urlparse, urlunparse

import requests
from pydantic import BaseModel, Field, HttpUrl, ValidationError, field_validator

from api.utils.common import hash128
from common.data_source.config import INDEX_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
)
from common.data_source.interfaces import (
    LoadConnector,
    PollConnector,
    SecondsSinceUnixEpoch,
)
from common.data_source.models import Document
from common.data_source.utils import rl_requests, retry_builder

try:
    from jsonpath import jsonpath as _jsonpath  # type: ignore[import]
except Exception:  # pragma: no cover – defensive import
    _jsonpath = None

logger = logging.getLogger(__name__)

# ===================================================================== #
# Constants                                                             #
# ===================================================================== #

class AuthType:
    NONE = "none"
    API_KEY_HEADER = "api_key_header"
    BEARER = "bearer"
    BASIC = "basic"


class PaginationType:
    NONE = "none"
    PAGE = "page"
    OFFSET = "offset"
    CURSOR = "cursor"


_AUTH_TYPES = {AuthType.NONE, AuthType.API_KEY_HEADER, AuthType.BEARER, AuthType.BASIC}
_PAGINATION_TYPES = {PaginationType.NONE, PaginationType.PAGE, PaginationType.OFFSET, PaginationType.CURSOR}
_SENSITIVE_HEADERS = {"authorization", "apikey", "api-key", "x-api-key"}


# ===================================================================== #
# Configuration schema (Pydantic)                                       #
# ===================================================================== #

class RestAPIConnectorConfig(BaseModel):
    """Validated schema for the REST API connector (used by UI and backend)."""

    url: HttpUrl
    method: str = "GET"
    headers: Dict[str, str] = Field(default_factory=dict)
    query_params: Dict[str, str] = Field(default_factory=dict)

    auth_type: str = AuthType.NONE
    auth_config: Dict[str, Any] = Field(default_factory=dict)

    items_path: Optional[str] = None
    id_field: Optional[str] = None
    content_fields: List[str] = Field(default_factory=list)
    metadata_fields: List[str] = Field(default_factory=list)

    pagination_type: str = PaginationType.NONE
    pagination_config: Dict[str, Any] = Field(default_factory=dict)

    poll_timestamp_field: Optional[str] = None
    request_body: Optional[Dict[str, Any]] = None

    field_type_hints: Dict[str, str] = Field(default_factory=dict)
    field_default_values: Dict[str, Any] = Field(default_factory=dict)
    content_template: Optional[str] = None

    batch_size: int = INDEX_BATCH_SIZE
    max_pages: int = 1000

    # ── Validators ──────────────────────────────────────────────────── #

    @field_validator("headers", mode="before")
    @classmethod
    def _coerce_headers(cls, v: Any) -> Dict[str, str]:
        return _text_to_dict(v)

    @field_validator("query_params", mode="before")
    @classmethod
    def _coerce_query_params(cls, v: Any) -> Dict[str, str]:
        return _text_to_dict(v)

    @field_validator("content_fields", "metadata_fields", mode="before")
    @classmethod
    def _coerce_field_list(cls, v: Any) -> List[str]:
        if v is None or v == "":
            return []
        if isinstance(v, str):
            return [p.strip() for p in v.split(",") if p.strip()]
        if isinstance(v, list):
            return [str(p).strip() for p in v if str(p).strip()]
        return v

    # ── Semantic checks ─────────────────────────────────────────────── #

    def normalized_method(self) -> str:
        m = (self.method or "GET").upper()
        if m not in {"GET", "POST"}:
            raise ConnectorValidationError(f"Unsupported HTTP method '{m}'. Only GET and POST are allowed.")
        return m

    def normalized_auth_type(self) -> str:
        if self.auth_type not in _AUTH_TYPES:
            raise ConnectorValidationError(f"Unsupported auth_type '{self.auth_type}'.")
        return self.auth_type

    def normalized_pagination_type(self) -> str:
        if self.pagination_type not in _PAGINATION_TYPES:
            raise ConnectorValidationError(f"Unsupported pagination_type '{self.pagination_type}'.")
        return self.pagination_type

    def ensure_required_fields(self) -> None:
        if not self.content_fields:
            raise ConnectorValidationError("At least one content field must be configured (content_fields).")


# ===================================================================== #
# Module-level helpers                                                  #
# ===================================================================== #

def _text_to_dict(v: Any) -> Dict[str, str]:
    """Parse a dict, JSON string, or ``key=value\\n`` text into a dict."""
    if v is None or v == "":
        return {}
    if isinstance(v, dict):
        return {str(k): str(vv) for k, vv in v.items()}
    if isinstance(v, str):
        try:
            parsed = json.loads(v)
            if isinstance(parsed, dict):
                return {str(k): str(vv) for k, vv in parsed.items()}
        except Exception:
            pass
        result: Dict[str, str] = {}
        for line in v.splitlines():
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            if "=" in line:
                k, _, val = line.partition("=")
                result[k.strip()] = val.strip()
        return result
    return v


def _strip_html(text: str) -> str:
    """Remove basic HTML tags and normalise whitespace."""
    if "<" not in text or ">" not in text:
        return text
    cleaned = re.sub(r"<[^>]+>", " ", text)
    return re.sub(r"\s+", " ", cleaned).strip()


def _coerce_to_text(value: Any) -> str:
    """Convert any value to a plain-text string."""
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    if isinstance(value, (int, float, bool)):
        return str(value)
    try:
        return json.dumps(value, ensure_ascii=False)
    except Exception:
        return str(value)


def _serialize_metadata_value(value: Any) -> Any:
    """Serialise a metadata value for storage."""
    if isinstance(value, datetime):
        if value.tzinfo is None:
            value = value.replace(tzinfo=timezone.utc)
        return value.isoformat()
    if isinstance(value, (int, float, bool, str)):
        return value
    try:
        return json.dumps(value, ensure_ascii=False)
    except Exception:
        return str(value)


# ===================================================================== #
# RestAPIConnector                                                      #
# ===================================================================== #

class RestAPIConnector(LoadConnector, PollConnector):
    """Configuration-driven REST API connector."""

    # ── Init ────────────────────────────────────────────────────────── #

    def __init__(
        self,
        url: str,
        method: str = "GET",
        headers: Optional[Dict[str, str]] = None,
        query_params: Optional[Dict[str, str]] = None,
        auth_type: str = AuthType.NONE,
        auth_config: Optional[Dict[str, Any]] = None,
        items_path: Optional[str] = None,
        id_field: Optional[str] = None,
        content_fields: Optional[List[str]] = None,
        metadata_fields: Optional[List[str]] = None,
        pagination_type: str = PaginationType.NONE,
        pagination_config: Optional[Dict[str, Any]] = None,
        poll_timestamp_field: Optional[str] = None,
        batch_size: int = INDEX_BATCH_SIZE,
        max_pages: int = 1000,
        request_body: Optional[Dict[str, Any]] = None,
        field_type_hints: Optional[Dict[str, str]] = None,
        field_default_values: Optional[Dict[str, Any]] = None,
        content_template: Optional[str] = None,
    ) -> None:
        # Separate any query params already embedded in the URL so that
        # pagination params don't create duplicates.
        parsed = urlparse(str(url))
        self._base_url = urlunparse((parsed.scheme, parsed.netloc, parsed.path, "", "", ""))
        self._url_params: Dict[str, str] = {}
        if parsed.query:
            for k, v_list in parse_qs(parsed.query, keep_blank_values=True).items():
                self._url_params[k] = v_list[-1]

        self._explicit_query_params: Dict[str, str] = query_params or {}
        self.url = self._base_url
        self.method = (method or "GET").upper()
        self._base_headers: Dict[str, str] = headers or {}
        self.auth_type = auth_type or AuthType.NONE
        self.auth_config: Dict[str, Any] = auth_config or {}
        self.items_path = items_path
        self.id_field = id_field
        self.content_fields: List[str] = content_fields or []
        self.metadata_fields: List[str] = metadata_fields or []
        self.pagination_type = pagination_type or PaginationType.NONE
        self.pagination_config: Dict[str, Any] = pagination_config or {}
        self._static_request_body: Dict[str, Any] = (
            request_body if request_body is not None
            else self.pagination_config.get("request_body") or {}
        )
        self.poll_timestamp_field = poll_timestamp_field
        self.batch_size = batch_size
        self.max_pages = max_pages
        self.field_type_hints: Dict[str, str] = field_type_hints or {}
        self.field_default_values: Dict[str, Any] = field_default_values or {}
        self.content_template = content_template

        self._credentials: Dict[str, Any] = {}
        self._auth_headers: Dict[str, str] = {}
        self._basic_auth: Optional[requests.auth.HTTPBasicAuth] = None

    # ── Credentials ─────────────────────────────────────────────────── #

    def load_credentials(self, credentials: Dict[str, Any]) -> Dict[str, Any] | None:
        """Apply authentication credentials (no network call).

        Use ``validate_config()`` to perform a live connectivity check.
        """
        self._credentials = credentials or {}
        self._build_auth()
        return None

    def _build_auth(self) -> None:
        self._auth_headers = {}
        self._basic_auth = None

        if self.auth_type == AuthType.NONE:
            return

        if self.auth_type == AuthType.API_KEY_HEADER:
            header_name = self.auth_config.get("header_name")
            api_key = (
                self._credentials.get("api_key")
                or self.auth_config.get("api_key_value")
                or self.auth_config.get("api_key")
            )
            if not header_name or not api_key:
                raise ConnectorMissingCredentialError(
                    "REST API (api_key_header) requires 'header_name' in auth_config and 'api_key' in credentials"
                )
            self._auth_headers[header_name] = str(api_key)
            return

        if self.auth_type == AuthType.BEARER:
            token = self._credentials.get("token") or self.auth_config.get("token")
            if not token:
                raise ConnectorMissingCredentialError("REST API (bearer) requires 'token' in credentials")
            self._auth_headers["Authorization"] = f"Bearer {token}"
            return

        if self.auth_type == AuthType.BASIC:
            username = self._credentials.get("username") or self.auth_config.get("username")
            password = self._credentials.get("password") or self.auth_config.get("password")
            if not username or password is None:
                raise ConnectorMissingCredentialError("REST API (basic) requires 'username' and 'password'")
            self._basic_auth = requests.auth.HTTPBasicAuth(str(username), str(password))
            return

        raise ConnectorValidationError(f"Unsupported auth_type: {self.auth_type}")

    # ── Config validation (test connection) ─────────────────────────── #

    @classmethod
    def validate_config(
        cls,
        config: Dict[str, Any],
        credentials: Optional[Dict[str, Any]] = None,
    ) -> RestAPIConnectorConfig:
        """Validate config schema and optionally perform a live API call."""
        try:
            cfg = RestAPIConnectorConfig(**config)
        except ValidationError as exc:
            raise ConnectorValidationError(f"Invalid REST API config: {exc}") from exc

        cfg.normalized_method()
        cfg.normalized_auth_type()
        cfg.normalized_pagination_type()
        cfg.ensure_required_fields()

        if credentials is None and cfg.auth_type != AuthType.NONE:
            return cfg

        connector = cls(
            url=str(cfg.url),
            method=cfg.normalized_method(),
            headers=cfg.headers,
            query_params=cfg.query_params,
            auth_type=cfg.normalized_auth_type(),
            auth_config=cfg.auth_config,
            items_path=cfg.items_path,
            id_field=cfg.id_field,
            content_fields=cfg.content_fields,
            metadata_fields=cfg.metadata_fields,
            pagination_type=cfg.normalized_pagination_type(),
            pagination_config=cfg.pagination_config,
            poll_timestamp_field=cfg.poll_timestamp_field,
            batch_size=cfg.batch_size,
            max_pages=min(cfg.max_pages, 10),
            request_body=cfg.request_body,
            field_type_hints=cfg.field_type_hints,
            field_default_values=cfg.field_default_values,
            content_template=cfg.content_template,
        )

        if credentials is not None:
            connector.load_credentials(credentials)
        else:
            connector._credentials = {}
            connector._build_auth()

        try:
            logger.info("Validating REST API connector by fetching first page")
            _ = next(connector._page_iter_for_validation())
        except StopIteration:
            pass

        return cfg

    # ── LoadConnector / PollConnector interface ─────────────────────── #

    def load_from_state(self) -> Generator[List[Document], None, None]:
        return self._yield_documents(time_window=None)

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> Generator[List[Document], None, None]:
        if not self.poll_timestamp_field:
            logger.warning(
                "poll_source called without poll_timestamp_field; falling back to full fetch with in-memory filtering."
            )
        return self._yield_documents(
            time_window=(
                datetime.fromtimestamp(start, tz=timezone.utc),
                datetime.fromtimestamp(end, tz=timezone.utc),
            )
        )

    # ── Document generation ─────────────────────────────────────────── #

    def _yield_documents(
        self,
        time_window: tuple[datetime, datetime] | None,
    ) -> Generator[List[Document], None, None]:
        batch: List[Document] = []
        for item in self._iter_items(time_window=time_window):
            try:
                doc = self._item_to_document(item)
            except Exception as exc:
                logger.warning("Failed to convert REST API item to Document: %s", exc)
                continue

            if time_window is not None and not self._doc_in_time_window(doc, *time_window):
                continue

            batch.append(doc)
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    # ── Pagination & page fetching ──────────────────────────────────── #

    def _iter_items(
        self,
        time_window: tuple[datetime, datetime] | None,
    ) -> Iterable[Mapping[str, Any]]:
        """Iterate over raw items across all pages."""
        page_count = 0

        page = int(self.pagination_config.get("start_page", 1))
        per_page = int(self.pagination_config.get("page_size", self.batch_size))
        if per_page <= 0:
            per_page = self.batch_size

        offset = int(self.pagination_config.get("start_offset", 0))
        limit = int(self.pagination_config.get("limit", per_page))
        if limit <= 0:
            limit = per_page

        cursor: Optional[str] = self.pagination_config.get("initial_cursor")

        while True:
            if page_count >= self.max_pages:
                logger.warning("REST API connector reached max_pages=%d, stopping.", self.max_pages)
                break

            params: Dict[str, Any] = {}
            if self.pagination_type == PaginationType.PAGE:
                self._apply_page_pagination(params, page, per_page)
            elif self.pagination_type == PaginationType.OFFSET:
                self._apply_offset_pagination(params, offset, limit)
            elif self.pagination_type == PaginationType.CURSOR and cursor is not None:
                self._apply_cursor_pagination(params, cursor)

            try:
                response_json = self._fetch_page(params)
            except (ConnectorValidationError, ConnectorMissingCredentialError):
                raise
            except Exception as exc:
                raise ConnectorValidationError(f"REST API page fetch failed: {exc}") from exc

            items = self._extract_items(response_json)
            if not items:
                break

            for item in items:
                if isinstance(item, Mapping):
                    yield item

            page_count += 1

            if self.pagination_type == PaginationType.NONE:
                break
            elif self.pagination_type == PaginationType.PAGE:
                if len(items) < per_page:
                    break
                page += 1
            elif self.pagination_type == PaginationType.OFFSET:
                if len(items) < limit:
                    break
                offset += limit
            elif self.pagination_type == PaginationType.CURSOR:
                next_cursor = self._extract_next_cursor(response_json)
                if not next_cursor:
                    break
                cursor = next_cursor

    def _page_iter_for_validation(self) -> Iterable[Mapping[str, Any]]:
        """Single-page iterator used only for connectivity checks."""
        response_json = self._fetch_page(params={})
        for item in self._extract_items(response_json):
            yield item

    @retry_builder(
        tries=5, delay=1, max_delay=30, backoff=2,
        exceptions=(requests.ConnectionError, requests.Timeout, requests.HTTPError),
    )
    def _fetch_page(self, params: Dict[str, Any]) -> Any:
        """Fetch a single page with retry and exponential backoff."""
        headers = {**self._base_headers, **self._auth_headers}

        merged: Dict[str, Any] = {**self._url_params}
        merged.update(self._explicit_query_params)
        merged.update(params)

        url, query_params = self._build_url_with_templates(merged)

        logger.debug(
            "REST API request: %s %s | params=%s | headers=%s",
            self.method, url,
            {k: ("***" if k.lower() in _SENSITIVE_HEADERS else v) for k, v in query_params.items()},
            {k: ("***" if k.lower() in _SENSITIVE_HEADERS else v) for k, v in headers.items()},
        )

        if self.method == "GET":
            resp = rl_requests.get(url, headers=headers, params=query_params, auth=self._basic_auth, timeout=60)
        elif self.method == "POST":
            resp = rl_requests.post(
                url, headers=headers, params=query_params,
                json=self._static_request_body or {}, auth=self._basic_auth, timeout=60,
            )
        else:
            raise ConnectorValidationError(f"Unsupported HTTP method: {self.method}")

        try:
            resp.raise_for_status()
        except requests.HTTPError as exc:
            status = exc.response.status_code if exc.response is not None else None
            if status in (401, 403):
                raise ConnectorMissingCredentialError(
                    f"REST API authentication failed with status {status}"
                ) from exc
            raise

        try:
            return resp.json()
        except ValueError as exc:
            raise ConnectorValidationError("REST API response is not valid JSON") from exc

    def _build_url_with_templates(self, params: Dict[str, Any]) -> tuple[str, Dict[str, Any]]:
        """Substitute ``{key}`` placeholders in the URL and return remaining query params."""
        url = self.url
        query_params = dict(params)
        used_keys: List[str] = []
        for key, value in list(query_params.items()):
            placeholder = "{" + key + "}"
            if placeholder in url:
                url = url.replace(placeholder, str(value))
                used_keys.append(key)
        for key in used_keys:
            query_params.pop(key, None)
        return url, query_params

    # ── Pagination helpers ──────────────────────────────────────────── #

    def _apply_page_pagination(self, params: Dict[str, Any], page: int, per_page: int) -> None:
        params[self.pagination_config.get("page_param", "page")] = page
        size_param = self.pagination_config.get("page_size_param")
        if size_param:
            params[size_param] = per_page

    def _apply_offset_pagination(self, params: Dict[str, Any], offset: int, limit: int) -> None:
        params[self.pagination_config.get("offset_param", "offset")] = offset
        limit_param = self.pagination_config.get("limit_param")
        if limit_param:
            params[limit_param] = limit

    def _apply_cursor_pagination(self, params: Dict[str, Any], cursor: str) -> None:
        params[self.pagination_config.get("cursor_param", "cursor")] = cursor

    # ── JSON extraction ─────────────────────────────────────────────── #

    def _extract_items(self, response_json: Any) -> List[Mapping[str, Any]]:
        """Extract the items array from a JSON response."""
        if self.items_path and _jsonpath is not None:
            try:
                matches = _jsonpath(response_json, self.items_path)
            except Exception as exc:
                raise ConnectorValidationError(
                    f"Failed to apply items JSONPath '{self.items_path}': {exc}"
                ) from exc
            if not matches:
                return []
            if len(matches) == 1 and isinstance(matches[0], list):
                items = matches[0]
            else:
                items = matches
        elif isinstance(response_json, list):
            items = response_json
        elif isinstance(response_json, dict):
            items = []
            for key in ("items", "results", "data", "records"):
                if key in response_json and isinstance(response_json[key], list):
                    items = response_json[key]
                    break
            else:
                for value in response_json.values():
                    if isinstance(value, list):
                        items = value
                        break
        else:
            items = []

        return [it for it in items if isinstance(it, Mapping)]

    def _extract_next_cursor(self, response_json: Any) -> Optional[str]:
        cursor_path = self.pagination_config.get("next_cursor_path")
        if not cursor_path:
            field = self.pagination_config.get("next_cursor_field")
            if field and isinstance(response_json, Mapping):
                value = response_json.get(field)
                return str(value) if value is not None else None
            return None

        if _jsonpath is None:
            return None

        try:
            matches = _jsonpath(response_json, cursor_path)
        except Exception:
            return None

        if not matches:
            return None
        return str(matches[0]) if matches[0] is not None else None

    # ── Item → Document mapping ─────────────────────────────────────── #

    def _item_to_document(self, item: Mapping[str, Any]) -> Document:
        # ID
        raw_id = self._get_typed_field_value(self.id_field, item) if self.id_field else None
        if raw_id is None:
            raw_id = hash128(f"rest_api_item:{repr(item)}")
        doc_id = hash128(f"rest_api:{raw_id}")

        # Content
        if self.content_template:
            content_text = self._render_content_template(item)
        else:
            parts = []
            for field in self.content_fields:
                val = self._get_typed_field_value(field, item)
                if val is not None:
                    text = _strip_html(_coerce_to_text(val))
                    if text:
                        parts.append(text)
            content_text = "\n\n".join(parts)
        blob = content_text.encode("utf-8")

        # Metadata
        metadata: Dict[str, Any] = {}
        for field in self.metadata_fields:
            value = self._get_typed_field_value(field, item)
            if value is not None:
                metadata[field] = _serialize_metadata_value(value)

        # Timestamp
        doc_updated_at = self._extract_timestamp(item) or datetime.now(timezone.utc)

        # Semantic identifier
        sem = str(self._extract_field(item, self.content_fields[0]) if self.content_fields else raw_id)
        sem = sem.replace("\n", " ").replace("\r", " ").strip()[:100] or str(doc_id)

        return Document(
            id=doc_id,
            source=DocumentSource.REST_API,
            semantic_identifier=sem,
            extension=".txt",
            blob=blob,
            doc_updated_at=doc_updated_at,
            size_bytes=len(blob),
            metadata=metadata or None,
        )

    # ── Field extraction ────────────────────────────────────────────── #

    def _extract_field(self, item: Mapping[str, Any], path: str) -> Any:
        """Extract a value using dot-notation with optional array indexing.

        Examples: ``country.name``, ``tags[0].label``, ``tags[*].label``
        """
        values = self._extract_field_values(item, path)
        if not values:
            return None
        return values[0] if len(values) == 1 else values

    def _extract_field_values(self, item: Mapping[str, Any], path: str) -> List[Any]:
        if not path:
            return []

        current_values: List[Any] = [item]
        for segment in path.split("."):
            if not segment:
                return []

            match = re.match(r'^(?P<key>[^\[\]]+)(\[(?P<index>\d+|\*)\])?$', segment)
            key = segment
            index: Optional[str] = None
            if match:
                key = match.group("key")
                index = match.group("index")

            next_values: List[Any] = []
            for value in current_values:
                if not isinstance(value, Mapping):
                    continue
                child = value.get(key)
                if child is None:
                    continue
                if index is None:
                    next_values.append(child)
                elif not isinstance(child, list):
                    continue
                elif index == "*":
                    next_values.extend(child)
                else:
                    try:
                        idx = int(index)
                    except ValueError:
                        continue
                    if 0 <= idx < len(child):
                        next_values.append(child[idx])

            current_values = next_values
            if not current_values:
                break

        return current_values

    def _get_typed_field_value(self, path: str, item: Mapping[str, Any]) -> Any:
        """Extract a field value, applying type hints, defaults, and array joining."""
        values = self._extract_field_values(item, path)
        if not values:
            return self.field_default_values.get(path)

        hint = self.field_type_hints.get(path)

        def _convert(v: Any) -> Any:
            if hint == "string":
                return "" if v is None else str(v)
            if hint == "number":
                if v is None:
                    return None
                try:
                    num = float(v)
                    return int(num) if num.is_integer() else num
                except Exception:
                    return None
            if hint == "date":
                if isinstance(v, datetime):
                    return v.isoformat()
                if isinstance(v, str):
                    dt = self._extract_timestamp({"_": v})
                    return dt.isoformat() if dt else str(v)
                return str(v) if v is not None else None
            return v

        converted = [_convert(v) for v in values]
        non_null = [v for v in converted if v is not None]
        if not non_null:
            return None
        if len(non_null) == 1:
            return non_null[0]
        return ", ".join(_coerce_to_text(v) for v in non_null)

    # ── Timestamp parsing ───────────────────────────────────────────── #

    def _extract_timestamp(self, item: Mapping[str, Any]) -> Optional[datetime]:
        if not self.poll_timestamp_field:
            return None

        value = self._extract_field(item, self.poll_timestamp_field)
        if isinstance(value, list) and value:
            value = value[0]
        if value is None:
            return None

        if isinstance(value, datetime):
            return (value if value.tzinfo else value.replace(tzinfo=timezone.utc)).astimezone(timezone.utc)

        if isinstance(value, (int, float)):
            try:
                return datetime.fromtimestamp(float(value), tz=timezone.utc)
            except Exception:
                return None

        if isinstance(value, str):
            ts = value.strip()
            for fmt in ("%Y-%m-%dT%H:%M:%S.%fZ", "%Y-%m-%dT%H:%M:%SZ", "%Y-%m-%d %H:%M:%S", "%Y-%m-%d"):
                try:
                    return datetime.strptime(ts, fmt).replace(tzinfo=timezone.utc)
                except Exception:
                    continue
            try:
                dt = datetime.fromisoformat(ts.replace("Z", "+00:00").replace(" ", "T"))
                return (dt if dt.tzinfo else dt.replace(tzinfo=timezone.utc)).astimezone(timezone.utc)
            except Exception:
                return None

        return None

    # ── Content template rendering ──────────────────────────────────── #

    def _render_content_template(self, item: Mapping[str, Any]) -> str:
        template = self.content_template or ""

        class _SafeDict(dict):
            def __missing__(self, key: str) -> str:
                return ""

        values: Dict[str, str] = {}
        for field_path in set(self.content_fields + self.metadata_fields):
            val = self._get_typed_field_value(field_path, item)
            if val is None:
                continue
            name = re.sub(r"\[\d+\]|\[\*\]", "", field_path.split(".")[-1]) or field_path
            values[name] = _coerce_to_text(val)

        try:
            rendered = template.format_map(_SafeDict(values))
        except Exception as exc:
            logger.warning("Failed to render content template: %s", exc)
            parts = [_coerce_to_text(self._get_typed_field_value(f, item)) for f in self.content_fields]
            rendered = "\n\n".join(p for p in parts if p)

        return _strip_html(rendered)

    # ── Misc helpers ────────────────────────────────────────────────── #

    @staticmethod
    def _doc_in_time_window(doc: Document, start: datetime, end: datetime) -> bool:
        if not doc.doc_updated_at:
            return False
        dt = doc.doc_updated_at
        dt = (dt if dt.tzinfo else dt.replace(tzinfo=timezone.utc)).astimezone(timezone.utc)
        return start <= dt < end
