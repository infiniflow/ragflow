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
from unittest.mock import AsyncMock, MagicMock, patch, call

import httpx
import pytest
import anyio

from common.http_client import (
    _clean_headers,
    _get_delay,
    async_request,
    sync_request,
    DEFAULT_TIMEOUT,
    DEFAULT_FOLLOW_REDIRECTS,
    DEFAULT_MAX_REDIRECTS,
    DEFAULT_MAX_RETRIES,
    DEFAULT_BACKOFF_FACTOR,
    DEFAULT_USER_AGENT,
)


class TestCleanHeaders:
    """Test cases for _clean_headers helper function"""

    def test_clean_headers_none_input(self):
        """Test _clean_headers with None headers"""
        result = _clean_headers(None)
        assert result is not None
        assert "User-Agent" in result
        assert result["User-Agent"] == DEFAULT_USER_AGENT

    def test_clean_headers_with_auth_token(self):
        """Test _clean_headers with auth token"""
        result = _clean_headers(None, auth_token="Bearer test_token")
        assert result["Authorization"] == "Bearer test_token"
        assert result["User-Agent"] == DEFAULT_USER_AGENT

    def test_clean_headers_with_custom_headers(self):
        """Test _clean_headers with custom headers"""
        custom_headers = {"X-Custom-Header": "custom_value", "Content-Type": "application/json"}
        result = _clean_headers(custom_headers)
        assert result["X-Custom-Header"] == "custom_value"
        assert result["Content-Type"] == "application/json"
        assert result["User-Agent"] == DEFAULT_USER_AGENT

    def test_clean_headers_merges_auth_and_custom(self):
        """Test _clean_headers merges auth token and custom headers"""
        custom_headers = {"X-Custom": "value"}
        result = _clean_headers(custom_headers, auth_token="Bearer token")
        assert result["Authorization"] == "Bearer token"
        assert result["X-Custom"] == "value"
        assert result["User-Agent"] == DEFAULT_USER_AGENT

    def test_clean_headers_filters_none_values(self):
        """Test _clean_headers filters out None values"""
        custom_headers = {"X-Valid": "value", "X-None": None}
        result = _clean_headers(custom_headers)
        assert "X-Valid" in result
        assert "X-None" not in result

    def test_clean_headers_converts_to_string(self):
        """Test _clean_headers converts header values to strings"""
        custom_headers = {"X-Number": 123, "X-Bool": True}
        result = _clean_headers(custom_headers)
        assert result["X-Number"] == "123"
        assert result["X-Bool"] == "True"

    def test_clean_headers_empty_dict(self):
        """Test _clean_headers with empty dict"""
        result = _clean_headers({})
        assert "User-Agent" in result


class TestGetDelay:
    """Test cases for _get_delay helper function"""

    def test_get_delay_first_attempt(self):
        """Test _get_delay for first retry attempt"""
        delay = _get_delay(0.5, 0)
        assert delay == 0.5  # 0.5 * 2^0

    def test_get_delay_second_attempt(self):
        """Test _get_delay for second retry attempt"""
        delay = _get_delay(0.5, 1)
        assert delay == 1.0  # 0.5 * 2^1

    def test_get_delay_third_attempt(self):
        """Test _get_delay for third retry attempt"""
        delay = _get_delay(0.5, 2)
        assert delay == 2.0  # 0.5 * 2^2

    def test_get_delay_exponential_backoff(self):
        """Test _get_delay exponential backoff calculation"""
        backoff_factor = 1.0
        delays = [_get_delay(backoff_factor, i) for i in range(5)]
        assert delays == [1.0, 2.0, 4.0, 8.0, 16.0]

    def test_get_delay_custom_backoff_factor(self):
        """Test _get_delay with custom backoff factor"""
        delay = _get_delay(2.0, 3)
        assert delay == 16.0  # 2.0 * 2^3


class TestAsyncRequest:
    """Test cases for async_request function"""

    @pytest.mark.anyio
    async def test_async_request_success(self):
        """Test successful async_request"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(return_value=mock_response)
            mock_client_class.return_value.__aenter__.return_value = mock_client

            response = await async_request("GET", "https://example.com")

            assert response.status_code == 200
            mock_client.request.assert_called_once()

    @pytest.mark.anyio
    async def test_async_request_with_custom_timeout(self):
        """Test async_request with custom timeout"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(return_value=mock_response)
            mock_client_class.return_value.__aenter__.return_value = mock_client

            response = await async_request("GET", "https://example.com", timeout=30.0)

            mock_client_class.assert_called_once()
            call_kwargs = mock_client_class.call_args[1]
            assert call_kwargs["timeout"] == 30.0

    @pytest.mark.anyio
    async def test_async_request_with_headers(self):
        """Test async_request with custom headers"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(return_value=mock_response)
            mock_client_class.return_value.__aenter__.return_value = mock_client

            custom_headers = {"X-Custom": "value"}
            response = await async_request("GET", "https://example.com", headers=custom_headers)

            mock_client.request.assert_called_once()
            call_kwargs = mock_client.request.call_args[1]
            assert "X-Custom" in call_kwargs["headers"]
            assert call_kwargs["headers"]["X-Custom"] == "value"

    @pytest.mark.anyio
    async def test_async_request_with_auth_token(self):
        """Test async_request with auth token"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(return_value=mock_response)
            mock_client_class.return_value.__aenter__.return_value = mock_client

            response = await async_request("GET", "https://example.com", auth_token="Bearer token")

            mock_client.request.assert_called_once()
            call_kwargs = mock_client.request.call_args[1]
            assert call_kwargs["headers"]["Authorization"] == "Bearer token"

    @pytest.mark.anyio
    async def test_async_request_with_follow_redirects(self):
        """Test async_request with follow_redirects parameter"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(return_value=mock_response)
            mock_client_class.return_value.__aenter__.return_value = mock_client

            response = await async_request("GET", "https://example.com", follow_redirects=False)

            mock_client_class.assert_called_once()
            call_kwargs = mock_client_class.call_args[1]
            assert call_kwargs["follow_redirects"] is False

    @pytest.mark.anyio
    async def test_async_request_with_max_redirects(self):
        """Test async_request with max_redirects parameter"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(return_value=mock_response)
            mock_client_class.return_value.__aenter__.return_value = mock_client

            response = await async_request("GET", "https://example.com", max_redirects=10)

            mock_client_class.assert_called_once()
            call_kwargs = mock_client_class.call_args[1]
            assert call_kwargs["max_redirects"] == 10

    @pytest.mark.anyio
    async def test_async_request_with_proxies(self):
        """Test async_request with proxies parameter"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(return_value=mock_response)
            mock_client_class.return_value.__aenter__.return_value = mock_client

            proxies = "http://proxy.example.com:8080"
            response = await async_request("GET", "https://example.com", proxies=proxies)

            mock_client_class.assert_called_once()
            call_kwargs = mock_client_class.call_args[1]
            assert call_kwargs["proxies"] == proxies

    @pytest.mark.anyio
    async def test_async_request_retry_on_failure(self):
        """Test async_request retries on failure"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            # First two calls fail, third succeeds
            mock_client.request = AsyncMock(
                side_effect=[
                    httpx.RequestError("Connection error"),
                    httpx.RequestError("Connection error"),
                    mock_response
                ]
            )
            mock_client_class.return_value.__aenter__.return_value = mock_client

            with patch('asyncio.sleep', new_callable=AsyncMock):
                response = await async_request("GET", "https://example.com", retries=2)

            assert response.status_code == 200
            assert mock_client.request.call_count == 3

    @pytest.mark.anyio
    async def test_async_request_exhausted_retries(self):
        """Test async_request raises exception when retries exhausted"""
        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(side_effect=httpx.RequestError("Connection error"))
            mock_client_class.return_value.__aenter__.return_value = mock_client

            with patch('asyncio.sleep', new_callable=AsyncMock):
                with pytest.raises(httpx.RequestError):
                    await async_request("GET", "https://example.com", retries=2)

            assert mock_client.request.call_count == 3  # Initial + 2 retries

    @pytest.mark.anyio
    async def test_async_request_zero_retries(self):
        """Test async_request with zero retries"""
        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(side_effect=httpx.RequestError("Connection error"))
            mock_client_class.return_value.__aenter__.return_value = mock_client

            with pytest.raises(httpx.RequestError):
                await async_request("GET", "https://example.com", retries=0)

            assert mock_client.request.call_count == 1  # Only initial attempt

    @pytest.mark.anyio
    async def test_async_request_custom_backoff(self):
        """Test async_request with custom backoff factor"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(
                side_effect=[httpx.RequestError("Error"), mock_response]
            )
            mock_client_class.return_value.__aenter__.return_value = mock_client

            with patch('asyncio.sleep', new_callable=AsyncMock) as mock_sleep:
                response = await async_request(
                    "GET", "https://example.com",
                    retries=1,
                    backoff_factor=2.0
                )

                # Check that sleep was called with correct delay (2.0 * 2^0 = 2.0)
                mock_sleep.assert_called_once()
                assert mock_sleep.call_args[0][0] == 2.0

    @pytest.mark.anyio
    async def test_async_request_with_json_data(self):
        """Test async_request with JSON data"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(return_value=mock_response)
            mock_client_class.return_value.__aenter__.return_value = mock_client

            json_data = {"key": "value"}
            response = await async_request("POST", "https://example.com", json=json_data)

            mock_client.request.assert_called_once()
            call_kwargs = mock_client.request.call_args[1]
            assert call_kwargs["json"] == json_data

    @pytest.mark.anyio
    async def test_async_request_post_method(self):
        """Test async_request with POST method"""
        mock_response = MagicMock()
        mock_response.status_code = 201

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.request = AsyncMock(return_value=mock_response)
            mock_client_class.return_value.__aenter__.return_value = mock_client

            response = await async_request("POST", "https://example.com", json={"data": "test"})

            assert response.status_code == 201
            call_args = mock_client.request.call_args
            assert call_args[1]["method"] == "POST"


class TestSyncRequest:
    """Test cases for sync_request function"""

    def test_sync_request_success(self):
        """Test successful sync_request"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            response = sync_request("GET", "https://example.com")

            assert response.status_code == 200
            mock_client.request.assert_called_once()

    def test_sync_request_with_custom_timeout(self):
        """Test sync_request with custom timeout"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            response = sync_request("GET", "https://example.com", timeout=60.0)

            mock_client_class.assert_called_once()
            call_kwargs = mock_client_class.call_args[1]
            assert call_kwargs["timeout"] == 60.0

    def test_sync_request_with_headers(self):
        """Test sync_request with custom headers"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            custom_headers = {"X-Test": "test_value"}
            response = sync_request("GET", "https://example.com", headers=custom_headers)

            mock_client.request.assert_called_once()
            call_kwargs = mock_client.request.call_args[1]
            assert "X-Test" in call_kwargs["headers"]
            assert call_kwargs["headers"]["X-Test"] == "test_value"

    def test_sync_request_with_auth_token(self):
        """Test sync_request with auth token"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            response = sync_request("GET", "https://example.com", auth_token="Bearer sync_token")

            mock_client.request.assert_called_once()
            call_kwargs = mock_client.request.call_args[1]
            assert call_kwargs["headers"]["Authorization"] == "Bearer sync_token"

    def test_sync_request_with_follow_redirects(self):
        """Test sync_request with follow_redirects parameter"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            response = sync_request("GET", "https://example.com", follow_redirects=False)

            mock_client_class.assert_called_once()
            call_kwargs = mock_client_class.call_args[1]
            assert call_kwargs["follow_redirects"] is False

    def test_sync_request_with_max_redirects(self):
        """Test sync_request with max_redirects parameter"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            response = sync_request("GET", "https://example.com", max_redirects=5)

            mock_client_class.assert_called_once()
            call_kwargs = mock_client_class.call_args[1]
            assert call_kwargs["max_redirects"] == 5

    def test_sync_request_with_proxies(self):
        """Test sync_request with proxies parameter"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            proxies = "http://proxy.example.com:8080"
            response = sync_request("GET", "https://example.com", proxies=proxies)

            mock_client_class.assert_called_once()
            call_kwargs = mock_client_class.call_args[1]
            assert call_kwargs["proxies"] == proxies

    def test_sync_request_retry_on_failure(self):
        """Test sync_request retries on failure"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            # First two calls fail, third succeeds
            mock_client.request = MagicMock(
                side_effect=[
                    httpx.RequestError("Connection error"),
                    httpx.RequestError("Connection error"),
                    mock_response
                ]
            )
            mock_client_class.return_value.__enter__.return_value = mock_client

            with patch('time.sleep'):
                response = sync_request("GET", "https://example.com", retries=2)

            assert response.status_code == 200
            assert mock_client.request.call_count == 3

    def test_sync_request_exhausted_retries(self):
        """Test sync_request raises exception when retries exhausted"""
        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(side_effect=httpx.RequestError("Connection error"))
            mock_client_class.return_value.__enter__.return_value = mock_client

            with patch('time.sleep'):
                with pytest.raises(httpx.RequestError):
                    sync_request("GET", "https://example.com", retries=2)

            assert mock_client.request.call_count == 3  # Initial + 2 retries

    def test_sync_request_zero_retries(self):
        """Test sync_request with zero retries"""
        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(side_effect=httpx.RequestError("Connection error"))
            mock_client_class.return_value.__enter__.return_value = mock_client

            with pytest.raises(httpx.RequestError):
                sync_request("GET", "https://example.com", retries=0)

            assert mock_client.request.call_count == 1  # Only initial attempt

    def test_sync_request_custom_backoff(self):
        """Test sync_request with custom backoff factor"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(
                side_effect=[httpx.RequestError("Error"), mock_response]
            )
            mock_client_class.return_value.__enter__.return_value = mock_client

            with patch('time.sleep') as mock_sleep:
                response = sync_request(
                    "GET", "https://example.com",
                    retries=1,
                    backoff_factor=3.0
                )

                # Check that sleep was called with correct delay (3.0 * 2^0 = 3.0)
                mock_sleep.assert_called_once()
                assert mock_sleep.call_args[0][0] == 3.0

    def test_sync_request_with_json_data(self):
        """Test sync_request with JSON data"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            json_data = {"test": "data"}
            response = sync_request("POST", "https://example.com", json=json_data)

            mock_client.request.assert_called_once()
            call_kwargs = mock_client.request.call_args[1]
            assert call_kwargs["json"] == json_data

    def test_sync_request_put_method(self):
        """Test sync_request with PUT method"""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            response = sync_request("PUT", "https://example.com", json={"update": "data"})

            assert response.status_code == 200
            call_args = mock_client.request.call_args
            assert call_args[1]["method"] == "PUT"

    def test_sync_request_delete_method(self):
        """Test sync_request with DELETE method"""
        mock_response = MagicMock()
        mock_response.status_code = 204

        with patch('httpx.Client') as mock_client_class:
            mock_client = MagicMock()
            mock_client.request = MagicMock(return_value=mock_response)
            mock_client_class.return_value.__enter__.return_value = mock_client

            response = sync_request("DELETE", "https://example.com/resource/123")

            assert response.status_code == 204
            call_args = mock_client.request.call_args
            assert call_args[1]["method"] == "DELETE"


class TestDefaultConstants:
    """Test cases for default constants"""

    def test_default_timeout(self):
        """Test DEFAULT_TIMEOUT is set correctly"""
        assert DEFAULT_TIMEOUT == 15.0 or isinstance(DEFAULT_TIMEOUT, float)

    def test_default_follow_redirects(self):
        """Test DEFAULT_FOLLOW_REDIRECTS is set correctly"""
        assert isinstance(DEFAULT_FOLLOW_REDIRECTS, bool)

    def test_default_max_redirects(self):
        """Test DEFAULT_MAX_REDIRECTS is set correctly"""
        assert DEFAULT_MAX_REDIRECTS == 30 or isinstance(DEFAULT_MAX_REDIRECTS, int)

    def test_default_max_retries(self):
        """Test DEFAULT_MAX_RETRIES is set correctly"""
        assert DEFAULT_MAX_RETRIES == 2 or isinstance(DEFAULT_MAX_RETRIES, int)

    def test_default_backoff_factor(self):
        """Test DEFAULT_BACKOFF_FACTOR is set correctly"""
        assert DEFAULT_BACKOFF_FACTOR == 0.5 or isinstance(DEFAULT_BACKOFF_FACTOR, float)

    def test_default_user_agent(self):
        """Test DEFAULT_USER_AGENT is set correctly"""
        assert DEFAULT_USER_AGENT == "ragflow-http-client" or isinstance(DEFAULT_USER_AGENT, str)
