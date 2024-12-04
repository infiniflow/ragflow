import logging
import time
from minio import Minio
from minio.error import S3Error
from io import BytesIO
from rag import settings
from rag.utils import singleton


@singleton
class RAGFlowMinio(object):
    def __init__(self):
        self.conn = None
        self.__open__()

    def __open__(self):
        try:
            if self.conn:
                self.__close__()
        except Exception:
            pass

        try:
            self.conn = Minio(settings.MINIO["host"],
                              access_key=settings.MINIO["user"],
                              secret_key=settings.MINIO["password"],
                              secure=False
                              )
            self.import_bucket = settings.MINIO["import_bucket"]
        except Exception:
            logging.exception(
                "Fail to connect %s " % settings.MINIO["host"])

        except Exception as e:
            minio_logger.error(
                "Fail to connect %s " % settings.MINIO["host"] + str(e))

    def __close__(self):
        del self.conn
        self.conn = None

    def health(self):
        bucket, fnm, binary = "txtxtxtxt1", "txtxtxtxt1", b"_t@@@1"
        if not self.conn.bucket_exists(bucket):
            self.conn.make_bucket(bucket)
        r = self.conn.put_object(bucket, fnm,
                                 BytesIO(binary),
                                 len(binary)
                                 )
        return r

    def get_properties(self, bucket, key):
        info = self.conn.stat_object(bucket_name=bucket, object_name=key)
        return {"name": info.object_name, "size": info.size, "etag": info.etag, "owner": info.owner_name}

    def list(self, bucket, dir, recursive=True):
        bucket = bucket if bucket else self.import_bucket
        if dir != "/":
            keys = self.conn.list_objects(bucket_name=bucket, prefix=dir, recursive=recursive)
        else:
            keys = self.conn.list_objects(bucket_name=bucket, recursive=recursive)
        data = [{"name": key.object_name, "size": key.size, "etag": key.etag, "owner": key.owner_name} for key in keys]
        return data

    def put(self, bucket, fnm, binary):
        for _ in range(3):
            try:
                if not self.conn.bucket_exists(bucket):
                    self.conn.make_bucket(bucket)

                r = self.conn.put_object(bucket, fnm,
                                         BytesIO(binary),
                                         len(binary)
                                         )
                return r
            except Exception:
                logging.exception(f"Fail to put {bucket}/{fnm}:")
                self.__open__()
                time.sleep(1)

    def rm(self, bucket, fnm):
        try:
            self.conn.remove_object(bucket, fnm)
        except Exception:
            logging.exception(f"Fail to remove {bucket}/{fnm}:")

    def get(self, bucket, filename):
        for _ in range(1):
            try:
                r = self.conn.get_object(bucket, filename)
                return r.read()
            except Exception:
                logging.exception(f"Fail to get {bucket}/{filename}")
                self.__open__()
                time.sleep(1)
        return

    def obj_exist(self, bucket, filename):
        try:
            if not self.conn.bucket_exists(bucket):
                return False
            if self.conn.stat_object(bucket, filename):
                return True
            else:
                return False
        except S3Error as e:
            if e.code in ["NoSuchKey", "NoSuchBucket", "ResourceNotFound"]:
                return False
        except Exception:
            logging.exception(f"obj_exist {bucket}/{filename} got exception")
            return False

    def get_presigned_url(self, bucket, fnm, expires):
        for _ in range(10):
            try:
                return self.conn.get_presigned_url("GET", bucket, fnm, expires)
            except Exception:
                logging.exception(f"Fail to get_presigned {bucket}/{fnm}:")
                self.__open__()
                time.sleep(1)
        return


MINIO = RAGFlowMinio()

if __name__ == "__main__":
    conn = RAGFlowMinio()
    fnm = "/opt/home/kevinhu/docgpt/upload/13/11-408.jpg"
    from PIL import Image

    img = Image.open(fnm)
    buff = BytesIO()
    img.save(buff, format='JPEG')
    print(conn.put("test", "11-408.jpg", buff.getvalue()))
    bts = conn.get("test", "11-408.jpg")
    img = Image.open(BytesIO(bts))
    img.save("test.jpg")
