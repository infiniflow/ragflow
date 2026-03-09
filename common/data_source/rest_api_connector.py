"""Generic REST API data source connector.

This connector is **configuration-driven** and is intended to be wired to a
UI-based configuration model in follow-up tickets. For Ticket 1 the focus is
on the core load / poll behavior and mapping to `Document` objects.
"""

from __future__ import annotations

import logging
import re
from datetime import datetime, timezone
from typing import Any, Dict, Generator, Iterable, List, Mapping, Optional

import requests
from pydantic import BaseModel, Field, HttpUrl, ValidationError

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
    # `jsonpath` is already present in the dependency graph (via akshare),
    # so we don't need to introduce a new project dependency.
    from jsonpath import jsonpath as _jsonpath  # type: ignore[import]
except Exception:  # pragma: no cover - defensive import
    _jsonpath = None


class AuthType(str):
    NONE = "none"
    API_KEY_HEADER = "api_key_header"
    BEARER = "bearer"
    BASIC = "basic"


class PaginationType(str):
    NONE = "none"
    PAGE = "page"
    OFFSET = "offset"
    CURSOR = "cursor"


class RestAPIConnectorConfig(BaseModel):
    """Schema for REST API connector configuration (used by UI and backend)."""

    url: HttpUrl
    method: str = "GET"
    headers: Dict[str, str] = Field(default_factory=dict)

    auth_type: str = AuthType.NONE
    auth_config: Dict[str, Any] = Field(default_factory=dict)

    items_path: Optional[str] = None
    id_field: Optional[str] = None
    content_fields: List[str] = Field(default_factory=list)
    metadata_fields: List[str] = Field(default_factory=list)

    pagination_type: str = PaginationType.NONE
    pagination_config: Dict[str, Any] = Field(default_factory=dict)

    poll_timestamp_field: Optional[str] = None
    # Only used for POST; for GET this is ignored.
    request_body: Optional[Dict[str, Any]] = None

    batch_size: int = INDEX_BATCH_SIZE
    max_pages: int = 1000

    def normalized_method(self) -> str:
        method = (self.method or "GET").upper()
        if method not in {"GET", "POST"}:
            raise ConnectorValidationError(
                f"Unsupported HTTP method '{self.method}'. Only GET and POST are allowed."
            )
        return method

    def normalized_auth_type(self) -> str:
        if self.auth_type not in {
            AuthType.NONE,
            AuthType.API_KEY_HEADER,
            AuthType.BEARER,
            AuthType.BASIC,
        }:
            raise ConnectorValidationError(f"Unsupported auth_type '{self.auth_type}'.")
        return self.auth_type

    def normalized_pagination_type(self) -> str:
        if self.pagination_type not in {
            PaginationType.NONE,
            PaginationType.PAGE,
            PaginationType.OFFSET,
            PaginationType.CURSOR,
        }:
            raise ConnectorValidationError(
                f"Unsupported pagination_type '{self.pagination_type}'."
            )
        return self.pagination_type

    def ensure_required_fields(self) -> None:
        if not self.content_fields:
            raise ConnectorValidationError(
                "At least one content field must be configured (content_fields)."
            )


class RestAPIConnector(LoadConnector, PollConnector):
    """Configuration-driven REST API connector.

    Parameters here are intentionally generic so that Ticket 2 can plug a
    validated config object directly into this class without changes.
    """

    def __init__(
        self,
        url: str,
        method: str = "GET",
        headers: Optional[Dict[str, str]] = None,
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
    ) -> None:
        self.url = url
        self.method = (method or "GET").upper()
        self._base_headers = headers or {}
        self.auth_type = auth_type or AuthType.NONE
        self.auth_config = auth_config or {}
        self.items_path = items_path
        self.id_field = id_field
        self.content_fields: List[str] = content_fields or []
        self.metadata_fields: List[str] = metadata_fields or []
        self.pagination_type = pagination_type or PaginationType.NONE
        self.pagination_config = pagination_config or {}
        # Allow POST body to be provided either via dedicated config field or
        # embedded in pagination_config for backward-compat.
        if request_body is None:
            request_body = self.pagination_config.get("request_body") or {}
        self._static_request_body: Dict[str, Any] = request_body
        self.poll_timestamp_field = poll_timestamp_field
        self.batch_size = batch_size
        self.max_pages = max_pages

        # Populated by `load_credentials`
        self._credentials: Dict[str, Any] = {}
        self._auth_headers: Dict[str, str] = {}
        self._basic_auth: Optional[requests.auth.HTTPBasicAuth] = None

    # --------------------------------------------------------------------- #
    # Credentials & validation
    # --------------------------------------------------------------------- #
    def load_credentials(self, credentials: Dict[str, Any]) -> Dict[str, Any] | None:
        """Load credentials and validate API connectivity.

        Expected `credentials` content depends on `auth_type`:
        - none:         no required keys
        - api_key_header:
              - "api_key": secret value
              - header name provided via `auth_config["header_name"]`
        - bearer:
              - "token": bearer token
        - basic:
              - "username"
              - "password"
        """
        self._credentials = credentials or {}
        self._build_auth()

        # Basic connectivity check – small single-page fetch.
        try:
            logging.info("Validating REST API connector by fetching first page")
            _ = next(self._page_iter_for_validation())
        except StopIteration:
            # No items is fine; connectivity is proven by lack of exception.
            pass
        except ConnectorValidationError:
            raise
        except Exception as exc:
            raise ConnectorValidationError(
                f"Failed to validate REST API connector: {exc}"
            ) from exc

        return None

    # ------------------------------------------------------------------ #
    # Static configuration validation entrypoint 
    # ------------------------------------------------------------------ #
    @classmethod
    def validate_config(
        cls,
        config: Dict[str, Any],
        credentials: Optional[Dict[str, Any]] = None,
    ) -> RestAPIConnectorConfig:
        """Validate connector configuration and perform a test API call.

        - Ensures required fields are present and values are in allowed sets.
        - Normalizes defaults (method, auth_type, pagination_type, etc.).
        - If `credentials` are provided (or auth_type == none), performs a
          single-page API call and verifies that `items_path` (if any) yields
          at least a structurally valid items list.
        """
        try:
            cfg = RestAPIConnectorConfig(**config)
        except ValidationError as exc:
            raise ConnectorValidationError(f"Invalid REST API config: {exc}") from exc

        # Semantic validations
        cfg.normalized_method()
        cfg.normalized_auth_type()
        cfg.normalized_pagination_type()
        cfg.ensure_required_fields()

        # Short-circuit: only structural validation requested, no network.
        if credentials is None and cfg.auth_type != AuthType.NONE:
            return cfg

        # Connectivity + items_path validation.
        connector = cls(
            url=str(cfg.url),
            method=cfg.normalized_method(),
            headers=cfg.headers,
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
            max_pages=min(cfg.max_pages, 10),  # cap for validation
            request_body=cfg.request_body,
        )

        if credentials is not None:
            connector.load_credentials(credentials)
        else:
            connector._credentials = {}
            connector._build_auth()
            try:
                logging.info("Validating REST API connector via test call (no auth).")
                _ = next(connector._page_iter_for_validation())
            except StopIteration:
                pass

        return cfg

    def _build_auth(self) -> None:
        """Derive auth headers / objects from `auth_type` and credentials."""
        self._auth_headers = {}
        self._basic_auth = None

        if self.auth_type == AuthType.NONE:
            return

        if self.auth_type == AuthType.API_KEY_HEADER:
            header_name = self.auth_config.get("header_name")
            api_key = self._credentials.get("api_key")
            if not header_name or not api_key:
                raise ConnectorMissingCredentialError(
                    "REST API (api_key_header) requires 'header_name' in auth_config "
                    "and 'api_key' in credentials"
                )
            self._auth_headers[header_name] = str(api_key)
            return

        if self.auth_type == AuthType.BEARER:
            token = self._credentials.get("token")
            if not token:
                raise ConnectorMissingCredentialError(
                    "REST API (bearer) requires 'token' in credentials"
                )
            self._auth_headers["Authorization"] = f"Bearer {token}"
            return

        if self.auth_type == AuthType.BASIC:
            username = self._credentials.get("username")
            password = self._credentials.get("password")
            if not username or password is None:
                raise ConnectorMissingCredentialError(
                    "REST API (basic) requires 'username' and 'password' in credentials"
                )
            self._basic_auth = requests.auth.HTTPBasicAuth(str(username), str(password))
            return

        raise ConnectorValidationError(f"Unsupported auth_type: {self.auth_type}")

    # --------------------------------------------------------------------- #
    # Core loading / polling
    # --------------------------------------------------------------------- #
    def load_from_state(self) -> Generator[List[Document], None, None]:
        """Full fetch with pagination."""
        return self._yield_documents(time_window=None)

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> Generator[List[Document], None, None]:
        """Incremental fetch using configured timestamp field."""
        if not self.poll_timestamp_field:
            logging.warning(
                "REST API connector poll_source called without poll_timestamp_field. "
                "Falling back to full fetch and in-memory filtering by doc_updated_at."
            )
        start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
        end_dt = datetime.fromtimestamp(end, tz=timezone.utc)
        return self._yield_documents(time_window=(start_dt, end_dt))

    def _yield_documents(
        self,
        time_window: tuple[datetime, datetime] | None,
    ) -> Generator[List[Document], None, None]:
        """Iterate over pages and yield `Document` batches."""
        batch: List[Document] = []
        for item in self._iter_items(time_window=time_window):
            try:
                doc = self._item_to_document(item)
            except Exception as exc:
                logging.warning(f"Failed to convert REST API item to Document: {exc}")
                continue

            if time_window is not None and not self._doc_in_time_window(
                doc, *time_window
            ):
                continue

            batch.append(doc)
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    # --------------------------------------------------------------------- #
    # Pagination & page fetching
    # --------------------------------------------------------------------- #
    def _iter_items(
        self,
        time_window: tuple[datetime, datetime] | None,
    ) -> Iterable[Mapping[str, Any]]:
        """Iterate over raw items across all pages."""
        page_count = 0
        page = int(self.pagination_config.get("start_page", 1))
        per_page = int(self.pagination_config.get("page_size", self.batch_size))
        offset = int(self.pagination_config.get("start_offset", 0))
        limit = int(self.pagination_config.get("limit", per_page))
        cursor: Optional[str] = self.pagination_config.get("initial_cursor")

        while True:
            if page_count >= self.max_pages:
                logging.warning(
                    "REST API connector reached max_pages=%d. Stopping pagination.",
                    self.max_pages,
                )
                break

            params: Dict[str, Any] = {}

            if self.pagination_type == PaginationType.PAGE:
                page_param = self.pagination_config.get("page_param", "page")
                size_param = self.pagination_config.get("page_size_param", "per_page")
                params[page_param] = page
                params[size_param] = per_page
            elif self.pagination_type == PaginationType.OFFSET:
                offset_param = self.pagination_config.get("offset_param", "offset")
                limit_param = self.pagination_config.get("limit_param", "limit")
                params[offset_param] = offset
                params[limit_param] = limit
            elif self.pagination_type == PaginationType.CURSOR and cursor is not None:
                cursor_param = self.pagination_config.get("cursor_param", "cursor")
                params[cursor_param] = cursor

            # NOTE: For Ticket 1 we intentionally *do not* push time-window
            # filters down as query params; filtering is in-memory only.
            try:
                response_json = self._fetch_page(params=params)
            except ConnectorValidationError:
                raise
            except Exception as exc:
                raise ConnectorValidationError(
                    f"REST API page fetch failed: {exc}"
                ) from exc

            items = self._extract_items(response_json)
            if not items:
                break

            for item in items:
                if isinstance(item, Mapping):
                    yield item
                else:
                    logging.debug(
                        "Skipping non-mapping REST API item of type %s", type(item)
                    )

            page_count += 1

            if self.pagination_type == PaginationType.NONE:
                break

            if self.pagination_type == PaginationType.PAGE:
                page += 1
                if len(items) < per_page:
                    break
            elif self.pagination_type == PaginationType.OFFSET:
                offset += limit
                if len(items) < limit:
                    break
            elif self.pagination_type == PaginationType.CURSOR:
                cursor = self._extract_next_cursor(response_json)
                if not cursor:
                    break

    def _page_iter_for_validation(self) -> Iterable[Mapping[str, Any]]:
        """Single-page iterator used only for connectivity checks."""
        response_json = self._fetch_page(params={})
        return self._extract_items(response_json)

    @retry_builder(tries=5, delay=1, max_delay=30, backoff=2)
    def _fetch_page(self, params: Dict[str, Any]) -> Any:
        """Fetch a single page with retry + exponential backoff."""
        headers = {**self._base_headers, **self._auth_headers}
        url, query_params = self._build_url_with_templates(params)

        if self.method == "GET":
            resp = rl_requests.get(
                url,
                headers=headers,
                params=query_params,
                auth=self._basic_auth,
                timeout=60,
            )
        elif self.method == "POST":
            body = self._static_request_body or {}
            resp = rl_requests.post(
                url,
                headers=headers,
                params=query_params,
                json=body,
                auth=self._basic_auth,
                timeout=60,
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
            raise ConnectorValidationError(
                "REST API response is not valid JSON"
            ) from exc

    def _build_url_with_templates(
        self,
        params: Dict[str, Any],
    ) -> tuple[str, Dict[str, Any]]:
        """Apply URL template variables like {page}, {offset}, {cursor}.

        Any param whose name appears as a `{name}` placeholder in the base URL
        will be substituted into the URL string and removed from the query
        parameter dict before the request is sent.
        """
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

    # --------------------------------------------------------------------- #
    # JSON extraction & mapping
    # --------------------------------------------------------------------- #
    def _extract_items(self, response_json: Any) -> List[Mapping[str, Any]]:
        """Extract the items array from a JSON response using JSONPath."""
        if self.items_path and _jsonpath is not None:
            try:
                matches = _jsonpath(response_json, self.items_path)
            except Exception as exc:
                raise ConnectorValidationError(
                    f"Failed to apply items JSONPath '{self.items_path}': {exc}"
                ) from exc

            if not matches:
                return []

            # Common JSONPath libraries return the array itself as the first element.
            if len(matches) == 1 and isinstance(matches[0], list):
                items = matches[0]
            else:
                items = matches
        else:
            # Fallback heuristics when no items_path is provided.
            if isinstance(response_json, list):
                items = response_json
            elif isinstance(response_json, dict):
                # Prefer "items" / "results" keys if present.
                for key in ("items", "results", "data"):
                    if key in response_json and isinstance(response_json[key], list):
                        items = response_json[key]
                        break
                else:
                    # Last resort: values of dict flattened if they are list-like.
                    items = []
                    for value in response_json.values():
                        if isinstance(value, list):
                            items.extend(value)
            else:
                items = []

        results: List[Mapping[str, Any]] = []
        for it in items:
            if isinstance(it, Mapping):
                results.append(it)
        return results

    def _extract_next_cursor(self, response_json: Any) -> Optional[str]:
        """Extract a cursor for cursor-based pagination."""
        cursor_path = self.pagination_config.get("next_cursor_path")
        if not cursor_path:
            # Simple top-level field name.
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
        value = matches[0]
        return str(value) if value is not None else None

    # --------------------------------------------------------------------- #
    # Item → Document mapping
    # --------------------------------------------------------------------- #
    def _item_to_document(self, item: Mapping[str, Any]) -> Document:
        """Map a single API item to a `Document`."""
        if self.id_field:
            raw_id = self._extract_field(item, self.id_field)
        else:
            raw_id = None

        if raw_id is None:
            # Fall back to hashing full item.
            raw_id = hash128(f"rest_api_item:{repr(item)}")

        doc_id = hash128(f"rest_api:{raw_id}")

        content_parts: List[str] = []
        for field in self.content_fields:
            value = self._extract_field(item, field)
            if value is None:
                continue
            text = self._coerce_to_text(value)
            text = self._strip_html(text)
            if text:
                content_parts.append(text)

        content_text = "\n\n".join(content_parts)
        blob = content_text.encode("utf-8")

        metadata: Dict[str, Any] = {}
        for field in self.metadata_fields:
            value = self._extract_field(item, field)
            if value is None:
                continue
            metadata[field] = self._serialize_metadata_value(value)

        doc_updated_at = self._extract_timestamp(item) or datetime.now(timezone.utc)

        # For semantic identifier, fall back to first content field or doc_id.
        semantic_identifier = str(
            self._extract_field(item, self.content_fields[0])
            if self.content_fields
            else raw_id
        )
        semantic_identifier = semantic_identifier.replace("\n", " ").replace("\r", " ")
        semantic_identifier = semantic_identifier.strip()[:100] or str(doc_id)

        return Document(
            id=doc_id,
            source=DocumentSource.REST_API,
            semantic_identifier=semantic_identifier,
            extension=".txt",
            blob=blob,
            doc_updated_at=doc_updated_at,
            size_bytes=len(blob),
            metadata=metadata or None,
        )

    # --------------------------------------------------------------------- #
    # Helpers
    # --------------------------------------------------------------------- #
    def _extract_field(self, item: Mapping[str, Any], path: str) -> Any:
        """Extract a field from an item using simple dot-notation."""
        if not path:
            return None

        current: Any = item
        for part in path.split("."):
            if not isinstance(current, Mapping):
                return None
            current = current.get(part)
            if current is None:
                return None
        return current

    def _extract_timestamp(self, item: Mapping[str, Any]) -> Optional[datetime]:
        """Extract timestamp from item using `poll_timestamp_field`."""
        if not self.poll_timestamp_field:
            return None

        value = self._extract_field(item, self.poll_timestamp_field)
        if value is None:
            return None

        if isinstance(value, datetime):
            if value.tzinfo is None:
                return value.replace(tzinfo=timezone.utc)
            return value.astimezone(timezone.utc)

        if isinstance(value, (int, float)):
            try:
                return datetime.fromtimestamp(float(value), tz=timezone.utc)
            except Exception:
                return None

        if isinstance(value, str):
            ts_str = value.strip()
            # Accept common ISO-ish formats.
            for fmt in (
                "%Y-%m-%dT%H:%M:%S.%fZ",
                "%Y-%m-%dT%H:%M:%SZ",
                "%Y-%m-%d %H:%M:%S",
                "%Y-%m-%d",
            ):
                try:
                    dt = datetime.strptime(ts_str, fmt)
                    return dt.replace(tzinfo=timezone.utc)
                except Exception:
                    continue
            try:
                dt = datetime.fromisoformat(
                    ts_str.replace("Z", "+00:00").replace(" ", "T")
                )
                if dt.tzinfo is None:
                    dt = dt.replace(tzinfo=timezone.utc)
                else:
                    dt = dt.astimezone(timezone.utc)
                return dt
            except Exception:
                return None

        return None

    @staticmethod
    def _strip_html(text: str) -> str:
        """Remove basic HTML tags from a string."""
        if "<" not in text or ">" not in text:
            return text
        # Lightweight tag stripper; we intentionally avoid bringing in
        # heavier HTML parsers here.
        cleaned = re.sub(r"<[^>]+>", " ", text)
        cleaned = re.sub(r"\s+", " ", cleaned)
        return cleaned.strip()

    @staticmethod
    def _coerce_to_text(value: Any) -> str:
        if value is None:
            return ""
        if isinstance(value, str):
            return value
        if isinstance(value, (int, float, bool)):
            return str(value)
        try:
            import json

            return json.dumps(value, ensure_ascii=False)
        except Exception:
            return str(value)

    @staticmethod
    def _serialize_metadata_value(value: Any) -> Any:
        if isinstance(value, datetime):
            if value.tzinfo is None:
                value = value.replace(tzinfo=timezone.utc)
            return value.isoformat()
        if isinstance(value, (int, float, bool, str)):
            return value
        try:
            import json

            return json.dumps(value, ensure_ascii=False)
        except Exception:
            return str(value)

    @staticmethod
    def _doc_in_time_window(
        doc: Document,
        start: datetime,
        end: datetime,
    ) -> bool:
        if not doc.doc_updated_at:
            return False
        dt = doc.doc_updated_at
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        else:
            dt = dt.astimezone(timezone.utc)
        return start <= dt < end


if __name__ == "__main__":  # pragma: no cover - manual smoke test helper
    logging.basicConfig(level=logging.INFO)
    example_conf = {
        "url": "https://example.com/api/items",
        "method": "GET",
        "items_path": "$.items[*]",
        "id_field": "id",
        "content_fields": ["title", "body"],
        "metadata_fields": ["category"],
        "pagination_type": PaginationType.NONE,
    }
    try:
        validated = RestAPIConnector.validate_config(example_conf, credentials=None)
        connector = RestAPIConnector(
            url=str(validated.url),
            method=validated.normalized_method(),
            headers=validated.headers,
            auth_type=validated.normalized_auth_type(),
            auth_config=validated.auth_config,
            items_path=validated.items_path,
            id_field=validated.id_field,
            content_fields=validated.content_fields,
            metadata_fields=validated.metadata_fields,
            pagination_type=validated.normalized_pagination_type(),
            pagination_config=validated.pagination_config,
            poll_timestamp_field=validated.poll_timestamp_field,
            batch_size=validated.batch_size,
            max_pages=validated.max_pages,
            request_body=validated.request_body,
        )
        for batch in connector.load_from_state():
            print(f"Loaded batch of {len(batch)} documents")
            break
    except Exception as exc:  # pragma: no cover - manual debug helper
        logging.error("Example REST API connector validation failed: %s", exc)

