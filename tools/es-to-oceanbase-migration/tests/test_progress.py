"""
Tests for progress tracking and resume capability.
"""

import json
import os
import tempfile
import pytest
from pathlib import Path

from es_ob_migration.progress import MigrationProgress, ProgressManager


class TestMigrationProgress:
    """Test MigrationProgress dataclass."""

    def test_create_basic_progress(self):
        """Test creating a basic progress object."""
        progress = MigrationProgress(
            es_index="ragflow_test",
            ob_table="ragflow_test",
        )
        
        assert progress.es_index == "ragflow_test"
        assert progress.ob_table == "ragflow_test"
        assert progress.total_documents == 0
        assert progress.migrated_documents == 0
        assert progress.status == "pending"
        assert progress.started_at != ""
        assert progress.updated_at != ""

    def test_create_progress_with_counts(self):
        """Test creating progress with document counts."""
        progress = MigrationProgress(
            es_index="ragflow_test",
            ob_table="ragflow_test",
            total_documents=1000,
            migrated_documents=500,
        )
        
        assert progress.total_documents == 1000
        assert progress.migrated_documents == 500

    def test_progress_default_values(self):
        """Test default values."""
        progress = MigrationProgress(
            es_index="test_index",
            ob_table="test_table",
        )
        
        assert progress.failed_documents == 0
        assert progress.last_sort_values == []
        assert progress.last_batch_ids == []
        assert progress.error_message == ""
        assert progress.schema_converted is False
        assert progress.table_created is False
        assert progress.indexes_created is False

    def test_progress_status_values(self):
        """Test various status values."""
        for status in ["pending", "running", "completed", "failed", "paused"]:
            progress = MigrationProgress(
                es_index="test",
                ob_table="test",
                status=status,
            )
            assert progress.status == status


class TestProgressManager:
    """Test ProgressManager class."""

    @pytest.fixture
    def temp_dir(self):
        """Create a temporary directory for tests."""
        with tempfile.TemporaryDirectory() as tmpdir:
            yield tmpdir

    @pytest.fixture
    def manager(self, temp_dir):
        """Create a ProgressManager with temp directory."""
        return ProgressManager(progress_dir=temp_dir)

    def test_create_progress_manager(self, temp_dir):
        """Test creating a progress manager."""
        manager = ProgressManager(progress_dir=temp_dir)
        assert manager.progress_dir.exists()

    def test_create_progress_manager_creates_dir(self, temp_dir):
        """Test that progress manager creates directory."""
        new_dir = os.path.join(temp_dir, "new_progress")
        ProgressManager(progress_dir=new_dir)
        assert Path(new_dir).exists()

    def test_create_progress(self, manager):
        """Test creating new progress."""
        progress = manager.create_progress(
            es_index="ragflow_abc123",
            ob_table="ragflow_abc123",
            total_documents=1000,
        )
        
        assert progress.es_index == "ragflow_abc123"
        assert progress.ob_table == "ragflow_abc123"
        assert progress.total_documents == 1000
        assert progress.status == "running"

    def test_save_and_load_progress(self, manager):
        """Test saving and loading progress."""
        # Create and save
        progress = manager.create_progress(
            es_index="ragflow_test",
            ob_table="ragflow_test",
            total_documents=500,
        )
        progress.migrated_documents = 250
        progress.last_sort_values = ["doc_250", 1234567890]
        manager.save_progress(progress)
        
        # Load
        loaded = manager.load_progress("ragflow_test", "ragflow_test")
        
        assert loaded is not None
        assert loaded.es_index == "ragflow_test"
        assert loaded.total_documents == 500
        assert loaded.migrated_documents == 250
        assert loaded.last_sort_values == ["doc_250", 1234567890]

    def test_load_nonexistent_progress(self, manager):
        """Test loading progress that doesn't exist."""
        loaded = manager.load_progress("nonexistent", "nonexistent")
        assert loaded is None

    def test_delete_progress(self, manager):
        """Test deleting progress."""
        # Create progress
        manager.create_progress(
            es_index="ragflow_delete_test",
            ob_table="ragflow_delete_test",
            total_documents=100,
        )
        
        # Verify it exists
        assert manager.load_progress("ragflow_delete_test", "ragflow_delete_test") is not None
        
        # Delete
        manager.delete_progress("ragflow_delete_test", "ragflow_delete_test")
        
        # Verify it's gone
        assert manager.load_progress("ragflow_delete_test", "ragflow_delete_test") is None

    def test_update_progress(self, manager):
        """Test updating progress."""
        progress = manager.create_progress(
            es_index="ragflow_update",
            ob_table="ragflow_update",
            total_documents=1000,
        )
        
        # Update
        manager.update_progress(
            progress,
            migrated_count=100,
            last_sort_values=["doc_100", 9999],
            last_batch_ids=["id1", "id2", "id3"],
        )
        
        assert progress.migrated_documents == 100
        assert progress.last_sort_values == ["doc_100", 9999]
        assert progress.last_batch_ids == ["id1", "id2", "id3"]

    def test_update_progress_multiple_batches(self, manager):
        """Test updating progress multiple times."""
        progress = manager.create_progress(
            es_index="ragflow_multi",
            ob_table="ragflow_multi",
            total_documents=1000,
        )
        
        # Update multiple times
        for i in range(1, 11):
            manager.update_progress(progress, migrated_count=100)
        
        assert progress.migrated_documents == 1000

    def test_mark_completed(self, manager):
        """Test marking migration as completed."""
        progress = manager.create_progress(
            es_index="ragflow_complete",
            ob_table="ragflow_complete",
            total_documents=100,
        )
        progress.migrated_documents = 100
        
        manager.mark_completed(progress)
        
        assert progress.status == "completed"

    def test_mark_failed(self, manager):
        """Test marking migration as failed."""
        progress = manager.create_progress(
            es_index="ragflow_fail",
            ob_table="ragflow_fail",
            total_documents=100,
        )
        
        manager.mark_failed(progress, "Connection timeout")
        
        assert progress.status == "failed"
        assert progress.error_message == "Connection timeout"

    def test_mark_paused(self, manager):
        """Test marking migration as paused."""
        progress = manager.create_progress(
            es_index="ragflow_pause",
            ob_table="ragflow_pause",
            total_documents=1000,
        )
        progress.migrated_documents = 500
        
        manager.mark_paused(progress)
        
        assert progress.status == "paused"

    def test_can_resume_running(self, manager):
        """Test can_resume for running migration."""
        manager.create_progress(
            es_index="ragflow_resume_running",
            ob_table="ragflow_resume_running",
            total_documents=1000,
        )
        
        assert manager.can_resume("ragflow_resume_running", "ragflow_resume_running") is True

    def test_can_resume_paused(self, manager):
        """Test can_resume for paused migration."""
        progress = manager.create_progress(
            es_index="ragflow_resume_paused",
            ob_table="ragflow_resume_paused",
            total_documents=1000,
        )
        manager.mark_paused(progress)
        
        assert manager.can_resume("ragflow_resume_paused", "ragflow_resume_paused") is True

    def test_can_resume_completed(self, manager):
        """Test can_resume for completed migration."""
        progress = manager.create_progress(
            es_index="ragflow_resume_complete",
            ob_table="ragflow_resume_complete",
            total_documents=100,
        )
        progress.migrated_documents = 100
        manager.mark_completed(progress)
        
        # Completed migrations should not be resumed
        assert manager.can_resume("ragflow_resume_complete", "ragflow_resume_complete") is False

    def test_can_resume_nonexistent(self, manager):
        """Test can_resume for nonexistent migration."""
        assert manager.can_resume("nonexistent", "nonexistent") is False

    def test_get_resume_info(self, manager):
        """Test getting resume information."""
        progress = manager.create_progress(
            es_index="ragflow_info",
            ob_table="ragflow_info",
            total_documents=1000,
        )
        progress.migrated_documents = 500
        progress.last_sort_values = ["doc_500", 12345]
        progress.schema_converted = True
        progress.table_created = True
        manager.save_progress(progress)
        
        info = manager.get_resume_info("ragflow_info", "ragflow_info")
        
        assert info is not None
        assert info["migrated_documents"] == 500
        assert info["total_documents"] == 1000
        assert info["last_sort_values"] == ["doc_500", 12345]
        assert info["schema_converted"] is True
        assert info["table_created"] is True
        assert info["status"] == "running"

    def test_get_resume_info_nonexistent(self, manager):
        """Test getting resume info for nonexistent migration."""
        info = manager.get_resume_info("nonexistent", "nonexistent")
        assert info is None

    def test_progress_file_path(self, manager):
        """Test progress file naming."""
        manager.create_progress(
            es_index="ragflow_abc123",
            ob_table="ragflow_abc123",
            total_documents=100,
        )
        
        expected_file = manager.progress_dir / "ragflow_abc123_to_ragflow_abc123.json"
        assert expected_file.exists()

    def test_progress_file_content(self, manager):
        """Test progress file JSON content."""
        progress = manager.create_progress(
            es_index="ragflow_json",
            ob_table="ragflow_json",
            total_documents=100,
        )
        progress.migrated_documents = 50
        manager.save_progress(progress)
        
        # Read file directly
        progress_file = manager.progress_dir / "ragflow_json_to_ragflow_json.json"
        with open(progress_file) as f:
            data = json.load(f)
        
        assert data["es_index"] == "ragflow_json"
        assert data["ob_table"] == "ragflow_json"
        assert data["total_documents"] == 100
        assert data["migrated_documents"] == 50
