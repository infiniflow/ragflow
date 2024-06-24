#
#  Copyright 2019 The FATE Authors. All Rights Reserved.
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
# Logger
import os

from api.utils.file_utils import get_project_base_directory
from api.utils.log_utils import LoggerFactory, getLogger

DEBUG = 0
LoggerFactory.set_directory(
    os.path.join(
        get_project_base_directory(),
        "logs",
        "flow"))
# {CRITICAL: 50, FATAL:50, ERROR:40, WARNING:30, WARN:30, INFO:20, DEBUG:10, NOTSET:0}
LoggerFactory.LEVEL = 30

flow_logger = getLogger("flow")
database_logger = getLogger("database")
FLOAT_ZERO = 1e-8
PARAM_MAXDEPTH = 5
