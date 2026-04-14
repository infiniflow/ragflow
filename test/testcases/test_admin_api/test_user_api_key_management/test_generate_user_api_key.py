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

from typing import Any, Dict, List

import pytest
import requests

from common.constants import RetCode
from conftest import generate_user_api_key, get_user_api_key, UNAUTHORIZED_ERROR_MESSAGE
from configs import EMAIL


class TestGenerateUserApiKey:
    @pytest.mark.p2
    def test_generate_user_api_key_success(self, admin_session: requests.Session) -> None:
        """Test successfully generating API key for a user"""
        # Use the test user email (get_user_details expects email)
        user_name: str = EMAIL

        # Generate API key
        response: Dict[str, Any] = generate_user_api_key(admin_session, user_name)

        # Verify response code, message, and data
        assert response.get("code") == RetCode.SUCCESS, f"Response code should be {RetCode.SUCCESS}, got {response.get('code')}"
        assert "message" in response, "Response should contain message"
        assert "data" in response, "Response should contain data"
        assert response.get("data") is not None, "API key generation should return data"

        result: Dict[str, Any] = response["data"]

        # Verify response structure
        assert "tenant_id" in result, "Response should contain tenant_id"
        assert "token" in result, "Response should contain token"
        assert "beta" in result, "Response should contain beta"
        assert "create_time" in result, "Response should contain create_time"
        assert "create_date" in result, "Response should contain create_date"

        # Verify token format (should start with "ragflow-")
        token: str = result["token"]
        assert isinstance(token, str), "Token should be a string"
        assert len(token) > 0, "Token should not be empty"

        # Verify beta is independently generated
        beta: str = result["beta"]
        assert isinstance(beta, str), "Beta should be a string"
        assert len(beta) == 32, "Beta should be 32 characters"
        # Beta should be independent from token (not derived from it)
        if token.startswith("ragflow-"):
            token_without_prefix: str = token.replace("ragflow-", "")[:32]
            assert beta != token_without_prefix, "Beta should be independently generated, not derived from token"

    @pytest.mark.p2
    def test_generate_user_api_key_appears_in_list(self, admin_session: requests.Session) -> None:
        """Test that generated API key appears in get_user_api_key list"""
        user_name: str = EMAIL

        # Generate API key
        generate_response: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert generate_response.get("code") == RetCode.SUCCESS, f"Generate should succeed, got code {generate_response.get('code')}"
        generated_key: Dict[str, Any] = generate_response["data"]
        token: str = generated_key["token"]

        # Get all API keys for the user
        get_response: Dict[str, Any] = get_user_api_key(admin_session, user_name)
        assert get_response.get("code") == RetCode.SUCCESS, f"Get should succeed, got code {get_response.get('code')}"
        api_keys: List[Dict[str, Any]] = get_response["data"]

        # Verify the generated key is in the list
        assert len(api_keys) > 0, "User should have at least one API key"
        token_found: bool = any(key.get("token") == token for key in api_keys)
        assert token_found, "Generated API key should appear in the list"

    @pytest.mark.p2
    def test_generate_user_api_key_response_structure(self, admin_session: requests.Session) -> None:
        """Test that generate_user_api_key returns correct response structure"""
        user_name: str = EMAIL

        response: Dict[str, Any] = generate_user_api_key(admin_session, user_name)

        # Verify response code, message, and data
        assert response.get("code") == RetCode.SUCCESS, f"Response code should be {RetCode.SUCCESS}, got {response.get('code')}"
        assert "message" in response, "Response should contain message"
        assert "data" in response, "Response should contain data"

        result: Dict[str, Any] = response["data"]

        # Verify all required fields
        assert "tenant_id" in result, "Response should have tenant_id"
        assert "token" in result, "Response should have token"
        assert "beta" in result, "Response should have beta"
        assert "create_time" in result, "Response should have create_time"
        assert "create_date" in result, "Response should have create_date"
        assert "update_time" in result, "Response should have update_time"
        assert "update_date" in result, "Response should have update_date"

        # Verify field types
        assert isinstance(result["tenant_id"], str), "tenant_id should be string"
        assert isinstance(result["token"], str), "token should be string"
        assert isinstance(result["beta"], str), "beta should be string"
        assert isinstance(result["create_time"], (int, type(None))), "create_time should be int or None"
        assert isinstance(result["create_date"], (str, type(None))), "create_date should be string or None"

    @pytest.mark.p2
    def test_generate_user_api_key_multiple_times(self, admin_session: requests.Session) -> None:
        """Test generating multiple API keys for the same user"""
        user_name: str = EMAIL

        # Generate first API key
        response1: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response1.get("code") == RetCode.SUCCESS, f"First generate should succeed, got code {response1.get('code')}"
        key1: Dict[str, Any] = response1["data"]
        token1: str = key1["token"]

        # Generate second API key
        response2: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response2.get("code") == RetCode.SUCCESS, f"Second generate should succeed, got code {response2.get('code')}"
        key2: Dict[str, Any] = response2["data"]
        token2: str = key2["token"]

        # Tokens should be different
        assert token1 != token2, "Multiple API keys should have different tokens"

        # Both should appear in the list
        get_response: Dict[str, Any] = get_user_api_key(admin_session, user_name)
        assert get_response.get("code") == RetCode.SUCCESS, f"Get should succeed, got code {get_response.get('code')}"
        api_keys: List[Dict[str, Any]] = get_response["data"]
        tokens: List[str] = [key.get("token") for key in api_keys]
        assert token1 in tokens, "First token should be in the list"
        assert token2 in tokens, "Second token should be in the list"

    @pytest.mark.p2
    def test_generate_user_api_key_nonexistent_user(self, admin_session: requests.Session) -> None:
        """Test generating API key for non-existent user fails"""
        response: Dict[str, Any] = generate_user_api_key(admin_session, "nonexistent_user_12345")

        # Verify error response
        assert response.get("code") == RetCode.NOT_FOUND, "Response code should indicate error"
        assert "message" in response, "Response should contain message"
        message: str = response.get("message", "")
        assert message == "User not found!", f"Message should indicate user not found, got: {message}"

    @pytest.mark.p2
    def test_generate_user_api_key_tenant_id_consistency(self, admin_session: requests.Session) -> None:
        """Test that generated API keys have consistent tenant_id"""
        user_name: str = EMAIL

        # Generate multiple API keys
        response1: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response1.get("code") == RetCode.SUCCESS, f"First generate should succeed, got code {response1.get('code')}"
        key1: Dict[str, Any] = response1["data"]

        response2: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response2.get("code") == RetCode.SUCCESS, f"Second generate should succeed, got code {response2.get('code')}"
        key2: Dict[str, Any] = response2["data"]

        # Tenant IDs should be the same for the same user
        assert key1["tenant_id"] == key2["tenant_id"], "Same user should have same tenant_id"

    @pytest.mark.p2
    def test_generate_user_api_key_token_format(self, admin_session: requests.Session) -> None:
        """Test that generated API key has correct format"""
        user_name: str = EMAIL

        response: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response.get("code") == RetCode.SUCCESS, f"Response code should be {RetCode.SUCCESS}, got {response.get('code')}"
        result: Dict[str, Any] = response["data"]
        token: str = result["token"]

        # Token should be a non-empty string
        assert isinstance(token, str), "Token should be a string"
        assert len(token) > 0, "Token should not be empty"

        # Beta should be independently generated (32 chars, not derived from token)
        beta: str = result["beta"]
        assert isinstance(beta, str), "Beta should be a string"
        assert len(beta) == 32, "Beta should be 32 characters"
        # Beta should be independent from token (not derived from it)
        if token.startswith("ragflow-"):
            token_without_prefix: str = token.replace("ragflow-", "")[:32]
            assert beta != token_without_prefix, "Beta should be independently generated, not derived from token"

    @pytest.mark.p1
    def test_generate_user_api_key_without_auth(self) -> None:
        """Test that generating API key without admin auth fails"""
        session: requests.Session = requests.Session()
        user_name: str = EMAIL

        response: Dict[str, Any] = generate_user_api_key(session, user_name)

        # Verify error response
        assert response.get("code") == RetCode.UNAUTHORIZED, "Response code should indicate error"
        assert "message" in response, "Response should contain message"
        message: str = response.get("message", "").lower()
        # The message is an HTML string indicating unauthorized user .
        assert message == UNAUTHORIZED_ERROR_MESSAGE

    @pytest.mark.p3
    def test_generate_user_api_key_timestamp_fields(self, admin_session: requests.Session) -> None:
        """Test that generated API key has correct timestamp fields"""
        user_name: str = EMAIL

        response: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response.get("code") == RetCode.SUCCESS, f"Response code should be {RetCode.SUCCESS}, got {response.get('code')}"
        result: Dict[str, Any] = response["data"]

        # create_time should be a timestamp (int)
        create_time: Any = result.get("create_time")
        assert create_time is None or isinstance(create_time, int), "create_time should be int or None"
        if create_time is not None:
            assert create_time > 0, "create_time should be positive"

        # create_date should be a date string
        create_date: Any = result.get("create_date")
        assert create_date is None or isinstance(create_date, str), "create_date should be string or None"

        # update_time and update_date should be None for new keys
        assert result.get("update_time") is None, "update_time should be None for new keys"
        assert result.get("update_date") is None, "update_date should be None for new keys"
