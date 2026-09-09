from datetime import datetime, timezone
import importlib
import sys
from pathlib import Path
from types import SimpleNamespace
from types import ModuleType

import pytest

import common

repo_root = Path(__file__).resolve().parents[3]
data_source_pkg = ModuleType("common.data_source")
data_source_pkg.__path__ = [str(repo_root / "common" / "data_source")]
sys.modules["common.data_source"] = data_source_pkg
setattr(common, "data_source", data_source_pkg)

DocumentSource = importlib.import_module("common.data_source.config").DocumentSource
RSSConnector = importlib.import_module("common.data_source.rss_connector").RSSConnector


class _FakeResponse:
    def __init__(self, content: bytes = b"feed") -> None:
        self.content = content

    def raise_for_status(self) -> None:
        return None


def _mock_feed(*entries, bozo=False, bozo_exception=None):
    return SimpleNamespace(
        entries=list(entries),
        bozo=bozo,
        bozo_exception=bozo_exception,
    )


def test_validate_connector_settings_rejects_invalid_feed_url():
    connector = RSSConnector(feed_url="ftp://example.com/feed.xml")

    with pytest.raises(ValueError, match="valid http or https URL"):
        connector.validate_connector_settings()


def test_validate_connector_settings_rejects_empty_feed(monkeypatch):
    monkeypatch.setattr("common.data_source.rss_connector.requests.get", lambda *_args, **_kwargs: _FakeResponse())
    monkeypatch.setattr(
        "common.data_source.rss_connector.feedparser.parse",
        lambda _content: _mock_feed(),
    )

    connector = RSSConnector(feed_url="https://example.com/feed.xml")

    with pytest.raises(ValueError, match="contains no entries"):
        connector.validate_connector_settings()


def test_load_from_state_builds_documents(monkeypatch):
    monkeypatch.setattr("common.data_source.rss_connector.requests.get", lambda *_args, **_kwargs: _FakeResponse())
    monkeypatch.setattr(
        "common.data_source.rss_connector.feedparser.parse",
        lambda _content: _mock_feed(
            {
                "id": "entry-1",
                "link": "https://example.com/posts/1",
                "title": "Post One",
                "content": [{"value": "<p>Hello <b>world</b></p>"}],
                "author": "Alice",
                "tags": [{"term": "news"}, {"term": "product"}],
                "updated": "Tue, 02 Jan 2024 15:04:05 GMT",
            }
        ),
    )

    connector = RSSConnector(feed_url="https://example.com/feed.xml")
    batch = next(connector.load_from_state())

    assert len(batch) == 1
    doc = batch[0]
    assert doc.source == DocumentSource.RSS
    assert doc.semantic_identifier == "Post One"
    assert doc.extension == ".txt"
    assert doc.metadata == {
        "feed_url": "https://example.com/feed.xml",
        "link": "https://example.com/posts/1",
        "author": "Alice",
        "categories": ["news", "product"],
    }
    assert "Hello" in doc.blob.decode("utf-8")
    assert "world" in doc.blob.decode("utf-8")


def test_poll_source_filters_entries_by_timestamp(monkeypatch):
    monkeypatch.setattr("common.data_source.rss_connector.requests.get", lambda *_args, **_kwargs: _FakeResponse())
    monkeypatch.setattr(
        "common.data_source.rss_connector.feedparser.parse",
        lambda _content: _mock_feed(
            {
                "id": "entry-1",
                "title": "Older",
                "summary": "older summary",
                "updated": "Mon, 01 Jan 2024 00:00:00 GMT",
            },
            {
                "id": "entry-2",
                "title": "Newer",
                "summary": "new summary",
                "updated": "Tue, 02 Jan 2024 00:00:00 GMT",
            },
        ),
    )

    connector = RSSConnector(feed_url="https://example.com/feed.xml")
    start = datetime(2024, 1, 1, tzinfo=timezone.utc).timestamp()
    end = datetime(2024, 1, 2, tzinfo=timezone.utc).timestamp()

    batches = list(connector.poll_source(start, end))

    assert len(batches) == 1
    assert [doc.semantic_identifier for doc in batches[0]] == ["Newer"]
