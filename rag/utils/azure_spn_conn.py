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
import os
import time
from common.decorator import singleton
from azure.identity import ClientSecretCredential, AzureAuthorityHosts
from azure.storage.filedatalake import FileSystemClient
from common import settings

_CLOUD_AUTHORITY_MAP = {
    "public": AzureAuthorityHosts.AZURE_PUBLIC_CLOUD,
    "china": AzureAuthorityHosts.AZURE_CHINA,
    "government": AzureAuthorityHosts.AZURE_GOVERNMENT,
    "germany": AzureAuthorityHosts.AZURE_GERMANY,
}


@singleton
class RAGFlowAzureSpnBlob:
    def __init__(self):
        self.conn = None
        self.account_url = os.getenv('ACCOUNT_URL', settings.AZURE["account_url"])
        self.client_id = os.getenv('CLIENT_ID', settings.AZURE["client_id"])
        self.secret = os.getenv('SECRET', settings.AZURE["secret"])
        self.tenant_id = os.getenv('TENANT_ID', settings.AZURE["tenant_id"])
        self.container_name = os.getenv('CONTAINER_NAME', settings.AZURE["container_name"])
        self.cloud = os.getenv('AZURE_CLOUD', settings.AZURE.get("cloud", "public")).lower()
        self.__open__()

    def __open__(self):
        try:
            if self.conn:
                self.__close__()
        except Exception:
            pass

        try:
            authority = _CLOUD_AUTHORITY_MAP.get(self.cloud, AzureAuthorityHosts.AZURE_PUBLIC_CLOUD)
            credentials = ClientSecretCredential(tenant_id=self.tenant_id, client_id=self.client_id,
                                                 client_secret=self.secret, authority=authority)
            self.conn = FileSystemClient(account_url=self.account_url, file_system_name=self.container_name,
                                         credential=credentials)
        except Exception:
            logging.exception("Fail to connect %s" % self.account_url)

    def __close__(self):
        del self.conn
        self.conn = None

    def health(self):
        _bucket, fnm, binary = "txtxtxtxt1", "txtxtxtxt1", b"_t@@@1"
        f = self.conn.create_file(f"{_bucket}/{fnm}")
        f.append_data(binary, offset=0, length=len(binary))
        return f.flush_data(len(binary))

    def put(self, bucket, fnm, binary, tenant_id=None):
        for _ in range(3):
            try:
                f = self.conn.create_file(f"{bucket}/{fnm}")
                f.append_data(binary, offset=0, length=len(binary))
                return f.flush_data(len(binary))
            except Exception:
                logging.exception(f"Fail put {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
                return None
        return None

    def rm(self, bucket, fnm):
        try:
            self.conn.delete_file(f"{bucket}/{fnm}")
        except Exception:
            logging.exception(f"Fail rm {bucket}/{fnm}")

    def get(self, bucket, fnm):
        for _ in range(1):
            try:
                client = self.conn.get_file_client(f"{bucket}/{fnm}")
                r = client.download_file()
                return r.read()
            except Exception:
                logging.exception(f"fail get {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return None

    def obj_exist(self, bucket, fnm):
        try:
            client = self.conn.get_file_client(f"{bucket}/{fnm}")
            return client.exists()
        except Exception:
            logging.exception(f"Fail put {bucket}/{fnm}")
        return False

    def get_presigned_url(self, bucket, fnm, expires):
        for _ in range(10):
            try:
                client = self.conn.get_file_client(f"{bucket}/{fnm}")
                return client.url
            except Exception:
                logging.exception(f"fail get {bucket}/{fnm}")
                self.__open__()
                time.sleep(1)
        return None
