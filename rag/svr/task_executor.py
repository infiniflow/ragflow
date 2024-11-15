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
import sys
from api.utils.log_utils import initRootLogger

CONSUMER_NO = "0" if len(sys.argv) < 2 else sys.argv[1]
initRootLogger(f"task_executor_{CONSUMER_NO}")
for module in ["pdfminer"]:
    module_logger = logging.getLogger(module)
    module_logger.setLevel(logging.WARNING)
for module in ["peewee"]:
    module_logger = logging.getLogger(module)
    module_logger.handlers.clear()
    module_logger.propagate = True

from datetime import datetime
import json
import os
import hashlib
import copy
import re
import sys
import time
import threading
from functools import partial
from io import BytesIO
from multiprocessing.context import TimeoutError
from timeit import default_timer as timer

import numpy as np

from api.db import LLMType, ParserType
from api.db.services.dialog_service import keyword_extraction, question_proposal
from api.db.services.document_service import DocumentService
from api.db.services.llm_service import LLMBundle
from api.db.services.task_service import TaskService
from api.db.services.file2document_service import File2DocumentService
from api import settings
from api.db.db_models import close_connection
from rag.app import laws, paper, presentation, manual, qa, table, book, resume, picture, naive, one, audio, \
    knowledge_graph, email
from rag.nlp import search, rag_tokenizer
from rag.raptor import RecursiveAbstractiveProcessing4TreeOrganizedRetrieval as Raptor
from rag.settings import DOC_MAXIMUM_SIZE, SVR_QUEUE_NAME
from rag.utils import rmSpace, num_tokens_from_string
from rag.utils.redis_conn import REDIS_CONN, Payload
from rag.utils.storage_factory import STORAGE_IMPL

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
    ParserType.KG.value: knowledge_graph
}

CONSUMER_NAME = "task_consumer_" + CONSUMER_NO
PAYLOAD: Payload | None = None
BOOT_AT = datetime.now().isoformat()
DONE_TASKS = 0
FAILED_TASKS = 0
PENDING_TASKS = 0
LAG_TASKS = 0


def set_progress(task_id, from_page=0, to_page=-1, prog=None, msg="Processing..."):
    global PAYLOAD
    if prog is not None and prog < 0:
        msg = "[ERROR]" + msg
    cancel = TaskService.do_cancel(task_id)
    if cancel:
        msg += " [Canceled]"
        prog = -1

    if to_page > 0:
        if msg:
            msg = f"Page({from_page + 1}~{to_page + 1}): " + msg
    d = {"progress_msg": msg}
    if prog is not None:
        d["progress"] = prog
    try:
        TaskService.update_progress(task_id, d)
    except Exception:
        logging.exception(f"set_progress({task_id}) got exception")

    close_connection()
    if cancel:
        if PAYLOAD:
            PAYLOAD.ack()
            PAYLOAD = None
        os._exit(0)


def collect():
    global CONSUMER_NAME, PAYLOAD, DONE_TASKS, FAILED_TASKS
    try:
        PAYLOAD = REDIS_CONN.get_unacked_for(CONSUMER_NAME, SVR_QUEUE_NAME, "rag_flow_svr_task_broker")
        if not PAYLOAD:
            PAYLOAD = REDIS_CONN.queue_consumer(SVR_QUEUE_NAME, "rag_flow_svr_task_broker", CONSUMER_NAME)
        if not PAYLOAD:
            time.sleep(1)
            return None
    except Exception:
        logging.exception("Get task event from queue exception")
        return None

    msg = PAYLOAD.get_message()
    if not msg:
        return None

    if TaskService.do_cancel(msg["id"]):
        DONE_TASKS += 1
        logging.info("Task {} has been canceled.".format(msg["id"]))
        return None
    task = TaskService.get_task(msg["id"])
    if not task:
        DONE_TASKS += 1
        logging.warning("{} empty task!".format(msg["id"]))
        return None

    if msg.get("type", "") == "raptor":
        task["task_type"] = "raptor"
    return task


def get_storage_binary(bucket, name):
    return STORAGE_IMPL.get(bucket, name)


def build(row):
    if row["size"] > DOC_MAXIMUM_SIZE:
        set_progress(row["id"], prog=-1, msg="File size exceeds( <= %dMb )" %
                                             (int(DOC_MAXIMUM_SIZE / 1024 / 1024)))
        return []

    callback = partial(
        set_progress,
        row["id"],
        row["from_page"],
        row["to_page"])
    chunker = FACTORY[row["parser_id"].lower()]
    try:
        st = timer()
        bucket, name = File2DocumentService.get_storage_address(doc_id=row["doc_id"])
        binary = get_storage_binary(bucket, name)
        logging.info(
            "From minio({}) {}/{}".format(timer() - st, row["location"], row["name"]))
    except TimeoutError:
        callback(-1, "Internal server error: Fetch file from minio timeout. Could you try it again.")
        logging.exception(
            "Minio {}/{} got timeout: Fetch file from minio timeout.".format(row["location"], row["name"]))
        raise
    except Exception as e:
        if re.search("(No such file|not found)", str(e)):
            callback(-1, "Can not find file <%s> from minio. Could you try it again?" % row["name"])
        else:
            callback(-1, "Get file from minio: %s" % str(e).replace("'", ""))
        logging.exception("Chunking {}/{} got exception".format(row["location"], row["name"]))
        raise

    try:
        cks = chunker.chunk(row["name"], binary=binary, from_page=row["from_page"],
                            to_page=row["to_page"], lang=row["language"], callback=callback,
                            kb_id=row["kb_id"], parser_config=row["parser_config"], tenant_id=row["tenant_id"])
        logging.info("Chunking({}) {}/{} done".format(timer() - st, row["location"], row["name"]))
    except Exception as e:
        callback(-1, "Internal server error while chunking: %s" %
                 str(e).replace("'", ""))
        logging.exception("Chunking {}/{} got exception".format(row["location"], row["name"]))
        raise

    docs = []
    doc = {
        "doc_id": row["doc_id"],
        "kb_id": str(row["kb_id"])
    }
    el = 0
    for ck in cks:
        d = copy.deepcopy(doc)
        d.update(ck)
        md5 = hashlib.md5()
        md5.update((ck["content_with_weight"] +
                    str(d["doc_id"])).encode("utf-8"))
        d["id"] = md5.hexdigest()
        d["create_time"] = str(datetime.now()).replace("T", " ")[:19]
        d["create_timestamp_flt"] = datetime.now().timestamp()
        if not d.get("image"):
            _ = d.pop("image", None)
            d["img_id"] = ""
            d["page_num_list"] = json.dumps([])
            d["position_list"] = json.dumps([])
            d["top_list"] = json.dumps([])
            docs.append(d)
            continue

        try:
            output_buffer = BytesIO()
            if isinstance(d["image"], bytes):
                output_buffer = BytesIO(d["image"])
            else:
                d["image"].save(output_buffer, format='JPEG')

            st = timer()
            STORAGE_IMPL.put(row["kb_id"], d["id"], output_buffer.getvalue())
            el += timer() - st
        except Exception:
            logging.exception(
                "Saving image of chunk {}/{}/{} got exception".format(row["location"], row["name"], d["_id"]))
            raise

        d["img_id"] = "{}-{}".format(row["kb_id"], d["id"])
        del d["image"]
        docs.append(d)
    logging.info("MINIO PUT({}):{}".format(row["name"], el))

    if row["parser_config"].get("auto_keywords", 0):
        st = timer()
        callback(msg="Start to generate keywords for every chunk ...")
        chat_mdl = LLMBundle(row["tenant_id"], LLMType.CHAT, llm_name=row["llm_id"], lang=row["language"])
        for d in docs:
            d["important_kwd"] = keyword_extraction(chat_mdl, d["content_with_weight"],
                                                    row["parser_config"]["auto_keywords"]).split(",")
            d["important_tks"] = rag_tokenizer.tokenize(" ".join(d["important_kwd"]))
        callback(msg="Keywords generation completed in {:.2f}s".format(timer() - st))

    if row["parser_config"].get("auto_questions", 0):
        st = timer()
        callback(msg="Start to generate questions for every chunk ...")
        chat_mdl = LLMBundle(row["tenant_id"], LLMType.CHAT, llm_name=row["llm_id"], lang=row["language"])
        for d in docs:
            qst = question_proposal(chat_mdl, d["content_with_weight"], row["parser_config"]["auto_questions"])
            d["content_with_weight"] = f"Question: \n{qst}\n\nAnswer:\n" + d["content_with_weight"]
            qst = rag_tokenizer.tokenize(qst)
            if "content_ltks" in d:
                d["content_ltks"] += " " + qst
            if "content_sm_ltks" in d:
                d["content_sm_ltks"] += " " + rag_tokenizer.fine_grained_tokenize(qst)
        callback(msg="Question generation completed in {:.2f}s".format(timer() - st))

    return docs


def init_kb(row, vector_size: int):
    idxnm = search.index_name(row["tenant_id"])
    return settings.docStoreConn.createIdx(idxnm, row["kb_id"], vector_size)


def embedding(docs, mdl, parser_config=None, callback=None):
    if parser_config is None:
        parser_config = {}
    batch_size = 32
    tts, cnts = [rmSpace(d["title_tks"]) for d in docs if d.get("title_tks")], [
        re.sub(r"</?(table|td|caption|tr|th)( [^<>]{0,12})?>", " ", d["content_with_weight"]) for d in docs]
    tk_count = 0
    if len(tts) == len(cnts):
        tts_ = np.array([])
        for i in range(0, len(tts), batch_size):
            vts, c = mdl.encode(tts[i: i + batch_size])
            if len(tts_) == 0:
                tts_ = vts
            else:
                tts_ = np.concatenate((tts_, vts), axis=0)
            tk_count += c
            callback(prog=0.6 + 0.1 * (i + 1) / len(tts), msg="")
        tts = tts_

    cnts_ = np.array([])
    for i in range(0, len(cnts), batch_size):
        vts, c = mdl.encode(cnts[i: i + batch_size])
        if len(cnts_) == 0:
            cnts_ = vts
        else:
            cnts_ = np.concatenate((cnts_, vts), axis=0)
        tk_count += c
        callback(prog=0.7 + 0.2 * (i + 1) / len(cnts), msg="")
    cnts = cnts_

    title_w = float(parser_config.get("filename_embd_weight", 0.1))
    vects = (title_w * tts + (1 - title_w) *
             cnts) if len(tts) == len(cnts) else cnts

    assert len(vects) == len(docs)
    vector_size = 0
    for i, d in enumerate(docs):
        v = vects[i].tolist()
        vector_size = len(v)
        d["q_%d_vec" % len(v)] = v
    return tk_count, vector_size


def run_raptor(row, chat_mdl, embd_mdl, callback=None):
    vts, _ = embd_mdl.encode(["ok"])
    vector_size = len(vts[0])
    vctr_nm = "q_%d_vec" % vector_size
    chunks = []
    for d in settings.retrievaler.chunk_list(row["doc_id"], row["tenant_id"], [str(row["kb_id"])],
                                             fields=["content_with_weight", vctr_nm]):
        chunks.append((d["content_with_weight"], np.array(d[vctr_nm])))

    raptor = Raptor(
        row["parser_config"]["raptor"].get("max_cluster", 64),
        chat_mdl,
        embd_mdl,
        row["parser_config"]["raptor"]["prompt"],
        row["parser_config"]["raptor"]["max_token"],
        row["parser_config"]["raptor"]["threshold"]
    )
    original_length = len(chunks)
    raptor(chunks, row["parser_config"]["raptor"]["random_seed"], callback)
    doc = {
        "doc_id": row["doc_id"],
        "kb_id": [str(row["kb_id"])],
        "docnm_kwd": row["name"],
        "title_tks": rag_tokenizer.tokenize(row["name"])
    }
    res = []
    tk_count = 0
    for content, vctr in chunks[original_length:]:
        d = copy.deepcopy(doc)
        md5 = hashlib.md5()
        md5.update((content + str(d["doc_id"])).encode("utf-8"))
        d["id"] = md5.hexdigest()
        d["create_time"] = str(datetime.now()).replace("T", " ")[:19]
        d["create_timestamp_flt"] = datetime.now().timestamp()
        d[vctr_nm] = vctr.tolist()
        d["content_with_weight"] = content
        d["content_ltks"] = rag_tokenizer.tokenize(content)
        d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
        res.append(d)
        tk_count += num_tokens_from_string(content)
    return res, tk_count, vector_size


def do_handle_task(r):
    callback = partial(set_progress, r["id"], r["from_page"], r["to_page"])
    try:
        embd_mdl = LLMBundle(r["tenant_id"], LLMType.EMBEDDING, llm_name=r["embd_id"], lang=r["language"])
    except Exception as e:
        callback(-1, msg=str(e))
        raise
    if r.get("task_type", "") == "raptor":
        try:
            chat_mdl = LLMBundle(r["tenant_id"], LLMType.CHAT, llm_name=r["llm_id"], lang=r["language"])
            cks, tk_count, vector_size = run_raptor(r, chat_mdl, embd_mdl, callback)
        except Exception as e:
            callback(-1, msg=str(e))
            raise
    else:
        st = timer()
        cks = build(r)
        logging.info("Build chunks({}): {}".format(r["name"], timer() - st))
        if cks is None:
            return
        if not cks:
            callback(1., "No chunk! Done!")
            return
        # TODO: exception handler
        ## set_progress(r["did"], -1, "ERROR: ")
        callback(
                msg="Finished slicing files ({} chunks in {:.2f}s). Start to embedding the content.".format(len(cks),
                                                                                                            timer() - st)
        )
        st = timer()
        try:
            tk_count, vector_size = embedding(cks, embd_mdl, r["parser_config"], callback)
        except Exception as e:
            callback(-1, "Embedding error:{}".format(str(e)))
            logging.exception("run_rembedding got exception")
            tk_count = 0
            raise
        logging.info("Embedding elapsed({}): {:.2f}".format(r["name"], timer() - st))
        callback(msg="Finished embedding (in {:.2f}s)! Start to build index!".format(timer() - st))
    # logging.info(f"task_executor init_kb index {search.index_name(r["tenant_id"])} embd_mdl {embd_mdl.llm_name} vector length {vector_size}")
    init_kb(r, vector_size)
    chunk_count = len(set([c["id"] for c in cks]))
    st = timer()
    es_r = ""
    es_bulk_size = 4
    for b in range(0, len(cks), es_bulk_size):
        es_r = settings.docStoreConn.insert(cks[b:b + es_bulk_size], search.index_name(r["tenant_id"]), r["kb_id"])
        if b % 128 == 0:
            callback(prog=0.8 + 0.1 * (b + 1) / len(cks), msg="")
    logging.info("Indexing elapsed({}): {:.2f}".format(r["name"], timer() - st))
    if es_r:
        callback(-1, "Insert chunk error, detail info please check log file. Please also check Elasticsearch/Infinity status!")
        settings.docStoreConn.delete({"doc_id": r["doc_id"]}, search.index_name(r["tenant_id"]), r["kb_id"])
        logging.error('Insert chunk error: ' + str(es_r))
        raise Exception('Insert chunk error: ' + str(es_r))

    if TaskService.do_cancel(r["id"]):
        settings.docStoreConn.delete({"doc_id": r["doc_id"]}, search.index_name(r["tenant_id"]), r["kb_id"])
        return

    callback(msg="Indexing elapsed in {:.2f}s.".format(timer() - st))
    callback(1., "Done!")
    DocumentService.increment_chunk_num(
        r["doc_id"], r["kb_id"], tk_count, chunk_count, 0)
    logging.info(
        "Chunk doc({}), token({}), chunks({}), elapsed:{:.2f}".format(
                r["id"], tk_count, len(cks), timer() - st))


def handle_task():
    global PAYLOAD, DONE_TASKS, FAILED_TASKS
    task = collect()
    if task:
        try:
            logging.info(f"handle_task begin for task {json.dumps(task)}")
            do_handle_task(task)
            DONE_TASKS += 1
            logging.exception(f"handle_task done for task {json.dumps(task)}")
        except Exception:
            FAILED_TASKS += 1
            logging.exception(f"handle_task got exception for task {json.dumps(task)}")
    if PAYLOAD:
        PAYLOAD.ack()
        PAYLOAD = None


def report_status():
    global CONSUMER_NAME, BOOT_AT, DONE_TASKS, FAILED_TASKS, PENDING_TASKS, LAG_TASKS
    REDIS_CONN.sadd("TASKEXE", CONSUMER_NAME)
    while True:
        try:
            now = datetime.now()
            group_info = REDIS_CONN.queue_info(SVR_QUEUE_NAME, "rag_flow_svr_task_broker")
            if group_info is not None:
                PENDING_TASKS = int(group_info["pending"])
                LAG_TASKS = int(group_info["lag"])

            heartbeat = json.dumps({
                "name": CONSUMER_NAME,
                "now": now.isoformat(),
                "boot_at": BOOT_AT,
                "done": DONE_TASKS,
                "failed": FAILED_TASKS,
                "pending": PENDING_TASKS,
                "lag": LAG_TASKS,
            })
            REDIS_CONN.zadd(CONSUMER_NAME, heartbeat, now.timestamp())
            logging.info(f"{CONSUMER_NAME} reported heartbeat: {heartbeat}")

            expired = REDIS_CONN.zcount(CONSUMER_NAME, 0, now.timestamp() - 60 * 30)
            if expired > 0:
                REDIS_CONN.zpopmin(CONSUMER_NAME, expired)
        except Exception:
            logging.exception("report_status got exception")
        time.sleep(30)

def main():
    background_thread = threading.Thread(target=report_status)
    background_thread.daemon = True
    background_thread.start()

    while True:
        handle_task()

if __name__ == "__main__":
    main()
