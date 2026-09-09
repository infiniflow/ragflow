"""Small adapter between the Feishu Wiki connector and RAGFlow sync windows."""

from __future__ import annotations

from datetime import UTC, datetime
from typing import Any

from common.data_source import FeishuWikiConnector


def _build_feishu_wiki_generator(
    conf: dict[str, Any],
    task: dict[str, Any],
    *,
    window_end: datetime | None = None,
):
    """Build a connector and select full or incremental document generation."""
    connector = FeishuWikiConnector.build_connector(conf)
    if task.get("reindex") == "1" or task.get("poll_range_start") is None:
        return connector, connector.load_from_state()

    effective_end = window_end or datetime.now(UTC)
    return connector, connector.poll_source(
        task["poll_range_start"].timestamp(),
        effective_end.timestamp(),
    )
