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
Dataflow Service Module.

Provides [`DataflowService`](rag/svr/task_executor_refactor/dataflow_service.py:42) for dataflow
pipeline execution.
"""

import abc
import copy
import logging
import re
from datetime import datetime
from timeit import default_timer as timer
from typing import Dict, List, Optional, Tuple

import numpy as np
import xxhash
from common import settings
from rag.svr.task_executor_refactor.embedding_utils import EmbeddingUtils
from rag.flow.pipeline import Pipeline

from api.db.services.canvas_service import UserCanvasService
from api.db.services.document_service import DocumentService
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.pipeline_operation_log_service import PipelineOperationLogService
from api.db.joint_services.tenant_model_service import get_model_config_from_provider_instance
from common.constants import LLMType, PipelineTaskType
from common.metadata_utils import update_metadata_to
from common.misc_utils import thread_pool_exec
from rag.nlp import rag_tokenizer, add_positions
from rag.svr.task_executor_refactor.constants import CANVAS_DEBUG_DOC_ID
from rag.svr.task_executor_refactor.task_context import TaskContext


class BillingHook(abc.ABC):
    """Abstract base for billing hooks on pipeline success/error.

    Implementations override the no-op methods to integrate with billing
    systems (e.g., consume quota on success, release hold on error).
    """

    async def on_pipeline_success(self) -> None:
        """Called when the dataflow pipeline completes successfully."""

    async def on_pipeline_error(self) -> None:
        """Called when the dataflow pipeline encounters an error."""


class DataflowService:
    """Service for dataflow pipeline execution.

    This service handles:
    - Dataflow DSL loading and execution
    - Chunk embedding for dataflow output
    - Chunk metadata processing and indexing
    """

    def __init__(
        self,
        ctx: TaskContext,
        billing_hook: Optional[BillingHook] = None,
        embedding_batch_size: int = None,
        doc_bulk_size: int = None,
    ):
        """Initialize DataflowService.

        Args:
            ctx: TaskContext containing task configuration and execution resources.
            billing_hook: Optional billing hook for pipeline success/error callbacks.
            embedding_batch_size: Batch size for embedding operations.
            doc_bulk_size: Batch size for document store inserts.
        """
        self._task_context = ctx
        self._billing_hook = billing_hook
        self._embedding_batch_size = embedding_batch_size or self._get_default_embedding_batch_size()
        self._doc_bulk_size = doc_bulk_size or self._get_default_bulk_size()

    async def run_dataflow(self) -> None:
        """Run a dataflow pipeline."""
        ctx = self._task_context
        pipeline = None
        try:
            task_start_ts = timer()
            dataflow_id = ctx.dataflow_id
            doc_id = ctx.doc_id
            task_id = ctx.id
            task_dataset_id = ctx.kb_id

            # Load DSL
            dsl = await self._load_dsl(dataflow_id)
            if dsl is None:
                return

            # Run pipeline
            pipeline = Pipeline(
                dsl, tenant_id=ctx.tenant_id, doc_id=doc_id,
                task_id=task_id, flow_id=dataflow_id
            )
            chunks = await pipeline.run(file=ctx.file) if ctx.file else await pipeline.run()

            if doc_id == CANVAS_DEBUG_DOC_ID:
                ctx.recording_context.record("dataflow_debug_result", "canvas_debug_mode")
                ctx.recording_context.record("dataflow_chunks", chunks)
                return

            if not chunks:
                ctx.recording_context.record("pipeline_output_count", 0)
                ctx.recording_context.record("pipeline_output_type", "empty")
                self._record_pipeline_log(doc_id, dataflow_id, pipeline)
                return

            embedding_token_consumption = chunks.get("embedding_token_consumption", 0)
            output_type = DataflowService._get_output_type(chunks)
            chunks = self._normalize_chunks(chunks)

            ctx.recording_context.record("pipeline_output_type", output_type)
            ctx.recording_context.record("pipeline_output_count", len(chunks))

            if not chunks:
                self._record_pipeline_log(doc_id, dataflow_id, pipeline)
                return

            # Embed chunks if needed
            keys = [k for o in chunks for k in list(o.keys())]
            if not any([re.match(r"q_[0-9]+_vec", k) for k in keys]):
                chunks, embedding_token_consumption = await self._embed_chunks(
                    chunks, embedding_token_consumption
                )
                if chunks is None:
                    self._record_pipeline_log(doc_id, dataflow_id, pipeline)
                    return

            # Process chunks
            metadata = self._process_chunks(chunks)

            # Update document metadata
            if metadata:
                self._update_document_metadata(doc_id, metadata)

            # Insert chunks
            start_ts = timer()
            self._progress(prog=0.82, msg="[DOC Engine]:\nStart to index...")
            e = await self._insert_chunks(
                task_id, ctx.tenant_id, ctx.kb_id, chunks
            )
            if not e:
                self._record_pipeline_log(doc_id, dataflow_id, pipeline)
                return

            time_cost = timer() - start_ts
            task_time_cost = timer() - task_start_ts
            self._progress(
                prog=1.,
                msg="Indexing done ({:.2f}s). Task done ({:.2f}s)".format(time_cost, task_time_cost)
            )

            # Update document stats
            if ctx.write_interceptor:
                ctx.write_interceptor.intercept("DocumentService.increment_chunk_num")
            else:
                DocumentService.increment_chunk_num(
                    doc_id, task_dataset_id, embedding_token_consumption, len(chunks), task_time_cost
                )

            logging.info(
                "[Done], chunks({}), token({}), elapsed:{:.2f}".format(
                    len(chunks), embedding_token_consumption, task_time_cost
                )
            )
            ctx.recording_context.record("dataflow_chunks", chunks)
            self._record_pipeline_log(doc_id, dataflow_id, pipeline)

            # Billing hook: pipeline succeeded
            if self._billing_hook:
                await self._billing_hook.on_pipeline_success()
        except Exception:
            if self._billing_hook:
                await self._billing_hook.on_pipeline_error()
            raise

    async def _load_dsl(self, dataflow_id: str) -> Optional[str]:
        """Load dataflow DSL from service."""
        ctx = self._task_context
        if ctx.task_type == "dataflow":
            e, cvs = UserCanvasService.get_by_id(dataflow_id)
            assert e, "User pipeline not found."
            return cvs.dsl
        else:
            e, pipeline_log = PipelineOperationLogService.get_by_id(dataflow_id)
            assert e, "Pipeline log not found."
            return pipeline_log.dsl

    @staticmethod
    def _get_output_type(chunks: Dict) -> str:
        """Determine output type from chunks dict."""
        if "chunks" in chunks:
            return "chunks"
        elif "json" in chunks:
            return "json"
        elif "markdown" in chunks:
            return "markdown"
        elif "text" in chunks:
            return "text"
        elif "html" in chunks:
            return "html"
        return "empty"

    @classmethod
    def _normalize_chunks(cls, chunks: Dict) -> List[Dict]:
        """Normalize chunks from various output formats."""
        if "chunks" in chunks:
            return copy.deepcopy(chunks["chunks"])
        elif "json" in chunks:
            return copy.deepcopy(chunks["json"])
        elif "markdown" in chunks:
            return [{"text": [chunks["markdown"]]}] if chunks["markdown"] else []
        elif "text" in chunks:
            return [{"text": [chunks["text"]]}] if chunks["text"] else []
        elif "html" in chunks:
            return [{"text": [chunks["html"]]}] if chunks["html"] else []
        return []

    async def _embed_chunks(
        self, chunks: List[Dict], token_consumption: int
    ) -> Tuple[Optional[List[Dict]], int]:
        """Embed chunks using the embedding model."""
        ctx = self._task_context
        try:
            self._progress(prog=0.82, msg="\n-------------------------------------\nStart to embedding...")
            e, kb = self._get_kb_by_id(ctx.kb_id)
            embedding_id = kb.embd_id
            embd_model_config = get_model_config_from_provider_instance(
                ctx.tenant_id, LLMType.EMBEDDING, embedding_id
            )
            from api.db.services.llm_service import LLMBundle
            with LLMBundle(ctx.tenant_id, embd_model_config) as embedding_model:

                # Prepare texts for embedding using EmbeddingUtils
                texts = EmbeddingUtils.prepare_texts_for_dataflow_embedding(chunks)
                delta = 0.20 / (len(texts) // self._embedding_batch_size + 1)
                prog = 0.8

                # Batch encode using EmbeddingUtils
                vects_batches = []
                for i in range(0, len(texts), self._embedding_batch_size):
                    batch = texts[i: i + self._embedding_batch_size]
                    async with ctx.embed_limiter:
                        vts, c = await thread_pool_exec(
                            self._encode_batch, batch, embedding_model
                        )
                    vects_batches.append(vts)
                    token_consumption += c
                    prog += delta
                    if i % (len(texts) // self._embedding_batch_size / 100 + 1) == 1:
                        self._progress(
                            prog=prog,
                            msg=f"{i + 1} / {len(texts) // self._embedding_batch_size}"
                        )

                # Stack vectors using EmbeddingUtils
                vects = EmbeddingUtils.stack_vectors(vects_batches)
                if len(vects) != len(chunks):
                    raise ValueError(f"Vector count mismatch: {len(vects)} vs {len(chunks)}")

                # Attach vectors using EmbeddingUtils
                EmbeddingUtils.attach_vectors(chunks, vects)

                return chunks, token_consumption

        except Exception as e:
            ctx.progress_cb(prog=-1, msg=f"[ERROR]: {e}")
            return None, token_consumption

    @classmethod
    async def _encode_batch(cls, txts: List[str], embedding_model) -> Tuple[np.ndarray, int]:
        """Batch encode texts using the embedding model with truncation."""
        truncated = EmbeddingUtils.truncate_texts(txts, embedding_model.max_length)
        return embedding_model.encode(truncated)

    def _process_chunks(self, chunks: List[Dict]) -> Dict:
        """Process chunks for metadata and indexing."""
        ctx = self._task_context
        metadata = {}
        for ck in chunks:
            ck["doc_id"] = ctx.doc_id
            ck["kb_id"] = [str(ctx.kb_id)]
            ck["docnm_kwd"] = ctx.name
            ck["create_time"] = str(datetime.now()).replace("T", " ")[:19]
            ck["create_timestamp_flt"] = datetime.now().timestamp()

            if not ck.get("id"):
                ck["id"] = xxhash.xxh64((ck["text"] + str(ck["doc_id"])).encode("utf-8")).hexdigest()

            if "questions" in ck:
                if "question_tks" not in ck:
                    ck["question_kwd"] = ck["questions"].split("\n")
                    ck["question_tks"] = rag_tokenizer.tokenize(str(ck["questions"]))
                del ck["questions"]

            if "keywords" in ck:
                if "important_tks" not in ck:
                    ck["important_kwd"] = [k for k in re.split(r"[,，;；、\r\n]+", ck["keywords"]) if k.strip()]
                    ck["important_tks"] = rag_tokenizer.tokenize(str(ck["keywords"]))
                del ck["keywords"]

            if "summary" in ck:
                if "content_ltks" not in ck:
                    ck["content_ltks"] = rag_tokenizer.tokenize(str(ck["summary"]))
                    ck["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(ck["content_ltks"])
                del ck["summary"]

            if "metadata" in ck:
                metadata = update_metadata_to(metadata, ck["metadata"])
                del ck["metadata"]

            if "content_with_weight" not in ck:
                ck["content_with_weight"] = ck["text"]
            del ck["text"]

            if "positions" in ck:
                add_positions(ck, ck["positions"])
                del ck["positions"]

        return metadata

    def _update_document_metadata(self, doc_id: str, metadata: Dict) -> None:
        """Update document metadata."""
        existing_meta = DocMetadataService.get_document_metadata(doc_id)
        existing_meta = existing_meta if isinstance(existing_meta, dict) else {}
        metadata = update_metadata_to(metadata, existing_meta)
        self._task_context.recording_context.record("run_dataflow_metadata", metadata)
        if self._task_context.write_interceptor:
            self._task_context.write_interceptor.intercept("DocMetadataService.update_document_metadata")
        else:
            DocMetadataService.update_document_metadata(doc_id, metadata)

    async def _insert_chunks(
        self, task_id: str, tenant_id: str, kb_id: str, chunks: List[Dict]
    ) -> bool:
        """Insert chunks into document store."""
        from rag.svr.task_executor_refactor.chunk_service import ChunkService
        chunk_service = ChunkService(self._task_context)
        return await chunk_service.insert_chunks(task_id, tenant_id, kb_id, chunks)

    def _record_pipeline_log(self, doc_id: str, dataflow_id: str, pipeline) -> None:
        """Record pipeline operation log."""
        if self._task_context.write_interceptor:
            self._task_context.write_interceptor.intercept("PipelineOperationLogService.create")
        else:
            PipelineOperationLogService.create(
                document_id=doc_id, pipeline_id=dataflow_id,
                task_type=PipelineTaskType.PARSE, dsl=str(pipeline)
            )

    @classmethod
    def _get_kb_by_id(cls, kb_id: str):
        """Get knowledge base by ID."""
        from api.db.services.knowledgebase_service import KnowledgebaseService
        return KnowledgebaseService.get_by_id(kb_id)

    def _progress(self, prog=None, msg=None):
        """Progress callback helper."""
        if prog is not None or msg is not None:
            self._task_context.progress_cb(prog=prog, msg=msg)

    @classmethod
    def _get_default_embedding_batch_size(cls) -> int:
        """Get default embedding batch size."""
        return settings.EMBEDDING_BATCH_SIZE

    @classmethod
    def _get_default_bulk_size(cls) -> int:
        """Get default bulk size."""
        return settings.DOC_BULK_SIZE
