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
import json
import random

import trio

from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from deepdoc.parser.pdf_parser import RAGFlowPdfParser
from graphrag.utils import chat_limiter, get_llm_cache, set_llm_cache
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.chunker.schema import ChunkerFromUpstream
from rag.flow.splitter.schema import SplitterFromUpstream
from rag.nlp import naive_merge, naive_merge_with_images, concat_img
from rag.prompts.prompts import keyword_extraction, question_proposal, detect_table_of_contents, \
    extract_table_of_contents, table_of_contents_index, toc_transformer
from rag.utils import num_tokens_from_string


class SplitterParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.chunk_token_size = 512
        self.delimiter = "\n"
        self.overlapped_percent = 0

    def check(self):
        self.check_valid_value(self.method.lower(), "Chunk method abnormal.", self.method_options)
        self.check_positive_integer(self.chunk_token_size, "Chunk token size.")
        self.check_nonnegative_number(self.page_rank, "Page rank value: (0, 10]")
        self.check_nonnegative_number(self.auto_keywords, "Auto-keyword value: (0, 10]")
        self.check_nonnegative_number(self.auto_questions, "Auto-question value: (0, 10]")
        self.check_decimal_float(self.overlapped_percent, "Overlapped percentage: [0, 1)")

    def get_input_form(self) -> dict[str, dict]:
        return {}


class Splitter(ProcessBase):
    component_name = "Splitter"

    async def _invoke(self, **kwargs):
        try:
            from_upstream = SplitterFromUpstream.model_validate(kwargs)
        except Exception as e:
            self.set_output("_ERROR", f"Input error: {str(e)}")
            return

        self.callback(random.randint(1, 5) / 100.0, "Start to split into chunks.")
        if from_upstream.output_format in ["markdown", "text", "html"]:
            if from_upstream.output_format == "markdown":
                payload = from_upstream.markdown_result
            elif from_upstream.output_format == "text":
                payload = from_upstream.text_result
            else:  # == "html"
                payload = from_upstream.html_result

            if not payload:
                payload = ""

            cks = naive_merge(
                payload,
                self._param.chunk_token_size,
                self._param.delimiter,
                self._param.overlapped_percent,
            )
            self.set_output("chunks", [{"text": c} for c in cks])

            self.callback(1, "Done.")
            return

        # json
        sections, section_images = [], []
        for o in from_upstream.json_result or []:
            sections.append((o.get("text", ""), o.get("position_tag", "")))
            section_images.append(o.get("image"))

        chunks, images = naive_merge_with_images(
            sections,
            section_images,
            self._param.chunk_token_size,
            self._param.delimiter,
            self._param.overlapped_percent,
        )

        self.set_output("chunks",  [
            {
                "text": RAGFlowPdfParser.remove_tag(c),
                "image": img,
                "positions": RAGFlowPdfParser.extract_positions(c),
            }
            for c, img in zip(chunks, images)
        ])
        self.callback(1, "Done.")
