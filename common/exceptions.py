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

from api.exceptions import RAGFlowError, NotFoundError, ValidationError, RetryableError  # noqa: F401 — re-exported for callers


class TaskCanceledException(Exception):
    def __init__(self, msg):
        self.msg = msg


class ArgumentException(ValidationError):
    def __init__(self, msg):
        super().__init__(msg)
        self.msg = msg


class NotFoundException(NotFoundError):
    def __init__(self, msg):
        super().__init__(msg)
        self.msg = msg


class ModelException(RAGFlowError):
    http_status = 503

    def __init__(self, msg, retryable=False):
        super().__init__(msg)
        self.msg = msg
        self.retryable = retryable