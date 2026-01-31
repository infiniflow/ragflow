"""Unit tests for Paperless-ngx connector"""
import pytest
from unittest.mock import Mock, patch, MagicMock
from datetime import datetime, timezone
import requests

from common.data_source.paperless_ngx_connector import PaperlessNgxConnector
from common.data_source.config import DocumentSource
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
)


@pytest.fixture
def connector():
    """Create a PaperlessNgxConnector instance for testing"""
    return PaperlessNgxConnector(
        base_url="https://paperless.example.com",
        batch_size=10,
        verify_ssl=True,
    )


@pytest.fixture
def credentials():
    """Sample credentials"""
    return {"api_token": "test-token-123"}


class TestPaperlessNgxConnector:
    """Test suite for PaperlessNgxConnector"""

    def test_init(self):
        """Test connector initialization"""
        connector = PaperlessNgxConnector(
            base_url="https://paperless.example.com/",
            batch_size=5,
            verify_ssl=False,
        )
        assert connector.base_url == "https://paperless.example.com"
        assert connector.batch_size == 5
        assert connector.verify_ssl is False
        assert connector.session is None
        assert connector.api_token is None

    def test_url_normalization_missing_double_slash(self):
        """Test URL normalization fixes missing // after scheme"""
        # http:hostname should become http://hostname
        connector = PaperlessNgxConnector(base_url="http:192.168.1.6:8000")
        assert connector.base_url == "http://192.168.1.6:8000"
        
        # https:hostname should become https://hostname
        connector = PaperlessNgxConnector(base_url="https:paperless.example.com")
        assert connector.base_url == "https://paperless.example.com"

    def test_url_normalization_no_scheme(self):
        """Test URL normalization adds https:// if no scheme provided"""
        connector = PaperlessNgxConnector(base_url="paperless.example.com")
        assert connector.base_url == "https://paperless.example.com"
        
        connector = PaperlessNgxConnector(base_url="localhost:8000")
        assert connector.base_url == "https://localhost:8000"

    def test_url_normalization_trailing_slash(self):
        """Test URL normalization removes trailing slashes"""
        connector = PaperlessNgxConnector(base_url="https://paperless.example.com///")
        assert connector.base_url == "https://paperless.example.com"

    def test_url_normalization_already_valid(self):
        """Test URL normalization preserves valid URLs"""
        # HTTP
        connector = PaperlessNgxConnector(base_url="http://localhost:8000")
        assert connector.base_url == "http://localhost:8000"
        
        # HTTPS
        connector = PaperlessNgxConnector(base_url="https://paperless.example.com")
        assert connector.base_url == "https://paperless.example.com"

    def test_url_validation_empty_url(self):
        """Test that empty URLs raise validation error"""
        with pytest.raises(ConnectorValidationError, match="URL cannot be empty"):
            PaperlessNgxConnector(base_url="")
        
        with pytest.raises(ConnectorValidationError, match="URL cannot be empty"):
            PaperlessNgxConnector(base_url="   ")

    def test_url_validation_invalid_url(self):
        """Test that invalid URLs raise validation error"""
        with pytest.raises(ConnectorValidationError, match="Invalid Paperless-ngx URL"):
            PaperlessNgxConnector(base_url="ht!tp://invalid")


    def test_load_credentials_success(self, connector, credentials):
        """Test loading credentials successfully"""
        result = connector.load_credentials(credentials)
        assert result is None
        assert connector.api_token == "test-token-123"
        assert connector.session is not None
        assert "Authorization" in connector.session.headers
        assert connector.session.headers["Authorization"] == "Token test-token-123"

    def test_load_credentials_missing_token(self, connector):
        """Test loading credentials without api_token"""
        with pytest.raises(ConnectorMissingCredentialError) as exc_info:
            connector.load_credentials({})
        assert "api_token" in str(exc_info.value)

    def test_get_api_url(self, connector):
        """Test API URL construction"""
        url = connector._get_api_url("documents/")
        assert url == "https://paperless.example.com/api/documents/"
        
        url = connector._get_api_url("/documents/")
        assert url == "https://paperless.example.com/api/documents/"

    @patch("requests.Session.get")
    def test_make_request_success(self, mock_get, connector, credentials):
        """Test successful API request"""
        connector.load_credentials(credentials)
        
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"results": []}
        mock_get.return_value = mock_response
        
        result = connector._make_request("documents/")
        assert result == {"results": []}
        mock_get.assert_called_once()

    @patch("requests.Session.get")
    def test_make_request_401_error(self, mock_get, connector, credentials):
        """Test API request with 401 Unauthorized"""
        connector.load_credentials(credentials)
        
        mock_response = Mock()
        mock_response.status_code = 401
        mock_get.return_value = mock_response
        
        with pytest.raises(CredentialExpiredError):
            connector._make_request("documents/")

    @patch("requests.Session.get")
    def test_make_request_403_error(self, mock_get, connector, credentials):
        """Test API request with 403 Forbidden"""
        connector.load_credentials(credentials)
        
        mock_response = Mock()
        mock_response.status_code = 403
        mock_get.return_value = mock_response
        
        with pytest.raises(InsufficientPermissionsError):
            connector._make_request("documents/")

    def test_make_request_without_credentials(self, connector):
        """Test making request without loading credentials"""
        with pytest.raises(ConnectorMissingCredentialError):
            connector._make_request("documents/")

    @patch("requests.Session.get")
    def test_download_document(self, mock_get, connector, credentials):
        """Test downloading a document"""
        connector.load_credentials(credentials)
        
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.content = b"PDF content here"
        mock_get.return_value = mock_response
        
        content = connector._download_document(123)
        assert content == b"PDF content here"

    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._make_request")
    def test_list_documents_pagination(self, mock_make_request, connector, credentials):
        """Test document listing with pagination"""
        connector.load_credentials(credentials)
        
        # Simulate two pages of results
        mock_make_request.side_effect = [
            {
                "results": [{"id": 1, "title": "Doc 1"}],
                "next": "https://paperless.example.com/api/documents/?page=2",
            },
            {
                "results": [{"id": 2, "title": "Doc 2"}],
                "next": None,
            },
        ]
        
        docs = connector._list_documents()
        assert len(docs) == 2
        assert docs[0]["id"] == 1
        assert docs[1]["id"] == 2

    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._make_request")
    def test_list_documents_with_time_filter(self, mock_make_request, connector, credentials):
        """Test document listing with time filtering"""
        connector.load_credentials(credentials)
        
        mock_make_request.return_value = {"results": [], "next": None}
        
        start_time = datetime(2024, 1, 1, tzinfo=timezone.utc)
        end_time = datetime(2024, 12, 31, tzinfo=timezone.utc)
        
        connector._list_documents(start_time=start_time, end_time=end_time)
        
        # Check that the request was made with time filters
        call_args = mock_make_request.call_args
        params = call_args[1]["params"]
        assert "modified__gte" in params
        assert "modified__lte" in params
        assert params["modified__gte"] == "2024-01-01T00:00:00"
        assert params["modified__lte"] == "2024-12-31T00:00:00"

    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._make_request")
    def test_validate_connector_settings_success(self, mock_make_request, connector, credentials):
        """Test successful connector validation"""
        connector.load_credentials(credentials)
        
        mock_make_request.return_value = {"results": []}
        
        # Should not raise any exception
        connector.validate_connector_settings()

    def test_validate_connector_settings_without_credentials(self, connector):
        """Test validation without credentials"""
        with pytest.raises(ConnectorMissingCredentialError):
            connector.validate_connector_settings()

    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._list_documents")
    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._download_document")
    def test_yield_paperless_documents(self, mock_download, mock_list, connector, credentials):
        """Test document generation"""
        connector.load_credentials(credentials)
        
        # Mock document metadata
        mock_list.return_value = [
            {
                "id": 1,
                "title": "Test Document",
                "original_file_name": "test.pdf",
                "modified": "2024-01-15T10:30:00Z",
                "correspondent": "John Doe",
                "tags": [1, 2],
                "content": "This is OCR content",
            }
        ]
        
        # Mock document download
        mock_download.return_value = b"PDF content"
        
        start = datetime(2024, 1, 1, tzinfo=timezone.utc)
        end = datetime(2024, 12, 31, tzinfo=timezone.utc)
        
        batches = list(connector._yield_paperless_documents(start, end))
        assert len(batches) == 1
        assert len(batches[0]) == 1
        
        doc = batches[0][0]
        assert doc.id == "paperless_ngx:https://paperless.example.com:1"
        assert doc.source == DocumentSource.PAPERLESS_NGX
        assert doc.semantic_identifier == "Test Document"
        assert doc.extension == ".pdf"
        assert doc.blob == b"PDF content"
        assert doc.size_bytes == len(b"PDF content")
        assert "title" in doc.metadata
        assert doc.metadata["title"] == "Test Document"

    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._yield_paperless_documents")
    def test_load_from_state(self, mock_yield, connector, credentials):
        """Test load_from_state method"""
        connector.load_credentials(credentials)
        
        mock_yield.return_value = iter([[]])
        
        generator = connector.load_from_state()
        list(generator)  # Consume the generator
        
        # Verify it was called with full date range
        call_args = mock_yield.call_args[0]
        assert call_args[0].year == 1970

    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._yield_paperless_documents")
    def test_poll_source(self, mock_yield, connector, credentials):
        """Test poll_source method"""
        connector.load_credentials(credentials)
        
        mock_yield.return_value = iter([[]])
        
        start_ts = datetime(2024, 1, 1, tzinfo=timezone.utc).timestamp()
        end_ts = datetime(2024, 12, 31, tzinfo=timezone.utc).timestamp()
        
        generator = connector.poll_source(start_ts, end_ts)
        list(generator)  # Consume the generator
        
        # Verify it was called with the provided timestamps
        call_args = mock_yield.call_args[0]
        assert call_args[0].year == 2024
        assert call_args[0].month == 1
        assert call_args[1].year == 2024
        assert call_args[1].month == 12

    def test_init_with_min_content_length(self):
        """Test connector initialization with custom min_content_length"""
        connector = PaperlessNgxConnector(
            base_url="https://paperless.example.com",
            batch_size=10,
            verify_ssl=True,
            min_content_length=200,
        )
        assert connector.min_content_length == 200

    def test_default_min_content_length(self):
        """Test connector initialization with default min_content_length"""
        connector = PaperlessNgxConnector(
            base_url="https://paperless.example.com"
        )
        assert connector.min_content_length == 100  # Default value

    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._list_documents")
    def test_ocr_content_preferred_over_download(self, mock_list, connector, credentials):
        """Test that OCR content is used when sufficient length"""
        connector.load_credentials(credentials)
        
        # Mock document with sufficient OCR content
        mock_list.return_value = [{
            "id": 1,
            "title": "Test Document",
            "original_file_name": "test.pdf",
            "modified": "2024-01-01T00:00:00Z",
            "content": "A" * 150,  # 150 chars, above default threshold of 100
            "correspondent": "Test Corp",
            "document_type": "Invoice",
            "tags": [1, 2],
            "created": "2024-01-01T00:00:00Z",
        }]
        
        generator = connector._yield_paperless_documents(
            datetime(2024, 1, 1, tzinfo=timezone.utc),
            datetime(2024, 12, 31, tzinfo=timezone.utc)
        )
        
        batches = list(generator)
        assert len(batches) == 1
        docs = batches[0]
        assert len(docs) == 1
        
        doc = docs[0]
        # Should use OCR content, not download PDF
        assert doc.extension == ".txt"  # Text file from OCR content
        assert doc.blob == ("A" * 150).encode('utf-8')
        assert doc.metadata["source_type"] == "ocr_content"

    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._list_documents")
    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._download_document")
    def test_pdf_download_when_content_too_short(self, mock_download, mock_list, connector, credentials):
        """Test that PDF is downloaded when OCR content is too short"""
        connector.load_credentials(credentials)
        
        # Mock document with insufficient OCR content
        mock_list.return_value = [{
            "id": 1,
            "title": "Test Document",
            "original_file_name": "test.pdf",
            "modified": "2024-01-01T00:00:00Z",
            "content": "Short",  # Only 5 chars, below threshold of 100
        }]
        
        mock_download.return_value = b"PDF content bytes"
        
        generator = connector._yield_paperless_documents(
            datetime(2024, 1, 1, tzinfo=timezone.utc),
            datetime(2024, 12, 31, tzinfo=timezone.utc)
        )
        
        batches = list(generator)
        assert len(batches) == 1
        docs = batches[0]
        assert len(docs) == 1
        
        # Verify PDF was downloaded
        mock_download.assert_called_once_with(1)
        
        doc = docs[0]
        assert doc.blob == b"PDF content bytes"
        assert doc.metadata["source_type"] == "pdf_download"
        # Should store preview of short content
        assert "ocr_content_preview" in doc.metadata

    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._list_documents")
    @patch("common.data_source.paperless_ngx_connector.PaperlessNgxConnector._download_document")
    def test_pdf_download_when_content_empty(self, mock_download, mock_list, connector, credentials):
        """Test that PDF is downloaded when OCR content is empty"""
        connector.load_credentials(credentials)
        
        # Mock document with no OCR content
        mock_list.return_value = [{
            "id": 1,
            "title": "Test Document",
            "original_file_name": "test.pdf",
            "modified": "2024-01-01T00:00:00Z",
            "content": "",  # Empty content
        }]
        
        mock_download.return_value = b"PDF content bytes"
        
        generator = connector._yield_paperless_documents(
            datetime(2024, 1, 1, tzinfo=timezone.utc),
            datetime(2024, 12, 31, tzinfo=timezone.utc)
        )
        
        batches = list(generator)
        
        # Verify PDF was downloaded
        mock_download.assert_called_once_with(1)
        
        doc = batches[0][0]
        assert doc.blob == b"PDF content bytes"
        assert doc.metadata["source_type"] == "pdf_download"


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
