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
Integration tests for CheckpointService with real database operations.

These tests use the actual CheckpointService implementation and database,
unlike the unit tests which use mocks.
"""

import pytest
from api.db.services.checkpoint_service import CheckpointService


class TestCheckpointServiceIntegration:
    """Integration tests for CheckpointService"""
    
    @pytest.fixture(autouse=True)
    def setup_and_teardown(self):
        """Setup and cleanup for each test"""
        # Setup: ensure clean state
        yield
        # Teardown: clean up test data
        # Note: In production, you'd clean up test checkpoints here
    
    def test_create_and_retrieve_checkpoint(self):
        """Test creating a checkpoint and retrieving it"""
        # Create checkpoint
        checkpoint = CheckpointService.create_checkpoint(
            task_id="test_task_001",
            task_type="raptor",
            doc_ids=["doc1", "doc2", "doc3"],
            config={"max_cluster": 64}
        )
        
        # Verify creation
        assert checkpoint is not None
        assert checkpoint.task_id == "test_task_001"
        assert checkpoint.task_type == "raptor"
        assert checkpoint.total_documents == 3
        assert checkpoint.status == "pending"
        
        # Retrieve by task_id
        retrieved = CheckpointService.get_by_task_id("test_task_001")
        assert retrieved is not None
        assert retrieved.id == checkpoint.id
        assert retrieved.task_id == "test_task_001"
    
    def test_document_completion_workflow(self):
        """Test marking documents as completed"""
        # Create checkpoint
        checkpoint = CheckpointService.create_checkpoint(
            task_id="test_task_002",
            task_type="raptor",
            doc_ids=["doc1", "doc2", "doc3"],
            config={}
        )
        
        # Initially all pending
        pending = CheckpointService.get_pending_documents(checkpoint.id)
        assert len(pending) == 3
        
        # Complete first document
        success = CheckpointService.save_document_completion(
            checkpoint.id,
            "doc1",
            token_count=1500,
            chunks=45
        )
        assert success is True
        
        # Check pending reduced
        pending = CheckpointService.get_pending_documents(checkpoint.id)
        assert len(pending) == 2
        assert "doc1" not in pending
        
        # Complete second document
        CheckpointService.save_document_completion(
            checkpoint.id,
            "doc2",
            token_count=2000,
            chunks=60
        )
        
        # Check status
        status = CheckpointService.get_checkpoint_status(checkpoint.id)
        assert status["completed_documents"] == 2
        assert status["pending_documents"] == 1
        assert status["token_count"] == 3500  # 1500 + 2000
    
    def test_document_failure_and_retry(self):
        """Test marking documents as failed and retry logic"""
        # Create checkpoint
        checkpoint = CheckpointService.create_checkpoint(
            task_id="test_task_003",
            task_type="raptor",
            doc_ids=["doc1", "doc2"],
            config={}
        )
        
        # Fail first document
        success = CheckpointService.save_document_failure(
            checkpoint.id,
            "doc1",
            error="API timeout after 60s"
        )
        assert success is True
        
        # Check failed documents
        failed = CheckpointService.get_failed_documents(checkpoint.id)
        assert len(failed) == 1
        assert failed[0]["doc_id"] == "doc1"
        assert "timeout" in failed[0]["error"].lower()
        
        # Should be able to retry (first failure)
        can_retry = CheckpointService.should_retry(checkpoint.id, "doc1", max_retries=3)
        assert can_retry is True
        
        # Reset for retry
        reset_success = CheckpointService.reset_document_for_retry(checkpoint.id, "doc1")
        assert reset_success is True
        
        # Should be back in pending
        pending = CheckpointService.get_pending_documents(checkpoint.id)
        assert "doc1" in pending
    
    def test_max_retries_exceeded(self):
        """Test that documents can't be retried indefinitely"""
        checkpoint = CheckpointService.create_checkpoint(
            task_id="test_task_004",
            task_type="raptor",
            doc_ids=["doc1"],
            config={}
        )
        
        # Fail 3 times
        for i in range(3):
            CheckpointService.save_document_failure(
                checkpoint.id,
                "doc1",
                error=f"Attempt {i+1} failed"
            )
            if i < 2:  # Reset for retry except last time
                CheckpointService.reset_document_for_retry(checkpoint.id, "doc1")
        
        # Should not be able to retry after 3 failures
        can_retry = CheckpointService.should_retry(checkpoint.id, "doc1", max_retries=3)
        assert can_retry is False
    
    def test_pause_and_resume(self):
        """Test pausing and resuming a checkpoint"""
        checkpoint = CheckpointService.create_checkpoint(
            task_id="test_task_005",
            task_type="raptor",
            doc_ids=["doc1", "doc2"],
            config={}
        )
        
        # Initially not paused
        assert CheckpointService.is_paused(checkpoint.id) is False
        
        # Pause
        success = CheckpointService.pause_checkpoint(checkpoint.id)
        assert success is True
        assert CheckpointService.is_paused(checkpoint.id) is True
        
        # Resume
        success = CheckpointService.resume_checkpoint(checkpoint.id)
        assert success is True
        assert CheckpointService.is_paused(checkpoint.id) is False
    
    def test_cancel_checkpoint(self):
        """Test cancelling a checkpoint"""
        checkpoint = CheckpointService.create_checkpoint(
            task_id="test_task_006",
            task_type="raptor",
            doc_ids=["doc1"],
            config={}
        )
        
        # Cancel
        success = CheckpointService.cancel_checkpoint(checkpoint.id)
        assert success is True
        assert CheckpointService.is_cancelled(checkpoint.id) is True
    
    def test_progress_calculation(self):
        """Test that progress is calculated correctly"""
        checkpoint = CheckpointService.create_checkpoint(
            task_id="test_task_007",
            task_type="raptor",
            doc_ids=["doc1", "doc2", "doc3", "doc4", "doc5"],
            config={}
        )
        
        # Complete 3 out of 5
        for doc_id in ["doc1", "doc2", "doc3"]:
            CheckpointService.save_document_completion(
                checkpoint.id,
                doc_id,
                token_count=1000,
                chunks=30
            )
        
        # Check progress
        status = CheckpointService.get_checkpoint_status(checkpoint.id)
        assert status["total_documents"] == 5
        assert status["completed_documents"] == 3
        assert status["pending_documents"] == 2
        assert status["progress"] == 0.6  # 3/5
    
    def test_resume_from_checkpoint(self):
        """Test resuming a task from checkpoint (real-world scenario)"""
        # Simulate: Task starts, processes 2 docs, then crashes
        checkpoint = CheckpointService.create_checkpoint(
            task_id="test_task_008",
            task_type="raptor",
            doc_ids=["doc1", "doc2", "doc3", "doc4", "doc5"],
            config={}
        )
        
        # Process first 2 documents
        CheckpointService.save_document_completion(checkpoint.id, "doc1", 1000, 30)
        CheckpointService.save_document_completion(checkpoint.id, "doc2", 1500, 45)
        
        # Simulate crash and restart - retrieve checkpoint
        resumed_checkpoint = CheckpointService.get_by_task_id("test_task_008")
        assert resumed_checkpoint is not None
        
        # Get pending documents (should skip completed ones)
        pending = CheckpointService.get_pending_documents(resumed_checkpoint.id)
        assert len(pending) == 3
        assert "doc1" not in pending
        assert "doc2" not in pending
        assert set(pending) == {"doc3", "doc4", "doc5"}
        
        # Continue processing remaining documents
        CheckpointService.save_document_completion(resumed_checkpoint.id, "doc3", 1200, 38)
        
        # Verify state
        status = CheckpointService.get_checkpoint_status(resumed_checkpoint.id)
        assert status["completed_documents"] == 3
        assert status["pending_documents"] == 2
        assert status["token_count"] == 3700  # 1000 + 1500 + 1200


if __name__ == "__main__":
    pytest.main([__file__, "-v", "-s"])
