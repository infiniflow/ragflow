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

"""
Chunk Post-Processor Module.

Provides post-processing functions for chunks:
- Keyword extraction
- Question generation
- Metadata generation
- Content tagging
"""

import asyncio
import json
import logging
import random
import re
from timeit import default_timer as timer
from typing import Dict, List

from common.constants import TAG_FLD, LLMType
from common.metadata_utils import turn2jsonschema, update_metadata_to
from common import settings
from rag.nlp import rag_tokenizer
from rag.svr.task_executor_refactor.task_context import TaskContext

from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.llm_service import LLMBundle
from api.db.joint_services.tenant_model_service import get_model_config_from_provider_instance
from rag.prompts.generator import gen_metadata, keyword_extraction, question_proposal, content_tagging
from rag.graphrag.utils import get_llm_cache, set_llm_cache, get_tags_from_cache, set_tags_to_cache


async def extract_keywords(docs: List[Dict], ctx: TaskContext) -> None:
    """Extract keywords for chunks.

    Args:
        docs: List of chunk dictionaries to process.
        ctx: TaskContext containing task configuration.
    """
    chat_limiter = ctx.chat_limiter

    st = timer()
    ctx.progress_cb(msg="Start to generate keywords for every chunk ...")
    chat_model_config = get_model_config_from_provider_instance(ctx.tenant_id, LLMType.CHAT, ctx.llm_id)
    with LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language) as chat_model:

        async def doc_keyword_extraction(chat_mdl, d, topn):
            cached = get_llm_cache(chat_mdl.llm_name, d["content_with_weight"], "keywords", {"topn": topn})
            if not cached:
                if ctx.has_canceled_func(ctx.id):
                    ctx.progress_cb(-1, msg="Task has been canceled.")
                    return
                async with chat_limiter:
                    cached = await keyword_extraction(chat_mdl, d["content_with_weight"], topn)
                set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, "keywords", {"topn": topn})
            if cached:
                d["important_kwd"] = [k for k in re.split(r"[,，;；、\r\n]+", cached) if k.strip()]
                d["important_tks"] = rag_tokenizer.tokenize(" ".join(d["important_kwd"]))
            return

        tasks = []
        for doc in docs:
            tasks.append(asyncio.create_task(doc_keyword_extraction(chat_model, doc, ctx.parser_config["auto_keywords"])))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error("Error in doc_keyword_extraction: {}".format(e))
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise
        ctx.progress_cb(msg="Keywords generation {} chunks completed in {:.2f}s".format(len(docs), timer() - st))


async def generate_questions(docs: List[Dict], ctx: TaskContext) -> None:
    """Generate questions for chunks.

    Args:
        docs: List of chunk dictionaries to process.
        ctx: TaskContext containing task configuration.
    """
    chat_limiter = ctx.chat_limiter

    st = timer()
    ctx.progress_cb(msg="Start to generate questions for every chunk ...")
    chat_model_config = get_model_config_from_provider_instance(ctx.tenant_id, LLMType.CHAT, ctx.llm_id)
    with LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language) as chat_model:

        async def doc_question_proposal(chat_mdl, d, topn):
            cached = get_llm_cache(chat_mdl.llm_name, d["content_with_weight"], "question", {"topn": topn})
            if not cached:
                if ctx.has_canceled_func(ctx.id):
                    ctx.progress_cb(-1, msg="Task has been canceled.")
                    return
                async with chat_limiter:
                    cached = await question_proposal(chat_mdl, d["content_with_weight"], topn)
                set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, "question", {"topn": topn})
            if cached:
                d["question_kwd"] = cached.split("\n")
                d["question_tks"] = rag_tokenizer.tokenize("\n".join(d["question_kwd"]))

        tasks = []
        for doc in docs:
            tasks.append(asyncio.create_task(doc_question_proposal(chat_model, doc, ctx.parser_config["auto_questions"])))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error("Error in doc_question_proposal", exc_info=e)
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise
        ctx.progress_cb(msg="Question generation {} chunks completed in {:.2f}s".format(len(docs), timer() - st))


def build_metadata_config(parser_config: dict) -> list:
    """Build the metadata configuration from parser_config.

    Extracts and normalizes ``metadata`` and ``built_in_metadata`` from the
    parser configuration into a single list or dict that is passed to the LLM
    cache and generation functions.

    This should be called once per ``generate_metadata`` invocation — the result
    is identical for every chunk within the same document parse session so
    extracting it avoids rebuilding inside the per-chunk async task.

    Args:
        parser_config: Configuration dict from the parser, expected to contain
            ``metadata`` (dict or list) and optionally ``built_in_metadata``
            (list of metadata item dicts).

    Returns:
        A list or dict representing the merged metadata configuration.
    """
    metadata_conf = parser_config.get("metadata", [])
    built_in_metadata = list(parser_config.get("built_in_metadata") or [])
    if isinstance(metadata_conf, dict):
        if not isinstance(metadata_conf.get("properties"), dict):
            metadata_conf = {"type": "object", "properties": {}}
        if built_in_metadata:
            metadata_conf = {
                **metadata_conf,
                "properties": {
                    **metadata_conf.get("properties", {}),
                    **turn2jsonschema(built_in_metadata).get("properties", {}),
                },
            }
    elif isinstance(metadata_conf, list):
        metadata_conf = metadata_conf + built_in_metadata
    else:
        metadata_conf = built_in_metadata
    return metadata_conf


async def generate_metadata(docs: List[Dict], ctx: TaskContext) -> None:
    """Generate metadata for chunks.

    Args:
        docs: List of chunk dictionaries to process.
        ctx: TaskContext containing task configuration.
    """
    chat_limiter = ctx.chat_limiter

    st = timer()
    ctx.progress_cb(msg="Start to generate meta-data for every chunk ...")
    chat_model_config = get_model_config_from_provider_instance(ctx.tenant_id, LLMType.CHAT, ctx.llm_id)
    with LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language) as chat_model:
        metadata_conf = build_metadata_config(ctx.parser_config)

        async def gen_metadata_task(chat_mdl, d):
            cached = get_llm_cache(chat_mdl.llm_name, d["content_with_weight"], "metadata", metadata_conf)
            if not cached:
                if ctx.has_canceled_func(ctx.id):
                    ctx.progress_cb(-1, msg="Task has been canceled.")
                    return
                async with chat_limiter:
                    cached = await gen_metadata(chat_mdl, turn2jsonschema(metadata_conf), d["content_with_weight"])
                set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, "metadata", metadata_conf)
            if cached:
                d["metadata_obj"] = cached

        tasks = []
        for doc in docs:
            tasks.append(asyncio.create_task(gen_metadata_task(chat_model, doc)))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error("Error in gen_metadata", exc_info=e)
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise

        metadata = {}
        for doc in docs:
            if "metadata_obj" in doc:
                metadata = update_metadata_to(metadata, doc["metadata_obj"])
                del doc["metadata_obj"]
        if metadata:
            existing_meta = DocMetadataService.get_document_metadata(ctx.doc_id)
            existing_meta = existing_meta if isinstance(existing_meta, dict) else {}
            metadata = update_metadata_to(metadata, existing_meta)
            if ctx.write_interceptor:
                ctx.write_interceptor.intercept("DocMetadataService.update_document_metadata")
            else:
                DocMetadataService.update_document_metadata(ctx.doc_id, metadata)
        ctx.progress_cb(msg="Metadata generation {} chunks completed in {:.2f}s".format(len(docs), timer() - st))


async def apply_tags(docs: List[Dict], ctx: TaskContext) -> None:
    """Apply tags to chunks.

    Args:
        docs: List of chunk dictionaries to process.
        ctx: TaskContext containing task configuration.
    """
    chat_limiter = ctx.chat_limiter

    ctx.progress_cb(msg="Start to tag for every chunk ...")
    kb_ids = ctx.kb_parser_config["tag_kb_ids"]
    tenant_id = ctx.tenant_id
    topn_tags = ctx.kb_parser_config.get("topn_tags", 3)
    S = 1000
    st = timer()
    examples = []
    all_tags = get_tags_from_cache(kb_ids)
    if not all_tags:
        all_tags = settings.retriever.all_tags_in_portion(tenant_id, kb_ids, S)
        set_tags_to_cache(kb_ids, all_tags)
    else:
        all_tags = json.loads(all_tags)
    chat_model_config = get_model_config_from_provider_instance(tenant_id, LLMType.CHAT, ctx.llm_id)
    with LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language) as chat_model:
        docs_to_tag = []
        for doc in docs:
            if ctx.has_canceled_func(ctx.id):
                ctx.progress_cb(-1, msg="Task has been canceled.")
                return
            if settings.retriever.tag_content(tenant_id, kb_ids, doc, all_tags, topn_tags=topn_tags, S=S) and len(doc.get(TAG_FLD, [])) > 0:
                examples.append({"content": doc["content_with_weight"], TAG_FLD: doc[TAG_FLD]})
            else:
                docs_to_tag.append(doc)

        async def doc_content_tagging(chat_mdl, d, topn_tags):
            cached = get_llm_cache(chat_mdl.llm_name, d["content_with_weight"], all_tags, {"topn": topn_tags})
            if not cached:
                if ctx.has_canceled_func(ctx.id):
                    ctx.progress_cb(-1, msg="Task has been canceled.")
                    return
                picked_examples = random.choices(examples, k=2) if len(examples) > 2 else examples
                if not picked_examples:
                    picked_examples.append({"content": "This is an example", TAG_FLD: {"example": 1}})
                async with chat_limiter:
                    cached = await content_tagging(
                        chat_mdl,
                        d["content_with_weight"],
                        all_tags,
                        picked_examples,
                        topn_tags,
                    )
                if cached:
                    cached = json.dumps(cached)
            if cached:
                set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, all_tags, {"topn": topn_tags})
                d[TAG_FLD] = json.loads(cached)

        tasks = []
        for doc in docs_to_tag:
            tasks.append(asyncio.create_task(doc_content_tagging(chat_model, doc, topn_tags)))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error("Error tagging docs: {}".format(e))
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise
        ctx.progress_cb(msg="Tagging {} chunks completed in {:.2f}s".format(len(docs), timer() - st))


def count_with_key(docs: List[Dict], key: str) -> int:
    """Count docs that have a specific key.

    Args:
        docs: List of chunk dictionaries.
        key: The key to check for.

    Returns:
        Count of docs that have the key.
    """
    return sum(1 for d in docs if d.get(key))


# =====================================================================
# Document post-chunking pipeline
# ---------------------------------------------------------------------
# Extracted from ``task_handler`` to keep the handler class small.
# The public entry point is :func:`run_document_post_chunking_if_last`;
# everything below is called (transitively) from there:
#   run_document_post_chunking_if_last
#     ├─ run_document_structure_compile
#     │    ├─ run_tree_templates
#     │    │    ├─ load_chunks_with_vec
#     │    │    ├─ rechunk_doc_by_tree
#     │    │    └─ raptor_tree_to_graph
#     │    └─ (streaming compile via chat models per template)
#     └─ handler._run_raptor       ← stays on the handler
#
# All entries take ``handler`` (``TaskHandler``) as their first arg so
# they can reach the handler's ``_task_context``, ``_run_raptor``, and
# ``_load_chunks_for_doc`` without a circular import.
# =====================================================================

import numpy as np  # noqa: E402
from typing import Callable, Optional  # noqa: E402

from common.exceptions import TaskCanceledException  # noqa: E402
from common.misc_utils import thread_pool_exec  # noqa: E402
from common.token_utils import num_tokens_from_string  # noqa: E402
from rag.nlp import search  # noqa: E402
from api.db.services.document_service import DocumentService  # noqa: E402
from api.db.services.compilation_template_service import (  # noqa: E402
    CompilationTemplateService,
)
from api.db.services.compilation_template_group_service import (  # noqa: E402
    CompilationTemplateGroupService,
)
from api.db.services.task_service import (  # noqa: E402
    abort_doc_chunking_counter,
    clear_doc_chunking_counter,
    credit_doc_chunking_task,
    is_doc_chunking_aborted,
)
from rag.advanced_rag.knowlege_compile.structure import (  # noqa: E402
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


# ----- parser_config helpers -----------------------------------------


def _parser_config_compilation_template_group_ids(parser_config) -> list[str]:
    def _normalize(raw) -> list[str]:
        if isinstance(raw, str):
            raw = [raw]
        if not isinstance(raw, list):
            return []
        ids: list[str] = []
        seen: set[str] = set()
        for gid in raw:
            if not isinstance(gid, str):
                continue
            gid = gid.strip()
            if gid and gid not in seen:
                seen.add(gid)
                ids.append(gid)
        return ids

    if not isinstance(parser_config, dict):
        return []
    if "compilation_template_group_id" in parser_config:
        return _normalize(parser_config.get("compilation_template_group_id"))
    ext = parser_config.get("ext")
    if isinstance(ext, dict):
        return _normalize(ext.get("compilation_template_group_id"))
    return []


def _parser_config_compilation_template_ids(parser_config, tenant_id: str) -> list[str]:
    template_ids: list[str] = []
    seen: set[str] = set()
    for group_id in _parser_config_compilation_template_group_ids(parser_config):
        for template_id in CompilationTemplateGroupService.resolve_template_ids(
            group_id,
            tenant_id,
        ):
            if template_id in seen:
                continue
            seen.add(template_id)
            template_ids.append(template_id)
    return template_ids


def _resolve_template_chat_llm_id(parser_cfg: dict, ctx) -> str:
    """Pick the chat model id for a knowledge-compilation template.

    Resolution order: template ``llm_id`` → doc ``parser_config.llm_id``
    → ``ctx.llm_id`` (the chunking task's default).
    """
    if isinstance(parser_cfg, dict):
        tid = parser_cfg.get("llm_id")
        if isinstance(tid, str) and tid.strip():
            return tid.strip()
    doc_cfg = getattr(ctx, "parser_config", None) or {}
    if isinstance(doc_cfg, dict):
        did = doc_cfg.get("llm_id")
        if isinstance(did, str) and did.strip():
            return did.strip()
    return ctx.llm_id


# ----- progress helper -----------------------------------------------


def cap_done_progress(progress_cb: Callable) -> Callable:
    """Wrap a progress callback so any ``prog >= 1`` gets clamped to
    ``0.99`` — the final ``1.0`` is reserved for the caller who owns
    the task's terminal state."""

    def capped_progress(*args, **kwargs):
        args = list(args)
        if args:
            prog = args[0]
            if isinstance(prog, (int, float)) and not isinstance(prog, bool) and prog >= 1:
                args[0] = 0.99
        if "prog" in kwargs:
            prog = kwargs["prog"]
            if isinstance(prog, (int, float)) and not isinstance(prog, bool) and prog >= 1:
                kwargs["prog"] = 0.99
        return progress_cb(*args, **kwargs)

    return capped_progress


# ----- tree helpers --------------------------------------------------


def raptor_tree_to_graph(tree: Dict) -> Dict:
    """Project a RAPTOR tree dict (from ``Raptor(is_tree=True)``) onto
    the ``{entities, relations}`` shape the document-structure graph
    endpoint already serves for ``page_index``-kind rows."""
    entities: list[dict] = []
    relations: list[dict] = []

    def _walk(node: dict, parent_id: Optional[str]) -> None:
        if not isinstance(node, dict):
            return
        title = node.get("title") or ""
        node_id = title
        ent: dict = {
            "name": node_id,
            "type": "tree_node",
            "description": node.get("description", title),
            "mention_count": 1,
        }
        src_ids = node.get("source_chunk_ids")
        if isinstance(src_ids, list) and src_ids:
            ent["source_chunk_ids"] = [s for s in src_ids if isinstance(s, str) and s]
        entities.append(ent)
        if parent_id is not None:
            relations.append({"from": parent_id, "to": node_id, "type": "child"})
        for child in node.get("children") or []:
            _walk(child, node_id)

    _walk(tree, None)
    return {"entities": entities, "relations": relations}


async def load_chunks_with_vec(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    vctr_nm: str,
) -> list[tuple[str, "np.ndarray", str]]:
    """Page through this doc's chunks pulling content + vector +
    chunk_id, in the shape ``RaptorService.build_doc_tree`` expects.
    Mirrors the streaming ``_load_chunks_for_doc`` loader but with the
    vector field pre-selected."""
    from common.doc_store.doc_store_base import OrderByExpr

    index_nm = search.index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index_nm, kb_id):
        return []
    select_fields = ["id", "doc_id", "content_with_weight", vctr_nm]
    order_by = OrderByExpr()
    order_by.asc("page_num_int")
    order_by.asc("top_int")

    out: list[tuple[str, "np.ndarray", str]] = []
    offset = 0
    PAGE = 500
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields,
                [],
                {"doc_id": [doc_id], "available_int": 1},
                [],
                order_by,
                offset,
                PAGE,
                index_nm,
                [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields)
        except Exception:
            logging.exception(
                "tree-template: failed to load chunks for doc=%s",
                doc_id,
            )
            break
        if not field_map:
            break
        for row_id, row in field_map.items():
            if row.get("compile_kwd"):
                continue
            text = row.get("content_with_weight") or ""
            vec = row.get(vctr_nm)
            if not text or vec is None:
                continue
            try:
                arr = np.asarray(vec, dtype=np.float32)
            except Exception:
                continue
            if arr.size == 0:
                continue
            out.append((text, arr, str(row_id)))
        if len(field_map) < PAGE:
            break
        offset += PAGE
    return out


async def rechunk_doc_by_tree(
    handler,
    tree: dict,
    template_id: str,
    embedding_model,
) -> None:
    """Merge each leaf cluster's source chunks into a single
    replacement chunk and rewrite the tree's leaf-cluster
    ``source_chunk_ids`` in-place. Original chunks are soft-deleted
    via ``available_int=0`` and stamped with ``superseded_by_chunk_id``.
    """
    from datetime import datetime
    from common.misc_utils import get_uuid

    ctx = handler._task_context

    cluster_id_map: dict[int, tuple[dict, list[str]]] = {}

    def _is_terminal(node: object) -> bool:
        return isinstance(node, dict) and not (node.get("children") or [])

    def _walk(node: object) -> None:
        if not isinstance(node, dict):
            return
        children = node.get("children") or []
        if children and all(_is_terminal(c) for c in children):
            src_ids: list[str] = []
            seen: set[str] = set()
            for c in children:
                for cid in c.get("source_chunk_ids") or []:
                    if isinstance(cid, str) and cid and cid not in seen:
                        seen.add(cid)
                        src_ids.append(cid)
            for cid in node.get("source_chunk_ids") or []:
                if isinstance(cid, str) and cid and cid not in seen:
                    seen.add(cid)
                    src_ids.append(cid)
            if src_ids:
                cluster_id_map[id(node)] = (node, src_ids)
        else:
            for c in children:
                _walk(c)

    _walk(tree)
    if not cluster_id_map:
        return

    all_source_ids = sorted({sid for _, ids in cluster_id_map.values() for sid in ids})

    from common.doc_store.doc_store_base import OrderByExpr

    index_nm = search.index_name(ctx.tenant_id)
    if not settings.docStoreConn.index_exist(index_nm, ctx.kb_id):
        return

    vctr_nm = "q_%d_vec" % len(embedding_model.encode(["x"])[0][0])
    select_fields = [
        "id",
        "doc_id",
        "kb_id",
        "content_with_weight",
        "page_num_int",
        "top_int",
        "position_int",
        "docnm_kwd",
        "title_tks",
        "title_sm_tks",
        "available_int",
    ]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields,
            [],
            {"id": all_source_ids, "available_int": 1},
            [],
            OrderByExpr(),
            0,
            len(all_source_ids) + 16,
            index_nm,
            [ctx.kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception(
            "rechunk: failed to load source chunks for doc=%s template=%s",
            ctx.doc_id,
            template_id,
        )
        return
    if not field_map:
        return

    chunks_by_id: dict[str, dict] = {str(rid): {**row, "id": str(rid)} for rid, row in field_map.items()}

    merged_rows: list[dict] = []
    cluster_new_id: dict[int, str] = {}

    for node_id_int, (node, src_ids) in cluster_id_map.items():
        cluster_chunks = [chunks_by_id[c] for c in src_ids if c in chunks_by_id]
        if not cluster_chunks:
            continue

        def _sort_key(c: dict) -> tuple:
            pages = c.get("page_num_int") or [0]
            tops = c.get("top_int") or [0]
            return (
                min(pages) if pages else 0,
                min(tops) if tops else 0,
                c.get("id") or "",
            )

        cluster_chunks.sort(key=_sort_key)

        merged_content = "\n\n".join((c.get("content_with_weight") or "") for c in cluster_chunks).strip()
        if not merged_content:
            continue
        page_union = sorted({p for c in cluster_chunks for p in (c.get("page_num_int") or [])})
        top_union = sorted({t for c in cluster_chunks for t in (c.get("top_int") or [])})

        base = dict(cluster_chunks[0])
        new_id = get_uuid()
        cluster_new_id[node_id_int] = new_id

        base.update(
            {
                "id": new_id,
                "content_with_weight": merged_content,
                "content_ltks": rag_tokenizer.tokenize(merged_content),
                "page_num_int": page_union,
                "top_int": top_union,
                "available_int": 1,
                "rechunk_kwd": "tree",
                "rechunked_from_template_id": template_id,
                "rechunked_from_chunk_ids": [c.get("id") for c in cluster_chunks if c.get("id")],
                "token_num": num_tokens_from_string(merged_content),
                "create_time": str(datetime.now()).replace("T", " ")[:19],
                "create_timestamp_flt": datetime.now().timestamp(),
            }
        )
        base["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(base["content_ltks"])
        merged_rows.append(base)

    if not merged_rows:
        return

    contents = [r["content_with_weight"] for r in merged_rows]
    try:
        vectors, _ = embedding_model.encode(contents)
    except Exception:
        logging.exception(
            "rechunk: embedding failed for doc=%s template=%s",
            ctx.doc_id,
            template_id,
        )
        return
    for row, vec in zip(merged_rows, vectors):
        try:
            row[vctr_nm] = np.asarray(vec, dtype=np.float32).tolist()
        except Exception:
            logging.exception(
                "rechunk: vector cast failed; skipping row %s",
                row.get("id"),
            )
            row[vctr_nm] = None
    merged_rows = [r for r in merged_rows if r.get(vctr_nm) is not None]
    if not merged_rows:
        return

    try:
        await thread_pool_exec(
            settings.docStoreConn.insert,
            merged_rows,
            index_nm,
            ctx.kb_id,
        )
    except Exception:
        logging.exception(
            "rechunk: insert failed for doc=%s template=%s",
            ctx.doc_id,
            template_id,
        )
        return

    for node_id_int, new_chunk_id in cluster_new_id.items():
        node, _ = cluster_id_map[node_id_int]
        node["source_chunk_ids"] = [new_chunk_id]
        for child in node.get("children") or []:
            if isinstance(child, dict):
                child["source_chunk_ids"] = [new_chunk_id]

    for node_id_int, new_chunk_id in cluster_new_id.items():
        _, src_ids = cluster_id_map[node_id_int]
        for cid in src_ids:
            try:
                await thread_pool_exec(
                    settings.docStoreConn.update,
                    {"id": cid},
                    {
                        "available_int": 0,
                        "superseded_by_chunk_id": new_chunk_id,
                    },
                    index_nm,
                    ctx.kb_id,
                )
            except Exception:
                logging.exception(
                    "rechunk: soft-delete failed for chunk=%s (merged=%s)",
                    cid,
                    new_chunk_id,
                )


async def run_tree_templates(
    handler,
    templates: list[tuple[str, dict]],
    chat_mdl_by_tid: dict[str, "LLMBundle"],
    embedding_model,
) -> None:
    """Run the ``tree``-kind compilation templates for the current
    doc. Each pair runs RAPTOR with ``is_tree=True`` via
    ``RaptorService.build_doc_tree`` and persists a single graph row
    via ``_struct_upsert_graph_json``."""
    from rag.svr.task_executor_refactor.raptor_service import RaptorService
    from rag.advanced_rag.knowlege_compile.structure import _struct_upsert_graph_json

    ctx = handler._task_context
    progress_cb = ctx.progress_cb

    try:
        doc_id = ctx.doc_id
    except Exception:
        doc_id = getattr(ctx, "_task", {}).get("doc_id") if hasattr(ctx, "_task") else None
    if not doc_id:
        logging.warning("tree-template: no doc_id on task context; skipping")
        return

    vctr_nm = "q_%d_vec" % len(embedding_model.encode(["x"])[0][0])
    chunks = await load_chunks_with_vec(
        ctx.tenant_id,
        ctx.kb_id,
        doc_id,
        vctr_nm,
    )
    if not chunks:
        progress_cb(msg=f"tree-template: doc {doc_id} has no chunks; skipping")
        return

    raptor_service = RaptorService(ctx)

    for idx, (template_id, parser_cfg) in enumerate(templates):
        raptor_cfg = (parser_cfg or {}).get("raptor") or {}
        raptor_config = {
            "prompt": raptor_cfg.get("prompt") or "Please write a concise summary of the following texts:\n{cluster_content}",
            "max_token": int(raptor_cfg.get("max_token") or 512),
            "threshold": float(raptor_cfg.get("threshold") or 0.1),
            "random_seed": int(raptor_cfg.get("random_seed") or 0),
            "max_cluster": int(raptor_cfg.get("max_cluster") or 64),
            "ext": raptor_cfg.get("ext") or {},
        }
        progress_cb(
            msg=f"tree-template ({idx + 1}/{len(templates)}): building tree for doc={doc_id}",
        )
        try:
            tree = await raptor_service.build_doc_tree(
                chunks=chunks,
                raptor_config=raptor_config,
                chat_mdl=chat_mdl_by_tid[template_id],
                embd_mdl=embedding_model,
                tree_builder="raptor",
                clustering_method="gmm",
                max_errors=3,
            )
        except Exception:
            logging.exception(
                "tree-template %s: RAPTOR build failed for doc %s",
                template_id,
                doc_id,
            )
            continue
        if tree is None:
            logging.info(
                "tree-template %s: no tree produced for doc %s",
                template_id,
                doc_id,
            )
            continue

        if bool((raptor_cfg or {}).get("rechunk")):
            try:
                await rechunk_doc_by_tree(
                    handler=handler,
                    tree=tree,
                    template_id=template_id,
                    embedding_model=embedding_model,
                )
            except Exception:
                logging.exception(
                    "tree-template %s: re-chunking failed for doc %s; persisting tree with original chunk ids",
                    template_id,
                    doc_id,
                )

        graph = raptor_tree_to_graph(tree)
        try:
            await _struct_upsert_graph_json(
                graph,
                ctx.tenant_id,
                ctx.kb_id,
                doc_id,
                compile_kwd="tree",
                compilation_template_id=template_id,
            )
        except Exception:
            logging.exception(
                "tree-template %s: graph upsert failed for doc %s",
                template_id,
                doc_id,
            )
            continue

        try:
            from rag.advanced_rag.knowlege_compile.dataset_nav import (
                upsert_dataset_nav_doc,
            )

            await upsert_dataset_nav_doc(
                ctx.tenant_id,
                ctx.kb_id,
                doc_id,
                tree,
            )
        except Exception:
            logging.exception(
                "tree-template %s: dataset_nav upsert failed for doc %s",
                template_id,
                doc_id,
            )

        progress_cb(
            msg=f"tree-template ({idx + 1}/{len(templates)}): persisted {len(graph['entities'])} node(s), {len(graph['relations'])} edge(s) for doc {doc_id}",
        )


async def run_document_structure_compile(handler, embedding_model: LLMBundle) -> None:
    """Run document-scoped knowledge compilation for non-artifact
    templates. Streams the doc's chunks (via
    ``handler._load_chunks_for_doc``) and fans each batch out to every
    configured non-artifact template, flushing accumulators through
    ``merge_compiled_structures`` at :data:`DOC_STRUCTURE_MERGE_MAX_DOCS`.

    After extract+merge, if any template has ``synthesis.enabled``,
    runs ``wiki_plan_from_reduction`` + ``wiki_refine_from_plan`` to
    generate synthesis output (wiki pages, essence paragraphs, etc.).
    Compile_kwd and REFINE prompt are read from the template config.
    """
    from api.apps.restful_apis.chunk_api import _compilation_template_kind

    ctx = handler._task_context
    template_ids = _parser_config_compilation_template_ids(ctx.parser_config, ctx.tenant_id)
    if not template_ids:
        return

    active_templates: list[tuple[str, dict]] = []
    for template_id in template_ids:
        template = CompilationTemplateService.get_saved(template_id, ctx.tenant_id)
        if not template:
            logging.warning(
                "document_structure_compile: template %s not found",
                template_id,
            )
            continue
        parser_cfg = template.get("config") or {}
        if not isinstance(parser_cfg, dict):
            logging.warning(
                "document_structure_compile: template %s config is invalid",
                template_id,
            )
            continue
        kind = _compilation_template_kind(parser_cfg.get("kind"))
        if not kind or kind == "artifacts":
            continue
        active_templates.append((template_id, parser_cfg))

    if not active_templates:
        return

    llm_bundle_cache: dict[str, LLMBundle] = {}
    chat_mdl_by_tid: dict[str, LLMBundle] = {}
    filtered_templates: list[tuple[str, dict]] = []
    for template_id, parser_cfg in active_templates:
        chat_llm_id = _resolve_template_chat_llm_id(parser_cfg, ctx)
        if chat_llm_id not in llm_bundle_cache:
            try:
                cfg = get_model_config_from_provider_instance(
                    ctx.tenant_id,
                    LLMType.CHAT,
                    chat_llm_id,
                )
                llm_bundle_cache[chat_llm_id] = LLMBundle(
                    ctx.tenant_id,
                    cfg,
                    lang=ctx.language,
                )
            except Exception:
                logging.exception(
                    "document_structure_compile: cannot resolve chat model %s for template %s; skipping",
                    chat_llm_id,
                    template_id,
                )
                continue
        chat_mdl_by_tid[template_id] = llm_bundle_cache[chat_llm_id]
        filtered_templates.append((template_id, parser_cfg))

    if not filtered_templates:
        return
    active_templates = filtered_templates

    tree_templates: list[tuple[str, dict]] = []
    non_tree_templates: list[tuple[str, dict]] = []
    for tid, cfg in active_templates:
        if _compilation_template_kind((cfg or {}).get("kind")) == "tree":
            tree_templates.append((tid, cfg))
        else:
            non_tree_templates.append((tid, cfg))

    if tree_templates:
        await run_tree_templates(
            handler,
            tree_templates,
            chat_mdl_by_tid,
            embedding_model,
        )

    if not non_tree_templates:
        return
    active_templates = non_tree_templates

    progress_cb = ctx.progress_cb
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
            ctx.tenant_id,
            ctx.kb_id,
            compilation_template_id=template_id,
            cancel_check=lambda: ctx.has_canceled_func(ctx.id),
        )
        acc.clear()
        if isinstance(info, dict):
            agg = agg_infos[template_id]
            for k in ("inserted", "updated", "duplicates_dropped"):
                agg[k] = agg.get(k, 0) + int(info.get(k, 0) or 0)

    progress_cb(msg=f"Start document knowledge compilation ({total} template(s)) ...")

    batch_no = 0
    async for batch in handler._load_chunks_for_doc(
        ctx.tenant_id,
        ctx.kb_id,
        ctx.doc_id,
        batch_size=DOC_STRUCTURE_COMPILE_BATCH_CHUNKS,
    ):
        batch_no += 1
        for chunk in batch:
            cid = chunk.get("id")
            if isinstance(cid, str) and cid not in chunks_by_id:
                text = chunk.get("content_with_weight") or ""
                chunks_by_id[cid] = text if isinstance(text, str) else ""
        for idx, (template_id, parser_cfg) in enumerate(active_templates):
            progress_cb(msg=f"  compile batch {batch_no} ({len(batch)} chunks) for template ({idx + 1}/{total})")
            docs = await compile_structure_from_text(
                batch,
                parser_cfg,
                chat_mdl_by_tid[template_id],
                embedding_model,
                ctx.doc_id,
                language=ctx.language,
                callback=progress_cb,
                compilation_template_id=template_id,
            )
            if docs:
                accumulators[template_id].extend(docs)
            if len(accumulators[template_id]) >= DOC_STRUCTURE_MERGE_MAX_DOCS:
                progress_cb(msg=f"  merge flush ({len(accumulators[template_id])} docs) for template ({idx + 1}/{total})")
                await _flush(template_id)

    for idx, (template_id, parser_cfg) in enumerate(active_templates):
        if ctx.has_canceled_func(ctx.id):
            raise TaskCanceledException(f"Task {ctx.id} was cancelled during document knowledge compilation")
        await _flush(template_id)
        agg = agg_infos[template_id]
        ctx.recording_context.record(f"document_structure_compile:{template_id}", agg)
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

            if ctx.has_canceled_func(ctx.id):
                raise TaskCanceledException(
                    f"Task {ctx.id} was cancelled before synthesis PLAN"
                )

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

                    progress_cb(
                        msg=f"Synthesis PLAN for template {template_id} (kind={compile_kwd}) ..."
                    )
                    plan = await wiki_plan_from_reduction(
                        chat_mdl=chat_mdl_by_tid[template_id],
                        embd_mdl=embedding_model,
                        tenant_id=ctx.tenant_id,
                        kb_id=ctx.kb_id,
                        callback=progress_cb,
                    )
                    if ctx.has_canceled_func(ctx.id):
                        raise TaskCanceledException(
                            f"Task {ctx.id} was cancelled after synthesis PLAN"
                        )

                    if not plan or not plan.get("pages"):
                        progress_cb(
                            msg=f"Synthesis: no pages planned for template {template_id}."
                        )
                    else:
                        progress_cb(
                            msg=f"Synthesis REFINE for template {template_id} ({len(plan['pages'])} page(s)) ..."
                        )
                        pages = await wiki_refine_from_plan(
                            chat_mdl=chat_mdl_by_tid[template_id],
                            embd_mdl=embedding_model,
                            tenant_id=ctx.tenant_id,
                            kb_id=ctx.kb_id,
                            callback=progress_cb,
                            example=example,
                        )
                        # Overwrite compile_kwd on every output page so the
                        # synthesis type is tracked correctly in ES.
                        for p in pages or []:
                            p["compile_kwd"] = compile_kwd
                        progress_cb(
                            msg=f"Synthesis done: {len(pages or [])} {compile_kwd} page(s) written."
                        )
                except Exception:
                    logging.exception(
                        "synthesis: failed for template %s", template_id,
                    )


async def run_document_post_chunking_if_last(
    handler,
    embedding_model: LLMBundle,
    vector_size: int,
    task_start_ts: float,
    chunks_len: int,
    token_count: int,
) -> bool:
    """Gate: only the last chunking task for a doc runs post-processing.
    Returns ``True`` if the caller may proceed to its own terminal
    progress update, ``False`` if the task was cancelled.

    The pass runs :func:`run_document_structure_compile` and
    ``handler._run_raptor`` concurrently — they read the same chunks
    but write disjoint ES rows.
    """
    ctx = handler._task_context
    task_id = ctx.id
    task_doc_id = ctx.doc_id

    if ctx.has_canceled_func(task_id):
        abort_doc_chunking_counter(task_doc_id)
        ctx.progress_cb(-1, msg="Task has been canceled.")
        return False

    chunking_aborted = is_doc_chunking_aborted(task_doc_id)
    remaining_chunking_tasks = 0 if ctx.write_interceptor else credit_doc_chunking_task(task_doc_id, task_id)
    if remaining_chunking_tasks != 0:
        if chunking_aborted:
            logging.info(
                "Chunking for doc %s was aborted before task %s reached post-processing; skip document finalizers.",
                task_doc_id,
                task_id,
            )
        elif remaining_chunking_tasks is not None and remaining_chunking_tasks < 0:
            logging.warning(
                "Chunking counter for doc %s is missing or expired after task %s; skip post-processing to avoid duplicate finalizers.",
                task_doc_id,
                task_id,
            )
        else:
            logging.info(
                "Chunk doc(%s), page(%s-%s), chunks(%s), token(%s), elapsed:%.2f; waiting for %s chunking task(s) before post-processing",
                ctx.name,
                ctx.from_page,
                ctx.to_page,
                chunks_len,
                token_count,
                timer() - task_start_ts,
                remaining_chunking_tasks,
            )
        return True

    async def _maybe_run_raptor():
        raptor_cfg = (ctx.parser_config or {}).get("raptor") or {}
        if not raptor_cfg.get("do_raptor"):
            return
        try:
            ok_doc, doc_obj = DocumentService.get_by_id(task_doc_id)
            if ok_doc and doc_obj is not None:
                ctx.progress_cb(msg="Starting RAPTOR task.")
                await handler._run_raptor(embedding_model, vector_size, mark_done=False)
            else:
                logging.warning(
                    "raptor: cannot resolve doc %s to queue per-doc task",
                    task_doc_id,
                )
        except Exception:
            logging.exception(
                "raptor: failed to queue per-doc task for doc %s",
                task_doc_id,
            )

    original_progress_cb = getattr(ctx, "_progress_cb", None)
    if original_progress_cb is not None:
        ctx._progress_cb = cap_done_progress(original_progress_cb)
    try:
        await asyncio.gather(
            run_document_structure_compile(handler, embedding_model),
            _maybe_run_raptor(),
        )
    finally:
        if original_progress_cb is not None:
            ctx._progress_cb = original_progress_cb
        clear_doc_chunking_counter(task_doc_id)

    if ctx.has_canceled_func(task_id):
        abort_doc_chunking_counter(task_doc_id)
        ctx.progress_cb(-1, msg="Task has been canceled.")
        return False
    return True
