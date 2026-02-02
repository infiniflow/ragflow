"""
Progress tracking and resume capability for migration.
"""

import json
import logging
from dataclasses import dataclass, field, asdict
from datetime import datetime
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


@dataclass
class MigrationProgress:
    """Migration progress state."""

    # Basic info
    es_index: str
    ob_table: str
    started_at: str = ""
    updated_at: str = ""

    # Progress counters
    total_documents: int = 0
    migrated_documents: int = 0
    failed_documents: int = 0

    # State for resume
    last_sort_values: list[Any] = field(default_factory=list)
    last_batch_ids: list[str] = field(default_factory=list)

    # Status
    status: str = "pending"  # pending, running, completed, failed, paused
    error_message: str = ""

    # Schema info
    schema_converted: bool = False
    table_created: bool = False
    indexes_created: bool = False

    def __post_init__(self):
        if not self.started_at:
            self.started_at = datetime.utcnow().isoformat()
        self.updated_at = datetime.utcnow().isoformat()


class ProgressManager:
    """Manage migration progress persistence."""

    def __init__(self, progress_dir: str = ".migration_progress"):
        """
        Initialize progress manager.

        Args:
            progress_dir: Directory to store progress files
        """
        self.progress_dir = Path(progress_dir)
        self.progress_dir.mkdir(parents=True, exist_ok=True)

    def _get_progress_file(self, es_index: str, ob_table: str) -> Path:
        """Get progress file path for a migration."""
        filename = f"{es_index}_to_{ob_table}.json"
        return self.progress_dir / filename

    def load_progress(
        self, es_index: str, ob_table: str
    ) -> MigrationProgress | None:
        """
        Load progress from file.

        Args:
            es_index: Elasticsearch index name
            ob_table: OceanBase table name

        Returns:
            MigrationProgress if exists, None otherwise
        """
        progress_file = self._get_progress_file(es_index, ob_table)

        if not progress_file.exists():
            return None

        try:
            with open(progress_file, "r") as f:
                data = json.load(f)
            progress = MigrationProgress(**data)
            logger.info(
                f"Loaded progress: {progress.migrated_documents}/{progress.total_documents} documents"
            )
            return progress
        except Exception as e:
            logger.warning(f"Failed to load progress: {e}")
            return None

    def save_progress(self, progress: MigrationProgress):
        """
        Save progress to file.

        Args:
            progress: MigrationProgress instance
        """
        progress.updated_at = datetime.utcnow().isoformat()
        progress_file = self._get_progress_file(progress.es_index, progress.ob_table)

        try:
            with open(progress_file, "w") as f:
                json.dump(asdict(progress), f, indent=2, default=str)
            logger.debug(f"Saved progress to {progress_file}")
        except Exception as e:
            logger.error(f"Failed to save progress: {e}")

    def delete_progress(self, es_index: str, ob_table: str):
        """Delete progress file."""
        progress_file = self._get_progress_file(es_index, ob_table)
        if progress_file.exists():
            progress_file.unlink()
            logger.info(f"Deleted progress file: {progress_file}")

    def create_progress(
        self,
        es_index: str,
        ob_table: str,
        total_documents: int,
    ) -> MigrationProgress:
        """
        Create new progress tracker.

        Args:
            es_index: Elasticsearch index name
            ob_table: OceanBase table name
            total_documents: Total documents to migrate

        Returns:
            New MigrationProgress instance
        """
        progress = MigrationProgress(
            es_index=es_index,
            ob_table=ob_table,
            total_documents=total_documents,
            status="running",
        )
        self.save_progress(progress)
        return progress

    def update_progress(
        self,
        progress: MigrationProgress,
        migrated_count: int,
        last_sort_values: list[Any] | None = None,
        last_batch_ids: list[str] | None = None,
    ):
        """
        Update progress after a batch.

        Args:
            progress: MigrationProgress instance
            migrated_count: Number of documents migrated in this batch
            last_sort_values: Sort values for search_after
            last_batch_ids: IDs of documents in last batch
        """
        progress.migrated_documents += migrated_count

        if last_sort_values:
            progress.last_sort_values = last_sort_values
        if last_batch_ids:
            progress.last_batch_ids = last_batch_ids

        self.save_progress(progress)

    def mark_completed(self, progress: MigrationProgress):
        """Mark migration as completed."""
        progress.status = "completed"
        progress.updated_at = datetime.utcnow().isoformat()
        self.save_progress(progress)
        logger.info(
            f"Migration completed: {progress.migrated_documents} documents"
        )

    def mark_failed(self, progress: MigrationProgress, error: str):
        """Mark migration as failed."""
        progress.status = "failed"
        progress.error_message = error
        progress.updated_at = datetime.utcnow().isoformat()
        self.save_progress(progress)
        logger.error(f"Migration failed: {error}")

    def mark_paused(self, progress: MigrationProgress):
        """Mark migration as paused (for resume later)."""
        progress.status = "paused"
        progress.updated_at = datetime.utcnow().isoformat()
        self.save_progress(progress)
        logger.info(
            f"Migration paused at {progress.migrated_documents}/{progress.total_documents}"
        )

    def can_resume(self, es_index: str, ob_table: str) -> bool:
        """Check if migration can be resumed."""
        progress = self.load_progress(es_index, ob_table)
        if not progress:
            return False
        return progress.status in ("running", "paused", "failed")

    def get_resume_info(self, es_index: str, ob_table: str) -> dict[str, Any] | None:
        """Get information needed to resume migration."""
        progress = self.load_progress(es_index, ob_table)
        if not progress:
            return None

        return {
            "migrated_documents": progress.migrated_documents,
            "total_documents": progress.total_documents,
            "last_sort_values": progress.last_sort_values,
            "last_batch_ids": progress.last_batch_ids,
            "schema_converted": progress.schema_converted,
            "table_created": progress.table_created,
            "indexes_created": progress.indexes_created,
            "status": progress.status,
        }
