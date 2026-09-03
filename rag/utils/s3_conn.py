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

import logging
import boto3
from botocore.exceptions import ClientError
from botocore.config import Config
import time
from io import BytesIO
from common.decorator import singleton
from common import settings


@singleton
class RAGFlowS3:
    def __init__(self):
        self.conn = None
        self.s3_config = settings.S3
        self.access_key = self.s3_config.get("access_key", None)
        self.secret_key = self.s3_config.get("secret_key", None)
        self.session_token = self.s3_config.get("session_token", None)
        self.region_name = self.s3_config.get("region_name") or self.s3_config.get("region")
        self.endpoint_url = self.s3_config.get("endpoint_url", None)
        self.signature_version = self.s3_config.get("signature_version", None)
        self.addressing_style = self.s3_config.get("addressing_style", None)
        self.bucket = self.s3_config.get("bucket", None)
        self.prefix_path = self.s3_config.get("prefix_path", None)
        self.__open__()

    @staticmethod
    def use_default_bucket(method):
        def wrapper(self, bucket, *args, **kwargs):
            # If there is a default bucket, use the default bucket
            # but preserve the original bucket identifier so callers that
            # need it as a key prefix (e.g. remove_bucket in single-bucket
            # mode) can still scope the operation to the logical bucket.
            original_bucket = bucket
            actual_bucket = self.bucket if self.bucket else bucket
            if self.bucket:
                kwargs["_orig_bucket"] = original_bucket
            return method(self, actual_bucket, *args, **kwargs)

        return wrapper

    @staticmethod
    def use_prefix_path(method):
        def wrapper(self, bucket, fnm, *args, **kwargs):
            # If the prefix path is set, use the prefix path.
            # The bucket passed from the upstream call is
            # used as the file prefix. This is especially useful when you're using the default bucket
            if self.prefix_path:
                fnm = f"{self.prefix_path}/{bucket}/{fnm}"
            return method(self, bucket, fnm, *args, **kwargs)

        return wrapper

    def __open__(self):
        try:
            if self.conn:
                self.__close__()
        except Exception:
            pass

        try:
            s3_params = {}
            config_kwargs = {}
            # if not set ak/sk, boto3 s3 client would try several ways to do the authentication
            # see doc: https://boto3.amazonaws.com/v1/documentation/api/latest/guide/credentials.html#configuring-credentials
            if self.access_key and self.secret_key:
                s3_params = {
                    "aws_access_key_id": self.access_key,
                    "aws_secret_access_key": self.secret_key,
                    "aws_session_token": self.session_token,
                }
            if self.region_name:
                s3_params["region_name"] = self.region_name
            if self.endpoint_url:
                s3_params["endpoint_url"] = self.endpoint_url

            # Configure signature_version and addressing_style through Config object
            if self.signature_version:
                config_kwargs["signature_version"] = self.signature_version
            if self.addressing_style:
                config_kwargs["s3"] = {"addressing_style": self.addressing_style}

            if config_kwargs:
                s3_params["config"] = Config(**config_kwargs)

            self.conn = [boto3.client("s3", **s3_params)]
        except Exception:
            logging.exception(f"Fail to connect at region {self.region_name} or endpoint {self.endpoint_url}")

    def __close__(self):
        del self.conn[0]
        self.conn = None

    @use_default_bucket
    def bucket_exists(self, bucket, *args, **kwargs):
        try:
            logging.debug(f"head_bucket bucketname {bucket}")
            self.conn[0].head_bucket(Bucket=bucket)
            exists = True
        except ClientError:
            logging.exception(f"head_bucket error {bucket}")
            exists = False
        return exists

    def health(self):
        try:
            if self.bucket:
                self.conn[0].head_bucket(Bucket=self.bucket)
            else:
                self.conn[0].list_buckets()
            return True
        except Exception as e:
            logging.warning(f"S3 health check failed: {e}")
            return False

    def get_properties(self, bucket, key):
        return {}

    def list(self, bucket, dir, recursive=True):
        return []

    @use_prefix_path
    @use_default_bucket
    def put(self, bucket, fnm, binary, *args, **kwargs):
        logging.debug(f"bucket name {bucket}; filename :{fnm}:")
        for _ in range(1):
            try:
                if not self.bucket_exists(bucket):
                    self.conn[0].create_bucket(Bucket=bucket)
                    logging.info(f"create bucket {bucket} ********")
                r = self.conn[0].upload_fileobj(BytesIO(binary), bucket, fnm)

                return r
            except Exception:
                logging.exception(f"Fail put {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)

    @use_prefix_path
    @use_default_bucket
    def rm(self, bucket, fnm, *args, **kwargs):
        try:
            self.conn[0].delete_object(Bucket=bucket, Key=fnm)
        except Exception:
            logging.exception(f"Fail rm {bucket}/{fnm}")

    @use_prefix_path
    @use_default_bucket
    def get(self, bucket, fnm, *args, **kwargs):
        for _ in range(1):
            try:
                r = self.conn[0].get_object(Bucket=bucket, Key=fnm)
                object_data = r["Body"].read()
                return object_data
            except Exception:
                logging.exception(f"fail get {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return None

    @use_prefix_path
    @use_default_bucket
    def obj_exist(self, bucket, fnm, *args, **kwargs):
        try:
            if self.conn[0].head_object(Bucket=bucket, Key=fnm):
                return True
        except ClientError as e:
            if e.response["Error"]["Code"] == "404":
                return False
            else:
                raise

    @use_prefix_path
    @use_default_bucket
    def get_presigned_url(self, bucket, fnm, expires, *args, **kwargs):
        for _ in range(10):
            try:
                r = self.conn[0].generate_presigned_url("get_object", Params={"Bucket": bucket, "Key": fnm}, ExpiresIn=expires)

                return r
            except Exception:
                logging.exception(f"fail get url {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return None

    def _resolve_path(self, bucket, fnm):
        """Apply default_bucket and prefix_path transformations."""
        actual_bucket = self.bucket if self.bucket else bucket
        actual_fnm = f"{self.prefix_path}/{bucket}/{fnm}" if self.prefix_path else fnm
        return actual_bucket, actual_fnm

    def copy(self, src_bucket, src_path, dest_bucket, dest_path):
        try:
            actual_src_bucket, actual_src_path = self._resolve_path(src_bucket, src_path)
            actual_dest_bucket, actual_dest_path = self._resolve_path(dest_bucket, dest_path)
            copy_source = {"Bucket": actual_src_bucket, "Key": actual_src_path}
            self.conn[0].copy_object(
                CopySource=copy_source,
                Bucket=actual_dest_bucket,
                Key=actual_dest_path,
            )
            return True
        except Exception:
            logging.exception(f"Fail to copy {src_bucket}/{src_path} -> {dest_bucket}/{dest_path}")
            return False

    def move(self, src_bucket, src_path, dest_bucket, dest_path):
        try:
            if self.copy(src_bucket, src_path, dest_bucket, dest_path):
                actual_src_bucket, actual_src_path = self._resolve_path(src_bucket, src_path)
                try:
                    self.conn[0].delete_object(Bucket=actual_src_bucket, Key=actual_src_path)
                    return True
                except Exception:
                    logging.exception(f"Copied but failed to delete source: {src_bucket}/{src_path}")
                    return False
            else:
                logging.error(f"Copy failed, move aborted: {src_bucket}/{src_path}")
                return False
        except Exception:
            logging.exception(f"Fail to move {src_bucket}/{src_path} -> {dest_bucket}/{dest_path}")
            return False

    def _delete_object_batches(self, bucket, pages):
        """Delete the objects listed in ``pages`` (an iterable of ListObjects
        pages) in batches. Handles versioned buckets by deleting every listed
        version and delete marker. Raises RuntimeError if any key fails."""
        for page in pages:
            objects = []
            for obj in page.get("Versions", []) + page.get("DeleteMarkers", []):
                entry = {"Key": obj["Key"]}
                if "VersionId" in obj:
                    entry["VersionId"] = obj["VersionId"]
                objects.append(entry)
            if not objects:
                continue
            result = self.conn[0].delete_objects(Bucket=bucket, Delete={"Objects": objects})
            if result.get("Errors"):
                failed = ", ".join(err.get("Key", "?") for err in result["Errors"])
                raise RuntimeError(f"S3 object deletion failed for keys: {failed}")

    def _head_bucket_or_none(self, bucket):
        """Return the head_bucket response, or None only when the bucket does
        not exist. Other client errors (403 Forbidden, 400) propagate so the
        caller's error handler reports them instead of treating the bucket as
        absent."""
        try:
            return self.conn[0].head_bucket(Bucket=bucket)
        except ClientError as e:
            code = e.response.get("Error", {}).get("Code", "") if isinstance(e.response, dict) else ""
            if code in ("404", "NoSuchBucket", "NotFound"):
                return None
            raise

    def _delete_all_versions(self, bucket, prefix=None):
        """Delete every object version and delete marker under ``bucket``
        (optionally scoped to ``prefix``). Raises if any batch fails."""
        paginator = self.conn[0].get_paginator("list_object_versions")
        pages = paginator.paginate(Bucket=bucket, Prefix=prefix) if prefix is not None else paginator.paginate(Bucket=bucket)
        self._delete_object_batches(bucket, pages)

    @use_default_bucket
    def remove_bucket(self, bucket, **kwargs):
        orig_bucket = kwargs.pop("_orig_bucket", None)
        try:
            if self.bucket:
                # Single bucket mode: remove objects with the logical-bucket
                # prefix, but do not remove the physical bucket. The prefix
                # matches the write path: use_prefix_path only prepends the
                # "{prefix_path}/{bucket}/" segment when prefix_path is set.
                if not self.prefix_path:
                    # A shared physical bucket with no prefix_path stores keys
                    # without a logical-bucket segment, so no cleanup prefix
                    # can scope this deletion to one knowledge base: run it
                    # and the whole bucket's data would go. Refuse instead;
                    # shared-bucket namespacing needs its own migration story.
                    logging.error(
                        "Refusing to remove logical bucket %s: STORAGE_S3 bucket is shared without a prefix_path, which cannot be scoped to one knowledge base",
                        orig_bucket or bucket,
                    )
                    return
                prefix = f"{self.prefix_path}/{orig_bucket}/" if orig_bucket else f"{self.prefix_path}/"
                self._delete_all_versions(bucket, prefix)
            else:
                if self._head_bucket_or_none(bucket) is None:
                    return
                self._delete_all_versions(bucket)
                self.conn[0].delete_bucket(Bucket=bucket)
        except Exception:
            # Storage deletions must not fail silently: lifecycle callers
            # (knowledge-base and account deletion) remove metadata after this
            # returns, so re-raise after logging the failure mode. str(e) is
            # omitted because S3 error responses can embed signed request URLs.
            logging.exception(f"Fail to remove bucket {bucket}")
            raise
