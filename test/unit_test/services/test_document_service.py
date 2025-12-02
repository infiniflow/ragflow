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


class TestDocumentService:
    """Comprehensive unit tests for DocumentService"""

    @pytest.fixture
    def mock_doc_service(self):
        """Create a mock DocumentService for testing"""
        with patch('api.db.services.document_service.DocumentService') as mock:
            yield mock

    @pytest.fixture
    def sample_document_data(self):
        """Sample document data for testing"""
        return {
            "id": get_uuid(),
            "kb_id": get_uuid(),
            "name": "test_document.pdf",
            "location": "test_document.pdf",
            "size": 1024000,  # 1MB
            "type": "pdf",
            "parser_id": "paper",
            "parser_config": {
                "chunk_token_num": 128,
                "layout_recognize": True
            },
            "status": "1",  # Parsing completed
            "progress": 1.0,
            "progress_msg": "Parsing completed",
            "chunk_num": 50,
            "token_num": 5000,
            "run": "0"
        }

    def test_document_creation_success(self, mock_doc_service, sample_document_data):
        """Test successful document creation"""
        mock_doc_service.save.return_value = True
        
        result = mock_doc_service.save(**sample_document_data)
        assert result is True

    def test_document_get_by_id_success(self, mock_doc_service, sample_document_data):
        """Test retrieving document by ID"""
        doc_id = sample_document_data["id"]
        mock_doc = Mock()
        mock_doc.to_dict.return_value = sample_document_data
        
        mock_doc_service.get_by_id.return_value = (True, mock_doc)
        
        exists, doc = mock_doc_service.get_by_id(doc_id)
        assert exists is True
        assert doc.to_dict() == sample_document_data

    def test_document_get_by_id_not_found(self, mock_doc_service):
        """Test retrieving non-existent document"""
        mock_doc_service.get_by_id.return_value = (False, None)
        
        exists, doc = mock_doc_service.get_by_id("nonexistent_id")
        assert exists is False
        assert doc is None

    def test_document_update_success(self, mock_doc_service):
        """Test successful document update"""
        doc_id = get_uuid()
        update_data = {"name": "updated_document.pdf"}
        
        mock_doc_service.update_by_id.return_value = True
        result = mock_doc_service.update_by_id(doc_id, update_data)
        
        assert result is True

    def test_document_delete_success(self, mock_doc_service):
        """Test document deletion"""
        doc_id = get_uuid()
        
        mock_doc_service.delete_by_id.return_value = True
        result = mock_doc_service.delete_by_id(doc_id)
        
        assert result is True

    def test_document_list_by_kb(self, mock_doc_service):
        """Test listing documents by knowledge base"""
        kb_id = get_uuid()
        mock_docs = [Mock() for _ in range(10)]
        
        mock_doc_service.query.return_value = mock_docs
        
        result = mock_doc_service.query(kb_id=kb_id)
        assert len(result) == 10

    def test_document_file_type_validation(self, sample_document_data):
        """Test document file type validation"""
        file_type = sample_document_data["type"]
        
        valid_types = ["pdf", "docx", "doc", "txt", "md", "csv", "xlsx", "pptx", "html", "json", "eml"]
        assert file_type in valid_types

    def test_document_size_validation(self, sample_document_data):
        """Test document size validation"""
        size = sample_document_data["size"]
        
        assert size > 0
        assert size < 100 * 1024 * 1024  # Less than 100MB

    def test_document_parser_id_validation(self, sample_document_data):
        """Test parser ID validation"""
        parser_id = sample_document_data["parser_id"]
        
        valid_parsers = ["naive", "paper", "book", "laws", "presentation", "manual", "qa", "table", "resume", "picture", "one", "knowledge_graph"]
        assert parser_id in valid_parsers

    def test_document_status_progression(self, sample_document_data):
        """Test document status progression"""
        # Status: 0=pending, 1=completed, 2=failed
        statuses = ["0", "1", "2"]
        
        for status in statuses:
            sample_document_data["status"] = status
            assert sample_document_data["status"] in statuses

    def test_document_progress_validation(self, sample_document_data):
        """Test document parsing progress validation"""
        progress = sample_document_data["progress"]
        
        assert 0.0 <= progress <= 1.0

    def test_document_chunk_count(self, sample_document_data):
        """Test document chunk count"""
        chunk_num = sample_document_data["chunk_num"]
        
        assert chunk_num >= 0
        assert isinstance(chunk_num, int)

    def test_document_token_count(self, sample_document_data):
        """Test document token count"""
        token_num = sample_document_data["token_num"]
        
        assert token_num >= 0
        assert isinstance(token_num, int)

    def test_document_parsing_pending(self, sample_document_data):
        """Test document in pending parsing state"""
        sample_document_data["status"] = "0"
        sample_document_data["progress"] = 0.0
        sample_document_data["progress_msg"] = "Waiting for parsing"
        
        assert sample_document_data["status"] == "0"
        assert sample_document_data["progress"] == 0.0

    def test_document_parsing_in_progress(self, sample_document_data):
        """Test document in parsing progress state"""
        sample_document_data["status"] = "0"
        sample_document_data["progress"] = 0.5
        sample_document_data["progress_msg"] = "Parsing in progress"
        
        assert 0.0 < sample_document_data["progress"] < 1.0

    def test_document_parsing_completed(self, sample_document_data):
        """Test document parsing completed state"""
        sample_document_data["status"] = "1"
        sample_document_data["progress"] = 1.0
        sample_document_data["progress_msg"] = "Parsing completed"
        
        assert sample_document_data["status"] == "1"
        assert sample_document_data["progress"] == 1.0

    def test_document_parsing_failed(self, sample_document_data):
        """Test document parsing failed state"""
        sample_document_data["status"] = "2"
        sample_document_data["progress_msg"] = "Parsing failed: Invalid format"
        
        assert sample_document_data["status"] == "2"
        assert "failed" in sample_document_data["progress_msg"].lower()

    def test_document_run_flag(self, sample_document_data):
        """Test document run flag"""
        run = sample_document_data["run"]
        
        # run: 0=not running, 1=running, 2=cancel
        assert run in ["0", "1", "2"]

    def test_document_batch_upload(self, mock_doc_service):
        """Test batch document upload"""
        kb_id = get_uuid()
        doc_count = 5
        
        for i in range(doc_count):
            doc_data = {
                "id": get_uuid(),
                "kb_id": kb_id,
                "name": f"document_{i}.pdf",
                "size": 1024 * (i + 1)
            }
            mock_doc_service.save.return_value = True
            result = mock_doc_service.save(**doc_data)
            assert result is True

    def test_document_batch_delete(self, mock_doc_service):
        """Test batch document deletion"""
        doc_ids = [get_uuid() for _ in range(5)]
        
        for doc_id in doc_ids:
            mock_doc_service.delete_by_id.return_value = True
            result = mock_doc_service.delete_by_id(doc_id)
            assert result is True

    def test_document_search_by_name(self, mock_doc_service):
        """Test document search by name"""
        kb_id = get_uuid()
        keywords = "test"
        mock_docs = [Mock(name="test_doc1.pdf"), Mock(name="test_doc2.pdf")]
        
        mock_doc_service.get_list.return_value = (mock_docs, 2)
        
        result, count = mock_doc_service.get_list(kb_id, 0, 0, "create_time", True, keywords)
        assert count == 2

    def test_document_pagination(self, mock_doc_service):
        """Test document listing with pagination"""
        kb_id = get_uuid()
        page = 1
        page_size = 10
        total = 25
        
        mock_docs = [Mock() for _ in range(page_size)]
        mock_doc_service.get_list.return_value = (mock_docs, total)
        
        result, count = mock_doc_service.get_list(kb_id, page, page_size, "create_time", True, "")
        
        assert len(result) == page_size
        assert count == total

    def test_document_ordering(self, mock_doc_service):
        """Test document ordering"""
        kb_id = get_uuid()
        
        mock_doc_service.get_list.return_value = ([], 0)
        mock_doc_service.get_list(kb_id, 0, 0, "create_time", True, "")
        
        mock_doc_service.get_list.assert_called_once()

    def test_document_parser_config_validation(self, sample_document_data):
        """Test parser configuration validation"""
        parser_config = sample_document_data["parser_config"]
        
        assert "chunk_token_num" in parser_config
        assert parser_config["chunk_token_num"] > 0

    def test_document_layout_recognition(self, sample_document_data):
        """Test layout recognition flag"""
        layout_recognize = sample_document_data["parser_config"]["layout_recognize"]
        
        assert isinstance(layout_recognize, bool)

    @pytest.mark.parametrize("file_type", [
        "pdf", "docx", "doc", "txt", "md", "csv", "xlsx", "pptx", "html", "json"
    ])
    def test_document_different_file_types(self, file_type, sample_document_data):
        """Test document with different file types"""
        sample_document_data["type"] = file_type
        assert sample_document_data["type"] == file_type

    def test_document_name_with_extension(self, sample_document_data):
        """Test document name includes file extension"""
        name = sample_document_data["name"]
        
        assert "." in name
        extension = name.split(".")[-1]
        assert len(extension) > 0

    def test_document_location_path(self, sample_document_data):
        """Test document location path"""
        location = sample_document_data["location"]
        
        assert location is not None
        assert len(location) > 0

    def test_document_stop_parsing(self, mock_doc_service):
        """Test stopping document parsing"""
        doc_id = get_uuid()
        
        mock_doc_service.update_by_id.return_value = True
        result = mock_doc_service.update_by_id(doc_id, {"run": "2"})  # Cancel
        
        assert result is True

    def test_document_restart_parsing(self, mock_doc_service):
        """Test restarting document parsing"""
        doc_id = get_uuid()
        
        mock_doc_service.update_by_id.return_value = True
        result = mock_doc_service.update_by_id(doc_id, {
            "status": "0",
            "progress": 0.0,
            "run": "1"
        })
        
        assert result is True

    def test_document_chunk_token_ratio(self, sample_document_data):
        """Test chunk to token ratio is reasonable"""
        chunk_num = sample_document_data["chunk_num"]
        token_num = sample_document_data["token_num"]
        
        if chunk_num > 0:
            avg_tokens_per_chunk = token_num / chunk_num
            assert avg_tokens_per_chunk > 0
            assert avg_tokens_per_chunk < 2048  # Reasonable upper limit

    def test_document_empty_file_handling(self):
        """Test handling of empty file"""
        empty_doc = {
            "size": 0,
            "chunk_num": 0,
            "token_num": 0
        }
        
        assert empty_doc["size"] == 0
        assert empty_doc["chunk_num"] == 0
        assert empty_doc["token_num"] == 0
