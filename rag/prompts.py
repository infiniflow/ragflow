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

import jinja2
import json_repair

from api import settings
from rag.prompt_template import load_prompt
from rag.settings import TAG_FLD
from rag.utils import encoder, num_tokens_from_string


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


def kb_prompt(kbinfos, max_tokens):
    from api.db.services.document_service import DocumentService

    knowledges = [ck["content_with_weight"] for ck in kbinfos["chunks"]]
    kwlg_len = len(knowledges)
    used_token_count = 0
    chunks_num = 0
    for i, c in enumerate(knowledges):
        used_token_count += num_tokens_from_string(c)
        chunks_num += 1
        if max_tokens * 0.97 < used_token_count:
            knowledges = knowledges[:i]
            logging.warning(f"Not all the retrieval into prompt: {len(knowledges)}/{kwlg_len}")
            break

    docs = DocumentService.get_by_ids([ck["doc_id"] for ck in kbinfos["chunks"][:chunks_num]])
    docs = {d.id: d.meta_fields for d in docs}

    doc2chunks = defaultdict(lambda: {"chunks": [], "meta": []})
    for i, ck in enumerate(kbinfos["chunks"][:chunks_num]):
        cnt = f"---\nID: {i}\n" + (f"URL: {ck['url']}\n" if "url" in ck else "")
        cnt += re.sub(r"( style=\"[^\"]+\"|</?(html|body|head|title)>|<!DOCTYPE html>)", " ", ck["content_with_weight"], flags=re.DOTALL | re.IGNORECASE)
        doc2chunks[ck["docnm_kwd"]]["chunks"].append(cnt)
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


CITATION_PROMPT_TEMPLATE = load_prompt("citation_prompt")
CONTENT_TAGGING_PROMPT_TEMPLATE = load_prompt("content_tagging_prompt")
CROSS_LANGUAGES_SYS_PROMPT_TEMPLATE = load_prompt("cross_languages_sys_prompt")
CROSS_LANGUAGES_USER_PROMPT_TEMPLATE = load_prompt("cross_languages_user_prompt")
FULL_QUESTION_PROMPT_TEMPLATE = load_prompt("full_question_prompt")
KEYWORD_PROMPT_TEMPLATE = load_prompt("keyword_prompt")
QUESTION_PROMPT_TEMPLATE = load_prompt("question_prompt")
VISION_LLM_DESCRIBE_PROMPT = load_prompt("vision_llm_describe_prompt")
VISION_LLM_FIGURE_DESCRIBE_PROMPT = load_prompt("vision_llm_figure_describe_prompt")

PROMPT_JINJA_ENV = jinja2.Environment(autoescape=False, trim_blocks=True, lstrip_blocks=True)


def citation_prompt() -> str:
    template = PROMPT_JINJA_ENV.from_string(CITATION_PROMPT_TEMPLATE)
    return template.render()


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


def full_question(tenant_id, llm_id, messages, language=None, full_question_prompt=None):
    from api.db import LLMType
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
    conversation = "\n".join(conv)
    today = datetime.date.today().isoformat()
    yesterday = (datetime.date.today() - datetime.timedelta(days=1)).isoformat()
    tomorrow = (datetime.date.today() + datetime.timedelta(days=1)).isoformat()

    # Use custom prompt if provided, otherwise use default template
    prompt_template = full_question_prompt if full_question_prompt else FULL_QUESTION_PROMPT_TEMPLATE
    template = PROMPT_JINJA_ENV.from_string(prompt_template)
    print("Full question prompt template:", template)
    rendered_prompt = template.render(
        today=today,
        yesterday=yesterday,
        tomorrow=tomorrow,
        conversation=conversation,
        language=language,
    )
    print("Full question prompt:", rendered_prompt)

    ans = chat_mdl.chat(rendered_prompt, [{"role": "user", "content": "Output: "}], {"temperature": 0.2})
    print("Full question answer:", ans)
    ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
    return ans if ans.find("**ERROR**") < 0 else messages[-1]["content"]


def cross_languages(tenant_id, llm_id, query, languages=[]):
    from api.db import LLMType
    from api.db.services.llm_service import LLMBundle

    if llm_id and llm_id2llm_type(llm_id) == "image2text":
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


def vision_llm_figure_describe_prompt_vietnamese() -> str:
    prompt = """
Bạn là một chuyên gia phân tích dữ liệu trực quan. Hãy phân tích hình ảnh và cung cấp mô tả chi tiết về nội dung của nó. Tập trung vào việc xác định loại biểu diễn dữ liệu trực quan (ví dụ: biểu đồ cột, biểu đồ tròn, biểu đồ đường, bảng, sơ đồ luồng), cấu trúc của hình ảnh, và bất kỳ chú thích hoặc nhãn văn bản nào có trong hình.

Các nhiệm vụ:
1. Mô tả cấu trúc tổng thể của biểu diễn trực quan. Chỉ rõ đây là biểu đồ, đồ thị, bảng hay sơ đồ.
2. Xác định và trích xuất các trục, chú giải (legend), tiêu đề hoặc nhãn có trong hình ảnh. Cung cấp chính xác văn bản nếu có thể.
3. Trích xuất các điểm dữ liệu từ các yếu tố trực quan (ví dụ: độ cao của cột, tọa độ của biểu đồ đường, phần trong biểu đồ tròn, các hàng và cột trong bảng).
4. Phân tích và giải thích bất kỳ xu hướng, sự so sánh hoặc mô hình nào được thể hiện trong dữ liệu.
5. Ghi lại bất kỳ chú thích, tiêu đề phụ hoặc ghi chú nào, và giải thích ý nghĩa của chúng đối với hình ảnh.
6. Chỉ bao gồm những chi tiết được thể hiện rõ ràng trong hình ảnh. Nếu một yếu tố (ví dụ: trục, chú giải hoặc chú thích) không tồn tại hoặc không nhìn thấy được, thì không cần đề cập đến.

Định dạng đầu ra (chỉ bao gồm các phần có liên quan đến nội dung hình ảnh):
- Loại hình trực quan: [Loại hình]
- Tiêu đề: [Văn bản tiêu đề, nếu có]
- Trục / Chú giải / Nhãn: [Chi tiết, nếu có]
- Dữ liệu: [Dữ liệu được trích xuất]
- Xu hướng / Nhận định: [Phân tích và diễn giải]
- Chú thích / Ghi chú: [Văn bản và ý nghĩa, nếu có]

Đảm bảo độ chính xác cao, rõ ràng và đầy đủ trong phần phân tích, và chỉ bao gồm thông tin thực sự có trong hình ảnh. Tránh những tuyên bố không cần thiết về các yếu tố bị thiếu.
"""
    return prompt

if __name__ == "__main__":
    print(CITATION_PROMPT_TEMPLATE)
    print(CONTENT_TAGGING_PROMPT_TEMPLATE)
    print(CROSS_LANGUAGES_SYS_PROMPT_TEMPLATE)
    print(CROSS_LANGUAGES_USER_PROMPT_TEMPLATE)
    print(FULL_QUESTION_PROMPT_TEMPLATE)
    print(KEYWORD_PROMPT_TEMPLATE)
    print(QUESTION_PROMPT_TEMPLATE)
    print(VISION_LLM_DESCRIBE_PROMPT)
    print(VISION_LLM_FIGURE_DESCRIBE_PROMPT)