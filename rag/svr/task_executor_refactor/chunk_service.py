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
Chunk Service Module.

Provides [`ChunkService`](rag/svr/task_executor_refactor/chunk_service.py:50) for document chunking,
post-processing (keywords, questions, metadata, tags), MinIO upload, and chunk insertion into document store.

This module orchestrates the chunk building pipeline by delegating to:
- [`chunk_builder`](rag/svr/task_executor_refactor/chunk_builder.py): Parser selection and document chunking
- [`chunk_post_processor`](rag/svr/task_executor_refactor/chunk_post_processor.py): Post-processing functions
"""

import asyncio
import copy
import logging
from datetime import datetime
from functools import partial
from timeit import default_timer as timer
from typing import Any, Dict, List

import xxhash
from common import settings
from common.connection_utils import timeout
from common.constants import PAGERANK_FLD, TAG_FLD
from common.misc_utils import thread_pool_exec
from common.float_utils import normalize_overlapped_percent
from rag.nlp import search
from rag.svr.task_executor_refactor.task_context import TaskContext
from rag.utils.base64_image import image2id

from api.db.services.task_service import TaskService
from rag.svr.task_executor_refactor.constants import GRAPH_RAPTOR_FAKE_DOC_ID

# Re-export for backward compatibility
from rag.svr.task_executor_refactor.chunk_builder import (
    get_parser,
    run_chunking,
    extract_outline,
)
from rag.svr.task_executor_refactor.chunk_post_processor import (
    extract_keywords,
    generate_questions,
    generate_metadata,
    apply_tags,
)


class ChunkService:
    """Service for document chunking and post-processing.

    This service handles:
    - Document chunking via parser modules (delegated to chunk_builder)
    - MinIO upload of chunk images
    - Keyword extraction (delegated to chunk_post_processor)
    - Question generation (delegated to chunk_post_processor)
    - Metadata generation (delegated to chunk_post_processor)
    - Content tagging (delegated to chunk_post_processor)
    - Table of contents generation
    - Chunk insertion into document store

    All intermediate results are recorded via RecordingContext for comparison.
    """

    def __init__(
        self,
        ctx: TaskContext,
    ):
        """Initialize ChunkService.

        Args:
            ctx: TaskContext containing task configuration and execution resources.
        """
        self._task_context = ctx

    @timeout(60 * 80, 1)
    async def build_chunks(
        self,
        storage_binary: bytes,
    ) -> List[Dict[str, Any]]:
        """Build chunks from document binary.

        This is the main entry point for chunk building. It orchestrates:
        1. File size validation
        2. Parser selection and chunking (delegated to chunk_builder)
        3. Outline extraction (delegated to chunk_builder)
        4. MinIO upload
        5. Post-processing (delegated to chunk_post_processor)

        Args:
            storage_binary: Binary content of the document.

        Returns:
            List of chunk dictionaries ready for embedding.
        """
        ctx = self._task_context
        # Validate file size
        if ctx.size > settings.DOC_MAXIMUM_SIZE:
            self._progress(prog=-1, msg="File size exceeds( <= %dMb )" % (int(settings.DOC_MAXIMUM_SIZE / 1024 / 1024)))
            self._task_context.recording_context.record("file_size_exceeded", True)
            return []
        ctx.recording_context.record("file_size_exceeded", False)
        ctx.recording_context.record("parser_id", ctx.parser_id)

        # Get parser
        chunker = get_parser(ctx.parser_id)

        # record config for compare
        chunk_config = {
            "parser_id": ctx.parser_id,
            "chunk_token_num": ctx.parser_config.get("chunk_token_num", 128),
            "overlapped_percent": normalize_overlapped_percent(ctx.parser_config.get("overlapped_percent", 0)),
            "delimiter": ctx.parser_config.get("delimiter", "\n!?。；！？"),
            "from_page": ctx.from_page,
            "to_page": ctx.to_page,
            "language": ctx.language,
            "layout_recognizer": ctx.parser_config.get("layout_recognizer"),
        }
        ctx.recording_context.record("chunk_config", chunk_config)

        # Run chunking (delegated)
        cks = await run_chunking(chunker, storage_binary, ctx)

        # Record raw chunks
        self._task_context.recording_context.record("raw_chunks", cks)

        # Extract outline (delegated)
        await extract_outline(cks, ctx)

        # Prepare docs and upload to MinIO
        docs = await self._prepare_docs_and_upload(cks)

        # Record docs after prep
        self._task_context.recording_context.record("docs_after_prep", docs)

        # Post-processing (delegated to chunk_post_processor)
        if ctx.parser_config.get("auto_keywords", 0):
            await extract_keywords(docs, ctx)
        keywords = [d for d in docs if d.get("important_kwd")]
        self._task_context.recording_context.record("keywords_extracted", keywords)

        if ctx.parser_config.get("auto_questions", 0):
            await generate_questions(docs, ctx)
        questions = [d for d in docs if d.get("question_kwd")]
        self._task_context.recording_context.record("questions_generated", questions)

        if ctx.parser_config.get("enable_metadata", False) and (ctx.parser_config.get("metadata") or ctx.parser_config.get("built_in_metadata")):
            await generate_metadata(docs, ctx)
        metadata_list = [d for d in docs if d.get("metadata_obj")]
        self._task_context.recording_context.record("metadata_list_generated", metadata_list)

        if ctx.kb_parser_config.get("tag_kb_ids", []):
            await apply_tags(docs, ctx)
        tags_applied = [d for d in docs if d.get(TAG_FLD)]
        self._task_context.recording_context.record("tags_applied", tags_applied)

        # Record final chunks
        self._task_context.recording_context.record("final_chunks", docs)
        final_chunk_ids = [c.get("id") for c in docs if isinstance(c, dict) and "id" in c]
        self._task_context.recording_context.record("final_chunk_ids_count", len(final_chunk_ids))

        return docs

    async def _prepare_docs_and_upload(self, cks: List[Dict]) -> List[Dict]:
        """Prepare docs and upload images to MinIO."""
        ctx = self._task_context
        docs = []
        doc = {"doc_id": ctx.doc_id, "kb_id": str(ctx.kb_id)}
        if ctx.pagerank:
            doc[PAGERANK_FLD] = int(ctx.pagerank)

        st = timer()

        @timeout(60)
        async def upload_to_minio(document, chunk):
            try:
                d = copy.deepcopy(document)
                d.update(chunk)
                d["id"] = xxhash.xxh64((chunk["content_with_weight"] + str(d["doc_id"])).encode("utf-8", "surrogatepass")).hexdigest()
                d["create_time"] = str(datetime.now()).replace("T", " ")[:19]
                d["create_timestamp_flt"] = datetime.now().timestamp()

                if d.get("img_id"):
                    docs.append(d)
                    return

                if not d.get("image"):
                    _ = d.pop("image", None)
                    d["img_id"] = ""
                    docs.append(d)
                    return

                await image2id(d, partial(settings.STORAGE_IMPL.put, tenant_id=ctx.tenant_id), d["id"], ctx.kb_id)
                docs.append(d)
            except Exception:
                logging.exception("Saving image of chunk {}/{}/{} got exception".format(ctx.location, ctx.name, d["id"]))
                raise

        tasks = []
        for ck in cks:
            tasks.append(asyncio.create_task(upload_to_minio(doc, ck)))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error(f"MINIO PUT({ctx.name}) got exception: {e}")
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise

        el = timer() - st
        logging.info("MINIO PUT({}) cost {:.3f} s".format(ctx.name, el))
        return docs

    def _progress(self, prog=None, msg=None):
        """Progress callback helper."""
        if prog is not None or msg is not None:
            self._task_context.progress_cb(prog=prog, msg=msg)

    # =========================================================================
    # Insert Service Methods (merged from insert_service.py)
    # =========================================================================

    async def insert_chunks(
        self,
        task_id: str,
        task_tenant_id: str,
        task_dataset_id: str,
        chunks: List[Dict[str, Any]],
        doc_bulk_size: int = None,
    ) -> bool:
        """Insert chunks into document store.

        Args:
            task_id: Task identifier.
            task_tenant_id: Tenant ID.
            task_dataset_id: Dataset/knowledge base ID.
            chunks: List of chunk dictionaries to insert.
            doc_bulk_size: Batch size for document store inserts.

        Returns:
            True if all chunks were inserted successfully, False otherwise.
        """
        doc_bulk_size = doc_bulk_size or settings.DOC_BULK_SIZE

        # Create mother chunks (summary chunks)
        mothers = self._create_mother_chunks(chunks)

        # Insert mother chunks
        if not await self._insert_mother_chunks(task_id, task_tenant_id, task_dataset_id, mothers, doc_bulk_size):
            return False

        # Insert main chunks
        return await self._insert_main_chunks(task_id, task_tenant_id, task_dataset_id, chunks, doc_bulk_size)

    @classmethod
    def _create_mother_chunks(cls, chunks: List[Dict]) -> List[Dict]:
        """Create mother chunks from summary fields.

        Mother chunks are summary/abstract chunks that are stored separately.
        """
        mothers = []
        mother_ids = set()

        for ck in chunks:
            mom = ck.get("mom") or ck.get("mom_with_weight") or ""
            if not mom:
                continue

            mom_id = xxhash.xxh64(mom.encode("utf-8")).hexdigest()
            ck["mom_id"] = mom_id

            if mom_id in mother_ids:
                continue

            mother_ids.add(mom_id)
            mom_ck = copy.deepcopy(ck)
            mom_ck["id"] = mom_id
            mom_ck["content_with_weight"] = mom
            mom_ck["available_int"] = 0

            # Keep only essential fields
            allowed_fields = ["id", "content_with_weight", "doc_id", "docnm_kwd", "kb_id", "available_int", "position_int", "create_timestamp_flt", "page_num_int", "top_int"]
            for fld in list(mom_ck.keys()):
                if fld not in allowed_fields:
                    del mom_ck[fld]

            mothers.append(mom_ck)

        return mothers

    async def _insert_mother_chunks(
        self,
        task_id: str,
        task_tenant_id: str,
        task_dataset_id: str,
        mothers: List[Dict],
        doc_bulk_size: int,
    ) -> bool:
        """Insert mother chunks in batches."""
        for b in range(0, len(mothers), doc_bulk_size):
            await self._intercept_doc_store_insert(mothers[b : b + doc_bulk_size], search.index_name(task_tenant_id), task_dataset_id)

            if self._task_context.has_canceled_func(task_id):
                self._task_context.progress_cb(-1, msg="Task has been canceled.")
                return False

        return True

    async def _intercept_doc_store_delete(self, condition: dict, index_name: str, task_dataset_id: str) -> Any:
        if self._task_context.write_interceptor:
            return self._task_context.write_interceptor.intercept("docStoreConn.delete")
        else:
            return await thread_pool_exec(settings.docStoreConn.delete, condition, index_name, task_dataset_id)

    async def _intercept_doc_store_insert(self, chunks: list, index_name: str, task_dataset_id: str) -> Any:
        if self._task_context.write_interceptor:
            if self._task_context.doc_id == GRAPH_RAPTOR_FAKE_DOC_ID:  # raptor - non-determinisic
                return self._task_context.write_interceptor.intercept("docStoreConn.insert", [])
            return self._task_context.write_interceptor.intercept("docStoreConn.insert")
        else:
            return await thread_pool_exec(settings.docStoreConn.insert, chunks, index_name, task_dataset_id)

    async def _insert_main_chunks(
        self,
        task_id: str,
        task_tenant_id: str,
        task_dataset_id: str,
        chunks: List[Dict],
        doc_bulk_size: int,
    ) -> bool:
        """Insert main chunks in batches with cancellation handling."""
        for b in range(0, len(chunks), doc_bulk_size):
            doc_store_result = await self._intercept_doc_store_insert(chunks[b : b + doc_bulk_size], search.index_name(task_tenant_id), task_dataset_id)

            if self._task_context.has_canceled_func(task_id):
                # Roll back partial RAPTOR summary inserts
                await self._rollback_raptor_chunks(task_id, task_tenant_id, task_dataset_id, chunks, b, doc_bulk_size)
                self._task_context.progress_cb(-1, msg="Task has been canceled.")
                return False

            if b % 128 == 0:
                self._task_context.progress_cb(prog=0.8 + 0.1 * (b + 1) / len(chunks), msg="")

            if doc_store_result:
                error_message = f"Insert chunk error: {doc_store_result}, please check log file and Elasticsearch/Infinity status!"
                self._task_context.progress_cb(-1, msg=error_message)
                raise Exception(error_message)

            # Update chunk IDs in task
            chunk_ids = [chunk["id"] for chunk in chunks[: b + doc_bulk_size]]
            if not await self._update_task_chunk_ids(task_id, chunk_ids):
                # Roll back on failure
                await self._rollback_insertion(task_tenant_id, task_dataset_id, chunk_ids)
                self._task_context.progress_cb(-1, msg=f"Chunk updates failed since task {task_id} is unknown.")
                return False

        return True

    async def _rollback_raptor_chunks(
        self,
        task_id: str,
        task_tenant_id: str,
        task_dataset_id: str,
        chunks: List[Dict],
        up_to_batch: int,
        doc_bulk_size: int,
    ):
        """Roll back partial RAPTOR summary inserts after cancellation."""
        raptor_ids = [c["id"] for c in chunks[: up_to_batch + doc_bulk_size] if c.get("raptor_kwd") == "raptor"]

        if raptor_ids:
            try:
                await self._intercept_doc_store_delete({"id": raptor_ids}, search.index_name(task_tenant_id), task_dataset_id)
                logging.info(
                    "insert_chunks: rolled back %d partial RAPTOR chunks after cancellation (task=%s)",
                    len(raptor_ids),
                    task_id,
                )
            except Exception:
                logging.exception(
                    "insert_chunks: failed to roll back partial RAPTOR chunks after cancellation (task=%s)",
                    task_id,
                )

    async def _update_task_chunk_ids(self, task_id: str, chunk_ids: List[str]) -> bool:
        """Update chunk IDs in the task record."""
        from peewee import DoesNotExist

        try:
            if self._task_context.write_interceptor:
                if self._task_context.doc_id == GRAPH_RAPTOR_FAKE_DOC_ID:
                    self._task_context.write_interceptor.intercept("TaskService.update_chunk_ids", True)
                else:
                    self._task_context.write_interceptor.intercept("TaskService.update_chunk_ids")
            else:
                TaskService.update_chunk_ids(task_id, " ".join(chunk_ids))
            return True
        except DoesNotExist:
            logging.warning(f"do_handle_task update_chunk_ids failed since task {task_id} is unknown.")
            return False

    async def _rollback_insertion(
        self,
        task_tenant_id: str,
        task_dataset_id: str,
        chunk_ids: List[str],
    ):
        """Roll back an insertion by deleting chunks and images."""
        await self._intercept_doc_store_delete({"id": chunk_ids}, search.index_name(task_tenant_id), task_dataset_id)

        # Delete associated images
        tasks = []
        for chunk_id in chunk_ids:
            tasks.append(asyncio.create_task(self._delete_image(task_dataset_id, chunk_id)))

        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error(f"delete_image failed: {e}")
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise

    async def _delete_image(self, kb_id: str, chunk_id: str):
        """Delete a chunk's image from storage."""
        try:
            async with self._task_context.minio_limiter:
                settings.STORAGE_IMPL.delete(kb_id, chunk_id)
        except Exception:
            logging.exception(f"Deleting image of chunk {chunk_id} got exception")
            raise
