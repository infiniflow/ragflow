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
from rag.graphrag.utils import get_llm_cache, set_llm_cache


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
            tasks.append(
                asyncio.create_task(doc_keyword_extraction(chat_model, doc, ctx.parser_config["auto_keywords"])))
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
            tasks.append(
                asyncio.create_task(doc_question_proposal(chat_model, doc, ctx.parser_config["auto_questions"])))
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
            cached = get_llm_cache(chat_mdl.llm_name, d["content_with_weight"], "metadata",
                                   metadata_conf)
            if not cached:
                if ctx.has_canceled_func(ctx.id):
                    ctx.progress_cb(-1, msg="Task has been canceled.")
                    return
                async with chat_limiter:
                    cached = await gen_metadata(chat_mdl,
                                                turn2jsonschema(metadata_conf),
                                                d["content_with_weight"])
                set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, "metadata",
                              metadata_conf)
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
    all_tags = settings.retriever.all_tags_in_portion(tenant_id, kb_ids, S)
    chat_model_config = get_model_config_from_provider_instance(tenant_id, LLMType.CHAT, ctx.llm_id)
    with LLMBundle(ctx.tenant_id, chat_model_config, lang=ctx.language) as chat_model:

        docs_to_tag = []
        for doc in docs:
            if ctx.has_canceled_func(ctx.id):
                ctx.progress_cb(-1, msg="Task has been canceled.")
                return
            if settings.retriever.tag_content(tenant_id, kb_ids, doc, all_tags, topn_tags=topn_tags, S=S) and len(
                    doc.get(TAG_FLD, [])) > 0:
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
                    picked_examples.append({"content": "This is an example", TAG_FLD: {'example': 1}})
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
