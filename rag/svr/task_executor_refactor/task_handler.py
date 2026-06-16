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
from rag.advanced_rag.knowlege_compile.artifact import artifact_map_from_chunks, artifact_plan_from_reduction, artifact_reduce_from_extracts, artifact_refine_from_plan
from rag.advanced_rag.knowlege_compile.structure import compile_structure_from_text, merge_compiled_structures
import xxhash

from timeit import default_timer as timer
from typing import Callable, Dict, List, Optional

from api.db.services.document_service import DocumentService, queue_per_doc_raptor_task
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.compilation_template_service import CompilationTemplateService
from api.db.joint_services.memory_message_service import handle_save_to_memory_task
from api.db.joint_services.tenant_model_service import (
    get_tenant_default_model_by_type,
    get_model_config_from_provider_instance
)
from api.db.services.llm_service import LLMBundle
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID
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



def _parser_config_compilation_template_ids(parser_config) -> list[str]:
    if not isinstance(parser_config, dict):
        return []
    ids = parser_config.get("compilation_template_ids")
    if isinstance(ids, list):
        return [str(x).strip() for x in ids if isinstance(x, str) and x.strip()]
    legacy = parser_config.get("compilation_template_id")
    if isinstance(legacy, str) and legacy.strip():
        return [legacy.strip()]
    ext = parser_config.get("ext")
    if isinstance(ext, dict):
        ids = ext.get("compilation_template_ids")
        if isinstance(ids, list):
            return [str(x).strip() for x in ids if isinstance(x, str) and x.strip()]
        legacy = ext.get("compilation_template_id")
        if isinstance(legacy, str) and legacy.strip():
            return [legacy.strip()]
    return []


def _compilation_template_kind(kind) -> str:
    if not isinstance(kind, str):
        return ""
    normalized = kind.strip().lower().replace("-", "_")
    if normalized in {"pageindex", "page_index", "knowledge_graph"}:
        return "timeline"
    return normalized


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

    async def handle_task(self) -> None:
        try:
            await self.handle()
        finally:
            task_id = self._task_context.id
            task_tenant_id = self._task_context.tenant_id
            task_dataset_id = self._task_context.kb_id
            task_doc_id = self._task_context.doc_id
            if self._task_context.has_canceled_func(task_id):
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
                await self._run_artifact(embedding_model)
            elif task_type == "evaluation":
                await self._run_evaluation()
            elif task_type == "reembedding":
                await self._run_reembedding()
            elif task_type == "clone":
                await self._run_clone()
            else:
                await self._run_standard_chunking(embedding_model)


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
        embedding_model: LLMBundle
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
            ctx.progress_cb(1., msg=f"No chunk built from {ctx.name}")
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

        # Build TOC if needed (TOC continues to run in parallel during ingest;
        # artifact_compilation has been moved to AFTER the chunk insert below
        # because REFINE needs to look the source chunks up in ES by id).
        toc_thread = None

        # Insert chunks
        chunk_count = len(set([chunk["id"] for chunk in chunks]))
        start_ts = timer()

        chunk_service = ChunkService(ctx=ctx)

        if ctx.has_canceled_func(task_id):
            ctx.progress_cb(-1, msg="Task has been canceled.")
            return

        insert_result = await chunk_service.insert_chunks(
            task_id, task_tenant_id, task_dataset_id, chunks
        )

        if not insert_result:
            ctx.recording_context.record("insertion_result", "failed")
            return
        ctx.recording_context.record("insertion_result", "success")

        # Post-processing
        post_processor = PostProcessor(ctx=ctx)
        await post_processor.process_table_parser_metadata(task_doc_id, chunks)

        ctx.progress_cb(msg="Indexing done ({:.2f}s).".format(timer() - start_ts))

        # Artifact compilation runs as a separate KB-wide task that the user
        # triggers from the "Artifact" generate button 鈥?handled by
        # TaskHandler._run_artifact when task_type == "artifact".

        toc_chunk = await self._process_toc_thread(toc_thread)
        if toc_chunk:
            ctx.recording_context.record("toc_chunk", [toc_chunk])
            await post_processor.insert_toc_chunk(toc_chunk, chunk_service)

        await self._run_document_structure_compile(chunks, embedding_model)

        # Per-doc RAPTOR auto-trigger. Reads ``parser_config.raptor.use_raptor``
        # which is set via the chunk-method dialog (default off). Queues
        # a doc-scoped raptor task so the executor picks it up and runs
        # ``_run_raptor`` against just this document's chunks.
        # Best-effort — a queue failure doesn't fail the chunking task.
        raptor_cfg = (ctx.parser_config or {}).get("raptor") or {}
        if raptor_cfg.get("use_raptor"):
            try:
                ok_doc, doc_obj = DocumentService.get_by_id(task_doc_id)
                if ok_doc and doc_obj is not None:
                    ctx.progress_cb(msg="Starting RAPTOR task.")
                    await self._run_raptor(embedding_model, vector_size)
                else:
                    logging.warning(
                        "raptor: cannot resolve doc %s to queue per-doc task", task_doc_id,
                    )
            except Exception:
                logging.exception(
                    "raptor: failed to queue per-doc task for doc %s", task_doc_id,
                )

        if ctx.has_canceled_func(task_id):
            ctx.progress_cb(-1, msg="Task has been canceled.")
            return

        # Update document stats
        if ctx.write_interceptor:
            ctx.write_interceptor.intercept("DocumentService.increment_chunk_num")
        else:
            DocumentService.increment_chunk_num(task_doc_id, task_dataset_id, token_count, chunk_count, 0)

        task_time_cost = timer() - task_start_ts
        ctx.recording_context.record("task_status", "completed")
        ctx.progress_cb(prog=1.0, msg="Task done ({:.2f}s)".format(task_time_cost))

        logging.info(
            "Chunk doc({}), page({}-{}), chunks({}), token({}), elapsed:{:.2f}".format(
                ctx.name, ctx.from_page, ctx.to_page,
                len(chunks), token_count, task_time_cost
            )
        )

    async def _run_document_structure_compile(self, chunks: list[dict], embedding_model: LLMBundle) -> None:
        """Run document-scoped knowledge compilation for non-artifact templates."""
        ctx = self._task_context
        template_ids = _parser_config_compilation_template_ids(ctx.parser_config)
        if not template_ids:
            return

        compile_chunks = sorted(chunks, key=lambda d: (
            d.get("page_num_int", 0)[0] if isinstance(d.get("page_num_int", 0), list) else d.get("page_num_int", 0),
            d.get("top_int", 0)[0] if isinstance(d.get("top_int", 0), list) else d.get("top_int", 0)
        ))

        doc_task_llm_id = ctx.parser_config.get("llm_id") or ctx.llm_id
        chat_model_config = get_model_config_from_provider_instance(
            ctx.tenant_id, LLMType.CHAT, doc_task_llm_id
        )
        chat_mdl = LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language)

        total = len(template_ids)
        progress_cb = ctx.progress_cb
        for idx, template_id in enumerate(template_ids):
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

            progress_cb(msg=f"Start document knowledge compilation ({idx + 1}/{total}) ...")
            docs = await compile_structure_from_text(
                compile_chunks,
                parser_cfg,
                chat_mdl,
                embedding_model,
                ctx.doc_id,
                language=ctx.language,
                callback=progress_cb,
                compilation_template_id=template_id,
            )
            info = await merge_compiled_structures(
                docs,
                chat_mdl,
                embedding_model,
                ctx.tenant_id,
                ctx.kb_id,
                compilation_template_id=template_id,
            )
            ctx.recording_context.record(f"document_structure_compile:{template_id}", info)
            progress_cb(msg=f"Document knowledge compilation done ({idx + 1}/{total}): {info}")

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
    
    async def _run_artifact(self, embedding_model):
        """KB-wide artifact compilation task. Runs after the user clicks the
        "Artifact" button in the dataset generate menu. Iterates every doc in
        the KB whose parser config has ``compilation_template_ids`` selected,
        runs MAP per-doc (which uses ES-stored resume rows to skip
        chunks already processed in a previous run), then runs REDUCE / PLAN /
        REFINE KB-wide and persists pages.

        Batching: each MAP call uses ``batch_size_cap=8`` and
        ``window_fraction=0.5`` 鈥?i.e. roll over to a new batch when the
        current batch reaches 8 chunks OR its accumulated token count
        exceeds 50% of the chat model's ``max_length``.
        """
        ctx = self._task_context
        progress = ctx.progress_cb
        progress(0.0, "Loading documents for artifact compilation...")

        # 1. Resolve KB metadata for PLAN.
        ok, kb = KnowledgebaseService.get_by_id(ctx.kb_id)
        if not ok:
            progress(-1, f"KB {ctx.kb_id} not found.")
            return
        kb_name = kb.name
        kb_description = kb.description

        # 2. Pick docs eligible for artifact compilation (those with a
        # compilation_template_ids stamped into their parser_config). The
        # frontend Artifact button targets the KB, but the per-doc opt-in
        # is what gates inclusion.
        all_docs, _ = await thread_pool_exec(
            DocumentService.get_by_kb_id,
            kb_id=ctx.kb_id, page_number=0, items_per_page=0,
            orderby="create_time", desc=False,
            keywords="", run_status=[], types=[], suffix=[],
        )
        eligible = []
        for d in all_docs or []:
            pc = d.get("parser_config") or {}
            for template_id in _parser_config_compilation_template_ids(pc):
                template = CompilationTemplateService.get_saved(template_id, ctx.tenant_id)
                config = template.get("config") if template else {}
                kind = _compilation_template_kind(config.get("kind") if isinstance(config, dict) else "")
                if kind == "artifacts":
                    eligible.append((d, template_id))
                    break
        if not eligible:
            progress(1.0, "No documents are configured for artifact compilation.")
            return

        # 3. Resolve chat model for the KB-level task. There's no per-doc
        # LLM here (the task has no real ctx.doc_id) so fall back to the
        # tenant default chat model 鈥?same pattern raptor/graphrag use.
        chat_model_config = get_tenant_default_model_by_type(ctx.tenant_id, LLMType.CHAT)
        chat_mdl = LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language)

        def _stage_cb(prefix: str):
            def _cb(*args, **kwargs):
                try:
                    if args and isinstance(args[0], (int, float)):
                        msg = args[1] if len(args) > 1 else kwargs.get("msg", "")
                        progress(msg=f"{prefix} {msg}")
                    else:
                        msg = kwargs.get("msg") or (args[0] if args else "")
                        progress(msg=f"{prefix} {msg}")
                except Exception:
                    logging.exception("artifact: progress callback failed")
            return _cb

        # 4. MAP per eligible doc. Each MAP call's own resume mechanism
        # (artifact_map_extract rows keyed by chunk_id) skips chunks that
        # were already processed in a prior run 鈥?this is the incremental
        # behavior the user asked for.
        n_docs = len(eligible)
        for i, (doc, template_id) in enumerate(eligible):
            doc_id = doc["id"]
            progress(0.05 + 0.6 * (i / n_docs), f"MAP {i + 1}/{n_docs}: {doc.get('name', doc_id)}")

            # Resolve the per-doc template's config (different docs may use
            # different templates within the same KB).
            template = CompilationTemplateService.get_saved(template_id, ctx.tenant_id)
            if not template:
                logging.warning("artifact: template %s not found for doc %s; skipping", template_id, doc_id)
                continue
            parser_cfg = template.get("config") or {}

            chunks = await self._load_chunks_for_doc(ctx.tenant_id, ctx.kb_id, doc_id)
            if not chunks:
                logging.info("artifact: no chunks for doc %s; skipping", doc_id)
                continue

            try:
                phase1 = await artifact_map_from_chunks(
                    chunks=chunks,
                    chat_mdl=chat_mdl,
                    embd_mdl=embedding_model,
                    doc_id=doc_id,
                    tenant_id=ctx.tenant_id,
                    kb_id=ctx.kb_id,
                    language=ctx.language,
                    callback=_stage_cb(f"[artifact MAP {i + 1}/{n_docs}]"),
                    parser_config=parser_cfg,
                    batch_size_cap=8,
                    window_fraction=0.5,
                )
                logging.info(
                    "artifact: MAP doc=%s entities=%d concepts=%d claims=%d relations=%d",
                    doc_id,
                    len(phase1.get("entities") or []),
                    len(phase1.get("concepts") or []),
                    len(phase1.get("claims") or []),
                    len(phase1.get("relations") or []),
                )
            except Exception:
                logging.exception("artifact: MAP failed for doc %s", doc_id)

        # 5. REDUCE / PLAN / REFINE KB-wide.
        try:
            progress(0.65, "Reducing extracts KB-wide...")
            await artifact_reduce_from_extracts(
                chat_mdl=chat_mdl,
                embd_mdl=embedding_model,
                tenant_id=ctx.tenant_id,
                kb_id=ctx.kb_id,
                force_rerun=True,
                callback=_stage_cb("[artifact REDUCE]"),
            )

            progress(0.75, "Planning artifact pages...")
            await artifact_plan_from_reduction(
                chat_mdl=chat_mdl,
                embd_mdl=embedding_model,
                tenant_id=ctx.tenant_id,
                kb_id=ctx.kb_id,
                kb_name=kb_name,
                kb_description=kb_description,
                force_rerun=True,
                callback=_stage_cb("[artifact PLAN]"),
            )

            progress(0.85, "Refining pages...")
            pages = await artifact_refine_from_plan(
                chat_mdl=chat_mdl,
                embd_mdl=embedding_model,
                tenant_id=ctx.tenant_id,
                kb_id=ctx.kb_id,
                force_rerun=True,
                callback=_stage_cb("[artifact REFINE]"),
            )
        except Exception:
            logging.exception("artifact: REDUCE/PLAN/REFINE failed for kb %s", ctx.kb_id)
            progress(-1, "Artifact pipeline failed during REDUCE/PLAN/REFINE.")
            return

        # 6. Persist searchable artifact_page rows.
        try:
            await self._persist_artifact_pages_to_es(ctx, pages or [], embedding_model)
        except Exception:
            logging.exception("artifact: ES persist failed for kb %s", ctx.kb_id)

        # 7. Materialize the canvas graph from the refined pages.
        # This is what the dataset Artifact tab's graph view reads.
        try:
            await self._persist_artifact_page_graph_to_es(ctx, pages or [])
        except Exception:
            logging.exception("artifact: page-graph persist failed for kb %s", ctx.kb_id)

        progress(1.0, f"Artifact compiled {len(pages or [])} page(s).")

    @staticmethod
    async def _load_chunks_for_doc(tenant_id: str, kb_id: str, doc_id: str) -> List[Dict]:
        """Load all available chunks for one document from the doc store.

        Paginates with a 500-row window. Skips ``compile_kwd``-marked rows
        (artifact pages, structure entities, etc.) by filtering on
        ``available_int=1`` + ``doc_id=<doc_id>`` 鈥?the artifact writer
        stores its rows under ``doc_id=<kb_id_str>`` as a sentinel, so a
        real-doc filter excludes them naturally.
        """
        from common.doc_store.doc_store_base import OrderByExpr

        index_nm = search.index_name(tenant_id)
        if not settings.docStoreConn.index_exist(index_nm, kb_id):
            return []

        select_fields = [
            "id", "doc_id", "content_with_weight",
            "page_num_int", "top_int",
        ]
        chunks: List[Dict] = []
        offset = 0
        PAGE = 500
        while True:
            try:
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    select_fields, [], {"doc_id": [doc_id], "available_int": 1},
                    [], OrderByExpr(), offset, PAGE,
                    index_nm, [kb_id],
                )
                field_map = settings.docStoreConn.get_fields(res, select_fields)
            except Exception:
                logging.exception("artifact: failed to load chunks for doc=%s", doc_id)
                break
            if not field_map:
                break
            for row_id, row in field_map.items():
                # Defensive: skip any row that snuck in with a compile_kwd
                # (some backends ignore the doc_id filter for sentinel rows).
                if row.get("compile_kwd"):
                    continue
                chunks.append({
                    "id": row_id,
                    "doc_id": row.get("doc_id") or doc_id,
                    "content_with_weight": row.get("content_with_weight") or "",
                    "page_num_int": row.get("page_num_int", 0),
                    "top_int": row.get("top_int", 0),
                })
            if len(field_map) < PAGE:
                break
            offset += PAGE
        # Stable ordering by (page, top) so MAP batches are reproducible.
        chunks.sort(key=lambda d: (
            (d.get("page_num_int", 0) or [0])[0] if isinstance(d.get("page_num_int"), list) else d.get("page_num_int", 0),
            (d.get("top_int", 0) or [0])[0] if isinstance(d.get("top_int"), list) else d.get("top_int", 0),
        ))
        return chunks

    async def _persist_artifact_pages_to_es(
        self, ctx: TaskContext, pages: List[Dict], embd_mdl,
    ) -> None:
        """Insert one ES row per generated artifact page using the
        knowledge-compilation schema:

          id                  xxh64(kb_id + ":" + slug)        鈥?16-char hex
          compile_kwd         "artifact_page"                  鈥?marker (unchanged)
          slug_kwd            page.slug
          title_kwd           page.title
          page_type_kwd       page.page_type
          entity_names_kwd    page.entity_names
          outlinks_kwd        page.outlinks
          related_kb_pages_kwd page.related_kb_pages
          source_chunk_ids    page.source_chunk_ids            (pass-through)
          source_doc_ids      page.source_doc_ids              (pass-through)
          kb_id               ctx.kb_id                        (pass-through)
          content_with_weight rendered markdown                鈥?for UI render
          content_ltks /
          content_sm_ltks     tokenize(content_md + summary)   鈥?for keyword search
          q_<dim>_vec         embed(summary)                   鈥?for vector search

        ``action`` is intentionally not stored 鈥?it's a planner artifact
        and has no meaning post-write.
        """
        if not pages:
            return

        from rag.nlp import rag_tokenizer

        index = search.index_name(ctx.tenant_id)
        kb_id_str = str(ctx.kb_id)

        # Batch the summary embeddings in one model call. The encoder rejects
        # empty strings on most providers, so swap empties for a single space 鈥?
        # they still yield a vector but contribute nothing meaningful, which
        # matches the "no summary" page's lack of semantic signal.
        summaries = [(p.get("summary") or "").strip() for p in pages]
        embed_inputs = [s if s else " " for s in summaries]
        try:
            embeddings, _ = await thread_pool_exec(embd_mdl.encode, embed_inputs)
        except Exception:
            logging.exception("artifact_persist: summary embedding batch failed for kb=%s", kb_id_str)
            return
        try:
            n_emb = len(embeddings) if embeddings is not None else 0
        except TypeError:
            n_emb = 0
        if n_emb != len(pages):
            logging.warning(
                "artifact_persist: embedding count %d != pages %d for kb=%s; aborting",
                n_emb, len(pages), kb_id_str,
            )
            return

        rows: List[Dict] = []
        for page, vec in zip(pages, embeddings):
            slug = page.get("slug") or ""
            if not slug:
                continue
            title = page.get("title") or slug
            summary = page.get("summary") or ""
            content_md = (
                page.get("content_md_rendered")
                or page.get("content_md")
                or page.get("content_md_raw")
                or ""
            )

            vec_list = vec.tolist() if hasattr(vec, "tolist") else list(vec)
            if not vec_list:
                logging.warning("artifact_persist: empty embedding for slug=%s; skipping", slug)
                continue

            text_for_search = (content_md + "\n\n" + summary).strip()
            content_ltks = rag_tokenizer.tokenize(text_for_search) if text_for_search else ""
            content_sm_ltks = (
                rag_tokenizer.fine_grained_tokenize(content_ltks) if content_ltks else ""
            )

            row_id = xxhash.xxh64(
                f"{kb_id_str}:{slug}".encode("utf-8", "surrogatepass"),
            ).hexdigest()

            rows.append({
                "id": row_id,
                "kb_id": kb_id_str,
                "doc_id": kb_id_str,  # sentinel; KB-scoped row, real provenance in source_doc_ids
                "compile_kwd": "artifact_page",
                "slug_kwd": slug,
                "title_kwd": title,
                "page_type_kwd": page.get("page_type") or "concept",
                "entity_names_kwd": list(page.get("entity_names") or []),
                "outlinks_kwd": list(page.get("outlinks") or []),
                "outlinks_int": len(list(page.get("outlinks") or [])),
                "related_kb_pages_kwd": list(page.get("related_kb_pages") or []),
                "source_chunk_ids": list(page.get("source_chunk_ids") or []),
                "source_doc_ids": list(page.get("source_doc_ids") or []),
                "content_with_weight": content_md,
                # Summary kept verbatim alongside the rendered body so the
                # viewer can render it as a distinct (smaller) block above
                # the main content.
                "summary_with_weight": summary,
                "content_ltks": content_ltks,
                "content_sm_ltks": content_sm_ltks,
                f"q_{len(vec_list)}_vec": vec_list,
                "available_int": 1,
            })

        if not rows:
            return

        try:
            await thread_pool_exec(settings.docStoreConn.insert, rows, index, ctx.kb_id)
        except Exception:
            logging.exception(
                "artifact_persist: bulk insert failed for kb=%s (rows=%d)",
                kb_id_str, len(rows),
            )

    @staticmethod
    def _build_artifact_page_graph(pages: List[Dict], kb_id: str) -> Dict:
        """Project the REFINE-emitted page list onto the canvas graph shape.

        Graph schema (what the frontend ``ForceGraph`` adapter consumes)::

            {
              "entities": [
                {
                  "slug":        "<page.slug>",       # stable id; UI uses for deep-link
                  "name":        "<page.title>",      # human-readable label
                  "aliases":     [<page.entity_names>],
                  "description": "<page.summary>",
                  "type":        "<page.page_type>",
                },
                ...
              ],
              "relations": [
                {"from": "<src_slug>", "to": "<dst_slug>"},
                ...
              ]
            }

        Dangling outlinks (a slug not present as a node in this KB) are
        dropped — they'd render as orphan edges otherwise. ``kb_id`` is
        accepted for symmetry with future per-KB metadata but not
        emitted on the graph; the persistence row carries it.
        """
        del kb_id  # currently unused on the graph blob itself
        by_slug: Dict[str, Dict] = {}
        for p in pages or []:
            slug = (p.get("slug") or "").strip()
            if not slug:
                continue
            outlinks_raw = p.get("outlinks") or []
            # ``weight`` is the page's outlink count — i.e. how many
            # other artifact pages this one points at. Drives node size
            # / importance on the canvas. Computed on the raw outlink
            # list (before dangling-target filtering) so visual weight
            # reflects what the writer actually emitted on this page,
            # not the post-filter graph topology.
            weight = len(outlinks_raw) if isinstance(outlinks_raw, list) else 0
            by_slug[slug] = {
                "slug": slug,
                "name": p.get("title") or slug,
                "aliases": list(p.get("entity_names") or []),
                "description": p.get("summary") or "",
                "type": p.get("page_type") or "concept",
                "weight": weight,
            }

        relations: List[Dict] = []
        for p in pages or []:
            src = (p.get("slug") or "").strip()
            if not src or src not in by_slug:
                continue
            for raw_target in (p.get("outlinks") or []):
                if isinstance(raw_target, str):
                    tgt = raw_target.strip()
                elif isinstance(raw_target, dict):
                    tgt = str(raw_target.get("slug") or "").strip()
                else:
                    tgt = ""
                if not tgt or tgt == src or tgt not in by_slug:
                    continue
                relations.append({"from": src, "to": tgt})

        return {"entities": list(by_slug.values()), "relations": relations}

    async def _persist_artifact_page_graph_to_es(
        self, ctx: TaskContext, pages: List[Dict],
    ) -> None:
        """Materialize and store the canvas graph derived from artifact pages.

        Writes a single non-searchable row with ``compile_kwd="artifact_page_graph"``
        whose ``content_with_weight`` is the JSON-serialized graph. The row id is
        deterministic per KB, so re-runs replace cleanly via delete-then-insert.

        ``dataset_api_service.get_artifact_graph`` reads exactly this row.
        """
        kb_id_str = str(ctx.kb_id)
        graph = self._build_artifact_page_graph(pages or [], kb_id_str)

        index = search.index_name(ctx.tenant_id)
        row_id = xxhash.xxh64(
            f"artifact_page_graph:{kb_id_str}".encode("utf-8", "surrogatepass"),
        ).hexdigest()
        row = {
            "id": row_id,
            "kb_id": kb_id_str,
            "doc_id": kb_id_str,  # sentinel: KB-scoped row, not a real document
            "compile_kwd": "artifact_page_graph",
            "source_id": [kb_id_str],
            "content_with_weight": json.dumps(graph, ensure_ascii=False),
            "available_int": 0,
        }
        try:
            await thread_pool_exec(
                settings.docStoreConn.delete,
                {"compile_kwd": "artifact_page_graph"},
                index, ctx.kb_id,
            )
        except Exception:
            logging.debug(
                "artifact_page_graph: prior delete failed; relying on id-upsert",
            )
        try:
            await thread_pool_exec(
                settings.docStoreConn.insert, [row], index, ctx.kb_id,
            )
        except Exception:
            logging.exception(
                "artifact_page_graph: insert failed for kb=%s", kb_id_str,
            )

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
