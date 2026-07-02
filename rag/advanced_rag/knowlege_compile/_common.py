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
"""Shared helpers for the knowlege_compile pipelines (structure + wiki).

Both ``structure.py`` (compile_structure_from_text / merge_compiled_structures)
and ``wiki.py`` (the MAP→REDUCE→PLAN→REFINE artifact pipeline) need the same set
of plumbing: encode-through-LLMBundle, stable id minting, search-tokenizer
pairs, order-preserving chunk-id unions, defensive LLMBundle validation, the
``chat_mdl.max_length * INPUT_UTILIZATION - prompt_overhead`` token-budget
calculation, and thin ES I/O wrappers.

Anything in this module is meant to be:
  - LLMBundle-aware but provider-agnostic;
  - Safe to import from either pipeline without circular references;
  - Synchronous unless an awaitable behaviour is required.

Heavier shared logic that is conceptually identical but happens to differ in
shape between the two pipelines (e.g. pairwise-cosine dedup, LLM "are these
the same?" batching) intentionally stays in each pipeline file for now —
extract those only when their shapes converge.
"""

from __future__ import annotations

import asyncio
import logging
import string
from typing import Any, Awaitable, Callable, Iterable, Optional

import xxhash

from common.misc_utils import thread_pool_exec
from common.token_utils import num_tokens_from_string
from rag.nlp import rag_tokenizer
from rag.prompts.generator import INPUT_UTILIZATION, gen_json, split_chunks


# ---------------------------------------------------------------------------
# ID minting
# ---------------------------------------------------------------------------


def stable_row_id(*parts) -> str:
    """xxh64 hexdigest of ``":".join(parts)`` — stable per part tuple, used
    as the ES row id when we want idempotent upserts.

    ``None`` parts become empty strings, everything else is ``str()``-ified.
    """
    key = ":".join("" if p is None else str(p) for p in parts)
    return xxhash.xxh64(key.encode("utf-8", "surrogatepass")).hexdigest()


# ---------------------------------------------------------------------------
# Embedding
# ---------------------------------------------------------------------------


async def encode(embd_mdl, texts: list[str]) -> list:
    """``LLMBundle.encode`` wrapped in ``thread_pool_exec``.

    Returns the embeddings list (drops the ``used_tokens`` count); empty
    input returns ``[]``. Caller is responsible for ensuring ``embd_mdl``
    is a real bundle — use :func:`ensure_llm_bundle` to validate at entry.
    """
    if not texts:
        return []
    embeddings, _ = await thread_pool_exec(embd_mdl.encode, texts)
    return list(embeddings)


# ---------------------------------------------------------------------------
# Tokenization for keyword search
# ---------------------------------------------------------------------------


def tokenize_for_search(text: str) -> tuple[str, str]:
    """Returns ``(content_ltks, content_sm_ltks)`` for a piece of text.

    Empty / non-string input returns ``("", "")``. Used wherever we write a
    searchable ES row that needs both tokenizations.
    """
    if not isinstance(text, str) or not text:
        return "", ""
    ltks = rag_tokenizer.tokenize(text)
    if not ltks:
        return "", ""
    sm = rag_tokenizer.fine_grained_tokenize(ltks)
    return ltks, sm


# ---------------------------------------------------------------------------
# Order-preserving union of string lists
# ---------------------------------------------------------------------------


def union_ordered(*lists: Optional[Iterable]) -> list[str]:
    """Concatenate iterables and dedupe, preserving first-seen order.
    Falsy values and non-strings are silently dropped.
    """
    seen_set: set[str] = set()
    seen: list[str] = []
    for lst in lists:
        if not lst:
            continue
        for v in lst:
            if not v or not isinstance(v, str):
                continue
            if v in seen_set:
                continue
            seen_set.add(v)
            seen.append(v)
    return seen


# ---------------------------------------------------------------------------
# Token-budget calculation for split_chunks
# ---------------------------------------------------------------------------


def make_input_budget(
    chat_mdl,
    *prompts: str,
    floor: int = 1024,
    utilization: float = INPUT_UTILIZATION,
) -> int:
    """``chat_mdl.max_length * utilization - num_tokens(sum of prompts)``,
    floored at ``floor``.

    Mirrors the budget idiom used by ``compile_structure_from_text`` and
    ``wiki_map_from_chunks``: caller passes the constant prompt scaffolding
    (system prompt + user template) — ``split_chunks`` then sizes batches
    to leave that much room.
    """
    overhead = num_tokens_from_string("".join(p or "" for p in prompts))
    budget = int(chat_mdl.max_length * utilization) - overhead
    return max(budget, floor)


# ---------------------------------------------------------------------------
# Defensive LLMBundle validation
# ---------------------------------------------------------------------------


def ensure_llm_bundle(mdl, method: str, *, label: str = "model"):
    """Return ``mdl`` if it exposes ``method``; otherwise try to unwrap a
    tuple, otherwise return ``None`` and log an error.

    Common cause for tuple inputs at call sites: ``LLMBundle.encode()`` and
    similar methods return ``(embeddings, used_tokens)``. If a caller stores
    the *result* of ``encode()`` into a variable named like
    ``embedding_model`` and passes that in, we end up with a tuple here.
    We unwrap with a warning so the pipeline keeps working while the caller
    is fixed.
    """
    if hasattr(mdl, method):
        return mdl
    if isinstance(mdl, tuple) and mdl and hasattr(mdl[0], method):
        logging.warning(
            "%s arrived as a %s; unwrapping to first element (check the call site — was %s()'s return value passed instead of the LLMBundle?)",
            label,
            type(mdl).__name__,
            method,
        )
        return mdl[0]
    logging.error(
        "%s has no .%s method (type=%s); aborting",
        label,
        method,
        type(mdl).__name__,
    )
    return None


# ---------------------------------------------------------------------------
# ES I/O wrappers
# ---------------------------------------------------------------------------


async def es_search(
    select_fields: list[str],
    condition: dict,
    *,
    tenant_id: str,
    kb_ids: list[str],
    match_expressions: list | None = None,
    offset: int = 0,
    limit: int = 1000,
    label: str = "es_search",
) -> dict:
    """Thin wrapper around ``docStoreConn.search`` + ``get_fields``.

    Returns ``{row_id: row_dict}``. Returns ``{}`` on failure (with a
    logged exception). ``label`` is included in the failure log so each
    call site is identifiable.
    """
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields,
            [],
            condition,
            match_expressions or [],
            OrderByExpr(),
            offset,
            limit,
            index,
            kb_ids,
        )
        return settings.docStoreConn.get_fields(res, select_fields) or {}
    except Exception:
        logging.exception("%s failed (condition=%r)", label, condition)
        return {}


async def es_insert(
    rows: list[dict],
    tenant_id: str,
    kb_id: str,
    *,
    label: str = "es_insert",
) -> None:
    """Bulk insert wrapped in ``thread_pool_exec``. Logs on failure."""
    if not rows:
        return
    from common import settings
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    try:
        await thread_pool_exec(settings.docStoreConn.insert, rows, index, kb_id)
    except Exception:
        logging.exception("%s failed (%d row(s))", label, len(rows))


async def es_delete(
    condition: dict,
    tenant_id: str,
    kb_id: str,
    *,
    label: str = "es_delete",
) -> None:
    """Bulk delete wrapped in ``thread_pool_exec``. Best-effort; logs on
    failure (some callers rely on id-based upsert as a fallback)."""
    from common import settings
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    try:
        await thread_pool_exec(settings.docStoreConn.delete, condition, index, kb_id)
    except Exception:
        logging.debug("%s failed (condition=%r); caller may rely on id-upsert", label, condition)


async def es_upsert_one(
    filter_condition: dict,
    row: dict,
    tenant_id: str,
    kb_id: str,
    *,
    label: str = "es_upsert_one",
) -> None:
    """Delete-by-filter then insert. Used when an in-place update would
    require knowing the existing row's id and we'd rather drop+re-create.

    Best-effort delete (failures are debug-logged) followed by the insert.
    Set ``row["id"]`` to a stable value derived from the filter
    (:func:`stable_row_id`) so id-based dedup at the connector catches any
    race that bypasses the delete.
    """
    await es_delete(filter_condition, tenant_id, kb_id, label=f"{label}.delete")
    await es_insert([row], tenant_id, kb_id, label=f"{label}.insert")


# ---------------------------------------------------------------------------
# Doc-vector field discovery
# ---------------------------------------------------------------------------


def find_vec_field(doc: dict) -> tuple[Optional[str], Optional[list]]:
    """Locate the ``q_<dim>_vec`` field on an ES doc dict. Returns
    ``(field_name, vec)`` or ``(None, None)`` if the doc carries no
    embedding."""
    for k, v in doc.items():
        if isinstance(k, str) and k.startswith("q_") and k.endswith("_vec"):
            return k, v
    return None, None


# ---------------------------------------------------------------------------
# Chunked-LLM pipeline engine
# ---------------------------------------------------------------------------
#
# Both artifact MAP and compile_structure_from_text follow the same outer shape:
#
#   1. Filter chunks (drop empty text, optionally skip a "resume" set);
#   2. Pack remaining chunks into batches via ``split_chunks`` sized to leave
#      room for the prompt scaffolding;
#   3. Run an LLM-driven ``process_batch`` over each batch in parallel under
#      an ``asyncio.Semaphore(max_workers)``;
#   4. Aggregate the per-batch results into a single value.
#
# The inner LLM call shape diverges between the pipelines — artifact uses a
# single ``gen_json`` per batch with ``[CHUNK_ID Cn]``-labelled bodies,
# structure uses two ``gen_json`` calls (nodes then edges) with ``---``
# separators and no per-chunk attribution. That divergence lives in each
# pipeline's ``process_batch`` closure; this engine only owns the scaffold.


def _default_chunk_text(chunk: dict) -> str:
    if not isinstance(chunk, dict):
        return ""
    text = chunk.get("text") or chunk.get("content_with_weight") or chunk.get("content") or ""
    return text if isinstance(text, str) else ""


def _default_label(position_in_batch: int) -> str:
    return f"C{position_in_batch + 1}"


def build_chunk_batches(
    chunks: list[dict],
    chat_mdl,
    *,
    prompt_overhead_tokens: int,
    resume_chunk_ids: Optional[set[str]] = None,
    scrub_text: Optional[Callable[[str], str]] = None,
    label_fn: Callable[[int], str] = _default_label,
    chunk_text_picker: Optional[Callable[[dict], str]] = None,
    budget_floor: int = 1024,
    batch_size_cap: Optional[int] = None,
    window_fraction: Optional[float] = None,
) -> tuple[list[list[dict]], dict]:
    """Filter chunks, pack into batches, return per-batch entries.

    Each batch entry is ``{"label": str, "chunk_id": str, "text": str}``
    where ``label`` is per-batch positional (default ``C1``, ``C2``, …) and
    ``text`` is the post-scrub chunk body. Empty or resume-skipped chunks
    are dropped.

    Two packing modes:
      - **Default (split_chunks)**: ``input_budget`` derived from
        ``chat_mdl.max_length * INPUT_UTILIZATION - prompt_overhead_tokens``.
        Used by ``structure.py`` and the legacy artifact MAP path.
      - **Cap+fraction (greedy)**: when ``batch_size_cap`` is provided,
        chunks are packed greedily with two cutoffs — chunk-count exceeds
        ``batch_size_cap`` OR accumulated tokens exceed
        ``chat_mdl.max_length * window_fraction``. This is the artifact
        compilation rule (BS=8, window=0.5).

    Returns ``(batches, info)`` where ``info`` is a small stats dict.
    """
    if not chunks:
        return [], {"total": 0, "kept": 0, "skipped_resume": 0, "skipped_empty": 0, "input_budget": 0, "n_batches": 0}

    picker = chunk_text_picker or _default_chunk_text
    resume_set = resume_chunk_ids or set()

    chunk_ids: list[str] = []
    chunk_texts: list[str] = []
    skipped_resume = 0
    skipped_empty = 0

    for chunk in chunks:
        cid = chunk.get("id") or chunk.get("chunk_id")
        if not cid:
            skipped_empty += 1
            continue
        if cid in resume_set:
            skipped_resume += 1
            continue
        text = picker(chunk)
        if not text or not text.strip():
            skipped_empty += 1
            continue
        if scrub_text is not None:
            text = scrub_text(text)
            if not text or not text.strip():
                skipped_empty += 1
                continue
        chunk_ids.append(cid)
        chunk_texts.append(text)

    if not chunk_texts:
        return [], {
            "total": len(chunks),
            "kept": 0,
            "skipped_resume": skipped_resume,
            "skipped_empty": skipped_empty,
            "input_budget": 0,
            "n_batches": 0,
        }

    batches: list[list[dict]] = []
    input_budget: int

    if batch_size_cap is not None:
        # Artifact mode — greedy bin-packing with chunk-count + token caps.
        fraction = window_fraction if window_fraction is not None else 0.5
        token_cap = max(int(chat_mdl.max_length * fraction), budget_floor)
        input_budget = token_cap

        current: list[dict] = []
        current_tks = 0
        for idx, text in enumerate(chunk_texts):
            tks = num_tokens_from_string(text)
            would_overflow_count = len(current) >= batch_size_cap
            would_overflow_tokens = current and (current_tks + tks > token_cap)
            if would_overflow_count or would_overflow_tokens:
                batches.append(current)
                current = []
                current_tks = 0
            current.append(
                {
                    "label": label_fn(len(current)),
                    "chunk_id": chunk_ids[idx],
                    "text": text,
                }
            )
            current_tks += tks
        if current:
            batches.append(current)
    else:
        input_budget = max(
            int(chat_mdl.max_length * INPUT_UTILIZATION) - prompt_overhead_tokens,
            budget_floor,
        )

        raw_batches = split_chunks(chunk_texts, input_budget) or []
        for batch in raw_batches:
            packed: list[dict] = []
            for position, item in enumerate(batch):
                for idx, text in item.items():
                    packed.append(
                        {
                            "label": label_fn(position),
                            "chunk_id": chunk_ids[idx],
                            "text": text,
                        }
                    )
            if packed:
                batches.append(packed)

    info = {
        "total": len(chunks),
        "kept": len(chunk_texts),
        "skipped_resume": skipped_resume,
        "skipped_empty": skipped_empty,
        "input_budget": input_budget,
        "n_batches": len(batches),
    }
    return batches, info


async def run_chunked_pipeline(
    batches: list[list[dict]],
    *,
    process_batch: Callable[..., Awaitable[Any]],
    aggregate: Optional[Callable[[list[Any]], Any]] = None,
    max_workers: int = 6,
    callback: Optional[Callable] = None,
    log_prefix: str = "chunked_pipeline",
) -> Any:
    """Run ``process_batch`` over each batch in parallel.

    ``process_batch`` is called as
    ``await process_batch(entries: list[dict], batch_idx: int, total: int)``
    and may return anything; ``aggregate`` (if given) is called with the
    list of per-batch results and its return value is the engine's return.
    Without ``aggregate`` the raw per-batch results list is returned.

    Cancel-on-error semantics: if any task raises, all sibling tasks are
    cancelled and the exception propagates.
    """
    if not batches:
        return aggregate([]) if aggregate else []

    total = len(batches)
    semaphore = asyncio.Semaphore(max_workers) if max_workers and max_workers > 0 else None

    async def _one(idx: int, entries: list[dict]) -> Any:
        async def _do() -> Any:
            return await process_batch(entries, idx, total)

        if semaphore is not None:
            async with semaphore:
                return await _do()
        return await _do()

    tasks = [asyncio.create_task(_one(i, b)) for i, b in enumerate(batches) if b]
    if not tasks:
        return aggregate([]) if aggregate else []

    try:
        results = await asyncio.gather(*tasks, return_exceptions=False)
    except Exception:
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise

    if callback:
        try:
            callback(1.0, f"{log_prefix}: {total} batch(es) complete")
        except Exception:
            logging.debug("%s: completion callback failed", log_prefix, exc_info=True)

    return aggregate(results) if aggregate else results


# ---------------------------------------------------------------------------
# Bulk dedup engine — exact + embedding + LLM disambiguation
# ---------------------------------------------------------------------------
#
# Replaces wiki's _wiki_exact_dedup_entities / _wiki_exact_dedup_concepts /
# _wiki_embedding_dedup_entities / _wiki_resolve_ambiguous_entities /
# _wiki_apply_merges with one parameterised engine. structure.py's
# merge_compiled_structures uses a different algorithm (incremental
# kept-set + per-pair LLM judgement) and stays as-is.


_PUNCT_TABLE = str.maketrans("", "", string.punctuation)


DEFAULT_DISAMBIGUATE_SYSTEM = "You are a named-entity resolution assistant. Return only JSON."


def normalize_key(name) -> str:
    """Lowercase + strip whitespace + strip ASCII punctuation. Used as the
    bucket key for exact dedup."""
    if not isinstance(name, str):
        return ""
    return name.lower().strip().translate(_PUNCT_TABLE)


def _exact_dedup_by_key(
    items: list[dict],
    *,
    name_key: str,
    type_key: Optional[str] = None,
    aggregate_extra: Optional[Callable[[list[dict]], dict]] = None,
) -> list[dict]:
    """Group items by ``(normalize(item[name_key]), item.get(type_key))``.

    Canonical record per group:
      - ``<name_key>``: the most-common spelling across the group
      - ``<type_key>`` (if given): the group's shared value
      - ``aliases``: sorted union of every name + every input alias, minus
        the canonical name
      - ``mention_count``: sum of input ``mention_count`` values (defaults
        to ``1`` per missing)
      - ``chunk_ids``: order-preserving union
      - ``_norm``: the normalized key (stripped by ``bulk_dedup_items``)
      - any extras from ``aggregate_extra(group)``
    """
    groups: dict[tuple, list[dict]] = {}
    for it in items:
        if not isinstance(it, dict):
            continue
        norm = normalize_key(it.get(name_key, ""))
        if not norm:
            continue
        key = (norm, it.get(type_key) if type_key else None)
        groups.setdefault(key, []).append(it)

    canonical: list[dict] = []
    for (norm, type_val), group in groups.items():
        name_counts: dict[str, int] = {}
        for it in group:
            n = it.get(name_key, "")
            if isinstance(n, str) and n:
                name_counts[n] = name_counts.get(n, 0) + 1
        best = max(name_counts, key=lambda k: name_counts[k]) if name_counts else ""

        aliases: set[str] = set()
        chunk_id_lists: list[list] = []
        mention_count = 0
        for it in group:
            n = it.get(name_key, "")
            if isinstance(n, str) and n:
                aliases.add(n)
            for a in it.get("aliases") or []:
                if isinstance(a, str) and a:
                    aliases.add(a)
            chunk_id_lists.append(it.get("chunk_ids") or [])
            mention_count += int(it.get("mention_count") or 1)
        aliases.discard(best)

        record: dict = {
            name_key: best,
            "aliases": sorted(aliases),
            "mention_count": mention_count,
            "chunk_ids": union_ordered(*chunk_id_lists),
            "_norm": norm,
        }
        if type_key:
            record[type_key] = type_val
        if aggregate_extra is not None:
            try:
                extras = aggregate_extra(group) or {}
                if isinstance(extras, dict):
                    record.update(extras)
            except Exception:
                logging.exception("bulk_dedup: aggregate_extra failed for group %r", norm)
        canonical.append(record)

    return canonical


async def _embedding_dedup(
    canonical: list[dict],
    embd_mdl,
    *,
    name_key: str,
    type_key: Optional[str] = None,
    merge_threshold: float = 0.90,
    ambiguous_low: float = 0.75,
) -> tuple[dict[int, int], list[tuple[int, int]], Optional[list]]:
    """Vectorised pairwise cosine; same-type-only when ``type_key`` given.

    Returns ``(merged_into, ambiguous_pairs, vectors)``. ``merged_into``
    is a union-find map ``index → parent_index``. ``ambiguous_pairs`` is the
    [ambiguous_low, merge_threshold) bucket (after removing pairs already
    linked by auto-merges). ``vectors`` is ``None`` on embedding failure
    (caller should skip dedup).
    """
    n = len(canonical)
    if n <= 1:
        return {}, [], []

    names = [it.get(name_key, "") for it in canonical]
    try:
        vectors = await encode(embd_mdl, names)
    except Exception:
        logging.exception("bulk_dedup: embedding batch failed")
        return {}, [], None
    if vectors is None or len(vectors) != n:
        return {}, [], None

    try:
        from sklearn.metrics.pairwise import cosine_similarity
        import numpy as np

        matrix = np.asarray([list(v) for v in vectors], dtype=float)
        sims = cosine_similarity(matrix)
    except Exception:
        logging.exception("bulk_dedup: pairwise cosine failed; skipping")
        return {}, [], vectors

    merged_into: dict[int, int] = {}

    def _root(i: int) -> int:
        while i in merged_into:
            i = merged_into[i]
        return i

    auto_pairs: list[tuple[int, int]] = []
    ambiguous_pairs: list[tuple[int, int]] = []

    for i in range(n):
        for j in range(i + 1, n):
            if type_key and canonical[i].get(type_key) != canonical[j].get(type_key):
                continue
            s = float(sims[i, j])
            if s >= merge_threshold:
                auto_pairs.append((i, j))
            elif s >= ambiguous_low:
                ambiguous_pairs.append((i, j))

    for i, j in auto_pairs:
        ri, rj = _root(i), _root(j)
        if ri == rj:
            continue
        if canonical[ri].get("mention_count", 0) >= canonical[rj].get("mention_count", 0):
            merged_into[rj] = ri
        else:
            merged_into[ri] = rj

    still_ambiguous = [(i, j) for i, j in ambiguous_pairs if _root(i) != _root(j)]
    return merged_into, still_ambiguous, vectors


async def _resolve_ambiguous_pairs(
    canonical: list[dict],
    ambiguous_pairs: list[tuple[int, int]],
    merged_into: dict[int, int],
    chat_mdl,
    *,
    name_key: str,
    type_key: Optional[str] = None,
    batch_size: int = 50,
    llm_timeout: int = 60,
    system_prompt: str = DEFAULT_DISAMBIGUATE_SYSTEM,
) -> dict[int, int]:
    """LLM-judged disambiguation in batches; returns updated ``merged_into``."""
    if not ambiguous_pairs:
        return merged_into

    def _root(i: int) -> int:
        while i in merged_into:
            i = merged_into[i]
        return i

    for start in range(0, len(ambiguous_pairs), batch_size):
        batch = ambiguous_pairs[start : start + batch_size]
        batch = [(i, j) for i, j in batch if _root(i) != _root(j)]
        if not batch:
            continue

        lines: list[str] = []
        for k, (i, j) in enumerate(batch):
            a_type = f" ({canonical[i].get(type_key, '')})" if type_key else ""
            b_type = f" ({canonical[j].get(type_key, '')})" if type_key else ""
            lines.append(f'{k + 1}. "{canonical[i].get(name_key, "")}"{a_type} vs "{canonical[j].get(name_key, "")}"{b_type}')

        user_prompt = (
            "For each pair below, determine if they refer to the same real-world entity.\n"
            f"Return a JSON array of exactly {len(batch)} booleans "
            "(true = same entity, false = different).\n"
            "Return ONLY the JSON array.\n\n" + "\n".join(lines)
        )

        try:
            res = await asyncio.wait_for(
                gen_json(system_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.0}),
                timeout=llm_timeout,
            )
        except asyncio.TimeoutError:
            logging.warning("bulk_dedup: disambiguation timed out (%d pairs)", len(batch))
            continue
        except Exception:
            logging.exception("bulk_dedup: disambiguation call failed (%d pairs)", len(batch))
            continue

        decisions = None
        if isinstance(res, list):
            decisions = res
        elif isinstance(res, dict):
            for v in res.values():
                if isinstance(v, list):
                    decisions = v
                    break
        if not isinstance(decisions, list):
            logging.warning("bulk_dedup: disambiguation returned unexpected shape: %r", type(res))
            continue

        for k, (i, j) in enumerate(batch):
            verdict = decisions[k] if k < len(decisions) else False
            if not verdict:
                continue
            ri, rj = _root(i), _root(j)
            if ri == rj:
                continue
            if canonical[ri].get("mention_count", 0) >= canonical[rj].get("mention_count", 0):
                merged_into[rj] = ri
            else:
                merged_into[ri] = rj

    return merged_into


def _apply_dedup_merges(
    canonical: list[dict],
    merged_into: dict[int, int],
    *,
    name_key: str,
) -> list[dict]:
    """Union-find collapse: sum ``mention_count``, union ``aliases`` and
    ``chunk_ids`` per canonical."""

    def _root(i: int) -> int:
        while i in merged_into:
            i = merged_into[i]
        return i

    roots: set[int] = {_root(i) for i in range(len(canonical))}
    out: list[dict] = []
    for ri in roots:
        base = dict(canonical[ri])
        aliases: set[str] = set(base.get("aliases") or [])
        chunk_id_lists: list[list] = [base.get("chunk_ids") or []]
        mention_count = int(base.get("mention_count") or 0)
        for i, it in enumerate(canonical):
            if i == ri or _root(i) != ri:
                continue
            mention_count += int(it.get("mention_count") or 0)
            aliases.update(it.get("aliases") or [])
            n = it.get(name_key)
            if isinstance(n, str) and n:
                aliases.add(n)
            chunk_id_lists.append(it.get("chunk_ids") or [])
        aliases.discard(base.get(name_key) or "")
        base["aliases"] = sorted(aliases)
        base["mention_count"] = mention_count
        base["chunk_ids"] = union_ordered(*chunk_id_lists)
        out.append(base)
    return out


async def bulk_dedup_items(
    items: list[dict],
    *,
    name_key: str,
    type_key: Optional[str] = None,
    chat_mdl=None,
    embd_mdl=None,
    merge_threshold: float = 0.90,
    ambiguous_low: float = 0.75,
    ambiguous_batch_size: int = 50,
    disambiguate_system_prompt: str = DEFAULT_DISAMBIGUATE_SYSTEM,
    llm_timeout: int = 60,
    aggregate_extra: Optional[Callable[[list[dict]], dict]] = None,
    strip_norm_key: bool = True,
) -> list[dict]:
    """Three-phase dedup → canonical items.

    Phase 1 (always): exact dedup by ``(normalize(item[name_key]),
    item.get(type_key))`` — groups by normalized key, sums mention_count,
    unions aliases and chunk_ids, optionally adds extras via
    ``aggregate_extra(group)``.

    Phase 2 (when ``embd_mdl`` is provided AND ``len(canonical) > 1``):
    vectorised pairwise cosine over the canonical ``name_key`` values.
    Pairs at similarity ≥ ``merge_threshold`` auto-merge; pairs in
    ``[ambiguous_low, merge_threshold)`` move to phase 3. When ``type_key``
    is given, pairs are only considered when both endpoints share the same
    type. Embedding failures cause this phase (and 3) to be skipped.

    Phase 3 (when ``chat_mdl`` is provided AND ambiguous pairs remain):
    batched LLM disambiguation via ``gen_json`` — each batch asks for a
    JSON array of booleans. True verdicts join the union-find.

    Apply: union-find collapse — sum mention_count, union aliases /
    chunk_ids per canonical.

    Setting both ``chat_mdl`` and ``embd_mdl`` to ``None`` makes this an
    exact-dedup-only call (which is what artifact uses for concepts).
    """
    canonical = _exact_dedup_by_key(
        items,
        name_key=name_key,
        type_key=type_key,
        aggregate_extra=aggregate_extra,
    )

    if len(canonical) > 1 and embd_mdl is not None:
        merged_into, ambig, vectors = await _embedding_dedup(
            canonical,
            embd_mdl,
            name_key=name_key,
            type_key=type_key,
            merge_threshold=merge_threshold,
            ambiguous_low=ambiguous_low,
        )
        if vectors is None:
            logging.warning("bulk_dedup: embedding phase skipped — keeping exact-dedup result")
        else:
            if chat_mdl is not None and ambig:
                merged_into = await _resolve_ambiguous_pairs(
                    canonical,
                    ambig,
                    merged_into,
                    chat_mdl,
                    name_key=name_key,
                    type_key=type_key,
                    batch_size=ambiguous_batch_size,
                    llm_timeout=llm_timeout,
                    system_prompt=disambiguate_system_prompt,
                )
            canonical = _apply_dedup_merges(canonical, merged_into, name_key=name_key)

    if strip_norm_key:
        for it in canonical:
            it.pop("_norm", None)
    return canonical


__all__ = [
    "stable_row_id",
    "encode",
    "tokenize_for_search",
    "union_ordered",
    "make_input_budget",
    "ensure_llm_bundle",
    "es_search",
    "es_insert",
    "es_delete",
    "es_upsert_one",
    "find_vec_field",
    # New engines
    "normalize_key",
    "build_chunk_batches",
    "run_chunked_pipeline",
    "bulk_dedup_items",
    "DEFAULT_DISAMBIGUATE_SYSTEM",
]
