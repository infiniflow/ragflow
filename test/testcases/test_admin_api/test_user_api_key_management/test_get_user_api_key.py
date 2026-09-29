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

from conftest import generate_user_api_key, get_user_api_key, UNAUTHORIZED_ERROR_MESSAGE
from common.constants import RetCode
from configs import EMAIL


class TestGetUserApiKey:
    @pytest.mark.p2
    def test_get_user_api_key_success(self, admin_session: requests.Session) -> None:
        """Test successfully getting API keys for a user with correct response structure"""
        user_name: str = EMAIL

        # Generate a test API key first
        generate_response: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert generate_response["code"] == RetCode.SUCCESS, generate_response
        generated_key: Dict[str, Any] = generate_response["data"]
        generated_token: str = generated_key["token"]

        # Get all API keys for the user
        get_response: Dict[str, Any] = get_user_api_key(admin_session, user_name)
        assert get_response["code"] == RetCode.SUCCESS, get_response
        assert "message" in get_response, "Response should contain message"
        assert "data" in get_response, "Response should contain data"

        api_keys: List[Dict[str, Any]] = get_response["data"]

        # Verify response is a list with at least one key
        assert isinstance(api_keys, list), "API keys should be returned as a list"
        assert len(api_keys) > 0, "User should have at least one API key"

        # Verify structure of each API key
        for key in api_keys:
            assert isinstance(key, dict), "Each API key should be a dictionary"
            assert "token" in key, "API key should contain token"
            assert "beta" in key, "API key should contain beta"
            assert "tenant_id" in key, "API key should contain tenant_id"
            assert "create_date" in key, "API key should contain create_date"

            # Verify field types
            assert isinstance(key["token"], str), "token should be string"
            assert isinstance(key["beta"], str), "beta should be string"
            assert isinstance(key["tenant_id"], str), "tenant_id should be string"
            assert isinstance(key.get("create_date"), (str, type(None))), "create_date should be string or None"
            assert isinstance(key.get("update_date"), (str, type(None))), "update_date should be string or None"

        # Verify the generated key is in the list
        token_found: bool = any(key.get("token") == generated_token for key in api_keys)
        assert token_found, "Generated API key should appear in the list"

    @pytest.mark.p2
    def test_get_user_api_key_nonexistent_user(self, admin_session: requests.Session) -> None:
        """Test getting API keys for non-existent user fails"""
        nonexistent_user: str = "nonexistent_user_12345"
        response: Dict[str, Any] = get_user_api_key(admin_session, nonexistent_user)

        assert response["code"] == RetCode.NOT_FOUND, response
        assert "message" in response, "Response should contain message"
        message: str = response["message"]
        expected_message: str = f"User '{nonexistent_user}' not found"
        assert message == expected_message, f"Message should indicate user not found, got: {message}"

    @pytest.mark.p2
    def test_get_user_api_key_empty_username(self, admin_session: requests.Session) -> None:
        """Test getting API keys with empty username"""
        response: Dict[str, Any] = get_user_api_key(admin_session, "")

        # Empty username should either return error or empty list
        if response["code"] == RetCode.SUCCESS:
            assert "data" in response, "Response should contain data"
            api_keys: List[Dict[str, Any]] = response["data"]
            assert isinstance(api_keys, list), "Should return a list"
            assert len(api_keys) == 0, "Empty username should return empty list"
        else:
            assert "message" in response, "Error response should contain message"
            assert len(response["message"]) > 0, "Error message should not be empty"

    @pytest.mark.p2
    def test_get_user_api_key_token_uniqueness(self, admin_session: requests.Session) -> None:
        """Test that all API keys in the list have unique tokens"""
        user_name: str = EMAIL

        # Generate multiple API keys
        response1: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response1["code"] == RetCode.SUCCESS, response1
        response2: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response2["code"] == RetCode.SUCCESS, response2

        # Get all API keys
        get_response: Dict[str, Any] = get_user_api_key(admin_session, user_name)
        assert get_response["code"] == RetCode.SUCCESS, get_response
        api_keys: List[Dict[str, Any]] = get_response["data"]

        # Verify all tokens are unique
        tokens: List[str] = [key.get("token") for key in api_keys if key.get("token")]
        assert len(tokens) == len(set(tokens)), "All API keys should have unique tokens"

    @pytest.mark.p2
    def test_get_user_api_key_tenant_id_consistency(self, admin_session: requests.Session) -> None:
        """Test that all API keys for a user have the same tenant_id"""
        user_name: str = EMAIL

        # Generate multiple API keys
        response1: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response1["code"] == RetCode.SUCCESS, response1
        response2: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert response2["code"] == RetCode.SUCCESS, response2

        # Get all API keys
        get_response: Dict[str, Any] = get_user_api_key(admin_session, user_name)
        assert get_response["code"] == RetCode.SUCCESS, get_response
        api_keys: List[Dict[str, Any]] = get_response["data"]

        # Verify all keys have the same tenant_id
        tenant_ids: List[str] = [key.get("tenant_id") for key in api_keys if key.get("tenant_id")]
        if len(tenant_ids) > 0:
            assert all(tid == tenant_ids[0] for tid in tenant_ids), "All API keys should have the same tenant_id"

    @pytest.mark.p2
    def test_get_user_api_key_beta_format(self, admin_session: requests.Session) -> None:
        """Test that beta field in API keys has correct format (32 characters)"""
        user_name: str = EMAIL

        # Generate a test API key
        generate_response: Dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert generate_response["code"] == RetCode.SUCCESS, generate_response

        # Get all API keys
        get_response: Dict[str, Any] = get_user_api_key(admin_session, user_name)
        assert get_response["code"] == RetCode.SUCCESS, get_response
        api_keys: List[Dict[str, Any]] = get_response["data"]

        # Verify beta format for all keys
        for key in api_keys:
            beta: str = key.get("beta", "")
            assert isinstance(beta, str), "beta should be a string"
            assert len(beta) == 32, f"beta should be 32 characters, got {len(beta)}"

    @pytest.mark.p3
    def test_get_user_api_key_without_auth(self) -> None:
        """Test that getting API keys without admin auth fails"""
        session: requests.Session = requests.Session()
        user_name: str = EMAIL

        response: Dict[str, Any] = get_user_api_key(session, user_name)

        assert response["code"] == RetCode.UNAUTHORIZED, response
        assert "message" in response, "Response should contain message"
        message: str = response["message"].lower()
        assert message == UNAUTHORIZED_ERROR_MESSAGE
