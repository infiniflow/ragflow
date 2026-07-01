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
import re

import numpy as np

from common.token_utils import num_tokens_from_string
from api.apps.restful_apis.chunk_api import _compilation_template_kind
# Wiki / artifact compilation pipeline lives in
# ``rag.svr.task_executor_refactor.dataset_wiki_generator`` — see the
# ``task_type == "artifact"`` branch of ``TaskHandler.run`` for the
# dispatch call.
from rag.advanced_rag.knowlege_compile.structure import (
    CHAIN_KINDS,
    compile_structure_from_text,
    merge_compiled_structures,
    validate_and_correct_chain,
)
import xxhash

from timeit import default_timer as timer
from typing import AsyncIterator, Callable, Dict, List, Optional

from api.db.services.document_service import DocumentService, queue_per_doc_raptor_task
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.compilation_template_service import CompilationTemplateService
from api.db.services.compilation_template_group_service import CompilationTemplateGroupService
from api.db.joint_services.memory_message_service import handle_save_to_memory_task
from api.db.joint_services.tenant_model_service import (
    get_tenant_default_model_by_type,
    get_model_config_from_provider_instance
)
from api.db.services.llm_service import LLMBundle
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID, clear_doc_chunking_counter, credit_doc_chunking_task
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



def _parser_config_compilation_template_group_ids(parser_config) -> list[str]:
    """Read template-group ids from a doc's parser_config.

    Templates were previously referenced as a list
    (``compilation_template_ids``); after the template-group refactor
    a doc instead points at one or more groups, and the orchestrator
    resolves each group's child templates at runtime. Old
    ``compilation_template_ids`` data is intentionally ignored per
    the migration spec.
    """
    def _normalize(raw) -> list[str]:
        if isinstance(raw, str):
            raw = [raw]
        if not isinstance(raw, list):
            return []
        ids: list[str] = []
        seen: set[str] = set()
        for gid in raw:
            if not isinstance(gid, str):
                continue
            gid = gid.strip()
            if gid and gid not in seen:
                seen.add(gid)
                ids.append(gid)
        return ids

    if not isinstance(parser_config, dict):
        return []
    if "compilation_template_group_id" in parser_config:
        return _normalize(parser_config.get("compilation_template_group_id"))
    ext = parser_config.get("ext")
    if isinstance(ext, dict):
        return _normalize(ext.get("compilation_template_group_id"))
    return []


def _parser_config_compilation_template_ids(parser_config, tenant_id: str) -> list[str]:
    """Resolve a doc's parser_config to compile-template ids by
    looking up configured groups. Returns ``[]`` if the doc has no
    group set or no group can be resolved.
    """
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


# Document-structure compilation tuning.
# - COMPILE_BATCH_CHUNKS bounds how many source chunks are handed to a single
#   compile_structure_from_text() invocation (the call fans them out across
#   max_workers internally, so a moderate window keeps memory + LLM-context
#   pressure predictable for long docs).
# - MERGE_MAX_DOCS bounds how many compiled ES-ready docs may accumulate
#   before we flush them through merge_compiled_structures(). The merger
#   does pairwise cosine + LLM duplicate-judging, so it is the more expensive
#   step; we cap the per-flush set to keep the local-dedup buckets tractable.
_DOC_STRUCTURE_COMPILE_BATCH_CHUNKS = 4
_DOC_STRUCTURE_MERGE_MAX_DOCS = 512

# Hard wall on the chain-validator LLM correction step. ``list`` and
# ``timeline`` kinds run this just before each merge flush; anything
# longer than this is treated as a blocked LLM and the uncorrected docs
# are flushed instead.
_STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S = 120.0

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
                clear_doc_chunking_counter(self._task_context.doc_id)
            raise
        finally:
            task_id = self._task_context.id
            task_tenant_id = self._task_context.tenant_id
            task_dataset_id = self._task_context.kb_id
            task_doc_id = self._task_context.doc_id
            if self._task_context.has_canceled_func(task_id):
                if self._is_standard_chunking_task(self._task_context.task_type):
                    clear_doc_chunking_counter(task_doc_id)
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
                        logging.exception(
                            f"Remove doc({task_doc_id}) from docStore failed when task({task_id}) canceled, exception: {e}")

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
            if task_type == "graphrag":
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
                embd_model_config = get_model_config_from_provider_instance(
                    task_tenant_id, LLMType.EMBEDDING, task_embedding_id
                )
            else:
                embd_model_config = get_tenant_default_model_by_type(
                    task_tenant_id, LLMType.EMBEDDING
                )
            embedding_model = LLMBundle(task_tenant_id, embd_model_config, lang=task_language)
            vts, _ = embedding_model.encode(["ok"])
            return embedding_model, len(vts[0])
        except Exception as e:
            error_message = f'Fail to bind embedding model: {str(e)}'
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
            kb_parser_config.update({
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
            })
            if ctx.write_interceptor:
                update_result = ctx.write_interceptor.intercept("KnowledgebaseService.update_by_id")
            else:
                update_result = KnowledgebaseService.update_by_id(kb.id, {"parser_config": kb_parser_config})

            if not update_result:
                ctx.progress_cb(prog=-1.0, msg="Internal error: Invalid RAPTOR configuration")
                return

        # Bind LLM for raptor
        chat_model_config = get_model_config_from_provider_instance(
            task_tenant_id, LLMType.CHAT, kb_task_llm_id
        )
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
                    ret = await self._delete_raptor_chunks(
                        cleanup_doc_id, task_tenant_id, task_dataset_id, keep_method
                    )
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
                raptor_doc_ids = {
                    str(c.get("doc_id")) for c in chunks if c.get("doc_id")
                }
                for raptor_doc_id in raptor_doc_ids:
                    try:
                        await raptor_service._persist_raptor_graph_to_es(raptor_doc_id)
                    except Exception:
                        logging.exception(
                            "raptor_graph: build failed for kb=%s doc=%s",
                            task_dataset_id, raptor_doc_id,
                        )

                # Update document stats
                if ctx.write_interceptor:
                    ctx.write_interceptor.intercept("DocumentService.increment_chunk_num")
                else:
                    DocumentService.increment_chunk_num(task_doc_id, task_dataset_id, token_count, len(chunks), 0)

            if mark_done:
                ctx.recording_context.record("task_status", "completed")
                ctx.progress_cb(prog=1.0, msg="RAPTOR done")

    async def _run_graphrag(
        self,
        embedding_model: LLMBundle
    ) -> None:
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
            kb_parser_config.update({
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
            })
            if ctx.write_interceptor:
                update_result = ctx.write_interceptor.intercept("KnowledgebaseService.update_by_id")
            else:
                update_result = KnowledgebaseService.update_by_id(kb.id, {"parser_config": kb_parser_config})
            if not update_result:
                ctx.progress_cb(prog=-1.0, msg="Internal error: Invalid GraphRAG configuration")
                return

        graphrag_conf = kb_parser_config.get("graphrag", {})
        start_ts = timer()
        chat_model_config = get_model_config_from_provider_instance(
            task_tenant_id, LLMType.CHAT, kb_task_llm_id
        )
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
            clear_doc_chunking_counter(ctx.doc_id)
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
        ctx.raw_task['llm_id'] = doc_task_llm_id

        # Build chunks
        start_ts = timer()
        chunk_service = ChunkService(ctx=ctx)

        # Get storage binary
        bucket, name = File2DocumentService.get_storage_address(doc_id=ctx.doc_id)
        binary = await self._get_storage_binary(bucket, name)
        if binary is None:
            raise FileNotFoundError(
                f"Can not find file <{ctx.name}> from minio. Could you try it again."
            )

        chunks = await chunk_service.build_chunks(binary)
        ctx.recording_context.record("chunks", chunks)
        chunk_ids = [c.get("id") for c in chunks if isinstance(c, dict) and "id" in c]
        ctx.recording_context.record("chunk_ids_count", len(chunk_ids))

        logging.info("Build document {}: {:.2f}s".format(ctx.name, timer() - start_ts))

        if not chunks:
            ctx.progress_cb(msg=f"No chunk built from {ctx.name}")
            if not await self._run_document_post_chunking_if_last(
                embedding_model, vector_size, task_start_ts, 0, 0,
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
            token_count, vector_size = await embedding_service.embed_chunks(
                chunks, embedding_model, ctx.parser_config
            )
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

        # Insert chunks
        chunk_count = len(set([chunk["id"] for chunk in chunks]))
        start_ts = timer()

        chunk_service = ChunkService(ctx=ctx)

        if ctx.has_canceled_func(task_id):
            clear_doc_chunking_counter(task_doc_id)
            ctx.progress_cb(-1, msg="Task has been canceled.")
            return

        insert_result = await chunk_service.insert_chunks(
            task_id, task_tenant_id, task_dataset_id, chunks
        )

        if not insert_result:
            ctx.recording_context.record("insertion_result", "failed")
            clear_doc_chunking_counter(task_doc_id)
            return
        ctx.recording_context.record("insertion_result", "success")

        # Post-processing
        post_processor = PostProcessor(ctx=ctx)
        await post_processor.process_table_parser_metadata(task_doc_id, chunks)

        ctx.progress_cb(msg="Indexing done ({:.2f}s).".format(timer() - start_ts))

        if ctx.has_canceled_func(task_id):
            clear_doc_chunking_counter(task_doc_id)
            ctx.progress_cb(-1, msg="Task has been canceled.")
            return

        # Update document stats
        if ctx.write_interceptor:
            ctx.write_interceptor.intercept("DocumentService.increment_chunk_num")
        else:
            DocumentService.increment_chunk_num(task_doc_id, task_dataset_id, token_count, chunk_count, 0)

        if not await self._run_document_post_chunking_if_last(
            embedding_model, vector_size, task_start_ts, len(chunks), token_count,
        ):
            return

        task_time_cost = timer() - task_start_ts
        ctx.recording_context.record("task_status", "completed")
        ctx.progress_cb(prog=1.0, msg="Task done ({:.2f}s)".format(task_time_cost))

        logging.info(
            "Chunk doc({}), page({}-{}), chunks({}), token({}), elapsed:{:.2f}".format(
                ctx.name, ctx.from_page, ctx.to_page,
                len(chunks), token_count, task_time_cost
            )
        )

    async def _run_document_post_chunking_if_last(
        self,
        embedding_model: LLMBundle,
        vector_size: int,
        task_start_ts: float,
        chunks_len: int,
        token_count: int,
    ) -> bool:
        ctx = self._task_context
        task_id = ctx.id
        task_doc_id = ctx.doc_id

        if ctx.has_canceled_func(task_id):
            clear_doc_chunking_counter(task_doc_id)
            ctx.progress_cb(-1, msg="Task has been canceled.")
            return False

        remaining_chunking_tasks = 0 if ctx.write_interceptor else credit_doc_chunking_task(task_doc_id, task_id)
        if remaining_chunking_tasks != 0:
            if remaining_chunking_tasks is not None and remaining_chunking_tasks < 0:
                logging.warning(
                    "Chunking counter for doc %s is missing or expired after task %s; "
                    "skip post-processing to avoid duplicate finalizers.",
                    task_doc_id,
                    task_id,
                )
            else:
                logging.info(
                    "Chunk doc(%s), page(%s-%s), chunks(%s), token(%s), elapsed:%.2f; "
                    "waiting for %s chunking task(s) before post-processing",
                    ctx.name,
                    ctx.from_page,
                    ctx.to_page,
                    chunks_len,
                    token_count,
                    timer() - task_start_ts,
                    remaining_chunking_tasks,
                )
            return True

        # I am the unique last chunking task for this doc. The Redis counter is
        # decremented atomically, and the per-task done sentinel prevents retry
        # double-credit.
        # Document-structure compile and RAPTOR are independent post-chunking
        # passes — they read the same chunks but write disjoint ES rows
        # (compile_kwd in {list,set,hypergraph,...} vs raptor_kwd="raptor"
        # + compile_kwd="raptor_graph"). Run them concurrently so the slower
        # of the two bounds wall time instead of their sum.
        async def _maybe_run_raptor():
            raptor_cfg = (ctx.parser_config or {}).get("raptor") or {}
            if not raptor_cfg.get("use_raptor"):
                return
            try:
                ok_doc, doc_obj = DocumentService.get_by_id(task_doc_id)
                if ok_doc and doc_obj is not None:
                    ctx.progress_cb(msg="Starting RAPTOR task.")
                    await self._run_raptor(embedding_model, vector_size, mark_done=False)
                else:
                    logging.warning(
                        "raptor: cannot resolve doc %s to queue per-doc task", task_doc_id,
                    )
            except Exception:
                logging.exception(
                    "raptor: failed to queue per-doc task for doc %s", task_doc_id,
                )

        original_progress_cb = getattr(ctx, "_progress_cb", None)
        if original_progress_cb is not None:
            ctx._progress_cb = self._cap_done_progress(original_progress_cb)
        try:
            # Structure-compile failures still propagate (prior behavior);
            # RAPTOR failures are swallowed inside _maybe_run_raptor (also prior
            # behavior), so a bare gather() is enough — no return_exceptions
            # needed.
            await asyncio.gather(
                self._run_document_structure_compile(embedding_model),
                _maybe_run_raptor(),
            )
        finally:
            if original_progress_cb is not None:
                ctx._progress_cb = original_progress_cb
            clear_doc_chunking_counter(task_doc_id)

        if ctx.has_canceled_func(task_id):
            clear_doc_chunking_counter(task_doc_id)
            ctx.progress_cb(-1, msg="Task has been canceled.")
            return False
        return True

    @staticmethod
    def _cap_done_progress(progress_cb: Callable) -> Callable:
        def capped_progress(*args, **kwargs):
            args = list(args)
            if args:
                prog = args[0]
                if isinstance(prog, (int, float)) and not isinstance(prog, bool) and prog >= 1:
                    args[0] = 0.99
            if "prog" in kwargs:
                prog = kwargs["prog"]
                if isinstance(prog, (int, float)) and not isinstance(prog, bool) and prog >= 1:
                    kwargs["prog"] = 0.99
            return progress_cb(*args, **kwargs)

        return capped_progress

    async def _run_document_structure_compile(self, embedding_model: LLMBundle) -> None:
        """Run document-scoped knowledge compilation for non-artifact templates.

        Streams the doc's chunks from the doc-store in batches of
        ``_DOC_STRUCTURE_COMPILE_BATCH_CHUNKS`` (so memory stays bounded for
        long documents) and fans each batch out to every configured
        non-artifact template:

          1. Per batch, per template: feed the batch through
             ``compile_structure_from_text`` and extend that template's
             ``accumulated`` list with the returned ES-ready docs.
          2. Per template: whenever ``accumulated`` reaches
             ``_DOC_STRUCTURE_MERGE_MAX_DOCS``, flush it through
             ``merge_compiled_structures``.
          3. After the stream finishes, drain each template's remainder
             with a final flush.

        Streaming once and fanning out keeps the doc-store read cost
        constant in the number of templates.
        """
        ctx = self._task_context
        template_ids = _parser_config_compilation_template_ids(ctx.parser_config, ctx.tenant_id)
        if not template_ids:
            return

        # Resolve template configs up-front; drop artifact + invalid entries.
        active_templates: list[tuple[str, dict]] = []
        for template_id in template_ids:
            template = CompilationTemplateService.get_saved(template_id, ctx.tenant_id)
            if not template:
                logging.warning("document_structure_compile: template %s not found", template_id)
                continue
            parser_cfg = template.get("config") or {}
            if not isinstance(parser_cfg, dict):
                logging.warning("document_structure_compile: template %s config is invalid", template_id)
                continue
            kind = _compilation_template_kind(parser_cfg.get("kind"))
            if not kind or kind == "artifacts":
                continue
            active_templates.append((template_id, parser_cfg))

        if not active_templates:
            return

        # Build a per-template chat_mdl using the template's own llm_id,
        # with a cache so two templates picking the same model share a
        # single LLMBundle. Templates whose llm_id cannot be resolved
        # are dropped with a warning rather than silently failing.
        llm_bundle_cache: dict[str, LLMBundle] = {}
        chat_mdl_by_tid: dict[str, LLMBundle] = {}
        filtered_templates: list[tuple[str, dict]] = []
        for template_id, parser_cfg in active_templates:
            chat_llm_id = _resolve_template_chat_llm_id(parser_cfg, ctx)
            if chat_llm_id not in llm_bundle_cache:
                try:
                    cfg = get_model_config_from_provider_instance(
                        ctx.tenant_id, LLMType.CHAT, chat_llm_id,
                    )
                    llm_bundle_cache[chat_llm_id] = LLMBundle(
                        ctx.tenant_id, cfg, lang=ctx.language,
                    )
                except Exception:
                    logging.exception(
                        "document_structure_compile: cannot resolve chat model %s for template %s; skipping",
                        chat_llm_id, template_id,
                    )
                    continue
            chat_mdl_by_tid[template_id] = llm_bundle_cache[chat_llm_id]
            filtered_templates.append((template_id, parser_cfg))

        if not filtered_templates:
            return
        active_templates = filtered_templates

        # Pull ``tree``-kind templates off the streaming-compile path —
        # they don't go through compile_structure_from_text. Each tree
        # template runs RAPTOR per-doc and persists one graph row via
        # _struct_upsert_graph_json. We handle them up-front so the
        # rest of the orchestrator doesn't have to special-case them.
        tree_templates: list[tuple[str, dict]] = []
        non_tree_templates: list[tuple[str, dict]] = []
        for tid, cfg in active_templates:
            if _compilation_template_kind((cfg or {}).get("kind")) == "tree":
                tree_templates.append((tid, cfg))
            else:
                non_tree_templates.append((tid, cfg))

        if tree_templates:
            await self._run_tree_templates(
                tree_templates, chat_mdl_by_tid, embedding_model,
            )

        if not non_tree_templates:
            return
        active_templates = non_tree_templates

        progress_cb = ctx.progress_cb
        total = len(active_templates)

        # Per-template accumulators + aggregate counters. ``template_kinds``
        # is captured up-front so ``_flush`` knows whether to run the
        # chain-shape validator (only ``list`` / ``timeline`` kinds qualify).
        accumulators: dict[str, list[dict]] = {tid: [] for tid, _ in active_templates}
        template_kinds: dict[str, str] = {
            tid: _compilation_template_kind((cfg or {}).get("kind"))
            for tid, cfg in active_templates
        }
        agg_infos: dict[str, dict] = {
            tid: {"inserted": 0, "updated": 0, "duplicates_dropped": 0}
            for tid, _ in active_templates
        }
        # Map ``chunk_id → content_with_weight`` so the chain validator can
        # show the LLM the source text behind any flagged relations. We
        # populate this as each batch streams in.
        chunks_by_id: dict[str, str] = {}

        async def _flush(template_id: str) -> None:
            acc = accumulators[template_id]
            if not acc:
                return
            kind = template_kinds.get(template_id, "")
            if kind in CHAIN_KINDS:
                # Best-effort chain validation; on timeout / exception we
                # fall through with the uncorrected accumulator so the
                # merge phase still runs.
                try:
                    acc = await asyncio.wait_for(
                        validate_and_correct_chain(
                            acc,
                            chunks_by_id,
                            chat_mdl_by_tid[template_id],
                            kind,
                            callback=progress_cb,
                        ),
                        timeout=_STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S,
                    )
                    accumulators[template_id] = acc
                except asyncio.TimeoutError:
                    logging.warning(
                        "chain validate: timed out after %ss for template %s; using uncorrected docs",
                        _STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S, template_id,
                    )
                except Exception:
                    logging.exception(
                        "chain validate: unexpected failure for template %s; using uncorrected docs",
                        template_id,
                    )
            info = await merge_compiled_structures(
                acc,
                chat_mdl_by_tid[template_id],
                embedding_model,
                ctx.tenant_id,
                ctx.kb_id,
                compilation_template_id=template_id,
                cancel_check=lambda: ctx.has_canceled_func(ctx.id),
            )
            acc.clear()
            if isinstance(info, dict):
                agg = agg_infos[template_id]
                for k in ("inserted", "updated", "duplicates_dropped"):
                    agg[k] = agg.get(k, 0) + int(info.get(k, 0) or 0)

        progress_cb(msg=f"Start document knowledge compilation ({total} template(s)) ...")

        batch_no = 0
        async for batch in self._load_chunks_for_doc(
            ctx.tenant_id, ctx.kb_id, ctx.doc_id,
            batch_size=_DOC_STRUCTURE_COMPILE_BATCH_CHUNKS,
        ):
            batch_no += 1
            # Snapshot this batch's chunks for the chain validator before
            # fanning out to templates. Only the (id, content) pair is
            # captured — keeps memory linear in the doc, not in the
            # template count.
            for chunk in batch:
                cid = chunk.get("id")
                if isinstance(cid, str) and cid not in chunks_by_id:
                    text = chunk.get("content_with_weight") or ""
                    chunks_by_id[cid] = text if isinstance(text, str) else ""
            for idx, (template_id, parser_cfg) in enumerate(active_templates):
                progress_cb(
                    msg=f"  compile batch {batch_no} ({len(batch)} chunks) for template ({idx + 1}/{total})"
                )
                docs = await compile_structure_from_text(
                    batch,
                    parser_cfg,
                    chat_mdl_by_tid[template_id],
                    embedding_model,
                    ctx.doc_id,
                    language=ctx.language,
                    callback=progress_cb,
                    compilation_template_id=template_id,
                )
                if docs:
                    accumulators[template_id].extend(docs)
                if len(accumulators[template_id]) >= _DOC_STRUCTURE_MERGE_MAX_DOCS:
                    progress_cb(
                        msg=f"  merge flush ({len(accumulators[template_id])} docs) for template ({idx + 1}/{total})"
                    )
                    await _flush(template_id)

        for idx, (template_id, _parser_cfg) in enumerate(active_templates):
            if ctx.has_canceled_func(ctx.id):
                raise TaskCanceledException(
                    f"Task {ctx.id} was cancelled during document knowledge compilation"
                )
            await _flush(template_id)
            agg = agg_infos[template_id]
            ctx.recording_context.record(f"document_structure_compile:{template_id}", agg)
            progress_cb(msg=f"Document knowledge compilation done ({idx + 1}/{total}): {agg}")

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
    def _raptor_tree_to_graph(tree: Dict) -> Dict:
        """Project a RAPTOR tree dict (from ``Raptor(is_tree=True)``)
        onto the ``{entities, relations}`` shape the document-structure
        graph endpoint already serves for ``page_index``-kind rows.

        * Every node — including leaves — becomes an entity, keyed by a
          stable per-tree node id (``n0``, ``n1``, …).
        * Every parent → child connection becomes a relation with
          ``type='child'`` so the existing TreeGraph renderer treats it
          as a hierarchy edge.
        * Leaf ``source_chunk_ids`` get hoisted onto the leaf entity so
          the UI can deep-link from a tree leaf back to the source
          chunks.
        """
        entities: list[dict] = []
        relations: list[dict] = []
        counter = {"n": 0}

        def _next_id() -> str:
            cid = f"n{counter['n']}"
            counter["n"] += 1
            return cid

        def _walk(node: dict, parent_id: Optional[str]) -> None:
            if not isinstance(node, dict):
                return
            title = node.get("title") or ""
            node_id = title #_next_id()
            ent: dict = {
                "name": node_id,
                "type": "tree_node",
                "description": node.get("description", title),
                "mention_count": 1,
            }
            src_ids = node.get("source_chunk_ids")
            if isinstance(src_ids, list) and src_ids:
                ent["source_chunk_ids"] = [s for s in src_ids if isinstance(s, str) and s]
            entities.append(ent)
            if parent_id is not None:
                relations.append({"from": parent_id, "to": node_id, "type": "child"})
            for child in node.get("children") or []:
                _walk(child, node_id)

        _walk(tree, None)
        return {"entities": entities, "relations": relations}

    async def _run_tree_templates(
        self,
        templates: list[tuple[str, dict]],
        chat_mdl_by_tid: dict[str, "LLMBundle"],
        embedding_model,
    ) -> None:
        """Run the ``tree``-kind compilation templates for every doc in
        this task's KB. Each (template, doc) pair runs RAPTOR with
        ``is_tree=True`` (via ``RaptorService.build_doc_tree``) and
        persists a single graph row keyed by
        ``(compile_kwd="tree", compilation_template_ids=[tid], doc_id)``
        through the existing ``_struct_upsert_graph_json`` helper — so
        the document-structure-graph endpoint reads it like every other
        per-template row.
        """
        from rag.svr.task_executor_refactor.raptor_service import RaptorService
        from rag.advanced_rag.knowlege_compile.structure import _struct_upsert_graph_json

        ctx = self._task_context
        progress_cb = ctx.progress_cb

        # Build chunks (text, vec, chunk_id) for the current doc from
        # the streaming chunk loader. We need the embedding vectors
        # alongside text — _load_chunks_for_doc doesn't include vectors
        # by default, so do a focused read here that picks them up.
        try:
            doc_id = ctx.doc_id
        except Exception:
            doc_id = getattr(ctx, "_task", {}).get("doc_id") if hasattr(ctx, "_task") else None
        if not doc_id:
            logging.warning("tree-template: no doc_id on task context; skipping")
            return

        vctr_nm = "q_%d_vec" % len(embedding_model.encode(["x"])[0][0])
        chunks = await self._load_chunks_with_vec(
            ctx.tenant_id, ctx.kb_id, doc_id, vctr_nm,
        )
        if not chunks:
            progress_cb(msg=f"tree-template: doc {doc_id} has no chunks; skipping")
            return

        raptor_service = RaptorService(ctx)

        for idx, (template_id, parser_cfg) in enumerate(templates):
            raptor_cfg = (parser_cfg or {}).get("raptor") or {}
            # Mirror RAPTOR's parser_config default shape so build_doc_tree
            # sees the same keys it does on the per-doc path.
            raptor_config = {
                "prompt": raptor_cfg.get("prompt") or "Please write a concise summary of the following texts:\n{cluster_content}",
                "max_token": int(raptor_cfg.get("max_token") or 512),
                "threshold": float(raptor_cfg.get("threshold") or 0.1),
                "random_seed": int(raptor_cfg.get("random_seed") or 0),
                "max_cluster": int(raptor_cfg.get("max_cluster") or 64),
                "ext": raptor_cfg.get("ext") or {},
            }
            progress_cb(
                msg=f"tree-template ({idx + 1}/{len(templates)}): "
                    f"building tree for doc={doc_id}",
            )
            try:
                tree = await raptor_service.build_doc_tree(
                    chunks=chunks,
                    raptor_config=raptor_config,
                    chat_mdl=chat_mdl_by_tid[template_id],
                    embd_mdl=embedding_model,
                    tree_builder="raptor",        # classic builder; PSI returns None
                    clustering_method="gmm",
                    max_errors=3,
                )
            except Exception:
                logging.exception(
                    "tree-template %s: RAPTOR build failed for doc %s",
                    template_id, doc_id,
                )
                continue
            if tree is None:
                logging.info(
                    "tree-template %s: no tree produced for doc %s",
                    template_id, doc_id,
                )
                continue

            # Optional re-chunking pass: if the template enables
            # ``raptor.rechunk``, merge each leaf cluster's source
            # chunks into a single replacement chunk and rewrite the
            # tree's source_chunk_ids in-place so the projected graph
            # below reflects the new IDs. Originals are soft-deleted
            # via ``available_int=0``. Failures here are logged but
            # don't block the graph upsert — the tree still represents
            # the original chunks in that case.
            if bool((raptor_cfg or {}).get("rechunk")):
                try:
                    await self._rechunk_doc_by_tree(
                        tree=tree,
                        template_id=template_id,
                        embedding_model=embedding_model,
                    )
                except Exception:
                    logging.exception(
                        "tree-template %s: re-chunking failed for doc %s; "
                        "persisting tree with original chunk ids",
                        template_id, doc_id,
                    )

            graph = self._raptor_tree_to_graph(tree)
            try:
                await _struct_upsert_graph_json(
                    graph,
                    ctx.tenant_id,
                    ctx.kb_id,
                    doc_id,
                    compile_kwd="tree",
                    compilation_template_id=template_id,
                )
            except Exception:
                logging.exception(
                    "tree-template %s: graph upsert failed for doc %s",
                    template_id, doc_id,
                )
                continue

            # Auto-append/update this doc's row in the KB's nav
            # markdown so a downstream router can locate the doc by
            # its short root summary. Cross-template by design (Q2:
            # union) — if a KB has multiple tree templates, whichever
            # finishes last for the doc wins. Best-effort: failures
            # here log but don't block subsequent docs/templates.
            try:
                from rag.advanced_rag.knowlege_compile.dataset_nav import (
                    upsert_dataset_nav_doc,
                )
                await upsert_dataset_nav_doc(
                    ctx.tenant_id, ctx.kb_id, doc_id, tree,
                )
            except Exception:
                logging.exception(
                    "tree-template %s: dataset_nav upsert failed for doc %s",
                    template_id, doc_id,
                )

            progress_cb(
                msg=f"tree-template ({idx + 1}/{len(templates)}): "
                    f"persisted {len(graph['entities'])} node(s), "
                    f"{len(graph['relations'])} edge(s) for doc {doc_id}",
            )

    @staticmethod
    async def _load_chunks_with_vec(
        tenant_id: str, kb_id: str, doc_id: str, vctr_nm: str,
    ) -> list[tuple[str, "np.ndarray", str]]:
        """Page through this doc's chunks pulling content + vector +
        chunk_id, in the shape ``RaptorService.build_doc_tree`` expects.
        Mirrors ``_load_chunks_for_doc`` but with the vector field.
        """
        from common.doc_store.doc_store_base import OrderByExpr

        index_nm = search.index_name(tenant_id)
        if not settings.docStoreConn.index_exist(index_nm, kb_id):
            return []
        select_fields = ["id", "doc_id", "content_with_weight", vctr_nm]
        order_by = OrderByExpr()
        order_by.asc("page_num_int")
        order_by.asc("top_int")

        out: list[tuple[str, "np.ndarray", str]] = []
        offset = 0
        PAGE = 500
        while True:
            try:
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    select_fields, [], {"doc_id": [doc_id], "available_int": 1},
                    [], order_by, offset, PAGE,
                    index_nm, [kb_id],
                )
                field_map = settings.docStoreConn.get_fields(res, select_fields)
            except Exception:
                logging.exception(
                    "tree-template: failed to load chunks for doc=%s", doc_id,
                )
                break
            if not field_map:
                break
            for row_id, row in field_map.items():
                if row.get("compile_kwd"):
                    continue
                text = row.get("content_with_weight") or ""
                vec = row.get(vctr_nm)
                if not text or vec is None:
                    continue
                try:
                    arr = np.asarray(vec, dtype=np.float32)
                except Exception:
                    continue
                if arr.size == 0:
                    continue
                out.append((text, arr, str(row_id)))
            if len(field_map) < PAGE:
                break
            offset += PAGE
        return out

    async def _rechunk_doc_by_tree(
        self,
        tree: dict,
        template_id: str,
        embedding_model,
    ) -> None:
        """Merge each leaf cluster's source chunks into a single
        replacement chunk and rewrite the tree's leaf-cluster
        ``source_chunk_ids`` in-place.

        A *leaf cluster* is an internal tree node whose every child is
        a terminal node (i.e., the lowest-level summary node in the
        RAPTOR tree, one level above the original-chunk leaves). For
        each such cluster:

        1. The source chunks are fetched from ES (only ``available_int=1``
           rows, so re-runs over already-rechunked state are no-ops).
        2. Chunks are sorted by ``(min(page_num_int), min(top_int))``
           with the chunk id as a stable tiebreaker.
        3. ``content_with_weight`` is concatenated with a blank line
           separator; ``page_num_int`` and ``top_int`` are union'd so
           the merged chunk still resolves positionally to its source
           pages.
        4. A fresh embedding is computed on the merged content
           (re-embed strategy — averaging source vectors was rejected
           in the spec because retrieval recall on the merged chunk
           would degrade).
        5. The merged chunk is inserted, the leaf cluster's
           ``source_chunk_ids`` (and those of its terminal children)
           are rewritten to ``[merged_chunk_id]`` so the persisted
           tree graph stays in sync.
        6. The original chunks are soft-deleted via
           ``available_int=0`` and stamped with
           ``superseded_by_chunk_id`` for traceability.

        On any per-cluster failure the cluster is left unchanged and
        the rest of the pass continues — the upsert path then writes
        a mixed tree where rechunked clusters have a single new id and
        un-rechunked clusters keep their originals.
        """
        from datetime import datetime
        from common.misc_utils import get_uuid
        from rag.nlp import rag_tokenizer

        ctx = self._task_context

        # --- 1. Collect leaf clusters --------------------------------
        # ``cluster_id_map`` keys the cluster by ``id(node)``; we store
        # the dict reference so we can rewrite ``source_chunk_ids``
        # in-place once we have a merged_chunk_id.
        cluster_id_map: dict[int, tuple[dict, list[str]]] = {}

        def _is_terminal(node: object) -> bool:
            return isinstance(node, dict) and not (node.get("children") or [])

        def _walk(node: object) -> None:
            if not isinstance(node, dict):
                return
            children = node.get("children") or []
            if children and all(_is_terminal(c) for c in children):
                src_ids: list[str] = []
                seen: set[str] = set()
                for c in children:
                    for cid in (c.get("source_chunk_ids") or []):
                        if isinstance(cid, str) and cid and cid not in seen:
                            seen.add(cid)
                            src_ids.append(cid)
                for cid in (node.get("source_chunk_ids") or []):
                    if isinstance(cid, str) and cid and cid not in seen:
                        seen.add(cid)
                        src_ids.append(cid)
                if src_ids:
                    cluster_id_map[id(node)] = (node, src_ids)
            else:
                for c in children:
                    _walk(c)

        _walk(tree)
        if not cluster_id_map:
            return

        all_source_ids = sorted({sid for _, ids in cluster_id_map.values() for sid in ids})

        # --- 2. Fetch source chunks ----------------------------------
        from common.doc_store.doc_store_base import OrderByExpr

        index_nm = search.index_name(ctx.tenant_id)
        if not settings.docStoreConn.index_exist(index_nm, ctx.kb_id):
            return

        # Vector field dim matches the embedding model in use. Mirrors
        # the lookup ``_run_tree_templates`` already does upstream.
        vctr_nm = "q_%d_vec" % len(embedding_model.encode(["x"])[0][0])
        select_fields = [
            "id", "doc_id", "kb_id", "content_with_weight",
            "page_num_int", "top_int", "position_int",
            "docnm_kwd", "title_tks", "title_sm_tks",
            "available_int",
        ]
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields, [],
                {"id": all_source_ids, "available_int": 1},
                [], OrderByExpr(), 0, len(all_source_ids) + 16,
                index_nm, [ctx.kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields)
        except Exception:
            logging.exception(
                "rechunk: failed to load source chunks for doc=%s template=%s",
                ctx.doc_id, template_id,
            )
            return
        if not field_map:
            return

        chunks_by_id: dict[str, dict] = {
            str(rid): {**row, "id": str(rid)} for rid, row in field_map.items()
        }

        # --- 3. Build per-cluster merged chunks ----------------------
        merged_rows: list[dict] = []
        # Map ``id(cluster_node) -> new_chunk_id`` so step 5 can rewrite
        # the tree in one pass after embeddings land.
        cluster_new_id: dict[int, str] = {}

        for node_id_int, (node, src_ids) in cluster_id_map.items():
            cluster_chunks = [chunks_by_id[c] for c in src_ids if c in chunks_by_id]
            if not cluster_chunks:
                continue

            def _sort_key(c: dict) -> tuple:
                pages = c.get("page_num_int") or [0]
                tops = c.get("top_int") or [0]
                return (
                    min(pages) if pages else 0,
                    min(tops) if tops else 0,
                    c.get("id") or "",
                )
            cluster_chunks.sort(key=_sort_key)

            merged_content = "\n\n".join(
                (c.get("content_with_weight") or "") for c in cluster_chunks
            ).strip()
            if not merged_content:
                continue
            page_union = sorted({
                p for c in cluster_chunks for p in (c.get("page_num_int") or [])
            })
            top_union = sorted({
                t for c in cluster_chunks for t in (c.get("top_int") or [])
            })

            # Re-use a source chunk as the template for kb/doc/tenant
            # metadata so we don't have to enumerate every ES field.
            base = dict(cluster_chunks[0])
            new_id = get_uuid()
            cluster_new_id[node_id_int] = new_id

            base.update({
                "id": new_id,
                "content_with_weight": merged_content,
                "content_ltks": rag_tokenizer.tokenize(merged_content),
                "page_num_int": page_union,
                "top_int": top_union,
                "available_int": 1,
                "rechunk_kwd": "tree",
                "rechunked_from_template_id": template_id,
                "rechunked_from_chunk_ids": [c.get("id") for c in cluster_chunks if c.get("id")],
                "token_num": num_tokens_from_string(merged_content),
                "create_time": str(datetime.now()).replace("T", " ")[:19],
                "create_timestamp_flt": datetime.now().timestamp(),
            })
            base["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(base["content_ltks"])
            merged_rows.append(base)

        if not merged_rows:
            return

        # --- 4. Embed merged content (re-embed strategy) -------------
        contents = [r["content_with_weight"] for r in merged_rows]
        try:
            vectors, _ = embedding_model.encode(contents)
        except Exception:
            logging.exception(
                "rechunk: embedding failed for doc=%s template=%s",
                ctx.doc_id, template_id,
            )
            return
        for row, vec in zip(merged_rows, vectors):
            try:
                row[vctr_nm] = np.asarray(vec, dtype=np.float32).tolist()
            except Exception:
                logging.exception("rechunk: vector cast failed; skipping row %s", row.get("id"))
                row[vctr_nm] = None
        merged_rows = [r for r in merged_rows if r.get(vctr_nm) is not None]
        if not merged_rows:
            return

        # --- 5. Insert merged, rewrite tree, soft-delete originals ---
        try:
            await thread_pool_exec(
                settings.docStoreConn.insert, merged_rows, index_nm, ctx.kb_id,
            )
        except Exception:
            logging.exception(
                "rechunk: insert failed for doc=%s template=%s",
                ctx.doc_id, template_id,
            )
            return

        # Rewrite source_chunk_ids on each affected cluster node and
        # its terminal children. Done after insert so a failure above
        # leaves the tree pointing at the still-active originals.
        for node_id_int, new_chunk_id in cluster_new_id.items():
            node, _ = cluster_id_map[node_id_int]
            node["source_chunk_ids"] = [new_chunk_id]
            for child in (node.get("children") or []):
                if isinstance(child, dict):
                    child["source_chunk_ids"] = [new_chunk_id]

        # Soft-delete each source chunk; record the merged id for
        # traceability so audit queries can still tell what replaced it.
        for node_id_int, new_chunk_id in cluster_new_id.items():
            _, src_ids = cluster_id_map[node_id_int]
            for cid in src_ids:
                try:
                    await thread_pool_exec(
                        settings.docStoreConn.update,
                        {"id": cid},
                        {
                            "available_int": 0,
                            "superseded_by_chunk_id": new_chunk_id,
                        },
                        index_nm,
                        ctx.kb_id,
                    )
                except Exception:
                    logging.exception(
                        "rechunk: soft-delete failed for chunk=%s (merged=%s)",
                        cid, new_chunk_id,
                    )

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
            "id", "doc_id", "content_with_weight",
            "page_num_int", "top_int",
        ]
        order_by = OrderByExpr()
        order_by.asc("page_num_int")
        order_by.asc("top_int")

        offset = 0
        while True:
            try:
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    select_fields, [], {"doc_id": [doc_id], "available_int": 1},
                    [], order_by, offset, batch_size,
                    index_nm, [kb_id],
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
                batch.append({
                    "id": row_id,
                    "doc_id": row.get("doc_id") or doc_id,
                    "content_with_weight": row.get("content_with_weight") or "",
                    "page_num_int": row.get("page_num_int", 0),
                    "top_int": row.get("top_int", 0),
                })
            if batch:
                yield batch
            if len(field_map) < batch_size:
                return
            offset += batch_size

    @classmethod
    def _build_toc(cls, ctx: TaskContext, docs: List[Dict], progress_cb: Callable) -> Optional[Dict]:
        """Build table of contents."""
        progress_cb(msg="Start to generate table of content ...")
        chat_model_config = get_model_config_from_provider_instance(
            ctx.tenant_id, LLMType.CHAT, ctx.llm_id
        )
        with LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language) as chat_mdl:

            docs = sorted(docs, key=lambda d: (
                d.get("page_num_int", 0)[0] if isinstance(d.get("page_num_int", 0), list) else d.get("page_num_int", 0),
                d.get("top_int", 0)[0] if isinstance(d.get("top_int", 0), list) else d.get("top_int", 0)
            ))

            # NOTE: asyncio.run() creates a new event loop in the worker thread
            # (this method is called via asyncio.to_thread), which is the
            # intended pattern for bridging sync -> async in a thread context.
            toc: list[dict] = asyncio.run(
                run_toc_from_text([d["content_with_weight"] for d in docs], chat_mdl, progress_cb)
            )
            logging.info("------------ T O C -------------\n" + json.dumps(toc, ensure_ascii=False, indent='  '))

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
                d["id"] = xxhash.xxh64(
                    (d["content_with_weight"] + str(d["doc_id"])).encode("utf-8", "surrogatepass")).hexdigest()
                return d
            return None

    async def _delete_raptor_chunks(
        self, doc_id: str, tenant_id: str, kb_id: str, keep_method: Optional[str]
    ) -> int:
        """Delete RAPTOR chunks."""
        if self._task_context.write_interceptor:
            return self._task_context.write_interceptor.intercept("delete_raptor_chunks")
        else:
            return await delete_raptor_chunks(doc_id, tenant_id, kb_id, keep_method)
