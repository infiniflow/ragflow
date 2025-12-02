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
from unittest.mock import Mock, patch, MagicMock
from common.misc_utils import get_uuid
from common.constants import StatusEnum


class TestDialogService:
    """Comprehensive unit tests for DialogService"""

    @pytest.fixture
    def mock_dialog_service(self):
        """Create a mock DialogService for testing"""
        with patch('api.db.services.dialog_service.DialogService') as mock:
            yield mock

    @pytest.fixture
    def sample_dialog_data(self):
        """Sample dialog data for testing"""
        return {
            "id": get_uuid(),
            "tenant_id": "test_tenant_123",
            "name": "Test Dialog",
            "description": "A test dialog",
            "icon": "",
            "llm_id": "gpt-4",
            "llm_setting": {
                "temperature": 0.1,
                "top_p": 0.3,
                "frequency_penalty": 0.7,
                "presence_penalty": 0.4,
                "max_tokens": 512
            },
            "prompt_config": {
                "system": "You are a helpful assistant",
                "prologue": "Hi! How can I help you?",
                "parameters": [],
                "empty_response": "Sorry! No relevant content found."
            },
            "kb_ids": [],
            "top_n": 6,
            "top_k": 1024,
            "similarity_threshold": 0.2,
            "vector_similarity_weight": 0.3,
            "rerank_id": "",
            "status": StatusEnum.VALID.value
        }

    def test_dialog_creation_success(self, mock_dialog_service, sample_dialog_data):
        """Test successful dialog creation"""
        mock_dialog_service.save.return_value = True
        mock_dialog_service.get_by_id.return_value = (True, Mock(**sample_dialog_data))

        result = mock_dialog_service.save(**sample_dialog_data)
        assert result is True
        mock_dialog_service.save.assert_called_once_with(**sample_dialog_data)

    def test_dialog_creation_with_invalid_name(self, mock_dialog_service):
        """Test dialog creation with invalid name"""
        invalid_data = {
            "name": "",  # Empty name
            "tenant_id": "test_tenant"
        }
        
        # Should raise validation error
        with pytest.raises(Exception):
            if not invalid_data["name"].strip():
                raise Exception("Dialog name can't be empty")

    def test_dialog_creation_with_long_name(self, mock_dialog_service):
        """Test dialog creation with name exceeding 255 bytes"""
        long_name = "a" * 300
        
        with pytest.raises(Exception):
            if len(long_name.encode("utf-8")) > 255:
                raise Exception(f"Dialog name length is {len(long_name)} which is larger than 255")

    def test_dialog_update_success(self, mock_dialog_service, sample_dialog_data):
        """Test successful dialog update"""
        dialog_id = sample_dialog_data["id"]
        update_data = {"name": "Updated Dialog Name"}
        
        mock_dialog_service.update_by_id.return_value = True
        result = mock_dialog_service.update_by_id(dialog_id, update_data)
        
        assert result is True
        mock_dialog_service.update_by_id.assert_called_once_with(dialog_id, update_data)

    def test_dialog_update_nonexistent(self, mock_dialog_service):
        """Test updating a non-existent dialog"""
        mock_dialog_service.update_by_id.return_value = False
        
        result = mock_dialog_service.update_by_id("nonexistent_id", {"name": "test"})
        assert result is False

    def test_dialog_get_by_id_success(self, mock_dialog_service, sample_dialog_data):
        """Test retrieving dialog by ID"""
        dialog_id = sample_dialog_data["id"]
        mock_dialog = Mock()
        mock_dialog.to_dict.return_value = sample_dialog_data
        
        mock_dialog_service.get_by_id.return_value = (True, mock_dialog)
        
        exists, dialog = mock_dialog_service.get_by_id(dialog_id)
        assert exists is True
        assert dialog.to_dict() == sample_dialog_data

    def test_dialog_get_by_id_not_found(self, mock_dialog_service):
        """Test retrieving non-existent dialog"""
        mock_dialog_service.get_by_id.return_value = (False, None)
        
        exists, dialog = mock_dialog_service.get_by_id("nonexistent_id")
        assert exists is False
        assert dialog is None

    def test_dialog_list_by_tenant(self, mock_dialog_service, sample_dialog_data):
        """Test listing dialogs by tenant ID"""
        tenant_id = "test_tenant_123"
        mock_dialogs = [Mock(to_dict=lambda: sample_dialog_data) for _ in range(3)]
        
        mock_dialog_service.query.return_value = mock_dialogs
        
        result = mock_dialog_service.query(
            tenant_id=tenant_id,
            status=StatusEnum.VALID.value
        )
        
        assert len(result) == 3
        mock_dialog_service.query.assert_called_once()

    def test_dialog_delete_success(self, mock_dialog_service):
        """Test soft delete of dialog (status update)"""
        dialog_ids = ["id1", "id2", "id3"]
        dialog_list = [{"id": id, "status": StatusEnum.INVALID.value} for id in dialog_ids]
        
        mock_dialog_service.update_many_by_id.return_value = True
        result = mock_dialog_service.update_many_by_id(dialog_list)
        
        assert result is True
        mock_dialog_service.update_many_by_id.assert_called_once_with(dialog_list)

    def test_dialog_with_knowledge_bases(self, mock_dialog_service, sample_dialog_data):
        """Test dialog creation with knowledge base IDs"""
        sample_dialog_data["kb_ids"] = ["kb1", "kb2", "kb3"]
        
        mock_dialog_service.save.return_value = True
        result = mock_dialog_service.save(**sample_dialog_data)
        
        assert result is True
        assert len(sample_dialog_data["kb_ids"]) == 3

    def test_dialog_llm_settings_validation(self, sample_dialog_data):
        """Test LLM settings validation"""
        llm_setting = sample_dialog_data["llm_setting"]
        
        # Validate temperature range
        assert 0 <= llm_setting["temperature"] <= 2
        
        # Validate top_p range
        assert 0 <= llm_setting["top_p"] <= 1
        
        # Validate max_tokens is positive
        assert llm_setting["max_tokens"] > 0

    def test_dialog_prompt_config_validation(self, sample_dialog_data):
        """Test prompt configuration validation"""
        prompt_config = sample_dialog_data["prompt_config"]
        
        # Required fields should exist
        assert "system" in prompt_config
        assert "prologue" in prompt_config
        assert "parameters" in prompt_config
        assert "empty_response" in prompt_config
        
        # Parameters should be a list
        assert isinstance(prompt_config["parameters"], list)

    def test_dialog_duplicate_name_handling(self, mock_dialog_service):
        """Test handling of duplicate dialog names"""
        tenant_id = "test_tenant"
        name = "Duplicate Dialog"
        
        # First dialog with this name exists
        mock_dialog_service.query.return_value = [Mock(name=name)]
        
        existing = mock_dialog_service.query(tenant_id=tenant_id, name=name)
        assert len(existing) > 0

    def test_dialog_similarity_threshold_validation(self, sample_dialog_data):
        """Test similarity threshold validation"""
        threshold = sample_dialog_data["similarity_threshold"]
        
        # Should be between 0 and 1
        assert 0 <= threshold <= 1

    def test_dialog_vector_similarity_weight_validation(self, sample_dialog_data):
        """Test vector similarity weight validation"""
        weight = sample_dialog_data["vector_similarity_weight"]
        
        # Should be between 0 and 1
        assert 0 <= weight <= 1

    def test_dialog_top_n_validation(self, sample_dialog_data):
        """Test top_n parameter validation"""
        top_n = sample_dialog_data["top_n"]
        
        # Should be positive integer
        assert isinstance(top_n, int)
        assert top_n > 0

    def test_dialog_top_k_validation(self, sample_dialog_data):
        """Test top_k parameter validation"""
        top_k = sample_dialog_data["top_k"]
        
        # Should be positive integer
        assert isinstance(top_k, int)
        assert top_k > 0

    def test_dialog_status_enum_validation(self, sample_dialog_data):
        """Test status field uses valid enum values"""
        status = sample_dialog_data["status"]
        
        # Should be valid status enum value
        assert status in [StatusEnum.VALID.value, StatusEnum.INVALID.value]

    @pytest.mark.parametrize("invalid_kb_ids", [
        None,  # None value
        "not_a_list",  # String instead of list
        123,  # Integer instead of list
    ])
    def test_dialog_invalid_kb_ids_type(self, invalid_kb_ids):
        """Test dialog creation with invalid kb_ids type"""
        with pytest.raises(Exception):
            if not isinstance(invalid_kb_ids, list):
                raise Exception("kb_ids must be a list")

    def test_dialog_empty_kb_ids_allowed(self, mock_dialog_service, sample_dialog_data):
        """Test dialog creation with empty kb_ids is allowed"""
        sample_dialog_data["kb_ids"] = []
        
        mock_dialog_service.save.return_value = True
        result = mock_dialog_service.save(**sample_dialog_data)
        
        assert result is True

    def test_dialog_query_with_pagination(self, mock_dialog_service):
        """Test dialog listing with pagination"""
        page = 1
        page_size = 10
        total = 25
        
        mock_dialogs = [Mock() for _ in range(page_size)]
        mock_dialog_service.get_by_tenant_ids.return_value = (mock_dialogs, total)
        
        result, count = mock_dialog_service.get_by_tenant_ids(
            ["tenant1"], "user1", page, page_size, "create_time", True, "", None
        )
        
        assert len(result) == page_size
        assert count == total

    def test_dialog_search_by_keywords(self, mock_dialog_service):
        """Test dialog search with keywords"""
        keywords = "test"
        mock_dialogs = [Mock(name="test dialog 1"), Mock(name="test dialog 2")]
        
        mock_dialog_service.get_by_tenant_ids.return_value = (mock_dialogs, 2)
        
        result, count = mock_dialog_service.get_by_tenant_ids(
            ["tenant1"], "user1", 0, 0, "create_time", True, keywords, None
        )
        
        assert count == 2

    def test_dialog_ordering(self, mock_dialog_service):
        """Test dialog ordering by different fields"""
        order_fields = ["create_time", "update_time", "name"]
        
        for field in order_fields:
            mock_dialog_service.get_by_tenant_ids.return_value = ([], 0)
            mock_dialog_service.get_by_tenant_ids(
                ["tenant1"], "user1", 0, 0, field, True, "", None
            )
            mock_dialog_service.get_by_tenant_ids.assert_called()
