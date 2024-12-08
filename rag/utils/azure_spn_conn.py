import logging
import os
import time
from rag import settings
from rag.utils import singleton
from azure.identity import ClientSecretCredential, AzureAuthorityHosts
from azure.storage.filedatalake import FileSystemClient


@singleton
class RAGFlowAzureSpnBlob(object):
    def __init__(self):
        self.conn = None
        self.account_url = os.getenv('ACCOUNT_URL', settings.AZURE["account_url"])
        self.client_id = os.getenv('CLIENT_ID', settings.AZURE["client_id"])
        self.secret = os.getenv('SECRET', settings.AZURE["secret"])
        self.tenant_id = os.getenv('TENANT_ID', settings.AZURE["tenant_id"])
        self.container_name = os.getenv('CONTAINER_NAME', settings.AZURE["container_name"])
        self.__open__()

    def __open__(self):
        try:
            if self.conn:
                self.__close__()
        except Exception:
            pass

        try:
            credentials = ClientSecretCredential(tenant_id=self.tenant_id, client_id=self.client_id, client_secret=self.secret, authority=AzureAuthorityHosts.AZURE_CHINA)
            self.conn = FileSystemClient(account_url=self.account_url, file_system_name=self.container_name, credential=credentials)
        except Exception:
            logging.exception("Fail to connect %s" % self.account_url)

    def __close__(self):
        del self.conn
        self.conn = None

    def health(self):
        _bucket, fnm, binary = "txtxtxtxt1", "txtxtxtxt1", b"_t@@@1"
        f = self.conn.create_file(fnm)
        f.append_data(binary, offset=0, length=len(binary))
        return f.flush_data(len(binary))

    def put(self, bucket, fnm, binary):
        for _ in range(3):
            try:
                f = self.conn.create_file(fnm)
                f.append_data(binary, offset=0, length=len(binary))
                return f.flush_data(len(binary))
            except Exception:
                logging.exception(f"Fail put {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)

    def rm(self, bucket, fnm):
        try:
            self.conn.delete_file(fnm)
        except Exception:
            logging.exception(f"Fail rm {bucket}/{fnm}")

    def get(self, bucket, fnm):
        for _ in range(1):
            try:
                client = self.conn.get_file_client(fnm)
                r = client.download_file()
                return r.read()
            except Exception:
                logging.exception(f"fail get {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return

    def obj_exist(self, bucket, fnm):
        try:
            client = self.conn.get_file_client(fnm)
            return client.exists()
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