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
import os
import hashlib
import copy
import re
import sys
from functools import partial
from timeit import default_timer as timer

from elasticsearch_dsl import Q

from api.db.services.task_service import TaskService
from rag.settings import cron_logger, DOC_MAXIMUM_SIZE
from rag.utils import ELASTICSEARCH
from rag.utils import MINIO
from rag.utils import rmSpace, findMaxTm

from rag.nlp import search
from io import BytesIO
import pandas as pd

from rag.app import laws, paper, presentation, manual, qa

from api.db import LLMType, ParserType
from api.db.services.document_service import DocumentService
from api.db.services.llm_service import LLMBundle
from api.settings import database_logger
from api.utils.file_utils import get_project_base_directory

BATCH_SIZE = 64

FACTORY = {
    ParserType.GENERAL.value: laws,
    ParserType.PAPER.value: paper,
    ParserType.PRESENTATION.value: presentation,
    ParserType.MANUAL.value: manual,
    ParserType.LAWS.value: laws,
    ParserType.QA.value: qa,
}


def set_progress(task_id, from_page, to_page, prog=None, msg="Processing..."):
    cancel = TaskService.do_cancel(task_id)
    if cancel:
        msg += " [Canceled]"
        prog = -1

    if to_page > 0: msg = f"Page({from_page}~{to_page}): " + msg
    d = {"progress_msg": msg}
    if prog is not None: d["progress"] = prog
    try:
        TaskService.update_by_id(task_id, d)
    except Exception as e:
        cron_logger.error("set_progress:({}), {}".format(task_id, str(e)))

    if cancel:sys.exit()


"""        
def chuck_doc(name, binary, tenant_id, cvmdl=None):
    suff = os.path.split(name)[-1].lower().split(".")[-1]
    if suff.find("pdf") >= 0:
        return PDF(binary)
    if suff.find("doc") >= 0:
        return DOC(binary)
    if re.match(r"(xlsx|xlsm|xltx|xltm)", suff):
        return EXC(binary)
    if suff.find("ppt") >= 0:
        return PPT(binary)
    if cvmdl and re.search(r"\.(jpg|jpeg|png|tif|gif|pcx|tga|exif|fpx|svg|psd|cdr|pcd|dxf|ufo|eps|ai|raw|WMF|webp|avif|apng|icon|ico)$",
                     name.lower()):
        txt = cvmdl.describe(binary)
        field = TextChunker.Fields()
        field.text_chunks = [(txt, binary)]
        field.table_chunks = []
        return field

    return TextChunker()(binary)
"""


def collect(comm, mod, tm):
    tasks = TaskService.get_tasks(tm, mod, comm)
    if len(tasks) == 0:
        return pd.DataFrame()
    tasks = pd.DataFrame(tasks)
    mtm = tasks["update_time"].max()
    cron_logger.info("TOTAL:{}, To:{}".format(len(tasks), mtm))
    return tasks


def build(row, cvmdl):
    if row["size"] > DOC_MAXIMUM_SIZE:
        set_progress(row["id"], -1, "File size exceeds( <= %dMb )" %
                     (int(DOC_MAXIMUM_SIZE / 1024 / 1024)))
        return []

    callback = partial(set_progress, row["id"], row["from_page"], row["to_page"])
    chunker = FACTORY[row["parser_id"]]
    try:
        cron_logger.info("Chunkking {}/{}".format(row["location"], row["name"]))
        cks = chunker.chunk(row["name"], MINIO.get(row["kb_id"], row["location"]), row["from_page"], row["to_page"],
                            callback)
    except Exception as e:
        if re.search("(No such file|not found)", str(e)):
            callback(-1, "Can not find file <%s>" % row["doc_name"])
        else:
            callback(-1, f"Internal server error: %s" % str(e).replace("'", ""))

        cron_logger.warn("Chunkking {}/{}: {}".format(row["location"], row["name"], str(e)))

        return []

    callback(msg="Finished slicing files. Start to embedding the content.")

    docs = []
    doc = {
        "doc_id": row["doc_id"],
        "kb_id": [str(row["kb_id"])]
    }
    for ck in cks:
        d = copy.deepcopy(doc)
        d.update(ck)
        md5 = hashlib.md5()
        md5.update((ck["content_with_weight"] + str(d["doc_id"])).encode("utf-8"))
        d["_id"] = md5.hexdigest()
        d["create_time"] = str(datetime.datetime.now()).replace("T", " ")[:19]
        if not d.get("image"):
            docs.append(d)
            continue

        output_buffer = BytesIO()
        if isinstance(d["image"], bytes):
            output_buffer = BytesIO(d["image"])
        else:
            d["image"].save(output_buffer, format='JPEG')

        MINIO.put(row["kb_id"], d["_id"], output_buffer.getvalue())
        d["img_id"] = "{}-{}".format(row["kb_id"], d["_id"])
        docs.append(d)

    return docs


def init_kb(row):
    idxnm = search.index_name(row["tenant_id"])
    if ELASTICSEARCH.indexExist(idxnm):
        return
    return ELASTICSEARCH.createIdx(idxnm, json.load(
        open(os.path.join(get_project_base_directory(), "conf", "mapping.json"), "r")))


def embedding(docs, mdl):
    tts, cnts = [d["docnm_kwd"] for d in docs if d.get("docnm_kwd")], [d["content_with_weight"] for d in docs]
    tk_count = 0
    if len(tts) == len(cnts):
        tts, c = mdl.encode(tts)
        tk_count += c

    cnts, c = mdl.encode(cnts)
    tk_count += c
    vects = (0.1 * tts + 0.9 * cnts) if len(tts) == len(cnts) else cnts

    assert len(vects) == len(docs)
    for i, d in enumerate(docs):
        v = vects[i].tolist()
        d["q_%d_vec" % len(v)] = v
    return tk_count


def main(comm, mod):
    tm_fnm = os.path.join(get_project_base_directory(), "rag/res", f"{comm}-{mod}.tm")
    tm = findMaxTm(tm_fnm)
    rows = collect(comm, mod, tm)
    if len(rows) == 0:
        return

    tmf = open(tm_fnm, "a+")
    for _, r in rows.iterrows():
        try:
            embd_mdl = LLMBundle(r["tenant_id"], LLMType.EMBEDDING)
            cv_mdl = LLMBundle(r["tenant_id"], LLMType.IMAGE2TEXT)
            # TODO: sequence2text model
        except Exception as e:
            set_progress(r["id"], -1, str(e))
            continue

        callback = partial(set_progress, r["id"], r["from_page"], r["to_page"])
        st_tm = timer()
        cks = build(r, cv_mdl)
        if not cks:
            tmf.write(str(r["update_time"]) + "\n")
            continue
        # TODO: exception handler
        ## set_progress(r["did"], -1, "ERROR: ")
        try:
            tk_count = embedding(cks, embd_mdl)
        except Exception as e:
            callback(-1, "Embedding error:{}".format(str(e)))
            cron_logger.error(str(e))
            continue

        callback(msg="Finished embedding! Start to build index!")
        init_kb(r)
        chunk_count = len(set([c["_id"] for c in cks]))
        es_r = ELASTICSEARCH.bulk(cks, search.index_name(r["tenant_id"]))
        if es_r:
            callback(-1, "Index failure!")
            cron_logger.error(str(es_r))
        else:
            if TaskService.do_cancel(r["id"]):
                ELASTICSEARCH.deleteByQuery(Q("match", doc_id=r["doc_id"]), idxnm=search.index_name(r["tenant_id"]))
            callback(1., "Done!")
            DocumentService.increment_chunk_num(r["doc_id"], r["kb_id"], tk_count, chunk_count, 0)
            cron_logger.info("Chunk doc({}), token({}), chunks({})".format(r["id"], tk_count, len(cks)))

        tmf.write(str(r["update_time"]) + "\n")
    tmf.close()


if __name__ == "__main__":
    peewee_logger = logging.getLogger('peewee')
    peewee_logger.propagate = False
    peewee_logger.addHandler(database_logger.handlers[0])
    peewee_logger.setLevel(database_logger.level)

    from mpi4py import MPI

    comm = MPI.COMM_WORLD
    main(comm.Get_size(), comm.Get_rank())
