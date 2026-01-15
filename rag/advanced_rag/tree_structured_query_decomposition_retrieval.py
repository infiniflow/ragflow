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
import asyncio
import logging
from functools import partial
from api.db.services.llm_service import LLMBundle
from rag.prompts import kb_prompt
from rag.prompts.generator import sufficiency_check, multi_queries_gen
from rag.utils.tavily_conn import Tavily
from timeit import default_timer as timer


class TreeStructuredQueryDecompositionRetrieval:
    def __init__(self,
                 chat_mdl: LLMBundle,
                 prompt_config: dict,
                 kb_retrieve: partial = None,
                 kg_retrieve: partial = None
                 ):
        self.chat_mdl = chat_mdl
        self.prompt_config = prompt_config
        self._kb_retrieve = kb_retrieve
        self._kg_retrieve = kg_retrieve
        self._lock = asyncio.Lock()

    async def _retrieve_information(self, search_query):
        """Retrieve information from different sources"""
        # 1. Knowledge base retrieval
        kbinfos = []
        try:
            kbinfos = await self._kb_retrieve(question=search_query) if self._kb_retrieve else {"chunks": [], "doc_aggs": []}
        except Exception as e:
            logging.error(f"Knowledge base retrieval error: {e}")

        # 2. Web retrieval (if Tavily API is configured)
        try:
            if self.prompt_config.get("tavily_api_key"):
                tav = Tavily(self.prompt_config["tavily_api_key"])
                tav_res = tav.retrieve_chunks(search_query)
                kbinfos["chunks"].extend(tav_res["chunks"])
                kbinfos["doc_aggs"].extend(tav_res["doc_aggs"])
        except Exception as e:
            logging.error(f"Web retrieval error: {e}")

        # 3. Knowledge graph retrieval (if configured)
        try:
            if self.prompt_config.get("use_kg") and self._kg_retrieve:
                ck = await self._kg_retrieve(question=search_query)
                if ck["content_with_weight"]:
                    kbinfos["chunks"].insert(0, ck)
        except Exception as e:
            logging.error(f"Knowledge graph retrieval error: {e}")

        return kbinfos

    async def _async_update_chunk_info(self, chunk_info, kbinfos):
        async with self._lock:
            """Update chunk information for citations"""
            if not chunk_info["chunks"]:
                # If this is the first retrieval, use the retrieval results directly
                for k in chunk_info.keys():
                    chunk_info[k] = kbinfos[k]
            else:
                # Merge newly retrieved information, avoiding duplicates
                cids = [c["chunk_id"] for c in chunk_info["chunks"]]
                for c in kbinfos["chunks"]:
                    if c["chunk_id"] not in cids:
                        chunk_info["chunks"].append(c)

                dids = [d["doc_id"] for d in chunk_info["doc_aggs"]]
                for d in kbinfos["doc_aggs"]:
                    if d["doc_id"] not in dids:
                        chunk_info["doc_aggs"].append(d)

    async def research(self, chunk_info, question, query, depth=3, callback=None):
        if callback:
            await callback("<START_DEEP_RESEARCH>")
        await self._research(chunk_info, question, query, depth, callback)
        if callback:
            await callback("<END_DEEP_RESEARCH>")

    async def _research(self, chunk_info, question, query, depth=3, callback=None):
        if depth == 0:
            #if callback:
            #    await callback("Reach the max search depth.")
            return ""
        if callback:
            await callback(f"Searching by `{query}`...")
        st = timer()
        ret = await self._retrieve_information(query)
        if callback:
            await callback("Retrieval %d results in %.1fms"%(len(ret["chunks"]), (timer()-st)*1000))
        await self._async_update_chunk_info(chunk_info, ret)
        ret = kb_prompt(ret, self.chat_mdl.max_length*0.5)

        if callback:
            await callback("Checking the sufficiency for retrieved information.")
        suff = await sufficiency_check(self.chat_mdl, question, ret)
        if suff["is_sufficient"]:
            if callback:
                await callback(f"Yes, the retrieved information is sufficient for '{question}'.")
            return ret

        #if callback:
        #    await callback("The retrieved information is not sufficient. Planing next steps...")
        succ_question_info = await multi_queries_gen(self.chat_mdl, question, query, suff["missing_information"], ret)
        if callback:
            await callback("Next step is to search for the following questions:</br> - " + "</br> - ".join(step["question"] for step in succ_question_info["questions"]))
        steps = []
        for step in succ_question_info["questions"]:
            steps.append(asyncio.create_task(self._research(chunk_info, step["question"], step["query"], depth-1, callback)))
        results = await asyncio.gather(*steps, return_exceptions=True)
        return "\n".join([str(r) for r in results])
