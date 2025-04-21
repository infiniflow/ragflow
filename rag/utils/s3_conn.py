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
        self.s3_config = settings.S3
        self.access_key = self.s3_config.get('access_key', None)
        self.secret_key = self.s3_config.get('secret_key', None)
        self.region = self.s3_config.get('region', None)
        self.endpoint_url = self.s3_config.get('endpoint_url', None)
        self.signature_version = self.s3_config.get('signature_version', None)
        self.addressing_style = self.s3_config.get('addressing_style', None)
        self.bucket = self.s3_config.get('bucket', None)
        self.prefix_path = self.s3_config.get('prefix_path', None)
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
            s3_params = {
                'aws_access_key_id': self.access_key,
                'aws_secret_access_key': self.secret_key,
            }
            if self.region in self.s3_config:
                s3_params['region_name'] = self.region
            if 'endpoint_url' in self.s3_config:
                s3_params['endpoint_url'] = self.endpoint_url
            if 'signature_version' in self.s3_config:
                s3_params['config'] = Config(s3={"signature_version": self.signature_version})
            if 'addressing_style' in self.s3_config:
                s3_params['config'] = Config(s3={"addressing_style": self.addressing_style})
            self.conn = boto3.client('s3', **s3_params)
        except Exception:
            logging.exception(f"Fail to connect at region {self.region} or endpoint {self.endpoint_url}")

    def __close__(self):
        del self.conn
        self.conn = None

    @use_default_bucket
    def bucket_exists(self, bucket):
        try:
            logging.debug(f"head_bucket bucketname {bucket}")
            self.conn.head_bucket(Bucket=bucket)
            exists = True
        except ClientError:
            logging.exception(f"head_bucket error {bucket}")
            exists = False
        return exists

    def health(self):
        bucket = self.bucket
        fnm = "txtxtxtxt1"
        fnm, binary = f"{self.prefix_path}/{fnm}" if self.prefix_path else fnm, b"_t@@@1"
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
            except Exception:
                logging.exception(f"Fail put {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)

    @use_prefix_path
    @use_default_bucket
    def rm(self, bucket, fnm):
        try:
            self.conn.delete_object(Bucket=bucket, Key=fnm)
        except Exception:
            logging.exception(f"Fail rm {bucket}/{fnm}")

    @use_prefix_path
    @use_default_bucket
    def get(self, bucket, fnm):
        for _ in range(1):
            try:
                r = self.conn.get_object(Bucket=bucket, Key=fnm)
                object_data = r['Body'].read()
                return object_data
            except Exception:
                logging.exception(f"fail get {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return

    @use_prefix_path
    @use_default_bucket
    def obj_exist(self, bucket, fnm):
        try:
            if self.conn.head_object(Bucket=bucket, Key=fnm):
                return True
        except ClientError as e:
            if e.response['Error']['Code'] == '404':
                return False
            else:
                raise

    @use_prefix_path
    @use_default_bucket
    def get_presigned_url(self, bucket, fnm, expires):
        for _ in range(10):
            try:
                r = self.conn.generate_presigned_url('get_object',
                                                     Params={'Bucket': bucket,
                                                             'Key': fnm},
                                                     ExpiresIn=expires)

                return r
            except Exception:
                logging.exception(f"fail get url {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return
