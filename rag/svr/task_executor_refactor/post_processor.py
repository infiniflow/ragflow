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

"""
Post Processor Module.

Provides [`PostProcessor`](rag/svr/task_executor_refactor/post_processor.py:42) for post-indexing
operations like table parser metadata aggregation and TOC insertion.
"""

import logging
from typing import Dict, List, Optional

from api.db.services.document_service import DocumentService
from api.db.services.doc_metadata_service import DocMetadataService
from common.metadata_utils import update_metadata_to
from rag.svr.task_executor_refactor.task_context import TaskContext
from rag.utils.table_es_metadata import (
    aggregate_table_manual_doc_metadata,
    merge_table_parser_config_from_kb,
    table_parser_strip_doc_metadata_keys,
)

class PostProcessor:
    """Service for post-indexing operations.

    This service handles:
    - Table parser metadata aggregation
    - Document metadata updates
    - TOC (Table of Contents) chunk insertion
    """

    def __init__(
        self,
        ctx: TaskContext,
    ):
        """Initialize PostProcessor.

        Args:
            ctx: TaskContext containing task configuration and execution resources.
        """
        self._task_context = ctx

    async def process_table_parser_metadata(
        self,
        task_doc_id: str,
        chunks: List[Dict],
    ) -> None:
        """Process table parser metadata aggregation.

        Args:
            task_doc_id: Document ID.
            chunks: List of chunk dictionaries.
        """
        ctx = self._task_context
        if ctx.parser_id.lower() != "table":
            return

        eff_pc = merge_table_parser_config_from_kb(ctx.raw_task)
        logging.debug(
            f"[TABLE_META_DEBUG] table post-index: table_column_mode={eff_pc.get('table_column_mode')!r}"
        )

        if eff_pc.get("table_column_mode") != "manual":
            return

        try:
            agg = aggregate_table_manual_doc_metadata(chunks, ctx.raw_task)
            logging.debug(f"[TABLE_META_DEBUG] aggregated metadata: {agg}")

            strip_keys = table_parser_strip_doc_metadata_keys(eff_pc)
            existing = DocMetadataService.get_document_metadata(task_doc_id)
            existing = existing if isinstance(existing, dict) else {}

            preserved = {k: v for k, v in existing.items() if k not in strip_keys}
            merged = update_metadata_to(dict(preserved), agg)

            logging.debug(
                f"[TABLE_META_DEBUG] calling update_document_metadata for doc_id={task_doc_id}, "
                f"meta_fields keys={list(merged.keys())}, "
                f"table_strip_key_count={len(strip_keys)}, agg_keys={list(agg.keys())}"
            )

            try:
                if self._task_context.write_interceptor:
                    self._task_context.write_interceptor.intercept("DocMetadataService.update_document_metadata")
                else:
                    DocMetadataService.update_document_metadata(task_doc_id, merged)
                logging.debug("[TABLE_META_DEBUG] update_document_metadata succeeded")
            except Exception as ue:
                logging.error(
                    "update_document_metadata failed (table parser, doc_id=%s): %s",
                    task_doc_id,
                    ue,
                    exc_info=True,
                )
        except Exception as e:
            logging.exception(
                "Table parser document metadata aggregation failed (doc_id=%s): %s",
                task_doc_id,
                e,
            )

    async def insert_toc_chunk(
        self,
        toc_chunk: Optional[Dict],
        chunk_service,
    ) -> bool:
        """Insert TOC chunk into document store.

        Args:
            toc_chunk: TOC chunk dictionary or None.
            chunk_service: ChunkService instance for chunk insertion.

        Returns:
            True if TOC chunk was inserted successfully, False otherwise.
        """
        ctx = self._task_context
        if toc_chunk is None:
            return False

        if self._task_context.has_canceled_func(ctx.id):
            self._task_context.progress_cb(-1, msg="Task has been canceled.")
            return False

        insert_result = await chunk_service.insert_chunks(ctx.id, ctx.tenant_id, ctx.kb_id, [toc_chunk])

        if not insert_result:
            self._task_context.recording_context.record("toc_inserted", False)
            return False

        self._task_context.recording_context.record("toc_inserted", True)

        if self._task_context.write_interceptor:
            self._task_context.write_interceptor.intercept("DocumentService.increment_chunk_num")
        else:
            DocumentService.increment_chunk_num(ctx.doc_id, ctx.kb_id, 0, 1, 0)

        return True

    def _progress(self, prog=None, msg=None):
        """Progress callback helper."""
        if prog is not None or msg is not None:
            self._task_context.progress_cb(prog=prog, msg=msg)
