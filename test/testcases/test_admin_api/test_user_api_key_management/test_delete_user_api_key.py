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

from typing import Any

import pytest
import requests

from conftest import delete_user_api_key, generate_user_api_key, get_user_api_key, UNAUTHORIZED_ERROR_MESSAGE
from common.constants import RetCode
from configs import EMAIL, HOST_ADDRESS, PASSWORD, VERSION


class TestDeleteUserApiKey:
    @pytest.mark.p2
    def test_delete_user_api_key_success(self, admin_session: requests.Session) -> None:
        """Test successfully deleting an API key for a user"""
        user_name: str = EMAIL

        # Generate an API key first
        generate_response: dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert generate_response.get("code") == RetCode.SUCCESS, f"Generate should succeed, got code {generate_response.get('code')}"
        generated_key: dict[str, Any] = generate_response["data"]
        token: str = generated_key["token"]

        # Delete the API key
        delete_response: dict[str, Any] = delete_user_api_key(admin_session, user_name, token)

        # Verify response
        assert delete_response.get("code") == RetCode.SUCCESS, f"Delete should succeed, got code {delete_response.get('code')}"
        assert "message" in delete_response, "Response should contain message"
        message: str = delete_response.get("message", "")
        assert message == "API key deleted successfully", f"Message should indicate success, got: {message}"

    @pytest.mark.p2
    def test_user_api_key_removed_from_list_after_deletion(self, admin_session: requests.Session) -> None:
        """Test that deleted API key is removed from the list"""
        user_name: str = EMAIL

        # Generate an API key
        generate_response: dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert generate_response.get("code") == RetCode.SUCCESS, f"Generate should succeed, got code {generate_response.get('code')}"
        generated_key: dict[str, Any] = generate_response["data"]
        token: str = generated_key["token"]

        # Verify the key exists in the list
        get_response_before: dict[str, Any] = get_user_api_key(admin_session, user_name)
        assert get_response_before.get("code") == RetCode.SUCCESS, f"Get should succeed, got code {get_response_before.get('code')}"
        api_keys_before: list[dict[str, Any]] = get_response_before["data"]
        token_found_before: bool = any(key.get("token") == token for key in api_keys_before)
        assert token_found_before, "Generated API key should be in the list before deletion"

        # Delete the API key
        delete_response: dict[str, Any] = delete_user_api_key(admin_session, user_name, token)
        assert delete_response.get("code") == RetCode.SUCCESS, f"Delete should succeed, got code {delete_response.get('code')}"

        # Verify the key is no longer in the list
        get_response_after: dict[str, Any] = get_user_api_key(admin_session, user_name)
        assert get_response_after.get("code") == RetCode.SUCCESS, f"Get should succeed, got code {get_response_after.get('code')}"
        api_keys_after: list[dict[str, Any]] = get_response_after["data"]
        token_found_after: bool = any(key.get("token") == token for key in api_keys_after)
        assert not token_found_after, "Deleted API key should not be in the list after deletion"

    @pytest.mark.p2
    def test_delete_user_api_key_response_structure(self, admin_session: requests.Session) -> None:
        """Test that delete_user_api_key returns correct response structure"""
        user_name: str = EMAIL

        # Generate an API key
        generate_response: dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert generate_response.get("code") == RetCode.SUCCESS, f"Generate should succeed, got code {generate_response.get('code')}"
        token: str = generate_response["data"]["token"]

        # Delete the API key
        delete_response: dict[str, Any] = delete_user_api_key(admin_session, user_name, token)

        # Verify response structure
        assert delete_response.get("code") == RetCode.SUCCESS, f"Response code should be {RetCode.SUCCESS}, got {delete_response.get('code')}"
        assert "message" in delete_response, "Response should contain message"
        # Data can be None for delete operations
        assert "data" in delete_response, "Response should contain data field"

    @pytest.mark.p2
    def test_delete_user_api_key_twice(self, admin_session: requests.Session) -> None:
        """Test that deleting the same token twice behaves correctly"""
        user_name: str = EMAIL

        # Generate an API key
        generate_response: dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert generate_response.get("code") == RetCode.SUCCESS, f"Generate should succeed, got code {generate_response.get('code')}"
        token: str = generate_response["data"]["token"]

        # Delete the API key first time
        delete_response1: dict[str, Any] = delete_user_api_key(admin_session, user_name, token)
        assert delete_response1.get("code") == RetCode.SUCCESS, f"First delete should succeed, got code {delete_response1.get('code')}"

        # Try to delete the same token again
        delete_response2: dict[str, Any] = delete_user_api_key(admin_session, user_name, token)

        # Second delete should fail since token no longer exists
        assert delete_response2.get("code") == RetCode.NOT_FOUND, "Second delete should fail for already deleted token"
        assert "message" in delete_response2, "Response should contain message"

    @pytest.mark.p2
    def test_delete_user_api_key_with_nonexistent_token(self, admin_session: requests.Session) -> None:
        """Test deleting a non-existent API key fails"""
        user_name: str = EMAIL
        nonexistent_token: str = "ragflow-nonexistent-token-12345"

        # Try to delete a non-existent token
        delete_response: dict[str, Any] = delete_user_api_key(admin_session, user_name, nonexistent_token)

        # Should return error
        assert delete_response.get("code") == RetCode.NOT_FOUND, "Delete should fail for non-existent token"
        assert "message" in delete_response, "Response should contain message"
        message: str = delete_response.get("message", "")
        assert message == "API key not found or could not be deleted", f"Message should indicate token not found, got: {message}"

    @pytest.mark.p2
    def test_delete_user_api_key_with_nonexistent_user(self, admin_session: requests.Session) -> None:
        """Test deleting API key for non-existent user fails"""
        nonexistent_user: str = "nonexistent_user_12345@example.com"
        token: str = "ragflow-test-token-12345"

        # Try to delete token for non-existent user
        delete_response: dict[str, Any] = delete_user_api_key(admin_session, nonexistent_user, token)

        # Should return error
        assert delete_response.get("code") == RetCode.NOT_FOUND, "Delete should fail for non-existent user"
        assert "message" in delete_response, "Response should contain message"
        message: str = delete_response.get("message", "")
        expected_message: str = f"User '{nonexistent_user}' not found"
        assert message == expected_message, f"Message should indicate user not found, got: {message}"

    @pytest.mark.p2
    def test_delete_user_api_key_wrong_user_token(self, admin_session: requests.Session) -> None:
        """Test that deleting a token belonging to another user fails"""
        user_name: str = EMAIL

        # create second user
        url: str = HOST_ADDRESS + f"/{VERSION}/user/register"
        user2_email: str = "qa2@ragflow.io"
        register_data: dict[str, str] = {"email": user2_email, "nickname": "qa2", "password": PASSWORD}
        res: Any = requests.post(url=url, json=register_data)
        res: dict[str, Any] = res.json()
        if res.get("code") != 0 and "has already registered" not in res.get("message"):
            raise Exception(f"Failed to create second user: {res.get("message")}")

        # Generate a token for the test user
        generate_response: dict[str, Any] = generate_user_api_key(admin_session, user_name)
        assert generate_response.get("code") == RetCode.SUCCESS, f"Generate should succeed, got code {generate_response.get('code')}"
        token: str = generate_response["data"]["token"]

        # Try to delete with the second username
        delete_response: dict[str, Any] = delete_user_api_key(admin_session, user2_email, token)

        # Should fail because user doesn't exist or token doesn't belong to that user
        assert delete_response.get("code") == RetCode.NOT_FOUND, "Delete should fail for wrong user"
        assert "message" in delete_response, "Response should contain message"
        message: str = delete_response.get("message", "")
        expected_message: str = "API key not found or could not be deleted"
        assert message == expected_message, f"Message should indicate user not found, got: {message}"

    @pytest.mark.p3
    def test_delete_user_api_key_without_auth(self) -> None:
        """Test that deleting API key without admin auth fails"""
        session: requests.Session = requests.Session()
        user_name: str = EMAIL
        token: str = "ragflow-test-token-12345"

        response: dict[str, Any] = delete_user_api_key(session, user_name, token)

        # Verify error response
        assert response.get("code") == RetCode.UNAUTHORIZED, "Response code should indicate error"
        assert "message" in response, "Response should contain message"
        message: str = response.get("message", "").lower()
        # The message is an HTML string indicating unauthorized user.
        assert message == UNAUTHORIZED_ERROR_MESSAGE
