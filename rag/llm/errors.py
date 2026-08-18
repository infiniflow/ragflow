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
import re
from enum import StrEnum


class ModelErrorCode(StrEnum):
    ERROR_RATE_LIMIT = "RATE_LIMIT_EXCEEDED"
    ERROR_AUTHENTICATION = "AUTH_ERROR"
    ERROR_INVALID_REQUEST = "INVALID_REQUEST"
    ERROR_SERVER = "SERVER_ERROR"
    ERROR_TIMEOUT = "TIMEOUT"
    ERROR_CONNECTION = "CONNECTION_ERROR"
    ERROR_MODEL = "MODEL_ERROR"
    ERROR_MAX_ROUNDS = "ERROR_MAX_ROUNDS"
    ERROR_CONTENT_FILTER = "CONTENT_FILTERED"
    ERROR_QUOTA = "QUOTA_EXCEEDED"
    ERROR_MAX_RETRIES = "MAX_RETRIES_EXCEEDED"
    ERROR_GENERIC = "GENERIC_ERROR"


def classify_model_error(error) -> ModelErrorCode:
    error_str = str(error).lower()
    keywords_mapping = [
        (["quota", "capacity", "credit", "billing", "balance", "欠费"], ModelErrorCode.ERROR_QUOTA),
        (["rate limit", "429", "tpm limit", "too many requests", "requests per minute"], ModelErrorCode.ERROR_RATE_LIMIT),
        (["auth", "key", "apikey", "401", "forbidden", "permission"], ModelErrorCode.ERROR_AUTHENTICATION),
        (["invalid", "bad request", "400", "format", "malformed", "parameter"], ModelErrorCode.ERROR_INVALID_REQUEST),
        (["server", "503", "502", "504", "500", "unavailable"], ModelErrorCode.ERROR_SERVER),
        (["timeout", "timed out"], ModelErrorCode.ERROR_TIMEOUT),
        (["connect", "network", "unreachable", "dns"], ModelErrorCode.ERROR_CONNECTION),
        (["filter", "content", "policy", "blocked", "safety", "inappropriate"], ModelErrorCode.ERROR_CONTENT_FILTER),
        (["model", "not found", "does not exist", "not available"], ModelErrorCode.ERROR_MODEL),
        (["max rounds"], ModelErrorCode.ERROR_MODEL),
    ]
    for words, code in keywords_mapping:
        if re.search("({})".format("|".join(re.escape(w) for w in words)), error_str):
            logging.debug("classify_model_error matched code=%s", code)
            return code
    logging.debug("classify_model_error fell back to ERROR_GENERIC")
    return ModelErrorCode.ERROR_GENERIC
