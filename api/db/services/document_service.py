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
import hashlib
import json
import os
import random
import re
import traceback
from concurrent.futures import ThreadPoolExecutor
from copy import deepcopy
from datetime import datetime
from io import BytesIO

from elasticsearch_dsl import Q
from peewee import fn

from api.db.db_utils import bulk_insert_into_db
from api.settings import stat_logger
from api.utils import current_timestamp, get_format_time, get_uuid
from api.utils.file_utils import get_project_base_directory
from graphrag.mind_map_extractor import MindMapExtractor
from rag.settings import SVR_QUEUE_NAME
from rag.utils.es_conn import ELASTICSEARCH
from rag.utils.storage_factory import STORAGE_IMPL
from rag.nlp import search, rag_tokenizer

from api.db import FileType, TaskStatus, ParserType, LLMType
from api.db.db_models import DB, Knowledgebase, Tenant, Task
from api.db.db_models import Document
from api.db.services.common_service import CommonService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db import StatusEnum
from rag.utils.redis_conn import REDIS_CONN


class DocumentService(CommonService):
    model = Document

    @classmethod
    @DB.connection_context()
    def get_list(cls, kb_id, page_number, items_per_page,
                     orderby, desc, keywords, id):
        docs =cls.model.select().where(cls.model.kb_id==kb_id)
        if id:
            docs = docs.where(
                cls.model.id== id )
        if keywords:
            docs = docs.where(
                fn.LOWER(cls.model.name).contains(keywords.lower())
            )
        count = docs.count()
        if desc:
            docs = docs.order_by(cls.model.getter_by(orderby).desc())
        else:
            docs = docs.order_by(cls.model.getter_by(orderby).asc())

        docs = docs.paginate(page_number, items_per_page)

        return list(docs.dicts()), count


    @classmethod
    @DB.connection_context()
    def get_by_kb_id(cls, kb_id, page_number, items_per_page,
                     orderby, desc, keywords):
        if keywords:
            docs = cls.model.select().where(
                (cls.model.kb_id == kb_id),
                (fn.LOWER(cls.model.name).contains(keywords.lower()))
            )
        else:
            docs = cls.model.select().where(cls.model.kb_id == kb_id)
        count = docs.count()
        if desc:
            docs = docs.order_by(cls.model.getter_by(orderby).desc())
        else:
            docs = docs.order_by(cls.model.getter_by(orderby).asc())

        docs = docs.paginate(page_number, items_per_page)

        return list(docs.dicts()), count

    @classmethod
    @DB.connection_context()
    def list_documents_in_dataset(cls, dataset_id, offset, count, order_by, descend, keywords):
        if keywords:
            docs = cls.model.select().where(
                (cls.model.kb_id == dataset_id),
                (fn.LOWER(cls.model.name).contains(keywords.lower()))
            )
        else:
            docs = cls.model.select().where(cls.model.kb_id == dataset_id)

        total = docs.count()

        if descend == 'True':
            docs = docs.order_by(cls.model.getter_by(order_by).desc())
        if descend == 'False':
            docs = docs.order_by(cls.model.getter_by(order_by).asc())

        docs = list(docs.dicts())
        docs_length = len(docs)

        if offset < 0 or offset > docs_length:
            raise IndexError("Offset is out of the valid range.")

        if count == -1:
            return docs[offset:], total

        return docs[offset:offset + count], total

    @classmethod
    @DB.connection_context()
    def insert(cls, doc):
        if not cls.save(**doc):
            raise RuntimeError("Database error (Document)!")
        e, doc = cls.get_by_id(doc["id"])
        if not e:
            raise RuntimeError("Database error (Document retrieval)!")
        e, kb = KnowledgebaseService.get_by_id(doc.kb_id)
        if not KnowledgebaseService.update_by_id(
                kb.id, {"doc_num": kb.doc_num + 1}):
            raise RuntimeError("Database error (Knowledgebase)!")
        return doc

    @classmethod
    @DB.connection_context()
    def remove_document(cls, doc, tenant_id):
        ELASTICSEARCH.deleteByQuery(
                Q("match", doc_id=doc.id), idxnm=search.index_name(tenant_id))
        cls.clear_chunk_num(doc.id)
        return cls.delete_by_id(doc.id)

    @classmethod
    @DB.connection_context()
    def get_newly_uploaded(cls):
        fields = [
            cls.model.id,
            cls.model.kb_id,
            cls.model.parser_id,
            cls.model.parser_config,
            cls.model.name,
            cls.model.type,
            cls.model.location,
            cls.model.size,
            Knowledgebase.tenant_id,
            Tenant.embd_id,
            Tenant.img2txt_id,
            Tenant.asr_id,
            cls.model.update_time]
        docs = cls.model.select(*fields) \
            .join(Knowledgebase, on=(cls.model.kb_id == Knowledgebase.id)) \
            .join(Tenant, on=(Knowledgebase.tenant_id == Tenant.id))\
            .where(
                cls.model.status == StatusEnum.VALID.value,
                ~(cls.model.type == FileType.VIRTUAL.value),
                cls.model.progress == 0,
                cls.model.update_time >= current_timestamp() - 1000 * 600,
                cls.model.run == TaskStatus.RUNNING.value)\
            .order_by(cls.model.update_time.asc())
        return list(docs.dicts())

    @classmethod
    @DB.connection_context()
    def get_unfinished_docs(cls):
        fields = [cls.model.id, cls.model.process_begin_at, cls.model.parser_config, cls.model.progress_msg, cls.model.run]
        docs = cls.model.select(*fields) \
            .where(
                cls.model.status == StatusEnum.VALID.value,
                ~(cls.model.type == FileType.VIRTUAL.value),
                cls.model.progress < 1,
                cls.model.progress > 0)
        return list(docs.dicts())

    @classmethod
    @DB.connection_context()
    def increment_chunk_num(cls, doc_id, kb_id, token_num, chunk_num, duation):
        num = cls.model.update(token_num=cls.model.token_num + token_num,
                               chunk_num=cls.model.chunk_num + chunk_num,
                               process_duation=cls.model.process_duation + duation).where(
            cls.model.id == doc_id).execute()
        if num == 0:
            raise LookupError(
                "Document not found which is supposed to be there")
        num = Knowledgebase.update(
            token_num=Knowledgebase.token_num +
            token_num,
            chunk_num=Knowledgebase.chunk_num +
            chunk_num).where(
            Knowledgebase.id == kb_id).execute()
        return num
    
    @classmethod
    @DB.connection_context()
    def decrement_chunk_num(cls, doc_id, kb_id, token_num, chunk_num, duation):
        num = cls.model.update(token_num=cls.model.token_num - token_num,
                               chunk_num=cls.model.chunk_num - chunk_num,
                               process_duation=cls.model.process_duation + duation).where(
            cls.model.id == doc_id).execute()
        if num == 0:
            raise LookupError(
                "Document not found which is supposed to be there")
        num = Knowledgebase.update(
            token_num=Knowledgebase.token_num -
            token_num,
            chunk_num=Knowledgebase.chunk_num -
            chunk_num
        ).where(
            Knowledgebase.id == kb_id).execute()
        return num
    
    @classmethod
    @DB.connection_context()
    def clear_chunk_num(cls, doc_id):
        doc = cls.model.get_by_id(doc_id)
        assert doc, "Can't fine document in database."

        num = Knowledgebase.update(
            token_num=Knowledgebase.token_num -
            doc.token_num,
            chunk_num=Knowledgebase.chunk_num -
            doc.chunk_num,
            doc_num=Knowledgebase.doc_num-1
        ).where(
            Knowledgebase.id == doc.kb_id).execute()
        return num

    @classmethod
    @DB.connection_context()
    def get_tenant_id(cls, doc_id):
        docs = cls.model.select(
            Knowledgebase.tenant_id).join(
            Knowledgebase, on=(
                Knowledgebase.id == cls.model.kb_id)).where(
                cls.model.id == doc_id, Knowledgebase.status == StatusEnum.VALID.value)
        docs = docs.dicts()
        if not docs:
            return
        return docs[0]["tenant_id"]

    @classmethod
    @DB.connection_context()
    def get_tenant_id_by_name(cls, name):
        docs = cls.model.select(
            Knowledgebase.tenant_id).join(
            Knowledgebase, on=(
                    Knowledgebase.id == cls.model.kb_id)).where(
            cls.model.name == name, Knowledgebase.status == StatusEnum.VALID.value)
        docs = docs.dicts()
        if not docs:
            return
        return docs[0]["tenant_id"]

    @classmethod
    @DB.connection_context()
    def get_embd_id(cls, doc_id):
        docs = cls.model.select(
            Knowledgebase.embd_id).join(
            Knowledgebase, on=(
                Knowledgebase.id == cls.model.kb_id)).where(
                cls.model.id == doc_id, Knowledgebase.status == StatusEnum.VALID.value)
        docs = docs.dicts()
        if not docs:
            return
        return docs[0]["embd_id"]
    
    @classmethod
    @DB.connection_context()
    def get_doc_id_by_doc_name(cls, doc_name):
        fields = [cls.model.id]
        doc_id = cls.model.select(*fields) \
            .where(cls.model.name == doc_name)
        doc_id = doc_id.dicts()
        if not doc_id:
            return
        return doc_id[0]["id"]

    @classmethod
    @DB.connection_context()
    def get_thumbnails(cls, docids):
        fields = [cls.model.id, cls.model.thumbnail]
        return list(cls.model.select(
            *fields).where(cls.model.id.in_(docids)).dicts())

    @classmethod
    @DB.connection_context()
    def update_parser_config(cls, id, config):
        e, d = cls.get_by_id(id)
        if not e:
            raise LookupError(f"Document({id}) not found.")

        def dfs_update(old, new):
            for k, v in new.items():
                if k not in old:
                    old[k] = v
                    continue
                if isinstance(v, dict):
                    assert isinstance(old[k], dict)
                    dfs_update(old[k], v)
                else:
                    old[k] = v
        dfs_update(d.parser_config, config)
        cls.update_by_id(id, {"parser_config": d.parser_config})

    @classmethod
    @DB.connection_context()
    def get_doc_count(cls, tenant_id):
        docs = cls.model.select(cls.model.id).join(Knowledgebase,
                                                   on=(Knowledgebase.id == cls.model.kb_id)).where(
            Knowledgebase.tenant_id == tenant_id)
        return len(docs)

    @classmethod
    @DB.connection_context()
    def begin2parse(cls, docid):
        cls.update_by_id(
            docid, {"progress": random.random() * 1 / 100.,
                    "progress_msg": "Task dispatched...",
                    "process_begin_at": get_format_time()
                    })

    @classmethod
    @DB.connection_context()
    def update_progress(cls):
        docs = cls.get_unfinished_docs()
        for d in docs:
            try:
                tsks = Task.query(doc_id=d["id"], order_by=Task.create_time)
                if not tsks:
                    continue
                msg = []
                prg = 0
                finished = True
                bad = 0
                e, doc = DocumentService.get_by_id(d["id"])
                status = doc.run#TaskStatus.RUNNING.value
                for t in tsks:
                    if 0 <= t.progress < 1:
                        finished = False
                    prg += t.progress if t.progress >= 0 else 0
                    if t.progress_msg not in msg:
                        msg.append(t.progress_msg)
                    if t.progress == -1:
                        bad += 1
                prg /= len(tsks)
                if finished and bad:
                    prg = -1
                    status = TaskStatus.FAIL.value
                elif finished:
                    if d["parser_config"].get("raptor", {}).get("use_raptor") and d["progress_msg"].lower().find(" raptor")<0:
                        queue_raptor_tasks(d)
                        prg *= 0.98
                        msg.append("------ RAPTOR -------")
                    else:
                        status = TaskStatus.DONE.value

                msg = "\n".join(msg)
                info = {
                    "process_duation": datetime.timestamp(
                        datetime.now()) -
                                       d["process_begin_at"].timestamp(),
                    "run": status}
                if prg != 0:
                    info["progress"] = prg
                if msg:
                    info["progress_msg"] = msg
                cls.update_by_id(d["id"], info)
            except Exception as e:
                stat_logger.error("fetch task exception:" + str(e))

    @classmethod
    @DB.connection_context()
    def get_kb_doc_count(cls, kb_id):
        return len(cls.model.select(cls.model.id).where(
            cls.model.kb_id == kb_id).dicts())


    @classmethod
    @DB.connection_context()
    def do_cancel(cls, doc_id):
        try:
            _, doc = DocumentService.get_by_id(doc_id)
            return doc.run == TaskStatus.CANCEL.value or doc.progress < 0
        except Exception as e:
            pass
        return False


def queue_raptor_tasks(doc):
    def new_task():
        nonlocal doc
        return {
            "id": get_uuid(),
            "doc_id": doc["id"],
            "from_page": 0,
            "to_page": -1,
            "progress_msg": "Start to do RAPTOR (Recursive Abstractive Processing For Tree-Organized Retrieval)."
        }

    task = new_task()
    bulk_insert_into_db(Task, [task], True)
    task["type"] = "raptor"
    assert REDIS_CONN.queue_product(SVR_QUEUE_NAME, message=task), "Can't access Redis. Please check the Redis' status."


def doc_upload_and_parse(conversation_id, file_objs, user_id):
    from rag.app import presentation, picture, naive, audio, email
    from api.db.services.dialog_service import ConversationService, DialogService
    from api.db.services.file_service import FileService
    from api.db.services.llm_service import LLMBundle
    from api.db.services.user_service import TenantService
    from api.db.services.api_service import API4ConversationService

    e, conv = ConversationService.get_by_id(conversation_id)
    if not e:
        e, conv = API4ConversationService.get_by_id(conversation_id)
    assert e, "Conversation not found!"

    e, dia = DialogService.get_by_id(conv.dialog_id)
    kb_id = dia.kb_ids[0]
    e, kb = KnowledgebaseService.get_by_id(kb_id)
    if not e:
        raise LookupError("Can't find this knowledgebase!")

    idxnm = search.index_name(kb.tenant_id)
    if not ELASTICSEARCH.indexExist(idxnm):
        ELASTICSEARCH.createIdx(idxnm, json.load(
            open(os.path.join(get_project_base_directory(), "conf", "mapping.json"), "r")))

    embd_mdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING, llm_name=kb.embd_id, lang=kb.language)

    err, files = FileService.upload_document(kb, file_objs, user_id)
    assert not err, "\n".join(err)

    def dummy(prog=None, msg=""):
        pass

    FACTORY = {
        ParserType.PRESENTATION.value: presentation,
        ParserType.PICTURE.value: picture,
        ParserType.AUDIO.value: audio,
        ParserType.EMAIL.value: email
    }
    parser_config = {"chunk_token_num": 4096, "delimiter": "\n!?;。；！？", "layout_recognize": False}
    exe = ThreadPoolExecutor(max_workers=12)
    threads = []
    doc_nm = {}
    for d, blob in files:
        doc_nm[d["id"]] = d["name"]
    for d, blob in files:
        kwargs = {
            "callback": dummy,
            "parser_config": parser_config,
            "from_page": 0,
            "to_page": 100000,
            "tenant_id": kb.tenant_id,
            "lang": kb.language
        }
        threads.append(exe.submit(FACTORY.get(d["parser_id"], naive).chunk, d["name"], blob, **kwargs))

    for (docinfo, _), th in zip(files, threads):
        docs = []
        doc = {
            "doc_id": docinfo["id"],
            "kb_id": [kb.id]
        }
        for ck in th.result():
            d = deepcopy(doc)
            d.update(ck)
            md5 = hashlib.md5()
            md5.update((ck["content_with_weight"] +
                        str(d["doc_id"])).encode("utf-8"))
            d["_id"] = md5.hexdigest()
            d["create_time"] = str(datetime.now()).replace("T", " ")[:19]
            d["create_timestamp_flt"] = datetime.now().timestamp()
            if not d.get("image"):
                docs.append(d)
                continue

            output_buffer = BytesIO()
            if isinstance(d["image"], bytes):
                output_buffer = BytesIO(d["image"])
            else:
                d["image"].save(output_buffer, format='JPEG')

            STORAGE_IMPL.put(kb.id, d["_id"], output_buffer.getvalue())
            d["img_id"] = "{}-{}".format(kb.id, d["_id"])
            del d["image"]
            docs.append(d)

    parser_ids = {d["id"]: d["parser_id"] for d, _ in files}
    docids = [d["id"] for d, _ in files]
    chunk_counts = {id: 0 for id in docids}
    token_counts = {id: 0 for id in docids}
    es_bulk_size = 64

    def embedding(doc_id, cnts, batch_size=16):
        nonlocal embd_mdl, chunk_counts, token_counts
        vects = []
        for i in range(0, len(cnts), batch_size):
            vts, c = embd_mdl.encode(cnts[i: i + batch_size])
            vects.extend(vts.tolist())
            chunk_counts[doc_id] += len(cnts[i:i + batch_size])
            token_counts[doc_id] += c
        return vects

    _, tenant = TenantService.get_by_id(kb.tenant_id)
    llm_bdl = LLMBundle(kb.tenant_id, LLMType.CHAT, tenant.llm_id)
    for doc_id in docids:
        cks = [c for c in docs if c["doc_id"] == doc_id]

        if parser_ids[doc_id] != ParserType.PICTURE.value:
            mindmap = MindMapExtractor(llm_bdl)
            try:
                mind_map = json.dumps(mindmap([c["content_with_weight"] for c in docs if c["doc_id"] == doc_id]).output,
                                      ensure_ascii=False, indent=2)
                if len(mind_map) < 32: raise Exception("Few content: " + mind_map)
                cks.append({
                    "id": get_uuid(),
                    "doc_id": doc_id,
                    "kb_id": [kb.id],
                    "docnm_kwd": doc_nm[doc_id],
                    "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", doc_nm[doc_id])),
                    "content_ltks": "",
                    "content_with_weight": mind_map,
                    "knowledge_graph_kwd": "mind_map"
                })
            except Exception as e:
                stat_logger.error("Mind map generation error:", traceback.format_exc())

        vects = embedding(doc_id, [c["content_with_weight"] for c in cks])
        assert len(cks) == len(vects)
        for i, d in enumerate(cks):
            v = vects[i]
            d["q_%d_vec" % len(v)] = v
        for b in range(0, len(cks), es_bulk_size):
            ELASTICSEARCH.bulk(cks[b:b + es_bulk_size], idxnm)

        DocumentService.increment_chunk_num(
            doc_id, kb.id, token_counts[doc_id], chunk_counts[doc_id], 0)

    return [d["id"] for d,_ in files]