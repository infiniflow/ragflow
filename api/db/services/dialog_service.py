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
import binascii
import logging
import re
import time
from copy import deepcopy
from datetime import datetime
from functools import partial
from timeit import default_timer as timer
from langfuse import Langfuse
from peewee import fn
from api.db.services.file_service import FileService
from common.constants import LLMType, ParserType, StatusEnum
from api.db.db_models import DB, Dialog
from api.db.services.common_service import CommonService
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.langfuse_service import TenantLangfuseService
from api.db.services.llm_service import LLMBundle
from common.metadata_utils import apply_meta_data_filter
from api.db.services.tenant_llm_service import TenantLLMService
from common.time_utils import current_timestamp, datetime_format
from rag.graphrag.general.mind_map_extractor import MindMapExtractor
from rag.advanced_rag import DeepResearcher
from rag.app.tag import label_question
from rag.nlp.search import index_name
from rag.prompts.generator import chunks_format, citation_prompt, cross_languages, full_question, kb_prompt, keyword_extraction, message_fit_in, \
    PROMPT_JINJA_ENV, ASK_SUMMARY
from common.token_utils import num_tokens_from_string
from rag.utils.tavily_conn import Tavily
from common.string_utils import remove_redundant_spaces
from common import settings


class DialogService(CommonService):
    model = Dialog

    @classmethod
    def save(cls, **kwargs):
        """Save a new record to database.

        This method creates a new record in the database with the provided field values,
        forcing an insert operation rather than an update.

        Args:
            **kwargs: Record field values as keyword arguments.

        Returns:
            Model instance: The created record object.
        """
        sample_obj = cls.model(**kwargs).save(force_insert=True)
        return sample_obj

    @classmethod
    def update_many_by_id(cls, data_list):
        """Update multiple records by their IDs.

        This method updates multiple records in the database, identified by their IDs.
        It automatically updates the update_time and update_date fields for each record.

        Args:
            data_list (list): List of dictionaries containing record data to update.
                             Each dictionary must include an 'id' field.
        """
        with DB.atomic():
            for data in data_list:
                data["update_time"] = current_timestamp()
                data["update_date"] = datetime_format(datetime.now())
                cls.model.update(data).where(cls.model.id == data["id"]).execute()

    @classmethod
    @DB.connection_context()
    def get_list(cls, tenant_id, page_number, items_per_page, orderby, desc, id, name):
        chats = cls.model.select()
        if id:
            chats = chats.where(cls.model.id == id)
        if name:
            chats = chats.where(cls.model.name == name)
        chats = chats.where((cls.model.tenant_id == tenant_id) & (cls.model.status == StatusEnum.VALID.value))
        if desc:
            chats = chats.order_by(cls.model.getter_by(orderby).desc())
        else:
            chats = chats.order_by(cls.model.getter_by(orderby).asc())

        chats = chats.paginate(page_number, items_per_page)

        return list(chats.dicts())

    @classmethod
    @DB.connection_context()
    def get_by_tenant_ids(cls, joined_tenant_ids, user_id, page_number, items_per_page, orderby, desc, keywords, parser_id=None):
        from api.db.db_models import User

        fields = [
            cls.model.id,
            cls.model.tenant_id,
            cls.model.name,
            cls.model.description,
            cls.model.language,
            cls.model.llm_id,
            cls.model.llm_setting,
            cls.model.prompt_type,
            cls.model.prompt_config,
            cls.model.similarity_threshold,
            cls.model.vector_similarity_weight,
            cls.model.top_n,
            cls.model.top_k,
            cls.model.do_refer,
            cls.model.rerank_id,
            cls.model.kb_ids,
            cls.model.icon,
            cls.model.status,
            User.nickname,
            User.avatar.alias("tenant_avatar"),
            cls.model.update_time,
            cls.model.create_time,
        ]
        if keywords:
            dialogs = (
                cls.model.select(*fields)
                .join(User, on=(cls.model.tenant_id == User.id))
                .where(
                    (cls.model.tenant_id.in_(joined_tenant_ids) | (cls.model.tenant_id == user_id)) & (cls.model.status == StatusEnum.VALID.value),
                    (fn.LOWER(cls.model.name).contains(keywords.lower())),
                )
            )
        else:
            dialogs = (
                cls.model.select(*fields)
                .join(User, on=(cls.model.tenant_id == User.id))
                .where(
                    (cls.model.tenant_id.in_(joined_tenant_ids) | (cls.model.tenant_id == user_id)) & (cls.model.status == StatusEnum.VALID.value),
                )
            )
        if parser_id:
            dialogs = dialogs.where(cls.model.parser_id == parser_id)
        if desc:
            dialogs = dialogs.order_by(cls.model.getter_by(orderby).desc())
        else:
            dialogs = dialogs.order_by(cls.model.getter_by(orderby).asc())

        count = dialogs.count()

        if page_number and items_per_page:
            dialogs = dialogs.paginate(page_number, items_per_page)

        return list(dialogs.dicts()), count

    @classmethod
    @DB.connection_context()
    def get_all_dialogs_by_tenant_id(cls, tenant_id):
        fields = [cls.model.id]
        dialogs = cls.model.select(*fields).where(cls.model.tenant_id == tenant_id)
        dialogs.order_by(cls.model.create_time.asc())
        offset, limit = 0, 100
        res = []
        while True:
            d_batch = dialogs.offset(offset).limit(limit)
            _temp = list(d_batch.dicts())
            if not _temp:
                break
            res.extend(_temp)
            offset += limit
        return res


async def async_chat_solo(dialog, messages, stream=True):
    attachments = ""
    if "files" in messages[-1]:
        attachments = "\n\n".join(FileService.get_files(messages[-1]["files"]))
    if TenantLLMService.llm_id2llm_type(dialog.llm_id) == "image2text":
        chat_mdl = LLMBundle(dialog.tenant_id, LLMType.IMAGE2TEXT, dialog.llm_id)
    else:
        chat_mdl = LLMBundle(dialog.tenant_id, LLMType.CHAT, dialog.llm_id)

    prompt_config = dialog.prompt_config
    tts_mdl = None
    if prompt_config.get("tts"):
        tts_mdl = LLMBundle(dialog.tenant_id, LLMType.TTS)
    msg = [{"role": m["role"], "content": re.sub(r"##\d+\$\$", "", m["content"])} for m in messages if m["role"] != "system"]
    if attachments and msg:
        msg[-1]["content"] += attachments
    if stream:
        stream_iter = chat_mdl.async_chat_streamly_delta(prompt_config.get("system", ""), msg, dialog.llm_setting)
        async for kind, value, state in _stream_with_think_delta(stream_iter):
            if kind == "marker":
                flags = {"start_to_think": True} if value == "<think>" else {"end_to_think": True}
                yield {"answer": "", "reference": {}, "audio_binary": None, "prompt": "", "created_at": time.time(), "final": False, **flags}
                continue
            yield {"answer": value, "reference": {}, "audio_binary": tts(tts_mdl, value), "prompt": "", "created_at": time.time(), "final": False}
    else:
        answer = await chat_mdl.async_chat(prompt_config.get("system", ""), msg, dialog.llm_setting)
        user_content = msg[-1].get("content", "[content not available]")
        logging.debug("User: {}|Assistant: {}".format(user_content, answer))
        yield {"answer": answer, "reference": {}, "audio_binary": tts(tts_mdl, answer), "prompt": "", "created_at": time.time()}


def get_models(dialog):
    embd_mdl, chat_mdl, rerank_mdl, tts_mdl = None, None, None, None
    kbs = KnowledgebaseService.get_by_ids(dialog.kb_ids)
    embedding_list = list(set([kb.embd_id for kb in kbs]))
    if len(embedding_list) > 1:
        raise Exception("**ERROR**: Knowledge bases use different embedding models.")

    if embedding_list:
        embd_mdl = LLMBundle(dialog.tenant_id, LLMType.EMBEDDING, embedding_list[0])
        if not embd_mdl:
            raise LookupError("Embedding model(%s) not found" % embedding_list[0])

    if TenantLLMService.llm_id2llm_type(dialog.llm_id) == "image2text":
        chat_mdl = LLMBundle(dialog.tenant_id, LLMType.IMAGE2TEXT, dialog.llm_id)
    else:
        chat_mdl = LLMBundle(dialog.tenant_id, LLMType.CHAT, dialog.llm_id)

    if dialog.rerank_id:
        rerank_mdl = LLMBundle(dialog.tenant_id, LLMType.RERANK, dialog.rerank_id)

    if dialog.prompt_config.get("tts"):
        tts_mdl = LLMBundle(dialog.tenant_id, LLMType.TTS)
    return kbs, embd_mdl, rerank_mdl, chat_mdl, tts_mdl


BAD_CITATION_PATTERNS = [
    re.compile(r"\(\s*ID\s*[: ]*\s*(\d+)\s*\)"),  # (ID: 12)
    re.compile(r"\[\s*ID\s*[: ]*\s*(\d+)\s*\]"),  # [ID: 12]
    re.compile(r"【\s*ID\s*[: ]*\s*(\d+)\s*】"),  # 【ID: 12】
    re.compile(r"ref\s*(\d+)", flags=re.IGNORECASE),  # ref12、REF 12
]


def repair_bad_citation_formats(answer: str, kbinfos: dict, idx: set):
    max_index = len(kbinfos["chunks"])

    def safe_add(i):
        if 0 <= i < max_index:
            idx.add(i)
            return True
        return False

    def find_and_replace(pattern, group_index=1, repl=lambda i: f"ID:{i}", flags=0):
        nonlocal answer

        def replacement(match):
            try:
                i = int(match.group(group_index))
                if safe_add(i):
                    return f"[{repl(i)}]"
            except Exception:
                pass
            return match.group(0)

        answer = re.sub(pattern, replacement, answer, flags=flags)

    for pattern in BAD_CITATION_PATTERNS:
        find_and_replace(pattern)

    return answer, idx


async def async_chat(dialog, messages, stream=True, **kwargs):
    logging.debug("Begin async_chat")
    assert messages[-1]["role"] == "user", "The last content of this conversation is not from user."
    if not dialog.kb_ids and not dialog.prompt_config.get("tavily_api_key"):
        async for ans in async_chat_solo(dialog, messages, stream):
            yield ans
        return

    chat_start_ts = timer()

    if TenantLLMService.llm_id2llm_type(dialog.llm_id) == "image2text":
        llm_model_config = TenantLLMService.get_model_config(dialog.tenant_id, LLMType.IMAGE2TEXT, dialog.llm_id)
    else:
        llm_model_config = TenantLLMService.get_model_config(dialog.tenant_id, LLMType.CHAT, dialog.llm_id)

    max_tokens = llm_model_config.get("max_tokens", 8192)

    check_llm_ts = timer()

    langfuse_tracer = None
    trace_context = {}
    langfuse_keys = TenantLangfuseService.filter_by_tenant(tenant_id=dialog.tenant_id)
    if langfuse_keys:
        langfuse = Langfuse(public_key=langfuse_keys.public_key, secret_key=langfuse_keys.secret_key, host=langfuse_keys.host)
        try:
            if langfuse.auth_check():
                langfuse_tracer = langfuse
                trace_id = langfuse_tracer.create_trace_id()
                trace_context = {"trace_id": trace_id}
        except Exception:
            # Skip langfuse tracing if connection fails
            pass

    check_langfuse_tracer_ts = timer()
    kbs, embd_mdl, rerank_mdl, chat_mdl, tts_mdl = get_models(dialog)
    toolcall_session, tools = kwargs.get("toolcall_session"), kwargs.get("tools")
    if toolcall_session and tools:
        chat_mdl.bind_tools(toolcall_session, tools)
    bind_models_ts = timer()

    retriever = settings.retriever
    questions = [m["content"] for m in messages if m["role"] == "user"][-3:]
    attachments = kwargs["doc_ids"].split(",") if "doc_ids" in kwargs else []
    attachments_= ""
    if "doc_ids" in messages[-1]:
        attachments = messages[-1]["doc_ids"]
    if "files" in messages[-1]:
        attachments_ = "\n\n".join(FileService.get_files(messages[-1]["files"]))

    prompt_config = dialog.prompt_config
    field_map = KnowledgebaseService.get_field_map(dialog.kb_ids)
    logging.debug(f"field_map retrieved: {field_map}")
    # try to use sql if field mapping is good to go
    if field_map:
        logging.debug("Use SQL to retrieval:{}".format(questions[-1]))
        ans = await use_sql(questions[-1], field_map, dialog.tenant_id, chat_mdl, prompt_config.get("quote", True), dialog.kb_ids)
        # For aggregate queries (COUNT, SUM, etc.), chunks may be empty but answer is still valid
        if ans and (ans.get("reference", {}).get("chunks") or ans.get("answer")):
            yield ans
            return
        else:
            logging.debug("SQL failed or returned no results, falling back to vector search")

    param_keys = [p["key"] for p in prompt_config.get("parameters", [])]
    logging.debug(f"attachments={attachments}, param_keys={param_keys}, embd_mdl={embd_mdl}")

    for p in prompt_config["parameters"]:
        if p["key"] == "knowledge":
            continue
        if p["key"] not in kwargs and not p["optional"]:
            raise KeyError("Miss parameter: " + p["key"])
        if p["key"] not in kwargs:
            prompt_config["system"] = prompt_config["system"].replace("{%s}" % p["key"], " ")

    if len(questions) > 1 and prompt_config.get("refine_multiturn"):
        questions = [await full_question(dialog.tenant_id, dialog.llm_id, messages)]
    else:
        questions = questions[-1:]

    if prompt_config.get("cross_languages"):
        questions = [await cross_languages(dialog.tenant_id, dialog.llm_id, questions[0], prompt_config["cross_languages"])]

    if dialog.meta_data_filter:
        metas = DocMetadataService.get_flatted_meta_by_kbs(dialog.kb_ids)
        attachments = await apply_meta_data_filter(
            dialog.meta_data_filter,
            metas,
            questions[-1],
            chat_mdl,
            attachments,
        )

    if prompt_config.get("keyword", False):
        questions[-1] += await keyword_extraction(chat_mdl, questions[-1])

    refine_question_ts = timer()

    thought = ""
    kbinfos = {"total": 0, "chunks": [], "doc_aggs": []}
    knowledges = []

    if attachments is not None and "knowledge" in param_keys:
        logging.debug("Proceeding with retrieval")
        tenant_ids = list(set([kb.tenant_id for kb in kbs]))
        knowledges = []
        if prompt_config.get("reasoning", False) or kwargs.get("reasoning"):
            reasoner = DeepResearcher(
                chat_mdl,
                prompt_config,
                partial(
                    retriever.retrieval,
                    embd_mdl=embd_mdl,
                    tenant_ids=tenant_ids,
                    kb_ids=dialog.kb_ids,
                    page=1,
                    page_size=dialog.top_n,
                    similarity_threshold=0.2,
                    vector_similarity_weight=0.3,
                    doc_ids=attachments,
                ),
            )
            queue = asyncio.Queue()
            async def callback(msg:str):
                nonlocal queue
                await queue.put(msg + "<br/>")

            await callback("<START_DEEP_RESEARCH>")
            task = asyncio.create_task(reasoner.research(kbinfos, questions[-1], questions[-1], callback=callback))
            while True:
                msg = await queue.get()
                if msg.find("<START_DEEP_RESEARCH>") == 0:
                    yield {"answer": "", "reference": {}, "audio_binary": None, "final": False, "start_to_think": True}
                elif msg.find("<END_DEEP_RESEARCH>") == 0:
                    yield {"answer": "", "reference": {}, "audio_binary": None, "final": False, "end_to_think": True}
                    break
                else:
                    yield {"answer": msg, "reference": {}, "audio_binary": None, "final": False}

            await task

        else:
            if embd_mdl:
                kbinfos = await retriever.retrieval(
                    " ".join(questions),
                    embd_mdl,
                    tenant_ids,
                    dialog.kb_ids,
                    1,
                    dialog.top_n,
                    dialog.similarity_threshold,
                    dialog.vector_similarity_weight,
                    doc_ids=attachments,
                    top=dialog.top_k,
                    aggs=True,
                    rerank_mdl=rerank_mdl,
                    rank_feature=label_question(" ".join(questions), kbs),
                )
                if prompt_config.get("toc_enhance"):
                    cks = await retriever.retrieval_by_toc(" ".join(questions), kbinfos["chunks"], tenant_ids, chat_mdl, dialog.top_n)
                    if cks:
                        kbinfos["chunks"] = cks
                kbinfos["chunks"] = retriever.retrieval_by_children(kbinfos["chunks"], tenant_ids)
            if prompt_config.get("tavily_api_key"):
                tav = Tavily(prompt_config["tavily_api_key"])
                tav_res = tav.retrieve_chunks(" ".join(questions))
                kbinfos["chunks"].extend(tav_res["chunks"])
                kbinfos["doc_aggs"].extend(tav_res["doc_aggs"])
            if prompt_config.get("use_kg"):
                ck = await settings.kg_retriever.retrieval(" ".join(questions), tenant_ids, dialog.kb_ids, embd_mdl,
                                                       LLMBundle(dialog.tenant_id, LLMType.CHAT))
                if ck["content_with_weight"]:
                    kbinfos["chunks"].insert(0, ck)

    knowledges = kb_prompt(kbinfos, max_tokens)
    logging.debug("{}->{}".format(" ".join(questions), "\n->".join(knowledges)))

    retrieval_ts = timer()
    if not knowledges and prompt_config.get("empty_response"):
        empty_res = prompt_config["empty_response"]
        yield {"answer": empty_res, "reference": kbinfos, "prompt": "\n\n### Query:\n%s" % " ".join(questions),
               "audio_binary": tts(tts_mdl, empty_res), "final": True}
        return

    kwargs["knowledge"] = "\n------\n" + "\n\n------\n\n".join(knowledges)
    gen_conf = dialog.llm_setting

    msg = [{"role": "system", "content": prompt_config["system"].format(**kwargs)+attachments_}]
    prompt4citation = ""
    if knowledges and (prompt_config.get("quote", True) and kwargs.get("quote", True)):
        prompt4citation = citation_prompt()
    msg.extend([{"role": m["role"], "content": re.sub(r"##\d+\$\$", "", m["content"])} for m in messages if m["role"] != "system"])
    used_token_count, msg = message_fit_in(msg, int(max_tokens * 0.95))
    assert len(msg) >= 2, f"message_fit_in has bug: {msg}"
    prompt = msg[0]["content"]

    if "max_tokens" in gen_conf:
        gen_conf["max_tokens"] = min(gen_conf["max_tokens"], max_tokens - used_token_count)

    def decorate_answer(answer):
        nonlocal embd_mdl, prompt_config, knowledges, kwargs, kbinfos, prompt, retrieval_ts, questions, langfuse_tracer

        refs = []
        ans = answer.split("</think>")
        think = ""
        if len(ans) == 2:
            think = ans[0] + "</think>"
            answer = ans[1]

        if knowledges and (prompt_config.get("quote", True) and kwargs.get("quote", True)):
            idx = set([])
            if embd_mdl and not re.search(r"\[ID:([0-9]+)\]", answer):
                answer, idx = retriever.insert_citations(
                    answer,
                    [ck["content_ltks"] for ck in kbinfos["chunks"]],
                    [ck["vector"] for ck in kbinfos["chunks"]],
                    embd_mdl,
                    tkweight=1 - dialog.vector_similarity_weight,
                    vtweight=dialog.vector_similarity_weight,
                )
            else:
                for match in re.finditer(r"\[ID:([0-9]+)\]", answer):
                    i = int(match.group(1))
                    if i < len(kbinfos["chunks"]):
                        idx.add(i)

            answer, idx = repair_bad_citation_formats(answer, kbinfos, idx)

            idx = set([kbinfos["chunks"][int(i)]["doc_id"] for i in idx])
            recall_docs = [d for d in kbinfos["doc_aggs"] if d["doc_id"] in idx]
            if not recall_docs:
                recall_docs = kbinfos["doc_aggs"]
            kbinfos["doc_aggs"] = recall_docs

            refs = deepcopy(kbinfos)
            for c in refs["chunks"]:
                if c.get("vector"):
                    del c["vector"]

        if answer.lower().find("invalid key") >= 0 or answer.lower().find("invalid api") >= 0:
            answer += " Please set LLM API-Key in 'User Setting -> Model providers -> API-Key'"
        finish_chat_ts = timer()

        total_time_cost = (finish_chat_ts - chat_start_ts) * 1000
        check_llm_time_cost = (check_llm_ts - chat_start_ts) * 1000
        check_langfuse_tracer_cost = (check_langfuse_tracer_ts - check_llm_ts) * 1000
        bind_embedding_time_cost = (bind_models_ts - check_langfuse_tracer_ts) * 1000
        refine_question_time_cost = (refine_question_ts - bind_models_ts) * 1000
        retrieval_time_cost = (retrieval_ts - refine_question_ts) * 1000
        generate_result_time_cost = (finish_chat_ts - retrieval_ts) * 1000

        tk_num = num_tokens_from_string(think + answer)
        prompt += "\n\n### Query:\n%s" % " ".join(questions)
        prompt = (
            f"{prompt}\n\n"
            "## Time elapsed:\n"
            f"  - Total: {total_time_cost:.1f}ms\n"
            f"  - Check LLM: {check_llm_time_cost:.1f}ms\n"
            f"  - Check Langfuse tracer: {check_langfuse_tracer_cost:.1f}ms\n"
            f"  - Bind models: {bind_embedding_time_cost:.1f}ms\n"
            f"  - Query refinement(LLM): {refine_question_time_cost:.1f}ms\n"
            f"  - Retrieval: {retrieval_time_cost:.1f}ms\n"
            f"  - Generate answer: {generate_result_time_cost:.1f}ms\n\n"
            "## Token usage:\n"
            f"  - Generated tokens(approximately): {tk_num}\n"
            f"  - Token speed: {int(tk_num / (generate_result_time_cost / 1000.0))}/s"
        )

        # Add a condition check to call the end method only if langfuse_tracer exists
        if langfuse_tracer and "langfuse_generation" in locals():
            langfuse_output = "\n" + re.sub(r"^.*?(### Query:.*)", r"\1", prompt, flags=re.DOTALL)
            langfuse_output = {"time_elapsed:": re.sub(r"\n", "  \n", langfuse_output), "created_at": time.time()}
            langfuse_generation.update(output=langfuse_output)
            langfuse_generation.end()

        return {"answer": think + answer, "reference": refs, "prompt": re.sub(r"\n", "  \n", prompt), "created_at": time.time()}

    if langfuse_tracer:
        langfuse_generation = langfuse_tracer.start_generation(
            trace_context=trace_context, name="chat", model=llm_model_config["llm_name"],
            input={"prompt": prompt, "prompt4citation": prompt4citation, "messages": msg}
        )

    if stream:
        stream_iter = chat_mdl.async_chat_streamly_delta(prompt + prompt4citation, msg[1:], gen_conf)
        last_state = None
        async for kind, value, state in _stream_with_think_delta(stream_iter):
            last_state = state
            if kind == "marker":
                flags = {"start_to_think": True} if value == "<think>" else {"end_to_think": True}
                yield {"answer": "", "reference": {}, "audio_binary": None, "final": False, **flags}
                continue
            yield {"answer": value, "reference": {}, "audio_binary": tts(tts_mdl, value), "final": False}
        full_answer = last_state.full_text if last_state else ""
        if full_answer:
            final = decorate_answer(thought + full_answer)
            final["final"] = True
            final["audio_binary"] = None
            final["answer"] = ""
            yield final
    else:
        answer = await chat_mdl.async_chat(prompt + prompt4citation, msg[1:], gen_conf)
        user_content = msg[-1].get("content", "[content not available]")
        logging.debug("User: {}|Assistant: {}".format(user_content, answer))
        res = decorate_answer(answer)
        res["audio_binary"] = tts(tts_mdl, answer)
        yield res

    return


async def use_sql(question, field_map, tenant_id, chat_mdl, quota=True, kb_ids=None):
    logging.debug(f"use_sql: Question: {question}")

    # Determine which document engine we're using
    if settings.DOC_ENGINE_INFINITY:
        doc_engine = "infinity"
    elif settings.DOC_ENGINE_OCEANBASE:
        doc_engine = "oceanbase"
    else:
        doc_engine = "es"

    # Construct the full table name
    # For Elasticsearch: ragflow_{tenant_id} (kb_id is in WHERE clause)
    # For Infinity: ragflow_{tenant_id}_{kb_id} (each KB has its own table)
    base_table = index_name(tenant_id)
    if doc_engine == "infinity" and kb_ids and len(kb_ids) == 1:
        # Infinity: append kb_id to table name
        table_name = f"{base_table}_{kb_ids[0]}"
        logging.debug(f"use_sql: Using Infinity table name: {table_name}")
    else:
        # Elasticsearch/OpenSearch: use base index name
        table_name = base_table
        logging.debug(f"use_sql: Using ES/OS table name: {table_name}")

    # Generate engine-specific SQL prompts
    if doc_engine == "infinity":
        # Build Infinity prompts with JSON extraction context
        json_field_names = list(field_map.keys())
        sys_prompt = """You are a Database Administrator. Write SQL for a table with JSON 'chunk_data' column.

JSON Extraction: json_extract_string(chunk_data, '$.FieldName')
Numeric Cast: CAST(json_extract_string(chunk_data, '$.FieldName') AS INTEGER/FLOAT)
NULL Check: json_extract_isnull(chunk_data, '$.FieldName') == false

RULES:
1. Use EXACT field names (case-sensitive) from the list below
2. For SELECT: include doc_id, docnm, and json_extract_string() for requested fields
3. For COUNT: use COUNT(*) or COUNT(DISTINCT json_extract_string(...))
4. Add AS alias for extracted field names
5. DO NOT select 'content' field
6. Only add NULL check (json_extract_isnull() == false) in WHERE clause when:
   - Question asks to "show me" or "display" specific columns
   - Question mentions "not null" or "excluding null"
   - Add NULL check for count specific column
   - DO NOT add NULL check for COUNT(*) queries (COUNT(*) counts all rows including nulls)
7. Output ONLY the SQL, no explanations"""
        user_prompt = """Table: {}
Fields (EXACT case): {}
{}
Question: {}
Write SQL using json_extract_string() with exact field names. Include doc_id, docnm for data queries. Only SQL.""".format(
            table_name,
            ", ".join(json_field_names),
            "\n".join([f"  - {field}" for field in json_field_names]),
            question
        )
    elif doc_engine == "oceanbase":
        # Build OceanBase prompts with JSON extraction context
        json_field_names = list(field_map.keys())
        sys_prompt = """You are a Database Administrator. Write SQL for a table with JSON 'chunk_data' column.

JSON Extraction: json_extract_string(chunk_data, '$.FieldName')
Numeric Cast: CAST(json_extract_string(chunk_data, '$.FieldName') AS INTEGER/FLOAT)
NULL Check: json_extract_isnull(chunk_data, '$.FieldName') == false

RULES:
1. Use EXACT field names (case-sensitive) from the list below
2. For SELECT: include doc_id, docnm_kwd, and json_extract_string() for requested fields
3. For COUNT: use COUNT(*) or COUNT(DISTINCT json_extract_string(...))
4. Add AS alias for extracted field names
5. DO NOT select 'content' field
6. Only add NULL check (json_extract_isnull() == false) in WHERE clause when:
   - Question asks to "show me" or "display" specific columns
   - Question mentions "not null" or "excluding null"
   - Add NULL check for count specific column
   - DO NOT add NULL check for COUNT(*) queries (COUNT(*) counts all rows including nulls)
7. Output ONLY the SQL, no explanations"""
        user_prompt = """Table: {}
Fields (EXACT case): {}
{}
Question: {}
Write SQL using json_extract_string() with exact field names. Include doc_id, docnm_kwd for data queries. Only SQL.""".format(
            table_name,
            ", ".join(json_field_names),
            "\n".join([f"  - {field}" for field in json_field_names]),
            question
        )
    else:
        # Build ES/OS prompts with direct field access
        sys_prompt = """You are a Database Administrator. Write SQL queries.

RULES:
1. Use EXACT field names from the schema below (e.g., product_tks, not product)
2. Quote field names starting with digit: "123_field"
3. Add IS NOT NULL in WHERE clause when:
   - Question asks to "show me" or "display" specific columns
4. Include doc_id/docnm in non-aggregate statement
5. Output ONLY the SQL, no explanations"""
        user_prompt = """Table: {}
Available fields:
{}
Question: {}
Write SQL using exact field names above. Include doc_id, docnm_kwd for data queries. Only SQL.""".format(
            table_name,
            "\n".join([f"  - {k} ({v})" for k, v in field_map.items()]),
            question
        )

    tried_times = 0

    async def get_table():
        nonlocal sys_prompt, user_prompt, question, tried_times
        sql = await chat_mdl.async_chat(sys_prompt, [{"role": "user", "content": user_prompt}], {"temperature": 0.06})
        logging.debug(f"use_sql: Raw SQL from LLM: {repr(sql[:500])}")
        # Remove think blocks if present (format: </think>...)
        sql = re.sub(r"</think>\n.*?\n\s*", "", sql, flags=re.DOTALL)
        sql = re.sub(r"思考\n.*?\n", "", sql, flags=re.DOTALL)
        # Remove markdown code blocks (```sql ... ```)
        sql = re.sub(r"```(?:sql)?\s*", "", sql, flags=re.IGNORECASE)
        sql = re.sub(r"```\s*$", "", sql, flags=re.IGNORECASE)
        # Remove trailing semicolon that ES SQL parser doesn't like
        sql = sql.rstrip().rstrip(';').strip()

        # Add kb_id filter for ES/OS only (Infinity already has it in table name)
        if doc_engine != "infinity" and kb_ids:
            # Build kb_filter: single KB or multiple KBs with OR
            if len(kb_ids) == 1:
                kb_filter = f"kb_id = '{kb_ids[0]}'"
            else:
                kb_filter = "(" + " OR ".join([f"kb_id = '{kb_id}'" for kb_id in kb_ids]) + ")"

            if "where " not in sql.lower():
                o = sql.lower().split("order by")
                if len(o) > 1:
                    sql = o[0] + f" WHERE {kb_filter}  order by " + o[1]
                else:
                    sql += f" WHERE {kb_filter}"
            elif "kb_id =" not in sql.lower() and "kb_id=" not in sql.lower():
                sql = re.sub(r"\bwhere\b ", f"where {kb_filter} and ", sql, flags=re.IGNORECASE)

        logging.debug(f"{question} get SQL(refined): {sql}")
        tried_times += 1
        logging.debug(f"use_sql: Executing SQL retrieval (attempt {tried_times})")
        tbl = settings.retriever.sql_retrieval(sql, format="json")
        if tbl is None:
            logging.debug("use_sql: SQL retrieval returned None")
            return None, sql
        logging.debug(f"use_sql: SQL retrieval completed, got {len(tbl.get('rows', []))} rows")
        return tbl, sql

    try:
        tbl, sql = await get_table()
        logging.debug(f"use_sql: Initial SQL execution SUCCESS. SQL: {sql}")
        logging.debug(f"use_sql: Retrieved {len(tbl.get('rows', []))} rows, columns: {[c['name'] for c in tbl.get('columns', [])]}")
    except Exception as e:
        logging.warning(f"use_sql: Initial SQL execution FAILED with error: {e}")
        # Build retry prompt with error information
        if doc_engine in ("infinity", "oceanbase"):
            # Build Infinity error retry prompt
            json_field_names = list(field_map.keys())
            user_prompt = """
Table name: {};
JSON fields available in 'chunk_data' column (use these exact names in json_extract_string):
{}

Question: {}
Please write the SQL using json_extract_string(chunk_data, '$.field_name') with the field names from the list above. Only SQL, no explanations.


The SQL error you provided last time is as follows:
{}

Please correct the error and write SQL again using json_extract_string(chunk_data, '$.field_name') syntax with the correct field names. Only SQL, no explanations.
""".format(table_name, "\n".join([f"  - {field}" for field in json_field_names]), question, e)
        else:
            # Build ES/OS error retry prompt
            user_prompt = """
        Table name: {};
        Table of database fields are as follows (use the field names directly in SQL):
        {}

        Question are as follows:
        {}
        Please write the SQL using the exact field names above, only SQL, without any other explanations or text.


        The SQL error you provided last time is as follows:
        {}

        Please correct the error and write SQL again using the exact field names above, only SQL, without any other explanations or text.
        """.format(table_name, "\n".join([f"{k} ({v})" for k, v in field_map.items()]), question, e)
        try:
            tbl, sql = await get_table()
            logging.debug(f"use_sql: Retry SQL execution SUCCESS. SQL: {sql}")
            logging.debug(f"use_sql: Retrieved {len(tbl.get('rows', []))} rows on retry")
        except Exception:
            logging.error("use_sql: Retry SQL execution also FAILED, returning None")
            return

    if len(tbl["rows"]) == 0:
        logging.warning(f"use_sql: No rows returned from SQL query, returning None. SQL: {sql}")
        return None

    logging.debug(f"use_sql: Proceeding with {len(tbl['rows'])} rows to build answer")

    docid_idx = set([ii for ii, c in enumerate(tbl["columns"]) if c["name"].lower() == "doc_id"])
    doc_name_idx = set([ii for ii, c in enumerate(tbl["columns"]) if c["name"].lower() in ["docnm_kwd", "docnm"]])

    logging.debug(f"use_sql: All columns: {[(i, c['name']) for i, c in enumerate(tbl['columns'])]}")
    logging.debug(f"use_sql: docid_idx={docid_idx}, doc_name_idx={doc_name_idx}")

    column_idx = [ii for ii in range(len(tbl["columns"])) if ii not in (docid_idx | doc_name_idx)]

    logging.debug(f"use_sql: column_idx={column_idx}")
    logging.debug(f"use_sql: field_map={field_map}")

    # Helper function to map column names to display names
    def map_column_name(col_name):
        if col_name.lower() == "count(star)":
            return "COUNT(*)"

        # First, try to extract AS alias from any expression (aggregate functions, json_extract_string, etc.)
        # Pattern: anything AS alias_name
        as_match = re.search(r'\s+AS\s+([^\s,)]+)', col_name, re.IGNORECASE)
        if as_match:
            alias = as_match.group(1).strip('"\'')

            # Use the alias for display name lookup
            if alias in field_map:
                display = field_map[alias]
                return re.sub(r"(/.*|（[^（）]+）)", "", display)
            # If alias not in field_map, try to match case-insensitively
            for field_key, display_value in field_map.items():
                if field_key.lower() == alias.lower():
                    return re.sub(r"(/.*|（[^（）]+）)", "", display_value)
            # Return alias as-is if no mapping found
            return alias

        # Try direct mapping first (for simple column names)
        if col_name in field_map:
            display = field_map[col_name]
            # Clean up any suffix patterns
            return re.sub(r"(/.*|（[^（）]+）)", "", display)

        # Try case-insensitive match for simple column names
        col_lower = col_name.lower()
        for field_key, display_value in field_map.items():
            if field_key.lower() == col_lower:
                return re.sub(r"(/.*|（[^（）]+）)", "", display_value)

        # For aggregate expressions or complex expressions without AS alias,
        # try to replace field names with display names
        result = col_name
        for field_name, display_name in field_map.items():
            # Replace field_name with display_name in the expression
            result = result.replace(field_name, display_name)

        # Clean up any suffix patterns
        result = re.sub(r"(/.*|（[^（）]+）)", "", result)
        return result

    # compose Markdown table
    columns = (
            "|" + "|".join(
        [map_column_name(tbl["columns"][i]["name"]) for i in column_idx]) + (
                "|Source|" if docid_idx and doc_name_idx else "|")
    )

    line = "|" + "|".join(["------" for _ in range(len(column_idx))]) + ("|------|" if docid_idx and docid_idx else "")

    # Build rows ensuring column names match values - create a dict for each row
    # keyed by column name to handle any SQL column order
    rows = []
    for row_idx, r in enumerate(tbl["rows"]):
        row_dict = {tbl["columns"][i]["name"]: r[i] for i in range(len(tbl["columns"])) if i < len(r)}
        if row_idx == 0:
            logging.debug(f"use_sql: First row data: {row_dict}")
        row_values = []
        for col_idx in column_idx:
            col_name = tbl["columns"][col_idx]["name"]
            value = row_dict.get(col_name, " ")
            row_values.append(remove_redundant_spaces(str(value)).replace("None", " "))
        # Add Source column with citation marker if Source column exists
        if docid_idx and doc_name_idx:
            row_values.append(f" ##{row_idx}$$")
        row_str = "|" + "|".join(row_values) + "|"
        if re.sub(r"[ |]+", "", row_str):
            rows.append(row_str)
    if quota:
        rows = "\n".join(rows)
    else:
        rows = "\n".join(rows)
    rows = re.sub(r"T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+Z)?\|", "|", rows)

    if not docid_idx or not doc_name_idx:
        logging.warning(f"use_sql: SQL missing required doc_id or docnm_kwd field. docid_idx={docid_idx}, doc_name_idx={doc_name_idx}. SQL: {sql}")
        # For aggregate queries (COUNT, SUM, AVG, MAX, MIN, DISTINCT), fetch doc_id, docnm_kwd separately
        # to provide source chunks, but keep the original table format answer
        if re.search(r"(count|sum|avg|max|min|distinct)\s*\(", sql.lower()):
            # Keep original table format as answer
            answer = "\n".join([columns, line, rows])

            # Now fetch doc_id, docnm_kwd to provide source chunks
            # Extract WHERE clause from the original SQL
            where_match = re.search(r"\bwhere\b(.+?)(?:\bgroup by\b|\border by\b|\blimit\b|$)", sql, re.IGNORECASE)
            if where_match:
                where_clause = where_match.group(1).strip()
                # Build a query to get doc_id and docnm_kwd with the same WHERE clause
                chunks_sql = f"select doc_id, docnm_kwd from {table_name} where {where_clause}"
                # Add LIMIT to avoid fetching too many chunks
                if "limit" not in chunks_sql.lower():
                    chunks_sql += " limit 20"
                logging.debug(f"use_sql: Fetching chunks with SQL: {chunks_sql}")
                try:
                    chunks_tbl = settings.retriever.sql_retrieval(chunks_sql, format="json")
                    if chunks_tbl.get("rows") and len(chunks_tbl["rows"]) > 0:
                        # Build chunks reference - use case-insensitive matching
                        chunks_did_idx = next((i for i, c in enumerate(chunks_tbl["columns"]) if c["name"].lower() == "doc_id"), None)
                        chunks_dn_idx = next((i for i, c in enumerate(chunks_tbl["columns"]) if c["name"].lower() in ["docnm_kwd", "docnm"]), None)
                        if chunks_did_idx is not None and chunks_dn_idx is not None:
                            chunks = [{"doc_id": r[chunks_did_idx], "docnm_kwd": r[chunks_dn_idx]} for r in chunks_tbl["rows"]]
                            # Build doc_aggs
                            doc_aggs = {}
                            for r in chunks_tbl["rows"]:
                                doc_id = r[chunks_did_idx]
                                doc_name = r[chunks_dn_idx]
                                if doc_id not in doc_aggs:
                                    doc_aggs[doc_id] = {"doc_name": doc_name, "count": 0}
                                doc_aggs[doc_id]["count"] += 1
                            doc_aggs_list = [{"doc_id": did, "doc_name": d["doc_name"], "count": d["count"]} for did, d in doc_aggs.items()]
                            logging.debug(f"use_sql: Returning aggregate answer with {len(chunks)} chunks from {len(doc_aggs)} documents")
                            return {"answer": answer, "reference": {"chunks": chunks, "doc_aggs": doc_aggs_list}, "prompt": sys_prompt}
                except Exception as e:
                    logging.warning(f"use_sql: Failed to fetch chunks: {e}")
            # Fallback: return answer without chunks
            return {"answer": answer, "reference": {"chunks": [], "doc_aggs": []}, "prompt": sys_prompt}
        # Fallback to table format for other cases
        return {"answer": "\n".join([columns, line, rows]), "reference": {"chunks": [], "doc_aggs": []}, "prompt": sys_prompt}

    docid_idx = list(docid_idx)[0]
    doc_name_idx = list(doc_name_idx)[0]
    doc_aggs = {}
    for r in tbl["rows"]:
        if r[docid_idx] not in doc_aggs:
            doc_aggs[r[docid_idx]] = {"doc_name": r[doc_name_idx], "count": 0}
        doc_aggs[r[docid_idx]]["count"] += 1

    result = {
        "answer": "\n".join([columns, line, rows]),
        "reference": {
            "chunks": [{"doc_id": r[docid_idx], "docnm_kwd": r[doc_name_idx]} for r in tbl["rows"]],
            "doc_aggs": [{"doc_id": did, "doc_name": d["doc_name"], "count": d["count"]} for did, d in doc_aggs.items()],
        },
        "prompt": sys_prompt,
    }
    logging.debug(f"use_sql: Returning answer with {len(result['reference']['chunks'])} chunks from {len(doc_aggs)} documents")
    return result

def clean_tts_text(text: str) -> str:
    if not text:
        return ""

    text = text.encode("utf-8", "ignore").decode("utf-8", "ignore")

    text = re.sub(r"[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]", "", text)

    emoji_pattern = re.compile(
        "[\U0001F600-\U0001F64F"
        "\U0001F300-\U0001F5FF"
        "\U0001F680-\U0001F6FF"
        "\U0001F1E0-\U0001F1FF"
        "\U00002700-\U000027BF"
        "\U0001F900-\U0001F9FF"
        "\U0001FA70-\U0001FAFF"
        "\U0001FAD0-\U0001FAFF]+",
        flags=re.UNICODE
    )
    text = emoji_pattern.sub("", text)

    text = re.sub(r"\s+", " ", text).strip()

    MAX_LEN = 500
    if len(text) > MAX_LEN:
        text = text[:MAX_LEN]

    return text

def tts(tts_mdl, text):
    if not tts_mdl or not text:
        return None
    text = clean_tts_text(text)
    if not text:
        return None
    bin = b""
    try:
        for chunk in tts_mdl.tts(text):
            bin += chunk
    except Exception as e:
        logging.error(f"TTS failed: {e}, text={text!r}")
        return None
    return binascii.hexlify(bin).decode("utf-8")


class _ThinkStreamState:
    def __init__(self) -> None:
        self.full_text = ""
        self.last_idx = 0
        self.endswith_think = False
        self.last_full = ""
        self.last_model_full = ""
        self.in_think = False
        self.buffer = ""


def _next_think_delta(state: _ThinkStreamState) -> str:
    full_text = state.full_text
    if full_text == state.last_full:
        return ""
    state.last_full = full_text
    delta_ans = full_text[state.last_idx:]

    if delta_ans.find("<think>") == 0:
        state.last_idx += len("<think>")
        return "<think>"
    if delta_ans.find("<think>") > 0:
        delta_text = full_text[state.last_idx:state.last_idx + delta_ans.find("<think>")]
        state.last_idx += delta_ans.find("<think>")
        return delta_text
    if delta_ans.endswith("</think>"):
        state.endswith_think = True
    elif state.endswith_think:
        state.endswith_think = False
        return "</think>"

    state.last_idx = len(full_text)
    if full_text.endswith("</think>"):
        state.last_idx -= len("</think>")
    return re.sub(r"(<think>|</think>)", "", delta_ans)


async def _stream_with_think_delta(stream_iter, min_tokens: int = 16):
    state = _ThinkStreamState()
    async for chunk in stream_iter:
        if not chunk:
            continue
        if chunk.startswith(state.last_model_full):
            new_part = chunk[len(state.last_model_full):]
            state.last_model_full = chunk
        else:
            new_part = chunk
            state.last_model_full += chunk
        if not new_part:
            continue
        state.full_text += new_part
        delta = _next_think_delta(state)
        if not delta:
            continue
        if delta in ("<think>", "</think>"):
            if delta == "<think>" and state.in_think:
                continue
            if delta == "</think>" and not state.in_think:
                continue
            if state.buffer:
                yield ("text", state.buffer, state)
                state.buffer = ""
            state.in_think = delta == "<think>"
            yield ("marker", delta, state)
            continue
        state.buffer += delta
        if num_tokens_from_string(state.buffer) < min_tokens:
            continue
        yield ("text", state.buffer, state)
        state.buffer = ""

    if state.buffer:
        yield ("text", state.buffer, state)
        state.buffer = ""
    if state.endswith_think:
        yield ("marker", "</think>", state)

async def async_ask(question, kb_ids, tenant_id, chat_llm_name=None, search_config={}):
    doc_ids = search_config.get("doc_ids", [])
    rerank_mdl = None
    kb_ids = search_config.get("kb_ids", kb_ids)
    chat_llm_name = search_config.get("chat_id", chat_llm_name)
    rerank_id = search_config.get("rerank_id", "")
    meta_data_filter = search_config.get("meta_data_filter")

    kbs = KnowledgebaseService.get_by_ids(kb_ids)
    embedding_list = list(set([kb.embd_id for kb in kbs]))

    is_knowledge_graph = all([kb.parser_id == ParserType.KG for kb in kbs])
    retriever = settings.retriever if not is_knowledge_graph else settings.kg_retriever

    embd_mdl = LLMBundle(tenant_id, LLMType.EMBEDDING, embedding_list[0])
    chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, chat_llm_name)
    if rerank_id:
        rerank_mdl = LLMBundle(tenant_id, LLMType.RERANK, rerank_id)
    max_tokens = chat_mdl.max_length
    tenant_ids = list(set([kb.tenant_id for kb in kbs]))

    if meta_data_filter:
        metas = DocMetadataService.get_flatted_meta_by_kbs(kb_ids)
        doc_ids = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl, doc_ids)

    kbinfos = await retriever.retrieval(
        question=question,
        embd_mdl=embd_mdl,
        tenant_ids=tenant_ids,
        kb_ids=kb_ids,
        page=1,
        page_size=12,
        similarity_threshold=search_config.get("similarity_threshold", 0.1),
        vector_similarity_weight=search_config.get("vector_similarity_weight", 0.3),
        top=search_config.get("top_k", 1024),
        doc_ids=doc_ids,
        aggs=True,
        rerank_mdl=rerank_mdl,
        rank_feature=label_question(question, kbs)
    )

    knowledges = kb_prompt(kbinfos, max_tokens)
    sys_prompt = PROMPT_JINJA_ENV.from_string(ASK_SUMMARY).render(knowledge="\n".join(knowledges))

    msg = [{"role": "user", "content": question}]

    def decorate_answer(answer):
        nonlocal knowledges, kbinfos, sys_prompt
        answer, idx = retriever.insert_citations(answer, [ck["content_ltks"] for ck in kbinfos["chunks"]], [ck["vector"] for ck in kbinfos["chunks"]],
                                                 embd_mdl, tkweight=0.7, vtweight=0.3)
        idx = set([kbinfos["chunks"][int(i)]["doc_id"] for i in idx])
        recall_docs = [d for d in kbinfos["doc_aggs"] if d["doc_id"] in idx]
        if not recall_docs:
            recall_docs = kbinfos["doc_aggs"]
        kbinfos["doc_aggs"] = recall_docs
        refs = deepcopy(kbinfos)
        for c in refs["chunks"]:
            if c.get("vector"):
                del c["vector"]

        if answer.lower().find("invalid key") >= 0 or answer.lower().find("invalid api") >= 0:
            answer += " Please set LLM API-Key in 'User Setting -> Model Providers -> API-Key'"
        refs["chunks"] = chunks_format(refs)
        return {"answer": answer, "reference": refs}

    stream_iter = chat_mdl.async_chat_streamly_delta(sys_prompt, msg, {"temperature": 0.1})
    last_state = None
    async for kind, value, state in _stream_with_think_delta(stream_iter):
        last_state = state
        if kind == "marker":
            flags = {"start_to_think": True} if value == "<think>" else {"end_to_think": True}
            yield {"answer": "", "reference": {}, "final": False, **flags}
            continue
        yield {"answer": value, "reference": {}, "final": False}
    full_answer = last_state.full_text if last_state else ""
    final = decorate_answer(full_answer)
    final["final"] = True
    final["answer"] = ""
    yield final


async def gen_mindmap(question, kb_ids, tenant_id, search_config={}):
    meta_data_filter = search_config.get("meta_data_filter", {})
    doc_ids = search_config.get("doc_ids", [])
    rerank_id = search_config.get("rerank_id", "")
    rerank_mdl = None
    kbs = KnowledgebaseService.get_by_ids(kb_ids)
    if not kbs:
        return {"error": "No KB selected"}
    embedding_list = list(set([kb.embd_id for kb in kbs]))
    tenant_ids = list(set([kb.tenant_id for kb in kbs]))

    embd_mdl = LLMBundle(tenant_id, LLMType.EMBEDDING, llm_name=embedding_list[0])
    chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, llm_name=search_config.get("chat_id", ""))
    if rerank_id:
        rerank_mdl = LLMBundle(tenant_id, LLMType.RERANK, rerank_id)

    if meta_data_filter:
        metas = DocMetadataService.get_flatted_meta_by_kbs(kb_ids)
        doc_ids = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl, doc_ids)

    ranks = await settings.retriever.retrieval(
        question=question,
        embd_mdl=embd_mdl,
        tenant_ids=tenant_ids,
        kb_ids=kb_ids,
        page=1,
        page_size=12,
        similarity_threshold=search_config.get("similarity_threshold", 0.2),
        vector_similarity_weight=search_config.get("vector_similarity_weight", 0.3),
        top=search_config.get("top_k", 1024),
        doc_ids=doc_ids,
        aggs=False,
        rerank_mdl=rerank_mdl,
        rank_feature=label_question(question, kbs),
    )
    mindmap = MindMapExtractor(chat_mdl)
    mind_map = await mindmap([c["content_with_weight"] for c in ranks["chunks"]])
    return mind_map.output
