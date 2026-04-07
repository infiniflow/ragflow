#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

from unittest.mock import MagicMock, patch

import pytest
import requests

from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
)
from common.data_source.rest_api_connector import (
    AuthType,
    PaginationType,
    RestAPIConnector,
    RestAPIConnectorConfig,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

VALID_URL = "https://api.example.com/v1/items"


def _make_connector(**overrides) -> RestAPIConnector:
    """Build a RestAPIConnector with sensible defaults, applying *overrides*."""
    defaults = dict(
        url=VALID_URL,
        content_fields=["title", "body"],
    )
    defaults.update(overrides)
    return RestAPIConnector(**defaults)


def _mock_response(json_data, status_code=200):
    """Return a ``requests.Response``-like mock."""
    resp = MagicMock(spec=requests.Response)
    resp.status_code = status_code
    resp.url = VALID_URL
    resp.json.return_value = json_data

    if status_code >= 400:
        http_error = requests.HTTPError(response=resp)
        resp.raise_for_status.side_effect = http_error
        resp.status_code = status_code
    else:
        resp.raise_for_status.return_value = None

    return resp


# ===================================================================== #
# 1. Config schema validation                                           #
# ===================================================================== #

class TestRestAPIConfig:
    """Test Pydantic RestAPIConnectorConfig schema validation."""

    def test_missing_url_raises_validation_error(self):
        """Missing url should fail Pydantic validation."""
        with pytest.raises(Exception):
            RestAPIConnectorConfig(content_fields=["title"])

    def test_missing_content_fields_detected(self):
        """An empty content_fields list should be caught by ensure_required_fields."""
        cfg = RestAPIConnectorConfig(url=VALID_URL, content_fields=[])
        with pytest.raises(ConnectorValidationError):
            cfg.ensure_required_fields()

    def test_valid_minimal_config(self):
        """Minimal valid config: url + content_fields."""
        cfg = RestAPIConnectorConfig(url=VALID_URL, content_fields=["title"])
        assert str(cfg.url).startswith("https://api.example.com")
        assert cfg.content_fields == ["title"]

    def test_auth_type_defaults_to_none(self):
        """auth_type should default to 'none'."""
        cfg = RestAPIConnectorConfig(url=VALID_URL, content_fields=["t"])
        assert cfg.auth_type == AuthType.NONE

    def test_pagination_type_defaults_to_none(self):
        """pagination_type should default to 'none'."""
        cfg = RestAPIConnectorConfig(url=VALID_URL, content_fields=["t"])
        assert cfg.pagination_type == PaginationType.NONE

    def test_string_to_dict_coercion_for_headers(self):
        """A key=value string should be coerced to a dict."""
        cfg = RestAPIConnectorConfig(
            url=VALID_URL, content_fields=["t"], headers="X-Custom=hello"
        )
        assert cfg.headers == {"X-Custom": "hello"}

    def test_string_to_list_coercion_for_content_fields(self):
        """A comma-separated string should be coerced to a list."""
        cfg = RestAPIConnectorConfig(url=VALID_URL, content_fields="title,content")
        assert cfg.content_fields == ["title", "content"]


# ===================================================================== #
# 2. SSRF URL validation                                                #
# ===================================================================== #

class TestSSRFValidation:
    """Test that unsafe URLs are blocked before any HTTP request is made."""

    def test_localhost_blocked(self):
        """localhost should be rejected."""
        with pytest.raises(ConnectorValidationError, match="localhost"):
            _make_connector(url="http://localhost/api")

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo")
    def test_loopback_ip_blocked(self, mock_dns):
        """127.0.0.1 should be rejected."""
        mock_dns.return_value = [(2, 1, 6, "", ("127.0.0.1", 0))]
        with pytest.raises(ConnectorValidationError, match="disallowed"):
            _make_connector(url="http://127.0.0.1/api")

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo")
    def test_cloud_metadata_ip_blocked(self, mock_dns):
        """169.254.169.254 (cloud metadata endpoint) should be rejected."""
        mock_dns.return_value = [(2, 1, 6, "", ("169.254.169.254", 0))]
        with pytest.raises(ConnectorValidationError, match="disallowed"):
            _make_connector(url="http://169.254.169.254/latest/meta-data/")

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo")
    def test_private_ip_192_blocked(self, mock_dns):
        """192.168.x.x should be rejected."""
        mock_dns.return_value = [(2, 1, 6, "", ("192.168.1.1", 0))]
        with pytest.raises(ConnectorValidationError, match="disallowed"):
            _make_connector(url="http://192.168.1.1/api")

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo")
    def test_private_ip_10_blocked(self, mock_dns):
        """10.x.x.x should be rejected."""
        mock_dns.return_value = [(2, 1, 6, "", ("10.0.0.1", 0))]
        with pytest.raises(ConnectorValidationError, match="disallowed"):
            _make_connector(url="http://10.0.0.1/api")

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo")
    def test_public_url_passes(self, mock_dns):
        """A public IP should pass validation."""
        mock_dns.return_value = [(2, 1, 6, "", ("93.184.216.34", 0))]
        c = _make_connector(url="https://example.com/api")
        assert c.url.startswith("https://")

    def test_ftp_scheme_blocked(self):
        """ftp:// should be rejected."""
        with pytest.raises(ConnectorValidationError, match="scheme"):
            _make_connector(url="ftp://example.com/file")

    def test_file_scheme_blocked(self):
        """file:// should be rejected."""
        with pytest.raises(ConnectorValidationError, match="scheme"):
            _make_connector(url="file:///etc/passwd")


# ===================================================================== #
# 3. Authentication setup                                               #
# ===================================================================== #

class TestAuthSetup:
    """Test _build_auth produces the correct headers / auth objects."""

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo",
           return_value=[(2, 1, 6, "", ("93.184.216.34", 0))])
    def test_auth_none(self, _dns):
        """auth_type=none should produce no auth headers."""
        c = _make_connector(auth_type=AuthType.NONE)
        c.load_credentials({})
        assert c._auth_headers == {}
        assert c._basic_auth is None

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo",
           return_value=[(2, 1, 6, "", ("93.184.216.34", 0))])
    def test_api_key_header(self, _dns):
        """api_key_header should set the specified header."""
        c = _make_connector(
            auth_type=AuthType.API_KEY_HEADER,
            auth_config={"header_name": "X-API-Key"},
        )
        c.load_credentials({"api_key": "secret123"})
        assert c._auth_headers == {"X-API-Key": "secret123"}

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo",
           return_value=[(2, 1, 6, "", ("93.184.216.34", 0))])
    def test_bearer_token(self, _dns):
        """bearer should set Authorization: Bearer <token>."""
        c = _make_connector(auth_type=AuthType.BEARER)
        c.load_credentials({"token": "tok_abc"})
        assert c._auth_headers == {"Authorization": "Bearer tok_abc"}

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo",
           return_value=[(2, 1, 6, "", ("93.184.216.34", 0))])
    def test_basic_auth(self, _dns):
        """basic should produce an HTTPBasicAuth object."""
        c = _make_connector(auth_type=AuthType.BASIC)
        c.load_credentials({"username": "user", "password": "pass"})
        assert c._basic_auth is not None
        assert c._basic_auth.username == "user"
        assert c._basic_auth.password == "pass"


# ===================================================================== #
# 4. Field extraction                                                   #
# ===================================================================== #

class TestFieldExtraction:
    """Test _extract_field / _extract_field_values dot-notation paths."""

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo",
           return_value=[(2, 1, 6, "", ("93.184.216.34", 0))])
    def setup_method(self, method, _dns=None):
        with patch("common.data_source.rest_api_connector.socket.getaddrinfo",
                    return_value=[(2, 1, 6, "", ("93.184.216.34", 0))]):
            self.connector = _make_connector()

    def test_simple_field(self):
        """Top-level field extraction."""
        assert self.connector._extract_field({"title": "Hello"}, "title") == "Hello"

    def test_dot_notation_nested(self):
        """Dot-notation nested field."""
        item = {"country": {"name": "Kuwait"}}
        assert self.connector._extract_field(item, "country.name") == "Kuwait"

    def test_array_wildcard(self):
        """Wildcard [*] returns all array elements."""
        item = {"tags": [{"name": "A"}, {"name": "B"}]}
        result = self.connector._extract_field(item, "tags[*].name")
        assert result == ["A", "B"]

    def test_missing_field_returns_none(self):
        """Missing field returns None."""
        assert self.connector._extract_field({"a": 1}, "nonexistent") is None

    def test_missing_field_with_default(self):
        """Missing field returns configured default value."""
        with patch("common.data_source.rest_api_connector.socket.getaddrinfo",
                    return_value=[(2, 1, 6, "", ("93.184.216.34", 0))]):
            c = _make_connector(field_default_values={"missing": "fallback"})
        result = c._get_typed_field_value("missing", {"other": 1})
        assert result == "fallback"

    def test_deeply_nested_path(self):
        """Multi-level dot-notation path."""
        item = {"a": {"b": {"c": {"d": 42}}}}
        assert self.connector._extract_field(item, "a.b.c.d") == 42


# ===================================================================== #
# 5. Items array detection                                              #
# ===================================================================== #

class TestItemsArrayDetection:
    """Test _extract_items auto-detection of the items array."""

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo",
           return_value=[(2, 1, 6, "", ("93.184.216.34", 0))])
    def setup_method(self, method, _dns=None):
        with patch("common.data_source.rest_api_connector.socket.getaddrinfo",
                    return_value=[(2, 1, 6, "", ("93.184.216.34", 0))]):
            self.connector = _make_connector()

    def test_items_key(self):
        """Detect 'items' key."""
        resp = {"items": [{"id": 1}]}
        assert self.connector._extract_items(resp) == [{"id": 1}]

    def test_results_key(self):
        """Detect 'results' key."""
        resp = {"results": [{"id": 2}]}
        assert self.connector._extract_items(resp) == [{"id": 2}]

    def test_data_key(self):
        """Detect 'data' key."""
        resp = {"data": [{"id": 3}]}
        assert self.connector._extract_items(resp) == [{"id": 3}]

    def test_records_key(self):
        """Detect 'records' key."""
        resp = {"records": [{"id": 4}]}
        assert self.connector._extract_items(resp) == [{"id": 4}]

    def test_custom_key_fallback(self):
        """Fall back to the first list value in the dict."""
        resp = {"totalCount": 5, "stories": [{"id": 5}]}
        assert self.connector._extract_items(resp) == [{"id": 5}]

    def test_response_is_list(self):
        """Response that is directly a list."""
        resp = [{"id": 6}, {"id": 7}]
        assert self.connector._extract_items(resp) == [{"id": 6}, {"id": 7}]

    def test_empty_response(self):
        """Empty dict returns empty list."""
        assert self.connector._extract_items({}) == []

    def test_no_list_in_response(self):
        """Dict with no list values returns empty list."""
        assert self.connector._extract_items({"count": 0}) == []


# ===================================================================== #
# 6. HTML stripping                                                     #
# ===================================================================== #

class TestHTMLStripping:
    """Test the _strip_html static method."""

    def test_basic_tag_removal(self):
        """Remove simple HTML tags."""
        assert RestAPIConnector._strip_html("<p>Hello</p>") == "Hello"

    def test_whitespace_collapsing(self):
        """Multiple whitespace chars collapse to single space."""
        assert RestAPIConnector._strip_html("<p>Hello</p>  <p>World</p>") == "Hello World"

    def test_empty_string(self):
        """Empty input returns empty output."""
        assert RestAPIConnector._strip_html("") == ""

    def test_plain_text_passthrough(self):
        """Text without HTML passes through unchanged."""
        assert RestAPIConnector._strip_html("Hello World") == "Hello World"

    def test_nested_tags(self):
        """Nested HTML tags are all stripped."""
        result = RestAPIConnector._strip_html("<div><p><b>Bold</b> text</p></div>")
        assert result == "Bold text"

    def test_html_with_attributes(self):
        """Tags with attributes are stripped."""
        result = RestAPIConnector._strip_html('<a href="http://x.com">Link</a>')
        assert result == "Link"


# ===================================================================== #
# 7. Document creation                                                  #
# ===================================================================== #

class TestDocumentCreation:
    """Test _item_to_document mapping."""

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo",
           return_value=[(2, 1, 6, "", ("93.184.216.34", 0))])
    def setup_method(self, method, _dns=None):
        with patch("common.data_source.rest_api_connector.socket.getaddrinfo",
                    return_value=[(2, 1, 6, "", ("93.184.216.34", 0))]):
            self.connector = _make_connector(
                id_field="id",
                content_fields=["title", "body"],
                metadata_fields=["author"],
            )

    def test_document_id_from_configured_field(self):
        """Document ID uses the configured id_field."""
        item = {"id": "abc", "title": "T", "body": "B", "author": "A"}
        doc = self.connector._item_to_document(item)
        assert doc.id is not None and len(doc.id) > 0

    def test_semantic_identifier_from_first_content_field(self):
        """semantic_identifier comes from the first content field."""
        item = {"id": "1", "title": "My Title", "body": "Body", "author": "A"}
        doc = self.connector._item_to_document(item)
        assert "My Title" in doc.semantic_identifier

    def test_content_blob_contains_all_fields(self):
        """Blob should contain both content fields."""
        item = {"id": "1", "title": "Title", "body": "Body text", "author": "A"}
        doc = self.connector._item_to_document(item)
        content = doc.blob.decode("utf-8")
        assert "Title" in content
        assert "Body text" in content

    def test_metadata_populated(self):
        """Metadata dict is populated from configured metadata_fields."""
        item = {"id": "1", "title": "T", "body": "B", "author": "Jane"}
        doc = self.connector._item_to_document(item)
        assert doc.metadata is not None
        assert doc.metadata["author"] == "Jane"

    def test_html_stripped_from_content(self):
        """HTML tags are removed from content fields."""
        item = {"id": "1", "title": "T", "body": "<p>Clean</p>", "author": "A"}
        doc = self.connector._item_to_document(item)
        content = doc.blob.decode("utf-8")
        assert "<p>" not in content
        assert "Clean" in content

    def test_extension_is_txt(self):
        """Document extension should be .txt."""
        item = {"id": "1", "title": "T", "body": "B", "author": "A"}
        doc = self.connector._item_to_document(item)
        assert doc.extension == ".txt"

    def test_missing_content_fields_graceful(self):
        """Missing content fields produce an empty blob gracefully."""
        item = {"id": "1", "author": "A"}
        doc = self.connector._item_to_document(item)
        assert doc.blob == b""


# ===================================================================== #
# 8. Pagination behaviour                                               #
# ===================================================================== #

class TestPaginationBehavior:
    """Test pagination iteration with mocked HTTP responses."""

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo",
           return_value=[(2, 1, 6, "", ("93.184.216.34", 0))])
    def _make_paged_connector(self, _dns, **overrides):
        defaults = dict(
            url=VALID_URL,
            content_fields=["title"],
            pagination_type=PaginationType.PAGE,
            pagination_config={"page_param": "page"},
            max_pages=100,
            request_delay=0,
        )
        defaults.update(overrides)
        return RestAPIConnector(**defaults)

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_page_pagination_increments(self, mock_rl):
        """Page-based pagination should increment the page param."""
        page1 = _mock_response({"items": [{"title": "A"}, {"title": "B"}]})
        page2 = _mock_response({"items": []})
        mock_rl.get.side_effect = [page1, page2]

        c = self._make_paged_connector()
        items = list(c._iter_items())
        assert len(items) == 2
        assert mock_rl.get.call_count == 2

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_offset_pagination_increments(self, mock_rl):
        """Offset-based pagination should increment offset by limit."""
        page1 = _mock_response({"items": [{"title": "A"}]})
        page2 = _mock_response({"items": []})
        mock_rl.get.side_effect = [page1, page2]

        with patch("common.data_source.rest_api_connector.socket.getaddrinfo",
                    return_value=[(2, 1, 6, "", ("93.184.216.34", 0))]):
            c = _make_connector(
                pagination_type=PaginationType.OFFSET,
                pagination_config={"offset_param": "offset", "limit_param": "limit", "limit": 10},
                request_delay=0,
            )
        items = list(c._iter_items())
        assert len(items) == 1

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_stops_on_empty_results(self, mock_rl):
        """Pagination stops when empty items are returned."""
        mock_rl.get.return_value = _mock_response({"items": []})

        c = self._make_paged_connector()
        items = list(c._iter_items())
        assert items == []
        assert mock_rl.get.call_count == 1

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_stops_when_fewer_items_than_page_size(self, mock_rl):
        """Pagination stops when fewer items than page_size are returned."""
        page1 = _mock_response({"items": [{"title": "A"}]})
        mock_rl.get.return_value = page1

        c = self._make_paged_connector(
            pagination_config={"page_param": "page", "page_size": 10},
        )
        items = list(c._iter_items())
        assert len(items) == 1
        assert mock_rl.get.call_count == 1

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_max_pages_cap(self, mock_rl):
        """Pagination respects the max_pages safety cap."""
        mock_rl.get.return_value = _mock_response(
            {"items": [{"title": "A"}, {"title": "B"}]}
        )

        c = self._make_paged_connector(
            max_pages=3,
            pagination_config={"page_param": "page", "page_size": 2},
        )
        list(c._iter_items())
        assert mock_rl.get.call_count == 3

    @patch("common.data_source.rest_api_connector.time.sleep")
    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_request_delay_applied(self, mock_rl, mock_sleep):
        """request_delay should cause a sleep between pages."""
        page1 = _mock_response({"items": [{"title": "A"}, {"title": "B"}]})
        page2 = _mock_response({"items": []})
        mock_rl.get.side_effect = [page1, page2]

        c = self._make_paged_connector(
            pagination_config={"page_param": "page", "page_size": 2},
        )
        c.request_delay = 1.5
        list(c._iter_items())
        mock_sleep.assert_called_once_with(1.5)


# ===================================================================== #
# 9. Non-retriable HTTP errors                                          #
# ===================================================================== #

class TestNonRetriableErrors:
    """Test that HTTP errors are classified correctly in _fetch_page."""

    @patch("common.data_source.rest_api_connector.socket.getaddrinfo",
           return_value=[(2, 1, 6, "", ("93.184.216.34", 0))])
    def _make_test_connector(self, _dns):
        c = _make_connector(request_delay=0)
        c.load_credentials({})
        return c

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_401_raises_credential_error(self, mock_rl):
        """401 should raise ConnectorMissingCredentialError immediately."""
        mock_rl.get.return_value = _mock_response({}, status_code=401)
        c = self._make_test_connector()
        with pytest.raises(ConnectorMissingCredentialError):
            c._fetch_page({})

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_403_raises_credential_error(self, mock_rl):
        """403 should raise ConnectorMissingCredentialError immediately."""
        mock_rl.get.return_value = _mock_response({}, status_code=403)
        c = self._make_test_connector()
        with pytest.raises(ConnectorMissingCredentialError):
            c._fetch_page({})

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_404_raises_validation_error(self, mock_rl):
        """404 should raise ConnectorValidationError (no retry)."""
        mock_rl.get.return_value = _mock_response({}, status_code=404)
        c = self._make_test_connector()
        with pytest.raises(ConnectorValidationError, match="non-retriable"):
            c._fetch_page({})

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_400_raises_validation_error(self, mock_rl):
        """400 should raise ConnectorValidationError (no retry)."""
        mock_rl.get.return_value = _mock_response({}, status_code=400)
        c = self._make_test_connector()
        with pytest.raises(ConnectorValidationError, match="non-retriable"):
            c._fetch_page({})

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_500_triggers_retry(self, mock_rl):
        """500 should raise HTTPError (which the retry decorator catches)."""
        mock_rl.get.return_value = _mock_response({}, status_code=500)
        c = self._make_test_connector()
        with pytest.raises(requests.HTTPError):
            c._fetch_page({})

    @patch("common.data_source.rest_api_connector.rl_requests")
    def test_429_triggers_retry(self, mock_rl):
        """429 should raise HTTPError (retriable, not ConnectorValidationError)."""
        mock_rl.get.return_value = _mock_response({}, status_code=429)
        c = self._make_test_connector()
        with pytest.raises(requests.HTTPError):
            c._fetch_page({})
