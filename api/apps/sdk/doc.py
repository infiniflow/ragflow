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
import pathlib
import datetime

from api.db.services.dialog_service import keyword_extraction
from rag.app.qa import rmPrefix, beAdoc
from rag.nlp import rag_tokenizer
from api.db import LLMType, ParserType
from api.db.services.llm_service import TenantLLMService
from api.settings import kg_retrievaler
import hashlib
import re
from api.utils.api_utils import token_required
from api.db.db_models import Task
from api.db.services.task_service import TaskService, queue_tasks
from api.utils.api_utils import server_error_response
from api.utils.api_utils import get_result, get_error_data_result
from io import BytesIO
from elasticsearch_dsl import Q
from flask import request, send_file
from api.db import FileSource, TaskStatus, FileType
from api.db.db_models import File
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.settings import RetCode, retrievaler
from api.utils.api_utils import construct_json_result,get_parser_config
from rag.nlp import search
from rag.utils import rmSpace
from rag.utils.es_conn import ELASTICSEARCH
from rag.utils.storage_factory import STORAGE_IMPL
import os

MAXIMUM_OF_UPLOADING_FILES = 256

MAXIMUM_OF_UPLOADING_FILES = 256

MAXIMUM_OF_UPLOADING_FILES = 256

MAXIMUM_OF_UPLOADING_FILES = 256


@manager.route('/dataset/<dataset_id>/document', methods=['POST'])
@token_required
def upload(dataset_id, tenant_id):
    if 'file' not in request.files:
        return get_error_data_result(
            retmsg='No file part!', retcode=RetCode.ARGUMENT_ERROR)
    file_objs = request.files.getlist('file')
    for file_obj in file_objs:
        if file_obj.filename == '':
            return get_result(
                retmsg='No file selected!', retcode=RetCode.ARGUMENT_ERROR)
    # total size
    total_size = 0
    for file_obj in file_objs:
        file_obj.seek(0, os.SEEK_END)
        total_size += file_obj.tell()
        file_obj.seek(0)
    MAX_TOTAL_FILE_SIZE=10*1024*1024
    if total_size > MAX_TOTAL_FILE_SIZE:
        return get_result(
            retmsg=f'Total file size exceeds 10MB limit! ({total_size / (1024 * 1024):.2f} MB)',
            retcode=RetCode.ARGUMENT_ERROR)
    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        raise LookupError(f"Can't find the dataset with ID {dataset_id}!")
    err, files= FileService.upload_document(kb, file_objs, tenant_id)
    if err:
        return get_result(
            retmsg="\n".join(err), retcode=RetCode.SERVER_ERROR)
    # rename key's name
    renamed_doc_list = []
    for file in files:
        doc = file[0]
        key_mapping = {
            "chunk_num": "chunk_count",
            "kb_id": "dataset_id",
            "token_num": "token_count",
            "parser_id": "chunk_method"
        }
        renamed_doc = {}
        for key, value in doc.items():
            new_key = key_mapping.get(key, key)
            renamed_doc[new_key] = value
        renamed_doc["run"] = "UNSTART"
        renamed_doc_list.append(renamed_doc)
    return get_result(data=renamed_doc_list)


@manager.route('/dataset/<dataset_id>/info/<document_id>', methods=['PUT'])
@token_required
def update_doc(tenant_id, dataset_id, document_id):
    req = request.json
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg="You don't own the dataset.")
    doc = DocumentService.query(kb_id=dataset_id, id=document_id)
    if not doc:
        return get_error_data_result(retmsg="The dataset doesn't own the document.")
    doc = doc[0]
    if "chunk_count" in req:
        if req["chunk_count"] != doc.chunk_num:
            return get_error_data_result(retmsg="Can't change `chunk_count`.")
    if "token_count" in req:
        if req["token_count"] != doc.token_num:
            return get_error_data_result(retmsg="Can't change `token_count`.")
    if "progress" in req:
        if req['progress'] != doc.progress:
            return get_error_data_result(retmsg="Can't change `progress`.")

    if "name" in req and req["name"] != doc.name:
        if pathlib.Path(req["name"].lower()).suffix != pathlib.Path(doc.name.lower()).suffix:
            return get_result(retmsg="The extension of file can't be changed", retcode=RetCode.ARGUMENT_ERROR)
        for d in DocumentService.query(name=req["name"], kb_id=doc.kb_id):
            if d.name == req["name"]:
                return get_error_data_result(
                    retmsg="Duplicated document name in the same dataset.")
        if not DocumentService.update_by_id(
                document_id, {"name": req["name"]}):
            return get_error_data_result(
                retmsg="Database error (Document rename)!")

        informs = File2DocumentService.get_by_document_id(document_id)
        if informs:
            e, file = FileService.get_by_id(informs[0].file_id)
            FileService.update_by_id(file.id, {"name": req["name"]})
    if "parser_config" in req:
        DocumentService.update_parser_config(doc.id, req["parser_config"])
    if "chunk_method" in req:
        valid_chunk_method = {"naive","manual","qa","table","paper","book","laws","presentation","picture","one","knowledge_graph","email"}
        if req.get("chunk_method") not in valid_chunk_method:
            return get_error_data_result(f"`chunk_method` {req['chunk_method']} doesn't exist")
        if doc.parser_id.lower() == req["chunk_method"].lower():
                return get_result()

        if doc.type == FileType.VISUAL or re.search(
                r"\.(ppt|pptx|pages)$", doc.name):
            return get_error_data_result(retmsg="Not supported yet!")

        e = DocumentService.update_by_id(doc.id,
                                         {"parser_id": req["chunk_method"], "progress": 0, "progress_msg": "",
                                          "run": TaskStatus.UNSTART.value})
        if not e:
            return get_error_data_result(retmsg="Document not found!")
        req["parser_config"] = get_parser_config(req["chunk_method"], req.get("parser_config"))
        if doc.token_num > 0:
            e = DocumentService.increment_chunk_num(doc.id, doc.kb_id, doc.token_num * -1, doc.chunk_num * -1,
                                                    doc.process_duation * -1)
            if not e:
                return get_error_data_result(retmsg="Document not found!")
            ELASTICSEARCH.deleteByQuery(
                Q("match", doc_id=doc.id), idxnm=search.index_name(tenant_id))

    return get_result()


@manager.route('/dataset/<dataset_id>/document/<document_id>', methods=['GET'])
@token_required
def download(tenant_id, dataset_id, document_id):
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg=f'You do not own the dataset {dataset_id}.')
    doc = DocumentService.query(kb_id=dataset_id, id=document_id)
    if not doc:
        return get_error_data_result(retmsg=f'The dataset not own the document {document_id}.')
    # The process of downloading
    doc_id, doc_location = File2DocumentService.get_storage_address(doc_id=document_id)  # minio address
    file_stream = STORAGE_IMPL.get(doc_id, doc_location)
    if not file_stream:
        return construct_json_result(message="This file is empty.", code=RetCode.DATA_ERROR)
    file = BytesIO(file_stream)
    # Use send_file with a proper filename and MIME type
    return send_file(
        file,
        as_attachment=True,
        download_name=doc[0].name,
        mimetype='application/octet-stream'  # Set a default MIME type
    )


@manager.route('/dataset/<dataset_id>/info', methods=['GET'])
@token_required
def list_docs(dataset_id, tenant_id):
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg=f"You don't own the dataset {dataset_id}. ")
    id = request.args.get("id")
    if not DocumentService.query(id=id,kb_id=dataset_id):
        return get_error_data_result(retmsg=f"You don't own the document {id}.")
    offset = int(request.args.get("offset", 1))
    keywords = request.args.get("keywords","")
    limit = int(request.args.get("limit", 1024))
    orderby = request.args.get("orderby", "create_time")
    if request.args.get("desc") == "False":
        desc = False
    else:
        desc = True
    docs, tol = DocumentService.get_list(dataset_id, offset, limit, orderby, desc, keywords, id)

    # rename key's name
    renamed_doc_list = []
    for doc in docs:
        key_mapping = {
            "chunk_num": "chunk_count",
            "kb_id": "dataset_id",
            "token_num": "token_count",
            "parser_id": "chunk_method"
        }
        run_mapping = {
         "0" :"UNSTART",
         "1":"RUNNING",
         "2":"CANCEL",
         "3":"DONE",
         "4":"FAIL"
        }
        renamed_doc = {}
        for key, value in doc.items():
            if key =="run":
                renamed_doc["run"]=run_mapping.get(str(value))
            new_key = key_mapping.get(key, key)
            renamed_doc[new_key] = value
        renamed_doc_list.append(renamed_doc)
    return get_result(data={"total": tol, "docs": renamed_doc_list})


@manager.route('/dataset/<dataset_id>/document', methods=['DELETE'])
@token_required
def delete(tenant_id,dataset_id):
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg=f"You don't own the dataset {dataset_id}. ")
    req = request.json
    if not req:
        doc_ids=None
    else:
        doc_ids=req.get("ids")
    if not doc_ids:
        doc_list = []
        docs=DocumentService.query(kb_id=dataset_id)
        for doc in docs:
            doc_list.append(doc.id)
    else:
        doc_list=doc_ids
    root_folder = FileService.get_root_folder(tenant_id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, tenant_id)
    errors = ""
    for doc_id in doc_list:
        try:
            e, doc = DocumentService.get_by_id(doc_id)
            if not e:
                return get_error_data_result(retmsg="Document not found!")
            tenant_id = DocumentService.get_tenant_id(doc_id)
            if not tenant_id:
                return get_error_data_result(retmsg="Tenant not found!")

            b, n = File2DocumentService.get_storage_address(doc_id=doc_id)

            if not DocumentService.remove_document(doc, tenant_id):
                return get_error_data_result(
                    retmsg="Database error (Document removal)!")

            f2d = File2DocumentService.get_by_document_id(doc_id)
            FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
            File2DocumentService.delete_by_document_id(doc_id)

            STORAGE_IMPL.rm(b, n)
        except Exception as e:
            errors += str(e)

    if errors:
        return get_result(retmsg=errors, retcode=RetCode.SERVER_ERROR)

    return get_result()


@manager.route('/dataset/<dataset_id>/chunk', methods=['POST'])
@token_required
def parse(tenant_id,dataset_id):
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg=f"You don't own the dataset {dataset_id}.")
    req = request.json
    if not req.get("document_ids"):
        return get_error_data_result("`document_ids` is required")
    for id in req["document_ids"]:
        doc = DocumentService.query(id=id,kb_id=dataset_id)
        if not doc:
            return get_error_data_result(retmsg=f"You don't own the document {id}.")
        if doc[0].progress != 0.0:
            return get_error_data_result("Can't stop parsing document with progress at 0 or 100")
        info = {"run": "1", "progress": 0}
        info["progress_msg"] = ""
        info["chunk_num"] = 0
        info["token_num"] = 0
        DocumentService.update_by_id(id, info)
        # if str(req["run"]) == TaskStatus.CANCEL.value:
        ELASTICSEARCH.deleteByQuery(
            Q("match", doc_id=id), idxnm=search.index_name(tenant_id))
        TaskService.filter_delete([Task.doc_id == id])
        e, doc = DocumentService.get_by_id(id)
        doc = doc.to_dict()
        doc["tenant_id"] = tenant_id
        bucket, name = File2DocumentService.get_storage_address(doc_id=doc["id"])
        queue_tasks(doc, bucket, name)
    return get_result()

@manager.route('/dataset/<dataset_id>/chunk', methods=['DELETE'])
@token_required
def stop_parsing(tenant_id,dataset_id):
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg=f"You don't own the dataset {dataset_id}.")
    req = request.json
    if not req.get("document_ids"):
        return get_error_data_result("`document_ids` is required")
    for id in req["document_ids"]:
        doc = DocumentService.query(id=id, kb_id=dataset_id)
        if not doc:
            return get_error_data_result(retmsg=f"You don't own the document {id}.")
        if doc[0].progress == 100.0 or doc[0].progress == 0.0:
            return get_error_data_result("Can't stop parsing document with progress at 0 or 100")
        info = {"run": "2", "progress": 0}
        DocumentService.update_by_id(id, info)
        # if str(req["run"]) == TaskStatus.CANCEL.value:
        tenant_id = DocumentService.get_tenant_id(id)
        ELASTICSEARCH.deleteByQuery(
            Q("match", doc_id=id), idxnm=search.index_name(tenant_id))
    return get_result()


@manager.route('/dataset/<dataset_id>/document/<document_id>/chunk', methods=['GET'])
@token_required
def list_chunks(tenant_id,dataset_id,document_id):
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg=f"You don't own the dataset {dataset_id}.")
    doc=DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(retmsg=f"You don't own the document {document_id}.")
    doc=doc[0]
    req = request.args
    doc_id = document_id
    page = int(req.get("offset", 1))
    size = int(req.get("limit", 30))
    question = req.get("keywords", "")
    query = {
        "doc_ids": [doc_id], "page": page, "size": size, "question": question, "sort": True
    }
    sres = retrievaler.search(query, search.index_name(tenant_id), highlight=True)
    key_mapping = {
        "chunk_num": "chunk_count",
        "kb_id": "dataset_id",
        "token_num": "token_count",
        "parser_id": "chunk_method"
    }
    run_mapping = {
        "0": "UNSTART",
        "1": "RUNNING",
        "2": "CANCEL",
        "3": "DONE",
        "4": "FAIL"
    }
    doc=doc.to_dict()
    renamed_doc = {}
    for key, value in doc.items():
        if key == "run":
            renamed_doc["run"] = run_mapping.get(str(value))
        new_key = key_mapping.get(key, key)
        renamed_doc[new_key] = value
    res = {"total": sres.total, "chunks": [], "doc": renamed_doc}
    origin_chunks = []
    sign = 0
    for id in sres.ids:
        d = {
            "chunk_id": id,
            "content_with_weight": rmSpace(sres.highlight[id]) if question and id in sres.highlight else sres.field[
                id].get(
                "content_with_weight", ""),
            "doc_id": sres.field[id]["doc_id"],
            "docnm_kwd": sres.field[id]["docnm_kwd"],
            "important_kwd": sres.field[id].get("important_kwd", []),
            "img_id": sres.field[id].get("img_id", ""),
            "available_int": sres.field[id].get("available_int", 1),
            "positions": sres.field[id].get("position_int", "").split("\t")
        }
        if len(d["positions"]) % 5 == 0:
            poss = []
            for i in range(0, len(d["positions"]), 5):
                poss.append([float(d["positions"][i]), float(d["positions"][i + 1]), float(d["positions"][i + 2]),
                             float(d["positions"][i + 3]), float(d["positions"][i + 4])])
            d["positions"] = poss

        origin_chunks.append(d)
        if req.get("id"):
            if req.get("id") == id:
                origin_chunks.clear()
                origin_chunks.append(d)
                sign = 1
                break
    if req.get("id"):
        if sign == 0:
            return get_error_data_result(f"Can't find this chunk {req.get('id')}")
    for chunk in origin_chunks:
        key_mapping = {
            "chunk_id": "id",
            "content_with_weight": "content",
            "doc_id": "document_id",
            "important_kwd": "important_keywords",
            "img_id": "image_id"
        }
        renamed_chunk = {}
        for key, value in chunk.items():
            new_key = key_mapping.get(key, key)
            renamed_chunk[new_key] = value
        res["chunks"].append(renamed_chunk)
    return get_result(data=res)



@manager.route('/dataset/<dataset_id>/document/<document_id>/chunk', methods=['POST'])
@token_required
def add_chunk(tenant_id,dataset_id,document_id):
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(retmsg=f"You don't own the document {document_id}.")
    doc = doc[0]
    req = request.json
    if not req.get("content"):
        return get_error_data_result(retmsg="`content` is required")
    if "important_keywords" in req:
        if type(req["important_keywords"]) != list:
            return get_error_data_result("`important_keywords` is required to be a list")
    md5 = hashlib.md5()
    md5.update((req["content"] + document_id).encode("utf-8"))

    chunk_id = md5.hexdigest()
    d = {"id": chunk_id, "content_ltks": rag_tokenizer.tokenize(req["content"]),
         "content_with_weight": req["content"]}
    d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
    d["important_kwd"] = req.get("important_keywords", [])
    d["important_tks"] = rag_tokenizer.tokenize(" ".join(req.get("important_keywords", [])))
    d["create_time"] = str(datetime.datetime.now()).replace("T", " ")[:19]
    d["create_timestamp_flt"] = datetime.datetime.now().timestamp()
    d["kb_id"] = [doc.kb_id]
    d["docnm_kwd"] = doc.name
    d["doc_id"] = doc.id
    embd_id = DocumentService.get_embd_id(document_id)
    embd_mdl = TenantLLMService.model_instance(
        tenant_id, LLMType.EMBEDDING.value, embd_id)

    v, c = embd_mdl.encode([doc.name, req["content"]])
    v = 0.1 * v[0] + 0.9 * v[1]
    d["q_%d_vec" % len(v)] = v.tolist()
    ELASTICSEARCH.upsert([d], search.index_name(tenant_id))

    DocumentService.increment_chunk_num(
        doc.id, doc.kb_id, c, 1, 0)
    d["chunk_id"] = chunk_id
    # rename keys
    key_mapping = {
        "chunk_id": "id",
        "content_with_weight": "content",
        "doc_id": "document_id",
        "important_kwd": "important_keywords",
        "kb_id": "dataset_id",
        "create_timestamp_flt": "create_timestamp",
        "create_time": "create_time",
        "document_keyword": "document",
    }
    renamed_chunk = {}
    for key, value in d.items():
        if key in key_mapping:
            new_key = key_mapping.get(key, key)
            renamed_chunk[new_key] = value
    return get_result(data={"chunk": renamed_chunk})
    # return get_result(data={"chunk_id": chunk_id})


@manager.route('dataset/<dataset_id>/document/<document_id>/chunk', methods=['DELETE'])
@token_required
def rm_chunk(tenant_id,dataset_id,document_id):
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(retmsg=f"You don't own the document {document_id}.")
    doc = doc[0]
    req = request.json
    if not req.get("chunk_ids"):
        return get_error_data_result("`chunk_ids` is required")
    query = {
        "doc_ids": [doc.id], "page": 1, "size": 1024, "question": "", "sort": True}
    sres = retrievaler.search(query, search.index_name(tenant_id), highlight=True)
    for chunk_id in req.get("chunk_ids"):
        if chunk_id not in sres.ids:
            return get_error_data_result(f"Chunk {chunk_id} not found")
    if not ELASTICSEARCH.deleteByQuery(
            Q("ids", values=req["chunk_ids"]), search.index_name(tenant_id)):
        return get_error_data_result(retmsg="Index updating failure")
    deleted_chunk_ids = req["chunk_ids"]
    chunk_number = len(deleted_chunk_ids)
    DocumentService.decrement_chunk_num(doc.id, doc.kb_id, 1, chunk_number, 0)
    return get_result()



@manager.route('/dataset/<dataset_id>/document/<document_id>/chunk/<chunk_id>', methods=['PUT'])
@token_required
def update_chunk(tenant_id,dataset_id,document_id,chunk_id):
    try:
        res = ELASTICSEARCH.get(
        chunk_id, search.index_name(
            tenant_id))
    except Exception as e:
        return get_error_data_result(f"Can't find this chunk {chunk_id}")
    if not KnowledgebaseService.query(id=dataset_id, tenant_id=tenant_id):
        return get_error_data_result(retmsg=f"You don't own the dataset {dataset_id}.")
    doc = DocumentService.query(id=document_id, kb_id=dataset_id)
    if not doc:
        return get_error_data_result(retmsg=f"You don't own the document {document_id}.")
    doc = doc[0]
    query = {
        "doc_ids": [document_id], "page": 1, "size": 1024, "question": "", "sort": True
    }
    sres = retrievaler.search(query, search.index_name(tenant_id), highlight=True)
    if chunk_id not in sres.ids:
        return get_error_data_result(f"You don't own the chunk {chunk_id}")
    req = request.json
    content=res["_source"].get("content_with_weight")
    d = {
        "id": chunk_id,
        "content_with_weight": req.get("content",content)}
    d["content_ltks"] = rag_tokenizer.tokenize(d["content_with_weight"])
    d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
    if "important_keywords" in req:
        if not isinstance(req["important_keywords"],list):
            return get_error_data_result("`important_keywords` should be a list")
        d["important_kwd"] = req.get("important_keywords")
        d["important_tks"] = rag_tokenizer.tokenize(" ".join(req["important_keywords"]))
    if "available" in req:
        d["available_int"] = int(req["available"])
    embd_id = DocumentService.get_embd_id(document_id)
    embd_mdl = TenantLLMService.model_instance(
        tenant_id, LLMType.EMBEDDING.value, embd_id)
    if doc.parser_id == ParserType.QA:
        arr = [
            t for t in re.split(
                r"[\n\t]",
                d["content_with_weight"]) if len(t) > 1]
        if len(arr) != 2:
            return get_error_data_result(
                retmsg="Q&A must be separated by TAB/ENTER key.")
        q, a = rmPrefix(arr[0]), rmPrefix(arr[1])
        d = beAdoc(d, arr[0], arr[1], not any(
            [rag_tokenizer.is_chinese(t) for t in q + a]))

    v, c = embd_mdl.encode([doc.name, d["content_with_weight"]])
    v = 0.1 * v[0] + 0.9 * v[1] if doc.parser_id != ParserType.QA else v[1]
    d["q_%d_vec" % len(v)] = v.tolist()
    ELASTICSEARCH.upsert([d], search.index_name(tenant_id))
    return get_result()



@manager.route('/retrieval', methods=['POST'])
@token_required
def retrieval_test(tenant_id):
    req = request.json
    if not req.get("dataset_ids"):
        return get_error_data_result("`datasets` is required.")
    kb_ids = req["dataset_ids"]
    if not isinstance(kb_ids,list):
        return get_error_data_result("`datasets` should be a list")
    kbs = KnowledgebaseService.get_by_ids(kb_ids)
    for id in kb_ids:
        if not KnowledgebaseService.query(id=id,tenant_id=tenant_id):
            return get_error_data_result(f"You don't own the dataset {id}.")
    embd_nms = list(set([kb.embd_id for kb in kbs]))
    if len(embd_nms) != 1:
        return get_result(
            retmsg='Datasets use different embedding models."',
            retcode=RetCode.AUTHENTICATION_ERROR)
    if "question" not in req:
        return get_error_data_result("`question` is required.")
    page = int(req.get("offset", 1))
    size = int(req.get("limit", 1024))
    question = req["question"]
    doc_ids = req.get("document_ids", [])
    if not isinstance(doc_ids,list):
        return get_error_data_result("`documents` should be a list")
    doc_ids_list=KnowledgebaseService.list_documents_by_ids(kb_ids)
    for doc_id in doc_ids:
        if doc_id not in doc_ids_list:
            return get_error_data_result(f"The datasets don't own the document {doc_id}")
    similarity_threshold = float(req.get("similarity_threshold", 0.2))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    top = int(req.get("top_k", 1024))
    if req.get("highlight")=="False" or  req.get("highlight")=="false":
        highlight = False
    else:
        highlight = True
    try:
        e, kb = KnowledgebaseService.get_by_id(kb_ids[0])
        if not e:
            return get_error_data_result(retmsg="Dataset not found!")
        embd_mdl = TenantLLMService.model_instance(
            kb.tenant_id, LLMType.EMBEDDING.value, llm_name=kb.embd_id)

        rerank_mdl = None
        if req.get("rerank_id"):
            rerank_mdl = TenantLLMService.model_instance(
                kb.tenant_id, LLMType.RERANK.value, llm_name=req["rerank_id"])

        if req.get("keyword", False):
            chat_mdl = TenantLLMService.model_instance(kb.tenant_id, LLMType.CHAT)
            question += keyword_extraction(chat_mdl, question)

        retr = retrievaler if kb.parser_id != ParserType.KG else kg_retrievaler
        ranks = retr.retrieval(question, embd_mdl, kb.tenant_id, kb_ids, page, size,
                               similarity_threshold, vector_similarity_weight, top,
                               doc_ids, rerank_mdl=rerank_mdl, highlight=highlight)
        for c in ranks["chunks"]:
            if "vector" in c:
                del c["vector"]

        ##rename keys
        renamed_chunks = []
        for chunk in ranks["chunks"]:
            key_mapping = {
                "chunk_id": "id",
                "content_with_weight": "content",
                "doc_id": "document_id",
                "important_kwd": "important_keywords",
                "docnm_kwd": "document_keyword"
            }
            rename_chunk = {}
            for key, value in chunk.items():
                new_key = key_mapping.get(key, key)
                rename_chunk[new_key] = value
            renamed_chunks.append(rename_chunk)
        ranks["chunks"] = renamed_chunks
        return get_result(data=ranks)
    except Exception as e:
        if str(e).find("not_found") > 0:
            return get_result(retmsg=f'No chunk found! Check the chunk status please!',
                                   retcode=RetCode.DATA_ERROR)
        return server_error_response(e)