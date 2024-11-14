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
import os.path
import logging
from logging.handlers import RotatingFileHandler

def get_project_base_directory():
    PROJECT_BASE = os.path.abspath(
        os.path.join(
            os.path.dirname(os.path.realpath(__file__)),
            os.pardir,
            os.pardir,
        )
    )
    return PROJECT_BASE

def initRootLogger(script_path: str, log_level: int = logging.INFO, log_format: str = "%(asctime)-15s %(levelname)-8s %(process)d %(message)s"):
    logger = logging.getLogger()
    if logger.hasHandlers():
        return

    script_name = os.path.basename(script_path)
    log_path = os.path.abspath(os.path.join(get_project_base_directory(), "logs", f"{os.path.splitext(script_name)[0]}.log"))

    os.makedirs(os.path.dirname(log_path), exist_ok=True)
    logger.setLevel(log_level)
    formatter = logging.Formatter(log_format)

    handler1 = RotatingFileHandler(log_path, maxBytes=10*1024*1024, backupCount=5)
    handler1.setLevel(log_level)
    handler1.setFormatter(formatter)
    logger.addHandler(handler1)

    handler2 = logging.StreamHandler()
    handler2.setLevel(log_level)
    handler2.setFormatter(formatter)
    logger.addHandler(handler2)

    msg = f"{script_name} log path: {log_path}"
    logger.info(msg)