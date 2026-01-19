from datetime import datetime, timezone
import logging
from typing import Any, Generator

import requests

from pyairtable import Api as AirtableApi

from common.data_source.config import AIRTABLE_CONNECTOR_SIZE_THRESHOLD, INDEX_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import Document, GenerateDocumentsOutput, SecondsSinceUnixEpoch
from common.data_source.utils import extract_size_bytes, get_file_ext

class AirtableClientNotSetUpError(PermissionError):
    def __init__(self) -> None:
        super().__init__(
            "Airtable client is not set up. Did you forget to call load_credentials()?"
        )


class AirtableConnector(LoadConnector, PollConnector):
    """
    Lightweight Airtable connector.

    This connector ingests Airtable attachments as raw blobs without
    parsing file content or generating text/image sections.
    """

    def __init__(
        self,
        base_id: str,
        table_name_or_id: str,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.base_id = base_id
        self.table_name_or_id = table_name_or_id
        self.batch_size = batch_size
        self._airtable_client: AirtableApi | None = None
        self.size_threshold = AIRTABLE_CONNECTOR_SIZE_THRESHOLD

    # -------------------------
    # Credentials
    # -------------------------
    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self._airtable_client = AirtableApi(credentials["airtable_access_token"])
        return None

    @property
    def airtable_client(self) -> AirtableApi:
        if not self._airtable_client:
            raise AirtableClientNotSetUpError()
        return self._airtable_client

    # -------------------------
    # Core logic
    # -------------------------
    def load_from_state(self) -> GenerateDocumentsOutput:
        """
        Fetch all Airtable records and ingest attachments as raw blobs.

        Each attachment is converted into a single Document(blob=...).
        """
        if not self._airtable_client:
            raise ConnectorMissingCredentialError("Airtable credentials not loaded")

        table = self.airtable_client.table(self.base_id, self.table_name_or_id)
        records = table.all()

        logging.info(
            f"Starting Airtable blob ingestion for table {self.table_name_or_id}, "
            f"{len(records)} records found."
        )

        batch: list[Document] = []

        for record in records:
            record_id = record.get("id")
            fields = record.get("fields", {})
            created_time = record.get("createdTime")

            for field_value in fields.values():
                # We only care about attachment fields (lists of dicts with url/filename)
                if not isinstance(field_value, list):
                    continue

                for attachment in field_value:
                    url = attachment.get("url")
                    filename = attachment.get("filename")
                    attachment_id = attachment.get("id")

                    if not url or not filename or not attachment_id:
                        continue

                    try:
                        resp = requests.get(url, timeout=30)
                        resp.raise_for_status()
                        content = resp.content
                    except Exception:
                        logging.exception(
                            f"Failed to download attachment {filename} "
                            f"(record={record_id})"
                        )
                        continue
                    size_bytes = extract_size_bytes(attachment)
                    if (
                        self.size_threshold is not None
                        and isinstance(size_bytes, int)
                        and size_bytes > self.size_threshold
                    ):
                        logging.warning(
                            f"{filename} exceeds size threshold of {self.size_threshold}. Skipping."
                        )
                        continue
                    batch.append(
                        Document(
                            id=f"airtable:{record_id}:{attachment_id}",
                            blob=content,
                            source=DocumentSource.AIRTABLE,
                            semantic_identifier=filename,
                            extension=get_file_ext(filename),
                            size_bytes=size_bytes if size_bytes else 0,
                            doc_updated_at=datetime.strptime(created_time, "%Y-%m-%dT%H:%M:%S.%fZ").replace(tzinfo=timezone.utc)
                        )
                    )

                    if len(batch) >= self.batch_size:
                        yield batch
                        batch = []

        if batch:
            yield batch

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Generator[list[Document], None, None]:
        """Poll source to get documents"""
        start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
        end_dt = datetime.fromtimestamp(end, tz=timezone.utc)

        for batch in self.load_from_state():
            filtered: list[Document] = []

            for doc in batch:
                if not doc.doc_updated_at:
                    continue

                doc_dt = doc.doc_updated_at.astimezone(timezone.utc)

                if start_dt <= doc_dt < end_dt:
                    filtered.append(doc)

            if filtered:
                yield filtered

if __name__ == "__main__":
    import os

    logging.basicConfig(level=logging.DEBUG)
    connector = AirtableConnector("xxx","xxx")
    connector.load_credentials({"airtable_access_token": os.environ.get("AIRTABLE_ACCESS_TOKEN")})
    connector.validate_connector_settings()
    document_batches = connector.load_from_state()
    try:
        first_batch = next(document_batches)
        print(f"Loaded {len(first_batch)} documents in first batch.")
        for doc in first_batch:
            print(f"- {doc.semantic_identifier} ({doc.size_bytes} bytes)")
    except StopIteration:
        print("No documents available in Dropbox.")