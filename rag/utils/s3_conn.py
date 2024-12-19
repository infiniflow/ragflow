import logging
import boto3
import os
from botocore.exceptions import ClientError
from botocore.client import Config
import time
from io import BytesIO
from rag import settings
from rag.settings import s3_logger
from rag.utils import singleton

@singleton
class RAGFlowS3(object):
    def __init__(self):
        self.conn = None
        self.import_bucket = settings.S3["import_bucket"]
        self.endpoint = settings.S3["endpoint"]
        self.access_key = settings.S3["access_key"]
        self.secret_key = settings.S3["secret_key"]
        self.region = settings.S3["region"]
        self.__open__()

    def __open__(self):
        try:
            if self.conn:
                self.__close__()
        except Exception:
            pass

        try:

            config = Config(
                s3={
                    'addressing_style': 'virtual'
                }
            )

            self.conn = boto3.client(
                's3',
                endpoint_url=self.endpoint,
                region_name=self.region,
                aws_access_key_id=self.access_key,
                aws_secret_access_key=self.secret_key,
                config=config
            )
        except Exception:
            logging.exception(
                "Fail to connect %s" % self.endpoint)

    def __close__(self):
        del self.conn
        self.conn = None

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
        bucket, fnm, binary = "txtxtxtxt1", "txtxtxtxt1", b"_t@@@1"

        if not self.bucket_exists(bucket):
            self.conn.create_bucket(Bucket=bucket)
            logging.debug(f"create bucket {bucket} ********")

        r = self.conn.upload_fileobj(BytesIO(binary), bucket, fnm)
        return r

    def get_properties(self, bucket, key):
        info = self.conn.stat_object(bucket_name=bucket, object_name=key)
        return {"name": info.key, "size": info.size, "etag": info.etag, "owner": info.owner}

    def list(self, bucket, dir, recursive=True):
        bucket = bucket if bucket else self.import_bucket
        if dir != "/":
            keys = self.conn.list_objects(bucket_name=bucket, prefix=dir, recursive=recursive)
        else:
            keys = self.conn.list_objects(bucket_name=bucket, recursive=recursive)
        data = [{"name": key.key, "size": key.size, "etag": key.etag, "owner": key.owner} for key in keys]
        return data

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

    def rm(self, bucket, fnm):
        try:
            self.conn.delete_object(Bucket=bucket, Key=fnm)
        except Exception:
            logging.exception(f"Fail rm {bucket}/{fnm}")

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

    def obj_exist(self, bucket, fnm):
        try:

            if self.conn.head_object(Bucket=bucket, Key=fnm):
                return True
        except ClientError as e:
            if e.response['Error']['Code'] == '404':

                return False
            else:
                raise

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
