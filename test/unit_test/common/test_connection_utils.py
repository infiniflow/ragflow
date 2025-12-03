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

import asyncio
import os
import time
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
import trio
import anyio

from common.connection_utils import timeout, construct_response, sync_construct_response
from common.constants import RetCode


class TestTimeoutDecorator:
    """Test cases for the timeout decorator"""

    def test_sync_function_success(self):
        """Test timeout decorator with successful sync function"""
        @timeout(seconds=5)
        def fast_function():
            return "success"

        result = fast_function()
        assert result == "success"

    def test_sync_function_with_args(self):
        """Test timeout decorator with sync function that has arguments"""
        @timeout(seconds=5)
        def add_numbers(a, b):
            return a + b

        result = add_numbers(3, 4)
        assert result == 7

    def test_sync_function_raises_exception(self):
        """Test timeout decorator propagates exceptions from sync function"""
        @timeout(seconds=5)
        def failing_function():
            raise ValueError("Test error")

        with pytest.raises(ValueError, match="Test error"):
            failing_function()

    def test_sync_function_timeout_disabled(self):
        """Test sync function when timeout is disabled (no ENABLE_TIMEOUT_ASSERTION)"""
        @timeout(seconds=1)
        def slow_function():
            time.sleep(0.1)
            return "completed"

        # Without ENABLE_TIMEOUT_ASSERTION, timeout should not trigger
        result = slow_function()
        assert result == "completed"

    def test_sync_function_timeout_enabled(self):
        """Test sync function timeout when ENABLE_TIMEOUT_ASSERTION is set"""
        @timeout(seconds=0.1, attempts=1)
        def slow_function():
            time.sleep(2)
            return "should not reach"

        with patch.dict(os.environ, {"ENABLE_TIMEOUT_ASSERTION": "1"}):
            with pytest.raises(TimeoutError, match="timed out after 0.1 seconds and 1 attempts"):
                slow_function()

    def test_sync_function_with_string_seconds(self):
        """Test timeout decorator with string seconds parameter"""
        @timeout(seconds="5")
        def fast_function():
            return "success"

        result = fast_function()
        assert result == "success"

    def test_sync_function_multiple_attempts(self):
        """Test sync function with multiple attempts"""
        call_count = 0

        @timeout(seconds=0.1, attempts=3)
        def function_with_attempts():
            nonlocal call_count
            call_count += 1
            time.sleep(0.05)
            return f"attempt_{call_count}"

        result = function_with_attempts()
        assert result == "attempt_1"

    @pytest.mark.anyio
    async def test_async_function_success(self):
        """Test timeout decorator with successful async function"""
        @timeout(seconds=5)
        async def fast_async_function():
            await anyio.sleep(0.01)
            return "async_success"

        result = await fast_async_function()
        assert result == "async_success"

    @pytest.mark.anyio
    async def test_async_function_with_args(self):
        """Test timeout decorator with async function that has arguments"""
        @timeout(seconds=5)
        async def async_multiply(x, y):
            await anyio.sleep(0.01)
            return x * y

        result = await async_multiply(6, 7)
        assert result == 42

    @pytest.mark.anyio
    async def test_async_function_none_timeout(self):
        """Test async function with None timeout (no timeout applied)"""
        @timeout(seconds=None)
        async def async_function():
            await anyio.sleep(0.01)
            return "no_timeout"

        result = await async_function()
        assert result == "no_timeout"

    @pytest.mark.anyio
    async def test_async_function_timeout_disabled(self):
        """Test async function when timeout is disabled"""
        @timeout(seconds=1)
        async def slow_async_function():
            await anyio.sleep(0.1)
            return "completed"

        # Without ENABLE_TIMEOUT_ASSERTION, timeout should not trigger
        result = await slow_async_function()
        assert result == "completed"

    @pytest.mark.anyio
    async def test_async_function_multiple_attempts(self):
        """Test async function with multiple attempts"""
        call_count = 0

        @timeout(seconds=0.1, attempts=3)
        async def function_with_attempts():
            nonlocal call_count
            call_count += 1
            await anyio.sleep(0.05)
            return f"attempt_{call_count}"

        result = await function_with_attempts()
        assert result == "attempt_1"


class TestConstructResponse:
    """Test cases for construct_response function"""

    @pytest.mark.anyio
    async def test_construct_response_default(self):
        """Test construct_response with default parameters"""
        with patch('common.connection_utils.make_response', new_callable=AsyncMock) as mock_make_response:
            with patch('common.connection_utils.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response
                mock_jsonify.return_value = {"code": RetCode.SUCCESS, "message": "success"}

                response = await construct_response()

                mock_jsonify.assert_called_once_with({"code": RetCode.SUCCESS, "message": "success"})
                assert response.headers["Access-Control-Allow-Origin"] == "*"
                assert response.headers["Access-Control-Allow-Method"] == "*"
                assert response.headers["Access-Control-Allow-Headers"] == "*"
                assert response.headers["Access-Control-Expose-Headers"] == "Authorization"

    @pytest.mark.anyio
    async def test_construct_response_with_data(self):
        """Test construct_response with data parameter"""
        with patch('common.connection_utils.make_response', new_callable=AsyncMock) as mock_make_response:
            with patch('common.connection_utils.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response

                test_data = {"key": "value"}
                response = await construct_response(data=test_data)

                call_args = mock_jsonify.call_args[0][0]
                assert call_args["code"] == RetCode.SUCCESS
                assert call_args["message"] == "success"
                assert call_args["data"] == test_data

    @pytest.mark.anyio
    async def test_construct_response_with_error_code(self):
        """Test construct_response with error code"""
        with patch('common.connection_utils.make_response', new_callable=AsyncMock) as mock_make_response:
            with patch('common.connection_utils.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response

                response = await construct_response(
                    code=RetCode.ARGUMENT_ERROR,
                    message="Invalid argument"
                )

                call_args = mock_jsonify.call_args[0][0]
                assert call_args["code"] == RetCode.ARGUMENT_ERROR
                assert call_args["message"] == "Invalid argument"

    @pytest.mark.anyio
    async def test_construct_response_with_auth(self):
        """Test construct_response with auth token"""
        with patch('common.connection_utils.make_response', new_callable=AsyncMock) as mock_make_response:
            with patch('common.connection_utils.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response

                auth_token = "Bearer test_token"
                response = await construct_response(auth=auth_token)

                assert response.headers["Authorization"] == auth_token

    @pytest.mark.anyio
    async def test_construct_response_none_values_excluded(self):
        """Test that None values are excluded from response (except code)"""
        with patch('common.connection_utils.make_response', new_callable=AsyncMock) as mock_make_response:
            with patch('common.connection_utils.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response

                response = await construct_response(
                    code=RetCode.SUCCESS,
                    message=None,
                    data=None
                )

                call_args = mock_jsonify.call_args[0][0]
                assert "code" in call_args
                assert "message" not in call_args
                assert "data" not in call_args


class TestSyncConstructResponse:
    """Test cases for sync_construct_response function"""

    def test_sync_construct_response_default(self):
        """Test sync_construct_response with default parameters"""
        with patch('flask.make_response') as mock_make_response:
            with patch('flask.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response
                mock_jsonify.return_value = {"code": RetCode.SUCCESS, "message": "success"}

                response = sync_construct_response()

                mock_jsonify.assert_called_once_with({"code": RetCode.SUCCESS, "message": "success"})
                assert response.headers["Access-Control-Allow-Origin"] == "*"
                assert response.headers["Access-Control-Allow-Method"] == "*"
                assert response.headers["Access-Control-Allow-Headers"] == "*"
                assert response.headers["Access-Control-Expose-Headers"] == "Authorization"

    def test_sync_construct_response_with_data(self):
        """Test sync_construct_response with data parameter"""
        with patch('flask.make_response') as mock_make_response:
            with patch('flask.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response

                test_data = {"result": "test"}
                response = sync_construct_response(data=test_data)

                call_args = mock_jsonify.call_args[0][0]
                assert call_args["code"] == RetCode.SUCCESS
                assert call_args["message"] == "success"
                assert call_args["data"] == test_data

    def test_sync_construct_response_with_error_code(self):
        """Test sync_construct_response with error code"""
        with patch('flask.make_response') as mock_make_response:
            with patch('flask.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response

                response = sync_construct_response(
                    code=RetCode.SERVER_ERROR,
                    message="Server error occurred"
                )

                call_args = mock_jsonify.call_args[0][0]
                assert call_args["code"] == RetCode.SERVER_ERROR
                assert call_args["message"] == "Server error occurred"

    def test_sync_construct_response_with_auth(self):
        """Test sync_construct_response with auth token"""
        with patch('flask.make_response') as mock_make_response:
            with patch('flask.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response

                auth_token = "Bearer sync_token"
                response = sync_construct_response(auth=auth_token)

                assert response.headers["Authorization"] == auth_token

    def test_sync_construct_response_none_values_excluded(self):
        """Test that None values are excluded from response (except code)"""
        with patch('flask.make_response') as mock_make_response:
            with patch('flask.jsonify') as mock_jsonify:
                mock_response = MagicMock()
                mock_response.headers = {}
                mock_make_response.return_value = mock_response

                response = sync_construct_response(
                    code=RetCode.SUCCESS,
                    message=None,
                    data=None
                )

                call_args = mock_jsonify.call_args[0][0]
                assert "code" in call_args
                assert "message" not in call_args
                assert "data" not in call_args
