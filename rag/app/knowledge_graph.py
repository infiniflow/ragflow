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
#

import re
from rag.app import naive
from rag.nlp import rag_tokenizer, tokenize_chunks


def chunk(filename, binary, tenant_id, from_page=0, to_page=100000,
          lang="Chinese", callback=None, **kwargs):
    parser_config = kwargs.get(
        "parser_config", {
            "chunk_token_num": 512, "delimiter": "\n!?;。；！？", "layout_recognize": True})
    eng = lang.lower() == "english"

    parser_config["layout_recognize"] = True
    sections = naive.chunk(filename, binary, from_page=from_page, to_page=to_page, section_only=True,
                           parser_config=parser_config, callback=callback)

    #chunks = build_knowledge_graph_chunks(tenant_id, sections, callback,
    #                                      parser_config.get("entity_types", ["organization", "person", "location", "event", "time"])
    #                                      )
    #for c in chunks:
    #    c["docnm_kwd"] = filename

    doc = {
        "docnm_kwd": filename,
        "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename)),
        "knowledge_graph_kwd": "text"
    }
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])
    chunks.extend(tokenize_chunks(sections, doc, eng))

    return chunks
