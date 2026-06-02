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

import networkx as nx

from api.db.services.document_service import DocumentService
from api.db.services.task_service import has_canceled
from common.exceptions import TaskCanceledException
from common.connection_utils import timeout
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
    does_graph_contains,
    get_graph,
    graph_merge,
    insert_chunks_bounded,
    set_graph,
    tidy_graph,
)
from common.misc_utils import thread_pool_exec
from rag.nlp import rag_tokenizer, search
from rag.utils.redis_conn import RedisDistributedLock
from common import settings
from common.doc_store.doc_store_base import OrderByExpr


DEFAULT_GRAPHRAG_BATCH_CHUNK_TOKEN_SIZE = 4096
MIN_GRAPHRAG_BATCH_CHUNK_TOKEN_SIZE = 512
MAX_GRAPHRAG_BATCH_CHUNK_TOKEN_SIZE = 8196
DEFAULT_GRAPHRAG_RETRY_ATTEMPTS = 2
DEFAULT_GRAPHRAG_RETRY_BACKOFF_SECONDS = 2.0
DEFAULT_GRAPHRAG_RETRY_BACKOFF_MAX_SECONDS = 60.0
DEFAULT_GRAPHRAG_BUILD_SUBGRAPH_TIMEOUT_PER_CHUNK_SECONDS = 300
DEFAULT_GRAPHRAG_BUILD_SUBGRAPH_MIN_TIMEOUT_SECONDS = 600
DEFAULT_GRAPHRAG_MERGE_TIMEOUT_SECONDS = 180
DEFAULT_GRAPHRAG_RESOLUTION_TIMEOUT_SECONDS = 1800
DEFAULT_GRAPHRAG_COMMUNITY_TIMEOUT_SECONDS = 1800
DEFAULT_GRAPHRAG_LOCK_ACQUIRE_TIMEOUT_SECONDS = 600


def _bounded_int_config(config: dict, key: str, default: int, minimum: int, maximum: int) -> int:
    value = config.get(key, default)
    if value is None:
        return default
    try:
        value = int(value)
    except (TypeError, ValueError):
        logging.warning("Invalid GraphRAG config %s=%r, using default %s", key, value, default)
        return default
    if value < minimum or value > maximum:
        logging.warning("Invalid GraphRAG config %s=%r, using default %s", key, value, default)
        return default
    return value


def _bounded_float_config(config: dict, key: str, default: float, minimum: float, maximum: float) -> float:
    value = config.get(key, default)
    if value is None:
        return default
    try:
        value = float(value)
    except (TypeError, ValueError):
        logging.warning("Invalid GraphRAG config %s=%r, using default %s", key, value, default)
        return default
    if value < minimum or value > maximum:
        logging.warning("Invalid GraphRAG config %s=%r, using default %s", key, value, default)
        return default
    return value


def _batch_chunk_token_size_config(config: dict, key: str, default: int) -> int:
    return _bounded_int_config(config, key, default, MIN_GRAPHRAG_BATCH_CHUNK_TOKEN_SIZE, MAX_GRAPHRAG_BATCH_CHUNK_TOKEN_SIZE)


def _lock_acquire_timeout_config(config: dict) -> int:
    value = _bounded_int_config(config, "lock_acquire_timeout_seconds", DEFAULT_GRAPHRAG_LOCK_ACQUIRE_TIMEOUT_SECONDS, 0, 86400)
    if value == 0:
        return DEFAULT_GRAPHRAG_LOCK_ACQUIRE_TIMEOUT_SECONDS
    return value


def _select_extractor_type(graphrag_config: dict):
    return graphrag_config.get("method", "light")


def _select_extractor(graphrag_config: dict):
    """Return the extractor class matching ``graphrag_config["method"]``.

    Supported values:
    - ``"general"``  – Microsoft GraphRAG LLM-based extractor (default in
      earlier versions).
    - ``"light"``   – LightRAG-style LLM-based extractor (the default when
      *method* is omitted or unrecognised).
    - ``"ner"``     – NER-based extractor using spaCy (no LLM
      needed for entity / relation extraction itself).
    """
    method = graphrag_config.get("method", "light")
    if method == "general":
        return GeneralKGExt
    if method == "ner":
        return NerKGExt
    return LightKGExt


def _has_cancel_and_exit(task_id: str, message: str, callback=None) -> None:
    if not task_id or not has_canceled(task_id):
        return
    if callback:
        callback(msg=message)
    raise TaskCanceledException(f"Task {task_id} was cancelled")


async def _run_with_retry(
    label: str,
    coro_factory,
    *,
    attempts: int,
    timeout_seconds: int | float,
    backoff_seconds: float,
    backoff_max_seconds: float,
    callback=None,
    task_id: str = "",
):
    attempts = max(1, attempts)
    last_error = None
    for attempt in range(1, attempts + 1):
        _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before {label}.", callback)
        try:
            if timeout_seconds and timeout_seconds > 0:
                return await asyncio.wait_for(coro_factory(), timeout=timeout_seconds)
            return await coro_factory()
        except (TaskCanceledException, asyncio.CancelledError):
            raise
        except asyncio.TimeoutError as e:
            last_error = e
            error_msg = f"timeout after {timeout_seconds}s"
        except Exception as e:
            last_error = e
            error_msg = repr(e)

        if attempt >= attempts:
            if callback:
                callback(msg=f"[GraphRAG] {label} FAILED after {attempt}/{attempts} attempts: {error_msg}")
            raise last_error

        wait = min(backoff_max_seconds, backoff_seconds * (2 ** (attempt - 1)))
        if callback:
            callback(msg=f"[GraphRAG] {label} failed attempt {attempt}/{attempts}: {error_msg}; retrying in {wait:.1f}s")
        logging.warning("GraphRAG %s failed attempt %s/%s: %s", label, attempt, attempts, error_msg)
        if wait > 0:
            await asyncio.sleep(wait)


async def _acquire_lock(lock: RedisDistributedLock, label: str, timeout_seconds: int, callback, task_id: str):
    if timeout_seconds <= 0:
        timeout_seconds = DEFAULT_GRAPHRAG_LOCK_ACQUIRE_TIMEOUT_SECONDS
    deadline = asyncio.get_running_loop().time() + timeout_seconds
    while True:
        _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before acquiring {label}.", callback)
        if lock.acquire():
            return

        remaining_seconds = deadline - asyncio.get_running_loop().time()
        if remaining_seconds <= 0:
            msg = f"[GraphRAG] failed to acquire {label} after {timeout_seconds}s"
            if callback:
                callback(msg=msg)
            raise asyncio.TimeoutError(msg)

        await asyncio.sleep(min(10, remaining_seconds))


async def load_subgraph_from_store(tenant_id: str, kb_id: str, doc_id: str):
    """Load a previously saved subgraph from the doc store.

    Filters directly by source_id (== doc_id) and knowledge_graph_kwd in the
    query so the doc store index does the heavy lifting.  Expects at most one
    matching chunk per doc_id (as written by generate_subgraph).
    Returns a networkx Graph on hit, or None on miss.
    """
    fields = ["content_with_weight", "source_id"]
    condition = {
        "knowledge_graph_kwd": ["subgraph"],
        "removed_kwd": "N",
        "source_id": [doc_id],
    }
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields, [], condition, [], OrderByExpr(),
            0, 1, search.index_name(tenant_id), [kb_id]
        )
        field_map = settings.docStoreConn.get_fields(res, fields)
        for cid, row in field_map.items():
            content = row.get("content_with_weight", "")
            if not content:
                continue
            try:
                data = json.loads(content)
                sg = nx.node_link_graph(data, edges="edges")
                sg.graph["source_id"] = [doc_id]
                logging.info(
                    "Checkpoint hit: subgraph for doc %s (tenant=%s kb=%s) found at chunk %s",
                    doc_id, tenant_id, kb_id, cid,
                )
                return sg
            except Exception:
                logging.exception(
                    "Failed to parse subgraph JSON for doc %s chunk %s", doc_id, cid
                )
    except Exception:
        logging.exception("Failed to load subgraph from store for doc %s", doc_id)
        return None
    logging.info(
        "Checkpoint miss: no subgraph for doc %s (tenant=%s kb=%s)",
        doc_id, tenant_id, kb_id,
    )
    return None


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
    max_parallel_docs: int = 4,
) -> dict:
    tenant_id, kb_id = row["tenant_id"], row["kb_id"]
    task_id = row["id"]
    start = asyncio.get_running_loop().time()
    fields_for_chunks = ["content_with_weight", "doc_id"]
    graphrag_config = kb_parser_config.get("graphrag", {})
    batch_chunk_token_size = _batch_chunk_token_size_config(graphrag_config, "batch_chunk_token_size", DEFAULT_GRAPHRAG_BATCH_CHUNK_TOKEN_SIZE)
    retry_attempts = _bounded_int_config(graphrag_config, "retry_attempts", DEFAULT_GRAPHRAG_RETRY_ATTEMPTS, 1, 10)
    retry_backoff_seconds = _bounded_float_config(graphrag_config, "retry_backoff_seconds", DEFAULT_GRAPHRAG_RETRY_BACKOFF_SECONDS, 0.0, 600.0)
    retry_backoff_max_seconds = _bounded_float_config(graphrag_config, "retry_backoff_max_seconds", DEFAULT_GRAPHRAG_RETRY_BACKOFF_MAX_SECONDS, 0.0, 3600.0)
    build_subgraph_retry_attempts = _bounded_int_config(graphrag_config, "build_subgraph_retry_attempts", retry_attempts, 1, 10)
    merge_retry_attempts = _bounded_int_config(graphrag_config, "merge_retry_attempts", retry_attempts, 1, 10)
    resolution_retry_attempts = _bounded_int_config(graphrag_config, "resolution_retry_attempts", retry_attempts, 1, 10)
    community_retry_attempts = _bounded_int_config(graphrag_config, "community_retry_attempts", retry_attempts, 1, 10)
    build_subgraph_timeout_per_chunk_seconds = _bounded_int_config(
        graphrag_config,
        "build_subgraph_timeout_per_chunk_seconds",
        DEFAULT_GRAPHRAG_BUILD_SUBGRAPH_TIMEOUT_PER_CHUNK_SECONDS,
        1,
        86400,
    )
    build_subgraph_min_timeout_seconds = _bounded_int_config(
        graphrag_config,
        "build_subgraph_min_timeout_seconds",
        DEFAULT_GRAPHRAG_BUILD_SUBGRAPH_MIN_TIMEOUT_SECONDS,
        1,
        86400,
    )
    merge_timeout_seconds = _bounded_int_config(graphrag_config, "merge_timeout_seconds", DEFAULT_GRAPHRAG_MERGE_TIMEOUT_SECONDS, 0, 86400)
    resolution_timeout_seconds = _bounded_int_config(graphrag_config, "resolution_timeout_seconds", DEFAULT_GRAPHRAG_RESOLUTION_TIMEOUT_SECONDS, 0, 86400)
    community_timeout_seconds = _bounded_int_config(graphrag_config, "community_timeout_seconds", DEFAULT_GRAPHRAG_COMMUNITY_TIMEOUT_SECONDS, 0, 86400)
    lock_acquire_timeout_seconds = _lock_acquire_timeout_config(graphrag_config)

    if not doc_ids:
        logging.info(f"Fetching all docs for {kb_id}")
        docs, _ = DocumentService.get_by_kb_id(
            kb_id=kb_id,
            page_number=0,
            items_per_page=0,
            orderby="create_time",
            desc=False,
            keywords="",
            run_status=[],
            types=[],
            suffix=[],
        )
        doc_ids = [doc["id"] for doc in docs]

    doc_ids = list(dict.fromkeys(doc_ids))
    if not doc_ids:
        callback(msg=f"[GraphRAG] dataset:{kb_id} has no processable doc_id.")
        return {"ok_docs": [], "failed_docs": [], "total_docs": 0, "total_chunks": 0, "seconds": 0.0}
    else:
        callback(msg=f"[GraphRAG] dataset:{kb_id} has {len(doc_ids)} documents to process.")

    def load_doc_chunks(doc_id: str) -> list[str]:
        from common.token_utils import num_tokens_from_string

        chunks = []
        current_chunk = ""

        raw_chunks = list(settings.retriever.chunk_list(
            doc_id,
            tenant_id,
            [kb_id],
            fields=fields_for_chunks,
            sort_by_position=True,
            retrieve_all=True
        ))

        callback(msg=f"[GraphRAG] chunk_list returned {len(raw_chunks)} raw chunks for doc:{doc_id}")

        contents = [content for chunk in raw_chunks if (content := chunk.get("content_with_weight", ""))
]
        # For NER-based extractionm, no need to batch extract entity and relation
        if _select_extractor_type(graphrag_config) == "ner":
            return contents

        for content in contents:
            if num_tokens_from_string(current_chunk + content) < batch_chunk_token_size:
                current_chunk += content
            else:
                if current_chunk:
                    chunks.append(current_chunk)
                current_chunk = content

        if current_chunk:
            chunks.append(current_chunk)

        callback(msg=f"[GraphRAG] chunk_list combine {len(raw_chunks)} raw chunks to {len(chunks)} chunks for LLM extraction for doc:{doc_id}")
        return chunks

    total_chunks = 0

    semaphore = asyncio.Semaphore(max_parallel_docs)

    subgraphs: dict[str, object] = {}
    failed_docs: list[tuple[str, str]] = []  # (doc_id, error)

    async def build_one(doc_id: str):
        nonlocal total_chunks

        _has_cancel_and_exit(task_id, f"Task {task_id} cancelled, stopping execution.", callback)

        kg_extractor = _select_extractor(graphrag_config)

        async with semaphore:
            # CHECKPOINT: bounded by semaphore so doc-store lookups respect max_parallel_docs
            _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before loading checkpoint for doc {doc_id}.", callback)
            existing_sg = await load_subgraph_from_store(tenant_id, kb_id, doc_id)
            if existing_sg:
                subgraphs[doc_id] = existing_sg
                callback(msg=f"[GraphRAG] doc:{doc_id} subgraph found in store, skipping LLM extraction.")
                return
            try:
                _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before loading chunks for doc {doc_id}.", callback)
                chunks = load_doc_chunks(doc_id)
                total_chunks += len(chunks)
                if not chunks:
                    callback(msg=f"[GraphRAG] doc:{doc_id} has no available chunks, skip generation.")
                    return

                build_subgraph_timeout_seconds = max(
                    build_subgraph_min_timeout_seconds,
                    len(chunks) * build_subgraph_timeout_per_chunk_seconds,
                )
                label = f"build_subgraph doc:{doc_id}"
                msg = f"[GraphRAG] {label}"
                callback(msg=f"{msg} start (chunks={len(chunks)}, timeout={build_subgraph_timeout_seconds}s, attempts={build_subgraph_retry_attempts})")

                _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before subgraph generation for doc {doc_id}.", callback)
                try:
                    async def build_subgraph_attempt():
                        checkpoint_sg = await load_subgraph_from_store(tenant_id, kb_id, doc_id)
                        if checkpoint_sg:
                            callback(msg=f"[GraphRAG] doc:{doc_id} subgraph found in store during retry, skipping LLM extraction.")
                            return checkpoint_sg
                        return await generate_subgraph(
                            kg_extractor,
                            tenant_id,
                            kb_id,
                            doc_id,
                            chunks,
                            language,
                            kb_parser_config.get("graphrag", {}).get("entity_types", []),
                            chat_model,
                            embedding_model,
                            callback,
                            task_id=task_id,
                        )

                    sg = await _run_with_retry(
                        label,
                        build_subgraph_attempt,
                        attempts=build_subgraph_retry_attempts,
                        timeout_seconds=build_subgraph_timeout_seconds,
                        backoff_seconds=retry_backoff_seconds,
                        backoff_max_seconds=retry_backoff_max_seconds,
                        callback=callback,
                        task_id=task_id,
                    )
                except asyncio.TimeoutError:
                    failed_docs.append((doc_id, f"timeout after {build_subgraph_timeout_seconds}s"))
                    callback(msg=f"{msg} FAILED: timeout after {build_subgraph_timeout_seconds}s")
                    return
                if sg:
                    subgraphs[doc_id] = sg
                    callback(msg=f"{msg} done")
                else:
                    failed_docs.append((doc_id, "subgraph is empty"))
                    callback(msg=f"{msg} empty")
            except TaskCanceledException as canceled:
                callback(msg=f"[GraphRAG] build_subgraph doc:{doc_id} FAILED: {canceled}")
                raise
            except Exception as e:
                failed_docs.append((doc_id, repr(e)))
                callback(msg=f"[GraphRAG] build_subgraph doc:{doc_id} FAILED: {e!r}")

    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before processing documents.", callback)

    tasks = [asyncio.create_task(build_one(doc_id)) for doc_id in doc_ids]
    try:
        await asyncio.gather(*tasks, return_exceptions=False)
    except Exception as e:
        logging.error(f"Error in asyncio.gather: {e}")
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise

    if total_chunks == 0 and not subgraphs:
        callback(msg=f"[GraphRAG] dataset:{kb_id} has no available chunks in all documents, skip.")
        return {"ok_docs": [], "failed_docs": [(doc_id, "no available chunks") for doc_id in doc_ids], "total_docs": len(doc_ids), "total_chunks": 0, "seconds": 0.0}

    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled after document processing.", callback)

    ok_docs = [d for d in doc_ids if d in subgraphs]
    final_graph = None

    # Determine whether the resolution/community phases still need to run on
    # this KB. Markers from a prior task let us skip already-completed phases
    # even when no new docs are merged this round (the resume path).
    resolution_pending = with_resolution and not has_phase_marker(kb_id, PHASE_RESOLUTION)
    community_pending = with_community and not has_phase_marker(kb_id, PHASE_COMMUNITY)

    if not ok_docs and not resolution_pending and not community_pending:
        callback(msg=f"[GraphRAG] dataset:{kb_id} no subgraphs to merge and no phases pending, end.")
        now = asyncio.get_running_loop().time()
        return {"ok_docs": [], "failed_docs": failed_docs, "total_docs": len(doc_ids), "total_chunks": total_chunks, "seconds": now - start}

    kb_lock = RedisDistributedLock(f"graphrag_task_{kb_id}", lock_value=f"batch_merge:{task_id}", timeout=1200)
    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before acquiring merge lock.", callback)
    await _acquire_lock(kb_lock, "merge lock", lock_acquire_timeout_seconds, callback, task_id)
    callback(msg=f"[GraphRAG] dataset:{kb_id} merge lock acquired")

    try:
        _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before merging subgraphs.", callback)

        union_nodes: set = set()

        for doc_id in ok_docs:
            _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before merging subgraph for doc {doc_id}.", callback)
            sg = subgraphs[doc_id]
            union_nodes.update(set(sg.nodes()))

            try:
                async def merge_subgraph_attempt():
                    current_graph = await get_graph(tenant_id, kb_id)
                    if current_graph and doc_id in current_graph.graph.get("source_id", []):
                        callback(msg=f"[GraphRAG] merge_subgraph doc:{doc_id} already merged, skipping retry.")
                        return current_graph
                    return await merge_subgraph(
                        tenant_id,
                        kb_id,
                        doc_id,
                        sg,
                        embedding_model,
                        callback,
                    )

                new_graph = await _run_with_retry(
                    f"merge_subgraph doc:{doc_id}",
                    merge_subgraph_attempt,
                    attempts=merge_retry_attempts,
                    timeout_seconds=merge_timeout_seconds,
                    backoff_seconds=retry_backoff_seconds,
                    backoff_max_seconds=retry_backoff_max_seconds,
                    callback=callback,
                    task_id=task_id,
                )
            except TaskCanceledException:
                raise
            except Exception as e:
                failed_docs.append((doc_id, f"merge failed: {e!r}"))
                callback(msg=f"[GraphRAG] merge_subgraph doc:{doc_id} FAILED: {e!r}")
                raise
            if new_graph is not None:
                final_graph = new_graph

        if ok_docs and final_graph is None:
            callback(msg=f"[GraphRAG] dataset:{kb_id} merge finished (no in-memory graph returned).")
        elif ok_docs:
            callback(msg=f"[GraphRAG] dataset:{kb_id} merge finished, graph ready.")
            # New content was merged into the global graph; any prior
            # resolution/community results are now stale and must be redone
            # on this or a future run. Clear phase markers accordingly.
            clear_phase_markers(kb_id)
            resolution_pending = with_resolution
            community_pending = with_community
            callback(msg=f"[GraphRAG] dataset:{kb_id} cleared phase markers after merge.")
    finally:
        kb_lock.release()

    if not with_resolution and not with_community:
        now = asyncio.get_running_loop().time()
        callback(msg=f"[GraphRAG] KB merge done in {now - start:.2f}s. ok={len(ok_docs)} / total={len(doc_ids)}")
        return {"ok_docs": ok_docs, "failed_docs": failed_docs, "total_docs": len(doc_ids), "total_chunks": total_chunks, "seconds": now - start}

    if not resolution_pending and not community_pending:
        now = asyncio.get_running_loop().time()
        callback(msg=f"[GraphRAG] dataset:{kb_id} all requested phases already complete; nothing to do.")
        return {"ok_docs": ok_docs, "failed_docs": failed_docs, "total_docs": len(doc_ids), "total_chunks": total_chunks, "seconds": now - start}

    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before resolution/community extraction.", callback)

    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before acquiring post-merge lock.", callback)
    await _acquire_lock(kb_lock, "post-merge lock", lock_acquire_timeout_seconds, callback, task_id)
    callback(msg=f"[GraphRAG] dataset:{kb_id} post-merge lock acquired for resolution/community")

    try:
        _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before resolution/community extraction.", callback)

        # Resume path: no docs were merged this round but pending phases
        # require the previously-persisted graph. Load it from the doc store.
        if final_graph is None:
            final_graph = await get_graph(tenant_id, kb_id)
            if final_graph is None:
                callback(msg=f"[GraphRAG] dataset:{kb_id} no persisted graph found; cannot run resolution/community.")
                now = asyncio.get_running_loop().time()
                return {"ok_docs": ok_docs, "failed_docs": failed_docs, "total_docs": len(doc_ids), "total_chunks": total_chunks, "seconds": now - start}
            callback(msg=f"[GraphRAG] dataset:{kb_id} loaded persisted graph for resume.")

        subgraph_nodes = set()
        for sg in subgraphs.values():
            subgraph_nodes.update(set(sg.nodes()))
        # On a pure-resume run (no new docs) the union of "newly added" nodes
        # is empty, but resolution still needs *some* anchor set. Fall back to
        # all graph nodes so candidate pairing actually finds something.
        if not subgraph_nodes:
            subgraph_nodes = set(final_graph.nodes())

        if resolution_pending:
            _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before entity resolution.", callback)

            async def run_resolution_attempt():
                graph_for_resolution = final_graph.copy()
                await resolve_entities(
                    graph_for_resolution,
                    subgraph_nodes,
                    tenant_id,
                    kb_id,
                    None,
                    chat_model,
                    embedding_model,
                    callback,
                    task_id=task_id,
                )
                return graph_for_resolution

            final_graph = await _run_with_retry(
                "entity resolution",
                run_resolution_attempt,
                attempts=resolution_retry_attempts,
                timeout_seconds=resolution_timeout_seconds,
                backoff_seconds=retry_backoff_seconds,
                backoff_max_seconds=retry_backoff_max_seconds,
                callback=callback,
                task_id=task_id,
            )
            set_phase_marker(kb_id, PHASE_RESOLUTION)
        elif with_resolution:
            callback(msg=f"[GraphRAG] dataset:{kb_id} resolution already completed previously, skipping.")

        if community_pending:
            _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before community extraction.", callback)

            async def run_community_attempt():
                await extract_community(
                    final_graph.copy(),
                    tenant_id,
                    kb_id,
                    None,
                    chat_model,
                    embedding_model,
                    callback,
                    task_id=task_id,
                )

            await _run_with_retry(
                "community extraction",
                run_community_attempt,
                attempts=community_retry_attempts,
                timeout_seconds=community_timeout_seconds,
                backoff_seconds=retry_backoff_seconds,
                backoff_max_seconds=retry_backoff_max_seconds,
                callback=callback,
                task_id=task_id,
            )
            set_phase_marker(kb_id, PHASE_COMMUNITY)
        elif with_community:
            callback(msg=f"[GraphRAG] dataset:{kb_id} community detection already completed previously, skipping.")
    finally:
        kb_lock.release()

    now = asyncio.get_running_loop().time()
    callback(msg=f"[GraphRAG] GraphRAG for KB {kb_id} done in {now - start:.2f} seconds. ok={len(ok_docs)} failed={len(failed_docs)} total_docs={len(doc_ids)} total_chunks={total_chunks}")
    return {
        "ok_docs": ok_docs,
        "failed_docs": failed_docs,  # [(doc_id, error), ...]
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
    task_id: str = "",
):
    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled during subgraph generation for doc {doc_id}.", callback)

    contains = await does_graph_contains(tenant_id, kb_id, doc_id)
    if contains:
        callback(msg=f"Graph already contains {doc_id}")
        return None
    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before extracting entities for doc {doc_id}.", callback)
    start = asyncio.get_running_loop().time()
    ext = extractor(
        llm_bdl,
        language=language,
        entity_types=entity_types,
    )
    ents, rels = await ext(doc_id, chunks, callback, task_id=task_id)
    subgraph = nx.Graph()

    for ent in ents:
        _has_cancel_and_exit(task_id, f"Task {task_id} cancelled during entity processing for doc {doc_id}.", callback)

        assert "description" in ent, f"entity {ent} does not have description"
        ent["source_id"] = [doc_id]
        subgraph.add_node(ent["entity_name"], **ent)

    ignored_rels = 0
    for rel in rels:
        _has_cancel_and_exit(task_id, f"Task {task_id} cancelled during relationship processing for doc {doc_id}.", callback)

        assert "description" in rel, f"relation {rel} does not have description"
        if not subgraph.has_node(rel["src_id"]) or not subgraph.has_node(rel["tgt_id"]):
            ignored_rels += 1
            continue
        rel["source_id"] = [doc_id]
        subgraph.add_edge(
            rel["src_id"],
            rel["tgt_id"],
            **rel,
        )
    if ignored_rels:
        callback(msg=f"ignored {ignored_rels} relations due to missing entities.")
    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before tidying subgraph for doc {doc_id}.", callback)
    tidy_graph(subgraph, callback, check_attribute=False)

    subgraph.graph["source_id"] = [doc_id]
    chunk = {
        "content_with_weight": json.dumps(nx.node_link_data(subgraph, edges="edges"), ensure_ascii=False),
        "knowledge_graph_kwd": "subgraph",
        "kb_id": kb_id,
        "source_id": [doc_id],
        "available_int": 0,
        "removed_kwd": "N",
    }
    cid = chunk_id(chunk)
    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before saving subgraph for doc {doc_id}.", callback)
    await thread_pool_exec(settings.docStoreConn.delete,{"knowledge_graph_kwd": "subgraph", "source_id": doc_id},search.index_name(tenant_id),kb_id,)
    await thread_pool_exec(settings.docStoreConn.insert,[{"id": cid, **chunk}],search.index_name(tenant_id),kb_id,)
    now = asyncio.get_running_loop().time()
    callback(msg=f"generated subgraph for doc {doc_id} in {now - start:.2f} seconds.")
    return subgraph


@timeout(60 * 3)
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


@timeout(60 * 30, 1)
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
    # Check if task has been canceled before resolution
    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled during entity resolution.", callback)

    start = asyncio.get_running_loop().time()
    er = EntityResolution(
        llm_bdl,
    )
    reso = await er(graph, subgraph_nodes, callback=callback, task_id=task_id)
    graph = reso.graph
    change = reso.change
    callback(msg=f"Graph resolution removed {len(change.removed_nodes)} nodes and {len(change.removed_edges)} edges.")
    callback(msg="Graph resolution updated pagerank.")

    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled after entity resolution.", callback)

    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before saving resolved graph.", callback)
    await set_graph(tenant_id, kb_id, embed_bdl, graph, change, callback)
    now = asyncio.get_running_loop().time()
    callback(msg=f"Graph resolution done in {now - start:.2f}s.")


@timeout(60 * 30, 1)
async def extract_community(
    graph,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    llm_bdl,
    embed_bdl,
    callback,
    task_id: str = "",
):
    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled before community extraction.", callback)

    start = asyncio.get_running_loop().time()
    ext = CommunityReportsExtractor(
        llm_bdl,
    )
    cr = await ext(graph, callback=callback, task_id=task_id)

    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled during community extraction.", callback)

    community_structure = cr.structured_output
    community_reports = cr.output
    doc_ids = graph.graph["source_id"]

    now = asyncio.get_running_loop().time()
    callback(msg=f"Graph extracted {len(cr.structured_output)} communities in {now - start:.2f}s.")
    start = now
    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled during community indexing.", callback)

    chunks = []
    for stru, rep in zip(community_structure, community_reports):
        obj = {
            "report": rep,
            "evidences": "\n".join([f.get("explanation", "") for f in stru["findings"]]),
        }
        # Deterministic id derived from (kb_id, community title) so reruns of
        # extract_community produce stable ids.  Combined with insert-then-
        # prune below, this means a crash mid-insert leaves the prior set of
        # community reports intact -- never the partial-delete state the old
        # delete-then-insert order produced.
        chunk_payload_for_id = {
            "content_with_weight": f"community_report::{stru['title']}",
            "kb_id": kb_id,
        }
        chunk = {
            "id": chunk_id(chunk_payload_for_id),
            "docnm_kwd": stru["title"],
            "title_tks": rag_tokenizer.tokenize(stru["title"]),
            "content_with_weight": json.dumps(obj, ensure_ascii=False),
            "content_ltks": rag_tokenizer.tokenize(obj["report"] + " " + obj["evidences"]),
            "knowledge_graph_kwd": "community_report",
            "weight_flt": stru["weight"],
            "entities_kwd": stru["entities"],
            "important_kwd": stru["entities"],
            "kb_id": kb_id,
            "source_id": list(doc_ids),
            "available_int": 0,
        }
        chunk["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(chunk["content_ltks"])
        chunks.append(chunk)

    new_ids: set[str] = {c["id"] for c in chunks}

    # Snapshot existing community_report ids BEFORE inserting so we can
    # delete exactly the stale set afterwards.  If the search fails we fall
    # back to the prior delete-everything-then-insert behaviour rather than
    # leaving an inconsistent mix.
    old_ids: list[str] = []
    try:
        existing_res = await thread_pool_exec(
            settings.docStoreConn.search,
            ["id"], [], {"knowledge_graph_kwd": ["community_report"]}, [], OrderByExpr(),
            0, 10000, search.index_name(tenant_id), [kb_id],
        )
        existing_fields = settings.docStoreConn.get_fields(existing_res, ["id"])
        old_ids = list(existing_fields.keys())
    except Exception:
        logging.exception("Failed to enumerate existing community reports for kb %s; falling back to delete-then-insert.", kb_id)
        await thread_pool_exec(settings.docStoreConn.delete, {"knowledge_graph_kwd": "community_report", "kb_id": kb_id}, search.index_name(tenant_id), kb_id)
        old_ids = []

    await insert_chunks_bounded(chunks, tenant_id, kb_id, callback=callback, label="Insert community reports")

    # Now that all new reports are persisted, prune stale rows.  Anything in
    # old_ids that is not also in new_ids is no longer current (community
    # composition changed across runs).  A failure here just leaves stale
    # rows; the new rows are already in place.
    stale_ids = [i for i in old_ids if i not in new_ids]
    if stale_ids:
        try:
            await thread_pool_exec(
                settings.docStoreConn.delete,
                {"knowledge_graph_kwd": ["community_report"], "id": stale_ids},
                search.index_name(tenant_id),
                kb_id,
            )
        except Exception:
            logging.exception("Failed to prune %d stale community reports for kb %s", len(stale_ids), kb_id)

    _has_cancel_and_exit(task_id, f"Task {task_id} cancelled after community indexing.", callback)

    now = asyncio.get_running_loop().time()
    callback(msg=f"Graph indexed {len(cr.structured_output)} communities in {now - start:.2f}s.")
    return community_structure, community_reports
