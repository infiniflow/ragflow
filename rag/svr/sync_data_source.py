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
from common.config_utils import show_configs
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
)
from common.constants import FileSource, TaskStatus
from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.models import ConnectorFailure, SeafileSyncScope
from common.data_source.webdav_connector import WebDAVConnector
from common.data_source.confluence_connector import ConfluenceConnector
from common.data_source.gmail_connector import GmailConnector
from common.data_source.box_connector import BoxConnector
from common.data_source.github.connector import GithubConnector
from common.data_source.gitlab_connector import GitlabConnector
from common.data_source.bitbucket.connector import BitbucketConnector
from common.data_source.interfaces import CheckpointOutputWrapper
from common.log_utils import init_root_logger
from common.signal_utils import start_tracemalloc_and_snapshot, stop_tracemalloc
from common.versions import get_ragflow_version
from box_sdk_gen import BoxOAuth, OAuthConfig, AccessToken
from collections import namedtuple

MAX_CONCURRENT_TASKS = int(os.environ.get("MAX_CONCURRENT_TASKS", "5"))
task_limiter = asyncio.Semaphore(MAX_CONCURRENT_TASKS)


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

        SyncLogsService.schedule(task["connector_id"], task["kb_id"], task["poll_range_start"])

    async def _run_task_logic(self, task: dict):
        """
        Executes the core synchronization pipeline for a data source task.
        
        This method retrieves documents from the external source via the `_generate` method,
        parses and upserts them into the Knowledge Base (KB), and handles stale document
        reconciliation (sync deletion) if a remote snapshot (`file_list`) is provided.
        """
        generate_output = await self._generate(task)
        # `_generate()` currently supports two outputs:
        # 1. `document_batch_generator`
        # 2. `(document_batch_generator, file_list)`
        if isinstance(generate_output, tuple):
            document_batch_generator, file_list = generate_output
        else:
            document_batch_generator = generate_output
            file_list = None

        failed_docs = 0
        added_docs = 0
        updated_docs = 0
        removed_docs = 0
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
                    "id": hash128(doc.id),
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
        if file_list == []:
            logging.warning(
                "%s deleted-file sync skipped because the snapshot was empty "
                "(connector_id=%s, kb_id=%s)",
                self.SOURCE_NAME,
                task["connector_id"],
                task["kb_id"],
            )
        elif file_list is not None:
            logging.info(
                "[%s] Starting stale document reconciliation. Snapshot size: %d "
                "(connector_id=%s, kb_id=%s)",
                self.SOURCE_NAME,
                len(file_list),
                task["connector_id"],
                task["kb_id"],
            )
            removed_docs, _ = ConnectorService.cleanup_stale_documents_for_task(
                task["id"],
                task["connector_id"],
                task["kb_id"],
                task["tenant_id"],
                file_list,
            )

        total_changed_docs = added_docs + updated_docs + removed_docs
        summary = (
            f"{prefix}sync summary till {next_update_info}: "
            f"total={total_changed_docs}, added={added_docs}, "
            f"updated={updated_docs}, deleted={removed_docs}"
        )
        if failed_docs > 0:
            summary = f"{summary}, skipped={failed_docs}"
        logging.info(summary)

        SyncLogsService.done(task["id"], task["connector_id"])
        task["poll_range_start"] = next_update

    async def _generate(self, task: dict):
        raise NotImplementedError

    def _get_source_prefix(self):
        return ""


class _BlobLikeBase(SyncBase):
    DEFAULT_BUCKET_TYPE: str = "s3"

    async def _generate(self, task: dict):
        bucket_type = self.conf.get("bucket_type", self.DEFAULT_BUCKET_TYPE)

        self.connector = BlobStorageConnector(
            bucket_type=bucket_type,
            bucket_name=self.conf["bucket_name"],
            prefix=self.conf.get("prefix", ""),
        )
        self.connector.set_allow_images(self.conf.get("allow_images", False))
        self.connector.load_credentials(self.conf["credentials"])

        file_list = None
        document_batch_generator = (
            self.connector.load_from_state()
            if task["reindex"] == "1" or not task["poll_range_start"]
            else self.connector.poll_source(
                task["poll_range_start"].timestamp(),
                datetime.now(timezone.utc).timestamp(),
            )
        )

        if (
            task["reindex"] != "1"
            and task["poll_range_start"]
            and self.conf.get("sync_deleted_files")
        ):
            file_list = []
            for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync():
                file_list.extend(slim_batch)

        _begin_info = (
            "totally"
            if task["reindex"] == "1" or not task["poll_range_start"]
            else "from {}".format(task["poll_range_start"])
        )

        logging.info(
            "Connect to {}: {}(prefix/{}) {}".format(
                bucket_type,
                self.conf["bucket_name"],
                self.conf.get("prefix", ""),
                _begin_info,
            )
        )
        if file_list is not None:
            return document_batch_generator, file_list
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

        return self.connector.poll_source(
            task["poll_range_start"].timestamp(),
            datetime.now(timezone.utc).timestamp(),
        )


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

        file_list = None
        # Determine the time range for synchronization based on reindex or poll_range_start
        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = 0.0
        else:
            start_time = task["poll_range_start"].timestamp()
            if self.conf.get("sync_deleted_files"):
                file_list = []
                for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync():
                    file_list.extend(slim_batch)
            
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
        return wrapper(), file_list


class Notion(SyncBase):
    SOURCE_NAME: str = FileSource.NOTION

    async def _generate(self, task: dict):
        self.connector = NotionConnector(root_page_id=self.conf["root_page_id"])
        self.connector.load_credentials(self.conf["credentials"])
        file_list = None
        document_generator = (
            self.connector.load_from_state()
            if task["reindex"] == "1" or not task["poll_range_start"]
            else self.connector.poll_source(task["poll_range_start"].timestamp(),
                                            datetime.now(timezone.utc).timestamp())
        )

        if (
            task["reindex"] != "1"
            and task["poll_range_start"]
            and self.conf.get("sync_deleted_files")
        ):
            file_list = []
            for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync():
                file_list.extend(slim_batch)

        _begin_info = "totally" if task["reindex"] == "1" or not task["poll_range_start"] else "from {}".format(
            task["poll_range_start"])
        self.log_connection("Notion", f"root({self.conf['root_page_id']})", task)
        if file_list is not None:
            return document_generator, file_list
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

        if task["reindex"] == "1" or not task["poll_range_start"]:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            poll_start = task["poll_range_start"]
            document_generator = self.connector.poll_source(
                poll_start.timestamp(), datetime.now(timezone.utc).timestamp()
            )
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
        """
        Generates document batches from Google Drive based on the given task configuration.
        Returns a tuple of (document_batches_generator, current_file_list) if deleted file
        syncing is enabled, otherwise (document_batches_generator, None).
        """
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

        file_list = None

        # Capture end_time BEFORE the snapshot to prevent the ingestion race condition
        end_time = datetime.now(timezone.utc).timestamp()

        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = 0.0
            _begin_info = "totally"
        else:
            start_time = task["poll_range_start"].timestamp()
            _begin_info = f"from {task['poll_range_start']}"
            
            if self.conf.get("sync_deleted_files"):
                file_list = []
                logging.info("Syncing deleted files (connector_id=%s)", task["connector_id"])
                SlimDoc = namedtuple('SlimDoc', ['id'])
                
                # Add observability timing so operators can track the O(N) cost
                import time
                snapshot_start = time.time()
                
                for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync():
                    file_list.extend(SlimDoc(doc.id) for doc in slim_batch)
                    
                logging.info("Slim snapshot fetched %d files in %.2f seconds", len(file_list), time.time() - snapshot_start)
                
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
        
        return document_batches(), file_list

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
        file_list = None

        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = 0.0
            _begin_info = "totally"
        else:
            start_time = task["poll_range_start"].timestamp()
            if self.conf.get("sync_deleted_files"):
                file_list = []
                for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync():
                    file_list.extend(slim_batch)
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
        if file_list is not None:
            return document_batches(), file_list
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
        pass


class Slack(SyncBase):
    SOURCE_NAME: str = FileSource.SLACK

    async def _generate(self, task: dict):
        pass


class Teams(SyncBase):
    SOURCE_NAME: str = FileSource.TEAMS

    async def _generate(self, task: dict):
        pass


class WebDAV(SyncBase):
    SOURCE_NAME: str = FileSource.WEBDAV

    async def _generate(self, task: dict):
        self.connector = WebDAVConnector(
            base_url=self.conf["base_url"],
            remote_path=self.conf.get("remote_path", "/")
        )
        self.connector.set_allow_images(self.conf.get("allow_images", False))
        self.connector.load_credentials(self.conf["credentials"])

        if task["reindex"] == "1" or not task["poll_range_start"]:
            document_batch_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            start_ts = task["poll_range_start"].timestamp()
            end_ts = datetime.now(timezone.utc).timestamp()
            document_batch_generator = self.connector.poll_source(start_ts, end_ts)
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
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                datetime.now(timezone.utc).timestamp(),
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
        file_list = None

        if task["reindex"] == "1" or poll_start is None:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            if self.conf.get("sync_deleted_files"):
                file_list = []
                for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync():
                    file_list.extend(slim_batch)
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                datetime.now(timezone.utc).timestamp(),
            )
            _begin_info = f"from {poll_start}"
        self.log_connection("Box", f"folder_id({self.conf['folder_id']})", task)
        if file_list is not None:
            return document_generator, file_list
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
        file_list = None

        if task.get("reindex") == "1" or poll_start is None:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            if self.conf.get("sync_deleted_files"):
                file_list = []
                for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync():
                    file_list.extend(slim_batch)
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

        if file_list is not None:
            return document_generator, file_list
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

        if task.get("reindex") == "1" or not task.get("poll_range_start"):
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            poll_start = task.get("poll_range_start")
            if poll_start is None:
                document_generator = self.connector.load_from_state()
                _begin_info = "totally"
            else:
                document_generator = self.connector.poll_source(
                    poll_start.timestamp(),
                    datetime.now(timezone.utc).timestamp(),
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
            include_prs=self.conf.get("include_pull_requests", False),
            include_issues=self.conf.get("include_issues", False),
        )

        credentials = self.conf.get("credentials", {})
        if "github_access_token" not in credentials:
            raise ValueError("Missing github_access_token in credentials")

        self.connector.load_credentials(
            {"github_access_token": credentials["github_access_token"]}
        )

        file_list = None
        if task.get("reindex") == "1" or not task.get("poll_range_start"):
            start_time = datetime.fromtimestamp(0, tz=timezone.utc)
        else:
            start_time = task.get("poll_range_start")
            if self.conf.get("sync_deleted_files"):
                file_list = []
                for slim_batch in self.connector.retrieve_all_slim_docs_perm_sync():
                    file_list.extend(slim_batch)

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

        return wrapper(), file_list

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
        if task["reindex"] == "1" or not task["poll_range_start"]:
            start_time = end_time - self.conf.get("poll_range",30) * 24 * 60 * 60
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
        self.connector = SeaFileConnector(
            seafile_url=conf["seafile_url"],
            batch_size=conf.get("batch_size", INDEX_BATCH_SIZE),
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
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                datetime.now(timezone.utc).timestamp(),
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
        self.connector = DingTalkAITableConnector(
            table_id=self.conf.get("table_id"),
            operator_id=self.conf.get("operator_id"),
            batch_size=self.conf.get("batch_size", INDEX_BATCH_SIZE),
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
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                datetime.now(timezone.utc).timestamp(),
            )
            _begin_info = f"from {poll_start}"

        self.log_connection(
            "DingTalk AI Table",
            f"table_id({self.conf.get('table_id')}), operator_id({self.conf.get('operator_id')})",
            task,
        )

        return document_generator


class MySQL(SyncBase):
    SOURCE_NAME: str = FileSource.MYSQL

    async def _generate(self, task: dict):
        self.connector = RDBMSConnector(
            db_type="mysql",
            host=self.conf.get("host", "localhost"),
            port=int(self.conf.get("port", 3306)),
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
            raise ValueError("MySQL connector is missing credentials.")

        self.connector.load_credentials(credentials)
        self.connector.validate_connector_settings()

        if task["reindex"] == "1" or not task["poll_range_start"]:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            poll_start = task["poll_range_start"]
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                datetime.now(timezone.utc).timestamp()
            )
            _begin_info = f"from {poll_start}"

        self.log_connection("MySQL", f"{self.conf.get('host')}:{self.conf.get('database')}", task)
        return document_generator


class PostgreSQL(SyncBase):
    SOURCE_NAME: str = FileSource.POSTGRESQL

    async def _generate(self, task: dict):
        self.connector = RDBMSConnector(
            db_type="postgresql",
            host=self.conf.get("host", "localhost"),
            port=int(self.conf.get("port", 5432)),
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
            raise ValueError("PostgreSQL connector is missing credentials.")

        self.connector.load_credentials(credentials)
        self.connector.validate_connector_settings()

        if task["reindex"] == "1" or not task["poll_range_start"]:
            document_generator = self.connector.load_from_state()
            _begin_info = "totally"
        else:
            poll_start = task["poll_range_start"]
            document_generator = self.connector.poll_source(
                poll_start.timestamp(),
                datetime.now(timezone.utc).timestamp()
            )
            _begin_info = f"from {poll_start}"

        self.log_connection("PostgreSQL", f"{self.conf.get('host')}:{self.conf.get('database')}", task)
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
}


async def dispatch_tasks():
    while True:
        try:
            list(SyncLogsService.list_sync_tasks()[0])
            break
        except Exception as e:
            logging.warning(f"DB is not ready yet: {e}")
            await asyncio.sleep(3)

    tasks = []
    for task in SyncLogsService.list_sync_tasks()[0]:
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
    logging.info("Received interrupt signal, shutting down...")
    stop_event.set()
    time.sleep(1)
    sys.exit(0)


CONSUMER_NO = "0" if len(sys.argv) < 2 else sys.argv[1]
CONSUMER_NAME = "data_sync_" + CONSUMER_NO


async def main():
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
