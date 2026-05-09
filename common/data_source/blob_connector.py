"""Blob storage connector"""
import logging
import os
from collections.abc import Iterator
from datetime import datetime, timezone
from typing import Any, Optional

import xxhash

from common.data_source.utils import (
    create_s3_client,
    detect_bucket_region,
    download_object,
    extract_size_bytes,
    get_file_ext,
)
from common.data_source.config import BlobType, DocumentSource, BLOB_STORAGE_SIZE_THRESHOLD, INDEX_BATCH_SIZE
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError
)
from common.data_source.interfaces import (
    FingerprintConnector,
    LoadConnector,
    PollConnector,
)
from common.data_source.models import (
    Document,
    KeyRecord,
    SecondsSinceUnixEpoch,
    GenerateDocumentsOutput,
    GenerateSlimDocumentOutput,
    SlimDocument,
)


def _normalize_etag(raw_etag: Optional[str]) -> Optional[str]:
    """Return a 32-char hex fingerprint derived from an S3 ETag.

    S3 ETags are MD5 (32 hex chars) for single-part uploads and "<md5>-<n>"
    (34+ chars) for multipart. We always hash so the column format is uniform
    regardless of upload type or provider quirks; equality of the hashed value
    is sufficient for change detection.
    """
    if not raw_etag:
        return None
    return xxhash.xxh128(raw_etag.strip('"').encode()).hexdigest()


class BlobStorageConnector(LoadConnector, PollConnector, FingerprintConnector):
    """Blob storage connector"""

    def __init__(
        self,
        bucket_type: str,
        bucket_name: str,
        prefix: str = "",
        batch_size: int = INDEX_BATCH_SIZE,
        european_residency: bool = False,
    ) -> None:
        self.bucket_type: BlobType = BlobType(bucket_type)
        self.bucket_name = bucket_name.strip()
        self.prefix = prefix if not prefix or prefix.endswith("/") else prefix + "/"
        self.batch_size = batch_size
        self.s3_client: Optional[Any] = None
        self._allow_images: bool | None = None
        self.size_threshold: int | None = BLOB_STORAGE_SIZE_THRESHOLD
        self.bucket_region: Optional[str] = None
        self.european_residency: bool = european_residency
        # Populated by list_keys() so a subsequent get_value(key) can find the
        # raw S3 object metadata (LastModified, ETag, Key, Size) without a second
        # head_object call. Lifetime is one list_keys() pass.
        self._listing_cache: dict[str, dict[str, Any]] = {}
        self._filename_counts: dict[str, int] = {}

    def set_allow_images(self, allow_images: bool) -> None:
        """Set whether to process images"""
        logging.info(f"Setting allow_images to {allow_images}.")
        self._allow_images = allow_images

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load credentials"""
        logging.debug(
            f"Loading credentials for {self.bucket_name} of type {self.bucket_type}"
        )

        # Validate credentials
        if self.bucket_type == BlobType.R2:
            if not all( 
                credentials.get(key)
                for key in ["r2_access_key_id", "r2_secret_access_key", "account_id"]
            ):
                raise ConnectorMissingCredentialError("Cloudflare R2")

        elif self.bucket_type == BlobType.S3:
            authentication_method = credentials.get("authentication_method", "access_key")

            if authentication_method == "access_key":
                if not all(
                    credentials.get(key)
                    for key in ["aws_access_key_id", "aws_secret_access_key"]
                ):
                    raise ConnectorMissingCredentialError("Amazon S3")

            elif authentication_method == "iam_role":
                if not credentials.get("aws_role_arn"):
                    raise ConnectorMissingCredentialError("Amazon S3 IAM role ARN is required")
                
            elif authentication_method == "assume_role":
                pass

            else:
                raise ConnectorMissingCredentialError("Unsupported S3 authentication method")

        elif self.bucket_type == BlobType.GOOGLE_CLOUD_STORAGE:
            if not all(
                credentials.get(key) for key in ["access_key_id", "secret_access_key"]
            ):
                raise ConnectorMissingCredentialError("Google Cloud Storage")

        elif self.bucket_type == BlobType.OCI_STORAGE:
            if not all(
                credentials.get(key)
                for key in ["namespace", "region", "access_key_id", "secret_access_key"]
            ):
                raise ConnectorMissingCredentialError("Oracle Cloud Infrastructure")

        elif self.bucket_type == BlobType.S3_COMPATIBLE:
            if not all(
                credentials.get(key)
                for key in ["endpoint_url", "aws_access_key_id", "aws_secret_access_key", "addressing_style"]
            ):
                raise ConnectorMissingCredentialError("S3 Compatible Storage")

        else:
            raise ValueError(f"Unsupported bucket type: {self.bucket_type}")

        # Create S3 client
        self.s3_client = create_s3_client(
            self.bucket_type, credentials, self.european_residency
        )

        # Detect bucket region (only important for S3)
        if self.bucket_type == BlobType.S3:
            self.bucket_region = detect_bucket_region(self.s3_client, self.bucket_name)

        return None

    def _build_document_from_obj(
        self,
        obj: dict[str, Any],
        filename_counts: dict[str, int],
    ) -> Optional[Document]:
        """Materialize a Document for one S3 object, downloading its body."""
        key = obj["Key"]
        file_name = os.path.basename(key)
        last_modified = obj["LastModified"].replace(tzinfo=timezone.utc)

        size_bytes = extract_size_bytes(obj)
        if (
            self.size_threshold is not None
            and isinstance(size_bytes, int)
            and size_bytes > self.size_threshold
        ):
            logging.warning(
                f"{file_name} exceeds size threshold of {self.size_threshold}. Skipping."
            )
            return None

        blob = download_object(
            self.s3_client, self.bucket_name, key, self.size_threshold
        )
        if blob is None:
            return None

        return Document(
            id=f"{self.bucket_type}:{self.bucket_name}:{key}",
            blob=blob,
            source=DocumentSource(self.bucket_type.value),
            semantic_identifier=self._get_semantic_id(key, file_name, filename_counts),
            extension=get_file_ext(file_name),
            doc_updated_at=last_modified,
            size_bytes=size_bytes if size_bytes else 0,
            fingerprint=_normalize_etag(obj.get("ETag")),
        )

    def _yield_blob_objects(
        self,
        start: datetime,
        end: datetime,
    ) -> GenerateDocumentsOutput:
        """Generate bucket objects"""
        all_objects, filename_counts = self._collect_blob_objects(start, end)

        batch: list[Document] = []
        for obj in all_objects:
            try:
                doc = self._build_document_from_obj(obj, filename_counts)
                if doc is None:
                    continue
                batch.append(doc)
                if len(batch) == self.batch_size:
                    yield batch
                    batch = []
            except Exception:
                logging.exception(f"Error decoding object {obj.get('Key')}")

        if batch:
            yield batch

    def list_keys(self) -> Iterator[KeyRecord]:
        """Enumerate the full bucket keyspace with per-object fingerprints.

        Cheap path: relies on list_objects_v2 which returns ETag in the listing,
        so no GetObject call is needed. Caches each object's metadata so a
        subsequent get_value(key) call can rebuild the Document without a second
        round-trip to S3.
        """
        if self.s3_client is None:
            raise ConnectorMissingCredentialError("Blob storage")

        all_objects, filename_counts = self._collect_blob_objects(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )
        self._filename_counts = filename_counts
        self._listing_cache = {}

        for obj in all_objects:
            doc_id = f"{self.bucket_type}:{self.bucket_name}:{obj['Key']}"
            self._listing_cache[doc_id] = obj
            yield KeyRecord(
                key=doc_id,
                fingerprint=_normalize_etag(obj.get("ETag")),
            )

    def get_value(self, key: str) -> Document:
        """Materialize the Document for a key previously yielded by list_keys().

        Must be called within the same list_keys() pass that produced the key,
        since the metadata cache lives on the connector instance and is reset
        each list_keys() call.
        """
        obj = self._listing_cache.get(key)
        if obj is None:
            raise KeyError(
                f"get_value({key!r}) called before list_keys() yielded the key, "
                "or after a subsequent list_keys() reset the cache"
            )
        doc = self._build_document_from_obj(obj, self._filename_counts)
        if doc is None:
            raise RuntimeError(f"Failed to materialize Document for key {key!r}")
        return doc

    def _collect_blob_objects(
        self,
        start: datetime,
        end: datetime,
    ) -> tuple[list[dict[str, Any]], dict[str, int]]:
        """Collect object metadata for files in the requested window."""
        if self.s3_client is None:
            raise ConnectorMissingCredentialError("Blob storage")

        paginator = self.s3_client.get_paginator("list_objects_v2")
        pages = paginator.paginate(Bucket=self.bucket_name, Prefix=self.prefix)

        # Collect all objects first to count filename occurrences
        all_objects: list[dict[str, Any]] = []
        for page in pages:
            if "Contents" not in page:
                continue
            for obj in page["Contents"]:
                if obj["Key"].endswith("/"):
                    continue
                last_modified = obj["LastModified"].replace(tzinfo=timezone.utc)
                if start < last_modified <= end:
                    all_objects.append(obj)

        filename_counts: dict[str, int] = {}
        for obj in all_objects:
            file_name = os.path.basename(obj["Key"])
            filename_counts[file_name] = filename_counts.get(file_name, 0) + 1

        return all_objects, filename_counts

    def _get_semantic_id(
        self,
        key: str,
        file_name: str,
        filename_counts: dict[str, int],
    ) -> str:
        """Use full relative path only when filenames collide."""
        if filename_counts.get(file_name, 0) > 1:
            relative_path = key
            if self.prefix and key.startswith(self.prefix):
                relative_path = key[len(self.prefix):]
            return relative_path.replace("/", " / ") if relative_path else file_name
        return file_name

    def retrieve_all_slim_docs_perm_sync(
        self,
        callback: Any = None,
    ) -> GenerateSlimDocumentOutput:
        """Return a full current snapshot of blob object IDs without downloading content."""
        del callback

        all_objects, _ = self._collect_blob_objects(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )

        batch: list[SlimDocument] = []
        for obj in all_objects:
            batch.append(
                SlimDocument(id=f"{self.bucket_type}:{self.bucket_name}:{obj['Key']}")
            )
            if len(batch) == self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Load documents from state"""
        logging.debug("Loading blob objects")
        return self._yield_blob_objects(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> GenerateDocumentsOutput:
        """Poll source to get documents"""
        if self.s3_client is None:
            raise ConnectorMissingCredentialError("Blob storage")

        start_datetime = datetime.fromtimestamp(start, tz=timezone.utc)
        end_datetime = datetime.fromtimestamp(end, tz=timezone.utc)

        for batch in self._yield_blob_objects(start_datetime, end_datetime):
            yield batch

    def validate_connector_settings(self) -> None:
        """Validate connector settings"""
        if self.s3_client is None:
            raise ConnectorMissingCredentialError(
                "Blob storage credentials not loaded."
            )

        if not self.bucket_name:
            raise ConnectorValidationError(
                "No bucket name was provided in connector settings."
            )

        try:
            # Lightweight validation step
            self.s3_client.list_objects_v2(
                Bucket=self.bucket_name, Prefix=self.prefix, MaxKeys=1
            )

        except Exception as e:
            error_code = getattr(e, 'response', {}).get('Error', {}).get('Code', '')
            status_code = getattr(e, 'response', {}).get('ResponseMetadata', {}).get('HTTPStatusCode')

            # Common S3 error scenarios
            if error_code in [
                "AccessDenied",
                "InvalidAccessKeyId",
                "SignatureDoesNotMatch",
            ]:
                if status_code == 403 or error_code == "AccessDenied":
                    raise InsufficientPermissionsError(
                        f"Insufficient permissions to list objects in bucket '{self.bucket_name}'. "
                        "Please check your bucket policy and/or IAM policy."
                    )
                if status_code == 401 or error_code == "SignatureDoesNotMatch":
                    raise CredentialExpiredError(
                        "Provided blob storage credentials appear invalid or expired."
                    )

                raise CredentialExpiredError(
                    f"Credential issue encountered ({error_code})."
                )

            if error_code == "NoSuchBucket" or status_code == 404:
                raise ConnectorValidationError(
                    f"Bucket '{self.bucket_name}' does not exist or cannot be found."
                )

            raise ConnectorValidationError(
                f"Unexpected S3 client error (code={error_code}, status={status_code}): {e}"
            )


if __name__ == "__main__":
    # Example usage
    credentials_dict = {
        "aws_access_key_id": os.environ.get("AWS_ACCESS_KEY_ID"),
        "aws_secret_access_key": os.environ.get("AWS_SECRET_ACCESS_KEY"),
    }

    # Initialize connector
    connector = BlobStorageConnector(
        bucket_type=os.environ.get("BUCKET_TYPE") or "s3",
        bucket_name=os.environ.get("BUCKET_NAME") or "yyboombucket",
        prefix="",
    )

    try:
        connector.load_credentials(credentials_dict)
        document_batch_generator = connector.load_from_state()
        for document_batch in document_batch_generator:
            print("First batch of documents:")
            for doc in document_batch:
                print(f"Document ID: {doc.id}")
                print(f"Semantic Identifier: {doc.semantic_identifier}")
                print(f"Source: {doc.source}")
                print(f"Updated At: {doc.doc_updated_at}")
                print("---")
            break

    except ConnectorMissingCredentialError as e:
        print(f"Error: {e}")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")
