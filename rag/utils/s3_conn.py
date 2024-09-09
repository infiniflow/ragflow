import boto3
import os
from botocore.exceptions import ClientError
from botocore.client import Config
import time
from io import BytesIO
from rag.settings import s3_logger
from rag.utils import singleton

@singleton
class RAGFlowS3(object):
    def __init__(self):
        self.conn = None
        self.endpoint = os.getenv('ENDPOINT', None)
        self.access_key = os.getenv('ACCESS_KEY', None)
        self.secret_key = os.getenv('SECRET_KEY', None)
        self.region = os.getenv('REGION', None)
        self.__open__()

    def __open__(self):
        try:
            if self.conn:
                self.__close__()
        except Exception as e:
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
        except Exception as e:
            s3_logger.error(
                "Fail to connect %s " % self.endpoint + str(e))

    def __close__(self):
        del self.conn
        self.conn = None

    def bucket_exists(self, bucket):
        try:
            s3_logger.error(f"head_bucket bucketname  {bucket}")
            self.conn.head_bucket(Bucket=bucket)
            exists = True
        except ClientError as e:
            s3_logger.error(f"head_bucket error {bucket}: " + str(e))
            exists = False
        return exists

    def health(self):
        bucket, fnm, binary = "txtxtxtxt1", "txtxtxtxt1", b"_t@@@1"

        if not self.bucket_exists(bucket):
            self.conn.create_bucket(Bucket=bucket)
            s3_logger.error(f"create bucket {bucket} ********")

        r = self.conn.upload_fileobj(BytesIO(binary), bucket, fnm)
        return r

    def get_properties(self, bucket, key):
        return {}

    def list(self, bucket, dir, recursive=True):
        return []

    def put(self, bucket, fnm, binary):
        s3_logger.error(f"bucket name {bucket}; filename :{fnm}:")
        for _ in range(1):
            try:
                if not self.bucket_exists(bucket):
                    self.conn.create_bucket(Bucket=bucket)
                    s3_logger.error(f"create bucket {bucket} ********")
                r = self.conn.upload_fileobj(BytesIO(binary), bucket, fnm)

                return r
            except Exception as e:
                s3_logger.error(f"Fail put {bucket}/{fnm}: " + str(e))
                self.__open__()
                time.sleep(1)

    def rm(self, bucket, fnm):
        try:
            self.conn.delete_object(Bucket=bucket, Key=fnm)
        except Exception as e:
            s3_logger.error(f"Fail rm {bucket}/{fnm}: " + str(e))

    def get(self, bucket, fnm):
        for _ in range(1):
            try:
                r = self.conn.get_object(Bucket=bucket, Key=fnm)
                object_data = r['Body'].read()
                return object_data
            except Exception as e:
                s3_logger.error(f"fail get {bucket}/{fnm}: " + str(e))
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
            except Exception as e:
                s3_logger.error(f"fail get url {bucket}/{fnm}: " + str(e))
                self.__open__()
                time.sleep(1)
        return
