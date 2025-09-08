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
import logging
import random
import re

import numpy as np
import trio

from api.db import LLMType
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.services.user_service import TenantService
from api.utils.api_utils import timeout
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.tokenizer.schema import TokenizerFromUpstream
from rag.nlp import rag_tokenizer
from rag.settings import EMBEDDING_BATCH_SIZE
from rag.svr.task_executor import embed_limiter
from rag.utils import truncate


class TokenizerParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.search_method = ["full_text", "embedding"]
        self.filename_embd_weight = 0.1

    def check(self):
        for v in self.search_method:
            self.check_valid_value(v.lower(), "Chunk method abnormal.", ["full_text", "embedding"])

    def get_input_form(self) -> dict[str, dict]:
        return {}


class Tokenizer(ProcessBase):
    component_name = "Tokenizer"

    async def _embedding(self, name, chunks):
        parts = sum(["full_text" in self._param.search_method, "embedding" in self._param.search_method])
        token_count = 0
        if self._canvas._kb_id:
            e, kb = KnowledgebaseService.get_by_id(self._canvas._kb_id)
            embedding_id = kb.embd_id
        else:
            e, ten = TenantService.get_by_id(self._canvas._tenant_id)
            embedding_id = ten.embd_id
        embedding_model = LLMBundle(self._canvas._tenant_id, LLMType.EMBEDDING, llm_name=embedding_id)
        texts = []
        for c in chunks:
            if c.get("questions"):
                texts.append("\n".join(c["questions"]))
            else:
                texts.append(re.sub(r"</?(table|td|caption|tr|th)( [^<>]{0,12})?>", " ", c["text"]))
        vts, c = embedding_model.encode([name])
        token_count += c
        tts = np.concatenate([vts[0] for _ in range(len(texts))], axis=0)

        @timeout(60)
        def batch_encode(txts):
            nonlocal embedding_model
            return embedding_model.encode([truncate(c, embedding_model.max_length - 10) for c in txts])

        cnts_ = np.array([])
        for i in range(0, len(texts), EMBEDDING_BATCH_SIZE):
            async with embed_limiter:
                vts, c = await trio.to_thread.run_sync(lambda: batch_encode(texts[i : i + EMBEDDING_BATCH_SIZE]))
            if len(cnts_) == 0:
                cnts_ = vts
            else:
                cnts_ = np.concatenate((cnts_, vts), axis=0)
            token_count += c
            if i % 33 == 32:
                self.callback(i * 1.0 / len(texts) / parts / EMBEDDING_BATCH_SIZE + 0.5 * (parts - 1))

        cnts = cnts_
        title_w = float(self._param.filename_embd_weight)
        vects = (title_w * tts + (1 - title_w) * cnts) if len(tts) == len(cnts) else cnts

        assert len(vects) == len(chunks)
        for i, ck in enumerate(chunks):
            v = vects[i].tolist()
            ck["q_%d_vec" % len(v)] = v
        return chunks, token_count

    async def _invoke(self, **kwargs):
        try:
            from_upstream = TokenizerFromUpstream.model_validate(kwargs)
        except Exception as e:
            self.set_output("_ERROR", f"Input error: {str(e)}")
            return

        parts = sum(["full_text" in self._param.search_method, "embedding" in self._param.search_method])
        if "full_text" in self._param.search_method:
            self.callback(random.randint(1, 5) / 100.0, "Start to tokenize.")
            if from_upstream.chunks:
                chunks = from_upstream.chunks
                for i, ck in enumerate(chunks):
                    if ck.get("questions"):
                        ck["question_tks"] = rag_tokenizer.tokenize("\n".join(ck["questions"]))
                    if ck.get("keywords"):
                        ck["important_tks"] = rag_tokenizer.tokenize("\n".join(ck["keywords"]))
                    ck["content_ltks"] = rag_tokenizer.tokenize(ck["text"])
                    ck["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(ck["content_ltks"])
                    if i % 100 == 99:
                        self.callback(i * 1.0 / len(chunks) / parts)
            elif from_upstream.output_format in ["markdown", "text"]:
                if from_upstream.output_format == "markdown":
                    payload = from_upstream.markdown_result
                else:  # == "text"
                    payload = from_upstream.text_result

                if not payload:
                    return ""

                ck = {"text": payload}
                if "full_text" in self._param.search_method:
                    ck["content_ltks"] = rag_tokenizer.tokenize(kwargs.get(kwargs["output_format"], ""))
                    ck["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(ck["content_ltks"])
                chunks = [ck]
            else:
                chunks = from_upstream.json_result
                for i, ck in enumerate(chunks):
                    ck["content_ltks"] = rag_tokenizer.tokenize(ck["text"])
                    ck["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(ck["content_ltks"])
                    if i % 100 == 99:
                        self.callback(i * 1.0 / len(chunks) / parts)

            self.callback(1.0 / parts, "Finish tokenizing.")

        if "embedding" in self._param.search_method:
            self.callback(random.randint(1, 5) / 100.0 + 0.5 * (parts - 1), "Start embedding inference.")

            if from_upstream.name.strip() == "":
                logging.warning("Tokenizer: empty name provided from upstream, embedding may be not accurate.")

            chunks, token_count = await self._embedding(from_upstream.name, chunks)
            self.set_output("embedding_token_consumption", token_count)

            self.callback(1.0, "Finish embedding.")

        self.set_output("chunks", chunks)
