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
from rag.utils import singleton
from rag import settings


@singleton
class RAGFlowS3:
    def __init__(self):
        self.conn = None
        self.gcs_client = None
        self.gcs_bucket_name = None
        self.s3_config = settings.S3
        self.access_key = self.s3_config.get("access_key", None)
        self.secret_key = self.s3_config.get("secret_key", None)
        self.region = self.s3_config.get("region", None)
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
            actual_bucket = self.bucket if self.bucket else bucket
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

            # if not set ak/sk, boto3 s3 client would try several ways to do the authentication
            # see doc: https://boto3.amazonaws.com/v1/documentation/api/latest/guide/credentials.html#configuring-credentials
            is_gcs = self.endpoint_url and "storage.googleapis.com" in self.endpoint_url

            if self.access_key and self.secret_key:
                s3_params = {
                    "aws_access_key_id": self.access_key,
                    "aws_secret_access_key": self.secret_key,
                }
            elif is_gcs:
                # For GCS, try to get credentials from Google Application Default Credentials
                try:
                    from google.auth import default
                    import google.auth.transport.requests

                    credentials, project = default(
                        scopes=["https://www.googleapis.com/auth/cloud-platform"]
                    )
                    request = google.auth.transport.requests.Request()
                    credentials.refresh(request)

                    # Use GOOG1E as access key ID with the OAuth token for GCS S3-compatible API
                    s3_params = {
                        "aws_access_key_id": "GOOG1E",
                        "aws_secret_access_key": credentials.token,
                    }
                    logging.info(
                        "Using Google Cloud Storage with Application Default Credentials"
                    )

                    # Also initialize native GCS client for operations that might fail with S3 API
                    try:
                        from google.cloud import storage

                        self.gcs_client = storage.Client(project=project or "toteqa")
                        self.gcs_bucket_name = self.bucket
                        logging.info(
                            "Native GCS client initialized for reliable operations"
                        )
                    except ImportError:
                        logging.warning(
                            "google-cloud-storage not available, S3-compatible mode only"
                        )
                        self.gcs_client = None

                except Exception as e:
                    logging.warning(f"Failed to get Google credentials: {e}")
                    self.gcs_client = None
            # For other services without explicit keys, let boto3 use Application Default Credentials

            if self.region and self.region in self.s3_config:
                s3_params["region_name"] = self.region
            if "endpoint_url" in self.s3_config and self.endpoint_url:
                s3_params["endpoint_url"] = self.endpoint_url

            # Build config object
            config_params = {}
            if "signature_version" in self.s3_config and self.signature_version:
                config_params["signature_version"] = self.signature_version
            if "addressing_style" in self.s3_config and self.addressing_style:
                config_params["addressing_style"] = self.addressing_style

            if config_params:
                s3_params["config"] = Config(s3=config_params)

            self.conn = boto3.client("s3", **s3_params)

            # Log which service we're connecting to
            if self.endpoint_url and "storage.googleapis.com" in self.endpoint_url:
                logging.info("Connecting to Google Cloud Storage via S3-compatible API")
            else:
                logging.info("Connecting to S3-compatible storage")

        except Exception:
            logging.exception(
                f"Fail to connect at region {self.region} or endpoint {self.endpoint_url}"
            )

    def __close__(self):
        del self.conn
        self.conn = None
        self.gcs_client = None

    @use_default_bucket
    def bucket_exists(self, bucket):
        try:
            logging.debug(f"head_bucket bucketname {bucket}")
            self.conn.head_bucket(Bucket=bucket)
            return True
        except ClientError as e:
            logging.exception(f"head_bucket error {bucket}")

            # Try GCS native client as fallback
            if self.gcs_client:
                try:
                    logging.info(f"Trying GCS native client to check bucket {bucket}")
                    gcs_bucket = self.gcs_client.bucket(bucket)
                    # Try to get bucket metadata to check if it exists
                    gcs_bucket.reload()
                    logging.info(
                        f"Bucket {bucket} exists (confirmed via GCS native client)"
                    )
                    return True
                except Exception as gcs_e:
                    logging.info(
                        f"Bucket {bucket} does not exist or not accessible via GCS: {gcs_e}"
                    )
                    return False

            return False

    def health(self):
        bucket = self.bucket
        fnm = "txtxtxtxt1"
        fnm, binary = (
            f"{self.prefix_path}/{fnm}" if self.prefix_path else fnm
        ), b"_t@@@1"
        if not self.bucket_exists(bucket):
            self.conn.create_bucket(Bucket=bucket)
            logging.debug(f"create bucket {bucket} ********")

        r = self.conn.upload_fileobj(BytesIO(binary), bucket, fnm)
        return r

    def get_properties(self, bucket, key):
        return {}

    def list(self, bucket, dir, recursive=True):
        return []

    @use_prefix_path
    @use_default_bucket
    def put(self, bucket, fnm, binary):
        logging.debug(f"bucket name {bucket}; filename :{fnm}:")
        for _ in range(1):
            try:
                if not self.bucket_exists(bucket):
                    self.conn.create_bucket(Bucket=bucket)
                    logging.info(f"create bucket {bucket} ********")
                r = self.conn.upload_fileobj(BytesIO(binary), bucket, fnm)
                return r
            except Exception as e:
                logging.exception(f"S3 API put failed for {bucket}/{fnm}: {e}")

                # Try GCS native client as fallback
                if self.gcs_client:
                    try:
                        logging.info(f"Trying GCS native client for put {bucket}/{fnm}")
                        gcs_bucket = self.gcs_client.bucket(bucket)
                        blob = gcs_bucket.blob(fnm)
                        blob.upload_from_string(binary)
                        logging.info(
                            f"Successfully uploaded {bucket}/{fnm} via GCS native client"
                        )
                        return True
                    except Exception as gcs_e:
                        logging.exception(
                            f"GCS native put also failed for {bucket}/{fnm}: {gcs_e}"
                        )
                        # Both S3 and GCS failed, raise the original S3 error
                        self.__open__()
                        time.sleep(1)
                        raise e
                else:
                    # No GCS fallback available, raise the original S3 error
                    self.__open__()
                    time.sleep(1)
                    raise e

    @use_prefix_path
    @use_default_bucket
    def rm(self, bucket, fnm):
        try:
            self.conn.delete_object(Bucket=bucket, Key=fnm)
        except Exception as e:
            logging.exception(f"S3 API rm failed for {bucket}/{fnm}: {e}")

            # Try GCS native client as fallback
            if self.gcs_client:
                try:
                    logging.info(f"Trying GCS native client to delete {bucket}/{fnm}")
                    gcs_bucket = self.gcs_client.bucket(bucket)
                    blob = gcs_bucket.blob(fnm)
                    blob.delete()
                    logging.info(
                        f"Successfully deleted {bucket}/{fnm} via GCS native client"
                    )
                except Exception as gcs_e:
                    logging.exception(
                        f"GCS native rm also failed for {bucket}/{fnm}: {gcs_e}"
                    )
            else:
                logging.exception(f"Fail rm {bucket}/{fnm}")

    @use_prefix_path
    @use_default_bucket
    def get(self, bucket, fnm):
        for _ in range(1):
            try:
                r = self.conn.get_object(Bucket=bucket, Key=fnm)
                object_data = r["Body"].read()
                return object_data
            except Exception as e:
                logging.exception(f"S3 API get failed for {bucket}/{fnm}: {e}")

                # Try GCS native client as fallback
                if self.gcs_client:
                    try:
                        logging.info(f"Trying GCS native client for get {bucket}/{fnm}")
                        gcs_bucket = self.gcs_client.bucket(bucket)
                        blob = gcs_bucket.blob(fnm)
                        data = blob.download_as_bytes()
                        logging.info(
                            f"Successfully downloaded {bucket}/{fnm} via GCS native client"
                        )
                        return data
                    except Exception as gcs_e:
                        logging.exception(
                            f"GCS native get also failed for {bucket}/{fnm}: {gcs_e}"
                        )

                self.__open__()
                time.sleep(1)
                raise e
        return

    @use_prefix_path
    @use_default_bucket
    def obj_exist(self, bucket, fnm):
        try:
            if self.conn.head_object(Bucket=bucket, Key=fnm):
                return True
        except ClientError as e:
            if e.response["Error"]["Code"] == "404":
                return False
            else:
                # For other errors (like 403 Forbidden), try GCS fallback
                logging.exception(f"S3 API obj_exist failed for {bucket}/{fnm}: {e}")

                if self.gcs_client:
                    try:
                        logging.info(
                            f"Trying GCS native client to check object {bucket}/{fnm}"
                        )
                        gcs_bucket = self.gcs_client.bucket(bucket)
                        blob = gcs_bucket.blob(fnm)
                        exists = blob.exists()
                        logging.info(
                            f"Object {bucket}/{fnm} exists: {exists} (via GCS native client)"
                        )
                        return exists
                    except Exception as gcs_e:
                        logging.exception(
                            f"GCS native obj_exist also failed for {bucket}/{fnm}: {gcs_e}"
                        )
                        return False

                # If no GCS fallback, assume file doesn't exist for non-404 errors
                return False

    @use_prefix_path
    @use_default_bucket
    def get_presigned_url(self, bucket, fnm, expires):
        for _ in range(10):
            try:
                r = self.conn.generate_presigned_url(
                    "get_object",
                    Params={"Bucket": bucket, "Key": fnm},
                    ExpiresIn=expires,
                )
                return r
            except Exception:
                logging.exception(f"fail get url {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return
