import os
import time
from minio import Minio
from io import BytesIO
from rag import settings
from rag.settings import minio_logger
from rag.utils import singleton


@singleton
class HuMinio(object):
    def __init__(self):
        self.conn = None
        self.__open__()

    def __open__(self):
        try:
            if self.conn:
                self.__close__()
        except Exception as e:
            pass

        try:
            self.conn = Minio(settings.MINIO["host"],
                              access_key=settings.MINIO["user"],
                              secret_key=settings.MINIO["password"],
                              secure=False
                              )
        except Exception as e:
            minio_logger.error(
                "Fail to connect %s " % settings.MINIO["host"] + str(e))

    def __close__(self):
        del self.conn
        self.conn = None

    def put(self, bucket, fnm, binary):
        for _ in range(10):
            try:
                if not self.conn.bucket_exists(bucket):
                    self.conn.make_bucket(bucket)

                r = self.conn.put_object(bucket, fnm,
                                         BytesIO(binary),
                                         len(binary)
                                         )
                return r
            except Exception as e:
                minio_logger.error(f"Fail put {bucket}/{fnm}: " + str(e))
                self.__open__()
                time.sleep(1)

    def rm(self, bucket, fnm):
        try:
            self.conn.remove_object(bucket, fnm)
        except Exception as e:
            minio_logger.error(f"Fail rm {bucket}/{fnm}: " + str(e))


    def get(self, bucket, fnm):
        for _ in range(10):
            try:
                r = self.conn.get_object(bucket, fnm)
                return r.read()
            except Exception as e:
                minio_logger.error(f"fail get {bucket}/{fnm}: " + str(e))
                self.__open__()
                time.sleep(1)
        return

    def obj_exist(self, bucket, fnm):
        try:
            if self.conn.stat_object(bucket, fnm):return True
            return False
        except Exception as e:
            minio_logger.error(f"Fail put {bucket}/{fnm}: " + str(e))
        return False


    def get_presigned_url(self, bucket, fnm, expires):
        for _ in range(10):
            try:
                return self.conn.get_presigned_url("GET", bucket, fnm, expires)
            except Exception as e:
                minio_logger.error(f"fail get {bucket}/{fnm}: " + str(e))
                self.__open__()
                time.sleep(1)
        return

MINIO = HuMinio()

if __name__ == "__main__":
    conn = HuMinio()
    fnm = "/opt/home/kevinhu/docgpt/upload/13/11-408.jpg"
    from PIL import Image
    img = Image.open(fnm)
    buff = BytesIO()
    img.save(buff, format='JPEG')
    print(conn.put("test", "11-408.jpg", buff.getvalue()))
    bts = conn.get("test", "11-408.jpg")
    img = Image.open(BytesIO(bts))
    img.save("test.jpg")
