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
Raptor Service Module.

Provides [`RaptorService`](rag/svr/task_executor_refactor/raptor_service.py:48) for RAPTOR
(Recursive Abstractive Processing for Tree-Organized Retrieval) summary generation.
"""

import copy
import json
import logging
import os
from datetime import datetime
from typing import Dict, List, Optional, Set, Tuple

import numpy as np
import xxhash

from api.db.services.document_service import DocumentService
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID
from common import settings
from common.connection_utils import timeout
from common.constants import PAGERANK_FLD
from common.misc_utils import thread_pool_exec
from common.token_utils import num_tokens_from_string
from rag.nlp import rag_tokenizer, search
from rag.utils.raptor_utils import (
    collect_raptor_chunk_ids,
    collect_raptor_methods,
    get_raptor_clustering_method,
    get_raptor_tree_builder,
    get_skip_reason,
    make_raptor_summary_chunk_id,
    should_skip_raptor,
)
from rag.svr.task_executor_refactor.task_context import TaskContext


def _sum_tree_text_tokens(tree) -> int:
    """Count tokens across every ``title`` string in the RAPTOR tree.

    Mirrors the legacy ``tk_count`` semantic (sum over summary texts)
    so the orchestrator's downstream logging / billing keeps working
    when the tree path replaces the per-summary rows. Walks the dict
    iteratively to avoid recursion-limit issues on deep trees.
    """
    if not isinstance(tree, dict):
        return 0
    total = 0
    stack = [tree]
    while stack:
        node = stack.pop()
        if not isinstance(node, dict):
            continue
        title = node.get("title")
        if isinstance(title, str) and title:
            total += num_tokens_from_string(title)
        children = node.get("children")
        if isinstance(children, list):
            stack.extend(children)
    return total


class RaptorService:
    """Service for RAPTOR summary generation.

    This service handles:
    - RAPTOR chunk method detection (checkpoint)
    - RAPTOR summary generation per document or dataset-level
    - Stale RAPTOR chunk cleanup
    - Auto-disable rules for certain file types
    """

    def __init__(
        self,
        ctx: TaskContext,
    ):
        """Initialize RaptorService.

        Args:
            ctx: TaskContext containing task configuration and execution resources.
        """
        self._task_context = ctx

    @timeout(3600)
    async def run_raptor_for_kb(
        self,
        kb_parser_config: Dict,
        chat_mdl,
        embd_mdl,
        vector_size: int,
        doc_ids: List[str],
    ) -> Tuple[List[Dict], int, List[Tuple[str, Optional[str]]]]:
        """Generate RAPTOR summaries for selected documents.

        Args:
            kb_parser_config: Knowledge base parser configuration.
            chat_mdl: Chat model bundle for RAPTOR.
            embd_mdl: Embedding model bundle for RAPTOR.
            vector_size: Vector dimension size.
            doc_ids: List of document IDs to process.

        Returns:
            Tuple of (chunks, token_count, cleanup_raptor_chunks).
        """
        raptor_config = kb_parser_config.get("raptor", {})
        tree_builder = get_raptor_tree_builder(raptor_config)
        clustering_method = get_raptor_clustering_method(raptor_config)
        vctr_nm = "q_%d_vec" % vector_size

        res = []
        tk_count = 0
        cleanup_raptor_chunks = []
        max_errors = int(os.environ.get("RAPTOR_MAX_ERRORS", 3))

        # Collect document info
        doc_info_by_id = self._collect_doc_info(doc_ids)

        # Determine scope
        if raptor_config.get("scope", "file") == "file":
            res, tk_count = await self._run_file_level_raptor(
                raptor_config, tree_builder, clustering_method, chat_mdl, embd_mdl, vctr_nm, doc_ids, doc_info_by_id, max_errors, res, tk_count, cleanup_raptor_chunks
            )
        else:
            res, tk_count = await self._run_dataset_level_raptor(
                raptor_config, tree_builder, clustering_method, chat_mdl, embd_mdl, vctr_nm, doc_ids, doc_info_by_id, max_errors, res, tk_count, cleanup_raptor_chunks
            )

        return res, tk_count, cleanup_raptor_chunks

    @classmethod
    def _collect_doc_info(cls, doc_ids: List[str]) -> Dict[str, Dict]:
        """Collect document info for all doc_ids."""
        doc_info_by_id = {}
        for doc_id in set(doc_ids):
            ok, source_doc = DocumentService.get_by_id(doc_id)
            if not ok or not source_doc:
                continue
            doc_info_by_id[doc_id] = {
                "name": getattr(source_doc, "name", ""),
                "type": getattr(source_doc, "type", ""),
                "parser_id": getattr(source_doc, "parser_id", ""),
                "parser_config": getattr(source_doc, "parser_config", {}) or {},
            }
        return doc_info_by_id

    async def _run_file_level_raptor(self, raptor_config, tree_builder, clustering_method, chat_mdl, embd_mdl, vctr_nm, doc_ids, doc_info_by_id, max_errors, res, tk_count, cleanup_raptor_chunks):
        """Run RAPTOR at file level (per document)."""
        ctx = self._task_context
        fake_doc_id = GRAPH_RAPTOR_FAKE_DOC_ID
        if self._task_context.write_interceptor:  # dry run mode
            dataset_methods = set()
        else:
            dataset_methods = await self._get_raptor_chunk_methods(fake_doc_id, ctx.tenant_id, ctx.kb_id)
        remove_dataset_summaries = bool(dataset_methods)
        has_file_level_target = False

        if dataset_methods:
            self._task_context.progress_cb(msg="[RAPTOR] will remove dataset-level summaries after file-level summaries are available.")

        for x, doc_id in enumerate(doc_ids):
            if self._should_skip_raptor(doc_id, doc_info_by_id, raptor_config):
                self._task_context.progress_cb(prog=(x + 1.0) / len(doc_ids))
                continue
            if self._task_context.write_interceptor:
                existing_methods = set()
            else:
                existing_methods = await self._get_raptor_chunk_methods(doc_id, ctx.tenant_id, ctx.kb_id)
            if tree_builder in existing_methods:
                has_file_level_target = True
                if existing_methods != {tree_builder}:
                    self._schedule_raptor_cleanup(doc_id, tree_builder, cleanup_raptor_chunks)
                    self._task_context.progress_cb(msg=f"[RAPTOR] doc:{doc_id} will remove old RAPTOR summaries after insert.")
                self._task_context.progress_cb(msg=f"[RAPTOR] doc:{doc_id} already has {tree_builder} RAPTOR chunks, skipping.")
                self._task_context.progress_cb(prog=(x + 1.0) / len(doc_ids))
                continue

            if existing_methods:
                self._task_context.progress_cb(msg=f"[RAPTOR] doc:{doc_id} will migrate RAPTOR summaries to {tree_builder} after insert.")

            chunks = self._load_doc_chunks(doc_id, vctr_nm)
            if not chunks:
                continue

            before_generate = len(res)
            new_chunks, new_tk_count = await self._generate_raptor(chunks, doc_id, raptor_config, chat_mdl, embd_mdl, tree_builder, clustering_method, max_errors, doc_info_by_id)
            res.extend(new_chunks)
            tk_count += new_tk_count

            if len(res) > before_generate:
                has_file_level_target = True
                if existing_methods:
                    self._schedule_raptor_cleanup(doc_id, tree_builder, cleanup_raptor_chunks)
            self._task_context.progress_cb(prog=(x + 1.0) / len(doc_ids))

        if remove_dataset_summaries:
            if has_file_level_target:
                self._schedule_raptor_cleanup(fake_doc_id, None, cleanup_raptor_chunks)
            else:
                self._task_context.progress_cb(msg="[RAPTOR] kept dataset-level summaries because no file-level summaries were built.")

        return res, tk_count

    async def _run_dataset_level_raptor(self, raptor_config, tree_builder, clustering_method, chat_mdl, embd_mdl, vctr_nm, doc_ids, doc_info_by_id, max_errors, res, tk_count, cleanup_raptor_chunks):
        """Run RAPTOR at dataset level (all documents combined)."""
        ctx = self._task_context
        fake_doc_id = GRAPH_RAPTOR_FAKE_DOC_ID
        migrated_file_docs = 0
        file_cleanup_doc_ids = []
        skipped_doc_ids = set()

        for doc_id in set(doc_ids):
            if self._should_skip_raptor(doc_id, doc_info_by_id, raptor_config):
                skipped_doc_ids.add(doc_id)
                continue
            if self._task_context.write_interceptor:
                existing_methods = set()
            else:
                existing_methods = await self._get_raptor_chunk_methods(doc_id, ctx.tenant_id, ctx.kb_id)
            if existing_methods:
                file_cleanup_doc_ids.append(doc_id)
                migrated_file_docs += 1

        if migrated_file_docs:
            self._task_context.progress_cb(msg=f"[RAPTOR] will remove file-level summaries for {migrated_file_docs} docs after dataset-level build succeeds.")

        if self._task_context.write_interceptor:
            existing_methods = set()
        else:
            existing_methods = await self._get_raptor_chunk_methods(fake_doc_id, ctx.tenant_id, ctx.kb_id)
        if tree_builder in existing_methods:
            if existing_methods != {tree_builder}:
                self._schedule_raptor_cleanup(fake_doc_id, tree_builder, cleanup_raptor_chunks)
                self._task_context.progress_cb(msg="[RAPTOR] will remove old dataset-level RAPTOR summaries after insert.")
            for doc_id in file_cleanup_doc_ids:
                self._schedule_raptor_cleanup(doc_id, None, cleanup_raptor_chunks)
            self._task_context.progress_cb(msg=f"[RAPTOR] dataset-level {tree_builder} summaries already exist, skipping.")
            return res, tk_count

        migrate_dataset_summaries = bool(existing_methods)
        if migrate_dataset_summaries:
            self._task_context.progress_cb(msg=f"[RAPTOR] will migrate dataset-level RAPTOR summaries to {tree_builder} after insert.")

        chunks = self._load_all_doc_chunks(doc_ids, vctr_nm, skipped_doc_ids)
        if not chunks:
            if skipped_doc_ids and len(skipped_doc_ids) == len(set(doc_ids)):
                self._task_context.progress_cb(msg="[RAPTOR] all documents were skipped by RAPTOR auto-disable rules.")
                return res, tk_count
            self._task_context.progress_cb(msg="[ERROR] No valid chunks with vectors found. Please ensure documents are parsed with the current embedding model.")
            return res, tk_count

        before_generate = len(res)
        new_chunks, new_tk_count = await self._generate_raptor(chunks, fake_doc_id, raptor_config, chat_mdl, embd_mdl, tree_builder, clustering_method, max_errors, doc_info_by_id)
        res.extend(new_chunks)
        tk_count += new_tk_count

        if len(res) > before_generate:
            for doc_id in file_cleanup_doc_ids:
                self._schedule_raptor_cleanup(doc_id, None, cleanup_raptor_chunks)
            if migrate_dataset_summaries:
                self._schedule_raptor_cleanup(fake_doc_id, tree_builder, cleanup_raptor_chunks)

        return res, tk_count

    def _should_skip_raptor(self, doc_id: str, doc_info_by_id: Dict, raptor_config: Dict) -> bool:
        """Check if RAPTOR should be skipped for a document."""
        ctx = self._task_context
        doc_info = doc_info_by_id.get(doc_id, {})
        file_type = doc_info.get("type") or ctx.raw_task.get("type", "")
        parser_id = doc_info.get("parser_id") or ctx.parser_id
        parser_config = doc_info.get("parser_config") or ctx.parser_config

        if should_skip_raptor(file_type, parser_id, parser_config, raptor_config):
            skip_reason = get_skip_reason(file_type, parser_id, parser_config)
            doc_name = doc_info.get("name") or doc_id
            logging.info("Skipping Raptor for document %s: %s", doc_name, skip_reason)
            self._task_context.progress_cb(msg=f"[RAPTOR] doc:{doc_id} skipped: {skip_reason}")
            return True
        return False

    def _load_doc_chunks(self, doc_id: str, vctr_nm: str) -> List[Tuple[str, np.ndarray, str]]:
        """Load chunks for a single document.

        Returns ``(content, vector, chunk_id)`` triples so downstream
        RAPTOR can attach ``source_chunk_ids`` provenance onto every
        summary it produces. ``chunk_id`` may be an empty string if the
        retriever didn't surface one — defensive against legacy rows.
        """
        ctx = self._task_context
        chunks: List[Tuple[str, np.ndarray, str]] = []
        skipped_chunks = 0

        # ``id`` is included so the source-chunk provenance survives
        # through summarization; the retriever otherwise drops it when
        # ``fields`` is provided.
        fields = ["id", "content_with_weight", vctr_nm]
        for d in settings.retriever.chunk_list(doc_id, ctx.tenant_id, [str(ctx.kb_id)], fields=fields, sort_by_position=True):
            if vctr_nm not in d or d[vctr_nm] is None:
                skipped_chunks += 1
                logging.warning(f"RAPTOR: Chunk missing vector field '{vctr_nm}' in doc {doc_id}, skipping")
                continue
            chunks.append((d["content_with_weight"], np.array(d[vctr_nm]), str(d.get("id") or "")))

        if skipped_chunks > 0:
            self._task_context.progress_cb(msg=f"[WARN] Skipped {skipped_chunks} chunks without vector field '{vctr_nm}' for doc {doc_id}.")
        if not chunks:
            logging.warning(f"RAPTOR: No valid chunks with vectors found for doc {doc_id}")
            self._task_context.progress_cb(msg=f"[WARN] No valid chunks with vectors found for doc {doc_id}, skipping")

        return chunks

    def _load_all_doc_chunks(self, doc_ids: List[str], vctr_nm: str, skipped_doc_ids: Set[str]) -> List[Tuple[str, np.ndarray, str]]:
        """Load chunks for all documents — returns provenance-carrying
        ``(content, vector, chunk_id)`` triples. See ``_load_doc_chunks``
        for the per-doc variant."""
        ctx = self._task_context
        chunks: List[Tuple[str, np.ndarray, str]] = []
        skipped_chunks = 0

        fields = ["id", "content_with_weight", vctr_nm]
        for doc_id in doc_ids:
            if doc_id in skipped_doc_ids:
                continue
            for d in settings.retriever.chunk_list(doc_id, ctx.tenant_id, [str(ctx.kb_id)], fields=fields, sort_by_position=True):
                if vctr_nm not in d or d[vctr_nm] is None:
                    skipped_chunks += 1
                    logging.warning(f"RAPTOR: Chunk missing vector field '{vctr_nm}' in doc {doc_id}, skipping")
                    continue
                chunks.append((d["content_with_weight"], np.array(d[vctr_nm]), str(d.get("id") or "")))

        if skipped_chunks > 0:
            self._task_context.progress_cb(msg=f"[WARN] Skipped {skipped_chunks} chunks without vector field '{vctr_nm}'.")

        return chunks

    async def _generate_raptor(
        self,
        chunks: List[Tuple[str, np.ndarray, str]],
        doc_id: str,
        raptor_config: Dict,
        chat_mdl,
        embd_mdl,
        tree_builder: str,
        clustering_method: str,
        max_errors: int,
        doc_info_by_id: Dict,
        is_tree: bool = False,
    ) -> Tuple[List[Dict], int]:
        """Run RAPTOR and generate summary chunks.

        ``chunks`` is the provenance-carrying triple shape produced by
        ``_load_doc_chunks`` / ``_load_all_doc_chunks``:
        ``(content, vector, chunk_id)``. Each leaf is wrapped into the
        ``(text, vec, [chunk_id])`` shape RAPTOR expects so every
        summary it produces carries the order-preserving deduped union
        of the leaf ids underneath it.
        """
        ctx = self._task_context
        from rag.advanced_rag.knowlege_compile.raptor import RecursiveAbstractiveProcessing4TreeOrganizedRetrieval as Raptor

        raptor_ext_config = raptor_config.get("ext") or {}
        assert chunks, "_generate_raptor must not be called with empty chunks"
        vctr_nm = "q_%d_vec" % len(chunks[0][1])

        raptor = Raptor(
            raptor_config.get("max_cluster", 64),
            chat_mdl,
            embd_mdl,
            raptor_config["prompt"],
            raptor_config["max_token"],
            raptor_config["threshold"],
            max_errors=max_errors,
            tree_builder=tree_builder,
            clustering_method=clustering_method,
            psi_exact_max_leaves=raptor_ext_config.get("psi_exact_max_leaves", 4096),
            psi_bucket_size=raptor_ext_config.get("psi_bucket_size", 1024),
        )

        # Seed each leaf with its own id as the start of its
        # ``source_chunk_ids`` provenance trail. The id may be empty
        # for malformed retriever rows; ``Raptor.__call__`` filters
        # those out of the union on the inbound normalize step.
        raptor_input = [(content, vctr, [chunk_id] if chunk_id else []) for content, vctr, chunk_id in chunks]

        effective_doc_name = ctx.name if doc_id == GRAPH_RAPTOR_FAKE_DOC_ID else doc_info_by_id.get(doc_id, {}).get("name") or ctx.name

        # Default path: ask RAPTOR for a single hierarchical tree dict
        # and persist it as ONE non-searchable ES row. PSI's
        # hyperedge-driven summarization can't form a strict
        # parent-of relation, so __call__(is_tree=True) raises
        # NotImplementedError there — catch and fall through to the
        # legacy per-summary materialization below for that case.
        original_length = len(chunks)
        try:
            processed_chunks, layers = await raptor(
                raptor_input,
                raptor_config["random_seed"],
                self._task_context.progress_cb,
                ctx.id,
                is_tree=is_tree,
            )
        except NotImplementedError:
            return await self._generate_raptor_legacy_rows(
                raptor,
                raptor_input,
                raptor_config,
                doc_id,
                effective_doc_name,
                tree_builder,
                vctr_nm,
            )

        if processed_chunks is None:
            return [], 0
        doc = {
            "doc_id": doc_id,
            "kb_id": [str(ctx.kb_id)],
            "docnm_kwd": effective_doc_name,
            "title_tks": rag_tokenizer.tokenize(effective_doc_name),
            "raptor_kwd": "raptor",
            "extra": {"raptor_method": tree_builder},
            "create_time": str(datetime.now()).replace("T", " ")[:19],
            "create_timestamp_flt": datetime.now().timestamp(),
        }
        if ctx.pagerank:
            doc[PAGERANK_FLD] = int(ctx.pagerank)

        if not is_tree:
            # Build index→layer mapping
            chunk_layer = {}
            for layer_idx, (layer_start, layer_end) in enumerate(layers):
                if layer_idx == 0:
                    continue
                for ci in range(layer_start, layer_end):
                    chunk_layer[ci] = layer_idx

            res = []
            tk_count = 0
            for idx, (content, vctr, _, _) in enumerate(processed_chunks[original_length:], start=original_length):
                d = copy.deepcopy(doc)
                d["id"] = make_raptor_summary_chunk_id(content, doc_id)
                d["create_time"] = str(datetime.now()).replace("T", " ")[:19]
                d["create_timestamp_flt"] = datetime.now().timestamp()
                d[vctr_nm] = vctr.tolist()
                d["content_with_weight"] = content
                d["content_ltks"] = rag_tokenizer.tokenize(content)
                d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
                d["raptor_layer_int"] = chunk_layer.get(idx, 1)
                res.append(d)
                tk_count += num_tokens_from_string(content)
            return res, tk_count

        row_id = xxhash.xxh64(
            f"raptor_tree:{doc_id}:{tree_builder}".encode("utf-8", "surrogatepass"),
        ).hexdigest()
        row = {
            **doc,
            "id": row_id,
            "raptor_kwd": "raptor_tree",
            "content_with_weight": json.dumps(processed_chunks, ensure_ascii=False),
            "available_int": 0,
        }
        return [row], _sum_tree_text_tokens(processed_chunks)

    async def build_doc_tree(
        self,
        chunks: List[Tuple[str, np.ndarray, str]],
        raptor_config: Dict,
        chat_mdl,
        embd_mdl,
        tree_builder: str,
        clustering_method: str,
        max_errors: int,
    ) -> Optional[Dict]:
        """Build a RAPTOR tree dict for one document — no ES IO.

        Used by the ``tree``-kind compilation template, which wraps the
        returned tree into a per-template structure-graph row. Returns
        None when the input has no chunks, the PSI builder is selected
        (which can't form a strict tree), or RAPTOR itself fails.
        """
        if not chunks:
            return None
        from rag.advanced_rag.knowlege_compile.raptor import RecursiveAbstractiveProcessing4TreeOrganizedRetrieval as Raptor

        raptor_ext_config = raptor_config.get("ext") or {}
        raptor = Raptor(
            raptor_config.get("max_cluster", 64),
            chat_mdl,
            embd_mdl,
            raptor_config["prompt"],
            raptor_config["max_token"],
            raptor_config["threshold"],
            max_errors=max_errors,
            tree_builder=tree_builder,
            clustering_method=clustering_method,
            psi_exact_max_leaves=raptor_ext_config.get("psi_exact_max_leaves", 4096),
            psi_bucket_size=raptor_ext_config.get("psi_bucket_size", 1024),
        )

        raptor_input = [(content, vctr, [chunk_id] if chunk_id else []) for content, vctr, chunk_id in chunks]
        try:
            tree, _ = await raptor(
                raptor_input,
                raptor_config["random_seed"],
                self._task_context.progress_cb,
                self._task_context.id,
                is_tree=True,
            )
        except NotImplementedError:
            # PSI builder — not supported in tree mode; surface as None
            # so the compilation-template path can skip the doc cleanly.
            logging.warning(
                "build_doc_tree: PSI builder doesn't support is_tree; skipping",
            )
            return None
        return tree if isinstance(tree, dict) else None

    async def _generate_raptor_legacy_rows(
        self,
        raptor,
        raptor_input,
        raptor_config,
        doc_id,
        effective_doc_name,
        tree_builder,
        vctr_nm,
    ) -> Tuple[List[Dict], int]:
        """Legacy per-summary materialization, kept only for PSI builds.

        PSI's hyperedge summaries don't map to a strict tree, so the
        ``is_tree=True`` default in ``_generate_raptor`` raises and
        falls through here. Same shape this function produced before
        the tree migration — one ES row per appended summary, marked
        ``raptor_kwd="raptor"``.
        """
        ctx = self._task_context
        original_length = len(raptor_input)
        processed_chunks, layers = await raptor(
            raptor_input,
            raptor_config["random_seed"],
            self._task_context.progress_cb,
            ctx.id,
        )

        doc = {
            "doc_id": doc_id,
            "kb_id": [str(ctx.kb_id)],
            "docnm_kwd": effective_doc_name,
            "title_tks": rag_tokenizer.tokenize(effective_doc_name),
            "raptor_kwd": "raptor",
            "extra": {"raptor_method": tree_builder},
        }
        if ctx.pagerank:
            doc[PAGERANK_FLD] = int(ctx.pagerank)

        chunk_layer = {}
        for layer_idx, (layer_start, layer_end) in enumerate(layers):
            if layer_idx == 0:
                continue
            for ci in range(layer_start, layer_end):
                chunk_layer[ci] = layer_idx

        res = []
        tk_count = 0
        for idx, item in enumerate(processed_chunks[original_length:], start=original_length):
            if len(item) >= 3:
                content, vctr, source_chunk_ids = item[0], item[1], item[2] or []
            else:
                content, vctr = item[0], item[1]
                source_chunk_ids = []
            d = copy.deepcopy(doc)
            d["id"] = make_raptor_summary_chunk_id(content, doc_id)
            d["create_time"] = str(datetime.now()).replace("T", " ")[:19]
            d["create_timestamp_flt"] = datetime.now().timestamp()
            d[vctr_nm] = vctr.tolist()
            d["content_with_weight"] = content
            d["content_ltks"] = rag_tokenizer.tokenize(content)
            d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
            d["raptor_layer_int"] = chunk_layer.get(idx, 1)
            if source_chunk_ids:
                d["source_chunk_ids"] = list(source_chunk_ids)
            res.append(d)
            tk_count += num_tokens_from_string(content)

        return res, tk_count

    @classmethod
    def _schedule_raptor_cleanup(cls, doc_id: str, keep_method: Optional[str], cleanup_list: List):
        """Queue stale RAPTOR summaries for deletion."""
        cleanup_plan = (doc_id, keep_method)
        if cleanup_plan not in cleanup_list:
            cleanup_list.append(cleanup_plan)

    @classmethod
    async def _get_raptor_chunk_methods(cls, doc_id: str, tenant_id: str, kb_id: str) -> Set[str]:
        """Get RAPTOR chunk methods for a document."""
        from common.doc_store.doc_store_base import OrderByExpr

        async def search_fields(fields: list, condition: dict, order_by=None):
            res = await thread_pool_exec(settings.docStoreConn.search, fields, [], condition, [], order_by or OrderByExpr(), 0, 10000, search.index_name(tenant_id), [kb_id])
            return settings.docStoreConn.get_fields(res, fields)

        try:
            # Accept both ``raptor`` (legacy per-summary rows, PSI
            # builder still produces these) and ``raptor_tree`` (new
            # single-row tree blob) so existing-method detection stays
            # accurate across the migration.
            primary = await search_fields(
                ["raptor_kwd", "extra"],
                {"doc_id": doc_id, "raptor_kwd": ["raptor", "raptor_tree"]},
            )
            if collect_raptor_chunk_ids(primary):
                return collect_raptor_methods(primary)

            return collect_raptor_methods(
                await search_fields(
                    ["raptor_kwd", "extra"],
                    {"doc_id": doc_id},
                    OrderByExpr().desc("create_timestamp_flt"),
                )
            )
        except Exception:
            logging.exception("Failed to check RAPTOR chunks for doc %s", doc_id)
            raise

    @staticmethod
    def _build_raptor_graph(rows: List[Dict]) -> Dict:
        """Project loaded RAPTOR summary rows onto the canvas graph shape.

        Each row contributes one entity::

            {
              "id":          xxh128(content)           # 32-char hex
              "name":        first 16 whitespace tokens
              "description": content_with_weight
              "source_chunk_ids": row.source_chunk_ids
            }

        Relations: full bipartite layer-by-layer fan-out — every node at
        layer K gets an edge to every node at layer K-1 (because we only
        loaded ``content_with_weight`` + ``raptor_layer_int`` we don't
        have the specific parent linkage). Self-edges and dangling
        targets are dropped (the latter only matters if the layer-int
        values are non-contiguous).
        """
        # Build entities. Dedup by id so two identical-content summaries
        # collapse to one node — the canvas can't render multiple nodes
        # at the same id anyway, and identical content is a defensible
        # collapse.
        by_id: Dict[str, Dict] = {}
        by_layer: Dict[int, List[str]] = {}

        for row in rows:
            content = row.get("content_with_weight")
            if not isinstance(content, str) or not content.strip():
                continue
            try:
                layer = int(row.get("raptor_layer_int") or 0)
            except (TypeError, ValueError):
                layer = 0
            if layer <= 0:
                # Layer 0 would be the original leaf chunks; RAPTOR
                # summaries start at layer 1. Anything claiming layer 0
                # here is malformed; skip.
                continue

            name = " ".join(content.split()[:16])
            nid = xxhash.xxh128(
                content.encode("utf-8", "surrogatepass"),
            ).hexdigest()  # 32-char hex
            if nid in by_id:
                continue
            source_chunk_ids = row.get("source_chunk_ids") or []
            if not isinstance(source_chunk_ids, list):
                source_chunk_ids = []
            by_id[nid] = {
                "id": nid,
                "name": name,
                "description": content,
                "source_chunk_ids": list(source_chunk_ids),
            }
            by_layer.setdefault(layer, []).append(nid)

        # Layered fan-out from parent (higher layer) → child (lower layer).
        relations: List[Dict] = []
        layers_sorted = sorted(by_layer.keys())
        for layer in layers_sorted:
            child_layer = layer - 1
            if child_layer not in by_layer:
                continue
            for parent in by_layer[layer]:
                for child in by_layer[child_layer]:
                    if parent == child:
                        continue
                    relations.append({"from": parent, "to": child})

        return {"entities": list(by_id.values()), "relations": relations}

    async def _persist_raptor_graph_to_es(self, doc_id: str) -> None:
        """Load the just-inserted RAPTOR summaries for ``doc_id`` and
        persist a single graph row that the dataset structure-graph
        endpoint can surface as a tree.

        Loads only ``content_with_weight`` + ``raptor_layer_int`` +
        ``source_chunk_ids`` (per
        the smallest-payload contract) and writes one row with::

            compile_kwd:                  "raptor_graph"
            compilation_template_kind_kwd:"raptor"
            doc_id:                       <doc_id>

        The row id is deterministic per ``(kb_id, doc_id)`` so re-runs
        delete-and-replace cleanly through the same primary key.
        ``knowledge_graph_kwd`` is intentionally NOT set — that field
        belongs to the KG feature; this row is identified via
        ``compile_kwd`` so the two paths stay semantically distinct.
        """
        from common.doc_store.doc_store_base import OrderByExpr

        ctx = self._task_context
        tenant_id = ctx.tenant_id
        kb_id_str = str(ctx.kb_id)
        index_nm = search.index_name(tenant_id)
        select_fields = ["content_with_weight", "raptor_layer_int", "source_chunk_ids"]
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields,
                [],
                {"raptor_kwd": ["raptor"], "doc_id": [doc_id]},
                [],
                OrderByExpr(),
                0,
                10000,
                index_nm,
                [kb_id_str],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields)
        except Exception:
            logging.exception(
                "raptor_graph: load failed for kb=%s doc=%s",
                kb_id_str,
                doc_id,
            )
            return

        rows = list((field_map or {}).values())
        if not rows:
            logging.info(
                "raptor_graph: no summaries to render for kb=%s doc=%s",
                kb_id_str,
                doc_id,
            )
            return

        graph = self._build_raptor_graph(rows)
        if not graph["entities"]:
            logging.info(
                "raptor_graph: projection produced no entities for kb=%s doc=%s",
                kb_id_str,
                doc_id,
            )
            return

        row_id = xxhash.xxh64(
            f"raptor_graph:{kb_id_str}:{doc_id}".encode("utf-8", "surrogatepass"),
        ).hexdigest()
        row = {
            "id": row_id,
            "kb_id": kb_id_str,
            "doc_id": doc_id,
            "compile_kwd": "raptor_graph",
            "compilation_template_kind_kwd": "raptor",
            "content_with_weight": json.dumps(graph, ensure_ascii=False),
            "available_int": 0,
        }
        try:
            await thread_pool_exec(
                settings.docStoreConn.delete,
                {"compile_kwd": "raptor_graph", "doc_id": [doc_id]},
                index_nm,
                ctx.kb_id,
            )
        except Exception:
            logging.debug(
                "raptor_graph: prior delete failed for kb=%s doc=%s; relying on id-upsert",
                kb_id_str,
                doc_id,
            )
        try:
            await thread_pool_exec(
                settings.docStoreConn.insert,
                [row],
                index_nm,
                ctx.kb_id,
            )
            logging.info(
                "raptor_graph: stored %d entities / %d relations for kb=%s doc=%s",
                len(graph["entities"]),
                len(graph["relations"]),
                kb_id_str,
                doc_id,
            )
        except Exception:
            logging.exception(
                "raptor_graph: insert failed for kb=%s doc=%s",
                kb_id_str,
                doc_id,
            )
