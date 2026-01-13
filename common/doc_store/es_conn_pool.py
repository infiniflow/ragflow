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
import time
from elasticsearch import Elasticsearch

from common.decorator import singleton
from core.config import app_config

ATTEMPT_TIME = 2


@singleton
class ElasticSearchConnectionPool:

    def __init__(self):
        self.es_cfg = app_config.doc_engine.es

        for es_host in self.es_cfg.hosts:
            for _ in range(ATTEMPT_TIME):
                try:
                    if self._connect():
                        break
                except Exception as e:
                    logging.warning(f"{str(e)}. Waiting Elasticsearch {es_host} to be healthy.")
                    time.sleep(5)

        if not hasattr(self, "es_conn") or not self.es_conn or not self.es_conn.ping():
            msg = f"Elasticsearch {self.es_cfg.hosts} is unhealthy in 10s."
            logging.error(msg)
            raise Exception(msg)
        v = self.info.get("version", {"number": "8.11.3"})
        v = v["number"].split(".")[0]
        if int(v) < 8:
            msg = f"Elasticsearch version must be greater than or equal to 8, current version: {v}"
            logging.error(msg)
            raise Exception(msg)

    def _connect(self):
        self.es_conn = Elasticsearch(
            self.es_cfg.hosts,
            basic_auth=(self.es_cfg.username, self.es_cfg.password) if self.es_cfg.username and self.es_cfg.password else None,
            verify_certs= self.es_cfg.verify_certs,
            timeout=600 )
        if self.es_conn:
            self.info = self.es_conn.info()
            return True
        return False

    def get_conn(self):
        return self.es_conn

    def refresh_conn(self):
        if self.es_conn.ping():
            return self.es_conn
        else:
            # close current if exist
            if self.es_conn:
                self.es_conn.close()
            self._connect()
            return self.es_conn

    def __del__(self):
        if hasattr(self, "es_conn") and self.es_conn:
            self.es_conn.close()


ES_CONN = ElasticSearchConnectionPool()
