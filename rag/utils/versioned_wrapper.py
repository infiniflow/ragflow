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

"""
Versioned Storage Wrapper for RAGFlow.

Wraps an existing S3- or MinIO-backed STORAGE_IMPL to add transparent
support for versioned buckets.

Supported backends:
  - S3 / OSS (boto3 SDK)       →  ``VersionId``
  - MinIO    (minio-py SDK)    →  ``version_id``

When a bucket has versioning enabled, this wrapper:

- Tracks the active VersionId for each (bucket, key) in Redis so that
  callers can retrieve the exact version they stored without changing
  the existing call signature.
- On ``put``, captures the returned VersionId and stores it as the
  active version.
- On ``get``, reads the active VersionId from Redis and fetches that
  specific version.  Falls back to the latest version when no tracked
  version exists.
- On ``rm``, permanently deletes **all versions** of an object in a
  versioned bucket (not just creating a delete-marker).
- Exposes ``list_versions``, ``get_version``, ``set_active_version``,
  ``get_active_version`` and ``delete_version`` for explicit version
  management.

The wrapper is **non-intrusive**: existing code that calls ``put/get/rm``
continues to work unchanged — it will simply operate on the "active"
version tracked by this wrapper.
"""

import logging
import time
from io import BytesIO
from typing import Optional

from common import settings

# ---------------------------------------------------------------------------
# Redis key layout for version tracking
# ---------------------------------------------------------------------------
_VERSION_KEY_PREFIX = "ragflow:obj_version"


def _redis_version_key(bucket: str, fnm: str) -> str:
    """Return the Redis key used to store the active version of an object."""
    return f"{_VERSION_KEY_PREFIX}:{bucket}:{fnm}"


# ---------------------------------------------------------------------------
# Helper: detect which backend the wrapped impl uses
# ---------------------------------------------------------------------------
def _detect_backend(storage_impl) -> str:
    """Return a backend identifier: ``"minio"``, ``"s3"`` or ``"unknown"``.

    The two backends use **completely different SDKs**:
      - MinIO → minio-py (``Minio`` client, scalar ``self.conn``)
      - S3/OSS → boto3 (``boto3.client("s3")``, stored as ``[client]`` list)

    They must dispatch to separate code paths.
    """
    cls_name = type(storage_impl).__name__.lower()
    if "minio" in cls_name:
        return "minio"
    if "s3" in cls_name or "oss" in cls_name:
        return "s3"

    # Fallback — detect from the underlying client object
    if hasattr(storage_impl, "conn"):
        conn = storage_impl.conn
        if isinstance(conn, list) and len(conn) > 0:
            conn = conn[0]
        mod = type(conn).__module__
        if "minio" in mod:
            return "minio"
        if "boto3" in mod or "botocore" in mod:
            return "s3"
    return "unknown"


class VersionedStorageWrapper:
    """Wrapper that adds version-awareness to S3 / MinIO storage backends.

    Parameters
    ----------
    storage_impl:
        An existing storage instance (RAGFlowS3, RAGFlowOSS, or RAGFlowMinio).
    redis_conn:
        A Redis connection used for version tracking.  If *None*, the
        wrapper lazily uses ``settings.REDIS_CONN``.
    version_ttl:
        TTL in seconds for version-tracking keys in Redis.  0 = no expiry.
    """

    def __init__(self, storage_impl, redis_conn=None, version_ttl: int = 0):
        self.storage_impl = storage_impl
        self.backend = _detect_backend(storage_impl)
        self.version_ttl = version_ttl

        # Lazy Redis initialisation
        self._redis = redis_conn

        # Verify backend is supported
        if self.backend == "unknown":
            raise ValueError(
                f"Unsupported storage implementation: "
                f"{type(storage_impl).__name__}. "
                f"Only S3/OSS (boto3) and MinIO (minio-py) are supported."
            )

        # Verify that the wrapped impl exposes the expected methods
        for method in ["put", "get", "rm", "obj_exist"]:
            if not hasattr(storage_impl, method):
                raise AttributeError(
                    f"Storage implementation missing required method: {method}"
                )

        logging.info(
            "VersionedStorageWrapper initialized, backend=%s", self.backend
        )

    # ------------------------------------------------------------------
    # Internal Redis helpers
    # ------------------------------------------------------------------
    @property
    def redis(self):
        if self._redis is None:
            self._redis = settings.REDIS_CONN
        return self._redis

    def _set_active_version(self, bucket: str, fnm: str, version_id: str):
        """Persist the active version for an object in Redis."""
        key = _redis_version_key(bucket, fnm)
        if version_id:
            self.redis.set(key, version_id)
            if self.version_ttl > 0:
                self.redis.expire(key, self.version_ttl)

    def _get_active_version(self, bucket: str, fnm: str) -> Optional[str]:
        """Return the active version for an object, or None."""
        key = _redis_version_key(bucket, fnm)
        v = self.redis.get(key)
        if v:
            return v.decode() if isinstance(v, bytes) else v
        return None

    def _del_active_version(self, bucket: str, fnm: str):
        """Remove the version-tracking key."""
        self.redis.delete(_redis_version_key(bucket, fnm))

    # ------------------------------------------------------------------
    # Internal: low-level SDK client access
    # ------------------------------------------------------------------
    def _boto3_client(self):
        """Return the raw boto3 S3 client (S3 / OSS).

        S3 stores ``self.conn`` as a ``[boto3.client]`` list; OSS stores it
        as a bare scalar — handle both.
        """
        conn = self.storage_impl.conn
        return conn[0] if isinstance(conn, list) else conn

    def _minio_client(self):
        """Return the raw minio-py ``Minio`` client."""
        return self.storage_impl.conn

    # ------------------------------------------------------------------
    # Path resolution (respects use_default_bucket / use_prefix_path)
    # ------------------------------------------------------------------
    def _resolve_backend_path(self, bucket: str, fnm: str):
        """Resolve (actual_bucket, actual_key) respecting decorators.

        The original storage implementations decorate ``put``/``get``/``rm``
        with ``@use_default_bucket`` and ``@use_prefix_path``.  Since we
        bypass those decorators when calling SDK methods directly, we
        replicate the path transformation here.
        """
        # RAGFlowS3 exposes a dedicated helper
        if hasattr(self.storage_impl, "_resolve_path"):
            return self.storage_impl._resolve_path(bucket, fnm)

        # Manual fallback: read bucket / prefix_path from the impl
        actual_bucket = bucket
        actual_fnm = fnm
        impl_bucket = getattr(self.storage_impl, "bucket", None) or None
        prefix = getattr(self.storage_impl, "prefix_path", None) or None

        if impl_bucket:
            actual_bucket = impl_bucket
            if prefix:
                actual_fnm = f"{prefix}/{bucket}/{fnm}"
            else:
                actual_fnm = f"{bucket}/{fnm}"
        elif prefix:
            actual_fnm = f"{prefix}/{fnm}"

        return actual_bucket, actual_fnm

    # ------------------------------------------------------------------
    # put with version capture
    # ------------------------------------------------------------------
    def _put_capture_version(self, bucket: str, fnm: str, binary: bytes,
                             tenant_id=None):
        """Upload and capture the resulting VersionId."""
        if self.backend == "minio":
            return self._put_minio(bucket, fnm, binary, tenant_id)
        return self._put_s3(bucket, fnm, binary, tenant_id)

    def _put_s3(self, bucket: str, fnm: str, binary: bytes, tenant_id):
        """Upload via boto3 ``upload_fileobj``, then ``head_object`` to
        capture ``VersionId``.

        boto3's ``upload_fileobj`` returns None, so an extra HEAD is needed.
        On non-versioned buckets, the returned ``VersionId`` is ``"null"``
        and we skip tracking.
        """
        r = self.storage_impl.put(bucket, fnm, binary, tenant_id=tenant_id)
        try:
            s3 = self._boto3_client()
            actual_bucket, actual_fnm = self._resolve_backend_path(bucket, fnm)
            head = s3.head_object(Bucket=actual_bucket, Key=actual_fnm)
            version_id = head.get("VersionId")
            if version_id and version_id != "null":
                self._set_active_version(bucket, fnm, version_id)
                logging.debug("Tracked VersionId %s for %s/%s",
                              version_id, bucket, fnm)
        except Exception:
            logging.debug(
                "Could not capture VersionId for %s/%s "
                "(bucket may not have versioning enabled)",
                bucket, fnm,
            )
        return r

    def _put_minio(self, bucket: str, fnm: str, binary: bytes, tenant_id):
        """Upload via minio-py ``put_object`` and extract ``version_id``
        from the returned ``ObjectWriteResult``.

        Unlike boto3, minio-py returns a result object that already contains
        the version_id, so no extra ``stat_object`` call is needed.
        """
        client = self._minio_client()
        actual_bucket, actual_fnm = self._resolve_backend_path(bucket, fnm)

        # Ensure bucket exists (same logic as RAGFlowMinio.put)
        impl_bucket = getattr(self.storage_impl, "bucket", None) or None
        if not impl_bucket and not client.bucket_exists(actual_bucket):
            client.make_bucket(actual_bucket)

        result = client.put_object(
            actual_bucket, actual_fnm,
            BytesIO(binary), len(binary),
        )
        version_id = getattr(result, "version_id", None)
        if version_id:
            self._set_active_version(bucket, fnm, version_id)
            logging.debug("Tracked version_id %s for %s/%s",
                          version_id, bucket, fnm)
        return result

    # ------------------------------------------------------------------
    # get with version awareness
    # ------------------------------------------------------------------
    def _get_versioned(self, bucket: str, fnm: str, tenant_id=None):
        """Get a specific version if tracked, otherwise get the latest."""
        active_version = self._get_active_version(bucket, fnm)

        if active_version is None:
            return self.storage_impl.get(bucket, fnm, tenant_id=tenant_id)

        try:
            if self.backend == "minio":
                return self._get_minio_version(bucket, fnm, active_version)
            return self._get_s3_version(bucket, fnm, active_version)
        except Exception:
            logging.warning(
                "Failed to get version %s of %s/%s, falling back to latest",
                active_version, bucket, fnm,
            )
            return self.storage_impl.get(bucket, fnm, tenant_id=tenant_id)

    def _get_s3_version(self, bucket: str, fnm: str, version_id: str):
        s3 = self._boto3_client()
        actual_bucket, actual_fnm = self._resolve_backend_path(bucket, fnm)
        r = s3.get_object(
            Bucket=actual_bucket, Key=actual_fnm, VersionId=version_id
        )
        return r["Body"].read()

    def _get_minio_version(self, bucket: str, fnm: str, version_id: str):
        client = self._minio_client()
        actual_bucket, actual_fnm = self._resolve_backend_path(bucket, fnm)
        response = client.get_object(
            actual_bucket, actual_fnm, version_id=version_id
        )
        data = response.read()
        response.close()
        response.release_conn()
        return data

    # ------------------------------------------------------------------
    # rm — purge all versions
    # ------------------------------------------------------------------
    def _rm_versioned(self, bucket: str, fnm: str):
        """Remove an object and all its versions in a versioned bucket."""
        if self.backend == "minio":
            self._rm_minio_all_versions(bucket, fnm)
        else:
            self._rm_s3_all_versions(bucket, fnm)

        self._del_active_version(bucket, fnm)

    def _rm_s3_all_versions(self, bucket: str, fnm: str):
        """Delete all versions + delete-markers of an object via boto3.

        Uses ``list_object_versions`` + ``delete_objects`` batch API.
        Falls back to a simple delete when version listing fails.
        """
        s3 = self._boto3_client()
        actual_bucket, actual_fnm = self._resolve_backend_path(bucket, fnm)

        try:
            objects_to_delete = []
            paginator = s3.get_paginator("list_object_versions")
            for page in paginator.paginate(
                Bucket=actual_bucket, Prefix=actual_fnm
            ):
                for version in page.get("Versions", []):
                    if version["Key"] == actual_fnm:
                        objects_to_delete.append(
                            {"Key": version["Key"],
                             "VersionId": version["VersionId"]}
                        )
                for marker in page.get("DeleteMarkers", []):
                    if marker["Key"] == actual_fnm:
                        objects_to_delete.append(
                            {"Key": marker["Key"],
                             "VersionId": marker["VersionId"]}
                        )
        except Exception:
            logging.exception(
                "Failed to list versions for %s/%s in versioned bucket - "
                "falling back to simple delete (may only create a "
                "delete-marker, not a permanent deletion)",
                actual_bucket, actual_fnm,
            )
            self.storage_impl.rm(bucket, fnm)
            return

        if objects_to_delete:
            try:
                s3.delete_objects(
                    Bucket=actual_bucket,
                    Delete={"Objects": objects_to_delete, "Quiet": True},
                )
                logging.info(
                    "Deleted %d version(s) of %s/%s",
                    len(objects_to_delete), actual_bucket, actual_fnm,
                )
            except Exception:
                logging.exception(
                    "Failed to delete versions of %s/%s",
                    actual_bucket, actual_fnm,
                )
        else:
            # No versions found — simple delete
            self.storage_impl.rm(bucket, fnm)

    def _rm_minio_all_versions(self, bucket: str, fnm: str):
        """Delete all versions of an object via minio-py SDK.

        Uses ``list_objects(include_version=True)`` then removes each
        version individually via ``remove_object(version_id=...)``.
        """
        client = self._minio_client()
        actual_bucket, actual_fnm = self._resolve_backend_path(bucket, fnm)

        try:
            objects = client.list_objects(
                actual_bucket, prefix=actual_fnm,
                recursive=True, include_version=True,
            )
            deleted = 0
            for obj in objects:
                if obj.object_name == actual_fnm and obj.version_id:
                    client.remove_object(
                        actual_bucket, obj.object_name,
                        version_id=obj.version_id,
                    )
                    deleted += 1
            if deleted > 0:
                logging.info(
                    "Deleted %d version(s) of %s/%s",
                    deleted, actual_bucket, actual_fnm,
                )
            else:
                # No versions found — simple delete
                client.remove_object(actual_bucket, actual_fnm)
        except Exception:
            logging.exception(
                "Failed to list versions for %s/%s via MinIO SDK",
                actual_bucket, actual_fnm,
            )
            self.storage_impl.rm(bucket, fnm)

    # ------------------------------------------------------------------
    # Public interface — drop-in compatible with existing STORAGE_IMPL
    # ------------------------------------------------------------------

    def put(self, bucket, fnm, binary, tenant_id=None):
        """Upload an object and track its version in a versioned bucket.

        On non-versioned buckets, no version is tracked (the returned
        version identifier is ``"null"`` / ``None``), and the wrapper
        behaves identically to the unwrapped impl.
        """
        return self._put_capture_version(
            bucket, fnm, binary, tenant_id=tenant_id
        )

    def get(self, bucket, fnm, tenant_id=None):
        """Retrieve an object.  If a version is tracked, fetch that
        specific version; otherwise fetch the latest."""
        return self._get_versioned(bucket, fnm, tenant_id=tenant_id)

    def rm(self, bucket, fnm, tenant_id=None):
        """Delete an object.  In a versioned bucket this permanently
        removes **all** versions (not just creating a delete-marker)."""
        return self._rm_versioned(bucket, fnm)

    def obj_exist(self, bucket, fnm, tenant_id=None):
        """Check whether an object exists (at any version)."""
        return self.storage_impl.obj_exist(bucket, fnm, tenant_id=tenant_id)

    def health(self):
        return self.storage_impl.health()

    def bucket_exists(self, bucket):
        if hasattr(self.storage_impl, "bucket_exists"):
            return self.storage_impl.bucket_exists(bucket)
        return False

    def get_presigned_url(self, bucket, fnm, expires, tenant_id=None):
        """Generate a presigned URL.

        If a version is tracked for the object, include the version
        parameter so the URL points to the exact version.
        """
        active_version = self._get_active_version(bucket, fnm)

        # S3 / OSS: include VersionId in presigned URL params
        if self.backend == "s3" and active_version:
            s3 = self._boto3_client()
            actual_bucket, actual_fnm = self._resolve_backend_path(bucket, fnm)
            for _ in range(10):
                try:
                    return s3.generate_presigned_url(
                        "get_object",
                        Params={
                            "Bucket": actual_bucket,
                            "Key": actual_fnm,
                            "VersionId": active_version,
                        },
                        ExpiresIn=expires,
                    )
                except Exception:
                    logging.exception(
                        "fail get versioned url %s/%s", bucket, fnm
                    )
                    time.sleep(1)
            return None

        # MinIO: delegate to the underlying impl (minio-py does not support
        # embedding version_id in presigned URLs through the simple API)
        if hasattr(self.storage_impl, "get_presigned_url"):
            return self.storage_impl.get_presigned_url(
                bucket, fnm, expires, tenant_id=tenant_id
            )
        return None

    def copy(self, src_bucket, src_path, dest_bucket, dest_path):
        if hasattr(self.storage_impl, "copy"):
            return self.storage_impl.copy(
                src_bucket, src_path, dest_bucket, dest_path
            )
        return False

    def move(self, src_bucket, src_path, dest_bucket, dest_path):
        if hasattr(self.storage_impl, "move"):
            return self.storage_impl.move(
                src_bucket, src_path, dest_bucket, dest_path
            )
        return False

    def remove_bucket(self, bucket, **kwargs):
        if hasattr(self.storage_impl, "remove_bucket"):
            return self.storage_impl.remove_bucket(bucket, **kwargs)
        return False

    def scan(self, bucket, fnm, tenant_id=None):
        if hasattr(self.storage_impl, "scan"):
            return self.storage_impl.scan(bucket, fnm, tenant_id=tenant_id)
        return None

    # ------------------------------------------------------------------
    # Extended version-aware API
    # ------------------------------------------------------------------

    def list_versions(self, bucket: str, fnm: str) -> list[dict]:
        """List all versions of an object.

        Returns a list of dicts:
            ``{"version_id": str, "last_modified": datetime,
              "is_latest": bool, "size": int}``
        """
        if self.backend == "minio":
            return self._list_versions_minio(bucket, fnm)
        return self._list_versions_s3(bucket, fnm)

    def _list_versions_s3(self, bucket: str, fnm: str) -> list[dict]:
        s3 = self._boto3_client()
        actual_bucket, actual_fnm = self._resolve_backend_path(bucket, fnm)
        versions = []
        try:
            paginator = s3.get_paginator("list_object_versions")
            for page in paginator.paginate(
                Bucket=actual_bucket, Prefix=actual_fnm
            ):
                for v in page.get("Versions", []):
                    if v["Key"] == actual_fnm:
                        versions.append({
                            "version_id": v["VersionId"],
                            "last_modified": v["LastModified"],
                            "is_latest": v.get("IsLatest", False),
                            "size": v.get("Size", 0),
                        })
        except Exception:
            logging.exception(
                "Failed to list versions for %s/%s",
                actual_bucket, actual_fnm,
            )
        return versions

    def _list_versions_minio(self, bucket: str, fnm: str) -> list[dict]:
        client = self._minio_client()
        actual_bucket, actual_fnm = self._resolve_backend_path(bucket, fnm)
        versions = []
        try:
            objects = client.list_objects(
                actual_bucket, prefix=actual_fnm,
                recursive=True, include_version=True,
            )
            for obj in objects:
                if obj.object_name == actual_fnm:
                    versions.append({
                        "version_id": obj.version_id,
                        "last_modified": obj.last_modified,
                        "is_latest": obj.is_latest,
                        "size": obj.size,
                    })
        except Exception:
            logging.exception(
                "Failed to list MinIO versions for %s/%s",
                actual_bucket, actual_fnm,
            )
        return versions

    def get_version(self, bucket: str, fnm: str, version_id: str):
        """Fetch a specific version of an object by its version identifier.

        Returns
        -------
        bytes or None
            The object content, or None on failure.
        """
        try:
            if self.backend == "minio":
                return self._get_minio_version(bucket, fnm, version_id)
            return self._get_s3_version(bucket, fnm, version_id)
        except Exception:
            logging.exception(
                "Failed to get version %s of %s/%s",
                version_id, bucket, fnm,
            )
            return None

    def set_active_version(self, bucket: str, fnm: str, version_id: str):
        """Explicitly set the active version for an object.

        After calling this, ``get(bucket, fnm)`` will return this
        specific version until it is changed or cleared.
        """
        self._set_active_version(bucket, fnm, version_id)
        logging.info("Active version for %s/%s set to %s",
                     bucket, fnm, version_id)

    def get_active_version(self, bucket: str, fnm: str) -> Optional[str]:
        """Return the currently tracked active version, or None."""
        return self._get_active_version(bucket, fnm)

    def delete_version(self, bucket: str, fnm: str, version_id: str):
        """Delete a single version of an object.

        If the deleted version was the active version, the tracking key
        is cleared so that subsequent ``get`` calls fall back to the
        latest version.
        """
        try:
            if self.backend == "minio":
                client = self._minio_client()
                actual_bucket, actual_fnm = self._resolve_backend_path(
                    bucket, fnm
                )
                client.remove_object(
                    actual_bucket, actual_fnm, version_id=version_id
                )
            else:
                s3 = self._boto3_client()
                actual_bucket, actual_fnm = self._resolve_backend_path(
                    bucket, fnm
                )
                s3.delete_object(
                    Bucket=actual_bucket,
                    Key=actual_fnm,
                    VersionId=version_id,
                )

            active = self._get_active_version(bucket, fnm)
            if active == version_id:
                self._del_active_version(bucket, fnm)

            logging.info("Deleted version %s of %s/%s",
                         version_id, bucket, fnm)
        except Exception:
            logging.exception(
                "Failed to delete version %s of %s/%s",
                version_id, bucket, fnm,
            )


# ---------------------------------------------------------------------------
# Factory helper
# ---------------------------------------------------------------------------

def create_versioned_storage(storage_impl, redis_conn=None,
                             version_ttl: int = 0):
    """Create a VersionedStorageWrapper around an existing storage impl.

    This is the recommended way to instantiate the wrapper.

    Parameters
    ----------
    storage_impl:
        An existing S3/OSS/MinIO storage backend instance.
    redis_conn:
        Optional Redis connection.  If None, the wrapper lazily uses
        ``settings.REDIS_CONN``.
    version_ttl:
        TTL in seconds for version-tracking keys in Redis.  0 = no expiry.

    Returns
    -------
    VersionedStorageWrapper
    """
    return VersionedStorageWrapper(
        storage_impl, redis_conn=redis_conn, version_ttl=version_ttl
    )
