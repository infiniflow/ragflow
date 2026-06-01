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
Chunk Builder Module.

Provides parser factory and document chunking logic:
- Parser module registration and selection
- Document chunking via parser
- PDF outline extraction
"""

import logging
from timeit import default_timer as timer
from typing import Dict, List

from common.constants import ParserType
from common.misc_utils import thread_pool_exec
from rag.svr.task_executor_refactor.task_context import TaskContext

from api.db.services.doc_metadata_service import DocMetadataService
from common.metadata_utils import update_metadata_to
from rag.utils.table_es_metadata import merge_table_parser_config_from_kb


def get_parser(parser_id: str):
    """Get parser module by ID.

    Args:
        parser_id: The parser identifier.

    Returns:
        The parser module for the given parser ID.
    """
    from rag.app import laws, paper, presentation, manual, qa, table, book, resume, picture, naive, one, audio, email, tag

    factory = {
        "general": naive,
        ParserType.NAIVE.value: naive,
        ParserType.PAPER.value: paper,
        ParserType.BOOK.value: book,
        ParserType.PRESENTATION.value: presentation,
        ParserType.MANUAL.value: manual,
        ParserType.LAWS.value: laws,
        ParserType.QA.value: qa,
        ParserType.TABLE.value: table,
        ParserType.RESUME.value: resume,
        ParserType.PICTURE.value: picture,
        ParserType.ONE.value: one,
        ParserType.AUDIO.value: audio,
        ParserType.EMAIL.value: email,
        ParserType.KG.value: naive,
        ParserType.TAG.value: tag,
    }
    return factory[parser_id.lower()]


async def run_chunking(
    chunker,
    binary: bytes,
    ctx: TaskContext,
) -> List[Dict]:
    """Run document chunking via parser.

    Args:
        chunker: The parser module to use.
        binary: Binary content of the document.
        ctx: TaskContext containing task configuration.

    Returns:
        List of chunk dictionaries.
    """
    st = timer()
    try:
        # Merge table parser config
        parser_config = merge_table_parser_config_from_kb(ctx.raw_task)

        async with ctx.chunk_limiter:
            cks = await thread_pool_exec(
                chunker.chunk,
                ctx.name,
                binary=binary,
                from_page=ctx.from_page,
                to_page=ctx.to_page,
                lang=ctx.language,
                callback=ctx.progress_cb,
                kb_id=ctx.kb_id,
                parser_config=parser_config,
                tenant_id=ctx.tenant_id,
            )
        logging.info("Chunking({}) {}/{} done".format(timer() - st, ctx.location, ctx.name))
        ctx.recording_context.record("parser_config_after_merge", parser_config)
        return cks
    except Exception as e:
        ctx.progress_cb(-1, msg="Internal server error while chunking: %s" % str(e).replace("'", ""))
        logging.exception("Chunking {}/{} got exception".format(ctx.location, ctx.name))
        raise


async def extract_outline(cks: List[Dict], ctx: TaskContext) -> None:
    """Extract and persist PDF outline if present.

    Args:
        cks: List of chunk dictionaries.
        ctx: TaskContext containing task configuration.
    """
    outline_data = cks[0].get("__outline__") if cks else None
    ctx.recording_context.record("outline_data", outline_data)

    if cks and cks[0].get("__outline__"):
        outline = cks[0].pop("__outline__")
        try:
            if ctx.write_interceptor:
                ctx.write_interceptor.intercept("DocMetadataService.update_document_metadata")
            else:
                temp_doc = DocMetadataService.get_document_metadata(ctx.doc_id) or {}
                DocMetadataService.update_document_metadata(
                    ctx.doc_id,
                    update_metadata_to({"outline": outline}, temp_doc)
                )

            logging.info("Persisted PDF outline (%d entries) for doc %s", len(outline), ctx.doc_id)
        except Exception as e:
            logging.warning("Failed to persist PDF outline for doc %s: %s", ctx.doc_id, e)
