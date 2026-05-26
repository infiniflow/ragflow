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
import datetime
import json
import logging
import re
from copy import deepcopy
from typing import Tuple
from jinja2.sandbox import SandboxedEnvironment
import json_repair
import xxhash
from common.misc_utils import hash_str2int, thread_pool_exec
from rag.nlp import rag_tokenizer
from rag.prompts.template import load_prompt
from common.constants import TAG_FLD
from common.token_utils import encoder, num_tokens_from_string

STOP_TOKEN = "<|STOP|>"
COMPLETE_TASK = "complete_task"
INPUT_UTILIZATION = 0.5


def get_value(d, k1, k2):
    return d.get(k1, d.get(k2))


def chunks_format(reference):
    if not reference or not isinstance(reference, dict):
        return []
    raw_chunks = reference.get("chunks", [])
    if not isinstance(raw_chunks, list):
        return []
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
            "row_id": chunk.get("row_id"),
            "doc_type": get_value(chunk, "doc_type_kwd", "doc_type"),
            "document_metadata": chunk.get("document_metadata"),
        }
        for chunk in raw_chunks
        if isinstance(chunk, dict)
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

    def trim_content(content, limit):
        limit = max(0, limit)
        return encoder.decode(encoder.encode(content)[:limit])

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
    total = ll + ll2
    if total <= 0:
        logging.debug(
            "message_fit_in degenerate token counts total=%s max_length=%s ll=%s ll2=%s preserved_roles=%s",
            total,
            max_length,
            ll,
            ll2,
            [m.get("role") for m in msg],
        )
        return 0, msg

    if len(msg) == 1:
        msg[0]["content"] = trim_content(msg[0]["content"], max_length)
        return count(), msg

    if ll / total > 0.8:
        preserved_last = min(ll2, max_length)
        msg[-1]["content"] = trim_content(msg_[-1]["content"], preserved_last)
        remaining = max(0, max_length - preserved_last)
        msg[0]["content"] = trim_content(msg_[0]["content"], remaining)
        return count(), msg

    preserved_system = min(ll, max_length)
    msg[0]["content"] = trim_content(msg_[0]["content"], preserved_system)
    remaining = max(0, max_length - preserved_system)
    msg[-1]["content"] = trim_content(msg_[-1]["content"], remaining)
    return count(), msg


def kb_prompt(kbinfos, max_tokens, hash_id=False):
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

    def draw_node(k, line):
        if line is not None and not isinstance(line, str):
            line = str(line)
        if not line:
            return ""
        return f"\n├── {k}: " + re.sub(r"\n+", " ", line, flags=re.DOTALL)

    knowledges = []
    for i, ck in enumerate(kbinfos["chunks"][:chunks_num]):
        cnt = "\nID: {}".format(i if not hash_id else hash_str2int(get_value(ck, "id", "chunk_id"), 500))
        cnt += draw_node("Title", get_value(ck, "docnm_kwd", "document_name"))
        cnt += draw_node("URL", ck.get('url', ''))
        meta = ck.get("document_metadata") or {}
        for k, v in meta.items():
            cnt += draw_node(k, v)
        cnt += "\n└── Content:\n"
        cnt += get_value(ck, "content", "content_with_weight")
        knowledges.append(cnt)

    return knowledges


def memory_prompt(message_list, max_tokens):
    used_token_count = 0
    content_list = []
    for message in message_list:
        current_content_tokens = num_tokens_from_string(message["content"])
        if used_token_count + current_content_tokens > max_tokens * 0.97:
            logging.warning(f"Not all the retrieval into prompt: {len(content_list)}/{len(message_list)}")
            break
        content_list.append(message["content"])
        used_token_count += current_content_tokens
    return content_list


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
VISION_LLM_FIGURE_DESCRIBE_PROMPT_WITH_CONTEXT = load_prompt("vision_llm_figure_describe_prompt_with_context")
STRUCTURED_OUTPUT_PROMPT = load_prompt("structured_output_prompt")

ANALYZE_TASK_SYSTEM = load_prompt("analyze_task_system")
ANALYZE_TASK_USER = load_prompt("analyze_task_user")
NEXT_STEP = load_prompt("next_step")
REFLECT = load_prompt("reflect")
SUMMARY4MEMORY = load_prompt("summary4memory")
RANK_MEMORY = load_prompt("rank_memory")
META_FILTER = load_prompt("meta_filter")
ASK_SUMMARY = load_prompt("ask_summary")

PROMPT_JINJA_ENV = SandboxedEnvironment(
    autoescape=False, trim_blocks=True, lstrip_blocks=True
)


def citation_prompt(user_defined_prompts: dict = {}) -> str:
    template = PROMPT_JINJA_ENV.from_string(user_defined_prompts.get("citation_guidelines", CITATION_PROMPT_TEMPLATE))
    return template.render()


def citation_plus(sources: str) -> str:
    template = PROMPT_JINJA_ENV.from_string(CITATION_PLUS_TEMPLATE)
    return template.render(example=citation_prompt(), sources=sources)


async def keyword_extraction(chat_mdl, content, topn=3):
    template = PROMPT_JINJA_ENV.from_string(KEYWORD_PROMPT_TEMPLATE)
    rendered_prompt = template.render(content=content, topn=topn)

    msg = [{"role": "system", "content": rendered_prompt}, {"role": "user", "content": "Output: "}]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = await chat_mdl.async_chat(rendered_prompt, msg[1:], {"temperature": 0.2})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


async def question_proposal(chat_mdl, content, topn=3):
    template = PROMPT_JINJA_ENV.from_string(QUESTION_PROMPT_TEMPLATE)
    rendered_prompt = template.render(content=content, topn=topn)

    msg = [{"role": "system", "content": rendered_prompt}, {"role": "user", "content": "Output: "}]
    _, msg = message_fit_in(msg, chat_mdl.max_length)
    kwd = await chat_mdl.async_chat(rendered_prompt, msg[1:], {"temperature": 0.2})
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


async def full_question(tenant_id=None, llm_id=None, messages=[], language=None, chat_mdl=None):
    from common.constants import LLMType
    from api.db.services.llm_service import LLMBundle
    from api.db.services.tenant_llm_service import TenantLLMService
    from api.db.joint_services.tenant_model_service import get_model_config_by_type_and_name

    if not chat_mdl:
        if TenantLLMService.llm_id2llm_type(llm_id) == "image2text":
            chat_model_config = get_model_config_by_type_and_name(tenant_id, LLMType.IMAGE2TEXT, llm_id)
        else:
            chat_model_config = get_model_config_by_type_and_name(tenant_id, LLMType.CHAT, llm_id)
        chat_mdl = LLMBundle(tenant_id, chat_model_config)
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

    ans = await chat_mdl.async_chat(rendered_prompt, [{"role": "user", "content": "Output: "}])
    ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
    return ans if ans.find("**ERROR**") < 0 else messages[-1]["content"]


async def cross_languages(tenant_id, llm_id, query, languages=[]):
    from common.constants import LLMType
    from api.db.services.llm_service import LLMBundle
    from api.db.services.tenant_llm_service import TenantLLMService
    from api.db.joint_services.tenant_model_service import get_model_config_by_type_and_name, get_tenant_default_model_by_type

    if llm_id and TenantLLMService.llm_id2llm_type(llm_id) == "image2text":
        chat_model_config = get_model_config_by_type_and_name(tenant_id, LLMType.IMAGE2TEXT, llm_id)
    else:
        if not llm_id:
            chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
        else:
            chat_model_config = get_model_config_by_type_and_name(tenant_id, LLMType.CHAT, llm_id)
    chat_mdl = LLMBundle(tenant_id, chat_model_config)
    rendered_sys_prompt = PROMPT_JINJA_ENV.from_string(CROSS_LANGUAGES_SYS_PROMPT_TEMPLATE).render()
    rendered_user_prompt = PROMPT_JINJA_ENV.from_string(CROSS_LANGUAGES_USER_PROMPT_TEMPLATE).render(query=query,
                                                                                                     languages=languages)

    ans = await chat_mdl.async_chat(rendered_sys_prompt, [{"role": "user", "content": rendered_user_prompt}],
                                    {"temperature": 0.2})
    ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
    if ans.find("**ERROR**") >= 0:
        return query
    return "\n".join([a for a in re.sub(r"(^Output:|\n+)", "", ans, flags=re.DOTALL).split("===") if a.strip()])


async def content_tagging(chat_mdl, content, all_tags, examples, topn=3):
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
    kwd = await chat_mdl.async_chat(rendered_prompt, msg[1:], {"temperature": 0.5})
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


def vision_llm_figure_describe_prompt_with_context(context_above: str, context_below: str) -> str:
    template = PROMPT_JINJA_ENV.from_string(VISION_LLM_FIGURE_DESCRIBE_PROMPT_WITH_CONTEXT)
    return template.render(context_above=context_above, context_below=context_below)


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
                    "properties": {
                        "answer": {"type": "string", "description": "The final answer to the user's question"}},
                    "required": ["answer"]
                }
            }
        }
    for idx, tool in enumerate(tools_description):
        name = tool["function"]["name"]
        desc[name] = tool

    return "\n\n".join([f"## {i + 1}. {fnm}\n{json.dumps(des, ensure_ascii=False, indent=4)}" for i, (fnm, des) in
                        enumerate(desc.items())])


def form_history(history, limit=-6):
    context = ""
    for h in history[limit:]:
        if h["role"] == "system":
            continue
        role = "USER"
        if h["role"].upper() != role:
            role = "AGENT"
        context += f"\n{role}: {h['content'][:2048] + ('...' if len(h['content']) > 2048 else '')}"
    return context


async def analyze_task_async(chat_mdl, prompt, task_name, tools_description: list[dict],
                             user_defined_prompts: dict = {}):
    tools_desc = tool_schema(tools_description)
    context = ""

    if user_defined_prompts.get("task_analysis"):
        template = PROMPT_JINJA_ENV.from_string(user_defined_prompts["task_analysis"])
    else:
        template = PROMPT_JINJA_ENV.from_string(ANALYZE_TASK_SYSTEM + "\n\n" + ANALYZE_TASK_USER)
    context = template.render(task=task_name, context=context, agent_prompt=prompt, tools_desc=tools_desc)
    kwd = await chat_mdl.async_chat(context, [{"role": "user", "content": "Please analyze it."}])
    if isinstance(kwd, tuple):
        kwd = kwd[0]
    kwd = re.sub(r"^.*</think>", "", kwd, flags=re.DOTALL)
    if kwd.find("**ERROR**") >= 0:
        return ""
    return kwd


async def next_step_async(chat_mdl, history: list, tools_description: list[dict], task_desc,
                          user_defined_prompts: dict = {}):
    if not tools_description:
        return "", 0
    desc = tool_schema(tools_description)
    template = PROMPT_JINJA_ENV.from_string(user_defined_prompts.get("plan_generation", NEXT_STEP))
    user_prompt = "\nWhat's the next tool to call? If ready OR IMPOSSIBLE TO BE READY, then call `complete_task`."
    hist = deepcopy(history)
    if hist[-1]["role"] == "user":
        hist[-1]["content"] += user_prompt
    else:
        hist.append({"role": "user", "content": user_prompt})
    json_str = await chat_mdl.async_chat(
        template.render(task_analysis=task_desc, desc=desc, today=datetime.datetime.now().strftime("%Y-%m-%d")),
        hist[1:],
        stop=["<|stop|>"],
    )
    tk_cnt = num_tokens_from_string(json_str)
    json_str = re.sub(r"^.*</think>", "", json_str, flags=re.DOTALL)
    return json_str, tk_cnt


async def reflect_async(chat_mdl, history: list[dict], tool_call_res: list[Tuple], user_defined_prompts: dict = {}):
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
    ans = await chat_mdl.async_chat(msg[0]["content"], msg[1:])
    ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
    return """
**Observation**
{}

**Reflection**
{}
    """.format(json.dumps(tool_calls, ensure_ascii=False, indent=2), ans)


def form_message(system_prompt, user_prompt):
    return [{"role": "system", "content": system_prompt}, {"role": "user", "content": user_prompt}]


def structured_output_prompt(schema=None) -> str:
    template = PROMPT_JINJA_ENV.from_string(STRUCTURED_OUTPUT_PROMPT)
    return template.render(schema=schema)


async def tool_call_summary(chat_mdl, name: str, params: dict, result: str, user_defined_prompts: dict = {}) -> str:
    template = PROMPT_JINJA_ENV.from_string(SUMMARY4MEMORY)
    system_prompt = template.render(name=name,
                                    params=json.dumps(params, ensure_ascii=False, indent=2),
                                    result=result)
    user_prompt = "→ Summary: "
    _, msg = message_fit_in(form_message(system_prompt, user_prompt), chat_mdl.max_length)
    ans = await chat_mdl.async_chat(msg[0]["content"], msg[1:])
    return re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)


async def rank_memories_async(chat_mdl, goal: str, sub_goal: str, tool_call_summaries: list[str],
                              user_defined_prompts: dict = {}):
    template = PROMPT_JINJA_ENV.from_string(RANK_MEMORY)
    system_prompt = template.render(goal=goal, sub_goal=sub_goal,
                                    results=[{"i": i, "content": s} for i, s in enumerate(tool_call_summaries)])
    user_prompt = " → rank: "
    _, msg = message_fit_in(form_message(system_prompt, user_prompt), chat_mdl.max_length)
    ans = await chat_mdl.async_chat(msg[0]["content"], msg[1:], stop="<|stop|>")
    return re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)


async def gen_meta_filter(chat_mdl, meta_data: dict, query: str, constraints: dict = None) -> dict:
    """Generate metadata filter conditions from a user query using an LLM.

    Args:
        chat_mdl: LLM bundle for generating filters
        meta_data: Dict of {key: set of values} - e.g. {"character": {"Caocao", "Liubei"}, "year": {2026}}
        query: User question (e.g. "Caocao in 2026")
        constraints: Optional dict of {key: operator} to constrain which op to use for a key

    Returns:
        Dict with "logic" ("and"/"or") and "conditions" list.
        Example return value:
            {
                "logic": "and",
                "conditions": [
                    {"key": "year", "value": "2026", "op": "="},
                    {"key": "character", "value": "Caocao", "op": "="}
                ]
            }

    The LLM is prompted with the available metadata keys and values, and is asked to
    generate filter conditions that match the user's query semantics.
    """
    meta_data_structure = {}
    for key, values in meta_data.items():
        meta_data_structure[key] = list(values.keys()) if isinstance(values, dict) else values

    sys_prompt = PROMPT_JINJA_ENV.from_string(META_FILTER).render(
        current_date=datetime.datetime.today().strftime('%Y-%m-%d'),
        metadata_keys=json.dumps(meta_data_structure),
        user_question=query,
        constraints=json.dumps(constraints) if constraints else None
    )
    user_prompt = "Generate filters:"
    ans = await chat_mdl.async_chat(sys_prompt, [{"role": "user", "content": user_prompt}])
    ans = re.sub(r"(^.*</think>|```json\n|```\n*$)", "", ans, flags=re.DOTALL)
    try:
        ans = json_repair.loads(ans)
        assert isinstance(ans, dict), ans
        assert "conditions" in ans and isinstance(ans["conditions"], list), ans
        return ans
    except Exception:
        logging.exception(f"Loading json failure: {ans}")

    return {"conditions": []}


async def gen_json(system_prompt: str, user_prompt: str, chat_mdl, gen_conf={}, max_retry=2):
    from rag.graphrag.utils import get_llm_cache, set_llm_cache
    cached = get_llm_cache(chat_mdl.llm_name, system_prompt, user_prompt, gen_conf)
    if cached:
        return json_repair.loads(cached)
    _, msg = message_fit_in(form_message(system_prompt, user_prompt), chat_mdl.max_length)
    err = ""
    ans = ""
    for _ in range(max_retry):
        if ans and err:
            msg[-1]["content"] += f"\nGenerated JSON is as following:\n{ans}\nBut exception while loading:\n{err}\nPlease reconsider and correct it."
        ans = await chat_mdl.async_chat(msg[0]["content"], msg[1:], gen_conf=gen_conf)
        ans = re.sub(r"(^.*</think>|```json\n|```\n*$)", "", ans, flags=re.DOTALL)
        try:
            res = json_repair.loads(ans)
            set_llm_cache(chat_mdl.llm_name, system_prompt, ans, user_prompt, gen_conf)
            return res
        except Exception as e:
            logging.exception(f"Loading json failure: {ans}")
            err += str(e)


TOC_DETECTION = load_prompt("toc_detection")


async def detect_table_of_contents(page_1024: list[str], chat_mdl):
    toc_secs = []
    for i, sec in enumerate(page_1024[:22]):
        ans = await gen_json(PROMPT_JINJA_ENV.from_string(TOC_DETECTION).render(page_txt=sec), "Only JSON please.",
                             chat_mdl)
        if toc_secs and not ans["exists"]:
            break
        toc_secs.append(sec)
    return toc_secs


TOC_EXTRACTION = load_prompt("toc_extraction")
TOC_EXTRACTION_CONTINUE = load_prompt("toc_extraction_continue")


async def extract_table_of_contents(toc_pages, chat_mdl):
    if not toc_pages:
        return []

    return await gen_json(PROMPT_JINJA_ENV.from_string(TOC_EXTRACTION).render(toc_page="\n".join(toc_pages)),
                          "Only JSON please.", chat_mdl)


async def toc_index_extractor(toc: list[dict], content: str, chat_mdl):
    tob_extractor_prompt = """
    You are given a table of contents in a json format and several pages of a document, your job is to add the physical_index to the table of contents in the json format.

    The provided pages contains tags like <physical_index_X> and <physical_index_X> to indicate the physical location of the page X.

    The structure variable is the numeric system which represents the index of the hierarchy section in the table of contents. For example, the first section has structure index 1, the first subsection has structure index 1.1, the second subsection has structure index 1.2, etc.

    The response should be in the following JSON format:
    [
        {
            "structure": <structure index, "x.x.x" or None> (string),
            "title": <title of the section>,
            "physical_index": "<physical_index_X>" (keep the format)
        },
        ...
    ]

    Only add the physical_index to the sections that are in the provided pages.
    If the title of the section are not in the provided pages, do not add the physical_index to it.
    Directly return the final JSON structure. Do not output anything else."""

    prompt = tob_extractor_prompt + '\nTable of contents:\n' + json.dumps(toc, ensure_ascii=False,
                                                                          indent=2) + '\nDocument pages:\n' + content
    return await gen_json(prompt, "Only JSON please.", chat_mdl)


TOC_INDEX = load_prompt("toc_index")


async def table_of_contents_index(toc_arr: list[dict], sections: list[str], chat_mdl):
    if not toc_arr or not sections:
        return []

    toc_map = {}
    for i, it in enumerate(toc_arr):
        k1 = (it["structure"] + it["title"]).replace(" ", "")
        k2 = it["title"].strip()
        if k1 not in toc_map:
            toc_map[k1] = []
        if k2 not in toc_map:
            toc_map[k2] = []
        toc_map[k1].append(i)
        toc_map[k2].append(i)

    for it in toc_arr:
        it["indices"] = []
    for i, sec in enumerate(sections):
        sec = sec.strip()
        if sec.replace(" ", "") in toc_map:
            for j in toc_map[sec.replace(" ", "")]:
                toc_arr[j]["indices"].append(i)

    all_pathes = []

    def dfs(start, path):
        nonlocal all_pathes
        if start >= len(toc_arr):
            if path:
                all_pathes.append(path)
            return
        if not toc_arr[start]["indices"]:
            dfs(start + 1, path)
            return
        added = False
        for j in toc_arr[start]["indices"]:
            if path and j < path[-1][0]:
                continue
            _path = deepcopy(path)
            _path.append((j, start))
            added = True
            dfs(start + 1, _path)
        if not added and path:
            all_pathes.append(path)

    dfs(0, [])
    path = max(all_pathes, key=lambda x: len(x))
    for it in toc_arr:
        it["indices"] = []
    for j, i in path:
        toc_arr[i]["indices"] = [j]
    print(json.dumps(toc_arr, ensure_ascii=False, indent=2))

    i = 0
    while i < len(toc_arr):
        it = toc_arr[i]
        if it["indices"]:
            i += 1
            continue

        if i > 0 and toc_arr[i - 1]["indices"]:
            st_i = toc_arr[i - 1]["indices"][-1]
        else:
            st_i = 0
        e = i + 1
        while e < len(toc_arr) and not toc_arr[e]["indices"]:
            e += 1
        if e >= len(toc_arr):
            e = len(sections)
        else:
            e = toc_arr[e]["indices"][0]

        for j in range(st_i, min(e + 1, len(sections))):
            ans = await gen_json(PROMPT_JINJA_ENV.from_string(TOC_INDEX).render(
                structure=it["structure"],
                title=it["title"],
                text=sections[j]), "Only JSON please.", chat_mdl)
            if ans["exist"] == "yes":
                it["indices"].append(j)
                break

        i += 1

    return toc_arr


async def check_if_toc_transformation_is_complete(content, toc, chat_mdl):
    prompt = """
    You are given a raw table of contents and a  table of contents.
    Your job is to check if the  table of contents is complete.

    Reply format:
    {{
        "thinking": <why do you think the cleaned table of contents is complete or not>
        "completed": "yes" or "no"
    }}
    Directly return the final JSON structure. Do not output anything else."""

    prompt = prompt + '\n Raw Table of contents:\n' + content + '\n Cleaned Table of contents:\n' + toc
    response = await gen_json(prompt, "Only JSON please.", chat_mdl)
    return response['completed']


async def toc_transformer(toc_pages, chat_mdl):
    init_prompt = """
    You are given a table of contents, You job is to transform the whole table of content into a JSON format included table_of_contents.

    The `structure` is the numeric system which represents the index of the hierarchy section in the table of contents. For example, the first section has structure index 1, the first subsection has structure index 1.1, the second subsection has structure index 1.2, etc.
    The `title` is a short phrase or a several-words term.

    The response should be in the following JSON format:
    [
        {
            "structure": <structure index, "x.x.x" or None> (string),
            "title": <title of the section>
        },
        ...
    ],
    You should transform the full table of contents in one go.
    Directly return the final JSON structure, do not output anything else. """

    toc_content = "\n".join(toc_pages)
    prompt = init_prompt + '\n Given table of contents\n:' + toc_content

    def clean_toc(arr):
        for a in arr:
            a["title"] = re.sub(r"[.·….]{2,}", "", a["title"])

    last_complete = await gen_json(prompt, "Only JSON please.", chat_mdl)
    if_complete = await check_if_toc_transformation_is_complete(toc_content,
                                                                json.dumps(last_complete, ensure_ascii=False, indent=2),
                                                                chat_mdl)
    clean_toc(last_complete)
    if if_complete == "yes":
        return last_complete

    while not (if_complete == "yes"):
        prompt = f"""
        Your task is to continue the table of contents json structure, directly output the remaining part of the json structure.
        The response should be in the following JSON format:

        The raw table of contents json structure is:
        {toc_content}

        The incomplete transformed table of contents json structure is:
        {json.dumps(last_complete[-24:], ensure_ascii=False, indent=2)}

        Please continue the json structure, directly output the remaining part of the json structure."""
        new_complete = await gen_json(prompt, "Only JSON please.", chat_mdl)
        if not new_complete or str(last_complete).find(str(new_complete)) >= 0:
            break
        clean_toc(new_complete)
        last_complete.extend(new_complete)
        if_complete = await check_if_toc_transformation_is_complete(toc_content,
                                                                    json.dumps(last_complete, ensure_ascii=False,
                                                                               indent=2), chat_mdl)

    return last_complete


TOC_LEVELS = load_prompt("assign_toc_levels")


async def assign_toc_levels(toc_secs, chat_mdl, gen_conf={"temperature": 0.2}):
    if not toc_secs:
        return []
    return await gen_json(
        PROMPT_JINJA_ENV.from_string(TOC_LEVELS).render(),
        str(toc_secs),
        chat_mdl,
        gen_conf
    )


TOC_FROM_TEXT_SYSTEM = load_prompt("toc_from_text_system")
TOC_FROM_TEXT_USER = load_prompt("toc_from_text_user")


# Generate TOC from text chunks with text llms
async def gen_toc_from_text(txt_info: dict, chat_mdl, callback=None):
    if callback:
        callback(msg="")
    try:
        ans = await gen_json(
            PROMPT_JINJA_ENV.from_string(TOC_FROM_TEXT_SYSTEM).render(),
            PROMPT_JINJA_ENV.from_string(TOC_FROM_TEXT_USER).render(
                text="\n".join([json.dumps(d, ensure_ascii=False) for d in txt_info["chunks"]])),
            chat_mdl,
            gen_conf={"temperature": 0.0, "top_p": 0.9}
        )
        txt_info["toc"] = ans if ans and not isinstance(ans, str) else []
    except Exception as e:
        logging.exception(e)


def split_chunks(chunks, max_length: int):
    """
    Pack chunks into batches according to max_length, returning [{"id": idx, "text": chunk_text}, ...].
    Do not split a single chunk, even if it exceeds max_length.
    """

    result = []
    batch, batch_tokens = [], 0

    for idx, chunk in enumerate(chunks):
        t = num_tokens_from_string(chunk)
        if batch_tokens + t > max_length:
            result.append(batch)
            batch, batch_tokens = [], 0
        batch.append({idx: chunk})
        batch_tokens += t
    if batch:
        result.append(batch)
    return result


async def run_toc_from_text(chunks, chat_mdl, callback=None):
    input_budget = int(chat_mdl.max_length * INPUT_UTILIZATION) - num_tokens_from_string(
        TOC_FROM_TEXT_USER + TOC_FROM_TEXT_SYSTEM
    )

    input_budget = 1024 if input_budget > 1024 else input_budget
    chunk_sections = split_chunks(chunks, input_budget)
    titles = []

    chunks_res = []
    tasks = []
    for i, chunk in enumerate(chunk_sections):
        if not chunk:
            continue
        chunks_res.append({"chunks": chunk})
        tasks.append(asyncio.create_task(gen_toc_from_text(chunks_res[-1], chat_mdl, callback)))
    try:
        await asyncio.gather(*tasks, return_exceptions=False)
    except Exception as e:
        logging.error(f"Error generating TOC: {e}")
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise

    for chunk in chunks_res:
        titles.extend(chunk.get("toc", []))

    # Filter out entries with title == -1
    prune = len(titles) > 512
    max_len = 12 if prune else 22
    filtered = []
    for x in titles:
        if not isinstance(x, dict) or not x.get("title") or x["title"] == "-1":
            continue
        if len(rag_tokenizer.tokenize(x["title"]).split(" ")) > max_len:
            continue
        if re.match(r"[0-9,.()/ -]+$", x["title"]):
            continue
        filtered.append(x)

    logging.info(f"\n\nFiltered TOC sections:\n{filtered}")
    if not filtered:
        return []

    # Generate initial level (level/title)
    raw_structure = [x.get("title", "") for x in filtered]

    # Assign hierarchy levels using LLM
    toc_with_levels = await assign_toc_levels(raw_structure, chat_mdl, {"temperature": 0.0, "top_p": 0.9})
    if not toc_with_levels:
        return []

    # Merge structure and content (by index)
    prune = len(toc_with_levels) > 512
    max_lvl = "0"
    sorted_list = sorted([t.get("level", "0") for t in toc_with_levels if isinstance(t, dict)])
    if sorted_list:
        max_lvl = sorted_list[-1]
    merged = []
    for _, (toc_item, src_item) in enumerate(zip(toc_with_levels, filtered)):
        if prune and toc_item.get("level", "0") >= max_lvl:
            continue
        merged.append({
            "level": toc_item.get("level", "0"),
            "title": toc_item.get("title", ""),
            "chunk_id": src_item.get("chunk_id", ""),
        })

    return merged


TOC_RELEVANCE_SYSTEM = load_prompt("toc_relevance_system")
TOC_RELEVANCE_USER = load_prompt("toc_relevance_user")
async def relevant_chunks_with_toc(query: str, toc: list[dict], chat_mdl, topn: int = 6):
    import numpy as np
    try:
        ans = await gen_json(
            PROMPT_JINJA_ENV.from_string(TOC_RELEVANCE_SYSTEM).render(),
            PROMPT_JINJA_ENV.from_string(TOC_RELEVANCE_USER).render(query=query, toc_json="[\n%s\n]\n" % "\n".join(
                [json.dumps({"level": d["level"], "title": d["title"]}, ensure_ascii=False) for d in toc])),
            chat_mdl,
            gen_conf={"temperature": 0.0, "top_p": 0.9}
        )
        id2score = {}
        for ti, sc in zip(toc, ans):
            if not isinstance(sc, dict) or sc.get("score", -1) < 1:
                continue
            for id in ti.get("ids", []):
                if id not in id2score:
                    id2score[id] = []
                id2score[id].append(sc["score"] / 5.)
        for id in id2score.keys():
            id2score[id] = np.mean(id2score[id])
        return [(id, sc) for id, sc in list(id2score.items()) if sc >= 0.3][:topn]
    except Exception as e:
        logging.exception(e)
    return []


META_DATA = load_prompt("meta_data")
async def gen_metadata(chat_mdl, schema: dict, content: str):
    template = PROMPT_JINJA_ENV.from_string(META_DATA)
    for k, desc in schema["properties"].items():
        if "enum" in desc and not desc.get("enum"):
            del desc["enum"]
        if desc.get("enum"):
            desc["description"] += "\n** Extracted values must strictly match the given list specified by `enum`. **"
    system_prompt = template.render(content=content, schema=schema)
    user_prompt = "Output: "
    _, msg = message_fit_in(form_message(system_prompt, user_prompt), chat_mdl.max_length)
    ans = await chat_mdl.async_chat(msg[0]["content"], msg[1:])
    return re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)


SUFFICIENCY_CHECK = load_prompt("sufficiency_check")
async def sufficiency_check(chat_mdl, question: str, ret_content: str):
    try:
        return await gen_json(
            PROMPT_JINJA_ENV.from_string(SUFFICIENCY_CHECK).render(question=question, retrieved_docs=ret_content),
            "Output:\n",
            chat_mdl
        )
    except Exception as e:
        logging.exception(e)
    return {}


MULTI_QUERIES_GEN = load_prompt("multi_queries_gen")
async def multi_queries_gen(chat_mdl, question: str, query:str, missing_infos:list[str], ret_content: str):
    try:
        return await gen_json(
            PROMPT_JINJA_ENV.from_string(MULTI_QUERIES_GEN).render(
                original_question=question,
                original_query=query,
                missing_info="\n - ".join(missing_infos),
                retrieved_docs=ret_content
            ),
            "Output:\n",
            chat_mdl
        )
    except Exception as e:
        logging.exception(e)
    return {}


# ---------------------------------------------------------------------------
# Structured-knowledge extraction: list / set / hypergraph
# ---------------------------------------------------------------------------
#
# The configuration shape mirrors the YAML templates used by Hyper-Extract
# (see D:/git/Hyper-Extract/hyperextract/templates/presets/general/*.yaml and
# hyperextract/utils/template_engine/parsers/guideline.py). It is delivered to
# us through ``document.parser_config`` as a JSON string and contains:
#
#   type:        "list" | "set" | "hypergraph"      (optional, can be inferred)
#   output:                                          schema description
#     # for list/set:
#     description: <str | list | {lang: str|list}>
#     fields: [{name, type, description, required?}, ...]
#     # for hypergraph:
#     description: ...
#     entities: {description, fields: [...]}
#     relations: {description, fields: [...]}
#   guideline:
#     # for list/set:
#     target: <localizable>
#     rules:  <localizable>
#     # for hypergraph:
#     target: <localizable>
#     rules_for_entities: <localizable>
#     rules_for_relations: <localizable>
#     rules_for_time: <localizable, optional>
#   identifiers:                                     (optional)
#     entity_id: <field name>                       (used to enumerate known
#                                                    nodes in stage 2)
#   options:                                         (optional)
#     observation_time: <str>                       (substituted into rules)
#
# Merging across chunks is intentionally NOT implemented here.

_STRUCT_TYPES = ("list", "set", "hypergraph")


def _struct_localize(value, language: str = "en") -> str:
    """Render multilingual values to a single string (mirrors loader._localize_data)."""
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        return "\n".join(f"{i + 1}. {item}" for i, item in enumerate(value))
    if isinstance(value, dict):
        v = value.get(language)
        if v is None and language != "en":
            v = value.get("en")
        if isinstance(v, str):
            return v
        if isinstance(v, list):
            return "\n".join(f"{i + 1}. {item}" for i, item in enumerate(v))
    return ""


def _struct_get(cfg: dict, *keys, default=None):
    """Case-insensitive lookup against the first matching key."""
    if not isinstance(cfg, dict):
        return default
    for k in keys:
        if k in cfg:
            return cfg[k]
        kl = k.lower()
        for ck in cfg.keys():
            if isinstance(ck, str) and ck.lower() == kl:
                return cfg[ck]
    return default


def _struct_infer_type(parser_config: dict) -> str:
    explicit = _struct_get(parser_config, "compile_type")
    if isinstance(explicit, str) and explicit.lower() in _STRUCT_TYPES:
        return explicit.lower()
    output = _struct_get(parser_config, "output", default={}) or {}
    if _struct_get(output, "entities") and _struct_get(output, "relations"):
        return "hypergraph"
    return "list"


def _struct_render_fields(fields: list, language: str) -> Tuple[str, str]:
    """Return (bulleted field descriptions, JSON skeleton for one item)."""
    lines = []
    skeleton_parts = []
    for f in fields or []:
        name = f.get("name", "")
        ftype = f.get("type", "str")
        desc = _struct_localize(f.get("description", ""), language)
        required = f.get("required")
        req_label = "optional" if required is False else "required"
        lines.append(f"- {name} ({ftype}, {req_label}): {desc}")
        if ftype == "list":
            placeholder = "[<string>, ...]"
        elif ftype == "int":
            placeholder = "<int>"
        elif ftype == "float":
            placeholder = "<float>"
        elif ftype == "bool":
            placeholder = "<true|false>"
        else:
            placeholder = "<string>"
        skeleton_parts.append(f'"{name}": {placeholder}')
    return "\n".join(lines), "{ " + ", ".join(skeleton_parts) + " }"


def _struct_hypergraph_prompts(parser_config: dict, language: str="en") -> Tuple[str, str]:
    autotype = _struct_get(parser_config, "compile_type", default="graph")
    guideline = _struct_get(parser_config, "guideline", default={}) or {}
    output = _struct_get(parser_config, "output", default={}) or {}
    options = _struct_get(parser_config, "options", default={}) or {}

    target = _struct_localize(_struct_get(guideline, "target"), language)
    rules_e = _struct_localize(_struct_get(guideline, "rules_for_entities"), language)
    rules_r = _struct_localize(_struct_get(guideline, "rules_for_relations"), language)
    rules_t = _struct_localize(_struct_get(guideline, "rules_for_time"), language)

    observation_time = _struct_get(options, "observation_time") or datetime.date.today().isoformat()
    if rules_t and "{observation_time}" in rules_t:
        rules_t = rules_t.replace("{observation_time}", observation_time)

    entities_cfg = _struct_get(output, "entities", default={}) or {}
    relations_cfg = _struct_get(output, "relations", default={}) or {}
    ent_desc = _struct_localize(_struct_get(entities_cfg, "description"), language)
    rel_desc = _struct_localize(_struct_get(relations_cfg, "description"), language)
    ent_fields_text, ent_skel = _struct_render_fields(_struct_get(entities_cfg, "fields", default=[]) or [], language)
    rel_fields_text, rel_skel = _struct_render_fields(_struct_get(relations_cfg, "fields", default=[]) or [], language)

    node_parts = [f"# Role and Task:\n{target}"] if target else []
    if rules_e:
        node_parts.append(f"## Entity Extraction Rules:\n{rules_e}")
    if ent_desc:
        node_parts.append(f"## Entity Description:\n{ent_desc}")
    node_parts.append(f"## Entity Fields:\n{ent_fields_text}")
    node_parts.append(
        "## Response Format:\n"
        "Reply with a single JSON object of the form: "
        f'{{"items": [{ent_skel}, ...]}}.\n'
        f"Auto-type: \"{autotype}\". "
        + ("Items must be unique. " if autotype == "set" else "")
        + "Return JSON only, no commentary."
    )
    node_prompt = "\n\n".join(node_parts)

    if not relations_cfg:
        return node_prompt, ""

    edge_parts = [f"# Role and Task:\n{target}"] if target else []
    if rules_r:
        edge_parts.append(f"## Relation Extraction Rules:\n{rules_r}")
    if rules_t:
        edge_parts.append(f"## Time Rules:\n{rules_t}")
    if rel_desc:
        edge_parts.append(f"## Relation Description:\n{rel_desc}")
    edge_parts.append(f"## Relation Fields:\n{rel_fields_text}")
    edge_parts.append("## Known Entities:\n{known_nodes}")
    edge_parts.append(
        "## Response Format:\n"
        "Reply with a single JSON object of the form: "
        f'{{"items": [{rel_skel}, ...]}}.\n'
        "Only create relations between entities listed in 'Known Entities'. "
        "Return JSON only, no commentary."
    )
    edge_prompt = "\n\n".join(edge_parts)

    return node_prompt, edge_prompt


def _struct_entity_id_field(parser_config: dict) -> str:
    identifiers = _struct_get(parser_config, "identifiers", default={}) or {}
    entity_id = _struct_get(identifiers, "entity_id")
    if isinstance(entity_id, str) and "{" not in entity_id and entity_id.strip():
        return entity_id.strip()
    entities_cfg = _struct_get(_struct_get(parser_config, "output", default={}) or {}, "entities", default={}) or {}
    for f in _struct_get(entities_cfg, "fields", default=[]) or []:
        if f.get("required") is not False:
            return f.get("name", "name")
    return "name"


def _struct_unwrap_items(res) -> list:
    if res is None:
        return []
    if isinstance(res, dict):
        items = res.get("items")
        if isinstance(items, list):
            return [it for it in items if isinstance(it, dict)]
        return []
    if isinstance(res, list):
        return [it for it in res if isinstance(it, dict)]
    return []


async def _struct_extract_hypergraph(text: str, parser_config: dict, chat_mdl, language: str) -> Tuple[list[dict], list[dict]]:
    node_prompt, edge_prompt_template = _struct_hypergraph_prompts(parser_config, language)

    user_prompt = f"## Source Text:\n{text}\n\n## Output (JSON only):"
    node_res = await gen_json(node_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.1})
    nodes = _struct_unwrap_items(node_res)

    id_field = _struct_entity_id_field(parser_config)
    known_keys = []
    for n in nodes:
        v = n.get(id_field)
        if v is None:
            continue
        v_str = str(v).strip()
        if v_str and v_str not in known_keys:
            known_keys.append(v_str)
    known_str = "- " + "\n- ".join(known_keys) if known_keys else "(none)"

    edge_prompt = edge_prompt_template.replace("{known_nodes}", known_str)
    edge_res = await gen_json(edge_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.1})
    edges = _struct_unwrap_items(edge_res)

    return nodes, edges


async def _struct_embed(payloads: list[str], embd_mdl) -> list:
    if not payloads:
        return []
    embeddings, _ = await thread_pool_exec(embd_mdl.encode, payloads)
    return list(embeddings)


def _struct_payload_description(payload: dict) -> str:
    """Concat string values of every non-description field (lists flattened)."""
    parts: list[str] = []
    for k, v in payload.items():
        if isinstance(v, (list, tuple)):
            for item in v:
                if item is None:
                    continue
                s = str(item).strip()
                if s:
                    parts.append(s)
        else:
            s = str(v).strip()
            if s:
                parts.append(s)
    return " ".join(parts)


def _struct_relation_member_fields(parser_config: dict) -> Tuple:
    """Return (source_field, target_field) for relation docs, or (None, None).

    Looks at ``identifiers.relation_members`` first (dict form for graph-style
    configs, e.g. ``{source: source, target: target}``); falls back to the
    conventional ``source`` / ``target`` field names if both appear in the
    relation schema.
    """
    identifiers = _struct_get(parser_config, "identifiers", default={}) or {}
    members = _struct_get(identifiers, "relation_members")
    if isinstance(members, dict):
        src = members.get("source") or members.get("src")
        tgt = members.get("target") or members.get("tgt")
        if src or tgt:
            return src, tgt

    relations_cfg = _struct_get(
        _struct_get(parser_config, "output", default={}) or {},
        "relations",
        default={},
    ) or {}
    field_names = {
        f.get("name")
        for f in (_struct_get(relations_cfg, "fields", default=[]) or [])
        if isinstance(f, dict)
    }
    if "source" in field_names and "target" in field_names:
        return "source", "target"
    return None, None


def _struct_to_es_doc(
    payload: dict,
    compile_kwd: str,
    doc_id: str,
    chunk_ids: list[str],
    vec,
    kind: str,
    src_field: str | None = None,
    target_field: str | None = None,
) -> dict:
    """Build one ES doc for an extracted entity or relation.

    Args:
        kind: ``"entity"`` or ``"relation"`` — written to ``structure_kwd``.
        src_field / target_field: when ``kind == "relation"`` and these field
            names exist on the payload, the resolved values are written to
            ``src_name_kwd`` / ``target_name_kwd``.
    """
    content_with_weight = json.dumps(payload, ensure_ascii=False)
    if hasattr(vec, "tolist"):
        vec_list = vec.tolist()
    else:
        vec_list = list(vec)
    doc_id_str = str(doc_id)

    description = _struct_payload_description(payload)

    content_ltks = rag_tokenizer.tokenize(description) if description else ""
    content_sm_ltks = rag_tokenizer.fine_grained_tokenize(content_ltks) if content_ltks else ""

    doc = {
        "content_with_weight": content_with_weight,
        "compile_kwd": compile_kwd,
        "structure_kwd": kind,
        "doc_id": doc_id_str,
        "chunk_ids": list(chunk_ids or []),
        "content_ltks": content_ltks,
        "content_sm_ltks": content_sm_ltks,
        f"q_{len(vec_list)}_vec": vec_list,
        "id": xxhash.xxh64(
            (content_with_weight + doc_id_str).encode("utf-8", "surrogatepass")
        ).hexdigest(),
    }

    if kind == "relation":
        if src_field:
            src_val = payload.get(src_field)
            if src_val is not None and str(src_val).strip():
                doc["src_name_kwd"] = str(src_val).strip()
        if target_field:
            tgt_val = payload.get(target_field)
            if tgt_val is not None and str(tgt_val).strip():
                doc["target_name_kwd"] = str(tgt_val).strip()

    return doc


async def _struct_process_batch(
    batch: list,
    batch_idx: int,
    total: int,
    chunk_ids: list,
    autotype: str,
    parser_config: dict,
    chat_mdl,
    embd_mdl,
    doc_id: str,
    language: str,
    callback,
    semaphore,
) -> list[dict]:
    """Process one packed batch end-to-end (extract → embed → ES docs).

    The semaphore (if any) is taken around the entire batch's LLM + embedding
    work to bound peak concurrency.
    """
    if not batch:
        return []

    batch_ids: list = []
    batch_segments: list[str] = []
    for item in batch:
        for idx, text in item.items():
            cid = chunk_ids[idx]
            if cid:
                batch_ids.append(cid)
            batch_segments.append(text)
    combined_text = "\n\n---\n\n".join(batch_segments)

    src_field, target_field = _struct_relation_member_fields(parser_config)

    async def _run() -> list[dict]:
        # For hypergraph, entity extraction MUST complete before edge extraction
        # within the same batch, because the edge prompt's {known_nodes}
        # placeholder is filled from this batch's extracted nodes — see
        # _struct_extract_hypergraph. Parallelism across batches is fine; the
        # two stages within one batch are strictly sequential.
        try:
            items, relations = await _struct_extract_hypergraph(combined_text, parser_config, chat_mdl, language)
        except Exception as e:
            logging.exception(f"compile_structure_from_text: extraction failed for batch {batch_idx}: {e}")
            return []

        payloads = items + relations
        kinds = ["entity"] * len(items) + ["relation"] * len(relations)
        if not payloads:
            if callback:
                callback((batch_idx + 1) / total, f"{batch_idx + 1}/{total} batches: 0 items")
            return []

        embed_inputs = [_struct_payload_description(p) for p in payloads]
        try:
            embeddings = await _struct_embed(embed_inputs, embd_mdl)
        except Exception as e:
            logging.exception(f"compile_structure_from_text: embedding failed for batch {batch_idx}: {e}")
            return []

        if len(embeddings) != len(payloads):
            logging.error(
                f"compile_structure_from_text: embedding count mismatch ({len(embeddings)} vs {len(payloads)}) for batch {batch_idx}"
            )
            return []

        docs = [
            _struct_to_es_doc(
                payload, autotype, doc_id, batch_ids, vec, kind,
                src_field=src_field, target_field=target_field,
            )
            for payload, vec, kind in zip(payloads, embeddings, kinds)
        ]

        if callback:
            callback((batch_idx + 1) / total, f"{batch_idx + 1}/{total} batches: {len(payloads)} items")

        return docs

    if semaphore is not None:
        async with semaphore:
            return await _run()
    return await _run()


async def compile_structure_from_text(
    chunks: list[dict],
    parser_config,
    chat_mdl,
    embd_mdl,
    doc_id: str,
    language: str = "en",
    callback=None,
    max_workers: int = 10,
) -> list[dict]:
    """Extract list/set/hypergraph structures from text chunks and prepare ES docs.

    Each chunk is processed independently — cross-chunk merging of entities and
    relations is deferred to a separate pipeline stage and is intentionally not
    performed here.

    Args:
        chunks: list of dicts; each must expose ``id`` and ``text`` (a
            ``content_with_weight`` fallback is also accepted).
        parser_config: dict already parsed from ``document.parser_config`` or
            the raw JSON string from the database.
        chat_mdl: LLMBundle for chat (used via ``gen_json``).
        embd_mdl: LLMBundle for embeddings (used via ``encode``).
        doc_id: source document id, embedded into every ES doc.
        language: language code for resolving multilingual config strings.
        callback: optional progress callback ``(prog: float, msg: str)``.

    Returns:
        List of ES-ready dicts shaped as::

            {
                "content_with_weight": <json>,
                "compile_kwd": "list" | "set" | "hypergraph",
                "doc_id": <doc_id>,
                "chunk_ids": [<chunk_id>, ...],
                "q_<dim>_vec": [...],
                "id": <xxhash>,
            }
    """
    if isinstance(parser_config, str):
        try:
            parser_config = json.loads(parser_config)
        except Exception as e:
            logging.exception(f"compile_structure_from_text: invalid parser_config JSON: {e}")
            return []
    if not isinstance(parser_config, dict):
        logging.error("compile_structure_from_text: parser_config must be a dict or JSON string")
        return []

    autotype = _struct_infer_type(parser_config)
    if autotype not in _STRUCT_TYPES:
        logging.error(f"compile_structure_from_text: unsupported type '{autotype}'")
        return []

    chunk_ids: list = []
    chunk_texts: list[str] = []
    for chunk in chunks:
        text = chunk.get("text") or chunk.get("content_with_weight") or chunk.get("content") or ""
        if not isinstance(text, str) or not text.strip():
            continue
        chunk_ids.append(chunk.get("id") or chunk.get("chunk_id"))
        chunk_texts.append(text)

    if not chunk_texts:
        return []

    node_prompt, edge_prompt = _struct_hypergraph_prompts(parser_config, language)
    prompt_overhead = max(num_tokens_from_string(node_prompt), num_tokens_from_string(edge_prompt))

    input_budget = int(chat_mdl.max_length * INPUT_UTILIZATION) - prompt_overhead
    if input_budget < 1024:
        input_budget = 1024

    batches = split_chunks(chunk_texts, input_budget)
    total = max(1, len(batches))
    semaphore = asyncio.Semaphore(max_workers) if max_workers and max_workers > 0 else None

    tasks = [
        asyncio.create_task(
            _struct_process_batch(
                batch, bi, total, chunk_ids, autotype,
                parser_config, chat_mdl, embd_mdl, doc_id,
                language, callback, semaphore,
            )
        )
        for bi, batch in enumerate(batches)
        if batch
    ]

    if not tasks:
        return []

    try:
        batch_results = await asyncio.gather(*tasks, return_exceptions=False)
    except Exception:
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise

    results: list[dict] = []
    for br in batch_results:
        if br:
            results.extend(br)
    return results
