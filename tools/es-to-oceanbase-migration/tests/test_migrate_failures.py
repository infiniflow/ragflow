"""
Tests for how the migrator reports batches that fail to insert.
"""

import pytest
from es_ob_migration.migrator import ESToOceanBaseMigrator


def _batch(start, size):
    return [{"_id": f"doc-{i}", "_source": {"kb_id": "kb"}} for i in range(start, start + size)]


class FakeESClient:
    def __init__(self, batches):
        self.batches = batches

    def health_check(self):
        return {"status": "green"}

    def get_index_mapping(self, index_name):
        return {"properties": {}}

    def count_documents(self, index_name):
        return sum(len(batch) for batch in self.batches)

    def scroll_documents(self, index_name, batch_size):
        yield from self.batches


class FakeOBClient:
    def __init__(self, fail_on_calls=()):
        self.fail_on_calls = set(fail_on_calls)
        self.insert_calls = 0
        self.created = False

    def health_check(self):
        return True

    def get_version(self):
        return "4.3.5"

    def table_exists(self, table_name):
        return self.created

    def create_ragflow_table(self, table_name, vector_size, create_indexes, create_fts_indexes):
        self.created = True

    def insert_batch(self, table_name, rows):
        self.insert_calls += 1
        if self.insert_calls in self.fail_on_calls:
            raise RuntimeError("insert rejected")
        return len(rows)


@pytest.fixture
def batches():
    return [_batch(0, 2), _batch(2, 2), _batch(4, 2)]


def _migrate(es_client, ob_client, progress_dir, resume=False):
    migrator = ESToOceanBaseMigrator(es_client, ob_client, progress_dir=str(progress_dir))
    return migrator, migrator.migrate("ragflow_idx", "ragflow_tbl", batch_size=2, resume=resume, verify_after=False)


def test_failed_batch_marks_migration_failed(tmp_path, batches):
    migrator, result = _migrate(FakeESClient(batches), FakeOBClient(fail_on_calls={2}), tmp_path)

    assert result["success"] is False
    assert result["migrated_documents"] == 4
    assert result["failed_documents"] == 2
    assert "2 documents failed" in result["error"]

    progress = migrator.progress_manager.load_progress("ragflow_idx", "ragflow_tbl")
    assert progress.status == "failed"
    assert migrator.progress_manager.can_resume("ragflow_idx", "ragflow_tbl")


def test_resume_after_failed_batch_succeeds(tmp_path, batches):
    _migrate(FakeESClient(batches), FakeOBClient(fail_on_calls={2}), tmp_path)

    ob_client = FakeOBClient()
    ob_client.created = True
    migrator, result = _migrate(FakeESClient(batches), ob_client, tmp_path, resume=True)

    assert result["success"] is True
    assert result["error"] is None
    assert result["failed_documents"] == 0
    progress = migrator.progress_manager.load_progress("ragflow_idx", "ragflow_tbl")
    assert progress.status == "completed"


def test_all_batches_inserted_marks_completed(tmp_path, batches):
    migrator, result = _migrate(FakeESClient(batches), FakeOBClient(), tmp_path)

    assert result["success"] is True
    assert result["migrated_documents"] == 6
    assert result["failed_documents"] == 0
    progress = migrator.progress_manager.load_progress("ragflow_idx", "ragflow_tbl")
    assert progress.status == "completed"
