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
from logging.handlers import RotatingFileHandler

from api.utils.file_utils import get_project_base_directory

LOG_LEVEL = logging.INFO
LOG_FILE = os.path.abspath(os.path.join(get_project_base_directory(), "logs", f"ragflow_{os.getpid()}.log"))
LOG_FORMAT = "%(asctime)-15s %(levelname)-8s %(process)d %(message)s"
logger = None

def getLogger():
    global logger
    if logger is not None:
        return logger

    print(f"log file path: {LOG_FILE}")
    os.makedirs(os.path.dirname(LOG_FILE), exist_ok=True)
    logger = logging.getLogger("ragflow")
    logger.setLevel(LOG_LEVEL)

    handler1 = RotatingFileHandler(LOG_FILE, maxBytes=10*1024*1024, backupCount=5)
    handler1.setLevel(LOG_LEVEL)
    formatter1 = logging.Formatter(LOG_FORMAT)
    handler1.setFormatter(formatter1)
    logger.addHandler(handler1)

    handler2 = logging.StreamHandler()
    handler2.setLevel(LOG_LEVEL)
    formatter2 = logging.Formatter(LOG_FORMAT)
    handler2.setFormatter(formatter2)
    logger.addHandler(handler2)

    return logger

logger = getLogger()
