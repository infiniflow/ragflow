import logging
import os
import time
from io import BytesIO
from rag import settings
from rag.utils import singleton
from azure.storage.blob import ContainerClient


@singleton
class RAGFlowAzureSasBlob(object):
    def __init__(self):
        self.conn = None
        self.container_url = os.getenv('CONTAINER_URL', settings.AZURE["container_url"])
        self.sas_token = os.getenv('SAS_TOKEN', settings.AZURE["sas_token"])
        self.__open__()

    def __open__(self):
        try:
            if self.conn:
                self.__close__()
        except Exception:
            pass

        try:
            self.conn = ContainerClient.from_container_url(self.container_url + "?" + self.sas_token)
        except Exception:
            logging.exception("Fail to connect %s " % self.container_url)

    def __close__(self):
        del self.conn
        self.conn = None

    def health(self):
        _bucket, fnm, binary = "txtxtxtxt1", "txtxtxtxt1", b"_t@@@1"
        return self.conn.upload_blob(name=fnm, data=BytesIO(binary), length=len(binary))

    def put(self, bucket, fnm, binary):
        for _ in range(3):
            try:
                return self.conn.upload_blob(name=fnm, data=BytesIO(binary), length=len(binary))
            except Exception:
                logging.exception(f"Fail put {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)

    def rm(self, bucket, fnm):
        try:
            self.conn.delete_blob(fnm)
        except Exception:
            logging.exception(f"Fail rm {bucket}/{fnm}")

    def get(self, bucket, fnm):
        for _ in range(1):
            try:
                r = self.conn.download_blob(fnm)
                return r.read()
            except Exception:
                logging.exception(f"fail get {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return

    def obj_exist(self, bucket, fnm):
        try:
            return self.conn.get_blob_client(fnm).exists()
        except Exception:
            logging.exception(f"Fail put {bucket}/{fnm}")
        return False

    def get_presigned_url(self, bucket, fnm, expires):
        for _ in range(10):
            try:
                return self.conn.get_presigned_url("GET", bucket, fnm, expires)
            except Exception:
                logging.exception(f"fail get {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return
