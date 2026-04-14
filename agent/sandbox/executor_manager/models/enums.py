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
from enum import Enum


class SupportLanguage(str, Enum):
    PYTHON = "python"
    NODEJS = "nodejs"


class ResultStatus(str, Enum):
    SUCCESS = "success"
    PROGRAM_ERROR = "program_error"
    RESOURCE_LIMIT_EXCEEDED = "resource_limit_exceeded"
    UNAUTHORIZED_ACCESS = "unauthorized_access"
    RUNTIME_ERROR = "runtime_error"
    PROGRAM_RUNNER_ERROR = "program_runner_error"


class ResourceLimitType(str, Enum):
    TIME = "time"
    MEMORY = "memory"
    OUTPUT = "output"


class UnauthorizedAccessType(str, Enum):
    DISALLOWED_SYSCALL = "disallowed_syscall"
    FILE_ACCESS = "file_access"
    NETWORK_ACCESS = "network_access"


class RuntimeErrorType(str, Enum):
    SIGNALLED = "signalled"
    NONZERO_EXIT = "nonzero_exit"
