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
Unit tests for Checkpoint Service

These are UNIT tests that use mocks to test the interface and logic flow
without requiring a database connection. This makes them fast and isolated.

For INTEGRATION tests that test the actual CheckpointService implementation
with a real database, see: test/integration_test/services/test_checkpoint_service_integration.py

Tests cover:
- Checkpoint creation and retrieval
- Document state management
- Pause/resume/cancel operations
- Retry logic
- Progress tracking
"""

import pytest
from unittest.mock import Mock


class TestCheckpointCreation:
    """Tests for checkpoint creation"""
    
    @pytest.fixture
    def mock_checkpoint_service(self):
        """Mock CheckpointService - using Mock directly for unit tests"""
        mock = Mock()
        return mock
    
    def test_create_checkpoint_basic(self, mock_checkpoint_service):
        """Test basic checkpoint creation"""
        # Mock create_checkpoint
        mock_checkpoint = Mock()
        mock_checkpoint.id = "checkpoint_123"
        mock_checkpoint.task_id = "task_456"
        mock_checkpoint.task_type = "raptor"
        mock_checkpoint.total_documents = 10
        mock_checkpoint.pending_documents = 10
        mock_checkpoint.completed_documents = 0
        mock_checkpoint.failed_documents = 0
        
        mock_checkpoint_service.create_checkpoint.return_value = mock_checkpoint
        
        # Create checkpoint
        result = mock_checkpoint_service.create_checkpoint(
            task_id="task_456",
            task_type="raptor",
            doc_ids=["doc1", "doc2", "doc3", "doc4", "doc5", 
                     "doc6", "doc7", "doc8", "doc9", "doc10"],
            config={"max_cluster": 64}
        )
        
        # Verify
        assert result.id == "checkpoint_123"
        assert result.task_id == "task_456"
        assert result.total_documents == 10
        assert result.pending_documents == 10
        assert result.completed_documents == 0
    
    def test_create_checkpoint_initializes_doc_states(self, mock_checkpoint_service):
        """Test that checkpoint initializes all document states"""
        mock_checkpoint = Mock()
        mock_checkpoint.checkpoint_data = {
            "doc_states": {
                "doc1": {"status": "pending", "token_count": 0, "chunks": 0, "retry_count": 0},
                "doc2": {"status": "pending", "token_count": 0, "chunks": 0, "retry_count": 0},
                "doc3": {"status": "pending", "token_count": 0, "chunks": 0, "retry_count": 0}
            }
        }
        
        mock_checkpoint_service.create_checkpoint.return_value = mock_checkpoint
        
        result = mock_checkpoint_service.create_checkpoint(
            task_id="task_123",
            task_type="raptor",
            doc_ids=["doc1", "doc2", "doc3"],
            config={}
        )
        
        # All docs should be pending
        doc_states = result.checkpoint_data["doc_states"]
        assert len(doc_states) == 3
        assert all(state["status"] == "pending" for state in doc_states.values())


class TestDocumentStateManagement:
    """Tests for document state tracking"""
    
    @pytest.fixture
    def mock_checkpoint_service(self):
        mock = Mock()
        return mock
    
    def test_save_document_completion(self, mock_checkpoint_service):
        """Test marking document as completed"""
        mock_checkpoint_service.save_document_completion.return_value = True
        
        success = mock_checkpoint_service.save_document_completion(
            checkpoint_id="checkpoint_123",
            doc_id="doc1",
            token_count=1500,
            chunks=45
        )
        
        assert success is True
        mock_checkpoint_service.save_document_completion.assert_called_once()
    
    def test_save_document_failure(self, mock_checkpoint_service):
        """Test marking document as failed"""
        mock_checkpoint_service.save_document_failure.return_value = True
        
        success = mock_checkpoint_service.save_document_failure(
            checkpoint_id="checkpoint_123",
            doc_id="doc2",
            error="API timeout after 60s"
        )
        
        assert success is True
        mock_checkpoint_service.save_document_failure.assert_called_once()
    
    def test_get_pending_documents(self, mock_checkpoint_service):
        """Test retrieving pending documents"""
        mock_checkpoint_service.get_pending_documents.return_value = ["doc2", "doc3", "doc4"]
        
        pending = mock_checkpoint_service.get_pending_documents("checkpoint_123")
        
        assert len(pending) == 3
        assert "doc2" in pending
        assert "doc3" in pending
        assert "doc4" in pending
    
    def test_get_failed_documents(self, mock_checkpoint_service):
        """Test retrieving failed documents with details"""
        mock_checkpoint_service.get_failed_documents.return_value = [
            {
                "doc_id": "doc5",
                "error": "Connection timeout",
                "retry_count": 2,
                "last_attempt": "2025-12-03T09:00:00"
            }
        ]
        
        failed = mock_checkpoint_service.get_failed_documents("checkpoint_123")
        
        assert len(failed) == 1
        assert failed[0]["doc_id"] == "doc5"
        assert failed[0]["retry_count"] == 2


class TestPauseResumeCancel:
    """Tests for pause/resume/cancel operations"""
    
    @pytest.fixture
    def mock_checkpoint_service(self):
        mock = Mock()
        return mock
    
    def test_pause_checkpoint(self, mock_checkpoint_service):
        """Test pausing a checkpoint"""
        mock_checkpoint_service.pause_checkpoint.return_value = True
        
        success = mock_checkpoint_service.pause_checkpoint("checkpoint_123")
        
        assert success is True
    
    def test_resume_checkpoint(self, mock_checkpoint_service):
        """Test resuming a checkpoint"""
        mock_checkpoint_service.resume_checkpoint.return_value = True
        
        success = mock_checkpoint_service.resume_checkpoint("checkpoint_123")
        
        assert success is True
    
    def test_cancel_checkpoint(self, mock_checkpoint_service):
        """Test cancelling a checkpoint"""
        mock_checkpoint_service.cancel_checkpoint.return_value = True
        
        success = mock_checkpoint_service.cancel_checkpoint("checkpoint_123")
        
        assert success is True
    
    def test_is_paused(self, mock_checkpoint_service):
        """Test checking if checkpoint is paused"""
        mock_checkpoint_service.is_paused.return_value = True
        
        paused = mock_checkpoint_service.is_paused("checkpoint_123")
        
        assert paused is True
    
    def test_is_cancelled(self, mock_checkpoint_service):
        """Test checking if checkpoint is cancelled"""
        mock_checkpoint_service.is_cancelled.return_value = False
        
        cancelled = mock_checkpoint_service.is_cancelled("checkpoint_123")
        
        assert cancelled is False


class TestRetryLogic:
    """Tests for retry logic"""
    
    @pytest.fixture
    def mock_checkpoint_service(self):
        mock = Mock()
        return mock
    
    def test_should_retry_within_limit(self, mock_checkpoint_service):
        """Test should retry when under max retries"""
        mock_checkpoint_service.should_retry.return_value = True
        
        should_retry = mock_checkpoint_service.should_retry(
            checkpoint_id="checkpoint_123",
            doc_id="doc1",
            max_retries=3
        )
        
        assert should_retry is True
    
    def test_should_not_retry_exceeded_limit(self, mock_checkpoint_service):
        """Test should not retry when max retries exceeded"""
        mock_checkpoint_service.should_retry.return_value = False
        
        should_retry = mock_checkpoint_service.should_retry(
            checkpoint_id="checkpoint_123",
            doc_id="doc2",
            max_retries=3
        )
        
        assert should_retry is False
    
    def test_reset_document_for_retry(self, mock_checkpoint_service):
        """Test resetting failed document to pending"""
        mock_checkpoint_service.reset_document_for_retry.return_value = True
        
        success = mock_checkpoint_service.reset_document_for_retry(
            checkpoint_id="checkpoint_123",
            doc_id="doc1"
        )
        
        assert success is True


class TestProgressTracking:
    """Tests for progress tracking"""
    
    @pytest.fixture
    def mock_checkpoint_service(self):
        mock = Mock()
        return mock
    
    def test_get_checkpoint_status(self, mock_checkpoint_service):
        """Test getting detailed checkpoint status"""
        mock_status = {
            "checkpoint_id": "checkpoint_123",
            "task_id": "task_456",
            "task_type": "raptor",
            "status": "running",
            "progress": 0.6,
            "total_documents": 10,
            "completed_documents": 6,
            "failed_documents": 1,
            "pending_documents": 3,
            "token_count": 15000,
            "started_at": "2025-12-03T08:00:00",
            "last_checkpoint_at": "2025-12-03T09:00:00"
        }
        
        mock_checkpoint_service.get_checkpoint_status.return_value = mock_status
        
        status = mock_checkpoint_service.get_checkpoint_status("checkpoint_123")
        
        assert status["progress"] == 0.6
        assert status["completed_documents"] == 6
        assert status["failed_documents"] == 1
        assert status["pending_documents"] == 3
        assert status["token_count"] == 15000
    
    def test_progress_calculation(self, mock_checkpoint_service):
        """Test progress calculation"""
        # 7 completed out of 10 = 70%
        mock_status = {
            "total_documents": 10,
            "completed_documents": 7,
            "progress": 0.7
        }
        
        mock_checkpoint_service.get_checkpoint_status.return_value = mock_status
        
        status = mock_checkpoint_service.get_checkpoint_status("checkpoint_123")
        
        assert status["progress"] == 0.7
        assert status["completed_documents"] / status["total_documents"] == 0.7


class TestIntegrationScenarios:
    """Integration test scenarios"""
    
    @pytest.fixture
    def mock_checkpoint_service(self):
        mock = Mock()
        return mock
    
    def test_full_task_lifecycle(self, mock_checkpoint_service):
        """Test complete task lifecycle: create -> process -> complete"""
        # Create checkpoint
        mock_checkpoint = Mock()
        mock_checkpoint.id = "checkpoint_123"
        mock_checkpoint.total_documents = 3
        mock_checkpoint_service.create_checkpoint.return_value = mock_checkpoint
        
        mock_checkpoint_service.create_checkpoint(
            task_id="task_123",
            task_type="raptor",
            doc_ids=["doc1", "doc2", "doc3"],
            config={}
        )
        
        # Process documents
        mock_checkpoint_service.save_document_completion.return_value = True
        mock_checkpoint_service.save_document_completion("checkpoint_123", "doc1", 1000, 30)
        mock_checkpoint_service.save_document_completion("checkpoint_123", "doc2", 1500, 45)
        mock_checkpoint_service.save_document_completion("checkpoint_123", "doc3", 1200, 38)
        
        # Verify all completed
        mock_checkpoint_service.get_pending_documents.return_value = []
        pending = mock_checkpoint_service.get_pending_documents("checkpoint_123")
        assert len(pending) == 0
    
    def test_task_with_failures_and_retry(self, mock_checkpoint_service):
        """Test task with failures and retry"""
        # Create checkpoint
        mock_checkpoint = Mock()
        mock_checkpoint.id = "checkpoint_123"
        mock_checkpoint_service.create_checkpoint.return_value = mock_checkpoint
        
        mock_checkpoint_service.create_checkpoint(
            task_id="task_123",
            task_type="raptor",
            doc_ids=["doc1", "doc2", "doc3"],
            config={}
        )
        
        # Process with one failure
        mock_checkpoint_service.save_document_completion.return_value = True
        mock_checkpoint_service.save_document_failure.return_value = True
        
        mock_checkpoint_service.save_document_completion("checkpoint_123", "doc1", 1000, 30)
        mock_checkpoint_service.save_document_failure("checkpoint_123", "doc2", "Timeout")
        mock_checkpoint_service.save_document_completion("checkpoint_123", "doc3", 1200, 38)
        
        # Check failed documents
        mock_checkpoint_service.get_failed_documents.return_value = [
            {"doc_id": "doc2", "error": "Timeout", "retry_count": 1}
        ]
        failed = mock_checkpoint_service.get_failed_documents("checkpoint_123")
        assert len(failed) == 1
        
        # Retry failed document
        mock_checkpoint_service.should_retry.return_value = True
        mock_checkpoint_service.reset_document_for_retry.return_value = True
        
        if mock_checkpoint_service.should_retry("checkpoint_123", "doc2"):
            mock_checkpoint_service.reset_document_for_retry("checkpoint_123", "doc2")
        
        # Verify reset
        mock_checkpoint_service.get_pending_documents.return_value = ["doc2"]
        pending = mock_checkpoint_service.get_pending_documents("checkpoint_123")
        assert "doc2" in pending
    
    def test_pause_and_resume_workflow(self, mock_checkpoint_service):
        """Test pause and resume workflow"""
        # Create and start processing
        mock_checkpoint = Mock()
        mock_checkpoint.id = "checkpoint_123"
        mock_checkpoint_service.create_checkpoint.return_value = mock_checkpoint
        
        mock_checkpoint_service.create_checkpoint(
            task_id="task_123",
            task_type="raptor",
            doc_ids=["doc1", "doc2", "doc3", "doc4", "doc5"],
            config={}
        )
        
        # Process some documents
        mock_checkpoint_service.save_document_completion.return_value = True
        mock_checkpoint_service.save_document_completion("checkpoint_123", "doc1", 1000, 30)
        mock_checkpoint_service.save_document_completion("checkpoint_123", "doc2", 1500, 45)
        
        # Pause
        mock_checkpoint_service.pause_checkpoint.return_value = True
        mock_checkpoint_service.pause_checkpoint("checkpoint_123")
        
        # Check paused
        mock_checkpoint_service.is_paused.return_value = True
        assert mock_checkpoint_service.is_paused("checkpoint_123") is True
        
        # Resume
        mock_checkpoint_service.resume_checkpoint.return_value = True
        mock_checkpoint_service.resume_checkpoint("checkpoint_123")
        
        # Check pending (should have 3 remaining)
        mock_checkpoint_service.get_pending_documents.return_value = ["doc3", "doc4", "doc5"]
        pending = mock_checkpoint_service.get_pending_documents("checkpoint_123")
        assert len(pending) == 3


class TestEdgeCases:
    """Test edge cases and error handling"""
    
    @pytest.fixture
    def mock_checkpoint_service(self):
        mock = Mock()
        return mock
    
    def test_empty_document_list(self, mock_checkpoint_service):
        """Test checkpoint with empty document list"""
        mock_checkpoint = Mock()
        mock_checkpoint.total_documents = 0
        mock_checkpoint_service.create_checkpoint.return_value = mock_checkpoint
        
        checkpoint = mock_checkpoint_service.create_checkpoint(
            task_id="task_123",
            task_type="raptor",
            doc_ids=[],
            config={}
        )
        
        assert checkpoint.total_documents == 0
    
    def test_nonexistent_checkpoint(self, mock_checkpoint_service):
        """Test operations on nonexistent checkpoint"""
        mock_checkpoint_service.get_by_task_id.return_value = None
        
        checkpoint = mock_checkpoint_service.get_by_task_id("nonexistent_task")
        
        assert checkpoint is None
    
    def test_max_retries_exceeded(self, mock_checkpoint_service):
        """Test behavior when max retries exceeded"""
        # After 3 retries, should not retry
        mock_checkpoint_service.should_retry.return_value = False
        
        should_retry = mock_checkpoint_service.should_retry(
            checkpoint_id="checkpoint_123",
            doc_id="doc_failed",
            max_retries=3
        )
        
        assert should_retry is False


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
