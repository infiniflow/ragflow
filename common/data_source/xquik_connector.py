"""Xquik search connector built on RAGFlow's REST API connector."""

from __future__ import annotations

import logging
import math
from collections.abc import Mapping
from datetime import datetime
from typing import Any

from common.data_source.config import DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError
from common.data_source.models import Document
from common.data_source.rest_api_connector import AuthType, PaginationType, RestAPIConnector

XQUIK_SEARCH_URL = "https://xquik.com/api/v1/x/tweets/search"
XQUIK_DEFAULT_PAGE_SIZE = 100
XQUIK_DEFAULT_MAX_PAGES = 10
XQUIK_DEFAULT_BATCH_SIZE = 32

logger = logging.getLogger(__name__)


def _positive_int(value: Any, default: int, field: str, maximum: int | None = None) -> int:
    if value in (None, ""):
        return default
    try:
        parsed = int(value)
    except (OverflowError, TypeError, ValueError) as exc:
        raise ConnectorValidationError(f"Xquik connector {field} must be a positive integer") from exc
    if isinstance(value, bool) or (isinstance(value, float) and not value.is_integer()):
        raise ConnectorValidationError(f"Xquik connector {field} must be a positive integer")
    if parsed <= 0 or (maximum is not None and parsed > maximum):
        suffix = f" from 1 to {maximum}" if maximum is not None else " a positive integer"
        raise ConnectorValidationError(f"Xquik connector {field} must be{suffix}")
    return parsed


def _non_negative_float(value: Any, default: float, field: str) -> float:
    if value in (None, ""):
        return default
    try:
        parsed = float(value)
    except (TypeError, ValueError) as exc:
        raise ConnectorValidationError(f"Xquik connector {field} must be a non-negative number") from exc
    if not math.isfinite(parsed) or parsed < 0:
        raise ConnectorValidationError(f"Xquik connector {field} must be a non-negative number")
    return parsed


class XquikConnector(RestAPIConnector):
    """Search X posts through Xquik and emit one RAGFlow document per post."""

    search_url = XQUIK_SEARCH_URL

    @classmethod
    def from_config(
        cls,
        config: Mapping[str, Any],
        *,
        since_time: datetime | None = None,
        until_time: datetime | None = None,
        validation: bool = False,
    ) -> XquikConnector:
        query = str(config.get("query") or "").strip()
        if not query:
            raise ConnectorValidationError("Xquik connector query is required")

        query_type = str(config.get("query_type") or "Latest").strip().title()
        if query_type not in {"Latest", "Top"}:
            raise ConnectorValidationError("Xquik connector query_type must be Latest or Top")

        page_size = _positive_int(config.get("page_size"), XQUIK_DEFAULT_PAGE_SIZE, "page_size", 10_000)
        max_pages = _positive_int(config.get("max_pages"), XQUIK_DEFAULT_MAX_PAGES, "max_pages", 1_000)
        batch_size = _positive_int(config.get("batch_size"), XQUIK_DEFAULT_BATCH_SIZE, "batch_size")
        request_delay = _non_negative_float(config.get("request_delay"), 0.5, "request_delay")

        credentials = config.get("credentials") or {}
        if not isinstance(credentials, Mapping):
            credentials = {}
        api_key = str(credentials.get("xquik_api_key") or "").strip()
        if not api_key:
            raise ConnectorMissingCredentialError("Xquik connector requires 'xquik_api_key' in credentials")

        logger.info(
            "Building Xquik connector: query_type=%s page_size=%d max_pages=%d batch_size=%d request_delay=%s validation=%s",
            query_type,
            page_size,
            max_pages,
            batch_size,
            request_delay,
            validation,
        )

        query_params = {
            "q": query,
            "queryType": query_type,
            "limit": str(1 if validation else page_size),
        }
        if since_time is not None:
            query_params["sinceTime"] = since_time.isoformat()
        if until_time is not None:
            query_params["untilTime"] = until_time.isoformat()

        connector = cls(
            url=cls.search_url,
            method="GET",
            query_params=query_params,
            auth_type=AuthType.API_KEY_HEADER,
            auth_config={"header_name": "x-api-key"},
            items_path="$.tweets",
            id_field="id",
            content_fields=["text", "author.username", "createdAt", "url"],
            metadata_fields=[
                "id",
                "createdAt",
                "url",
                "lang",
                "author.id",
                "author.username",
                "author.name",
                "author.verified",
                "likeCount",
                "replyCount",
                "retweetCount",
                "quoteCount",
                "viewCount",
                "bookmarkCount",
                "media[*].mediaUrl",
            ],
            pagination_type=PaginationType.NONE if validation else PaginationType.CURSOR,
            pagination_config={
                "cursor_param": "cursor",
                "next_cursor_field": "next_cursor",
                "has_next_page_field": "has_next_page",
                "page_size": page_size,
            },
            poll_timestamp_field="createdAt",
            batch_size=batch_size,
            max_pages=1 if validation else max_pages,
            request_delay=request_delay,
            content_template="Author: @{author_username}\nPublished: {createdAt}\nURL: {url}\n\n{text}",
        )
        connector.load_credentials({"api_key": api_key})
        return connector

    @classmethod
    def build_connector(cls, config: dict[str, Any]) -> XquikConnector:
        return cls.from_config(config, validation=True)

    def _item_to_document(self, item: Mapping[str, Any]) -> Document:
        document = super()._item_to_document(item)
        document.source = DocumentSource.XQUIK
        return document
