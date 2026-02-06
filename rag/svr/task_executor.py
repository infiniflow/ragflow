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

import time


from common.misc_utils import thread_pool_exec

start_ts = time.time()

import asyncio
import socket
import concurrent
# from beartype import BeartypeConf
# from beartype.claw import beartype_all  # <-- you didn't sign up for this
# beartype_all(conf=BeartypeConf(violation_type=UserWarning))    # <-- emit warnings from all code
import random
import sys
import threading

from api.db import PIPELINE_SPECIAL_PROGRESS_FREEZE_TASK_TYPES
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.pipeline_operation_log_service import PipelineOperationLogService
from api.db.joint_services.memory_message_service import handle_save_to_memory_task
from common.connection_utils import timeout
from common.metadata_utils import turn2jsonschema, update_metadata_to
from rag.utils.base64_image import image2id
from rag.utils.raptor_utils import should_skip_raptor, get_skip_reason
from common.log_utils import init_root_logger
from common.config_utils import show_configs
from rag.graphrag.general.index import run_graphrag_for_kb
from rag.graphrag.utils import get_llm_cache, set_llm_cache, get_tags_from_cache, set_tags_to_cache
from rag.prompts.generator import keyword_extraction, question_proposal, content_tagging, run_toc_from_text, \
    gen_metadata
import logging
import os
from datetime import datetime
import json
import xxhash
import copy
import re
from functools import partial
from multiprocessing.context import TimeoutError
from timeit import default_timer as timer
import signal
import exceptiongroup
import faulthandler
import numpy as np
from peewee import DoesNotExist
from common.constants import LLMType, ParserType, PipelineTaskType
from api.db.services.document_service import DocumentService
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.llm_service import LLMBundle
from api.db.services.task_service import TaskService, has_canceled, CANVAS_DEBUG_DOC_ID, GRAPH_RAPTOR_FAKE_DOC_ID
from api.db.services.file2document_service import File2DocumentService
from common.versions import get_ragflow_version
from api.db.db_models import close_connection
from rag.app import laws, paper, presentation, manual, qa, table, book, resume, picture, naive, one, audio, \
    email, tag
from rag.nlp import search, rag_tokenizer, add_positions
from rag.raptor import RecursiveAbstractiveProcessing4TreeOrganizedRetrieval as Raptor
from common.token_utils import num_tokens_from_string, truncate
from rag.utils.redis_conn import REDIS_CONN, RedisDistributedLock
from rag.graphrag.utils import chat_limiter
from common.signal_utils import start_tracemalloc_and_snapshot, stop_tracemalloc
from common.exceptions import TaskCanceledException
from common import settings
from common.constants import PAGERANK_FLD, TAG_FLD, SVR_CONSUMER_GROUP_NAME

BATCH_SIZE = 64

FACTORY = {
    "general": naive,
    ParserType.NAIVE.value: naive,
    ParserType.PAPER.value: paper,
    ParserType.BOOK.value: book,
    ParserType.PRESENTATION.value: presentation,
    ParserType.MANUAL.value: manual,
    ParserType.LAWS.value: laws,
    ParserType.QA.value: qa,
    ParserType.TABLE.value: table,
    ParserType.RESUME.value: resume,
    ParserType.PICTURE.value: picture,
    ParserType.ONE.value: one,
    ParserType.AUDIO.value: audio,
    ParserType.EMAIL.value: email,
    ParserType.KG.value: naive,
    ParserType.TAG.value: tag
}

TASK_TYPE_TO_PIPELINE_TASK_TYPE = {
    "dataflow": PipelineTaskType.PARSE,
    "raptor": PipelineTaskType.RAPTOR,
    "graphrag": PipelineTaskType.GRAPH_RAG,
    "mindmap": PipelineTaskType.MINDMAP,
    "memory": PipelineTaskType.MEMORY,
}

UNACKED_ITERATOR = None

CONSUMER_NO = "0" if len(sys.argv) < 2 else sys.argv[1]
CONSUMER_NAME = "task_executor_" + CONSUMER_NO
BOOT_AT = datetime.now().astimezone().isoformat(timespec="milliseconds")
PENDING_TASKS = 0
LAG_TASKS = 0
DONE_TASKS = 0
FAILED_TASKS = 0

CURRENT_TASKS = {}

MAX_CONCURRENT_TASKS = int(os.environ.get('MAX_CONCURRENT_TASKS', "5"))
MAX_CONCURRENT_CHUNK_BUILDERS = int(os.environ.get('MAX_CONCURRENT_CHUNK_BUILDERS', "1"))
MAX_CONCURRENT_MINIO = int(os.environ.get('MAX_CONCURRENT_MINIO', '10'))
task_limiter = asyncio.Semaphore(MAX_CONCURRENT_TASKS)
chunk_limiter = asyncio.Semaphore(MAX_CONCURRENT_CHUNK_BUILDERS)
embed_limiter = asyncio.Semaphore(MAX_CONCURRENT_CHUNK_BUILDERS)
minio_limiter = asyncio.Semaphore(MAX_CONCURRENT_MINIO)
kg_limiter = asyncio.Semaphore(2)
WORKER_HEARTBEAT_TIMEOUT = int(os.environ.get('WORKER_HEARTBEAT_TIMEOUT', '120'))
stop_event = threading.Event()


def signal_handler(sig, frame):
    logging.info("Received interrupt signal, shutting down...")
    stop_event.set()
    time.sleep(1)
    sys.exit(0)


def set_progress(task_id, from_page=0, to_page=-1, prog=None, msg="Processing..."):
    try:
        if prog is not None and prog < 0:
            msg = "[ERROR]" + msg
        cancel = has_canceled(task_id)

        if cancel:
            msg += " [Canceled]"
            prog = -1

        if to_page > 0:
            if msg:
                if from_page < to_page:
                    msg = f"Page({from_page + 1}~{to_page + 1}): " + msg
        if msg:
            msg = datetime.now().strftime("%H:%M:%S") + " " + msg
        d = {"progress_msg": msg}
        if prog is not None:
            d["progress"] = prog

        TaskService.update_progress(task_id, d)

        close_connection()
        if cancel:
            raise TaskCanceledException(msg)
        logging.info(f"set_progress({task_id}), progress: {prog}, progress_msg: {msg}")
    except DoesNotExist:
        logging.warning(f"set_progress({task_id}) got exception DoesNotExist")
    except Exception as e:
        logging.exception(f"set_progress({task_id}), progress: {prog}, progress_msg: {msg}, got exception: {e}")


async def collect():
    global CONSUMER_NAME, DONE_TASKS, FAILED_TASKS
    global UNACKED_ITERATOR

    svr_queue_names = settings.get_svr_queue_names()
    redis_msg = None
    try:
        if not UNACKED_ITERATOR:
            UNACKED_ITERATOR = REDIS_CONN.get_unacked_iterator(svr_queue_names, SVR_CONSUMER_GROUP_NAME, CONSUMER_NAME)
        try:
            redis_msg = next(UNACKED_ITERATOR)
        except StopIteration:
            for svr_queue_name in svr_queue_names:
                redis_msg = REDIS_CONN.queue_consumer(svr_queue_name, SVR_CONSUMER_GROUP_NAME, CONSUMER_NAME)
                if redis_msg:
                    break
    except Exception as e:
        logging.exception(f"collect got exception: {e}")
        return None, None

    if not redis_msg:
        return None, None
    msg = redis_msg.get_message()
    if not msg:
        logging.error(f"collect got empty message of {redis_msg.get_msg_id()}")
        redis_msg.ack()
        return None, None

    canceled = False
    if msg.get("doc_id", "") in [GRAPH_RAPTOR_FAKE_DOC_ID, CANVAS_DEBUG_DOC_ID]:
        task = msg
        if task["task_type"] in PIPELINE_SPECIAL_PROGRESS_FREEZE_TASK_TYPES:
            task = TaskService.get_task(msg["id"], msg["doc_ids"])
            if task:
                task["doc_id"] = msg["doc_id"]
                task["doc_ids"] = msg.get("doc_ids", []) or []
    elif msg.get("task_type") == PipelineTaskType.MEMORY.lower():
        _, task_obj = TaskService.get_by_id(msg["id"])
        task = task_obj.to_dict()
    else:
        task = TaskService.get_task(msg["id"])

    if task:
        canceled = has_canceled(task["id"])
    if not task or canceled:
        state = "is unknown" if not task else "has been cancelled"
        FAILED_TASKS += 1
        logging.warning(f"collect task {msg['id']} {state}")
        redis_msg.ack()
        return None, None

    task_type = msg.get("task_type", "")
    task["task_type"] = task_type
    if task_type[:8] == "dataflow":
        task["tenant_id"] = msg["tenant_id"]
        task["dataflow_id"] = msg["dataflow_id"]
        task["kb_id"] = msg.get("kb_id", "")
    if task_type[:6] == "memory":
        task["memory_id"] = msg["memory_id"]
        task["source_id"] = msg["source_id"]
        task["message_dict"] = msg["message_dict"]
    return redis_msg, task


async def get_storage_binary(bucket, name):
    return await thread_pool_exec(settings.STORAGE_IMPL.get, bucket, name)


@timeout(60 * 80, 1)
async def build_chunks(task, progress_callback):
    if task["size"] > settings.DOC_MAXIMUM_SIZE:
        set_progress(task["id"], prog=-1, msg="File size exceeds( <= %dMb )" %
                                              (int(settings.DOC_MAXIMUM_SIZE / 1024 / 1024)))
        return []

    chunker = FACTORY[task["parser_id"].lower()]
    try:
        st = timer()
        bucket, name = File2DocumentService.get_storage_address(doc_id=task["doc_id"])
        binary = await get_storage_binary(bucket, name)
        logging.info("From minio({}) {}/{}".format(timer() - st, task["location"], task["name"]))
    except TimeoutError:
        progress_callback(-1, "Internal server error: Fetch file from minio timeout. Could you try it again.")
        logging.exception(
            "Minio {}/{} got timeout: Fetch file from minio timeout.".format(task["location"], task["name"]))
        raise
    except Exception as e:
        if re.search("(No such file|not found)", str(e)):
            progress_callback(-1, "Can not find file <%s> from minio. Could you try it again?" % task["name"])
        else:
            progress_callback(-1, "Get file from minio: %s" % str(e).replace("'", ""))
        logging.exception("Chunking {}/{} got exception".format(task["location"], task["name"]))
        raise

    try:
        async with chunk_limiter:
            cks = await thread_pool_exec(
                chunker.chunk,
                task["name"],
                binary=binary,
                from_page=task["from_page"],
                to_page=task["to_page"],
                lang=task["language"],
                callback=progress_callback,
                kb_id=task["kb_id"],
                parser_config=task["parser_config"],
                tenant_id=task["tenant_id"],
            )
        logging.info("Chunking({}) {}/{} done".format(timer() - st, task["location"], task["name"]))
    except TaskCanceledException:
        raise
    except Exception as e:
        progress_callback(-1, "Internal server error while chunking: %s" % str(e).replace("'", ""))
        logging.exception("Chunking {}/{} got exception".format(task["location"], task["name"]))
        raise

    docs = []
    doc = {
        "doc_id": task["doc_id"],
        "kb_id": str(task["kb_id"])
    }
    if task["pagerank"]:
        doc[PAGERANK_FLD] = int(task["pagerank"])
    st = timer()

    @timeout(60)
    async def upload_to_minio(document, chunk):
        try:
            d = copy.deepcopy(document)
            d.update(chunk)
            d["id"] = xxhash.xxh64(
                (chunk["content_with_weight"] + str(d["doc_id"])).encode("utf-8", "surrogatepass")).hexdigest()
            d["create_time"] = str(datetime.now()).replace("T", " ")[:19]
            d["create_timestamp_flt"] = datetime.now().timestamp()

            if d.get("img_id"):
                docs.append(d)
                return

            if not d.get("image"):
                _ = d.pop("image", None)
                d["img_id"] = ""
                docs.append(d)
                return
            await image2id(d, partial(settings.STORAGE_IMPL.put, tenant_id=task["tenant_id"]), d["id"], task["kb_id"])
            docs.append(d)
        except Exception:
            logging.exception(
                "Saving image of chunk {}/{}/{} got exception".format(task["location"], task["name"], d["id"]))
            raise

    tasks = []
    for ck in cks:
        tasks.append(asyncio.create_task(upload_to_minio(doc, ck)))
    try:
        await asyncio.gather(*tasks, return_exceptions=False)
    except Exception as e:
        logging.error(f"MINIO PUT({task['name']}) got exception: {e}")
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise

    el = timer() - st
    logging.info("MINIO PUT({}) cost {:.3f} s".format(task["name"], el))

    if task["parser_config"].get("auto_keywords", 0):
        st = timer()
        progress_callback(msg="Start to generate keywords for every chunk ...")
        chat_mdl = LLMBundle(task["tenant_id"], LLMType.CHAT, llm_name=task["llm_id"], lang=task["language"])

        async def doc_keyword_extraction(chat_mdl, d, topn):
            cached = get_llm_cache(chat_mdl.llm_name, d["content_with_weight"], "keywords", {"topn": topn})
            if not cached:
                if has_canceled(task["id"]):
                    progress_callback(-1, msg="Task has been canceled.")
                    return
                async with chat_limiter:
                    cached = await keyword_extraction(chat_mdl, d["content_with_weight"], topn)
                set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, "keywords", {"topn": topn})
            if cached:
                d["important_kwd"] = cached.split(",")
                d["important_tks"] = rag_tokenizer.tokenize(" ".join(d["important_kwd"]))
            return

        tasks = []
        for d in docs:
            tasks.append(
                asyncio.create_task(doc_keyword_extraction(chat_mdl, d, task["parser_config"]["auto_keywords"])))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error("Error in doc_keyword_extraction: {}".format(e))
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise
        progress_callback(msg="Keywords generation {} chunks completed in {:.2f}s".format(len(docs), timer() - st))

    if task["parser_config"].get("auto_questions", 0):
        st = timer()
        progress_callback(msg="Start to generate questions for every chunk ...")
        chat_mdl = LLMBundle(task["tenant_id"], LLMType.CHAT, llm_name=task["llm_id"], lang=task["language"])

        async def doc_question_proposal(chat_mdl, d, topn):
            cached = get_llm_cache(chat_mdl.llm_name, d["content_with_weight"], "question", {"topn": topn})
            if not cached:
                if has_canceled(task["id"]):
                    progress_callback(-1, msg="Task has been canceled.")
                    return
                async with chat_limiter:
                    cached = await question_proposal(chat_mdl, d["content_with_weight"], topn)
                set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, "question", {"topn": topn})
            if cached:
                d["question_kwd"] = cached.split("\n")
                d["question_tks"] = rag_tokenizer.tokenize("\n".join(d["question_kwd"]))

        tasks = []
        for d in docs:
            tasks.append(
                asyncio.create_task(doc_question_proposal(chat_mdl, d, task["parser_config"]["auto_questions"])))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error("Error in doc_question_proposal", exc_info=e)
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise
        progress_callback(msg="Question generation {} chunks completed in {:.2f}s".format(len(docs), timer() - st))

    if task["parser_config"].get("enable_metadata", False) and task["parser_config"].get("metadata"):
        st = timer()
        progress_callback(msg="Start to generate meta-data for every chunk ...")
        chat_mdl = LLMBundle(task["tenant_id"], LLMType.CHAT, llm_name=task["llm_id"], lang=task["language"])

        async def gen_metadata_task(chat_mdl, d):
            cached = get_llm_cache(chat_mdl.llm_name, d["content_with_weight"], "metadata",
                                   task["parser_config"]["metadata"])
            if not cached:
                if has_canceled(task["id"]):
                    progress_callback(-1, msg="Task has been canceled.")
                    return
                async with chat_limiter:
                    cached = await gen_metadata(chat_mdl,
                                                turn2jsonschema(task["parser_config"]["metadata"]),
                                                d["content_with_weight"])
                set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, "metadata",
                              task["parser_config"]["metadata"])
            if cached:
                d["metadata_obj"] = cached

        tasks = []
        for d in docs:
            tasks.append(asyncio.create_task(gen_metadata_task(chat_mdl, d)))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error("Error in doc_question_proposal", exc_info=e)
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise
        metadata = {}
        for doc in docs:
            metadata = update_metadata_to(metadata, doc["metadata_obj"])
            del doc["metadata_obj"]
        if metadata:
            existing_meta = DocMetadataService.get_document_metadata(task["doc_id"])
            existing_meta = existing_meta if isinstance(existing_meta, dict) else {}
            metadata = update_metadata_to(metadata, existing_meta)
            DocMetadataService.update_document_metadata(task["doc_id"], metadata)
        progress_callback(msg="Question generation {} chunks completed in {:.2f}s".format(len(docs), timer() - st))

    if task["kb_parser_config"].get("tag_kb_ids", []):
        progress_callback(msg="Start to tag for every chunk ...")
        kb_ids = task["kb_parser_config"]["tag_kb_ids"]
        tenant_id = task["tenant_id"]
        topn_tags = task["kb_parser_config"].get("topn_tags", 3)
        S = 1000
        st = timer()
        examples = []
        all_tags = get_tags_from_cache(kb_ids)
        if not all_tags:
            all_tags = settings.retriever.all_tags_in_portion(tenant_id, kb_ids, S)
            set_tags_to_cache(kb_ids, all_tags)
        else:
            all_tags = json.loads(all_tags)

        chat_mdl = LLMBundle(task["tenant_id"], LLMType.CHAT, llm_name=task["llm_id"], lang=task["language"])

        docs_to_tag = []
        for d in docs:
            task_canceled = has_canceled(task["id"])
            if task_canceled:
                progress_callback(-1, msg="Task has been canceled.")
                return None
            if settings.retriever.tag_content(tenant_id, kb_ids, d, all_tags, topn_tags=topn_tags, S=S) and len(
                    d[TAG_FLD]) > 0:
                examples.append({"content": d["content_with_weight"], TAG_FLD: d[TAG_FLD]})
            else:
                docs_to_tag.append(d)

        async def doc_content_tagging(chat_mdl, d, topn_tags):
            cached = get_llm_cache(chat_mdl.llm_name, d["content_with_weight"], all_tags, {"topn": topn_tags})
            if not cached:
                if has_canceled(task["id"]):
                    progress_callback(-1, msg="Task has been canceled.")
                    return
                picked_examples = random.choices(examples, k=2) if len(examples) > 2 else examples
                if not picked_examples:
                    picked_examples.append({"content": "This is an example", TAG_FLD: {'example': 1}})
                async with chat_limiter:
                    cached = await content_tagging(
                        chat_mdl,
                        d["content_with_weight"],
                        all_tags,
                        picked_examples,
                        topn_tags,
                    )
                if cached:
                    cached = json.dumps(cached)
            if cached:
                set_llm_cache(chat_mdl.llm_name, d["content_with_weight"], cached, all_tags, {"topn": topn_tags})
                d[TAG_FLD] = json.loads(cached)

        tasks = []
        for d in docs_to_tag:
            tasks.append(asyncio.create_task(doc_content_tagging(chat_mdl, d, topn_tags)))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error("Error tagging docs: {}".format(e))
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise
        progress_callback(msg="Tagging {} chunks completed in {:.2f}s".format(len(docs), timer() - st))

    return docs


def build_TOC(task, docs, progress_callback):
    progress_callback(msg="Start to generate table of content ...")
    chat_mdl = LLMBundle(task["tenant_id"], LLMType.CHAT, llm_name=task["llm_id"], lang=task["language"])
    docs = sorted(docs, key=lambda d: (
        d.get("page_num_int", 0)[0] if isinstance(d.get("page_num_int", 0), list) else d.get("page_num_int", 0),
        d.get("top_int", 0)[0] if isinstance(d.get("top_int", 0), list) else d.get("top_int", 0)
    ))
    toc: list[dict] = asyncio.run(
        run_toc_from_text([d["content_with_weight"] for d in docs], chat_mdl, progress_callback))
    logging.info("------------ T O C -------------\n" + json.dumps(toc, ensure_ascii=False, indent='  '))
    for ii, item in enumerate(toc):
        try:
            chunk_val = item.pop("chunk_id", None)
            if chunk_val is None or str(chunk_val).strip() == "":
                logging.warning(f"Index {ii}: chunk_id is missing or empty. Skipping.")
                continue
            curr_idx = int(chunk_val)
            if curr_idx >= len(docs):
                logging.error(f"Index {ii}: chunk_id {curr_idx} exceeds docs length {len(docs)}.")
                continue
            item["ids"] = [docs[curr_idx]["id"]]
            if ii + 1 < len(toc):
                next_chunk_val = toc[ii + 1].get("chunk_id", "")
                if str(next_chunk_val).strip() != "":
                    next_idx = int(next_chunk_val)
                    for jj in range(curr_idx + 1, min(next_idx + 1, len(docs))):
                        item["ids"].append(docs[jj]["id"])
                else:
                    logging.warning(f"Index {ii + 1}: next chunk_id is empty, range fill skipped.")
        except (ValueError, TypeError) as e:
            logging.error(f"Index {ii}: Data conversion error - {e}")
        except Exception as e:
            logging.exception(f"Index {ii}: Unexpected error - {e}")

    if toc:
        d = copy.deepcopy(docs[-1])
        d["content_with_weight"] = json.dumps(toc, ensure_ascii=False)
        d["toc_kwd"] = "toc"
        d["available_int"] = 0
        d["page_num_int"] = [100000000]
        d["id"] = xxhash.xxh64(
            (d["content_with_weight"] + str(d["doc_id"])).encode("utf-8", "surrogatepass")).hexdigest()
        return d
    return None


def init_kb(row, vector_size: int):
    idxnm = search.index_name(row["tenant_id"])
    parser_id = row.get("parser_id", None)
    return settings.docStoreConn.create_idx(idxnm, row.get("kb_id", ""), vector_size, parser_id)


async def embedding(docs, mdl, parser_config=None, callback=None):
    if parser_config is None:
        parser_config = {}
    tts, cnts = [], []
    for d in docs:
        tts.append(d.get("docnm_kwd", "Title"))
        c = "\n".join(d.get("question_kwd", []))
        if not c:
            c = d["content_with_weight"]
        c = re.sub(r"</?(table|td|caption|tr|th)( [^<>]{0,12})?>", " ", c)
        if not c:
            c = "None"
        cnts.append(c)

    tk_count = 0
    if len(tts) == len(cnts):
        vts, c = await thread_pool_exec(mdl.encode, tts[0:1])
        tts = np.tile(vts[0], (len(cnts), 1))
        tk_count += c

    @timeout(60)
    def batch_encode(txts):
        nonlocal mdl
        return mdl.encode([truncate(c, mdl.max_length - 10) for c in txts])

    cnts_ = np.array([])
    for i in range(0, len(cnts), settings.EMBEDDING_BATCH_SIZE):
        async with embed_limiter:
            vts, c = await thread_pool_exec(batch_encode, cnts[i: i + settings.EMBEDDING_BATCH_SIZE])
        if len(cnts_) == 0:
            cnts_ = vts
        else:
            cnts_ = np.concatenate((cnts_, vts), axis=0)
        tk_count += c
        callback(prog=0.7 + 0.2 * (i + 1) / len(cnts), msg="")
    cnts = cnts_
    filename_embd_weight = parser_config.get("filename_embd_weight", 0.1)  # due to the db support none value
    if not filename_embd_weight:
        filename_embd_weight = 0.1
    title_w = float(filename_embd_weight)
    if tts.ndim == 2 and cnts.ndim == 2 and tts.shape == cnts.shape:
        vects = title_w * tts + (1 - title_w) * cnts
    else:
        vects = cnts

    assert len(vects) == len(docs)
    vector_size = 0
    for i, d in enumerate(docs):
        v = vects[i].tolist()
        vector_size = len(v)
        d["q_%d_vec" % len(v)] = v
    return tk_count, vector_size


async def run_dataflow(task: dict):
    from api.db.services.canvas_service import UserCanvasService
    from rag.flow.pipeline import Pipeline

    task_start_ts = timer()
    dataflow_id = task["dataflow_id"]
    doc_id = task["doc_id"]
    task_id = task["id"]
    task_dataset_id = task["kb_id"]

    if task["task_type"] == "dataflow":
        e, cvs = UserCanvasService.get_by_id(dataflow_id)
        assert e, "User pipeline not found."
        dsl = cvs.dsl
    else:
        e, pipeline_log = PipelineOperationLogService.get_by_id(dataflow_id)
        assert e, "Pipeline log not found."
        dsl = pipeline_log.dsl
        dataflow_id = pipeline_log.pipeline_id
    pipeline = Pipeline(dsl, tenant_id=task["tenant_id"], doc_id=doc_id, task_id=task_id, flow_id=dataflow_id)
    chunks = await pipeline.run(file=task["file"]) if task.get("file") else await pipeline.run()
    if doc_id == CANVAS_DEBUG_DOC_ID:
        return

    if not chunks:
        PipelineOperationLogService.create(document_id=doc_id, pipeline_id=dataflow_id,
                                           task_type=PipelineTaskType.PARSE, dsl=str(pipeline))
        return

    embedding_token_consumption = chunks.get("embedding_token_consumption", 0)
    if chunks.get("chunks"):
        chunks = copy.deepcopy(chunks["chunks"])
    elif chunks.get("json"):
        chunks = copy.deepcopy(chunks["json"])
    elif chunks.get("markdown"):
        chunks = [{"text": [chunks["markdown"]]}]
    elif chunks.get("text"):
        chunks = [{"text": [chunks["text"]]}]
    elif chunks.get("html"):
        chunks = [{"text": [chunks["html"]]}]

    keys = [k for o in chunks for k in list(o.keys())]
    if not any([re.match(r"q_[0-9]+_vec", k) for k in keys]):
        try:
            set_progress(task_id, prog=0.82, msg="\n-------------------------------------\nStart to embedding...")
            e, kb = KnowledgebaseService.get_by_id(task["kb_id"])
            embedding_id = kb.embd_id
            embedding_model = LLMBundle(task["tenant_id"], LLMType.EMBEDDING, llm_name=embedding_id)

            @timeout(60)
            def batch_encode(txts):
                nonlocal embedding_model
                return embedding_model.encode([truncate(c, embedding_model.max_length - 10) for c in txts])

            vects = np.array([])
            texts = [o.get("questions", o.get("summary", o["text"])) for o in chunks]
            delta = 0.20 / (len(texts) // settings.EMBEDDING_BATCH_SIZE + 1)
            prog = 0.8
            for i in range(0, len(texts), settings.EMBEDDING_BATCH_SIZE):
                async with embed_limiter:
                    vts, c = await thread_pool_exec(batch_encode, texts[i: i + settings.EMBEDDING_BATCH_SIZE])
                if len(vects) == 0:
                    vects = vts
                else:
                    vects = np.concatenate((vects, vts), axis=0)
                embedding_token_consumption += c
                prog += delta
                if i % (len(texts) // settings.EMBEDDING_BATCH_SIZE / 100 + 1) == 1:
                    set_progress(task_id, prog=prog, msg=f"{i + 1} / {len(texts) // settings.EMBEDDING_BATCH_SIZE}")

            assert len(vects) == len(chunks)
            for i, ck in enumerate(chunks):
                v = vects[i].tolist()
                ck["q_%d_vec" % len(v)] = v
        except Exception as e:
            set_progress(task_id, prog=-1, msg=f"[ERROR]: {e}")
            PipelineOperationLogService.create(document_id=doc_id, pipeline_id=dataflow_id,
                                               task_type=PipelineTaskType.PARSE, dsl=str(pipeline))
            return

    metadata = {}
    for ck in chunks:
        ck["doc_id"] = doc_id
        ck["kb_id"] = [str(task["kb_id"])]
        ck["docnm_kwd"] = task["name"]
        ck["create_time"] = str(datetime.now()).replace("T", " ")[:19]
        ck["create_timestamp_flt"] = datetime.now().timestamp()
        if not ck.get("id"):
            ck["id"] = xxhash.xxh64((ck["text"] + str(ck["doc_id"])).encode("utf-8")).hexdigest()
        if "questions" in ck:
            if "question_tks" not in ck:
                ck["question_kwd"] = ck["questions"].split("\n")
                ck["question_tks"] = rag_tokenizer.tokenize(str(ck["questions"]))
            del ck["questions"]
        if "keywords" in ck:
            if "important_tks" not in ck:
                ck["important_kwd"] = ck["keywords"].split(",")
                ck["important_tks"] = rag_tokenizer.tokenize(str(ck["keywords"]))
            del ck["keywords"]
        if "summary" in ck:
            if "content_ltks" not in ck:
                ck["content_ltks"] = rag_tokenizer.tokenize(str(ck["summary"]))
                ck["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(ck["content_ltks"])
            del ck["summary"]
        if "metadata" in ck:
            metadata = update_metadata_to(metadata, ck["metadata"])
            del ck["metadata"]
        if "content_with_weight" not in ck:
            ck["content_with_weight"] = ck["text"]
        del ck["text"]
        if "positions" in ck:
            add_positions(ck, ck["positions"])
            del ck["positions"]

    if metadata:
        existing_meta = DocMetadataService.get_document_metadata(doc_id)
        existing_meta = existing_meta if isinstance(existing_meta, dict) else {}
        metadata = update_metadata_to(metadata, existing_meta)
        DocMetadataService.update_document_metadata(doc_id, metadata)

    start_ts = timer()
    set_progress(task_id, prog=0.82, msg="[DOC Engine]:\nStart to index...")
    e = await insert_chunks(task_id, task["tenant_id"], task["kb_id"], chunks, partial(set_progress, task_id, 0, 100000000))
    if not e:
        PipelineOperationLogService.create(document_id=doc_id, pipeline_id=dataflow_id,
                                           task_type=PipelineTaskType.PARSE, dsl=str(pipeline))
        return

    time_cost = timer() - start_ts
    task_time_cost = timer() - task_start_ts
    set_progress(task_id, prog=1., msg="Indexing done ({:.2f}s). Task done ({:.2f}s)".format(time_cost, task_time_cost))
    DocumentService.increment_chunk_num(doc_id, task_dataset_id, embedding_token_consumption, len(chunks),
                                        task_time_cost)
    logging.info("[Done], chunks({}), token({}), elapsed:{:.2f}".format(len(chunks), embedding_token_consumption,
                                                                        task_time_cost))
    PipelineOperationLogService.create(document_id=doc_id, pipeline_id=dataflow_id, task_type=PipelineTaskType.PARSE,
                                       dsl=str(pipeline))


@timeout(3600)
async def run_raptor_for_kb(row, kb_parser_config, chat_mdl, embd_mdl, vector_size, callback=None, doc_ids=[]):
    fake_doc_id = GRAPH_RAPTOR_FAKE_DOC_ID

    raptor_config = kb_parser_config.get("raptor", {})
    vctr_nm = "q_%d_vec" % vector_size

    res = []
    tk_count = 0
    max_errors = int(os.environ.get("RAPTOR_MAX_ERRORS", 3))

    async def generate(chunks, did):
        nonlocal tk_count, res
        raptor = Raptor(
            raptor_config.get("max_cluster", 64),
            chat_mdl,
            embd_mdl,
            raptor_config["prompt"],
            raptor_config["max_token"],
            raptor_config["threshold"],
            max_errors=max_errors,
        )
        original_length = len(chunks)
        chunks = await raptor(chunks, kb_parser_config["raptor"]["random_seed"], callback, row["id"])
        doc = {
            "doc_id": did,
            "kb_id": [str(row["kb_id"])],
            "docnm_kwd": row["name"],
            "title_tks": rag_tokenizer.tokenize(row["name"]),
            "raptor_kwd": "raptor"
        }
        if row["pagerank"]:
            doc[PAGERANK_FLD] = int(row["pagerank"])

        for content, vctr in chunks[original_length:]:
            d = copy.deepcopy(doc)
            d["id"] = xxhash.xxh64((content + str(fake_doc_id)).encode("utf-8")).hexdigest()
            d["create_time"] = str(datetime.now()).replace("T", " ")[:19]
            d["create_timestamp_flt"] = datetime.now().timestamp()
            d[vctr_nm] = vctr.tolist()
            d["content_with_weight"] = content
            d["content_ltks"] = rag_tokenizer.tokenize(content)
            d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
            res.append(d)
            tk_count += num_tokens_from_string(content)

    if raptor_config.get("scope", "file") == "file":
        for x, doc_id in enumerate(doc_ids):
            chunks = []
            skipped_chunks = 0
            for d in settings.retriever.chunk_list(doc_id, row["tenant_id"], [str(row["kb_id"])],
                                                   fields=["content_with_weight", vctr_nm],
                                                   sort_by_position=True):
                # Skip chunks that don't have the required vector field (may have been indexed with different embedding model)
                if vctr_nm not in d or d[vctr_nm] is None:
                    skipped_chunks += 1
                    logging.warning(f"RAPTOR: Chunk missing vector field '{vctr_nm}' in doc {doc_id}, skipping")
                    continue
                chunks.append((d["content_with_weight"], np.array(d[vctr_nm])))
            
            if skipped_chunks > 0:
                callback(msg=f"[WARN] Skipped {skipped_chunks} chunks without vector field '{vctr_nm}' for doc {doc_id}. Consider re-parsing the document with the current embedding model.")
            
            if not chunks:
                logging.warning(f"RAPTOR: No valid chunks with vectors found for doc {doc_id}")
                callback(msg=f"[WARN] No valid chunks with vectors found for doc {doc_id}, skipping")
                continue
                
            await generate(chunks, doc_id)
            callback(prog=(x + 1.) / len(doc_ids))
    else:
        chunks = []
        skipped_chunks = 0
        for doc_id in doc_ids:
            for d in settings.retriever.chunk_list(doc_id, row["tenant_id"], [str(row["kb_id"])],
                                                   fields=["content_with_weight", vctr_nm],
                                                   sort_by_position=True):
                # Skip chunks that don't have the required vector field
                if vctr_nm not in d or d[vctr_nm] is None:
                    skipped_chunks += 1
                    logging.warning(f"RAPTOR: Chunk missing vector field '{vctr_nm}' in doc {doc_id}, skipping")
                    continue
                chunks.append((d["content_with_weight"], np.array(d[vctr_nm])))

        if skipped_chunks > 0:
            callback(msg=f"[WARN] Skipped {skipped_chunks} chunks without vector field '{vctr_nm}'. Consider re-parsing documents with the current embedding model.")

        if not chunks:
            logging.error(f"RAPTOR: No valid chunks with vectors found in any document for kb {row['kb_id']}")
            callback(msg=f"[ERROR] No valid chunks with vectors found. Please ensure documents are parsed with the current embedding model (vector size: {vector_size}).")
            return res, tk_count

        await generate(chunks, fake_doc_id)

    return res, tk_count


async def delete_image(kb_id, chunk_id):
    try:
        async with minio_limiter:
            settings.STORAGE_IMPL.delete(kb_id, chunk_id)
    except Exception:
        logging.exception(f"Deleting image of chunk {chunk_id} got exception")
        raise


async def insert_chunks(task_id, task_tenant_id, task_dataset_id, chunks, progress_callback):
    """
    Insert chunks into document store (Elasticsearch OR Infinity).

    Args:
        task_id: Task identifier
        task_tenant_id: Tenant ID
        task_dataset_id: Dataset/knowledge base ID
        chunks: List of chunk dictionaries to insert
        progress_callback: Callback function for progress updates
    """
    mothers = []
    mother_ids = set([])
    for ck in chunks:
        mom = ck.get("mom") or ck.get("mom_with_weight") or ""
        if not mom:
            continue
        id = xxhash.xxh64(mom.encode("utf-8")).hexdigest()
        ck["mom_id"] = id
        if id in mother_ids:
            continue
        mother_ids.add(id)
        mom_ck = copy.deepcopy(ck)
        mom_ck["id"] = id
        mom_ck["content_with_weight"] = mom
        mom_ck["available_int"] = 0
        flds = list(mom_ck.keys())
        for fld in flds:
            if fld not in ["id", "content_with_weight", "doc_id", "docnm_kwd", "kb_id", "available_int",
                           "position_int"]:
                del mom_ck[fld]
        mothers.append(mom_ck)

    for b in range(0, len(mothers), settings.DOC_BULK_SIZE):
        await thread_pool_exec(settings.docStoreConn.insert, mothers[b:b + settings.DOC_BULK_SIZE],
                                search.index_name(task_tenant_id), task_dataset_id, )
        task_canceled = has_canceled(task_id)
        if task_canceled:
            progress_callback(-1, msg="Task has been canceled.")
            return False

    for b in range(0, len(chunks), settings.DOC_BULK_SIZE):
        doc_store_result = await thread_pool_exec(settings.docStoreConn.insert, chunks[b:b + settings.DOC_BULK_SIZE],
                                                   search.index_name(task_tenant_id), task_dataset_id, )
        task_canceled = has_canceled(task_id)
        if task_canceled:
            progress_callback(-1, msg="Task has been canceled.")
            return False
        if b % 128 == 0:
            progress_callback(prog=0.8 + 0.1 * (b + 1) / len(chunks), msg="")
        if doc_store_result:
            error_message = f"Insert chunk error: {doc_store_result}, please check log file and Elasticsearch/Infinity status!"
            progress_callback(-1, msg=error_message)
            raise Exception(error_message)
        chunk_ids = [chunk["id"] for chunk in chunks[:b + settings.DOC_BULK_SIZE]]
        chunk_ids_str = " ".join(chunk_ids)
        try:
            TaskService.update_chunk_ids(task_id, chunk_ids_str)
        except DoesNotExist:
            logging.warning(f"do_handle_task update_chunk_ids failed since task {task_id} is unknown.")
            doc_store_result = await thread_pool_exec(settings.docStoreConn.delete, {"id": chunk_ids},
                                                       search.index_name(task_tenant_id), task_dataset_id, )
            tasks = []
            for chunk_id in chunk_ids:
                tasks.append(asyncio.create_task(delete_image(task_dataset_id, chunk_id)))
            try:
                await asyncio.gather(*tasks, return_exceptions=False)
            except Exception as e:
                logging.error(f"delete_image failed: {e}")
                for t in tasks:
                    t.cancel()
                await asyncio.gather(*tasks, return_exceptions=True)
                raise
            progress_callback(-1, msg=f"Chunk updates failed since task {task_id} is unknown.")
            return False
    return True


@timeout(60 * 60 * 3, 1)
async def do_handle_task(task):
    task_type = task.get("task_type", "")

    if task_type == "memory":
        await handle_save_to_memory_task(task)
        return

    if task_type == "dataflow" and task.get("doc_id", "") == CANVAS_DEBUG_DOC_ID:
        await run_dataflow(task)
        return

    task_id = task["id"]
    task_from_page = task["from_page"]
    task_to_page = task["to_page"]
    task_tenant_id = task["tenant_id"]
    task_embedding_id = task["embd_id"]
    task_language = task["language"]
    doc_task_llm_id = task["parser_config"].get("llm_id") or task["llm_id"]
    kb_task_llm_id = task['kb_parser_config'].get("llm_id") or task["llm_id"]
    task['llm_id'] = kb_task_llm_id
    task_dataset_id = task["kb_id"]
    task_doc_id = task["doc_id"]
    task_document_name = task["name"]
    task_parser_config = task["parser_config"]
    task_start_ts = timer()
    toc_thread = None
    executor = concurrent.futures.ThreadPoolExecutor()

    # prepare the progress callback function
    progress_callback = partial(set_progress, task_id, task_from_page, task_to_page)

    task_canceled = has_canceled(task_id)
    if task_canceled:
        progress_callback(-1, msg="Task has been canceled.")
        return

    try:
        # bind embedding model
        embedding_model = LLMBundle(task_tenant_id, LLMType.EMBEDDING, llm_name=task_embedding_id, lang=task_language)
        vts, _ = embedding_model.encode(["ok"])
        vector_size = len(vts[0])
    except Exception as e:
        error_message = f'Fail to bind embedding model: {str(e)}'
        progress_callback(-1, msg=error_message)
        logging.exception(error_message)
        raise

    init_kb(task, vector_size)

    if task_type[:len("dataflow")] == "dataflow":
        await run_dataflow(task)
        return

    if task_type == "raptor":
        ok, kb = KnowledgebaseService.get_by_id(task_dataset_id)
        if not ok:
            progress_callback(prog=-1.0, msg="Cannot found valid dataset for RAPTOR task")
            return

        kb_parser_config = kb.parser_config
        if not kb_parser_config.get("raptor", {}).get("use_raptor", False):
            kb_parser_config.update(
                {
                    "raptor": {
                        "use_raptor": True,
                        "prompt": "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
                        "max_token": 256,
                        "threshold": 0.1,
                        "max_cluster": 64,
                        "random_seed": 0,
                        "scope": "file"
                    },
                }
            )
            if not KnowledgebaseService.update_by_id(kb.id, {"parser_config": kb_parser_config}):
                progress_callback(prog=-1.0, msg="Internal error: Invalid RAPTOR configuration")
                return

        # Check if Raptor should be skipped for structured data
        file_type = task.get("type", "")
        parser_id = task.get("parser_id", "")
        raptor_config = kb_parser_config.get("raptor", {})

        if should_skip_raptor(file_type, parser_id, task_parser_config, raptor_config):
            skip_reason = get_skip_reason(file_type, parser_id, task_parser_config)
            logging.info(f"Skipping Raptor for document {task_document_name}: {skip_reason}")
            progress_callback(prog=1.0, msg=f"Raptor skipped: {skip_reason}")
            return

        # bind LLM for raptor
        chat_model = LLMBundle(task_tenant_id, LLMType.CHAT, llm_name=kb_task_llm_id, lang=task_language)
        # run RAPTOR
        async with kg_limiter:
            chunks, token_count = await run_raptor_for_kb(
                row=task,
                kb_parser_config=kb_parser_config,
                chat_mdl=chat_model,
                embd_mdl=embedding_model,
                vector_size=vector_size,
                callback=progress_callback,
                doc_ids=task.get("doc_ids", []),
            )
        if fake_doc_ids := task.get("doc_ids", []):
            task_doc_id = fake_doc_ids[0]  # use the first document ID to represent this task for logging purposes
    # Either using graphrag or Standard chunking methods
    elif task_type == "graphrag":
        ok, kb = KnowledgebaseService.get_by_id(task_dataset_id)
        if not ok:
            progress_callback(prog=-1.0, msg="Cannot found valid dataset for GraphRAG task")
            return

        kb_parser_config = kb.parser_config
        if not kb_parser_config.get("graphrag", {}).get("use_graphrag", False):
            kb_parser_config.update(
                {
                    "graphrag": {
                        "use_graphrag": True,
                        "entity_types": [
                            "organization",
                            "person",
                            "geo",
                            "event",
                            "category",
                        ],
                        "method": "light",
                    }
                }
            )
            if not KnowledgebaseService.update_by_id(kb.id, {"parser_config": kb_parser_config}):
                progress_callback(prog=-1.0, msg="Internal error: Invalid GraphRAG configuration")
                return

        graphrag_conf = kb_parser_config.get("graphrag", {})
        start_ts = timer()
        chat_model = LLMBundle(task_tenant_id, LLMType.CHAT, llm_name=kb_task_llm_id, lang=task_language)
        with_resolution = graphrag_conf.get("resolution", False)
        with_community = graphrag_conf.get("community", False)
        async with kg_limiter:
            # await run_graphrag(task, task_language, with_resolution, with_community, chat_model, embedding_model, progress_callback)
            result = await run_graphrag_for_kb(
                row=task,
                doc_ids=task.get("doc_ids", []),
                language=task_language,
                kb_parser_config=kb_parser_config,
                chat_model=chat_model,
                embedding_model=embedding_model,
                callback=progress_callback,
                with_resolution=with_resolution,
                with_community=with_community,
            )
            logging.info(f"GraphRAG task result for task {task}:\n{result}")
        progress_callback(prog=1.0, msg="Knowledge Graph done ({:.2f}s)".format(timer() - start_ts))
        return
    elif task_type == "mindmap":
        progress_callback(1, "place holder")
        pass
        return
    else:
        # Standard chunking methods
        task['llm_id'] = doc_task_llm_id
        start_ts = timer()
        chunks = await build_chunks(task, progress_callback)
        logging.info("Build document {}: {:.2f}s".format(task_document_name, timer() - start_ts))
        if not chunks:
            progress_callback(1., msg=f"No chunk built from {task_document_name}")
            return
        progress_callback(msg="Generate {} chunks".format(len(chunks)))
        start_ts = timer()
        try:
            token_count, vector_size = await embedding(chunks, embedding_model, task_parser_config, progress_callback)
        except Exception as e:
            error_message = "Generate embedding error:{}".format(str(e))
            progress_callback(-1, error_message)
            logging.exception(error_message)
            token_count = 0
            raise
        progress_message = "Embedding chunks ({:.2f}s)".format(timer() - start_ts)
        logging.info(progress_message)
        progress_callback(msg=progress_message)
        if task["parser_id"].lower() == "naive" and task["parser_config"].get("toc_extraction", False):
            toc_thread = executor.submit(build_TOC, task, chunks, progress_callback)

    chunk_count = len(set([chunk["id"] for chunk in chunks]))
    start_ts = timer()

    async def _maybe_insert_chunks(_chunks):
        if has_canceled(task_id):
            return True
        insert_result = await insert_chunks(task_id, task_tenant_id, task_dataset_id, _chunks, progress_callback)
        return bool(insert_result)

    try:
        if not await _maybe_insert_chunks(chunks):
            return

        logging.info(
            "Indexing doc({}), page({}-{}), chunks({}), elapsed: {:.2f}".format(
                task_document_name, task_from_page, task_to_page, len(chunks), timer() - start_ts
            )
        )

        DocumentService.increment_chunk_num(task_doc_id, task_dataset_id, token_count, chunk_count, 0)

        progress_callback(msg="Indexing done ({:.2f}s).".format(timer() - start_ts))

        if toc_thread:
            d = toc_thread.result()
            if d:
                if not await _maybe_insert_chunks([d]):
                    return
                DocumentService.increment_chunk_num(task_doc_id, task_dataset_id, 0, 1, 0)

        if has_canceled(task_id):
            progress_callback(-1, msg="Task has been canceled.")
            return

        task_time_cost = timer() - task_start_ts
        progress_callback(prog=1.0, msg="Task done ({:.2f}s)".format(task_time_cost))
        logging.info(
            "Chunk doc({}), page({}-{}), chunks({}), token({}), elapsed:{:.2f}".format(
                task_document_name, task_from_page, task_to_page, len(chunks), token_count, task_time_cost
            )
        )

    finally:
        if has_canceled(task_id):
            try:
                exists = await thread_pool_exec(
                    settings.docStoreConn.index_exist,
                    search.index_name(task_tenant_id),
                    task_dataset_id,
                )
                if exists:
                    await thread_pool_exec(
                        settings.docStoreConn.delete,
                        {"doc_id": task_doc_id},
                        search.index_name(task_tenant_id),
                        task_dataset_id,
                    )
            except Exception as e:
                logging.exception(
                    f"Remove doc({task_doc_id}) from docStore failed when task({task_id}) canceled, exception: {e}")


async def handle_task():
    global DONE_TASKS, FAILED_TASKS
    redis_msg, task = await collect()
    if not task:
        await asyncio.sleep(5)
        return

    task_type = task["task_type"]
    pipeline_task_type = TASK_TYPE_TO_PIPELINE_TASK_TYPE.get(task_type,
                                                             PipelineTaskType.PARSE) or PipelineTaskType.PARSE
    task_id = task["id"]
    try:
        logging.info(f"handle_task begin for task {json.dumps(task)}")
        CURRENT_TASKS[task["id"]] = copy.deepcopy(task)
        await do_handle_task(task)
        DONE_TASKS += 1
        CURRENT_TASKS.pop(task_id, None)
        logging.info(f"handle_task done for task {json.dumps(task)}")
    except Exception as e:
        FAILED_TASKS += 1
        CURRENT_TASKS.pop(task_id, None)
        try:
            err_msg = str(e)
            while isinstance(e, exceptiongroup.ExceptionGroup):
                e = e.exceptions[0]
                err_msg += ' -- ' + str(e)
            set_progress(task_id, prog=-1, msg=f"[Exception]: {err_msg}")
        except Exception as e:
            logging.exception(f"[Exception]: {str(e)}")
            pass
        logging.exception(f"handle_task got exception for task {json.dumps(task)}")
    finally:
        task_document_ids = []
        if task_type in ["graphrag", "raptor", "mindmap"]:
            task_document_ids = task["doc_ids"]
        if not task.get("dataflow_id", ""):
            PipelineOperationLogService.record_pipeline_operation(document_id=task["doc_id"], pipeline_id="",
                                                                  task_type=pipeline_task_type,
                                                                  fake_document_ids=task_document_ids)

    redis_msg.ack()


async def get_server_ip() -> str:
    # get ip by udp
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("8.8.8.8", 80))
            return s.getsockname()[0]
    except Exception as e:
        logging.error(str(e))
        return 'Unknown'


async def report_status():
    """
    Periodically reports the executor's heartbeat
    """
    global PENDING_TASKS, LAG_TASKS, DONE_TASKS, FAILED_TASKS

    ip_address = await get_server_ip()
    pid = os.getpid()

    # Register the executor in Redis
    REDIS_CONN.sadd("TASKEXE", CONSUMER_NAME)
    redis_lock = RedisDistributedLock("clean_task_executor", lock_value=CONSUMER_NAME, timeout=60)

    while True:
        now = datetime.now()
        now_ts = now.timestamp()

        group_info = REDIS_CONN.queue_info(settings.get_svr_queue_name(0), SVR_CONSUMER_GROUP_NAME) or {}
        PENDING_TASKS = int(group_info.get("pending", 0))
        LAG_TASKS = int(group_info.get("lag", 0))

        current = copy.deepcopy(CURRENT_TASKS)
        heartbeat = json.dumps({
            "ip_address": ip_address,
            "pid": pid,
            "name": CONSUMER_NAME,
            "now": now.astimezone().isoformat(timespec="milliseconds"),
            "boot_at": BOOT_AT,
            "pending": PENDING_TASKS,
            "lag": LAG_TASKS,
            "done": DONE_TASKS,
            "failed": FAILED_TASKS,
            "current": current,
        })

        # Report heartbeat to Redis
        try:
            REDIS_CONN.zadd(CONSUMER_NAME, heartbeat, now_ts)
        except Exception as e:
            logging.warning(f"Failed to report heartbeat: {e}")
        else:
            logging.info(f"{CONSUMER_NAME} reported heartbeat: {heartbeat}")

        # Clean up own expired heartbeat
        try:
            REDIS_CONN.zremrangebyscore(CONSUMER_NAME, 0, now_ts - 60 * 30)
        except Exception as e:
            logging.warning(f"Failed to clean heartbeat: {e}")

        # Clean other executors
        lock_acquired = False
        try:
            lock_acquired = redis_lock.acquire()
        except Exception as e:
            logging.warning(f"Failed to acquire Redis lock: {e}")
        if lock_acquired:
            try:
                task_executors = REDIS_CONN.smembers("TASKEXE") or set()
                for worker_name in task_executors:
                    if worker_name == CONSUMER_NAME:
                        continue
                    try:
                        last_heartbeat = REDIS_CONN.REDIS.zrevrange(worker_name, 0, 0, withscores=True)
                    except Exception as e:
                        logging.warning(f"Failed to read zset for {worker_name}: {e}")
                        continue

                    if not last_heartbeat or now_ts - last_heartbeat[0][1] > WORKER_HEARTBEAT_TIMEOUT:
                        logging.info(f"{worker_name} expired, removed")
                        REDIS_CONN.srem("TASKEXE", worker_name)
                        REDIS_CONN.delete(worker_name)
            except Exception as e:
                logging.warning(f"Failed to clean other executors: {e}")
            finally:
                redis_lock.release()
        await asyncio.sleep(30)


async def task_manager():
    try:
        await handle_task()
    finally:
        task_limiter.release()


async def main():
    # Stagger executor startup to prevent connection storm to Infinity
    # Extract worker number from CONSUMER_NAME (e.g., "task_executor_abc123_5" -> 5)
    try:
        worker_num = int(CONSUMER_NAME.rsplit("_", 1)[-1])
        # Add random delay: base delay + worker_num * 2.0s + random jitter
        # This spreads out connection attempts over several seconds
        startup_delay = worker_num * 2.0 + random.uniform(0, 0.5)
        if startup_delay > 0:
            logging.info(f"Staggering startup by {startup_delay:.2f}s to prevent connection storm")
            await asyncio.sleep(startup_delay)
    except (ValueError, IndexError):
        pass  # Non-standard consumer name, skip delay

    logging.info(r"""
    ____                      __  _
   /  _/___  ____ ____  _____/ /_(_)___  ____     ________  ______   _____  _____
   / // __ \/ __ `/ _ \/ ___/ __/ / __ \/ __ \   / ___/ _ \/ ___/ | / / _ \/ ___/
 _/ // / / / /_/ /  __(__  ) /_/ / /_/ / / / /  (__  )  __/ /   | |/ /  __/ /
/___/_/ /_/\__, /\___/____/\__/_/\____/_/ /_/  /____/\___/_/    |___/\___/_/
          /____/
    """)
    logging.info(f'RAGFlow version: {get_ragflow_version()}')
    show_configs()
    settings.init_settings()
    settings.check_and_install_torch()
    logging.info(f'default embedding config: {settings.EMBEDDING_CFG}')
    settings.print_rag_settings()
    if sys.platform != "win32":
        signal.signal(signal.SIGUSR1, start_tracemalloc_and_snapshot)
        signal.signal(signal.SIGUSR2, stop_tracemalloc)
    TRACE_MALLOC_ENABLED = int(os.environ.get('TRACE_MALLOC_ENABLED', "0"))
    if TRACE_MALLOC_ENABLED:
        start_tracemalloc_and_snapshot(None, None)

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    report_task = asyncio.create_task(report_status())
    tasks = []

    logging.info(f"RAGFlow ingestion is ready after {time.time() - start_ts}s initialization.")
    try:
        while not stop_event.is_set():
            await task_limiter.acquire()
            t = asyncio.create_task(task_manager())
            tasks.append(t)
    finally:
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        report_task.cancel()
        await asyncio.gather(report_task, return_exceptions=True)
    logging.error("BUG!!! You should not reach here!!!")


if __name__ == "__main__":
    faulthandler.enable()
    init_root_logger(CONSUMER_NAME)
    asyncio.run(main())
