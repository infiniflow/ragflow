#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from core.config import app_config

def print_rag_settings():
    logging.info(f"MAX_CONTENT_LENGTH: {app_config.rag.doc_maximum_size}")
    logging.info(f"MAX_FILE_COUNT_PER_USER: {app_config.rag.max_file_num_per_user}")
