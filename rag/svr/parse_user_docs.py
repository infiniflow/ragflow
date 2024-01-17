#
#  Copyright 2019 The RAG Flow Authors. All Rights Reserved.
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
import time
import random
import re
from timeit import default_timer as timer

from rag.llm import EmbeddingModel, CvModel
from rag.settings import cron_logger, DOC_MAXIMUM_SIZE
from rag.utils import ELASTICSEARCH
from rag.utils import MINIO
from rag.utils import rmSpace, findMaxTm

from rag.nlp import huchunk, huqie, search
from io import BytesIO
import pandas as pd
from elasticsearch_dsl import Q
from PIL import Image
from rag.parser import (
    PdfParser,
    DocxParser,
    ExcelParser
)
from rag.nlp.huchunk import (
    PdfChunker,
    DocxChunker,
    ExcelChunker,
    PptChunker,
    TextChunker
)
from web_server.db import LLMType
from web_server.db.services.document_service import DocumentService
from web_server.db.services.llm_service import TenantLLMService
from web_server.settings import database_logger
from web_server.utils import get_format_time
from web_server.utils.file_utils import get_project_base_directory

BATCH_SIZE = 64

PDF = PdfChunker(PdfParser())
DOC = DocxChunker(DocxParser())
EXC = ExcelChunker(ExcelParser())
PPT = PptChunker()


def chuck_doc(name, binary, cvmdl=None):
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

    return TextChunker()(binary)


def collect(comm, mod, tm):
    docs = DocumentService.get_newly_uploaded(tm, mod, comm)
    if len(docs) == 0:
        return pd.DataFrame()
    docs = pd.DataFrame(docs)
    mtm = docs["update_time"].max()
    cron_logger.info("TOTAL:{}, To:{}".format(len(docs), mtm))
    return docs


def set_progress(docid, prog, msg="Processing...", begin=False):
    d = {"progress": prog, "progress_msg": msg}
    if begin:
        d["process_begin_at"] = get_format_time()
    try:
        DocumentService.update_by_id(
            docid, {"progress": prog, "progress_msg": msg})
    except Exception as e:
        cron_logger.error("set_progress:({}), {}".format(docid, str(e)))


def build(row, cvmdl):
    if row["size"] > DOC_MAXIMUM_SIZE:
        set_progress(row["id"], -1, "File size exceeds( <= %dMb )" %
                     (int(DOC_MAXIMUM_SIZE / 1024 / 1024)))
        return []

    # res = ELASTICSEARCH.search(Q("term", doc_id=row["id"]))
    # if ELASTICSEARCH.getTotal(res) > 0:
    #     ELASTICSEARCH.updateScriptByQuery(Q("term", doc_id=row["id"]),
    #                                       scripts="""
    #                            if(!ctx._source.kb_id.contains('%s'))
    #                              ctx._source.kb_id.add('%s');
    #                            """ % (str(row["kb_id"]), str(row["kb_id"])),
    #         idxnm=search.index_name(row["tenant_id"])
    #     )
    #     set_progress(row["id"], 1, "Done")
    #     return []

    random.seed(time.time())
    set_progress(row["id"], random.randint(0, 20) /
                 100., "Finished preparing! Start to slice file!", True)
    try:
        cron_logger.info("Chunkking {}/{}".format(row["location"], row["name"]))
        obj = chuck_doc(row["name"], MINIO.get(row["kb_id"], row["location"]), cvmdl)
    except Exception as e:
        if re.search("(No such file|not found)", str(e)):
            set_progress(
                row["id"], -1, "Can not find file <%s>" %
                row["doc_name"])
        else:
            set_progress(
                row["id"], -1, f"Internal server error: %s" %
                str(e).replace(
                    "'", ""))

        cron_logger.warn("Chunkking {}/{}: {}".format(row["location"], row["name"], str(e)))

        return []

    if not obj.text_chunks and not obj.table_chunks:
        set_progress(
            row["id"],
            1,
            "Nothing added! Mostly, file type unsupported yet.")
        return []

    set_progress(row["id"], random.randint(20, 60) / 100.,
                 "Finished slicing files. Start to embedding the content.")

    doc = {
        "doc_id": row["id"],
        "kb_id": [str(row["kb_id"])],
        "docnm_kwd": os.path.split(row["location"])[-1],
        "title_tks": huqie.qie(row["name"])
    }
    doc["title_sm_tks"] = huqie.qieqie(doc["title_tks"])
    output_buffer = BytesIO()
    docs = []
    md5 = hashlib.md5()
    for txt, img in obj.text_chunks:
        d = copy.deepcopy(doc)
        md5.update((txt + str(d["doc_id"])).encode("utf-8"))
        d["_id"] = md5.hexdigest()
        d["content_ltks"] = huqie.qie(txt)
        d["content_sm_ltks"] = huqie.qieqie(d["content_ltks"])
        if not img:
            docs.append(d)
            continue

        if isinstance(img, bytes):
            output_buffer = BytesIO(img)
        else:
            img.save(output_buffer, format='JPEG')

        MINIO.put(row["kb_id"], d["_id"], output_buffer.getvalue())
        d["img_id"] = "{}-{}".format(row["kb_id"], d["_id"])
        d["create_time"] = str(datetime.datetime.now()).replace("T", " ")[:19]
        docs.append(d)

    for arr, img in obj.table_chunks:
        for i, txt in enumerate(arr):
            d = copy.deepcopy(doc)
            d["content_ltks"] = huqie.qie(txt)
            md5.update((txt + str(d["doc_id"])).encode("utf-8"))
            d["_id"] = md5.hexdigest()
            if not img:
                docs.append(d)
                continue
            img.save(output_buffer, format='JPEG')
            MINIO.put(row["kb_id"], d["_id"], output_buffer.getvalue())
            d["img_id"] = "{}-{}".format(row["kb_id"], d["_id"])
            d["create_time"] = str(datetime.datetime.now()).replace("T", " ")[:19]
            docs.append(d)
    set_progress(row["id"], random.randint(60, 70) /
                 100., "Continue embedding the content.")

    return docs


def init_kb(row):
    idxnm = search.index_name(row["tenant_id"])
    if ELASTICSEARCH.indexExist(idxnm):
        return
    return ELASTICSEARCH.createIdx(idxnm, json.load(
        open(os.path.join(get_project_base_directory(), "conf", "mapping.json"), "r")))


def embedding(docs, mdl):
    tts, cnts = [rmSpace(d["title_tks"]) for d in docs], [rmSpace(d["content_ltks"]) for d in docs]
    tk_count = 0
    tts, c = mdl.encode(tts)
    tk_count += c
    cnts, c = mdl.encode(cnts)
    tk_count += c
    vects = 0.1 * tts + 0.9 * cnts
    assert len(vects) == len(docs)
    for i, d in enumerate(docs):
        v = vects[i].tolist()
        d["q_%d_vec"%len(v)] = v
    return tk_count


def main(comm, mod):
    global model
    from rag.llm import HuEmbedding
    model = HuEmbedding()
    tm_fnm = os.path.join(get_project_base_directory(), "rag/res", f"{comm}-{mod}.tm")
    tm = findMaxTm(tm_fnm)
    rows = collect(comm, mod, tm)
    if len(rows) == 0:
        return

    tmf = open(tm_fnm, "a+")
    for _, r in rows.iterrows():
        embd_mdl = TenantLLMService.model_instance(r["tenant_id"], LLMType.EMBEDDING)
        if not embd_mdl:
            set_progress(r["id"], -1, "Can't find embedding model!")
            cron_logger.error("Tenant({}) can't find embedding model!".format(r["tenant_id"]))
            continue
        cv_mdl = TenantLLMService.model_instance(r["tenant_id"], LLMType.IMAGE2TEXT)
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
            set_progress(r["id"], -1, "Embedding error:{}".format(str(e)))
            cron_logger.error(str(e))
            continue


        set_progress(r["id"], random.randint(70, 95) / 100.,
                     "Finished embedding! Start to build index!")
        init_kb(r)
        es_r = ELASTICSEARCH.bulk(cks, search.index_name(r["tenant_id"]))
        if es_r:
            set_progress(r["id"], -1, "Index failure!")
            cron_logger.error(str(es_r))
        else:
            set_progress(r["id"], 1., "Done!")
            DocumentService.increment_chunk_num(r["id"], r["kb_id"], tk_count, len(cks), timer()-st_tm)
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
