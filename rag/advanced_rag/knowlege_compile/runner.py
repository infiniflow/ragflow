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
"""Handler-free core for document-scoped knowledge (structure) compilation.

Extracted from
``rag/svr/task_executor_refactor/chunk_post_processor.py`` so the same
template resolution, batching, accumulate / merge-flush and synthesis logic
can be driven from two places:

* the chunking **task executor**, which streams a document's chunks out of
  the doc store (``run_document_structure_compile``), and
* the ``rag.flow`` **Compiler** component, which receives chunks in-memory
  from an upstream pipeline node.

Only the non-``tree`` template kinds are handled here. ``tree`` templates run
RAPTOR over the whole document and are still driven from the task executor
(``run_tree_templates``), which owns the doc-store reload + ``RaptorService``.
"""

from __future__ import annotations

import asyncio
import logging
from typing import AsyncIterator, Callable

from api.db.services.compilation_template_service import CompilationTemplateService
from api.db.services.compilation_template_group_service import (
    CompilationTemplateGroupService,
)
from api.db.services.llm_service import LLMBundle
from common.exceptions import TaskCanceledException
from rag.advanced_rag.knowlege_compile.structure import (
    LLMCallPool,
    MERGE_SCOPE_DATASET,
    MERGE_SCOPE_DOC,
    compile_structure_from_text,
    cleanup_timeline_isolated_entities,
    merge_compiled_structures,
    rebuild_dataset_structure_graph_json,
    rebuild_structure_graph_json,
)


# ----- tunables ------------------------------------------------------
# Bound how many source chunks are handed to a single
# ``compile_structure_from_text`` invocation.
DOC_STRUCTURE_COMPILE_BATCH_CHUNKS = 4

# Bound the number of batch/template extraction calls in flight. Results are
# committed in submission order so accumulator updates and merge flushes stay
# deterministic while the LLM calls run concurrently.
DOC_STRUCTURE_COMPILE_MAX_IN_FLIGHT = 15

# Total task-scoped Chat LLM capacity shared by compile, chain validation and
# merge decisions. A request waits in the priority queue when all slots are busy.
DOC_STRUCTURE_LLM_POOL_SIZE = 20

# Bound how many compiled ES-ready docs may accumulate before we flush
# them through ``merge_compiled_structures``. The merger does pairwise
# cosine + LLM duplicate-judging, so it's the more expensive step; we
# cap the per-flush set to keep the local-dedup buckets tractable.
DOC_STRUCTURE_MERGE_MAX_DOCS = 512

# Hard wall on the chain-validator LLM correction step.
STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S = 120.0


# ----- template resolution -------------------------------------------


def resolve_template_ids_from_groups(group_ids, tenant_id: str) -> list[str]:
    """Resolve an ordered, de-duplicated list of compilation-template ids
    from a list of template-*group* ids.

    Mirrors ``_parser_config_compilation_template_ids`` but takes the group
    ids directly (the ``rag.flow`` Compiler carries them as a component
    parameter rather than inside ``parser_config``).
    """
    if isinstance(group_ids, str):
        group_ids = [group_ids]
    template_ids: list[str] = []
    seen: set[str] = set()
    for group_id in group_ids or []:
        if not isinstance(group_id, str) or not group_id.strip():
            continue
        for template_id in CompilationTemplateGroupService.resolve_template_ids(
            group_id.strip(),
            tenant_id,
        ):
            if template_id in seen:
                continue
            seen.add(template_id)
            template_ids.append(template_id)
    return template_ids


def load_active_templates(template_ids, tenant_id: str) -> list[tuple[str, dict]]:
    """Load each template's saved config and keep only the ones that drive a
    real, non-``artifacts`` structure compilation.

    Returns ``[(template_id, parser_cfg), ...]`` — templates that are missing,
    have an invalid config, or resolve to no/``artifacts`` kind are dropped
    (with a warning for the missing/invalid cases).
    """
    from api.apps.restful_apis.chunk_api import _compilation_template_kind

    active_templates: list[tuple[str, dict]] = []
    for template_id in template_ids:
        template = CompilationTemplateService.get_saved(template_id, tenant_id)
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
    return active_templates


def split_tree_templates(
    active_templates: list[tuple[str, dict]],
) -> tuple[list[tuple[str, dict]], list[tuple[str, dict]]]:
    """Partition templates into ``(tree, non_tree)`` by kind."""
    from api.apps.restful_apis.chunk_api import _compilation_template_kind

    tree_templates: list[tuple[str, dict]] = []
    non_tree_templates: list[tuple[str, dict]] = []
    for tid, cfg in active_templates:
        if _compilation_template_kind((cfg or {}).get("kind")) == "tree":
            tree_templates.append((tid, cfg))
        else:
            non_tree_templates.append((tid, cfg))
    return tree_templates, non_tree_templates


def _is_page_index_template(parser_cfg: dict) -> bool:
    kind = (parser_cfg or {}).get("kind")
    if not isinstance(kind, str):
        return False
    return kind.strip().lower().replace("-", "_") in {"page_index", "pageindex"}


def _page_index_graph_summary(graph: dict, limit: int = 80) -> str:
    entities = graph.get("entities") if isinstance(graph, dict) else None
    if not isinstance(entities, list):
        return ""

    lines: list[str] = []
    for entity in entities:
        if not isinstance(entity, dict):
            continue
        name = str(entity.get("name") or "").strip()
        description = str(entity.get("discription") or entity.get("description") or "").strip()
        text = f"{name}: {description}".strip(": ").strip()
        if text:
            lines.append(text)
        if len(lines) >= limit:
            break
    return "\n".join(lines)


async def _upsert_dataset_nav_from_page_index(
    *,
    active_templates: list[tuple[str, dict]],
    chat_mdl_by_tid: dict[str, LLMBundle],
    embedding_model: LLMBundle,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    progress_cb: Callable[..., None],
    cancel_check: Callable[[], bool],
) -> None:
    page_index_templates = [(template_id, parser_cfg) for template_id, parser_cfg in active_templates if _is_page_index_template(parser_cfg)]
    if not page_index_templates:
        return

    summaries: list[str] = []
    chat_mdl = None
    for template_id, _ in page_index_templates:
        if cancel_check():
            raise TaskCanceledException("Task was cancelled before dataset navigation update")
        try:
            graph = await rebuild_structure_graph_json(
                tenant_id,
                kb_id,
                doc_id,
                "timeline",
                compilation_template_id=template_id,
            )
        except Exception:
            logging.exception(
                "page_index: failed to rebuild graph summary for dataset_nav doc %s template %s",
                doc_id,
                template_id,
            )
            continue

        summary = _page_index_graph_summary(graph)
        if summary:
            summaries.append(summary)
            chat_mdl = chat_mdl or chat_mdl_by_tid.get(template_id)

    if not summaries:
        logging.info("page_index: no dataset_nav summary for doc %s", doc_id)
        return

    if cancel_check():
        raise TaskCanceledException("Task was cancelled before dataset navigation upsert")
    try:
        from rag.advanced_rag.knowlege_compile.dataset_nav import (
            upsert_dataset_nav_doc,
        )

        progress_cb(msg=f"page_index: updating dataset navigation for doc {doc_id} ...")
        await upsert_dataset_nav_doc(
            tenant_id,
            kb_id,
            doc_id,
            "\n\n".join(summaries),
            embd_mdl=embedding_model,
            chat_mdl=chat_mdl,
        )
    except TaskCanceledException:
        raise
    except Exception:
        logging.exception("page_index: dataset_nav upsert failed for doc %s", doc_id)


# ----- non-tree compilation core -------------------------------------


async def run_structure_compile_over_batches(
    *,
    active_templates: list[tuple[str, dict]],
    chat_mdl_by_tid: dict[str, LLMBundle],
    embedding_model: LLMBundle,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    language: str,
    chunk_batches: AsyncIterator[list[dict]],
    progress_cb: Callable[..., None],
    cancel_check: Callable[[], bool] = lambda: False,
    record: Callable[[str, dict], None] | None = None,
) -> dict[str, dict]:
    """Extract + merge structures for every non-``tree`` template over an
    async stream of chunk batches, then run the optional synthesis phase.

    ``active_templates`` must already be the non-tree subset with a resolved
    chat model in ``chat_mdl_by_tid``. Chunks arrive as an async iterator of
    batches so callers can stream them from the doc store or hand over an
    in-memory list; each ``dict`` must expose ``id`` and text
    (``content_with_weight`` / ``text``).

    Returns ``{template_id: {"inserted", "updated", "duplicates_dropped"}}``.
    Raises :class:`TaskCanceledException` when ``cancel_check`` trips.
    """
    from api.apps.restful_apis.chunk_api import _compilation_template_kind

    if not active_templates:
        return {}

    total = len(active_templates)
    llm_pool = LLMCallPool(DOC_STRUCTURE_LLM_POOL_SIZE)

    accumulators: dict[str, list[dict]] = {tid: [] for tid, _ in active_templates}
    template_kinds: dict[str, str] = {tid: _compilation_template_kind((cfg or {}).get("kind")) for tid, cfg in active_templates}
    # ``dataset_merge`` hyper-parameter (per template config): when truthy the
    # merge dedups entities/relations across the whole dataset (KB), collapsing
    # cross-document duplicates onto one canonical row, instead of merging only
    # within the current document.
    merge_scope_by_tid: dict[str, str] = {tid: (MERGE_SCOPE_DATASET if bool((cfg or {}).get("dataset_merge")) else MERGE_SCOPE_DOC) for tid, cfg in active_templates}
    # compile_kwd(s) each template actually wrote, harvested from flush results
    # so a dataset-scope template can rebuild its dataset graph once at the end.
    compile_kwds_by_tid: dict[str, set[str]] = {tid: set() for tid, _ in active_templates}
    agg_infos: dict[str, dict] = {tid: {"inserted": 0, "updated": 0, "duplicates_dropped": 0} for tid, _ in active_templates}
    chunks_by_id: dict[str, str] = {}
    flush_sequence = 0
    flush_tasks: set[asyncio.Task[None]] = set()
    doc_storage_condition = asyncio.Condition()
    next_doc_storage_sequence = 0

    async def _flush(template_id: str) -> None:
        nonlocal flush_sequence
        acc = accumulators[template_id]
        if not acc:
            return
        docs = list(acc)
        acc.clear()
        flush_sequence += 1
        sequence = flush_sequence - 1
        timing_context = f"{doc_id}:{template_id}:flush-{flush_sequence}"

        async def _run_flush() -> None:
            nonlocal next_doc_storage_sequence
            doc_storage_acquired = False
            doc_storage_released = False

            async def _wait_for_doc_storage() -> None:
                nonlocal doc_storage_acquired
                async with doc_storage_condition:
                    await doc_storage_condition.wait_for(lambda: next_doc_storage_sequence == sequence)
                    doc_storage_acquired = True

            async def _release_doc_storage() -> None:
                nonlocal next_doc_storage_sequence, doc_storage_released
                async with doc_storage_condition:
                    if next_doc_storage_sequence != sequence:
                        raise RuntimeError(f"ES sequence mismatch: expected {next_doc_storage_sequence}, releasing {sequence}")
                    next_doc_storage_sequence += 1
                    doc_storage_released = True
                    doc_storage_condition.notify_all()

            kind = template_kinds.get(template_id, "")
            merge_chat_mdl = llm_pool.wrap(
                chat_mdl_by_tid[template_id],
                priority=20,
                label=f"merge:{template_id}",
                context=timing_context,
            )
            try:
                info = await merge_compiled_structures(
                    docs,
                    merge_chat_mdl,
                    embedding_model,
                    tenant_id,
                    kb_id,
                    compilation_template_id=template_id,
                    cancel_check=cancel_check,
                    timing_context=timing_context,
                    chunks_by_id=chunks_by_id,
                    chain_kind=kind,
                    chain_callback=progress_cb,
                    chain_timeout_seconds=STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S,
                    doc_storage_waiter=_wait_for_doc_storage,
                    doc_storage_releaser=_release_doc_storage,
                    merge_scope=merge_scope_by_tid[template_id],
                )
            finally:
                if not doc_storage_released:
                    if not doc_storage_acquired:
                        await _wait_for_doc_storage()
                    await _release_doc_storage()
            if isinstance(info, dict):
                agg = agg_infos[template_id]
                for k in ("inserted", "updated", "duplicates_dropped"):
                    agg[k] = agg.get(k, 0) + int(info.get(k, 0) or 0)
                for compile_kwd in info.get("compile_kwds") or []:
                    if compile_kwd:
                        compile_kwds_by_tid[template_id].add(str(compile_kwd))

        flush_tasks.add(asyncio.create_task(_run_flush()))

    progress_cb(msg=f"Start document knowledge compilation ({total} template(s)) ...")

    async def _compile_batch(batch_no: int, batch: list[dict], template_id: str, parser_cfg: dict) -> list[dict]:
        context = f"{doc_id}:{template_id}:compile-batch-{batch_no}"
        progress_cb(msg=f"  compile batch {batch_no} ({len(batch)} chunks) for template ({template_ids_by_id[template_id]}/{total})")
        compile_chat_mdl = llm_pool.wrap(
            chat_mdl_by_tid[template_id],
            priority=30,
            label=f"compile:{template_id}:batch-{batch_no}",
            context=context,
        )
        return await compile_structure_from_text(
            batch,
            parser_cfg,
            compile_chat_mdl,
            embedding_model,
            doc_id,
            language=language,
            callback=progress_cb,
            max_workers=3,
            compilation_template_id=template_id,
        )

    async def _commit_result(batch_no: int, batch_len: int, template_id: str, docs: list[dict]) -> None:
        if docs:
            accumulators[template_id].extend(docs)
        if len(accumulators[template_id]) >= DOC_STRUCTURE_MERGE_MAX_DOCS:
            progress_cb(msg=f"  merge flush ({len(accumulators[template_id])} docs) for batch {batch_no} ({batch_len} chunks) for template ({template_ids_by_id[template_id]}/{total})")
            await _flush(template_id)

    template_ids_by_id = {template_id: idx + 1 for idx, (template_id, _) in enumerate(active_templates)}
    inflight: dict[asyncio.Task[list[dict]], tuple[int, int, int, str]] = {}
    completed: dict[int, tuple[int, int, str, list[dict]]] = {}
    submit_sequence = 0
    commit_sequence = 0

    async def _commit_ready() -> None:
        nonlocal commit_sequence
        while commit_sequence in completed:
            batch_no, batch_len, template_id, docs = completed.pop(commit_sequence)
            await _commit_result(batch_no, batch_len, template_id, docs)
            commit_sequence += 1

    async def _cancel_pending() -> None:
        pending = [task for task in (*inflight, *flush_tasks) if not task.done()]
        for task in pending:
            task.cancel()
        if pending:
            await asyncio.gather(*pending, return_exceptions=True)
        inflight.clear()
        flush_tasks.clear()

    async def _reap_one() -> None:
        if not inflight:
            return
        try:
            done, _ = await asyncio.wait(tuple(inflight), return_when=asyncio.FIRST_COMPLETED)
        except BaseException:
            await _cancel_pending()
            raise
        for task in done:
            sequence, batch_no, batch_len, template_id = inflight.pop(task)
            try:
                docs = task.result()
            except BaseException:
                await _cancel_pending()
                raise
            completed[sequence] = (batch_no, batch_len, template_id, docs)
        await _commit_ready()

    async def _submit_batches() -> None:
        nonlocal submit_sequence
        batch_no = 0
        try:
            async for batch in chunk_batches:
                batch_no += 1
                for chunk in batch:
                    cid = chunk.get("id")
                    if isinstance(cid, str) and cid not in chunks_by_id:
                        text = chunk.get("content_with_weight") or chunk.get("text") or ""
                        chunks_by_id[cid] = text if isinstance(text, str) else ""
                for template_id, parser_cfg in active_templates:
                    if cancel_check():
                        raise TaskCanceledException("Task was cancelled during document knowledge compilation")
                    task = asyncio.create_task(_compile_batch(batch_no, batch, template_id, parser_cfg))
                    inflight[task] = (submit_sequence, batch_no, len(batch), template_id)
                    submit_sequence += 1
                    if len(inflight) + len(completed) >= DOC_STRUCTURE_COMPILE_MAX_IN_FLIGHT:
                        await _reap_one()
        except BaseException:
            await _cancel_pending()
            raise

    await _submit_batches()

    while inflight:
        if cancel_check():
            await _cancel_pending()
            raise TaskCanceledException("Task was cancelled during document knowledge compilation")
        await _reap_one()
    await _commit_ready()

    for template_id, _ in active_templates:
        if cancel_check():
            await _cancel_pending()
            raise TaskCanceledException("Task was cancelled before merge flush")
        await _flush(template_id)
    if flush_tasks:
        try:
            await asyncio.gather(*flush_tasks)
        except BaseException:
            await _cancel_pending()
            raise
        finally:
            flush_tasks.clear()

    # ── Dataset structure graph ──────────────────────────────────────────
    # For dataset-scope templates the entity/relation rows are now merged
    # across documents, so (re)project them into a single KB-wide graph. This
    # runs once per document-parse completion; the last document to finish
    # produces the complete dataset graph. Best-effort — a failure here must
    # not fail the parse.
    for template_id, _ in active_templates:
        if merge_scope_by_tid[template_id] != MERGE_SCOPE_DATASET:
            continue
        # The row is filtered by the template's *top-level* kind (e.g.
        # ``knowledge_graph``, ``session_graph``), which — unlike ``config.kind``
        # — distinguishes the knowledge_graph family. Resolve it from the
        # template record; on failure leave it unstamped (the read side falls
        # back to resolving kind from the template id).
        structure_kind = None
        try:
            saved_template = CompilationTemplateService.get_saved(template_id, tenant_id)
            if saved_template:
                structure_kind = (saved_template.get("kind") or "").strip() or None
        except Exception:
            logging.exception("dataset structure graph: failed to resolve top-level kind for template %s", template_id)
        for compile_kwd in sorted(compile_kwds_by_tid[template_id]):
            if cancel_check():
                raise TaskCanceledException("Task was cancelled before dataset structure graph rebuild")
            try:
                progress_cb(msg=f"Rebuilding dataset structure graph (compile_kwd={compile_kwd}) ...")
                await rebuild_dataset_structure_graph_json(
                    tenant_id,
                    kb_id,
                    compile_kwd,
                    compilation_template_id=template_id,
                    structure_kind=structure_kind,
                )
            except TaskCanceledException:
                raise
            except Exception:
                logging.exception(
                    "dataset structure graph rebuild failed for kb=%s compile_kwd=%s template=%s",
                    kb_id,
                    compile_kwd,
                    template_id,
                )

    await _upsert_dataset_nav_from_page_index(
        active_templates=active_templates,
        chat_mdl_by_tid=chat_mdl_by_tid,
        embedding_model=embedding_model,
        tenant_id=tenant_id,
        kb_id=kb_id,
        doc_id=doc_id,
        progress_cb=progress_cb,
        cancel_check=cancel_check,
    )
    # Timeline entity cleanup must happen after every flush has completed;
    # otherwise an entity can look isolated in one flush and be referenced by
    # a relation from a later flush. Keep this scoped to timeline templates.
    for template_id, _ in active_templates:
        if template_kinds.get(template_id) != "timeline":
            continue
        try:
            await cleanup_timeline_isolated_entities(
                tenant_id,
                kb_id,
                doc_id,
                compilation_template_id=template_id,
            )
        except Exception:
            logging.exception(
                "document_structure_compile: timeline isolated-entity cleanup failed for template=%s",
                template_id,
            )

    for idx, (template_id, parser_cfg) in enumerate(active_templates):
        if cancel_check():
            raise TaskCanceledException("Task was cancelled during document knowledge compilation")
        agg = agg_infos[template_id]
        if record:
            record(f"document_structure_compile:{template_id}", agg)
        progress_cb(msg=f"Document knowledge compilation done ({idx + 1}/{total}): {agg}")

        # ── Synthesis phase ──────────────────────────────────────────────
        # If the template has synthesis.enabled, run wiki PLAN+REFINE
        # to generate output (wiki page, essence paragraph, etc.).
        synthesis_cfg = (parser_cfg or {}).get("synthesis") or {}
        if synthesis_cfg.get("enabled"):
            example = synthesis_cfg.get("example")
            compile_kwd = synthesis_cfg.get("compile_kwd", "artifact_page")
            plan_cfg = synthesis_cfg.get("plan") or {}

            # Reserved for future wiki_plan_from_reduction extension:
            # entity_type_filter, mention_count_threshold, top_n
            if plan_cfg:
                logging.debug(
                    "synthesis: template %s plan config %r reserved for future use",
                    template_id,
                    plan_cfg,
                )

            if cancel_check():
                raise TaskCanceledException("Task was cancelled before synthesis PLAN")

            if not example:
                logging.warning(
                    "synthesis: template %s has synthesis.enabled but no example; skipping",
                    template_id,
                )
            else:
                try:
                    from rag.advanced_rag.knowlege_compile.wiki import (
                        wiki_plan_from_reduction,
                        wiki_refine_from_plan,
                    )

                    progress_cb(msg=f"Synthesis PLAN for template {template_id} (kind={compile_kwd}) ...")
                    plan = await wiki_plan_from_reduction(
                        chat_mdl=llm_pool.wrap(
                            chat_mdl_by_tid[template_id],
                            priority=20,
                            label=f"synthesis-plan:{template_id}",
                            context=f"{doc_id}:{template_id}:synthesis-plan",
                        ),
                        embd_mdl=embedding_model,
                        tenant_id=tenant_id,
                        kb_id=kb_id,
                        callback=progress_cb,
                    )
                    if cancel_check():
                        raise TaskCanceledException("Task was cancelled after synthesis PLAN")

                    if not plan or not plan.get("pages"):
                        progress_cb(msg=f"Synthesis: no pages planned for template {template_id}.")
                    else:
                        progress_cb(msg=f"Synthesis REFINE for template {template_id} ({len(plan['pages'])} page(s)) ...")
                        pages = await wiki_refine_from_plan(
                            chat_mdl=llm_pool.wrap(
                                chat_mdl_by_tid[template_id],
                                priority=20,
                                label=f"synthesis-refine:{template_id}",
                                context=f"{doc_id}:{template_id}:synthesis-refine",
                            ),
                            embd_mdl=embedding_model,
                            tenant_id=tenant_id,
                            kb_id=kb_id,
                            callback=progress_cb,
                            example=example,
                        )
                        # Overwrite compile_kwd on every output page so the
                        # synthesis type is tracked correctly in ES.
                        for p in pages or []:
                            p["compile_kwd"] = compile_kwd
                        progress_cb(msg=f"Synthesis done: {len(pages or [])} {compile_kwd} page(s) written.")
                except TaskCanceledException:
                    raise
                except Exception:
                    logging.exception("synthesis: failed for template %s", template_id)

    return agg_infos
