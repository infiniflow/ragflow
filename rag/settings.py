#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import os
from api.utils import get_base_config, decrypt_database_config
from api.utils.file_utils import get_project_base_directory
from api.utils.log_utils import LoggerFactory, getLogger


# Server
RAG_CONF_PATH = os.path.join(get_project_base_directory(), "conf")
SUBPROCESS_STD_LOG_NAME = "std.log"

ES = get_base_config("es", {})
MINIO = decrypt_database_config(name="minio")
try:
    REDIS = decrypt_database_config(name="redis")
except Exception as e:
    REDIS = {}
    pass
DOC_MAXIMUM_SIZE = 128 * 1024 * 1024

# Logger
LoggerFactory.set_directory(
    os.path.join(
        get_project_base_directory(),
        "logs",
        "rag"))
# {CRITICAL: 50, FATAL:50, ERROR:40, WARNING:30, WARN:30, INFO:20, DEBUG:10, NOTSET:0}
LoggerFactory.LEVEL = 30

es_logger = getLogger("es")
minio_logger = getLogger("minio")
cron_logger = getLogger("cron_logger")
cron_logger.setLevel(20)
chunk_logger = getLogger("chunk_logger")
database_logger = getLogger("database")

SVR_QUEUE_NAME = "rag_flow_svr_queue"
SVR_QUEUE_RETENTION = 60*60
SVR_QUEUE_MAX_LEN = 1024
SVR_CONSUMER_NAME = "rag_flow_svr_consumer"
SVR_CONSUMER_GROUP_NAME = "rag_flow_svr_consumer_group"
