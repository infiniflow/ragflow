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
import os
from common.config_utils import get_base_config, decrypt_database_config

EMBEDDING_MDL = ""

EMBEDDING_CFG = ""

DOC_ENGINE = os.getenv('DOC_ENGINE', 'elasticsearch')

docStoreConn = None

retriever = None

# move from rag.settings
ES = {}
INFINITY = {}
AZURE = {}
S3 = {}
MINIO = {}
OSS = {}
OS = {}
REDIS = {}

STORAGE_IMPL_TYPE = os.getenv('STORAGE_IMPL', 'MINIO')

# Initialize the selected configuration data based on environment variables to solve the problem of initialization errors due to lack of configuration
if DOC_ENGINE == 'elasticsearch':
    ES = get_base_config("es", {})
elif DOC_ENGINE == 'opensearch':
    OS = get_base_config("os", {})
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
    try:
        REDIS = get_base_config("redis", {})
    except Exception:
        REDIS = {}