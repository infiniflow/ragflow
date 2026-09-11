"""Fail-closed object removal for Business Documents export cleanup."""

from __future__ import annotations

from collections.abc import Callable
from typing import Any


_MISSING_CODES = {
    "404",
    "BLOBNOTFOUND",
    "NOSUCHBUCKET",
    "NOSUCHKEY",
    "NOSUCHOBJECT",
    "NOT_FOUND",
    "PATHNOTFOUND",
    "RESOURCENOTFOUND",
}


class StorageRemovalVerificationError(RuntimeError):
    """The backend could not prove that one exact object is absent."""


def _is_missing_error(error: Exception) -> bool:
    values = [
        getattr(error, "code", None),
        getattr(error, "error_code", None),
        getattr(error, "status_code", None),
    ]
    response = getattr(error, "response", None)
    if isinstance(response, dict):
        response_error = response.get("Error")
        if isinstance(response_error, dict):
            values.append(response_error.get("Code"))
        response_metadata = response.get("ResponseMetadata")
        if isinstance(response_metadata, dict):
            values.append(response_metadata.get("HTTPStatusCode"))
    return any(str(value).strip().upper() in _MISSING_CODES for value in values if value is not None)


def _delete_allowing_missing(operation: Callable[[], Any]) -> None:
    try:
        operation()
    except Exception as error:
        if not _is_missing_error(error):
            raise


def _head_confirms_absence(operation: Callable[[], Any]) -> bool:
    try:
        operation()
    except Exception as error:
        if _is_missing_error(error):
            return True
        raise
    return False


def _exists_confirms_absence(operation: Callable[[], Any]) -> bool:
    try:
        return operation() is False
    except Exception as error:
        if _is_missing_error(error):
            return True
        raise


def _unwrap_encrypted_storage(storage: Any) -> Any:
    storage_type = type(storage)
    if storage_type.__module__ == "rag.utils.encrypted_storage" and storage_type.__name__ == "EncryptedStorageWrapper":
        return storage.storage_impl
    return storage


def _remove_minio(storage: Any, bucket: str, key: str) -> bool:
    actual_bucket, actual_key = storage._resolve_bucket_and_path(bucket, key)
    _delete_allowing_missing(lambda: storage.conn.remove_object(actual_bucket, actual_key))
    return _head_confirms_absence(lambda: storage.conn.stat_object(actual_bucket, actual_key))


def _remove_s3(storage: Any, bucket: str, key: str) -> bool:
    actual_bucket, actual_key = storage._resolve_path(bucket, key)
    client = storage.conn[0]
    _delete_allowing_missing(lambda: client.delete_object(Bucket=actual_bucket, Key=actual_key))
    return _head_confirms_absence(lambda: client.head_object(Bucket=actual_bucket, Key=actual_key))


def _remove_oss(storage: Any, bucket: str, key: str) -> bool:
    actual_bucket = storage.bucket or bucket
    actual_key = f"{storage.prefix_path}/{key}" if storage.prefix_path else key
    _delete_allowing_missing(lambda: storage.conn.delete_object(Bucket=actual_bucket, Key=actual_key))
    return _head_confirms_absence(lambda: storage.conn.head_object(Bucket=actual_bucket, Key=actual_key))


def _remove_gcs(storage: Any, bucket: str, key: str) -> bool:
    bucket_object = storage.client.bucket(storage.bucket_name)
    blob = bucket_object.blob(storage._get_blob_path(bucket, key))
    _delete_allowing_missing(blob.delete)
    return _exists_confirms_absence(blob.exists)


def _remove_azure_spn(storage: Any, bucket: str, key: str) -> bool:
    path = f"{bucket}/{key}"
    _delete_allowing_missing(lambda: storage.conn.delete_file(path))
    file_client = storage.conn.get_file_client(path)
    return _head_confirms_absence(file_client.get_file_properties)


def _remove_azure_sas(storage: Any, bucket: str, key: str) -> bool:
    blob_client = storage.conn.get_blob_client(f"{bucket}/{key}")
    _delete_allowing_missing(blob_client.delete_blob)
    return _head_confirms_absence(blob_client.get_blob_properties)


def _remove_opendal(storage: Any, bucket: str, key: str) -> bool:
    path = f"{bucket}/{key}"
    _delete_allowing_missing(lambda: storage._operator.delete(path))
    return _exists_confirms_absence(lambda: storage._operator.exists(path))


_REMOVERS: dict[str, Callable[[Any, str, str], bool]] = {
    "AZURE_SAS": _remove_azure_sas,
    "AZURE_SPN": _remove_azure_spn,
    "AWS_S3": _remove_s3,
    "GCS": _remove_gcs,
    "MINIO": _remove_minio,
    "OPENDAL": _remove_opendal,
    "OSS": _remove_oss,
}


class BusinessDocumentStorageAdapter:
    """Delegate normal storage calls and strictly verify cleanup operations."""

    def __init__(self, storage: Any, backend_type: str) -> None:
        self._storage = storage
        self._backend_type = str(backend_type).upper()

    def put(self, bucket: str, key: str, content: bytes) -> Any:
        return self._storage.put(bucket, key, content)

    def get(self, bucket: str, key: str) -> Any:
        return self._storage.get(bucket, key)

    def remove_and_confirm_absent(self, bucket: str, key: str) -> bool:
        remover = _REMOVERS.get(self._backend_type)
        if remover is None:
            raise StorageRemovalVerificationError(f"Unsupported storage backend: {self._backend_type}")
        return remover(_unwrap_encrypted_storage(self._storage), bucket, key) is True
