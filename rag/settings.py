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
import logging
from api.utils import get_base_config, decrypt_database_config
from api.utils.file_utils import get_project_base_directory

# Server
RAG_CONF_PATH = os.path.join(get_project_base_directory(), "conf")

# Get storage type and document engine from system environment variables
STORAGE_IMPL_TYPE = os.getenv('STORAGE_IMPL', 'MINIO')
DOC_ENGINE = os.getenv('DOC_ENGINE', 'elasticsearch')

ES = {}
INFINITY = {}
AZURE = {}
S3 = {}
MINIO = {}
OSS = {}

# Initialize the selected configuration data based on environment variables to solve the problem of initialization errors due to lack of configuration
if DOC_ENGINE == 'elasticsearch':
    ES = get_base_config("es", {})
elif DOC_ENGINE == 'infinity':
    INFINITY = get_base_config("infinity", {"uri": "infinity:23817"})

if STORAGE_IMPL_TYPE in ['AZURE_SPN', 'AZURE_SAS']:
    AZURE = get_base_config("azure", {})
elif STORAGE_IMPL_TYPE == 'AWS_S3':
    S3 = get_base_config("s3", {})
elif STORAGE_IMPL_TYPE == 'MINIO':
    MINIO = decrypt_database_config(name="minio")
elif STORAGE_IMPL_TYPE == 'OSS':
    OSS = get_base_config("oss", {})

try:
    REDIS = decrypt_database_config(name="redis")
except Exception:
    REDIS = {}
    pass
DOC_MAXIMUM_SIZE = int(os.environ.get("MAX_CONTENT_LENGTH", 128 * 1024 * 1024))

SVR_QUEUE_NAME = "rag_flow_svr_queue"
SVR_CONSUMER_GROUP_NAME = "rag_flow_svr_task_broker"
PAGERANK_FLD = "pagerank_fea"
TAG_FLD = "tag_feas"

PARALLEL_DEVICES = None
try:
    import torch.cuda
    PARALLEL_DEVICES = torch.cuda.device_count()
    logging.info(f"found {PARALLEL_DEVICES} gpus")
except Exception:
    logging.info("can't import package 'torch'")

def print_rag_settings():
    logging.info(f"MAX_CONTENT_LENGTH: {DOC_MAXIMUM_SIZE}")
    logging.info(f"MAX_FILE_COUNT_PER_USER: {int(os.environ.get('MAX_FILE_NUM_PER_USER', 0))}")


def get_svr_queue_name(priority: int) -> str:
    if priority == 0:
        return SVR_QUEUE_NAME
    return f"{SVR_QUEUE_NAME}_{priority}"

def get_svr_queue_names():
    return [get_svr_queue_name(priority) for priority in [1, 0]]
