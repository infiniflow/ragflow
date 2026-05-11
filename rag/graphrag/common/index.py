#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import asyncio
import json
import logging
import os
from dataclasses import dataclass, fields
from typing import Any

import networkx as nx

from api.db.services.document_service import DocumentService
from api.db.services.task_service import has_canceled
from common import settings
from common.doc_store.doc_store_base import OrderByExpr
from common.exceptions import TaskCanceledException
from common.misc_utils import thread_pool_exec
from rag.graphrag.entity_resolution import EntityResolution
from rag.graphrag.general.community_reports_extractor import CommunityReportsExtractor
from rag.graphrag.general.extractor import Extractor
from rag.graphrag.general.graph_extractor import GraphExtractor as GeneralKGExt
from rag.graphrag.light.graph_extractor import GraphExtractor as LightKGExt
from rag.graphrag.ner.graph_extractor import GraphExtractor as NerKGExt
from rag.graphrag.phase_markers import (
    PHASE_COMMUNITY,
    PHASE_RESOLUTION,
    clear_phase_markers,
    has_phase_marker,
    set_phase_marker,
)
from rag.graphrag.utils import (
    GraphChange,
    chunk_id,
    get_graph,
    graph_merge,
    insert_chunks_bounded,
    set_graph,
    tidy_graph,
)
from rag.nlp import rag_tokenizer, search
from rag.utils.redis_conn import RedisDistributedLock


@dataclass(frozen=True)
class GraphRAGRunContext:
    graph_config_key: str = "graphrag"
    extractor_method_key: str = "method"
    default_extractor_method: str = "light"
    entity_types_key: str = "entity_types"

    doc_fetch_page_number: int = 0
    doc_fetch_items_per_page: int = 0
    doc_fetch_orderby: str = "create_time"
    doc_fetch_desc: bool = False
    doc_fetch_keywords: str = ""
    doc_fetch_run_status: tuple[Any, ...] = ()
    doc_fetch_types: tuple[Any, ...] = ()
    doc_fetch_suffix: tuple[Any, ...] = ()

    chunk_fields: tuple[str, ...] = ("content_with_weight", "doc_id")
    chunk_content_field: str = "content_with_weight"
    raw_chunk_max_count: int = 10000
    chunk_token_limit: int = 4096
    report_raw_chunk_count: bool = False

    timeout_assertion_env: str = "ENABLE_TIMEOUT_ASSERTION"
    disabled_timeout_seconds: float = 10_000_000_000
    build_timeout_min_seconds: float = 120
    build_timeout_seconds_per_chunk: float = 600
    merge_timeout_seconds: float = 60 * 3
    resolution_timeout_seconds: float = 60 * 30
    community_timeout_seconds: float = 60 * 30

    max_parallel_docs: int = 4
    lock_name_prefix: str = "graphrag_task"
    merge_lock_value: str = "batch_merge"
    lock_timeout_seconds: int = 1200

    graph_kwd: str = "graph"
    subgraph_kwd: str = "subgraph"
    community_report_kwd: str = "community_report"
    removed_kwd_field: str = "removed_kwd"
    removed_active_value: str = "N"
    available_int: int = 0

    subgraph_lookup_limit: int = 1
    stale_checkpoint_search_limit: int = 10000
    community_report_search_limit: int = 10000
    doc_store_retry_attempts: int = 3
    doc_store_retry_base_delay_seconds: float = 1.0

    @classmethod
    def coerce(cls, value: "GraphRAGRunContext | dict[str, Any] | None") -> "GraphRAGRunContext":
        if value is None:
            return cls()
        if isinstance(value, cls):
            return value
        if isinstance(value, dict):
            allowed = {field.name for field in fields(cls)}
            unknown = sorted(set(value) - allowed)
            if unknown:
                raise ValueError(f"Unknown GraphRAGRunContext fields: {unknown}")
            return cls(**value)
        raise TypeError(f"run_context must be GraphRAGRunContext, dict, or None, got {type(value)!r}")


def _timeout_assertion_enabled(run_context: GraphRAGRunContext) -> bool:
    return bool(os.environ.get(run_context.timeout_assertion_env))


def _deadline_for_chunks(chunk_count: int, run_context: GraphRAGRunContext) -> float:
    if not _timeout_assertion_enabled(run_context):
        return run_context.disabled_timeout_seconds
    return max(
        run_context.build_timeout_min_seconds,
        chunk_count * run_context.build_timeout_seconds_per_chunk,
    )


async def _maybe_wait_for(awaitable, timeout_seconds: float, run_context: GraphRAGRunContext):
    if _timeout_assertion_enabled(run_context):
        return await asyncio.wait_for(awaitable, timeout=timeout_seconds)
    return await awaitable


def _task_id(row: dict) -> str:
    return row.get("id", "")


def _check_cancelled(row: dict, callback, message: str):
    task_id = _task_id(row)
    if task_id and has_canceled(task_id):
        callback(msg=message)
        raise TaskCanceledException(f"Task {task_id} was cancelled")


def _select_extractor(graphrag_config: dict, run_context: GraphRAGRunContext):
    method = graphrag_config.get(run_context.extractor_method_key, run_context.default_extractor_method)
    if method == "general":
        return GeneralKGExt
    if method == "ner":
        return NerKGExt
    return LightKGExt


async def _retry_doc_store_step(label: str, callback, operation, run_context: GraphRAGRunContext):
    for attempt in range(1, run_context.doc_store_retry_attempts + 1):
        try:
            return await operation()
        except (TaskCanceledException, asyncio.CancelledError):
            raise
        except Exception as exc:
            if attempt >= run_context.doc_store_retry_attempts:
                logging.exception("%s failed after %s attempts", label, run_context.doc_store_retry_attempts)
                raise
            delay = run_context.doc_store_retry_base_delay_seconds * (2 ** (attempt - 1))
            logging.warning(
                "%s failed on attempt %s/%s: %r; retrying in %.1fs",
                label,
                attempt,
                run_context.doc_store_retry_attempts,
                exc,
                delay,
            )
            callback(msg=f"[GraphRAG] {label} failed on attempt {attempt}/{run_context.doc_store_retry_attempts}, retrying in {delay:.1f}s.")
            await asyncio.sleep(delay)


async def load_subgraph_from_store(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    run_context: GraphRAGRunContext,
):
    fields_for_store = ["content_with_weight", "source_id"]
    condition = {
        "knowledge_graph_kwd": [run_context.subgraph_kwd],
        run_context.removed_kwd_field: run_context.removed_active_value,
        "source_id": [doc_id],
    }
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields_for_store,
            [],
            condition,
            [],
            OrderByExpr(),
            0,
            run_context.subgraph_lookup_limit,
            search.index_name(tenant_id),
            [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, fields_for_store)
        for cid, row in field_map.items():
            content = row.get("content_with_weight", "")
            if not content:
                continue
            try:
                subgraph = nx.node_link_graph(json.loads(content), edges="edges")
                subgraph.graph["source_id"] = [doc_id]
                logging.info(
                    "Checkpoint hit: subgraph for doc %s (tenant=%s kb=%s) found at chunk %s",
                    doc_id,
                    tenant_id,
                    kb_id,
                    cid,
                )
                return subgraph
            except Exception:
                logging.exception("Failed to parse subgraph JSON for doc %s chunk %s", doc_id, cid)
    except Exception:
        logging.exception("Failed to load subgraph from store for doc %s", doc_id)
        return None

    logging.info("Checkpoint miss: no subgraph for doc %s (tenant=%s kb=%s)", doc_id, tenant_id, kb_id)
    return None


async def does_graph_contains(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    run_context: GraphRAGRunContext,
) -> bool:
    fields_for_store = ["source_id"]
    condition = {
        "knowledge_graph_kwd": [run_context.graph_kwd],
        run_context.removed_kwd_field: run_context.removed_active_value,
    }
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        fields_for_store,
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        1,
        search.index_name(tenant_id),
        [kb_id],
    )
    fields_map = settings.docStoreConn.get_fields(res, fields_for_store)
    graph_doc_ids = set()
    for chunk_id_ in fields_map.keys():
        graph_doc_ids = set(fields_map[chunk_id_]["source_id"])
    return doc_id in graph_doc_ids


async def _stage_resolve_doc_ids(
    kb_id: str,
    doc_ids: list[str],
    run_context: GraphRAGRunContext,
    callback,
) -> list[str]:
    if not doc_ids:
        logging.info("Fetching all docs for %s", kb_id)
        docs, _ = DocumentService.get_by_kb_id(
            kb_id=kb_id,
            page_number=run_context.doc_fetch_page_number,
            items_per_page=run_context.doc_fetch_items_per_page,
            orderby=run_context.doc_fetch_orderby,
            desc=run_context.doc_fetch_desc,
            keywords=run_context.doc_fetch_keywords,
            run_status=list(run_context.doc_fetch_run_status),
            types=list(run_context.doc_fetch_types),
            suffix=list(run_context.doc_fetch_suffix),
        )
        doc_ids = [doc["id"] for doc in docs]

    doc_ids = list(dict.fromkeys(doc_ids))
    if not doc_ids:
        callback(msg=f"[GraphRAG] kb:{kb_id} has no processable doc_id.")
    return doc_ids


def _load_doc_chunks(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    run_context: GraphRAGRunContext,
    callback,
) -> list[str]:
    from common.token_utils import num_tokens_from_string

    chunks: list[str] = []
    current_chunk = ""
    current_tokens = 0

    raw_chunks = list(
        settings.retriever.chunk_list(
            doc_id,
            tenant_id,
            [kb_id],
            max_count=run_context.raw_chunk_max_count,
            fields=list(run_context.chunk_fields),
            sort_by_position=True,
        )
    )

    if run_context.report_raw_chunk_count:
        callback(msg=f"[GraphRAG] chunk_list returned {len(raw_chunks)} raw chunks for doc {doc_id}")

    for raw_chunk in raw_chunks:
        content = raw_chunk.get(run_context.chunk_content_field, "")
        if not content:
            continue

        content_tokens = num_tokens_from_string(content)
        if current_chunk and current_tokens + content_tokens >= run_context.chunk_token_limit:
            chunks.append(current_chunk)
            current_chunk = content
            current_tokens = content_tokens
        else:
            current_chunk += content
            current_tokens += content_tokens

    if current_chunk:
        chunks.append(current_chunk)

    return chunks


async def _stage_load_chunks(
    tenant_id: str,
    kb_id: str,
    doc_ids: list[str],
    run_context: GraphRAGRunContext,
    callback,
) -> tuple[dict[str, list[str]], list[tuple[str, str]], int]:
    all_doc_chunks: dict[str, list[str]] = {}
    failed_docs: list[tuple[str, str]] = []
    total_chunks = 0

    for doc_id in doc_ids:
        try:
            chunks = _load_doc_chunks(tenant_id, kb_id, doc_id, run_context, callback)
        except Exception as exc:
            failed_docs.append((doc_id, f"load chunks failed: {exc!r}"))
            callback(msg=f"[GraphRAG] doc:{doc_id} load chunks FAILED: {exc!r}")
            continue

        all_doc_chunks[doc_id] = chunks
        total_chunks += len(chunks)

    return all_doc_chunks, failed_docs, total_chunks


async def _stage_build_subgraphs(
    row: dict,
    tenant_id: str,
    kb_id: str,
    doc_ids: list[str],
    all_doc_chunks: dict[str, list[str]],
    language: str,
    kb_parser_config: dict,
    chat_model,
    embedding_model,
    run_context: GraphRAGRunContext,
    callback,
    *,
    max_parallel_docs: int,
) -> tuple[dict[str, nx.Graph], list[tuple[str, str]]]:
    semaphore = asyncio.Semaphore(max_parallel_docs)
    subgraphs: dict[str, nx.Graph] = {}
    failed_docs: list[tuple[str, str]] = []
    graphrag_config = kb_parser_config.get(run_context.graph_config_key, {})
    entity_types = graphrag_config.get(run_context.entity_types_key, [])
    kg_extractor = _select_extractor(graphrag_config, run_context)

    async def build_one(doc_id: str):
        _check_cancelled(row, callback, f"Task {_task_id(row)} cancelled, stopping execution.")

        chunks = all_doc_chunks.get(doc_id, [])
        if not chunks:
            callback(msg=f"[GraphRAG] doc:{doc_id} has no available chunks, skip generation.")
            return

        deadline = _deadline_for_chunks(len(chunks), run_context)

        async with semaphore:
            existing_subgraph = await load_subgraph_from_store(tenant_id, kb_id, doc_id, run_context)
            if existing_subgraph:
                subgraphs[doc_id] = existing_subgraph
                callback(msg=f"[GraphRAG] doc:{doc_id} subgraph found in store, skipping LLM extraction.")
                return

            msg = f"[GraphRAG] build_subgraph doc:{doc_id}"
            callback(msg=f"{msg} start (chunks={len(chunks)}, timeout={deadline}s)")
            try:
                subgraph = await asyncio.wait_for(
                    generate_subgraph(
                        kg_extractor,
                        tenant_id,
                        kb_id,
                        doc_id,
                        chunks,
                        language,
                        entity_types,
                        chat_model,
                        embedding_model,
                        callback,
                        run_context,
                        task_id=_task_id(row),
                    ),
                    timeout=deadline,
                )
            except asyncio.TimeoutError:
                failed_docs.append((doc_id, "timeout"))
                callback(msg=f"{msg} FAILED: timeout")
                return
            except TaskCanceledException as canceled:
                callback(msg=f"[GraphRAG] build_subgraph doc:{doc_id} cancelled: {canceled}")
                raise
            except asyncio.CancelledError:
                callback(msg=f"[GraphRAG] build_subgraph doc:{doc_id} cancelled.")
                raise
            except Exception as exc:
                failed_docs.append((doc_id, repr(exc)))
                callback(msg=f"[GraphRAG] build_subgraph doc:{doc_id} FAILED: {exc!r}")
                return

            if subgraph:
                subgraphs[doc_id] = subgraph
                callback(msg=f"{msg} done")
            else:
                failed_docs.append((doc_id, "subgraph is empty"))
                callback(msg=f"{msg} empty")

    tasks = [asyncio.create_task(build_one(doc_id)) for doc_id in doc_ids]
    try:
        await asyncio.gather(*tasks, return_exceptions=False)
    except Exception:
        for task in tasks:
            task.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise

    return subgraphs, failed_docs


async def _stage_merge_subgraphs(
    row: dict,
    tenant_id: str,
    kb_id: str,
    doc_ids: list[str],
    subgraphs: dict[str, nx.Graph],
    embedding_model,
    run_context: GraphRAGRunContext,
    callback,
    *,
    with_resolution: bool,
    with_community: bool,
) -> tuple[list[str], nx.Graph | None, bool, bool]:
    ok_docs = [doc_id for doc_id in doc_ids if doc_id in subgraphs]
    final_graph = None

    resolution_pending = with_resolution and not has_phase_marker(kb_id, PHASE_RESOLUTION)
    community_pending = with_community and not has_phase_marker(kb_id, PHASE_COMMUNITY)

    if not ok_docs and not resolution_pending and not community_pending:
        return ok_docs, final_graph, resolution_pending, community_pending

    kb_lock = RedisDistributedLock(
        f"{run_context.lock_name_prefix}_{kb_id}",
        lock_value=run_context.merge_lock_value,
        timeout=run_context.lock_timeout_seconds,
    )
    await kb_lock.spin_acquire()
    callback(msg=f"[GraphRAG] kb:{kb_id} merge lock acquired")

    try:
        _check_cancelled(row, callback, f"Task {_task_id(row)} cancelled before merging subgraphs.")

        for doc_id in ok_docs:
            new_graph = await _maybe_wait_for(
                merge_subgraph(
                    tenant_id,
                    kb_id,
                    doc_id,
                    subgraphs[doc_id],
                    embedding_model,
                    callback,
                ),
                run_context.merge_timeout_seconds,
                run_context,
            )
            if new_graph is not None:
                final_graph = new_graph

        if ok_docs and final_graph is None:
            callback(msg=f"[GraphRAG] kb:{kb_id} merge finished (no in-memory graph returned).")
        elif ok_docs:
            callback(msg=f"[GraphRAG] kb:{kb_id} merge finished, graph ready.")
            clear_phase_markers(kb_id)
            resolution_pending = with_resolution
            community_pending = with_community
            callback(msg=f"[GraphRAG] kb:{kb_id} cleared phase markers after merge.")
    finally:
        kb_lock.release()

    return ok_docs, final_graph, resolution_pending, community_pending


async def _stage_resolution_and_community(
    row: dict,
    tenant_id: str,
    kb_id: str,
    subgraphs: dict[str, nx.Graph],
    final_graph: nx.Graph | None,
    chat_model,
    embedding_model,
    run_context: GraphRAGRunContext,
    callback,
    *,
    with_resolution: bool,
    with_community: bool,
    resolution_pending: bool,
    community_pending: bool,
) -> nx.Graph | None:
    if not with_resolution and not with_community:
        return final_graph
    if not resolution_pending and not community_pending:
        return final_graph

    _check_cancelled(row, callback, f"Task {_task_id(row)} cancelled before resolution/community extraction.")

    kb_lock = RedisDistributedLock(
        f"{run_context.lock_name_prefix}_{kb_id}",
        lock_value=run_context.merge_lock_value,
        timeout=run_context.lock_timeout_seconds,
    )
    await kb_lock.spin_acquire()
    callback(msg=f"[GraphRAG] kb:{kb_id} post-merge lock acquired for resolution/community")

    try:
        if final_graph is None:
            final_graph = await get_graph(tenant_id, kb_id)
            if final_graph is None:
                callback(msg=f"[GraphRAG] kb:{kb_id} no persisted graph found; cannot run resolution/community.")
                return None
            callback(msg=f"[GraphRAG] kb:{kb_id} loaded persisted graph for resume.")

        subgraph_nodes = set()
        for subgraph in subgraphs.values():
            subgraph_nodes.update(set(subgraph.nodes()))
        if not subgraph_nodes:
            subgraph_nodes = set(final_graph.nodes())

        if resolution_pending:
            await _maybe_wait_for(
                resolve_entities(
                    final_graph,
                    subgraph_nodes,
                    tenant_id,
                    kb_id,
                    None,
                    chat_model,
                    embedding_model,
                    callback,
                    task_id=_task_id(row),
                ),
                run_context.resolution_timeout_seconds,
                run_context,
            )
            set_phase_marker(kb_id, PHASE_RESOLUTION)
        elif with_resolution:
            callback(msg=f"[GraphRAG] kb:{kb_id} resolution already completed previously, skipping.")

        if community_pending:
            await _maybe_wait_for(
                extract_community(
                    final_graph,
                    tenant_id,
                    kb_id,
                    None,
                    chat_model,
                    embedding_model,
                    callback,
                    run_context,
                    task_id=_task_id(row),
                ),
                run_context.community_timeout_seconds,
                run_context,
            )
            set_phase_marker(kb_id, PHASE_COMMUNITY)
        elif with_community:
            callback(msg=f"[GraphRAG] kb:{kb_id} community detection already completed previously, skipping.")
    finally:
        kb_lock.release()

    return final_graph


async def run_graphrag_for_kb(
    row: dict,
    doc_ids: list[str],
    language: str,
    kb_parser_config: dict,
    chat_model,
    embedding_model,
    callback,
    *,
    with_resolution: bool = True,
    with_community: bool = True,
    max_parallel_docs: int | None = None,
    run_context: GraphRAGRunContext | dict[str, Any] | None = None,
) -> dict:
    run_context = GraphRAGRunContext.coerce(run_context)
    max_parallel_docs = max_parallel_docs or run_context.max_parallel_docs
    tenant_id, kb_id = row["tenant_id"], row["kb_id"]
    start = asyncio.get_running_loop().time()

    doc_ids = await _stage_resolve_doc_ids(kb_id, doc_ids, run_context, callback)
    if not doc_ids:
        return {"ok_docs": [], "failed_docs": [], "total_docs": 0, "total_chunks": 0, "seconds": 0.0}

    all_doc_chunks, failed_docs, total_chunks = await _stage_load_chunks(
        tenant_id,
        kb_id,
        doc_ids,
        run_context,
        callback,
    )
    if total_chunks == 0:
        callback(msg=f"[GraphRAG] kb:{kb_id} has no available chunks in all documents, skip.")
        return {
            "ok_docs": [],
            "failed_docs": failed_docs or doc_ids,
            "total_docs": len(doc_ids),
            "total_chunks": 0,
            "seconds": 0.0,
        }

    _check_cancelled(row, callback, f"Task {_task_id(row)} cancelled before processing documents.")
    subgraphs, build_failed_docs = await _stage_build_subgraphs(
        row,
        tenant_id,
        kb_id,
        doc_ids,
        all_doc_chunks,
        language,
        kb_parser_config,
        chat_model,
        embedding_model,
        run_context,
        callback,
        max_parallel_docs=max_parallel_docs,
    )
    failed_docs.extend(build_failed_docs)
    _check_cancelled(row, callback, f"Task {_task_id(row)} cancelled after document processing.")

    ok_docs, final_graph, resolution_pending, community_pending = await _stage_merge_subgraphs(
        row,
        tenant_id,
        kb_id,
        doc_ids,
        subgraphs,
        embedding_model,
        run_context,
        callback,
        with_resolution=with_resolution,
        with_community=with_community,
    )

    if not ok_docs and not resolution_pending and not community_pending:
        callback(msg=f"[GraphRAG] kb:{kb_id} no subgraphs to merge and no phases pending, end.")
        now = asyncio.get_running_loop().time()
        return {
            "ok_docs": [],
            "failed_docs": failed_docs,
            "total_docs": len(doc_ids),
            "total_chunks": total_chunks,
            "seconds": now - start,
        }

    if not with_resolution and not with_community:
        now = asyncio.get_running_loop().time()
        callback(msg=f"[GraphRAG] KB merge done in {now - start:.2f}s. ok={len(ok_docs)} / total={len(doc_ids)}")
        return {
            "ok_docs": ok_docs,
            "failed_docs": failed_docs,
            "total_docs": len(doc_ids),
            "total_chunks": total_chunks,
            "seconds": now - start,
        }

    if not resolution_pending and not community_pending:
        now = asyncio.get_running_loop().time()
        callback(msg=f"[GraphRAG] kb:{kb_id} all requested phases already complete; nothing to do.")
        return {
            "ok_docs": ok_docs,
            "failed_docs": failed_docs,
            "total_docs": len(doc_ids),
            "total_chunks": total_chunks,
            "seconds": now - start,
        }

    final_graph = await _stage_resolution_and_community(
        row,
        tenant_id,
        kb_id,
        subgraphs,
        final_graph,
        chat_model,
        embedding_model,
        run_context,
        callback,
        with_resolution=with_resolution,
        with_community=with_community,
        resolution_pending=resolution_pending,
        community_pending=community_pending,
    )
    if final_graph is None and (resolution_pending or community_pending):
        now = asyncio.get_running_loop().time()
        return {
            "ok_docs": ok_docs,
            "failed_docs": failed_docs,
            "total_docs": len(doc_ids),
            "total_chunks": total_chunks,
            "seconds": now - start,
        }

    now = asyncio.get_running_loop().time()
    callback(
        msg=(
            f"[GraphRAG] GraphRAG for KB {kb_id} done in {now - start:.2f} seconds. "
            f"ok={len(ok_docs)} failed={len(failed_docs)} total_docs={len(doc_ids)} total_chunks={total_chunks}"
        )
    )
    return {
        "ok_docs": ok_docs,
        "failed_docs": failed_docs,
        "total_docs": len(doc_ids),
        "total_chunks": total_chunks,
        "seconds": now - start,
    }


async def generate_subgraph(
    extractor: Extractor,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    chunks: list[str],
    language,
    entity_types,
    llm_bdl,
    embed_bdl,
    callback,
    run_context: GraphRAGRunContext,
    task_id: str = "",
):
    if task_id and has_canceled(task_id):
        callback(msg=f"Task {task_id} cancelled during subgraph generation for doc {doc_id}.")
        raise TaskCanceledException(f"Task {task_id} was cancelled")

    contains = await does_graph_contains(tenant_id, kb_id, doc_id, run_context)
    if contains:
        callback(msg=f"Graph already contains {doc_id}")
        return None

    start = asyncio.get_running_loop().time()
    ext = extractor(llm_bdl, language=language, entity_types=entity_types)
    ents, rels = await ext(doc_id, chunks, callback, task_id=task_id)
    subgraph = nx.Graph()

    for ent in ents:
        if task_id and has_canceled(task_id):
            callback(msg=f"Task {task_id} cancelled during entity processing for doc {doc_id}.")
            raise TaskCanceledException(f"Task {task_id} was cancelled")

        assert "description" in ent, f"entity {ent} does not have description"
        ent["source_id"] = [doc_id]
        subgraph.add_node(ent["entity_name"], **ent)

    ignored_rels = 0
    for rel in rels:
        if task_id and has_canceled(task_id):
            callback(msg=f"Task {task_id} cancelled during relationship processing for doc {doc_id}.")
            raise TaskCanceledException(f"Task {task_id} was cancelled")

        assert "description" in rel, f"relation {rel} does not have description"
        if not subgraph.has_node(rel["src_id"]) or not subgraph.has_node(rel["tgt_id"]):
            ignored_rels += 1
            continue
        rel["source_id"] = [doc_id]
        subgraph.add_edge(rel["src_id"], rel["tgt_id"], **rel)

    if ignored_rels:
        callback(msg=f"ignored {ignored_rels} relations due to missing entities.")
    tidy_graph(subgraph, callback, check_attribute=False)

    subgraph.graph["source_id"] = [doc_id]
    chunk = {
        "content_with_weight": json.dumps(nx.node_link_data(subgraph, edges="edges"), ensure_ascii=False),
        "knowledge_graph_kwd": run_context.subgraph_kwd,
        "kb_id": kb_id,
        "source_id": [doc_id],
        "available_int": run_context.available_int,
        run_context.removed_kwd_field: run_context.removed_active_value,
    }
    cid = chunk_id(chunk)

    old_checkpoint_ids: list[str] = []
    try:
        existing_res = await _retry_doc_store_step(
            f"enumerate subgraph checkpoints for doc {doc_id}",
            callback,
            lambda: thread_pool_exec(
                settings.docStoreConn.search,
                ["id"],
                [],
                {"knowledge_graph_kwd": [run_context.subgraph_kwd], "source_id": [doc_id]},
                [],
                OrderByExpr(),
                0,
                run_context.stale_checkpoint_search_limit,
                search.index_name(tenant_id),
                [kb_id],
            ),
            run_context,
        )
        existing_fields = settings.docStoreConn.get_fields(existing_res, ["id"])
        old_checkpoint_ids = list(existing_fields.keys())
    except Exception:
        logging.exception("Failed to enumerate old subgraph checkpoints for doc %s; preserving old checkpoints.", doc_id)

    if cid not in old_checkpoint_ids:
        await _retry_doc_store_step(
            f"insert subgraph checkpoint for doc {doc_id}",
            callback,
            lambda: thread_pool_exec(
                settings.docStoreConn.insert,
                [{"id": cid, **chunk}],
                search.index_name(tenant_id),
                kb_id,
            ),
            run_context,
        )
    else:
        callback(msg=f"[GraphRAG] doc:{doc_id} subgraph checkpoint already current.")

    stale_checkpoint_ids = [old_id for old_id in old_checkpoint_ids if old_id != cid]
    if stale_checkpoint_ids:
        try:
            await _retry_doc_store_step(
                f"prune stale subgraph checkpoints for doc {doc_id}",
                callback,
                lambda: thread_pool_exec(
                    settings.docStoreConn.delete,
                    {"knowledge_graph_kwd": [run_context.subgraph_kwd], "id": stale_checkpoint_ids},
                    search.index_name(tenant_id),
                    kb_id,
                ),
                run_context,
            )
        except Exception:
            logging.exception("Failed to prune stale subgraph checkpoints for doc %s", doc_id)

    now = asyncio.get_running_loop().time()
    callback(msg=f"generated subgraph for doc {doc_id} in {now - start:.2f} seconds.")
    return subgraph


async def merge_subgraph(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    subgraph: nx.Graph,
    embedding_model,
    callback,
):
    start = asyncio.get_running_loop().time()
    change = GraphChange()
    old_graph = await get_graph(tenant_id, kb_id, subgraph.graph["source_id"])
    if old_graph is not None:
        logging.info("Merge with an exiting graph...................")
        tidy_graph(old_graph, callback)
        new_graph = graph_merge(old_graph, subgraph, change)
    else:
        new_graph = subgraph
        change.added_updated_nodes = set(new_graph.nodes())
        change.added_updated_edges = set(new_graph.edges())

    pr = nx.pagerank(new_graph)
    for node_name, pagerank in pr.items():
        new_graph.nodes[node_name]["pagerank"] = pagerank

    await set_graph(tenant_id, kb_id, embedding_model, new_graph, change, callback)
    now = asyncio.get_running_loop().time()
    callback(msg=f"merging subgraph for doc {doc_id} into the global graph done in {now - start:.2f} seconds.")
    return new_graph


async def resolve_entities(
    graph,
    subgraph_nodes: set[str],
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    llm_bdl,
    embed_bdl,
    callback,
    task_id: str = "",
):
    if task_id and has_canceled(task_id):
        callback(msg=f"Task {task_id} cancelled during entity resolution.")
        raise TaskCanceledException(f"Task {task_id} was cancelled")

    start = asyncio.get_running_loop().time()
    er = EntityResolution(llm_bdl)
    reso = await er(graph, subgraph_nodes, callback=callback, task_id=task_id)
    graph = reso.graph
    change = reso.change
    callback(msg=f"Graph resolution removed {len(change.removed_nodes)} nodes and {len(change.removed_edges)} edges.")
    callback(msg="Graph resolution updated pagerank.")

    if task_id and has_canceled(task_id):
        callback(msg=f"Task {task_id} cancelled after entity resolution.")
        raise TaskCanceledException(f"Task {task_id} was cancelled")

    await set_graph(tenant_id, kb_id, embed_bdl, graph, change, callback)
    now = asyncio.get_running_loop().time()
    callback(msg=f"Graph resolution done in {now - start:.2f}s.")


async def extract_community(
    graph,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    llm_bdl,
    embed_bdl,
    callback,
    run_context: GraphRAGRunContext,
    task_id: str = "",
):
    if task_id and has_canceled(task_id):
        callback(msg=f"Task {task_id} cancelled before community extraction.")
        raise TaskCanceledException(f"Task {task_id} was cancelled")

    start = asyncio.get_running_loop().time()
    ext = CommunityReportsExtractor(llm_bdl)
    cr = await ext(graph, callback=callback, task_id=task_id)

    if task_id and has_canceled(task_id):
        callback(msg=f"Task {task_id} cancelled during community extraction.")
        raise TaskCanceledException(f"Task {task_id} was cancelled")

    community_structure = cr.structured_output
    community_reports = cr.output
    doc_ids = graph.graph["source_id"]

    now = asyncio.get_running_loop().time()
    callback(msg=f"Graph extracted {len(cr.structured_output)} communities in {now - start:.2f}s.")
    start = now
    if task_id and has_canceled(task_id):
        callback(msg=f"Task {task_id} cancelled during community indexing.")
        raise TaskCanceledException(f"Task {task_id} was cancelled")

    chunks = []
    for structure, report in zip(community_structure, community_reports):
        obj = {
            "report": report,
            "evidences": "\n".join([finding.get("explanation", "") for finding in structure["findings"]]),
        }
        chunk_payload_for_id = {
            "content_with_weight": f"community_report::{structure['title']}",
            "kb_id": kb_id,
        }
        chunk = {
            "id": chunk_id(chunk_payload_for_id),
            "docnm_kwd": structure["title"],
            "title_tks": rag_tokenizer.tokenize(structure["title"]),
            "content_with_weight": json.dumps(obj, ensure_ascii=False),
            "content_ltks": rag_tokenizer.tokenize(obj["report"] + " " + obj["evidences"]),
            "knowledge_graph_kwd": run_context.community_report_kwd,
            "weight_flt": structure["weight"],
            "entities_kwd": structure["entities"],
            "important_kwd": structure["entities"],
            "kb_id": kb_id,
            "source_id": list(doc_ids),
            "available_int": run_context.available_int,
        }
        chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
        chunks.append(chunk)

    new_ids: set[str] = {chunk["id"] for chunk in chunks}
    old_ids: list[str] = []
    try:
        existing_res = await thread_pool_exec(
            settings.docStoreConn.search,
            ["id"],
            [],
            {"knowledge_graph_kwd": [run_context.community_report_kwd]},
            [],
            OrderByExpr(),
            0,
            run_context.community_report_search_limit,
            search.index_name(tenant_id),
            [kb_id],
        )
        existing_fields = settings.docStoreConn.get_fields(existing_res, ["id"])
        old_ids = list(existing_fields.keys())
    except Exception:
        logging.exception("Failed to enumerate existing community reports for kb %s; preserving old reports and skipping prune.", kb_id)

    await insert_chunks_bounded(chunks, tenant_id, kb_id, callback=callback, label="Insert community reports")

    stale_ids = [old_id for old_id in old_ids if old_id not in new_ids]
    if stale_ids:
        try:
            await thread_pool_exec(
                settings.docStoreConn.delete,
                {"knowledge_graph_kwd": [run_context.community_report_kwd], "id": stale_ids},
                search.index_name(tenant_id),
                kb_id,
            )
        except Exception:
            logging.exception("Failed to prune %d stale community reports for kb %s", len(stale_ids), kb_id)

    if task_id and has_canceled(task_id):
        callback(msg=f"Task {task_id} cancelled after community indexing.")
        raise TaskCanceledException(f"Task {task_id} was cancelled")

    now = asyncio.get_running_loop().time()
    callback(msg=f"Graph indexed {len(cr.structured_output)} communities in {now - start:.2f}s.")
    return community_structure, community_reports
