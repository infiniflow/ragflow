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

"""Dataset-level navigation markdown for tree-kind compilations.

After a doc finishes a ``tree``-kind compilation template the helper
``upsert_dataset_nav_doc`` here appends (or refreshes) one line in the
KB's nav markdown — one line per doc, each line carrying the doc id +
a short summary lifted from the per-doc tree's root.

Storage: a single ES row per KB under ``compile_kwd="dataset_nav"``,
``available_int=0`` (so retrievers never surface it). The markdown
body lives in ``md_with_weight``; ``doc_count_int`` and ``doc_ids_kwd``
mirror the markdown's order for fast cap-check and dedup.

Concurrency: every write is wrapped in a ``RedisDistributedLock``
keyed by ``f"dataset_nav:{kb_id}"`` — multiple task executors
finishing tree templates for the same KB in parallel must not
interleave their read-modify-writes.

The router/retrieval side that *consumes* this markdown is
intentionally out of scope here.
"""

from __future__ import annotations

import asyncio
import logging
import re
from typing import Any, Iterable

import xxhash

from common.token_utils import num_tokens_from_string
from rag.utils.redis_conn import RedisDistributedLock


# Hard cap on the number of docs we record in the nav markdown.
# Beyond this we no-op on adds; the next doc to drop out of the KB
# frees a slot via ``remove_dataset_nav_doc``.
MAX_DATASET_NAV_DOCS = 128

# Hard cap on the per-doc summary length, in tokens. Long summaries
# bloat the markdown and slow downstream LLM passes that ingest the
# whole nav blob; 128 tokens is enough for 1-2 sentences in either
# Chinese or English text.
MAX_DOC_SUMMARY_TOKENS = 128

_COMPILE_KWD = "dataset_nav"

# Lock TTL — long enough that an ES round-trip can't expire it mid-write
# but short enough that a crashed executor doesn't pin the KB.
_LOCK_TIMEOUT_S = 30
_LOCK_BLOCKING_TIMEOUT_S = 5


def _nav_row_id(kb_id: str) -> str:
    """Stable per-KB row id. Mirrors the pattern used by ``skill_all``."""
    return xxhash.xxh64(
        f"dataset_nav:{kb_id}".encode("utf-8", "surrogatepass"),
    ).hexdigest()


def _nav_lock_key(kb_id: str) -> str:
    return f"dataset_nav:{kb_id}"


# Each line of the markdown looks like ``- **<doc_id>**: <summary>``.
# The ``doc_id`` part is anchored at the start of a bullet so a simple
# regex can locate the line on remove without touching adjacent lines.
_LINE_RE = re.compile(r"^- \*\*([^*]+)\*\*:.*$")


def _format_line(doc_id: str, summary: str) -> str:
    # Strip newlines from the summary so each doc stays on a single
    # markdown line. Multi-line summaries break the dedup regex and
    # confuse downstream consumers that split on ``\n``.
    one_line = summary.replace("\n", " ").replace("\r", " ").strip()
    return f"- **{doc_id}**: {one_line}"


def _truncate_summary(text: str) -> str:
    """Trim ``text`` to ``MAX_DOC_SUMMARY_TOKENS`` tokens.

    Uses the project's tokenizer so the cap matches what the LLM will
    see. Falls back to a generous character cap if tokenization is
    unavailable.
    """
    if not text:
        return ""
    text = text.strip()
    try:
        n = num_tokens_from_string(text)
    except Exception:
        # Best-effort character cap — 4 chars per token is a safe lower
        # bound for English; Chinese is closer to 1 char per token but
        # 4x still keeps the row size sane.
        return text[: MAX_DOC_SUMMARY_TOKENS * 4]
    if n <= MAX_DOC_SUMMARY_TOKENS:
        return text
    # Binary-search the right character cut so we land at exactly the
    # token budget. ``num_tokens_from_string`` is cheap enough that a
    # handful of probes per call is fine.
    lo, hi = 0, len(text)
    while lo < hi:
        mid = (lo + hi + 1) // 2
        try:
            tn = num_tokens_from_string(text[:mid])
        except Exception:
            tn = mid // 4
        if tn <= MAX_DOC_SUMMARY_TOKENS:
            lo = mid
        else:
            hi = mid - 1
    return text[:lo].rstrip()


def _extract_root_summary_from_tree(tree: dict | None) -> str:
    """Pull the doc-level abstract out of a RAPTOR-built tree.

    Convention used by ``_raptor_tree_to_graph``: the root node's
    ``title`` field carries the LLM summary at the highest layer.
    Internal nodes lower in the tree carry their own per-cluster
    summaries. We just take the root.
    """
    if not isinstance(tree, dict):
        return ""
    title = tree.get("title") or ""
    if isinstance(title, str) and title.strip():
        return title.strip()
    # Some RAPTOR shapes use ``summary`` or ``content`` instead.
    for alt in ("summary", "content_with_weight", "content"):
        v = tree.get(alt)
        if isinstance(v, str) and v.strip():
            return v.strip()
    return ""


def _parse_existing_lines(md: str) -> list[tuple[str, str]]:
    """Return ``(doc_id, raw_line)`` tuples in markdown order.

    We keep the *raw* line so callers that just want to update one
    doc's line don't have to re-derive the formatting. Lines that
    don't match the per-doc shape (e.g. headers, blank lines) are
    skipped — they're never written by this module, but a future
    schema bump might add them and we shouldn't crash on it.
    """
    out: list[tuple[str, str]] = []
    if not md:
        return out
    for line in md.splitlines():
        m = _LINE_RE.match(line)
        if not m:
            continue
        out.append((m.group(1), line))
    return out


def _render_md(entries: Iterable[tuple[str, str]]) -> str:
    return "\n".join(line for _doc_id, line in entries)


def _row_id_field(row: dict | None) -> dict:
    if row and isinstance(row, dict):
        return row
    return {}


async def _get_existing(tenant_id: str, kb_id: str) -> dict | None:
    """Read the existing nav row, or ``None`` if it doesn't exist yet."""
    from common import settings
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return None
    try:
        existing = await thread_pool_exec(
            settings.docStoreConn.get,
            _nav_row_id(kb_id),
            index,
            [kb_id],
        )
    except Exception:
        logging.exception(
            "dataset_nav: read failed for kb=%s",
            kb_id,
        )
        return None
    return _row_id_field(existing) or None


async def _write_row(tenant_id: str, kb_id: str, payload: dict) -> None:
    """Upsert the nav row in the doc store."""
    from common import settings
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    row_id = _nav_row_id(kb_id)
    payload = {
        "id": row_id,
        "kb_id": kb_id,
        "doc_id": kb_id,
        "compile_kwd": _COMPILE_KWD,
        "knowledge_graph_kwd": "graph",
        "available_int": 0,
        **payload,
    }
    existing = await thread_pool_exec(
        settings.docStoreConn.get,
        row_id,
        index,
        [kb_id],
    )
    if existing:
        await thread_pool_exec(
            settings.docStoreConn.update,
            {"id": row_id},
            {k: v for k, v in payload.items() if k != "id"},
            index,
            kb_id,
        )
    else:
        await thread_pool_exec(
            settings.docStoreConn.insert,
            [payload],
            index,
            kb_id,
        )


# --------------------------------------------------------------------
# Public surface
# --------------------------------------------------------------------


async def upsert_dataset_nav_doc(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    summary_or_tree: Any,
) -> None:
    """Add or refresh a doc's line in the KB's nav markdown.

    ``summary_or_tree`` can be:
      - a plain string (taken as-is and truncated to ``MAX_DOC_SUMMARY_TOKENS``)
      - a tree dict (the root summary is extracted via
        ``_extract_root_summary_from_tree``)

    The 128-doc cap is enforced here: if the doc isn't already in the
    markdown and the row is full, the call is a no-op. Existing docs
    always get their summary updated regardless of count.

    Called from ``_run_tree_templates`` after each successful
    ``_struct_upsert_graph_json``.
    """
    if not doc_id or not kb_id:
        return

    if isinstance(summary_or_tree, dict):
        summary = _extract_root_summary_from_tree(summary_or_tree)
    elif isinstance(summary_or_tree, str):
        summary = summary_or_tree
    else:
        summary = ""
    summary = _truncate_summary(summary)
    if not summary:
        # Nothing to record — a tree with no root summary means the
        # RAPTOR pass produced a degenerate result; safer to skip than
        # to write an empty line.
        logging.info(
            "dataset_nav: skipping doc=%s (kb=%s) — no usable summary",
            doc_id,
            kb_id,
        )
        return

    new_line = _format_line(doc_id, summary)
    lock = RedisDistributedLock(
        _nav_lock_key(kb_id),
        timeout=_LOCK_TIMEOUT_S,
        blocking_timeout=_LOCK_BLOCKING_TIMEOUT_S,
    )
    try:
        await lock.spin_acquire()
    except Exception:
        logging.exception(
            "dataset_nav: lock acquire failed for kb=%s; proceeding lock-free",
            kb_id,
        )

    try:
        existing = await _get_existing(tenant_id, kb_id)
        md = (existing or {}).get("md_with_weight") or ""
        entries = _parse_existing_lines(md)

        replaced = False
        for i, (existing_doc_id, _) in enumerate(entries):
            if existing_doc_id == doc_id:
                entries[i] = (doc_id, new_line)
                replaced = True
                break
        if not replaced:
            if len(entries) >= MAX_DATASET_NAV_DOCS:
                logging.info(
                    "dataset_nav: kb=%s already at cap (%d); skipping doc=%s",
                    kb_id,
                    MAX_DATASET_NAV_DOCS,
                    doc_id,
                )
                return
            entries.append((doc_id, new_line))

        payload = {
            "md_with_weight": _render_md(entries),
            "doc_count_int": len(entries),
            "doc_ids_kwd": [doc_id for doc_id, _ in entries],
        }
        try:
            await _write_row(tenant_id, kb_id, payload)
        except Exception:
            logging.exception(
                "dataset_nav: write failed for kb=%s doc=%s",
                kb_id,
                doc_id,
            )
    finally:
        try:
            lock.release()
        except Exception:
            logging.exception("dataset_nav: lock release failed for kb=%s", kb_id)


async def remove_dataset_nav_doc(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
) -> None:
    """Remove ``doc_id``'s line from the KB's nav markdown.

    Called from ``DocumentService.remove_document`` so the markdown
    stays in sync with the KB's doc set. No-op if the row doesn't
    exist or the doc isn't represented in it.
    """
    if not doc_id or not kb_id:
        return

    lock = RedisDistributedLock(
        _nav_lock_key(kb_id),
        timeout=_LOCK_TIMEOUT_S,
        blocking_timeout=_LOCK_BLOCKING_TIMEOUT_S,
    )
    try:
        await lock.spin_acquire()
    except Exception:
        logging.exception(
            "dataset_nav: lock acquire failed for kb=%s; proceeding lock-free",
            kb_id,
        )

    try:
        existing = await _get_existing(tenant_id, kb_id)
        if not existing:
            return
        md = existing.get("md_with_weight") or ""
        entries = _parse_existing_lines(md)
        before = len(entries)
        entries = [(d, line) for (d, line) in entries if d != doc_id]
        if len(entries) == before:
            return

        payload = {
            "md_with_weight": _render_md(entries),
            "doc_count_int": len(entries),
            "doc_ids_kwd": [d for d, _ in entries],
        }
        try:
            await _write_row(tenant_id, kb_id, payload)
        except Exception:
            logging.exception(
                "dataset_nav: remove-write failed for kb=%s doc=%s",
                kb_id,
                doc_id,
            )
    finally:
        try:
            lock.release()
        except Exception:
            logging.exception("dataset_nav: lock release failed for kb=%s", kb_id)


def remove_dataset_nav_doc_sync(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
) -> None:
    """Sync wrapper around ``remove_dataset_nav_doc``.

    ``DocumentService.remove_document`` is synchronous (Peewee-driven)
    and the doc-store helpers it calls are sync too. We need a sync
    bridge so the delete path can invoke this without spinning up an
    event loop.

    Strategy: run the async helper on the current loop if one is
    available; otherwise spin a fresh loop for the duration of the
    call. Any failure is logged and swallowed — the doc-delete path
    must never fail because of nav-md cleanup.
    """
    try:
        loop = asyncio.new_event_loop()
        try:
            loop.run_until_complete(
                remove_dataset_nav_doc(tenant_id, kb_id, doc_id),
            )
        finally:
            loop.close()
    except Exception:
        logging.exception(
            "dataset_nav: sync remove failed for kb=%s doc=%s",
            kb_id,
            doc_id,
        )
