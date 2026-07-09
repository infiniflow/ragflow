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
    CHAIN_KINDS,
    compile_structure_from_text,
    merge_compiled_structures,
    validate_and_correct_chain,
)


# ----- tunables ------------------------------------------------------
# Bound how many source chunks are handed to a single
# ``compile_structure_from_text`` invocation. The call fans them out
# across max_workers internally, so a moderate window keeps memory +
# LLM-context pressure predictable for long docs.
DOC_STRUCTURE_COMPILE_BATCH_CHUNKS = 4

# Bound how many compiled ES-ready docs may accumulate before we flush
# them through ``merge_compiled_structures``. The merger does pairwise
# cosine + LLM duplicate-judging, so it's the more expensive step; we
# cap the per-flush set to keep the local-dedup buckets tractable.
DOC_STRUCTURE_MERGE_MAX_DOCS = 512

# Hard wall on the chain-validator LLM correction step. ``list`` and
# ``timeline`` kinds run this just before each merge flush; anything
# longer than this is treated as a blocked LLM and the uncorrected
# docs are flushed instead.
STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S = 120.0


# ----- template resolution -------------------------------------------


def resolve_template_ids_from_groups(group_ids, tenant_id: str) -> list[str]:
    """Resolve an ordered, de-duplicated list of compilation-template ids
    from a list of template-*group* ids.

    Mirrors ``_parser_config_compilation_template_ids`` but takes the group
    ids directly (the ``rag.flow`` Compiler carries them as a component
    parameter rather than inside ``parser_config``).
    """
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

    accumulators: dict[str, list[dict]] = {tid: [] for tid, _ in active_templates}
    template_kinds: dict[str, str] = {tid: _compilation_template_kind((cfg or {}).get("kind")) for tid, cfg in active_templates}
    agg_infos: dict[str, dict] = {tid: {"inserted": 0, "updated": 0, "duplicates_dropped": 0} for tid, _ in active_templates}
    chunks_by_id: dict[str, str] = {}

    async def _flush(template_id: str) -> None:
        acc = accumulators[template_id]
        if not acc:
            return
        kind = template_kinds.get(template_id, "")
        if kind in CHAIN_KINDS:
            try:
                acc = await asyncio.wait_for(
                    validate_and_correct_chain(
                        acc,
                        chunks_by_id,
                        chat_mdl_by_tid[template_id],
                        kind,
                        callback=progress_cb,
                    ),
                    timeout=STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S,
                )
                accumulators[template_id] = acc
            except asyncio.TimeoutError:
                logging.warning(
                    "chain validate: timed out after %ss for template %s; using uncorrected docs",
                    STRUCTURE_CHAIN_CORRECTION_TIMEOUT_S,
                    template_id,
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
            tenant_id,
            kb_id,
            compilation_template_id=template_id,
            cancel_check=cancel_check,
        )
        acc.clear()
        if isinstance(info, dict):
            agg = agg_infos[template_id]
            for k in ("inserted", "updated", "duplicates_dropped"):
                agg[k] = agg.get(k, 0) + int(info.get(k, 0) or 0)

    progress_cb(msg=f"Start document knowledge compilation ({total} template(s)) ...")

    batch_no = 0
    async for batch in chunk_batches:
        batch_no += 1
        for chunk in batch:
            cid = chunk.get("id")
            if isinstance(cid, str) and cid not in chunks_by_id:
                text = chunk.get("content_with_weight") or chunk.get("text") or ""
                chunks_by_id[cid] = text if isinstance(text, str) else ""
        for idx, (template_id, parser_cfg) in enumerate(active_templates):
            progress_cb(msg=f"  compile batch {batch_no} ({len(batch)} chunks) for template ({idx + 1}/{total})")
            docs = await compile_structure_from_text(
                batch,
                parser_cfg,
                chat_mdl_by_tid[template_id],
                embedding_model,
                doc_id,
                language=language,
                callback=progress_cb,
                compilation_template_id=template_id,
            )
            if docs:
                accumulators[template_id].extend(docs)
            if len(accumulators[template_id]) >= DOC_STRUCTURE_MERGE_MAX_DOCS:
                progress_cb(msg=f"  merge flush ({len(accumulators[template_id])} docs) for template ({idx + 1}/{total})")
                await _flush(template_id)

    for idx, (template_id, parser_cfg) in enumerate(active_templates):
        if cancel_check():
            raise TaskCanceledException("Task was cancelled during document knowledge compilation")
        await _flush(template_id)
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
                    template_id, plan_cfg,
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
                        chat_mdl=chat_mdl_by_tid[template_id],
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
                            chat_mdl=chat_mdl_by_tid[template_id],
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
