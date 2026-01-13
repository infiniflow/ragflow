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

"""
Unit tests for MinerU parser fault tolerance features.
"""

import pytest
import requests
from unittest.mock import Mock, patch

from deepdoc.parser.mineru_parser import (
    MinerUParser,
    BatchStatus,
    BatchErrorType,
    BatchInfo,
    BatchProcessingResult,
    API_TIMEOUT_SECONDS,
    MAX_RETRIES_5XX,
    RETRY_BACKOFF_BASE
)


class TestMinerUParserErrorClassification:
    """Test error classification for batch processing"""

    def setup_method(self):
        """Setup test fixtures"""
        self.parser = MinerUParser(mineru_api="http://test-api")

    def test_classify_timeout_error(self):
        """Test classification of timeout errors"""
        error = requests.exceptions.Timeout("Request timed out")
        result = self.parser._classify_error(error)
        assert result == BatchErrorType.TIMEOUT

    def test_classify_5xx_error(self):
        """Test classification of 5xx server errors"""
        mock_response = Mock()
        mock_response.status_code = 500
        error = requests.exceptions.HTTPError(response=mock_response)
        result = self.parser._classify_error(error)
        assert result == BatchErrorType.SERVER_ERROR_5XX

    def test_classify_4xx_error(self):
        """Test classification of 4xx client errors"""
        mock_response = Mock()
        mock_response.status_code = 404
        error = requests.exceptions.HTTPError(response=mock_response)
        result = self.parser._classify_error(error)
        assert result == BatchErrorType.CLIENT_ERROR_4XX

    def test_classify_network_error(self):
        """Test classification of network errors"""
        error = requests.exceptions.ConnectionError("Connection failed")
        result = self.parser._classify_error(error)
        assert result == BatchErrorType.NETWORK_ERROR

    def test_classify_unknown_error(self):
        """Test classification of unknown errors"""
        error = ValueError("Unknown error")
        result = self.parser._classify_error(error)
        assert result == BatchErrorType.UNKNOWN


class TestMinerUParserRetryLogic:
    """Test retry logic for batch processing"""

    def setup_method(self):
        """Setup test fixtures"""
        self.parser = MinerUParser(mineru_api="http://test-api")

    def test_should_retry_5xx_error_under_max(self):
        """Test that 5xx errors should be retried under max retries"""
        assert self.parser._should_retry(BatchErrorType.SERVER_ERROR_5XX, 0) is True
        assert self.parser._should_retry(BatchErrorType.SERVER_ERROR_5XX, 1) is True
        assert self.parser._should_retry(BatchErrorType.SERVER_ERROR_5XX, 2) is True

    def test_should_not_retry_5xx_error_at_max(self):
        """Test that 5xx errors should not be retried at max retries"""
        assert self.parser._should_retry(BatchErrorType.SERVER_ERROR_5XX, MAX_RETRIES_5XX) is False

    def test_should_not_retry_timeout(self):
        """Test that timeout errors should not be retried"""
        assert self.parser._should_retry(BatchErrorType.TIMEOUT, 0) is False

    def test_should_not_retry_4xx_error(self):
        """Test that 4xx errors should not be retried"""
        assert self.parser._should_retry(BatchErrorType.CLIENT_ERROR_4XX, 0) is False

    def test_should_not_retry_network_error(self):
        """Test that network errors should not be retried"""
        assert self.parser._should_retry(BatchErrorType.NETWORK_ERROR, 0) is False

    def test_calculate_backoff_delay(self):
        """Test exponential backoff delay calculation"""
        assert self.parser._calculate_backoff_delay(0) == RETRY_BACKOFF_BASE ** 0  # 1
        assert self.parser._calculate_backoff_delay(1) == RETRY_BACKOFF_BASE ** 1  # 2
        assert self.parser._calculate_backoff_delay(2) == RETRY_BACKOFF_BASE ** 2  # 4


class TestBatchInfo:
    """Test BatchInfo dataclass"""

    def test_batch_info_creation(self):
        """Test creating a BatchInfo instance"""
        batch = BatchInfo(
            batch_idx=0,
            start_page=0,
            end_page=29,
            status=BatchStatus.PENDING
        )
        assert batch.batch_idx == 0
        assert batch.start_page == 0
        assert batch.end_page == 29
        assert batch.status == BatchStatus.PENDING
        assert batch.error_type is None
        assert batch.error_message is None
        assert batch.retry_count == 0
        assert batch.content_count == 0

    def test_batch_info_with_error(self):
        """Test BatchInfo with error information"""
        batch = BatchInfo(
            batch_idx=1,
            start_page=30,
            end_page=59,
            status=BatchStatus.FAILED,
            error_type=BatchErrorType.SERVER_ERROR_5XX,
            error_message="Server error occurred",
            retry_count=2
        )
        assert batch.status == BatchStatus.FAILED
        assert batch.error_type == BatchErrorType.SERVER_ERROR_5XX
        assert batch.error_message == "Server error occurred"
        assert batch.retry_count == 2


class TestBatchProcessingResult:
    """Test BatchProcessingResult dataclass"""

    def test_batch_processing_result_success(self):
        """Test successful batch processing result"""
        result = BatchProcessingResult(
            total_batches=3,
            successful_batches=3,
            failed_batches=[],
            total_content_blocks=150,
            overall_status="success"
        )
        assert result.total_batches == 3
        assert result.successful_batches == 3
        assert len(result.failed_batches) == 0
        assert result.total_content_blocks == 150
        assert result.overall_status == "success"

    def test_batch_processing_result_partial(self):
        """Test partial success batch processing result"""
        failed_batch = BatchInfo(
            batch_idx=1,
            start_page=30,
            end_page=59,
            status=BatchStatus.FAILED,
            error_type=BatchErrorType.TIMEOUT
        )
        result = BatchProcessingResult(
            total_batches=3,
            successful_batches=2,
            failed_batches=[failed_batch],
            total_content_blocks=100,
            overall_status="partial_success"
        )
        assert result.total_batches == 3
        assert result.successful_batches == 2
        assert len(result.failed_batches) == 1
        assert result.total_content_blocks == 100
        assert result.overall_status == "partial_success"


class TestConstants:
    """Test that constants are properly defined"""

    def test_api_timeout_increased(self):
        """Test that API timeout is set to 2 hours"""
        assert API_TIMEOUT_SECONDS == 7200, "API timeout should be 2 hours (7200 seconds)"

    def test_max_retries_defined(self):
        """Test that max retries is properly defined"""
        assert MAX_RETRIES_5XX == 3, "Max retries should be 3"

    def test_retry_backoff_base_defined(self):
        """Test that retry backoff base is properly defined"""
        assert RETRY_BACKOFF_BASE == 2, "Retry backoff base should be 2"


class TestGetBatchProcessingResult:
    """Test getting batch processing result"""

    def test_get_batch_processing_result_none_initially(self):
        """Test that batch processing result is None initially"""
        parser = MinerUParser(mineru_api="http://test-api")
        assert parser.get_batch_processing_result() is None

    def test_get_batch_processing_result_after_setting(self):
        """Test that batch processing result can be retrieved after setting"""
        parser = MinerUParser(mineru_api="http://test-api")
        result = BatchProcessingResult(
            total_batches=2,
            successful_batches=2,
            failed_batches=[],
            total_content_blocks=100,
            overall_status="success"
        )
        parser.batch_processing_result = result
        retrieved = parser.get_batch_processing_result()
        assert retrieved is not None
        assert retrieved.total_batches == 2
        assert retrieved.overall_status == "success"
