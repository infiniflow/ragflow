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
from functools import reduce
from optparse import check_choice

from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from deepdoc.parser.pdf_parser import RAGFlowPdfParser, PlainParser, VisionParser
from rag.app.naive import Markdown
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.llm.cv_model import Base as VLM
from rag.nlp import concat_img, naive_merge, naive_merge_with_images


class ChunkerParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.method_options = ["general", "q&a", "resume", "manual", "table", "paper", "book", "laws", "presentation", "one"]
        self.method = "general"
        self.chunk_token_size = 512
        self.delimiter = "\n"
        self.overlapped_percent = 0
        self.page_rank = 0
        self.auto_keywords = 0
        self.auto_question = 0
        self.tag_sets = []
        self.llm_setting = {
            "llm_name": "",
            "lang": "Chinese"
        }

    def check(self):
        self.check_valid_value(self.method.lower(), "Chunk method abnormal.", self.method_options)
        self.check_positive_integer(self.chunk_token_size, "Chunk token size.")
        self.check_nonnegative_number(self.page_rank, "Page rank value: (0, 10]")
        self.check_nonnegative_number(self.auto_keywords, "Auto-keyword value: (0, 10]")
        self.check_nonnegative_number(self.auto_question, "Auto-question value: (0, 10]")
        self.check_decimal_float(self.overlapped_percent, "Overlapped percentage: [0, 1)")


class Chunker(ProcessBase):
    component_name = "Chunker"

    def _general(self, **kwargs):
        if kwargs.get("output_format") in ["markdown", "text"]:
            cks = naive_merge(kwargs.get("markdown"), self._param.chunk_token_size, self._param.delimiter, self._param.overlapped_percent)
            return [{"text": c} for c in cks]

        sections, section_images = [], []
        for o in kwargs["json"]:
            sections.append((o["text"], o.get("position_tag","")))
            section_images.append(o.get("image"))

        chunks, images = naive_merge_with_images(sections, section_images,self._param.chunk_token_size, self._param.delimiter, self._param.overlapped_percent)
        return [{
            "text": RAGFlowPdfParser.remove_tag(c),
            "image": img,
            "positions": RAGFlowPdfParser.extract_positions(c)
        } for c,img in zip(chunks,images)]

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
        chunks = function_map["general"](kwargs)
        if self._param.auto_keywords:
            llm_setting = self._param.llm_setting
            chat_mdl = LLMBundle(self._canvas._tenant_id, LLMType.CHAT, llm_name=llm_setting["llm_name"], lang=llm_setting["lang"])

            async def doc_keyword_extraction(chat_mdl, ck, topn):
                cached = get_llm_cache(chat_mdl.llm_name, d["text"], "keywords", {"topn": topn})
                if not cached:
                    async with chat_limiter:
                        cached = await trio.to_thread.run_sync(lambda: keyword_extraction(chat_mdl, d["content_with_weight"], topn))
                    set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, "keywords", {"topn": topn})
                if cached:
                    d["important_kwd"] = cached.split(",")
                    d["important_tks"] = rag_tokenizer.tokenize(" ".join(d["important_kwd"]))
                return
            async with trio.open_nursery() as nursery:
                for d in docs:
                    nursery.start_soon(doc_keyword_extraction, chat_mdl, d, task["parser_config"]["auto_keywords"])
