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
"""OpenFable-style tree builder for page_index templates.

Replaces the ``batch_size_cap=1`` / one-chunk-at-a-time extraction with a
tree-first approach: the LLM sees ALL chunks (or a coherent partition for
long documents) and builds a complete 4-level table-of-contents tree in one
pass, then converts tree nodes → entities and parent→child edges → relations.

Strategy
--------
- **Short docs** (total tokens ≤ 70% context window): single-pass TreeBuild
  — 1 LLM call produces the complete tree.
- **Long docs** (> 70% context window): progressive build —
  sequential partitions → per-partition 3-level TreeBuild →
  1 TreeMerge call (summary-only, very cheap) → unified 4-level tree.

Output
------
The same list of ES-ready dicts that ``compile_structure_from_text``
produces (entity + relation rows via ``_struct_to_doc_storage_doc``), so
downstream merge / dataset_nav code requires zero changes.
"""

from __future__ import annotations

import logging
from collections import Counter

from common.token_utils import num_tokens_from_string
from rag.prompts.generator import gen_json

from .structure import (
    _struct_embed,
    _struct_relation_member_fields,
    _struct_to_doc_storage_doc,
)

logger = logging.getLogger(__name__)

# ── Constants ────────────────────────────────────────────────────────────────

_CONTENT_BUDGET_FRACTION = 0.70  # 70% of model context window for chunks
_MAX_DEPTH = 4  # max tree depth (root → section → subsection → leaf)
_DEFAULT_CONTEXT_WINDOW = 100_000  # fallback when model doesn't advertise max_length

# ── Prompts ──────────────────────────────────────────────────────────────────

TREE_BUILD_SYSTEM = """\
You are a robust table-of-contents builder. Given numbered text chunks from a
document, organize them into a hierarchical JSON tree.

TREE RULES:
- Every chunk_index (0..N-1) MUST appear exactly once as a leaf node.
- Extract actual headings from the text when present (第x章, 1., 1.1, etc.).
- title: clean heading text, ≤25 Chinese or ≤80 English characters.
- summary: 1-2 compressed sentences. Keep only the 2-4 most important facts:
  key numbers, dates, names, locations, and conclusions. Aim for 50-100 English
  words or 80-180 Chinese characters.
- For a parent node with no direct body before the next child heading,
  synthesize a brief higher-level overview from the child-section content;
  do NOT copy a child's summary verbatim.
- Do NOT invent headings not present in the text. If a chunk has no clear
  heading, use a short descriptive label derived from its content.
- Maximum depth: 4 levels (root > section > subsection > leaf).
- Do not reduce descriptions to generic labels like "heading for X" or
  "section heading". Write actual compressed content summaries.

OUTPUT: exactly one JSON object:
{
  "root": {
    "type": "internal",
    "node_type": "root|section|subsection",
    "title": "the exact heading text",
    "summary": "1-2 sentence compressed summary",
    "children": [
      {"type": "internal", "node_type": "section", "title": "...", "summary": "...",
       "children": [{"type": "leaf", "chunk_index": 0}, ...]},
      {"type": "leaf", "chunk_index": 5},
      ...
    ]
  }
}

Return ONLY the JSON object, no other text."""

TREE_BUILD_PARTITION_SYSTEM = """\
You are a table-of-contents builder for one PARTITION of a larger document.

Same rules as full-document tree building, but:
- Build a 3-level tree: root > section > leaf (NO subsection level).
- The root should describe this partition's content scope.
- Every chunk_index (relative to this partition: 0..N-1) MUST appear exactly
  once as a leaf.

OUTPUT: exactly one JSON object:
{
  "root": {
    "type": "internal",
    "node_type": "root|section",
    "title": "partition heading",
    "summary": "1-2 sentence summary of this partition",
    "children": [
      {"type": "internal", "node_type": "section", "title": "...", "summary": "...",
       "children": [{"type": "leaf", "chunk_index": 0}, ...]},
      {"type": "leaf", "chunk_index": 3},
      ...
    ]
  }
}

Return ONLY the JSON object, no other text."""

TREE_MERGE_SYSTEM = """\
You are merging partial table-of-contents summaries from multiple partitions
of the same document into one unified root.

Each summary below describes one partition's overall content. Create a unified
document title and a comprehensive summary that covers ALL partitions.

OUTPUT: exactly one JSON object:
{"merged_title": "unified document title", "merged_summary": "2-3 sentence overview covering all parts"}

Return ONLY the JSON object, no other text."""


# ── Content budget ──────────────────────────────────────────────────────────


def _content_budget(chat_mdl) -> int:
    """Return the maximum token budget for chunk content in one LLM call."""
    max_tokens = getattr(chat_mdl, "max_length", None)
    if not max_tokens:
        max_tokens = _DEFAULT_CONTEXT_WINDOW
    return int(max_tokens * _CONTENT_BUDGET_FRACTION)


# ── Partitioning ────────────────────────────────────────────────────────────


def _partition_by_budget(
    chunks: list[dict],
    content_budget: int,
) -> list[list[dict]]:
    """Split chunks into sequential, non-overlapping groups ≤ ``content_budget`` tokens each.

    Uses greedy bin-packing: start a new partition when adding the next chunk
    would exceed the budget.
    """
    partitions: list[list[dict]] = []
    current: list[dict] = []
    current_tokens = 0

    for ch in chunks:
        text = ch.get("text") or ch.get("content_with_weight") or ""
        tks = num_tokens_from_string(text)
        if current_tokens + tks > content_budget and current:
            partitions.append(current)
            current = [ch]
            current_tokens = tks
        else:
            current.append(ch)
            current_tokens += tks

    if current:
        partitions.append(current)
    return partitions


# ── Validation ──────────────────────────────────────────────────────────────


def _validate_coverage(root: dict, num_chunks: int) -> None:
    """Ensure every chunk_index 0..num_chunks-1 appears exactly once in the tree."""
    all_indexes: list[int] = []

    def _collect(node: dict) -> None:
        if node.get("type") == "leaf":
            idx = node.get("chunk_index")
            if isinstance(idx, int):
                all_indexes.append(idx)
        else:
            for child in node.get("children") or []:
                _collect(child)

    _collect(root)

    expected = set(range(num_chunks))
    found = set(all_indexes)

    missing = sorted(expected - found)
    extra = sorted(found - expected)
    if missing:
        raise PageIndexTreeError(f"Missing chunk indexes: {missing}")
    if extra:
        raise PageIndexTreeError(f"Unexpected extra chunk indexes: {extra}")

    dupes = sorted(idx for idx, cnt in Counter(all_indexes).items() if cnt > 1)
    if dupes:
        raise PageIndexTreeError(f"Duplicate chunk indexes: {dupes}")


# ── Chunk-id mapping ────────────────────────────────────────────────────────


def _build_chunk_id_map(chunks: list[dict]) -> list[str]:
    """Build a list mapping chunk_index → chunk_id.

    Returns [cid or "" for each chunk in order].
    """
    result: list[str] = []
    for ch in chunks:
        cid = ch.get("id") or ch.get("chunk_id")
        result.append(cid if isinstance(cid, str) else "")
    return result


def _collect_subtree_chunk_ids(
    node: dict,
    chunk_id_map: list[str],
) -> list[str]:
    """Walk a subtree and collect all leaf chunk_ids."""
    ids: list[str] = []

    def _walk(n: dict) -> None:
        if n.get("type") == "leaf":
            idx = n.get("chunk_index")
            if isinstance(idx, int) and 0 <= idx < len(chunk_id_map):
                cid = chunk_id_map[idx]
                if cid:
                    ids.append(cid)
        else:
            for child in n.get("children") or []:
                _walk(child)

    _walk(node)
    # Order-preserving dedup
    seen: set[str] = set()
    result: list[str] = []
    for cid in ids:
        if cid not in seen:
            seen.add(cid)
            result.append(cid)
    return result


# ── Single-pass tree build (short documents) ────────────────────────────────


async def _single_pass_build(
    chunks: list[dict],
    chat_mdl,
) -> dict:
    """One LLM call: all chunks → complete 4-level tree."""
    chunk_lines: list[str] = []
    for i, ch in enumerate(chunks):
        text = ch.get("text") or ch.get("content_with_weight") or ""
        chunk_lines.append(f"Chunk {i}:\n{text}")

    user_msg = f"{len(chunks)} chunks (indexes 0 to {len(chunks) - 1}). Every index MUST appear exactly once as a leaf.\n\n" + "\n\n".join(chunk_lines)

    gen_conf = {"temperature": 0.0, "response_format": {"type": "json_object"}}

    try:
        raw = await gen_json(TREE_BUILD_SYSTEM, user_msg, chat_mdl, gen_conf=gen_conf)
    except Exception:
        logger.exception("page_index_tree: single-pass TreeBuild LLM call failed")
        raise

    root = raw.get("root") if isinstance(raw, dict) else None
    if not isinstance(root, dict):
        raise PageIndexTreeError("LLM did not return a valid root node")

    _validate_coverage(root, len(chunks))
    return root


# ── Progressive build (long documents) ──────────────────────────────────────


async def _build_partition_tree(
    part_chunks: list[dict],
    chat_mdl,
    part_idx: int,
) -> dict:
    """Build a 3-level tree (root → section → leaf) for one partition."""
    chunk_lines: list[str] = []
    for i, ch in enumerate(part_chunks):
        text = ch.get("text") or ch.get("content_with_weight") or ""
        chunk_lines.append(f"Chunk {i}:\n{text}")

    user_msg = f"Partition {part_idx}: {len(part_chunks)} chunks (indexes 0 to {len(part_chunks) - 1}).\n\n" + "\n\n".join(chunk_lines)

    gen_conf = {"temperature": 0.0, "response_format": {"type": "json_object"}}

    try:
        raw = await gen_json(
            TREE_BUILD_PARTITION_SYSTEM,
            user_msg,
            chat_mdl,
            gen_conf=gen_conf,
        )
    except Exception:
        logger.exception("page_index_tree: partition %d TreeBuild LLM call failed", part_idx)
        raise

    root = raw.get("root") if isinstance(raw, dict) else None
    if not isinstance(root, dict):
        raise PageIndexTreeError(f"Partition {part_idx}: LLM did not return a valid root node")

    _validate_coverage(root, len(part_chunks))
    return root


async def _merge_partition_roots(
    partial_roots: list[dict],
    chat_mdl,
) -> dict:
    """LLM creates a unified root from partition summaries only (very cheap call)."""
    summaries_text = "\n\n".join(f"Part {i}: Title: {r.get('title', '')}, Summary: {r.get('summary', '')}" for i, r in enumerate(partial_roots))

    gen_conf = {"temperature": 0.0, "response_format": {"type": "json_object"}}

    try:
        merged = await gen_json(
            TREE_MERGE_SYSTEM,
            f"Partial tree summaries:\n\n{summaries_text}",
            chat_mdl,
            gen_conf=gen_conf,
        )
    except Exception:
        logger.exception("page_index_tree: TreeMerge LLM call failed")
        raise

    if not isinstance(merged, dict):
        raise PageIndexTreeError("TreeMerge: LLM did not return a valid JSON object")

    return {
        "type": "internal",
        "node_type": "root",
        "title": merged.get("merged_title", "Document"),
        "summary": merged.get("merged_summary", ""),
        "children": [],
    }


async def _progressive_build(
    chunks: list[dict],
    chat_mdl,
    content_budget: int,
) -> tuple[dict, list[list[dict]]]:
    """Partition → per-partition TreeBuild → TreeMerge → unified tree.

    Returns (unified_root, partition_chunk_lists) where partition_chunk_lists[i]
    maps each partition's chunk_index back to the original chunks dicts.
    """
    partitions = _partition_by_budget(chunks, content_budget)
    logger.info(
        "page_index_tree: progressive build — %d chunks in %d partitions",
        len(chunks),
        len(partitions),
    )

    # Step 1: Build each partition tree (3-level, no subsection).
    partial_roots: list[dict] = []
    for part_idx, part_chunks in enumerate(partitions):
        root = await _build_partition_tree(part_chunks, chat_mdl, part_idx)
        partial_roots.append(root)

    # Step 2: TreeMerge — unified root from partition summaries.
    merged_root = await _merge_partition_roots(partial_roots, chat_mdl)

    # Step 3: Attach partition roots as children of unified root.
    # Each partition root becomes a "section" under the unified root.
    merged_root["children"] = []
    for part_idx, part_root in enumerate(partial_roots):
        part_root["node_type"] = "section"
        merged_root["children"].append(part_root)

    return merged_root, partitions


# ── Tree → ES docs conversion ───────────────────────────────────────────────


async def _tree_to_es_docs(
    root: dict,
    chunks: list[dict],
    chunk_id_map: list[str],
    embd_mdl,
    doc_id: str,
    compilation_template_id: str | None,
    src_field: str | None,
    target_field: str | None,
) -> list[dict]:
    """Walk the tree DFS, emit entity + include-relation ES docs.

    Each internal node → entity (type="title").
    Each parent→child edge → relation (type="include").
    Every entity carries the union of all chunk_ids from its subtree.
    """
    entities: list[dict] = []
    relations: list[dict] = []

    def _walk(node: dict, parent_title: str | None) -> None:
        if node.get("type") == "leaf":
            return

        title = (node.get("title") or "").strip()
        summary = (node.get("summary") or "").strip()

        if title or summary:
            entity_payload = {
                "type": "title",
                "name": title or "-1",
                "description": summary,
            }
            entities.append(entity_payload)

            if parent_title:
                relations.append(
                    {
                        "type": "include",
                        "source": parent_title,
                        "target": title,
                        "description": f"'{parent_title}' includes '{title}'",
                    }
                )

        next_parent = title if title else parent_title
        for child in node.get("children") or []:
            _walk(child, next_parent)

    _walk(root, None)

    # ── Embed all payloads ────────────────────────────────────────────────
    all_payloads: list[dict] = entities + relations
    if not all_payloads:
        return []

    embed_texts: list[str] = []
    for p in all_payloads:
        if "name" in p:
            # Entity: embed name + description
            embed_texts.append(f"{p.get('name', '')}: {p.get('description', '')}")
        else:
            # Relation: embed source → target
            embed_texts.append(f"{p.get('source', '')} includes {p.get('target', '')}")

    try:
        embeddings = await _struct_embed(embd_mdl, embed_texts)
    except Exception:
        logger.exception("page_index_tree: embedding failed")
        return []

    if len(embeddings) != len(all_payloads):
        logger.error(
            "page_index_tree: embedding count mismatch (%d vs %d)",
            len(embeddings),
            len(all_payloads),
        )
        return []

    # ── Build ES docs ─────────────────────────────────────────────────────

    # Pre-compute subtree chunk_ids for each internal node. We walk the tree
    # once and assign chunk_ids to each entity based on its position.
    subtree_chunk_ids: list[list[str]] = []

    def _collect_subtree_ids(node: dict) -> list[str]:
        """Return chunk_ids for this node's subtree, appending to the global list for entities."""
        if node.get("type") == "leaf":
            idx = node.get("chunk_index")
            if isinstance(idx, int) and 0 <= idx < len(chunk_id_map):
                cid = chunk_id_map[idx]
                return [cid] if cid else []
            return []
        ids_list: list[str] = []
        for child in node.get("children") or []:
            ids_list.extend(_collect_subtree_ids(child))
        # Dedup preserving order
        seen: set[str] = set()
        deduped: list[str] = []
        for cid in ids_list:
            if cid and cid not in seen:
                seen.add(cid)
                deduped.append(cid)
        subtree_chunk_ids.append(deduped)
        return deduped

    _collect_subtree_ids(root)

    # subtree_chunk_ids now has one entry per internal node, in DFS order.
    # entities list is also in DFS order. Zip them.
    es_docs: list[dict] = []
    ei = 0  # entity index into subtree_chunk_ids
    for i, payload in enumerate(all_payloads):
        is_entity = "name" in payload
        if is_entity:
            chunk_ids = subtree_chunk_ids[ei] if ei < len(subtree_chunk_ids) else []
            ei += 1
        else:
            # Relation: use the child entity's chunk_ids (next entity in DFS).
            # Since relations and entities are interleaved in DFS order
            # (entity → its children → next sibling entity), we use the chunk_ids
            # from the entity at position ei (the child we just walked into).
            # Fall back to empty if out of bounds.
            chunk_ids = subtree_chunk_ids[ei] if ei < len(subtree_chunk_ids) else []

        es_docs.append(
            _struct_to_doc_storage_doc(
                payload,
                "page_index",  # compile_kwd — matches what compile_structure_from_text uses
                doc_id,
                chunk_ids,
                embeddings[i],
                "entity" if is_entity else "relation",
                src_field=src_field,
                target_field=target_field,
                compilation_template_id=compilation_template_id,
                compilation_template_kind="page_index",
            )
        )

    return es_docs


# ── Public API ───────────────────────────────────────────────────────────────


async def build_page_index_tree(
    chunks: list[dict],
    chat_mdl,
    embd_mdl,
    doc_id: str,
    parser_config: dict,
    *,
    compilation_template_id: str | None = None,
) -> list[dict]:
    """Build a page_index tree and return ES-ready doc dicts.

    Drop-in replacement for ``compile_structure_from_text`` when the template
    kind is ``page_index``. The output format is identical — a list of dicts
    each containing ``content_with_weight`` (JSON payload), ``compile_kwd``,
    ``knowledge_graph_kwd``, embedding vector, and metadata.

    Args:
        chunks: list of dicts; each must expose ``id`` (or ``chunk_id``) and
            text (``content_with_weight`` or ``text`` field).
        chat_mdl: LLMBundle for chat (used via ``gen_json``).
        embd_mdl: LLMBundle for embeddings.
        doc_id: source document id, embedded into every ES doc.
        parser_config: the template config dict (same as passed to
            ``compile_structure_from_text``).
        compilation_template_id: stamped onto every output row for filtering.

    Returns:
        List of ES-ready dicts in the same shape as ``compile_structure_from_text``.
        Returns ``[]`` on empty input or complete failure.
    """
    if not chunks:
        return []

    # Resolve source/target field names for relation docs.
    src_field, target_field = _struct_relation_member_fields(parser_config)

    chat_mdl_ok = getattr(chat_mdl, "max_length", None)
    if not chat_mdl_ok:
        logger.warning(
            "page_index_tree: chat_mdl has no max_length; using default %d",
            _DEFAULT_CONTEXT_WINDOW,
        )

    budget = _content_budget(chat_mdl)
    total_tokens = sum(num_tokens_from_string(c.get("text") or c.get("content_with_weight") or "") for c in chunks)

    # Build the chunk_id map (chunk_index → chunk_id string).
    chunk_id_map = _build_chunk_id_map(chunks)

    try:
        if total_tokens <= budget:
            logger.info(
                "page_index_tree: single-pass build — %d chunks, ~%d tokens",
                len(chunks),
                total_tokens,
            )
            tree_root = await _single_pass_build(chunks, chat_mdl)
        else:
            logger.info(
                "page_index_tree: progressive build — %d chunks, ~%d tokens (budget=%d)",
                len(chunks),
                total_tokens,
                budget,
            )
            tree_root, _ = await _progressive_build(chunks, chat_mdl, budget)
    except PageIndexTreeError:
        logger.exception("page_index_tree: tree construction failed")
        return []
    except Exception:
        logger.exception("page_index_tree: unexpected failure")
        return []

    # Convert tree → ES docs.
    try:
        es_docs = await _tree_to_es_docs(
            tree_root,
            chunks,
            chunk_id_map,
            embd_mdl,
            doc_id,
            compilation_template_id,
            src_field,
            target_field,
        )
    except Exception:
        logger.exception("page_index_tree: tree-to-ES conversion failed")
        return []

    logger.info(
        "page_index_tree: produced %d ES docs (%d entities, %d relations)",
        len(es_docs),
        sum(1 for d in es_docs if d.get("knowledge_graph_kwd") == "entity"),
        sum(1 for d in es_docs if d.get("knowledge_graph_kwd") == "relation"),
    )
    return es_docs


class PageIndexTreeError(Exception):
    """Tree construction or validation failure."""

    pass
