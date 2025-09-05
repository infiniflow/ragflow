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
import random

import trio

from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from deepdoc.parser.pdf_parser import RAGFlowPdfParser
from graphrag.utils import chat_limiter, get_llm_cache, set_llm_cache
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.chunker.schema import ChunkerFromUpstream
from rag.nlp import naive_merge, naive_merge_with_images
from rag.prompts.prompts import keyword_extraction, question_proposal


class ChunkerParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.method_options = [
            # General
            "general",
            "onetable",
            # Customer Service
            "q&a",
            "manual",
            # Recruitment
            "resume",
            # Education & Research
            "book",
            "paper",
            "laws",
            "presentation",
            # Other
            # "Tag" # TODO: Other method
        ]
        self.method = "general"
        self.chunk_token_size = 512
        self.delimiter = "\n"
        self.overlapped_percent = 0
        self.page_rank = 0
        self.auto_keywords = 0
        self.auto_questions = 0
        self.tag_sets = []
        self.llm_setting = {"llm_name": "", "lang": "Chinese"}

    def check(self):
        self.check_valid_value(self.method.lower(), "Chunk method abnormal.", self.method_options)
        self.check_positive_integer(self.chunk_token_size, "Chunk token size.")
        self.check_nonnegative_number(self.page_rank, "Page rank value: (0, 10]")
        self.check_nonnegative_number(self.auto_keywords, "Auto-keyword value: (0, 10]")
        self.check_nonnegative_number(self.auto_questions, "Auto-question value: (0, 10]")
        self.check_decimal_float(self.overlapped_percent, "Overlapped percentage: [0, 1)")

    def get_input_form(self) -> dict[str, dict]:
        return {}


class Chunker(ProcessBase):
    component_name = "Chunker"

    def _general(self, from_upstream: ChunkerFromUpstream):
        self.callback(random.randint(1, 5) / 100.0, "Start to chunk via `General`.")
        if from_upstream.output_format in ["markdown", "text"]:
            if from_upstream.output_format == "markdown":
                payload = from_upstream.markdown_result
            else:  # == "text"
                payload = from_upstream.text_result

            if not payload:
                payload = ""

            cks = naive_merge(
                payload,
                self._param.chunk_token_size,
                self._param.delimiter,
                self._param.overlapped_percent,
            )
            return [{"text": c} for c in cks]

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

        return [
            {
                "text": RAGFlowPdfParser.remove_tag(c),
                "image": img,
                "positions": RAGFlowPdfParser.extract_positions(c),
            }
            for c, img in zip(chunks, images)
        ]

    def _q_and_a(self, from_upstream: ChunkerFromUpstream):
        pass

    def _resume(self, from_upstream: ChunkerFromUpstream):
        pass

    def _manual(self, from_upstream: ChunkerFromUpstream):
        pass

    def _table(self, from_upstream: ChunkerFromUpstream):
        pass

    def _paper(self, from_upstream: ChunkerFromUpstream):
        pass

    def _book(self, from_upstream: ChunkerFromUpstream):
        pass

    def _laws(self, from_upstream: ChunkerFromUpstream):
        pass

    def _presentation(self, from_upstream: ChunkerFromUpstream):
        pass

    def _one(self, from_upstream: ChunkerFromUpstream):
        pass

    async def _invoke(self, **kwargs):
        function_map = {
            "general": self._general,
            "q&a": self._q_and_a,
            "resume": self._resume,
            "manual": self._manual,
            "table": self._table,
            "paper": self._paper,
            "book": self._book,
            "laws": self._laws,
            "presentation": self._presentation,
            "one": self._one,
        }

        try:
            from_upstream = ChunkerFromUpstream.model_validate(kwargs)
        except Exception as e:
            self.set_output("_ERROR", f"Input error: {str(e)}")
            return

        chunks = function_map[self._param.method](from_upstream)
        llm_setting = self._param.llm_setting

        async def auto_keywords():
            nonlocal chunks, llm_setting
            chat_mdl = LLMBundle(self._canvas._tenant_id, LLMType.CHAT, llm_name=llm_setting["llm_name"], lang=llm_setting["lang"])

            async def doc_keyword_extraction(chat_mdl, ck, topn):
                cached = get_llm_cache(chat_mdl.llm_name, ck["text"], "keywords", {"topn": topn})
                if not cached:
                    async with chat_limiter:
                        cached = await trio.to_thread.run_sync(lambda: keyword_extraction(chat_mdl, ck["text"], topn))
                    set_llm_cache(chat_mdl.llm_name, ck["text"], cached, "keywords", {"topn": topn})
                if cached:
                    ck["keywords"] = cached.split(",")

            async with trio.open_nursery() as nursery:
                for ck in chunks:
                    nursery.start_soon(doc_keyword_extraction, chat_mdl, ck, self._param.auto_keywords)

        async def auto_questions():
            nonlocal chunks, llm_setting
            chat_mdl = LLMBundle(self._canvas._tenant_id, LLMType.CHAT, llm_name=llm_setting["llm_name"], lang=llm_setting["lang"])

            async def doc_question_proposal(chat_mdl, d, topn):
                cached = get_llm_cache(chat_mdl.llm_name, ck["text"], "question", {"topn": topn})
                if not cached:
                    async with chat_limiter:
                        cached = await trio.to_thread.run_sync(lambda: question_proposal(chat_mdl, ck["text"], topn))
                    set_llm_cache(chat_mdl.llm_name, ck["text"], cached, "question", {"topn": topn})
                if cached:
                    d["questions"] = cached.split("\n")

            async with trio.open_nursery() as nursery:
                for ck in chunks:
                    nursery.start_soon(doc_question_proposal, chat_mdl, ck, self._param.auto_questions)

        async with trio.open_nursery() as nursery:
            if self._param.auto_questions:
                nursery.start_soon(auto_questions)
            if self._param.auto_keywords:
                nursery.start_soon(auto_keywords)

        if self._param.page_rank:
            for ck in chunks:
                ck["page_rank"] = self._param.page_rank

        self.set_output("chunks", chunks)
