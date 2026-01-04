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

import requests
from enum import Enum
from dataclasses import dataclass
from typing import List, Optional

# Constants
API_TIMEOUT_SECONDS = 7200  # 2 hours
MAX_RETRIES_5XX = 3
RETRY_BACKOFF_BASE = 2


class BatchStatus(Enum):
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"


class BatchErrorType(Enum):
    TIMEOUT = "timeout"
    SERVER_ERROR_5XX = "server_error_5xx"
    CLIENT_ERROR_4XX = "client_error_4xx"
    NETWORK_ERROR = "network_error"
    UNKNOWN = "unknown"


@dataclass
class BatchInfo:
    batch_idx: int
    start_page: int
    end_page: int
    status: BatchStatus
    error_type: Optional[BatchErrorType] = None
    error_message: Optional[str] = None
    retry_count: int = 0
    content_count: int = 0


@dataclass
class BatchProcessingResult:
    total_batches: int
    successful_batches: int
    failed_batches: List[BatchInfo]
    total_content_blocks: int
    overall_status: str


class MinerUParser:
    def __init__(self, mineru_api: str):
        self.mineru_api = mineru_api
        self.batch_processing_result: Optional[BatchProcessingResult] = None

    def _classify_error(self, error: Exception) -> BatchErrorType:
        if isinstance(error, requests.exceptions.Timeout):
            return BatchErrorType.TIMEOUT
        elif isinstance(error, requests.exceptions.HTTPError):
            status_code = error.response.status_code if error.response else 500
            if 500 <= status_code < 600:
                return BatchErrorType.SERVER_ERROR_5XX
            elif 400 <= status_code < 500:
                return BatchErrorType.CLIENT_ERROR_4XX
        elif isinstance(error, requests.exceptions.ConnectionError):
            return BatchErrorType.NETWORK_ERROR
        else:
            return BatchErrorType.UNKNOWN

    def _should_retry(self, error_type: BatchErrorType, retry_count: int) -> bool:
        if error_type == BatchErrorType.SERVER_ERROR_5XX:
            return retry_count < MAX_RETRIES_5XX
        return False

    def _calculate_backoff_delay(self, retry_count: int) -> int:
        return RETRY_BACKOFF_BASE ** retry_count

    def get_batch_processing_result(self) -> Optional[BatchProcessingResult]:
        return self.batch_processing_result