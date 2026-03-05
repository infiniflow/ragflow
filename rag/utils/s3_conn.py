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
        self.region_name = self.s3_config.get("region_name", None)
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
            # but preserve the original bucket identifier so it can be
            # used as a path prefix inside the physical/default bucket.
            original_bucket = bucket
            actual_bucket = self.bucket if self.bucket else bucket
            if self.bucket:
                # pass original identifier forward for use by other decorators
                kwargs["_orig_bucket"] = original_bucket
            return method(self, actual_bucket, *args, **kwargs)

        return wrapper

    @staticmethod
    def use_prefix_path(method):
        def wrapper(self, bucket, fnm, *args, **kwargs):
            # If a default S3 bucket is configured, the use_default_bucket
            # decorator will have replaced the `bucket` arg with the physical
            # bucket name and forwarded the original identifier as `_orig_bucket`.
            # Prefer that original identifier when constructing the key path so
            # objects are stored under <physical-bucket>/<identifier>/...
            orig_bucket = kwargs.pop("_orig_bucket", None)

            if self.prefix_path:
                # If a prefix_path is configured, include it and then the identifier
                if orig_bucket:
                    fnm = f"{self.prefix_path}/{orig_bucket}/{fnm}"
                else:
                    fnm = f"{self.prefix_path}/{fnm}"
            else:
                # No prefix_path configured. If orig_bucket exists and the
                # physical bucket equals configured default, use orig_bucket as a path.
                if orig_bucket and bucket == self.bucket:
                    fnm = f"{orig_bucket}/{fnm}"

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
    def bucket_exists(self, bucket, _orig_bucket=None, *args, **kwargs):
        try:
            if self.bucket:
                # Single bucket mode: check for objects with prefix
                prefix = ""
                if self.prefix_path:
                    prefix = f"{self.prefix_path}/"
                if _orig_bucket:
                    prefix += f"{_orig_bucket}/"

                # Check if physical bucket exists
                if not self._physical_bucket_exists(bucket):
                    return False

                # Check if any objects exist with the prefix
                return self.conn[0].list_objects_v2(Bucket=bucket, Prefix=prefix, MaxKeys=1).get("KeyCount", 0) > 0
            else:
                # Multi-bucket mode: check physical bucket
                return self._physical_bucket_exists(bucket)
        except Exception:
            logging.exception(f"bucket_exists {bucket} got exception")
            return False

    def _physical_bucket_exists(self, bucket: str) -> bool:
        try:
            logging.debug(f"head_bucket bucketname {bucket}")
            self.conn[0].head_bucket(Bucket=bucket)
            return True
        except ClientError:
            logging.info(f"Bucket {bucket} does not exist")
            return False

    def health(self):
        try:
            self.conn[0].list_buckets(MaxBuckets=1)
            return True
        except Exception:
            return False

    def get_properties(self, bucket, key):
        return {}

    def list(self, bucket, dir, recursive=True):
        return []

    @use_default_bucket
    @use_prefix_path
    def put(self, bucket, fnm, binary, *args, **kwargs):
        logging.debug(f"bucket name {bucket}; filename :{fnm}:")
        for _ in range(3):
            try:
                if not self._physical_bucket_exists(bucket):
                    self.conn[0].create_bucket(Bucket=bucket)
                    logging.info(f"create bucket {bucket} ********")
                r = self.conn[0].upload_fileobj(BytesIO(binary), bucket, fnm)

                return r
            except Exception:
                logging.exception(f"Fail put {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)

    @use_default_bucket
    @use_prefix_path
    def rm(self, bucket, fnm, *args, **kwargs):
        try:
            self.conn[0].delete_object(Bucket=bucket, Key=fnm)
        except Exception:
            logging.exception(f"Fail rm {bucket}/{fnm}")

    @use_default_bucket
    @use_prefix_path
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

    @use_default_bucket
    @use_prefix_path
    def obj_exist(self, bucket, fnm, *args, **kwargs):
        try:
            if self.conn[0].head_object(Bucket=bucket, Key=fnm):
                return True
        except ClientError as e:
            if e.response["Error"]["Code"] == "404":
                return False
            else:
                raise

    @use_default_bucket
    @use_prefix_path
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

    @use_default_bucket
    def remove_bucket(self, bucket, **kwargs):
        if not self._physical_bucket_exists(bucket):
            return

        orig_bucket = kwargs.pop("_orig_bucket", None)

        prefix = ""
        if self.prefix_path:
            prefix = f"{self.prefix_path}/"
        if orig_bucket:
            prefix += f"{orig_bucket}/"

        try:
            # delete all objects with the prefix
            paginator = self.conn[0].get_paginator("list_objects_v2")
            for page in paginator.paginate(Bucket=bucket, Prefix=prefix):
                if "Contents" not in page:
                    continue
                objects = [{"Key": obj["Key"]} for obj in page["Contents"]]
                self.conn[0].delete_objects(Bucket=bucket, Delete={"Objects": objects})

            # do NOT delete bucket in single bucket mode
            if not self.bucket:
                self.conn[0].delete_bucket(Bucket=bucket)
        except Exception as e:
            logging.error(f"Fail to remove bucket {bucket}: {str(e)}")
