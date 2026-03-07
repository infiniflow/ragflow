"""DingTalk AI Table connector for RAGFlow. By the way, "notable" is a reference to the DingTalk AI Table.

This connector ingests records from DingTalk AI Table as documents.
It first retrieves all sheets from a specified table, then fetches all records
from each sheet.

API Documentation:
- GetAllSheets: https://open.dingtalk.com/document/development/api-notable-getallsheets
- ListRecords: https://open.dingtalk.com/document/development/api-notable-listrecords
"""

import json
import logging
from datetime import datetime, timezone
from typing import Any

from alibabacloud_dingtalk.notable_1_0.client import Client as NotableClient
from alibabacloud_dingtalk.notable_1_0 import models as notable_models
from alibabacloud_tea_openapi import models as open_api_models
from alibabacloud_tea_util import models as util_models
from alibabacloud_tea_util.client import Client as UtilClient

from common.data_source.config import INDEX_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch
from common.data_source.models import Document, GenerateDocumentsOutput

logger = logging.getLogger(__name__)

# Document ID prefix for DingTalk Notable
_DINGTALK_AI_TABLE_DOC_ID_PREFIX = "dingtalk_ai_table:"


class DingTalkAITableClientNotSetUpError(PermissionError):
    """Exception raised when DingTalk Notable client is not initialized."""

    def __init__(self) -> None:
        super().__init__("DingTalk Notable client is not set up. Did you forget to call load_credentials()?")


class DingTalkAITableConnector(LoadConnector, PollConnector):
    """
    DingTalk AI Table (Notable) connector for accessing table records.

    This connector:
    1. Retrieves all sheets from a specified Notable table using GetAllSheets API
    2. For each sheet, fetches all records using ListRecords API with pagination
    3. Converts each record into a Document for RAGFlow ingestion

    Required credentials:
    - access_token: DingTalk access token (x-acs-dingtalk-access-token)
    - operator_id: User's unionId for API calls

    Configuration:
    - table_id: The Notable table ID (e.g., 'qnYxxx')
    """

    def __init__(
        self,
        table_id: str,
        operator_id: str,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        """
        Initialize the DingTalk Notable connector.

        Args:
            table_id: The Notable table ID
            operator_id: User's unionId for API calls
            batch_size: Number of records per batch for document generation
        """
        self.table_id = table_id
        self.operator_id = operator_id
        self.batch_size = batch_size
        self._client: NotableClient | None = None
        self._access_token: str | None = None

    def _create_client(self) -> NotableClient:
        """Create DingTalk Notable API client."""
        config = open_api_models.Config()
        config.protocol = "https"
        config.region_id = "central"
        return NotableClient(config)

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """
        Load DingTalk credentials.

        Args:
            credentials: Dictionary containing 'access_token'

        Returns:
            None
        """
        access_token = credentials.get("access_token")
        if not access_token:
            raise ConnectorMissingCredentialError("DingTalk access_token is required")

        self._access_token = access_token
        self._client = self._create_client()
        return None

    @property
    def client(self) -> NotableClient:
        """Get the DingTalk AITable client."""
        if self._client is None:
            raise DingTalkAITableClientNotSetUpError()
        return self._client

    @property
    def access_token(self) -> str:
        """Get the access token."""
        if self._access_token is None:
            raise ConnectorMissingCredentialError("DingTalk access_token not loaded")
        return self._access_token

    def validate_connector_settings(self) -> None:
        """Validate DingTalk connector settings by trying to get all sheets."""
        if self._client is None or self._access_token is None:
            raise ConnectorMissingCredentialError("DingTalk Notable")

        try:
            # Try to get sheets to validate credentials
            headers = notable_models.GetAllSheetsHeaders()
            headers.x_acs_dingtalk_access_token = self._access_token

            request = notable_models.GetAllSheetsRequest(
                operator_id=self.operator_id,
            )

            self.client.get_all_sheets_with_options(
                self.table_id,
                request,
                headers,
                util_models.RuntimeOptions(),
            )
        except Exception as e:
            logger.exception("[DingTalk Notable]: Failed to validate credentials")
            raise ConnectorValidationError(f"DingTalk Notable credential validation failed: {e}")

    def _get_all_sheets(self) -> list[dict[str, Any]]:
        """
        Retrieve all sheets from the Notable table.

        Returns:
            List of sheet information dictionaries
        """
        headers = notable_models.GetAllSheetsHeaders()
        headers.x_acs_dingtalk_access_token = self._access_token

        request = notable_models.GetAllSheetsRequest(
            operator_id=self.operator_id,
        )

        try:
            response = self.client.get_all_sheets_with_options(
                self.table_id,
                request,
                headers,
                util_models.RuntimeOptions(),
            )

            sheets = []
            if response.body and response.body.value:
                for sheet in response.body.value:
                    sheets.append(
                        {
                            "id": sheet.id,
                            "name": sheet.name,
                        }
                    )

            logger.info(f"[DingTalk Notable]: Found {len(sheets)} sheets in table {self.table_id}")
            return sheets

        except Exception as e:
            logger.exception(f"[DingTalk Notable]: Failed to get sheets: {e}")
            raise

    def _list_records(
        self,
        sheet_id: str,
        next_token: str | None = None,
        max_results: int = 100,
    ) -> tuple[list[dict[str, Any]], str | None]:
        """
        List records from a specific sheet with pagination.

        Args:
            sheet_id: The sheet ID
            next_token: Token for pagination
            max_results: Maximum number of results per page

        Returns:
            Tuple of (records list, next_token or None if no more)
        """
        headers = notable_models.ListRecordsHeaders()
        headers.x_acs_dingtalk_access_token = self._access_token

        request = notable_models.ListRecordsRequest(
            operator_id=self.operator_id,
            max_results=max_results,
            next_token=next_token or "",
        )

        try:
            response = self.client.list_records_with_options(
                self.table_id,
                sheet_id,
                request,
                headers,
                util_models.RuntimeOptions(),
            )

            records = []
            new_next_token = None

            if response.body:
                if response.body.records:
                    for record in response.body.records:
                        records.append(
                            {
                                "id": record.id,
                                "fields": record.fields,
                            }
                        )
                if response.body.next_token:
                    new_next_token = response.body.next_token

            return records, new_next_token

        except Exception as e:
            if not UtilClient.empty(getattr(e, "code", None)) and not UtilClient.empty(getattr(e, "message", None)):
                logger.error(f"[DingTalk AITable]: API error - code: {e.code}, message: {e.message}")
            raise

    def _get_all_records(self, sheet_id: str) -> list[dict[str, Any]]:
        """
        Retrieve all records from a sheet with pagination.

        Args:
            sheet_id: The sheet ID

        Returns:
            List of all records
        """
        all_records = []
        next_token = None

        while True:
            records, next_token = self._list_records(
                sheet_id=sheet_id,
                next_token=next_token,
            )
            all_records.extend(records)

            if not next_token:
                break

        logger.info(f"[DingTalk Notable]: Retrieved {len(all_records)} records from sheet {sheet_id}")
        return all_records

    def _convert_record_to_document(
        self,
        record: dict[str, Any],
        sheet_id: str,
        sheet_name: str,
    ) -> Document:
        """
        Convert a Notable record to a Document.

        Args:
            record: The record dictionary
            sheet_id: The sheet ID
            sheet_name: The sheet name

        Returns:
            Document object
        """
        record_id = record.get("id", "unknown")
        fields = record.get("fields", {})

        # Convert fields to JSON string for blob content
        content = json.dumps(fields, ensure_ascii=False, indent=2)
        blob = content.encode("utf-8")

        # Create semantic identifier from record fields
        # Try to find a meaningful title/name field
        semantic_identifier = f"{sheet_name} - Record {record_id}"

        # Try to find a title-like field
        for key, value in fields.items():
            if isinstance(value, str) and len(value) > 0 and len(value) < 100:
                semantic_identifier = f"{sheet_name} - {value[:50]}"
                break

        # Metadata
        metadata: dict[str, str | list[str]] = {
            "table_id": self.table_id,
            "sheet_id": sheet_id,
            "sheet_name": sheet_name,
            "record_id": record_id,
        }

        # Create document
        doc = Document(
            id=f"{_DINGTALK_AI_TABLE_DOC_ID_PREFIX}{self.table_id}:{sheet_id}:{record_id}",
            source=DocumentSource.DINGTALK_AI_TABLE,
            semantic_identifier=semantic_identifier,
            extension=".json",
            blob=blob,
            size_bytes=len(blob),
            doc_updated_at=datetime.now(timezone.utc),
            metadata=metadata,
        )

        return doc

    def _yield_documents_from_table(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> GenerateDocumentsOutput:
        """
        Yield documents from all sheets in the table.

        Args:
            start: Optional start timestamp for filtering
            end: Optional end timestamp for filtering

        Yields:
            Lists of Document objects
        """
        # Get all sheets
        sheets = self._get_all_sheets()

        batch: list[Document] = []

        for sheet in sheets:
            sheet_id = sheet["id"]
            sheet_name = sheet["name"]

            # Get all records from this sheet
            records = self._get_all_records(sheet_id)

            for record in records:
                doc = self._convert_record_to_document(
                    record=record,
                    sheet_id=sheet_id,
                    sheet_name=sheet_name,
                )

                # Apply time filtering if specified
                if start is not None or end is not None:
                    doc_time = doc.doc_updated_at.timestamp() if doc.doc_updated_at else None
                    if doc_time is not None:
                        if start is not None and doc_time < start:
                            continue
                        if end is not None and doc_time > end:
                            continue

                batch.append(doc)

                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

        if batch:
            yield batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        """
        Load all documents from the DingTalk Notable table.

        Yields:
            Lists of Document objects
        """
        return self._yield_documents_from_table()

    def poll_source(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> GenerateDocumentsOutput:
        """
        Poll for documents within a time range.

        Args:
            start: Start timestamp
            end: End timestamp

        Yields:
            Lists of Document objects
        """
        return self._yield_documents_from_table(start=start, end=end)


if __name__ == "__main__":
    import os

    logging.basicConfig(level=logging.DEBUG)

    # Example usage
    table_id = os.environ.get("DINGTALK_AI_TABLE_BASE_ID", "")
    operator_id = os.environ.get("DINGTALK_OPERATOR_ID", "")
    access_token = os.environ.get("DINGTALK_ACCESS_TOKEN", "")

    if not all([table_id, operator_id, access_token]):
        print("Please set DINGTALK_AI_TABLE_BASE_ID, DINGTALK_OPERATOR_ID, and DINGTALK_ACCESS_TOKEN environment variables")
        exit(1)

    connector = DingTalkAITableConnector(
        table_id=table_id,
        operator_id=operator_id,
    )
    connector.load_credentials({"access_token": access_token})

    try:
        connector.validate_connector_settings()
        print("Connector settings validated successfully")
    except Exception as e:
        print(f"Validation failed: {e}")
        exit(1)

    document_batches = connector.load_from_state()
    try:
        first_batch = next(document_batches)
        print(f"Loaded {len(first_batch)} documents in first batch.")
        for doc in first_batch[:5]:  # Print first 5 docs
            print(f"- {doc.semantic_identifier} ({doc.size_bytes} bytes)")
            print(f"  Metadata: {doc.metadata}")
    except StopIteration:
        print("No documents available in DingTalk Notable table.")
