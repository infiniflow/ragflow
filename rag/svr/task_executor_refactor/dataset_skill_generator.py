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

"""Corpus → Skill tree generator.

Extracted from ``rag.svr.task_executor_refactor.task_handler`` where the
same pipeline previously lived as a set of ``_skill_*`` methods and one
``_corpus2skill`` orchestrator. The public entry point is
:func:`run_corpus2skill`; the module-level helpers are internal but
kept accessible so tests can exercise the individual phases.

Design notes:

* The pipeline is per-KB: given every parsed doc in a KB it produces a
  hierarchical "skill" tree by summarizing each doc, RAPTOR-clustering
  the summaries, summarizing each cluster, then repeating until the top
  fan-out is at or below :data:`SKILL_MAX_TOP_CLUSTERS`.
* Each node lands in ES twice: one per-node row under
  ``compile_kwd="skill"`` carrying markdown metadata + a leaf-vs-branch
  contents section, and one aggregate row under
  ``compile_kwd="skill_all"`` holding the whole recursive tree as JSON
  for cheap sidebar reads.
* The layout mirrors Corpus2Skill's ``SKILL.md`` / ``INDEX.md`` naming
  so a future on-disk export is a straight projection.

The extraction keeps the callable surface minimal:

    run_corpus2skill(ctx, embedding_model, load_chunks_for_doc)

``load_chunks_for_doc`` is injected rather than imported to keep the
module decoupled from ``TaskHandler``'s streaming chunk loader — any
async iterator that yields batches of ``{content_with_weight: str, ...}``
dicts will do.
"""

from __future__ import annotations

import asyncio
import json
import logging
import re
from dataclasses import dataclass, field
from typing import AsyncIterator, Callable, Dict, Optional

import numpy as np
import xxhash

from common import settings
from common.constants import LLMType
from common.misc_utils import thread_pool_exec
from common.token_utils import num_tokens_from_string
from rag.nlp import search
from rag.svr.task_executor_refactor.task_context import TaskContext


# ----- tunables ------------------------------------------------------
# Stop folding clusters once we've boiled the KB down to ≤ this many
# top-level nodes. Mirrors Corpus2Skill's default top-of-tree fan-out.
SKILL_MAX_TOP_CLUSTERS = 8
# Per-doc summary is built from a budget of this fraction of the chat
# model's context window. Stops adding chunks once cumulative tokens
# hit the cap.
SKILL_DOC_BUDGET_FRACTION = 0.5
# Concurrency caps for the two LLM-bound stages.
SKILL_DOC_SUMMARY_CONCURRENCY = 8
SKILL_LABEL_CONCURRENCY = 10
# Defensive cap on the clustering loop — a degenerate clustering that
# keeps returning N clusters for N inputs would otherwise loop forever.
SKILL_MAX_TREE_ITERATIONS = 12
# Page size for the streaming chunk reader used during per-doc
# summarization.
SKILL_CHUNK_BATCH = 64


# A ``load_chunks_for_doc`` callable takes ``(tenant_id, kb_id, doc_id,
# batch_size)`` and returns an async iterator of chunk-batch lists.
ChunkLoader = Callable[
    [str, str, str],  # positional: tenant_id, kb_id, doc_id
    AsyncIterator[list[dict]],
]


@dataclass
class SkillNode:
    """One node in the corpus → skill hierarchy.

    Leaves wrap a single doc; branch nodes wrap a cluster of child
    nodes plus the LLM summary of their summaries. ``doc_ids`` is the
    flattened leaf set under this node, so any branch can quickly
    report ``num_documents``. ``doc_texts`` carries first-page
    previews keyed by ``doc_id`` only for leaves whose markdown nav
    file needs the first-line title — branches inherit it as a no-op
    union so the dict is non-empty across the tree.
    """

    level: int
    label: str
    summary: str
    vec: np.ndarray
    doc_ids: list[str]
    doc_texts: dict[str, str] = field(default_factory=dict)
    children: list["SkillNode"] = field(default_factory=list)
    folder_name: str = ""


# ----- helpers -------------------------------------------------------


def skill_safe_name(text: str, max_len: int = 50) -> str:
    """Lowercase, hyphen-only, max-len-clamped slug. Mirrors
    Corpus2Skill ``_safe_name`` for cross-system stability of folder
    names."""
    name = (text or "").lower().strip()
    name = re.sub(r"[^a-z0-9\s-]", "", name)
    name = re.sub(r"\s+", "-", name)
    name = name.strip("-")[:max_len]
    return name


async def label_skill_node_one(
    summary: str,
    chat_mdl,
    semaphore: asyncio.Semaphore,
) -> str:
    """Generate a single fs-safe label: 2–5 word lowercase
    hyphenated label, max_tokens=20, sanitized to [a-z0-9-], capped
    at 50 chars, falls back to "cluster" on any failure.
    """
    async with semaphore:
        try:
            cnt = await chat_mdl.async_chat(
                "You generate short filesystem-safe cluster labels. Reply with the label only.",
                [
                    {
                        "role": "user",
                        "content": (
                            "Generate a short (2-5 word) filesystem-safe label for this cluster. "
                            "Use lowercase(MUST be in the same language as 'Summary'), hyphens instead of spaces. No quotes.\n\n"
                            f"Summary: {(summary or '')[:500]}"
                        ),
                    }
                ],
                {"max_tokens": 20, "temperature": 0.0},
            )
            raw = (cnt or "").strip().lower()
            label = re.sub(r"[^a-z0-9-]", "-", raw)
            label = re.sub(r"-+", "-", label).strip("-")[:50]
            return label or "cluster"
        except Exception:
            logging.exception("skill: label generation failed; using fallback")
            return "cluster"


async def doc_summary_for_skill(
    doc_id: str,
    raptor,
    chat_mdl,
    ctx: TaskContext,
    load_chunks_for_doc: Callable[..., AsyncIterator[list[dict]]],
) -> Optional["SkillNode"]:
    """Concatenate chunks up to half the chat model's context budget,
    then summarize via RAPTOR's ``_summarize_texts`` (which also
    returns the embedding). Returns a leaf-shaped :class:`SkillNode`
    or ``None`` if the doc has no usable chunks."""
    max_ctx = int(getattr(chat_mdl, "max_length", 4096) or 4096)
    budget = max(512, int(max_ctx * SKILL_DOC_BUDGET_FRACTION))

    accumulated: list[str] = []
    running = 0
    async for batch in load_chunks_for_doc(
        ctx.tenant_id,
        ctx.kb_id,
        doc_id,
        batch_size=SKILL_CHUNK_BATCH,
    ):
        for chunk in batch:
            text = chunk.get("content_with_weight") or ""
            if not isinstance(text, str) or not text:
                continue
            t_tokens = num_tokens_from_string(text)
            if running + t_tokens > budget and accumulated:
                break
            accumulated.append(text)
            running += t_tokens
        else:
            continue
        break

    if not accumulated:
        return None

    result = await raptor._summarize_texts(accumulated, callback=None, task_id="")
    if result is None:
        return None
    title, summary_text, vec = result

    doc_preview = accumulated[0][:600] if accumulated else ""
    return SkillNode(
        level=0,
        label="",  # filled in phase 5
        summary=summary_text or title,
        vec=np.asarray(vec),
        doc_ids=[doc_id],
        doc_texts={doc_id: doc_preview},
        children=[],
        folder_name="",  # filled in phase 5
    )


def build_skill_md(node: "SkillNode") -> str:
    """SKILL.md (depth 0) / INDEX.md (deeper) text. Mirrors
    Corpus2Skill's ``_format_skill_md`` (skill_builder.py:193): YAML
    frontmatter (name / description / level / num_documents), then
    ``## Overview`` with the full summary, then ``## Contents`` —
    sub-groups for branches, ``- `doc_id`: <first 120 chars>`` for
    leaves.
    """
    depth = node.level
    name = node.folder_name or node.label or f"cluster-{depth}"
    desc = (node.summary or "")[:300].replace("\n", " ").strip()
    lines: list[str] = [
        "---",
        f"name: {name}",
        "description: >",
        f"  {desc}",
        f"level: {depth}",
        f"num_documents: {len(node.doc_ids)}",
        "---",
        "",
        "## Overview",
        "",
        (node.summary or "").strip() or "(no summary)",
        "",
        "## Contents",
        "",
    ]
    if node.children:
        lines.append("### Sub-groups (directories)")
        lines.append("")
        for child in node.children:
            child_name = child.folder_name or child.label or "cluster"
            summary_snip = (child.summary or "")[:200].replace("\n", " ").strip()
            lines.append(f"- **{child_name}/** ({len(child.doc_ids)} docs): {summary_snip}")
        lines.append("")
    else:
        lines.append(f"### Documents ({len(node.doc_ids)} items)")
        lines.append("")
        for doc_id in node.doc_ids:
            preview = node.doc_texts.get(doc_id, "")
            first_line = (preview.split("\n", 1)[0] if preview else "").strip()[:120]
            lines.append(f"- `{doc_id}`: {first_line}")
        lines.append("")
    return "\n".join(lines)


def skill_node_es_row(ctx: TaskContext, node: "SkillNode") -> Dict:
    """Build the ES row for one tree node. Stable id from
    (kb_id, folder_name) so re-runs upsert cleanly."""
    kb_id_str = str(ctx.kb_id)
    row_id = xxhash.xxh64(
        f"skill:{kb_id_str}:{node.folder_name}".encode("utf-8", "surrogatepass"),
    ).hexdigest()
    return {
        "id": row_id,
        "kb_id": kb_id_str,
        "doc_id": kb_id_str,  # KB-scoped sentinel
        "compile_kwd": "skill",
        "skill_kwd": node.folder_name,
        "depth_int": int(node.level),
        "children_kwd": [c.folder_name for c in node.children],
        "source_doc_ids": list(node.doc_ids),
        "md_with_weight": build_skill_md(node),
        "available_int": 1,
    }


def skill_tree_md_snippet(node: "SkillNode") -> str:
    """Return only the frontmatter/preamble before the Overview body.

    The one-shot tree browser needs enough metadata to render the skill
    directory without loading every full node body up front.
    """
    md = build_skill_md(node)
    return md.split("\n## Overview", 1)[0].strip()


def skill_tree_node(node: "SkillNode") -> Dict:
    return {
        "skill_kwd": node.folder_name,
        "md_with_weight": skill_tree_md_snippet(node),
        "children_kwd": [skill_tree_node(child) for child in node.children],
    }


def skill_all_es_row(ctx: TaskContext, roots: list["SkillNode"]) -> Dict:
    """Build the aggregate tree row loaded by the Skills sidebar."""
    kb_id_str = str(ctx.kb_id)
    row_id = xxhash.xxh64(
        f"skill_all:{kb_id_str}".encode("utf-8", "surrogatepass"),
    ).hexdigest()
    return {
        "id": row_id,
        "kb_id": kb_id_str,
        "doc_id": kb_id_str,
        "compile_kwd": "skill_all",
        "skill_with_weight": json.dumps(
            [skill_tree_node(root) for root in roots],
            ensure_ascii=False,
            indent=2,
        ),
        "available_int": 1,
    }


# ----- main entry ----------------------------------------------------


async def run_corpus2skill(
    ctx: TaskContext,
    embedding_model,
    load_chunks_for_doc: Callable[..., AsyncIterator[list[dict]]],
) -> None:
    """Build a hierarchical skill tree for the current KB and persist
    one ES row per node under ``compile_kwd="skill"`` plus a full
    recursive aggregate row under ``compile_kwd="skill_all"``.

    Always-rebuild semantics for v1: every parsed doc in the KB is
    re-summarized on each call. (Incremental "only changed docs" is a
    TODO — needs a per-doc content-hash similar to MAP's
    ``chunk_hash_kwd``.)
    """
    # Local imports so the module doesn't drag in the API service layer
    # at import time — that's a source of circular-import risk given how
    # much lives under ``api.db.services``.
    from api.db.services.document_service import DocumentService
    from api.db.services.llm_service import LLMBundle
    from api.db.joint_services.tenant_model_service import (
        get_tenant_default_model_by_type,
    )
    from rag.advanced_rag.knowlege_compile.raptor import (
        RecursiveAbstractiveProcessing4TreeOrganizedRetrieval as Raptor,
    )

    progress = ctx.progress_cb
    progress(0.0, "skill: loading documents")

    # ---- Phase 0: chat model + RAPTOR instance for summarization/clustering.
    chat_model_config = get_tenant_default_model_by_type(ctx.tenant_id, LLMType.CHAT)
    chat_mdl = LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language)

    raptor = Raptor(
        max_cluster=128,
        llm_model=chat_mdl,
        embd_model=embedding_model,
        prompt="Please write a concise summary of the following texts:\n{cluster_content}",
        max_token=256,
        threshold=0.1,
        max_errors=3,
    )

    # ---- Phase 1: per-doc summaries.
    all_docs, _ = await thread_pool_exec(
        DocumentService.get_by_kb_id,
        kb_id=ctx.kb_id,
        page_number=0,
        items_per_page=0,
        orderby="create_time",
        desc=False,
        keywords="",
        run_status=[],
        types=[],
        suffix=[],
    )
    eligible_docs = [d for d in (all_docs or []) if d.get("id")]
    if not eligible_docs:
        progress(1.0, "skill: no documents in KB")
        return

    # Phase-1 gate: bail before spinning up N per-doc summarizations.
    if ctx.has_canceled_func(ctx.id):
        progress(-1, "skill: task has been canceled")
        return

    n_docs = len(eligible_docs)
    progress(0.05, f"skill: summarizing {n_docs} document(s)")
    doc_sem = asyncio.Semaphore(SKILL_DOC_SUMMARY_CONCURRENCY)

    async def _summarize_doc(d: Dict) -> Optional[SkillNode]:
        async with doc_sem:
            try:
                return await doc_summary_for_skill(
                    d["id"],
                    raptor,
                    chat_mdl,
                    ctx,
                    load_chunks_for_doc,
                )
            except Exception:
                logging.exception(
                    "skill: doc summary failed for doc=%s",
                    d.get("id"),
                )
                return None

    leaf_results = await asyncio.gather(
        *(_summarize_doc(d) for d in eligible_docs),
        return_exceptions=False,
    )
    leaves: list[SkillNode] = [n for n in leaf_results if n is not None]
    if not leaves:
        progress(1.0, "skill: no doc summaries produced")
        return

    # Post-Phase-1 gate: bail before starting the iterative clustering.
    if ctx.has_canceled_func(ctx.id):
        progress(-1, "skill: task has been canceled")
        return

    # ---- Phase 2-4: iterative clustering until ≤ MAX_TOP.
    current_layer = leaves
    level = 0
    for iteration in range(SKILL_MAX_TREE_ITERATIONS):
        # Per-iteration gate: caps the wasted LLM cost when the task is
        # canceled mid-way through a many-layer clustering run.
        if ctx.has_canceled_func(ctx.id):
            progress(-1, "skill: task has been canceled")
            return
        if len(current_layer) <= SKILL_MAX_TOP_CLUSTERS:
            break
        progress(
            0.3 + 0.4 * iteration / SKILL_MAX_TREE_ITERATIONS,
            f"skill: clustering layer {level} ({len(current_layer)} nodes)",
        )
        try:
            embeddings = np.asarray([n.vec for n in current_layer])
            n_clusters, labels = raptor.clustering(
                embeddings,
                random_state=0,
                task_id="",
            )
        except Exception:
            logging.exception("skill: clustering failed at level %d", level)
            break
        if n_clusters <= 0 or n_clusters >= len(current_layer):
            # No reduction → stop to avoid an infinite loop.
            logging.warning(
                "skill: clustering did not reduce node count (%d → %d); stopping",
                len(current_layer),
                n_clusters,
            )
            break

        cluster_buckets: dict[int, list[SkillNode]] = {}
        for idx, lbl in enumerate(labels):
            cluster_buckets.setdefault(int(lbl), []).append(current_layer[idx])

        async def _summarize_cluster(children: list[SkillNode]) -> Optional[SkillNode]:
            texts = [c.summary for c in children if c.summary]
            if not texts:
                return None
            try:
                res = await raptor._summarize_texts(texts, callback=None, task_id="")
            except Exception:
                logging.exception("skill: cluster summary failed")
                return None
            if res is None:
                return None
            title, summary_text, vec = res
            merged_doc_ids: list[str] = []
            seen: set[str] = set()
            merged_doc_texts: dict[str, str] = {}
            for c in children:
                for did in c.doc_ids:
                    if did and did not in seen:
                        seen.add(did)
                        merged_doc_ids.append(did)
                merged_doc_texts.update(c.doc_texts)
            return SkillNode(
                level=level + 1,
                label="",
                summary=summary_text or title,
                vec=np.asarray(vec),
                doc_ids=merged_doc_ids,
                doc_texts=merged_doc_texts,
                children=list(children),
                folder_name="",
            )

        parent_results = await asyncio.gather(
            *(_summarize_cluster(children) for children in cluster_buckets.values()),
            return_exceptions=False,
        )
        next_layer = [p for p in parent_results if p is not None]
        if not next_layer:
            logging.warning("skill: no cluster summaries produced at level %d", level)
            break
        current_layer = next_layer
        level += 1

    roots: list[SkillNode] = current_layer

    # ---- Phase 5: label every node (concurrent), then assign folders.
    all_nodes: list[SkillNode] = []

    def _collect(node: SkillNode) -> None:
        all_nodes.append(node)
        for c in node.children:
            _collect(c)

    for r in roots:
        _collect(r)

    progress(0.75, f"skill: labelling {len(all_nodes)} cluster node(s)")
    label_sem = asyncio.Semaphore(SKILL_LABEL_CONCURRENCY)
    labels_out = await asyncio.gather(
        *(label_skill_node_one(n.summary, chat_mdl, label_sem) for n in all_nodes),
        return_exceptions=False,
    )
    for n, lbl in zip(all_nodes, labels_out):
        n.label = lbl or "cluster"

    # Folder naming: roots get ``skill-NN-<label>``; deeper nodes get
    # ``group-NN-<label>`` keyed by their position in the parent's
    # children list (mirrors Corpus2Skill's skill_builder).
    def _assign_folders(node: SkillNode, idx: int, is_root: bool) -> None:
        slug = skill_safe_name(node.label)
        prefix = f"skill-{idx:02d}" if is_root else f"group-{idx:02d}"
        node.folder_name = f"{prefix}-{slug}" if slug else prefix
        for ci, child in enumerate(node.children):
            _assign_folders(child, ci, is_root=False)

    for ri, root in enumerate(roots):
        _assign_folders(root, ri, is_root=True)

    # Final gate before the destructive delete + bulk insert. This is
    # the most important check — without it a late cancel would still
    # wipe the KB's existing ``skill``/``skill_all`` rows AND spend a
    # full bulk-insert round-trip on data the caller no longer wants.
    if ctx.has_canceled_func(ctx.id):
        progress(-1, "skill: task has been canceled")
        return

    # ---- Phase 6: clean + bulk insert.
    index = search.index_name(ctx.tenant_id)
    try:
        await thread_pool_exec(
            settings.docStoreConn.delete,
            {"compile_kwd": ["skill", "skill_all"]},
            index,
            ctx.kb_id,
        )
    except Exception:
        logging.debug("skill: prior delete failed; relying on id-upsert")

    rows = [skill_node_es_row(ctx, n) for n in all_nodes]
    rows.append(skill_all_es_row(ctx, roots))
    if not rows:
        progress(1.0, "skill: nothing to persist")
        return

    try:
        await thread_pool_exec(
            settings.docStoreConn.insert,
            rows,
            index,
            ctx.kb_id,
        )
    except Exception:
        logging.exception("skill: bulk insert failed (rows=%d)", len(rows))
        return

    progress(
        1.0,
        f"skill: built {len(roots)} top-level skill(s), {len(all_nodes)} total node(s), {len(leaves)} doc(s)",
    )
