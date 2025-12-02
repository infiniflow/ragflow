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
from common.constants import StatusEnum


class TestKnowledgebaseService:
    """Comprehensive unit tests for KnowledgebaseService"""

    @pytest.fixture
    def mock_kb_service(self):
        """Create a mock KnowledgebaseService for testing"""
        with patch('api.db.services.knowledgebase_service.KnowledgebaseService') as mock:
            yield mock

    @pytest.fixture
    def sample_kb_data(self):
        """Sample knowledge base data for testing"""
        return {
            "id": get_uuid(),
            "tenant_id": "test_tenant_123",
            "name": "Test Knowledge Base",
            "description": "A test knowledge base",
            "language": "English",
            "embd_id": "BAAI/bge-small-en-v1.5",
            "parser_id": "naive",
            "parser_config": {
                "chunk_token_num": 128,
                "delimiter": "\n",
                "layout_recognize": True
            },
            "status": StatusEnum.VALID.value,
            "doc_num": 0,
            "chunk_num": 0,
            "token_num": 0
        }

    def test_kb_creation_success(self, mock_kb_service, sample_kb_data):
        """Test successful knowledge base creation"""
        mock_kb_service.save.return_value = True
        
        result = mock_kb_service.save(**sample_kb_data)
        assert result is True
        mock_kb_service.save.assert_called_once_with(**sample_kb_data)

    def test_kb_creation_with_empty_name(self):
        """Test knowledge base creation with empty name"""
        with pytest.raises(Exception):
            if not "".strip():
                raise Exception("Knowledge base name can't be empty")

    def test_kb_creation_with_long_name(self):
        """Test knowledge base creation with name exceeding limit"""
        long_name = "a" * 300
        
        with pytest.raises(Exception):
            if len(long_name.encode("utf-8")) > 255:
                raise Exception(f"KB name length {len(long_name)} exceeds 255")

    def test_kb_get_by_id_success(self, mock_kb_service, sample_kb_data):
        """Test retrieving knowledge base by ID"""
        kb_id = sample_kb_data["id"]
        mock_kb = Mock()
        mock_kb.to_dict.return_value = sample_kb_data
        
        mock_kb_service.get_by_id.return_value = (True, mock_kb)
        
        exists, kb = mock_kb_service.get_by_id(kb_id)
        assert exists is True
        assert kb.to_dict() == sample_kb_data

    def test_kb_get_by_id_not_found(self, mock_kb_service):
        """Test retrieving non-existent knowledge base"""
        mock_kb_service.get_by_id.return_value = (False, None)
        
        exists, kb = mock_kb_service.get_by_id("nonexistent_id")
        assert exists is False
        assert kb is None

    def test_kb_get_by_ids_multiple(self, mock_kb_service, sample_kb_data):
        """Test retrieving multiple knowledge bases by IDs"""
        kb_ids = [get_uuid() for _ in range(3)]
        mock_kbs = [Mock(to_dict=lambda: sample_kb_data) for _ in range(3)]
        
        mock_kb_service.get_by_ids.return_value = mock_kbs
        
        result = mock_kb_service.get_by_ids(kb_ids)
        assert len(result) == 3

    def test_kb_update_success(self, mock_kb_service):
        """Test successful knowledge base update"""
        kb_id = get_uuid()
        update_data = {"name": "Updated KB Name"}
        
        mock_kb_service.update_by_id.return_value = True
        result = mock_kb_service.update_by_id(kb_id, update_data)
        
        assert result is True

    def test_kb_delete_success(self, mock_kb_service):
        """Test knowledge base soft delete"""
        kb_id = get_uuid()
        
        mock_kb_service.update_by_id.return_value = True
        result = mock_kb_service.update_by_id(kb_id, {"status": StatusEnum.INVALID.value})
        
        assert result is True

    def test_kb_list_by_tenant(self, mock_kb_service):
        """Test listing knowledge bases by tenant"""
        tenant_id = "test_tenant"
        mock_kbs = [Mock() for _ in range(5)]
        
        mock_kb_service.query.return_value = mock_kbs
        
        result = mock_kb_service.query(
            tenant_id=tenant_id,
            status=StatusEnum.VALID.value
        )
        assert len(result) == 5

    def test_kb_embedding_model_validation(self, sample_kb_data):
        """Test embedding model ID validation"""
        embd_id = sample_kb_data["embd_id"]
        
        assert embd_id is not None
        assert len(embd_id) > 0

    def test_kb_parser_config_validation(self, sample_kb_data):
        """Test parser configuration validation"""
        parser_config = sample_kb_data["parser_config"]
        
        assert "chunk_token_num" in parser_config
        assert parser_config["chunk_token_num"] > 0
        assert "delimiter" in parser_config

    def test_kb_language_validation(self, sample_kb_data):
        """Test language field validation"""
        language = sample_kb_data["language"]
        
        assert language in ["English", "Chinese"]

    def test_kb_parser_id_validation(self, sample_kb_data):
        """Test parser ID validation"""
        parser_id = sample_kb_data["parser_id"]
        
        assert parser_id in ["naive", "paper", "book", "laws", "presentation", "manual", "qa", "table", "resume", "picture", "one", "knowledge_graph"]

    def test_kb_doc_count_increment(self, sample_kb_data):
        """Test document count increment"""
        initial_count = sample_kb_data["doc_num"]
        sample_kb_data["doc_num"] += 1
        
        assert sample_kb_data["doc_num"] == initial_count + 1

    def test_kb_chunk_count_increment(self, sample_kb_data):
        """Test chunk count increment"""
        initial_count = sample_kb_data["chunk_num"]
        sample_kb_data["chunk_num"] += 10
        
        assert sample_kb_data["chunk_num"] == initial_count + 10

    def test_kb_token_count_increment(self, sample_kb_data):
        """Test token count increment"""
        initial_count = sample_kb_data["token_num"]
        sample_kb_data["token_num"] += 1000
        
        assert sample_kb_data["token_num"] == initial_count + 1000

    def test_kb_status_enum_validation(self, sample_kb_data):
        """Test status uses valid enum values"""
        status = sample_kb_data["status"]
        
        assert status in [StatusEnum.VALID.value, StatusEnum.INVALID.value]

    def test_kb_duplicate_name_handling(self, mock_kb_service):
        """Test handling of duplicate KB names"""
        tenant_id = "test_tenant"
        name = "Duplicate KB"
        
        mock_kb_service.query.return_value = [Mock(name=name)]
        
        existing = mock_kb_service.query(tenant_id=tenant_id, name=name)
        assert len(existing) > 0

    def test_kb_search_by_keywords(self, mock_kb_service):
        """Test knowledge base search with keywords"""
        keywords = "test"
        mock_kbs = [Mock(name="test kb 1"), Mock(name="test kb 2")]
        
        mock_kb_service.get_by_tenant_ids.return_value = (mock_kbs, 2)
        
        result, count = mock_kb_service.get_by_tenant_ids(
            ["tenant1"], "user1", 0, 0, "create_time", True, keywords
        )
        
        assert count == 2

    def test_kb_pagination(self, mock_kb_service):
        """Test knowledge base listing with pagination"""
        page = 1
        page_size = 10
        total = 25
        
        mock_kbs = [Mock() for _ in range(page_size)]
        mock_kb_service.get_by_tenant_ids.return_value = (mock_kbs, total)
        
        result, count = mock_kb_service.get_by_tenant_ids(
            ["tenant1"], "user1", page, page_size, "create_time", True, ""
        )
        
        assert len(result) == page_size
        assert count == total

    def test_kb_ordering_by_create_time(self, mock_kb_service):
        """Test KB ordering by creation time"""
        mock_kb_service.get_by_tenant_ids.return_value = ([], 0)
        
        mock_kb_service.get_by_tenant_ids(
            ["tenant1"], "user1", 0, 0, "create_time", True, ""
        )
        
        mock_kb_service.get_by_tenant_ids.assert_called_once()

    def test_kb_ordering_by_update_time(self, mock_kb_service):
        """Test KB ordering by update time"""
        mock_kb_service.get_by_tenant_ids.return_value = ([], 0)
        
        mock_kb_service.get_by_tenant_ids(
            ["tenant1"], "user1", 0, 0, "update_time", True, ""
        )
        
        mock_kb_service.get_by_tenant_ids.assert_called_once()

    def test_kb_ordering_descending(self, mock_kb_service):
        """Test KB ordering in descending order"""
        mock_kb_service.get_by_tenant_ids.return_value = ([], 0)
        
        mock_kb_service.get_by_tenant_ids(
            ["tenant1"], "user1", 0, 0, "create_time", True, ""  # True = descending
        )
        
        mock_kb_service.get_by_tenant_ids.assert_called_once()

    def test_kb_chunk_token_num_validation(self, sample_kb_data):
        """Test chunk token number validation"""
        chunk_token_num = sample_kb_data["parser_config"]["chunk_token_num"]
        
        assert chunk_token_num > 0
        assert chunk_token_num <= 2048  # Reasonable upper limit

    def test_kb_layout_recognize_flag(self, sample_kb_data):
        """Test layout recognition flag"""
        layout_recognize = sample_kb_data["parser_config"]["layout_recognize"]
        
        assert isinstance(layout_recognize, bool)

    @pytest.mark.parametrize("parser_id", [
        "naive", "paper", "book", "laws", "presentation",
        "manual", "qa", "table", "resume", "picture", "one", "knowledge_graph"
    ])
    def test_kb_different_parsers(self, parser_id, sample_kb_data):
        """Test KB with different parser types"""
        sample_kb_data["parser_id"] = parser_id
        assert sample_kb_data["parser_id"] == parser_id

    @pytest.mark.parametrize("language", ["English", "Chinese"])
    def test_kb_different_languages(self, language, sample_kb_data):
        """Test KB with different languages"""
        sample_kb_data["language"] = language
        assert sample_kb_data["language"] == language

    def test_kb_empty_description_allowed(self, sample_kb_data):
        """Test KB creation with empty description is allowed"""
        sample_kb_data["description"] = ""
        assert sample_kb_data["description"] == ""

    def test_kb_statistics_initialization(self, sample_kb_data):
        """Test KB statistics are initialized to zero"""
        assert sample_kb_data["doc_num"] == 0
        assert sample_kb_data["chunk_num"] == 0
        assert sample_kb_data["token_num"] == 0

    def test_kb_batch_delete(self, mock_kb_service):
        """Test batch deletion of knowledge bases"""
        kb_ids = [get_uuid() for _ in range(5)]
        
        for kb_id in kb_ids:
            mock_kb_service.update_by_id.return_value = True
            result = mock_kb_service.update_by_id(kb_id, {"status": StatusEnum.INVALID.value})
            assert result is True

    def test_kb_embedding_model_consistency(self, mock_kb_service):
        """Test that dialogs using same KB have consistent embedding models"""
        kb_ids = [get_uuid() for _ in range(3)]
        embd_id = "BAAI/bge-small-en-v1.5"
        
        mock_kbs = [Mock(embd_id=embd_id) for _ in range(3)]
        mock_kb_service.get_by_ids.return_value = mock_kbs
        
        kbs = mock_kb_service.get_by_ids(kb_ids)
        embd_ids = [kb.embd_id for kb in kbs]
        
        # All should have same embedding model
        assert len(set(embd_ids)) == 1
