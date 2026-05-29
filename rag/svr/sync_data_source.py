#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

# from beartype import BeartypeConf
# from beartype.claw import beartype_all  # <-- you didn't sign up for this
# beartype_all(conf=BeartypeConf(violation_type=UserWarning))    # <-- emit warnings from all code


import time
start_ts = time.perf_counter()

import asyncio
import copy
import faulthandler
import logging
import os
import signal
import sys
import threading
import traceback
from datetime import datetime, timezone
from typing import Any

from flask import json

from api.utils.common import hash128
from api.db.services.connector_service import ConnectorService, SyncLogsService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from common import settings
from common.constants import ConnectorTaskType, FileSource, TaskStatus
from common.config_utils import show_configs
from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source import (
    BlobStorageConnector,
    RSSConnector,
    NotionConnector,
    DiscordConnector,
    GoogleDriveConnector,
    MoodleConnector,
    JiraConnector,
    DropboxConnector,
    AirtableConnector,
    AsanaConnector,
    ImapConnector,
    ZendeskConnector,
    SeaFileConnector,
    RDBMSConnector,
    DingTalkAITableConnector,
    RestAPIConnector,
    OneDriveConnector,
    OutlookConnector,
    TeamsConnector,
    SlackConnector,
    SharePointConnector,
)
from common.data_source.models import ConnectorFailure, SeafileSyncScope
from common.data_source.webdav_connector import WebDAVConnector
from common.data_source.confluence_connector import ConfluenceConnector
from common.data_source.gmail_connector import GmailConnector
from common.data_source.box_connector import BoxConnector
from common.data_source.github.connector import GithubConnector
from common.data_source.gitlab_connector import GitlabConnector
from common.data_source.bitbucket.connector import BitbucketConnector
from common.data_source.interfaces import CheckpointOutputWrapper
from common.data_source.exceptions import ConnectorValidationError
from common.log_utils import init_root_logger
from common.signal_utils import start_tracemalloc_and_snapshot, stop_tracemalloc
from common.versions import get_ragflow_version
from box_sdk_gen import BoxOAuth, OAuthConfig, AccessToken
MAX_CONCURRENT_TASKS = int(os.environ.get("MAX_CONCURRENT_TASKS", "5"))
task_limiter = asyncio.Semaphore(MAX_CONCURRENT_TASKS)


def _redact_mailbox(value: str) -> str:
    """Return a privacy-preserving representation of a UPN / email / object id.

    Sync logs surface connector configuration verbatim, so leaking the
    full mailbox list of a tenant is enough to inventory their org from
    a single log file. Preserve the first two characters of the local
    part as a debugging hint and mask the rest.
    """
    if not value:
        return "<empty>"
    if "@" in value:
        local, _, _domain = value.partition("@")
        local_mask = local if len(local) <= 2 else local[:2] + "***"
        return f"{local_mask}@***"
    return f"{value[:4]}***" if len(value) > 4 else "***"


class SyncBase:
    """
    Base class for all data source synchronization connectors.
    
    Defines the standard interface for connecting to external APIs, polling for 
    new or updated documents, and managing synchronization state intervals.
    """
    SOURCE_NAME: str = None

    def __init__(self, conf: dict) -> None:
        self.conf = conf

    @staticmethod
    def _format_window_boundary(value: datetime | None) -> str:
        if value is None:
            return "beginning"
        return value.astimezone().strftime("%Y-%m-%d %H:%M:%S %Z")

    @classmethod
    def window_info(cls, task: dict) -> str:
        window_start = None
        if task.get("reindex") != "1" and task.get("poll_range_start"):
            window_start = task["poll_range_start"]
        window_end = datetime.now(timezone.utc)
        return (
            f"sync window: {cls._format_window_boundary(window_start)}"
            f" -> {cls._format_window_boundary(window_end)}"
        )

    @classmethod
    def log_connection(
        cls,
        name: str,
        details: str,
        task: dict,
        extra: str = "",
    ):
        if task.get("skip_connection_log"):
            return
        if extra:
            logging.info("Connect to %s: %s, %s, %s", name, details, cls.window_info(task), extra)
            return
        logging.info("Connect to %s: %s, %s", name, details, cls.window_info(task))

    async def __call__(self, task: dict):
        """
        Entry point for executing a synchronization task worker.
        
        Manages task execution boundaries including status logging, asynchronous 
        timeouts, and top-level exception handling, while delegating the core 
        ingestion logic to `_run_task_logic`.
        """
        SyncLogsService.start(task["id"], task["connector_id"])

        async with task_limiter:
            try:
                await asyncio.wait_for(self._run_task_logic(task), timeout=task["timeout_secs"])

            except asyncio.TimeoutError:
                msg = f"Task timeout after {task['timeout_secs']} seconds"
                SyncLogsService.update_by_id(task["id"], {"status": TaskStatus.FAIL, "error_msg": msg})
                return

            except Exception as ex:
                msg = "\n".join([
                    "".join(traceback.format_exception_only(None, ex)).strip(),
                    "".join(traceback.format_exception(None, ex, ex.__traceback__)).strip(),
                ])
                SyncLogsService.update_by_id(task["id"], {
                    "status": TaskStatus.FAIL,
                    "full_exception_trace": msg,
                    "error_msg": str(ex)
                })
                return

        task_type = task.get("task_type", ConnectorTaskType.SYNC)
        if task_type == ConnectorTaskType.SYNC:
            SyncLogsService.schedule(
                task["connector_id"],
                task["kb_id"],
                task.get("poll_range_start"),
                task_type=ConnectorTaskType.SYNC,
            )
        elif task_type == ConnectorTaskType.PRUNE and self.conf.get("sync_deleted_files"):
            SyncLogsService.schedule(
                task["connector_id"],
                task["kb_id"],
                task_type=ConnectorTaskType.PRUNE,
            )

    async def _run_task_logic(self, task: dict):
        task_type = task.get("task_type", ConnectorTaskType.SYNC)
        if task_type == ConnectorTaskType.PRUNE:
            await self._run_prune_task_logic(task)
            return
        await self._run_sync_task_logic(task)

    async def _run_sync_task_logic(self, task: dict):
        """
        Executes the core synchronization pipeline for a data source task.
        """
        document_batch_generator = await self._generate(task)

        failed_docs = 0
        added_docs = 0
        updated_docs = 0
        next_update = datetime(1970, 1, 1, tzinfo=timezone.utc)
        source_type = f"{self.SOURCE_NAME}/{task['connector_id']}"
        existing_doc_ids = {
            doc["id"]
            for doc in DocumentService.list_doc_headers_by_kb_and_source_type(
                task["kb_id"],
                source_type,
            )
        }

        if task["poll_range_start"]:
            next_update = task["poll_range_start"]

        for document_batch in document_batch_generator:
            if not document_batch:
                continue

            max_update = max(doc.doc_updated_at for doc in document_batch)
            next_update = max(next_update, max_update)

            docs = []
            for doc in document_batch:
                d = {
                    "id": hash128(f"{task['connector_id']}:{doc.id}"),
                    "connector_id": task["connector_id"],
                    "source": self.SOURCE_NAME,
                    "semantic_identifier": doc.semantic_identifier,
                    "extension": doc.extension,
                    "size_bytes": doc.size_bytes,
                    "doc_updated_at": doc.doc_updated_at,
                    "blob": doc.blob,
                }
                if doc.metadata:
                    d["metadata"] = doc.metadata
                if getattr(doc, "fingerprint", None):
                    d["fingerprint"] = doc.fingerprint
                docs.append(d)

            try:
                e, kb = KnowledgebaseService.get_by_id(task["kb_id"])
                err, dids = SyncLogsService.duplicate_and_parse(
                    kb, docs, task["tenant_id"],
                    f"{self.SOURCE_NAME}/{task['connector_id']}",
                    task["auto_parse"]
                )
                SyncLogsService.increase_docs(
                    task["id"], max_update,
                    len(docs), "\n".join(err), len(err)
                )
                changed_doc_ids = set(dids)
                updated_in_batch = len(changed_doc_ids & existing_doc_ids)
                added_in_batch = len(changed_doc_ids) - updated_in_batch
                added_docs += added_in_batch
                updated_docs += updated_in_batch
                existing_doc_ids.update(changed_doc_ids)

            except Exception as batch_ex:
                msg = str(batch_ex)
                code = getattr(batch_ex, "args", [None])[0]

                if code == 1267 or "collation" in msg.lower():
                    logging.warning(f"Skipping {len(docs)} document(s) due to collation conflict")
                else:
                    logging.error(f"Error processing batch: {msg}")

                failed_docs += len(docs)
                continue

        prefix = self._get_source_prefix()
        prefix = f"{prefix} " if prefix else ""
        next_update_info = self._format_window_boundary(next_update)

        total_changed_docs = added_docs + updated_docs
        summary = (
            f"{prefix}sync summary till {next_update_info}: "
            f"total={total_changed_docs}, added={added_docs}, "
            f"updated={updated_docs}"
        )
        if failed_docs > 0:
            summary = f"{summary}, skipped={failed_docs}"
        logging.info(summary)

        if (
            isinstance(self, _RDBMSBase)
            and failed_docs == 0
        ):
            self.connector.persist_sync_state()
        SyncLogsService.done(task["id"], task["connector_id"])
        task["poll_range_start"] = next_update

    async def _run_prune_task_logic(self, task: dict):
        if not self.conf.get("sync_deleted_files"):
            SyncLogsService.done(task["id"], task["connector_id"])
            return

        await self._initialize_for_prune(task)

        file_list = self._collect_prune_snapshot(task)
        if file_list is None:
            logging.warning(
                "%s prune snapshot retrieval failed (connector_id=%s, kb_id=%s)",
                self.SOURCE_NAME,
                task["connector_id"],
                task["kb_id"],
            )
            SyncLogsService.done(task["id"], task["connector_id"])
            return

        removed_docs, cleanup_errors = ConnectorService.cleanup_stale_documents_for_task(
            task["id"],
            task["connector_id"],
            task["kb_id"],
            task["tenant_id"],
            file_list,
        )
        logging.info(
            "%s prune summary: deleted=%s, errors=%s",
            self.SOURCE_NAME,
            removed_docs,
            len(cleanup_errors),
        )
        SyncLogsService.done(task["id"], task["connector_id"])

    async def _generate(self, task: dict):
        raise NotImplementedError

    def _get_source_prefix(self):
        return ""

    async def _initialize_for_prune(self, task: dict):
        await self._generate(task)

    def _get_prune_snapshot_kwargs(self, task: dict) -> dict[str, Any]:
        return {}

    def _collect_prune_snapshot(self, task: dict):
        if not getattr(self, "connector", None):
            return None
        if not hasattr(self.connector, "retrieve_all_slim_docs_perm_sync"):
            return None

        file_list = []
        snapshot_kwargs = self._get_prune_snapshot_kwargs(task)
        try:
            for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync(**snapshot_kwargs):
                file_list.extend(slim_batch)
        except TypeError:
            for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync():
                file_list.extend(slim_batch)
        except Exception:
            logging.exception(
                "%s prune snapshot failed (connector_id=%s, kb_id=%s)",
                self.SOURCE_NAME,
                task["connector_id"],
                task["kb_id"],
            )
            return None
        return file_list


class _BlobLikeBase(SyncBase):
    DEFAULT_BUCKET_TYPE: str = "s3"

    def _fingerprint_filtered_generator(self, task: dict):
        """Generator that uses list_keys() + get_value() to skip unchanged objects.

        Pre-loads {doc_id: content_hash} for the connector's existing docs in
        this KB, iterates the bucket via list_keys(), and only materializes a
        Document (one GetObject call) when the listing fingerprint differs from
        the persisted content_hash. Unchanged objects are skipped entirely --
        no download, no re-parse.

        Per-key fetch failures are counted and surfaced via SyncLogsService so
        a partially failing sync (e.g. throttling, IAM regression mid-run)
        doesn't silently report DONE while half the bucket is unreachable.
        Connectors yielding KeyRecord(deleted=True) are skipped here -- actual
        deletion reconciliation lives in the unified delete pass (PR-4).
        """
        source_type = f"{self.SOURCE_NAME}/{task['connector_id']}"
        existing_fingerprints = DocumentService.list_id_content_hash_map_by_kb_and_source_type(
            task["kb_id"], source_type,
        )

        bypass_count = 0
        fetch_count = 0
        fail_count = 0
        batch = []
        for key_record in self.connector.list_keys():
            if key_record.deleted:
                continue

            doc_id = hash128(key_record.key)
            stored = existing_fingerprints.get(doc_id, "")
            if key_record.fingerprint and stored and key_record.fingerprint == stored:
                bypass_count += 1
                continue

            try:
                doc = self.connector.get_value(key_record.key)
            except Exception as ex:
                fail_count += 1
                logging.exception(
                    "Failed to fetch %s from %s: %s",
                    key_record.key,
                    self.SOURCE_NAME,
                    ex,
                )
                continue

            fetch_count += 1
            batch.append(doc)
            if len(batch) >= self.connector.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

        log_msg = (
            "[%s] fingerprint sync: %d bypassed, %d fetched, %d failed "
            "(connector_id=%s, kb_id=%s)"
        )
        log_args = (
            self.SOURCE_NAME,
            bypass_count,
            fetch_count,
            fail_count,
            task["connector_id"],
            task["kb_id"],
        )
        # Use WARNING when any fetch failed so partial-bucket regressions
        # (auth, throttling, IAM drift) surface without diving into the
        # per-exception traces above.
        if fail_count:
            logging.warning(log_msg, *log_args)
        else:
            logging.info(log_msg, *log_args)

    async def _generate(self, task: dict):
        bucket_type = self.conf.get("bucket_type", self.DEFAULT_BUCKET_TYPE)

        self.connector = BlobStorageConnector(
            bucket_type=bucket_type,
            bucket_name=self.conf["bucket_name"],
            prefix=self.conf.get("prefix", ""),
        )
        self.connector.set_allow_images(self.conf.get("allow_images", False))
        self.connector.load_credentials(self.conf["credentials"])

        # Fingerprint-bypass path: skip GetObject for unchanged ETags. Disabled
        # on full reindex (we want to re-fetch everything in that case).
        use_fingerprint_path = task["reindex"] != "1"
        if use_fingerprint_path:
            document_batch_generator = self._fingerprint_filtered_generator(task)
        else:
            document_batch_generator = self.connector.load_from_state()

        _begin_info = (
            "fingerprint-bypass"
            if use_fingerprint_path
            else "full reindex"
        )

        logging.info(
            "Connect to {}: {}(prefix/{}) {}".format(
                bucket_type,
                self.conf["bucket_name"],
                self.conf.get("prefix", ""),
                _begin_info,
            )
        )
        return document_batch_generator


class S3(_BlobLikeBase):
    SOURCE_NAME: str = FileSource.S3
    DEFAULT_BUCKET_TYPE: str = "s3"


class R2(_BlobLikeBase):
    SOURCE_NAME: str = FileSource.R2
    DEFAULT_BUCKET_TYPE: str = "r2"


class OCI_STORAGE(_BlobLikeBase):
    SOURCE_NAME: str = FileSource.OCI_STORAGE
    DEFAULT_BUCKET_TYPE: str = "oci_storage"


class GOOGLE_CLOUD_STORAGE(_BlobLikeBase):
    SOURCE_NAME: str = FileSource.GOOGLE_CLOUD_STORAGE
    DEFAULT_BUCKET_TYPE: str = "google_cloud_storage"


class RSS(SyncBase):
    SOURCE_NAME: str = FileSource.RSS

    async def _generate(self, task: dict):
        self.connector = RSSConnector(
            feed_url=self.conf["feed_url"],
            batch_size=self.conf.get("batch_size", INDEX_BATCH_SIZE),
        )
        self.connector.load_credentials(self.conf.get("credentials", {}))
        self.connector.validate_connector_settings()

        if task["reindex"] == "1" or not task["poll_range_start"]:
            return self.connector.load_from_state()

        end_time = datetime.now(timezone.utc).timestamp()

        document_generator = self.connector.poll_source(
            task["poll_range_start"].timestamp(),
            end_time,
        )
        return document_generator


class Confluence(SyncBase):
    SOURCE_NAME: str = FileSource.CONFLUENCE

    async def _generate(self, task: dict):
        from common.data_source.config import DocumentSource
        from common.data_source.interfaces import StaticCredentialsProvider

        index_mode = (self.conf.get("index_mode") or "everything").lower()
        if index_mode not in {"everything", "space", "page"}:
            index_mode = "everything"

        space = ""
        page_id = ""

        index_recursively = False
        if index_mode == "space":
            space = (self.conf.get("space") or "").strip()
            if not space:
                raise ValueError("Space Key is required when indexing a specific Confluence space.")
        elif index_mode == "page":
            page_id = (self.conf.get("page_id") or "").strip()
            if not page_id:
                raise ValueError("Page ID is required when indexing a specific Confluence page.")
            index_recursively = bool(self.conf.get("index_recursively", False))

        self.connector = ConfluenceConnector(
            wiki_base=self.conf["wiki_base"],
            is_cloud=self.conf.get("is_cloud", True),
            space=space,
            page_id=page_id,
            index_recursively=index_recursively,
            
        )

        credentials_provider = StaticCredentialsProvider(tenant_id=task["tenant_id"],
                                                         connector_name=DocumentSource.CONFLUENCE,
                                                         credential_json=self.conf["credentials"])
        self.connector.set_credentials_provider(credentials_provider)

        # Determine the time range for synchronization based on reindex or poll_range_start
        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = 0.0
        else:
            start_time = task["poll_range_start"].timestamp()
            
        end_time = datetime.now(timezone.utc).timestamp()

        raw_batch_size = self.conf.get("sync_batch_size") or self.conf.get("batch_size") or INDEX_BATCH_SIZE
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        def document_batches():
            checkpoint = self.connector.build_dummy_checkpoint()
            pending_docs = []
            iterations = 0
            iteration_limit = 100_000

            while checkpoint.has_more:
                wrapper = CheckpointOutputWrapper()
                doc_generator = wrapper(self.connector.load_from_checkpoint(start_time, end_time, checkpoint))
                for document, failure, next_checkpoint in doc_generator:
                    if failure is not None:
                        logging.warning("Confluence connector failure: %s",
                                        getattr(failure, "failure_message", failure))
                        continue
                    if document is not None:
                        pending_docs.append(document)
                        if len(pending_docs) >= batch_size:
                            yield pending_docs
                            pending_docs = []
                    if next_checkpoint is not None:
                        checkpoint = next_checkpoint

                iterations += 1
                if iterations > iteration_limit:
                    raise RuntimeError("Too many iterations while loading Confluence documents.")

            if pending_docs:
                yield pending_docs

        def wrapper():
            for batch in document_batches():
                yield batch

        self.log_connection("Confluence", self.conf["wiki_base"], task)
        return wrapper()


class Notion(SyncBase):
    SOURCE_NAME: str = FileSource.NOTION

    async def _generate(self, task: dict):
        self.connector = NotionConnector(root_page_id=self.conf["root_page_id"])
        self.connector.load_credentials(self.conf["credentials"])
        document_generator = (
            self.connector.load_from_state()
            if task["reindex"] == "1" or not task["poll_range_start"]
            else self.connector.poll_source(task["poll_range_start"].timestamp(),
                                            datetime.now(timezone.utc).timestamp())
        )

        _begin_info = "totally" if task["reindex"] == "1" or not task["poll_range_start"] else "from {}".format(
            task["poll_range_start"])
        self.log_connection("Notion", f"root({self.conf['root_page_id']})", task)
        return document_generator


class Discord(SyncBase):
    SOURCE_NAME: str = FileSource.DISCORD

    async def _generate(self, task: dict):
        server_ids: str | None = self.conf.get("server_ids", None)
        # "channel1,channel2"
        channel_names: str | None = self.conf.get("channel_names", None)

        self.connector = DiscordConnector(
            server_ids=server_ids.split(",") if server_ids else [],
            channel_names=channel_names.split(",") if channel_names else [],
            start_date=datetime(1970, 1, 1, tzinfo=timezone.utc).strftime("%Y-%m-%d"),
            batch_size=self.conf.get("batch_size", 1024),
        )
        self.connector.load_credentials(self.conf["credentials"])
        document_generator = (
            self.connector.load_from_state()
            if task["reindex"] == "1" or not task["poll_range_start"]
            else self.connector.poll_source(task["poll_range_start"].timestamp(),
                                            datetime.now(timezone.utc).timestamp())
        )

        _begin_info = "totally" if task["reindex"] == "1" or not task["poll_range_start"] else "from {}".format(
            task["poll_range_start"])
        self.log_connection("Discord", f"servers({server_ids}), channel({channel_names})", task)
        return document_generator


class Gmail(SyncBase):
    SOURCE_NAME: str = FileSource.GMAIL

    async def _generate(self, task: dict):
        # Gmail sync reuses the generic LoadConnector/PollConnector interface
        # implemented by common.data_source.gmail_connector.GmailConnector.
        #
        # Config expectations (self.conf):
        #   credentials: Gmail / Workspace OAuth JSON (with primary admin email)
        #   batch_size:  optional, defaults to INDEX_BATCH_SIZE
        batch_size = self.conf.get("batch_size", INDEX_BATCH_SIZE)

        self.connector = GmailConnector(batch_size=batch_size)

        credentials = self.conf.get("credentials")
        if not credentials:
            raise ValueError("Gmail connector is missing credentials.")

        new_credentials = self.connector.load_credentials(credentials)
        if new_credentials:
            # Persist rotated / refreshed credentials back to connector config
            try:
                updated_conf = copy.deepcopy(self.conf)
                updated_conf["credentials"] = new_credentials
                ConnectorService.update_by_id(task["connector_id"], {"config": updated_conf})
                self.conf = updated_conf
                logging.info(
                    "Persisted refreshed Gmail credentials for connector %s",
                    task["connector_id"],
                )
            except Exception:
                logging.exception(
                    "Failed to persist refreshed Gmail credentials for connector %s",
                    task["connector_id"],
                )

        # Decide between full reindex and incremental polling by time range.
        if task["reindex"] == "1" or not task.get("poll_range_start"):
            start_time = None
            end_time = None
            _begin_info = "totally"
            document_generator = self.connector.load_from_state()
        else:
            poll_start = task["poll_range_start"]
            # Defensive: if poll_start is somehow None, fall back to full load
            if poll_start is None:
                start_time = None
                end_time = None
                _begin_info = "totally"
                document_generator = self.connector.load_from_state()
            else:
                start_time = poll_start.timestamp()
                end_time = datetime.now(timezone.utc).timestamp()
                _begin_info = f"from {poll_start}"
                document_generator = self.connector.poll_source(start_time, end_time)

        try:
            admin_email = self.connector.primary_admin_email
        except RuntimeError:
            admin_email = "unknown"
        self.log_connection("Gmail", f"as {admin_email}", task)
        return document_generator


class Dropbox(SyncBase):
    SOURCE_NAME: str = FileSource.DROPBOX

    async def _generate(self, task: dict):
        self.connector = DropboxConnector(batch_size=self.conf.get("batch_size", INDEX_BATCH_SIZE))
        self.connector.load_credentials(self.conf["credentials"])
        poll_start = task["poll_range_start"]
        if task["reindex"] == "1" or not poll_start:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            end_time = datetime.now(timezone.utc).timestamp()
            document_generator = self.connector.poll_source(poll_start.timestamp(), end_time)
            _begin_info = f"from {poll_start}"

        self.log_connection("Dropbox", "workspace", task)
        return document_generator


class GoogleDrive(SyncBase):
    """
    Data synchronization connector for Google Drive.
    Handles both full re-indexing and incremental polling, including the capability
    to synchronize deleted files by retrieving a lightweight snapshot of current files.
    """
    SOURCE_NAME: str = FileSource.GOOGLE_DRIVE

    async def _generate(self, task: dict):
        """Generates document batches from Google Drive, handling both full and incremental syncs."""
        connector_kwargs = {
            "include_shared_drives": self.conf.get("include_shared_drives", False),
            "include_my_drives": self.conf.get("include_my_drives", False),
            "include_files_shared_with_me": self.conf.get("include_files_shared_with_me", False),
            "shared_drive_urls": self.conf.get("shared_drive_urls"),
            "my_drive_emails": self.conf.get("my_drive_emails"),
            "shared_folder_urls": self.conf.get("shared_folder_urls"),
            "specific_user_emails": self.conf.get("specific_user_emails"),
            "batch_size": self.conf.get("batch_size", INDEX_BATCH_SIZE),
        }
        self.connector = GoogleDriveConnector(**connector_kwargs)
        self.connector.set_allow_images(self.conf.get("allow_images", False))

        credentials = self.conf.get("credentials")
        if not credentials:
            raise ValueError("Google Drive connector is missing credentials.")

        new_credentials = self.connector.load_credentials(credentials)
        if new_credentials:
            self._persist_rotated_credentials(task["connector_id"], new_credentials)

        # Capture end_time BEFORE the snapshot to prevent the ingestion race condition
        end_time = datetime.now(timezone.utc).timestamp()

        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = 0.0
            _begin_info = "totally"
        else:
            start_time = task["poll_range_start"].timestamp()
            _begin_info = f"from {task['poll_range_start']}"
                
        raw_batch_size = self.conf.get("sync_batch_size") or self.conf.get("batch_size") or INDEX_BATCH_SIZE
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        def document_batches():
            """Yields paginated batches of parsed Google Drive documents using checkpoints."""
            checkpoint = self.connector.build_dummy_checkpoint()
            pending_docs = []
            iterations = 0
            iteration_limit = 100_000

            while checkpoint.has_more:
                wrapper = CheckpointOutputWrapper()
                doc_generator = wrapper(self.connector.load_from_checkpoint(start_time, end_time, checkpoint))
                for document, failure, next_checkpoint in doc_generator:
                    if failure is not None:
                        logging.warning("Google Drive connector failure: %s",
                                        getattr(failure, "failure_message", failure))
                        continue
                    if document is not None:
                        pending_docs.append(document)
                        if len(pending_docs) >= batch_size:
                            yield pending_docs
                            pending_docs = []
                    if next_checkpoint is not None:
                        checkpoint = next_checkpoint

                iterations += 1
                if iterations > iteration_limit:
                    raise RuntimeError("Too many iterations while loading Google Drive documents.")

            if pending_docs:
                yield pending_docs

        try:
            admin_email = self.connector.primary_admin_email
        except RuntimeError:
            admin_email = "unknown"
        self.log_connection("Google Drive", f"as {admin_email}", task)
        
        return document_batches()

    def _persist_rotated_credentials(self, connector_id: str, credentials: dict[str, Any]) -> None:
        """Saves refreshed OAuth credentials back to the database configuration."""
        try:
            updated_conf = copy.deepcopy(self.conf)
            updated_conf["credentials"] = credentials
            ConnectorService.update_by_id(connector_id, {"config": updated_conf})
            self.conf = updated_conf
            logging.info("Persisted refreshed Google Drive credentials for connector %s", connector_id)
        except Exception:
            logging.exception("Failed to persist refreshed Google Drive credentials for connector %s", connector_id)
            
class Jira(SyncBase):
    SOURCE_NAME: str = FileSource.JIRA

    def _get_source_prefix(self):
        return "[Jira]"

    async def _generate(self, task: dict):
        connector_kwargs = {
            "jira_base_url": self.conf["base_url"],
            "project_key": self.conf.get("project_key"),
            "jql_query": self.conf.get("jql_query"),
            "batch_size": self.conf.get("batch_size", INDEX_BATCH_SIZE),
            "include_comments": self.conf.get("include_comments", True),
            "include_attachments": self.conf.get("include_attachments", False),
            "labels_to_skip": self._normalize_list(self.conf.get("labels_to_skip")),
            "comment_email_blacklist": self._normalize_list(self.conf.get("comment_email_blacklist")),
            "scoped_token": self.conf.get("scoped_token", False),
            "attachment_size_limit": self.conf.get("attachment_size_limit"),
            "timezone_offset": self.conf.get("timezone_offset"),
            "time_buffer_seconds": self.conf.get("time_buffer_seconds"),
        }

        self.connector = JiraConnector(**connector_kwargs)

        credentials = self.conf.get("credentials")
        if not credentials:
            raise ValueError("Jira connector is missing credentials.")

        self.connector.load_credentials(credentials)
        self.connector.validate_connector_settings()

        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = 0.0
            _begin_info = "totally"
        else:
            start_time = task["poll_range_start"].timestamp()
            _begin_info = f"from {task['poll_range_start']}"

        end_time = datetime.now(timezone.utc).timestamp()

        raw_batch_size = self.conf.get("sync_batch_size") or self.conf.get("batch_size") or INDEX_BATCH_SIZE
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        def document_batches():
            checkpoint = self.connector.build_dummy_checkpoint()
            pending_docs = []
            iterations = 0
            iteration_limit = 100_000

            while checkpoint.has_more:
                wrapper = CheckpointOutputWrapper()
                generator = wrapper(
                    self.connector.load_from_checkpoint(
                        start_time,
                        end_time,
                        checkpoint,
                    )
                )
                for document, failure, next_checkpoint in generator:
                    if failure is not None:
                        logging.warning(
                            f"[Jira] Jira connector failure: {getattr(failure, 'failure_message', failure)}"
                        )
                        continue
                    if document is not None:
                        pending_docs.append(document)
                        if len(pending_docs) >= batch_size:
                            yield pending_docs
                            pending_docs = []
                    if next_checkpoint is not None:
                        checkpoint = next_checkpoint

                iterations += 1
                if iterations > iteration_limit:
                    logging.error(f"[Jira] Task {task.get('id')} exceeded iteration limit ({iteration_limit}).")
                    raise RuntimeError("Too many iterations while loading Jira documents.")

            if pending_docs:
                yield pending_docs

        self.log_connection(
            "Jira",
            connector_kwargs["jira_base_url"],
            task,
            (
                f"sync_batch_size={batch_size}, "
                f"overlap_buffer_s={getattr(self.connector, 'time_buffer_seconds', connector_kwargs.get('time_buffer_seconds'))}"
            ),
        )
        return document_batches()

    @staticmethod
    def _normalize_list(values: Any) -> list[str] | None:
        if values is None:
            return None
        if isinstance(values, str):
            values = [item.strip() for item in values.split(",")]
        return [str(value).strip() for value in values if value is not None and str(value).strip()]


class SharePoint(SyncBase):
    SOURCE_NAME: str = FileSource.SHAREPOINT

    async def _generate(self, task: dict):
        self.connector = SharePointConnector(
            batch_size=self.conf.get("batch_size", INDEX_BATCH_SIZE),
        )

        credentials = self.conf.get("credentials") or {}
        self.connector.load_credentials(credentials)
        self.connector.validate_connector_settings()

        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = 0.0
            _begin_info = "totally"
        else:
            start_time = task["poll_range_start"].timestamp()
            _begin_info = f"from {task['poll_range_start']}"

        end_time = datetime.now(timezone.utc).timestamp()

        raw_batch_size = self.conf.get("sync_batch_size") or self.conf.get("batch_size") or INDEX_BATCH_SIZE
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        def document_batches():
            checkpoint = self.connector.build_dummy_checkpoint()
            pending_docs = []
            iterations = 0
            iteration_limit = 100_000

            while checkpoint.has_more:
                wrapper = CheckpointOutputWrapper()
                doc_generator = wrapper(
                    self.connector.load_from_checkpoint(start_time, end_time, checkpoint)
                )
                for document, failure, next_checkpoint in doc_generator:
                    if failure is not None:
                        logging.warning(
                            "SharePoint connector failure: %s",
                            getattr(failure, "failure_message", failure),
                        )
                        continue
                    if document is not None:
                        pending_docs.append(document)
                        if len(pending_docs) >= batch_size:
                            yield pending_docs
                            pending_docs = []
                    if next_checkpoint is not None:
                        checkpoint = next_checkpoint

                iterations += 1
                if iterations > iteration_limit:
                    raise RuntimeError("Too many iterations while loading SharePoint documents.")

            if pending_docs:
                yield pending_docs

        self.log_connection("SharePoint", self.conf.get("credentials", {}).get("site_url", ""), task)
        return document_batches()


class OneDrive(SyncBase):
    SOURCE_NAME: str = FileSource.ONEDRIVE

    async def _generate(self, task: dict):
        raw_batch_size = self.conf.get("batch_size", INDEX_BATCH_SIZE)
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        self.connector = OneDriveConnector(
            batch_size=batch_size,
            folder_path=self.conf.get("folder_path") or None,
        )
        self.connector.load_credentials(self.conf["credentials"])

        # Always route through load_from_checkpoint so the connector owns the
        # delta-link bookkeeping; incremental runs pass the previous poll
        # range start as the lastModifiedDateTime floor while the same delta
        # walk drives both modes. poll_source disregarded the checkpoint
        # entirely, which would have re-walked every drive's root each run.
        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_ts = 0.0
        else:
            start_ts = task["poll_range_start"].timestamp()
        end_ts = datetime.now(timezone.utc).timestamp()
        checkpoint = self.connector.build_dummy_checkpoint()
        document_batch_generator = self.connector.load_from_checkpoint(
            start_ts, end_ts, checkpoint
        )

        self.log_connection(
            "OneDrive",
            self.conf.get("folder_path", "/") or "/",
            task,
        )

        def wrapper():
            for document_batch in document_batch_generator:
                yield document_batch

        return wrapper()


class Outlook(SyncBase):
    SOURCE_NAME: str = FileSource.OUTLOOK

    async def _generate(self, task: dict):
        raw_batch_size = self.conf.get("batch_size", INDEX_BATCH_SIZE)
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        raw_user_ids = self.conf.get("user_ids")
        if isinstance(raw_user_ids, str):
            user_ids = [u.strip() for u in raw_user_ids.split(",") if u.strip()]
        elif isinstance(raw_user_ids, list):
            user_ids = [str(u).strip() for u in raw_user_ids if str(u).strip()]
        else:
            user_ids = []

        self.connector = OutlookConnector(
            batch_size=batch_size,
            folder=self.conf.get("folder") or "inbox",
            user_ids=user_ids or None,
        )
        self.connector.load_credentials(self.conf["credentials"])

        # Always route through load_from_checkpoint so the connector owns the
        # delta-link bookkeeping; incremental runs pass the previous poll
        # range start as the receivedDateTime floor while the same delta
        # walk drives both modes. poll_source disregarded the checkpoint
        # entirely, which would have re-walked every mailbox each run.
        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_ts = 0.0
        else:
            start_ts = task["poll_range_start"].timestamp()
        end_ts = datetime.now(timezone.utc).timestamp()
        checkpoint = self.connector.build_dummy_checkpoint()
        document_batch_generator = self.connector.load_from_checkpoint(
            start_ts, end_ts, checkpoint
        )

        # Redact mailbox identifiers — full UPN / email lists in connector
        # logs leak PII (the entire org's mail directory ends up in
        # tail-of-logs output). Surface the folder, the count, and a small
        # masked preview so operators can still spot a misconfigured run.
        if user_ids:
            preview = ",".join(_redact_mailbox(u) for u in user_ids[:3])
            if len(user_ids) > 3:
                preview = f"{preview},+{len(user_ids) - 3} more"
            details = "{}@{} users (preview: {})".format(
                self.conf.get("folder", "inbox"),
                len(user_ids),
                preview,
            )
        else:
            details = "{}@<all-users>".format(self.conf.get("folder", "inbox"))
        self.log_connection("Outlook", details, task)

        def wrapper():
            for document_batch in document_batch_generator:
                yield document_batch

        return wrapper()


class Slack(SyncBase):
    SOURCE_NAME: str = FileSource.SLACK

    async def _generate(self, task: dict):
        from common.data_source.config import DocumentSource
        from common.data_source.interfaces import StaticCredentialsProvider

        channels_conf = self.conf.get("channels")
        if isinstance(channels_conf, str):
            channels = [c.strip() for c in channels_conf.split(",") if c.strip()]
        elif isinstance(channels_conf, list):
            channels = [str(c).strip() for c in channels_conf if str(c).strip()]
        else:
            channels = None

        raw_batch_size = self.conf.get("batch_size", INDEX_BATCH_SIZE)
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        self.connector = SlackConnector(
            channels=channels or None,
            channel_regex_enabled=bool(self.conf.get("channel_regex_enabled", False)),
            batch_size=batch_size,
        )

        credentials = self.conf.get("credentials") or {}
        if not credentials.get("slack_bot_token"):
            raise ValueError("Slack connector is missing the bot token credential.")

        credentials_provider = StaticCredentialsProvider(
            tenant_id=task["tenant_id"],
            connector_name=DocumentSource.SLACK,
            credential_json=credentials,
        )
        self.connector.set_credentials_provider(credentials_provider)
        self.connector.validate_connector_settings()

        poll_start = task["poll_range_start"]
        if task["reindex"] == "1" or not poll_start:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            end_time = datetime.now(timezone.utc).timestamp()
            document_generator = self.connector.poll_source(poll_start.timestamp(), end_time)
            _begin_info = f"from {poll_start}"

        self.log_connection(
            "Slack",
            f"channels({', '.join(channels) if channels else 'all'})",
            task,
        )
        return document_generator


class Teams(SyncBase):
    SOURCE_NAME: str = FileSource.TEAMS

    async def _generate(self, task: dict):
        self.connector = TeamsConnector(
            batch_size=self.conf.get("batch_size", INDEX_BATCH_SIZE),
        )

        credentials = self.conf.get("credentials") or {}
        self.connector.load_credentials(credentials)
        self.connector.validate_connector_settings()

        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = 0.0
            _begin_info = "totally"
        else:
            start_time = task["poll_range_start"].timestamp()
            _begin_info = f"from {task['poll_range_start']}"

        end_time = datetime.now(timezone.utc).timestamp()

        raw_batch_size = self.conf.get("sync_batch_size") or self.conf.get("batch_size") or INDEX_BATCH_SIZE
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        def document_batches():
            checkpoint = self.connector.build_dummy_checkpoint()
            pending_docs = []
            iterations = 0
            iteration_limit = 100_000

            while checkpoint.has_more:
                wrapper = CheckpointOutputWrapper()
                doc_generator = wrapper(
                    self.connector.load_from_checkpoint(start_time, end_time, checkpoint)
                )
                for document, failure, next_checkpoint in doc_generator:
                    if failure is not None:
                        logging.warning(
                            "Teams connector failure: %s",
                            getattr(failure, "failure_message", failure),
                        )
                        continue
                    if document is not None:
                        pending_docs.append(document)
                        if len(pending_docs) >= batch_size:
                            yield pending_docs
                            pending_docs = []
                    if next_checkpoint is not None:
                        checkpoint = next_checkpoint

                iterations += 1
                if iterations > iteration_limit:
                    raise RuntimeError("Too many iterations while loading Teams documents.")

            if pending_docs:
                yield pending_docs

        self.log_connection("Microsoft Teams", "workspace", task)
        return document_batches()


class WebDAV(SyncBase):
    SOURCE_NAME: str = FileSource.WEBDAV

    async def _generate(self, task: dict):
        raw_batch_size = self.conf.get("batch_size", INDEX_BATCH_SIZE)
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        self.connector = WebDAVConnector(
            base_url=self.conf["base_url"],
            remote_path=self.conf.get("remote_path", "/"),
            batch_size=batch_size,
        )
        self.connector.set_allow_images(self.conf.get("allow_images", False))
        self.connector.load_credentials(self.conf["credentials"])

        if task["reindex"] == "1" or not task["poll_range_start"]:
            document_batch_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            end_ts = datetime.now(timezone.utc).timestamp()
            document_batch_generator = self.connector.poll_source(
                task["poll_range_start"].timestamp(),
                end_ts,
            )
            _begin_info = "from {}".format(task["poll_range_start"])

        self.log_connection("WebDAV", f"{self.conf['base_url']}(path: {self.conf.get('remote_path', '/')})", task)

        def wrapper():
            for document_batch in document_batch_generator:
                yield document_batch

        return wrapper()


class Moodle(SyncBase):
    SOURCE_NAME: str = FileSource.MOODLE

    async def _generate(self, task: dict):
        self.connector = MoodleConnector(
            moodle_url=self.conf["moodle_url"],
            batch_size=self.conf.get("batch_size", INDEX_BATCH_SIZE)
        )

        self.connector.load_credentials(self.conf["credentials"])

        # Determine the time range for synchronization based on reindex or poll_range_start
        poll_start = task.get("poll_range_start")

        if task["reindex"] == "1" or poll_start is None:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            # Freeze the poll end time BEFORE the slim snapshot so that the
            # snapshot and the poll cover the same point in time. Without
            # this, a module created between the snapshot and the poll
            # could be polled as new and at the same time be missing from
            # the slim list, which would mark it as stale and delete it.
            end_ts = datetime.now(timezone.utc).timestamp()
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                end_ts,
            )
            _begin_info = f"from {poll_start}"

        self.log_connection("Moodle", self.conf["moodle_url"], task)
        return document_generator


class BOX(SyncBase):
    SOURCE_NAME: str = FileSource.BOX

    async def _generate(self, task: dict):
        self.connector = BoxConnector(
            folder_id=self.conf.get("folder_id", "0"),
        )

        credential = json.loads(self.conf['credentials']['box_tokens'])

        auth = BoxOAuth(
            OAuthConfig(
                client_id=credential['client_id'],
                client_secret=credential['client_secret'],
            )
        )

        token = AccessToken(
            access_token=credential['access_token'],
            refresh_token=credential['refresh_token'],
        )
        auth.token_storage.store(token)

        self.connector.load_credentials(auth)
        poll_start = task["poll_range_start"]

        if task["reindex"] == "1" or poll_start is None:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                datetime.now(timezone.utc).timestamp(),
            )
            _begin_info = f"from {poll_start}"
        self.log_connection("Box", f"folder_id({self.conf['folder_id']})", task)
        return document_generator


class Airtable(SyncBase):
    SOURCE_NAME: str = FileSource.AIRTABLE

    async def _generate(self, task: dict):
        """
        Sync files from Airtable attachments.
        """

        self.connector = AirtableConnector(
            base_id=self.conf.get("base_id"),
            table_name_or_id=self.conf.get("table_name_or_id"),
        )

        credentials = self.conf.get("credentials", {})
        if "airtable_access_token" not in credentials:
            raise ValueError("Missing airtable_access_token in credentials")

        self.connector.load_credentials(
            {"airtable_access_token": credentials["airtable_access_token"]}
        )

        poll_start = task.get("poll_range_start")

        if task.get("reindex") == "1" or poll_start is None:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                datetime.now(timezone.utc).timestamp(),
            )
            _begin_info = f"from {poll_start}"

        self.log_connection(
            "Airtable",
            f"base_id({self.conf.get('base_id')}), table({self.conf.get('table_name_or_id')})",
            task,
        )

        return document_generator

class Asana(SyncBase):
    SOURCE_NAME: str = FileSource.ASANA

    async def _generate(self, task: dict):
        self.connector = AsanaConnector(
            self.conf.get("asana_workspace_id"),
            self.conf.get("asana_project_ids"),
            self.conf.get("asana_team_id"),
        )
        credentials = self.conf.get("credentials", {})
        if "asana_api_token_secret" not in credentials:
            raise ValueError("Missing asana_api_token_secret in credentials")

        self.connector.load_credentials(
            {"asana_api_token_secret": credentials["asana_api_token_secret"]}
        )

        poll_start = task.get("poll_range_start")

        if task.get("reindex") == "1" or not poll_start:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            end_time = datetime.now(timezone.utc).timestamp()
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                end_time,
            )
            _begin_info = f"from {poll_start}"

        self.log_connection(
            "Asana",
            f"workspace_id({self.conf.get('asana_workspace_id')}), project_ids({self.conf.get('asana_project_ids')}), team_id({self.conf.get('asana_team_id')})",
            task,
        )

        return document_generator

class Github(SyncBase):
    SOURCE_NAME: str = FileSource.GITHUB

    async def _generate(self, task: dict):
        """
        Sync files from Github repositories.
        """
        from common.data_source.connector_runner import ConnectorRunner

        self.connector = GithubConnector(
            repo_owner=self.conf.get("repository_owner"),
            repositories=self.conf.get("repository_name"),
            include_prs=self.conf.get("include_pull_requests", True),
            include_issues=self.conf.get("include_issues", True),
        )

        credentials = self.conf.get("credentials", {})
        if "github_access_token" not in credentials:
            raise ValueError("Missing github_access_token in credentials")

        self.connector.load_credentials(
            {"github_access_token": credentials["github_access_token"]}
        )

        if task.get("reindex") == "1" or not task.get("poll_range_start"):
            start_time = datetime.fromtimestamp(0, tz=timezone.utc)
        else:
            start_time = task.get("poll_range_start")

        end_time = datetime.now(timezone.utc)

        runner = ConnectorRunner(
            connector=self.connector,
            batch_size=self.conf.get("batch_size", INDEX_BATCH_SIZE),
            include_permissions=False,
            time_range=(start_time, end_time)
        )

        def document_batches():
            checkpoint = self.connector.build_dummy_checkpoint()

            while checkpoint.has_more:
                for doc_batch, failure, next_checkpoint in runner.run(checkpoint):
                    if failure is not None:
                        logging.warning(
                            "Github connector failure: %s",
                            getattr(failure, "failure_message", failure),
                        )
                        continue
                    if doc_batch is not None:
                        yield doc_batch
                    if next_checkpoint is not None:
                        checkpoint = next_checkpoint

        def wrapper():
            for batch in document_batches():
                yield batch

        self.log_connection(
            "Github",
            f"org_name({self.conf.get('repository_owner')}), repo_names({self.conf.get('repository_name')})",
            task,
        )

        return wrapper()

class IMAP(SyncBase):
    SOURCE_NAME: str = FileSource.IMAP

    async def _generate(self, task):
        from common.data_source.config import DocumentSource
        from common.data_source.interfaces import StaticCredentialsProvider
        self.connector = ImapConnector(
            host=self.conf.get("imap_host"),
            port=self.conf.get("imap_port"),
            mailboxes=self.conf.get("imap_mailbox"),
        )
        credentials_provider = StaticCredentialsProvider(tenant_id=task["tenant_id"], connector_name=DocumentSource.IMAP, credential_json=self.conf["credentials"])
        self.connector.set_credentials_provider(credentials_provider)
        end_time = datetime.now(timezone.utc).timestamp()
        try:
            poll_range_days = float(self.conf.get("poll_range", 30))
        except (TypeError, ValueError):
            poll_range_days = 30
        default_initial_sync_start = end_time - poll_range_days * 24 * 60 * 60
        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = default_initial_sync_start
            _begin_info = "totally"
        else:
            start_time = task["poll_range_start"].timestamp()
            _begin_info = f"from {task['poll_range_start']}"

        if task["reindex"] == "1":
            initial_sync_start = default_initial_sync_start
            should_persist_initial_start = True
        else:
            initial_sync_start = self.conf.get("imap_initial_sync_start")
            should_persist_initial_start = initial_sync_start is None
            try:
                initial_sync_start = float(initial_sync_start)
            except (TypeError, ValueError):
                initial_sync_start = (
                    0 if task["poll_range_start"] else default_initial_sync_start
                )
                should_persist_initial_start = True

        if should_persist_initial_start:
            updated_conf = copy.deepcopy(self.conf)
            updated_conf["imap_initial_sync_start"] = initial_sync_start
            try:
                ConnectorService.update_by_id(
                    task["connector_id"], {"config": updated_conf}
                )
                self.conf = updated_conf
            except Exception:
                logging.exception(
                    "Failed to persist IMAP initial sync start for connector %s",
                    task["connector_id"],
                )

        self._prune_snapshot_kwargs = {
            "start": initial_sync_start,
            "end": end_time,
        }

        raw_batch_size = self.conf.get("sync_batch_size") or self.conf.get("batch_size") or INDEX_BATCH_SIZE
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        def document_batches():
            checkpoint = self.connector.build_dummy_checkpoint()
            pending_docs = []
            iterations = 0
            iteration_limit = 100_000
            while checkpoint.has_more:
                wrapper = CheckpointOutputWrapper()
                doc_generator = wrapper(self.connector.load_from_checkpoint(start_time, end_time, checkpoint))
                for document, failure, next_checkpoint in doc_generator:
                    if failure is not None:
                        logging.warning("IMAP connector failure: %s", getattr(failure, "failure_message", failure))
                        continue
                    if document is not None:
                        pending_docs.append(document)
                        if len(pending_docs) >= batch_size:
                            yield pending_docs
                            pending_docs = []
                    if next_checkpoint is not None:
                        checkpoint = next_checkpoint

                iterations += 1
                if iterations > iteration_limit:
                    raise RuntimeError("Too many iterations while loading IMAP documents.")

            if pending_docs:
                yield pending_docs

        def wrapper():
            for batch in document_batches():
                yield batch

        self.log_connection(
            "IMAP",
            f"host({self.conf['imap_host']}) port({self.conf['imap_port']}) user({self.conf['credentials']['imap_username']}) folder({self.conf['imap_mailbox']})",
            task,
        )
        return wrapper()

    def _get_prune_snapshot_kwargs(self, task: dict) -> dict[str, Any]:
        return getattr(self, "_prune_snapshot_kwargs", {})

class Zendesk(SyncBase):

    SOURCE_NAME: str = FileSource.ZENDESK
    async def _generate(self, task: dict):
        self.connector = ZendeskConnector(content_type=self.conf.get("zendesk_content_type"))
        self.connector.load_credentials(self.conf["credentials"])

        end_time = datetime.now(timezone.utc).timestamp()
        if task["reindex"] == "1" or not task.get("poll_range_start"):
            start_time = 0
            _begin_info = "totally"
        else:
            start_time = task["poll_range_start"].timestamp()
            _begin_info = f"from {task['poll_range_start']}"

        raw_batch_size = (
            self.conf.get("sync_batch_size")
            or self.conf.get("batch_size")
            or INDEX_BATCH_SIZE
        )
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE

        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        def document_batches():
            checkpoint = self.connector.build_dummy_checkpoint()
            pending_docs = []
            iterations = 0
            iteration_limit = 100_000

            while checkpoint.has_more:
                wrapper = CheckpointOutputWrapper()
                doc_generator = wrapper(
                    self.connector.load_from_checkpoint(
                        start_time, end_time, checkpoint
                    )
                )

                for document, failure, next_checkpoint in doc_generator:
                    if failure is not None:
                        logging.warning(
                            "Zendesk connector failure: %s",
                            getattr(failure, "failure_message", failure),
                        )
                        continue

                    if document is not None:
                        pending_docs.append(document)
                        if len(pending_docs) >= batch_size:
                            yield pending_docs
                            pending_docs = []

                    if next_checkpoint is not None:
                        checkpoint = next_checkpoint

                iterations += 1
                if iterations > iteration_limit:
                    raise RuntimeError(
                        "Too many iterations while loading Zendesk documents."
                    )

            if pending_docs:
                yield pending_docs

        def wrapper():
            for batch in document_batches():
                yield batch

        self.log_connection("Zendesk", f"subdomain({self.conf['credentials'].get('zendesk_subdomain')})", task)
        return wrapper()


class Gitlab(SyncBase):
    SOURCE_NAME: str = FileSource.GITLAB

    async def _generate(self, task: dict):
        """
        Sync files from GitLab attachments.
        """

        self.connector = GitlabConnector(
            project_owner= self.conf.get("project_owner"),
            project_name= self.conf.get("project_name"),
            include_mrs = self.conf.get("include_mrs", False),
            include_issues = self.conf.get("include_issues", False),
            include_code_files=  self.conf.get("include_code_files", False),
        )

        self.connector.load_credentials(
            {
                "gitlab_access_token": self.conf.get("credentials", {}).get("gitlab_access_token"),
                "gitlab_url": self.conf.get("gitlab_url"),
            }
        )

        if task["reindex"] == "1" or not task["poll_range_start"]:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            poll_start = task["poll_range_start"]
            if poll_start is None:
                document_generator = self.connector.load_from_state()
                _begin_info = "totally"
            else:
                document_generator = self.connector.poll_source(
                    poll_start.timestamp(),
                    datetime.now(timezone.utc).timestamp()
                )
                _begin_info = "from {}".format(poll_start)
        self.log_connection("Gitlab", f"({self.conf['project_name']})", task)
        return document_generator


class Bitbucket(SyncBase):
    SOURCE_NAME: str = FileSource.BITBUCKET

    async def _generate(self, task: dict):
        self.connector = BitbucketConnector(
            workspace=self.conf.get("workspace"),
            repositories=self.conf.get("repository_slugs"),
            projects=self.conf.get("projects"),
        )

        self.connector.load_credentials(
            {
            "bitbucket_email": self.conf["credentials"].get("bitbucket_account_email"),
            "bitbucket_api_token": self.conf["credentials"].get("bitbucket_api_token"),
            }
        )

        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = datetime.fromtimestamp(0, tz=timezone.utc)
            _begin_info = "totally"
        else:
            start_time = task.get("poll_range_start")
            _begin_info = f"from {start_time}"
        
        end_time = datetime.now(timezone.utc)

        def document_batches():
            checkpoint = self.connector.build_dummy_checkpoint()

            while checkpoint.has_more:
                gen = self.connector.load_from_checkpoint(
                    start=start_time.timestamp(), 
                    end=end_time.timestamp(), 
                    checkpoint=checkpoint)
                
                while True:
                    try:
                        item = next(gen)
                        if isinstance(item, ConnectorFailure):
                            logging.exception(
                                "Bitbucket connector failure: %s",
                                item.failure_message)
                            break
                        yield [item]
                    except StopIteration as e:
                        checkpoint = e.value
                        break
        
        def wrapper():
            for batch in document_batches():
                yield batch

        self.log_connection("Bitbucket", f"workspace({self.conf.get('workspace')})", task)
        return wrapper()


class SeaFile(SyncBase):
    SOURCE_NAME: str = FileSource.SEAFILE

    async def _generate(self, task: dict):
        conf = self.conf
        raw_batch_size = conf.get("batch_size", INDEX_BATCH_SIZE)
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        self.connector = SeaFileConnector(
            seafile_url=conf["seafile_url"],
            batch_size=batch_size,
            include_shared=conf.get("include_shared", True),
            sync_scope=conf.get("sync_scope", SeafileSyncScope.ACCOUNT),
            repo_id=conf.get("repo_id") or None,
            sync_path=conf.get("sync_path") or None,
        )
        self.connector.load_credentials(conf["credentials"])

        poll_start = task.get("poll_range_start")
        if task["reindex"] == "1" or poll_start is None:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            end_ts = datetime.now(timezone.utc).timestamp()
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                end_ts,
            )
            _begin_info = f"from {poll_start}"

        scope = conf.get("sync_scope", "account")
        extra = ""
        if scope in ("library", "directory"):
            extra = f" repo_id={conf.get('repo_id')}"
        if scope == "directory":
            extra += f" path={conf.get('sync_path')}"

        self.log_connection("SeaFile", f"{conf['seafile_url']} (scope={scope}{extra})", task)
        return document_generator


class DingTalkAITable(SyncBase):
    SOURCE_NAME: str = FileSource.DINGTALK_AI_TABLE

    async def _generate(self, task: dict):
        """
        Sync records from DingTalk AI Table (Notable).
        """
        raw_batch_size = self.conf.get("batch_size", INDEX_BATCH_SIZE)
        try:
            batch_size = int(raw_batch_size)
        except (TypeError, ValueError):
            batch_size = INDEX_BATCH_SIZE
        if batch_size <= 0:
            batch_size = INDEX_BATCH_SIZE

        self.connector = DingTalkAITableConnector(
            table_id=self.conf.get("table_id"),
            operator_id=self.conf.get("operator_id"),
            batch_size=batch_size,
        )

        credentials = self.conf.get("credentials", {})
        if "access_token" not in credentials:
            raise ValueError("Missing access_token in credentials")

        self.connector.load_credentials(
            {"access_token": credentials["access_token"]}
        )

        poll_start = task.get("poll_range_start")

        if task.get("reindex") == "1" or poll_start is None:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            end_ts = datetime.now(timezone.utc).timestamp()
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                end_ts,
            )
            _begin_info = f"from {poll_start}"

        self.log_connection(
            "DingTalk AI Table",
            f"table_id({self.conf.get('table_id')}), operator_id({self.conf.get('operator_id')})",
            task,
        )

        return document_generator


class _RDBMSBase(SyncBase):
    DB_TYPE: str = ""
    LOG_NAME: str = ""
    DEFAULT_PORT: int = 0

    async def _generate(self, task: dict):
        self.connector = RDBMSConnector(
            db_type=self.DB_TYPE,
            host=self.conf.get("host", "localhost"),
            port=int(self.conf.get("port", self.DEFAULT_PORT)),
            database=self.conf.get("database", ""),
            query=self.conf.get("query", ""),
            content_columns=self.conf.get("content_columns", ""),
            metadata_columns=self.conf.get("metadata_columns", ""),
            id_column=self.conf.get("id_column") or None,
            timestamp_column=self.conf.get("timestamp_column") or None,
            batch_size=self.conf.get("batch_size", INDEX_BATCH_SIZE),
        )

        credentials = self.conf.get("credentials")
        if not credentials:
            raise ValueError(f"{self.DB_TYPE} connector is missing credentials.")

        self.connector.load_credentials(credentials)
        self.connector.validate_connector_settings()
        self.connector.prepare_sync_state(task["connector_id"], self.conf)

        if task["reindex"] == "1" or not task["poll_range_start"]:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        elif not self.connector.timestamp_column:
            document_generator = self.connector.load_from_state()
            _begin_info = f"from {task['poll_range_start']}"
        else:
            poll_start = task["poll_range_start"]
            start_cursor_value = self.connector.get_saved_sync_cursor_value()
            document_generator = self.connector.load_from_cursor_range(
                start_cursor_value,
                self.connector._pending_sync_cursor_value,
            )
            _begin_info = f"from {poll_start}"

        self.log_connection(self.LOG_NAME, f"{self.conf.get('host')}:{self.conf.get('database')}", task)
        return document_generator


class MySQL(_RDBMSBase):
    SOURCE_NAME: str = FileSource.MYSQL
    DB_TYPE: str = "mysql"
    LOG_NAME: str = "MySQL"
    DEFAULT_PORT: int = 3306


class PostgreSQL(_RDBMSBase):
    SOURCE_NAME: str = FileSource.POSTGRESQL
    DB_TYPE: str = "postgresql"
    LOG_NAME: str = "PostgreSQL"
    DEFAULT_PORT: int = 5432


class REST_API(SyncBase):
    SOURCE_NAME: str = FileSource.REST_API

    async def _generate(self, task: dict):
        try:
            cfg = RestAPIConnector.parse_storage_config(self.conf)
        except ConnectorValidationError as exc:
            raise ValueError(str(exc)) from exc

        self.connector = RestAPIConnector.from_parsed_config(cfg)
        self.connector.load_credentials(self.conf.get("credentials") or {})

        poll_start = task.get("poll_range_start")
        if task.get("reindex") == "1" or poll_start is None:
            document_generator = self.connector.load_from_state()
            begin_info = "totally"
        else:
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                datetime.now(timezone.utc).timestamp(),
            )
            begin_info = f"from {poll_start}"

        logging.info("Connect to REST API: %s %s %s", self.conf.get("method", "GET"), self.conf.get("url"), begin_info)
        return document_generator


func_factory = {
    FileSource.RSS: RSS,
    FileSource.S3: S3,
    FileSource.R2: R2,
    FileSource.OCI_STORAGE: OCI_STORAGE,
    FileSource.GOOGLE_CLOUD_STORAGE: GOOGLE_CLOUD_STORAGE,
    FileSource.NOTION: Notion,
    FileSource.DISCORD: Discord,
    FileSource.CONFLUENCE: Confluence,
    FileSource.GMAIL: Gmail,
    FileSource.GOOGLE_DRIVE: GoogleDrive,
    FileSource.JIRA: Jira,
    FileSource.SHAREPOINT: SharePoint,
    FileSource.ONEDRIVE: OneDrive,
    FileSource.OUTLOOK: Outlook,
    FileSource.SLACK: Slack,
    FileSource.TEAMS: Teams,
    FileSource.MOODLE: Moodle,
    FileSource.DROPBOX: Dropbox,
    FileSource.WEBDAV: WebDAV,
    FileSource.BOX: BOX,
    FileSource.AIRTABLE: Airtable,
    FileSource.ASANA: Asana,
    FileSource.IMAP: IMAP,
    FileSource.ZENDESK: Zendesk,
    FileSource.GITHUB: Github,
    FileSource.GITLAB: Gitlab,
    FileSource.BITBUCKET: Bitbucket,
    FileSource.SEAFILE: SeaFile,
    FileSource.MYSQL: MySQL,
    FileSource.POSTGRESQL: PostgreSQL,
    FileSource.DINGTALK_AI_TABLE: DingTalkAITable,
    FileSource.REST_API: REST_API,
}


async def dispatch_tasks():
    """Polls the database for pending synchronization tasks and dispatches them concurrently."""
    while True:
        try:
            SyncLogsService.list_due_sync_tasks()
            SyncLogsService.list_due_prune_tasks()
            break
        except Exception as e:
            logging.warning(f"DB is not ready yet: {e}")
            await asyncio.sleep(3)

    due_sync_tasks = SyncLogsService.list_due_sync_tasks()
    due_prune_tasks = SyncLogsService.list_due_prune_tasks()
    tasks = []
    for task in [*due_sync_tasks, *due_prune_tasks]:
        if task["poll_range_start"]:
            task["poll_range_start"] = task["poll_range_start"].astimezone(timezone.utc)
        if task["poll_range_end"]:
            task["poll_range_end"] = task["poll_range_end"].astimezone(timezone.utc)
        func = func_factory[task["source"]](task["config"])
        tasks.append(asyncio.create_task(func(task)))

    try:
        await asyncio.gather(*tasks, return_exceptions=False)
    except Exception as e:
        logging.error(f"Error in dispatch_tasks: {e}")
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise
    await asyncio.sleep(1)


stop_event = threading.Event()


def signal_handler(sig, frame):
    """Handles system interruption signals to ensure a graceful worker shutdown."""
    logging.info("Received interrupt signal, shutting down...")
    stop_event.set()
    time.sleep(1)
    sys.exit(0)


CONSUMER_NO = "0" if len(sys.argv) < 2 else sys.argv[1]
CONSUMER_NAME = "data_sync_" + CONSUMER_NO


async def main():
    """Entry point for the RAGFlow data synchronization worker process."""
    logging.info(r"""
  _____        _           _____
 |  __ \      | |         / ____|
 | |  | | __ _| |_ __ _  | (___  _   _ _ __   ___
 | |  | |/ _` | __/ _` |  \___ \| | | | '_ \ / __|
 | |__| | (_| | || (_| |  ____) | |_| | | | | (__
 |_____/ \__,_|\__\__,_| |_____/ \__, |_| |_|\___|
                                  __/ |
                                 |___/
    """)
    logging.info(f"RAGFlow data sync version: {get_ragflow_version()}")
    show_configs()
    settings.init_settings()
    if sys.platform != "win32":
        signal.signal(signal.SIGUSR1, start_tracemalloc_and_snapshot)
        signal.signal(signal.SIGUSR2, stop_tracemalloc)
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    logging.info(f"RAGFlow data sync is ready after {time.perf_counter() - start_ts}s initialization.")
    while not stop_event.is_set():
        await dispatch_tasks()
    logging.error("BUG!!! You should not reach here!!!")


if __name__ == "__main__":
    faulthandler.enable()
    init_root_logger(CONSUMER_NAME)
    asyncio.run(main())
