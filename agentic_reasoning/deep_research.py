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
import logging
import re
from functools import partial
from agentic_reasoning.prompts import BEGIN_SEARCH_QUERY, BEGIN_SEARCH_RESULT, END_SEARCH_RESULT, MAX_SEARCH_LIMIT, \
    END_SEARCH_QUERY, REASON_PROMPT, RELEVANT_EXTRACTION_PROMPT
from api.db.services.llm_service import LLMBundle
from rag.nlp import extract_between
from rag.prompts import kb_prompt
from rag.utils.tavily_conn import Tavily


class DeepResearcher:
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

    def thinking(self, chunk_info: dict, question: str):
        def rm_query_tags(line):
            pattern = re.escape(BEGIN_SEARCH_QUERY) + r"(.*?)" + re.escape(END_SEARCH_QUERY)
            return re.sub(pattern, "", line)

        def rm_result_tags(line):
            pattern = re.escape(BEGIN_SEARCH_RESULT) + r"(.*?)" + re.escape(END_SEARCH_RESULT)
            return re.sub(pattern, "", line)

        executed_search_queries = []
        msg_hisotry = [{"role": "user", "content": f'Question:\"{question}\"\n'}]
        all_reasoning_steps = []
        think = "<think>"
        for ii in range(MAX_SEARCH_LIMIT + 1):
            if ii == MAX_SEARCH_LIMIT - 1:
                summary_think = f"\n{BEGIN_SEARCH_RESULT}\nThe maximum search limit is exceeded. You are not allowed to search.\n{END_SEARCH_RESULT}\n"
                yield {"answer": think + summary_think + "</think>", "reference": {}, "audio_binary": None}
                all_reasoning_steps.append(summary_think)
                msg_hisotry.append({"role": "assistant", "content": summary_think})
                break

            query_think = ""
            if msg_hisotry[-1]["role"] != "user":
                msg_hisotry.append({"role": "user", "content": "Continues reasoning with the new information.\n"})
            else:
                msg_hisotry[-1]["content"] += "\n\nContinues reasoning with the new information.\n"
            for ans in self.chat_mdl.chat_streamly(REASON_PROMPT, msg_hisotry, {"temperature": 0.7}):
                ans = re.sub(r"<think>.*</think>", "", ans, flags=re.DOTALL)
                if not ans:
                    continue
                query_think = ans
                yield {"answer": think + rm_query_tags(query_think) + "</think>", "reference": {}, "audio_binary": None}

            think += rm_query_tags(query_think)
            all_reasoning_steps.append(query_think)
            queries = extract_between(query_think, BEGIN_SEARCH_QUERY, END_SEARCH_QUERY)
            if not queries:
                if ii > 0:
                    break
                queries = [question]

            for search_query in queries:
                logging.info(f"[THINK]Query: {ii}. {search_query}")
                msg_hisotry.append({"role": "assistant", "content": search_query})
                think += f"\n\n> {ii +1}. {search_query}\n\n"
                yield {"answer": think + "</think>", "reference": {}, "audio_binary": None}

                summary_think = ""
                # The search query has been searched in previous steps.
                if search_query in executed_search_queries:
                    summary_think = f"\n{BEGIN_SEARCH_RESULT}\nYou have searched this query. Please refer to previous results.\n{END_SEARCH_RESULT}\n"
                    yield {"answer": think + summary_think + "</think>", "reference": {}, "audio_binary": None}
                    all_reasoning_steps.append(summary_think)
                    msg_hisotry.append({"role": "user", "content": summary_think})
                    think += summary_think
                    continue

                truncated_prev_reasoning = ""
                for i, step in enumerate(all_reasoning_steps):
                    truncated_prev_reasoning += f"Step {i + 1}: {step}\n\n"

                prev_steps = truncated_prev_reasoning.split('\n\n')
                if len(prev_steps) <= 5:
                    truncated_prev_reasoning = '\n\n'.join(prev_steps)
                else:
                    truncated_prev_reasoning = ''
                    for i, step in enumerate(prev_steps):
                        if i == 0 or i >= len(prev_steps) - 4 or BEGIN_SEARCH_QUERY in step or BEGIN_SEARCH_RESULT in step:
                            truncated_prev_reasoning += step + '\n\n'
                        else:
                            if truncated_prev_reasoning[-len('\n\n...\n\n'):] != '\n\n...\n\n':
                                truncated_prev_reasoning += '...\n\n'
                truncated_prev_reasoning = truncated_prev_reasoning.strip('\n')

                # Retrieval procedure:
                # 1. KB search
                # 2. Web search (optional)
                # 3. KG search (optional)
                kbinfos = self._kb_retrieve(question=search_query) if self._kb_retrieve else {"chunks": [], "doc_aggs": []}

                if self.prompt_config.get("tavily_api_key"):
                    tav = Tavily(self.prompt_config["tavily_api_key"])
                    tav_res = tav.retrieve_chunks(" ".join(search_query))
                    kbinfos["chunks"].extend(tav_res["chunks"])
                    kbinfos["doc_aggs"].extend(tav_res["doc_aggs"])
                if self.prompt_config.get("use_kg") and self._kg_retrieve:
                    ck = self._kg_retrieve(question=search_query)
                    if ck["content_with_weight"]:
                        kbinfos["chunks"].insert(0, ck)

                # Merge chunk info for citations
                if not chunk_info["chunks"]:
                    for k in chunk_info.keys():
                        chunk_info[k] = kbinfos[k]
                else:
                    cids = [c["chunk_id"] for c in chunk_info["chunks"]]
                    for c in kbinfos["chunks"]:
                        if c["chunk_id"] in cids:
                            continue
                        chunk_info["chunks"].append(c)
                    dids = [d["doc_id"] for d in chunk_info["doc_aggs"]]
                    for d in kbinfos["doc_aggs"]:
                        if d["doc_id"] in dids:
                            continue
                        chunk_info["doc_aggs"].append(d)

                think += "\n\n"
                for ans in self.chat_mdl.chat_streamly(
                        RELEVANT_EXTRACTION_PROMPT.format(
                            prev_reasoning=truncated_prev_reasoning,
                            search_query=search_query,
                            document="\n".join(kb_prompt(kbinfos, 4096))
                        ),
                        [{"role": "user",
                          "content": f'Now you should analyze each web page and find helpful information based on the current search query "{search_query}" and previous reasoning steps.'}],
                        {"temperature": 0.7}):
                    ans = re.sub(r"<think>.*</think>", "", ans, flags=re.DOTALL)
                    if not ans:
                        continue
                    summary_think = ans
                    yield {"answer": think + rm_result_tags(summary_think) + "</think>", "reference": {}, "audio_binary": None}

                all_reasoning_steps.append(summary_think)
                msg_hisotry.append(
                    {"role": "user", "content": f"\n\n{BEGIN_SEARCH_RESULT}{summary_think}{END_SEARCH_RESULT}\n\n"})
                think += rm_result_tags(summary_think)
                logging.info(f"[THINK]Summary: {ii}. {summary_think}")

        yield think + "</think>"
