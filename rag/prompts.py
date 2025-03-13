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
import datetime
import json
import logging
import re
from collections import defaultdict
import json_repair
from api import settings
from api.db import LLMType
from api.db.services.document_service import DocumentService
from api.db.services.llm_service import TenantLLMService, LLMBundle
from rag.settings import TAG_FLD
from rag.utils import num_tokens_from_string, encoder


def chunks_format(reference):
    def get_value(d, k1, k2):
        return d.get(k1, d.get(k2))

    return [{
        "id": get_value(chunk, "chunk_id", "id"),
        "content": get_value(chunk, "content", "content_with_weight"),
        "document_id": get_value(chunk, "doc_id", "document_id"),
        "document_name": get_value(chunk, "docnm_kwd", "document_name"),
        "dataset_id": get_value(chunk, "kb_id", "dataset_id"),
        "image_id": get_value(chunk, "image_id", "img_id"),
        "positions": get_value(chunk, "positions", "position_int"),
        "url": chunk.get("url")
    } for chunk in reference.get("chunks", [])]


def llm_id2llm_type(llm_id):
    llm_id, _ = TenantLLMService.split_model_name_and_factory(llm_id)

    llm_factories = settings.FACTORY_LLM_INFOS  
    for llm_factory in llm_factories:
        for llm in llm_factory["llm"]:
            if llm_id == llm["llm_name"]:
                return llm["model_type"].strip(",")[-1]


def message_fit_in(msg, max_length=4000):
    def count():
        nonlocal msg
        tks_cnts = []
        for m in msg:
            tks_cnts.append(
                {"role": m["role"], "count": num_tokens_from_string(m["content"])})
        total = 0
        for m in tks_cnts:
            total += m["count"]
        return total

    c = count()
    if c < max_length:
        return c, msg

    msg_ = [m for m in msg if m["role"] == "system"]
    if len(msg) > 1:
        msg_.append(msg[-1])
    msg = msg_
    c = count()
    if c < max_length:
        return c, msg

    ll = num_tokens_from_string(msg_[0]["content"])
    ll2 = num_tokens_from_string(msg_[-1]["content"])
    if ll / (ll + ll2) > 0.8:
        m = msg_[0]["content"]
        m = encoder.decode(encoder.encode(m)[:max_length - ll2])
        msg[0]["content"] = m
        return max_length, msg

    m = msg_[-1]["content"]
    m = encoder.decode(encoder.encode(m)[:max_length - ll2])
    msg[-1]["content"] = m
    return max_length, msg


def kb_prompt(kbinfos, max_tokens):
    knowledges = [ck["content_with_weight"] for ck in kbinfos["chunks"]]
    used_token_count = 0
    chunks_num = 0
    for i, c in enumerate(knowledges):
        used_token_count += num_tokens_from_string(c)
        chunks_num += 1
        if max_tokens * 0.97 < used_token_count:
            knowledges = knowledges[:i]
            logging.warning(f"Not all the retrieval into prompt: {i+1}/{len(knowledges)}")
            break

    docs = DocumentService.get_by_ids([ck["doc_id"] for ck in kbinfos["chunks"][:chunks_num]])
    docs = {d.id: d.meta_fields for d in docs}

    doc2chunks = defaultdict(lambda: {"chunks": [], "meta": []})
    for i, ck in enumerate(kbinfos["chunks"][:chunks_num]):
        doc2chunks[ck["docnm_kwd"]]["chunks"].append((f"URL: {ck['url']}\n" if "url" in ck else "") + f"ID: {i}\n" + ck["content_with_weight"])
        doc2chunks[ck["docnm_kwd"]]["meta"] = docs.get(ck["doc_id"], {})

    knowledges = []
    for nm, cks_meta in doc2chunks.items():
        txt = f"\nDocument: {nm} \n"
        for k, v in cks_meta["meta"].items():
            txt += f"{k}: {v}\n"
        txt += "Relevant fragments as following:\n"
        for i, chunk in enumerate(cks_meta["chunks"], 1):
            txt += f"{chunk}\n"
        knowledges.append(txt)
    return knowledges


def citation_prompt():
    return """

# Citation requirements:
- Inserts CITATIONS in format '##i$$ ##j$$' where i,j are the ID of the content you are citing and encapsulated with '##' and '$$'.
- Inserts the CITATION symbols at the end of a sentence, AND NO MORE than 4 citations.
- DO NOT insert CITATION in the answer if the content is not from retrieved chunks.

--- Example START ---
<SYSTEM>: Here is the knowledge base:

Document: Elon Musk Breaks Silence on Crypto, Warns Against Dogecoin ...
URL: https://blockworks.co/news/elon-musk-crypto-dogecoin
ID: 0
The Tesla co-founder advised against going all-in on dogecoin, but Elon Musk said it’s still his favorite crypto...

Document: Elon Musk's Dogecoin tweet sparks social media frenzy
ID: 1
Musk said he is 'willing to serve' D.O.G.E. – shorthand for Dogecoin.

Document: Causal effect of Elon Musk tweets on Dogecoin price
ID: 2
If you think of Dogecoin — the cryptocurrency based on a meme — you can’t help but also think of Elon Musk...

Document: Elon Musk's Tweet Ignites Dogecoin's Future In Public Services
ID: 3
The market is heating up after Elon Musk's announcement about Dogecoin. Is this a new era for crypto?...

      The above is the knowledge base.

<USER>: What's the Elon's view on dogecoin?

<ASSISTANT>: Musk has consistently expressed his fondness for Dogecoin, often citing its humor and the inclusion of dogs in its branding. He has referred to it as his favorite cryptocurrency ##0$$ ##1$$.
Recently, Musk has hinted at potential future roles for Dogecoin. His tweets have sparked speculation about Dogecoin's potential integration into public services ##3$$.
Overall, while Musk enjoys Dogecoin and often promotes it, he also warns against over-investing in it, reflecting both his personal amusement and caution regarding its speculative nature.

--- Example END ---

"""


def keyword_extraction(chat_mdl, content, topn=3):
    prompt = f"""
Role: You're a text analyzer. 
Task: extract the most important keywords/phrases of a given piece of text content.
Requirements: 
  - Summarize the text content, and give top {topn} important keywords/phrases.
  - The keywords MUST be in language of the given piece of text content.
  - The keywords are delimited by ENGLISH COMMA.
  - Keywords ONLY in output.

### Text Content 
{content}

"""
    msg = [
        {"role": "system", "content": prompt},
        {"role": "user", "content": "Output: "}
    ]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = chat_mdl.chat(prompt, msg[1:], {"temperature": 0.2})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"<think>.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


def question_proposal(chat_mdl, content, topn=3):
    prompt = f"""
Role: You're a text analyzer. 
Task:  propose {topn} questions about a given piece of text content.
Requirements: 
  - Understand and summarize the text content, and propose top {topn} important questions.
  - The questions SHOULD NOT have overlapping meanings.
  - The questions SHOULD cover the main content of the text as much as possible.
  - The questions MUST be in language of the given piece of text content.
  - One question per line.
  - Question ONLY in output.

### Text Content 
{content}

"""
    msg = [
        {"role": "system", "content": prompt},
        {"role": "user", "content": "Output: "}
    ]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = chat_mdl.chat(prompt, msg[1:], {"temperature": 0.2})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"<think>.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


def full_question(tenant_id, llm_id, messages, language=None):
    if llm_id2llm_type(llm_id) == "image2text":
        chat_mdl = LLMBundle(tenant_id, LLMType.IMAGE2TEXT, llm_id)
    else:
        chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, llm_id)
    conv = []
    for m in messages:
        if m["role"] not in ["user", "assistant"]:
            continue
        conv.append("{}: {}".format(m["role"].upper(), m["content"]))
    conv = "\n".join(conv)
    today = datetime.date.today().isoformat()
    yesterday = (datetime.date.today() - datetime.timedelta(days=1)).isoformat()
    tomorrow = (datetime.date.today() + datetime.timedelta(days=1)).isoformat()
    prompt = f"""
Role: A helpful assistant

Task and steps: 
    1. Generate a full user question that would follow the conversation.
    2. If the user's question involves relative date, you need to convert it into absolute date based on the current date, which is {today}. For example: 'yesterday' would be converted to {yesterday}.

Requirements & Restrictions:
  - If the user's latest question is completely, don't do anything, just return the original question.
  - DON'T generate anything except a refined question."""
    if language:
        prompt += f"""
  - Text generated MUST be in {language}."""
    else:
        prompt += """
  - Text generated MUST be in the same language of the original user's question.
"""
    prompt += f"""

######################
-Examples-
######################

# Example 1
## Conversation
USER: What is the name of Donald Trump's father?
ASSISTANT:  Fred Trump.
USER: And his mother?
###############
Output: What's the name of Donald Trump's mother?

------------
# Example 2
## Conversation
USER: What is the name of Donald Trump's father?
ASSISTANT:  Fred Trump.
USER: And his mother?
ASSISTANT:  Mary Trump.
User: What's her full name?
###############
Output: What's the full name of Donald Trump's mother Mary Trump?

------------
# Example 3
## Conversation
USER: What's the weather today in London?
ASSISTANT:  Cloudy.
USER: What's about tomorrow in Rochester?
###############
Output: What's the weather in Rochester on {tomorrow}?

######################
# Real Data
## Conversation
{conv}
###############
    """
    ans = chat_mdl.chat(prompt, [{"role": "user", "content": "Output: "}], {"temperature": 0.2})
    ans = re.sub(r"<think>.*</think>", "", ans, flags=re.DOTALL)
    return ans if ans.find("**ERROR**") < 0 else messages[-1]["content"]


def content_tagging(chat_mdl, content, all_tags, examples, topn=3):
    prompt = f"""
Role: You're a text analyzer. 

Task: Tag (put on some labels) to a given piece of text content based on the examples and the entire tag set.

Steps:: 
  - Comprehend the tag/label set.
  - Comprehend examples which all consist of both text content and assigned tags with relevance score in format of JSON.
  - Summarize the text content, and tag it with top {topn} most relevant tags from the set of tag/label and the corresponding relevance score.

Requirements
  - The tags MUST be from the tag set.
  - The output MUST be in JSON format only, the key is tag and the value is its relevance score.
  - The relevance score must be range from 1 to 10.
  - Keywords ONLY in output.

# TAG SET
{", ".join(all_tags)}

"""
    for i, ex in enumerate(examples):
        prompt += """
# Examples {}
### Text Content
{}

Output:
{}

        """.format(i, ex["content"], json.dumps(ex[TAG_FLD], indent=2, ensure_ascii=False))

    prompt += f"""
# Real Data
### Text Content
{content}

"""
    msg = [
        {"role": "system", "content": prompt},
        {"role": "user", "content": "Output: "}
    ]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = chat_mdl.chat(prompt, msg[1:], {"temperature": 0.5})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"<think>.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        raise Exception(kwd)

    try:
        return json_repair.loads(kwd)
    except json_repair.JSONDecodeError:
        try:
            result = kwd.replace(prompt[:-1], '').replace('user', '').replace('model', '').strip()
            result = '{' + result.split('{')[1].split('}')[0] + '}'
            return json_repair.loads(result)
        except Exception as e:
            logging.exception(f"JSON parsing error: {result} -> {e}")
            raise e
