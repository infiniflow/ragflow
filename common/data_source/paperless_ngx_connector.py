"""Paperless-ngx connector for syncing documents from Paperless-ngx instances"""
import logging
import os
from datetime import datetime, timezone
from typing import Any, Optional
from urllib.parse import urljoin

import requests

from common.data_source.utils import get_file_ext
from common.data_source.config import (
    DocumentSource,
    INDEX_BATCH_SIZE,
    BLOB_STORAGE_SIZE_THRESHOLD,
    REQUEST_TIMEOUT_SECONDS,
)
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
)
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import Document, SecondsSinceUnixEpoch, GenerateDocumentsOutput


class PaperlessNgxConnector(LoadConnector, PollConnector):
    """Paperless-ngx connector for syncing documents from Paperless-ngx instances"""

    def __init__(
        self,
        base_url: str,
        batch_size: int = INDEX_BATCH_SIZE,
        verify_ssl: bool = True,
    ) -> None:
        """Initialize Paperless-ngx connector
        
        Args:
            base_url: Base URL of the Paperless-ngx instance (e.g., "https://paperless.example.com")
            batch_size: Number of documents per batch
            verify_ssl: Whether to verify SSL certificates (default: True)
        """
        self.base_url = base_url.rstrip("/")
        self.batch_size = batch_size
        self.verify_ssl = verify_ssl
        self.api_token: Optional[str] = None
        self._allow_images: bool | None = None
        self.size_threshold: int | None = BLOB_STORAGE_SIZE_THRESHOLD
        self.session: Optional[requests.Session] = None

    def set_allow_images(self, allow_images: bool) -> None:
        """Set whether to process images"""
        logging.info(f"Setting allow_images to {allow_images}.")
        self._allow_images = allow_images

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load credentials and initialize session
        
        Args:
            credentials: Dictionary containing 'api_token'
        
        Returns:
            None
        
        Raises:
            ConnectorMissingCredentialError: If required credentials are missing
        """
        logging.debug(f"Loading credentials for Paperless-ngx server {self.base_url}")

        self.api_token = credentials.get("api_token")
        
        if not self.api_token:
            raise ConnectorMissingCredentialError(
                "Paperless-ngx requires 'api_token' credential"
            )

        # Initialize session with auth headers
        self.session = requests.Session()
        self.session.headers.update({
            "Authorization": f"Token {self.api_token}",
            "Accept": "application/json",
        })
        self.session.verify = self.verify_ssl

        return None

    def _get_api_url(self, endpoint: str) -> str:
        """Construct full API URL for an endpoint
        
        Args:
            endpoint: API endpoint path
            
        Returns:
            Full URL to the API endpoint
        """
        # Paperless-ngx API is typically at /api/
        api_base = urljoin(self.base_url + "/", "api/")
        return urljoin(api_base, endpoint.lstrip("/"))

    def _make_request(
        self,
        endpoint: str,
        params: Optional[dict] = None,
        timeout: int = REQUEST_TIMEOUT_SECONDS,
    ) -> dict:
        """Make a request to the Paperless-ngx API
        
        Args:
            endpoint: API endpoint
            params: Query parameters
            timeout: Request timeout in seconds
            
        Returns:
            JSON response as dict
            
        Raises:
            ConnectorMissingCredentialError: If session not initialized
            CredentialExpiredError: If credentials are invalid
            InsufficientPermissionsError: If access is denied
        """
        if self.session is None:
            raise ConnectorMissingCredentialError("Session not initialized")

        url = self._get_api_url(endpoint)
        
        try:
            response = self.session.get(url, params=params, timeout=timeout)
            
            if response.status_code == 401:
                raise CredentialExpiredError("Paperless-ngx API token is invalid or expired")
            elif response.status_code == 403:
                raise InsufficientPermissionsError(
                    "Insufficient permissions to access Paperless-ngx API"
                )
            
            response.raise_for_status()
            return response.json()
            
        except requests.exceptions.Timeout:
            raise ConnectorValidationError(f"Request to {url} timed out after {timeout} seconds")
        except requests.exceptions.ConnectionError as e:
            raise ConnectorValidationError(f"Failed to connect to Paperless-ngx server: {e}")
        except requests.exceptions.RequestException as e:
            raise ConnectorValidationError(f"Request to Paperless-ngx API failed: {e}")

    def _download_document(self, document_id: int) -> bytes:
        """Download document content from Paperless-ngx
        
        Args:
            document_id: Paperless-ngx document ID
            
        Returns:
            Document content as bytes
        """
        if self.session is None:
            raise ConnectorMissingCredentialError("Session not initialized")

        # Use the download endpoint
        url = self._get_api_url(f"documents/{document_id}/download/")
        
        try:
            response = self.session.get(
                url,
                timeout=REQUEST_TIMEOUT_SECONDS,
                stream=True,
            )
            response.raise_for_status()
            
            # Read content
            content = response.content
            
            if not content:
                logging.warning(f"Downloaded content is empty for document {document_id}")
                
            return content
            
        except requests.exceptions.RequestException as e:
            logging.error(f"Failed to download document {document_id}: {e}")
            raise

    def _list_documents(
        self,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
    ) -> list[dict]:
        """List documents from Paperless-ngx with optional time filtering
        
        Args:
            start_time: Filter documents modified after this time
            end_time: Filter documents modified before this time
            
        Returns:
            List of document metadata dictionaries
        """
        all_docs = []
        page = 1
        page_size = 100  # Paperless-ngx default page size
        
        while True:
            params = {
                "page": page,
                "page_size": page_size,
            }
            
            # Add time filters if provided
            # Paperless-ngx uses 'modified__gte' and 'modified__lte' for filtering
            if start_time:
                # Format: YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS
                params["modified__gte"] = start_time.strftime("%Y-%m-%dT%H:%M:%S")
            if end_time:
                params["modified__lte"] = end_time.strftime("%Y-%m-%dT%H:%M:%S")
            
            try:
                response = self._make_request("documents/", params=params)
                
                results = response.get("results", [])
                if not results:
                    break
                
                all_docs.extend(results)
                
                # Check if there are more pages
                if not response.get("next"):
                    break
                    
                page += 1
                
            except Exception as e:
                logging.error(f"Error listing documents (page {page}): {e}")
                break
        
        logging.info(f"Found {len(all_docs)} documents in Paperless-ngx")
        return all_docs

    def _yield_paperless_documents(
        self,
        start: datetime,
        end: datetime,
    ) -> GenerateDocumentsOutput:
        """Generate documents from Paperless-ngx
        
        Args:
            start: Start datetime for filtering
            end: End datetime for filtering
            
        Yields:
            Batches of documents
        """
        if self.session is None:
            raise ConnectorMissingCredentialError("Session not initialized")

        logging.info(f"Searching for documents in Paperless-ngx between {start} and {end}")
        documents_meta = self._list_documents(start_time=start, end_time=end)
        logging.info(f"Found {len(documents_meta)} documents matching time criteria")
        
        batch: list[Document] = []
        
        for doc_meta in documents_meta:
            doc_id = doc_meta.get("id")
            title = doc_meta.get("title", f"Document {doc_id}")
            original_filename = doc_meta.get("original_file_name", title)
            
            # Parse modified time
            modified_str = doc_meta.get("modified")
            if modified_str:
                try:
                    # Paperless-ngx returns ISO format timestamps
                    modified = datetime.fromisoformat(modified_str.replace("Z", "+00:00"))
                except (ValueError, AttributeError):
                    logging.warning(f"Could not parse modified time for doc {doc_id}: {modified_str}")
                    modified = datetime.now(timezone.utc)
            else:
                modified = datetime.now(timezone.utc)
            
            # Get file extension from original filename
            file_ext = get_file_ext(original_filename)
            
            try:
                # Download document content
                logging.debug(f"Downloading document: {doc_id} - {title}")
                blob = self._download_document(doc_id)
                
                if not blob or len(blob) == 0:
                    logging.warning(f"Downloaded content is empty for document {doc_id}")
                    continue
                
                # Check size threshold
                size_bytes = len(blob)
                if (
                    self.size_threshold is not None
                    and size_bytes > self.size_threshold
                ):
                    logging.warning(
                        f"Document {doc_id} ({title}) exceeds size threshold of {self.size_threshold}. Skipping."
                    )
                    continue
                
                # Build metadata
                metadata = {
                    "title": title,
                    "original_filename": original_filename,
                }
                
                # Add optional metadata fields
                if doc_meta.get("correspondent"):
                    metadata["correspondent"] = str(doc_meta["correspondent"])
                if doc_meta.get("document_type"):
                    metadata["document_type"] = str(doc_meta["document_type"])
                if doc_meta.get("tags"):
                    # Tags is a list of IDs, could fetch tag names but keep it simple
                    metadata["tags"] = ",".join(str(t) for t in doc_meta["tags"])
                if doc_meta.get("created"):
                    metadata["created"] = doc_meta["created"]
                if doc_meta.get("content"):
                    # Paperless-ngx provides OCR'd content
                    metadata["ocr_content"] = doc_meta["content"][:500]  # First 500 chars
                
                batch.append(
                    Document(
                        id=f"paperless_ngx:{self.base_url}:{doc_id}",
                        blob=blob,
                        source=DocumentSource.PAPERLESS_NGX,
                        semantic_identifier=title,
                        extension=file_ext,
                        doc_updated_at=modified,
                        size_bytes=size_bytes,
                        metadata=metadata,
                    )
                )
                
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []
                    
            except Exception as e:
                logging.exception(f"Error processing document {doc_id} ({title}): {e}")
                continue
        
        if batch:
            yield batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Load all documents from Paperless-ngx
        
        Yields:
            Batches of documents
        """
        logging.debug(f"Loading all documents from Paperless-ngx server {self.base_url}")
        return self._yield_paperless_documents(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> GenerateDocumentsOutput:
        """Poll Paperless-ngx for updated documents
        
        Args:
            start: Start timestamp (seconds since Unix epoch)
            end: End timestamp (seconds since Unix epoch)
            
        Yields:
            Batches of documents
        """
        if self.session is None:
            raise ConnectorMissingCredentialError("Session not initialized")

        start_datetime = datetime.fromtimestamp(start, tz=timezone.utc)
        end_datetime = datetime.fromtimestamp(end, tz=timezone.utc)

        for batch in self._yield_paperless_documents(start_datetime, end_datetime):
            yield batch

    def validate_connector_settings(self) -> None:
        """Validate Paperless-ngx connector settings
        
        Raises:
            ConnectorMissingCredentialError: If credentials not loaded
            ConnectorValidationError: If settings are invalid
            CredentialExpiredError: If credentials are invalid
            InsufficientPermissionsError: If access is denied
        """
        if self.session is None:
            raise ConnectorMissingCredentialError("Paperless-ngx credentials not loaded.")

        if not self.base_url:
            raise ConnectorValidationError("No base URL was provided in connector settings.")

        try:
            # Try to list documents with a small page size to validate access
            response = self._make_request("documents/", params={"page_size": 1})
            
            # Check if we got a valid response
            if "results" not in response:
                raise ConnectorValidationError(
                    "Unexpected response format from Paperless-ngx API"
                )
            
            logging.info("Paperless-ngx connector settings validated successfully")
            
        except (CredentialExpiredError, InsufficientPermissionsError):
            raise
        except Exception as e:
            raise ConnectorValidationError(
                f"Paperless-ngx validation failed: {repr(e)}"
            )


if __name__ == "__main__":
    # Example usage
    credentials_dict = {
        "api_token": os.environ.get("PAPERLESS_API_TOKEN", "your-api-token-here"),
    }

    connector = PaperlessNgxConnector(
        base_url=os.environ.get("PAPERLESS_BASE_URL", "http://localhost:8000"),
        verify_ssl=False,  # For local testing
    )

    try:
        connector.load_credentials(credentials_dict)
        connector.validate_connector_settings()
        
        document_batch_generator = connector.load_from_state()
        for document_batch in document_batch_generator:
            print(f"Batch of {len(document_batch)} documents:")
            for doc in document_batch:
                print(f"  Document ID: {doc.id}")
                print(f"  Title: {doc.semantic_identifier}")
                print(f"  Source: {doc.source}")
                print(f"  Extension: {doc.extension}")
                print(f"  Size: {doc.size_bytes} bytes")
                print(f"  Updated At: {doc.doc_updated_at}")
                print(f"  Metadata: {doc.metadata}")
                print("---")
            break  # Just show first batch

    except ConnectorMissingCredentialError as e:
        print(f"Credential Error: {e}")
    except ConnectorValidationError as e:
        print(f"Validation Error: {e}")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")
