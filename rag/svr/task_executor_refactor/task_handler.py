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
Task Handler Module.

Provides [`TaskHandler`](rag/svr/task_executor_refactor/task_handler.py:56) as the main entry point
for handling document processing tasks with refactored, testable methods.
"""

import asyncio
import logging
import json

# Wiki / artifact compilation pipeline lives in
# ``rag.svr.task_executor_refactor.dataset_wiki_generator`` — see the
# ``task_type == "artifact"`` branch of ``TaskHandler.run`` for the
# dispatch call.
# Document-structure compilation helpers (CHAIN_KINDS,
# compile_structure_from_text, merge_compiled_structures,
# validate_and_correct_chain) moved to ``chunk_post_processor``.
import xxhash

from timeit import default_timer as timer
from typing import AsyncIterator, Callable, Dict, List, Optional

from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.compilation_template_group_service import CompilationTemplateGroupService
from api.db.joint_services.memory_message_service import handle_save_to_memory_task
from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type, get_model_config_from_provider_instance
from api.db.services.llm_service import LLMBundle
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID, abort_doc_chunking_counter
from common.constants import LLMType
from common.exceptions import TaskCanceledException
from common.connection_utils import timeout
from common.misc_utils import thread_pool_exec
from rag.nlp import search
from rag.svr.task_executor_refactor.constants import CANVAS_DEBUG_DOC_ID
from rag.svr.task_executor_refactor.chunk_service import ChunkService
from rag.svr.task_executor_refactor.dataflow_service import BillingHook, DataflowService
from rag.svr.task_executor_refactor.embedding_service import EmbeddingService
from rag.svr.task_executor_refactor.post_processor import PostProcessor
from rag.svr.task_executor_refactor.raptor_service import RaptorService
from rag.svr.task_executor_refactor.raptor_utils import delete_raptor_chunks
from rag.svr.task_executor_refactor.recording_context import RecordingContext
from rag.svr.task_executor_refactor.task_context import TaskContext
from rag.graphrag.general.index import run_graphrag_for_kb
from api.db.services.file2document_service import File2DocumentService
from rag.prompts.generator import run_toc_from_text
from common import settings


def _parser_config_compilation_template_ids(parser_config, tenant_id: str) -> list[str]:
    """Resolve a doc's parser_config to compile-template ids by
    looking up configured groups. Returns ``[]`` if the doc has no
    group set or no group can be resolved.
    """
    from rag.svr.task_executor_refactor.chunk_post_processor import (
        _parser_config_compilation_template_group_ids,
    )

    template_ids: list[str] = []
    seen: set[str] = set()
    for group_id in _parser_config_compilation_template_group_ids(parser_config):
        for template_id in CompilationTemplateGroupService.resolve_template_ids(group_id, tenant_id):
            if template_id in seen:
                continue
            seen.add(template_id)
            template_ids.append(template_id)
    return template_ids


def _resolve_template_chat_llm_id(parser_cfg: dict, ctx) -> str:
    """Pick the chat model id for a knowledge-compilation template.

    Resolution order:
      1. The template's own ``llm_id`` (what the user picked in the
         compilation-template panel).
      2. The doc's ``parser_config.llm_id`` (the doc-level chunking
         model).
      3. ``ctx.llm_id`` (the chunking task's default).
    """
    if isinstance(parser_cfg, dict):
        tid = parser_cfg.get("llm_id")
        if isinstance(tid, str) and tid.strip():
            return tid.strip()
    doc_cfg = getattr(ctx, "parser_config", None) or {}
    if isinstance(doc_cfg, dict):
        did = doc_cfg.get("llm_id")
        if isinstance(did, str) and did.strip():
            return did.strip()
    return ctx.llm_id


# Document-structure compilation tunables
# (DOC_STRUCTURE_COMPILE_BATCH_CHUNKS, DOC_STRUCTURE_MERGE_MAX_DOCS,
# STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S) moved to
# ``chunk_post_processor``.

# Wiki / artifact tunables (``WIKI_MAP_BATCH_CHUNKS``,
# ``WIKI_GRAPH_MAX_CHUNK_IDS_PER_NODE``, commit-title / comments
# templates) moved to ``dataset_wiki_generator``.

# The corpus → skill compilation pipeline lives in
# ``rag.svr.task_executor_refactor.dataset_skill_generator``. Its entry
# point is :func:`run_corpus2skill`; this handler invokes it from the
# ``task_type == "skill"`` branch of ``run`` below.


class TaskHandler:
    """Main task handler for document processing.

    This class orchestrates the entire document processing pipeline:
    1. Task type detection (memory, dataflow, raptor, graphrag, standard)
    2. Model binding (embedding, chat)
    3. Chunk building or RAPTOR/GraphRAG execution
    4. Embedding
    5. Indexing
    6. Post-processing (TOC, table metadata)

    All intermediate results are recorded via RecordingContext for comparison.
    """

    def __init__(
        self,
        ctx: TaskContext,
        billing_hook: Optional[BillingHook] = None,
    ):
        """Initialize TaskHandler.

        Args:
            ctx: TaskContext containing task configuration and execution resources.
            billing_hook: Optional billing hook for pipeline success/error callbacks.
        """
        self._task_context = ctx
        self._billing_hook = billing_hook

    @staticmethod
    def _is_standard_chunking_task(task_type: str) -> bool:
        task_type = (task_type or "").lower()
        return task_type not in {
            "memory",
            "raptor",
            "graphrag",
            "mindmap",
            "artifact",
            "skill",
            "evaluation",
            "reembedding",
            "clone",
        } and not task_type.startswith("dataflow")

    async def handle_task(self) -> None:
        try:
            await self.handle()
        except Exception:
            if self._is_standard_chunking_task(self._task_context.task_type):
                abort_doc_chunking_counter(self._task_context.doc_id)
            raise
        finally:
            task_id = self._task_context.id
            task_tenant_id = self._task_context.tenant_id
            task_dataset_id = self._task_context.kb_id
            task_doc_id = self._task_context.doc_id
            if self._task_context.has_canceled_func(task_id):
                if self._is_standard_chunking_task(self._task_context.task_type):
                    abort_doc_chunking_counter(task_doc_id)
                    try:
                        exists = await thread_pool_exec(
                            settings.docStoreConn.index_exist,
                            search.index_name(task_tenant_id),
                            task_dataset_id,
                        )
                        if exists:
                            ret = await thread_pool_exec(
                                settings.docStoreConn.delete,
                                {"doc_id": task_doc_id},
                                search.index_name(task_tenant_id),
                                task_dataset_id,
                            )
                            self._task_context.recording_context.save_func_return_value("docStoreConn.delete", ret)
                    except Exception as e:
                        logging.exception(f"Remove doc({task_doc_id}) from docStore failed when task({task_id}) canceled, exception: {e}")

    @timeout(60 * 60 * 3, 1)
    async def handle(self) -> None:
        """Handle a document processing task."""
        ctx = self._task_context
        task_type = ctx.task_type
        task_id = ctx.id

        # Handle memory tasks
        if task_type == "memory":
            # ignore when it's dry run - no change on handle_save_to_memory_task when refactor
            if isinstance(ctx.write_interceptor, RecordingContext):
                logging.info(f"dry run, ignore handle_save_to_memory_task {task_id}")
            else:
                # actual run - not dry run
                await handle_save_to_memory_task(ctx.raw_task)
            return

        # Check if task is canceled
        if ctx.has_canceled_func(task_id):
            ctx.progress_cb(-1, msg="Task has been canceled.")
            return

        # Language defaults to "Chinese" via TaskContext._DEFAULTS 鈥?safe to bind model directly.
        # Bind embedding model (matching original do_handle_task order: bind + init_kb before routing)
        result = await self._bind_embedding_model()
        if result is None:
            return
        embedding_model, vector_size = result

        with embedding_model:
            self._init_kb(vector_size)

            # Handle dataflow tasks (after init_kb, matching original behavior)
            if task_type == "dataflow" and ctx.doc_id == CANVAS_DEBUG_DOC_ID:
                await self._run_dataflow()
                return

            if task_type.startswith("dataflow"):
                await self._run_dataflow()
                return

            # Route to appropriate handler
            if task_type == "raptor":
                await self._run_raptor(embedding_model, vector_size)
            elif task_type == "graphrag":
                await self._run_graphrag(embedding_model)
            elif task_type == "mindmap":
                ctx.progress_cb(1, "place holder")
            elif task_type == "artifact":
                from rag.svr.task_executor_refactor.dataset_wiki_generator import (
                    run_wiki,
                )

                await run_wiki(
                    self._task_context,
                    embedding_model,
                    self._load_chunks_for_doc,
                )
            elif task_type == "skill":
                from rag.svr.task_executor_refactor.dataset_skill_generator import (
                    run_corpus2skill,
                )

                await run_corpus2skill(
                    self._task_context,
                    embedding_model,
                    self._load_chunks_for_doc,
                )
            elif task_type == "evaluation":
                await self._run_evaluation()
            elif task_type == "reembedding":
                await self._run_reembedding()
            elif task_type == "clone":
                await self._run_clone()
            else:
                await self._run_standard_chunking(embedding_model, vector_size)

    def _init_kb(self, vector_size: int) -> None:
        """Initialize knowledge base index."""
        ctx = self._task_context
        idxnm = search.index_name(ctx.tenant_id)
        parser_id = ctx.parser_id
        # Create index if not exists
        settings.docStoreConn.create_idx(idxnm, ctx.kb_id, vector_size, parser_id)

    async def _run_dataflow(self) -> None:
        """Run dataflow pipeline."""
        dataflow_service = DataflowService(
            ctx=self._task_context,
            billing_hook=self._billing_hook,
        )
        await dataflow_service.run_dataflow()

    async def _run_evaluation(self) -> None:
        """Run evaluation task."""
        ctx = self._task_context
        ctx.progress_cb(1, "Evaluation task placeholder")

    async def _run_reembedding(self) -> None:
        """Run reembedding task."""
        ctx = self._task_context
        ctx.progress_cb(1, "Reembedding task placeholder")

    async def _run_clone(self) -> None:
        """Run clone task."""
        ctx = self._task_context
        ctx.progress_cb(1, "Clone task placeholder")

    async def _bind_embedding_model(self) -> Optional[tuple]:
        """Bind embedding model to task.

        Returns:
            Tuple of (embedding_model, vector_size) on success, or None on failure.
        """
        ctx = self._task_context
        task_tenant_id = ctx.tenant_id
        task_embedding_id = ctx.embd_id
        task_language = ctx.language

        try:
            if task_embedding_id:
                embd_model_config = get_model_config_from_provider_instance(task_tenant_id, LLMType.EMBEDDING, task_embedding_id)
            else:
                embd_model_config = get_tenant_default_model_by_type(task_tenant_id, LLMType.EMBEDDING)
            embedding_model = LLMBundle(task_tenant_id, embd_model_config, lang=task_language)
            vts, _ = embedding_model.encode(["ok"])
            return embedding_model, len(vts[0])
        except Exception as e:
            error_message = f"Fail to bind embedding model: {str(e)}"
            ctx.progress_cb(-1, msg=error_message)
            logging.exception(error_message)
            raise

    async def _run_raptor(
        self,
        embedding_model: LLMBundle,
        vector_size: int,
        mark_done: bool = True,
    ) -> None:
        """Run RAPTOR summary generation."""
        ctx = self._task_context
        task_tenant_id = ctx.tenant_id
        task_dataset_id = ctx.kb_id
        kb_task_llm_id = ctx.kb_parser_config.get("llm_id") or ctx.llm_id

        ok, kb = KnowledgebaseService.get_by_id(task_dataset_id)
        if not ok:
            ctx.progress_cb(prog=-1.0, msg="Cannot found valid dataset for RAPTOR task")
            return

        kb_parser_config = kb.parser_config
        if not kb_parser_config.get("raptor", {}).get("use_raptor", False):
            kb_parser_config.update(
                {
                    "raptor": {
                        "use_raptor": True,
                        "prompt": "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
                        "max_token": 256,
                        "threshold": 0.1,
                        "max_cluster": 64,
                        "random_seed": 0,
                        "scope": "file",
                        "clustering_method": "gmm",
                        "tree_builder": "raptor",
                    },
                }
            )
            if ctx.write_interceptor:
                update_result = ctx.write_interceptor.intercept("KnowledgebaseService.update_by_id")
            else:
                update_result = KnowledgebaseService.update_by_id(kb.id, {"parser_config": kb_parser_config})

            if not update_result:
                ctx.progress_cb(prog=-1.0, msg="Internal error: Invalid RAPTOR configuration")
                return

        # Bind LLM for raptor
        chat_model_config = get_model_config_from_provider_instance(task_tenant_id, LLMType.CHAT, kb_task_llm_id)
        with LLMBundle(task_tenant_id, chat_model_config, lang=ctx.language) as chat_model:
            # Run RAPTOR
            raptor_service = RaptorService(ctx=ctx)

            async with ctx.kg_limiter:
                chunks, token_count, raptor_cleanup_chunks = await raptor_service.run_raptor_for_kb(
                    kb_parser_config=kb_parser_config,
                    chat_mdl=chat_model,
                    embd_mdl=embedding_model,
                    vector_size=vector_size,
                    doc_ids=ctx.doc_ids or [ctx.doc_id],
                )

            ctx.recording_context.record("raptor_chunks", chunks)
            ctx.recording_context.record("raptor_token_count", token_count)

            # Insert RAPTOR chunks
            if chunks:
                task_doc_id = (ctx.doc_ids or [ctx.doc_id] or [GRAPH_RAPTOR_FAKE_DOC_ID])[0]
                chunk_service = ChunkService(ctx=ctx)
                insert_result = await chunk_service.insert_chunks(ctx.id, task_tenant_id, task_dataset_id, chunks)
                if insert_result:
                    ctx.recording_context.record("insertion_result", "success")
                else:
                    ctx.recording_context.record("insertion_result", "failed")

                # Cleanup stale RAPTOR chunks
                cleaned_chunks = 0
                for cleanup_doc_id, keep_method in raptor_cleanup_chunks:
                    ret = await self._delete_raptor_chunks(cleanup_doc_id, task_tenant_id, task_dataset_id, keep_method)
                    cleaned_chunks += ret

                if cleaned_chunks:
                    ctx.progress_cb(msg=f"Cleaned up {cleaned_chunks} stale RAPTOR chunks.")

                # Build the per-doc RAPTOR tree graph from the just-
                # inserted summaries. Each chunk in ``chunks`` carries
                # the doc_id it was written under (real doc id for
                # scope="file"; GRAPH_RAPTOR_FAKE_DOC_ID for the
                # dataset-scope path). We materialize one graph row per
                # distinct doc_id so the dataset structure-graph
                # endpoint can surface a RAPTOR tab per document.
                # Failure here is best-effort — the summaries are
                # already persisted; the tab just won't render.
                raptor_doc_ids = {str(c.get("doc_id")) for c in chunks if c.get("doc_id")}
                for raptor_doc_id in raptor_doc_ids:
                    try:
                        await raptor_service._persist_raptor_graph_to_es(raptor_doc_id)
                    except Exception:
                        logging.exception(
                            "raptor_graph: build failed for kb=%s doc=%s",
                            task_dataset_id,
                            raptor_doc_id,
                        )

                # Update document stats
                if ctx.write_interceptor:
                    ctx.write_interceptor.intercept("DocumentService.increment_chunk_num")
                else:
                    DocumentService.increment_chunk_num(task_doc_id, task_dataset_id, token_count, len(chunks), 0)

            if mark_done:
                ctx.recording_context.record("task_status", "completed")
                ctx.progress_cb(prog=1.0, msg="RAPTOR done")

    async def _run_graphrag(self, embedding_model: LLMBundle) -> None:
        """Run GraphRAG."""
        ctx = self._task_context
        task_tenant_id = ctx.tenant_id
        task_dataset_id = ctx.kb_id
        kb_task_llm_id = ctx.kb_parser_config.get("llm_id") or ctx.llm_id
        task_language = ctx.language

        ok, kb = KnowledgebaseService.get_by_id(task_dataset_id)
        if not ok:
            ctx.progress_cb(prog=-1.0, msg="Cannot found valid dataset for GraphRAG task")
            return

        kb_parser_config = kb.parser_config
        if not kb_parser_config.get("graphrag", {}).get("use_graphrag", False):
            kb_parser_config.update(
                {
                    "graphrag": {
                        "use_graphrag": True,
                        "entity_types": [
                            "organization",
                            "person",
                            "geo",
                            "event",
                            "category",
                        ],
                        "method": "light",
                        "batch_chunk_token_size": 4096,
                        "retry_attempts": 2,
                        "retry_backoff_seconds": 2.0,
                        "retry_backoff_max_seconds": 60.0,
                        "build_subgraph_timeout_per_chunk_seconds": 300,
                        "build_subgraph_min_timeout_seconds": 600,
                        "merge_timeout_seconds": 180,
                        "resolution_timeout_seconds": 1800,
                        "community_timeout_seconds": 1800,
                        "lock_acquire_timeout_seconds": 600,
                    }
                }
            )
            if ctx.write_interceptor:
                update_result = ctx.write_interceptor.intercept("KnowledgebaseService.update_by_id")
            else:
                update_result = KnowledgebaseService.update_by_id(kb.id, {"parser_config": kb_parser_config})
            if not update_result:
                ctx.progress_cb(prog=-1.0, msg="Internal error: Invalid GraphRAG configuration")
                return

        graphrag_conf = kb_parser_config.get("graphrag", {})
        start_ts = timer()
        chat_model_config = get_model_config_from_provider_instance(task_tenant_id, LLMType.CHAT, kb_task_llm_id)
        with LLMBundle(task_tenant_id, chat_model_config, lang=task_language) as chat_model:
            with_resolution = graphrag_conf.get("resolution", False)
            with_community = graphrag_conf.get("community", False)

            async with ctx.kg_limiter:
                result = await run_graphrag_for_kb(
                    row=ctx.raw_task,
                    doc_ids=ctx.doc_ids,
                    language=task_language,
                    kb_parser_config=kb_parser_config,
                    chat_model=chat_model,
                    embedding_model=embedding_model,
                    callback=ctx.progress_cb,
                    with_resolution=with_resolution,
                    with_community=with_community,
                )
                logging.info(f"GraphRAG task result for task {ctx.raw_task}:\n{result}")

            ctx.recording_context.record("graphrag_result", result)
            ctx.progress_cb(prog=1.0, msg="Knowledge Graph done ({:.2f}s)".format(timer() - start_ts))

    async def _run_standard_chunking(
        self,
        embedding_model: LLMBundle,
        vector_size: int,
    ) -> None:
        ctx = self._task_context
        try:
            await self._run_standard_chunking_impl(embedding_model, vector_size)
        except Exception:
            abort_doc_chunking_counter(ctx.doc_id)
            raise

    async def _run_standard_chunking_impl(
        self,
        embedding_model: LLMBundle,
        vector_size: int,
    ) -> None:
        """Run standard chunking pipeline."""
        ctx = self._task_context
        task_id = ctx.id
        task_tenant_id = ctx.tenant_id
        task_dataset_id = ctx.kb_id
        task_doc_id = ctx.doc_id
        task_start_ts = timer()
        doc_task_llm_id = ctx.parser_config.get("llm_id") or ctx.llm_id
        ctx.raw_task["llm_id"] = doc_task_llm_id

        # Build chunks
        start_ts = timer()
        chunk_service = ChunkService(ctx=ctx)

        # Get storage binary
        bucket, name = File2DocumentService.get_storage_address(doc_id=ctx.doc_id)
        binary = await self._get_storage_binary(bucket, name)
        if binary is None:
            raise FileNotFoundError(f"Can not find file <{ctx.name}> from minio. Could you try it again.")

        chunks = await chunk_service.build_chunks(binary)
        ctx.recording_context.record("chunks", chunks)
        chunk_ids = [c.get("id") for c in chunks if isinstance(c, dict) and "id" in c]
        ctx.recording_context.record("chunk_ids_count", len(chunk_ids))

        logging.info("Build document {}: {:.2f}s".format(ctx.name, timer() - start_ts))

        if not chunks:
            ctx.progress_cb(msg=f"No chunk built from {ctx.name}")
            if not await self._run_document_post_chunking_if_last(
                embedding_model,
                vector_size,
                task_start_ts,
                0,
                0,
            ):
                return
            task_time_cost = timer() - task_start_ts
            ctx.recording_context.record("task_status", "completed")
            ctx.progress_cb(prog=1.0, msg="Task done ({:.2f}s)".format(task_time_cost))
            return

        ctx.progress_cb(msg="Generate {} chunks".format(len(chunks)))

        # Embed chunks
        start_ts = timer()
        embedding_service = EmbeddingService(ctx=ctx)
        try:
            token_count, vector_size = await embedding_service.embed_chunks(chunks, embedding_model, ctx.parser_config)
        except TaskCanceledException:
            raise
        except Exception as e:
            error_message = "Generate embedding error:{}".format(str(e))
            ctx.progress_cb(-1, error_message)
            logging.exception(error_message)
            raise

        ctx.recording_context.record("token_count", token_count)
        ctx.recording_context.record("vector_size", vector_size)
        progress_message = "Embedding chunks ({:.2f}s)".format(timer() - start_ts)
        logging.info(progress_message)
        ctx.progress_cb(msg=progress_message)

        toc_thread = None
        if ctx.parser_id.lower() == "naive" and ctx.parser_config.get("toc_extraction", False):
            toc_thread = asyncio.create_task(asyncio.to_thread(self._build_toc, ctx, chunks, ctx.progress_cb))

        # Insert chunks
        chunk_count = len(set([chunk["id"] for chunk in chunks]))
        start_ts = timer()

        chunk_service = ChunkService(ctx=ctx)

        if ctx.has_canceled_func(task_id):
            abort_doc_chunking_counter(task_doc_id)
            ctx.progress_cb(-1, msg="Task has been canceled.")
            return

        insert_result = await chunk_service.insert_chunks(task_id, task_tenant_id, task_dataset_id, chunks)

        if not insert_result:
            ctx.recording_context.record("insertion_result", "failed")
            abort_doc_chunking_counter(task_doc_id)
            return
        ctx.recording_context.record("insertion_result", "success")

        # Post-processing
        post_processor = PostProcessor(ctx=ctx)
        await post_processor.process_table_parser_metadata(task_doc_id, chunks)

        ctx.progress_cb(msg="Indexing done ({:.2f}s).".format(timer() - start_ts))

        toc_chunk = await self._process_toc_thread(toc_thread)
        if toc_chunk:
            ctx.recording_context.record("toc_chunk", [toc_chunk])
            await post_processor.insert_toc_chunk(toc_chunk, chunk_service)

        if ctx.has_canceled_func(task_id):
            abort_doc_chunking_counter(task_doc_id)
            ctx.progress_cb(-1, msg="Task has been canceled.")
            return

        # Update document stats
        if ctx.write_interceptor:
            ctx.write_interceptor.intercept("DocumentService.increment_chunk_num")
        else:
            DocumentService.increment_chunk_num(task_doc_id, task_dataset_id, token_count, chunk_count, 0)

        if not await self._run_document_post_chunking_if_last(
            embedding_model,
            vector_size,
            task_start_ts,
            len(chunks),
            token_count,
        ):
            return

        task_time_cost = timer() - task_start_ts
        ctx.recording_context.record("task_status", "completed")
        ctx.progress_cb(prog=1.0, msg="Task done ({:.2f}s)".format(task_time_cost))

        logging.info("Chunk doc({}), page({}-{}), chunks({}), token({}), elapsed:{:.2f}".format(ctx.name, ctx.from_page, ctx.to_page, len(chunks), token_count, task_time_cost))

    async def _run_document_post_chunking_if_last(
        self,
        embedding_model: LLMBundle,
        vector_size: int,
        task_start_ts: float,
        chunks_len: int,
        token_count: int,
    ) -> bool:
        """Thin delegator. The pipeline lives in
        ``rag.svr.task_executor_refactor.chunk_post_processor``.
        """
        from rag.svr.task_executor_refactor.chunk_post_processor import (
            run_document_post_chunking_if_last,
        )

        return await run_document_post_chunking_if_last(
            self,
            embedding_model,
            vector_size,
            task_start_ts,
            chunks_len,
            token_count,
        )

    async def _process_toc_thread(self, toc_thread):
        try:
            if toc_thread:
                return await toc_thread
            else:
                return None
        finally:
            if toc_thread is not None and not toc_thread.done():
                toc_thread.cancel()

    @classmethod
    async def _get_storage_binary(cls, bucket: str, name: str) -> bytes:
        from common import settings

        """Get binary from storage."""
        return await thread_pool_exec(settings.STORAGE_IMPL.get, bucket, name)

    @staticmethod
    async def _load_chunks_for_doc(
        tenant_id: str,
        kb_id: str,
        doc_id: str,
        batch_size: int = 500,
    ) -> AsyncIterator[List[Dict]]:
        """Stream a document's chunks from the doc store one batch at a time.

        Async generator that yields successive batches of up to ``batch_size``
        chunks. Order is pushed to the doc store via
        ``OrderByExpr().asc("page_num_int").asc("top_int")`` so callers do
        not need to re-sort. Rows with a ``compile_kwd`` marker (artifact
        pages, structure entities, etc.) are filtered out defensively.

        Memory is bounded by ``batch_size``: at most one page is materialised
        at a time, so long documents do not balloon the worker's heap.
        """
        from common.doc_store.doc_store_base import OrderByExpr

        index_nm = search.index_name(tenant_id)
        if not settings.docStoreConn.index_exist(index_nm, kb_id):
            return

        select_fields = [
            "id",
            "doc_id",
            "content_with_weight",
            "page_num_int",
            "top_int",
        ]
        order_by = OrderByExpr()
        order_by.asc("page_num_int")
        order_by.asc("top_int")

        offset = 0
        while True:
            try:
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    select_fields,
                    [],
                    {"doc_id": [doc_id], "available_int": 1},
                    [],
                    order_by,
                    offset,
                    batch_size,
                    index_nm,
                    [kb_id],
                )
                field_map = settings.docStoreConn.get_fields(res, select_fields)
            except Exception:
                logging.exception("load_chunks_for_doc: failed to load chunks for doc=%s", doc_id)
                return
            if not field_map:
                return

            batch: List[Dict] = []
            for row_id, row in field_map.items():
                if row.get("compile_kwd"):
                    continue
                batch.append(
                    {
                        "id": row_id,
                        "doc_id": row.get("doc_id") or doc_id,
                        "content_with_weight": row.get("content_with_weight") or "",
                        "page_num_int": row.get("page_num_int", 0),
                        "top_int": row.get("top_int", 0),
                    }
                )
            if batch:
                yield batch
            if len(field_map) < batch_size:
                return
            offset += batch_size

    @classmethod
    def _build_toc(cls, ctx: TaskContext, docs: List[Dict], progress_cb: Callable) -> Optional[Dict]:
        """Build table of contents."""
        progress_cb(msg="Start to generate table of content ...")
        chat_model_config = get_model_config_from_provider_instance(ctx.tenant_id, LLMType.CHAT, ctx.llm_id)
        with LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language) as chat_mdl:
            docs = sorted(
                docs,
                key=lambda d: (
                    d.get("page_num_int", 0)[0] if isinstance(d.get("page_num_int", 0), list) else d.get("page_num_int", 0),
                    d.get("top_int", 0)[0] if isinstance(d.get("top_int", 0), list) else d.get("top_int", 0),
                ),
            )

            # NOTE: asyncio.run() creates a new event loop in the worker thread
            # (this method is called via asyncio.to_thread), which is the
            # intended pattern for bridging sync -> async in a thread context.
            toc: list[dict] = asyncio.run(run_toc_from_text([d["content_with_weight"] for d in docs], chat_mdl, progress_cb))
            logging.info("------------ T O C -------------\n" + json.dumps(toc, ensure_ascii=False, indent="  "))

            for ii, item in enumerate(toc):
                try:
                    chunk_val = item.pop("chunk_id", None)
                    if chunk_val is None or str(chunk_val).strip() == "":
                        logging.warning(f"Index {ii}: chunk_id is missing or empty. Skipping.")
                        continue
                    curr_idx = int(chunk_val or -1)
                    if curr_idx >= len(docs):
                        logging.error(f"Index {ii}: chunk_id {curr_idx} exceeds docs length {len(docs)}.")
                        continue
                    item["ids"] = [docs[curr_idx]["id"]]
                    if ii + 1 < len(toc):
                        next_chunk_val = toc[ii + 1].get("chunk_id", "")
                        if str(next_chunk_val).strip() != "":
                            next_idx = int(next_chunk_val)
                            for jj in range(curr_idx + 1, min(next_idx + 1, len(docs))):
                                item["ids"].append(docs[jj]["id"])
                        else:
                            logging.warning(f"Index {ii + 1}: next chunk_id is empty, range fill skipped.")
                except (ValueError, TypeError) as e:
                    logging.error(f"Index {ii}: Data conversion error - {e}")
                except Exception as e:
                    logging.exception(f"Index {ii}: Unexpected error - {e}")

            if toc:
                import copy

                d = copy.deepcopy(docs[-1])
                d["content_with_weight"] = json.dumps(toc, ensure_ascii=False)
                d["toc_kwd"] = "toc"
                d["available_int"] = 0
                d["page_num_int"] = [100000000]
                d["id"] = xxhash.xxh64((d["content_with_weight"] + str(d["doc_id"])).encode("utf-8", "surrogatepass")).hexdigest()
                return d
            return None

    async def _delete_raptor_chunks(self, doc_id: str, tenant_id: str, kb_id: str, keep_method: Optional[str]) -> int:
        """Delete RAPTOR chunks."""
        if self._task_context.write_interceptor:
            return self._task_context.write_interceptor.intercept("delete_raptor_chunks")
        else:
            return await delete_raptor_chunks(doc_id, tenant_id, kb_id, keep_method)
