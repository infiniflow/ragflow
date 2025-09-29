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
import re
import json
import math
import time
import jinja2
import base64
import logging
import datetime
import unicodedata
import pdfplumber
import json_repair
from os import write
from io import BytesIO
from typing import Tuple
from copy import deepcopy
from api.db import LLMType
from api.utils import hash_str2int
from rag.prompts.prompt_template import load_prompt
from rag.settings import TAG_FLD
from rag.utils import encoder, num_tokens_from_string


STOP_TOKEN="<|STOP|>"
COMPLETE_TASK="complete_task"
MAX_RETRIES = 3

def get_value(d, k1, k2):
    return d.get(k1, d.get(k2))


def chunks_format(reference):

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


def kb_prompt(kbinfos, max_tokens, hash_id=False):
    from api.db.services.document_service import DocumentService

    knowledges = [get_value(ck, "content", "content_with_weight") for ck in kbinfos["chunks"]]
    kwlg_len = len(knowledges)
    used_token_count = 0
    chunks_num = 0
    for i, c in enumerate(knowledges):
        if not c:
            continue
        used_token_count += num_tokens_from_string(c)
        chunks_num += 1
        if max_tokens * 0.97 < used_token_count:
            knowledges = knowledges[:i]
            logging.warning(f"Not all the retrieval into prompt: {len(knowledges)}/{kwlg_len}")
            break

    docs = DocumentService.get_by_ids([get_value(ck, "doc_id", "document_id") for ck in kbinfos["chunks"][:chunks_num]])
    docs = {d.id: d.meta_fields for d in docs}

    def draw_node(k, line):
        if line is not None and not isinstance(line, str):
            line = str(line)
        if not line:
            return ""
        return f"\n├── {k}: " + re.sub(r"\n+", " ", line, flags=re.DOTALL)

    knowledges = []
    for i, ck in enumerate(kbinfos["chunks"][:chunks_num]):
        cnt = "\nID: {}".format(i if not hash_id else hash_str2int(get_value(ck, "id", "chunk_id"), 100))
        cnt += draw_node("Title", get_value(ck, "docnm_kwd", "document_name"))
        cnt += draw_node("URL", ck['url'])  if "url" in ck else ""
        for k, v in docs.get(get_value(ck, "doc_id", "document_id"), {}).items():
            cnt += draw_node(k, v)
        cnt += "\n└── Content:\n"
        cnt += get_value(ck, "content", "content_with_weight")
        knowledges.append(cnt)

    return knowledges


CITATION_PROMPT_TEMPLATE = load_prompt("citation_prompt")
CITATION_PLUS_TEMPLATE = load_prompt("citation_plus")
CONTENT_TAGGING_PROMPT_TEMPLATE = load_prompt("content_tagging_prompt")
CROSS_LANGUAGES_SYS_PROMPT_TEMPLATE = load_prompt("cross_languages_sys_prompt")
CROSS_LANGUAGES_USER_PROMPT_TEMPLATE = load_prompt("cross_languages_user_prompt")
FULL_QUESTION_PROMPT_TEMPLATE = load_prompt("full_question_prompt")
KEYWORD_PROMPT_TEMPLATE = load_prompt("keyword_prompt")
QUESTION_PROMPT_TEMPLATE = load_prompt("question_prompt")
VISION_LLM_DESCRIBE_PROMPT = load_prompt("vision_llm_describe_prompt")
VISION_LLM_FIGURE_DESCRIBE_PROMPT = load_prompt("vision_llm_figure_describe_prompt")

ANALYZE_TASK_SYSTEM = load_prompt("analyze_task_system")
ANALYZE_TASK_USER = load_prompt("analyze_task_user")
NEXT_STEP = load_prompt("next_step")
REFLECT = load_prompt("reflect")
SUMMARY4MEMORY = load_prompt("summary4memory")
RANK_MEMORY = load_prompt("rank_memory")
META_FILTER = load_prompt("meta_filter")
ASK_SUMMARY = load_prompt("ask_summary")

PROMPT_JINJA_ENV = jinja2.Environment(autoescape=False, trim_blocks=True, lstrip_blocks=True)


def citation_prompt(user_defined_prompts: dict={}) -> str:
    template = PROMPT_JINJA_ENV.from_string(user_defined_prompts.get("citation_guidelines", CITATION_PROMPT_TEMPLATE))
    return template.render()


def citation_plus(sources: str) -> str:
    template = PROMPT_JINJA_ENV.from_string(CITATION_PLUS_TEMPLATE)
    return template.render(example=citation_prompt(), sources=sources)


def keyword_extraction(chat_mdl, content, topn=3):
    template = PROMPT_JINJA_ENV.from_string(KEYWORD_PROMPT_TEMPLATE)
    rendered_prompt = template.render(content=content, topn=topn)

    msg = [{"role": "system", "content": rendered_prompt}, {"role": "user", "content": "Output: "}]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = chat_mdl.chat(rendered_prompt, msg[1:], {"temperature": 0.2})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


def question_proposal(chat_mdl, content, topn=3):
    template = PROMPT_JINJA_ENV.from_string(QUESTION_PROMPT_TEMPLATE)
    rendered_prompt = template.render(content=content, topn=topn)

    msg = [{"role": "system", "content": rendered_prompt}, {"role": "user", "content": "Output: "}]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = chat_mdl.chat(rendered_prompt, msg[1:], {"temperature": 0.2})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


def full_question(tenant_id=None, llm_id=None, messages=[], language=None, chat_mdl=None):
    from api.db import LLMType
    from api.db.services.llm_service import LLMBundle
    from api.db.services.tenant_llm_service import TenantLLMService

    if not chat_mdl:
        if TenantLLMService.llm_id2llm_type(llm_id) == "image2text":
            chat_mdl = LLMBundle(tenant_id, LLMType.IMAGE2TEXT, llm_id)
        else:
            chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, llm_id)
    conv = []
    for m in messages:
        if m["role"] not in ["user", "assistant"]:
            continue
        conv.append("{}: {}".format(m["role"].upper(), m["content"]))
    conversation = "\n".join(conv)
    today = datetime.date.today().isoformat()
    yesterday = (datetime.date.today() - datetime.timedelta(days=1)).isoformat()
    tomorrow = (datetime.date.today() + datetime.timedelta(days=1)).isoformat()

    template = PROMPT_JINJA_ENV.from_string(FULL_QUESTION_PROMPT_TEMPLATE)
    rendered_prompt = template.render(
        today=today,
        yesterday=yesterday,
        tomorrow=tomorrow,
        conversation=conversation,
        language=language,
    )

    ans = chat_mdl.chat(rendered_prompt, [{"role": "user", "content": "Output: "}])
    ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
    return ans if ans.find("**ERROR**") < 0 else messages[-1]["content"]


def cross_languages(tenant_id, llm_id, query, languages=[]):
    from api.db import LLMType
    from api.db.services.llm_service import LLMBundle
    from api.db.services.tenant_llm_service import TenantLLMService

    if llm_id and TenantLLMService.llm_id2llm_type(llm_id) == "image2text":
        chat_mdl = LLMBundle(tenant_id, LLMType.IMAGE2TEXT, llm_id)
    else:
        chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, llm_id)

    rendered_sys_prompt = PROMPT_JINJA_ENV.from_string(CROSS_LANGUAGES_SYS_PROMPT_TEMPLATE).render()
    rendered_user_prompt = PROMPT_JINJA_ENV.from_string(CROSS_LANGUAGES_USER_PROMPT_TEMPLATE).render(query=query, languages=languages)

    ans = chat_mdl.chat(rendered_sys_prompt, [{"role": "user", "content": rendered_user_prompt}], {"temperature": 0.2})
    ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
    if ans.find("**ERROR**") >= 0:
        return query
    return "\n".join([a for a in re.sub(r"(^Output:|\n+)", "", ans, flags=re.DOTALL).split("===") if a.strip()])


def content_tagging(chat_mdl, content, all_tags, examples, topn=3):
    template = PROMPT_JINJA_ENV.from_string(CONTENT_TAGGING_PROMPT_TEMPLATE)

    for ex in examples:
        ex["tags_json"] = json.dumps(ex[TAG_FLD], indent=2, ensure_ascii=False)

    rendered_prompt = template.render(
        topn=topn,
        all_tags=all_tags,
        examples=examples,
        content=content,
    )

    msg = [{"role": "system", "content": rendered_prompt}, {"role": "user", "content": "Output: "}]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = chat_mdl.chat(rendered_prompt, msg[1:], {"temperature": 0.5})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        raise Exception(kwd)

    try:
        obj = json_repair.loads(kwd)
    except json_repair.JSONDecodeError:
        try:
            result = kwd.replace(rendered_prompt[:-1], "").replace("user", "").replace("model", "").strip()
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
    template = PROMPT_JINJA_ENV.from_string(VISION_LLM_DESCRIBE_PROMPT)

    return template.render(page=page)


def vision_llm_figure_describe_prompt() -> str:
    template = PROMPT_JINJA_ENV.from_string(VISION_LLM_FIGURE_DESCRIBE_PROMPT)
    return template.render()


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


def form_history(history, limit=-6):
    context = ""
    for h in history[limit:]:
        if h["role"] == "system":
            continue
        role = "USER"
        if h["role"].upper()!= role:
            role = "AGENT"
        context += f"\n{role}: {h['content'][:2048] + ('...' if len(h['content'])>2048 else '')}"
    return context


def analyze_task(chat_mdl, prompt, task_name, tools_description: list[dict], user_defined_prompts: dict={}):
    tools_desc = tool_schema(tools_description)
    context = ""

    if user_defined_prompts.get("task_analysis"):
        template = PROMPT_JINJA_ENV.from_string(user_defined_prompts["task_analysis"])
    else:
        template = PROMPT_JINJA_ENV.from_string(ANALYZE_TASK_SYSTEM + "\n\n" + ANALYZE_TASK_USER)
    context = template.render(task=task_name, context=context, agent_prompt=prompt, tools_desc=tools_desc)
    kwd = chat_mdl.chat(context, [{"role": "user", "content": "Please analyze it."}])
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


def next_step(chat_mdl, history:list, tools_description: list[dict], task_desc, user_defined_prompts: dict={}):
    if not tools_description:
        return ""
    desc = tool_schema(tools_description)
    template = PROMPT_JINJA_ENV.from_string(user_defined_prompts.get("plan_generation", NEXT_STEP))
    user_prompt = "\nWhat's the next tool to call? If ready OR IMPOSSIBLE TO BE READY, then call `complete_task`."
    hist = deepcopy(history)
    if hist[-1]["role"] == "user":
        hist[-1]["content"] += user_prompt
    else:
        hist.append({"role": "user", "content": user_prompt})
    json_str = chat_mdl.chat(template.render(task_analysis=task_desc, desc=desc, today=datetime.datetime.now().strftime("%Y-%m-%d")),
                             hist[1:], stop=["<|stop|>"])
    tk_cnt = num_tokens_from_string(json_str)
    json_str = re.sub(r"^.*</think>", "", json_str, flags=re.DOTALL)
    return json_str, tk_cnt


def reflect(chat_mdl, history: list[dict], tool_call_res: list[Tuple], user_defined_prompts: dict={}):
    tool_calls = [{"name": p[0], "result": p[1]} for p in tool_call_res]
    goal = history[1]["content"]
    template = PROMPT_JINJA_ENV.from_string(user_defined_prompts.get("reflection", REFLECT))
    user_prompt = template.render(goal=goal, tool_calls=tool_calls)
    hist = deepcopy(history)
    if hist[-1]["role"] == "user":
        hist[-1]["content"] += user_prompt
    else:
        hist.append({"role": "user", "content": user_prompt})
    _, msg = message_fit_in(hist, chat_mdl.max_length)
    ans = chat_mdl.chat(msg[0]["content"], msg[1:])
    ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
    return """
**Observation**
{}

**Reflection**
{}
    """.format(json.dumps(tool_calls, ensure_ascii=False, indent=2), ans)


def form_message(system_prompt, user_prompt):
    return [{"role": "system", "content": system_prompt},{"role": "user", "content": user_prompt}]


def tool_call_summary(chat_mdl, name: str, params: dict, result: str, user_defined_prompts: dict={}) -> str:
    template = PROMPT_JINJA_ENV.from_string(SUMMARY4MEMORY)
    system_prompt = template.render(name=name,
                           params=json.dumps(params, ensure_ascii=False, indent=2),
                           result=result)
    user_prompt = "→ Summary: "
    _, msg = message_fit_in(form_message(system_prompt, user_prompt), chat_mdl.max_length)
    ans = chat_mdl.chat(msg[0]["content"], msg[1:])
    return re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)


def rank_memories(chat_mdl, goal:str, sub_goal:str, tool_call_summaries: list[str], user_defined_prompts: dict={}):
    template = PROMPT_JINJA_ENV.from_string(RANK_MEMORY)
    system_prompt = template.render(goal=goal, sub_goal=sub_goal, results=[{"i": i, "content": s} for i,s in enumerate(tool_call_summaries)])
    user_prompt = " → rank: "
    _, msg = message_fit_in(form_message(system_prompt, user_prompt), chat_mdl.max_length)
    ans = chat_mdl.chat(msg[0]["content"], msg[1:], stop="<|stop|>")
    return re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)


def gen_meta_filter(chat_mdl, meta_data:dict, query: str) -> list:
    sys_prompt = PROMPT_JINJA_ENV.from_string(META_FILTER).render(
        current_date=datetime.datetime.today().strftime('%Y-%m-%d'),
        metadata_keys=json.dumps(meta_data),
        user_question=query
    )
    user_prompt = "Generate filters:"
    ans = chat_mdl.chat(sys_prompt, [{"role": "user", "content": user_prompt}])
    ans = re.sub(r"(^.*</think>|```json\n|```\n*$)", "", ans, flags=re.DOTALL)
    try:
        ans = json_repair.loads(ans)
        assert isinstance(ans, list), ans
        return ans
    except Exception:
        logging.exception(f"Loading json failure: {ans}")
    return []


def gen_json(system_prompt:str, user_prompt:str, chat_mdl):
    _, msg = message_fit_in(form_message(system_prompt, user_prompt), 1000000)
    ans = chat_mdl.chat(msg[0]["content"], msg[1:])
    ans = re.sub(r"(^.*</think>|```json\n|```\n*$)", "", ans, flags=re.DOTALL)
    try:
        return json_repair.loads(ans)
    except Exception:
        logging.exception(f"Loading json failure: {ans}")


TOC_DETECTION = load_prompt("toc_detection")
def detect_table_of_contents(pages:list[str], chat_mdl):
    for i, sec in enumerate(pages):
        if sec == "":
            continue
        ans = gen_json(PROMPT_JINJA_ENV.from_string(TOC_DETECTION).render(), f"Input:{sec}", chat_mdl)
        print(f"TOC detection for page {i}: {ans}")
        if ans.get("exists", False):
            return i
    return -1


TOC_FROM_IMG_SYSTEM = load_prompt("toc_from_img_system")
TOC_FROM_IMG_USER = load_prompt("toc_from_img_user")
def gen_toc_from_img(img_url, vision_mdl):
    ans = gen_json(PROMPT_JINJA_ENV.from_string(TOC_FROM_IMG_SYSTEM).render(),
                   PROMPT_JINJA_ENV.from_string(TOC_FROM_IMG_USER).render(url=img_url), 
                   vision_mdl)
    return ans

TOC_LEVELS = load_prompt("assign_toc_levels")
def assign_toc_levels(toc_secs, chat_mdl):
    ans = gen_json(PROMPT_JINJA_ENV.from_string(TOC_LEVELS).render(),
                   str(toc_secs),
                   chat_mdl
                   )
    
    return ans

def gen_image_from_page(page):
    pil_img = page.to_image(resolution=300, antialias=True).original
    img_buf = BytesIO()

    if pil_img.mode in ("RGBA", "LA"):
        pil_img = pil_img.convert("RGB")    

    pil_img.save(img_buf, format="JPEG")
    b64 = base64.b64encode(img_buf.getvalue()).decode("utf-8")
    img_buf.close()

    img_url = f"data:image/jpeg;base64,{b64}"
    return img_url


def build_img_toc_messages(url):
    return [
        {
            "role": "system",
            "content": """You are a strict Table-of-Contents (TOC) extractor.

INPUT:
- You will receive one page of a PDF as an image.

YOUR TASK:
1. Determine if this page is a TOC (Table of Contents).
   - A TOC page usually has short, list-like headings (e.g. "Chapter 1", "第一章", "Section 2.3"),
     often aligned or followed by dots/leaders and page numbers.
   - A TOC page contains at least 3 such distinct headings.
   - If the page is mostly narrative text, title page, author info, or ads, it is NOT a TOC.

2. If it IS a TOC page:
   - Return ONLY a **valid JSON array**.
   - Each element must be an object with two fields:
       {"structure": "0", "title": "<the heading text>"}
   - "structure" must always be the string "0".
   - "title" must be the exact heading text extracted from the TOC (do not invent or summarize).
   - Keep the order as it appears on the page.

3. If it is NOT a TOC page:
   - Return ONLY the following JSON:
       [
         {"structure": "0", "title": "-1"}
       ]

STRICT RULES:
- Do NOT include explanations, reasoning, or any text outside the JSON.
- Do NOT wrap the output in ```json fences.
- Output must start with `[` and end with `]`.
- Ensure the JSON is syntactically valid (no trailing commas).

EXAMPLES:

Example A (TOC page):
[
  {"structure": "0", "title": "Introduction"},
  {"structure": "0", "title": "Chapter 1: Basics"},
  {"structure": "0", "title": "Chapter 2: Advanced Topics"}
]

Example B (NOT a TOC page):
[
  {"structure": "0", "title": "-1"}
]"""
        },
        {
            "role": "user",
            "content": [
                {"type": "image_url", "image_url": {"url": url, "detail": "high"}}
            ],
        },
    ]


def gen_toc_from_pdf(filename, empty_pages, start_page, chat_mdl):
    toc = []

    with pdfplumber.open(BytesIO(filename)) as pdf:
        for i, page in enumerate(prune_pages(pdf.pages)):
            if i in empty_pages or i < start_page:
                continue

            # Render the page to a high-res JPEG data URL for vision model consumption
            img_url = gen_image_from_page(page)
            msg = build_img_toc_messages(img_url)

            # Exponential backoff on transient parsing/LLM errors
            delay = 1.0
            for attempt in range(MAX_RETRIES):
                try:
                    raw = chat_mdl.chat(
                        msg[0]["content"],
                        msg[1:],
                        {"temperature": 0.2},
                    )
                    raw = re.sub(
                        r"(^.*</think>|```json\n|```\n*$)",
                        "",
                        raw,
                        flags=re.DOTALL,
                    )
                    ans = json_repair.loads(raw)
                    break
                except Exception as e:
                    logging.warning(
                        f"TOC page {i} attempt {attempt}/{MAX_RETRIES} failed: {e}"
                    )
                    if attempt < MAX_RETRIES:
                        time.sleep(delay)
                        delay *= 2
                    else:
                        logging.exception(
                            f"TOC page {i} failed after retries."
                        )
                        ans = None

            # If the page is not a TOC, stop and return collected TOC with the page index
            if ans[0].get("title") == "-1":
                return toc, i
            else:
                toc.extend(ans)

        return toc, -1


def prune_pages(pages):
    # Heuristic: only scan the first 25% (up to 25) pages as likely TOC candidates
    total = len(pages)

    if total <= 100:
        N = math.ceil(total * 0.25)
    else:
        N = 25
    
    N = max(1, N)
    return pages[:N]


def get_page_num(section):
    # Extract the starting page number from a section meta string like '@7-8'
    if not section:
        return 0
    
    poss = section[1].split('\t')[0]
    head = poss.lstrip('@')     # '7-8'
    m = re.match(r'^(\d+)(?:-(\d+))?$', head)
    if not m:
        return 0

    # Return the first page number as the section's page hint
    return int(m.group(1))


def match_toc_sections(
        sections,
        toc,
        start_section_idx,
        min_coverage
):
    """
    Match each TOC title to the best section index starting from start_section_idx.
    Cursor moves forward after a match; unmatched items get index -1.
    Only strip trailing page dots/numbers (e.g., "...... 12") and normalize (NFKC, spaces, lower).
    Leading numbering (e.g., "1.", "第一章", "Chapter 1") is intentionally preserved for matching.
    """
    import re
    import unicodedata
    from difflib import SequenceMatcher

    # Regex to remove only trailing dot leaders and page numbers (e.g., "...... 12", " 23")
    P_TRAILING_PAGE = re.compile(
        r"""(
              [\.\·…]{2,}\s*\d+\s*$   # ...... 12 / …… 23 / ···· 7
            | \s+\d+\s*$              #      12
        )""",
        re.X
    )

    def normalize(s: str) -> str:
        # Unicode normalize, lower, collapse whitespace
        if not s:
            return ""
        s = unicodedata.normalize("NFKC", s).lower()
        s = s.replace("\u3000", " ")           # full-width space -> half-width space
        s = re.sub(r"\s+", " ", s).strip()
        return s

    def strip_trailing_page(s: str) -> str:
        # Remove only trailing page cues; keep numbering/punctuation for multi-language robustness
        s = normalize(s)
        s = P_TRAILING_PAGE.sub("", s).strip()
        return s

    # Build normalized section candidates starting at the provided cursor
    norm_sections = []
    for idx, sec in enumerate(sections):
        if idx < max(0, start_section_idx):
            continue
        text = sec[0] if isinstance(sec, (list, tuple)) else str(sec)
        n_full = normalize(text)
        n_core = strip_trailing_page(text)   # only remove trailing page markers
        if n_core:
            norm_sections.append((idx, n_full, n_core))

    def similarity(a: str, b: str) -> float:
        if not a or not b:
            return 0.0
        return SequenceMatcher(None, a, b).ratio()

    res = []
    scan_from = max(0, start_section_idx)

    for item in toc:
        title = item.get("title") if isinstance(item, dict) else str(item)
        t_full = normalize(title)
        t_core = strip_trailing_page(title)

        best_idx = -1
        best_score = -1.0

        if t_core:
            # Linear forward scan to keep document order
            for idx, s_full, s_core in norm_sections:
                if idx < scan_from:
                    continue

                # Strong match: exact equality on full or core
                if s_full == t_full or s_core == t_core:
                    best_idx, best_score = idx, 1.0
                    break

                # Bidirectional containment with coverage threshold (symmetric-ish)
                if t_core in s_core or s_core in t_core:
                    overlap = min(len(t_core), len(s_core))
                    cov = overlap / max(len(t_core), len(s_core), 1)
                    if cov >= float(min_coverage) and cov > best_score:
                        best_idx, best_score = idx, cov
                        # Keep scanning; might find a higher score

                # Fuzzy similarity fallback
                sim = similarity(t_core, s_core)
                if sim >= float(min_coverage) and sim > best_score:
                    best_idx, best_score = idx, sim

        res.append((title, best_idx))
        if best_idx != -1:
            scan_from = best_idx + 1 

    return res


def run_toc(filename,
            sections,
            chat_mdl,
            vision_mdl,
            min_coverage = 0.5
            ):
    
    # 1) Get pages
    max_page = get_page_num(sections[-1])
    pages = ["" for _ in range(max_page)]
    page_begin_idx = [-1 for _ in range(max_page)] 
    for idx, sec in enumerate(sections):
        print(idx, "\t",sec)
        page_num = get_page_num(sec)
        pages[page_num-1] += sec[0] + "\n"
        if page_begin_idx[page_num-1] == -1:
            page_begin_idx[page_num-1] = idx    
    print("Max page number:", max_page)

    # 2) Prune pages to remove unlikely TOC candidates
    pruned_pages = prune_pages(pages)
    empty_pages = [i for i, p in enumerate(pruned_pages) if p == ""]
    
    # 3) Detect TOC
    toc_start_page = detect_table_of_contents(pruned_pages, chat_mdl)
    print("\n\nDetected TOC start page:\n", toc_start_page)

    # 4) Generate TOC from images
    toc_secs, start_page_idx = gen_toc_from_pdf(filename, empty_pages, toc_start_page, vision_mdl)
    print("\n\nDetected TOC sections:\n", toc_secs)

    # 5) Assign hierarchy levels to TOC
    toc_with_levels = assign_toc_levels(toc_secs, chat_mdl)
    print("\n\nDetected TOC with levels:\n", toc_with_levels)

    # 6) match TOC with sections
    start_section_idx = page_begin_idx[start_page_idx] if start_page_idx >=0 and start_page_idx < len(page_begin_idx) else 0
    print("\n\nStart section index for matching:", start_section_idx)

    pairs = match_toc_sections(sections, toc_with_levels, start_section_idx, min_coverage)
    print("\n\nMatched TOC sections with indices:\n", pairs)

    return pairs # [(title, section_idx), ...] 
