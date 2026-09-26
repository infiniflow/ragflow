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
from urllib.parse import urlparse

from elasticsearch import Elasticsearch

from common import settings
from common.decorator import singleton

MAX_RETRIES = 6
ATTEMPT_TIME = MAX_RETRIES
HEALTH_CHECK_BASE_DELAY_SECONDS = 5


@singleton
class ElasticSearchConnectionPool:
    def __init__(self):
        if hasattr(settings, "ES"):
            self.ES_CONFIG = settings.ES
        else:
            self.ES_CONFIG = settings.get_base_config("es", {})

        # Fail fast on a misconfigured scheme instead of retrying a config error.
        if self.ES_CONFIG.get("api_key"):
            self._require_https_hosts()

        for attempt in range(MAX_RETRIES):
            try:
                if self._connect():
                    break
            except Exception as e:
                logging.warning(
                    "Elasticsearch %s connection attempt %d/%d failed: %s",
                    self.ES_CONFIG["hosts"],
                    attempt + 1,
                    MAX_RETRIES,
                    e,
                )
                if attempt == MAX_RETRIES - 1:
                    raise
                if hasattr(self, "es_conn") and self.es_conn:
                    try:
                        self.es_conn.close()
                    except Exception:
                        logging.exception(
                            "Failed to close partially-initialized Elasticsearch client before retry.",
                        )
                time.sleep(HEALTH_CHECK_BASE_DELAY_SECONDS * (2**attempt))
                continue

        if not hasattr(self, "es_conn") or not self.es_conn or not self.es_conn.ping():
            msg = f"Elasticsearch {self.ES_CONFIG['hosts']} is unhealthy after {MAX_RETRIES} attempts."
            logging.error(msg)
            raise Exception(msg)
        v = self.info.get("version", {"number": "8.11.3"})
        v = v["number"].split(".")[0]
        if int(v) < 8:
            msg = f"Elasticsearch version must be greater than or equal to 8, current version: {v}"
            logging.error(msg)
            raise Exception(msg)

    def _connect(self):
        if self.ES_CONFIG.get("api_key"):
            self._require_https_hosts()
            logging.info("Connecting to Elasticsearch using API key authentication")
            self.es_conn = Elasticsearch(
                self.ES_CONFIG["hosts"].split(","),
                api_key=self.ES_CONFIG["api_key"],
                # API keys must never travel over an unverified or cleartext connection.
                verify_certs=self.ES_CONFIG.get("verify_certs", True),
                timeout=600,
            )
        else:
            logging.info("Connecting to Elasticsearch using basic authentication")
            self.es_conn = Elasticsearch(
                self.ES_CONFIG["hosts"].split(","),
                basic_auth=(self.ES_CONFIG["username"], self.ES_CONFIG["password"]) if "username" in self.ES_CONFIG and "password" in self.ES_CONFIG else None,
                verify_certs=self.ES_CONFIG.get("verify_certs", False),
                timeout=600,
            )
        if self.es_conn:
            self.info = self.es_conn.info()
            return True
        return False

    def _require_https_hosts(self):
        hosts = [h.strip() for h in self.ES_CONFIG["hosts"].split(",")]
        insecure = [h for h in hosts if urlparse(h).scheme != "https"]
        if insecure:
            raise ValueError(f"Elasticsearch API key authentication requires HTTPS hosts, got non-HTTPS host(s): {insecure}. Set ES_HOST_URL to an https:// endpoint when ELASTIC_API_KEY is configured.")

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
