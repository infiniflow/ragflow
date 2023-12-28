import logging
import time
from util import config
from minio import Minio
from io import BytesIO


class HuMinio(object):
    def __init__(self, env):
        self.config = config.init(env)
        self.conn = None
        self.__open__()

    def __open__(self):
        try:
            if self.conn:
                self.__close__()
        except Exception as e:
            pass

        try:
            self.conn = Minio(self.config.get("minio_host"),
                              access_key=self.config.get("minio_user"),
                              secret_key=self.config.get("minio_password"),
                              secure=False
                              )
        except Exception as e:
            logging.error(
                "Fail to connect %s " %
                self.config.get("minio_host") + str(e))

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
                logging.error(f"Fail put {bucket}/{fnm}: " + str(e))
                self.__open__()
                time.sleep(1)

    def get(self, bucket, fnm):
        for _ in range(10):
            try:
                r = self.conn.get_object(bucket, fnm)
                return r.read()
            except Exception as e:
                logging.error(f"fail get {bucket}/{fnm}: " + str(e))
                self.__open__()
                time.sleep(1)
        return

    def get_presigned_url(self, bucket, fnm, expires):
        for _ in range(10):
            try:
                return self.conn.get_presigned_url("GET", bucket, fnm, expires)
            except Exception as e:
                logging.error(f"fail get {bucket}/{fnm}: " + str(e))
                self.__open__()
                time.sleep(1)
        return


if __name__ == "__main__":
    conn = HuMinio("infiniflow")
    fnm = "/opt/home/kevinhu/docgpt/upload/13/11-408.jpg"
    from PIL import Image
    img = Image.open(fnm)
    buff = BytesIO()
    img.save(buff, format='JPEG')
    print(conn.put("test", "11-408.jpg", buff.getvalue()))
    bts = conn.get("test", "11-408.jpg")
    img = Image.open(BytesIO(bts))
    img.save("test.jpg")
