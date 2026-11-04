#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
"""Carry pre-v0.24 document metadata out of the orphaned MySQL column.

#12826 (v0.24.0) moved metadata into a per-tenant document engine store without
migrating the data; the column survives with its values because Peewee never
drops columns.

Shared by the startup migration in ``api.db.db_models`` and
``tools/scripts/backfill_doc_meta.py``. Data movement only -- the migration
markers belong to ``api.db.db_models`` -- and no runtime imports, so the helpers
stay exercisable on their own. Values are copied verbatim, already split by the
v0.23 write path.
"""

import inspect
import json
import logging
import time

logger = logging.getLogger("document_meta_backfill")

MIGRATION_NAME = "doc_meta_fields_backfill"

# Value written to that marker once the copy completes. Only the marker's
# presence is tested, so any non-empty value would do; this is what shows up in
# system_settings for anyone looking.
MIGRATION_DONE = "done"

DEFAULT_BATCH_SIZE = 500

# Rows the scan may buffer across tenants before the part-filled batches are
# written anyway. Batching is what makes each write cheap, but a tenant holding
# only a few documents never fills one, and such tenants are scattered over the
# whole table, so without this they would all be held until the scan ends.
PENDING_BATCH_LIMIT = 8

# A stored JSON text equal to one of these carries no metadata and is skipped.
EMPTY_META_TEXTS = frozenset(["{}", "null", "[]"])

# Keyset pagination reads forward on the primary key, so each page is bounded and
# a restart stays cheap on large tables.
_SCAN_SQL = """
SELECT d.id, d.kb_id, k.tenant_id, d.meta_fields
FROM document d
LEFT JOIN knowledgebase k ON k.id = d.kb_id
WHERE d.id > %s
  AND d.meta_fields IS NOT NULL
  AND d.meta_fields <> %s
{tenant_clause}
ORDER BY d.id
LIMIT %s
"""

_COUNT_SQL = """
SELECT COUNT(*)
FROM document d
LEFT JOIN knowledgebase k ON k.id = d.kb_id
WHERE d.meta_fields IS NOT NULL
  AND d.meta_fields <> %s
{tenant_clause}
"""


def has_legacy_column(db) -> bool:
    """Report whether the orphaned ``document.meta_fields`` column is present.

    Probed by selecting from it rather than by reading information_schema, whose
    row set spans every schema on the server.
    """
    try:
        db.execute_sql("SELECT meta_fields FROM document LIMIT 1")
    except Exception as e:
        message = str(e).lower()
        if "unknown column" in message or "does not exist" in message or "undefined_column" in message:
            return False
        raise
    return True


def count_candidates(db, tenant_id="") -> int:
    """How many documents still hold a legacy value.

    Only NULL and empty text are excluded, so this overcounts cells the scan will
    later report as empty: it is a starting estimate for the log, not a promise.
    """
    tenant_clause = "AND k.tenant_id = %s" if tenant_id else ""
    params = [""]
    if tenant_id:
        params.append(tenant_id)
    cursor = db.execute_sql(_COUNT_SQL.format(tenant_clause=tenant_clause), params)
    row = cursor.fetchone()
    return int(row[0]) if row else 0


def parse_meta_fields(raw):
    """Decode one legacy cell into a dict, or return None when it holds nothing.

    The column is a LONGTEXT holding JSON text, not a native JSON column, so
    values arrive as strings.
    """
    if raw is None:
        return None
    if isinstance(raw, dict):
        return raw or None
    if isinstance(raw, (bytes, bytearray)):
        raw = raw.decode("utf-8", errors="replace")
    if not isinstance(raw, str):
        return None
    text = raw.strip()
    if not text or text in EMPTY_META_TEXTS:
        return None
    try:
        parsed = json.loads(text)
    except (ValueError, TypeError):
        return None
    if not isinstance(parsed, dict) or not parsed:
        return None
    return parsed


def is_blank_meta_text(raw) -> bool:
    """Whether a legacy cell holds no metadata, rather than metadata we cannot read.

    Counted apart from malformed JSON so that the common case -- a document that
    never had any -- does not read as corruption. The test is what the cell parses
    to, not how it is written: "{ }" and "[ ]" hold nothing just as "{}" and "[]"
    do. Text that does not parse at all really is unreadable.
    """
    if raw is None:
        return True
    if isinstance(raw, dict):
        return not raw
    if isinstance(raw, (bytes, bytearray)):
        raw = raw.decode("utf-8", errors="replace")
    if not isinstance(raw, str):
        return False
    text = raw.strip()
    if not text or text in EMPTY_META_TEXTS:
        return True
    try:
        parsed = json.loads(text)
    except (ValueError, TypeError):
        return False
    return parsed is None or (isinstance(parsed, (dict, list)) and not parsed)


def _scan(db, after_id, batch_size, tenant_id):
    """Read one keyset page of candidate rows.

    ``after_id`` advances within one run; it is not a resume point, because every
    run starts from the beginning and the copy is idempotent.
    """
    tenant_clause = "AND k.tenant_id = %s" if tenant_id else ""
    params = [after_id, ""]
    if tenant_id:
        params.append(tenant_id)
    params.append(batch_size)
    cursor = db.execute_sql(_SCAN_SQL.format(tenant_clause=tenant_clause), params)
    columns = [d[0] for d in cursor.description]
    return [dict(zip(columns, row)) for row in cursor.fetchall()]


def _doc_ids_from(results):
    """Pull document ids out of any backend's search result shape.

    Returns None when the shape is unrecognised, which callers treat as "cannot
    tell" rather than "nothing matched".
    """
    if results is None:
        return None
    ids = set()
    if isinstance(results, tuple) and len(results) == 2:
        results = results[0]  # Infinity returns (DataFrame, total)
    if hasattr(results, "iterrows"):  # pandas DataFrame
        for _, row in results.iterrows():
            value = row.get("id")
            if value:
                ids.add(str(value))
        return ids
    if hasattr(results, "get") and "hits" in results:  # Elasticsearch / OpenSearch
        for hit in results.get("hits", {}).get("hits", []):
            value = hit.get("_id")
            if value:
                ids.add(str(value))
        return ids
    if hasattr(results, "chunks"):  # GaussDB / OceanBase SearchResult
        for chunk in results.chunks or []:
            value = (chunk or {}).get("id")
            if value:
                ids.add(str(value))
        return ids
    if isinstance(results, list):
        for item in results:
            if isinstance(item, dict):
                value = item.get("id") or item.get("doc_id")
                if value:
                    ids.add(str(value))
        return ids
    return None


class MetadataWriter:
    """Writes metadata rows into the per-tenant document metadata stores.

    Probes once per tenant rather than per KB because document ids are globally
    unique UUIDs; inserts stay per KB because ``insert()`` stamps kb_id onto every
    row it is given.
    """

    def __init__(self, conn, execute: bool):
        self.conn = conn
        self.execute = execute
        self._supports_refresh = None
        self._kb_ids_param = None
        self._order_by_factory = None
        self._existence = {}
        self._created = set()
        self._written_tenants = set()

    def insert_rows(self, rows, index_name, kb_id):
        if self._supports_refresh is None:
            try:
                self._supports_refresh = "refresh" in inspect.signature(self.conn.insert).parameters
            except (TypeError, ValueError):
                self._supports_refresh = False
        errors = self.conn.insert(rows, index_name, kb_id, refresh=False) if self._supports_refresh else self.conn.insert(rows, index_name, kb_id)
        # A batch that failed wholesale wrote nothing, so refreshing its index
        # would be wasted I/O. A list shorter than the batch means some rows
        # landed, which is exactly when a refresh matters.
        if not errors or len(errors) < len(rows):
            self._written_tenants.add(index_name)
        return errors

    def kb_ids_param_name(self) -> str:
        """Name of the search argument carrying the knowledge base ids.

        Every connector except SereneDB names it ``knowledgebase_ids``; SereneDB
        names it ``dataset_ids`` and declares it without a default, so the wrong
        name either vanishes into ``**kwargs`` or raises TypeError -- and either
        way the filter silently goes unset.
        """
        if self._kb_ids_param is None:
            try:
                params = inspect.signature(self.conn.search).parameters
            except (TypeError, ValueError):
                params = {}
            if "knowledgebase_ids" in params:
                self._kb_ids_param = "knowledgebase_ids"
            elif "dataset_ids" in params:
                self._kb_ids_param = "dataset_ids"
            else:
                self._kb_ids_param = ""
        return self._kb_ids_param

    def ensure_store(self, index_name) -> bool:
        """Create the per-tenant store when it was missing before this run.

        Infinity's insert cannot create it implicitly: with no vector column to
        infer a size from, it raises instead of falling back.
        """
        if index_name in self._created:
            return True
        if self._existed_before(index_name):
            return True
        if not self.execute:
            return True
        if self.conn.create_doc_meta_idx(index_name) is False:
            logger.error("failed to create metadata store %s", index_name)
            return False
        self._created.add(index_name)
        logger.info("created metadata store %s", index_name)
        return True

    def _existed_before(self, index_name) -> bool:
        if index_name not in self._existence:
            try:
                self._existence[index_name] = bool(self.conn.index_exist(index_name, ""))
            except Exception as e:  # noqa: BLE001 - the driver error type depends on the backend
                logger.warning("metadata store existence check failed for %s: %s", index_name, e)
                # Assume it exists so a broken probe never makes us create one.
                self._existence[index_name] = True
        return self._existence[index_name]

    def _order_by(self):
        """An empty sort expression, imported lazily to keep this module importable without the runtime."""
        if self._order_by_factory is None:
            from common.doc_store.doc_store_base import OrderByExpr

            self._order_by_factory = OrderByExpr
        return self._order_by_factory()

    def needs_conflict_check(self, index_name) -> bool:
        """False only for a store this run created: it was empty, and probing it fails on a dry run."""
        return self._existence.get(index_name, True)

    def existing_ids(self, index_name, kb_ids, doc_ids):
        """Return the subset of ``doc_ids`` already present, or None on failure.

        None is a hard answer: the caller refuses the batch rather than risk
        overwriting metadata a user edited after the upgrade.
        """
        kwargs = {"index_names": index_name}
        kb_param = self.kb_ids_param_name()
        if kb_param:
            kwargs[kb_param] = list(kb_ids)
        try:
            results = self.conn.search(
                select_fields=["id"],
                highlight_fields=[],
                condition={"kb_id": list(kb_ids), "id": list(doc_ids)},
                match_expressions=[],
                order_by=self._order_by(),
                offset=0,
                limit=len(doc_ids),
                **kwargs,
            )
        except Exception as e:  # noqa: BLE001 - a failed probe must not abort the run
            logger.warning("existence check failed for %s: %s", index_name, e)
            return None
        return _doc_ids_from(results)

    def refresh_all(self):
        """Make writes visible, once per tenant rather than once per batch.

        Only Elasticsearch and OpenSearch need it; the others are immediately
        consistent or write through a table.
        """
        refresh_idx = getattr(self.conn, "refresh_idx", None)
        if not callable(refresh_idx):
            return
        for index_name in sorted(self._written_tenants):
            try:
                if refresh_idx(index_name):
                    logger.info("refreshed metadata store %s", index_name)
                else:
                    logger.warning("failed to refresh metadata store %s", index_name)
            except Exception as e:  # noqa: BLE001 - refresh is best effort
                logger.warning("failed to refresh metadata store %s: %s", index_name, e)


def _flush(writer, tenant_id, batch, stats, on_conflict, execute):
    """Write one tenant's batch, or only account for it in a dry run."""
    index_name = f"ragflow_doc_meta_{tenant_id}"
    if not writer.ensure_store(index_name):
        stats["failed"] += len(batch)
        return

    if on_conflict == "skip" and writer.needs_conflict_check(index_name):
        kb_ids = sorted({row["kb_id"] for row in batch})
        present = writer.existing_ids(index_name, kb_ids, [row["id"] for row in batch])
        if present is None:
            logger.error(
                "skipped a batch of %d documents in tenant %s: cannot tell which documents already have metadata",
                len(batch),
                tenant_id,
            )
            stats["failed"] += len(batch)
            return
        if present:
            before = len(batch)
            batch = [row for row in batch if row["id"] not in present]
            stats["skipped"] += before - len(batch)
    if not batch:
        return

    if not execute:
        stats["written"] += len(batch)
        return

    # Insert per KB; the connector's insert() stamps kb_id on every row.
    by_kb: dict = {}
    for row in batch:
        by_kb.setdefault(row["kb_id"], []).append(row)
    for kb_id, rows in by_kb.items():
        errors = writer.insert_rows(rows, index_name, kb_id)
        if errors:
            # The connector reports only the rows that failed, so a partial
            # failure loses len(errors) rows rather than the whole batch, and the
            # rows that did land still count as written.
            failed = min(len(errors), len(rows)) if isinstance(errors, list) else len(rows)
            logger.error("failed to write %d of %d rows to %s: %s", failed, len(rows), index_name, errors)
            stats["failed"] += failed
            stats["written"] += len(rows) - failed
            continue
        stats["written"] += len(rows)


def _drain(writer, pending, stats, batch_size, on_conflict, execute, full_only):
    """Flush pending tenants holding a full batch, or all of them.

    Returns rows still buffered, so the caller can tell when the part-filled
    tenants have waited long enough.
    """
    buffered = 0
    for tenant_id in list(pending):
        rows = pending[tenant_id]
        while rows and (len(rows) >= batch_size or not full_only):
            batch, rows = rows[:batch_size], rows[batch_size:]
            pending[tenant_id] = rows
            _flush(writer, tenant_id, batch, stats, on_conflict, execute)
        if rows:
            buffered += len(rows)
        else:
            del pending[tenant_id]
    return buffered


def backfill_document_meta_fields(
    db,
    conn,
    *,
    batch_size=DEFAULT_BATCH_SIZE,
    tenant_id="",
    on_conflict="skip",
    execute=True,
):
    """Copy legacy metadata values into the per-tenant document metadata stores.

    Scans the whole table; ``on_conflict="skip"`` makes the copy idempotent so an
    interrupted run can be retried safely. Returns a stats dict.
    """
    writer = MetadataWriter(conn, execute)
    stats = {
        "scanned": 0,
        "parsed": 0,
        "empty": 0,
        "unparsable": 0,
        "orphan": 0,
        "skipped": 0,
        "written": 0,
        "failed": 0,
    }
    pending: dict = {}
    started = time.time()
    after_id = ""  # keyset cursor, advanced within this run only

    while True:
        rows = _scan(db, after_id, batch_size, tenant_id)
        if not rows:
            break
        after_id = rows[-1]["id"]

        for row in rows:
            stats["scanned"] += 1
            if not row.get("tenant_id"):
                stats["orphan"] += 1  # knowledgebase row is gone
                continue
            raw = row.get("meta_fields")
            if is_blank_meta_text(raw):
                # No metadata was ever stored for this document; see
                # is_blank_meta_text for why that is not counted as unparsable.
                stats["empty"] += 1
                continue
            meta = parse_meta_fields(raw)
            if meta is None:
                stats["unparsable"] += 1
                continue
            stats["parsed"] += 1
            pending.setdefault(row["tenant_id"], []).append({"id": row["id"], "kb_id": row["kb_id"], "meta_fields": meta})

        buffered = _drain(writer, pending, stats, batch_size, on_conflict, execute, full_only=True)
        if buffered >= batch_size * PENDING_BATCH_LIMIT:
            # The part-filled tenants have waited as long as that is worth; write
            # them now rather than carry them to the end of the scan.
            _drain(writer, pending, stats, batch_size, on_conflict, execute, full_only=False)

    _drain(writer, pending, stats, batch_size, on_conflict, execute, full_only=False)
    writer.refresh_all()

    stats["elapsed"] = time.time() - started
    logger.info(
        "document metadata backfill: scanned=%d parsed=%d empty=%d unparsable=%d orphan(no such knowledgebase)=%d already-present=%d %s=%d failed=%d in %.1fs",
        stats["scanned"],
        stats["parsed"],
        stats["empty"],
        stats["unparsable"],
        stats["orphan"],
        stats["skipped"],
        "written" if execute else "would-write",
        stats["written"],
        stats["failed"],
        stats["elapsed"],
    )
    return stats
