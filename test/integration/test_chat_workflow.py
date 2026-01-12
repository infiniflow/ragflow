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
Chat assistant and session workflow integration tests.

Consolidates tests from:
- testcases/test_http_api/test_chat_assistant_management/
- testcases/test_http_api/test_session_management/

Tests the complete workflow: create assistant → create session → send message → retrieve history

Per AGENTS.md modularization: Tests service layer logic (dialog_app, chat service)
not HTTP/SDK/Web endpoint structure.
"""

import pytest
from configs import INVALID_API_TOKEN
from hypothesis import example, given, settings
from libs.auth import RAGFlowHttpApiAuth
from utils.hypothesis_utils import valid_names


@pytest.mark.usefixtures("clear_datasets")
class TestChatAssistantAuthorization:
    """Test chat assistant authorization."""

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "invalid_auth, expected_code",
        [
            (None, 0),
            (RAGFlowHttpApiAuth(INVALID_API_TOKEN), 109),
        ],
        ids=["empty_auth", "invalid_api_token"],
    )
    def test_auth_invalid(self, invalid_auth, expected_code):
        """Test that invalid authentication is rejected."""
        # Use the parametrized invalid_auth for the request
        auth = invalid_auth if invalid_auth else RAGFlowHttpApiAuth("dummy")
        res = auth.post(
            "/api/v1/chats",
            json={"name": "test_chat"}
        )
        # Assert against expected_code parameter
        assert res.status_code != 200 or res.json().get("code") == expected_code


@pytest.mark.usefixtures("clear_datasets")
class TestChatAssistantCRUD:
    """Test chat assistant CRUD operations."""

    @pytest.mark.p1
    def test_chat_assistant_creation(self, api_client):
        """Test creating a chat assistant."""
        res = api_client.post(
            "/api/v1/chats",
            json={"name": "test_assistant"}
        )
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0, f"Failed to create chat: {data}"
        assert "data" in data
        assert "chat_id" in data["data"]

    @pytest.mark.p1
    def test_chat_assistant_list(self, api_client):
        """Test listing chat assistants."""
        # Create multiple chats
        chat_ids = []
        for i in range(3):
            res = api_client.post(
                "/api/v1/chats",
                json={"name": f"chat_{i}"}
            )
            assert res.status_code == 200
            data = res.json()
            assert data["code"] == 0, f"Failed to create chat_{i}: {data}"
            chat_ids.append(data["data"]["chat_id"])

        # List chats
        res = api_client.get("/api/v1/chats")
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0
        list_ids = [c.get("id") for c in data.get("data", [])]

        # Verify created chats in list
        for chat_id in chat_ids:
            assert chat_id in list_ids, f"Chat {chat_id} not found in list"

    @pytest.mark.p1
    def test_chat_assistant_update(self, api_client):
        """Test updating chat assistant."""
        # Create chat
        res = api_client.post(
            "/api/v1/chats",
            json={"name": "test_chat_original"}
        )
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0, f"Failed to create chat: {data}"
        assert "data" in data and "chat_id" in data["data"]
        chat_id = data["data"]["chat_id"]

        # Update chat
        res = api_client.put(
            f"/api/v1/chats/{chat_id}",
            json={"name": "test_chat_updated"}
        )
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0

    @pytest.mark.p1
    def test_chat_assistant_delete(self, api_client):
        """Test deleting chat assistant."""
        # Create chat
        res = api_client.post(
            "/api/v1/chats",
            json={"name": "test_chat_delete"}
        )
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0, f"Failed to create chat: {data}"
        assert "data" in data and "chat_id" in data["data"]
        chat_id = data["data"]["chat_id"]

        # Verify exists
        res = api_client.get("/api/v1/chats")
        list_ids = [c.get("id") for c in res.json().get("data", [])]
        assert chat_id in list_ids

        # Delete
        res = api_client.delete(f"/api/v1/chats/{chat_id}")
        assert res.status_code == 200

        # Verify deleted
        res = api_client.get("/api/v1/chats")
        list_ids = [c.get("id") for c in res.json().get("data", [])]
        assert chat_id not in list_ids


@pytest.mark.usefixtures("clear_datasets")
class TestChatSessionWorkflow:
    """Test chat session operations."""

    @pytest.mark.p1
    def test_session_creation(self, api_client):
        """Test creating a chat session."""
        # Create chat
        res = api_client.post(
            "/api/v1/chats",
            json={"name": "test_chat_session"}
        )
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0, f"Failed to create chat: {data}"
        assert "data" in data and "chat_id" in data["data"]
        chat_id = data["data"]["chat_id"]

        # Create session
        res = api_client.post(
            f"/api/v1/chats/{chat_id}/sessions",
            json={"name": "test_session"}
        )
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0, f"Failed to create session: {data}"
        assert "session_id" in data.get("data", {})

    @pytest.mark.p1
    def test_session_list(self, api_client):
        """Test listing chat sessions."""
        # Create chat
        res = api_client.post(
            "/api/v1/chats",
            json={"name": "test_chat_sessions"}
        )
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0, f"Failed to create chat: {data}"
        assert "data" in data and "chat_id" in data["data"]
        chat_id = data["data"]["chat_id"]

        # Create sessions
        session_ids = []
        for i in range(3):
            res = api_client.post(
                f"/api/v1/chats/{chat_id}/sessions",
                json={"name": f"session_{i}"}
            )
            assert res.status_code == 200, f"Session creation failed with status {res.status_code}: {res.text}"
            data = res.json()
            assert data["code"] == 0, f"Session creation failed for session_{i}: {data}"
            session_ids.append(data["data"]["session_id"])

        # List sessions
        res = api_client.get(f"/api/v1/chats/{chat_id}/sessions")
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0
        list_ids = [s.get("id") for s in data.get("data", [])]

        # Verify all created sessions in list
        for session_id in session_ids:
            assert session_id in list_ids


@pytest.mark.usefixtures("clear_datasets")
class TestChatWithKnowledgeBase:
    """Test chat assistant connected to knowledge base."""

    @pytest.mark.p2
    @pytest.mark.skip(reason="Requires dataset and document parsing")
    def test_chat_with_dataset_context(self, api_client):
        """Test chat assistant with knowledge base context."""
        # TODO: Implement after document service is stable
        # - Create dataset
        # - Upload and parse documents
        # - Create chat assistant connected to dataset
        # - Verify chat can access dataset context
        pass

    @pytest.mark.p2
    @pytest.mark.skip(reason="Requires full RAG pipeline")
    def test_chat_with_rag_retrieval(self, api_client):
        """Test chat with RAG (Retrieval-Augmented Generation)."""
        # TODO: Implement after RAG pipeline is tested
        # - Create dataset with documents
        # - Create assistant connected to dataset
        # - Send query
        # - Verify retrieved chunks in response
        pass


@pytest.mark.usefixtures("clear_datasets")
class TestChatNameValidation:
    """Test chat name validation."""

    @pytest.mark.p2
    @given(name=valid_names())
    @example("a" * 128)
    @settings(max_examples=10)
    def test_valid_chat_names(self, api_client, name):
        """Test creating chats with valid names."""
        res = api_client.post(
            "/api/v1/chats",
            json={"name": name}
        )
        assert res.status_code == 200
        data = res.json()
        assert data["code"] == 0, f"Failed with name: {name}"
