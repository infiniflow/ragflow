#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import logging
from datetime import UTC, datetime
from unittest.mock import patch

import pytest

from common.data_source.config import DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError
from common.data_source.rest_api_connector import PaginationType, RestAPIConnector
from common.data_source.xquik_connector import XquikConnector

_MOCK_DNS = [(2, 1, 6, "", ("93.184.216.34", 0))]


def _config(**overrides):
    config = {
        "query": "ragflow lang:en",
        "query_type": "Top",
        "page_size": 50,
        "max_pages": 4,
        "batch_size": 8,
        "request_delay": 0,
        "credentials": {"xquik_api_key": "xq_test_key"},
    }
    config.update(overrides)
    return config


@patch("common.data_source.rest_api_connector.socket.getaddrinfo", return_value=_MOCK_DNS)
def test_xquik_configures_bounded_search_and_time_window(_dns):
    start = datetime(2026, 8, 24, 10, 0, tzinfo=UTC)
    end = datetime(2026, 8, 25, 0, 0, tzinfo=UTC)

    connector = XquikConnector.from_config(_config(), since_time=start, until_time=end)

    assert connector.pagination_type == PaginationType.CURSOR
    assert connector.max_pages == 4
    assert connector.batch_size == 8
    assert connector._explicit_query_params == {
        "q": "ragflow lang:en",
        "queryType": "Top",
        "limit": "50",
        "sinceTime": "2026-08-24T10:00:00+00:00",
        "untilTime": "2026-08-25T00:00:00+00:00",
    }
    assert connector._auth_headers == {"x-api-key": "xq_test_key"}


@patch("common.data_source.rest_api_connector.socket.getaddrinfo", return_value=_MOCK_DNS)
def test_xquik_validation_fetches_at_most_one_post(_dns):
    connector = XquikConnector.build_connector(_config(page_size=500, max_pages=50))

    assert connector.pagination_type == PaginationType.NONE
    assert connector.max_pages == 1
    assert connector._explicit_query_params["limit"] == "1"


@patch("common.data_source.rest_api_connector.socket.getaddrinfo", return_value=_MOCK_DNS)
def test_xquik_logs_safe_resolved_config(_dns, caplog):
    caplog.set_level(logging.INFO, logger="common.data_source.xquik_connector")

    XquikConnector.from_config(_config())

    assert "query_type=Top page_size=50 max_pages=4 batch_size=8 request_delay=0.0 validation=False" in caplog.text
    assert "ragflow lang:en" not in caplog.text
    assert "xq_test_key" not in caplog.text


@patch("common.data_source.rest_api_connector.socket.getaddrinfo", return_value=_MOCK_DNS)
def test_xquik_maps_post_content_metadata_and_source(_dns):
    connector = XquikConnector.from_config(_config())

    document = connector._item_to_document(
        {
            "id": "101",
            "text": "First post",
            "createdAt": "2026-08-24T10:15:00Z",
            "url": "https://x.com/alice/status/101",
            "lang": "en",
            "author": {"id": "1", "username": "alice", "name": "Alice", "verified": True},
            "likeCount": 4,
            "media": [{"mediaUrl": "https://pbs.twimg.com/media/example.jpg"}],
        }
    )

    assert document.source == DocumentSource.XQUIK
    assert document.blob.decode() == ("Author: @alice\nPublished: 2026-08-24T10:15:00Z\nURL: https://x.com/alice/status/101\n\nFirst post")
    assert document.doc_updated_at == datetime(2026, 8, 24, 10, 15, tzinfo=UTC)
    assert document.metadata["author.username"] == "alice"
    assert document.metadata["media[*].mediaUrl"] == "https://pbs.twimg.com/media/example.jpg"


@patch("common.data_source.rest_api_connector.socket.getaddrinfo", return_value=_MOCK_DNS)
def test_xquik_continues_after_empty_page_when_response_has_more(_dns):
    connector = XquikConnector.from_config(_config())
    responses = iter(
        [
            {"tweets": [], "has_next_page": True, "next_cursor": "cursor-2"},
            {
                "tweets": [{"id": "101", "text": "Older post"}],
                "has_next_page": False,
                "next_cursor": "ignored-cursor",
            },
        ]
    )
    requests = []

    def fetch_page(params):
        requests.append(dict(params))
        return next(responses)

    connector._fetch_page = fetch_page

    items = list(connector._iter_items())

    assert items == [{"id": "101", "text": "Older post"}]
    assert requests == [{}, {"cursor": "cursor-2"}]


@pytest.mark.parametrize(
    ("override", "error_type", "message"),
    [
        ({"query": ""}, ConnectorValidationError, "query is required"),
        ({"query_type": "Popular"}, ConnectorValidationError, "Latest or Top"),
        ({"page_size": 1.5}, ConnectorValidationError, "positive integer"),
        ({"page_size": 10_001}, ConnectorValidationError, "from 1 to 10000"),
        ({"max_pages": 1_001}, ConnectorValidationError, "from 1 to 1000"),
        ({"request_delay": -1}, ConnectorValidationError, "non-negative"),
        ({"request_delay": float("inf")}, ConnectorValidationError, "non-negative"),
        ({"credentials": {}}, ConnectorMissingCredentialError, "xquik_api_key"),
    ],
)
def test_xquik_rejects_invalid_config(override, error_type, message):
    with pytest.raises(error_type, match=message):
        XquikConnector.from_config(_config(**override))


@patch("common.data_source.rest_api_connector.socket.getaddrinfo", return_value=_MOCK_DNS)
def test_rest_api_cursor_stops_when_provider_repeats_cursor(_dns):
    connector = RestAPIConnector(
        url="https://api.example.com/items",
        content_fields=["title"],
        pagination_type=PaginationType.CURSOR,
        pagination_config={"next_cursor_field": "next_cursor"},
        max_pages=20,
        request_delay=0,
    )
    requests = []

    def fetch_page(params):
        requests.append(dict(params))
        return {"items": [{"title": "Page"}], "next_cursor": "same-cursor"}

    connector._fetch_page = fetch_page

    items = list(connector._iter_items())

    assert len(items) == 2
    assert requests == [{}, {"cursor": "same-cursor"}]


@patch("common.data_source.rest_api_connector.socket.getaddrinfo", return_value=_MOCK_DNS)
def test_rest_api_normalizes_numeric_initial_cursor_before_cycle_check(_dns):
    connector = RestAPIConnector(
        url="https://api.example.com/items",
        content_fields=["title"],
        pagination_type=PaginationType.CURSOR,
        pagination_config={"initial_cursor": 123, "next_cursor_field": "next_cursor"},
        max_pages=20,
        request_delay=0,
    )
    requests = []

    def fetch_page(params):
        requests.append(dict(params))
        return {"items": [{"title": "Page"}], "next_cursor": 123}

    connector._fetch_page = fetch_page

    assert len(list(connector._iter_items())) == 1
    assert requests == [{"cursor": "123"}]
