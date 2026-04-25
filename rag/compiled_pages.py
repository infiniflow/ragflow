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
#
"""
Compiled Knowledge Pages
========================
Synthesises wiki-style knowledge pages from document chunks as an optional
retrieval layer that sits above raw chunks.

Processing flow per document:
  chunks  →  section grouping  →  LLM synthesis  →  embed  →  store
Each resulting "compiled page" is stored with:
  - compiled_page_kwd  = "page"
  - compiled_page_type_kwd  = "topic_overview" | "faq" | "entity_summary" | "policy_summary"
  - source_chunk_ids_kwd  = [<chunk_id>, ...]  (for citation tracing)
"""
import logging
from datetime import datetime

import xxhash

from api.db.services.task_service import has_canceled
from common.exceptions import TaskCanceledException
from common.token_utils import num_tokens_from_string
from rag.graphrag.utils import chat_limiter

# Marker field values (must stay in sync with deletion filter in kb_app.py)
COMPILED_PAGE_KWD = "compiled_page_kwd"
COMPILED_PAGE_TYPE_KWD = "compiled_page_type_kwd"
SOURCE_CHUNK_IDS_KWD = "source_chunk_ids_kwd"

# -----------------------------------------------------------------------
# LLM synthesis prompt (system + user are kept separate)
# -----------------------------------------------------------------------
_SYSTEM_PROMPT = """\
You are a knowledge compiler. Your job is to produce a single, \
well-structured knowledge page from the provided excerpts.

Rules:
- Detect the content type automatically:
    * If the excerpts contain Q&A pairs → format as an FAQ page.
    * If they describe policies, rules, or compliance requirements → write a policy summary.
    * If they are about a specific concept, entity, or term → write an entity summary.
    * Otherwise → write a topic overview.
- Start with a one-line title that clearly names the topic.
- Be concise yet complete (200–600 words).
- Preserve numbers, proper nouns, and technical terms exactly.
- Do NOT add information not present in the excerpts."""

_USER_TEMPLATE = "Excerpts:\n{content}\n\nKnowledge page:"


# -----------------------------------------------------------------------
# Helpers
# -----------------------------------------------------------------------

def _detect_page_type(text: str) -> str:
    """Heuristically classify the compiled page type from its synthesised text."""
    lower = text.lower()
    faq_score = sum(1 for sig in ("q:", "q.", "question:", "faq", "frequently asked", "a:", "answer:") if sig in lower)
    policy_score = sum(1 for sig in ("policy", "rule", "regulation", "requirement", "must", "shall", "prohibited", "compliance") if sig in lower)
    entity_score = sum(1 for sig in ("is a", "refers to", "definition", "entity", "term", "concept", "meaning") if sig in lower)
    scores = {"faq": faq_score, "policy_summary": policy_score, "entity_summary": entity_score}
    best_type, best_score = max(scores.items(), key=lambda x: x[1])
    return best_type if best_score > 0 else "topic_overview"


def _group_chunks_into_sections(chunks: list[dict], max_tokens: int = 2048) -> list[list[dict]]:
    """
    Greedily pack consecutive chunks into sections without exceeding *max_tokens*.
    Each section is later condensed into one compiled page.
    """
    sections: list[list[dict]] = []
    current: list[dict] = []
    current_tok = 0

    for chunk in chunks:
        tok = num_tokens_from_string(chunk.get("content_with_weight", ""))
        if current_tok + tok > max_tokens and current:
            sections.append(current)
            current = [chunk]
            current_tok = tok
        else:
            current.append(chunk)
            current_tok += tok

    if current:
        sections.append(current)
    return sections


async def _call_llm(chat_mdl, content: str, task_id: str) -> str | None:
    """Call the LLM to synthesise a compiled page; returns None on failure."""
    if task_id and has_canceled(task_id):
        raise TaskCanceledException(f"Task {task_id} cancelled before LLM call")

    async with chat_limiter:
        try:
            response = await chat_mdl.async_chat(
                _SYSTEM_PROMPT,
                [{"role": "user", "content": _USER_TEMPLATE.format(content=content)}],
                {"temperature": 0.1, "max_tokens": 1024},
            )
        except Exception as exc:
            logging.warning(f"CompiledPages: LLM call failed — {exc}")
            return None

    if response and "**ERROR**" in response:
        logging.warning(f"CompiledPages: LLM returned error marker: {response[:200]}")
        return None

    return response or None


# -----------------------------------------------------------------------
# Main entry point
# -----------------------------------------------------------------------

async def build_compiled_pages(
    row: dict,
    doc_ids: list[str],
    chat_mdl,
    embd_mdl,
    vector_size: int,
    callback=None,
    task_id: str = "",
    max_section_tokens: int = 2048,
) -> tuple[list[dict], int]:
    """
    Build compiled knowledge pages for a knowledge base.

    Parameters
    ----------
    row : dict
        Task row (same shape as used by RAPTOR/GraphRAG handlers in task_executor).
    doc_ids : list[str]
        IDs of source documents to compile.
    chat_mdl : LLMBundle
        Chat model used for page synthesis.
    embd_mdl : LLMBundle
        Embedding model used to embed synthesised pages.
    vector_size : int
        Dimensionality of the embedding vectors.
    callback : callable, optional
        Progress callback ``callback(prog=float, msg=str)``.
    task_id : str
        Redis task ID for cancellation checks.
    max_section_tokens : int
        Maximum token budget per document section / compiled page.

    Returns
    -------
    (pages, token_count) where *pages* is a list of chunk dicts ready for
    insertion into the doc store, and *token_count* is the total tokens
    consumed by the synthesised pages.
    """
    from api.db.services.document_service import DocumentService
    from common import settings
    from rag.nlp import rag_tokenizer

    if callback is None:
        def callback(**_kw): pass

    vctr_nm = f"q_{vector_size}_vec"
    fake_doc_id = row.get("doc_id", "")
    kb_id = str(row["kb_id"])
    tenant_id = row["tenant_id"]

    results: list[dict] = []
    total_tokens = 0
    total_docs = max(len(doc_ids), 1)

    for doc_idx, doc_id in enumerate(doc_ids):
        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} cancelled")

        ok, source_doc = DocumentService.get_by_id(doc_id)
        if not ok or not source_doc:
            logging.warning(f"CompiledPages: document {doc_id} not found, skipping")
            continue

        doc_name = getattr(source_doc, "name", doc_id)
        callback(msg=f"Compiling pages for '{doc_name}' ({doc_idx + 1}/{total_docs})")

        # Retrieve all chunks for this document, ordered by position
        chunks = list(settings.retriever.chunk_list(
            doc_id,
            tenant_id,
            [kb_id],
            fields=["content_with_weight", "id"],
            sort_by_position=True,
        ))

        if not chunks:
            logging.warning(f"CompiledPages: no chunks for document {doc_id}, skipping")
            continue

        sections = _group_chunks_into_sections(chunks, max_section_tokens)
        total_sections = len(sections)

        for sec_idx, section in enumerate(sections):
            if task_id and has_canceled(task_id):
                raise TaskCanceledException(f"Task {task_id} cancelled")

            combined_text = "\n\n".join(c.get("content_with_weight", "") for c in section)
            source_ids = [c["id"] for c in section if "id" in c]

            # LLM synthesis
            synthesised = await _call_llm(chat_mdl, combined_text, task_id)
            if not synthesised:
                logging.warning(
                    f"CompiledPages: synthesis failed for doc '{doc_name}' section {sec_idx}, "
                    "falling back to raw content concatenation"
                )
                synthesised = combined_text

            page_type = _detect_page_type(synthesised)

            # Embed the synthesised page
            try:
                vts, _ = embd_mdl.encode([synthesised])
                vctr = vts[0].tolist()
            except Exception as exc:
                logging.warning(f"CompiledPages: embedding failed for doc '{doc_name}' section {sec_idx}: {exc}")
                continue

            # Build the section label: "Doc name (§1 of 3)"
            if total_sections > 1:
                page_title = f"{doc_name} (§{sec_idx + 1} of {total_sections})"
            else:
                page_title = doc_name

            chunk_id = xxhash.xxh64(
                (synthesised[:200] + doc_id + str(sec_idx)).encode("utf-8")
            ).hexdigest()

            d = {
                "id": chunk_id,
                "doc_id": fake_doc_id,
                "kb_id": [kb_id],
                "docnm_kwd": page_title,
                "title_tks": rag_tokenizer.tokenize(page_title),
                "content_with_weight": synthesised,
                "content_ltks": rag_tokenizer.tokenize(synthesised),
                "content_sm_ltks": rag_tokenizer.fine_grained_tokenize(
                    rag_tokenizer.tokenize(synthesised)
                ),
                vctr_nm: vctr,
                COMPILED_PAGE_KWD: "page",
                COMPILED_PAGE_TYPE_KWD: page_type,
                SOURCE_CHUNK_IDS_KWD: source_ids,
                "available_int": 1,
                "create_time": str(datetime.now()).replace("T", " ")[:19],
                "create_timestamp_flt": datetime.now().timestamp(),
            }
            results.append(d)
            total_tokens += num_tokens_from_string(synthesised)

        callback(prog=(doc_idx + 1.0) / total_docs)

    return results, total_tokens
