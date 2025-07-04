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
from copy import deepcopy

import json_repair

from api import settings
from api.db import LLMType
from rag.settings import TAG_FLD
from rag.utils import encoder, num_tokens_from_string

STOP_TOKEN="<|STOP|>"
COMPLETE_TASK="complete_task"

def chunks_format(reference):
    def get_value(d, k1, k2):
        return d.get(k1, d.get(k2))

    return [
        {
            "id": get_value(chunk, "chunk_id", "id"),
            "content": get_value(chunk, "content", "content_with_weight"),
            "document_id": get_value(chunk, "doc_id", "document_id"),
            "document_name": get_value(chunk, "docnm_kwd", "document_name"),
            "dataset_id": get_value(chunk, "kb_id", "dataset_id"),
            "image_id": get_value(chunk, "image_id", "img_id"),
            "positions": get_value(chunk, "positions", "position_int"),
            "url": chunk.get("url"),
            "similarity": chunk.get("similarity"),
            "vector_similarity": chunk.get("vector_similarity"),
            "term_similarity": chunk.get("term_similarity"),
            "doc_type": chunk.get("doc_type_kwd"),
        }
        for chunk in reference.get("chunks", [])
    ]


def llm_id2llm_type(llm_id):
    from api.db.services.llm_service import TenantLLMService

    llm_id, *_ = TenantLLMService.split_model_name_and_factory(llm_id)

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
            tks_cnts.append({"role": m["role"], "count": num_tokens_from_string(m["content"])})
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
        m = encoder.decode(encoder.encode(m)[: max_length - ll2])
        msg[0]["content"] = m
        return max_length, msg

    m = msg_[-1]["content"]
    m = encoder.decode(encoder.encode(m)[: max_length - ll2])
    msg[-1]["content"] = m
    return max_length, msg


def kb_prompt(kbinfos, max_tokens, prefix=""):
    from api.db.services.document_service import DocumentService

    knowledges = [ck["content_with_weight"] for ck in kbinfos["chunks"]]
    used_token_count = 0
    chunks_num = 0
    for i, c in enumerate(knowledges):
        used_token_count += num_tokens_from_string(c)
        chunks_num += 1
        if max_tokens * 0.97 < used_token_count:
            knowledges = knowledges[:i]
            logging.warning(f"Not all the retrieval into prompt: {i + 1}/{len(knowledges)}")
            break

    docs = DocumentService.get_by_ids([ck["doc_id"] for ck in kbinfos["chunks"][:chunks_num]])
    docs = {d.id: d.meta_fields for d in docs}

    def draw_node(k, line):
        return f"\n├── {k}: " + re.sub(r"\n+", " ", line, flags=re.DOTALL)

    knowledges = []
    for i, ck in enumerate(kbinfos["chunks"][:chunks_num]):
        cnt = f"\nID: " + (f"{prefix}_{i}" if prefix else f"{i}")
        cnt += draw_node("Title", ck["docnm_kwd"])
        cnt += draw_node("URL", ck['url'])  if "url" in ck else ""
        for k, v in docs.get(ck["doc_id"], {}).items():
            cnt += draw_node(k, v)
        cnt += "\n└── Content:\n"
        cnt += ck["content_with_weight"]
        knowledges.append(cnt)

    return knowledges


def citation_prompt():
    return """

# Citation requirements:

- Use a uniform citation format such as [ID:i] [ID:j], where "i" and "j" are document IDs enclosed in square brackets. Separate multiple IDs with spaces (e.g., [ID:0] [ID:1]).
- Citation markers must be placed at the end of a sentence, separated by a space from the final punctuation (e.g., period, question mark). A maximum of 4 citations are allowed per sentence.
- DO NOT insert CITATION in the answer if the content is not from retrieved chunks.
- DO NOT use standalone Document IDs (e.g., '#ID#').
- Citations ALWAYS in the "[ID:i]" format.
- STRICTLY prohibit the use of strikethrough symbols (e.g., ~~) or any other non-standard formatting syntax.
- Any failure to adhere to the above rules, including but not limited to incorrect formatting, use of prohibited styles, or unsupported citations, will be considered an error, and no citation will be added for that sentence.

--- Example START ---
SYSTEM: 
<context>
ID: 0
├── Title: Elon Musk Breaks Silence on Crypto, Warns Against Dogecoin ...
├── URL: https://blockworks.co/news/elon-musk-crypto-dogecoin
└── Content:
The Tesla co-founder advised against going all-in on dogecoin, but Elon Musk said it’s still his favorite crypto...

ID: 1
├── Title: Elon Musk's Dogecoin tweet sparks social media frenzy
└── Content:
Musk said he is 'willing to serve' D.O.G.E. – shorthand for Dogecoin.

ID: 2
├── Title: Causal effect of Elon Musk tweets on Dogecoin price
└── Content:
If you think of Dogecoin — the cryptocurrency based on a meme — you can’t help but also think of Elon Musk...

ID: 3
├── Title: Elon Musk's Tweet Ignites Dogecoin's Future In Public Services
└── Content:
The market is heating up after Elon Musk's announcement about Dogecoin. Is this a new era for crypto?...

</context>

USER: What's the Elon's view on dogecoin?

ASSISTANT: Musk has consistently expressed his fondness for Dogecoin, often citing its humor and the inclusion of dogs in its branding. He has referred to it as his favorite cryptocurrency [ID:0] [ID:1].
Recently, Musk has hinted at potential future roles for Dogecoin. His tweets have sparked speculation about Dogecoin's potential integration into public services [ID:3].
Overall, while Musk enjoys Dogecoin and often promotes it, he also warns against over-investing in it, reflecting both his personal amusement and caution regarding its speculative nature.

--- Example END ---

"""


def keyword_extraction(chat_mdl, content, topn=3):
    prompt = f"""
Role: You are a text analyzer.
Task: Extract the most important keywords/phrases of a given piece of text content.
Requirements:
  - Summarize the text content, and give the top {topn} important keywords/phrases.
  - The keywords MUST be in the same language as the given piece of text content.
  - The keywords are delimited by ENGLISH COMMA.
  - Output keywords ONLY.

### Text Content
{content}

"""
    msg = [{"role": "system", "content": prompt}, {"role": "user", "content": "Output: "}]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = chat_mdl.chat(prompt, msg[1:], {"temperature": 0.2})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


def question_proposal(chat_mdl, content, topn=3):
    prompt = f"""
Role: You are a text analyzer.
Task: Propose {topn} questions about a given piece of text content.
Requirements:
  - Understand and summarize the text content, and propose the top {topn} important questions.
  - The questions SHOULD NOT have overlapping meanings.
  - The questions SHOULD cover the main content of the text as much as possible.
  - The questions MUST be in the same language as the given piece of text content.
  - One question per line.
  - Output questions ONLY.

### Text Content
{content}

"""
    msg = [{"role": "system", "content": prompt}, {"role": "user", "content": "Output: "}]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = chat_mdl.chat(prompt, msg[1:], {"temperature": 0.2})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


def full_question(tenant_id, llm_id, messages, language=None):
    from api.db.services.llm_service import LLMBundle

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
  - If the user's latest question is already complete, don't do anything, just return the original question.
  - DON'T generate anything except a refined question."""
    if language:
        prompt += f"""
  - Text generated MUST be in {language}."""
    else:
        prompt += """
  - Text generated MUST be in the same language as the original user's question.
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
    ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
    return ans if ans.find("**ERROR**") < 0 else messages[-1]["content"]


def cross_languages(tenant_id, llm_id, query, languages=[]):
    from api.db.services.llm_service import LLMBundle

    if llm_id and llm_id2llm_type(llm_id) == "image2text":
        chat_mdl = LLMBundle(tenant_id, LLMType.IMAGE2TEXT, llm_id)
    else:
        chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, llm_id)

    sys_prompt = """
Act as a streamlined multilingual translator. Strictly output translations separated by ### without any explanations or formatting. Follow these rules:

1. Accept batch translation requests in format:
[source text]
=== 
[target languages separated by commas]

2. Always maintain:
- Original formatting (tables/lists/spacing)
- Technical terminology accuracy
- Cultural context appropriateness

3. Output format:
[language1 translation] 
### 
[language1 translation]

**Examples:**
Input:
Hello World! Let's discuss AI safety.
===
Chinese, French, Japanese

Output:
你好世界！让我们讨论人工智能安全问题。
###
Bonjour le monde ! Parlons de la sécurité de l'IA.
###
こんにちは世界！AIの安全性について話し合いましょう。
"""
    user_prompt = f"""
Input:
{query}
===
{", ".join(languages)}

Output:
"""

    ans = chat_mdl.chat(sys_prompt, [{"role": "user", "content": user_prompt}], {"temperature": 0.2})
    ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
    if ans.find("**ERROR**") >= 0:
        return query
    return "\n".join([a for a in re.sub(r"(^Output:|\n+)", "", ans, flags=re.DOTALL).split("===") if a.strip()])


def content_tagging(chat_mdl, content, all_tags, examples, topn=3):
    prompt = f"""
Role: You are a text analyzer.

Task: Add tags (labels) to a given piece of text content based on the examples and the entire tag set.

Steps:
  - Review the tag/label set.
  - Review examples which all consist of both text content and assigned tags with relevance score in JSON format.
  - Summarize the text content, and tag it with the top {topn} most relevant tags from the set of tags/labels and the corresponding relevance score.

Requirements:
  - The tags MUST be from the tag set.
  - The output MUST be in JSON format only, the key is tag and the value is its relevance score.
  - The relevance score must range from 1 to 10.
  - Output keywords ONLY.

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
    msg = [{"role": "system", "content": prompt}, {"role": "user", "content": "Output: "}]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = chat_mdl.chat(prompt, msg[1:], {"temperature": 0.5})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        raise Exception(kwd)

    try:
        obj = json_repair.loads(kwd)
    except json_repair.JSONDecodeError:
        try:
            result = kwd.replace(prompt[:-1], "").replace("user", "").replace("model", "").strip()
            result = "{" + result.split("{")[1].split("}")[0] + "}"
            obj = json_repair.loads(result)
        except Exception as e:
            logging.exception(f"JSON parsing error: {result} -> {e}")
            raise e
    res = {}
    for k, v in obj.items():
        try:
            if int(v) > 0:
                res[str(k)] = int(v)
        except Exception:
            pass
    return res


def vision_llm_describe_prompt(page=None) -> str:
    prompt_en = """
INSTRUCTION:
Transcribe the content from the provided PDF page image into clean Markdown format.
- Only output the content transcribed from the image.
- Do NOT output this instruction or any other explanation.
- If the content is missing or you do not understand the input, return an empty string.

RULES:
1. Do NOT generate examples, demonstrations, or templates.
2. Do NOT output any extra text such as 'Example', 'Example Output', or similar.
3. Do NOT generate any tables, headings, or content that is not explicitly present in the image.
4. Transcribe content word-for-word. Do NOT modify, translate, or omit any content.
5. Do NOT explain Markdown or mention that you are using Markdown.
6. Do NOT wrap the output in ```markdown or ``` blocks.
7. Only apply Markdown structure to headings, paragraphs, lists, and tables, strictly based on the layout of the image. Do NOT create tables unless an actual table exists in the image.
8. Preserve the original language, information, and order exactly as shown in the image.
"""

    if page is not None:
        prompt_en += f"\nAt the end of the transcription, add the page divider: `--- Page {page} ---`."

    prompt_en += """
FAILURE HANDLING:
- If you do not detect valid content in the image, return an empty string.
"""
    return prompt_en


def vision_llm_figure_describe_prompt() -> str:
    prompt = """
You are an expert visual data analyst. Analyze the image and provide a comprehensive description of its content. Focus on identifying the type of visual data representation (e.g., bar chart, pie chart, line graph, table, flowchart), its structure, and any text captions or labels included in the image.

Tasks:
1. Describe the overall structure of the visual representation. Specify if it is a chart, graph, table, or diagram.
2. Identify and extract any axes, legends, titles, or labels present in the image. Provide the exact text where available.
3. Extract the data points from the visual elements (e.g., bar heights, line graph coordinates, pie chart segments, table rows and columns).
4. Analyze and explain any trends, comparisons, or patterns shown in the data.
5. Capture any annotations, captions, or footnotes, and explain their relevance to the image.
6. Only include details that are explicitly present in the image. If an element (e.g., axis, legend, or caption) does not exist or is not visible, do not mention it.

Output format (include only sections relevant to the image content):
- Visual Type: [Type]
- Title: [Title text, if available]
- Axes / Legends / Labels: [Details, if available]
- Data Points: [Extracted data]
- Trends / Insights: [Analysis and interpretation]
- Captions / Annotations: [Text and relevance, if available]

Ensure high accuracy, clarity, and completeness in your analysis, and include only the information present in the image. Avoid unnecessary statements about missing elements.
"""
    return prompt


def tool_schema(tools_description: list[dict], complete_task=False):
    if not tools_description:
        return ""
    desc = {}
    if complete_task:
        desc[COMPLETE_TASK] = {
            "type": "function",
            "function": {
                "name": COMPLETE_TASK,
                "description": "When you have the final answer and are ready to complete the task, call this function with your answer",
                "parameters": {
                    "type": "object",
                    "properties": {"answer":{"type":"string", "description": "The final answer to the user's question"}},
                    "required": ["answer"]
                }
            }
        }
    for tool in tools_description:
        desc[tool["function"]["name"]] = tool

    return "\n\n".join([f"## {i+1}. {fnm}\n{json.dumps(des, ensure_ascii=False, indent=4)}" for i, (fnm, des) in enumerate(desc.items())])


def analyze_task(chat_mdl, history:list, tools_description: list[dict]):
    tools_desc = tool_schema(tools_description)
    task = ""
    for h in history:
        if h["role"] == "user":
            task = h["content"]
            break
    context = ""
    if len(history) > 6:
        for h in history[-6:]:
            if h["role"] == "system":
                continue
            context += f"\n{h['role'].upper()}: {h['content'][:1024] + ('...' if len(h['content'])>1024 else '')}"

    system_prompt = """
Your responsibility is to execute assigned tasks to a high standard. Please:
1. Carefully analyze the task requirements.
2. Develop a reasonable execution plan.
3. Execute step-by-step and document the reasoning process.
4. Provide clear and accurate results.

If difficulties are encountered, clearly state the problem and explore alternative approaches.
Completed Task Analysis Prompt:
"""
    user_prompt = f"""
Please analyze the following task:

Task: {task}

{f"Context: {context}" if context else ""}

**Analysis Requirements:**
1. What is the core objective of the task?
2. What is the complexity level of the task?
3. What types of specialized skills are required?
4. Does the task need to be decomposed into subtasks? (If yes, propose the subtask structure)
5. What are the expected success criteria?

**Available Sub-Agents and Their Specializations:**
{tools_desc}

Provide a detailed analysis of the task based on the above requirements.
"""
    kwd, tk = chat_mdl._chat([{"role": "system", "content": system_prompt},{"role": "user", "content": user_prompt}], {})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return "", 0
    return kwd, tk


def next_step(chat_mdl, history:list, tools_description: list[dict]):
    if not tools_description:
        return ""
    task_analisys, tk = analyze_task(chat_mdl, history, tools_description)
    desc = tool_schema(tools_description)
    sys_prompt = f"""
You are an expert Planning Agent tasked with solving problems efficiently through structured plans.
Your job is:
1. Based on the task analysis, chose a right tool to execute.
2. Track progress and adapt plans(tool calls) when necessary.
3. Use `complete_task` if no further step you need to take from tools. (All necessary steps done or little hope to be done)

# ========== TASK ANALYSIS =============
{task_analisys}


# ==========  TOOLS (JSON-Schema) ==========
You may invoke only the tools listed below.  
Return a JSON object with exactly two top-level keys:  
• "name": the tool to call  
• "arguments": an object whose keys/values satisfy the schema

{desc}

# ==========  RESPONSE FORMAT ==========
✦ **When you need a tool**  
Return ONLY the Json (no additional keys, no commentary, end with `<|stop|>`), such as following:
{{
  "name": "<tool_name>",
  "arguments": {{ /* tool arguments matching its schema */ }}
}}<|stop|>

✦ **When you are certain the task is solved OR no further information can be obtained**  
Return ONLY:
{{
  "name": "complete_task",
  "arguments": {{ "answer": "<final answer text>" }}
}}<|stop|>

⚠️ Any output that is not valid JSON or that contains extra fields will be rejected.

# ==========  REASONING & REFLECTION ==========
You may think privately (not shown to the user) before producing each JSON object.  
Internal guideline:
1. **Reason**: Analyse the user question; decide which tool (if any) is needed.  
2. **Act**: Emit the JSON object to call the tool.  

"""
    user_prompt = "\nWhat's the next tool to call? If ready OR IMPOSSIBLE TO BE READY, then call `complete_task`."
    hist = deepcopy(history)
    hist[0]["content"] = sys_prompt
    if hist[-1]["role"] == "user":
        hist[-1]["content"] += user_prompt
    else:
        hist.append({"role": "user", "content": user_prompt})
    json_str, tk_cnt = chat_mdl._chat(hist, {}, stop=["<|stop|>"])
    tk_cnt += tk
    json_str = re.sub(r"^.*</think>", "", json_str, flags=re.DOTALL)
    return json_str, tk_cnt


def post_function_call_promt(function_name, result):
    return f"""
**Act**
Call `{function_name}`

**Observe**
After calling the function above, the response is as following:
{result}

**Reflect**: 

"""


def tool_call_summary(inputs, results):
    results = re.sub(r"!\[[a-z]+\]\(data:image/png;base64[^()]+\)", "", str(results), flags=re.DOTALL)[:4096]
    return f"""**Task Instruction:**

    You are tasked with reading and analyzing tool call result based on the following inputs: **Inputs for current call**, and **Results**. Your objective is to extract relevant and helpful information for **Inputs for current call** from the **Results** and seamlessly integrate this information into the previous steps to continue reasoning for the original question.

    **Guidelines:**

    1. **Analyze the Results:**
    - Carefully review the content of each results of tool call.
    - Identify factual information that is relevant to the **Inputs for current call** and can aid in the reasoning process for the original question.

    2. **Extract Relevant Information:**
    - Select the information from the Searched Web Pages that directly contributes to advancing the previous reasoning steps.
    - Ensure that the extracted information is accurate and relevant.

    - **Inputs for current call:**  
    {inputs}

    - **Results:**  
    {results}

    """