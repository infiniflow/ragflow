#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

"""KB-wide structure-graph merge task.

Runs when the user POSTs to ``/datasets/<id>/index`` with a structure index
type (``structure_graph`` / ``structure_mindmap`` / ``timeline`` /
``session_graph`` / ``session_essence``, or ``structure`` for merge-all). It
re-projects every document's already-merged ``entity`` / ``relation`` rows into
the KB-wide ``knowledge_graph_kwd="dataset_graph"`` rows via
:func:`rag.advanced_rag.knowlege_compile.structure.rebuild_dataset_structure_graph_json`
— the same merge the per-document parse runner performs at flush time
(``rag.advanced_rag.knowlege_compile.runner``), exposed here as an on-demand
re-merge.

The task carries the target kind in ``task_type``; the driver enumerates the
``(compile_kwd, template_id)`` pairs actually present in the store, keeps only
dataset-merge templates of the requested kind (all kinds for merge-all), and
rebuilds one dataset graph per pair. It performs no LLM work.
"""

from __future__ import annotations

import logging
from typing import Optional

from common import settings
from common.misc_utils import thread_pool_exec
from rag.nlp import search
from rag.advanced_rag.knowlege_compile.structure import (
    rebuild_dataset_structure_graph_json,
)
from rag.svr.task_executor_refactor.task_context import TaskContext


# Structure merge task_type -> the template's top-level ``kind`` as stored on
# ``dataset_graph`` rows. ``None`` (the merge-all ``structure`` type) rebuilds
# every dataset-merge kind regardless of its top-level kind.
_STRUCTURE_TASK_TYPE_TO_KIND: dict[str, Optional[str]] = {
    "structure_graph": "knowledge_graph",
    "structure_mindmap": "mind_map",
    "timeline": "timeline",
    "session_graph": "session_graph",
    "session_essence": "session_essence",
    "structure": None,  # merge-all
}

STRUCTURE_MERGE_TASK_TYPES = frozenset(_STRUCTURE_TASK_TYPE_TO_KIND)


def is_structure_merge_task(task_type: str) -> bool:
    return (task_type or "").lower() in STRUCTURE_MERGE_TASK_TYPES


async def _collect_structure_pairs(tenant_id: str, kb_id: str) -> set[tuple[str, str]]:
    """Distinct ``(compile_kwd, template_id)`` pairs across the KB's structure
    ``entity`` / ``relation`` rows — the inputs each dataset graph is rebuilt
    from. Rows without a ``compilation_template_ids`` are skipped: the merge is
    always template-scoped (an untemplated rebuild would over-merge unrelated
    kinds sharing a ``compile_kwd``)."""
    from common.doc_store.doc_store_base import OrderByExpr

    index = search.index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return set()

    select_fields = ["id", "compile_kwd", "compilation_template_ids"]
    pairs: set[tuple[str, str]] = set()
    offset = 0
    page_size = 1000
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields,
                [],
                {"knowledge_graph_kwd": ["entity", "relation"]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                index,
                [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields) or {}
        except Exception:
            logging.exception("structure_merge: failed to scan entity/relation rows for kb=%s", kb_id)
            break
        if not field_map:
            break
        for row in field_map.values():
            compile_kwd = row.get("compile_kwd")
            if not isinstance(compile_kwd, str) or not compile_kwd:
                continue
            raw_tids = row.get("compilation_template_ids")
            if isinstance(raw_tids, str):
                tids = [raw_tids] if raw_tids else []
            elif isinstance(raw_tids, list):
                tids = [t for t in raw_tids if isinstance(t, str) and t]
            else:
                tids = []
            for tid in tids:
                pairs.add((compile_kwd, tid))
        if len(field_map) < page_size:
            break
        offset += page_size
    return pairs


async def run_structure_merge(ctx: TaskContext) -> None:
    """Rebuild the KB-wide dataset structure graph(s) for the task's kind.

    Enumerates the ``(compile_kwd, template_id)`` pairs present in the store,
    filters to dataset-merge templates of the requested kind (all kinds for the
    merge-all ``structure`` type), and rebuilds one ``dataset_graph`` row per
    pair. Per-pair failures are logged and skipped so one bad template does not
    abort the whole merge.
    """
    from api.db.services.compilation_template_service import CompilationTemplateService

    progress = ctx.progress_cb
    task_type = (ctx.task_type or "").lower()
    target_kind = _STRUCTURE_TASK_TYPE_TO_KIND.get(task_type)
    merge_all = task_type == "structure"

    def _canceled() -> bool:
        try:
            return bool(ctx.has_canceled_func(ctx.id))
        except Exception:
            return False

    progress(0.0, "Collecting structure graphs to merge...")
    pairs = await _collect_structure_pairs(ctx.tenant_id, ctx.kb_id)
    if not pairs:
        progress(1.0, "No structure graphs to merge.")
        return

    # Resolve each template once: keep only dataset-merge templates, and (unless
    # merge-all) only those whose top-level kind matches the requested kind.
    # ``template_meta`` maps template_id -> (keep: bool, structure_kind: str|None).
    template_meta: dict[str, tuple[bool, Optional[str]]] = {}

    def _resolve_template(template_id: str) -> tuple[bool, Optional[str]]:
        cached = template_meta.get(template_id)
        if cached is not None:
            return cached
        keep = False
        structure_kind: Optional[str] = None
        try:
            saved = CompilationTemplateService.get_saved(template_id, ctx.tenant_id)
            if saved:
                structure_kind = (saved.get("kind") or "").strip() or None
                config = saved.get("config") or {}
                dataset_merge = bool(config.get("dataset_merge")) if isinstance(config, dict) else False
                kind_ok = merge_all or (structure_kind == target_kind)
                keep = dataset_merge and kind_ok
        except Exception:
            logging.exception("structure_merge: failed to resolve template %s for kb=%s", template_id, ctx.kb_id)
        result = (keep, structure_kind)
        template_meta[template_id] = result
        return result

    eligible = []
    for compile_kwd, template_id in sorted(pairs):
        keep, structure_kind = _resolve_template(template_id)
        if keep:
            eligible.append((compile_kwd, template_id, structure_kind))

    if not eligible:
        kind_label = "any kind" if merge_all else (target_kind or task_type)
        progress(1.0, f"No dataset-merge structure templates to merge for {kind_label}.")
        return

    total = len(eligible)
    rebuilt = 0
    for i, (compile_kwd, template_id, structure_kind) in enumerate(eligible):
        if _canceled():
            progress(-1, "Task has been canceled.")
            return
        progress(
            0.05 + 0.9 * (i / total),
            f"Merging structure graph {i + 1}/{total} (compile_kwd={compile_kwd}) ...",
        )
        try:
            await rebuild_dataset_structure_graph_json(
                ctx.tenant_id,
                ctx.kb_id,
                compile_kwd,
                compilation_template_id=template_id,
                structure_kind=structure_kind,
            )
            rebuilt += 1
        except Exception:
            logging.exception(
                "structure_merge: rebuild failed for kb=%s compile_kwd=%s template=%s",
                ctx.kb_id,
                compile_kwd,
                template_id,
            )

    progress(1.0, f"Merged {rebuilt}/{total} structure graph(s).")
