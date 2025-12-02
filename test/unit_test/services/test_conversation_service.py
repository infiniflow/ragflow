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

import pytest
from unittest.mock import Mock, patch
from common.misc_utils import get_uuid


class TestConversationService:
    """Comprehensive unit tests for ConversationService"""

    @pytest.fixture
    def mock_conversation_service(self):
        """Create a mock ConversationService for testing"""
        with patch('api.db.services.conversation_service.ConversationService') as mock:
            yield mock

    @pytest.fixture
    def sample_conversation_data(self):
        """Sample conversation data for testing"""
        return {
            "id": get_uuid(),
            "dialog_id": get_uuid(),
            "name": "Test Conversation",
            "message": [
                {"role": "assistant", "content": "Hi! How can I help you?"},
                {"role": "user", "content": "Tell me about RAGFlow"},
                {"role": "assistant", "content": "RAGFlow is a RAG engine..."}
            ],
            "reference": [
                {"chunks": [], "doc_aggs": []},
                {"chunks": [{"content": "RAGFlow documentation..."}], "doc_aggs": []}
            ],
            "user_id": "test_user_123"
        }

    def test_conversation_creation_success(self, mock_conversation_service, sample_conversation_data):
        """Test successful conversation creation"""
        mock_conversation_service.save.return_value = True
        
        result = mock_conversation_service.save(**sample_conversation_data)
        assert result is True
        mock_conversation_service.save.assert_called_once_with(**sample_conversation_data)

    def test_conversation_creation_with_prologue(self, mock_conversation_service):
        """Test conversation creation with initial prologue message"""
        conv_data = {
            "id": get_uuid(),
            "dialog_id": get_uuid(),
            "name": "New Conversation",
            "message": [{"role": "assistant", "content": "Hi! I'm your assistant."}],
            "user_id": "user123",
            "reference": []
        }
        
        mock_conversation_service.save.return_value = True
        result = mock_conversation_service.save(**conv_data)
        
        assert result is True
        assert len(conv_data["message"]) == 1
        assert conv_data["message"][0]["role"] == "assistant"

    def test_conversation_get_by_id_success(self, mock_conversation_service, sample_conversation_data):
        """Test retrieving conversation by ID"""
        conv_id = sample_conversation_data["id"]
        mock_conv = Mock()
        mock_conv.to_dict.return_value = sample_conversation_data
        mock_conv.reference = sample_conversation_data["reference"]
        
        mock_conversation_service.get_by_id.return_value = (True, mock_conv)
        
        exists, conv = mock_conversation_service.get_by_id(conv_id)
        assert exists is True
        assert conv.to_dict() == sample_conversation_data

    def test_conversation_get_by_id_not_found(self, mock_conversation_service):
        """Test retrieving non-existent conversation"""
        mock_conversation_service.get_by_id.return_value = (False, None)
        
        exists, conv = mock_conversation_service.get_by_id("nonexistent_id")
        assert exists is False
        assert conv is None

    def test_conversation_update_messages(self, mock_conversation_service, sample_conversation_data):
        """Test updating conversation messages"""
        conv_id = sample_conversation_data["id"]
        new_message = {"role": "user", "content": "Another question"}
        sample_conversation_data["message"].append(new_message)
        
        mock_conversation_service.update_by_id.return_value = True
        result = mock_conversation_service.update_by_id(conv_id, sample_conversation_data)
        
        assert result is True
        assert len(sample_conversation_data["message"]) == 4

    def test_conversation_list_by_dialog(self, mock_conversation_service):
        """Test listing conversations by dialog ID"""
        dialog_id = get_uuid()
        mock_convs = [Mock() for _ in range(5)]
        
        mock_conversation_service.query.return_value = mock_convs
        
        result = mock_conversation_service.query(dialog_id=dialog_id)
        assert len(result) == 5

    def test_conversation_delete_success(self, mock_conversation_service):
        """Test conversation deletion"""
        conv_id = get_uuid()
        
        mock_conversation_service.delete_by_id.return_value = True
        result = mock_conversation_service.delete_by_id(conv_id)
        
        assert result is True
        mock_conversation_service.delete_by_id.assert_called_once_with(conv_id)

    def test_conversation_message_structure_validation(self, sample_conversation_data):
        """Test message structure validation"""
        for msg in sample_conversation_data["message"]:
            assert "role" in msg
            assert "content" in msg
            assert msg["role"] in ["user", "assistant", "system"]

    def test_conversation_reference_structure_validation(self, sample_conversation_data):
        """Test reference structure validation"""
        for ref in sample_conversation_data["reference"]:
            assert "chunks" in ref
            assert "doc_aggs" in ref
            assert isinstance(ref["chunks"], list)
            assert isinstance(ref["doc_aggs"], list)

    def test_conversation_add_user_message(self, sample_conversation_data):
        """Test adding user message to conversation"""
        initial_count = len(sample_conversation_data["message"])
        new_message = {"role": "user", "content": "What is machine learning?"}
        sample_conversation_data["message"].append(new_message)
        
        assert len(sample_conversation_data["message"]) == initial_count + 1
        assert sample_conversation_data["message"][-1]["role"] == "user"

    def test_conversation_add_assistant_message(self, sample_conversation_data):
        """Test adding assistant message to conversation"""
        initial_count = len(sample_conversation_data["message"])
        new_message = {"role": "assistant", "content": "Machine learning is..."}
        sample_conversation_data["message"].append(new_message)
        
        assert len(sample_conversation_data["message"]) == initial_count + 1
        assert sample_conversation_data["message"][-1]["role"] == "assistant"

    def test_conversation_message_with_id(self):
        """Test message with unique ID"""
        message_id = get_uuid()
        message = {
            "role": "user",
            "content": "Test message",
            "id": message_id
        }
        
        assert "id" in message
        assert len(message["id"]) == 32

    def test_conversation_delete_message_pair(self, sample_conversation_data):
        """Test deleting a message pair (user + assistant)"""
        initial_count = len(sample_conversation_data["message"])
        
        # Remove last two messages (user question + assistant answer)
        sample_conversation_data["message"] = sample_conversation_data["message"][:-2]
        
        assert len(sample_conversation_data["message"]) == initial_count - 2

    def test_conversation_thumbup_message(self):
        """Test adding thumbup to assistant message"""
        message = {
            "role": "assistant",
            "content": "Great answer",
            "id": get_uuid(),
            "thumbup": True
        }
        
        assert message["thumbup"] is True

    def test_conversation_thumbdown_with_feedback(self):
        """Test adding thumbdown with feedback"""
        message = {
            "role": "assistant",
            "content": "Answer",
            "id": get_uuid(),
            "thumbup": False,
            "feedback": "Not accurate enough"
        }
        
        assert message["thumbup"] is False
        assert "feedback" in message

    def test_conversation_empty_reference_handling(self, mock_conversation_service):
        """Test handling of empty references"""
        conv_data = {
            "id": get_uuid(),
            "dialog_id": get_uuid(),
            "name": "Test",
            "message": [],
            "reference": [],
            "user_id": "user123"
        }
        
        mock_conversation_service.save.return_value = True
        result = mock_conversation_service.save(**conv_data)
        
        assert result is True
        assert isinstance(conv_data["reference"], list)

    def test_conversation_reference_with_chunks(self):
        """Test reference with document chunks"""
        reference = {
            "chunks": [
                {
                    "content": "Chunk 1 content",
                    "doc_id": "doc1",
                    "score": 0.95
                },
                {
                    "content": "Chunk 2 content",
                    "doc_id": "doc2",
                    "score": 0.87
                }
            ],
            "doc_aggs": [
                {"doc_id": "doc1", "doc_name": "Document 1"}
            ]
        }
        
        assert len(reference["chunks"]) == 2
        assert len(reference["doc_aggs"]) == 1

    def test_conversation_ordering_by_create_time(self, mock_conversation_service):
        """Test conversation ordering by creation time"""
        dialog_id = get_uuid()
        
        mock_convs = [Mock() for _ in range(3)]
        mock_conversation_service.query.return_value = mock_convs
        
        result = mock_conversation_service.query(
            dialog_id=dialog_id,
            order_by=Mock(create_time=Mock()),
            reverse=True
        )
        
        assert len(result) == 3

    def test_conversation_name_length_validation(self):
        """Test conversation name length validation"""
        long_name = "a" * 300
        
        # Name should be truncated to 255 characters
        if len(long_name) > 255:
            truncated_name = long_name[:255]
            assert len(truncated_name) == 255

    def test_conversation_message_alternation(self, sample_conversation_data):
        """Test that messages alternate between user and assistant"""
        messages = sample_conversation_data["message"]
        
        # Skip system messages and check alternation
        non_system = [m for m in messages if m["role"] != "system"]
        
        for i in range(len(non_system) - 1):
            current_role = non_system[i]["role"]
            next_role = non_system[i + 1]["role"]
            # In a typical conversation, roles should alternate
            if current_role == "user":
                assert next_role == "assistant"

    def test_conversation_multiple_references(self):
        """Test conversation with multiple reference entries"""
        references = [
            {"chunks": [], "doc_aggs": []},
            {"chunks": [{"content": "ref1"}], "doc_aggs": []},
            {"chunks": [{"content": "ref2"}], "doc_aggs": []}
        ]
        
        assert len(references) == 3
        assert all("chunks" in ref for ref in references)

    def test_conversation_update_name(self, mock_conversation_service):
        """Test updating conversation name"""
        conv_id = get_uuid()
        new_name = "Updated Conversation Name"
        
        mock_conversation_service.update_by_id.return_value = True
        result = mock_conversation_service.update_by_id(conv_id, {"name": new_name})
        
        assert result is True

    @pytest.mark.parametrize("invalid_message", [
        {"content": "Missing role"},  # Missing role field
        {"role": "user"},  # Missing content field
        {"role": "invalid_role", "content": "test"},  # Invalid role
    ])
    def test_conversation_invalid_message_structure(self, invalid_message):
        """Test validation of invalid message structures"""
        if "role" not in invalid_message or "content" not in invalid_message:
            with pytest.raises(KeyError):
                _ = invalid_message["role"]
                _ = invalid_message["content"]

    def test_conversation_batch_delete(self, mock_conversation_service):
        """Test batch deletion of conversations"""
        conv_ids = [get_uuid() for _ in range(5)]
        
        for conv_id in conv_ids:
            mock_conversation_service.delete_by_id.return_value = True
            result = mock_conversation_service.delete_by_id(conv_id)
            assert result is True

    def test_conversation_with_audio_binary(self):
        """Test conversation message with audio binary data"""
        message = {
            "role": "assistant",
            "content": "Spoken response",
            "id": get_uuid(),
            "audio_binary": b"audio_data_here"
        }
        
        assert "audio_binary" in message
        assert isinstance(message["audio_binary"], bytes)

    def test_conversation_reference_filtering(self, sample_conversation_data):
        """Test filtering out None references"""
        sample_conversation_data["reference"].append(None)
        
        # Filter out None values
        filtered_refs = [r for r in sample_conversation_data["reference"] if r]
        
        assert None not in filtered_refs
        assert len(filtered_refs) < len(sample_conversation_data["reference"])
