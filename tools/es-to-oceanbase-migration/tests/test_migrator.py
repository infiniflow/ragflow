"""
Tests for the migration flow with fake ES/OB clients.

Covers resume behavior: --resume continues from the persisted search_after
cursor, and legacy progress files (written without a cursor) fall back to
a full re-scan with a warning.
"""

import logging

from es_ob_migration.migrator import ESToOceanBaseMigrator
from es_ob_migration.progress import ProgressManager

# Sort values mimic the [{sort_field: "asc"}, {"_id": "asc"}] ordering used
# by ESClient.scroll_documents: a document's sort value is its global
# position in the index, so search_after uniquely seeds the next page.
DOCS = [
    {"_id": f"doc{i}", "_source": {"doc_id": f"doc{i}", "content": f"c{i}"}}
    for i in range(6)
]


class FakeESClient:
    """Minimal ES client double with an ordered, cursor-aware scroll."""

    def health_check(self):
        return {"status": "green"}

    def count_documents(self, index_name):
        return len(DOCS)

    def get_index_mapping(self, index_name):
        return {
            "mappings": {
                "properties": {
                    "doc_id": {"type": "keyword"},
                    "content": {"type": "text"},
                    "content_ltks": {"type": "text"},
                }
            }
        }

    def scroll_documents(
        self,
        index_name,
        batch_size=1000,
        query=None,
        sort_field="_doc",
        search_after=None,
    ):
        start = (search_after[0] + 1) if search_after else 0
        for i in range(start, len(DOCS), batch_size):
            batch = [dict(d) for d in DOCS[i:i + batch_size]]
            batch[-1]["_sort"] = [i + len(batch) - 1]
            yield batch


class FakeOBClient:
    """Minimal OB client double that records every row sent to insert_batch."""

    def __init__(self):
        self.rows = []
        self._table_exists_calls = 0

    def health_check(self):
        return True

    def get_version(self):
        return "fake-4.0"

    def table_exists(self, table):
        self._table_exists_calls += 1
        # First call ever: table does not exist (fresh-start path);
        # afterwards it is treated as existing
        return self._table_exists_calls > 1

    def create_ragflow_table(self, **kwargs):
        pass

    def add_vector_column(self, *args, **kwargs):
        pass

    def insert_batch(self, table, rows):
        self.rows.extend(rows)
        return len(rows)


def _make_migrator(tmp_path, ob_client):
    return ESToOceanBaseMigrator(
        FakeESClient(), ob_client, progress_dir=str(tmp_path)
    )


def _pause_run(tmp_path):
    """Mark the saved progress as paused, as after an interrupted run."""
    manager = ProgressManager(str(tmp_path))
    progress = manager.load_progress("idx", "tbl")
    progress.status = "paused"
    manager.save_progress(progress)
    return progress


class TestMigrateResume:
    """Resume continues from the persisted search_after cursor."""

    def test_first_run_persists_cursor(self, tmp_path):
        """A fresh migration persists a non-empty search_after cursor."""
        result = _make_migrator(tmp_path, FakeOBClient()).migrate(
            "idx", "tbl", batch_size=2, verify_after=False
        )

        assert result["success"] is True
        assert result["migrated_documents"] == 6

        progress = ProgressManager(str(tmp_path)).load_progress("idx", "tbl")
        assert progress.last_sort_values, "cursor not persisted after first run"

    def test_resume_continues_from_cursor(self, tmp_path):
        """Resume with a cursor re-sends nothing and keeps the migrated
        count within the total."""
        first = _make_migrator(tmp_path, FakeOBClient()).migrate(
            "idx", "tbl", batch_size=2, verify_after=False
        )
        assert first["success"] is True

        _pause_run(tmp_path)

        ob_client = FakeOBClient()
        result = _make_migrator(tmp_path, ob_client).migrate(
            "idx", "tbl", batch_size=2, resume=True, verify_after=False
        )

        assert result["success"] is True
        assert result["migrated_documents"] == 6
        assert result["migrated_documents"] <= result["total_documents"]
        assert ob_client.rows == [], "documents were re-sent to OceanBase on resume"

    def test_resume_without_cursor_falls_back_to_full_rescan(
        self, tmp_path, caplog
    ):
        """A progress file written by an older version has no cursor: resume
        falls back to a full re-scan (previous behavior) and logs a warning."""
        first = _make_migrator(tmp_path, FakeOBClient()).migrate(
            "idx", "tbl", batch_size=2, verify_after=False
        )
        assert first["success"] is True

        progress = _pause_run(tmp_path)
        progress.last_sort_values = []
        ProgressManager(str(tmp_path)).save_progress(progress)

        ob_client = FakeOBClient()
        with caplog.at_level(logging.WARNING):
            result = _make_migrator(tmp_path, ob_client).migrate(
                "idx", "tbl", batch_size=2, resume=True, verify_after=False
            )

        assert any("search_after cursor" in record.message for record in caplog.records)
        assert len(ob_client.rows) == 6, "legacy resume should re-scan the whole index"
        # Degraded counter matches the pre-fix behavior for legacy files:
        # previously_migrated + total
        assert result["migrated_documents"] == 12
